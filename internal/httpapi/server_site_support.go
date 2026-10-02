package httpapi

import (
	"errors"
	"net/http"

	"github.com/Stealth-deplover/stealth/internal/apikey"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/google/uuid"
)

// authorizeSiteDeploymentWrite verifies a caller may write to the target
// project/site before an upload stages or commits an artifact. API-key actors
// were already bound to projectID by requireProjectManagement, so only the
// write scope remains; console actors must resolve an owner/admin role and the
// site must exist in the project.
func (s *Server) authorizeSiteDeploymentWrite(w http.ResponseWriter, r *http.Request, projectID, siteID uuid.UUID) bool {
	actor := siteActorFrom(r)
	if actor.Kind == repository.SiteAPIKeyActor {
		if !apikey.HasScope(actor.APIKeyScopes, "sites.write") {
			writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to manage Sites")
			return false
		}
		return true
	}
	if _, err := s.repo.GetSite(r.Context(), projectID, siteID, actor); err != nil {
		if siteResourceError(w, err) {
			return false
		}
		internalError(s, w, err)
		return false
	}
	return true
}

func sitePathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	siteID, ok := pathUUID(w, r, "siteID")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, siteID, true
}

func siteDeploymentPathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	deploymentID, ok := pathUUID(w, r, "deploymentID")
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return projectID, siteID, deploymentID, true
}

func siteResourceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "project or site was not found")
		return true
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to manage Sites")
		return true
	case errors.Is(err, repository.ErrSiteQuotaExceeded):
		writeError(w, http.StatusConflict, "quota_exceeded", "site artifact quota would be exceeded")
		return true
	case errors.Is(err, repository.ErrSiteDeploymentActive):
		writeError(w, http.StatusConflict, "deployment_active", "the active site deployment cannot be deleted")
		return true
	case errors.Is(err, repository.ErrSiteDisabled):
		writeError(w, http.StatusConflict, "site_disabled", "the site is disabled")
		return true
	case errors.Is(err, repository.ErrInvalidSiteTransition):
		writeError(w, http.StatusConflict, "invalid_transition", "the site deployment is not ready to activate")
		return true
	case errors.Is(err, repository.ErrInvalidSiteSettings):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "site settings are invalid")
		return true
	default:
		return false
	}
}
