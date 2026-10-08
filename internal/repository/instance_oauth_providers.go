package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrOAuthProviderInvalid rejects a provider name the instance does not
// support. The set is fixed so a typo cannot silently create a dead row.
var ErrOAuthProviderInvalid = errors.New("oauth provider is not supported")

// ErrOAuthProviderUnavailable reports a missing encryption key. Reads that
// must decrypt the client secret fail closed rather than returning empty.
var ErrOAuthProviderUnavailable = errors.New("oauth provider storage is unavailable")

// OAuthProviderStatus is the safe, non-secret projection for the Console. The
// client secret is never part of this shape.
type OAuthProviderStatus struct {
	Provider   string    `json:"provider"`
	ClientID   string    `json:"client_id"`
	Configured bool      `json:"configured"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OAuthProviderCredentials is one decrypted provider record. It is only built
// for trusted callers that must complete a provider round trip.
type OAuthProviderCredentials struct {
	Provider     string
	ClientID     string
	ClientSecret string
}

// ValidOAuthProvider reports whether the instance supports this provider.
func ValidOAuthProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github", "google":
		return true
	default:
		return false
	}
}

// ListOAuthProviders returns every configured provider without secrets.
func (r *Repository) ListOAuthProviders(ctx context.Context) ([]OAuthProviderStatus, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT provider, client_id, updated_at
		FROM instance_oauth_providers
		ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	statuses := make([]OAuthProviderStatus, 0, 2)
	for rows.Next() {
		var status OAuthProviderStatus
		if err := rows.Scan(&status.Provider, &status.ClientID, &status.UpdatedAt); err != nil {
			return nil, err
		}
		status.Configured = true
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

// OAuthProviderCredentials reads and decrypts one provider. A provider without
// stored credentials is ErrNotFound so callers can fall back to the
// deployment environment.
func (r *Repository) OAuthProviderCredentials(ctx context.Context, provider string) (OAuthProviderCredentials, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !ValidOAuthProvider(provider) {
		return OAuthProviderCredentials{}, ErrOAuthProviderInvalid
	}
	var clientID string
	var ciphertext []byte
	err := r.pool.QueryRow(ctx, `
		SELECT client_id, client_secret_ciphertext
		FROM instance_oauth_providers
		WHERE provider=$1`, provider).Scan(&clientID, &ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthProviderCredentials{}, ErrNotFound
	}
	if err != nil {
		return OAuthProviderCredentials{}, err
	}
	if len(ciphertext) == 0 {
		return OAuthProviderCredentials{}, ErrOAuthProviderUnavailable
	}
	if r.oauthCipher == nil {
		return OAuthProviderCredentials{}, ErrOAuthProviderUnavailable
	}
	plaintext, err := r.oauthCipher.Decrypt(ciphertext)
	if err != nil {
		return OAuthProviderCredentials{}, errors.New("oauth provider credential decryption failed")
	}
	return OAuthProviderCredentials{
		Provider:     provider,
		ClientID:     clientID,
		ClientSecret: string(plaintext),
	}, nil
}

// UpsertOAuthProvider stores or replaces one provider's credentials.
func (r *Repository) UpsertOAuthProvider(ctx context.Context, provider, clientID, clientSecret string) (OAuthProviderStatus, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !ValidOAuthProvider(provider) {
		return OAuthProviderStatus{}, ErrOAuthProviderInvalid
	}
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || len(clientID) > 256 || clientSecret == "" || len(clientSecret) > 512 {
		return OAuthProviderStatus{}, errors.New("oauth provider credentials are invalid")
	}
	if r.oauthCipher == nil {
		return OAuthProviderStatus{}, ErrOAuthProviderUnavailable
	}
	ciphertext, err := r.oauthCipher.Encrypt([]byte(clientSecret))
	if err != nil {
		return OAuthProviderStatus{}, err
	}
	var status OAuthProviderStatus
	err = r.pool.QueryRow(ctx, `
		INSERT INTO instance_oauth_providers (provider, client_id, client_secret_ciphertext)
		VALUES ($1,$2,$3)
		ON CONFLICT (provider) DO UPDATE
			SET client_id=$2, client_secret_ciphertext=$3, updated_at=now()
		RETURNING provider, client_id, updated_at`, provider, clientID, ciphertext).
		Scan(&status.Provider, &status.ClientID, &status.UpdatedAt)
	if err != nil {
		return OAuthProviderStatus{}, err
	}
	status.Configured = true
	return status, nil
}

// DeleteOAuthProvider removes one provider's credentials.
func (r *Repository) DeleteOAuthProvider(ctx context.Context, provider string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !ValidOAuthProvider(provider) {
		return ErrOAuthProviderInvalid
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM instance_oauth_providers WHERE provider=$1`, provider); err != nil {
		return err
	}
	return nil
}
