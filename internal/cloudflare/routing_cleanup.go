package cloudflare

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domainname"
)

func (r *Reconciler) cleanupRetiring(ctx context.Context, client Client) (bool, error) {
	retiring, err := r.store.ListRetiringCloudflareWildcardDNS(ctx)
	if err != nil {
		return false, fmt.Errorf("list retiring wildcard records: %w", err)
	}
	changed := false
	for _, item := range retiring {
		record, err := client.GetDNSRecord(ctx, item.ZoneID, item.RecordID)
		if errors.Is(err, ErrResourceNotFound) {
			if err := r.store.CompleteCloudflareWildcardRetirement(ctx, item.RecordID); err != nil {
				return changed, fmt.Errorf("complete missing wildcard cleanup record: %w", err)
			}
			changed = true
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("revalidate old wildcard DNS record: %w", err)
		}
		if record.ID != item.RecordID || !exactOwnedWildcard(record, item.Hostname, item.Target) {
			return changed, fmt.Errorf("%w: refusing to delete stored record %s because its hostname or target changed", ErrRoutingConflict, item.RecordID)
		}
		if err := client.DeleteDNSRecord(ctx, item.ZoneID, item.RecordID); err != nil && !errors.Is(err, ErrResourceNotFound) {
			return changed, fmt.Errorf("delete revalidated old wildcard DNS record: %w", err)
		}
		if err := r.store.CompleteCloudflareWildcardRetirement(ctx, item.RecordID); err != nil {
			return changed, fmt.Errorf("complete wildcard cleanup record: %w", err)
		}
		changed = true
		r.logger.Info("cloudflare wildcard DNS deleted", "hostname", item.Hostname, "zone_id", item.ZoneID)
	}
	return changed, nil
}

func longestContainingZone(zones []Zone, hostname string) (Zone, error) {
	hostname = domainname.Canonical(hostname)
	var matches []Zone
	longest := 0
	for _, zone := range zones {
		zoneName, err := domainname.NormalizeDomain(zone.Name)
		if err != nil || !zoneContainsName(zoneName, hostname) {
			continue
		}
		if len(zoneName) > longest {
			matches = matches[:0]
			longest = len(zoneName)
		}
		if len(zoneName) == longest {
			matches = append(matches, Zone{ID: zone.ID, Name: zoneName, Status: zone.Status})
		}
	}
	if len(matches) == 0 {
		return Zone{}, errors.New("Cloudflare token cannot access a DNS zone containing the workload domain; add Zone Read and DNS Edit access for its zone")
	}
	slices.SortFunc(matches, func(a, b Zone) int { return cmp.Compare(a.ID, b.ID) })
	if len(matches) > 1 && matches[0].Name == matches[1].Name {
		return Zone{}, errors.New("Cloudflare returned multiple equally specific zones for the workload domain")
	}
	return matches[0], nil
}

func zoneContainsName(zoneName, hostname string) bool {
	zoneName = domainname.Canonical(zoneName)
	hostname = domainname.Canonical(hostname)
	return zoneName != "" && (hostname == zoneName || strings.HasSuffix(hostname, "."+zoneName))
}

func containsAccount(accounts []Account, id string) bool {
	for _, account := range accounts {
		if strings.TrimSpace(account.ID) == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

func workloadOrigin(hostname string) string {
	if hostname == "" {
		return ""
	}
	return "http://traefik:8080"
}

func (r *Reconciler) recordFailure(ctx context.Context, token string, err error) error {
	message := safeReconcileError(err, token)
	if persistErr := r.store.RecordCloudflareReconcileFailure(ctx, message); persistErr != nil {
		return errors.Join(fmt.Errorf("%s", message), fmt.Errorf("persist Cloudflare error state: %w", persistErr))
	}
	if errors.Is(err, ErrRoutingConflict) {
		r.logger.Error("cloudflare reconcile conflict", "error", message)
		return fmt.Errorf("%w: %s", ErrRoutingConflict, message)
	}
	r.logger.Error("cloudflare provider unavailable", "error", message)
	if errors.Is(err, ErrUnauthorized) {
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	}
	return errors.New(message)
}

func (r *Reconciler) recordConsoleOriginFailure(ctx context.Context, token, expected string, err error) error {
	message := safeReconcileError(err, token)
	if persistErr := r.store.RecordCloudflareConsoleOriginFailure(ctx, expected, message); persistErr != nil {
		return errors.Join(errors.New(message), fmt.Errorf("persist Cloudflare Console-origin error state: %w", persistErr))
	}
	r.logger.Error("cloudflare Console-origin reconcile failed", "error", message)
	if errors.Is(err, ErrUnauthorized) {
		return fmt.Errorf("%w: %s", ErrUnauthorized, message)
	}
	return errors.New(message)
}

func safeReconcileError(err error, token string) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrUnauthorized) {
		return "Cloudflare token was rejected or lacks required permissions; reconnect a scoped token with access to the account and required zones"
	}
	message := strings.TrimSpace(err.Error())
	if token != "" {
		message = strings.ReplaceAll(message, token, "[redacted]")
	}
	message = strings.Map(func(r rune) rune {
		if r == '\x00' || r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, message)
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
