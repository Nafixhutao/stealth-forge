package httpapi

import (
	"errors"
	"mime"
	"net/http"
	"os"
)

func (s *Server) listAgentArtifacts(w http.ResponseWriter, r *http.Request) {
	agentID, ok := pathUUID(w, r, "agentID")
	if !ok {
		return
	}
	limit, cursor, ok := page(w, r)
	if !ok {
		return
	}
	items, next, err := s.repo.ListAgentArtifacts(
		r.Context(), mustUUID(accountFrom(r).ID), agentID, limit, cursor,
	)
	if err != nil {
		if agentRunResourceError(w, err) {
			return
		}
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"artifacts":  items,
		"pagination": paginationOf(limit, next),
	})
}

func (s *Server) downloadAgentArtifact(w http.ResponseWriter, r *http.Request) {
	agentID, ok := pathUUID(w, r, "agentID")
	if !ok {
		return
	}
	artifactID, ok := pathUUID(w, r, "artifactID")
	if !ok {
		return
	}
	if s.storage == nil {
		internalError(s, w, errors.New("storage is unavailable"))
		return
	}
	item, err := s.repo.AgentArtifactForDownload(
		r.Context(), mustUUID(accountFrom(r).ID), agentID, artifactID,
	)
	if err != nil {
		if agentRunResourceError(w, err) {
			return
		}
		internalError(s, w, err)
		return
	}
	file, err := s.storage.OpenRelative(r.Context(), item.StoragePath)
	if errors.Is(err, os.ErrNotExist) {
		internalError(s, w, errors.New("agent artifact references a missing blob"))
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	defer file.Close()

	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	contentDisposition := disposition
	if formatted := mime.FormatMediaType(disposition, map[string]string{"filename": item.Title}); formatted != "" {
		contentDisposition = formatted
	}
	w.Header().Set("Content-Disposition", contentDisposition)
	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, item.Title, item.CreatedAt, file)
}
