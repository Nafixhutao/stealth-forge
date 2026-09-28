package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSchemaFingerprintPostgreSQLIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to verify schema fingerprints against PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	defer admin.Close()
	randomSuffix := make([]byte, 8)
	if _, err := rand.Read(randomSuffix); err != nil {
		t.Fatal(err)
	}
	schema := "stealth_fingerprint_test_" + hex.EncodeToString(randomSuffix)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`) }()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect to isolated test schema: %v", err)
	}
	defer pool.Close()

	withoutLedger, err := SchemaFingerprint(ctx, pool)
	if err != nil {
		t.Fatalf("fingerprint missing ledger: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	emptyLedger, err := SchemaFingerprint(ctx, pool)
	if err != nil {
		t.Fatalf("fingerprint empty ledger: %v", err)
	}
	if emptyLedger != withoutLedger {
		t.Fatalf("missing ledger fingerprint %q differs from empty ledger %q", withoutLedger, emptyLedger)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1), ($2)`, "000002_second.up.sql", "000001_first.up.sql"); err != nil {
		t.Fatal(err)
	}
	withMigrations, err := SchemaFingerprint(ctx, pool)
	if err != nil {
		t.Fatalf("fingerprint applied migrations: %v", err)
	}
	if withMigrations == emptyLedger {
		t.Fatal("applying migrations did not change the schema fingerprint")
	}
	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET applied_at = $1`, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	afterTimestampChange, err := SchemaFingerprint(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if afterTimestampChange != withMigrations {
		t.Fatalf("timestamp-only change altered the schema fingerprint: %q != %q", afterTimestampChange, withMigrations)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, "000003_third.up.sql"); err != nil {
		t.Fatal(err)
	}
	withAdditionalMigration, err := SchemaFingerprint(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if withAdditionalMigration == withMigrations {
		t.Fatalf("additional migration did not change schema fingerprint %q", withMigrations)
	}
}
