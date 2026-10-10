package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrIdentityLinkedElsewhere is returned when a provider identity already
// belongs to a different account. Reassigning it silently would let one
// provider login take over another account.
var ErrIdentityLinkedElsewhere = errors.New("identity is already linked to another account")

// ListAccountIdentities returns the external identities linked to one account,
// newest first. Secrets are never part of this projection.
func (r *Repository) ListAccountIdentities(ctx context.Context, accountID uuid.UUID) ([]domain.AccountIdentity, error) {
	if r == nil || r.pool == nil {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT provider, provider_login, display_name, avatar_url, created_at
		FROM account_identities
		WHERE account_id=$1
		ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := make([]domain.AccountIdentity, 0, 2)
	for rows.Next() {
		var identity domain.AccountIdentity
		if err := rows.Scan(&identity.Provider, &identity.ProviderLogin, &identity.DisplayName, &identity.AvatarURL, &identity.CreatedAt); err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

// LinkAccountIdentity attaches an external identity to an account. It refuses
// to steal an identity that already belongs to another account, and it treats
// re-linking the same account idempotently by refreshing the provider profile.
func (r *Repository) LinkAccountIdentity(
	ctx context.Context,
	accountID uuid.UUID,
	provider, providerUserID, providerLogin, providerEmail, displayName, avatarURL string,
) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !ValidOAuthProvider(provider) || strings.TrimSpace(providerUserID) == "" {
		return ErrOAuthProviderInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock any existing row for this provider identity so two concurrent links
	// cannot both pass the ownership check.
	var ownerID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT account_id FROM account_identities
		WHERE provider=$1 AND provider_user_id=$2
		FOR UPDATE`, provider, providerUserID).Scan(&ownerID)
	switch {
	case err == nil && ownerID != accountID:
		return ErrIdentityLinkedElsewhere
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO account_identities
		  (account_id, provider, provider_user_id, provider_login, provider_email, display_name, avatar_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (account_id, provider) DO UPDATE SET
		  provider_user_id=EXCLUDED.provider_user_id,
		  provider_login=EXCLUDED.provider_login,
		  provider_email=EXCLUDED.provider_email,
		  display_name=EXCLUDED.display_name,
		  avatar_url=EXCLUDED.avatar_url,
		  updated_at=now()`,
		accountID, provider, providerUserID, providerLogin,
		nullableString(providerEmail), nullableString(displayName), nullableString(avatarURL),
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// The unique (provider, provider_user_id) index raced with another
			// request; the identity belongs to someone else.
			return ErrIdentityLinkedElsewhere
		}
		return err
	}
	return tx.Commit(ctx)
}

// DeleteAccountIdentity unlinks one provider from an account.
func (r *Repository) DeleteAccountIdentity(ctx context.Context, accountID uuid.UUID, provider string) error {
	if r == nil || r.pool == nil {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `
		DELETE FROM account_identities WHERE account_id=$1 AND provider=$2`,
		accountID, strings.ToLower(strings.TrimSpace(provider)))
	return err
}
