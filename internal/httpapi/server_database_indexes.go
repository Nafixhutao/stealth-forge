package httpapi

import (
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listDatabaseIndexes(w http.ResponseWriter, r *http.Request) {
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
	limit, cursor, ok := page(w, r)
	if !ok {
		return
	}
	var cursorID *uuid.UUID
	if cursor != "" {
		parsed, _ := uuid.Parse(cursor)
		cursorID = &parsed
	}
	items, next, err := s.repo.ListDatabaseIndexes(r.Context(), projectID, databaseID, tableID, databaseActorFrom(r), limit, cursorID)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"indexes": items, "pagination": paginationOf(limit, next)})
}

func (s *Server) createDatabaseIndex(w http.ResponseWriter, r *http.Request) {
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
	var req databaseIndexRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item, err := s.repo.CreateDatabaseIndex(r.Context(), uuid.Must(uuid.NewV7()), projectID, databaseID, tableID, databaseActorFrom(r), repository.DatabaseIndexInput{Name: req.Name, Type: req.Type, ColumnKeys: req.ColumnKeys, Directions: req.Directions})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 201, map[string]domain.DatabaseIndex{"index": item})
}

func (s *Server) deleteDatabaseIndex(w http.ResponseWriter, r *http.Request) {
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
	indexID, ok := pathUUID(w, r, "indexID")
	if !ok {
		return
	}
	if err := s.repo.DeleteDatabaseIndex(r.Context(), projectID, databaseID, tableID, indexID, databaseActorFrom(r)); databaseResourceError(w, err) {
		return
	} else if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(204)
}
