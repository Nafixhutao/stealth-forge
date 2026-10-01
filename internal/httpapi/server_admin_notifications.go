package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listAdminNotificationChannels(w http.ResponseWriter, r *http.Request) {
	limit, ok := adminConfigLimit(w, r)
	if !ok {
		return
	}
	items, err := s.repo.ListAdminNotificationChannels(r.Context(), limit)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminNotificationChannelsResponse{Items: items})
}

func (s *Server) createAdminNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var request adminNotificationChannelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	config, err := json.Marshal(request.Config)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminNotification)
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
	item, err := s.repo.CreateAdminNotificationChannel(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminNotificationChannelInput{Name: request.Name, Kind: request.Kind, Enabled: enabled, Config: config})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) getAdminNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "channelID")
	if !ok {
		return
	}
	item, err := s.repo.AdminNotificationChannelByID(r.Context(), id)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateAdminNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "channelID")
	if !ok {
		return
	}
	var request adminNotificationChannelRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	config, err := json.Marshal(request.Config)
	if err != nil {
		adminControlError(s, w, repository.ErrInvalidAdminNotification)
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	item, err := s.repo.UpdateAdminNotificationChannel(r.Context(), mustUUID(accountFrom(r).ID), id, repository.AdminNotificationChannelInput{Name: request.Name, Kind: request.Kind, Enabled: enabled, Config: config})
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteAdminNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "channelID")
	if !ok {
		return
	}
	if err := s.repo.DeleteAdminNotificationChannel(r.Context(), mustUUID(accountFrom(r).ID), id); err != nil {
		adminControlError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testAdminNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "channelID")
	if !ok || s.repo == nil {
		return
	}
	deliveryID, err := s.repo.EnqueueAdminNotificationTest(r.Context(), mustUUID(accountFrom(r).ID), id)
	if err != nil {
		adminControlError(s, w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, adminNotificationTestResponse{DeliveryID: deliveryID.String(), Status: "pending"})
}
