package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// databaseSettings owns the database connection environment contract. The
// application still receives one Config snapshot, but pool sizing and
// connection lifetime rules stay together instead of being interleaved with
// unrelated HTTP, auth, and storage settings.
type databaseSettings struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	// StatementTimeout and IdleInTransactionTimeout bound a stuck query or an
	// idle transaction so a single operation cannot pin a pooled connection
	// indefinitely. Zero disables the corresponding PostgreSQL setting.
	StatementTimeout         time.Duration
	IdleInTransactionTimeout time.Duration
}

func loadDatabaseSettings() (databaseSettings, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return databaseSettings{}, fmt.Errorf("DATABASE_URL is required")
	}
	maxConns, err := boundedInt32("DATABASE_MAX_CONNS", "16", 1, 256)
	if err != nil {
		return databaseSettings{}, err
	}
	minConns, err := boundedInt32("DATABASE_MIN_CONNS", "2", 0, maxConns)
	if err != nil {
		return databaseSettings{}, err
	}
	maxConnLifetime, err := time.ParseDuration(value("DATABASE_MAX_CONN_LIFETIME", "1h"))
	if err != nil || maxConnLifetime <= 0 || maxConnLifetime > 7*24*time.Hour {
		return databaseSettings{}, fmt.Errorf(
			"DATABASE_MAX_CONN_LIFETIME must be a positive duration no longer than 168h",
		)
	}
	maxConnIdleTime, err := time.ParseDuration(value("DATABASE_MAX_CONN_IDLE_TIME", "30m"))
	if err != nil || maxConnIdleTime <= 0 || maxConnIdleTime > 7*24*time.Hour {
		return databaseSettings{}, fmt.Errorf(
			"DATABASE_MAX_CONN_IDLE_TIME must be a positive duration no longer than 168h",
		)
	}
	// The default statement timeout (15m) is above the 10m migration apply cap
	// so it bounds a genuinely stuck worker query without failing a coordinated
	// upgrade. "0" disables the setting.
	statementTimeout, err := parseDatabaseTimeout("DATABASE_STATEMENT_TIMEOUT", "15m")
	if err != nil {
		return databaseSettings{}, err
	}
	idleInTransactionTimeout, err := parseDatabaseTimeout("DATABASE_IDLE_IN_TRANSACTION_TIMEOUT", "60s")
	if err != nil {
		return databaseSettings{}, err
	}
	return databaseSettings{
		URL:                      databaseURL,
		MaxConns:                 maxConns,
		MinConns:                 minConns,
		MaxConnLifetime:          maxConnLifetime,
		MaxConnIdleTime:          maxConnIdleTime,
		StatementTimeout:         statementTimeout,
		IdleInTransactionTimeout: idleInTransactionTimeout,
	}, nil
}

// parseDatabaseTimeout accepts a PostgreSQL session timeout. "0" disables the
// setting; any other value must be a positive duration no longer than 24h.
func parseDatabaseTimeout(name, fallback string) (time.Duration, error) {
	raw := value(name, fallback)
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 || parsed > 24*time.Hour {
		return 0, fmt.Errorf("%s must be a duration between 0 and 24h (0 disables it)", name)
	}
	return parsed, nil
}

func (s databaseSettings) apply(c *Config) {
	c.DatabaseURL = s.URL
	c.DatabaseMaxConns = s.MaxConns
	c.DatabaseMinConns = s.MinConns
	c.DatabaseMaxConnLifetime = s.MaxConnLifetime
	c.DatabaseMaxConnIdleTime = s.MaxConnIdleTime
	c.DatabaseStatementTimeout = s.StatementTimeout
	c.DatabaseIdleInTransactionTimeout = s.IdleInTransactionTimeout
}

func (c *Config) applyDatabaseDefaults() {
	if c.DatabaseMaxConns <= 0 {
		c.DatabaseMaxConns = 16
	}
	if c.DatabaseMaxConns > 256 {
		c.DatabaseMaxConns = 256
	}
	if c.DatabaseMinConns < 0 {
		c.DatabaseMinConns = 0
	}
	if c.DatabaseMinConns > c.DatabaseMaxConns {
		c.DatabaseMinConns = c.DatabaseMaxConns
	}
	if c.DatabaseMaxConnLifetime <= 0 {
		c.DatabaseMaxConnLifetime = time.Hour
	}
	if c.DatabaseMaxConnIdleTime <= 0 {
		c.DatabaseMaxConnIdleTime = 30 * time.Minute
	}
	if c.DatabaseStatementTimeout <= 0 {
		c.DatabaseStatementTimeout = 15 * time.Minute
	}
	if c.DatabaseIdleInTransactionTimeout <= 0 {
		c.DatabaseIdleInTransactionTimeout = 60 * time.Second
	}
}
