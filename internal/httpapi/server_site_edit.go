package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/Nafixhutao/stealth-forge/internal/sitestore"
	"github.com/google/uuid"
)

// editSiteDeploymentFileRequest asks for a new release built from an existing
// one, with one file's contents replaced.
type editSiteDeploymentFileRequest struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Activate *bool  `json:"activate"`
}

// editSiteDeploymentFile publishes an edited copy of a release.
//
// A release is immutable: this never writes into a published directory. It
// copies the source release into a staging tree, replaces one existing file
// there, and publishes that tree as a new release — so the previous release
// stays exactly as it was and remains deployable.
//
// Everything that could make that unsafe is checked before the copy is
// published: the caller must be able to write Sites, the path must be a plain
// relative path that already exists in the release, the content must fit the
// configured artifact budget, symlinks are skipped by the copy rather than
// followed, and the quota is enforced by the same path an upload uses.
func (s *Server) editSiteDeploymentFile(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	if s.sites == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "site artifact storage is not ready")
		return
	}
	var req editSiteDeploymentFileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" || len(path) > 1024 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "path is required")
		return
	}
	if int64(len(req.Content)) > s.config.SitesMaxArtifactSize {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "the file exceeds the configured maximum size")
		return
	}
	activate := true
	if req.Activate != nil {
		activate = *req.Activate
	}

	actor := siteActorFrom(r)
	source, sourceArtifact, err := s.repo.GetSiteDeploymentArtifactPathForWrite(
		r.Context(), projectID, siteID, deploymentID, actor,
	)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	if source.BuildStatus != "succeeded" || sourceArtifact == "" {
		writeError(w, http.StatusConflict, "not_ready", "this release has no published files to edit")
		return
	}

	newDeploymentID := uuid.Must(uuid.NewV7())
	staging, artifactPath, err := s.sites.BeginStaging(projectID, siteID, newDeploymentID)
	if err != nil {
		internalError(s, w, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			s.sites.CleanupStaging(staging)
		}
	}()

	stats, err := s.sites.CopyForEdit(sourceArtifact, staging, sitestore.EditLimits{
		MaxBytes: s.config.SitesMaxExpandedBytes,
		MaxFiles: s.config.SitesMaxFiles,
	})
	if err != nil {
		writeSiteEditError(s, w, err)
		return
	}
	previous, written, err := s.sites.WriteStagedFile(staging, path, []byte(req.Content))
	if err != nil {
		writeSiteEditError(s, w, err)
		return
	}
	// The entry point is what makes the artifact a site; an edit cannot remove
	// it, but this also catches a release that never had one.
	if err := sitestore.ValidateEntrypoint(staging, "index.html"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "the release must keep a regular index.html at its root")
		return
	}
	digest, err := s.sites.DigestArtifact(staging)
	if err != nil {
		internalError(s, w, err)
		return
	}
	totalBytes := stats.Bytes - previous + written

	publishCleanup := repository.ArtifactCleanupInput{
		ProjectID:    projectID,
		StoreKind:    repository.ArtifactCleanupSites,
		Operation:    repository.ArtifactCleanupRelative,
		RelativePath: artifactPath,
	}
	if err := s.repo.ReserveArtifactPublishCleanup(r.Context(), publishCleanup); err != nil {
		internalError(s, w, err)
		return
	}
	if err := s.sites.CommitDirectory(staging, artifactPath); err != nil {
		internalError(s, w, err)
		return
	}

	// From here the artifact exists on disk, so a failure has to remove it: a
	// published directory with no row is unreachable and would sit in the quota
	// forever. `committed` is only set once the row is written.
	sourceName := editSourceName(path)
	var createdBy *uuid.UUID
	if actor.Kind == repository.SiteConsoleActor && actor.AccountID != uuid.Nil {
		createdBy = &actor.AccountID
	}
	archiveSize := totalBytes
	if archiveSize < 1 {
		archiveSize = 1
	}
	item, err := s.repo.CreateSiteDeployment(r.Context(), newDeploymentID, projectID, siteID, actor, repository.SiteDeploymentInput{
		Source:             "upload",
		SourceName:         &sourceName,
		SizeBytes:          totalBytes,
		ArchiveSizeBytes:   archiveSize,
		ChecksumSHA256:     digest,
		ArtifactPath:       artifactPath,
		CreatedByAccountID: createdBy,
		Activate:           activate,
		PublishCleanup:     &publishCleanup,
	})
	if siteResourceError(w, err) {
		// The row was refused (quota, conflict), so the artifact it would have
		// described is removed rather than left behind.
		_ = s.sites.RemoveRelative(r.Context(), artifactPath)
		return
	}
	if err != nil {
		_ = s.sites.RemoveRelative(r.Context(), artifactPath)
		internalError(s, w, err)
		return
	}
	committed = true
	writeJSON(w, http.StatusCreated, map[string]domain.SiteDeployment{"deployment": item})
}

// editSourceName describes an edit in the release's source_name column.
//
// The column is a single file name, not a path: the schema rejects "/" and "\",
// so a nested edit such as assets/app.css cannot be recorded verbatim. The base
// name identifies the file well enough for the release list, and the full path is
// in the audit trail.
func editSourceName(path string) string {
	base := path
	if index := strings.LastIndexByte(base, '/'); index >= 0 {
		base = base[index+1:]
	}
	name := "edit: " + base
	if len(name) > 255 {
		name = name[:255]
	}
	return strings.TrimRight(name, " ")
}

// writeSiteEditError maps the storage-level refusals of an edit onto the API's
// error envelope.
func writeSiteEditError(s *Server, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sitestore.ErrEditTargetMissing):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "the release has no such file")
	case errors.Is(err, sitestore.ErrInvalidFile), errors.Is(err, sitestore.ErrInvalidPath):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "path is not a file inside this release")
	default:
		internalError(s, w, err)
	}
}
