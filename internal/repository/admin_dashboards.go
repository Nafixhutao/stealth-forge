package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AdminDashboardInput struct {
	Name        string
	Description string
	Definition  json.RawMessage
}

func (r *Repository) ListAdminDashboards(ctx context.Context, limit int) ([]domain.AdminDashboard, error) {
	if limit < 1 || limit > adminDashboardMaxLimit {
		return nil, ErrInvalidAdminDashboard
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text,name,description,definition,created_by_account_id::text,created_at,updated_at FROM admin_dashboards ORDER BY updated_at DESC,id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AdminDashboard, 0, limit)
	for rows.Next() {
		item, scanErr := scanAdminDashboard(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AdminDashboardByID(ctx context.Context, id uuid.UUID) (domain.AdminDashboard, error) {
	if id == uuid.Nil {
		return domain.AdminDashboard{}, ErrNotFound
	}
	item, err := scanAdminDashboard(r.pool.QueryRow(ctx, `SELECT id::text,name,description,definition,created_by_account_id::text,created_at,updated_at FROM admin_dashboards WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminDashboard{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateAdminDashboard(ctx context.Context, accountID, id uuid.UUID, input AdminDashboardInput) (domain.AdminDashboard, error) {
	normalized, err := normalizeAdminDashboardInput(input)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminDashboard{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_dashboards (id,name,description,definition,created_by_account_id) VALUES ($1,$2,$3,$4,$5)`, id, normalized.Name, normalized.Description, normalized.Definition, accountID); err != nil {
		return domain.AdminDashboard{}, mapError(err)
	}
	item, err := scanAdminDashboard(tx.QueryRow(ctx, `SELECT id::text,name,description,definition,created_by_account_id::text,created_at,updated_at FROM admin_dashboards WHERE id=$1`, id))
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.dashboard.create", "admin_dashboard", id, map[string]any{}); err != nil {
		return domain.AdminDashboard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminDashboard{}, err
	}
	return item, nil
}

func (r *Repository) UpdateAdminDashboard(ctx context.Context, accountID, id uuid.UUID, input AdminDashboardInput) (domain.AdminDashboard, error) {
	normalized, err := normalizeAdminDashboardInput(input)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminDashboard{}, err
	}
	var lockedID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM admin_dashboards WHERE id=$1 FOR UPDATE`, id).Scan(&lockedID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminDashboard{}, err
	}
	if lockedID == uuid.Nil {
		return domain.AdminDashboard{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_dashboards SET name=$2,description=$3,definition=$4,updated_at=now() WHERE id=$1`, id, normalized.Name, normalized.Description, normalized.Definition); err != nil {
		return domain.AdminDashboard{}, mapError(err)
	}
	item, err := scanAdminDashboard(tx.QueryRow(ctx, `SELECT id::text,name,description,definition,created_by_account_id::text,created_at,updated_at FROM admin_dashboards WHERE id=$1`, id))
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.dashboard.update", "admin_dashboard", id, map[string]any{}); err != nil {
		return domain.AdminDashboard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminDashboard{}, err
	}
	return item, nil
}

func (r *Repository) DeleteAdminDashboard(ctx context.Context, accountID, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM admin_dashboards WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.dashboard.delete", "admin_dashboard", id, map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanAdminDashboard(row interface{ Scan(...any) error }) (domain.AdminDashboard, error) {
	var item domain.AdminDashboard
	var definition []byte
	if err := row.Scan(&item.ID, &item.Name, &item.Description, &definition, &item.CreatedByAccountID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.AdminDashboard{}, err
	}
	if !json.Valid(definition) || json.Unmarshal(definition, &item.Definition) != nil {
		return domain.AdminDashboard{}, ErrInvalidAdminDashboard
	}
	if item.Definition == nil {
		item.Definition = map[string]any{}
	}
	return item, nil
}

func normalizeAdminDashboardInput(input AdminDashboardInput) (AdminDashboardInput, error) {
	name, err := normalizeAdminControlText(input.Name, 1, 120)
	if err != nil {
		return AdminDashboardInput{}, ErrInvalidAdminDashboard
	}
	description := strings.TrimSpace(input.Description)
	if len(description) > 2000 || strings.ContainsAny(description, "\x00\r\n") {
		return AdminDashboardInput{}, ErrInvalidAdminDashboard
	}
	if len(input.Definition) == 0 || len(input.Definition) > adminDashboardMaxDef || !json.Valid(input.Definition) {
		return AdminDashboardInput{}, ErrInvalidAdminDashboard
	}
	var definition map[string]any
	if err := json.Unmarshal(input.Definition, &definition); err != nil || definition == nil {
		return AdminDashboardInput{}, ErrInvalidAdminDashboard
	}
	if err := validateAdminDashboardDefinition(definition); err != nil {
		return AdminDashboardInput{}, err
	}
	return AdminDashboardInput{Name: name, Description: description, Definition: append(json.RawMessage(nil), input.Definition...)}, nil
}

func validateAdminDashboardDefinition(definition map[string]any) error {
	panels, ok := definition["panels"].([]any)
	if !ok || len(panels) > 64 {
		return ErrInvalidAdminDashboard
	}
	allowedTypes := map[string]bool{
		"metric": true, "time_series": true, "logs": true, "table": true,
		"stat": true, "heatmap": true, "error_groups": true,
		"monitor_status": true, "service_health": true,
	}
	for _, raw := range panels {
		panel, ok := raw.(map[string]any)
		if !ok || len(panel) > 12 || containsForbiddenDashboardKey(panel) {
			return ErrInvalidAdminDashboard
		}
		kind, ok := panel["type"].(string)
		if !ok || !allowedTypes[kind] {
			return ErrInvalidAdminDashboard
		}
		for key, value := range panel {
			switch key {
			case "id", "type", "title", "metric", "service", "level", "query":
			default:
				return ErrInvalidAdminDashboard
			}
			if text, isText := value.(string); isText && !validAdminControlText(text, 0, 512) {
				return ErrInvalidAdminDashboard
			}
		}
	}
	return nil
}

func containsForbiddenDashboardKey(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			key = strings.ToLower(key)
			if key == "sql" || strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "authorization") {
				return true
			}
			if containsForbiddenDashboardKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if containsForbiddenDashboardKey(child) {
				return true
			}
		}
	}
	return false
}
