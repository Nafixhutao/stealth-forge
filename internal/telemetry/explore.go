package telemetry

import (
	"context"
	"time"
)

// Explorer contains bounded, domain-specific queries used by the admin
// control room. It is intentionally separate from Store so existing API test
// doubles do not gain an accidental obligation to implement every new view.
type Explorer interface {
	QueryLogVolume(context.Context, LogVolumeQuery) (LogVolumeResult, error)
	QueryErrorGroups(context.Context, ErrorGroupsQuery) (ErrorGroupsResult, error)
	QueryServiceMap(context.Context, ServiceMapQuery) (ServiceMapResult, error)
}

// InfrastructureExplorer is kept separate from Explorer so a consumer that
// only needs logs, errors, or topology does not accidentally lose test
// doubles when infrastructure views evolve.
type InfrastructureExplorer interface {
	QueryInfrastructure(context.Context, InfrastructureQuery) (InfrastructureResult, error)
}

type OverviewExplorer interface {
	QueryHTTPOverview(context.Context, HTTPOverviewQuery) (HTTPOverviewResult, error)
}

// AlertExplorer exposes only the bounded measurements that the trusted alert
// worker can evaluate. It deliberately does not accept SQL, expressions, or
// arbitrary column names.
type AlertExplorer interface {
	EvaluateAlert(context.Context, AlertQuery) (AlertValue, error)
}

type AlertQuery struct {
	Kind        string
	Range       TimeRange
	Metric      string
	Service     string
	Level       string
	Search      string
	Aggregation string
	Percentile  string
}

type AlertValue struct {
	Value       float64
	SampleCount uint64
	Available   bool
}

type HTTPOverviewQuery struct {
	Range   TimeRange
	Service string
}

type HTTPOverviewResult struct {
	RequestRate  float64 `json:"request_rate"`
	ErrorRate    float64 `json:"error_rate"`
	P50LatencyMS float64 `json:"p50_latency_ms"`
	P95LatencyMS float64 `json:"p95_latency_ms"`
	P99LatencyMS float64 `json:"p99_latency_ms"`
	SampleCount  uint64  `json:"sample_count"`
}

type LogVolumeQuery struct {
	Range   TimeRange
	Service string
	Level   string
	Search  string
	Limit   int
}

type LogVolumeBucket struct {
	Timestamp time.Time `json:"timestamp"`
	Count     uint64    `json:"count"`
}

type LogVolumeResult struct {
	Items []LogVolumeBucket `json:"items"`
}

type ErrorGroupsQuery struct {
	Range   TimeRange
	Service string
	Search  string
	Limit   int
}

type ErrorGroup struct {
	Fingerprint     string    `json:"fingerprint"`
	Service         string    `json:"service"`
	ErrorType       string    `json:"error_type"`
	Message         string    `json:"message"`
	Status          string    `json:"status"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	OccurrenceCount uint64    `json:"occurrence_count"`
	TraceID         string    `json:"trace_id,omitempty"`
}

type ErrorGroupsResult struct {
	Items []ErrorGroup `json:"items"`
}

type ServiceMapQuery struct {
	Range TimeRange
	Limit int
}

type ServiceMapEdge struct {
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	RequestCount uint64  `json:"request_count"`
	ErrorCount   uint64  `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
	P95LatencyMS float64 `json:"p95_latency_ms"`
}

type ServiceMapResult struct {
	Items []ServiceMapEdge `json:"items"`
}

type InfrastructureQuery struct {
	Range TimeRange
	Scope string
	Limit int
}

type InfrastructureMetric struct {
	Timestamp          time.Time         `json:"timestamp"`
	Scope              string            `json:"scope"`
	Name               string            `json:"name"`
	Service            string            `json:"service"`
	Value              float64           `json:"value"`
	Attributes         map[string]string `json:"attributes,omitempty"`
	ResourceAttributes map[string]string `json:"resource_attributes,omitempty"`
}

type InfrastructureResult struct {
	Items []InfrastructureMetric `json:"items"`
}
