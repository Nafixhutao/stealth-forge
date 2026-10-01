package httpapi

import (
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listAdminIncidents(w http.ResponseWriter, r *http.Request) {
	limit, ok := adminConfigLimit(w, r)
	if !ok {
		return
	}
	items, err := s.repo.ListAdminIncidents(r.Context(), limit)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminIncidentsResponse{Items: items})
}

func (s *Server) createAdminIncident(w http.ResponseWriter, r *http.Request) {
	var request adminIncidentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		internalError(s, w, err)
		return
	}
	item, err := s.repo.CreateAdminIncident(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminIncidentInput{
		Title: request.Title, Severity: request.Severity, Status: request.Status, Services: request.Services, Message: request.Message,
	})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminIncidentResponse{Incident: item})
}

func (s *Server) getAdminIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "incidentID")
	if !ok {
		return
	}
	item, err := s.repo.AdminIncidentByID(r.Context(), id)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminIncidentResponse{Incident: item})
}

func (s *Server) updateAdminIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "incidentID")
	if !ok {
		return
	}
	var request adminIncidentPatchRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := s.repo.UpdateAdminIncident(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminIncidentPatch{
		Title: request.Title, Severity: request.Severity, Status: request.Status, Services: request.Services, Message: request.Message,
	})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminIncidentResponse{Incident: item})
}

func (s *Server) addAdminIncidentEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "incidentID")
	if !ok {
		return
	}
	var request adminIncidentEventRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		internalError(s, w, err)
		return
	}
	event, err := s.repo.AddAdminIncidentEvent(r.Context(), mustUUID(accountFrom(r).ID), id, eventID, repository.AdminIncidentEventInput{Kind: request.Kind, Message: request.Message})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, event)
}
