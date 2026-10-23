package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/apikey"
	"github.com/Nafixhutao/stealth-forge/internal/auth"
	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Project-scoped authentication: management actors, function-execution
// actors, App sessions, and the project session cookie helpers.
func (s *Server) requireProjectManagement(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, err := repository.ParseUUID(chi.URLParam(r, "projectID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "projectID must be a UUID")
			return
		}
		// An explicit server key takes precedence over any ambient Console
		// cookie, so its project binding and scopes can never be bypassed by a
		// browser session. The application-user cookie is intentionally ignored.
		secret := r.Header.Get("X-Stealth-Key")
		if secret != "" {
			if err := apikey.ValidateSecret(secret); err != nil {
				if !s.allowFailedProjectAPIKeyAuth(w, r, projectID) {
					return
				}
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			key, err := s.repo.AuthenticateProjectAPIKey(r.Context(), projectID, apikey.HashSecret(secret))
			if errors.Is(err, repository.ErrNotFound) {
				if !s.allowFailedProjectAPIKeyAuth(w, r, projectID) {
					return
				}
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
			keyID, err := repository.ParseUUID(key.ID)
			if err != nil {
				internalError(s, w, err)
				return
			}
			if err := s.repo.TouchProjectAPIKey(r.Context(), keyID); err != nil {
				internalError(s, w, err)
				return
			}
			ctx := context.WithValue(
				r.Context(),
				projectActorContextKey,
				projectActor{kind: apiKeyProjectActor, apiKeyID: keyID, scopes: key.Scopes},
			)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if cookie, err := r.Cookie(s.config.SessionCookieName); err == nil && cookie.Value != "" {
			account, sessionID, ok := s.lookupSession(w, r, cookie.Value)
			if !ok {
				return
			}
			// A session alone is not authority over a project: the account must
			// belong to the project's organization. Without this check the only
			// thing standing between a signed-in account and another tenant's
			// project was the role lookup inside each repository, so any handler
			// that forgot one would have been reachable. A non-member gets 404
			// rather than 403, which is the same answer the API gives for a
			// project that does not exist.
			accountID, parseErr := repository.ParseUUID(account.ID)
			if parseErr != nil {
				internalError(s, w, parseErr)
				return
			}
			if _, roleErr := s.repo.ProjectRoleForAccount(r.Context(), projectID, accountID); roleErr != nil {
				if errors.Is(roleErr, repository.ErrNotFound) {
					writeError(w, http.StatusNotFound, "not_found", "project was not found")
					return
				}
				internalError(s, w, roleErr)
				return
			}
			ctx := context.WithValue(r.Context(), accountContextKey, account)
			recordTraceAccount(ctx, account)
			ctx = context.WithValue(ctx, sessionContextKey, sessionID)
			ctx = context.WithValue(ctx, projectActorContextKey, projectActor{kind: consoleProjectActor})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
	})
}

// requireFunctionExecutionActor accepts the three actors that may invoke a
// function: a Console/API-key management actor, an authenticated project
// user, or an anonymous caller when the function grants "any". Credentials
// are explicit and ordered; a malformed credential never falls through to a
// weaker actor.
func (s *Server) requireFunctionExecutionActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, err := repository.ParseUUID(chi.URLParam(r, "projectID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "projectID must be a UUID")
			return
		}
		if secret := r.Header.Get("X-Stealth-Key"); secret != "" {
			if err := apikey.ValidateSecret(secret); err != nil {
				if !s.allowFailedProjectAPIKeyAuth(w, r, projectID) {
					return
				}
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			key, err := s.repo.AuthenticateProjectAPIKey(r.Context(), projectID, apikey.HashSecret(secret))
			if errors.Is(err, repository.ErrNotFound) {
				if !s.allowFailedProjectAPIKeyAuth(w, r, projectID) {
					return
				}
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
			keyID, err := repository.ParseUUID(key.ID)
			if err != nil {
				internalError(s, w, err)
				return
			}
			if err := s.repo.TouchProjectAPIKey(r.Context(), keyID); err != nil {
				internalError(s, w, err)
				return
			}
			ctx := context.WithValue(
				r.Context(),
				projectActorContextKey,
				projectActor{kind: apiKeyProjectActor, apiKeyID: keyID, scopes: key.Scopes},
			)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if cookie, err := r.Cookie(projectSessionCookieName(projectID)); err == nil && cookie.Value != "" {
			user, _, err := s.repo.ApplicationUserBySession(r.Context(), projectID, auth.HashSessionToken(cookie.Value))
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "application authentication is required")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
			ctx := context.WithValue(r.Context(), projectUserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if cookie, err := r.Cookie(s.config.SessionCookieName); err == nil && cookie.Value != "" {
			account, sessionID, ok := s.lookupSession(w, r, cookie.Value)
			if !ok {
				return
			}
			ctx := context.WithValue(r.Context(), accountContextKey, account)
			recordTraceAccount(ctx, account)
			ctx = context.WithValue(ctx, sessionContextKey, sessionID)
			ctx = context.WithValue(ctx, projectActorContextKey, projectActor{kind: consoleProjectActor})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// No cookie is an intentional anonymous application invocation. The
		// repository checks execute_permissions before accepting it.
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowFailedProjectAPIKeyAuth(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) bool {
	decision, err := s.limiter.Allow(
		r.Context(),
		ratelimit.ProjectIPKey("api_key_auth", projectID.String(), s.requestClientIP(r)),
		s.config.AuthRateLimit,
		s.config.AuthRateWindow,
	)
	if err != nil {
		s.logger.Error("API key rate limiter failed", "error", err)
		writeError(
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
	return true
}

func projectActorFrom(r *http.Request) projectActor {
	return requestValue[projectActor](r, projectActorContextKey, "requireProjectManagement or requireFunctionExecutionActor")
}

func (s *Server) requireProjectAppSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, err := repository.ParseUUID(chi.URLParam(r, "projectID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "projectID must be a UUID")
			return
		}
		cookie, err := r.Cookie(projectSessionCookieName(projectID))
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "application authentication is required")
			return
		}
		item, sessionID, err := s.repo.ApplicationUserBySession(
			r.Context(),
			projectID,
			auth.HashSessionToken(cookie.Value),
		)
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "application authentication is required")
			return
		}
		if err != nil {
			internalError(s, w, err)
			return
		}
		ctx := context.WithValue(r.Context(), projectUserContextKey, item)
		ctx = context.WithValue(ctx, projectUserSessionContextKey, sessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func projectUserFrom(r *http.Request) domain.ApplicationUser {
	return requestValue[domain.ApplicationUser](r, projectUserContextKey, "requireProjectAppSession")
}

func projectUserSessionFrom(r *http.Request) uuid.UUID {
	return requestValue[uuid.UUID](r, projectUserSessionContextKey, "requireProjectAppSession")
}

func projectSessionCookieName(projectID uuid.UUID) string {
	return "stealth_app_" + strings.ReplaceAll(projectID.String(), "-", "")
}

func projectSessionCookiePath(projectID uuid.UUID) string {
	return "/v1/projects/" + projectID.String()
}

func (s *Server) setProjectSessionCookie(w http.ResponseWriter, projectID uuid.UUID, token string) {
	expires := time.Now().UTC().Add(s.config.AppSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     projectSessionCookieName(projectID),
		Value:    token,
		Path:     projectSessionCookiePath(projectID),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: projectSessionSameSite(s.config.CookieSecure),
		MaxAge:   int(s.config.AppSessionTTL.Seconds()),
		Expires:  expires,
	})
}

func (s *Server) clearProjectSessionCookie(w http.ResponseWriter, projectID uuid.UUID) {
	http.SetCookie(w, &http.Cookie{
		Name:     projectSessionCookieName(projectID),
		Value:    "",
		Path:     projectSessionCookiePath(projectID),
		HttpOnly: true,
		Secure:   s.config.CookieSecure,
		SameSite: projectSessionSameSite(s.config.CookieSecure),
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}
