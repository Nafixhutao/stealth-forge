package httpapi

import (
	"errors"
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

// Messaging handlers are split by resource: providers, topics, subscribers, and
// messages/deliveries. This file owns the actor projection, request shapes,
// path helpers, and the shared error mapping every messaging handler uses.

func messagingActorFrom(r *http.Request) repository.MessagingActor {
	actor := projectActorFrom(r)
	if actor.kind == apiKeyProjectActor {
		return repository.MessagingActor{
			Kind:         repository.MessagingAPIKeyActor,
			APIKeyID:     actor.apiKeyID,
			APIKeyScopes: actor.scopes,
		}
	}
	account, ok := r.Context().Value(accountContextKey).(domain.Account)
	if !ok {
		return repository.MessagingActor{}
	}
	return repository.MessagingActor{Kind: repository.MessagingConsoleActor, AccountID: mustUUID(account.ID)}
}

type messagingProviderRequest struct {
	Name        string            `json:"name"`
	Channel     string            `json:"channel"`
	Provider    string            `json:"provider"`
	Credentials map[string]string `json:"credentials"`
	Enabled     *bool             `json:"enabled"`
}

type messagingProviderPatchRequest struct {
	Name        *string            `json:"name"`
	Channel     *string            `json:"channel"`
	Provider    *string            `json:"provider"`
	Credentials *map[string]string `json:"credentials"`
	Enabled     *bool              `json:"enabled"`
}

type messagingTopicRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

type messagingTopicPatchRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

type messagingSubscriberRequest struct {
	Channel string `json:"channel"`
	Address string `json:"address"`
	Enabled *bool  `json:"enabled"`
}

type messagingMessageRequest struct {
	TopicID string            `json:"topic_id"`
	Channel string            `json:"channel"`
	Subject string            `json:"subject"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data"`
}

func messagingProviderPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	providerID, ok := pathUUID(w, r, "providerID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, providerID, true
}

func messagingTopicPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	topicID, ok := pathUUID(w, r, "topicID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, topicID, true
}

func messagingSubscriberPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, topicID, ok := messagingTopicPathIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	subscriberID, ok := pathUUID(w, r, "subscriberID")
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, topicID, subscriberID, true
}

func messagingMessagePathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	messageID, ok := pathUUID(w, r, "messageID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, messageID, true
}

func messagingResourceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "project or messaging resource was not found")
		return true
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to manage project messaging")
		return true
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "a messaging resource with this identity already exists")
		return true
	case errors.Is(err, repository.ErrInvalidMessaging), errors.Is(err, repository.ErrMessagingAddressInvalid):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "messaging settings are invalid")
		return true
	case errors.Is(err, repository.ErrMessagingNotReady):
		writeError(w, http.StatusServiceUnavailable, "not_ready", "messaging encryption is not configured")
		return true
	case errors.Is(err, repository.ErrMessagingProviderUnavailable):
		writeError(
			w,
			http.StatusConflict,
			"provider_unavailable",
			"no enabled provider is configured for this message channel",
		)
		return true
	case errors.Is(err, repository.ErrMessagingNoRecipients):
		writeError(
			w,
			http.StatusUnprocessableEntity,
			"no_recipients",
			"the topic has no active subscribers for this channel",
		)
		return true
	case errors.Is(err, repository.ErrMessagingTooManyRecipients):
		writeError(
			w,
			http.StatusUnprocessableEntity,
			"too_many_recipients",
			"the topic exceeds the per-message recipient limit",
		)
		return true
	case errors.Is(err, repository.ErrMessagingMessageTerminal):
		writeError(
			w,
			http.StatusConflict,
			"message_terminal",
			"the message is already terminal and cannot be cancelled",
		)
		return true
	case errors.Is(err, repository.ErrNoMessagingDelivery), errors.Is(err, repository.ErrMessagingDeliveryNotFound):
		writeError(w, http.StatusNotFound, "not_found", "messaging delivery was not found")
		return true
	default:
		return false
	}
}
