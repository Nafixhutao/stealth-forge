package domain

import "time"

// Agent is the persisted configuration for a project coding agent. Provider
// credentials and execution output are deliberately not part of this
// projection; execution output belongs to AgentRun resources.
type Agent struct {
	ID                 string     `json:"id"`
	ProjectID          string     `json:"project_id"`
	ProjectName        string     `json:"project_name"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	Branch             string     `json:"branch"`
	Provider           string     `json:"provider"`
	Model              string     `json:"model"`
	CurrentTask        *string    `json:"current_task,omitempty"`
	LastActiveAt       *time.Time `json:"last_active_at,omitempty"`
	Tools              []string   `json:"tools"`
	Instructions       *string    `json:"instructions,omitempty"`
	CreatedByAccountID *string    `json:"created_by_account_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AgentRun is a durable request to execute an Agent. A run may remain queued
// until an execution worker and provider connection are available; the API
// never turns an accepted request into a fabricated completed response.
type AgentRun struct {
	ID                 string           `json:"id"`
	AgentID            string           `json:"agent_id"`
	ProjectID          string           `json:"project_id"`
	Prompt             string           `json:"prompt"`
	Status             string           `json:"status"`
	OutputText         *string          `json:"output_text,omitempty"`
	ErrorMessage       *string          `json:"error_message,omitempty"`
	Steps              []AgentRunStep   `json:"steps"`
	Changes            []AgentRunChange `json:"changes"`
	CreatedByAccountID *string          `json:"created_by_account_id,omitempty"`
	QueuedAt           time.Time        `json:"queued_at"`
	StartedAt          *time.Time       `json:"started_at,omitempty"`
	FinishedAt         *time.Time       `json:"finished_at,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

type AgentRunStep struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Target string `json:"target"`
	Status string `json:"status"`
}

type AgentRunChange struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status"`
}

type AgentRunLog struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	ProjectID string    `json:"project_id"`
	Sequence  int64     `json:"sequence"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
