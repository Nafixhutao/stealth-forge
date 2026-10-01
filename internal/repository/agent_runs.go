package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Stealth-deplover/stealth/internal/domain"
)

var (
	ErrInvalidAgentRun           = errors.New("invalid agent run")
	ErrInvalidAgentRunTransition = errors.New("invalid agent run transition")
	ErrAgentRunNotAvailable      = errors.New("agent run is not available")
	ErrNoAgentRunJob             = errors.New("no agent run job available")
)

const (
	agentRunMaxPrompt     = 20_000
	agentRunMaxOutput     = 100_000
	agentRunMaxError      = 4_000
	agentRunMaxSteps      = 256
	agentRunMaxChanges    = 256
	agentRunMaxLogBytes   = 16_000
	agentRunMaxWorkerID   = 128
	agentRunMaxStepLabel  = 256
	agentRunMaxStepTarget = 2_000
	agentRunMaxChangePath = 1_000
)

const agentRunProjection = `r.id,r.agent_id,r.project_id,r.created_by_account_id,r.prompt,r.status,r.output_text,r.error_message,r.steps,r.changes,r.queued_at,r.started_at,r.finished_at,r.created_at,r.updated_at`
const agentRunReturningProjection = `id,agent_id,project_id,created_by_account_id,prompt,status,output_text,error_message,steps,changes,queued_at,started_at,finished_at,created_at,updated_at`
const agentRunLogProjection = `l.id,l.run_id,l.project_id,l.sequence,l.level,l.message,l.created_at`

type agentRunScanner interface {
	Scan(...any) error
}

type AgentRunInput struct {
	Prompt string
}

// AgentRunResult is only accepted by a trusted worker. Provider credentials,
// source contents, and tool execution details never travel through the
// Console API.
type AgentRunResult struct {
	Status       string
	OutputText   *string
	ErrorMessage *string
	Steps        []domain.AgentRunStep
	Changes      []domain.AgentRunChange
}

type AgentRunJob struct {
	Run   domain.AgentRun
	Agent domain.Agent
}

func scanAgentRun(row agentRunScanner) (domain.AgentRun, error) {
	var item domain.AgentRun
	var stepsJSON, changesJSON []byte
	err := row.Scan(
		&item.ID, &item.AgentID, &item.ProjectID, &item.CreatedByAccountID, &item.Prompt,
		&item.Status, &item.OutputText, &item.ErrorMessage, &stepsJSON, &changesJSON,
		&item.QueuedAt, &item.StartedAt, &item.FinishedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return domain.AgentRun{}, err
	}
	if len(stepsJSON) == 0 {
		item.Steps = []domain.AgentRunStep{}
	} else if err := json.Unmarshal(stepsJSON, &item.Steps); err != nil {
		return domain.AgentRun{}, fmt.Errorf("decode agent run steps: %w", err)
	}
	if len(changesJSON) == 0 {
		item.Changes = []domain.AgentRunChange{}
	} else if err := json.Unmarshal(changesJSON, &item.Changes); err != nil {
		return domain.AgentRun{}, fmt.Errorf("decode agent run changes: %w", err)
	}
	if item.Steps == nil {
		item.Steps = []domain.AgentRunStep{}
	}
	if item.Changes == nil {
		item.Changes = []domain.AgentRunChange{}
	}
	return item, nil
}

func scanAgentRunLog(row agentRunScanner) (domain.AgentRunLog, error) {
	var item domain.AgentRunLog
	return item, row.Scan(&item.ID, &item.RunID, &item.ProjectID, &item.Sequence, &item.Level, &item.Message, &item.CreatedAt)
}

func normalizeAgentRunPrompt(prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("%w: prompt is required", ErrInvalidAgentRun)
	}
	if utf8.RuneCountInString(prompt) > agentRunMaxPrompt {
		return "", fmt.Errorf("%w: prompt must be at most %d characters", ErrInvalidAgentRun, agentRunMaxPrompt)
	}
	if strings.ContainsRune(prompt, '\x00') {
		return "", fmt.Errorf("%w: prompt cannot contain NUL", ErrInvalidAgentRun)
	}
	return prompt, nil
}

func normalizeAgentRunWorkerID(workerID string) (string, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > agentRunMaxWorkerID || strings.ContainsAny(workerID, "\x00\r\n\t") {
		return "", fmt.Errorf("%w: worker id is invalid", ErrInvalidAgentRun)
	}
	return workerID, nil
}

func normalizeAgentRunResult(result AgentRunResult) (AgentRunResult, []byte, []byte, error) {
	if result.Status != "completed" && result.Status != "failed" && result.Status != "cancelled" {
		return AgentRunResult{}, nil, nil, fmt.Errorf("%w: terminal status is invalid", ErrInvalidAgentRunTransition)
	}
	if result.OutputText != nil {
		value := *result.OutputText
		if len(value) > agentRunMaxOutput || strings.ContainsRune(value, '\x00') {
			return AgentRunResult{}, nil, nil, fmt.Errorf("%w: output_text is too large or contains NUL", ErrInvalidAgentRun)
		}
		result.OutputText = &value
	}
	if result.ErrorMessage != nil {
		value := strings.TrimSpace(*result.ErrorMessage)
		if len(value) > agentRunMaxError || strings.ContainsRune(value, '\x00') {
			return AgentRunResult{}, nil, nil, fmt.Errorf("%w: error_message is too large or contains NUL", ErrInvalidAgentRun)
		}
		if value == "" {
			result.ErrorMessage = nil
		} else {
			result.ErrorMessage = &value
		}
	}
	if len(result.Steps) > agentRunMaxSteps || len(result.Changes) > agentRunMaxChanges {
		return AgentRunResult{}, nil, nil, fmt.Errorf("%w: result contains too many steps or changes", ErrInvalidAgentRun)
	}
	if result.Steps == nil {
		result.Steps = []domain.AgentRunStep{}
	}
	if result.Changes == nil {
		result.Changes = []domain.AgentRunChange{}
	}
	for index := range result.Steps {
		step := &result.Steps[index]
		step.ID = strings.TrimSpace(step.ID)
		step.Type = strings.TrimSpace(step.Type)
		step.Label = strings.TrimSpace(step.Label)
		step.Target = strings.TrimSpace(step.Target)
		step.Status = strings.TrimSpace(step.Status)
		if !validAgentRunStep(*step) {
			return AgentRunResult{}, nil, nil, fmt.Errorf("%w: step %d is invalid", ErrInvalidAgentRun, index)
		}
	}
	for index := range result.Changes {
		change := &result.Changes[index]
		change.Path = strings.TrimSpace(change.Path)
		change.Status = strings.TrimSpace(change.Status)
		if change.Path == "" || len(change.Path) > agentRunMaxChangePath || strings.ContainsAny(change.Path, "\x00\r\n") || (change.Status != "added" && change.Status != "modified") || change.Additions < 0 || change.Deletions < 0 {
			return AgentRunResult{}, nil, nil, fmt.Errorf("%w: change %d is invalid", ErrInvalidAgentRun, index)
		}
	}
	stepsJSON, err := json.Marshal(result.Steps)
	if err != nil {
		return AgentRunResult{}, nil, nil, err
	}
	changesJSON, err := json.Marshal(result.Changes)
	if err != nil {
		return AgentRunResult{}, nil, nil, err
	}
	return result, stepsJSON, changesJSON, nil
}

func validAgentRunStep(step domain.AgentRunStep) bool {
	if step.ID == "" || len(step.ID) > 128 || strings.ContainsAny(step.ID, "\x00\r\n\t") {
		return false
	}
	if step.Type != "read" && step.Type != "edit" && step.Type != "search" && step.Type != "command" && step.Type != "check" {
		return false
	}
	if step.Label == "" || utf8.RuneCountInString(step.Label) > agentRunMaxStepLabel || strings.ContainsAny(step.Label, "\x00\r\n") {
		return false
	}
	if step.Target == "" || utf8.RuneCountInString(step.Target) > agentRunMaxStepTarget || strings.ContainsAny(step.Target, "\x00\r\n") {
		return false
	}
	return step.Status == "pending" || step.Status == "done"
}
