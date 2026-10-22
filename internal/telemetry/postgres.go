package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore keeps telemetry on the existing control-plane database. The
// source is the bounded root-request index already written by the HTTP API;
// no collector, sidecar, or second ingestion table is required.
type PostgresStore struct {
	pool             *pgxpool.Pool
	maxQueryDuration time.Duration
	maxQueryRange    time.Duration
	maxQueryRows     int
	retention        time.Duration
}

// Open builds the PostgreSQL-backed telemetry store. The pool is the existing
// control-plane database; no collector, sidecar, or second store is required.
func Open(pool *pgxpool.Pool, cfg Config) (ManagedStore, error) {
	return NewPostgres(pool, cfg)
}

func NewPostgres(pool *pgxpool.Pool, cfg Config) (*PostgresStore, error) {
	if pool == nil {
		return nil, ErrDisabled
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
	return &PostgresStore{
		pool:             pool,
		maxQueryDuration: cfg.MaxQueryDuration,
		maxQueryRange:    cfg.MaxQueryRange,
		maxQueryRows:     cfg.MaxQueryRows,
		retention:        cfg.Retention,
	}, nil
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return ErrDisabled
	}
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping telemetry backend: %w", err)
	}
	return nil
}

// Migrate is a no-op because the normal numbered PostgreSQL migration runner
// owns the request-index schema. Keeping the method makes the process wiring
// identical for both optional stores.
func (s *PostgresStore) Migrate(context.Context) error { return nil }

// Close is a no-op: the pool is owned by the process composition root.
func (s *PostgresStore) Close() error { return nil }

func (s *PostgresStore) validate(queryRange TimeRange, limit int) error {
	if s == nil || s.pool == nil {
		return ErrDisabled
	}
	if err := queryRange.validate(s.maxQueryRange); err != nil {
		return err
	}
	if limit < 1 || limit > s.maxQueryRows {
		return fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidQuery, s.maxQueryRows)
	}
	return nil
}

func (s *PostgresStore) queryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.maxQueryDuration)
}

// requestLogFilter is shared by logs, volume, and alert queries. $1/$2 are the
// time range, $3 the service, $4 the search text, and $5 the severity. The
// caller appends only ordering, cursor, or grouping clauses around it.
const requestLogFilter = `
  started_at >= $1 AND started_at < $2
  AND ($3 = '' OR $3 = 'api')
  AND ($4 = '' OR method ILIKE '%' || $4 || '%' OR route ILIKE '%' || $4 || '%')
  AND (
    $5 = '' OR
    (upper($5) IN ('ERROR', 'FATAL') AND status >= 500) OR
    (upper($5) IN ('WARN', 'WARNING') AND status >= 400 AND status < 500) OR
    (upper($5) = 'INFO' AND status < 400)
  )`

const postgresLogsQuery = `
SELECT id::text, trace_id, COALESCE(span_id, ''), method, route, status,
       duration_ms, response_bytes, started_at
FROM http_traces
WHERE ` + requestLogFilter + `
ORDER BY started_at DESC, id DESC
LIMIT $6`

const postgresLogsAfterQuery = `
SELECT id::text, trace_id, COALESCE(span_id, ''), method, route, status,
       duration_ms, response_bytes, started_at
FROM http_traces
WHERE ` + requestLogFilter + `
  AND (started_at, id) > ($6, $7::uuid)
ORDER BY started_at ASC, id ASC
LIMIT $8`

func (s *PostgresStore) QueryLogs(ctx context.Context, query LogsQuery) (LogsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return LogsResult{}, err
	}
	statement := postgresLogsQuery
	args := []any{
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		boundedFilter(query.Service, 128),
		boundedFilter(query.Search, 256),
		boundedFilter(query.Level, 64),
	}
	if query.After != nil {
		if _, err := uuid.Parse(query.After.EventID); err != nil {
			return LogsResult{}, fmt.Errorf("%w: log cursor event id is invalid", ErrInvalidQuery)
		}
		statement = postgresLogsAfterQuery
		args = append(args, query.After.Timestamp.UTC(), query.After.EventID)
	}
	args = append(args, query.Limit)
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.pool.Query(queryContext, statement, args...)
	if err != nil {
		return LogsResult{}, fmt.Errorf("query PostgreSQL telemetry logs: %w", err)
	}
	defer rows.Close()
	result := LogsResult{Items: make([]LogRecord, 0, query.Limit)}
	for rows.Next() {
		var row postgresLogRow
		if err := rows.Scan(
			&row.EventID,
			&row.TraceID,
			&row.SpanID,
			&row.Method,
			&row.Route,
			&row.Status,
			&row.DurationMS,
			&row.ResponseBytes,
			&row.Timestamp,
		); err != nil {
			return LogsResult{}, fmt.Errorf("scan PostgreSQL telemetry log: %w", err)
		}
		result.Items = append(result.Items, postgresLogRecord(row))
	}
	if err := rows.Err(); err != nil {
		return LogsResult{}, fmt.Errorf("read PostgreSQL telemetry logs: %w", err)
	}
	return result, nil
}

func postgresLogRecord(row postgresLogRow) LogRecord {
	return LogRecord{
		Timestamp: row.Timestamp,
		TraceID:   row.TraceID,
		SpanID:    row.SpanID,
		Severity:  postgresLogSeverity(row.Status),
		Service:   "api",
		Body: fmt.Sprintf(
			"%s %s -> %d (%dms)",
			row.Method,
			row.Route,
			row.Status,
			row.DurationMS,
		),
		Attributes: map[string]string{
			"http.request.method":       row.Method,
			"http.route":                row.Route,
			"http.response.status_code": strconv.Itoa(row.Status),
			"duration_ms":               strconv.Itoa(row.DurationMS),
			"response_bytes":            strconv.FormatInt(row.ResponseBytes, 10),
		},
		ResourceAttributes: map[string]string{"service.name": "api"},
		EventID:            row.EventID,
	}
}

// postgresLogRow is the scan target for one request-index row. Grouping the
// columns keeps the scan list and the projection next to each other.
type postgresLogRow struct {
	EventID       string
	TraceID       string
	SpanID        string
	Method        string
	Route         string
	Status        int
	DurationMS    int
	ResponseBytes int64
	Timestamp     time.Time
}

func postgresLogSeverity(status int) string {
	switch {
	case status >= 500:
		return "ERROR"
	case status >= 400:
		return "WARN"
	default:
		return "INFO"
	}
}

const postgresTracesQuery = `
SELECT started_at, trace_id, COALESCE(NULLIF(span_id, ''), id::text),
       method, route, status, duration_ms
FROM http_traces
WHERE started_at >= $1 AND started_at < $2
  AND ($3 = '' OR $3 = 'api')
  AND ($4 = '' OR trace_id = $4)
  AND ($5 = 0 OR duration_ms >= $5)
ORDER BY started_at DESC, id DESC
LIMIT $6`

func (s *PostgresStore) QueryTraces(ctx context.Context, query TracesQuery) (TracesResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return TracesResult{}, err
	}
	if math.IsNaN(query.MinMs) || math.IsInf(query.MinMs, 0) || query.MinMs < 0 || query.MinMs > 24*60*60*1000 {
		return TracesResult{}, fmt.Errorf("%w: min duration is outside the allowed range", ErrInvalidQuery)
	}
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.pool.Query(queryContext, postgresTracesQuery,
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		boundedFilter(query.Service, 128),
		boundedFilter(query.TraceID, 64),
		math.Ceil(query.MinMs),
		query.Limit,
	)
	if err != nil {
		return TracesResult{}, fmt.Errorf("query PostgreSQL telemetry traces: %w", err)
	}
	defer rows.Close()
	result := TracesResult{Items: make([]SpanRecord, 0, query.Limit)}
	for rows.Next() {
		var item SpanRecord
		var method, route string
		var status, durationMS int
		if err := rows.Scan(&item.Timestamp, &item.TraceID, &item.SpanID, &method, &route, &status, &durationMS); err != nil {
			return TracesResult{}, fmt.Errorf("scan PostgreSQL telemetry trace: %w", err)
		}
		item.Name = method + " " + route
		item.Kind = "server"
		item.Service = "api"
		item.DurationNs = uint64(max(durationMS, 0)) * uint64(time.Millisecond)
		item.Status = "ok"
		if status >= 500 {
			item.Status = "error"
			item.StatusMessage = http.StatusText(status)
		}
		item.Attributes = map[string]string{
			"http.request.method":       method,
			"http.route":                route,
			"http.response.status_code": strconv.Itoa(status),
		}
		item.ResourceAttributes = map[string]string{"service.name": "api"}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return TracesResult{}, fmt.Errorf("read PostgreSQL telemetry traces: %w", err)
	}
	return result, nil
}

const postgresHTTPOverviewQuery = `
SELECT
  CASE WHEN $3::float8 > 0 THEN count(*)::float8 / $3::float8 ELSE 0 END,
  COALESCE(
    count(*) FILTER (WHERE status >= 500)::float8 / NULLIF(count(*), 0),
    0
  ),
  COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY duration_ms), 0),
  COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms), 0),
  COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY duration_ms), 0),
  count(*)
FROM http_traces
WHERE started_at >= $1 AND started_at < $2
  AND ($4 = '' OR $4 = 'api')`

func (s *PostgresStore) QueryHTTPOverview(ctx context.Context, query HTTPOverviewQuery) (HTTPOverviewResult, error) {
	if err := s.validate(query.Range, 1); err != nil {
		return HTTPOverviewResult{}, err
	}
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	var sampleCount int64
	var result HTTPOverviewResult
	err := s.pool.QueryRow(queryContext, postgresHTTPOverviewQuery,
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		query.Range.To.Sub(query.Range.From).Seconds(),
		boundedFilter(query.Service, 128),
	).Scan(
		&result.RequestRate,
		&result.ErrorRate,
		&result.P50LatencyMS,
		&result.P95LatencyMS,
		&result.P99LatencyMS,
		&sampleCount,
	)
	if err != nil {
		return HTTPOverviewResult{}, fmt.Errorf("query PostgreSQL HTTP overview: %w", err)
	}
	result.SampleCount = uint64(max(sampleCount, 0))
	return result, nil
}

const postgresErrorGroupsQuery = `
SELECT status, route, min(started_at), max(started_at), count(*),
       (array_agg(trace_id ORDER BY started_at DESC))[1]
FROM http_traces
WHERE started_at >= $1 AND started_at < $2
  AND status >= 500
  AND ($3 = '' OR $3 = 'api')
  AND ($4 = '' OR route ILIKE '%' || $4 || '%')
GROUP BY status, route
ORDER BY max(started_at) DESC, route ASC
LIMIT $5`

func (s *PostgresStore) QueryErrorGroups(ctx context.Context, query ErrorGroupsQuery) (ErrorGroupsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return ErrorGroupsResult{}, err
	}
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.pool.Query(queryContext, postgresErrorGroupsQuery,
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		boundedFilter(query.Service, 128),
		boundedFilter(query.Search, 256),
		query.Limit,
	)
	if err != nil {
		return ErrorGroupsResult{}, fmt.Errorf("query PostgreSQL error groups: %w", err)
	}
	defer rows.Close()
	result := ErrorGroupsResult{Items: make([]ErrorGroup, 0, query.Limit)}
	for rows.Next() {
		var status int
		var route, traceID string
		var firstSeen, lastSeen time.Time
		var count int64
		if err := rows.Scan(&status, &route, &firstSeen, &lastSeen, &count, &traceID); err != nil {
			return ErrorGroupsResult{}, fmt.Errorf("scan PostgreSQL error group: %w", err)
		}
		errorType := fmt.Sprintf("HTTP %d", status)
		result.Items = append(result.Items, ErrorGroup{
			Fingerprint:     postgresErrorFingerprint("api", errorType, route),
			Service:         "api",
			ErrorType:       errorType,
			Message:         route,
			Status:          "open",
			FirstSeen:       firstSeen,
			LastSeen:        lastSeen,
			OccurrenceCount: uint64(max(count, 0)),
			TraceID:         traceID,
		})
	}
	if err := rows.Err(); err != nil {
		return ErrorGroupsResult{}, fmt.Errorf("read PostgreSQL error groups: %w", err)
	}
	return result, nil
}

func postgresErrorFingerprint(service, errorType, message string) string {
	value := sha256.Sum256([]byte(service + "\x00" + errorType + "\x00" + message))
	return hex.EncodeToString(value[:])
}

func postgresVolumeQuery(interval string) string {
	return `
SELECT date_bin('` + interval + `', started_at, TIMESTAMPTZ '1970-01-01'), count(*)
FROM http_traces
WHERE ` + requestLogFilter + `
GROUP BY 1 ORDER BY 1 ASC LIMIT $6`
}

var (
	postgresMinuteVolumeQuery      = postgresVolumeQuery("1 minute")
	postgresFiveMinuteVolumeQuery  = postgresVolumeQuery("5 minutes")
	postgresQuarterHourVolumeQuery = postgresVolumeQuery("15 minutes")
	postgresHourVolumeQuery        = postgresVolumeQuery("1 hour")
	postgresDayVolumeQuery         = postgresVolumeQuery("1 day")
)

func (s *PostgresStore) QueryLogVolume(ctx context.Context, query LogVolumeQuery) (LogVolumeResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return LogVolumeResult{}, err
	}
	statement := postgresVolumeQueryFor(query.Range)
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.pool.Query(queryContext, statement,
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		boundedFilter(query.Service, 128),
		boundedFilter(query.Search, 256),
		boundedFilter(query.Level, 64),
		query.Limit,
	)
	if err != nil {
		return LogVolumeResult{}, fmt.Errorf("query PostgreSQL log volume: %w", err)
	}
	defer rows.Close()
	result := LogVolumeResult{Items: make([]LogVolumeBucket, 0, query.Limit)}
	for rows.Next() {
		var item LogVolumeBucket
		var count int64
		if err := rows.Scan(&item.Timestamp, &count); err != nil {
			return LogVolumeResult{}, fmt.Errorf("scan PostgreSQL log volume: %w", err)
		}
		item.Count = uint64(max(count, 0))
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return LogVolumeResult{}, fmt.Errorf("read PostgreSQL log volume: %w", err)
	}
	return result, nil
}

func postgresVolumeQueryFor(queryRange TimeRange) string {
	duration := queryRange.To.Sub(queryRange.From)
	switch {
	case duration <= time.Hour:
		return postgresMinuteVolumeQuery
	case duration <= 6*time.Hour:
		return postgresFiveMinuteVolumeQuery
	case duration <= 24*time.Hour:
		return postgresQuarterHourVolumeQuery
	case duration <= 7*24*time.Hour:
		return postgresHourVolumeQuery
	default:
		return postgresDayVolumeQuery
	}
}

const postgresSourcesQuery = `
SELECT 'api', signal, max(started_at), count(*)
FROM (
  SELECT started_at, 'logs'::text AS signal FROM http_traces
  WHERE started_at >= $1 AND started_at < $2
  UNION ALL
  SELECT started_at, 'traces'::text FROM http_traces
  WHERE started_at >= $1 AND started_at < $2
) signals
GROUP BY signal
ORDER BY signal ASC
LIMIT $3`

func (s *PostgresStore) ListSources(ctx context.Context, query SourcesQuery) (SourcesResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return SourcesResult{}, err
	}
	queryContext, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.pool.Query(queryContext, postgresSourcesQuery,
		query.Range.From.UTC(),
		query.Range.To.UTC(),
		query.Limit,
	)
	if err != nil {
		return SourcesResult{}, fmt.Errorf("query PostgreSQL telemetry sources: %w", err)
	}
	defer rows.Close()
	result := SourcesResult{Items: make([]SourceRecord, 0, query.Limit)}
	for rows.Next() {
		var item SourceRecord
		var volume int64
		if err := rows.Scan(&item.Service, &item.Signal, &item.LastReceived, &volume); err != nil {
			return SourcesResult{}, fmt.Errorf("scan PostgreSQL telemetry source: %w", err)
		}
		item.Volume = uint64(max(volume, 0))
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return SourcesResult{}, fmt.Errorf("read PostgreSQL telemetry sources: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) QueryMetrics(ctx context.Context, query MetricsQuery) (MetricsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return MetricsResult{}, err
	}
	return MetricsResult{Items: []MetricRecord{}}, nil
}

func (s *PostgresStore) QueryInfrastructure(
	ctx context.Context,
	query InfrastructureQuery,
) (
	InfrastructureResult,
	error,
) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return InfrastructureResult{}, err
	}
	return InfrastructureResult{Items: []InfrastructureMetric{}}, nil
}

func (s *PostgresStore) QueryServiceMap(ctx context.Context, query ServiceMapQuery) (ServiceMapResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return ServiceMapResult{}, err
	}
	return ServiceMapResult{Items: []ServiceMapEdge{}}, nil
}

const postgresLogMatchAlertQuery = `
SELECT count(*)
FROM http_traces
WHERE ` + requestLogFilter

func (s *PostgresStore) EvaluateAlert(ctx context.Context, query AlertQuery) (AlertValue, error) {
	if err := s.validate(query.Range, 1); err != nil {
		return AlertValue{}, err
	}
	switch query.Kind {
	case "error_rate", "service_health":
		result, err := s.QueryHTTPOverview(ctx, HTTPOverviewQuery{Range: query.Range, Service: query.Service})
		if err != nil {
			return AlertValue{}, err
		}
		return AlertValue{Value: result.ErrorRate, SampleCount: result.SampleCount, Available: result.SampleCount > 0}, nil
	case "latency":
		result, err := s.QueryHTTPOverview(ctx, HTTPOverviewQuery{Range: query.Range, Service: query.Service})
		if err != nil {
			return AlertValue{}, err
		}
		value := result.P95LatencyMS
		switch query.Percentile {
		case "p50":
			value = result.P50LatencyMS
		case "p99":
			value = result.P99LatencyMS
		case "", "p95":
		default:
			return AlertValue{}, fmt.Errorf("%w: unsupported latency percentile", ErrInvalidQuery)
		}
		return AlertValue{Value: value, SampleCount: result.SampleCount, Available: result.SampleCount > 0}, nil
	case "log_match":
		search := strings.TrimSpace(query.Search)
		if search == "" || len(search) > 256 {
			return AlertValue{}, fmt.Errorf("%w: log search text is required", ErrInvalidQuery)
		}
		queryContext, cancel := s.queryContext(ctx)
		defer cancel()
		var count int64
		err := s.pool.QueryRow(queryContext, postgresLogMatchAlertQuery,
			query.Range.From.UTC(),
			query.Range.To.UTC(),
			boundedFilter(query.Service, 128),
			search,
			boundedFilter(query.Level, 64),
		).Scan(&count)
		if err != nil {
			return AlertValue{}, fmt.Errorf("query PostgreSQL log alert: %w", err)
		}
		return AlertValue{Value: float64(max(count, 0)), SampleCount: uint64(max(count, 0)), Available: true}, nil
	case "metric_threshold", "disk_pressure":
		// Metrics remain sampled in-process by the dedicated host/database/
		// Redis collectors; they are not duplicated into the request index.
		return AlertValue{Available: false}, nil
	default:
		return AlertValue{}, fmt.Errorf("%w: alert kind is not telemetry-backed", ErrInvalidQuery)
	}
}
