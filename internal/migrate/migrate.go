package migrate

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var files embed.FS

const advisoryLockID int64 = 8_105_202_601

// noTransactionDirective marks a migration that must run outside a transaction.
// CREATE INDEX CONCURRENTLY cannot run inside one, and such a migration is
// intentionally not atomic: if it fails partway the operator re-runs it after
// correcting the cause, so it must be written idempotently (for example with
// IF NOT EXISTS). The directive must appear on a leading comment line before
// any SQL statement.
const noTransactionDirective = "-- migrate:no-transaction"

// runsInTransaction reports whether a migration file should be wrapped in a
// transaction. It inspects only the leading comment block, so a directive in
// the middle of the file is ignored.
func runsInTransaction(sql string) bool {
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "--") {
			return true
		}
		if strings.EqualFold(trimmed, noTransactionDirective) {
			return false
		}
	}
	return true
}

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	}()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := fs.ReadDir(files, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	for _, name := range names {
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}
		sql, err := files.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if !runsInTransaction(string(sql)) {
			// Non-transactional path: run the statements directly, then record
			// the ledger row. The migration is responsible for being idempotent
			// because a partial failure is not rolled back.
			if _, err = conn.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("apply migration %s: %w", name, err)
			}
			if _, err = conn.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
				return fmt.Errorf("record migration %s: %w", name, err)
			}
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// SchemaFingerprint returns a stable fingerprint of the applied migration
// ledger. An absent ledger has the same fingerprint as an empty ledger, which
// is the state of a database before its first migration. Rollback uses this
// value to allow image-only rollback and refuse releases whose schema ledger
// has advanced; it never attempts reverse SQL migrations.
func SchemaFingerprint(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return "", fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
	}()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return "", fmt.Errorf("inspect migration ledger: %w", err)
	}
	names := []string{}
	if exists {
		rows, err := conn.Query(ctx, `SELECT name FROM schema_migrations ORDER BY name`)
		if err != nil {
			return "", fmt.Errorf("read migration ledger: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return "", fmt.Errorf("read migration ledger entry: %w", err)
			}
			names = append(names, name)
		}
		if err := rows.Err(); err != nil {
			return "", fmt.Errorf("iterate migration ledger: %w", err)
		}
	}
	hash := sha256.New()
	for _, name := range names {
		_, _ = fmt.Fprintf(hash, "%d:%s\n", len(name), name)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
