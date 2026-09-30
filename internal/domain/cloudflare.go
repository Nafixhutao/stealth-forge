package domain

import "time"

// CloudflareConnection is the private, instance-scoped provider identity used
// by the trusted worker. APIToken is populated only after decryption and must
// never be serialized or included in API-facing projections.
type CloudflareConnection struct {
	AccountID                   string     `json:"-"`
	ConsoleZoneID               string     `json:"-"`
	ConsoleHostname             string     `json:"-"`
	TunnelID                    string     `json:"-"`
	TunnelName                  string     `json:"-"`
	ConsoleRecordID             string     `json:"-"`
	APIToken                    string     `json:"-"`
	WorkloadZoneID              string     `json:"-"`
	WorkloadZoneName            string     `json:"-"`
	WildcardHostname            string     `json:"-"`
	WildcardRecordID            string     `json:"-"`
	Status                      string     `json:"-"`
	EdgeTLSStatus               string     `json:"-"`
	EdgeTLSError                string     `json:"-"`
	ConsoleOriginDesired        string     `json:"-"`
	ConsoleOriginObserved       string     `json:"-"`
	ConsoleOriginStatus         string     `json:"-"`
	ConsoleOriginLastError      string     `json:"-"`
	ConsoleOriginUpdatedAt      *time.Time `json:"-"`
	ConsolePublicVerifiedAt     *time.Time `json:"-"`
	ConsolePublicVerifiedOrigin string     `json:"-"`
	LastReconciledAt            *time.Time `json:"-"`
	LastError                   string     `json:"-"`
	ConfiguredAt                *time.Time `json:"-"`
	UpdatedAt                   time.Time  `json:"-"`
	WorkloadBaseDomain          *string    `json:"-"`
}

type CloudflareRetiringWildcardDNS struct {
	RecordID string
	ZoneID   string
	Hostname string
	Target   string
}

// CloudflareRoutingStatus is the safe admin response. It intentionally has
// no token, ciphertext, tunnel token, account secret, or raw provider body.
type CloudflareRoutingStatus struct {
	Configured                  bool       `json:"configured"`
	Status                      string     `json:"status"`
	ConsoleHostname             string     `json:"console_hostname,omitempty"`
	WorkloadHostname            *string    `json:"workload_hostname"`
	Zone                        string     `json:"zone,omitempty"`
	EdgeTLSStatus               string     `json:"edge_tls_status"`
	EdgeTLSError                string     `json:"edge_tls_error,omitempty"`
	ConsoleOriginDesired        string     `json:"console_origin_desired"`
	ConsoleOriginObserved       string     `json:"console_origin_observed"`
	ConsoleOriginStatus         string     `json:"console_origin_status"`
	ConsoleOriginLastError      string     `json:"console_origin_last_error,omitempty"`
	ConsoleOriginUpdatedAt      *time.Time `json:"console_origin_updated_at,omitempty"`
	ConsolePublicVerifiedAt     *time.Time `json:"console_public_verified_at,omitempty"`
	ConsolePublicVerifiedOrigin string     `json:"console_public_verified_origin,omitempty"`
	LastReconciledAt            *time.Time `json:"last_reconciled_at,omitempty"`
	LastError                   string     `json:"last_error,omitempty"`
}

type CloudflareRoutingUpdate struct {
	ExpectedWorkloadBaseDomain *string
	WorkloadZoneID             string
	WorkloadZoneName           string
	WildcardHostname           string
	WildcardRecordID           string
	EdgeTLSStatus              string
	EdgeTLSError               string
	ConsoleOriginDesired       string
	ConsoleOriginObserved      string
}
