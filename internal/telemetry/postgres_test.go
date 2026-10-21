package telemetry

import (
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
