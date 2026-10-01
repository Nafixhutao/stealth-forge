package cloudflare

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stealth-deplover/stealth/internal/domainname"
)

func (r *Reconciler) ReconcileConsoleOrigin(ctx context.Context) (origin string, err error) {
	if r == nil || r.store == nil || r.client == nil {
		return "", errors.New("Cloudflare reconciler is not configured")
	}
	release, acquired, err := r.store.TryCloudflareReconcileLock(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire Cloudflare reconcile lock: %w", err)
	}
	if !acquired {
		return "", ErrLockNotAcquired
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release Cloudflare reconcile lock: %w", releaseErr))
		}
	}()

	connection, err := r.store.CloudflareReconcileSnapshot(ctx)
	if err != nil {
		return "", fmt.Errorf("read Cloudflare desired state: %w", err)
	}
	if !hasCloudflareConnectionIntent(connection) {
		return "", nil
	}
	desired := connection.ConsoleOriginDesired
	if desired != ConsoleOriginProxy && desired != ConsoleOriginTraefik {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, errors.New("saved Console origin is invalid; run stealth ingress rollback"))
	}
	if !connectionConfigured(connection) {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, errors.New("Cloudflare connection is unavailable; an Instance Owner must reconnect the scoped token for the existing Named Tunnel"))
	}
	provider, err := r.client(connection.APIToken)
	if err != nil {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, errors.New("Cloudflare API client could not be initialized"))
	}
	providerCtx, cancel := context.WithTimeout(ctx, r.httpTimeout)
	defer cancel()
	if err := validateExistingTunnel(providerCtx, provider, connection.AccountID, connection.TunnelID, connection.TunnelName); err != nil {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, err)
	}
	current, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
	if err != nil {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, err)
	}
	want, observed, err := patchConsoleIngress(current, connection.ConsoleHostname, connection.WildcardHostname, desired)
	if err != nil {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, err)
	}
	if observed != desired {
		if err := provider.ConfigureTunnel(providerCtx, connection.AccountID, connection.TunnelID, want); err != nil {
			return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, err)
		}
	}
	verified, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
	if err != nil {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, err)
	}
	if !sameTunnelRules(verified, want) {
		return "", r.recordConsoleOriginFailure(ctx, connection.APIToken, desired, errors.New("Cloudflare Tunnel Console-origin read-back did not match the requested change; HIGH SEVERITY: provider rollback is unverified"))
	}
	completed, err := r.store.CompleteCloudflareConsoleOrigin(ctx, desired, desired)
	if err != nil {
		return "", fmt.Errorf("persist Cloudflare Console-origin observation: %w", err)
	}
	if !completed {
		return "", ErrCloudflareDesiredStateChanged
	}
	r.logger.Info("cloudflare console origin reconciled", "desired_origin", desired, "observed_origin", desired, "changed", observed != desired)
	return desired, nil
}

// VerifyConsoleOrigin checks only the existing Tunnel's Console route. It is
// intentionally independent of workload zone and certificate readiness.
func (r *Reconciler) VerifyConsoleOrigin(ctx context.Context) (origin string, err error) {
	if r == nil || r.store == nil || r.client == nil {
		return "", errors.New("Cloudflare reconciler is not configured")
	}
	release, acquired, err := r.store.TryCloudflareReconcileLock(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire Cloudflare reconcile lock: %w", err)
	}
	if !acquired {
		return "", ErrLockNotAcquired
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release Cloudflare reconcile lock: %w", releaseErr))
		}
	}()
	connection, err := r.store.CloudflareReconcileSnapshot(ctx)
	if err != nil {
		return "", fmt.Errorf("read Cloudflare desired state: %w", err)
	}
	if !connectionConfigured(connection) {
		return "", errors.New("Cloudflare Named Tunnel is not fully configured")
	}
	if connection.ConsoleOriginDesired != ConsoleOriginProxy && connection.ConsoleOriginDesired != ConsoleOriginTraefik {
		return "", errors.New("saved Console origin is invalid")
	}
	provider, err := r.client(connection.APIToken)
	if err != nil {
		return "", errors.New("Cloudflare API client could not be initialized")
	}
	providerCtx, cancel := context.WithTimeout(ctx, r.httpTimeout)
	defer cancel()
	if err := validateExistingTunnel(providerCtx, provider, connection.AccountID, connection.TunnelID, connection.TunnelName); err != nil {
		return "", err
	}
	current, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
	if err != nil {
		return "", err
	}
	_, observed, err := patchConsoleIngress(current, connection.ConsoleHostname, connection.WildcardHostname, connection.ConsoleOriginDesired)
	if err != nil {
		return "", err
	}
	if observed != connection.ConsoleOriginDesired {
		return "", errors.New("Cloudflare Tunnel Console origin differs from durable desired state")
	}
	return observed, nil
}

// VerifyProvider reads the current durable desired state and Cloudflare
// Tunnel configuration without changing either. It shares the reconciler's
// PostgreSQL lock so an operator verification cannot observe a mid-write
// configuration from another worker.
func (r *Reconciler) VerifyProvider(ctx context.Context) (origin string, err error) {
	if r == nil || r.store == nil || r.client == nil {
		return "", errors.New("Cloudflare reconciler is not configured")
	}
	release, acquired, err := r.store.TryCloudflareReconcileLock(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire Cloudflare reconcile lock: %w", err)
	}
	if !acquired {
		return "", ErrLockNotAcquired
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release Cloudflare reconcile lock: %w", releaseErr))
		}
	}()

	connection, err := r.store.CloudflareReconcileSnapshot(ctx)
	if err != nil {
		return "", fmt.Errorf("read Cloudflare desired state: %w", err)
	}
	if !connectionConfigured(connection) {
		return "", errors.New("Cloudflare Named Tunnel is not fully configured")
	}
	if connection.ConsoleOriginDesired != ConsoleOriginProxy && connection.ConsoleOriginDesired != ConsoleOriginTraefik {
		return "", errors.New("saved Console origin is invalid")
	}
	provider, err := r.client(connection.APIToken)
	if err != nil {
		return "", errors.New("Cloudflare API client could not be initialized")
	}
	providerCtx, cancel := context.WithTimeout(ctx, r.httpTimeout)
	defer cancel()
	zones, err := validateExistingConnection(providerCtx, provider, connection.AccountID, connection.ConsoleZoneID, connection.ConsoleHostname, connection.TunnelID, connection.TunnelName)
	if err != nil {
		return "", err
	}
	wildcardHostname := ""
	if connection.WorkloadBaseDomain != nil {
		workloadDomain, normalizeErr := domainname.NormalizeDomain(*connection.WorkloadBaseDomain)
		if normalizeErr != nil || workloadDomain != *connection.WorkloadBaseDomain {
			return "", errors.New("saved workload domain is invalid")
		}
		if _, err := longestContainingZone(zones, workloadDomain); err != nil {
			return "", err
		}
		wildcardHostname = "*." + workloadDomain
	}
	current, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
	if err != nil {
		return "", err
	}
	want := desiredTunnelIngress(connection.ConsoleHostname, wildcardHostname, connection.ConsoleOriginDesired)
	if !sameIngress(current, want) {
		return "", errors.New("Cloudflare Tunnel ingress differs from the durable Console and workload routing contract")
	}
	return connection.ConsoleOriginDesired, nil
}

// ValidateExistingTunnel validates a replacement token against the durable
// tunnel identity and discovers the Console DNS record without changing any
// provider state. If zoneID is empty, it uses longest-suffix zone discovery.
