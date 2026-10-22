package httpapi

import (
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listMessagingSubscribers(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
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
	items, next, canManage, err := s.repo.ListMessagingSubscribers(
		r.Context(),
		projectID,
		topicID,
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
		map[string]any{"subscribers": items, "pagination": paginationOf(limit, next), "can_manage": canManage},
	)
}

func (s *Server) createMessagingSubscriber(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
	if !ok {
		return
	}
	var req messagingSubscriberRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item, err := s.repo.CreateMessagingSubscriber(
		r.Context(),
		uuid.Must(uuid.NewV7()),
		projectID,
		topicID,
		messagingActorFrom(r),
		repository.MessagingSubscriberInput{Channel: req.Channel, Address: req.Address, Enabled: enabled},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.MessagingSubscriber{"subscriber": item})
}

func (s *Server) getMessagingSubscriber(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, subscriberID, ok := messagingSubscriberPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetMessagingSubscriber(r.Context(), projectID, topicID, subscriberID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingSubscriber{"subscriber": item})
}

func (s *Server) deleteMessagingSubscriber(w http.ResponseWriter, r *http.Request) {
	projectID, topicID, subscriberID, ok := messagingSubscriberPathIDs(w, r)
	if !ok {
		return
	}
	err := s.repo.DeleteMessagingSubscriber(r.Context(), projectID, topicID, subscriberID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
