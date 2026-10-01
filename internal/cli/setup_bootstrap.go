package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/bootstrap"
)

func (a *App) bootstrapStatus(ctx context.Context, endpoint string) (bootstrapStatusPayload, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return bootstrapStatusPayload{}, err
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		return bootstrapStatusPayload{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return bootstrapStatusPayload{}, &bootstrapHTTPError{status: response.StatusCode}
	}
	var payload bootstrapStatusPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return bootstrapStatusPayload{}, fmt.Errorf("decode bootstrap status: %w", err)
	}
	return payload, nil
}

func (a *App) bootstrapAdoptionAccounts(ctx context.Context, endpoint string, key []byte) ([]bootstrapAdoptionAccountPayload, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set(bootstrap.CLIProofHeader, bootstrap.CLIProof(key))
	response, err := a.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &bootstrapHTTPError{status: response.StatusCode}
	}
	var payload bootstrapAdoptionAccountsPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode adoption accounts: %w", err)
	}
	return payload.Accounts, nil
}

func (a *App) adoptBootstrapOwner(ctx context.Context, endpoint string, key []byte, accountID string) error {
	body, err := json.Marshal(map[string]string{"account_id": accountID})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(bootstrap.CLIProofHeader, bootstrap.CLIProof(key))
	response, err := a.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &bootstrapHTTPError{status: response.StatusCode}
	}
	return nil
}

func (a *App) createBootstrapSession(ctx context.Context, endpoint string, key []byte) (bootstrapSessionPayload, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return bootstrapSessionPayload{}, err
	}
	request.Header.Set(bootstrap.CLIProofHeader, bootstrap.CLIProof(key))
	response, err := a.httpClient.Do(request)
	if err != nil {
		return bootstrapSessionPayload{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return bootstrapSessionPayload{}, &bootstrapHTTPError{status: response.StatusCode}
	}
	var payload bootstrapSessionPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return bootstrapSessionPayload{}, fmt.Errorf("decode bootstrap session: %w", err)
	}
	if !bootstrap.ValidCode(payload.SetupCode) || !payload.ExpiresAt.After(time.Now().UTC()) {
		return bootstrapSessionPayload{}, fmt.Errorf("bootstrap API returned an invalid setup session")
	}
	return payload, nil
}

func (a *App) recoverSetupSession(ctx context.Context, endpoint string, key []byte) (bootstrapSessionPayload, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return bootstrapSessionPayload{}, err
	}
	request.Header.Set(bootstrap.CLIProofHeader, bootstrap.CLIProof(key))
	response, err := a.httpClient.Do(request)
	if err != nil {
		return bootstrapSessionPayload{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return bootstrapSessionPayload{}, &bootstrapHTTPError{status: response.StatusCode}
	}
	var payload bootstrapSessionPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload); err != nil {
		return bootstrapSessionPayload{}, fmt.Errorf("decode setup recovery session: %w", err)
	}
	if !bootstrap.ValidCode(payload.SetupCode) || !payload.ExpiresAt.After(time.Now().UTC()) {
		return bootstrapSessionPayload{}, fmt.Errorf("setup API returned an invalid recovery session")
	}
	return payload, nil
}

func bootstrapCLIKey(values map[string]string) ([]byte, error) {
	raw := strings.TrimSpace(values["BOOTSTRAP_CLI_KEY"])
	if raw == "" {
		return nil, fmt.Errorf("installation config has no dedicated bootstrap CLI key; add BOOTSTRAP_CLI_KEY before retrying (FUNCTIONS_SECRET_KEY cannot be reused)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(raw)
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("installation bootstrap CLI key is invalid")
	}
	return key, nil
}

func isBootstrapComplete(err error) bool {
	var statusErr *bootstrapHTTPError
	return errors.As(err, &statusErr) && statusErr.status == http.StatusGone
}
