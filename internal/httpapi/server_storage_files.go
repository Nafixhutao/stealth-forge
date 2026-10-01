package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/google/uuid"
)

func (s *Server) listStorageFiles(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
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
	items, next, canManage, err := s.repo.ListStorageFiles(r.Context(), projectID, bucketID, storageActorFrom(r), limit, cursorID)
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) uploadStorageFile(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	bucketID, ok := pathUUID(w, r, "bucketID")
	if !ok {
		return
	}
	actor := storageActorFrom(r)
	bucket, err := s.repo.AuthorizeStorageBucket(r.Context(), projectID, bucketID, actor, "create")
	if storageResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	if s.storage == nil {
		internalError(s, w, errors.New("storage is unavailable"))
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be multipart/form-data")
		return
	}
	fileID := uuid.Must(uuid.NewV7())
	var prepared storage.PreparedFile
	var hasFile bool
	var partFilename string
	var explicitName string
	var readPermissions, updatePermissions, deletePermissions *[]string
	seenFields := make(map[string]struct{}, 4)
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			cleanupPrepared(s.storage, &prepared)
			if isMaxBytesError(nextErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the configured upload limit")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_request", "multipart body is invalid")
			}
			return
		}
		field := part.FormName()
		if _, seen := seenFields[field]; seen {
			cleanupPrepared(s.storage, &prepared)
			writeError(w, http.StatusBadRequest, "invalid_request", "multipart fields may occur only once")
			return
		}
		seenFields[field] = struct{}{}
		switch field {
		case "file":
			if hasFile {
				cleanupPrepared(s.storage, &prepared)
				writeError(w, http.StatusBadRequest, "invalid_request", "multipart body may contain only one file field")
				return
			}
			hasFile = true
			filename := part.FileName()
			if filename != "" {
				if filenameErr := storage.ValidateFilename(filename); filenameErr != nil {
					writeError(w, http.StatusUnprocessableEntity, "validation_error", "filename is invalid")
					return
				}
				partFilename = filename
			}
			declaredType := part.Header.Get("Content-Type")
			prepared, err = s.storage.BeginUploadWithLimit(r.Context(), projectID, bucketID, fileID, part, declaredType, bucket.MaxFileSizeBytes)
			if err != nil {
				cleanupPrepared(s.storage, &prepared)
				if isMaxBytesError(err) || errors.Is(err, storage.ErrTooLarge) {
					writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "file exceeds the configured maximum size")
				} else if errors.Is(err, storage.ErrInvalidMIME) {
					writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "file MIME type is invalid")
				} else {
					internalError(s, w, err)
				}
				return
			}
		case "name", "read_permissions", "update_permissions", "delete_permissions":
			value, readErr := readMultipartField(part, 16*1024)
			if readErr != nil {
				cleanupPrepared(s.storage, &prepared)
				if isMaxBytesError(readErr) {
					writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "multipart field is too large")
				} else {
					writeError(w, http.StatusBadRequest, "invalid_request", "multipart field is invalid")
				}
				return
			}
			switch field {
			case "name":
				explicitName = strings.TrimSpace(value)
			case "read_permissions":
				readPermissions, err = parseStoragePermissionsField(value)
			case "update_permissions":
				updatePermissions, err = parseStoragePermissionsField(value)
			case "delete_permissions":
				deletePermissions, err = parseStoragePermissionsField(value)
			}
			if err != nil {
				cleanupPrepared(s.storage, &prepared)
				writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
				return
			}
		default:
			cleanupPrepared(s.storage, &prepared)
			writeError(w, http.StatusBadRequest, "invalid_request", "unsupported multipart field")
			return
		}
	}
	if !hasFile {
		writeError(w, http.StatusBadRequest, "invalid_request", "multipart body must include a file field")
		return
	}
	filenameField := partFilename
	if explicitName != "" {
		if partFilename != "" {
			cleanupPrepared(s.storage, &prepared)
			writeError(w, http.StatusBadRequest, "invalid_request", "name cannot be combined with a multipart filename")
			return
		}
		filenameField = explicitName
	}
	if err := storage.ValidateFilename(filenameField); err != nil {
		cleanupPrepared(s.storage, &prepared)
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "filename is invalid")
		return
	}
	if actor.Kind == repository.StorageAnonymousActor && (readPermissions == nil || updatePermissions == nil || deletePermissions == nil) {
		cleanupPrepared(s.storage, &prepared)
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "anonymous uploads must specify read_permissions, update_permissions, and delete_permissions")
		return
	}
	publishCleanup := repository.ArtifactCleanupInput{
		ProjectID:    projectID,
		StoreKind:    repository.ArtifactCleanupStorage,
		Operation:    repository.ArtifactCleanupRelative,
		RelativePath: prepared.RelativePath,
	}
	if err := s.repo.ReserveArtifactPublishCleanup(r.Context(), publishCleanup); err != nil {
		cleanupPrepared(s.storage, &prepared)
		internalError(s, w, err)
		return
	}
	// Publish after the durable reservation. If the metadata transaction fails,
	// the reservation is retained for the trusted cleanup worker.
	if err := s.storage.Commit(r.Context(), &prepared); err != nil {
		cleanupPrepared(s.storage, &prepared)
		internalError(s, w, err)
		return
	}
	var creator *uuid.UUID
	if actor.Kind == repository.StorageApplicationActor {
		creatorID := actor.ProjectUserID
		creator = &creatorID
	}
	item, err := s.repo.CreateStorageFile(r.Context(), fileID, projectID, bucketID, actor, repository.StorageFileInput{Name: filenameField, MimeType: prepared.ContentType, SizeBytes: prepared.Size, ChecksumSHA256: prepared.Checksum, StoragePath: prepared.RelativePath, ReadPermissions: readPermissions, UpdatePermissions: updatePermissions, DeletePermissions: deletePermissions, CreatorProjectUserID: creator, PublishCleanup: &publishCleanup})
	if storageResourceError(w, err) {
		// The quota check runs before the metadata transaction can commit, so
		// this typed failure has an unambiguous rollback. Other metadata errors
		// retain the durable reservation instead of risking deletion after an
		// ambiguous commit result.
		if errors.Is(err, repository.ErrStorageQuotaExceeded) {
			if cleanupErr := s.storage.RemoveRelative(r.Context(), prepared.RelativePath); cleanupErr != nil {
				s.logger.Error("failed to clean rejected storage blob", "error", cleanupErr)
			}
		}
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.StorageFile{"file": item})
}

func cleanupPrepared(store storage.BlobStore, prepared *storage.PreparedFile) {
	if store != nil {
		store.Cleanup(prepared)
	}
}

func readMultipartField(part *multipart.Part, max int64) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, max+1))
	if err != nil {
		return "", err
	}
	if int64(len(value)) > max {
		return "", &http.MaxBytesError{Limit: max}
	}
	return string(value), nil
}

func parseStoragePermissionsField(raw string) (*[]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		values := []string{}
		return &values, nil
	}
	var values []string
	if strings.HasPrefix(raw, "[") {
		decoder := json.NewDecoder(strings.NewReader(raw))
		if err := decoder.Decode(&values); err != nil {
			return nil, errors.New("permissions must be a JSON array or comma-separated list")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, errors.New("permissions must contain one JSON value")
		}
	} else {
		for _, value := range strings.Split(raw, ",") {
			if strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
	}
	return &values, nil
}

func isMaxBytesError(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}
