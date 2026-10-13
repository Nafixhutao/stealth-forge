package domain

import "time"

type Account struct {
	ID            string `json:"id"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified"`
	// InstanceRole is intentionally separate from organization membership. An
	// instance owner is not implicitly a member of every organization.
	InstanceRole   string    `json:"instance_role,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	ProviderUserID string    `json:"provider_user_id,omitempty"`
	ProviderLogin  string    `json:"provider_login,omitempty"`
	DisplayName    string    `json:"display_name,omitempty"`
	AvatarURL      string    `json:"avatar_url,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AccountIdentity is one linked external sign-in identity. It never carries
// provider secrets; the provider user id is an opaque provider-side id.
type AccountIdentity struct {
	Provider      string    `json:"provider"`
	ProviderLogin string    `json:"provider_login,omitempty"`
	DisplayName   string    `json:"display_name,omitempty"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// AdminAccount is the Admin Console's directory row for one Console account.
// It carries no password material and no provider secrets; the email is the
// account's own address and is masked by the Console before display.
type AdminAccount struct {
	ID               string    `json:"id"`
	Email            string    `json:"email"`
	EmailVerified    bool      `json:"email_verified"`
	InstanceRole     string    `json:"instance_role,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	Providers        []string  `json:"providers"`
	AvatarURL        string    `json:"avatar_url,omitempty"`
	DisplayName      string    `json:"display_name,omitempty"`
	LastSignInAt     string    `json:"last_sign_in_at,omitempty"`
	LastSignInMethod string    `json:"last_sign_in_method,omitempty"`
}

// ConsoleSession is the safe, non-secret projection of a Console session.
// The bearer token and its hash are never returned to callers.
type ConsoleSession struct {
	ID        string    `json:"id"`
	IsCurrent bool      `json:"is_current"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
