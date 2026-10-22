package repository

// Function execution logs are worker-appended and actor-read. They are kept
// apart from execution lifecycle so the lease and transition code stays
// readable, mirroring functions_deployment_lifecycle.go for build logs.

import (
	"context"
	"errors"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) AppendFunctionExecutionLog(
	ctx context.Context,
	projectID, functionID, executionID, id uuid.UUID,
	sequence int64,
	level, message string,
) (domain.FunctionExecutionLog, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.FunctionExecutionLog{}, err
	}
	defer tx.Rollback(ctx)
	_, err = scanFunctionExecution(
		tx.QueryRow(
			ctx,
			`SELECT `+functionExecutionProjection+` FROM function_executions WHERE project_id=$1 AND function_id=$2 AND id=$3 FOR UPDATE`,
			projectID,
			functionID,
			executionID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FunctionExecutionLog{}, ErrNotFound
	}
	if err != nil {
		return domain.FunctionExecutionLog{}, err
	}
	if sequence <= 0 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM function_execution_logs WHERE project_id=$1 AND execution_id=$2`, projectID, executionID).Scan(&sequence); err != nil {
			return domain.FunctionExecutionLog{}, err
		}
	}
	item, err := scanFunctionExecutionLog(
		tx.QueryRow(
			ctx,
			`INSERT INTO function_execution_logs (id,execution_id,function_id,project_id,sequence,level,message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+functionExecutionLogProjection,
			id,
			executionID,
			functionID,
			projectID,
			sequence,
			level,
			message,
		),
	)
	if err != nil {
		return domain.FunctionExecutionLog{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.FunctionExecutionLog{}, err
	}
	return item, nil
}

func (r *Repository) ListFunctionExecutionLogs(
	ctx context.Context,
	projectID, functionID, executionID uuid.UUID,
	actor FunctionActor,
	limit int,
	after int64,
) ([]domain.FunctionExecutionLog, error) {
	if _, err := r.requireFunctionRead(ctx, projectID, actor); err != nil {
		return nil, err
	}
	if _, err := r.functionByID(ctx, r.pool, projectID, functionID, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+functionExecutionLogProjection+` FROM function_execution_logs WHERE project_id=$1 AND function_id=$2 AND execution_id=$3 AND sequence>$4 ORDER BY sequence LIMIT $5`,
		projectID,
		functionID,
		executionID,
		after,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.FunctionExecutionLog, 0, limit)
	for rows.Next() {
		item, scanErr := scanFunctionExecutionLog(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
