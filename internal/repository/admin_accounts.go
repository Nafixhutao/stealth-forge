package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListInstanceAccounts returns every Console account for the Admin Console
// directory: identity flags, instance role, the sign-in methods it has linked,
// and its most recent session. It reads only durable rows — no telemetry.
func (r *Repository) ListInstanceAccounts(ctx context.Context) ([]domain.AdminAccount, error) {
	if r == nil || r.pool == nil {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
		  a.id,
		  a.email,
		  a.email_verified,
		  COALESCE(ir.role, ''),
		  a.plan,
		  a.created_at,
		  COALESCE((
		    SELECT array_agg(ai.provider ORDER BY ai.provider)
		    FROM account_identities ai WHERE ai.account_id = a.id
		  ), ARRAY[]::text[]) AS providers,
		  COALESCE((
		    SELECT ai.avatar_url
		    FROM account_identities ai
		    WHERE ai.account_id = a.id AND ai.avatar_url IS NOT NULL AND ai.avatar_url <> ''
		    ORDER BY ai.created_at DESC
		    LIMIT 1
		  ), '') AS avatar_url,
		  COALESCE((
		    SELECT ai.display_name
		    FROM account_identities ai
		    WHERE ai.account_id = a.id AND ai.display_name IS NOT NULL AND ai.display_name <> ''
		    ORDER BY ai.created_at DESC
		    LIMIT 1
		  ), '') AS display_name,
		  s.created_at,
		  COALESCE(s.auth_method, '')
		FROM accounts a
		LEFT JOIN instance_roles ir ON ir.account_id = a.id
		LEFT JOIN LATERAL (
		  SELECT created_at, auth_method
		  FROM sessions
		  WHERE account_id = a.id
		  ORDER BY created_at DESC
		  LIMIT 1
		) s ON TRUE
		ORDER BY a.created_at DESC, a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]domain.AdminAccount, 0, 32)
	for rows.Next() {
		var item domain.AdminAccount
		var lastSignInAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Email, &item.EmailVerified, &item.InstanceRole,
			&item.Plan, &item.CreatedAt, &item.Providers, &item.AvatarURL, &item.DisplayName,
			&lastSignInAt, &item.LastSignInMethod,
		); err != nil {
			return nil, err
		}
		if lastSignInAt.Valid {
			item.LastSignInAt = lastSignInAt.Time.UTC().Format(time.RFC3339)
		}
		accounts = append(accounts, item)
	}
	return accounts, rows.Err()
}

// AdminAccountPlanInput carries a plan change from the Admin Console.
type AdminAccountPlanInput struct {
	Plan string
}

// validAccountPlans mirrors the accounts_plan_valid CHECK constraint.
var validAccountPlans = map[string]bool{
	"free": true, "go": true, "plus": true, "pro": true,
}

// UpdateInstanceAccountPlan sets one Console account's subscription plan and
// records the change in the instance audit trail.
func (r *Repository) UpdateInstanceAccountPlan(ctx context.Context, actorID, accountID uuid.UUID, plan string) (domain.AdminAccount, error) {
	if r == nil || r.pool == nil {
		return domain.AdminAccount{}, ErrNotFound
	}
	if !validAccountPlans[plan] {
		return domain.AdminAccount{}, fmt.Errorf("%w: plan is unsupported", ErrInvalidBackup)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminAccount{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id=$1)`, accountID).Scan(&exists); err != nil {
		return domain.AdminAccount{}, err
	}
	if !exists {
		return domain.AdminAccount{}, ErrNotFound
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE accounts SET plan=$2, plan_updated_at=now(), updated_at=now() WHERE id=$1`,
		accountID, plan,
	); err != nil {
		return domain.AdminAccount{}, err
	}

	if err := writeInstanceAuditTx(ctx, tx, actorID, "admin.account.plan_updated", "account", accountID, map[string]any{
		"plan": plan,
	}); err != nil {
		return domain.AdminAccount{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.AdminAccount{}, err
	}
	return r.adminAccountByID(ctx, accountID)
}

// adminAccountByID projects one directory row, mirroring ListInstanceAccounts.
func (r *Repository) adminAccountByID(ctx context.Context, accountID uuid.UUID) (domain.AdminAccount, error) {
	var item domain.AdminAccount
	var lastSignInAt sql.NullTime
	err := r.pool.QueryRow(ctx, `
		SELECT
		  a.id,
		  a.email,
		  a.email_verified,
		  COALESCE(ir.role, ''),
		  a.plan,
		  a.created_at,
		  COALESCE((
		    SELECT array_agg(ai.provider ORDER BY ai.provider)
		    FROM account_identities ai WHERE ai.account_id = a.id
		  ), ARRAY[]::text[]) AS providers,
		  COALESCE((
		    SELECT ai.avatar_url
		    FROM account_identities ai
		    WHERE ai.account_id = a.id AND ai.avatar_url IS NOT NULL AND ai.avatar_url <> ''
		    ORDER BY ai.created_at DESC
		    LIMIT 1
		  ), '') AS avatar_url,
		  COALESCE((
		    SELECT ai.display_name
		    FROM account_identities ai
		    WHERE ai.account_id = a.id AND ai.display_name IS NOT NULL AND ai.display_name <> ''
		    ORDER BY ai.created_at DESC
		    LIMIT 1
		  ), '') AS display_name,
		  s.created_at,
		  COALESCE(s.auth_method, '')
		FROM accounts a
		LEFT JOIN instance_roles ir ON ir.account_id = a.id
		LEFT JOIN LATERAL (
		  SELECT created_at, auth_method
		  FROM sessions
		  WHERE account_id = a.id
		  ORDER BY created_at DESC
		  LIMIT 1
		) s ON TRUE
		WHERE a.id = $1`, accountID).Scan(
		&item.ID, &item.Email, &item.EmailVerified, &item.InstanceRole,
		&item.Plan, &item.CreatedAt, &item.Providers, &item.AvatarURL, &item.DisplayName,
		&lastSignInAt, &item.LastSignInMethod,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.AdminAccount{}, ErrNotFound
		}
		return domain.AdminAccount{}, err
	}
	if lastSignInAt.Valid {
		item.LastSignInAt = lastSignInAt.Time.UTC().Format(time.RFC3339)
	}
	return item, nil
}
