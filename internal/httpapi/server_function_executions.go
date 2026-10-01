package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
)

func (s *Server) listFunctionExecutions(w http.ResponseWriter, r *http.Request) {
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
	items, next, err := s.repo.ListFunctionExecutions(r.Context(), projectID, functionID, functionActorFrom(r), limit, cursorID)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"executions": items, "pagination": paginationOf(limit, next)})
}

// createFunctionExecution accepts a bounded JSON payload and only enqueues
// work. The worker is the sole component allowed to read source artifacts or
// start a runtime container; this handler never executes user code inline.
func (s *Server) createFunctionExecution(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, ok := functionPathIDs(w, r)
	if !ok {
		return
	}
	var req functionExecutionRequest
	if r.ContentLength == 0 {
		req.Input = json.RawMessage(`{}`)
	} else if !decodeJSON(w, r, &req) {
		return
	}
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = "manual"
	}
	if !functionExecutionTriggerPattern.MatchString(trigger) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "trigger must start with a letter or number and contain only letters, numbers, dots, underscores, or hyphens")
		return
	}
	input := req.Input
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if len(input) > 65536 {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "input must be valid JSON no larger than 65536 bytes")
		return
	}
	if !json.Valid(input) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "input must be valid JSON")
		return
	}
	var item domain.FunctionExecution
	var err error
	if _, management := r.Context().Value(projectActorContextKey).(projectActor); management {
		item, err = s.repo.CreateFunctionExecutionForActor(r.Context(), uuid.Must(uuid.NewV7()), projectID, functionID, functionActorFrom(r), trigger, input)
	} else {
		var projectUserID *uuid.UUID
		if user, ok := r.Context().Value(projectUserContextKey).(domain.ApplicationUser); ok {
			parsed := mustUUID(user.ID)
			projectUserID = &parsed
		}
		item, err = s.repo.CreateFunctionExecutionForApplication(r.Context(), uuid.Must(uuid.NewV7()), projectID, functionID, projectUserID, trigger, input)
	}
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]domain.FunctionExecution{"execution": item})
}

func (s *Server) getFunctionExecution(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, executionID, ok := functionExecutionPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetFunctionExecution(r.Context(), projectID, functionID, executionID, functionActorFrom(r))
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.FunctionExecution{"execution": item})
}

func (s *Server) listFunctionExecutionLogs(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, executionID, ok := functionExecutionPathIDs(w, r)
	if !ok {
		return
	}
	limit, _, ok := page(w, r)
	if !ok {
		return
	}
	after := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "after must be a non-negative integer")
			return
		}
		after = parsed
	}
	items, err := s.repo.ListFunctionExecutionLogs(r.Context(), projectID, functionID, executionID, functionActorFrom(r), limit, after)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	next := ""
	if len(items) == limit {
		next = strconv.FormatInt(items[len(items)-1].Sequence, 10)
	}
	var nextCursor *string
	if next != "" {
		nextCursor = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items, "pagination": pagination{Limit: limit, NextCursor: nextCursor}})
}

func (s *Server) listFunctionBuildLogs(w http.ResponseWriter, r *http.Request) {
	projectID, functionID, deploymentID, ok := functionDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	limit, _, ok := page(w, r)
	if !ok {
		return
	}
	after := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "after must be a non-negative integer")
			return
		}
		after = parsed
	}
	items, err := s.repo.ListFunctionBuildLogs(r.Context(), projectID, functionID, deploymentID, functionActorFrom(r), limit, after)
	if functionResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	next := ""
	if len(items) == limit {
		next = strconv.FormatInt(items[len(items)-1].Sequence, 10)
	}
	var nextCursor *string
	if next != "" {
		nextCursor = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items, "pagination": pagination{Limit: limit, NextCursor: nextCursor}})
}
