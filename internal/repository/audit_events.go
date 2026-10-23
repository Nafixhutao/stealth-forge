package repository

import (
	"context"
	"encoding/json"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const auditEventProjection = `
	e.id::text,
	e.organization_id::text,
	e.actor_account_id::text,
	a.email,
	e.action,
	e.target_type,
	e.target_id::text,
	e.metadata,
	e.created_at`

type auditEventScanner interface {
	Scan(...any) error
}

func scanAuditEvent(row auditEventScanner) (domain.AuditEvent, error) {
	var item domain.AuditEvent
	var metadata []byte
	if err := row.Scan(
		&item.ID,
		&item.OrganizationID,
		&item.ActorAccountID,
		&item.ActorEmail,
		&item.Action,
		&item.TargetType,
		&item.TargetID,
		&metadata,
		&item.CreatedAt,
	); err != nil {
		return domain.AuditEvent{}, err
	}
	if len(metadata) == 0 || !json.Valid(metadata) {
		item.Metadata = json.RawMessage(`{}`)
	} else {
		item.Metadata = json.RawMessage(metadata)
	}
	return item, nil
}

func scanAuditEvents(rows pgx.Rows, limit int) ([]domain.AuditEvent, error) {
	items := make([]domain.AuditEvent, 0, limit)
	for rows.Next() {
		item, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAuditEvents returns the newest tenant audit events visible to a member.
// UUIDv7 event IDs provide a monotonic cursor for this descending projection;
// the API keeps the cursor opaque to callers while validating it as a UUID.
func (r *Repository) ListAuditEvents(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	limit int,
	cursor string,
) ([]domain.AuditEvent, string, error) {
	if err := r.requireMembership(ctx, organizationID, accountID); err != nil {
		return nil, "", err
	}
	return r.listAuditEvents(ctx, organizationID, accountID, "", limit, cursor)
}

// ListProjectAuditEvents returns only events that carry the project scope in
// their metadata (plus the project creation event, whose target is the
// project itself). Older events without that scope stay visible at the
// organization level but cannot be misattributed to a project.
func (r *Repository) ListProjectAuditEvents(
	ctx context.Context,
	projectID, accountID uuid.UUID,
	limit int,
	cursor string,
) ([]domain.AuditEvent, string, error) {
	organizationID, err := r.projectOrganizationForMember(ctx, projectID, accountID)
	if err != nil {
		return nil, "", err
	}
	return r.listAuditEvents(ctx, organizationID, accountID, projectID.String(), limit, cursor)
}

func (r *Repository) listAuditEvents(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	projectID string,
	limit int,
	cursor string,
) ([]domain.AuditEvent, string, error) {
	query := `
		SELECT ` + auditEventProjection + `
		FROM audit_events e
		LEFT JOIN accounts a ON a.id=e.actor_account_id
		WHERE e.organization_id=$1 AND ($2='' OR e.id::text<$2)`
	args := []any{organizationID, cursor, limit + 1}
	if projectID != "" {
		query += ` AND (e.metadata->>'project_id'=$4 OR (e.target_type='project' AND e.target_id::text=$4))`
		args = append(args, projectID)
	}
	query += `
		ORDER BY e.id DESC
		LIMIT $3`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	items, err := scanAuditEvents(rows, limit)
	if err != nil {
		return nil, "", err
	}

	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, nil
}
