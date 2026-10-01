package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func (s *ClickHouseStore) QueryLogVolume(ctx context.Context, query LogVolumeQuery) (LogVolumeResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return LogVolumeResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return LogVolumeResult{}, err
	}
	statement := logVolumeQueryFor(query.Range)
	rows, err := s.query(ctx, statement,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
		clickhouse.Named("level", boundedFilter(query.Level, 64)),
		clickhouse.Named("search", boundedFilter(query.Search, 256)),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return LogVolumeResult{}, err
	}
	defer rows.Close()
	result := LogVolumeResult{Items: make([]LogVolumeBucket, 0, query.Limit)}
	for rows.Next() {
		var item LogVolumeBucket
		if err := rows.Scan(&item.Timestamp, &item.Count); err != nil {
			return LogVolumeResult{}, fmt.Errorf("scan log volume: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return LogVolumeResult{}, fmt.Errorf("read log volume: %w", err)
	}
	return result, nil
}

func logVolumeQueryFor(queryRange TimeRange) string {
	duration := queryRange.To.Sub(queryRange.From)
	switch {
	case duration <= time.Hour:
		return logVolumeMinuteQuery
	case duration <= 6*time.Hour:
		return logVolumeFiveMinuteQuery
	case duration <= 24*time.Hour:
		return logVolumeQuarterHourQuery
	case duration <= 7*24*time.Hour:
		return logVolumeHourQuery
	default:
		return logVolumeDayQuery
	}
}

func (s *ClickHouseStore) QueryErrorGroups(ctx context.Context, query ErrorGroupsQuery) (ErrorGroupsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return ErrorGroupsResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return ErrorGroupsResult{}, err
	}
	rows, err := s.query(ctx, errorGroupsQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
		clickhouse.Named("search", boundedFilter(query.Search, 256)),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return ErrorGroupsResult{}, err
	}
	defer rows.Close()
	result := ErrorGroupsResult{Items: make([]ErrorGroup, 0, query.Limit)}
	for rows.Next() {
		var service, severity, message, traceID string
		var firstSeen, lastSeen time.Time
		var count uint64
		if err := rows.Scan(&service, &severity, &message, &firstSeen, &lastSeen, &count, &traceID); err != nil {
			return ErrorGroupsResult{}, fmt.Errorf("scan error group: %w", err)
		}
		message = redactText(message)
		fingerprintInput := service + "\x00" + severity + "\x00" + message
		fingerprint := sha256.Sum256([]byte(fingerprintInput))
		result.Items = append(result.Items, ErrorGroup{
			Fingerprint: hex.EncodeToString(fingerprint[:]), Service: service,
			ErrorType: normalizedErrorType(severity), Message: message,
			Status:    "open",
			FirstSeen: firstSeen, LastSeen: lastSeen, OccurrenceCount: count,
			TraceID: traceID,
		})
	}
	if err := rows.Err(); err != nil {
		return ErrorGroupsResult{}, fmt.Errorf("read error groups: %w", err)
	}
	return result, nil
}

func normalizedErrorType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "error"
	}
	return value
}

func (s *ClickHouseStore) QueryServiceMap(ctx context.Context, query ServiceMapQuery) (ServiceMapResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return ServiceMapResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return ServiceMapResult{}, err
	}
	rows, err := s.query(ctx, serviceMapQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return ServiceMapResult{}, err
	}
	defer rows.Close()
	result := ServiceMapResult{Items: make([]ServiceMapEdge, 0, query.Limit)}
	for rows.Next() {
		var item ServiceMapEdge
		if err := rows.Scan(&item.Source, &item.Target, &item.RequestCount, &item.ErrorCount, &item.P95LatencyMS); err != nil {
			return ServiceMapResult{}, fmt.Errorf("scan service map edge: %w", err)
		}
		if item.RequestCount > 0 {
			item.ErrorRate = float64(item.ErrorCount) / float64(item.RequestCount)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ServiceMapResult{}, fmt.Errorf("read service map: %w", err)
	}
	return result, nil
}

func (s *ClickHouseStore) QueryInfrastructure(ctx context.Context, query InfrastructureQuery) (InfrastructureResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return InfrastructureResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return InfrastructureResult{}, err
	}
	query.Scope = strings.TrimSpace(strings.ToLower(query.Scope))
	if query.Scope != "" && query.Scope != "host" && query.Scope != "containers" && query.Scope != "postgres" && query.Scope != "redis" && query.Scope != "services" {
		return InfrastructureResult{}, fmt.Errorf("%w: infrastructure scope is unsupported", ErrInvalidQuery)
	}
	rows, err := s.query(ctx, infrastructureQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.Seconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.Seconds),
		clickhouse.Named("scope", query.Scope),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return InfrastructureResult{}, err
	}
	defer rows.Close()
	result := InfrastructureResult{Items: make([]InfrastructureMetric, 0, query.Limit)}
	for rows.Next() {
		var item InfrastructureMetric
		var attributes, resourceAttributes map[string]string
		if err := rows.Scan(&item.Timestamp, &item.Scope, &item.Name, &item.Service, &item.Value, &attributes, &resourceAttributes); err != nil {
			return InfrastructureResult{}, fmt.Errorf("scan infrastructure metric: %w", err)
		}
		item.Attributes = redactAttributes(attributes)
		item.ResourceAttributes = redactAttributes(resourceAttributes)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return InfrastructureResult{}, fmt.Errorf("read infrastructure metrics: %w", err)
	}
	return result, nil
}
