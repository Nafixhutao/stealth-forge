package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/ratelimit"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Rate limiting for project operations, Agent runs, and the public account
// endpoints. Every limiter runs after the route's authentication middleware.
// Cross-origin browser clients need the project session cookie on credentialed
// requests. SameSite=None is only emitted alongside Secure; local HTTP
// development keeps Lax so browsers do not discard the cookie outright.
func projectSessionSameSite(secure bool) http.SameSite {
	if secure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (s *Server) allowPublicAuth(
	w http.ResponseWriter,
	r *http.Request,
	operation string,
	projectID uuid.UUID,
	normalizedEmail string,
) bool {
	clientIP := s.requestClientIP(r)
	keys := []string{
		ratelimit.ProjectIPKey(operation, projectID.String(), clientIP),
		ratelimit.Key(operation, projectID.String(), normalizedEmail, clientIP),
	}
	for _, key := range keys {
		decision, err := s.limiter.Allow(r.Context(), key, s.config.AuthRateLimit, s.config.AuthRateWindow)
		if err != nil {
			s.logger.Error("public auth rate limiter failed", "operation", operation, "error", err)
			writeError(
				w,
				http.StatusServiceUnavailable,
				"service_unavailable",
				"authentication protection is temporarily unavailable",
			)
			return false
		}
		if decision.Allowed {
			continue
		}
		return writeRateLimited(w, decision.RetryAfter)
	}
	return true
}

// allowAccountAuth applies the same two-dimensional protection used by
// project Auth to the Console's public recovery/verification endpoints. The
// literal namespace keeps account addresses and project addresses separate in
// Redis without putting raw PII in keys.
func (s *Server) allowAccountAuth(w http.ResponseWriter, r *http.Request, operation, normalizedEmail string) bool {
	clientIP := s.requestClientIP(r)
	keys := []string{
		ratelimit.ProjectIPKey(operation, "console", clientIP),
		ratelimit.Key(operation, "console", normalizedEmail, clientIP),
	}
	for _, key := range keys {
		decision, err := s.limiter.Allow(r.Context(), key, s.config.AuthRateLimit, s.config.AuthRateWindow)
		if err != nil {
			s.logger.Error("account auth rate limiter failed", "operation", operation, "error", err)
			writeError(
				w,
				http.StatusServiceUnavailable,
				"service_unavailable",
				"authentication protection is temporarily unavailable",
			)
			return false
		}
		if decision.Allowed {
			continue
		}
		return writeRateLimited(w, decision.RetryAfter)
	}
	return true
}

// rateLimitProjectOperation applies a bounded safety budget after the route's
// authentication middleware has established an actor. Authenticated requests
// use a stable project/actor bucket; anonymous function calls use a separate
// project/IP bucket so clients behind a trusted proxy do not share one bucket.
func (s *Server) rateLimitProjectOperation(operation string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID, err := repository.ParseUUID(chi.URLParam(r, "projectID"))
			if err != nil {
				writeError(w, http.StatusBadRequest, "validation_error", "projectID must be a UUID")
				return
			}
			if !s.allowOperationRateLimit(w, r, operation, projectID.String(), rateLimitActorID(r)) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) rateLimitAgentRun(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agentID, err := repository.ParseUUID(chi.URLParam(r, "agentID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "agentID must be a UUID")
			return
		}
		account, ok := r.Context().Value(accountContextKey).(domain.Account)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
			return
		}
		// Agent runs are budgeted by project, not by the Agent resource. The
		// narrow repository lookup avoids loading the full projection before the
		// existing transactional CreateAgentRun authorization/lock path.
		projectID, err := s.repo.AgentProjectID(r.Context(), mustUUID(account.ID), agentID)
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "agent was not found")
			return
		}
		if err != nil {
			internalError(s, w, err)
			return
		}
		if !s.allowOperationRateLimit(w, r, "agent_run", projectID.String(), "account:"+account.ID) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowOperationRateLimit(
	w http.ResponseWriter,
	r *http.Request,
	operation, scope, actorID string,
) bool {
	key := ratelimit.ActorKey(operation, scope, actorID)
	if actorID == "" {
		key = ratelimit.ProjectIPKey(operation, scope, s.requestClientIP(r))
	}
	decision, err := s.limiter.Allow(
		r.Context(),
		key,
		s.config.ProjectOperationRateLimit,
		s.config.ProjectOperationRateWindow,
	)
	if err != nil {
		s.logger.Error(
			"project operation rate limiter failed",
			"operation",
			operation,
			"scope",
			scope,
			"request_id",
			requestIDFrom(r.Context()),
			"error",
			err,
		)
		writeError(
			w,
			http.StatusServiceUnavailable,
			"service_unavailable",
			"rate limiting protection is temporarily unavailable",
		)
		return false
	}
	if !decision.Allowed {
		return writeOperationRateLimited(w, decision.RetryAfter)
	}
	return true
}

func rateLimitActorID(r *http.Request) string {
	if actor, ok := r.Context().Value(projectActorContextKey).(projectActor); ok {
		if actor.kind == apiKeyProjectActor {
			return "api_key:" + actor.apiKeyID.String()
		}
		if account, ok := r.Context().Value(accountContextKey).(domain.Account); ok {
			return "account:" + account.ID
		}
	}
	if actor, ok := r.Context().Value(projectDataActorContextKey).(projectDataActor); ok {
		return databaseRateLimitActorID(actor.actor)
	}
	if actor, ok := r.Context().Value(projectStorageActorContextKey).(repository.StorageActor); ok {
		return databaseRateLimitActorID(actor)
	}
	if user, ok := r.Context().Value(projectUserContextKey).(domain.ApplicationUser); ok {
		return "project_user:" + user.ID
	}
	if account, ok := r.Context().Value(accountContextKey).(domain.Account); ok {
		return "account:" + account.ID
	}
	return ""
}

func databaseRateLimitActorID(actor repository.DatabaseActor) string {
	switch actor.Kind {
	case repository.DatabaseConsoleActor:
		return "account:" + actor.AccountID.String()
	case repository.DatabaseAPIKeyActor:
		return "api_key:" + actor.APIKeyID.String()
	case repository.DatabaseApplicationActor:
		return "project_user:" + actor.ProjectUserID.String()
	default:
		return ""
	}
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) bool {
	return writeRateLimitedMessage(w, retryAfter, "too many authentication attempts; retry later")
}

func writeOperationRateLimited(w http.ResponseWriter, retryAfter time.Duration) bool {
	return writeRateLimitedMessage(w, retryAfter, "operation rate limit exceeded; retry later")
}

func writeRateLimitedMessage(w http.ResponseWriter, retryAfter time.Duration, message string) bool {
	retryAfterSeconds := int(retryAfter / time.Second)
	if retryAfter%time.Second != 0 {
		retryAfterSeconds++
	}
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	writeError(w, http.StatusTooManyRequests, "rate_limited", message)
	return false
}
