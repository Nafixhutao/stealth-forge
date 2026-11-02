package agentrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
	"github.com/google/uuid"
)

// RustAdapter runs the trusted Rust agent binary as a child process. The Go
// worker keeps ownership of the queue, leases, logs, and cancellation; the
// binary only receives one bounded job and returns one bounded result.
type RustAdapter struct {
	BinaryPath string
	// WorkRoot is the worker path where the shared staging volume is mounted.
	// Produced files are read from <WorkRoot>/agent/<run id>.
	WorkRoot  string
	Artifacts ArtifactStore
	Logger    *slog.Logger
}

type rustJob struct {
	Run   domain.AgentRun `json:"run"`
	Agent domain.Agent    `json:"agent"`
}

type rustResult struct {
	OutputText   *string                 `json:"output_text"`
	ErrorMessage *string                 `json:"error_message"`
	Steps        []domain.AgentRunStep   `json:"steps"`
	Changes      []domain.AgentRunChange `json:"changes"`
	Files        []rustFile              `json:"files"`
}

type rustFile struct {
	Path      string `json:"path"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

// Execute implements Adapter. A structured failure from the binary is returned
// as a failed result rather than an error, so the run's own error_message is
// persisted instead of being replaced by a generic worker message.
func (a *RustAdapter) Execute(ctx context.Context, job Job) (repository.AgentRunResult, error) {
	if strings.TrimSpace(a.BinaryPath) == "" {
		return repository.AgentRunResult{}, errors.New("agent runtime binary is not configured")
	}
	payload, err := json.Marshal(rustJob{Run: job.Run, Agent: job.Agent})
	if err != nil {
		return repository.AgentRunResult{}, fmt.Errorf("encode agent job: %w", err)
	}

	cmd := exec.CommandContext(ctx, a.BinaryPath)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	a.logStderr(stderr.String(), job.Run.ID)

	if runErr != nil {
		if ctx.Err() != nil {
			return repository.AgentRunResult{}, ctx.Err()
		}
		if result, ok := decodeRustResult(stdout.Bytes()); ok && hasMessage(result.ErrorMessage) {
			return failedRustResult(result), nil
		}
		return repository.AgentRunResult{}, fmt.Errorf("agent runtime failed: %w", runErr)
	}

	result, ok := decodeRustResult(stdout.Bytes())
	if !ok {
		return repository.AgentRunResult{}, errors.New("agent runtime returned an unreadable result")
	}
	if hasMessage(result.ErrorMessage) {
		return failedRustResult(result), nil
	}
	if a.Logger != nil && len(result.Files) > 0 {
		a.Logger.Info("agent produced files", "run_id", job.Run.ID, "count", len(result.Files))
	}
	return repository.AgentRunResult{
		Status:     "completed",
		OutputText: result.OutputText,
		Steps:      nonNilSteps(result.Steps),
		Changes:    nonNilChanges(result.Changes),
		Artifacts:  a.collectArtifacts(ctx, job, result.Files),
	}, nil
}

// collectArtifacts uploads the run's files to Storage. A missing or unreadable
// file is logged and skipped: a run that produced a valid document should not
// be failed because one secondary file vanished.
func (a *RustAdapter) collectArtifacts(
	ctx context.Context,
	job Job,
	files []rustFile,
) []domain.AgentArtifact {
	if a.Artifacts == nil || strings.TrimSpace(a.WorkRoot) == "" || len(files) == 0 {
		return nil
	}
	projectID, err := uuid.Parse(job.Run.ProjectID)
	if err != nil {
		return nil
	}
	runDir := filepath.Join(a.WorkRoot, "agent", job.Run.ID)
	artifacts := make([]domain.AgentArtifact, 0, len(files))
	for _, file := range files {
		relative := filepath.Clean(file.Path)
		if relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			continue
		}
		path := filepath.Join(runDir, relative)
		if !strings.HasPrefix(path, runDir+string(os.PathSeparator)) {
			continue
		}
		handle, err := os.Open(path)
		if err != nil {
			a.logf("agent artifact missing", job.Run.ID, path, err)
			continue
		}
		artifactID := uuid.Must(uuid.NewV7())
		storagePath, saveErr := a.Artifacts.Save(
			ctx, artifactID, projectID, filepath.Base(relative), handle,
		)
		_ = handle.Close()
		if saveErr != nil {
			a.logf("agent artifact upload failed", job.Run.ID, path, saveErr)
			continue
		}
		artifacts = append(artifacts, domain.AgentArtifact{
			ID:          artifactID.String(),
			Kind:        "document",
			Title:       filepath.Base(relative),
			MimeType:    file.MimeType,
			StoragePath: storagePath,
			SizeBytes:   file.SizeBytes,
		})
	}
	if a.Logger != nil && len(artifacts) > 0 {
		a.Logger.Info("agent artifacts stored", "run_id", job.Run.ID, "count", len(artifacts))
	}
	return artifacts
}

func (a *RustAdapter) logf(message, runID, path string, err error) {
	if a.Logger == nil {
		return
	}
	a.Logger.Warn(message, "run_id", runID, "path", path, "error", err)
}

func (a *RustAdapter) logStderr(raw, runID string) {
	if a.Logger == nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		a.Logger.Info("agent runtime", "run_id", runID, "line", line)
	}
}

func decodeRustResult(raw []byte) (rustResult, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return rustResult{}, false
	}
	var result rustResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return rustResult{}, false
	}
	return result, true
}

func hasMessage(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func failedRustResult(result rustResult) repository.AgentRunResult {
	message := "agent provider execution failed"
	if hasMessage(result.ErrorMessage) {
		message = strings.TrimSpace(*result.ErrorMessage)
	}
	if len(message) > maxPublicError {
		message = message[:maxPublicError]
	}
	return repository.AgentRunResult{
		Status:       "failed",
		OutputText:   result.OutputText,
		ErrorMessage: &message,
		Steps:        nonNilSteps(result.Steps),
		Changes:      nonNilChanges(result.Changes),
	}
}

func nonNilSteps(steps []domain.AgentRunStep) []domain.AgentRunStep {
	if steps == nil {
		return []domain.AgentRunStep{}
	}
	return steps
}

func nonNilChanges(changes []domain.AgentRunChange) []domain.AgentRunChange {
	if changes == nil {
		return []domain.AgentRunChange{}
	}
	return changes
}
