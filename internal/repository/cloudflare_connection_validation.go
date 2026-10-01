package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/domainname"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func validateCloudflareConnectionInput(input CloudflareConnectionInput, requireToken bool) error {
	fields := []struct{ value, label string }{
		{input.AccountID, "account"}, {input.ConsoleZoneID, "console zone"}, {input.TunnelID, "tunnel"},
		{input.TunnelName, "tunnel name"}, {input.ConsoleRecordID, "console DNS record"},
	}
	for _, field := range fields {
		value := strings.TrimSpace(field.value)
		if value == "" || len(value) > 255 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("Cloudflare %s identity is invalid", field.label)
		}
	}
	hostname, err := domainname.NormalizeHostname(input.ConsoleHostname)
	if err != nil || hostname != domainname.Canonical(input.ConsoleHostname) {
		return errors.New("Cloudflare Console hostname is invalid")
	}
	if requireToken && (strings.TrimSpace(input.APIToken) == "" || len(input.APIToken) > 4096 || strings.ContainsAny(input.APIToken, "\x00\r\n")) {
		return errors.New("Cloudflare API token is invalid")
	}
	return nil
}

func validateCloudflareObserved(update domain.CloudflareRoutingUpdate) error {
	if (update.ConsoleOriginDesired != "proxy" && update.ConsoleOriginDesired != "traefik") || update.ConsoleOriginObserved != update.ConsoleOriginDesired {
		return errors.New("Cloudflare Console origin observation is invalid")
	}
	if update.ExpectedWorkloadBaseDomain != nil {
		domain, err := domainname.NormalizeDomain(*update.ExpectedWorkloadBaseDomain)
		if err != nil || domain != *update.ExpectedWorkloadBaseDomain {
			return errors.New("Cloudflare workload domain is invalid")
		}
		expected := "*." + domain
		if update.WildcardHostname != expected || update.WorkloadZoneID == "" || update.WorkloadZoneName == "" || update.WildcardRecordID == "" {
			return errors.New("Cloudflare wildcard observation is incomplete")
		}
		switch update.EdgeTLSStatus {
		case "pending", "ready", "action_required", "error":
		default:
			return errors.New("Cloudflare workload edge TLS observation is incomplete")
		}
	} else if update.WorkloadZoneID != "" || update.WorkloadZoneName != "" || update.WildcardHostname != "" || update.WildcardRecordID != "" {
		return errors.New("Cloudflare clear observation must not contain wildcard resources")
	} else if update.EdgeTLSStatus != "not_applicable" {
		return errors.New("Cloudflare clear observation must mark edge TLS not applicable")
	}
	if len(update.EdgeTLSError) > 512 || strings.ContainsAny(update.EdgeTLSError, "\x00\r\n") {
		return errors.New("Cloudflare edge TLS reason is invalid")
	}
	return nil
}

func sameCloudflareIdentity(existing domain.CloudflareConnection, input CloudflareConnectionInput) bool {
	return existing.AccountID == input.AccountID && existing.ConsoleZoneID == input.ConsoleZoneID && existing.ConsoleHostname == input.ConsoleHostname && existing.TunnelID == input.TunnelID && existing.TunnelName == input.TunnelName
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func queueRetiringWildcardTx(ctx context.Context, tx pgx.Tx, zoneID, hostname, recordID, target string) error {
	if zoneID == "" || hostname == "" || recordID == "" || target == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO cloudflare_retiring_wildcard_dns (record_id,zone_id,hostname,target) VALUES ($1,$2,$3,$4) ON CONFLICT (record_id) DO NOTHING`, recordID, zoneID, hostname, target)
	return err
}

func normalizeCloudflareError(value string) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\x00' || r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, value))
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "�")
	}
	if len(value) > 512 {
		value = value[:512]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value
}

func writeSystemCloudflareAuditTx(ctx context.Context, tx pgx.Tx, action string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (id,organization_id,actor_account_id,action,target_type,target_id,metadata) VALUES ($1,NULL,NULL,$2,'cloudflare_connection',NULL,$3)`, id, action, encoded); err != nil {
		return err
	}
	return enqueueAdminRealtimeEventTx(ctx, tx, action, "cloudflare_connection", uuid.Nil, nil)
}
