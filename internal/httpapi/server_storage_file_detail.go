package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"os"

	"github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/storage"
)

func (s *Server) getStorageFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	fileID, ok := pathUUID(w, r, "fileID")
	if !ok {
		return
	}
	item, err := s.repo.GetStorageFile(r.Context(), projectID, bucketID, fileID, storageActorFrom(r))
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.StorageFile{"file": item})
}

type storageFilePatchRequest struct {
	Name              *string   `json:"name"`
	ReadPermissions   *[]string `json:"read_permissions"`
	UpdatePermissions *[]string `json:"update_permissions"`
	DeletePermissions *[]string `json:"delete_permissions"`
}

func (s *Server) updateStorageFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	fileID, ok := pathUUID(w, r, "fileID")
	if !ok {
		return
	}
	var req storageFilePatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == nil && req.ReadPermissions == nil && req.UpdatePermissions == nil && req.DeletePermissions == nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one file setting is required")
		return
	}
	if req.Name != nil {
		if err := storage.ValidateFilename(*req.Name); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "filename is invalid")
			return
		}
	}
	item, err := s.repo.UpdateStorageFile(r.Context(), projectID, bucketID, fileID, storageActorFrom(r), repository.StorageFilePatch{Name: req.Name, ReadPermissions: req.ReadPermissions, UpdatePermissions: req.UpdatePermissions, DeletePermissions: req.DeletePermissions})
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.StorageFile{"file": item})
}

func (s *Server) downloadStorageFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	fileID, ok := pathUUID(w, r, "fileID")
	if !ok {
		return
	}
	if s.storage == nil {
		internalError(s, w, errors.New("storage is unavailable"))
		return
	}
	item, path, err := s.repo.StorageFileForDownload(r.Context(), projectID, bucketID, fileID, storageActorFrom(r))
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	file, err := s.storage.OpenRelative(r.Context(), path)
	if errors.Is(err, os.ErrNotExist) {
		internalError(s, w, errors.New("storage metadata references a missing blob"))
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	defer file.Close()
	contentDisposition := "attachment"
	if formatted := mime.FormatMediaType("attachment", map[string]string{"filename": item.Name}); formatted != "" {
		contentDisposition = formatted
	}
	w.Header().Set("Content-Disposition", contentDisposition)
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, item.Name, item.CreatedAt, file)
}

func (s *Server) deleteStorageFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	fileID, ok := pathUUID(w, r, "fileID")
	if !ok {
		return
	}
	_, err := s.repo.DeleteStorageFile(r.Context(), projectID, bucketID, fileID, storageActorFrom(r))
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func storageResourceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRowHidden):
		writeError(w, http.StatusNotFound, "not_found", "storage resource was not found")
		return true
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to access this storage resource")
		return true
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "storage resource conflicts with an existing resource")
		return true
	case errors.Is(err, repository.ErrStorageQuotaExceeded):
		writeError(w, http.StatusRequestEntityTooLarge, "storage_quota_exceeded", "storage bucket quota would be exceeded")
		return true
	case errors.Is(err, repository.ErrStorageFileTooLarge), errors.Is(err, storage.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "file exceeds the configured maximum size")
		return true
	case errors.Is(err, database.ErrInvalidPermissions), errors.Is(err, database.ErrDuplicatePermission), errors.Is(err, storage.ErrInvalidFilename), errors.Is(err, storage.ErrInvalidMIME):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return true
	}
	return false
}
