package domain

import "time"

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationPlan is the safe billing projection exposed to Console
// operators. Commercial plan definitions are server-owned; tenant responses
// contain the effective limits and current counts but never provider secrets
// or payment identifiers.
type OrganizationPlan struct {
	OrganizationID     string                 `json:"organization_id"`
	PlanKey            string                 `json:"plan_key"`
	Status             string                 `json:"status"`
	CurrentPeriodStart string                 `json:"current_period_start"`
	CurrentPeriodEnd   string                 `json:"current_period_end"`
	Limits             OrganizationPlanLimits `json:"limits"`
	Usage              OrganizationPlanUsage  `json:"usage"`
}

type OrganizationPlanLimits struct {
	Projects       int64 `json:"projects"`
	Members        int64 `json:"members"`
	Databases      int64 `json:"databases"`
	StorageBuckets int64 `json:"storage_buckets"`
	Functions      int64 `json:"functions"`
	Sites          int64 `json:"sites"`
	Apps           int64 `json:"apps"`
}

type OrganizationPlanUsage struct {
	Projects       int64 `json:"projects"`
	Members        int64 `json:"members"`
	Databases      int64 `json:"databases"`
	StorageBuckets int64 `json:"storage_buckets"`
	Functions      int64 `json:"functions"`
	Sites          int64 `json:"sites"`
	Apps           int64 `json:"apps"`
}

type Membership struct {
	OrganizationID string    `json:"organization_id"`
	AccountID      string    `json:"account_id"`
	Email          *string   `json:"email,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	ProviderLogin  string    `json:"provider_login,omitempty"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
}

// OrganizationInvitation is the safe projection of a pending or historical
// organization invitation. The opaque token and its hash are deliberately not
// part of this DTO.
type OrganizationInvitation struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organization_id"`
	Email              string     `json:"email"`
	Role               string     `json:"role"`
	InvitedByAccountID *string    `json:"invited_by_account_id,omitempty"`
	InvitedByEmail     *string    `json:"invited_by_email,omitempty"`
	Status             string     `json:"status"`
	ExpiresAt          time.Time  `json:"expires_at"`
	AcceptedAt         *time.Time `json:"accepted_at,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// OrganizationIncident is a durable operational record visible to every
// organization member. Updates contain the operator-authored timeline; no
// provider credentials or telemetry payloads are embedded here.
type OrganizationIncident struct {
	ID                 string                       `json:"id"`
	OrganizationID     string                       `json:"organization_id"`
	CreatedByAccountID *string                      `json:"created_by_account_id,omitempty"`
	CreatedByEmail     *string                      `json:"created_by_email,omitempty"`
	Title              string                       `json:"title"`
	Severity           string                       `json:"severity"`
	Status             string                       `json:"status"`
	Services           []string                     `json:"services"`
	StartedAt          time.Time                    `json:"started_at"`
	ResolvedAt         *time.Time                   `json:"resolved_at,omitempty"`
	Updates            []OrganizationIncidentUpdate `json:"updates"`
	CreatedAt          time.Time                    `json:"created_at"`
	UpdatedAt          time.Time                    `json:"updated_at"`
}

type OrganizationIncidentUpdate struct {
	ID              string    `json:"id"`
	IncidentID      string    `json:"incident_id"`
	AuthorAccountID *string   `json:"author_account_id,omitempty"`
	AuthorEmail     *string   `json:"author_email,omitempty"`
	Status          string    `json:"status"`
	Message         string    `json:"message"`
	CreatedAt       time.Time `json:"created_at"`
}
