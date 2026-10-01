package httpapi

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/functionrunner"
	"github.com/Stealth-deplover/stealth/internal/functionstore"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/sitestore"
	"github.com/Stealth-deplover/stealth/internal/storage"
	"github.com/google/uuid"
)

func (s *Server) uploadSiteDeployment(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return
	}
	if s.siteArchives == nil || s.sites == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "site artifact storage is not ready")
		return
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be multipart/form-data")
		return
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	deploymentID := uuid.Must(uuid.NewV7())
	var prepared functionstore.PreparedArtifact
	var sourceFilename, sourceNameOverride string
	var buildRuntime, buildCommand, outputDirectory string
	activate := true
	haveSource, haveActivate := false, false
	sourceCommitted := false
	defer func() {
		if !sourceCommitted {
			s.siteArchives.Cleanup(&prepared)
		}
	}()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			if isMaxBytesError(nextErr) {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds the configured upload limit")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart upload")
			}
			return
		}
		switch part.FormName() {
		case "source":
			if haveSource {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "source may only be provided once")
				return
			}
			haveSource = true
			sourceFilename = part.FileName()
			if sourceFilename != "" {
				if err := storage.ValidateFilename(sourceFilename); err != nil {
					_ = part.Close()
					writeError(w, http.StatusUnprocessableEntity, "validation_error", "source filename is invalid")
					return
				}
			}
			prepared, err = s.siteArchives.BeginUploadWithLimit(r.Context(), projectID, siteID, deploymentID, part, s.config.SitesMaxArtifactSize)
			_ = part.Close()
			if errors.Is(err, functionstore.ErrTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "site archive exceeds the configured maximum size")
				return
			}
			if err != nil {
				internalError(s, w, err)
				return
			}
		case "source_name":
			value, readErr := readFunctionMultipartField(part, 512)
			_ = part.Close()
			if readErr != nil || storage.ValidateFilename(value) != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "source_name is invalid")
				return
			}
			sourceNameOverride = value
		case "activate":
			if haveActivate {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate may only be provided once")
				return
			}
			haveActivate = true
			value, readErr := readFunctionMultipartField(part, 16)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate must be true or false")
				return
			}
			activate, err = strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "activate must be true or false")
				return
			}
		case "build_runtime":
			if buildRuntime != "" {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "build_runtime may only be provided once")
				return
			}
			value, readErr := readFunctionMultipartField(part, 64)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "build_runtime is invalid")
				return
			}
			buildRuntime = value
		case "build_command":
			if buildCommand != "" {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "build_command may only be provided once")
				return
			}
			value, readErr := readFunctionMultipartField(part, maxSiteBuildCommandBytes+1)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "build_command is invalid")
				return
			}
			buildCommand = value
		case "output_directory":
			if outputDirectory != "" {
				_ = part.Close()
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "output_directory may only be provided once")
				return
			}
			value, readErr := readFunctionMultipartField(part, 256)
			_ = part.Close()
			if readErr != nil {
				writeError(w, http.StatusUnprocessableEntity, "validation_error", "output_directory is invalid")
				return
			}
			outputDirectory = value
		default:
			_ = part.Close()
			writeError(w, http.StatusUnprocessableEntity, "validation_error", "unsupported deployment field")
			return
		}
	}
	if !haveSource || prepared.TempPath == "" {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "source is required")
		return
	}
	sourceName := sourceFilename
	if sourceNameOverride != "" {
		sourceName = sourceNameOverride
	}
	if sourceName == "" || storage.ValidateFilename(sourceName) != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "source filename is required and must be safe")
		return
	}
	buildOptions, err := parseSiteBuildOptions(buildRuntime, buildCommand, outputDirectory)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	if buildOptions.Command != "" {
		artifactPath, pathErr := sitestore.ArtifactRelativePath(projectID, siteID, deploymentID)
		if pathErr != nil {
			internalError(s, w, pathErr)
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
		sourceCommitted = true
		actor := siteActorFrom(r)
		var createdBy *uuid.UUID
		if actor.Kind == repository.SiteConsoleActor && actor.AccountID != uuid.Nil {
			createdBy = &actor.AccountID
		}
		item, err := s.repo.CreateSiteDeployment(r.Context(), deploymentID, projectID, siteID, actor, repository.SiteDeploymentInput{
			Source:             "upload",
			SourceName:         &sourceName,
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
		return
	}
	archive, err := os.Open(prepared.TempPath)
	if err != nil {
		internalError(s, w, err)
		return
	}
	staging, artifactPath, err := s.sites.BeginStaging(projectID, siteID, deploymentID)
	if err != nil {
		_ = archive.Close()
		internalError(s, w, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			s.sites.CleanupStaging(staging)
		}
	}()
	limits := functionrunner.ArchiveLimits{MaxBytes: s.config.SitesMaxExpandedBytes, MaxFiles: s.config.SitesMaxFiles, MaxEntry: s.config.SitesMaxExpandedBytes, MaxCompressed: s.config.SitesMaxArtifactSize}
	stats, extractErr := functionrunner.Extract(r.Context(), archive, sourceName, staging, limits)
	closeErr := archive.Close()
	if extractErr != nil {
		writeSiteArchiveError(s, w, extractErr)
		return
	}
	if closeErr != nil {
		internalError(s, w, closeErr)
		return
	}
	if stats.Files == 0 {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "site archive must contain at least one file")
		return
	}
	if err := sitestore.ValidateEntrypoint(staging, "index.html"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "site archive must contain a regular index.html at its root")
		return
	}
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
	committed = true
	actor := siteActorFrom(r)
	var createdBy *uuid.UUID
	if actor.Kind == repository.SiteConsoleActor && actor.AccountID != uuid.Nil {
		createdBy = &actor.AccountID
	}
	item, err := s.repo.CreateSiteDeployment(r.Context(), deploymentID, projectID, siteID, actor, repository.SiteDeploymentInput{Source: "upload", SourceName: &sourceName, SizeBytes: stats.Bytes, ArchiveSizeBytes: prepared.Size, ChecksumSHA256: prepared.Checksum, ArtifactPath: artifactPath, CreatedByAccountID: createdBy, Activate: activate, PublishCleanup: &publishCleanup})
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.SiteDeployment{"deployment": item})
}

// createGitSiteDeployment downloads one immutable public Git archive and then
// feeds it through the same queued, network-isolated Site builder used by
// multipart source uploads. The fetcher reconstructs provider URLs from
// validated components; clients never supply an arbitrary upstream URL.
