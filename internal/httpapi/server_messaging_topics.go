package httpapi

import (
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listMessagingTopics(w http.ResponseWriter, r *http.Request) {
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
	items, next, canManage, err := s.repo.ListMessagingTopics(
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
		map[string]any{"topics": items, "pagination": paginationOf(limit, next), "can_manage": canManage},
	)
}

func (s *Server) createMessagingTopic(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req messagingTopicRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := s.repo.CreateMessagingTopic(
		r.Context(),
		uuid.Must(uuid.NewV7()),
		projectID,
		messagingActorFrom(r),
		repository.MessagingTopicInput{Name: req.Name, Description: req.Description, Enabled: enabled},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.MessagingTopic{"topic": item})
}

func (s *Server) getMessagingTopic(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetMessagingTopic(r.Context(), projectID, topicID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingTopic{"topic": item})
}

func (s *Server) updateMessagingTopic(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
	if !ok {
		return
	}
	var req messagingTopicPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == nil && req.Description == nil && req.Enabled == nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "at least one topic field must be provided")
		return
	}
	item, err := s.repo.UpdateMessagingTopic(
		r.Context(),
		projectID,
		topicID,
		messagingActorFrom(r),
		repository.MessagingTopicPatch{Name: req.Name, Description: req.Description, Enabled: req.Enabled},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingTopic{"topic": item})
}

func (s *Server) deleteMessagingTopic(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
	if !ok {
		return
	}
	err := s.repo.DeleteMessagingTopic(r.Context(), projectID, topicID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
