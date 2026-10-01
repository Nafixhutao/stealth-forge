package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
)

func (s *Server) listSiteDeployments(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
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
	items, next, canManage, err := s.repo.ListSiteDeployments(r.Context(), projectID, siteID, siteActorFrom(r), limit, cursorID)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) listSiteBuildLogs(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	limit, _, ok := page(w, r)
	if !ok {
		return
	}
	after := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("after")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "after must be a non-negative integer")
			return
		}
		after = parsed
	}
	items, err := s.repo.ListSiteBuildLogs(r.Context(), projectID, siteID, deploymentID, siteActorFrom(r), limit, after)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	next := ""
	if len(items) == limit {
		next = strconv.FormatInt(items[len(items)-1].Sequence, 10)
	}
	var nextCursor *string
	if next != "" {
		nextCursor = &next
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": items, "pagination": pagination{Limit: limit, NextCursor: nextCursor}})
}

func (s *Server) getSiteDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetSiteDeployment(r.Context(), projectID, siteID, deploymentID, siteActorFrom(r))
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.SiteDeployment{"deployment": item})
}
