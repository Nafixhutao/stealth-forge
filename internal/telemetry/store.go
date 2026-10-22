// Package telemetry provides the authenticated, bounded query boundary for
// Stealth's observability data. Request telemetry is stored in PostgreSQL, so
// the API owns this boundary and the browser never receives database
// credentials or arbitrary SQL access.
package telemetry

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDisabled     = errors.New("telemetry backend is disabled")
	ErrInvalidQuery = errors.New("invalid telemetry query")
)

// Pinger is the small health capability used by the Admin overview. Keeping
// it separate lets optional query capabilities evolve without changing health
// reporting.
type Pinger interface {
	Ping(context.Context) error
}

// Config contains the server-side safety limits applied to every query.
type Config struct {
	MaxQueryDuration time.Duration
	MaxQueryRange    time.Duration
	MaxQueryRows     int
	Retention        time.Duration
}

// Store is the only data access contract exposed to HTTP handlers. New query
// features should add a constrained domain method rather than accepting SQL.
type Store interface {
	Ping(context.Context) error
	QueryLogs(context.Context, LogsQuery) (LogsResult, error)
	QueryTraces(context.Context, TracesQuery) (TracesResult, error)
	QueryMetrics(context.Context, MetricsQuery) (MetricsResult, error)
	ListSources(context.Context, SourcesQuery) (SourcesResult, error)
}

// ManagedStore is the lifecycle surface used by process composition. The
// PostgreSQL implementation owns no external connection, so Migrate and Close
// are no-ops kept for a uniform composition contract.
type ManagedStore interface {
	Store
	Explorer
	InfrastructureExplorer
	OverviewExplorer
	AlertExplorer
	Migrate(context.Context) error
	Close() error
}
