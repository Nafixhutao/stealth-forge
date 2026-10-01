package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/auth"
	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const projectDataActorContextKey contextKey = "project-data-actor"

type projectDataActor struct {
	actor repository.DatabaseActor
}

func (s *Server) requireProjectDataActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, err := repository.ParseUUID(chi.URLParam(r, "projectID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "projectID must be a UUID")
			return
		}
		// Explicit server credentials win over ambient browser state. This is
		// the only path that accepts X-Stealth-Key; app cookies are never used
		// for Console management authorization.
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
			ctx := context.WithValue(r.Context(), projectDataActorContextKey, projectDataActor{actor: repository.DatabaseActor{Kind: repository.DatabaseAPIKeyActor, APIKeyID: keyID, APIKeyScopes: key.Scopes}})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if cookie, err := r.Cookie(projectSessionCookieName(projectID)); err == nil && cookie.Value != "" {
			user, _, err := s.repo.ApplicationUserBySession(r.Context(), projectID, authHashSessionToken(cookie.Value))
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, http.StatusUnauthorized, "unauthorized", "application authentication is required")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
			ctx := context.WithValue(r.Context(), projectDataActorContextKey, projectDataActor{actor: repository.DatabaseActor{Kind: repository.DatabaseApplicationActor, ProjectUserID: mustUUID(user.ID)}})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if cookie, err := r.Cookie(s.config.SessionCookieName); err == nil && cookie.Value != "" {
			account, sessionID, err := s.repo.AccountBySession(r.Context(), authHashSessionToken(cookie.Value))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			ctx := context.WithValue(r.Context(), accountContextKey, account)
			ctx = context.WithValue(ctx, sessionContextKey, sessionID)
			ctx = context.WithValue(ctx, projectDataActorContextKey, projectDataActor{actor: repository.DatabaseActor{Kind: repository.DatabaseConsoleActor, AccountID: mustUUID(account.ID)}})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		ctx := context.WithValue(r.Context(), projectDataActorContextKey, projectDataActor{actor: repository.DatabaseActor{Kind: repository.DatabaseAnonymousActor}})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// These tiny wrappers keep this file independent from the unexported helpers
// in server.go while preserving the same token hashing implementation.
func authHashSessionToken(token string) []byte { return auth.HashSessionToken(token) }
func mustUUID(value string) uuid.UUID          { parsed, _ := uuid.Parse(value); return parsed }

func databaseActorFrom(r *http.Request) repository.DatabaseActor {
	if value, ok := r.Context().Value(projectDataActorContextKey).(projectDataActor); ok {
		return value.actor
	}
	actor := projectActorFrom(r)
	if actor.kind == apiKeyProjectActor {
		return repository.DatabaseActor{Kind: repository.DatabaseAPIKeyActor, APIKeyID: actor.apiKeyID, APIKeyScopes: actor.scopes}
	}
	return repository.DatabaseActor{Kind: repository.DatabaseConsoleActor, AccountID: mustUUID(accountFrom(r).ID)}
}

type databaseCreateRequest struct {
	Name string `json:"name"`
}

type databaseTableRequest struct {
	Name              string   `json:"name"`
	RowSecurity       *bool    `json:"row_security"`
	CreatePermissions []string `json:"create_permissions"`
	ReadPermissions   []string `json:"read_permissions"`
	UpdatePermissions []string `json:"update_permissions"`
	DeletePermissions []string `json:"delete_permissions"`
}

type databaseColumnRequest struct {
	Key         string          `json:"key"`
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	VarcharSize *int            `json:"varchar_size"`
	Default     json.RawMessage `json:"default"`
}

type databaseIndexRequest struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	ColumnKeys []string `json:"column_keys"`
	Directions []string `json:"directions"`
}

type databaseRowRequest struct {
	Data              json.RawMessage `json:"data"`
	ReadPermissions   *[]string       `json:"read_permissions"`
	UpdatePermissions *[]string       `json:"update_permissions"`
	DeletePermissions *[]string       `json:"delete_permissions"`
}

func (s *Server) listProjectDatabases(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	limit, cursor, ok := page(w, r)
	if !ok {
		return
	}
	var cursorID *uuid.UUID
	if cursor != "" {
		parsed, _ := uuid.Parse(cursor)
		cursorID = &parsed
	}
	items, next, canManage, err := s.repo.ListProjectDatabases(r.Context(), projectID, databaseActorFrom(r), limit, cursorID)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"databases": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) createProjectDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req databaseCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := dbcore.ValidateName(req.Name)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.CreateProjectDatabase(r.Context(), uuid.Must(uuid.NewV7()), projectID, databaseActorFrom(r), name)
	if planLimitError(w, err) {
		return
	}
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.ProjectDatabase{"database": item})
}

func (s *Server) getProjectDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	item, err := s.repo.GetProjectDatabase(r.Context(), projectID, databaseID, databaseActorFrom(r))
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.ProjectDatabase{"database": item})
}

func (s *Server) deleteProjectDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	if err := s.repo.DeleteProjectDatabase(r.Context(), projectID, databaseID, databaseActorFrom(r)); databaseResourceError(w, err) {
		return
	} else if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
