package httpapi

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/functionstore"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/google/uuid"
)

func (s *Server) listFunctionDeployments(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, ok := functionPathIDs(w, r)
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
	items, next, canManage, err := s.repo.ListFunctionDeployments(r.Context(), projectID, functionID, functionActorFrom(r), limit, cursorID)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) getFunctionDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, deploymentID, ok := functionDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetFunctionDeployment(r.Context(), projectID, functionID, deploymentID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.FunctionDeployment{"deployment": item})
}

func (s *Server) uploadFunctionDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return
	}
	if s.functions == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "function artifact storage is not ready")
		return
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be multipart/form-data")
		return
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	deploymentID := uuid.Must(uuid.NewV7())
	var prepared functionstore.PreparedArtifact
	var sourceName string
	var activate bool
	haveSource, haveActivate := false, false
	cleanup := func() { s.functions.Cleanup(&prepared) }
	defer cleanup()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			if isMaxBytesError(nextErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the configured upload limit")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart upload")
			}
			return
		}
		field := part.FormName()
		switch field {
		case "source":
			if haveSource {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "source may only be provided once")
				return
			}
			haveSource = true
			sourceName = part.FileName()
			if sourceName != "" {
				if err := storage.ValidateFilename(sourceName); err != nil {
					_ = part.Close()
					writeError(w, http.StatusUnprocessableEntity, "validation_error", "source filename is invalid")
					return
				}
			}
			prepared, err = s.functions.BeginUploadWithLimit(r.Context(), projectID, functionID, deploymentID, part, s.config.FunctionsMaxArtifactSize)
			_ = part.Close()
			if errors.Is(err, functionstore.ErrTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "source artifact exceeds the configured maximum size")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
		case "source_name":
			value, readErr := readFunctionMultipartField(part, 512)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "source_name is invalid")
				return
			}
			sourceName = value
			if err := storage.ValidateFilename(sourceName); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "source_name is invalid")
				return
			}
		case "activate":
			if haveActivate {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate may only be provided once")
				return
			}
			haveActivate = true
			value, readErr := readFunctionMultipartField(part, 16)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate must be true or false")
				return
			}
			activate, err = strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate must be true or false")
				return
			}
		default:
			_ = part.Close()
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "unsupported deployment field")
			return
		}
	}
	if !haveSource || prepared.TempPath == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "source is required")
		return
	}
	if sourceName == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "source filename is required")
		return
	}
	if err := storage.ValidateFilename(sourceName); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "source filename is invalid")
		return
	}
	publishCleanup := repository.ArtifactCleanupInput{
		ProjectID:    projectID,
		StoreKind:    repository.ArtifactCleanupFunctions,
		Operation:    repository.ArtifactCleanupRelative,
		RelativePath: prepared.RelativePath,
	}
	if err := s.repo.ReserveArtifactPublishCleanup(r.Context(), publishCleanup); err != nil {
		cleanup()
		internalError(s, w, err)
		return
	}
	if err := s.functions.Commit(r.Context(), &prepared); err != nil {
		internalError(s, w, err)
		return
	}
	name := sourceName
	actor := functionActorFrom(r)
	var createdBy *uuid.UUID
	if actor.Kind == repository.FunctionConsoleActor && actor.AccountID != uuid.Nil {
		createdBy = &actor.AccountID
	}
	item, err := s.repo.CreateFunctionDeployment(r.Context(), deploymentID, projectID, functionID, actor, repository.FunctionDeploymentInput{Source: "upload", SourceName: &name, SizeBytes: prepared.Size, ChecksumSHA256: prepared.Checksum, SourcePath: prepared.RelativePath, CreatedByAccountID: createdBy, Activate: activate, PublishCleanup: &publishCleanup})
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.FunctionDeployment{"deployment": item})
}

func readFunctionMultipartField(part *multipart.Part, max int64) (string, error) {
	if max <= 0 {
		return "", errors.New("invalid multipart field limit")
	}
	data, err := io.ReadAll(io.LimitReader(part, max+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > max {
		return "", errors.New("multipart field is too large")
	}
	return string(data), nil
}

func (s *Server) deleteFunctionDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, deploymentID, ok := functionDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	_, err := s.repo.DeleteFunctionDeploymentWithArtifacts(r.Context(), projectID, functionID, deploymentID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) activateFunctionDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, deploymentID, ok := functionDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	actor := functionActorFrom(r)
	item, function, err := s.repo.ActivateFunctionDeployment(r.Context(), projectID, functionID, deploymentID, actor)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"function": function, "deployment": item})
}
