package domain

import "time"

type Project struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProjectServiceLayout stores the durable editor position for one resource in
// a project's Services canvas. Resource IDs are validated against their
// project-owned table by the repository because the resource types are
// intentionally represented by one polymorphic projection table.
type ProjectServiceLayout struct {
	ProjectID    string    `json:"project_id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ApplicationUser is a user belonging to a project application. It is
// intentionally separate from Account, which represents a Console operator.
// Password hashes are never part of this DTO.
type ApplicationUser struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"project_id"`
	Email         string    `json:"email"`
	Name          *string   `json:"name"`
	Status        string    `json:"status"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProjectAuthSettings struct {
	ProjectID           string    `json:"project_id"`
	RegistrationEnabled bool      `json:"registration_enabled"`
	CORSOrigins         []string  `json:"cors_origins"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ProjectAPIKey is the safe management projection. Secret and hash material
// are deliberately absent; the full secret is returned only at creation.
type ProjectAPIKey struct {
	ID         string     `json:"id"`
	ProjectID  string     `json:"project_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ProjectAPIKeyAuth is internal request actor data. It is never serialized.
type ProjectAPIKeyAuth struct {
	ID         string
	ProjectID  string
	Scopes     []string
	LastUsedAt *time.Time
}
