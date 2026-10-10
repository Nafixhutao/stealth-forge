package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
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
			&item.CreatedAt, &item.Providers, &item.AvatarURL, &item.DisplayName,
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
