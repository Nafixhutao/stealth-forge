package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

var (
	functionRuntimePattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]{0,31}$`)
	functionVariablePattern         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,119}$`)
	functionExecutionTriggerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

var supportedFunctionRuntimes = map[string]struct{}{
	"node-22":     {},
	"python-3.13": {},
	"go-1.24":     {},
}

const (
	functionVariableMaxValueBytes       = 64 * 1024
	functionVariableMaxDescriptionBytes = 2000
	functionMaxDescriptionBytes         = 2000
)

// functionRequest is shared by create and patch. Pointer fields preserve the
// distinction between an omitted setting and a false/zero value on PATCH.
type functionRequest struct {
	Name               *string   `json:"name"`
	Runtime            *string   `json:"runtime"`
	Entrypoint         *string   `json:"entrypoint"`
	Commands           *string   `json:"commands"`
	TimeoutSeconds     *int      `json:"timeout_seconds"`
	Enabled            *bool     `json:"enabled"`
	Logging            *bool     `json:"logging"`
	ExecutePermissions *[]string `json:"execute_permissions"`
	Description        *string   `json:"description"`
	ArtifactQuotaBytes *int64    `json:"artifact_quota_bytes"`
}

type functionVariableRequest struct {
	Key         string  `json:"key"`
	Kind        string  `json:"kind"`
	IsSecret    *bool   `json:"is_secret"`
	Value       *string `json:"value"`
	Description *string `json:"description"`
}

type functionVariablePatchRequest struct {
	Key         *string `json:"key"`
	Value       *string `json:"value"`
	Description *string `json:"description"`
}

type functionExecutionRequest struct {
	Trigger string          `json:"trigger"`
	Input   json.RawMessage `json:"input"`
}

func functionActorFrom(r *http.Request) repository.FunctionActor {
	actor, ok := r.Context().Value(projectActorContextKey).(projectActor)
	if !ok {
		return repository.FunctionActor{}
	}
	if actor.kind == apiKeyProjectActor {
		return repository.FunctionActor{Kind: repository.FunctionAPIKeyActor, APIKeyID: actor.apiKeyID, APIKeyScopes: actor.scopes}
	}
	account, ok := r.Context().Value(accountContextKey).(domain.Account)
	if !ok {
		return repository.FunctionActor{}
	}
	return repository.FunctionActor{Kind: repository.FunctionConsoleActor, AccountID: mustUUID(account.ID)}
}

func parseFunctionCreateRequest(s *Server, req functionRequest) (repository.FunctionInput, error) {
	nameValue := ""
	if req.Name != nil {
		nameValue = *req.Name
	}
	name, err := validateFunctionName(nameValue)
	if err != nil {
		return repository.FunctionInput{}, err
	}
	runtimeValue := "node-22"
	if req.Runtime != nil {
		runtimeValue = *req.Runtime
	}
	runtime, err := validateFunctionRuntime(runtimeValue)
	if err != nil {
		return repository.FunctionInput{}, err
	}
	entrypointValue := "src/main.js"
	if req.Entrypoint != nil {
		entrypointValue = *req.Entrypoint
	}
	entrypoint, err := validateFunctionEntrypoint(entrypointValue)
	if err != nil {
		return repository.FunctionInput{}, err
	}
	commandsValue := ""
	if req.Commands != nil {
		commandsValue = *req.Commands
	}
	commands, err := validateFunctionCommands(commandsValue)
	if err != nil {
		return repository.FunctionInput{}, err
	}
	timeout := 15
	if req.TimeoutSeconds != nil {
		timeout = *req.TimeoutSeconds
	}
	if timeout < 1 || timeout > 900 {
		return repository.FunctionInput{}, errors.New("timeout_seconds must be between 1 and 900")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	status := "active"
	if !enabled {
		status = "disabled"
	}
	logging := true
	if req.Logging != nil {
		logging = *req.Logging
	}
	permissions := []string{}
	if req.ExecutePermissions != nil {
		permissions = append([]string(nil), (*req.ExecutePermissions)...)
	}
	quota := s.config.FunctionsDefaultQuotaBytes
	if req.ArtifactQuotaBytes != nil {
		quota = *req.ArtifactQuotaBytes
	}
	if quota <= 0 {
		return repository.FunctionInput{}, errors.New("artifact_quota_bytes must be positive")
	}
	if err := validateFunctionDescription(req.Description); err != nil {
		return repository.FunctionInput{}, err
	}
	return repository.FunctionInput{
		Name:               name,
		Runtime:            runtime,
		Entrypoint:         entrypoint,
		Commands:           commands,
		TimeoutSeconds:     timeout,
		Enabled:            enabled,
		Logging:            logging,
		ExecutePermissions: permissions,
		Description:        req.Description,
		Status:             status,
		ArtifactQuotaBytes: quota,
	}, nil
}

func parseFunctionPatchRequest(req functionRequest) (repository.FunctionPatch, error) {
	patch := repository.FunctionPatch{}
	changed := false
	if req.Name != nil {
		value, err := validateFunctionName(*req.Name)
		if err != nil {
			return patch, err
		}
		patch.Name = &value
		changed = true
	}
	if req.Runtime != nil {
		value, err := validateFunctionRuntime(*req.Runtime)
		if err != nil {
			return patch, err
		}
		patch.Runtime = &value
		changed = true
	}
	if req.Entrypoint != nil {
		value, err := validateFunctionEntrypoint(*req.Entrypoint)
		if err != nil {
			return patch, err
		}
		patch.Entrypoint = &value
		changed = true
	}
	if req.Commands != nil {
		value, err := validateFunctionCommands(*req.Commands)
		if err != nil {
			return patch, err
		}
		patch.Commands = &value
		changed = true
	}
	if req.TimeoutSeconds != nil {
		if *req.TimeoutSeconds < 1 || *req.TimeoutSeconds > 900 {
			return patch, errors.New("timeout_seconds must be between 1 and 900")
		}
		patch.TimeoutSeconds = req.TimeoutSeconds
		changed = true
	}
	if req.Enabled != nil {
		patch.Enabled = req.Enabled
		changed = true
	}
	if req.Logging != nil {
		patch.Logging = req.Logging
		changed = true
	}
	if req.ExecutePermissions != nil {
		value := append([]string(nil), (*req.ExecutePermissions)...)
		patch.ExecutePermissions = &value
		changed = true
	}
	if req.Description != nil {
		if err := validateFunctionDescription(req.Description); err != nil {
			return patch, err
		}
		patch.Description = req.Description
		changed = true
	}
	if req.ArtifactQuotaBytes != nil {
		if *req.ArtifactQuotaBytes <= 0 {
			return patch, errors.New("artifact_quota_bytes must be positive")
		}
		patch.ArtifactQuotaBytes = req.ArtifactQuotaBytes
		changed = true
	}
	if !changed {
		return patch, errors.New("at least one function setting is required")
	}
	return patch, nil
}

func validateFunctionName(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`).MatchString(value) {
		return "", errors.New("name must use lowercase letters, numbers, and hyphens and be 2 to 63 characters")
	}
	return value, nil
}

func validateFunctionRuntime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !functionRuntimePattern.MatchString(value) {
		return "", errors.New("runtime is invalid")
	}
	if _, ok := supportedFunctionRuntimes[value]; !ok {
		return "", errors.New("runtime must be one of node-22, python-3.13, or go-1.24")
	}
	return value, nil
}

func validateFunctionEntrypoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || value == "." || value == ".." || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\\x00\r\n") {
		return "", errors.New("entrypoint must be a non-empty safe path")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("entrypoint must be a non-empty safe path")
		}
	}
	return value, nil
}

func validateFunctionCommands(value string) (string, error) {
	if len(value) > 4000 || strings.ContainsRune(value, '\x00') {
		return "", errors.New("commands must be at most 4000 bytes and cannot contain NUL")
	}
	return value, nil
}

func validateFunctionDescription(value *string) error {
	if value == nil {
		return nil
	}
	if len(*value) > functionMaxDescriptionBytes || strings.ContainsRune(*value, '\x00') {
		return errors.New("description must be at most 2000 bytes and cannot contain NUL")
	}
	return nil
}

func (s *Server) listFunctions(w http.ResponseWriter, r *http.Request) {
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
	items, next, canManage, err := s.repo.ListFunctions(r.Context(), projectID, functionActorFrom(r), limit, cursorID)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"functions": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) getFunction(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	functionID, ok := pathUUID(w, r, "functionID")
	if !ok {
		return
	}
	item, err := s.repo.GetFunction(r.Context(), projectID, functionID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.Function{"function": item})
}

func (s *Server) createFunction(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req functionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	input, err := parseFunctionCreateRequest(s, req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.CreateFunction(r.Context(), uuid.Must(uuid.NewV7()), projectID, functionActorFrom(r), input)
	if planLimitError(w, err) {
		return
	}
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.Function{"function": item})
}

func (s *Server) updateFunction(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	functionID, ok := pathUUID(w, r, "functionID")
	if !ok {
		return
	}
	var req functionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	patch, err := parseFunctionPatchRequest(req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.UpdateFunction(r.Context(), projectID, functionID, functionActorFrom(r), patch)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.Function{"function": item})
}

func (s *Server) deleteFunction(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	functionID, ok := pathUUID(w, r, "functionID")
	if !ok {
		return
	}
	_, err := s.repo.DeleteFunction(r.Context(), projectID, functionID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
