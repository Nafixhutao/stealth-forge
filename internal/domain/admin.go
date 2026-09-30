package domain

import "time"

// AdminOperation is a safe, instance-wide projection of durable work already
// owned by the control plane. It deliberately joins existing deployment,
// execution, Agent, cleanup, and backup records instead of introducing a
// second operations ledger.
type AdminOperation struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Name        string     `json:"name"`
	ProjectID   *string    `json:"project_id,omitempty"`
	ProjectName *string    `json:"project_name,omitempty"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	DurationMS  int64      `json:"duration_ms"`
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AdminOperationSummary contains only counts derived from durable control
// plane state. A zero value means no work is currently represented, not a
// fabricated telemetry measurement.
type AdminOperationSummary struct {
	ActiveDeployments int64 `json:"active_deployments"`
	QueuedJobs        int64 `json:"queued_jobs"`
	RunningJobs       int64 `json:"running_jobs"`
	FailedJobs        int64 `json:"failed_jobs"`
}

// AdminMonitor is the safe instance-level projection of a persisted monitor.
// Secret headers, request bodies, heartbeat tokens, and provider credentials
// are held encrypted by the control plane and never appear in this DTO.
type AdminMonitor struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Kind               string         `json:"kind"`
	Target             string         `json:"target"`
	IntervalSeconds    int            `json:"interval_seconds"`
	TimeoutMS          int            `json:"timeout_ms"`
	Enabled            bool           `json:"enabled"`
	PublicConfig       map[string]any `json:"config"`
	Status             string         `json:"status"`
	LastCheckedAt      *time.Time     `json:"last_checked_at,omitempty"`
	LastSuccessAt      *time.Time     `json:"last_success_at,omitempty"`
	LastFailureAt      *time.Time     `json:"last_failure_at,omitempty"`
	LastLatencyMS      *int64         `json:"last_latency_ms,omitempty"`
	LastStatusCode     *int           `json:"last_status_code,omitempty"`
	LastError          *string        `json:"last_error,omitempty"`
	LastHeartbeatAt    *time.Time     `json:"last_heartbeat_at,omitempty"`
	NextCheckAt        time.Time      `json:"next_check_at"`
	SecretConfigured   bool           `json:"secret_configured"`
	CreatedByAccountID *string        `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// AdminMonitorCheck is a bounded monitor execution record. Details contain
// only protocol-safe facts, never response bodies or credentials.
type AdminMonitorCheck struct {
	ID         string         `json:"id"`
	MonitorID  string         `json:"monitor_id"`
	CheckedAt  time.Time      `json:"checked_at"`
	Success    bool           `json:"success"`
	LatencyMS  int64          `json:"latency_ms"`
	StatusCode *int           `json:"status_code,omitempty"`
	Error      *string        `json:"error,omitempty"`
	Details    map[string]any `json:"details"`
}

// AdminAlertRule is the durable, owner-configured alert definition and its
// current state. Condition is constrained by the API and evaluated by the
// trusted worker; it is not an arbitrary SQL or expression payload.
type AdminAlertRule struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Kind               string         `json:"kind"`
	Condition          map[string]any `json:"condition"`
	Severity           string         `json:"severity"`
	ForSeconds         int            `json:"for_seconds"`
	Enabled            bool           `json:"enabled"`
	State              string         `json:"state"`
	PendingSince       *time.Time     `json:"pending_since,omitempty"`
	LastEvaluatedAt    *time.Time     `json:"last_evaluated_at,omitempty"`
	LastValue          *float64       `json:"last_value,omitempty"`
	LastError          *string        `json:"last_error,omitempty"`
	CreatedByAccountID *string        `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type AdminAlertEvent struct {
	ID               string    `json:"id"`
	RuleID           string    `json:"rule_id"`
	RuleName         string    `json:"rule_name"`
	RuleKind         string    `json:"rule_kind"`
	Severity         string    `json:"severity"`
	State            string    `json:"state"`
	Value            *float64  `json:"value,omitempty"`
	Message          string    `json:"message"`
	OccurredAt       time.Time `json:"occurred_at"`
	SourceRuleExists bool      `json:"source_rule_exists"`
}

// AdminNotificationChannel never exposes its encrypted configuration. The
// worker decrypts it only while constructing a bounded delivery request.
type AdminNotificationChannel struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	Kind               string     `json:"kind"`
	Enabled            bool       `json:"enabled"`
	SecretConfigured   bool       `json:"secret_configured"`
	LastDeliveryAt     *time.Time `json:"last_delivery_at,omitempty"`
	LastDeliveryStatus *string    `json:"last_delivery_status,omitempty"`
	LastError          *string    `json:"last_error,omitempty"`
	CreatedByAccountID *string    `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type AdminNotificationDelivery struct {
	ID           string     `json:"id"`
	ChannelID    string     `json:"channel_id"`
	AlertEventID *string    `json:"alert_event_id,omitempty"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	AvailableAt  time.Time  `json:"available_at"`
	LastError    *string    `json:"last_error,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AdminIncident struct {
	ID                 string               `json:"id"`
	Title              string               `json:"title"`
	Severity           string               `json:"severity"`
	Status             string               `json:"status"`
	Services           []string             `json:"services"`
	StartedAt          time.Time            `json:"started_at"`
	ResolvedAt         *time.Time           `json:"resolved_at,omitempty"`
	CreatedByAccountID *string              `json:"created_by_account_id,omitempty"`
	Events             []AdminIncidentEvent `json:"events"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

type AdminIncidentEvent struct {
	ID             string    `json:"id"`
	IncidentID     string    `json:"incident_id"`
	Kind           string    `json:"kind"`
	Message        string    `json:"message"`
	ActorAccountID *string   `json:"actor_account_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type AdminDashboard struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	Definition         map[string]any `json:"definition"`
	CreatedByAccountID *string        `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type AdminStatusPage struct {
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	IsPublic           bool             `json:"is_public"`
	Components         []map[string]any `json:"components"`
	PublishedIncidents []string         `json:"published_incidents"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// AdminPublicStatusPage is deliberately smaller than the owner configuration
// projection. It contains only components and incidents explicitly published
// by the owner; incident notes, audit data, and internal diagnostics stay
// private.
type AdminPublicStatusPage struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Components  []map[string]any      `json:"components"`
	Incidents   []AdminPublicIncident `json:"incidents"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

type AdminPublicIncident struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Severity   string     `json:"severity"`
	Status     string     `json:"status"`
	Services   []string   `json:"services"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
