// Package telemetry provides the authenticated, bounded query boundary for
// Stealth's high-volume observability data. The API owns this boundary; the
// browser never receives ClickHouse credentials or arbitrary SQL access.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var (
	ErrDisabled     = errors.New("telemetry backend is disabled")
	ErrInvalidQuery = errors.New("invalid telemetry query")
)

// Config contains only the ClickHouse connection and server-side safety
// limits. Password is held in memory by the process and is never included in
// errors, logs, or query text.
type Config struct {
	Address          string
	Database         string
	Username         string
	Password         string
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

type ClickHouseStore struct {
	conn             driver.Conn
	database         string
	maxQueryDuration time.Duration
	maxQueryRange    time.Duration
	maxQueryRows     int
	retention        time.Duration
}

const logEventIDAttribute = "stealth.log.event_id"

func New(cfg Config) (*ClickHouseStore, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		return nil, ErrDisabled
	}
	if cfg.Database == "" {
		cfg.Database = "stealth_telemetry"
	}
	if !isIdentifier(cfg.Database) {
		return nil, fmt.Errorf("%w: telemetry database must be a valid identifier", ErrInvalidQuery)
	}
	if cfg.Username == "" {
		cfg.Username = "stealth"
	}
	if cfg.MaxQueryDuration <= 0 {
		cfg.MaxQueryDuration = 10 * time.Second
	}
	if cfg.MaxQueryRange <= 0 {
		cfg.MaxQueryRange = 30 * 24 * time.Hour
	}
	if cfg.MaxQueryRows <= 0 {
		cfg.MaxQueryRows = 1000
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 30 * 24 * time.Hour
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: strings.Split(cfg.Address, ","),
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout: 5 * time.Second,
		ReadTimeout: cfg.MaxQueryDuration,
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("open telemetry store: %w", err)
	}
	return &ClickHouseStore{conn: conn, database: cfg.Database, maxQueryDuration: cfg.MaxQueryDuration, maxQueryRange: cfg.MaxQueryRange, maxQueryRows: cfg.MaxQueryRows, retention: cfg.Retention}, nil
}

// NewWithConn is intentionally small and is useful for integration tests
// that connect to a real ClickHouse instance as well as unit tests that embed
// driver.Conn and inspect the bounded query contract.
func NewWithConn(conn driver.Conn, cfg Config) *ClickHouseStore {
	if cfg.MaxQueryDuration <= 0 {
		cfg.MaxQueryDuration = 10 * time.Second
	}
	if cfg.MaxQueryRange <= 0 {
		cfg.MaxQueryRange = 30 * 24 * time.Hour
	}
	if cfg.MaxQueryRows <= 0 {
		cfg.MaxQueryRows = 1000
	}
	if cfg.Database == "" {
		cfg.Database = "stealth_telemetry"
	}
	return &ClickHouseStore{conn: conn, database: cfg.Database, maxQueryDuration: cfg.MaxQueryDuration, maxQueryRange: cfg.MaxQueryRange, maxQueryRows: cfg.MaxQueryRows, retention: cfg.Retention}
}

func (s *ClickHouseStore) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *ClickHouseStore) Ping(ctx context.Context) error {
	if s == nil || s.conn == nil {
		return ErrDisabled
	}
	if err := s.conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping telemetry backend: %w", err)
	}
	return nil
}
