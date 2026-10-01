package telemetry

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func (s *ClickHouseStore) QueryLogs(ctx context.Context, query LogsQuery) (LogsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return LogsResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return LogsResult{}, err
	}
	statement := logsQuery
	args := []any{
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("from_metrics", query.Range.From.UTC(), clickhouse.Seconds),
		clickhouse.DateNamed("to_metrics", query.Range.To.UTC(), clickhouse.Seconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
		clickhouse.Named("level", boundedFilter(query.Level, 64)),
		clickhouse.Named("search", boundedFilter(query.Search, 256)),
	}
	if query.After != nil {
		statement = logsAfterQuery
		args = append(args,
			clickhouse.DateNamed("after_timestamp", query.After.Timestamp.UTC(), clickhouse.NanoSeconds),
			clickhouse.Named("after_event_id", boundedFilter(query.After.EventID, 256)),
		)
	} else {
		args = append(args, clickhouse.Named("live_tail", boolToUint(query.LiveTail)))
	}
	args = append(args, clickhouse.Named("limit", limit))
	rows, err := s.query(ctx, statement, args...)
	if err != nil {
		return LogsResult{}, err
	}
	defer rows.Close()
	result := LogsResult{Items: make([]LogRecord, 0, query.Limit)}
	for rows.Next() {
		var item LogRecord
		var attributes, resourceAttributes map[string]string
		var containerName, imageName, imageID, composeProject, composeService, composeContainerNumber string
		if err := rows.Scan(&item.Timestamp, &item.TraceID, &item.SpanID, &item.Severity, &item.Service, &item.Body, &attributes, &resourceAttributes, &containerName, &imageName, &imageID, &composeProject, &composeService, &composeContainerNumber, &item.EventID); err != nil {
			return LogsResult{}, fmt.Errorf("scan telemetry log: %w", err)
		}
		item.Body = redactText(item.Body)
		delete(attributes, logEventIDAttribute)
		item.Attributes = redactAttributes(attributes)
		item.ResourceAttributes = redactAttributes(resourceAttributes)
		item.ResourceAttributes = enrichContainerAttributes(item.ResourceAttributes, containerName, imageName, imageID, composeProject, composeService, composeContainerNumber)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return LogsResult{}, fmt.Errorf("read telemetry logs: %w", err)
	}
	return result, nil
}

func boolToUint(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func enrichContainerAttributes(attributes map[string]string, containerName, imageName, imageID, composeProject, composeService, composeContainerNumber string) map[string]string {
	values := map[string]string{
		"container.name":                  containerName,
		"container.image.name":            imageName,
		"container.image.id":              imageID,
		"docker.compose.project":          composeProject,
		"docker.compose.service":          composeService,
		"docker.compose.container_number": composeContainerNumber,
	}
	for key, value := range values {
		if value == "" {
			delete(values, key)
		}
	}
	if len(values) == 0 {
		return attributes
	}
	if attributes == nil {
		attributes = make(map[string]string, len(values))
	} else {
		copy := make(map[string]string, len(attributes)+len(values))
		for key, value := range attributes {
			copy[key] = value
		}
		attributes = copy
	}
	for key, value := range values {
		if _, exists := attributes[key]; !exists || attributes[key] == "" {
			attributes[key] = value
		}
	}
	return attributes
}

func (s *ClickHouseStore) QueryTraces(ctx context.Context, query TracesQuery) (TracesResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return TracesResult{}, err
	}
	if math.IsNaN(query.MinMs) || math.IsInf(query.MinMs, 0) || query.MinMs < 0 || query.MinMs > 24*60*60*1000 {
		return TracesResult{}, fmt.Errorf("%w: min duration is outside the allowed range", ErrInvalidQuery)
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return TracesResult{}, err
	}
	minDuration := uint64(query.MinMs * float64(time.Millisecond))
	rows, err := s.query(ctx, tracesQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
		clickhouse.Named("trace_id", boundedFilter(query.TraceID, 64)),
		clickhouse.Named("min_duration_ns", minDuration),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return TracesResult{}, err
	}
	defer rows.Close()
	result := TracesResult{Items: make([]SpanRecord, 0, query.Limit)}
	for rows.Next() {
		var item SpanRecord
		var attributes, resourceAttributes map[string]string
		if err := rows.Scan(&item.Timestamp, &item.TraceID, &item.SpanID, &item.ParentSpanID, &item.Name, &item.Kind, &item.Service, &item.DurationNs, &item.Status, &item.StatusMessage, &attributes, &resourceAttributes); err != nil {
			return TracesResult{}, fmt.Errorf("scan telemetry span: %w", err)
		}
		item.StatusMessage = redactText(item.StatusMessage)
		item.Attributes = redactAttributes(attributes)
		item.ResourceAttributes = redactAttributes(resourceAttributes)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return TracesResult{}, fmt.Errorf("read telemetry traces: %w", err)
	}
	return result, nil
}

func (s *ClickHouseStore) QueryMetrics(ctx context.Context, query MetricsQuery) (MetricsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return MetricsResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return MetricsResult{}, err
	}
	rows, err := s.query(ctx, metricsQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.Seconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.Seconds),
		clickhouse.Named("service", boundedFilter(query.Service, 128)),
		clickhouse.Named("name", boundedFilter(query.Name, 256)),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return MetricsResult{}, err
	}
	defer rows.Close()
	result := MetricsResult{Items: make([]MetricRecord, 0, query.Limit)}
	for rows.Next() {
		var item MetricRecord
		var attributes, resourceAttributes map[string]string
		var scalarValue, metricSum, metricMin, metricMax float64
		var metricCount, zeroCount uint64
		var bucketCounts, positiveBucketCounts, negativeBucketCounts []uint64
		var explicitBounds, quantiles, quantileValues []float64
		var scale, positiveOffset, negativeOffset, aggregationTemporality int32
		if err := rows.Scan(&item.Timestamp, &item.Name, &item.Service, &scalarValue, &attributes, &resourceAttributes, &item.Kind, &metricCount, &metricSum, &bucketCounts, &explicitBounds, &quantiles, &quantileValues, &scale, &zeroCount, &positiveOffset, &positiveBucketCounts, &negativeOffset, &negativeBucketCounts, &metricMin, &metricMax, &aggregationTemporality); err != nil {
			return MetricsResult{}, fmt.Errorf("scan telemetry metric: %w", err)
		}
		switch item.Kind {
		case "gauge", "sum":
			item.Value = finiteMetricValue(scalarValue)
		case "histogram":
			item.Histogram = &MetricHistogram{Count: metricCount, Sum: metricSum, BucketCounts: bucketCounts, ExplicitBounds: explicitBounds, Min: finiteMetricValue(metricMin), Max: finiteMetricValue(metricMax), AggregationTemporality: aggregationTemporality}
		case "summary":
			item.Summary = &MetricSummary{Count: metricCount, Sum: metricSum, Quantiles: metricQuantiles(quantiles, quantileValues), AggregationTemporality: aggregationTemporality}
		case "exponential_histogram":
			item.Exponential = &MetricExponentialHistogram{Count: metricCount, Sum: metricSum, Scale: scale, ZeroCount: zeroCount, PositiveOffset: positiveOffset, PositiveBucketCounts: positiveBucketCounts, NegativeOffset: negativeOffset, NegativeBucketCounts: negativeBucketCounts, Min: finiteMetricValue(metricMin), Max: finiteMetricValue(metricMax), AggregationTemporality: aggregationTemporality}
		}
		item.Attributes = redactAttributes(attributes)
		item.ResourceAttributes = redactAttributes(resourceAttributes)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return MetricsResult{}, fmt.Errorf("read telemetry metrics: %w", err)
	}
	return result, nil
}

func finiteMetricValue(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func metricQuantiles(quantiles, values []float64) []MetricQuantile {
	count := len(quantiles)
	if len(values) < count {
		count = len(values)
	}
	result := make([]MetricQuantile, 0, count)
	for index := 0; index < count; index++ {
		if math.IsNaN(quantiles[index]) || math.IsInf(quantiles[index], 0) || math.IsNaN(values[index]) || math.IsInf(values[index], 0) {
			continue
		}
		result = append(result, MetricQuantile{Quantile: quantiles[index], Value: values[index]})
	}
	return result
}

func (s *ClickHouseStore) ListSources(ctx context.Context, query SourcesQuery) (SourcesResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return SourcesResult{}, err
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return SourcesResult{}, err
	}
	rows, err := s.query(ctx, sourcesQuery,
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("from_metrics", query.Range.From.UTC(), clickhouse.Seconds),
		clickhouse.DateNamed("to_metrics", query.Range.To.UTC(), clickhouse.Seconds),
		clickhouse.Named("limit", limit),
	)
	if err != nil {
		return SourcesResult{}, err
	}
	defer rows.Close()
	result := SourcesResult{Items: make([]SourceRecord, 0, query.Limit)}
	for rows.Next() {
		var item SourceRecord
		if err := rows.Scan(&item.Service, &item.Signal, &item.LastReceived, &item.Volume); err != nil {
			return SourcesResult{}, fmt.Errorf("scan telemetry source: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return SourcesResult{}, fmt.Errorf("read telemetry sources: %w", err)
	}
	return result, nil
}
