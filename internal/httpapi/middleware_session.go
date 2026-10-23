package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Nafixhutao/stealth-forge/internal/auth"
	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

// Console session lookup and cookie handling, plus the request body limit.

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.config.SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, 401, "unauthorized", "authentication is required")
			return
		}
		account, sessionID, ok := s.lookupSession(w, r, cookie.Value)
		if !ok {
			return
		}
		ctx := context.WithValue(r.Context(), accountContextKey, account)
		recordTraceAccount(ctx, account)
		ctx = context.WithValue(ctx, sessionContextKey, sessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// lookupSession resolves a session cookie to an account. A missing, expired,
// or revoked session is a 401, but any other repository failure is an
// infrastructure error that is logged and returned as a 500, so a database
// outage is never silently reported to clients as a mass logout.
func (s *Server) lookupSession(w http.ResponseWriter, r *http.Request, token string) (domain.Account, uuid.UUID, bool) {
	account, sessionID, err := s.repo.AccountBySession(r.Context(), auth.HashSessionToken(token))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
			return domain.Account{}, uuid.Nil, false
		}
		internalError(s, w, err)
		return domain.Account{}, uuid.Nil, false
	}
	return account, sessionID, true
}

func accountFrom(r *http.Request) domain.Account {
	return requestValue[domain.Account](r, accountContextKey, "requireSession")
}

func sessionFrom(r *http.Request) uuid.UUID {
	return requestValue[uuid.UUID](r, sessionContextKey, "requireSession")
}
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     s.config.SessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.CookieSecure,
			SameSite: projectSessionSameSite(s.config.CookieSecure),
			MaxAge:   int(s.config.SessionTTL.Seconds()),
			Expires:  time.Now().UTC().Add(s.config.SessionTTL),
		},
	)
}
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     s.config.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   s.config.CookieSecure,
			SameSite: projectSessionSameSite(s.config.CookieSecure),
			MaxAge:   -1,
			Expires:  time.Unix(1, 0),
		},
	)
}

func (s *Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(maxBodyBytes)
		contentType := strings.ToLower(r.Header.Get("Content-Type"))
		isMultipart := strings.HasPrefix(contentType, "multipart/form-data")
		path := r.URL.Path
		isStorageUpload := strings.Contains(path, "/storage/")
		isFunctionDeployment := strings.Contains(path, "/functions/") && strings.Contains(path, "/deployments")
		isSiteDeployment := strings.Contains(path, "/sites/") && strings.Contains(path, "/deployments")
		isAppDeployment := strings.Contains(path, "/apps/") && strings.Contains(path, "/deployments")
		if isMultipart && (isStorageUpload || isFunctionDeployment || isSiteDeployment || isAppDeployment) {
			configured := s.config.StorageMaxFileSize
			if strings.Contains(r.URL.Path, "/functions/") {
				configured = s.config.FunctionsMaxArtifactSize
			}
			if strings.Contains(r.URL.Path, "/sites/") {
				configured = s.config.SitesMaxArtifactSize
			}
			if strings.Contains(r.URL.Path, "/apps/") {
				configured = s.config.AppsMaxSourceArchiveBytes
			}
			limit = configured + maxMultipartOverhead
			if limit < configured || limit <= 0 {
				limit = int64(maxBodyBytes)
			}
		}
		if r.ContentLength > limit {
			writeError(
				w,
				http.StatusRequestEntityTooLarge,
				"payload_too_large",
				"request body exceeds the configured upload limit",
			)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
