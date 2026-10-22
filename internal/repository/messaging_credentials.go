package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MessagingProviderCredentials struct {
	ProviderID uuid.UUID
	ProjectID  uuid.UUID
	Channel    string
	Provider   string
	Enabled    bool
	Values     map[string]string
}

func (r *Repository) MessagingProviderCredentials(
	ctx context.Context,
	projectID,
	providerID uuid.UUID,
) (
	MessagingProviderCredentials,
	error,
) {
	if r.messagingCipher == nil {
		return MessagingProviderCredentials{}, ErrMessagingNotReady
	}
	var result MessagingProviderCredentials
	var ciphertext []byte
	err := r.pool.QueryRow(ctx, `SELECT id,project_id,channel,provider,enabled,credentials_ciphertext FROM project_messaging_providers WHERE project_id=$1 AND id=$2`, projectID, providerID).Scan(&result.ProviderID, &result.ProjectID, &result.Channel, &result.Provider, &result.Enabled, &ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return MessagingProviderCredentials{}, ErrNotFound
	}
	if err != nil {
		return MessagingProviderCredentials{}, err
	}
	plaintext, err := r.messagingCipher.Decrypt(ciphertext)
	if err != nil {
		return MessagingProviderCredentials{}, fmt.Errorf("%w: decrypt provider credentials", ErrMessagingNotReady)
	}
	if err := json.Unmarshal(plaintext, &result.Values); err != nil || result.Values == nil {
		return MessagingProviderCredentials{}, fmt.Errorf("%w: provider credentials are corrupt", ErrMessagingNotReady)
	}
	return result, nil
}
