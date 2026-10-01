package telemetry

import (
	"context"
	"fmt"
	"time"
)

type TimeRange struct {
	From time.Time
	To   time.Time
}

func (r TimeRange) validate(maxRange time.Duration) error {
	if r.From.IsZero() || r.To.IsZero() || r.From.Location() == nil || r.To.Location() == nil {
		return fmt.Errorf("%w: from and to are required", ErrInvalidQuery)
	}
	from, to := r.From.UTC(), r.To.UTC()
	if !to.After(from) {
		return fmt.Errorf("%w: to must be after from", ErrInvalidQuery)
	}
	if maxRange > 0 && to.Sub(from) > maxRange {
		return fmt.Errorf("%w: time range exceeds the configured limit", ErrInvalidQuery)
	}
	if to.After(time.Now().UTC().Add(5 * time.Minute)) {
		return fmt.Errorf("%w: time range cannot extend far into the future", ErrInvalidQuery)
	}
	return nil
}

type LogsQuery struct {
	Range    TimeRange
	Service  string
	Level    string
	Search   string
	Limit    int
	After    *LogCursor
	LiveTail bool
}

type LogRecord struct {
	Timestamp          time.Time         `json:"timestamp"`
	TraceID            string            `json:"trace_id,omitempty"`
	SpanID             string            `json:"span_id,omitempty"`
	Severity           string            `json:"level,omitempty"`
	Service            string            `json:"service"`
	Body               string            `json:"message"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	ResourceAttributes map[string]string `json:"resource_attributes,omitempty"`
	EventID            string            `json:"-"`
}

type LogsResult struct {
	Items []LogRecord `json:"items"`
}

// ContainerLogsQuery is the narrowly scoped App runtime-log query. Container
// IDs must come from the PostgreSQL runtime-source registry, never a caller.
type ContainerLogsQuery struct {
	ContainerIDs []string
	Range        TimeRange
	Level        string
	Search       string
	Limit        int
	After        *LogCursor
}

type ContainerLogRecord struct {
	Timestamp time.Time
	Severity  string
	Body      string
	EventID   string
}

type ContainerLogsResult struct {
	Items []ContainerLogRecord
}

// ContainerLogReader is an optional narrow capability so existing telemetry
// stores/fakes do not need App-specific query methods.
type ContainerLogReader interface {
	QueryContainerLogs(context.Context, ContainerLogsQuery) (ContainerLogsResult, error)
}

type TracesQuery struct {
	Range   TimeRange
	Service string
	TraceID string
	MinMs   float64
	Limit   int
}

type SpanRecord struct {
	Timestamp          time.Time         `json:"timestamp"`
	TraceID            string            `json:"trace_id"`
	SpanID             string            `json:"span_id"`
	ParentSpanID       string            `json:"parent_span_id,omitempty"`
	Name               string            `json:"name"`
	Kind               string            `json:"kind"`
	Service            string            `json:"service"`
	DurationNs         uint64            `json:"duration_ns"`
	Status             string            `json:"status"`
	StatusMessage      string            `json:"status_message,omitempty"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	ResourceAttributes map[string]string `json:"resource_attributes,omitempty"`
}

type TracesResult struct {
	Items []SpanRecord `json:"items"`
}

type MetricsQuery struct {
	Range   TimeRange
	Service string
	Name    string
	Limit   int
}

type MetricRecord struct {
	Timestamp          time.Time                   `json:"timestamp"`
	Name               string                      `json:"name"`
	Service            string                      `json:"service"`
	Value              *float64                    `json:"value,omitempty"`
	Kind               string                      `json:"kind"`
	Attributes         map[string]string           `json:"attributes,omitempty"`
	ResourceAttributes map[string]string           `json:"resource_attributes,omitempty"`
	Histogram          *MetricHistogram            `json:"histogram,omitempty"`
	Summary            *MetricSummary              `json:"summary,omitempty"`
	Exponential        *MetricExponentialHistogram `json:"exponential_histogram,omitempty"`
}

type MetricHistogram struct {
	Count                  uint64    `json:"count"`
	Sum                    float64   `json:"sum"`
	BucketCounts           []uint64  `json:"bucket_counts"`
	ExplicitBounds         []float64 `json:"explicit_bounds"`
	Min                    *float64  `json:"min,omitempty"`
	Max                    *float64  `json:"max,omitempty"`
	AggregationTemporality int32     `json:"aggregation_temporality"`
}

type MetricQuantile struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}

type MetricSummary struct {
	Count                  uint64           `json:"count"`
	Sum                    float64          `json:"sum"`
	Quantiles              []MetricQuantile `json:"quantiles"`
	AggregationTemporality int32            `json:"aggregation_temporality,omitempty"`
}

type MetricExponentialHistogram struct {
	Count                  uint64   `json:"count"`
	Sum                    float64  `json:"sum"`
	Scale                  int32    `json:"scale"`
	ZeroCount              uint64   `json:"zero_count"`
	PositiveOffset         int32    `json:"positive_offset"`
	PositiveBucketCounts   []uint64 `json:"positive_bucket_counts"`
	NegativeOffset         int32    `json:"negative_offset"`
	NegativeBucketCounts   []uint64 `json:"negative_bucket_counts"`
	Min                    *float64 `json:"min,omitempty"`
	Max                    *float64 `json:"max,omitempty"`
	AggregationTemporality int32    `json:"aggregation_temporality"`
}

type MetricsResult struct {
	Items []MetricRecord `json:"items"`
}

type SourcesQuery struct {
	Range TimeRange
	Limit int
}

type SourceRecord struct {
	Service      string    `json:"service"`
	Signal       string    `json:"signal"`
	LastReceived time.Time `json:"last_received"`
	Volume       uint64    `json:"volume"`
}

type SourcesResult struct {
	Items []SourceRecord `json:"items"`
}
