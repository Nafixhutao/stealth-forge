package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/domainname"
)

func ValidateExistingTunnel(ctx context.Context, client Client, accountID, zoneID, consoleHostname, tunnelID, expectedTunnelName string, workloadBaseDomain *string) (domain.CloudflareConnection, error) {
	if client == nil {
		return domain.CloudflareConnection{}, errors.New("Cloudflare client is unavailable")
	}
	accountID = strings.TrimSpace(accountID)
	tunnelID = strings.TrimSpace(tunnelID)
	consoleHostname, err := domainname.NormalizeHostname(consoleHostname)
	if err != nil || accountID == "" || tunnelID == "" {
		return domain.CloudflareConnection{}, errors.New("Cloudflare account, tunnel, and Console hostname are required")
	}
	accounts, err := client.ListAccounts(ctx)
	if err != nil {
		return domain.CloudflareConnection{}, err
	}
	if !containsAccount(accounts, accountID) {
		return domain.CloudflareConnection{}, errors.New("Cloudflare token cannot access the configured account")
	}
	zones, err := client.ListZones(ctx, accountID)
	if err != nil {
		return domain.CloudflareConnection{}, err
	}
	var consoleZone Zone
	if zoneID != "" {
		for _, zone := range zones {
			if zone.ID == zoneID && zoneContainsName(zone.Name, consoleHostname) {
				consoleZone = zone
				break
			}
		}
		if consoleZone.ID == "" {
			return domain.CloudflareConnection{}, errors.New("Cloudflare token cannot read the configured Console zone")
		}
	} else {
		consoleZone, err = longestContainingZone(zones, consoleHostname)
		if err != nil {
			return domain.CloudflareConnection{}, fmt.Errorf("Console zone discovery failed: %w", err)
		}
	}
	status, err := client.TunnelStatus(ctx, accountID, tunnelID)
	if err != nil {
		return domain.CloudflareConnection{}, err
	}
	if status.ID != "" && status.ID != tunnelID {
		return domain.CloudflareConnection{}, errors.New("Cloudflare returned a different tunnel identity")
	}
	tunnelName := strings.TrimSpace(status.Name)
	if expectedTunnelName != "" && tunnelName != "" && tunnelName != expectedTunnelName {
		return domain.CloudflareConnection{}, errors.New("Cloudflare tunnel name does not match the saved connection")
	}
	if tunnelName == "" {
		tunnelName = strings.TrimSpace(expectedTunnelName)
	}
	if tunnelName == "" {
		return domain.CloudflareConnection{}, errors.New("Cloudflare did not return the existing tunnel name")
	}
	ingress, err := client.TunnelConfiguration(ctx, accountID, tunnelID)
	if err != nil {
		return domain.CloudflareConnection{}, err
	}
	if !hasConsoleRouteAndCatchAll(ingress, consoleHostname) {
		return domain.CloudflareConnection{}, errors.New("existing tunnel does not contain a supported Console origin and 404 catch-all")
	}
	consoleRecords, err := client.ListDNSRecords(ctx, consoleZone.ID, consoleHostname)
	if err != nil {
		return domain.CloudflareConnection{}, validationProviderError("Cloudflare token cannot read the Console DNS record", err)
	}
	consoleRecord, err := findMatchingDNS(consoleRecords, consoleHostname, tunnelID+"."+tunnelServiceDomain)
	if err != nil {
		return domain.CloudflareConnection{}, fmt.Errorf("Console DNS validation failed: %w", err)
	}
	if workloadBaseDomain != nil {
		workloadDomain, normalizeErr := domainname.NormalizeDomain(*workloadBaseDomain)
		if normalizeErr != nil {
			return domain.CloudflareConnection{}, errors.New("workload domain is invalid")
		}
		workloadZone, zoneErr := longestContainingZone(zones, workloadDomain)
		if zoneErr != nil {
			return domain.CloudflareConnection{}, zoneErr
		}
		if _, err := client.ListDNSRecords(ctx, workloadZone.ID, "*."+workloadDomain); err != nil {
			return domain.CloudflareConnection{}, validationProviderError("Cloudflare token cannot read the workload DNS zone", err)
		}
		if _, err := client.ListCertificatePacks(ctx, workloadZone.ID); err != nil {
			return domain.CloudflareConnection{}, validationProviderError("Cloudflare token cannot inspect edge certificates; grant SSL and Certificates Read for the workload zone", err)
		}
		// The settings are read-only and informational; Total TLS is not
		// accepted as proof because Cloudflare excludes Tunnel hostnames.
		_, _ = client.TotalTLSSettings(ctx, workloadZone.ID)
	}
	return domain.CloudflareConnection{
		AccountID: accountID, ConsoleZoneID: consoleZone.ID, ConsoleHostname: consoleHostname,
		TunnelID: tunnelID, TunnelName: tunnelName, ConsoleRecordID: consoleRecord.ID,
	}, nil
}

func validationProviderError(message string, err error) error {
	if errors.Is(err, ErrUnauthorized) {
		return fmt.Errorf("%s: %w", message, ErrUnauthorized)
	}
	return errors.New(message)
}

func validateExistingConnection(ctx context.Context, client Client, accountID, consoleZoneID, consoleHostname, tunnelID, tunnelName string) ([]Zone, error) {
	if err := validateExistingTunnel(ctx, client, accountID, tunnelID, tunnelName); err != nil {
		return nil, err
	}
	zones, err := client.ListZones(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !zoneContainsHostname(zones, consoleZoneID, consoleHostname) {
		return nil, errors.New("Cloudflare token cannot read the configured Console zone")
	}
	return zones, nil
}

func validateExistingTunnel(ctx context.Context, client Client, accountID, tunnelID, tunnelName string) error {
	accounts, err := client.ListAccounts(ctx)
	if err != nil {
		return err
	}
	if !containsAccount(accounts, accountID) {
		return errors.New("Cloudflare token cannot access the configured account")
	}
	status, err := client.TunnelStatus(ctx, accountID, tunnelID)
	if err != nil {
		return err
	}
	if status.ID != "" && status.ID != tunnelID {
		return errors.New("Cloudflare returned a different tunnel identity")
	}
	if tunnelName != "" && status.Name != "" && status.Name != tunnelName {
		return errors.New("configured Cloudflare tunnel identity changed")
	}
	return nil
}

func connectionConfigured(connection domain.CloudflareConnection) bool {
	return strings.TrimSpace(connection.APIToken) != "" && strings.TrimSpace(connection.AccountID) != "" && strings.TrimSpace(connection.ConsoleZoneID) != "" && strings.TrimSpace(connection.ConsoleHostname) != "" && strings.TrimSpace(connection.TunnelID) != "" && strings.TrimSpace(connection.TunnelName) != "" && strings.TrimSpace(connection.ConsoleRecordID) != ""
}

func hasCloudflareConnectionIntent(connection domain.CloudflareConnection) bool {
	return strings.TrimSpace(connection.APIToken) != "" || strings.TrimSpace(connection.AccountID) != "" ||
		strings.TrimSpace(connection.ConsoleZoneID) != "" || strings.TrimSpace(connection.ConsoleHostname) != "" ||
		strings.TrimSpace(connection.TunnelID) != "" || strings.TrimSpace(connection.TunnelName) != "" ||
		strings.TrimSpace(connection.ConsoleRecordID) != ""
}

func cloudflareRoutingStatus(edgeTLSStatus string) string {
	switch edgeTLSStatus {
	case EdgeTLSNotApplicable, EdgeTLSReady:
		return "ready"
	case EdgeTLSPending:
		return "pending"
	default:
		return "error"
	}
}

func boundedEdgeTLSReason(reason string) string {
	reason = strings.Map(func(r rune) rune {
		if r == '\x00' || r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, strings.TrimSpace(reason))
	if len(reason) > 512 {
		return reason[:512]
	}
	return reason
}
