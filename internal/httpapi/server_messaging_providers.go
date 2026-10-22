package httpapi

import (
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listMessagingProviders(w http.ResponseWriter, r *http.Request) {
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
	items, next, canManage, err := s.repo.ListMessagingProviders(
		r.Context(),
		projectID,
		messagingActorFrom(r),
		limit,
		cursorID,
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"providers": items, "pagination": paginationOf(limit, next), "can_manage": canManage},
	)
}

func (s *Server) createMessagingProvider(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req messagingProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := s.repo.CreateMessagingProvider(
		r.Context(),
		uuid.Must(uuid.NewV7()),
		projectID,
		messagingActorFrom(r),
		repository.MessagingProviderInput{
			Name:        req.Name,
			Channel:     req.Channel,
			Provider:    req.Provider,
			Credentials: req.Credentials,
			Enabled:     enabled,
		},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.MessagingProvider{"provider": item})
}

func (s *Server) getMessagingProvider(w http.ResponseWriter, r *http.Request) {
	projectID, providerID, ok := messagingProviderPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetMessagingProvider(r.Context(), projectID, providerID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingProvider{"provider": item})
}

func (s *Server) updateMessagingProvider(w http.ResponseWriter, r *http.Request) {
	projectID, providerID, ok := messagingProviderPathIDs(w, r)
	if !ok {
		return
	}
	var req messagingProviderPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	noFieldsProvided := req.Name == nil && req.Channel == nil && req.Provider == nil && req.Credentials == nil &&
		req.Enabled == nil
	if noFieldsProvided {
		writeError(
			w,
			http.StatusUnprocessableEntity,
			"validation_error",
			"at least one provider field must be provided",
		)
		return
	}
	item, err := s.repo.UpdateMessagingProvider(
		r.Context(),
		projectID,
		providerID,
		messagingActorFrom(r),
		repository.MessagingProviderPatch{
			Name:        req.Name,
			Channel:     req.Channel,
			Provider:    req.Provider,
			Credentials: req.Credentials,
			Enabled:     req.Enabled,
		},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingProvider{"provider": item})
}

func (s *Server) deleteMessagingProvider(w http.ResponseWriter, r *http.Request) {
	projectID, providerID, ok := messagingProviderPathIDs(w, r)
	if !ok {
		return
	}
	err := s.repo.DeleteMessagingProvider(r.Context(), projectID, providerID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
