package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/auth"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/validate"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const projectStorageActorContextKey contextKey = "project-storage-actor"

// requireProjectStorageActor applies the same explicit actor precedence as
// Database data routes: X-Stealth-Key, project app cookie, Console cookie,
// then anonymous. Invalid credentials stop the request; they never fall
// through to a weaker actor.
func (s *Server) requireProjectStorageActor(next http.Handler) http.Handler {
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
			ctx := context.WithValue(r.Context(), projectStorageActorContextKey, repository.StorageActor{Kind: repository.StorageAPIKeyActor, APIKeyID: keyID, APIKeyScopes: key.Scopes})
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
			ctx := context.WithValue(r.Context(), projectStorageActorContextKey, repository.StorageActor{Kind: repository.StorageApplicationActor, ProjectUserID: mustUUID(user.ID)})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if cookie, err := r.Cookie(s.config.SessionCookieName); err == nil && cookie.Value != "" {
			account, sessionID, err := s.repo.AccountBySession(r.Context(), auth.HashSessionToken(cookie.Value))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
				return
			}
			ctx := context.WithValue(r.Context(), accountContextKey, account)
			ctx = context.WithValue(ctx, sessionContextKey, sessionID)
			ctx = context.WithValue(ctx, projectStorageActorContextKey, repository.StorageActor{Kind: repository.StorageConsoleActor, AccountID: mustUUID(account.ID)})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		ctx := context.WithValue(r.Context(), projectStorageActorContextKey, repository.StorageActor{Kind: repository.StorageAnonymousActor})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func storageActorFrom(r *http.Request) repository.StorageActor {
	if actor, ok := r.Context().Value(projectStorageActorContextKey).(repository.StorageActor); ok {
		return actor
	}
	if actor, ok := r.Context().Value(projectActorContextKey).(projectActor); ok {
		if actor.kind == apiKeyProjectActor {
			return repository.StorageActor{Kind: repository.StorageAPIKeyActor, APIKeyID: actor.apiKeyID, APIKeyScopes: actor.scopes}
		}
		return repository.StorageActor{Kind: repository.StorageConsoleActor, AccountID: mustUUID(accountFrom(r).ID)}
	}
	return repository.StorageActor{Kind: repository.StorageAnonymousActor}
}

func managementStorageActorFrom(r *http.Request) repository.StorageActor {
	return storageActorFrom(r)
}

type storageBucketRequest struct {
	Name              string    `json:"name"`
	FileSecurity      *bool     `json:"file_security"`
	CreatePermissions *[]string `json:"create_permissions"`
	ReadPermissions   *[]string `json:"read_permissions"`
	UpdatePermissions *[]string `json:"update_permissions"`
	DeletePermissions *[]string `json:"delete_permissions"`
	// write_permissions is a compatibility alias for clients that model a
	// single write grant. It maps to both create and update when those fields
	// are omitted explicitly.
	WritePermissions *[]string `json:"write_permissions"`
	MaxFileSizeBytes *int64    `json:"max_file_size_bytes"`
	QuotaBytes       *int64    `json:"quota_bytes"`
}

func (s *Server) listStorageBuckets(w http.ResponseWriter, r *http.Request) {
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
		parsed := mustUUID(cursor)
		cursorID = &parsed
	}
	items, next, canManage, err := s.repo.ListStorageBuckets(r.Context(), projectID, managementStorageActorFrom(r), limit, cursorID)
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) createStorageBucket(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req storageBucketRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := validate.Slug(req.Name, "name")
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	fileSecurity := true
	if req.FileSecurity != nil {
		fileSecurity = *req.FileSecurity
	}
	quota := s.config.StorageDefaultQuotaBytes
	if req.QuotaBytes != nil {
		quota = *req.QuotaBytes
	}
	maxFileSize := s.config.StorageMaxFileSize
	if req.MaxFileSizeBytes != nil {
		maxFileSize = *req.MaxFileSizeBytes
	}
	if quota <= 0 || maxFileSize <= 0 || maxFileSize > s.config.StorageMaxFileSize || maxFileSize > quota {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "max_file_size_bytes must be positive, within STORAGE_MAX_FILE_SIZE, and no larger than quota_bytes")
		return
	}
	createPermissions, readPermissions, updatePermissions, deletePermissions, err := bucketPermissionsFromRequest(req, false)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.CreateStorageBucket(r.Context(), uuid.Must(uuid.NewV7()), projectID, managementStorageActorFrom(r), repository.StorageBucketInput{
		Name: name, FileSecurity: fileSecurity, CreatePermissions: createPermissions, ReadPermissions: readPermissions, UpdatePermissions: updatePermissions, DeletePermissions: deletePermissions, MaxFileSizeBytes: maxFileSize, QuotaBytes: quota,
	})
	if planLimitError(w, err) {
		return
	}
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.StorageBucket{"bucket": item})
}

func bucketPermissionsFromRequest(req storageBucketRequest, update bool) ([]string, []string, []string, []string, error) {
	if req.WritePermissions != nil {
		if req.CreatePermissions != nil || req.UpdatePermissions != nil {
			return nil, nil, nil, nil, errors.New("write_permissions cannot be combined with create_permissions or update_permissions")
		}
		req.CreatePermissions = req.WritePermissions
		req.UpdatePermissions = req.WritePermissions
	}
	if update {
		return dereferencePermissions(req.CreatePermissions), dereferencePermissions(req.ReadPermissions), dereferencePermissions(req.UpdatePermissions), dereferencePermissions(req.DeletePermissions), nil
	}
	return permissionsOrEmpty(req.CreatePermissions), permissionsOrEmpty(req.ReadPermissions), permissionsOrEmpty(req.UpdatePermissions), permissionsOrEmpty(req.DeletePermissions), nil
}

func permissionsOrEmpty(value *[]string) []string {
	if value == nil {
		return []string{}
	}
	return append([]string(nil), (*value)...)
}

func dereferencePermissions(value *[]string) []string {
	if value == nil {
		return nil
	}
	return append([]string(nil), (*value)...)
}

func (s *Server) getStorageBucket(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	item, err := s.repo.GetStorageBucket(r.Context(), projectID, bucketID, managementStorageActorFrom(r))
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.StorageBucket{"bucket": item})
}

func (s *Server) updateStorageBucket(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	var req storageBucketRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" && req.FileSecurity == nil && req.CreatePermissions == nil && req.ReadPermissions == nil && req.UpdatePermissions == nil && req.DeletePermissions == nil && req.WritePermissions == nil && req.MaxFileSizeBytes == nil && req.QuotaBytes == nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one bucket setting is required")
		return
	}
	var name *string
	if req.Name != "" {
		validated, err := validate.Slug(req.Name, "name")
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
			return
		}
		name = &validated
	}
	if req.MaxFileSizeBytes != nil && (*req.MaxFileSizeBytes <= 0 || *req.MaxFileSizeBytes > s.config.StorageMaxFileSize) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "max_file_size_bytes must be within STORAGE_MAX_FILE_SIZE")
		return
	}
	if req.QuotaBytes != nil && *req.QuotaBytes <= 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "quota_bytes must be positive")
		return
	}
	createPermissions, readPermissions, updatePermissions, deletePermissions, err := bucketPermissionsFromRequest(req, true)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if req.WritePermissions != nil {
		// bucketPermissionsFromRequest normalizes the compatibility alias on a
		// copy; retain presence information so PATCH does not silently ignore it.
		req.CreatePermissions = req.WritePermissions
		req.UpdatePermissions = req.WritePermissions
	}
	item, err := s.repo.UpdateStorageBucket(r.Context(), projectID, bucketID, managementStorageActorFrom(r), repository.StorageBucketPatch{Name: name, FileSecurity: req.FileSecurity, CreatePermissions: permissionPointer(req.CreatePermissions, createPermissions), ReadPermissions: permissionPointer(req.ReadPermissions, readPermissions), UpdatePermissions: permissionPointer(req.UpdatePermissions, updatePermissions), DeletePermissions: permissionPointer(req.DeletePermissions, deletePermissions), MaxFileSizeBytes: req.MaxFileSizeBytes, QuotaBytes: req.QuotaBytes})
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.StorageBucket{"bucket": item})
}

func permissionPointer(original *[]string, value []string) *[]string {
	if original == nil {
		return nil
	}
	return &value
}

func (s *Server) deleteStorageBucket(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	_, err := s.repo.DeleteStorageBucket(r.Context(), projectID, bucketID, managementStorageActorFrom(r))
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
