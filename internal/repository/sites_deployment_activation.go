package repository

// Site deployment activation promotes a built deployment to the active pointer
// under the same transaction that records the audit trail.

import (
	"context"
	"errors"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func activateSiteDeploymentTx(
	ctx context.Context,
	tx pgx.Tx,
	projectID, siteID, deploymentID uuid.UUID,
	item domain.SiteDeployment,
) (domain.SiteDeployment, error) {
	var current *uuid.UUID
	var status string
	if err := tx.QueryRow(ctx, `SELECT active_deployment_id,status FROM project_sites WHERE project_id=$1 AND id=$2 FOR UPDATE`, projectID, siteID).Scan(&current, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.SiteDeployment{}, ErrNotFound
		}
		return domain.SiteDeployment{}, err
	}
	if status != "active" {
		return domain.SiteDeployment{}, ErrSiteDisabled
	}
	locked, _, err := (&Repository{pool: nil}).siteDeploymentByID(ctx, tx, projectID, siteID, deploymentID, true, true)
	if err != nil {
		return domain.SiteDeployment{}, err
	}
	if locked.Status == "active" && current != nil && *current == deploymentID {
		return locked, nil
	}
	if locked.Status != "ready" || locked.BuildStatus != "succeeded" {
		return domain.SiteDeployment{}, ErrInvalidSiteTransition
	}
	if current != nil && *current != deploymentID {
		if _, err := tx.Exec(ctx, `UPDATE site_deployments SET status='superseded',finished_at=now(),updated_at=now() WHERE project_id=$1 AND site_id=$2 AND id=$3 AND status='active'`, projectID, siteID, *current); err != nil {
			return domain.SiteDeployment{}, err
		}
	}
	updated, err := scanSiteDeploymentPublic(
		tx.QueryRow(
			ctx,
			`UPDATE site_deployments SET status='active',activated_at=COALESCE(activated_at,now()),updated_at=now() WHERE project_id=$1 AND site_id=$2 AND id=$3 RETURNING `+siteDeploymentProjection,
			projectID,
			siteID,
			deploymentID,
		),
	)
	if err != nil {
		return domain.SiteDeployment{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_sites SET active_deployment_id=$3,updated_at=now() WHERE project_id=$1 AND id=$2`, projectID, siteID, deploymentID); err != nil {
		return domain.SiteDeployment{}, err
	}
	_ = item
	return updated, nil
}

func (r *Repository) ActivateSiteDeployment(
	ctx context.Context,
	projectID, siteID, deploymentID uuid.UUID,
	actor SiteActor,
) (domain.SiteDeployment, domain.Site, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	defer tx.Rollback(ctx)
	if err := r.requireSiteWriteTx(ctx, tx, projectID, actor); err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	item, _, err := r.siteDeploymentByID(ctx, tx, projectID, siteID, deploymentID, false, true)
	if err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	item, err = activateSiteDeploymentTx(ctx, tx, projectID, siteID, deploymentID, item)
	if err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	site, err := r.siteByID(ctx, tx, projectID, siteID, false)
	if err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	if err := r.auditSite(ctx, tx, projectID, actor, "site_deployment.activate", "site_deployment", deploymentID, map[string]any{"site_id": siteID.String(), "version": item.Version}); err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.SiteDeployment{}, domain.Site{}, err
	}
	return item, site, nil
}
