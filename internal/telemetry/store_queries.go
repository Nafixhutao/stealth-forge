package telemetry

import (
	"context"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const dockerContainerMetadataQuery = `
SELECT ContainerID,
       argMax(ContainerName, TimeUnix) AS ContainerName,
       argMax(ImageName, TimeUnix) AS ImageName,
       argMax(ImageID, TimeUnix) AS ImageID,
       argMax(ComposeProject, TimeUnix) AS ComposeProject,
       argMax(ComposeService, TimeUnix) AS ComposeService,
       argMax(ComposeContainerNumber, TimeUnix) AS ComposeContainerNumber
FROM (
  SELECT TimeUnix,
         ResourceAttributes['container.id'] AS ContainerID,
         if(ResourceAttributes['container.name'] != '', ResourceAttributes['container.name'], Attributes['container.name']) AS ContainerName,
         if(ResourceAttributes['container.image.name'] != '', ResourceAttributes['container.image.name'], Attributes['container.image.name']) AS ImageName,
         if(ResourceAttributes['container.image.id'] != '', ResourceAttributes['container.image.id'], Attributes['container.image.id']) AS ImageID,
         if(Attributes['docker.compose.project'] != '', Attributes['docker.compose.project'], ResourceAttributes['docker.compose.project']) AS ComposeProject,
         if(Attributes['service.name'] != '', Attributes['service.name'], ResourceAttributes['docker.compose.service']) AS ComposeService,
         if(Attributes['docker.compose.container_number'] != '', Attributes['docker.compose.container_number'], ResourceAttributes['docker.compose.container_number']) AS ComposeContainerNumber
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
  UNION ALL
  SELECT TimeUnix,
         ResourceAttributes['container.id'] AS ContainerID,
         if(ResourceAttributes['container.name'] != '', ResourceAttributes['container.name'], Attributes['container.name']) AS ContainerName,
         if(ResourceAttributes['container.image.name'] != '', ResourceAttributes['container.image.name'], Attributes['container.image.name']) AS ImageName,
         if(ResourceAttributes['container.image.id'] != '', ResourceAttributes['container.image.id'], Attributes['container.image.id']) AS ImageID,
         if(Attributes['docker.compose.project'] != '', Attributes['docker.compose.project'], ResourceAttributes['docker.compose.project']) AS ComposeProject,
         if(Attributes['service.name'] != '', Attributes['service.name'], ResourceAttributes['docker.compose.service']) AS ComposeService,
         if(Attributes['docker.compose.container_number'] != '', Attributes['docker.compose.container_number'], ResourceAttributes['docker.compose.container_number']) AS ComposeContainerNumber
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
)
WHERE ContainerID != ''
GROUP BY ContainerID`

const logsQuery = `
WITH container_metadata AS (` + dockerContainerMetadataQuery + `)
SELECT Timestamp, TraceId, SpanId, SeverityText, ServiceName, Body,
       LogAttributes, ResourceAttributes,
       ifNull(container_metadata.ContainerName, ''), ifNull(container_metadata.ImageName, ''),
       ifNull(container_metadata.ImageID, ''), ifNull(container_metadata.ComposeProject, ''),
       ifNull(container_metadata.ComposeService, ''), ifNull(container_metadata.ComposeContainerNumber, ''),
       LogAttributes['stealth.log.event_id'] AS EventID
FROM otel_logs
LEFT JOIN container_metadata ON ResourceAttributes['container.id'] = container_metadata.ContainerID
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
  AND ({live_tail:UInt8} = 0 OR LogAttributes['stealth.log.event_id'] != '')
ORDER BY Timestamp DESC, EventID DESC
LIMIT {limit:UInt32}`

// logsAfterQuery is intentionally separate from logsQuery so the normal
// explorer keeps its newest-first contract while the live tail can advance in
// chronological order from a complete cursor. The tuple predicate mirrors
// every ORDER BY key; timestamp-only polling is not sufficient when several
// records share the same timestamp. Rows without the Collector-generated
// event ID are excluded because they cannot participate in a lossless cursor.
const logsAfterQuery = `
WITH container_metadata AS (` + dockerContainerMetadataQuery + `)
SELECT Timestamp, TraceId, SpanId, SeverityText, ServiceName, Body,
       LogAttributes, ResourceAttributes,
       ifNull(container_metadata.ContainerName, ''), ifNull(container_metadata.ImageName, ''),
       ifNull(container_metadata.ImageID, ''), ifNull(container_metadata.ComposeProject, ''),
       ifNull(container_metadata.ComposeService, ''), ifNull(container_metadata.ComposeContainerNumber, ''),
       LogAttributes['stealth.log.event_id'] AS EventID
FROM otel_logs
LEFT JOIN container_metadata ON ResourceAttributes['container.id'] = container_metadata.ContainerID
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
  AND LogAttributes['stealth.log.event_id'] != ''
  AND (Timestamp, LogAttributes['stealth.log.event_id']) >
      ({after_timestamp:DateTime64(9)}, {after_event_id:String})
ORDER BY Timestamp ASC, EventID ASC
LIMIT {limit:UInt32}`

const appContainerLogsQuery = `
SELECT Timestamp, SeverityText, Body, LogAttributes['stealth.log.event_id'] AS EventID
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND has({container_ids:Array(String)}, ResourceAttributes['container.id'])
  AND LogAttributes['stealth.log.event_id'] != ''
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
ORDER BY Timestamp DESC, EventID DESC
LIMIT {limit:UInt32}`

const appContainerLogsAfterQuery = `
SELECT Timestamp, SeverityText, Body, LogAttributes['stealth.log.event_id'] AS EventID
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND has({container_ids:Array(String)}, ResourceAttributes['container.id'])
  AND LogAttributes['stealth.log.event_id'] != ''
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
  AND (Timestamp, LogAttributes['stealth.log.event_id']) >
      ({after_timestamp:DateTime64(9)}, {after_event_id:String})
ORDER BY Timestamp ASC, EventID ASC
LIMIT {limit:UInt32}`

var runtimeContainerIDPattern = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

func (s *ClickHouseStore) QueryContainerLogs(ctx context.Context, query ContainerLogsQuery) (ContainerLogsResult, error) {
	if err := s.validate(query.Range, query.Limit); err != nil {
		return ContainerLogsResult{}, err
	}
	if len(query.ContainerIDs) == 0 || len(query.ContainerIDs) > 2048 {
		return ContainerLogsResult{}, fmt.Errorf("%w: runtime container source count is outside the allowed range", ErrInvalidQuery)
	}
	ids := make([]string, 0, len(query.ContainerIDs))
	seen := make(map[string]struct{}, len(query.ContainerIDs))
	for _, id := range query.ContainerIDs {
		if !runtimeContainerIDPattern.MatchString(id) {
			return ContainerLogsResult{}, fmt.Errorf("%w: invalid runtime container source", ErrInvalidQuery)
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(query.Level) > 64 || len(query.Search) > 256 {
		return ContainerLogsResult{}, fmt.Errorf("%w: App runtime log filter exceeds the allowed length", ErrInvalidQuery)
	}
	limit, err := clickHouseLimit(query.Limit)
	if err != nil {
		return ContainerLogsResult{}, err
	}
	statement := appContainerLogsQuery
	args := []any{
		clickhouse.DateNamed("from", query.Range.From.UTC(), clickhouse.NanoSeconds),
		clickhouse.DateNamed("to", query.Range.To.UTC(), clickhouse.NanoSeconds),
		clickhouse.Named("container_ids", ids),
		clickhouse.Named("level", query.Level),
		clickhouse.Named("search", query.Search),
	}
	if query.After != nil {
		statement = appContainerLogsAfterQuery
		args = append(args,
			clickhouse.DateNamed("after_timestamp", query.After.Timestamp.UTC(), clickhouse.NanoSeconds),
			clickhouse.Named("after_event_id", query.After.EventID),
		)
	}
	args = append(args, clickhouse.Named("limit", limit))
	rows, err := s.query(ctx, statement, args...)
	if err != nil {
		return ContainerLogsResult{}, err
	}
	defer rows.Close()
	result := ContainerLogsResult{Items: make([]ContainerLogRecord, 0, query.Limit)}
	for rows.Next() {
		var item ContainerLogRecord
		if err := rows.Scan(&item.Timestamp, &item.Severity, &item.Body, &item.EventID); err != nil {
			return ContainerLogsResult{}, fmt.Errorf("scan App runtime log: %w", err)
		}
		if item.EventID == "" || len(item.EventID) > 256 {
			continue
		}
		item.Body = boundedRuntimeLogBody(redactText(item.Body), 16<<10)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ContainerLogsResult{}, fmt.Errorf("read App runtime logs: %w", err)
	}
	if query.After == nil {
		for left, right := 0, len(result.Items)-1; left < right; left, right = left+1, right-1 {
			result.Items[left], result.Items[right] = result.Items[right], result.Items[left]
		}
	}
	return result, nil
}

func boundedRuntimeLogBody(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

const tracesQuery = `
SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, SpanKind,
       ServiceName, Duration, StatusCode, StatusMessage,
       SpanAttributes, ResourceAttributes
FROM otel_traces
WHERE Timestamp >= {from:DateTime64(9)}
  AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({trace_id:String} = '' OR TraceId = {trace_id:String})
  AND ({min_duration_ns:UInt64} = 0 OR Duration >= {min_duration_ns:UInt64})
ORDER BY Timestamp DESC, TraceId DESC, SpanId DESC
LIMIT {limit:UInt32}`

const metricsQuery = `
SELECT TimeUnix, MetricName, ServiceName, ScalarValue, Attributes, ResourceAttributes,
       MetricKind, MetricCount, MetricSum, BucketCounts, ExplicitBounds,
       Quantiles, QuantileValues, Scale, ZeroCount, PositiveOffset,
       PositiveBucketCounts, NegativeOffset, NegativeBucketCounts, MetricMin,
       MetricMax, AggregationTemporality
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value AS ScalarValue, Attributes, ResourceAttributes,
         'gauge' AS MetricKind, toUInt64(0) AS MetricCount, toFloat64(0) AS MetricSum,
         emptyArrayUInt64() AS BucketCounts, emptyArrayFloat64() AS ExplicitBounds,
         emptyArrayFloat64() AS Quantiles, emptyArrayFloat64() AS QuantileValues,
         toInt32(0) AS Scale, toUInt64(0) AS ZeroCount, toInt32(0) AS PositiveOffset,
         emptyArrayUInt64() AS PositiveBucketCounts, toInt32(0) AS NegativeOffset,
         emptyArrayUInt64() AS NegativeBucketCounts, toFloat64(0) AS MetricMin,
         toFloat64(0) AS MetricMax, toInt32(0) AS AggregationTemporality
  FROM otel_metrics_gauge
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value AS ScalarValue, Attributes, ResourceAttributes,
         'sum' AS MetricKind, toUInt64(0) AS MetricCount, toFloat64(0) AS MetricSum,
         emptyArrayUInt64() AS BucketCounts, emptyArrayFloat64() AS ExplicitBounds,
         emptyArrayFloat64() AS Quantiles, emptyArrayFloat64() AS QuantileValues,
         toInt32(0) AS Scale, toUInt64(0) AS ZeroCount, toInt32(0) AS PositiveOffset,
         emptyArrayUInt64() AS PositiveBucketCounts, toInt32(0) AS NegativeOffset,
         emptyArrayUInt64() AS NegativeBucketCounts, toFloat64(0) AS MetricMin,
         toFloat64(0) AS MetricMax, toInt32(0) AS AggregationTemporality
  FROM otel_metrics_sum
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, toFloat64(0) AS ScalarValue, Attributes, ResourceAttributes,
         'histogram' AS MetricKind, Count AS MetricCount, Sum AS MetricSum,
         BucketCounts, ExplicitBounds, emptyArrayFloat64() AS Quantiles,
         emptyArrayFloat64() AS QuantileValues, toInt32(0) AS Scale,
         toUInt64(0) AS ZeroCount, toInt32(0) AS PositiveOffset,
         emptyArrayUInt64() AS PositiveBucketCounts, toInt32(0) AS NegativeOffset,
         emptyArrayUInt64() AS NegativeBucketCounts, Min AS MetricMin,
         Max AS MetricMax, AggregationTemporality
  FROM otel_metrics_histogram
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, toFloat64(0) AS ScalarValue, Attributes, ResourceAttributes,
         'summary' AS MetricKind, Count AS MetricCount, Sum AS MetricSum,
         emptyArrayUInt64() AS BucketCounts, emptyArrayFloat64() AS ExplicitBounds,
         ValueAtQuantiles.Quantile AS Quantiles,
         ValueAtQuantiles.Value AS QuantileValues, toInt32(0) AS Scale,
         toUInt64(0) AS ZeroCount, toInt32(0) AS PositiveOffset,
         emptyArrayUInt64() AS PositiveBucketCounts, toInt32(0) AS NegativeOffset,
         emptyArrayUInt64() AS NegativeBucketCounts, toFloat64(0) AS MetricMin,
         toFloat64(0) AS MetricMax, toInt32(0) AS AggregationTemporality
  FROM otel_metrics_summary
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, toFloat64(0) AS ScalarValue, Attributes, ResourceAttributes,
         'exponential_histogram' AS MetricKind, Count AS MetricCount, Sum AS MetricSum,
         emptyArrayUInt64() AS BucketCounts, emptyArrayFloat64() AS ExplicitBounds,
         emptyArrayFloat64() AS Quantiles, emptyArrayFloat64() AS QuantileValues,
         Scale, ZeroCount, PositiveOffset, PositiveBucketCounts, NegativeOffset,
         NegativeBucketCounts, Min AS MetricMin, Max AS MetricMax,
         AggregationTemporality
  FROM otel_metrics_exp_histogram
)
WHERE TimeUnix >= {from:DateTime}
  AND TimeUnix < {to:DateTime}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({name:String} = '' OR MetricName = {name:String})
ORDER BY TimeUnix DESC, ServiceName ASC, MetricName ASC, MetricKind ASC
LIMIT {limit:UInt32}`

const sourcesQuery = `
SELECT ServiceName, Signal, max(LastReceived) AS LastReceived, sum(Volume) AS Volume
FROM (
  SELECT ServiceName, 'logs' AS Signal, max(Timestamp) AS LastReceived, count() AS Volume
  FROM otel_logs
  WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  GROUP BY ServiceName
  UNION ALL
  SELECT ServiceName, 'traces' AS Signal, max(Timestamp) AS LastReceived, count() AS Volume
  FROM otel_traces
  WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  GROUP BY ServiceName
  UNION ALL
	  SELECT ServiceName, 'metrics' AS Signal, max(TimeUnix) AS LastReceived, count() AS Volume
	  FROM otel_metrics_gauge
	  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
	  GROUP BY ServiceName
	  UNION ALL
	  SELECT ServiceName, 'metrics' AS Signal, max(TimeUnix) AS LastReceived, count() AS Volume
	  FROM otel_metrics_sum
	  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
	  GROUP BY ServiceName
	  UNION ALL
	  SELECT ServiceName, 'metrics' AS Signal, max(TimeUnix) AS LastReceived, count() AS Volume
	  FROM otel_metrics_histogram
	  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
	  GROUP BY ServiceName
	  UNION ALL
	  SELECT ServiceName, 'metrics' AS Signal, max(TimeUnix) AS LastReceived, count() AS Volume
	  FROM otel_metrics_summary
	  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
	  GROUP BY ServiceName
	  UNION ALL
	  SELECT ServiceName, 'metrics' AS Signal, max(TimeUnix) AS LastReceived, count() AS Volume
	  FROM otel_metrics_exp_histogram
	  WHERE TimeUnix >= {from_metrics:DateTime} AND TimeUnix < {to_metrics:DateTime}
	  GROUP BY ServiceName
)
GROUP BY ServiceName, Signal
ORDER BY LastReceived DESC, ServiceName ASC, Signal ASC
LIMIT {limit:UInt32}`
