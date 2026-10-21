package httpapi

import (
	"net/http"

	"github.com/Nafixhutao/stealth-forge/internal/sitestore"
)

// siteDeploymentFilesResponse is the source browser's index: every regular file
// in one published release, with its size. Contents are not inlined here — the
// existing public serve route for the deployment already returns a file, so the
// viewer fetches only the file the operator opens.
type siteDeploymentFilesResponse struct {
	Files     []sitestore.FileEntry `json:"files"`
	Truncated bool                  `json:"truncated"`
}

// listSiteDeploymentFiles lists the files of one immutable release.
//
// The artifact path comes from the repository, which resolves it from the
// deployment's own row after checking the caller's project role; nothing in the
// request can name a storage location. A deployment that has not finished
// building has no artifact yet and answers 409 rather than an empty list, so the
// UI can tell "still building" from "this release is empty".
func (s *Server) listSiteDeploymentFiles(w http.ResponseWriter, r *http.Request) {
	projectID, siteID, deploymentID, ok := siteDeploymentPathIDs(w, r)
	if !ok {
		return
	}
	if s.sites == nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "site artifact storage is not ready")
		return
	}
	item, artifactPath, err := s.repo.GetSiteDeploymentArtifactPath(
		r.Context(), projectID, siteID, deploymentID, siteActorFrom(r),
	)
	if siteResourceError(w, err) {
		return
	}
	if err != nil {
		internalError(s, w, err)
		return
	}
	if item.BuildStatus != "succeeded" || artifactPath == "" {
		writeError(w, http.StatusConflict, "not_ready", "this deployment has no published files yet")
		return
	}
	files, truncated, err := s.sites.ListFiles(artifactPath, sitestore.MaxListedFiles)
	if err != nil {
		internalError(s, w, err)
		return
	}
	writeJSON(w, http.StatusOK, siteDeploymentFilesResponse{Files: files, Truncated: truncated})
}
