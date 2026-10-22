package telemetry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/migrate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStoreQueriesRequestTelemetryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL telemetry integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgres(pool, Config{
		MaxQueryDuration: 5 * time.Second,
		MaxQueryRange:    time.Hour,
		MaxQueryRows:     100,
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ids := []uuid.UUID{
		uuid.Must(uuid.NewV7()),
		uuid.Must(uuid.NewV7()),
		uuid.Must(uuid.NewV7()),
	}
	rows := []struct {
		traceID string
		method  string
		route   string
		status  int
		ms      int64
		offset  time.Duration
	}{
		{"telemetry-integration-ok", "GET", "/v1/projects/{projectID}", 200, 20, -2 * time.Minute},
		{"telemetry-integration-warn", "GET", "/v1/projects/{projectID}/sites", 404, 30, -time.Minute},
		{"telemetry-integration-error", "POST", "/v1/projects/{projectID}/apps", 500, 40, -30 * time.Second},
	}
	for index, row := range rows {
		started := now.Add(row.offset)
		if _, err := pool.Exec(ctx, `
			INSERT INTO http_traces
				(id,trace_id,span_id,method,route,status,duration_ms,response_bytes,started_at,finished_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`,
			ids[index], row.traceID, ids[index].String()[:16], row.method, row.route,
			row.status, row.ms, 128, started,
		); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM http_traces WHERE id = ANY($1)`, ids)
	})

	queryRange := TimeRange{From: now.Add(-5 * time.Minute), To: now.Add(time.Minute)}
	// Other integration packages share the database and may leave unrelated
	// traces inside the same time window. Scope every assertion to this
	// fixture's route prefix so the test remains deterministic in a full run.
	logs, err := store.QueryLogs(ctx, LogsQuery{
		Range: queryRange, Search: "/v1/projects/{projectID}", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs.Items) != 3 || logs.Items[0].Severity != "ERROR" {
		t.Fatalf("unexpected logs: %+v", logs.Items)
	}

	traces, err := store.QueryTraces(ctx, TracesQuery{Range: queryRange, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	// QueryTraces has no substring search, so filter the bounded response to
	// the three fixture trace IDs. Unrelated integration traffic may still be
	// present when packages share a database.
	fixtureTraces := make([]SpanRecord, 0, len(rows))
	for _, item := range traces.Items {
		for _, row := range rows {
			if item.TraceID == row.traceID {
				fixtureTraces = append(fixtureTraces, item)
				break
			}
		}
	}
	if len(fixtureTraces) != 3 || fixtureTraces[0].Status != "error" {
		t.Fatalf("unexpected traces: %+v", traces.Items)
	}

	errorsResult, err := store.QueryErrorGroups(ctx, ErrorGroupsQuery{Range: queryRange, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(errorsResult.Items) != 1 || errorsResult.Items[0].OccurrenceCount != 1 {
		t.Fatalf("unexpected error groups: %+v", errorsResult.Items)
	}

	// HTTP overview is intentionally service-wide, so isolate it from other
	// integration fixtures that may share the same database and time window.
	overviewRange := TimeRange{
		From: now.Add(-2*time.Minute - time.Millisecond),
		To:   now.Add(-30*time.Second + time.Millisecond),
	}
	overview, err := store.QueryHTTPOverview(ctx, HTTPOverviewQuery{Range: overviewRange})
	if err != nil {
		t.Fatal(err)
	}
	if overview.SampleCount != 3 || overview.ErrorRate < 0.33 || overview.ErrorRate > 0.34 {
		t.Fatalf("unexpected HTTP overview: %+v", overview)
	}

	alert, err := store.EvaluateAlert(ctx, AlertQuery{
		Kind: "error_rate", Range: overviewRange,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !alert.Available || alert.SampleCount != 3 {
		t.Fatalf("unexpected error-rate alert value: %+v", alert)
	}

	if _, ok := any(store).(ContainerLogReader); ok {
		t.Fatal("PostgreSQL request telemetry must not claim App runtime-log support")
	}
}
