package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/Stealth-deplover/stealth/internal/validate"
	"github.com/google/uuid"
)

type siteRequest struct {
	Name               *string `json:"name"`
	Framework          *string `json:"framework"`
	Enabled            *bool   `json:"enabled"`
	Status             *string `json:"status"`
	ArtifactQuotaBytes *int64  `json:"artifact_quota_bytes"`
}

const maxSiteBuildCommandBytes = 4000

type siteBuildOptions struct {
	Runtime         string
	Command         string
	OutputDirectory string
}

type siteGitDeploymentRequest struct {
	Repository      string `json:"repository"`
	Ref             string `json:"ref"`
	BuildRuntime    string `json:"build_runtime"`
	BuildCommand    string `json:"build_command"`
	OutputDirectory string `json:"output_directory"`
	Activate        *bool  `json:"activate"`
}

func siteActorFrom(r *http.Request) repository.SiteActor {
	actor, ok := r.Context().Value(projectActorContextKey).(projectActor)
	if !ok {
		return repository.SiteActor{}
	}
	if actor.kind == apiKeyProjectActor {
		return repository.SiteActor{Kind: repository.SiteAPIKeyActor, APIKeyID: actor.apiKeyID, APIKeyScopes: actor.scopes}
	}
	account, ok := r.Context().Value(accountContextKey).(domain.Account)
	if !ok {
		return repository.SiteActor{}
	}
	return repository.SiteActor{Kind: repository.SiteConsoleActor, AccountID: mustUUID(account.ID)}
}

func parseSiteCreateRequest(s *Server, req siteRequest) (repository.SiteInput, error) {
	if req.Name == nil {
		return repository.SiteInput{}, errors.New("name is required")
	}
	name, err := validateSiteName(*req.Name)
	if err != nil {
		return repository.SiteInput{}, err
	}
	framework := "static"
	if req.Framework != nil {
		framework = strings.ToLower(strings.TrimSpace(*req.Framework))
	}
	if framework != "static" {
		return repository.SiteInput{}, errors.New("framework must be static")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	status := "active"
	if req.Status != nil {
		status = strings.ToLower(strings.TrimSpace(*req.Status))
	}
	if req.Enabled != nil && req.Status == nil {
		if enabled {
			status = "active"
		} else {
			status = "disabled"
		}
	} else if req.Enabled == nil && req.Status != nil {
		enabled = status == "active"
	}
	if status != "active" && status != "disabled" || (status == "active") != enabled {
		return repository.SiteInput{}, errors.New("status and enabled must describe an active or disabled site consistently")
	}
	quota := s.config.SitesDefaultQuotaBytes
	if req.ArtifactQuotaBytes != nil {
		quota = *req.ArtifactQuotaBytes
	}
	if quota <= 0 {
		return repository.SiteInput{}, errors.New("artifact_quota_bytes must be positive")
	}
	return repository.SiteInput{Name: name, Framework: framework, Enabled: enabled, Status: status, ArtifactQuotaBytes: quota}, nil
}

func parseSitePatchRequest(req siteRequest) (repository.SitePatch, error) {
	patch := repository.SitePatch{}
	changed := false
	if req.Name != nil {
		value, err := validateSiteName(*req.Name)
		if err != nil {
			return patch, err
		}
		patch.Name = &value
		changed = true
	}
	if req.Framework != nil {
		value := strings.ToLower(strings.TrimSpace(*req.Framework))
		if value != "static" {
			return patch, errors.New("framework must be static")
		}
		patch.Framework = &value
		changed = true
	}
	if req.Enabled != nil {
		patch.Enabled = req.Enabled
		changed = true
	}
	if req.Status != nil {
		value := strings.ToLower(strings.TrimSpace(*req.Status))
		if value != "active" && value != "disabled" {
			return patch, errors.New("status must be active or disabled")
		}
		patch.Status = &value
		changed = true
	}
	if req.ArtifactQuotaBytes != nil {
		if *req.ArtifactQuotaBytes <= 0 {
			return patch, errors.New("artifact_quota_bytes must be positive")
		}
		patch.ArtifactQuotaBytes = req.ArtifactQuotaBytes
		changed = true
	}
	if !changed {
		return patch, errors.New("at least one site setting is required")
	}
	return patch, nil
}

func validateSiteName(value string) (string, error) {
	return validate.Slug(value, "name")
}

func parseSiteBuildOptions(runtime, command, outputDirectory string) (siteBuildOptions, error) {
	command = strings.TrimSpace(command)
	runtime = strings.ToLower(strings.TrimSpace(runtime))
	outputDirectory = strings.TrimSpace(outputDirectory)
	if command == "" && runtime == "" && outputDirectory == "" {
		return siteBuildOptions{}, nil
	}
	if command == "" {
		return siteBuildOptions{}, errors.New("build_command is required when build options are provided")
	}
	if len(command) > maxSiteBuildCommandBytes || strings.ContainsRune(command, 0) {
		return siteBuildOptions{}, errors.New("build_command must be at most 4000 bytes and cannot contain NUL")
	}
	if runtime == "" {
		runtime = "node-22"
	}
	switch runtime {
	case "node-22", "python-3.13", "go-1.24":
	default:
		return siteBuildOptions{}, errors.New("build_runtime must be one of node-22, python-3.13, or go-1.24")
	}
	if outputDirectory == "" {
		outputDirectory = "."
	}
	if len(outputDirectory) > 255 || strings.HasPrefix(outputDirectory, "/") || strings.ContainsAny(outputDirectory, "\\\x00\r\n") {
		return siteBuildOptions{}, errors.New("output_directory must be a safe relative path")
	}
	if outputDirectory != "." {
		for _, part := range strings.Split(outputDirectory, "/") {
			if part == "" || part == "." || part == ".." {
				return siteBuildOptions{}, errors.New("output_directory must be a safe relative path")
			}
			for _, char := range part {
				if !(char == '-' || char == '_' || char == '.' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
					return siteBuildOptions{}, errors.New("output_directory must be a safe relative path")
				}
			}
		}
	}
	return siteBuildOptions{Runtime: runtime, Command: command, OutputDirectory: outputDirectory}, nil
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
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
	items, next, canManage, err := s.repo.ListSites(r.Context(), projectID, siteActorFrom(r), limit, cursorID)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": items, "pagination": paginationOf(limit, next), "can_manage": canManage})
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return
	}
	item, err := s.repo.GetSite(r.Context(), projectID, siteID, siteActorFrom(r))
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.Site{"site": item})
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectID")
	if !ok {
		return
	}
	var req siteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	input, err := parseSiteCreateRequest(s, req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.CreateSite(r.Context(), uuid.Must(uuid.NewV7()), projectID, siteActorFrom(r), input)
	if planLimitError(w, err) {
		return
	}
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]domain.Site{"site": item})
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return
	}
	var req siteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	patch, err := parseSitePatchRequest(req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	item, err := s.repo.UpdateSite(r.Context(), projectID, siteID, siteActorFrom(r), patch)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]domain.Site{"site": item})
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, ok := sitePathIDs(w, r)
	if !ok {
		return
	}
	_, err := s.repo.DeleteSite(r.Context(), projectID, siteID, siteActorFrom(r))
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
