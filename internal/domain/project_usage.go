package domain

import "time"

// ProjectUsage is a point-in-time aggregate for the project Console. Values
// are derived from tenant-owned PostgreSQL rows; no request or billing data is
// guessed when the underlying subsystem has no durable counter yet.
type ProjectUsage struct {
	ProjectID                  string    `json:"project_id"`
	CapturedAt                 time.Time `json:"captured_at"`
	ApplicationUsers           int64     `json:"application_users"`
	DatabaseCount              int64     `json:"database_count"`
	DatabaseTableCount         int64     `json:"database_table_count"`
	DatabaseRowCount           int64     `json:"database_row_count"`
	StorageFileCount           int64     `json:"storage_file_count"`
	StorageBytes               int64     `json:"storage_bytes"`
	StorageQuotaBytes          int64     `json:"storage_quota_bytes"`
	FunctionCount              int64     `json:"function_count"`
	FunctionArtifactBytes      int64     `json:"function_artifact_bytes"`
	FunctionQuotaBytes         int64     `json:"function_quota_bytes"`
	SiteCount                  int64     `json:"site_count"`
	SiteArtifactBytes          int64     `json:"site_artifact_bytes"`
	SiteReservedBytes          int64     `json:"site_reserved_bytes"`
	SiteQuotaBytes             int64     `json:"site_quota_bytes"`
	RealtimeEventCount         int64     `json:"realtime_event_count"`
	WebhookDeliveryCount7      int64     `json:"webhook_delivery_count_7d"`
	APIRequestCount30D         int64     `json:"api_request_count_30d"`
	APIEgressBytes30D          int64     `json:"api_egress_bytes_30d"`
	FunctionInvocationCount30D int64     `json:"function_invocation_count_30d"`
	FunctionFailureCount30D    int64     `json:"function_failure_count_30d"`
	FunctionComputeMS30D       int64     `json:"function_compute_ms_30d"`
}

// ProjectUsageDay is one durable UTC calendar bucket. Missing days are not
// synthesized; callers can fill gaps when rendering charts without confusing
// unavailable data with a measured zero.
type ProjectUsageDay struct {
	Date                    string `json:"date"`
	APIRequestCount         int64  `json:"api_request_count"`
	APIEgressBytes          int64  `json:"api_egress_bytes"`
	FunctionInvocationCount int64  `json:"function_invocation_count"`
	FunctionFailureCount    int64  `json:"function_failure_count"`
	FunctionComputeMS       int64  `json:"function_compute_ms"`
}

type ProjectUsageMetering struct {
	ProjectID string            `json:"project_id"`
	From      string            `json:"from"`
	To        string            `json:"to"`
	Days      []ProjectUsageDay `json:"days"`
	Totals    ProjectUsageDay   `json:"totals"`
}
