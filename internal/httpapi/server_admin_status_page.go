package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/repository"
)

func (s *Server) getAdminStatusPage(w http.ResponseWriter, r *http.Request) {
	item, err := s.repo.AdminStatusPage(r.Context())
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateAdminStatusPage(w http.ResponseWriter, r *http.Request) {
	var request adminStatusPageRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	components, err := json.Marshal(request.Components)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminStatus)
		return
	}
	item, err := s.repo.UpdateAdminStatusPage(r.Context(), mustUUID(accountFrom(r).ID), repository.AdminStatusPageInput{
		Name: request.Name, Description: request.Description, IsPublic: request.IsPublic, Components: components, PublishedIncidents: request.PublishedIncidents,
	})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) publicAdminStatusPage(w http.ResponseWriter, r *http.Request) {
	item, err := s.repo.PublicAdminStatusPage(r.Context())
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "status page is not published")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=60")
	writeJSON(w, http.StatusOK, item)
}
