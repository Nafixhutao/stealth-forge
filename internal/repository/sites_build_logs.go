package repository

// Site build logs are worker-appended and actor-read. They are kept apart from
// deployment leases and activation, mirroring the Functions build-log file.

import (
	"context"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
)

// AppendSiteBuildLog appends one lifecycle message. A non-positive sequence
// asks the repository to allocate the next sequence while locking the
// deployment, which keeps concurrent workers and retries ordered.
func (r *Repository) AppendSiteBuildLog(
	ctx context.Context,
	projectID, siteID, deploymentID, id uuid.UUID,
	sequence int64,
	level, message string,
) (domain.SiteBuildLog, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.SiteBuildLog{}, err
	}
	defer tx.Rollback(ctx)
	if sequence <= 0 {
		if _, _, err := r.siteDeploymentByID(ctx, tx, projectID, siteID, deploymentID, true, false); err != nil {
			return domain.SiteBuildLog{}, err
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM site_build_logs WHERE project_id=$1 AND site_id=$2 AND deployment_id=$3`, projectID, siteID, deploymentID).Scan(&sequence); err != nil {
			return domain.SiteBuildLog{}, err
		}
	} else if _, _, err := r.siteDeploymentByID(ctx, tx, projectID, siteID, deploymentID, false, false); err != nil {
		return domain.SiteBuildLog{}, err
	}
	level = strings.ToLower(strings.TrimSpace(level))
	message = normalizeSiteBuildLogMessage(message)
	item, err := scanSiteBuildLog(
		tx.QueryRow(
			ctx,
			`INSERT INTO site_build_logs (id,deployment_id,site_id,project_id,sequence,level,message) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+siteBuildLogProjection,
			id,
			deploymentID,
			siteID,
			projectID,
			sequence,
			level,
			message,
		),
	)
	if err != nil {
		return domain.SiteBuildLog{}, mapError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SiteBuildLog{}, err
	}
	return item, nil
}

// ListSiteBuildLogs returns only logs for the requested tenant/site/deployment
// tuple. The sequence cursor is stable while a worker appends new messages.
func (r *Repository) ListSiteBuildLogs(
	ctx context.Context,
	projectID, siteID, deploymentID uuid.UUID,
	actor SiteActor,
	limit int,
	after int64,
) ([]domain.SiteBuildLog, error) {
	if _, err := r.requireSiteRead(ctx, projectID, actor); err != nil {
		return nil, err
	}
	if _, err := r.siteByID(ctx, r.pool, projectID, siteID, false); err != nil {
		return nil, err
	}
	if _, _, err := r.siteDeploymentByID(ctx, r.pool, projectID, siteID, deploymentID, false, false); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(
		ctx,
		`SELECT `+siteBuildLogProjection+` FROM site_build_logs WHERE project_id=$1 AND site_id=$2 AND deployment_id=$3 AND sequence>$4 ORDER BY sequence LIMIT $5`,
		projectID,
		siteID,
		deploymentID,
		after,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.SiteBuildLog, 0, limit)
	for rows.Next() {
		item, scanErr := scanSiteBuildLog(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
