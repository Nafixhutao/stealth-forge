package httpapi

import (
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listDatabaseRows(w http.ResponseWriter, r *http.Request) {
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
	actor := databaseActorFrom(r)
	schema, err := s.repo.DatabaseTableSchema(r.Context(), projectID, databaseID, tableID, actor)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	query, err := parseRowQuery(r, schema)
	if err != nil {
		writeDatabaseQueryError(w, err)
		return
	}
	items, next, err := s.repo.ListDatabaseRows(r.Context(), projectID, databaseID, tableID, actor, query)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"rows": items, "pagination": paginationOf(query.Limit, next)})
}

func (s *Server) createDatabaseRow(w http.ResponseWriter, r *http.Request) {
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
	var req databaseRowRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	data, err := decodeRowObject(req.Data)
	if err != nil {
		writeError(w, 422, "validation_error", err.Error())
		return
	}
	item, err := s.repo.CreateDatabaseRow(r.Context(), uuid.Must(uuid.NewV7()), projectID, databaseID, tableID, databaseActorFrom(r), repository.DatabaseRowInput{Data: data, ReadPermissions: req.ReadPermissions, UpdatePermissions: req.UpdatePermissions, DeletePermissions: req.DeletePermissions})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 201, map[string]domain.DatabaseRow{"row": item})
}

func (s *Server) getDatabaseRow(w http.ResponseWriter, r *http.Request) {
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
	rowID, ok := pathUUID(w, r, "rowID")
	if !ok {
		return
	}
	item, err := s.repo.GetDatabaseRow(r.Context(), projectID, databaseID, tableID, rowID, databaseActorFrom(r))
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]domain.DatabaseRow{"row": item})
}

func (s *Server) updateDatabaseRow(w http.ResponseWriter, r *http.Request) {
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
	rowID, ok := pathUUID(w, r, "rowID")
	if !ok {
		return
	}
	var req databaseRowRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var data map[string]any
	if len(req.Data) > 0 {
		var err error
		data, err = decodeRowObject(req.Data)
		if err != nil {
			writeError(w, 422, "validation_error", err.Error())
			return
		}
	}
	item, err := s.repo.UpdateDatabaseRow(r.Context(), projectID, databaseID, tableID, rowID, databaseActorFrom(r), repository.DatabaseRowPatch{Data: data, ReadPermissions: req.ReadPermissions, UpdatePermissions: req.UpdatePermissions, DeletePermissions: req.DeletePermissions})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]domain.DatabaseRow{"row": item})
}

func (s *Server) deleteDatabaseRow(w http.ResponseWriter, r *http.Request) {
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
	rowID, ok := pathUUID(w, r, "rowID")
	if !ok {
		return
	}
	if err := s.repo.DeleteDatabaseRow(r.Context(), projectID, databaseID, tableID, rowID, databaseActorFrom(r)); databaseResourceError(w, err) {
		return
	} else if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(204)
}
