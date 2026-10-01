package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/domainname"
)

const tunnelServiceDomain = "cfargotunnel.com"

type RoutingStore interface {
	TryCloudflareReconcileLock(context.Context) (func() error, bool, error)
	CloudflareReconcileSnapshot(context.Context) (domain.CloudflareConnection, error)
	CompleteCloudflareReconcile(context.Context, domain.CloudflareRoutingUpdate) (bool, error)
	CompleteCloudflareConsoleOrigin(context.Context, string, string) (bool, error)
	RecordCloudflareConsoleOriginFailure(context.Context, string, string) error
	ListRetiringCloudflareWildcardDNS(context.Context) ([]domain.CloudflareRetiringWildcardDNS, error)
	CompleteCloudflareWildcardRetirement(context.Context, string) error
	RecordCloudflareReconcileFailure(context.Context, string) error
}

type ClientFactory func(string) (Client, error)

type Reconciler struct {
	store       RoutingStore
	client      ClientFactory
	interval    time.Duration
	logger      *slog.Logger
	httpTimeout time.Duration
}

type ReconcileResult struct {
	LockAcquired          bool
	Changed               bool
	Status                string
	EdgeTLSStatus         string
	EdgeTLSReason         string
	Hostname              string
	Zone                  string
	ConsoleOriginDesired  string
	ConsoleOriginObserved string
}

func NewReconciler(store RoutingStore, client ClientFactory, interval time.Duration, logger *slog.Logger) (*Reconciler, error) {
	if store == nil || client == nil {
		return nil, errors.New("Cloudflare reconciler requires a store and API client")
	}
	if interval < time.Second || interval > 5*time.Minute {
		return nil, errors.New("Cloudflare reconcile interval must be between 1s and 5m")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Reconciler{store: store, client: client, interval: interval, logger: logger, httpTimeout: 25 * time.Second}, nil
}

// Run performs an immediate startup reconcile, then bounded periodic passes.
// A provider failure is persisted and retried on the next cadence; it does not
// stop unrelated worker capabilities.
func (r *Reconciler) Run(ctx context.Context) error {
	r.runPass(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.runPass(ctx)
		}
	}
}

func (r *Reconciler) runPass(ctx context.Context) {
	origin, originErr := r.ReconcileConsoleOrigin(ctx)
	if errors.Is(originErr, ErrLockNotAcquired) {
		r.logger.Debug("cloudflare console-origin reconcile skipped; another worker or host command holds the lock")
		return
	}
	if originErr != nil {
		r.logger.Error("cloudflare Console-origin reconcile failed", "error", safeReconcileError(originErr, ""))
	} else if origin != "" {
		r.logger.Debug("cloudflare Console-origin ready", "origin", origin)
	}
	result, err := r.Reconcile(ctx)
	switch {
	case errors.Is(err, ErrLockNotAcquired):
		r.logger.Debug("cloudflare reconcile skipped; another worker holds the lock")
	case errors.Is(err, ErrUnauthorized):
		r.logger.Error("cloudflare token unauthorized", "error", safeReconcileError(err, ""))
	case result.EdgeTLSStatus == EdgeTLSError && err != nil:
		r.logger.Error("cloudflare edge TLS inspection failed", "edge_tls_status", result.EdgeTLSStatus, "error", safeReconcileError(err, ""))
	case result.EdgeTLSStatus == EdgeTLSActionRequired:
		r.logger.Error("cloudflare edge TLS action required", "workload_hostname", result.Hostname, "zone", result.Zone, "reason", result.EdgeTLSReason)
	case result.EdgeTLSStatus == EdgeTLSPending:
		r.logger.Info("cloudflare edge TLS pending", "workload_hostname", result.Hostname, "zone", result.Zone, "reason", result.EdgeTLSReason)
	case errors.Is(err, ErrRoutingConflict):
		r.logger.Error("cloudflare reconcile conflict", "error", safeReconcileError(err, ""))
	case err != nil:
		r.logger.Error("cloudflare reconcile failed", "error", safeReconcileError(err, ""))
	case result.Changed:
		r.logger.Info("cloudflare reconcile success", "workload_hostname", result.Hostname, "zone", result.Zone)
	case result.Status == "unconfigured":
		r.logger.Debug("cloudflare reconcile skipped; no Cloudflare connection is configured")
	default:
		r.logger.Debug("cloudflare already converged", "workload_hostname", result.Hostname, "zone", result.Zone)
	}
}

func (r *Reconciler) Reconcile(ctx context.Context) (result ReconcileResult, err error) {
	if r == nil || r.store == nil || r.client == nil {
		return result, errors.New("Cloudflare reconciler is not configured")
	}
	release, acquired, err := r.store.TryCloudflareReconcileLock(ctx)
	if err != nil {
		return result, fmt.Errorf("acquire Cloudflare reconcile lock: %w", err)
	}
	if !acquired {
		return result, ErrLockNotAcquired
	}
	result.LockAcquired = true
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release Cloudflare reconcile lock: %w", releaseErr))
		}
	}()

	connection, err := r.store.CloudflareReconcileSnapshot(ctx)
	if err != nil {
		return result, fmt.Errorf("read Cloudflare desired state: %w", err)
	}
	result.Status = connection.Status
	result.ConsoleOriginDesired = connection.ConsoleOriginDesired
	if !hasCloudflareConnectionIntent(connection) {
		// workload_base_domain is provider-neutral. An instance without a
		// Cloudflare identity and credential is deliberately outside this
		// reconciler, even when it has a workload domain configured.
		result.Status = "unconfigured"
		result.EdgeTLSStatus = EdgeTLSNotApplicable
		return result, nil
	}
	r.logger.Info("cloudflare reconcile start", "tunnel_id", connection.TunnelID)
	if !connectionConfigured(connection) {
		result.Status = "error"
		err = errors.New("Cloudflare connection is unavailable; an Instance Owner must reconnect the scoped token")
		return result, r.recordFailure(ctx, connection.APIToken, err)
	}
	if connection.ConsoleOriginDesired != ConsoleOriginProxy && connection.ConsoleOriginDesired != ConsoleOriginTraefik {
		return result, r.recordFailure(ctx, connection.APIToken, errors.New("saved Console origin is invalid; run stealth ingress rollback"))
	}
	provider, err := r.client(connection.APIToken)
	if err != nil {
		return result, r.recordFailure(ctx, connection.APIToken, errors.New("Cloudflare API client could not be initialized"))
	}
	providerCtx, cancel := context.WithTimeout(ctx, r.httpTimeout)
	defer cancel()

	zones, err := validateExistingConnection(providerCtx, provider, connection.AccountID, connection.ConsoleZoneID, connection.ConsoleHostname, connection.TunnelID, connection.TunnelName)
	if err != nil {
		return result, r.recordFailure(ctx, connection.APIToken, err)
	}

	var workloadZone Zone
	var wildcardHostname, wildcardRecordID string
	if connection.WorkloadBaseDomain != nil {
		workloadDomain, normalizeErr := domainname.NormalizeDomain(*connection.WorkloadBaseDomain)
		if normalizeErr != nil || workloadDomain != *connection.WorkloadBaseDomain {
			return result, r.recordFailure(ctx, connection.APIToken, errors.New("saved workload domain is invalid; correct the Instance domain setting"))
		}
		workloadZone, err = longestContainingZone(zones, workloadDomain)
		if err != nil {
			return result, r.recordFailure(ctx, connection.APIToken, err)
		}
		wildcardHostname = "*." + workloadDomain
		record, changed, ensureErr := ensureWildcardDNS(providerCtx, provider, workloadZone.ID, wildcardHostname, connection.TunnelID+"."+tunnelServiceDomain, connection.WorkloadZoneID, connection.WildcardHostname, connection.WildcardRecordID)
		if ensureErr != nil {
			return result, r.recordFailure(ctx, connection.APIToken, ensureErr)
		}
		wildcardRecordID = record.ID
		result.Changed = result.Changed || changed
		if changed {
			r.logger.Info("cloudflare wildcard DNS reconciled", "hostname", wildcardHostname, "zone", workloadZone.Name, "record_id", record.ID)
		}
	}
	result.Hostname = wildcardHostname
	result.Zone = workloadZone.Name

	desiredIngress := desiredTunnelIngress(connection.ConsoleHostname, wildcardHostname, connection.ConsoleOriginDesired)
	currentIngress, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
	if err != nil {
		return result, r.recordFailure(ctx, connection.APIToken, err)
	}
	if !sameIngress(currentIngress, desiredIngress) {
		if err := provider.ConfigureTunnel(providerCtx, connection.AccountID, connection.TunnelID, desiredIngress); err != nil {
			return result, r.recordFailure(ctx, connection.APIToken, err)
		}
		verified, err := provider.TunnelConfiguration(providerCtx, connection.AccountID, connection.TunnelID)
		if err != nil {
			return result, r.recordFailure(ctx, connection.APIToken, err)
		}
		if !sameIngress(verified, desiredIngress) {
			return result, r.recordFailure(ctx, connection.APIToken, errors.New("Cloudflare tunnel ingress did not match the requested Console and workload routes"))
		}
		result.Changed = true
		r.logger.Info("cloudflare tunnel ingress updated", "console_origin", consoleOriginService(connection.ConsoleOriginDesired), "workload_origin", workloadOrigin(wildcardHostname))
	}
	result.ConsoleOriginObserved = connection.ConsoleOriginDesired

	tlsObservation := edgeTLSObservation{Status: EdgeTLSNotApplicable}
	var tlsInspectionErr error
	if connection.WorkloadBaseDomain != nil {
		tlsObservation, tlsInspectionErr = inspectWorkloadEdgeTLS(providerCtx, provider, workloadZone, *connection.WorkloadBaseDomain)
	}
	tlsObservation.Reason = boundedEdgeTLSReason(tlsObservation.Reason)
	result.EdgeTLSStatus = tlsObservation.Status
	result.EdgeTLSReason = tlsObservation.Reason

	update := domain.CloudflareRoutingUpdate{
		ExpectedWorkloadBaseDomain: cloneString(connection.WorkloadBaseDomain),
		EdgeTLSStatus:              tlsObservation.Status,
		EdgeTLSError:               tlsObservation.Reason,
		ConsoleOriginDesired:       connection.ConsoleOriginDesired,
		ConsoleOriginObserved:      result.ConsoleOriginObserved,
	}
	if connection.WorkloadBaseDomain != nil {
		update.WorkloadZoneID = workloadZone.ID
		update.WorkloadZoneName = workloadZone.Name
		update.WildcardHostname = wildcardHostname
		update.WildcardRecordID = wildcardRecordID
	}
	completed, err := r.store.CompleteCloudflareReconcile(ctx, update)
	if err != nil {
		return result, r.recordFailure(ctx, connection.APIToken, fmt.Errorf("persist Cloudflare observed state: %w", err))
	}
	if !completed {
		result.Status = "pending"
		return result, nil
	}
	result.Status = cloudflareRoutingStatus(tlsObservation.Status)
	if tlsInspectionErr != nil {
		if errors.Is(tlsInspectionErr, ErrUnauthorized) {
			return result, ErrUnauthorized
		}
		return result, errors.New(safeReconcileError(tlsInspectionErr, connection.APIToken))
	}
	if result.Status != "ready" {
		return result, nil
	}
	retired, err := r.cleanupRetiring(providerCtx, provider)
	if err != nil {
		return result, r.recordFailure(ctx, connection.APIToken, err)
	}
	result.Changed = result.Changed || retired
	if result.Changed {
		r.logger.Info("cloudflare reconcile success", "workload_hostname", result.Hostname, "zone", result.Zone)
	}
	return result, nil
}

// ReconcileConsoleOrigin is the emergency-safe origin transition used by
// host-initiated cutover and rollback. It shares the distributed Cloudflare
// lock and only changes the Console rule in the existing Named Tunnel config.
// Workload DNS, certificates and retiring records are deliberately outside
// this recovery primitive.
var (
	ErrLockNotAcquired               = errors.New("Cloudflare reconcile lock was not acquired")
	ErrCloudflareDesiredStateChanged = errors.New("Cloudflare Console-origin desired state changed during provider reconciliation")
)
