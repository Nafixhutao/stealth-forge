package repository

import (
	"context"
	"errors"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) saveCloudflareConnection(ctx context.Context, actor uuid.UUID, input CloudflareConnectionInput, requireOwner bool) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	if err := validateCloudflareConnectionInput(input, true); err != nil {
		return err
	}
	if r.cloudflareCipher == nil {
		return ErrCloudflareConnectionUnavailable
	}
	ciphertext, err := r.cloudflareCipher.Encrypt([]byte(input.APIToken))
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if requireOwner {
		if err := requireInstanceOwnerTx(ctx, tx, actor); err != nil {
			return err
		}
	}
	var existing domain.CloudflareConnection
	var existingToken []byte
	if err := tx.QueryRow(ctx, `SELECT COALESCE(account_id,''),COALESCE(console_zone_id,''),COALESCE(console_hostname,''),COALESCE(tunnel_id,''),COALESCE(tunnel_name,''),COALESCE(console_record_id,''),api_token_ciphertext FROM cloudflare_connections WHERE id=TRUE FOR UPDATE`).Scan(
		&existing.AccountID, &existing.ConsoleZoneID, &existing.ConsoleHostname, &existing.TunnelID, &existing.TunnelName, &existing.ConsoleRecordID, &existingToken); err != nil {
		return err
	}
	if len(existingToken) > 0 {
		if !sameCloudflareIdentity(existing, input) {
			return ErrCloudflareConnectionConflict
		}
		if !requireOwner {
			return tx.Commit(ctx)
		}
		if _, err := tx.Exec(ctx, `UPDATE cloudflare_connections SET api_token_ciphertext=$1,console_record_id=$2,status='pending',last_error=NULL,
			edge_tls_status=CASE WHEN (SELECT workload_base_domain FROM instance_domain_settings WHERE id=TRUE) IS NULL THEN 'not_applicable' ELSE 'pending' END,
			edge_tls_error=NULL,console_origin_status='pending',console_origin_last_error=NULL,console_origin_updated_at=now(),
			console_public_verified_at=NULL,console_public_verified_origin=NULL,updated_at=now() WHERE id=TRUE`, ciphertext, input.ConsoleRecordID); err != nil {
			return err
		}
		if err := writeInstanceAuditTx(ctx, tx, actor, "admin.cloudflare.connection.update", "cloudflare_connection", uuid.Nil, map[string]any{"credential_replaced": true}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if existing.TunnelID != "" && !sameCloudflareIdentity(existing, input) {
		return ErrCloudflareConnectionConflict
	}
	_, err = tx.Exec(ctx, `
		UPDATE cloudflare_connections
		SET account_id=$1,console_zone_id=$2,console_hostname=$3,tunnel_id=$4,tunnel_name=$5,console_record_id=$6,
		    api_token_ciphertext=$7,status='pending',last_error=NULL,configured_at=COALESCE(configured_at,now()),
		    edge_tls_status=CASE WHEN (SELECT workload_base_domain FROM instance_domain_settings WHERE id=TRUE) IS NULL THEN 'not_applicable' ELSE 'pending' END,
		    edge_tls_error=NULL,console_origin_status='pending',console_origin_last_error=NULL,console_origin_updated_at=now(),
		    console_public_verified_at=NULL,console_public_verified_origin=NULL,updated_at=now()
		WHERE id=TRUE`, input.AccountID, input.ConsoleZoneID, input.ConsoleHostname, input.TunnelID, input.TunnelName, input.ConsoleRecordID, ciphertext)
	if err != nil {
		return err
	}
	if requireOwner {
		if err := writeInstanceAuditTx(ctx, tx, actor, "admin.cloudflare.connection.update", "cloudflare_connection", uuid.Nil, map[string]any{"credential_replaced": false, "configured": true}); err != nil {
			return err
		}
	} else if err := writeSystemCloudflareAuditTx(ctx, tx, "admin.cloudflare.connection.established", map[string]any{"configured": true}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetCloudflareConnectionByOwner establishes or reconnects the existing tunnel
// under live Instance Owner authorization. It never creates provider
// resources; the API adapter validates the identity and token first.
func (r *Repository) SetCloudflareConnectionByOwner(ctx context.Context, actor uuid.UUID, input CloudflareConnectionInput) error {
	return r.saveCloudflareConnection(ctx, actor, input, true)
}

func (r *Repository) CompleteCloudflareReconcile(ctx context.Context, update domain.CloudflareRoutingUpdate) (bool, error) {
	if r == nil || r.pool == nil {
		return false, ErrNotFound
	}
	if err := validateCloudflareObserved(update); err != nil {
		return false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var currentDomain *string
	if err := tx.QueryRow(ctx, `SELECT workload_base_domain FROM instance_domain_settings WHERE id=TRUE`).Scan(&currentDomain); err != nil {
		return false, err
	}
	var accountID, tunnelID, oldZoneID, oldHostname, oldRecordID, currentConsoleOrigin string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(account_id,''),COALESCE(tunnel_id,''),COALESCE(workload_zone_id,''),COALESCE(wildcard_hostname,''),COALESCE(wildcard_record_id,''),console_origin_desired FROM cloudflare_connections WHERE id=TRUE AND api_token_ciphertext IS NOT NULL FOR UPDATE`).Scan(&accountID, &tunnelID, &oldZoneID, &oldHostname, &oldRecordID, &currentConsoleOrigin); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrCloudflareIdentityRequired
	} else if err != nil {
		return false, err
	}
	if currentConsoleOrigin != update.ConsoleOriginDesired {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if !sameOptionalString(currentDomain, update.ExpectedWorkloadBaseDomain) {
		if update.WildcardRecordID != "" && update.WildcardRecordID != oldRecordID {
			if err := queueRetiringWildcardTx(ctx, tx, update.WorkloadZoneID, update.WildcardHostname, update.WildcardRecordID, tunnelID+".cfargotunnel.com"); err != nil {
				return false, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE cloudflare_connections SET status='pending',last_error=NULL,
			edge_tls_status=CASE WHEN $1::text IS NULL THEN 'not_applicable' ELSE 'pending' END,
			edge_tls_error=NULL,console_origin_observed=$2,console_origin_status='ready',console_origin_last_error=NULL,
			console_origin_updated_at=now(),updated_at=now() WHERE id=TRUE`, nullableDomainValue(currentDomain), update.ConsoleOriginObserved); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	newHost := update.WildcardHostname
	if newHost != "" && (oldRecordID != update.WildcardRecordID || oldHostname != newHost || oldZoneID != update.WorkloadZoneID) && oldRecordID != "" {
		if err := queueRetiringWildcardTx(ctx, tx, oldZoneID, oldHostname, oldRecordID, tunnelID+".cfargotunnel.com"); err != nil {
			return false, err
		}
	}
	if newHost == "" && oldRecordID != "" {
		if err := queueRetiringWildcardTx(ctx, tx, oldZoneID, oldHostname, oldRecordID, tunnelID+".cfargotunnel.com"); err != nil {
			return false, err
		}
	}
	overallStatus := "ready"
	if update.ExpectedWorkloadBaseDomain != nil {
		switch update.EdgeTLSStatus {
		case "pending":
			overallStatus = "pending"
		case "action_required", "error":
			overallStatus = "error"
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE cloudflare_connections
		SET workload_zone_id=NULLIF($1,''),workload_zone_name=NULLIF($2,''),wildcard_hostname=NULLIF($3,''),wildcard_record_id=NULLIF($4,''),
		    status=$5,edge_tls_status=$6,edge_tls_error=NULLIF($7,''),last_error=NULL,last_reconciled_at=now(),
		    console_origin_observed=$8,console_origin_status='ready',console_origin_last_error=NULL,console_origin_updated_at=now(),updated_at=now()
		WHERE id=TRUE`, update.WorkloadZoneID, update.WorkloadZoneName, update.WildcardHostname, update.WildcardRecordID, overallStatus, update.EdgeTLSStatus, update.EdgeTLSError, update.ConsoleOriginObserved)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) ListRetiringCloudflareWildcardDNS(ctx context.Context) ([]domain.CloudflareRetiringWildcardDNS, error) {
	if r == nil || r.pool == nil {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT record_id,zone_id,hostname,target FROM cloudflare_retiring_wildcard_dns ORDER BY queued_at,record_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.CloudflareRetiringWildcardDNS, 0)
	for rows.Next() {
		var item domain.CloudflareRetiringWildcardDNS
		if err := rows.Scan(&item.RecordID, &item.ZoneID, &item.Hostname, &item.Target); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CompleteCloudflareWildcardRetirement(ctx context.Context, recordID string) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM cloudflare_retiring_wildcard_dns WHERE record_id=$1`, recordID)
	return err
}

func (r *Repository) RecordCloudflareReconcileFailure(ctx context.Context, message string) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections SET status='error',last_error=$1,updated_at=now() WHERE id=TRUE`, normalizeCloudflareError(message))
	return err
}

// CompleteCloudflareConsoleOrigin persists only the independently observed
// Tunnel Console origin. The desired-state predicate prevents an older
// provider operation from overwriting a newer host request.
func (r *Repository) CompleteCloudflareConsoleOrigin(ctx context.Context, expectedOrigin, observedOrigin string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, ErrNotFound
	}
	if (expectedOrigin != "proxy" && expectedOrigin != "traefik") || observedOrigin != expectedOrigin {
		return false, errors.New("Cloudflare Console-origin observation is invalid")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections
		SET console_origin_observed=$2,console_origin_status='ready',console_origin_last_error=NULL,
		    console_public_verified_at=CASE WHEN console_origin_observed IS DISTINCT FROM $2 THEN NULL ELSE console_public_verified_at END,
		    console_public_verified_origin=CASE WHEN console_origin_observed IS DISTINCT FROM $2 THEN NULL ELSE console_public_verified_origin END,
		    console_origin_updated_at=now(),updated_at=now()
		WHERE id=TRUE AND console_origin_desired=$1 AND api_token_ciphertext IS NOT NULL
		  AND octet_length(api_token_ciphertext)>0 AND account_id IS NOT NULL AND tunnel_id IS NOT NULL`, expectedOrigin, observedOrigin)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RecordCloudflareConsoleOriginFailure leaves workload DNS/TLS status alone.
func (r *Repository) RecordCloudflareConsoleOriginFailure(ctx context.Context, expectedOrigin, message string) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections
		SET console_origin_status='error',console_origin_last_error=$2,
		    console_origin_updated_at=now(),console_public_verified_at=NULL,
		    console_public_verified_origin=NULL,updated_at=now()
		WHERE id=TRUE AND console_origin_desired=$1`, expectedOrigin, normalizeCloudflareError(message))
	return err
}

// SetCloudflareConsoleOriginDesired records a host-authorized durable request.
// The regular worker remains responsible for provider side effects.
func (r *Repository) SetCloudflareConsoleOriginDesired(ctx context.Context, origin, action string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, ErrNotFound
	}
	if origin != "proxy" && origin != "traefik" {
		return false, errors.New("Console origin must be proxy or traefik")
	}
	if action != "cutover" && action != "rollback" {
		return false, errors.New("Console origin action is invalid")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var current string
	var configured bool
	if err := tx.QueryRow(ctx, `SELECT console_origin_desired,api_token_ciphertext IS NOT NULL AND octet_length(api_token_ciphertext)>0 AND account_id IS NOT NULL AND tunnel_id IS NOT NULL FROM cloudflare_connections WHERE id=TRUE FOR UPDATE`).Scan(&current, &configured); err != nil {
		return false, err
	}
	if !configured {
		return false, ErrCloudflareConnectionUnavailable
	}
	if current == origin {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE cloudflare_connections SET console_origin_desired=$1,console_origin_status='pending',console_origin_last_error=NULL,
		console_origin_updated_at=now(),console_public_verified_at=NULL,console_public_verified_origin=NULL,updated_at=now() WHERE id=TRUE`, origin); err != nil {
		return false, err
	}
	auditAction := "admin.cloudflare.console_origin.cutover"
	if action == "rollback" {
		auditAction = "admin.cloudflare.console_origin.rollback"
	}
	if err := writeSystemCloudflareAuditTx(ctx, tx, auditAction, map[string]any{
		"previous_origin": current,
		"desired_origin":  origin,
		"source":          "host_cli",
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// RecordCloudflareConsoleOriginVerification persists public HTTPS evidence
// only while desired and provider-observed origins still match the verified
// origin. It never stores response bodies or secrets.
func (r *Repository) RecordCloudflareConsoleOriginVerification(ctx context.Context, origin string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, ErrNotFound
	}
	if origin != "proxy" && origin != "traefik" {
		return false, errors.New("Console origin is invalid")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections
		SET console_public_verified_at=now(),console_public_verified_origin=$1,console_origin_last_error=NULL
		WHERE id=TRUE AND console_origin_desired=$1 AND console_origin_observed=$1 AND console_origin_status='ready'`, origin)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) MarkCloudflareRoutingPending(ctx context.Context) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections SET status='pending',last_error=NULL,
		edge_tls_status=CASE WHEN (SELECT workload_base_domain FROM instance_domain_settings WHERE id=TRUE) IS NULL THEN 'not_applicable' ELSE 'pending' END,
		edge_tls_error=NULL,updated_at=now() WHERE id=TRUE AND api_token_ciphertext IS NOT NULL`)
	return err
}
