package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/bootstrap"
	"github.com/Nafixhutao/stealth-forge/internal/githubauth"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/Nafixhutao/stealth-forge/internal/setupstate"
	"github.com/google/uuid"
)

type bootstrapStatusResponse struct {
	SetupRequired bool `json:"setup_required"`
}

type bootstrapSessionResponse struct {
	SetupCode string    `json:"setup_code"`
	ExpiresAt time.Time `json:"expires_at"`
}

type verifyBootstrapCodeRequest struct {
	SetupCode string `json:"setup_code"`
}

type verifyBootstrapCodeResponse struct {
	AuthorizationSessionID string    `json:"authorization_session_id"`
	ExpiresAt              time.Time `json:"expires_at"`
}

type adoptInstanceOwnerRequest struct {
	AccountID string `json:"account_id"`
}

func (s *Server) bootstrapStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeBootstrapJSON(w, http.StatusOK, bootstrapStatusResponse{SetupRequired: status.SetupRequired})
}

func (s *Server) createBootstrapSession(w http.ResponseWriter, r *http.Request) {
	if !s.bootstrapConfigured() {
		writeBootstrapError(
			w,
			http.StatusServiceUnavailable,
			"github_not_configured",
			"GitHub first-owner authentication is not configured",
		)
		return
	}
	if !s.verifyBootstrapCLI(w, r) {
		return
	}
	code, err := bootstrap.GenerateCode()
	if err != nil {
		internalError(s, w, err)
		return
	}
	expiresAt := time.Now().UTC().Add(bootstrap.CodeLifetime)
	if err := s.bootstrap.CreateBootstrapSession(r.Context(), repository.BootstrapSessionInput{
		ID:        uuid.Must(uuid.NewV7()),
		CodeHash:  bootstrap.HashCode(code),
		ExpiresAt: expiresAt,
	}); err != nil {
		switch {
		case errors.Is(err, repository.ErrBootstrapSealed):
			writeBootstrapError(w, http.StatusGone, "bootstrap_complete", "instance setup has already been completed")
		case errors.Is(err, repository.ErrInvalidBootstrapCode):
			writeBootstrapError(
				w,
				http.StatusUnprocessableEntity,
				"invalid_bootstrap_session",
				"unable to create a setup session",
			)
		default:
			internalError(s, w, err)
		}
		return
	}
	writeBootstrapJSON(w, http.StatusCreated, bootstrapSessionResponse{SetupCode: code, ExpiresAt: expiresAt})
}

func (s *Server) verifyBootstrapCode(w http.ResponseWriter, r *http.Request) {
	var req verifyBootstrapCodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.allowBootstrapAttempt(w, r, "verify") {
		return
	}
	if !bootstrap.ValidCode(req.SetupCode) {
		writeBootstrapError(
			w,
			http.StatusUnauthorized,
			"invalid_bootstrap_code",
			"invalid, expired, or already used setup code",
		)
		return
	}
	verification, err := s.bootstrap.VerifyBootstrapCode(r.Context(), bootstrap.HashCode(req.SetupCode))
	if err != nil {
		if errors.Is(err, repository.ErrBootstrapSealed) && s.config.SetupMode && s.setupState != nil {
			codeHash := bootstrap.HashCode(req.SetupCode)
			state, stateErr := s.setupState.Load(r.Context())
			encodedHash := base64.RawURLEncoding.EncodeToString(codeHash)
			_, sessionErr := uuid.Parse(state.SetupSessionID)
			stateValid := stateErr == nil && state.Phase != setupstate.PhaseComplete &&
				state.SetupExpiresAt.After(time.Now().UTC())
			sessionValid := sessionErr == nil
			if stateValid && sessionValid &&
				subtle.ConstantTimeCompare([]byte(state.SetupCodeHash), []byte(encodedHash)) == 1 {
				if cookieErr := s.setSetupCookie(w, r, state.SetupSessionID, codeHash, state.SetupExpiresAt); cookieErr != nil {
					internalError(s, w, cookieErr)
					return
				}
				writeBootstrapJSON(
					w,
					http.StatusOK,
					verifyBootstrapCodeResponse{
						AuthorizationSessionID: state.SetupSessionID,
						ExpiresAt:              state.SetupExpiresAt,
					},
				)
				return
			}
		}
		switch {
		case errors.Is(err, repository.ErrBootstrapSealed):
			writeBootstrapError(w, http.StatusGone, "bootstrap_complete", "instance setup has already been completed")
		case errors.Is(err, repository.ErrInvalidBootstrapCode):
			writeBootstrapError(
				w,
				http.StatusUnauthorized,
				"invalid_bootstrap_code",
				"invalid, expired, or already used setup code",
			)
		default:
			internalError(s, w, err)
		}
		return
	}
	if s.config.SetupMode {
		if s.setupState == nil {
			writeBootstrapError(
				w,
				http.StatusServiceUnavailable,
				"setup_unavailable",
				"browser setup is temporarily unavailable",
			)
			return
		}
		codeHash := bootstrap.HashCode(req.SetupCode)
		_, claimErr := s.setupState.Update(r.Context(), func(state *setupstate.State) error {
			if setupstate.InstallationLocked(*state) {
				return errors.New("installation is already in progress or complete")
			}
			if state.SetupSessionID != "" || state.SetupCodeHash != "" {
				return errors.New("setup code already claimed")
			}
			state.SetupSessionID = verification.ID.String()
			state.SetupCodeHash = base64.RawURLEncoding.EncodeToString(codeHash)
			state.SetupExpiresAt = verification.ExpiresAt
			state.GitHub.AuthorizationSession = verification.ID.String()
			return nil
		})
		if claimErr != nil {
			writeBootstrapError(
				w,
				http.StatusConflict,
				"setup_code_used",
				"this setup code has already been used; request a new code from the local CLI",
			)
			return
		}
		if err := s.setSetupCookie(w, r, verification.ID.String(), codeHash, verification.ExpiresAt); err != nil {
			_, _ = s.setupState.Update(r.Context(), func(state *setupstate.State) error {
				if setupstate.InstallationLocked(*state) {
					return nil
				}
				if state.SetupSessionID == verification.ID.String() &&
					state.SetupCodeHash == base64.RawURLEncoding.EncodeToString(codeHash) {
					state.SetupSessionID = ""
					state.SetupCodeHash = ""
					state.SetupExpiresAt = time.Time{}
				}
				return nil
			})
			internalError(s, w, err)
			return
		}
	}
	writeBootstrapJSON(
		w,
		http.StatusOK,
		verifyBootstrapCodeResponse{
			AuthorizationSessionID: verification.ID.String(),
			ExpiresAt:              verification.ExpiresAt,
		},
	)
}

type githubOwnerResult = bootstrap.OwnerResult

func (s *Server) createGitHubInstanceOwner(
	ctx context.Context,
	authorization bootstrap.GitHubAuthorization,
	user githubauth.User,
) (githubOwnerResult, error) {
	creator := bootstrap.OwnerCreator{
		Store:      s.bootstrap,
		Handoff:    s.setupHandoff,
		SetupMode:  s.config.SetupMode,
		SessionTTL: s.config.SessionTTL,
	}
	return creator.CreateGitHubInstanceOwner(ctx, authorization, user)
}

func (s *Server) listBootstrapAdoptionAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.verifyBootstrapCLI(w, r) {
		return
	}
	accounts, err := s.bootstrap.ListBootstrapAdoptionAccounts(r.Context())
	if err != nil {
		if errors.Is(err, repository.ErrBootstrapSealed) {
			writeBootstrapError(
				w,
				http.StatusConflict,
				"adoption_unavailable",
				"instance owner adoption is not available",
			)
			return
		}
		internalError(s, w, err)
		return
	}
	writeBootstrapJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (s *Server) adoptBootstrapOwner(w http.ResponseWriter, r *http.Request) {
	if !s.verifyBootstrapCLI(w, r) {
		return
	}
	var req adoptInstanceOwnerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(req.AccountID))
	if err != nil {
		writeBootstrapError(w, http.StatusUnprocessableEntity, "validation_error", "account_id must be a UUID")
		return
	}
	if err := s.bootstrap.AdoptInstanceOwner(r.Context(), accountID); err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			writeBootstrapError(w, http.StatusNotFound, "not_found", "account was not found")
		case errors.Is(err, repository.ErrBootstrapSealed):
			writeBootstrapError(
				w,
				http.StatusConflict,
				"adoption_unavailable",
				"instance owner adoption is not available",
			)
		default:
			internalError(s, w, err)
		}
		return
	}
	writeBootstrapJSON(w, http.StatusOK, map[string]string{"status": "adopted", "account_id": accountID.String()})
}

func (s *Server) verifyBootstrapCLI(w http.ResponseWriter, r *http.Request) bool {
	if !bootstrap.VerifyCLIProof(s.bootstrapCLIKey(), r.Header.Get(bootstrap.CLIProofHeader)) {
		writeBootstrapError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return false
	}
	return true
}

func (s *Server) bootstrapConfigured() bool {
	return len(s.bootstrapCLIKey()) == 32 && (s.config.SetupMode || strings.TrimSpace(s.config.GitHubAppClientID) != "")
}

func (s *Server) bootstrapCLIKey() []byte {
	return s.config.BootstrapCLIKey
}

func (s *Server) allowBootstrapAttempt(w http.ResponseWriter, r *http.Request, dimension string) bool {
	return s.allowBootstrapAttemptWith(w, r, dimension, s.config.AuthRateLimit, s.config.AuthRateWindow, "bootstrap")
}

func (s *Server) allowBootstrapAttemptWith(
	w http.ResponseWriter,
	r *http.Request,
	dimension string,
	limit int,
	window time.Duration,
	operation string,
) bool {
	keys := []string{ratelimit.InstanceIPKey(operation, s.requestClientIP(r))}
	if dimension != "" {
		keys = append(
			keys,
			ratelimit.InstanceKey(operation, strings.ToLower(strings.TrimSpace(dimension)), s.requestClientIP(r)),
		)
	}
	for _, key := range keys {
		decision, err := s.limiter.Allow(r.Context(), key, limit, window)
		if err != nil {
			s.logger.Error("bootstrap rate limiter failed", "error", err)
			writeBootstrapError(
				w,
				http.StatusServiceUnavailable,
				"service_unavailable",
				"authentication protection is temporarily unavailable",
			)
			return false
		}
		if !decision.Allowed {
			return writeRateLimited(w, decision.RetryAfter)
		}
	}
	return true
}

func writeBootstrapJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, value)
}

func writeBootstrapError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeError(w, status, code, message)
}
