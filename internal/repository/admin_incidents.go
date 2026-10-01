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

type AdminIncidentInput struct {
	Title    string
	Severity string
	Status   string
	Services []string
	Message  string
}

type AdminIncidentPatch struct {
	Title    *string
	Severity *string
	Status   *string
	Services *[]string
	Message  *string
}

type AdminIncidentEventInput struct {
	Kind    string
	Message string
}

func (r *Repository) ListAdminIncidents(ctx context.Context, limit int) ([]domain.AdminIncident, error) {
	if limit < 1 || limit > adminIncidentMaxLimit {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidAdminIncident, adminIncidentMaxLimit)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT i.id::text,i.title,i.severity,i.status,i.services,i.started_at,i.resolved_at,
		       i.created_by_account_id::text,i.created_at,i.updated_at
		FROM admin_incidents i ORDER BY i.started_at DESC,i.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AdminIncident, 0, limit)
	for rows.Next() {
		item, scanErr := scanAdminIncident(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AdminIncidentByID(ctx context.Context, id uuid.UUID) (domain.AdminIncident, error) {
	if id == uuid.Nil {
		return domain.AdminIncident{}, ErrNotFound
	}
	item, err := scanAdminIncident(r.pool.QueryRow(ctx, `
		SELECT i.id::text,i.title,i.severity,i.status,i.services,i.started_at,i.resolved_at,
		       i.created_by_account_id::text,i.created_at,i.updated_at
		FROM admin_incidents i WHERE i.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminIncident{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminIncident{}, err
	}
	if err := loadAdminIncidentEvents(ctx, r.pool, &item); err != nil {
		return domain.AdminIncident{}, err
	}
	return item, nil
}

func (r *Repository) CreateAdminIncident(ctx context.Context, accountID, id uuid.UUID, input AdminIncidentInput) (domain.AdminIncident, error) {
	normalized, err := normalizeAdminIncidentInput(input)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminIncident{}, err
	}
	var resolvedAt any
	if normalized.Status == "resolved" {
		resolvedAt = time.Now().UTC()
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO admin_incidents (id,title,severity,status,services,resolved_at,created_by_account_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, normalized.Title, normalized.Severity, normalized.Status, normalized.Services, resolvedAt, accountID)
	if err != nil {
		return domain.AdminIncident{}, mapError(err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return domain.AdminIncident{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_incident_events (id,incident_id,kind,message,actor_account_id) VALUES ($1,$2,'note',$3,$4)`, eventID, id, normalized.Message, accountID)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.incident.create", "admin_incident", id, map[string]any{"severity": normalized.Severity, "status": normalized.Status}); err != nil {
		return domain.AdminIncident{}, err
	}
	item, err := scanAdminIncident(tx.QueryRow(ctx, `
		SELECT i.id::text,i.title,i.severity,i.status,i.services,i.started_at,i.resolved_at,
		       i.created_by_account_id::text,i.created_at,i.updated_at FROM admin_incidents i WHERE i.id=$1`, id))
	if err != nil {
		return domain.AdminIncident{}, err
	}
	if err := loadAdminIncidentEvents(ctx, tx, &item); err != nil {
		return domain.AdminIncident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminIncident{}, err
	}
	return item, nil
}

func (r *Repository) UpdateAdminIncident(ctx context.Context, accountID, id uuid.UUID, patch AdminIncidentPatch) (domain.AdminIncident, error) {
	normalized, err := normalizeAdminIncidentPatch(patch)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminIncident{}, err
	}
	item, err := scanAdminIncident(tx.QueryRow(ctx, `
		SELECT i.id::text,i.title,i.severity,i.status,i.services,i.started_at,i.resolved_at,
		       i.created_by_account_id::text,i.created_at,i.updated_at FROM admin_incidents i WHERE i.id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminIncident{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminIncident{}, err
	}
	title := item.Title
	if normalized.Title != nil {
		title = *normalized.Title
	}
	severity := item.Severity
	if normalized.Severity != nil {
		severity = *normalized.Severity
	}
	status := item.Status
	if normalized.Status != nil {
		status = *normalized.Status
	}
	services := item.Services
	if normalized.Services != nil {
		services = *normalized.Services
	}
	message := "Incident updated from the admin console."
	if normalized.Message != nil && *normalized.Message != "" {
		message = *normalized.Message
	}
	var resolvedAt any
	if status == "resolved" {
		resolvedAt = time.Now().UTC()
	}
	_, err = tx.Exec(ctx, `UPDATE admin_incidents SET title=$2,severity=$3,status=$4,services=$5,resolved_at=$6,updated_at=now() WHERE id=$1`, id, title, severity, status, services, resolvedAt)
	if err != nil {
		return domain.AdminIncident{}, mapError(err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return domain.AdminIncident{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_incident_events (id,incident_id,kind,message,actor_account_id) VALUES ($1,$2,'configuration',$3,$4)`, eventID, id, message, accountID)
	if err != nil {
		return domain.AdminIncident{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.incident.update", "admin_incident", id, map[string]any{"status": status}); err != nil {
		return domain.AdminIncident{}, err
	}
	item, err = scanAdminIncident(tx.QueryRow(ctx, `SELECT i.id::text,i.title,i.severity,i.status,i.services,i.started_at,i.resolved_at,i.created_by_account_id::text,i.created_at,i.updated_at FROM admin_incidents i WHERE i.id=$1`, id))
	if err != nil {
		return domain.AdminIncident{}, err
	}
	if err := loadAdminIncidentEvents(ctx, tx, &item); err != nil {
		return domain.AdminIncident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminIncident{}, err
	}
	return item, nil
}

func (r *Repository) AddAdminIncidentEvent(ctx context.Context, accountID, incidentID, eventID uuid.UUID, input AdminIncidentEventInput) (domain.AdminIncidentEvent, error) {
	kind, message, err := normalizeAdminIncidentEvent(input)
	if err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM admin_incidents WHERE id=$1)`, incidentID).Scan(&exists); err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	if !exists {
		return domain.AdminIncidentEvent{}, ErrNotFound
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_incident_events (id,incident_id,kind,message,actor_account_id) VALUES ($1,$2,$3,$4,$5)`, eventID, incidentID, kind, message, accountID)
	if err != nil {
		return domain.AdminIncidentEvent{}, mapError(err)
	}
	_, err = tx.Exec(ctx, `UPDATE admin_incidents SET updated_at=now() WHERE id=$1`, incidentID)
	if err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.incident.note", "admin_incident", incidentID, map[string]any{"kind": kind}); err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	var item domain.AdminIncidentEvent
	err = tx.QueryRow(ctx, `SELECT id::text,incident_id::text,kind,message,actor_account_id::text,created_at FROM admin_incident_events WHERE id=$1`, eventID).Scan(&item.ID, &item.IncidentID, &item.Kind, &item.Message, &item.ActorAccountID, &item.CreatedAt)
	if err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminIncidentEvent{}, err
	}
	return item, nil
}

func scanAdminIncident(row interface{ Scan(...any) error }) (domain.AdminIncident, error) {
	var item domain.AdminIncident
	if err := row.Scan(&item.ID, &item.Title, &item.Severity, &item.Status, &item.Services, &item.StartedAt, &item.ResolvedAt, &item.CreatedByAccountID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.AdminIncident{}, err
	}
	if item.Services == nil {
		item.Services = []string{}
	}
	item.Events = []domain.AdminIncidentEvent{}
	return item, nil
}

func loadAdminIncidentEvents(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, item *domain.AdminIncident) error {
	rows, err := queryer.Query(ctx, `SELECT id::text,incident_id::text,kind,message,actor_account_id::text,created_at FROM admin_incident_events WHERE incident_id=$1 ORDER BY created_at ASC,id ASC`, uuid.MustParse(item.ID))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var event domain.AdminIncidentEvent
		if err := rows.Scan(&event.ID, &event.IncidentID, &event.Kind, &event.Message, &event.ActorAccountID, &event.CreatedAt); err != nil {
			return err
		}
		item.Events = append(item.Events, event)
	}
	return rows.Err()
}

func normalizeAdminIncidentInput(input AdminIncidentInput) (AdminIncidentInput, error) {
	title, err := normalizeAdminControlText(input.Title, 3, 240)
	if err != nil {
		return AdminIncidentInput{}, fmt.Errorf("%w: title is invalid", ErrInvalidAdminIncident)
	}
	severity, err := normalizeAdminSeverity(input.Severity)
	if err != nil {
		return AdminIncidentInput{}, ErrInvalidAdminIncident
	}
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "investigating"
	}
	if !validAdminIncidentStatus(status) {
		return AdminIncidentInput{}, ErrInvalidAdminIncident
	}
	services, err := normalizeAdminIncidentServices(input.Services)
	if err != nil {
		return AdminIncidentInput{}, err
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = "Incident opened manually from the admin console."
	}
	if !validAdminControlText(message, 1, 2000) {
		return AdminIncidentInput{}, ErrInvalidAdminIncident
	}
	return AdminIncidentInput{Title: title, Severity: severity, Status: status, Services: services, Message: message}, nil
}

func normalizeAdminIncidentPatch(patch AdminIncidentPatch) (AdminIncidentPatch, error) {
	if patch.Title == nil && patch.Severity == nil && patch.Status == nil && patch.Services == nil && patch.Message == nil {
		return AdminIncidentPatch{}, ErrInvalidAdminIncident
	}
	if patch.Title != nil {
		value, err := normalizeAdminControlText(*patch.Title, 3, 240)
		if err != nil {
			return AdminIncidentPatch{}, ErrInvalidAdminIncident
		}
		patch.Title = &value
	}
	if patch.Severity != nil {
		value, err := normalizeAdminSeverity(*patch.Severity)
		if err != nil {
			return AdminIncidentPatch{}, ErrInvalidAdminIncident
		}
		patch.Severity = &value
	}
	if patch.Status != nil {
		value := strings.ToLower(strings.TrimSpace(*patch.Status))
		if !validAdminIncidentStatus(value) {
			return AdminIncidentPatch{}, ErrInvalidAdminIncident
		}
		patch.Status = &value
	}
	if patch.Services != nil {
		value, err := normalizeAdminIncidentServices(*patch.Services)
		if err != nil {
			return AdminIncidentPatch{}, err
		}
		patch.Services = &value
	}
	if patch.Message != nil && *patch.Message != "" && !validAdminControlText(*patch.Message, 1, 2000) {
		return AdminIncidentPatch{}, ErrInvalidAdminIncident
	}
	return patch, nil
}

func normalizeAdminIncidentEvent(input AdminIncidentEventInput) (string, string, error) {
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	valid := map[string]bool{"alert": true, "deployment": true, "restart": true, "backup": true, "configuration": true, "monitor": true, "note": true}
	if !valid[kind] || !validAdminControlText(input.Message, 1, 2000) {
		return "", "", ErrInvalidAdminIncident
	}
	return kind, strings.TrimSpace(input.Message), nil
}

func normalizeAdminIncidentServices(values []string) ([]string, error) {
	if len(values) < 1 || len(values) > adminIncidentMaxServices {
		return nil, ErrInvalidAdminIncident
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validAdminControlText(value, 1, 128) {
			return nil, ErrInvalidAdminIncident
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return nil, ErrInvalidAdminIncident
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validAdminIncidentStatus(value string) bool {
	switch value {
	case "investigating", "identified", "monitoring", "resolved":
		return true
	default:
		return false
	}
}
