package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listAdminDashboards(w http.ResponseWriter, r *http.Request) {
	limit, ok := adminConfigLimit(w, r)
	if !ok {
		return
	}
	items, err := s.repo.ListAdminDashboards(r.Context(), limit)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminDashboardsResponse{Items: items})
}

func (s *Server) createAdminDashboard(w http.ResponseWriter, r *http.Request) {
	var request adminDashboardRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		internalError(s, w, err)
		return
	}
	definition, err := json.Marshal(request.Definition)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminDashboard)
		return
	}
	item, err := s.repo.CreateAdminDashboard(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminDashboardInput{Name: request.Name, Description: request.Description, Definition: definition})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminDashboardResponse{Dashboard: item})
}

func (s *Server) getAdminDashboard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dashboardID")
	if !ok {
		return
	}
	item, err := s.repo.AdminDashboardByID(r.Context(), id)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminDashboardResponse{Dashboard: item})
}

func (s *Server) updateAdminDashboard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dashboardID")
	if !ok {
		return
	}
	var request adminDashboardRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	definition, err := json.Marshal(request.Definition)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminDashboard)
		return
	}
	item, err := s.repo.UpdateAdminDashboard(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminDashboardInput{Name: request.Name, Description: request.Description, Definition: definition})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminDashboardResponse{Dashboard: item})
}

func (s *Server) deleteAdminDashboard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dashboardID")
	if !ok {
		return
	}
	if err := s.repo.DeleteAdminDashboard(r.Context(), mustUUID(accountFrom(r).ID), id); err != nil {
		adminControlError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
