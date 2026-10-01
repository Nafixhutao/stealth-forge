package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ClaimNextAgentRun(ctx context.Context, workerID string) (AgentRunJob, error) {
	return r.ClaimNextAgentRunForProviders(ctx, workerID, nil)
}

// ClaimNextAgentRunForProviders atomically leases one queued run whose
// provider is present in allowedProviders. A nil provider list means all
// providers (the legacy control-plane primitive); an empty list claims
// nothing. Comparing normalized lowercase values keeps existing agent rows
// written with display-case provider names compatible with workers.
func (r *Repository) ClaimNextAgentRunForProviders(ctx context.Context, workerID string, allowedProviders []string) (AgentRunJob, error) {
	workerID, err := normalizeAgentRunWorkerID(workerID)
	if err != nil {
		return AgentRunJob{}, err
	}
	var providerArg any
	if allowedProviders != nil {
		normalizedProviders := make([]string, 0, len(allowedProviders))
		seen := make(map[string]struct{}, len(allowedProviders))
		for _, provider := range allowedProviders {
			provider = strings.ToLower(strings.TrimSpace(provider))
			if provider == "" || strings.ContainsAny(provider, "\x00\r\n\t") {
				return AgentRunJob{}, fmt.Errorf("%w: provider filter is invalid", ErrInvalidAgentRun)
			}
			if _, exists := seen[provider]; exists {
				continue
			}
			seen[provider] = struct{}{}
			normalizedProviders = append(normalizedProviders, provider)
		}
		providerArg = normalizedProviders
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AgentRunJob{}, err
	}
	defer tx.Rollback(ctx)
	var runID, agentID, projectID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT r.id,r.agent_id,r.project_id
		FROM agent_runs r
		JOIN project_agents a ON a.id=r.agent_id AND a.project_id=r.project_id
		WHERE r.status='queued'
		  AND ($1::text[] IS NULL OR lower(a.provider)=ANY($1::text[]))
		ORDER BY r.queued_at,r.id
		FOR UPDATE OF r SKIP LOCKED
		LIMIT 1`, providerArg).Scan(&runID, &agentID, &projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRunJob{}, ErrNoAgentRunJob
	}
	if err != nil {
		return AgentRunJob{}, err
	}
	run, err := scanAgentRun(tx.QueryRow(ctx, `
		UPDATE agent_runs
		SET status='running',started_at=COALESCE(started_at,now()),claimed_at=now(),worker_id=$4,updated_at=now()
		WHERE id=$1 AND agent_id=$2 AND project_id=$3 AND status='queued'
		RETURNING `+agentRunReturningProjection, runID, agentID, projectID, workerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRunJob{}, ErrNoAgentRunJob
	}
	if err != nil {
		return AgentRunJob{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "agent.run.running", "agent_run", runID, map[string]any{"agent_id": agentID.String(), "status": run.Status}); err != nil {
		return AgentRunJob{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_agents SET status='running',last_active_at=now(),updated_at=now() WHERE id=$1 AND project_id=$2`, agentID, projectID); err != nil {
		return AgentRunJob{}, err
	}
	agent, err := scanAgent(tx.QueryRow(ctx, `SELECT `+agentProjection+` FROM project_agents a JOIN projects p ON p.id=a.project_id WHERE a.id=$1 AND a.project_id=$2`, agentID, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRunJob{}, ErrAgentRunNotAvailable
	}
	if err != nil {
		return AgentRunJob{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AgentRunJob{}, err
	}
	return AgentRunJob{Run: run, Agent: agent}, nil
}

// TransitionAgentRun persists a worker result while fencing stale workers by
// worker_id. Only a claimed running run may become terminal.
func (r *Repository) TransitionAgentRun(ctx context.Context, projectID, agentID, runID uuid.UUID, workerID string, result AgentRunResult) (domain.AgentRun, error) {
	workerID, err := normalizeAgentRunWorkerID(workerID)
	if err != nil {
		return domain.AgentRun{}, err
	}
	result, stepsJSON, changesJSON, err := normalizeAgentRunResult(result)
	if err != nil {
		return domain.AgentRun{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AgentRun{}, err
	}
	defer tx.Rollback(ctx)
	var currentStatus, currentWorker string
	err = tx.QueryRow(ctx, `SELECT status,COALESCE(worker_id,'') FROM agent_runs WHERE project_id=$1 AND agent_id=$2 AND id=$3 FOR UPDATE`, projectID, agentID, runID).Scan(&currentStatus, &currentWorker)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentRun{}, ErrNotFound
	}
	if err != nil {
		return domain.AgentRun{}, err
	}
	if currentStatus != "running" || currentWorker != workerID {
		return domain.AgentRun{}, ErrAgentRunNotAvailable
	}
	run, err := scanAgentRun(tx.QueryRow(ctx, `
		UPDATE agent_runs
		SET status=$4,output_text=$5,error_message=$6,steps=$7,changes=$8,finished_at=now(),claimed_at=NULL,worker_id=NULL,updated_at=now()
		WHERE project_id=$1 AND agent_id=$2 AND id=$3 AND status='running'
		RETURNING `+agentRunReturningProjection, projectID, agentID, runID, result.Status, result.OutputText, result.ErrorMessage, stepsJSON, changesJSON))
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
	metadata := map[string]any{"project_id": projectID.String(), "agent_id": agentID.String(), "status": run.Status}
	if err := writeAuditMetadata(ctx, tx, orgID, uuid.Nil, "agent.run."+run.Status, "agent_run", runID, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := r.enqueueWebhookEventTx(ctx, tx, projectID, "agent.run."+run.Status, "agent_run", runID, metadata); err != nil {
		return domain.AgentRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AgentRun{}, err
	}
	return run, nil
}

func (r *Repository) AppendAgentRunLog(ctx context.Context, projectID, agentID, runID uuid.UUID, workerID string, id uuid.UUID, sequence int64, level, message string) (domain.AgentRunLog, error) {
	workerID, err := normalizeAgentRunWorkerID(workerID)
	if err != nil {
		return domain.AgentRunLog{}, err
	}
	level = strings.ToLower(strings.TrimSpace(level))
	message = strings.TrimSpace(message)
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		return domain.AgentRunLog{}, fmt.Errorf("%w: log level is invalid", ErrInvalidAgentRun)
	}
	if message == "" || len(message) > agentRunMaxLogBytes || strings.ContainsRune(message, '\x00') {
		return domain.AgentRunLog{}, fmt.Errorf("%w: log message is invalid", ErrInvalidAgentRun)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AgentRunLog{}, err
	}
	defer tx.Rollback(ctx)
	var currentWorker string
	err = tx.QueryRow(ctx, `SELECT COALESCE(worker_id,'') FROM agent_runs WHERE project_id=$1 AND agent_id=$2 AND id=$3 AND status='running' FOR UPDATE`, projectID, agentID, runID).Scan(&currentWorker)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AgentRunLog{}, ErrAgentRunNotAvailable
	}
	if err != nil {
		return domain.AgentRunLog{}, err
	}
	if currentWorker != workerID {
		return domain.AgentRunLog{}, ErrAgentRunNotAvailable
	}
	if sequence <= 0 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM agent_run_logs WHERE project_id=$1 AND run_id=$2`, projectID, runID).Scan(&sequence); err != nil {
			return domain.AgentRunLog{}, err
		}
	}
	item, err := scanAgentRunLog(tx.QueryRow(ctx, `
		INSERT INTO agent_run_logs (id,run_id,project_id,sequence,level,message)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id,run_id,project_id,sequence,level,message,created_at`, id, runID, projectID, sequence, level, message))
	if err != nil {
		return domain.AgentRunLog{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AgentRunLog{}, err
	}
	return item, nil
}

func (r *Repository) ListAgentRunLogs(ctx context.Context, accountID, agentID, runID uuid.UUID, limit int, after int64) ([]domain.AgentRunLog, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 100", ErrInvalidAgentRun)
	}
	if after < 0 {
		return nil, fmt.Errorf("%w: after must be non-negative", ErrInvalidAgentRun)
	}
	if _, err := r.AgentRunByID(ctx, accountID, agentID, runID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+agentRunLogProjection+` FROM agent_run_logs l WHERE l.project_id=(SELECT project_id FROM agent_runs WHERE id=$1 AND agent_id=$2) AND l.run_id=$1 AND l.sequence>$3 ORDER BY l.sequence LIMIT $4`, runID, agentID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AgentRunLog, 0, limit)
	for rows.Next() {
		item, scanErr := scanAgentRunLog(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) RequeueStaleAgentRuns(ctx context.Context, maxAge time.Duration) (int64, error) {
	if maxAge <= 0 {
		return 0, fmt.Errorf("%w: max age must be positive", ErrInvalidAgentRun)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE agent_runs SET status='queued',started_at=NULL,claimed_at=NULL,worker_id=NULL,updated_at=now() WHERE status='running' AND claimed_at IS NOT NULL AND claimed_at < now() - ($1::double precision * interval '1 second') RETURNING id,agent_id,project_id`, maxAge.Seconds())
	if err != nil {
		return 0, err
	}
	type agentProject struct{ runID, agentID, projectID uuid.UUID }
	changed := make([]agentProject, 0)
	for rows.Next() {
		var value agentProject
		if err := rows.Scan(&value.runID, &value.agentID, &value.projectID); err != nil {
			rows.Close()
			return 0, err
		}
		changed = append(changed, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	seen := make(map[uuid.UUID]struct{}, len(changed))
	for _, value := range changed {
		if _, ok := seen[value.agentID]; ok {
			continue
		}
		seen[value.agentID] = struct{}{}
		if err := r.refreshAgentStatusTx(ctx, tx, value.agentID, value.projectID); err != nil {
			return 0, err
		}
	}
	for _, value := range changed {
		if err := r.enqueueWebhookEventTx(ctx, tx, value.projectID, "agent.run.queued", "agent_run", value.runID, map[string]any{
			"agent_id":  value.agentID.String(),
			"status":    "queued",
			"recovered": true,
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(changed)), nil
}

func (r *Repository) refreshAgentStatusTx(ctx context.Context, tx pgx.Tx, agentID, projectID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE project_agents
		SET status=CASE
			WHEN EXISTS (SELECT 1 FROM agent_runs WHERE agent_id=$1 AND project_id=$2 AND status='running') THEN 'running'
			WHEN EXISTS (SELECT 1 FROM agent_runs WHERE agent_id=$1 AND project_id=$2 AND status='queued') THEN 'active'
			ELSE 'idle'
		END,
		last_active_at=now(),updated_at=now()
		WHERE id=$1 AND project_id=$2`, agentID, projectID)
	return err
}
