package httpapi

import (
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listFunctionVariables(w http.ResponseWriter, r *http.Request) {
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
	items, next, canManage, err := s.repo.ListFunctionVariables(r.Context(), projectID, functionID, functionActorFrom(r), limit, cursorID)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"variables": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) getFunctionVariable(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, variableID, ok := functionVariablePathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetFunctionVariable(r.Context(), projectID, functionID, variableID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.FunctionVariable{"variable": item})
}

func (s *Server) createFunctionVariable(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return
	}
	var req functionVariableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !functionVariablePattern.MatchString(req.Key) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "key must start with a letter or underscore and contain only letters, numbers, and underscores")
		return
	}
	if req.Value == nil || len(*req.Value) == 0 || len(*req.Value) > functionVariableMaxValueBytes {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "value must contain between 1 and 65536 bytes")
		return
	}
	if req.Description != nil && (len(*req.Description) > functionVariableMaxDescriptionBytes || strings.ContainsRune(*req.Description, '\x00')) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "description must be at most 2000 bytes and cannot contain NUL")
		return
	}
	kind, secret, err := functionVariableKind(req.Kind, req.IsSecret)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if s.functionCipher == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "function secret encryption is not ready")
		return
	}
	item, err := s.repo.CreateFunctionVariable(r.Context(), uuid.Must(uuid.NewV7()), projectID, functionID, functionActorFrom(r), repository.FunctionVariableInput{Key: req.Key, Kind: kind, IsSecret: &secret, Value: req.Value, Description: req.Description, Cipher: s.functionCipher})
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.FunctionVariable{"variable": item})
}

func (s *Server) updateFunctionVariable(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, variableID, ok := functionVariablePathIDs(w, r)
	if !ok {
		return
	}
	var req functionVariablePatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	patch := repository.FunctionVariablePatch{Key: req.Key, Value: req.Value, Cipher: s.functionCipher}
	patch.SetValue = req.Value != nil
	if req.Description != nil {
		patch.SetDescription = true
	}
	if patch.Key != nil && !functionVariablePattern.MatchString(*patch.Key) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "key must start with a letter or underscore and contain only letters, numbers, and underscores")
		return
	}
	if patch.Value != nil && (len(*patch.Value) == 0 || len(*patch.Value) > functionVariableMaxValueBytes) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "value must contain between 1 and 65536 bytes")
		return
	}
	if patch.Description != nil && (len(*patch.Description) > functionVariableMaxDescriptionBytes || strings.ContainsRune(*patch.Description, '\x00')) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "description must be at most 2000 bytes and cannot contain NUL")
		return
	}
	if patch.SetValue || patch.ClearValue {
		if s.functionCipher == nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "function secret encryption is not ready")
			return
		}
	}
	if patch.Key == nil && !patch.SetValue && !patch.SetDescription {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one variable setting is required")
		return
	}
	item, err := s.repo.UpdateFunctionVariable(r.Context(), projectID, functionID, variableID, functionActorFrom(r), patch)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.FunctionVariable{"variable": item})
}

func (s *Server) deleteFunctionVariable(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, variableID, ok := functionVariablePathIDs(w, r)
	if !ok {
		return
	}
	err := s.repo.DeleteFunctionVariable(r.Context(), projectID, functionID, variableID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
