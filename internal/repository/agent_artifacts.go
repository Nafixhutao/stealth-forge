package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const agentArtifactColumns = `id,project_id,agent_id,run_id,kind,title,version,mime_type,storage_path,size_bytes,created_at`

// insertAgentArtifactsTx persists the files a run produced inside the run's
// terminal transition, so a completed run can never point at artifacts that
// were rolled back. Versions are allocated per (agent, kind, mime_type).
func insertAgentArtifactsTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID,
	agentID,
	runID uuid.UUID,
	artifacts []domain.AgentArtifact,
) error {
	for index := range artifacts {
		item := &artifacts[index]
		item.Kind = strings.TrimSpace(item.Kind)
		item.Title = strings.TrimSpace(item.Title)
		item.MimeType = strings.TrimSpace(item.MimeType)
		item.StoragePath = strings.TrimSpace(item.StoragePath)
		if item.Kind == "" {
			item.Kind = "document"
		}
		if item.Title == "" || item.MimeType == "" || item.StoragePath == "" {
			return fmt.Errorf("%w: artifact %d is missing required metadata", ErrInvalidAgentRun, index)
		}
		artifactID := uuid.Must(uuid.NewV7())
		if raw := strings.TrimSpace(item.ID); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				return fmt.Errorf("%w: artifact %d id is invalid", ErrInvalidAgentRun, index)
			}
			artifactID = parsed
		}
		var version int
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM agent_artifacts
			WHERE agent_id = $1 AND kind = $2 AND mime_type = $3`,
			agentID, item.Kind, item.MimeType,
		).Scan(&version); err != nil {
			return err
		}
		if item.SizeBytes < 0 {
			item.SizeBytes = 0
		}
		item.ID = artifactID.String()
		item.ProjectID = projectID.String()
		item.AgentID = agentID.String()
		item.RunID = runID.String()
		item.Version = version
		if _, err := tx.Exec(ctx, `
			INSERT INTO agent_artifacts
				(id, project_id, agent_id, run_id, kind, title, version, mime_type, storage_path, size_bytes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			artifactID, projectID, agentID, runID, item.Kind, item.Title,
			version, item.MimeType, item.StoragePath, item.SizeBytes,
		); err != nil {
			return mapError(err)
		}
	}
	return nil
}

func scanAgentArtifact(row agentRunScanner) (domain.AgentArtifact, error) {
	var item domain.AgentArtifact
	err := row.Scan(
		&item.ID, &item.ProjectID, &item.AgentID, &item.RunID, &item.Kind,
		&item.Title, &item.Version, &item.MimeType, &item.StoragePath,
		&item.SizeBytes, &item.CreatedAt,
	)
	return item, err
}

// ListAgentArtifacts returns an agent's artifacts newest first. The caller must
// be a member of the project that owns the agent.
func (r *Repository) ListAgentArtifacts(
	ctx context.Context,
	accountID,
	agentID uuid.UUID,
	limit int,
	cursor string,
) ([]domain.AgentArtifact, string, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var projectID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT a.project_id
		FROM project_agents a
		JOIN projects p ON p.id = a.project_id
		JOIN organization_memberships m
		  ON m.organization_id = p.organization_id AND m.account_id = $2
		WHERE a.id = $1`, agentID, accountID).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}

	args := []any{projectID, agentID}
	query := `SELECT ` + agentArtifactColumns + `
		FROM agent_artifacts WHERE project_id = $1 AND agent_id = $2`
	if cursor != "" {
		cursorID, err := uuid.Parse(cursor)
		if err != nil {
			return nil, "", ErrInvalidAgentRun
		}
		args = append(args, cursorID)
		query += ` AND id < $3`
	}
	query += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]domain.AgentArtifact, 0, limit+1)
	for rows.Next() {
		item, err := scanAgentArtifact(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, nil
}

// AgentArtifactForDownload resolves one artifact and verifies the caller's
// project membership in the same query.
func (r *Repository) AgentArtifactForDownload(
	ctx context.Context,
	accountID,
	agentID,
	artifactID uuid.UUID,
) (domain.AgentArtifact, error) {
	item, err := scanAgentArtifact(r.pool.QueryRow(ctx, `
		SELECT a.id, a.project_id, a.agent_id, a.run_id, a.kind, a.title,
		       a.version, a.mime_type, a.storage_path, a.size_bytes, a.created_at
		FROM agent_artifacts a
		JOIN project_agents ag ON ag.id = a.agent_id AND ag.project_id = a.project_id
		JOIN projects p ON p.id = a.project_id
		JOIN organization_memberships m
		  ON m.organization_id = p.organization_id AND m.account_id = $3
		WHERE a.agent_id = $1 AND a.id = $2`, agentID, artifactID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentArtifact{}, ErrNotFound
	}
	return item, err
}
