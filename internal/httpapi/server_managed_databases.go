package httpapi

import (
	"errors"
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/dbsecret"
	"github.com/Nafixhutao/stealth-forge/internal/manageddb"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) registerManagedDatabaseRoutes(r chi.Router) {
	r.With(s.requireProjectManagement).Get("/projects/{projectID}/managed-databases", s.listManagedDatabases)
	r.With(s.requireProjectManagement).Post("/projects/{projectID}/managed-databases", s.createManagedDatabase)
	r.With(s.requireProjectManagement).
		Delete("/projects/{projectID}/managed-databases/{databaseID}", s.deleteManagedDatabase)
	r.With(s.requireProjectManagement).
		Post("/projects/{projectID}/managed-databases/{databaseID}/password", s.revealManagedDatabasePassword)
	r.With(s.requireProjectManagement).
		Post("/projects/{projectID}/managed-databases/{databaseID}/password/rotate", s.rotateManagedDatabasePassword)
	r.With(s.requireProjectManagement).
		Get("/projects/{projectID}/managed-databases/{databaseID}/backups", s.listManagedDatabaseBackups)
	r.With(s.requireProjectManagement).
		Post("/projects/{projectID}/managed-databases/{databaseID}/backups/{backupID}/restore", s.restoreManagedDatabase)
}

// managedDatabaseActor keeps the hosted-database surface Console-only. A
// project API key is intentionally refused: revealing a database password is a
// human action, not a server-to-server capability.
func (s *Server) managedDatabaseActor(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	if s.managedDatabases == nil {
		if s.config.ManagedDatabasesEnabled {
			writeError(w, http.StatusServiceUnavailable, "feature_unavailable", "managed database service is temporarily unavailable")
		} else {
			writeError(w, http.StatusServiceUnavailable, "feature_disabled", "managed databases are not enabled on this installation")
		}
		return uuid.Nil, uuid.Nil, false
	}
	if projectActorFrom(r).kind != consoleProjectActor {
		writeError(w, http.StatusForbidden, "forbidden", "managed databases require a Console session")
		return uuid.Nil, uuid.Nil, false
	}
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	actorID, err := uuid.Parse(accountFrom(r).ID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, actorID, true
}

func (s *Server) listManagedDatabases(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	items, err := s.managedDatabases.List(r.Context(), projectID)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"databases":  items,
		"can_manage": true,
	})
}

type createManagedDatabaseRequest struct {
	Name string `json:"name"`
}

func (s *Server) createManagedDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, actorID, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	var req createManagedDatabaseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.managedDatabases.Create(r.Context(), projectID, actorID, req.Name)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"database": item})
}

func (s *Server) deleteManagedDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, actorID, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	if err := s.managedDatabases.Delete(r.Context(), projectID, actorID, databaseID); err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revealManagedDatabasePassword(w http.ResponseWriter, r *http.Request) {
	projectID, actorID, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	password, err := s.managedDatabases.RevealPassword(r.Context(), projectID, actorID, databaseID)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": password})
}

func (s *Server) rotateManagedDatabasePassword(w http.ResponseWriter, r *http.Request) {
	projectID, actorID, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	password, err := s.managedDatabases.RotatePassword(r.Context(), projectID, actorID, databaseID)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": password})
}

func (s *Server) listManagedDatabaseBackups(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	backups, restore, hasRestore, err := s.managedDatabases.ListBackups(r.Context(), projectID, databaseID)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	response := map[string]any{"backups": backups}
	if hasRestore {
		response["latest_restore"] = restore
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) restoreManagedDatabase(w http.ResponseWriter, r *http.Request) {
	projectID, actorID, ok := s.managedDatabaseActor(w, r)
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	backupID, ok := pathUUID(w, r, "backupID")
	if !ok {
		return
	}
	restore, err := s.managedDatabases.RequestRestore(r.Context(), projectID, actorID, databaseID, backupID)
	if err != nil {
		s.managedDatabaseError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"restore": restore})
}

func (s *Server) managedDatabaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "database was not found")
	case errors.Is(err, repository.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "this project already has a free managed database")
	case errors.Is(err, manageddb.ErrInvalidName):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
	case errors.Is(err, dbsecret.ErrInvalidCiphertext):
		internalError(s, w, err)
	default:
		internalError(s, w, err)
	}
}
