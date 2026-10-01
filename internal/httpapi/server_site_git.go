package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/functionrunner"
	"github.com/Stealth-deplover/stealth/internal/functionstore"
	"github.com/Stealth-deplover/stealth/internal/gitarchive"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/sitestore"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/google/uuid"
)

func (s *Server) createGitSiteDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return
	}
	if s.siteArchives == nil || s.siteGitFetcher == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "Git deployment storage is not ready")
		return
	}
	var req siteGitDeploymentRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Repository) == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "repository is required")
		return
	}
	buildOptions, err := parseSiteBuildOptions(req.BuildRuntime, req.BuildCommand, req.OutputDirectory)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if buildOptions.Command == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "build_command is required for a Git deployment")
		return
	}
	if s.siteGitSlots == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "Git deployment capacity is not ready")
		return
	}
	select {
	case s.siteGitSlots <- struct{}{}:
		defer func() { <-s.siteGitSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusTooManyRequests, "git_deployment_busy", "too many Git deployments are downloading; retry later")
		return
	}
	activate := true
	if req.Activate != nil {
		activate = *req.Activate
	}
	deploymentID := uuid.Must(uuid.NewV7())
	archive, err := s.siteGitFetcher.Fetch(r.Context(), req.Repository, req.Ref, s.config.SitesMaxArtifactSize)
	if err != nil {
		switch {
		case errors.Is(err, gitarchive.ErrInvalidRepository), errors.Is(err, gitarchive.ErrInvalidRef):
			writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		case errors.Is(err, gitarchive.ErrTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Git archive exceeds the configured maximum size")
		case errors.Is(err, gitarchive.ErrUnavailable):
			writeError(w, http.StatusBadGateway, "git_archive_unavailable", "the Git provider archive could not be downloaded")
		default:
			internalError(s, w, err)
		}
		return
	}
	if archive.Body == nil || storage.ValidateFilename(archive.Filename) != nil || (archive.Provider != "github" && archive.Provider != "gitlab") || archive.Repository == "" || archive.Ref == "" {
		if archive.Body != nil {
			_ = archive.Body.Close()
		}
		internalError(s, w, errors.New("Git provider returned an invalid archive descriptor"))
		return
	}
	defer archive.Body.Close()
	prepared, err := s.siteArchives.BeginUploadWithLimit(r.Context(), projectID, siteID, deploymentID, archive.Body, s.config.SitesMaxArtifactSize)
	if errors.Is(err, functionstore.ErrTooLarge) || errors.Is(err, gitarchive.ErrTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Git archive exceeds the configured maximum size")
		return
	}
	if errors.Is(err, gitarchive.ErrUnavailable) {
		writeError(w, http.StatusBadGateway, "git_archive_unavailable", "the Git provider archive could not be downloaded")
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			s.siteArchives.Cleanup(&prepared)
		}
	}()
	artifactPath, err := sitestore.ArtifactRelativePath(projectID, siteID, deploymentID)
	if err != nil {
		internalError(s, w, err)
		return
	}
	publishCleanup := repository.ArtifactCleanupInput{
		ProjectID:    projectID,
		StoreKind:    repository.ArtifactCleanupSiteArchives,
		Operation:    repository.ArtifactCleanupRelative,
		RelativePath: prepared.RelativePath,
	}
	if err := s.repo.ReserveArtifactPublishCleanup(r.Context(), publishCleanup); err != nil {
		internalError(s, w, err)
		return
	}
	if err := s.siteArchives.Commit(r.Context(), &prepared); err != nil {
		internalError(s, w, err)
		return
	}
	committed = true
	actor := siteActorFrom(r)
	var createdBy *uuid.UUID
	if actor.Kind == repository.SiteConsoleActor && actor.AccountID != uuid.Nil {
		createdBy = &actor.AccountID
	}
	gitRepository, gitRef := archive.Repository, archive.Ref
	item, err := s.repo.CreateSiteDeployment(r.Context(), deploymentID, projectID, siteID, actor, repository.SiteDeploymentInput{
		Source:             archive.Provider,
		SourceName:         &archive.Filename,
		GitRepository:      &gitRepository,
		GitRef:             &gitRef,
		SizeBytes:          0,
		ArchiveSizeBytes:   prepared.Size,
		ChecksumSHA256:     prepared.Checksum,
		ArtifactPath:       artifactPath,
		SourcePath:         &prepared.RelativePath,
		BuildRuntime:       buildOptions.Runtime,
		BuildCommand:       buildOptions.Command,
		OutputDirectory:    buildOptions.OutputDirectory,
		ReservedBytes:      s.config.SitesMaxExpandedBytes,
		CreatedByAccountID: createdBy,
		Activate:           activate,
		PublishCleanup:     &publishCleanup,
	})
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.SiteDeployment{"deployment": item})
}

func writeSiteArchiveError(s *Server, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, functionrunner.ErrArchiveTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "site archive expands beyond the configured limits")
	case errors.Is(err, functionrunner.ErrUnsupportedArchive):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "site archive must be a .zip, .tar, .tar.gz, or .tgz file")
	case errors.Is(err, functionrunner.ErrArchiveTraversal), errors.Is(err, functionrunner.ErrArchiveEntry):
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "site archive contains an unsafe or duplicate entry")
	default:
		internalError(s, w, err)
	}
}
