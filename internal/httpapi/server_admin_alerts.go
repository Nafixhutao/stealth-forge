package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listAdminAlertRules(w http.ResponseWriter, r *http.Request) {
	limit, ok := adminConfigLimit(w, r)
	if !ok {
		return
	}
	items, err := s.repo.ListAdminAlertRules(r.Context(), limit)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminAlertRulesResponse{Items: items})
}

func (s *Server) listAdminAlertEvents(w http.ResponseWriter, r *http.Request) {
	query, ok := adminAlertEventQuery(w, r, nil)
	if !ok {
		return
	}
	page, err := s.repo.QueryAdminAlertEvents(r.Context(), query)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeAdminAlertEventPage(w, page)
}

func (s *Server) createAdminAlertRule(w http.ResponseWriter, r *http.Request) {
	var request adminAlertRuleRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	condition, err := json.Marshal(request.Condition)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminAlert)
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	id, err := uuid.NewV7()
	if err != nil {
		internalError(s, w, err)
		return
	}
	item, err := s.repo.CreateAdminAlertRule(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminAlertRuleInput{
		Name: request.Name, Kind: request.Kind, Condition: condition, Severity: request.Severity,
		ForSeconds: request.ForSeconds, Enabled: enabled,
	})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminAlertRuleResponse{Rule: item})
}

func (s *Server) getAdminAlertRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "alertRuleID")
	if !ok {
		return
	}
	item, err := s.repo.AdminAlertRuleByID(r.Context(), id)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	events, err := s.repo.ListAdminAlertEvents(r.Context(), id, 50)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminAlertRuleResponse{Rule: item, Events: events})
}

func (s *Server) listAdminAlertRuleEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "alertRuleID")
	if !ok {
		return
	}
	query, ok := adminAlertEventQuery(w, r, &id)
	if !ok {
		return
	}
	page, err := s.repo.QueryAdminAlertEvents(r.Context(), query)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeAdminAlertEventPage(w, page)
}

func adminAlertEventQuery(w http.ResponseWriter, r *http.Request, ruleID *uuid.UUID) (repository.AdminAlertEventQuery, bool) {
	limit, ok := adminConfigLimit(w, r)
	if !ok {
		return repository.AdminAlertEventQuery{}, false
	}
	from, ok := adminAlertEventTime(w, r, "from")
	if !ok {
		return repository.AdminAlertEventQuery{}, false
	}
	to, ok := adminAlertEventTime(w, r, "to")
	if !ok {
		return repository.AdminAlertEventQuery{}, false
	}
	if from != nil && to != nil && !to.After(*from) {
		writeError(w, http.StatusBadRequest, "validation_error", "from must be before to")
		return repository.AdminAlertEventQuery{}, false
	}
	var cursor *repository.AdminAlertEventCursor
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		parsed, err := repository.DecodeAdminAlertEventCursor(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", "cursor is invalid")
			return repository.AdminAlertEventQuery{}, false
		}
		cursor = &parsed
	}
	return repository.AdminAlertEventQuery{RuleID: ruleID, From: from, To: to, Cursor: cursor, Limit: limit}, true
}

func adminAlertEventTime(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", key+" must be an RFC3339 timestamp")
		return nil, false
	}
	parsed = parsed.UTC()
	return &parsed, true
}

func writeAdminAlertEventPage(w http.ResponseWriter, page repository.AdminAlertEventPage) {
	response := adminAlertEventsResponse{Items: page.Items}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) updateAdminAlertRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "alertRuleID")
	if !ok {
		return
	}
	var request adminAlertRuleRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	condition, err := json.Marshal(request.Condition)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminAlert)
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	item, err := s.repo.UpdateAdminAlertRule(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminAlertRulePatch{
		Name: &request.Name, Kind: &request.Kind, Condition: condition, Severity: &request.Severity,
		ForSeconds: &request.ForSeconds, Enabled: &enabled,
	})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminAlertRuleResponse{Rule: item})
}

func (s *Server) deleteAdminAlertRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "alertRuleID")
	if !ok {
		return
	}
	if err := s.repo.DeleteAdminAlertRule(r.Context(), mustUUID(accountFrom(r).ID), id); err != nil {
		adminControlError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
