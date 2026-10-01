package telemetry

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func (s *ClickHouseStore) QueryHTTPOverview(ctx context.Context, query HTTPOverviewQuery) (HTTPOverviewResult, error) {
	if err := s.validate(query.Range, 1); err != nil {
		return HTTPOverviewResult{}, err
	}
	seconds := query.Range.To.Sub(query.Range.From).Seconds()
	rows, err := s.query(ctx, httpOverviewQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("seconds", seconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
	)
	if err != nil {
		return HTTPOverviewResult{}, err
	}
	defer rows.Close()
	var result HTTPOverviewResult
	if rows.Next() {
		if err := rows.Scan(&result.RequestRate, &result.ErrorRate, &result.P50LatencyMS, &result.P95LatencyMS, &result.P99LatencyMS, &result.SampleCount); err != nil {
			return HTTPOverviewResult{}, fmt.Errorf("scan HTTP overview: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return HTTPOverviewResult{}, fmt.Errorf("read HTTP overview: %w", err)
	}
	return result, nil
}

const metricAlertAverageQuery = `
SELECT avg(Value), count()
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE MetricName = {metric:String}
  AND ({service:String} = '' OR ServiceName = {service:String})`

const metricAlertMaximumQuery = `
SELECT max(Value), count()
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE MetricName = {metric:String}
  AND ({service:String} = '' OR ServiceName = {service:String})`

const metricAlertMinimumQuery = `
SELECT min(Value), count()
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE MetricName = {metric:String}
  AND ({service:String} = '' OR ServiceName = {service:String})`

const metricAlertSumQuery = `
SELECT sum(Value), count()
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE MetricName = {metric:String}
  AND ({service:String} = '' OR ServiceName = {service:String})`

const metricAlertLatestQuery = `
SELECT argMax(Value, TimeUnix), count()
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE MetricName = {metric:String}
  AND ({service:String} = '' OR ServiceName = {service:String})`

const logMatchAlertQuery = `
SELECT count()
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND positionCaseInsensitiveUTF8(Body, {search:String}) > 0`

// EvaluateAlert is the telemetry side of the alert engine. Every branch is a
// fixed query with typed parameters and bounded time range; the caller only
// supplies validated dimensions and chooses the comparison operator later.
func (s *ClickHouseStore) EvaluateAlert(ctx context.Context, query AlertQuery) (AlertValue, error) {
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
	case "metric_threshold", "disk_pressure":
		metric := strings.TrimSpace(query.Metric)
		if query.Kind == "disk_pressure" && metric == "" {
			metric = "system.filesystem.utilization"
		}
		if metric == "" || len(metric) > 256 {
			return AlertValue{}, fmt.Errorf("%w: metric is required", ErrInvalidQuery)
		}
		statement, err := metricAlertStatement(query.Aggregation)
		if err != nil {
			return AlertValue{}, err
		}
		rows, err := s.query(ctx, statement,
			clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.Seconds),
			clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.Seconds),
			clickhouse.Named("metric", metric),
			clickhouse.Named("service", boundedFilter(query.Service, 128)),
		)
		if err != nil {
			return AlertValue{}, err
		}
		defer rows.Close()
		var value float64
		var count uint64
		if rows.Next() {
			if err := rows.Scan(&value, &count); err != nil {
				return AlertValue{}, fmt.Errorf("scan metric alert value: %w", err)
			}
		}
		if err := rows.Err(); err != nil {
			return AlertValue{}, fmt.Errorf("read metric alert value: %w", err)
		}
		return AlertValue{Value: value, SampleCount: count, Available: count > 0}, nil
	case "log_match":
		search := strings.TrimSpace(query.Search)
		if search == "" || len(search) > 256 {
			return AlertValue{}, fmt.Errorf("%w: log search text is required", ErrInvalidQuery)
		}
		rows, err := s.query(ctx, logMatchAlertQuery,
			clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
			clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
			clickhouse.Named("service", boundedFilter(query.Service, 128)),
			clickhouse.Named("level", boundedFilter(query.Level, 64)),
			clickhouse.Named("search", search),
		)
		if err != nil {
			return AlertValue{}, err
		}
		defer rows.Close()
		var count uint64
		if rows.Next() {
			if err := rows.Scan(&count); err != nil {
				return AlertValue{}, fmt.Errorf("scan log alert value: %w", err)
			}
		}
		if err := rows.Err(); err != nil {
			return AlertValue{}, fmt.Errorf("read log alert value: %w", err)
		}
		return AlertValue{Value: float64(count), SampleCount: count, Available: true}, nil
	default:
		return AlertValue{}, fmt.Errorf("%w: alert kind is not telemetry-backed", ErrInvalidQuery)
	}
}
