package domain

// InstanceDomainSettings separates the Console hostname derived from
// PUBLIC_APP_URL from the optional workload-hosting suffix stored in
// PostgreSQL.
type InstanceDomainSettings struct {
	InstanceHostname   string  `json:"instance_hostname"`
	WorkloadBaseDomain *string `json:"workload_base_domain"`
}

// PlatformRoute is the PostgreSQL-derived desired state consumed by the
// Traefik file-provider reconciler. It contains no filesystem or router
// implementation details.
type PlatformRoute struct {
	SiteID   string
	Hostname string
}

// AppPlatformRoute is private worker input for the generated App route file.
// RouteIdentity is the trusted runtime incarnation used to derive the backend
// target; it is not exposed by the App API.
type AppPlatformRoute struct {
	AppID         string
	RouteIdentity string
	Hostname      string
	Port          int
}
