package telemetry

import (
	"fmt"
	"strings"
)

func metricAlertStatement(aggregation string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(aggregation)) {
	case "", "avg":
		return metricAlertAverageQuery, nil
	case "max":
		return metricAlertMaximumQuery, nil
	case "min":
		return metricAlertMinimumQuery, nil
	case "sum":
		return metricAlertSumQuery, nil
	case "latest":
		return metricAlertLatestQuery, nil
	default:
		return "", fmt.Errorf("%w: unsupported metric aggregation", ErrInvalidQuery)
	}
}

// Infrastructure metrics are intentionally selected by a fixed allowlist of
// signal namespaces. The browser can choose a scope, but it cannot turn this
// endpoint into an arbitrary ClickHouse metric query.
const infrastructureQuery = `
SELECT TimeUnix,
       multiIf(startsWith(MetricName, 'container.'), 'containers',
               startsWith(MetricName, 'postgresql.'), 'postgres',
               startsWith(MetricName, 'redis.'), 'redis',
               startsWith(MetricName, 'system.'), 'host',
               startsWith(MetricName, 'process.'), 'host',
               startsWith(MetricName, 'load.'), 'host',
               startsWith(MetricName, 'disk.'), 'host',
               startsWith(MetricName, 'filesystem.'), 'host',
               startsWith(MetricName, 'network.'), 'host',
               startsWith(MetricName, 'paging.'), 'host',
               'services') AS Scope,
       MetricName, ServiceName, Value, Attributes, ResourceAttributes
FROM (
  SELECT TimeUnix, MetricName, ServiceName, Value, Attributes, ResourceAttributes
  FROM otel_metrics_gauge
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
  UNION ALL
  SELECT TimeUnix, MetricName, ServiceName, Value, Attributes, ResourceAttributes
  FROM otel_metrics_sum
  WHERE TimeUnix >= {from:DateTime} AND TimeUnix < {to:DateTime}
)
WHERE (startsWith(MetricName, 'system.')
    OR startsWith(MetricName, 'process.')
    OR startsWith(MetricName, 'load.')
    OR startsWith(MetricName, 'disk.')
    OR startsWith(MetricName, 'filesystem.')
    OR startsWith(MetricName, 'network.')
    OR startsWith(MetricName, 'paging.')
    OR startsWith(MetricName, 'container.')
    OR startsWith(MetricName, 'postgresql.')
    OR startsWith(MetricName, 'redis.')
    OR startsWith(MetricName, 'stealth_'))
  AND ({scope:String} = '' OR Scope = {scope:String})
ORDER BY TimeUnix DESC, Scope ASC, ServiceName ASC, MetricName ASC
LIMIT {limit:UInt32}`

const logVolumeMinuteQuery = `
SELECT toStartOfInterval(Timestamp, INTERVAL 1 MINUTE) AS Bucket, count() AS Count
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
GROUP BY Bucket ORDER BY Bucket ASC LIMIT {limit:UInt32}`

const logVolumeFiveMinuteQuery = `
SELECT toStartOfInterval(Timestamp, INTERVAL 5 MINUTE) AS Bucket, count() AS Count
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
GROUP BY Bucket ORDER BY Bucket ASC LIMIT {limit:UInt32}`

const logVolumeQuarterHourQuery = `
SELECT toStartOfInterval(Timestamp, INTERVAL 15 MINUTE) AS Bucket, count() AS Count
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
GROUP BY Bucket ORDER BY Bucket ASC LIMIT {limit:UInt32}`

const logVolumeHourQuery = `
SELECT toStartOfInterval(Timestamp, INTERVAL 1 HOUR) AS Bucket, count() AS Count
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
GROUP BY Bucket ORDER BY Bucket ASC LIMIT {limit:UInt32}`

const logVolumeDayQuery = `
SELECT toStartOfInterval(Timestamp, INTERVAL 1 DAY) AS Bucket, count() AS Count
FROM otel_logs
WHERE Timestamp >= {from:DateTime64(9)} AND Timestamp < {to:DateTime64(9)}
  AND ({service:String} = '' OR ServiceName = {service:String})
  AND ({level:String} = '' OR SeverityText = {level:String})
  AND ({search:String} = '' OR positionCaseInsensitiveUTF8(Body, {search:String}) > 0)
GROUP BY Bucket ORDER BY Bucket ASC LIMIT {limit:UInt32}`
