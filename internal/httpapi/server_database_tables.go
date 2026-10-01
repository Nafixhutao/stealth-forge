package httpapi

import (
	"net/http"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listDatabaseTables(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	limit, cursor, ok := page(w, r)
	if !ok {
		return
	}
	var cursorID *uuid.UUID
	if cursor != "" {
		parsed, _ := uuid.Parse(cursor)
		cursorID = &parsed
	}
	items, next, canManage, err := s.repo.ListDatabaseTables(r.Context(), projectID, databaseID, databaseActorFrom(r), limit, cursorID)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) createDatabaseTable(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	var req databaseTableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := dbcore.ValidateName(req.Name)
	if err != nil {
		writeError(w, 422, "validation_error", err.Error())
		return
	}
	rowSecurity := true
	if req.RowSecurity != nil {
		rowSecurity = *req.RowSecurity
	}
	item, err := s.repo.CreateDatabaseTable(r.Context(), uuid.Must(uuid.NewV7()), projectID, databaseID, databaseActorFrom(r), repository.DatabaseTableInput{Name: name, RowSecurity: rowSecurity, CreatePermissions: req.CreatePermissions, ReadPermissions: req.ReadPermissions, UpdatePermissions: req.UpdatePermissions, DeletePermissions: req.DeletePermissions})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.DatabaseTable{"table": item})
}

func (s *Server) getDatabaseTable(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	tableID, ok := pathUUID(w, r, "tableID")
	if !ok {
		return
	}
	item, err := s.repo.GetDatabaseTable(r.Context(), projectID, databaseID, tableID, databaseActorFrom(r))
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]domain.DatabaseTable{"table": item})
}

func (s *Server) updateDatabaseTable(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	tableID, ok := pathUUID(w, r, "tableID")
	if !ok {
		return
	}
	var req databaseTableRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.RowSecurity == nil {
		writeError(w, 422, "validation_error", "row_security is required")
		return
	}
	item, err := s.repo.UpdateDatabaseTable(r.Context(), projectID, databaseID, tableID, databaseActorFrom(r), repository.DatabaseTableInput{RowSecurity: *req.RowSecurity, CreatePermissions: req.CreatePermissions, ReadPermissions: req.ReadPermissions, UpdatePermissions: req.UpdatePermissions, DeletePermissions: req.DeletePermissions})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]domain.DatabaseTable{"table": item})
}

func (s *Server) deleteDatabaseTable(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	databaseID, ok := pathUUID(w, r, "databaseID")
	if !ok {
		return
	}
	tableID, ok := pathUUID(w, r, "tableID")
	if !ok {
		return
	}
	if err := s.repo.DeleteDatabaseTable(r.Context(), projectID, databaseID, tableID, databaseActorFrom(r)); databaseResourceError(w, err) {
		return
	} else if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(204)
}
