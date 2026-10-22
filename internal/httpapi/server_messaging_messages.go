package httpapi

import (
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listMessagingMessages(w http.ResponseWriter, r *http.Request) {
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
	items, next, canManage, err := s.repo.ListMessagingMessages(
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
		map[string]any{"messages": items, "pagination": paginationOf(limit, next), "can_manage": canManage},
	)
}

func (s *Server) createMessagingMessage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req messagingMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	topicID, err := repository.ParseUUID(req.TopicID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "topic_id must be a UUID")
		return
	}
	result, err := s.repo.CreateMessagingMessage(
		r.Context(),
		uuid.Must(uuid.NewV7()),
		projectID,
		messagingActorFrom(r),
		repository.MessagingMessageInput{
			TopicID:        topicID,
			Channel:        req.Channel,
			Subject:        req.Subject,
			Body:           req.Body,
			Data:           req.Data,
			IdempotencyKey: r.Header.Get("Idempotency-Key"),
		},
	)
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, map[string]domain.MessagingMessage{"message": result.Message})
}

func (s *Server) getMessagingMessage(w http.ResponseWriter, r *http.Request) {
	projectID, messageID, ok := messagingMessagePathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetMessagingMessage(r.Context(), projectID, messageID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingMessage{"message": item})
}

func (s *Server) cancelMessagingMessage(w http.ResponseWriter, r *http.Request) {
	projectID, messageID, ok := messagingMessagePathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.CancelMessagingMessage(r.Context(), projectID, messageID, messagingActorFrom(r))
	if messagingResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.MessagingMessage{"message": item})
}

func (s *Server) listMessagingDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID, messageID, ok := messagingMessagePathIDs(w, r)
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
	items, next, err := s.repo.ListMessagingDeliveries(
		r.Context(),
		projectID,
		messageID,
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
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": items, "pagination": paginationOf(limit, next)})
}
