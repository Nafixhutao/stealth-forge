package telemetry

import (
	"strings"
	"testing"
	"time"
)

func TestPostgresLogSeverity(t *testing.T) {
	cases := map[int]string{
		200: "INFO",
		302: "INFO",
		404: "WARN",
		429: "WARN",
		500: "ERROR",
		503: "ERROR",
	}
	for status, want := range cases {
		if got := postgresLogSeverity(status); got != want {
			t.Fatalf("postgresLogSeverity(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestPostgresErrorFingerprintIsStableAndScoped(t *testing.T) {
	first := postgresErrorFingerprint("api", "HTTP 500", "/v1/projects/{projectID}")
	second := postgresErrorFingerprint("api", "HTTP 500", "/v1/projects/{projectID}")
	otherRoute := postgresErrorFingerprint("api", "HTTP 500", "/v1/admin/overview")
	if first != second {
		t.Fatal("identical error groups must have the same fingerprint")
	}
	if first == otherRoute {
		t.Fatal("different routes must not share a fingerprint")
	}
	if len(first) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(first))
	}
}

func TestPostgresVolumeQueryUsesBoundedBuckets(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		duration time.Duration
		want     string
	}{
		{time.Hour, postgresMinuteVolumeQuery},
		{6 * time.Hour, postgresFiveMinuteVolumeQuery},
		{24 * time.Hour, postgresQuarterHourVolumeQuery},
		{7 * 24 * time.Hour, postgresHourVolumeQuery},
		{30 * 24 * time.Hour, postgresDayVolumeQuery},
	}
	for _, test := range cases {
		queryRange := TimeRange{From: now.Add(-test.duration), To: now}
		if got := postgresVolumeQueryFor(queryRange); got != test.want {
			t.Fatalf("bucket query for %s was not the expected fixed statement", test.duration)
		}
	}
}

func TestPostgresLogProjectionKeepsSeverityAndTraceIdentity(t *testing.T) {
	item := postgresLogRecord(
		"0199fb52-7a6d-7f91-8f6c-ab95ef2fe3c1",
		"4bf92f3577b34da6a3ce929d0e0e4736",
		"00f067aa0ba902b7",
		"POST",
		"/v1/projects/{projectID}/apps",
		503,
		42,
		128,
		time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
	)
	if item.Severity != "ERROR" || item.TraceID == "" || item.EventID == "" {
		t.Fatalf("unexpected PostgreSQL log projection: %+v", item)
	}
	if item.Body != "POST /v1/projects/{projectID}/apps -> 503 (42ms)" {
		t.Fatalf("unexpected log body: %q", item.Body)
	}
}

func TestRequestLogFilterIsSharedByEveryPostgresLogSurface(t *testing.T) {
	for name, statement := range map[string]string{
		"logs":       postgresLogsQuery,
		"logs-after": postgresLogsAfterQuery,
		"volume":     postgresMinuteVolumeQuery,
		"alert":      postgresLogMatchAlertQuery,
	} {
		if !strings.Contains(statement, requestLogFilter) {
			t.Fatalf("%s query does not use the shared log filter", name)
		}
	}
}

func TestRequestLogFilterRetainsMethodSearch(t *testing.T) {
	if !strings.Contains(requestLogFilter, "method ILIKE") {
		t.Fatal("shared log filter dropped method search")
	}
}

func TestPostgresSourcesDoNotClaimMetrics(t *testing.T) {
	if strings.Contains(postgresSourcesQuery, "'metrics'") {
		t.Fatal("request traces must not be reported as a metrics signal source")
	}
}
