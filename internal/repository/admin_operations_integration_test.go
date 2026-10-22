package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/migrate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestListInstanceAuditEventsScansNullableColumnsIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	eventID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_events (id,organization_id,actor_account_id,action,target_type,target_id,metadata)
		VALUES ($1,NULL,NULL,'admin.system_cleanup','system_cleanup',NULL,'{"source":"integration"}'::jsonb)`,
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_events WHERE id=$1`, eventID)
	})

	items, _, err := New(pool).ListInstanceAuditEvents(ctx, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range items {
		if item.ID != eventID.String() {
			continue
		}
		found = true
		if item.OrganizationID != nil || item.ActorAccountID != nil || item.ActorEmail != nil || item.TargetID != nil {
			t.Fatalf("nullable audit columns decoded incorrectly: %+v", item)
		}
	}
	if !found {
		t.Fatalf("nullable audit event %s was not returned", eventID)
	}
}
