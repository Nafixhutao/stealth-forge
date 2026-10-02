package repository

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AdminAlertRuleInput struct {
	Name       string
	Kind       string
	Condition  json.RawMessage
	Severity   string
	ForSeconds int
	Enabled    bool
}

type AdminAlertRulePatch struct {
	Name       *string
	Kind       *string
	Condition  json.RawMessage
	Severity   *string
	ForSeconds *int
	Enabled    *bool
}

const adminAlertRuleProjection = `
	r.id::text,r.name,r.kind,r.condition,r.severity,r.for_seconds,r.enabled,r.state,
	r.pending_since,r.last_evaluated_at,r.last_value,r.last_error,
	r.created_by_account_id::text,r.created_at,r.updated_at`

func (r *Repository) ListAdminAlertRules(ctx context.Context, limit int) ([]domain.AdminAlertRule, error) {
	if limit < 1 || limit > adminAlertMaxLimit {
		return nil, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidAdminAlert, adminAlertMaxLimit)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r ORDER BY r.updated_at DESC,r.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.AdminAlertRule, 0, limit)
	for rows.Next() {
		item, scanErr := scanAdminAlertRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AdminAlertRuleByID(ctx context.Context, id uuid.UUID) (domain.AdminAlertRule, error) {
	if id == uuid.Nil {
		return domain.AdminAlertRule{}, ErrNotFound
	}
	item, err := scanAdminAlertRule(r.pool.QueryRow(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminAlertRule{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) CreateAdminAlertRule(ctx context.Context, accountID, id uuid.UUID, input AdminAlertRuleInput) (domain.AdminAlertRule, error) {
	normalized, err := normalizeAdminAlertRuleInput(input)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := validateAdminAlertMonitorReferenceTx(ctx, tx, normalized.Kind, normalized.Condition); err != nil {
		return domain.AdminAlertRule{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_alert_rules (id,name,kind,condition,severity,for_seconds,enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, normalized.Name, normalized.Kind, normalized.Condition, normalized.Severity, normalized.ForSeconds, normalized.Enabled); err != nil {
		return domain.AdminAlertRule{}, mapError(err)
	}
	item, err := scanAdminAlertRule(tx.QueryRow(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r WHERE r.id=$1`, id))
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.alert.create", "admin_alert_rule", id, map[string]any{"kind": normalized.Kind, "severity": normalized.Severity}); err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminAlertRule{}, err
	}
	return item, nil
}

func (r *Repository) UpdateAdminAlertRule(ctx context.Context, accountID, id uuid.UUID, patch AdminAlertRulePatch) (domain.AdminAlertRule, error) {
	normalized, err := normalizeAdminAlertRulePatch(patch)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	defer tx.Rollback(ctx)
	item, err := updateAdminAlertRuleTx(ctx, tx, accountID, id, normalized)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminAlertRule{}, err
	}
	return item, nil
}

// updateAdminAlertRuleTx locks every monitor involved in the current or final
// relationship before locking the rule row. Monitor-backed relationship
// changes therefore use monitor -> rule ordering, matching monitor updates and
// monitor-worker evaluation. The caller must commit or roll back tx.
func updateAdminAlertRuleTx(ctx context.Context, tx pgx.Tx, accountID, id uuid.UUID, normalized AdminAlertRulePatch) (domain.AdminAlertRule, error) {
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminAlertRule{}, err
	}
	var current domain.AdminAlertRule
	current, err := scanAdminAlertRule(tx.QueryRow(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminAlertRule{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	_, kind, condition, _, _, _, err := mergedAdminAlertRuleValues(current, normalized)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := validateAdminAlertCondition(kind, condition); err != nil {
		return domain.AdminAlertRule{}, err
	}
	currentCondition, err := json.Marshal(current.Condition)
	if err != nil {
		return domain.AdminAlertRule{}, ErrInvalidAdminAlert
	}
	currentMonitorID, currentMonitorBacked, err := adminAlertRuleMonitorID(current.Kind, currentCondition)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	finalMonitorID, finalMonitorBacked, err := adminAlertRuleMonitorID(kind, condition)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	lockedMonitorKinds, err := lockAdminAlertMonitorsTx(ctx, tx, currentMonitorID, finalMonitorID)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if currentMonitorBacked {
		if err := validateLockedAdminAlertMonitor(current.Kind, currentMonitorID, lockedMonitorKinds); err != nil {
			return domain.AdminAlertRule{}, err
		}
	}
	if finalMonitorBacked {
		if err := validateLockedAdminAlertMonitor(kind, finalMonitorID, lockedMonitorKinds); err != nil {
			return domain.AdminAlertRule{}, err
		}
	}

	current, err = scanAdminAlertRule(tx.QueryRow(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r WHERE r.id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminAlertRule{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	name, kind, condition, severity, forSeconds, enabled, err := mergedAdminAlertRuleValues(current, normalized)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := validateAdminAlertCondition(kind, condition); err != nil {
		return domain.AdminAlertRule{}, err
	}
	finalMonitorID, finalMonitorBacked, err = adminAlertRuleMonitorID(kind, condition)
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if finalMonitorBacked {
		if err := validateLockedAdminAlertMonitor(kind, finalMonitorID, lockedMonitorKinds); err != nil {
			if errors.Is(err, ErrInvalidAdminAlert) {
				return domain.AdminAlertRule{}, err
			}
			return domain.AdminAlertRule{}, ErrAdminAlertRuleConflict
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE admin_alert_rules
		SET name=$2,kind=$3,condition=$4,severity=$5,for_seconds=$6,enabled=$7,
		    state=CASE WHEN $7 THEN CASE WHEN state='muted' THEN 'normal' ELSE state END ELSE 'muted' END,
		    pending_since=CASE WHEN $7 THEN pending_since ELSE NULL END,updated_at=now()
		WHERE id=$1`, id, name, kind, condition, severity, forSeconds, enabled); err != nil {
		return domain.AdminAlertRule{}, mapError(err)
	}
	item, err := scanAdminAlertRule(tx.QueryRow(ctx, `SELECT `+adminAlertRuleProjection+` FROM admin_alert_rules r WHERE r.id=$1`, id))
	if err != nil {
		return domain.AdminAlertRule{}, err
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.alert.update", "admin_alert_rule", id, map[string]any{"kind": kind, "enabled": enabled}); err != nil {
		return domain.AdminAlertRule{}, err
	}
	return item, nil
}

func mergedAdminAlertRuleValues(current domain.AdminAlertRule, normalized AdminAlertRulePatch) (string, string, json.RawMessage, string, int, bool, error) {
	name := current.Name
	if normalized.Name != nil {
		name = *normalized.Name
	}
	kind := current.Kind
	if normalized.Kind != nil {
		kind = *normalized.Kind
	}
	condition, err := json.Marshal(current.Condition)
	if err != nil {
		return "", "", nil, "", 0, false, ErrInvalidAdminAlert
	}
	if normalized.Condition != nil {
		condition = append(json.RawMessage(nil), normalized.Condition...)
	}
	severity := current.Severity
	if normalized.Severity != nil {
		severity = *normalized.Severity
	}
	forSeconds := current.ForSeconds
	if normalized.ForSeconds != nil {
		forSeconds = *normalized.ForSeconds
	}
	enabled := current.Enabled
	if normalized.Enabled != nil {
		enabled = *normalized.Enabled
	}
	return name, kind, condition, severity, forSeconds, enabled, nil
}

func lockAdminAlertMonitorsTx(ctx context.Context, tx pgx.Tx, monitorIDs ...uuid.UUID) (map[uuid.UUID]string, error) {
	unique := make(map[uuid.UUID]struct{}, len(monitorIDs))
	ordered := make([]uuid.UUID, 0, len(monitorIDs))
	for _, monitorID := range monitorIDs {
		if monitorID == uuid.Nil {
			continue
		}
		if _, ok := unique[monitorID]; ok {
			continue
		}
		unique[monitorID] = struct{}{}
		ordered = append(ordered, monitorID)
	}
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return cmp.Compare(a.String(), b.String()) })
	locked := make(map[uuid.UUID]string, len(ordered))
	for _, monitorID := range ordered {
		var monitorKind string
		err := tx.QueryRow(ctx, `SELECT kind FROM admin_monitors WHERE id=$1 FOR UPDATE`, monitorID).Scan(&monitorKind)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: monitor does not exist", ErrInvalidAdminAlert)
		}
		if err != nil {
			return nil, err
		}
		locked[monitorID] = monitorKind
	}
	return locked, nil
}

func validateLockedAdminAlertMonitor(ruleKind string, monitorID uuid.UUID, lockedMonitorKinds map[uuid.UUID]string) error {
	monitorKind, ok := lockedMonitorKinds[monitorID]
	if !ok {
		return ErrAdminAlertRuleConflict
	}
	return validateAdminAlertMonitorCompatibility(ruleKind, monitorKind)
}

func adminAlertRuleMonitorID(kind string, raw json.RawMessage) (uuid.UUID, bool, error) {
	if kind != "monitor_failure" && kind != "heartbeat_failure" && kind != "certificate_expiry" {
		return uuid.Nil, false, nil
	}
	var condition map[string]any
	if err := json.Unmarshal(raw, &condition); err != nil {
		return uuid.Nil, false, fmt.Errorf("%w: monitor condition is invalid", ErrInvalidAdminAlert)
	}
	monitorIDValue, ok := condition["monitor_id"].(string)
	if !ok {
		return uuid.Nil, false, fmt.Errorf("%w: monitor_id is invalid", ErrInvalidAdminAlert)
	}
	monitorID, err := uuid.Parse(strings.TrimSpace(monitorIDValue))
	if err != nil || monitorID == uuid.Nil {
		return uuid.Nil, false, fmt.Errorf("%w: monitor_id is invalid", ErrInvalidAdminAlert)
	}
	return monitorID, true, nil
}

func validateAdminAlertMonitorReferenceTx(ctx context.Context, tx pgx.Tx, kind string, raw json.RawMessage) error {
	monitorID, monitorBacked, err := adminAlertRuleMonitorID(kind, raw)
	if err != nil || !monitorBacked {
		return err
	}
	lockedMonitorKinds, err := lockAdminAlertMonitorsTx(ctx, tx, monitorID)
	if err != nil {
		return err
	}
	return validateLockedAdminAlertMonitor(kind, monitorID, lockedMonitorKinds)
}

func (r *Repository) DeleteAdminAlertRule(ctx context.Context, accountID, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := deleteAdminAlertRuleTx(ctx, tx, accountID, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func deleteAdminAlertRuleTx(ctx context.Context, tx pgx.Tx, accountID, id uuid.UUID) error {
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return err
	}
	var name, kind, severity string
	if err := tx.QueryRow(ctx, `
		SELECT name,kind,severity
		FROM admin_alert_rules
		WHERE id=$1
		FOR UPDATE`, id).Scan(&name, &kind, &severity); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `DELETE FROM admin_alert_rules WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.alert.delete", "admin_alert_rule", id, map[string]any{
		"name":     name,
		"kind":     kind,
		"severity": severity,
	}); err != nil {
		return err
	}
	return nil
}

func (r *Repository) ListAdminAlertEvents(ctx context.Context, ruleID uuid.UUID, limit int) ([]domain.AdminAlertEvent, error) {
	page, err := r.QueryAdminAlertEvents(ctx, AdminAlertEventQuery{RuleID: &ruleID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (r *Repository) ListRecentAdminAlertEvents(ctx context.Context, limit int) ([]domain.AdminAlertEvent, error) {
	page, err := r.QueryAdminAlertEvents(ctx, AdminAlertEventQuery{Limit: limit})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func scanAdminAlertRule(row interface{ Scan(...any) error }) (domain.AdminAlertRule, error) {
	var item domain.AdminAlertRule
	var condition []byte
	if err := row.Scan(&item.ID, &item.Name, &item.Kind, &condition, &item.Severity, &item.ForSeconds, &item.Enabled, &item.State, &item.PendingSince, &item.LastEvaluatedAt, &item.LastValue, &item.LastError, &item.CreatedByAccountID, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return domain.AdminAlertRule{}, err
	}
	if !json.Valid(condition) || json.Unmarshal(condition, &item.Condition) != nil {
		return domain.AdminAlertRule{}, ErrInvalidAdminAlert
	}
	if item.Condition == nil {
		item.Condition = map[string]any{}
	}
	return item, nil
}
