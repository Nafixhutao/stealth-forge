package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/functionstore"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/google/uuid"
)

// authorizeFunctionDeploymentWrite verifies a caller may write to the target
// project/function before an upload stages or commits an artifact. API-key
// actors were already bound to projectID by requireProjectManagement, so only
// the write scope remains; console actors must resolve an owner/admin role and
// the function must exist in the project.
func (s *Server) authorizeFunctionDeploymentWrite(w http.ResponseWriter, r *http.Request, projectID, functionID uuid.UUID) bool {
	actor := functionActorFrom(r)
	if actor.Kind == repository.FunctionAPIKeyActor {
		if !apikey.HasScope(actor.APIKeyScopes, "functions.write") {
			writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to manage Functions")
			return false
		}
		return true
	}
	if _, err := s.repo.GetFunction(r.Context(), projectID, functionID, actor); err != nil {
		if functionResourceError(w, err) {
			return false
		}
		internalError(s, w, err)
		return false
	}
	return true
}

func functionPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	functionID, ok := pathUUID(w, r, "functionID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, functionID, true
}

func functionVariablePathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	variableID, ok := pathUUID(w, r, "variableID")
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, functionID, variableID, true
}

func functionDeploymentPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	deploymentID, ok := pathUUID(w, r, "deploymentID")
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, functionID, deploymentID, true
}

func functionExecutionPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	executionID, ok := pathUUID(w, r, "executionID")
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, functionID, executionID, true
}

func functionVariableKind(kind string, isSecret *bool) (string, bool, error) {
	return normalizeFunctionKind(kind, isSecret)
}

func normalizeFunctionKind(kind string, isSecret *bool) (string, bool, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		if isSecret != nil && *isSecret {
			kind = "secret"
		} else {
			kind = "variable"
		}
	}
	if kind != "variable" && kind != "secret" {
		return "", false, errors.New("kind must be variable or secret")
	}
	secret := kind == "secret"
	if isSecret != nil && *isSecret != secret {
		return "", false, errors.New("kind and secret must agree")
	}
	return kind, secret, nil
}

func functionResourceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRowHidden):
		writeError(w, http.StatusNotFound, "not_found", "function resource was not found")
		return true
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to access this function resource")
		return true
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "function resource conflicts with an existing resource")
		return true
	case errors.Is(err, repository.ErrFunctionQuotaExceeded):
		writeError(w, http.StatusRequestEntityTooLarge, "function_quota_exceeded", "function artifact quota would be exceeded")
		return true
	case errors.Is(err, repository.ErrFunctionArtifactTooLarge), errors.Is(err, functionstore.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "function source artifact exceeds the configured maximum size")
		return true
	case errors.Is(err, repository.ErrFunctionSecretUnavailable):
		writeError(w, http.StatusServiceUnavailable, "not_ready", "function secret encryption is not ready")
		return true
	case errors.Is(err, repository.ErrDeploymentActive), errors.Is(err, repository.ErrInvalidFunctionTransition), errors.Is(err, repository.ErrExecutionNotAvailable), errors.Is(err, repository.ErrFunctionDisabled):
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return true
	case errors.Is(err, repository.ErrInvalidFunctionVariable), errors.Is(err, repository.ErrInvalidFunctionSettings), errors.Is(err, database.ErrInvalidPermissions), errors.Is(err, database.ErrDuplicatePermission), errors.Is(err, storage.ErrInvalidFilename), errors.Is(err, functionstore.ErrInvalidPath):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return true
	}
	return false
}
