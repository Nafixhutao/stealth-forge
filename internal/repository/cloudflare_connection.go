package repository

import (
	"context"
	"errors"
	"sync"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrCloudflareConnectionUnavailable = errors.New("Cloudflare connection is unavailable")
	ErrCloudflareConnectionConflict    = errors.New("Cloudflare connection conflicts with the existing tunnel")
	ErrCloudflareIdentityRequired      = errors.New("Cloudflare tunnel identity is required before reconnecting")
)

const cloudflareReconcileLockID int64 = 8_105_202_603

type CloudflareConnectionInput struct {
	AccountID       string
	ConsoleZoneID   string
	ConsoleHostname string
	TunnelID        string
	TunnelName      string
	ConsoleRecordID string
	APIToken        string
}

// TryCloudflareReconcileLock holds one connection-scoped PostgreSQL advisory
// lock across provider HTTP work. A second worker skips rather than racing the
// tunnel configuration or wildcard record state.
func (r *Repository) TryCloudflareReconcileLock(ctx context.Context) (func() error, bool, error) {
	if r == nil || r.pool == nil {
		return nil, false, ErrNotFound
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, cloudflareReconcileLockID).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			_, releaseErr = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, cloudflareReconcileLockID)
			conn.Release()
		})
		return releaseErr
	}, true, nil
}

// CloudflareConnectionDetails returns only non-secret provider identity and
// the authoritative workload domain. API handlers use it to validate a
// replacement token without loading the existing credential.
func (r *Repository) CloudflareConnectionDetails(ctx context.Context) (domain.CloudflareConnection, error) {
	return r.readCloudflareConnection(ctx, false)
}

// CloudflareReconcileSnapshot is the worker-only credential read. The token
// is decrypted in memory and is excluded from all JSON serialization.
func (r *Repository) CloudflareReconcileSnapshot(ctx context.Context) (domain.CloudflareConnection, error) {
	return r.readCloudflareConnection(ctx, true)
}

func (r *Repository) readCloudflareConnection(ctx context.Context, decrypt bool) (domain.CloudflareConnection, error) {
	if r == nil || r.pool == nil {
		return domain.CloudflareConnection{}, ErrNotFound
	}
	var connection domain.CloudflareConnection
	var ciphertext []byte
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(c.account_id,''),COALESCE(c.console_zone_id,''),COALESCE(c.console_hostname,''),COALESCE(c.tunnel_id,''),COALESCE(c.tunnel_name,''),COALESCE(c.console_record_id,''),
		       c.api_token_ciphertext,COALESCE(c.workload_zone_id,''),COALESCE(c.workload_zone_name,''),COALESCE(c.wildcard_hostname,''),COALESCE(c.wildcard_record_id,''),
		       c.status,c.edge_tls_status,COALESCE(c.edge_tls_error,''),c.console_origin_desired,c.console_origin_observed,c.console_origin_status,
		       COALESCE(c.console_origin_last_error,''),c.console_origin_updated_at,c.console_public_verified_at,COALESCE(c.console_public_verified_origin,''),
		       c.last_reconciled_at,COALESCE(c.last_error,''),c.configured_at,c.updated_at,d.workload_base_domain
		FROM cloudflare_connections c
	LEFT JOIN instance_domain_settings d ON d.id=TRUE
	WHERE c.id=TRUE`).Scan(
		&connection.AccountID, &connection.ConsoleZoneID, &connection.ConsoleHostname, &connection.TunnelID, &connection.TunnelName, &connection.ConsoleRecordID,
		&ciphertext, &connection.WorkloadZoneID, &connection.WorkloadZoneName, &connection.WildcardHostname, &connection.WildcardRecordID,
		&connection.Status, &connection.EdgeTLSStatus, &connection.EdgeTLSError, &connection.ConsoleOriginDesired, &connection.ConsoleOriginObserved, &connection.ConsoleOriginStatus,
		&connection.ConsoleOriginLastError, &connection.ConsoleOriginUpdatedAt, &connection.ConsolePublicVerifiedAt, &connection.ConsolePublicVerifiedOrigin,
		&connection.LastReconciledAt, &connection.LastError, &connection.ConfiguredAt, &connection.UpdatedAt, &connection.WorkloadBaseDomain)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CloudflareConnection{}, ErrNotFound
	}
	if err != nil {
		return domain.CloudflareConnection{}, err
	}
	if decrypt && len(ciphertext) > 0 {
		if r.cloudflareCipher == nil {
			return domain.CloudflareConnection{}, ErrCloudflareConnectionUnavailable
		}
		plaintext, err := r.cloudflareCipher.Decrypt(ciphertext)
		if err != nil {
			return domain.CloudflareConnection{}, errors.New("Cloudflare credential decryption failed")
		}
		connection.APIToken = string(plaintext)
	}
	return connection, nil
}

func (r *Repository) CloudflareRoutingStatus(ctx context.Context) (domain.CloudflareRoutingStatus, error) {
	connection, err := r.CloudflareConnectionDetails(ctx)
	if err != nil {
		return domain.CloudflareRoutingStatus{}, err
	}
	var configured bool
	if err := r.pool.QueryRow(ctx, `SELECT api_token_ciphertext IS NOT NULL AND octet_length(api_token_ciphertext)>0 FROM cloudflare_connections WHERE id=TRUE`).Scan(&configured); err != nil {
		return domain.CloudflareRoutingStatus{}, err
	}
	status := connection.Status
	if status == "" {
		status = "unconfigured"
	}
	edgeTLSStatus := connection.EdgeTLSStatus
	if edgeTLSStatus == "" {
		edgeTLSStatus = "not_applicable"
	}
	var hostname *string
	if connection.WorkloadBaseDomain != nil {
		value := "*." + *connection.WorkloadBaseDomain
		hostname = &value
	}
	return domain.CloudflareRoutingStatus{
		Configured:                  configured,
		Status:                      status,
		ConsoleHostname:             connection.ConsoleHostname,
		WorkloadHostname:            hostname,
		Zone:                        connection.WorkloadZoneName,
		EdgeTLSStatus:               edgeTLSStatus,
		EdgeTLSError:                connection.EdgeTLSError,
		ConsoleOriginDesired:        connection.ConsoleOriginDesired,
		ConsoleOriginObserved:       connection.ConsoleOriginObserved,
		ConsoleOriginStatus:         connection.ConsoleOriginStatus,
		ConsoleOriginLastError:      connection.ConsoleOriginLastError,
		ConsoleOriginUpdatedAt:      connection.ConsoleOriginUpdatedAt,
		ConsolePublicVerifiedAt:     connection.ConsolePublicVerifiedAt,
		ConsolePublicVerifiedOrigin: connection.ConsolePublicVerifiedOrigin,
		LastReconciledAt:            connection.LastReconciledAt,
		LastError:                   connection.LastError,
	}, nil
}

// SaveCloudflareConnectionFromSetup persists the setup API's server-held
// plaintext token only after named tunnel and Console DNS provisioning have
// completed. A retry with the same identity is a no-op; setup cannot replace a
// newer production connection.
func (r *Repository) SaveCloudflareConnectionFromSetup(ctx context.Context, input CloudflareConnectionInput) error {
	return r.saveCloudflareConnection(ctx, uuid.Nil, input, false)
}

// ImportCloudflareConnectionOnce bridges a completed encrypted setup state to
// PostgreSQL. It never replaces a row that already has a credential or a
// different identity and can persist an identity without a recoverable token
// so an Instance Owner can reconnect explicitly.
func (r *Repository) ImportCloudflareConnectionOnce(ctx context.Context, input CloudflareConnectionInput, unavailableReason string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, ErrNotFound
	}
	if err := validateCloudflareConnectionInput(input, input.APIToken != ""); err != nil {
		return false, err
	}
	var ciphertext []byte
	if input.APIToken != "" {
		if r.cloudflareCipher == nil {
			return false, ErrCloudflareConnectionUnavailable
		}
		var err error
		ciphertext, err = r.cloudflareCipher.Encrypt([]byte(input.APIToken))
		if err != nil {
			return false, err
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var existingToken []byte
	var existingAccount, existingZone, existingHostname, existingTunnel string
	if err := tx.QueryRow(ctx, `SELECT api_token_ciphertext,COALESCE(account_id,''),COALESCE(console_zone_id,''),COALESCE(console_hostname,''),COALESCE(tunnel_id,'') FROM cloudflare_connections WHERE id=TRUE FOR UPDATE`).Scan(&existingToken, &existingAccount, &existingZone, &existingHostname, &existingTunnel); err != nil {
		return false, err
	}
	if len(existingToken) > 0 || existingTunnel != "" {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	status := "pending"
	lastError := any(nil)
	if len(ciphertext) == 0 {
		status = "error"
		lastError = normalizeCloudflareError(unavailableReason)
	}
	_, err = tx.Exec(ctx, `
		UPDATE cloudflare_connections
		SET account_id=$1,console_zone_id=$2,console_hostname=$3,tunnel_id=$4,tunnel_name=$5,console_record_id=$6,
		    api_token_ciphertext=$7,status=$8,last_error=$9,configured_at=CASE WHEN $7::bytea IS NULL THEN configured_at ELSE now() END,
		    edge_tls_status=CASE WHEN $7::bytea IS NOT NULL AND (SELECT workload_base_domain FROM instance_domain_settings WHERE id=TRUE) IS NOT NULL THEN 'pending' ELSE 'not_applicable' END,
		    edge_tls_error=NULL,
		    updated_at=now()
		WHERE id=TRUE`, input.AccountID, input.ConsoleZoneID, input.ConsoleHostname, input.TunnelID, input.TunnelName, input.ConsoleRecordID, nullableBytes(ciphertext), status, lastError)
	if err != nil {
		return false, err
	}
	if err := writeSystemCloudflareAuditTx(ctx, tx, "admin.cloudflare.connection.import", map[string]any{
		"configured": len(ciphertext) > 0, "credential_recovered": len(ciphertext) > 0,
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// MarkCloudflareConnectionUnavailable leaves an unconfigured status with an
// actionable, bounded reason when a legacy setup credential cannot be read.
func (r *Repository) MarkCloudflareConnectionUnavailable(ctx context.Context, reason string) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `UPDATE cloudflare_connections SET status='error',last_error=$1,updated_at=now() WHERE id=TRUE AND api_token_ciphertext IS NULL`, normalizeCloudflareError(reason))
	return err
}
