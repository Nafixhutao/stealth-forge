package config

import (
	"fmt"
	"strconv"
	"time"
)

// telemetryStoreSettings owns the server-side query limits applied to the
// PostgreSQL request index. The index is part of the control-plane database,
// so there is no separate telemetry address or credential to configure.
type telemetryStoreSettings struct {
	maxDuration time.Duration
	maxRange    time.Duration
	maxRows     int
	retention   time.Duration
}

func loadTelemetryStoreSettings() (telemetryStoreSettings, error) {
	maxDuration, err := boundedDuration("TELEMETRY_MAX_QUERY_DURATION", "10s", time.Second, time.Minute)
	if err != nil {
		return telemetryStoreSettings{}, err
	}
	maxRange, err := boundedDuration("TELEMETRY_MAX_QUERY_RANGE", "720h", 15*time.Minute, 365*24*time.Hour)
	if err != nil {
		return telemetryStoreSettings{}, err
	}
	maxRows, err := boundedInt("TELEMETRY_MAX_QUERY_ROWS", "1000", 1, 10000)
	if err != nil {
		return telemetryStoreSettings{}, err
	}
	retention, err := boundedDuration("TELEMETRY_RETENTION", "720h", 24*time.Hour, 10*365*24*time.Hour)
	if err != nil {
		return telemetryStoreSettings{}, err
	}

	return telemetryStoreSettings{
		maxDuration: maxDuration,
		maxRange:    maxRange,
		maxRows:     maxRows,
		retention:   retention,
	}, nil
}

func (s telemetryStoreSettings) apply(c *Config) {
	c.TelemetryMaxQueryDuration = s.maxDuration
	c.TelemetryMaxQueryRange = s.maxRange
	c.TelemetryMaxQueryRows = s.maxRows
	c.TelemetryRetention = s.retention
}

func (c *Config) applyTelemetryStoreDefaults() {
	if c.TelemetryMaxQueryDuration <= 0 {
		c.TelemetryMaxQueryDuration = 10 * time.Second
	}
	if c.TelemetryMaxQueryRange <= 0 {
		c.TelemetryMaxQueryRange = 30 * 24 * time.Hour
	}
	if c.TelemetryMaxQueryRows <= 0 {
		c.TelemetryMaxQueryRows = 1000
	}
	if c.TelemetryRetention <= 0 {
		c.TelemetryRetention = 30 * 24 * time.Hour
	}
}

func boundedDuration(name, fallback string, minimum, maximum time.Duration) (time.Duration, error) {
	parsed, err := time.ParseDuration(value(name, fallback))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %s and %s", name, minimum, maximum)
	}
	return parsed, nil
}

func boundedInt(name, fallback string, minimum, maximum int) (int, error) {
	parsed, err := strconv.Atoi(value(name, fallback))
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return parsed, nil
}
