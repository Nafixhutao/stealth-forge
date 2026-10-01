package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

func (s *Server) listDatabaseColumns(w http.ResponseWriter, r *http.Request) {
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
	items, next, err := s.repo.ListDatabaseColumns(r.Context(), projectID, databaseID, tableID, databaseActorFrom(r), limit, cursorID)
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"columns": items, "pagination": paginationOf(limit, next)})
}

func (s *Server) createDatabaseColumn(w http.ResponseWriter, r *http.Request) {
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
	var req databaseColumnRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Default) == 0 {
		req.Default = nil
	}
	var defaultValue any
	hasDefault := len(req.Default) > 0
	if hasDefault {
		decoder := json.NewDecoder(bytes.NewReader(req.Default))
		decoder.UseNumber()
		if err := decoder.Decode(&defaultValue); err != nil {
			writeError(w, 422, "validation_error", "default must be valid JSON")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeError(w, 422, "validation_error", "default must contain one JSON value")
			return
		}
	}
	item, err := s.repo.CreateDatabaseColumn(r.Context(), uuid.Must(uuid.NewV7()), projectID, databaseID, tableID, databaseActorFrom(r), repository.DatabaseColumnInput{Key: req.Key, Type: dbcore.ColumnType(req.Type), Required: req.Required, VarcharSize: req.VarcharSize, Default: defaultValue, HasDefault: hasDefault})
	if databaseResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, 201, map[string]domain.DatabaseColumn{"column": item})
}

func (s *Server) deleteDatabaseColumn(w http.ResponseWriter, r *http.Request) {
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
	columnID, ok := pathUUID(w, r, "columnID")
	if !ok {
		return
	}
	if err := s.repo.DeleteDatabaseColumn(r.Context(), projectID, databaseID, tableID, columnID, databaseActorFrom(r)); databaseResourceError(w, err) {
		return
	} else if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(204)
}
