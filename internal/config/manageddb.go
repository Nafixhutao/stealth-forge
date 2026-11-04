package config

import (
	"fmt"
	"strconv"
	"strings"
)

// managedDatabaseSettings owns the hosted MySQL free tier. The feature is
// opt-in so installations that have not provisioned the tenant cluster keep
// starting with the routes disabled.
type managedDatabaseSettings struct {
	enabled       bool
	adminDSN      string
	secretKey     []byte
	host          string
	port          int
	phpMyAdminURL string
	hostname      string
	container     string
}

func loadManagedDatabaseSettings() (managedDatabaseSettings, error) {
	enabled := strings.EqualFold(strings.TrimSpace(value("MANAGED_DATABASES_ENABLED", "false")), "true")
	settings := managedDatabaseSettings{
		enabled:       enabled,
		adminDSN:      strings.TrimSpace(value("TENANT_MYSQL_ADMIN_DSN", "")),
		host:          strings.TrimSpace(value("TENANT_MYSQL_HOST", "tenant-mysql")),
		phpMyAdminURL: strings.TrimSpace(value("TENANT_PHPMYADMIN_URL", "/db/")),
		hostname:      strings.ToLower(strings.TrimSpace(value("MANAGED_DATABASE_HOSTNAME", ""))),
		container:     strings.TrimSpace(value("TENANT_MYSQL_CONTAINER", "stealth-tenant-mysql-1")),
	}
	port, err := strconv.Atoi(strings.TrimSpace(value("TENANT_MYSQL_PORT", "3306")))
	if err != nil || port < 1 || port > 65535 {
		return managedDatabaseSettings{}, fmt.Errorf("TENANT_MYSQL_PORT must be a port between 1 and 65535")
	}
	settings.port = port
	secretKey, err := loadOptionalSecretKey("MANAGED_DATABASE_SECRET_KEY")
	if err != nil {
		return managedDatabaseSettings{}, err
	}
	settings.secretKey = secretKey
	if !enabled {
		return settings, nil
	}
	if settings.adminDSN == "" {
		return managedDatabaseSettings{}, fmt.Errorf("TENANT_MYSQL_ADMIN_DSN is required when MANAGED_DATABASES_ENABLED is true")
	}
	if len(settings.secretKey) != 32 {
		return managedDatabaseSettings{}, fmt.Errorf("MANAGED_DATABASE_SECRET_KEY must be base64-encoded 32 bytes")
	}
	if settings.host == "" {
		return managedDatabaseSettings{}, fmt.Errorf("TENANT_MYSQL_HOST must not be empty")
	}
	if settings.phpMyAdminURL == "" {
		return managedDatabaseSettings{}, fmt.Errorf("TENANT_PHPMYADMIN_URL must not be empty")
	}
	return settings, nil
}

func (s managedDatabaseSettings) apply(c *Config) {
	c.ManagedDatabasesEnabled = s.enabled
	c.TenantMySQLAdminDSN = s.adminDSN
	c.ManagedDatabaseSecretKey = cloneBytes(s.secretKey)
	c.TenantMySQLHost = s.host
	c.TenantMySQLPort = s.port
	c.TenantPHPMyAdminURL = s.phpMyAdminURL
	c.ManagedDatabaseHostname = s.hostname
	c.TenantMySQLContainer = s.container
}
