package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IdentitySignupInput creates an account from an external provider identity.
// When an account already exists for the same email and the provider vouches
// for that email, the identity is linked to it instead of creating a second
// account; the caller then receives that account's session.
type IdentitySignupInput struct {
	AccountID        uuid.UUID
	OrganizationID   uuid.UUID
	SessionID        uuid.UUID
	Email            string
	EmailVerified    bool
	OrganizationName string
	OrganizationSlug string
	TokenHash        []byte
	SessionExpiresAt time.Time

	Provider       string
	ProviderUserID string
	ProviderLogin  string
	DisplayName    string
	AvatarURL      string
}

// SignupWithIdentity provisions an account from an OAuth identity, or signs
// into the existing account that owns the same verified email. The whole
// operation is one transaction so a partial account can never exist.
func (r *Repository) SignupWithIdentity(ctx context.Context, input IdentitySignupInput) (domain.Account, domain.Organization, error) {
	if r == nil || r.pool == nil {
		return domain.Account{}, domain.Organization{}, ErrNotFound
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" {
		return domain.Account{}, domain.Organization{}, ErrNotFound
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireBootstrapSealedTx(ctx, tx); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}

	// An account with this email already exists: attach the identity to it
	// rather than creating a duplicate. Only a provider-verified email may
	// claim an existing account.
	var existingID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE email=$1 FOR UPDATE`, email).Scan(&existingID)
	switch {
	case err == nil:
		if !input.EmailVerified {
			return domain.Account{}, domain.Organization{}, ErrIdentityEmailUnverified
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
			existingID, input.Provider, input.ProviderUserID, input.ProviderLogin,
			nullableString(email), nullableString(input.DisplayName), nullableString(input.AvatarURL),
		); err != nil {
			return domain.Account{}, domain.Organization{}, mapIdentityConflict(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sessions (id,account_id,token_hash,expires_at,auth_method) VALUES ($1,$2,$3,$4,$5)`,
			input.SessionID, existingID, input.TokenHash, input.SessionExpiresAt, input.Provider); err != nil {
			return domain.Account{}, domain.Organization{}, err
		}
		if err := writeAudit(ctx, tx, uuid.Nil, existingID, "account.identity_link", "account", existingID); err != nil {
			return domain.Account{}, domain.Organization{}, err
		}
		account, err := accountByIDTx(ctx, tx, existingID)
		if err != nil {
			return domain.Account{}, domain.Organization{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Account{}, domain.Organization{}, err
		}
		return account, domain.Organization{}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return domain.Account{}, domain.Organization{}, err
	}

	account := domain.Account{ID: input.AccountID.String(), Email: email, EmailVerified: input.EmailVerified}
	organization := domain.Organization{
		ID:   input.OrganizationID.String(),
		Name: input.OrganizationName,
		Slug: input.OrganizationSlug,
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO accounts (id,email,email_verified,password_hash) VALUES ($1,$2,$3,NULL) RETURNING created_at`,
		input.AccountID, email, input.EmailVerified,
	).Scan(&account.CreatedAt); err != nil {
		return domain.Account{}, domain.Organization{}, mapError(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO organizations (id,name,slug) VALUES ($1,$2,$3) RETURNING created_at`,
		input.OrganizationID, input.OrganizationName, input.OrganizationSlug).Scan(&organization.CreatedAt); err != nil {
		return domain.Account{}, domain.Organization{}, mapError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization_plans (organization_id) VALUES ($1)`, input.OrganizationID); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO organization_memberships (organization_id,account_id,role) VALUES ($1,$2,'owner')`,
		input.OrganizationID, input.AccountID); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_identities
		  (account_id, provider, provider_user_id, provider_login, provider_email, display_name, avatar_url)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		input.AccountID, input.Provider, input.ProviderUserID, input.ProviderLogin,
		nullableString(email), nullableString(input.DisplayName), nullableString(input.AvatarURL),
	); err != nil {
		return domain.Account{}, domain.Organization{}, mapIdentityConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (id,account_id,token_hash,expires_at,auth_method) VALUES ($1,$2,$3,$4,$5)`,
		input.SessionID, input.AccountID, input.TokenHash, input.SessionExpiresAt, input.Provider); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	if err := writeAudit(ctx, tx, uuid.Nil, input.AccountID, "account.signup", "account", input.AccountID); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	if err := writeAudit(ctx, tx, input.OrganizationID, input.AccountID, "organization.create", "organization", input.OrganizationID); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, domain.Organization{}, err
	}
	return account, organization, nil
}

// ErrIdentityEmailUnverified is returned when an OAuth identity claims an
// existing account email the provider has not verified.
var ErrIdentityEmailUnverified = errors.New("provider email is not verified")

func mapIdentityConflict(err error) error {
	if mapError(err) == ErrConflict {
		return ErrIdentityLinkedElsewhere
	}
	return err
}

func accountByIDTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID) (domain.Account, error) {
	return scanAccount(tx.QueryRow(ctx, `
		SELECT `+accountSelectColumns+`
		FROM accounts a
		LEFT JOIN instance_roles ir ON ir.account_id=a.id
		LEFT JOIN account_identities ai ON ai.account_id=a.id AND ai.provider='github'
		WHERE a.id=$1`, accountID))
}
