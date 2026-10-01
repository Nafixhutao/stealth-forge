package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ListAgentRuns(ctx context.Context, accountID, agentID uuid.UUID, limit int, cursor *uuid.UUID) ([]domain.AgentRun, string, bool, error) {
	if limit < 1 || limit > 100 {
		return nil, "", false, fmt.Errorf("%w: limit must be between 1 and 100", ErrInvalidAgentRun)
	}
	agent, err := r.AgentByID(ctx, accountID, agentID)
	if err != nil {
		return nil, "", false, err
	}
	projectID, err := uuid.Parse(agent.ProjectID)
	if err != nil {
		return nil, "", false, fmt.Errorf("parse Agent project id: %w", err)
	}
	canManage, err := r.AgentProjectCanManage(ctx, accountID, projectID)
	if err != nil {
		return nil, "", false, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+agentRunProjection+`
		FROM agent_runs r
		JOIN project_agents a ON a.id=r.agent_id AND a.project_id=r.project_id
		JOIN projects p ON p.id=r.project_id
		JOIN organization_memberships m ON m.organization_id=p.organization_id
		WHERE r.agent_id=$1 AND m.account_id=$2 AND ($3::uuid IS NULL OR r.id<$3)
		ORDER BY r.id DESC
		LIMIT $4`, agentID, accountID, cursor, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	items := make([]domain.AgentRun, 0, limit)
	for rows.Next() {
		item, scanErr := scanAgentRun(rows)
		if scanErr != nil {
			return nil, "", false, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, canManage, nil
}

func (r *Repository) AgentRunByID(ctx context.Context, accountID, agentID, runID uuid.UUID) (domain.AgentRun, error) {
	item, err := scanAgentRun(r.pool.QueryRow(ctx, `
		SELECT `+agentRunProjection+`
		FROM agent_runs r
		JOIN project_agents a ON a.id=r.agent_id AND a.project_id=r.project_id
		JOIN projects p ON p.id=r.project_id
		JOIN organization_memberships m ON m.organization_id=p.organization_id
		WHERE r.agent_id=$1 AND r.id=$2 AND m.account_id=$3`, agentID, runID, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentRun{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateAgentRun(ctx context.Context, id, accountID, agentID uuid.UUID, input AgentRunInput) (domain.AgentRun, error) {
	prompt, err := normalizeAgentRunPrompt(input.Prompt)
	if err != nil {
		return domain.AgentRun{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AgentRun{}, err
	}
	defer tx.Rollback(ctx)
	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT project_id FROM project_agents WHERE id=$1 FOR UPDATE`, agentID).Scan(&projectID); errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentRun{}, ErrNotFound
	} else if err != nil {
		return domain.AgentRun{}, err
	}
	if err := requireProjectRoleTx(ctx, tx, projectID, accountID, "owner", "admin"); err != nil {
		return domain.AgentRun{}, err
	}
	item, err := scanAgentRun(tx.QueryRow(ctx, `
		INSERT INTO agent_runs (id,agent_id,project_id,created_by_account_id,prompt)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING `+agentRunReturningProjection, id, agentID, projectID, accountID, prompt))
	if err != nil {
		return domain.AgentRun{}, mapError(err)
	}
	if err := r.refreshAgentStatusTx(ctx, tx, agentID, projectID); err != nil {
		return domain.AgentRun{}, err
	}
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	metadata := map[string]any{"project_id": projectID.String(), "agent_id": agentID.String(), "status": item.Status}
	if err := writeAuditMetadata(ctx, tx, orgID, accountID, "agent.run.accepted", "agent_run", id, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "agent.run.accepted", "agent_run", id, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AgentRun{}, err
	}
	return item, nil
}

func (r *Repository) CancelAgentRun(ctx context.Context, accountID, agentID, runID uuid.UUID) (domain.AgentRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AgentRun{}, err
	}
	defer tx.Rollback(ctx)
	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT project_id FROM agent_runs WHERE agent_id=$1 AND id=$2 FOR UPDATE`, agentID, runID).Scan(&projectID); errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentRun{}, ErrNotFound
	} else if err != nil {
		return domain.AgentRun{}, err
	}
	if err := requireProjectRoleTx(ctx, tx, projectID, accountID, "owner", "admin"); err != nil {
		return domain.AgentRun{}, err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM agent_runs WHERE agent_id=$1 AND id=$2`, agentID, runID).Scan(&status); err != nil {
		return domain.AgentRun{}, err
	}
	if status != "queued" && status != "running" {
		return domain.AgentRun{}, ErrInvalidAgentRunTransition
	}
	item, err := scanAgentRun(tx.QueryRow(ctx, `
		UPDATE agent_runs
		SET status='cancelled',finished_at=now(),claimed_at=NULL,worker_id=NULL,updated_at=now()
		WHERE agent_id=$1 AND id=$2
		RETURNING `+agentRunReturningProjection, agentID, runID))
	if err != nil {
		return domain.AgentRun{}, err
	}
	if err := r.refreshAgentStatusTx(ctx, tx, agentID, projectID); err != nil {
		return domain.AgentRun{}, err
	}
	orgID, err := projectOrganizationIDValue(ctx, tx, projectID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	metadata := map[string]any{"project_id": projectID.String(), "agent_id": agentID.String(), "status": item.Status}
	if err := writeAuditMetadata(ctx, tx, orgID, accountID, "agent.run.cancelled", "agent_run", runID, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "agent.run.cancelled", "agent_run", runID, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AgentRun{}, err
	}
	return item, nil
}

// ClaimNextAgentRun atomically leases one queued run. It is intentionally
// separate from the HTTP API so a future provider worker can use the same
// queue without exposing worker credentials or filesystem details.
