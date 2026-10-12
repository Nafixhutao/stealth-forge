package repository

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func requireBootstrapSealedTx(ctx context.Context, tx pgx.Tx) error {
	var sealed bool
	err := tx.QueryRow(ctx, `
		SELECT (b.sealed_at IS NOT NULL OR EXISTS (
			SELECT 1 FROM instance_roles WHERE role = 'instance_owner'
		))
		FROM instance_bootstrap b
		WHERE b.id = TRUE
		FOR UPDATE`).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !sealed {
		return ErrBootstrapRequired
	}
	return nil
}

// BootstrapSession stores only the durable portion of a one-time setup
// session. The plaintext code is generated and returned by the HTTP layer but
// is never passed to this repository.
func (r *BootstrapRepository) CreateBootstrapSession(ctx context.Context, input BootstrapSessionInput) error {
	if input.ID == uuid.Nil {
		return ErrInvalidBootstrapCode
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var sealed bool
	err = tx.QueryRow(ctx, `
		SELECT (b.sealed_at IS NOT NULL OR EXISTS (
			SELECT 1 FROM instance_roles WHERE role = 'instance_owner'
		))
		FROM instance_bootstrap b
		WHERE b.id = TRUE
		FOR UPDATE`).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if sealed {
		return ErrBootstrapSealed
	}
	if len(input.CodeHash) != 32 || input.ExpiresAt.Before(time.Now().UTC()) {
		return ErrInvalidBootstrapCode
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bootstrap_sessions
		SET invalidated_at = now(),github_status='none',github_device_code_ciphertext=NULL,
		    github_user_code=NULL,github_verification_uri=NULL,github_expires_at=NULL,
		    github_interval_seconds=NULL,github_next_poll_at=NULL
		WHERE used_at IS NULL AND invalidated_at IS NULL`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bootstrap_sessions (id,code_hash,expires_at) VALUES ($1,$2,$3)`, input.ID, input.CodeHash, input.ExpiresAt); err != nil {
		return mapError(err)
	}
	if err := writeAuditMetadata(ctx, tx, uuid.Nil, uuid.Nil, "instance.bootstrap.session_created", "bootstrap_session", input.ID, map[string]any{"expires_at": input.ExpiresAt}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *BootstrapRepository) BootstrapStatus(ctx context.Context) (BootstrapStatus, error) {
	var setupRequired bool
	err := r.pool.QueryRow(ctx, `
		SELECT b.sealed_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM instance_roles WHERE role = 'instance_owner')
		FROM instance_bootstrap b
		WHERE b.id = TRUE`).Scan(&setupRequired)
	if errors.Is(err, pgx.ErrNoRows) {
		return BootstrapStatus{}, ErrNotFound
	}
	return BootstrapStatus{SetupRequired: setupRequired}, err
}

// VerifyBootstrapCode checks the operator-provided code and returns only the
// opaque database session ID needed for subsequent GitHub first-owner
// authorization.
// The setup code itself never leaves this request boundary in a response.
func (r *BootstrapRepository) VerifyBootstrapCode(ctx context.Context, codeHash []byte) (BootstrapVerification, error) {
	if len(codeHash) != 32 {
		return BootstrapVerification{}, ErrInvalidBootstrapCode
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return BootstrapVerification{}, err
	}
	defer tx.Rollback(ctx)

	var sealed bool
	err = tx.QueryRow(ctx, `
		SELECT (b.sealed_at IS NOT NULL OR EXISTS (
			SELECT 1 FROM instance_roles WHERE role = 'instance_owner'
		))
		FROM instance_bootstrap b
		WHERE b.id = TRUE
		FOR UPDATE`).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return BootstrapVerification{}, ErrNotFound
	}
	if err != nil {
		return BootstrapVerification{}, err
	}
	if sealed {
		return BootstrapVerification{}, ErrBootstrapSealed
	}

	var verification BootstrapVerification
	err = tx.QueryRow(ctx, `
		SELECT id,expires_at
		FROM bootstrap_sessions
		WHERE code_hash=$1 AND used_at IS NULL AND invalidated_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE`, codeHash).Scan(&verification.ID, &verification.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BootstrapVerification{}, ErrInvalidBootstrapCode
	}
	if err != nil {
		return BootstrapVerification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BootstrapVerification{}, err
	}
	return verification, nil
}

func (r *BootstrapRepository) CreateGitHubInstanceOwner(
	ctx context.Context,
	input GitHubOwnerInput,
) (domain.Account, error) {
	if input.BootstrapSessionID == uuid.Nil || input.AccountID == uuid.Nil || input.SessionID == uuid.Nil ||
		len(input.BootstrapCodeHash) != 32 ||
		len(input.TokenHash) != 32 ||
		input.ProviderUserID == "" ||
		input.ProviderLogin == "" ||
		!input.SessionExpiresAt.After(time.Now().UTC()) {
		return domain.Account{}, ErrGitHubIdentity
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Account{}, err
	}
	defer tx.Rollback(ctx)
	var sealed bool
	err = tx.QueryRow(ctx, `SELECT (sealed_at IS NOT NULL OR EXISTS (SELECT 1 FROM instance_roles WHERE role='instance_owner')) FROM instance_bootstrap WHERE id=TRUE FOR UPDATE`).
		Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, ErrNotFound
	}
	if err != nil {
		return domain.Account{}, err
	}
	if sealed {
		return domain.Account{}, ErrBootstrapSealed
	}
	var storedHash []byte
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT code_hash,expires_at FROM bootstrap_sessions WHERE id=$1 AND used_at IS NULL AND invalidated_at IS NULL AND expires_at > now() FOR UPDATE`, input.BootstrapSessionID).
		Scan(&storedHash, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, ErrInvalidBootstrapCode
	}
	if err != nil {
		return domain.Account{}, err
	}
	if subtle.ConstantTimeCompare(input.BootstrapCodeHash, storedHash) != 1 {
		return domain.Account{}, ErrInvalidBootstrapCode
	}
	account := domain.Account{
		ID:             input.AccountID.String(),
		EmailVerified:  false,
		InstanceRole:   "instance_owner",
		Provider:       "github",
		ProviderUserID: input.ProviderUserID,
		ProviderLogin:  input.ProviderLogin,
		DisplayName:    input.DisplayName,
		AvatarURL:      input.AvatarURL,
	}
	if err := tx.QueryRow(ctx, `INSERT INTO accounts (id,email,password_hash) VALUES ($1,NULL,NULL) RETURNING created_at`, input.AccountID).Scan(&account.CreatedAt); err != nil {
		return domain.Account{}, mapError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_identities (account_id,provider,provider_user_id,provider_login,provider_email,display_name,avatar_url) VALUES ($1,'github',$2,$3,$4,$5,$6)`, input.AccountID, input.ProviderUserID, input.ProviderLogin, nullableString(input.ProviderEmail), nullableString(input.DisplayName), nullableString(input.AvatarURL)); err != nil {
		return domain.Account{}, mapError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO instance_roles (account_id,role) VALUES ($1,'instance_owner')`, input.AccountID); err != nil {
		return domain.Account{}, mapError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (id,account_id,token_hash,expires_at) VALUES ($1,$2,$3,$4)`, input.SessionID, input.AccountID, input.TokenHash, input.SessionExpiresAt); err != nil {
		return domain.Account{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE bootstrap_sessions SET used_at=now(),github_status='none',github_device_code_ciphertext=NULL,github_user_code=NULL,github_verification_uri=NULL,github_expires_at=NULL,github_interval_seconds=NULL,github_next_poll_at=NULL WHERE id=$1`, input.BootstrapSessionID); err != nil {
		return domain.Account{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE bootstrap_sessions SET invalidated_at=now(),github_status='none',github_device_code_ciphertext=NULL,github_user_code=NULL,github_verification_uri=NULL,github_expires_at=NULL,github_interval_seconds=NULL,github_next_poll_at=NULL WHERE used_at IS NULL AND invalidated_at IS NULL`); err != nil {
		return domain.Account{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE instance_bootstrap SET sealed_at=now(),sealed_reason='instance_owner_created' WHERE id=TRUE`); err != nil {
		return domain.Account{}, err
	}
	identityMetadata := map[string]any{
		"provider":         "github",
		"provider_user_id": input.ProviderUserID,
		"provider_login":   input.ProviderLogin,
	}
	if err := writeAuditMetadata(ctx, tx, uuid.Nil, input.AccountID, "bootstrap.github.authorized", "account", input.AccountID, identityMetadata); err != nil {
		return domain.Account{}, err
	}
	if err := writeAuditMetadata(ctx, tx, uuid.Nil, input.AccountID, "instance.owner.created", "instance_owner", input.AccountID, map[string]any{"role": "instance_owner", "provider": "github"}); err != nil {
		return domain.Account{}, err
	}
	if err := writeAuditMetadata(ctx, tx, uuid.Nil, input.AccountID, "instance.bootstrap.sealed", "instance_bootstrap", input.AccountID, map[string]any{"reason": "instance_owner_created"}); err != nil {
		return domain.Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, err
	}
	return account, nil
}

func (r *BootstrapRepository) ListBootstrapAdoptionAccounts(ctx context.Context) ([]BootstrapAdoptionAccount, error) {
	var sealedReason sql.NullString
	var sealedAt sql.NullTime
	if err := r.pool.QueryRow(ctx, `SELECT sealed_at,sealed_reason FROM instance_bootstrap WHERE id=TRUE`).Scan(&sealedAt, &sealedReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !sealedAt.Valid || sealedReason.String != "legacy_installation" {
		return nil, ErrBootstrapSealed
	}
	rows, err := r.pool.Query(
		ctx,
		`SELECT a.id,a.email,ai.provider,ai.provider_login,a.created_at FROM accounts a LEFT JOIN account_identities ai ON ai.account_id=a.id AND ai.provider='github' ORDER BY a.created_at ASC,a.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]BootstrapAdoptionAccount, 0)
	for rows.Next() {
		var account BootstrapAdoptionAccount
		var email, provider, login sql.NullString
		if err := rows.Scan(&account.ID, &email, &provider, &login, &account.CreatedAt); err != nil {
			return nil, err
		}
		account.Email = email.String
		account.Provider = provider.String
		account.ProviderLogin = login.String
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *BootstrapRepository) AdoptInstanceOwner(ctx context.Context, accountID uuid.UUID) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sealedAt sql.NullTime
	var reason sql.NullString
	err = tx.QueryRow(ctx, `SELECT sealed_at,sealed_reason FROM instance_bootstrap WHERE id=TRUE FOR UPDATE`).
		Scan(&sealedAt, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if reason.String != "legacy_installation" {
		return ErrBootstrapSealed
	}
	var ownerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM instance_roles WHERE role='instance_owner')`).Scan(&ownerExists); err != nil {
		return err
	}
	if ownerExists {
		return ErrBootstrapSealed
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id=$1)`, accountID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO instance_roles (account_id,role) VALUES ($1,'instance_owner')`, accountID); err != nil {
		return mapError(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE instance_bootstrap SET sealed_at=COALESCE(sealed_at,now()),sealed_reason='instance_owner_adopted' WHERE id=TRUE`); err != nil {
		return err
	}
	if err := writeAuditMetadata(ctx, tx, uuid.Nil, accountID, "instance.owner.adopted", "instance_owner", accountID, map[string]any{"role": "instance_owner", "reason": "operator_adoption"}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
