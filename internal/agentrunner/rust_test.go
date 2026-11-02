package agentrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
)

// The adapter is verified at its process boundary: a deterministic script in
// place of the Rust binary proves the stdin/stdout contract without needing
// Rust in the Go test run.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func testJob() Job {
	return Job{
		Run: domain.AgentRun{
			ID:        "run-1",
			AgentID:   "agent-1",
			ProjectID: "project-1",
			Prompt:    "make a letter",
		},
		Agent: domain.Agent{ID: "agent-1", Name: "Agent", Provider: "rinsdev"},
	}
}

func TestRustAdapterReturnsCompletedResult(t *testing.T) {
	binary := writeScript(t, `cat >/dev/null
printf '%s' '{"output_text":"done","steps":[{"id":"s1","type":"command","label":"shell","target":"ls","status":"done"}],"changes":[],"files":[{"path":"letter.docx","mime_type":"application/vnd.openxmlformats-officedocument.wordprocessingml.document","size_bytes":10}]}'
`)
	adapter := &RustAdapter{BinaryPath: binary}

	result, err := adapter.Execute(context.Background(), testJob())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if result.OutputText == nil || *result.OutputText != "done" {
		t.Fatalf("output_text = %v", result.OutputText)
	}
	if len(result.Steps) != 1 || result.Steps[0].Type != "command" {
		t.Fatalf("steps = %+v", result.Steps)
	}
}

func TestRustAdapterTurnsStructuredFailureIntoFailedResult(t *testing.T) {
	binary := writeScript(t, `cat >/dev/null
printf '%s' '{"output_text":null,"error_message":"model said no","steps":[],"changes":[],"files":[]}'
`)
	adapter := &RustAdapter{BinaryPath: binary}

	result, err := adapter.Execute(context.Background(), testJob())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if result.ErrorMessage == nil || *result.ErrorMessage != "model said no" {
		t.Fatalf("error_message = %v", result.ErrorMessage)
	}
}

func TestRustAdapterRejectsUnreadableOutput(t *testing.T) {
	binary := writeScript(t, `cat >/dev/null
printf 'not json'
`)
	adapter := &RustAdapter{BinaryPath: binary}

	if _, err := adapter.Execute(context.Background(), testJob()); err == nil {
		t.Fatal("expected an error for unreadable output")
	}
}

func TestRustAdapterRequiresBinary(t *testing.T) {
	adapter := &RustAdapter{}

	_, err := adapter.Execute(context.Background(), testJob())
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v", err)
	}
}
