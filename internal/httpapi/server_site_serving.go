package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/sitestore"
	"github.com/go-chi/chi/v5"
)

func (s *Server) deleteSiteDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	_, err := s.repo.DeleteSiteDeploymentWithArtifact(r.Context(), projectID, siteID, deploymentID, siteActorFrom(r))
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) activateSiteDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	item, site, err := s.repo.ActivateSiteDeployment(r.Context(), projectID, siteID, deploymentID, siteActorFrom(r))
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"site": site, "deployment": item})
}

// serveSiteFile is intentionally unauthenticated: a published Site is a
// public web origin. PostgreSQL resolves the active deployment first, then
// sitestore validates every filesystem component before opening the file.
func (s *Server) serveSiteFile(w http.ResponseWriter, r *http.Request) {
	siteID, err := repository.ParseUUID(chi.URLParam(r, "siteID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "site was not found")
		return
	}
	if s.sites == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "site artifact storage is not ready")
		return
	}
	artifact, err := s.repo.GetActiveSiteArtifact(r.Context(), siteID)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "site was not found")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	s.servePublishedSiteFile(w, r, artifact)
}

// serveSiteDeploymentFile exposes a ready immutable release at a preview URL.
// It is public by design: the deployment UUID is the capability-like URL and
// the Site must still be enabled. The route never accepts a filesystem path
// from the client as an artifact locator.
func (s *Server) serveSiteDeploymentFile(w http.ResponseWriter, r *http.Request) {
	siteID, err := repository.ParseUUID(chi.URLParam(r, "siteID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "site was not found")
		return
	}
	deploymentID, err := repository.ParseUUID(chi.URLParam(r, "deploymentID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "site deployment was not found")
		return
	}
	if s.sites == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "site artifact storage is not ready")
		return
	}
	artifact, err := s.repo.GetSiteDeploymentArtifact(r.Context(), siteID, deploymentID)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "site deployment was not found")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	s.servePublishedSiteFile(w, r, artifact)
}

func (s *Server) servePublishedSiteFile(w http.ResponseWriter, r *http.Request, artifact repository.SitePublicArtifact) {
	requested := chi.URLParam(r, "*")
	if requested == "" {
		requested = "index.html"
	}
	file, info, err := s.sites.OpenFile(artifact.ArtifactPath, requested)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "not_found", "site file was not found")
		return
	}
	if errors.Is(err, sitestore.ErrInvalidFile) || errors.Is(err, sitestore.ErrInvalidPath) {
		writeError(w, http.StatusNotFound, "not_found", "site file was not found")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	defer file.Close()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	// The deployment checksum alone is identical for every file in a release,
	// so a shared ETag made conditional and range requests return the wrong
	// body. Mix in the requested path and the file identity for a per-file tag.
	fileTag := sha256.Sum256([]byte(
		artifact.Deployment.ChecksumSHA256 + "\x00" + requested + "\x00" +
			strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10),
	))
	w.Header().Set("ETag", `"`+hex.EncodeToString(fileTag[:])+`"`)
	ext := strings.ToLower(filepath.Ext(requested))
	if mimeType := mime.TypeByExtension(ext); mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	if ext == ".html" || ext == ".htm" || requested == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60")
	}
	http.ServeContent(w, r, filepath.Base(requested), info.ModTime(), file)
}
