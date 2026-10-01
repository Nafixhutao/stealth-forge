package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domainname"
)

func ensureWildcardDNS(ctx context.Context, client Client, zoneID, hostname, target, oldZoneID, oldHostname, storedRecordID string) (DNSRecord, bool, error) {
	var changed bool
	if storedRecordID != "" && oldZoneID == zoneID && domainname.Canonical(oldHostname) == domainname.Canonical(hostname) {
		stored, err := client.GetDNSRecord(ctx, zoneID, storedRecordID)
		if err == nil {
			if stored.ID != storedRecordID || !exactOwnedWildcard(stored, hostname, target) {
				return DNSRecord{}, false, fmt.Errorf("%w: stored record %s no longer matches %s and the Stealth tunnel", ErrRoutingConflict, storedRecordID, hostname)
			}
			if stored.Proxied && stored.TTL == 1 {
				return stored, false, nil
			}
			updated, err := client.UpdateDNSRecord(ctx, zoneID, storedRecordID, DNSRecord{Type: "CNAME", Name: hostname, Content: target, Proxied: true, TTL: 1})
			if err != nil {
				return DNSRecord{}, false, fmt.Errorf("restore wildcard DNS proxy and TTL: %w", err)
			}
			if updated.ID != storedRecordID || !exactOwnedWildcard(updated, hostname, target) || !updated.Proxied || updated.TTL != 1 {
				return DNSRecord{}, false, fmt.Errorf("%w: Cloudflare returned an unexpected wildcard DNS record after update", ErrRoutingConflict)
			}
			return updated, true, nil
		}
		if !errors.Is(err, ErrResourceNotFound) {
			return DNSRecord{}, false, fmt.Errorf("verify stored wildcard DNS identity: %w", err)
		}
	}
	records, err := client.ListDNSRecords(ctx, zoneID, hostname)
	if err != nil {
		return DNSRecord{}, false, fmt.Errorf("list wildcard DNS records: %w", err)
	}
	matching, err := findMatchingDNS(records, hostname, target)
	if err != nil {
		return DNSRecord{}, false, err
	}
	if matching.ID == "" {
		matching, err = client.CreateDNSRecord(ctx, zoneID, DNSRecord{Type: "CNAME", Name: hostname, Content: target, Proxied: true, TTL: 1})
		if err != nil {
			return DNSRecord{}, false, fmt.Errorf("create wildcard DNS record: %w", err)
		}
		if !exactOwnedWildcard(matching, hostname, target) || !matching.Proxied || matching.TTL != 1 {
			return DNSRecord{}, false, fmt.Errorf("%w: Cloudflare returned an unexpected wildcard DNS record after create", ErrRoutingConflict)
		}
		changed = true
	}
	if !matching.Proxied || matching.TTL != 1 {
		matchingID := matching.ID
		matching, err = client.UpdateDNSRecord(ctx, zoneID, matching.ID, DNSRecord{Type: "CNAME", Name: hostname, Content: target, Proxied: true, TTL: 1})
		if err != nil {
			return DNSRecord{}, false, fmt.Errorf("configure wildcard DNS record: %w", err)
		}
		if matching.ID != matchingID || !exactOwnedWildcard(matching, hostname, target) || !matching.Proxied || matching.TTL != 1 {
			return DNSRecord{}, false, fmt.Errorf("%w: Cloudflare returned an unexpected wildcard DNS record after update", ErrRoutingConflict)
		}
		changed = true
	}
	return matching, changed, nil
}

func findMatchingDNS(records []DNSRecord, hostname, target string) (DNSRecord, error) {
	hostname, target = domainname.Canonical(hostname), domainname.Canonical(target)
	var matching DNSRecord
	for _, record := range records {
		if domainname.Canonical(record.Name) != hostname {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(record.Type)) != "CNAME" {
			return DNSRecord{}, fmt.Errorf("%w: %s already has an incompatible record type", ErrRoutingConflict, hostname)
		}
		if domainname.Canonical(record.Content) != target {
			return DNSRecord{}, fmt.Errorf("%w: %s already points to another CNAME target", ErrRoutingConflict, hostname)
		}
		if strings.TrimSpace(record.ID) == "" {
			return DNSRecord{}, errors.New("Cloudflare returned an incomplete wildcard DNS record identity")
		}
		if matching.ID != "" {
			return DNSRecord{}, fmt.Errorf("%w: multiple CNAME records exist for %s", ErrRoutingConflict, hostname)
		}
		matching = record
	}
	return matching, nil
}

func exactOwnedWildcard(record DNSRecord, hostname, target string) bool {
	return strings.TrimSpace(record.ID) != "" && strings.EqualFold(strings.TrimSpace(record.Type), "CNAME") && domainname.Canonical(record.Name) == domainname.Canonical(hostname) && domainname.Canonical(record.Content) == domainname.Canonical(target)
}
