package repository

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AdminStatusPageInput struct {
	Name               string
	Description        string
	IsPublic           bool
	Components         json.RawMessage
	PublishedIncidents []string
}

func (r *Repository) AdminStatusPage(ctx context.Context) (domain.AdminStatusPage, error) {
	var item domain.AdminStatusPage
	var components, published []byte
	err := r.pool.QueryRow(ctx, `SELECT name,description,is_public,components,published_incidents,updated_at FROM admin_status_page_config WHERE id=TRUE`).Scan(&item.Name, &item.Description, &item.IsPublic, &components, &published, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminStatusPage{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminStatusPage{}, err
	}
	if !json.Valid(components) || json.Unmarshal(components, &item.Components) != nil {
		return domain.AdminStatusPage{}, ErrInvalidAdminStatus
	}
	if item.Components == nil {
		item.Components = []map[string]any{}
	}
	if !json.Valid(published) || json.Unmarshal(published, &item.PublishedIncidents) != nil {
		return domain.AdminStatusPage{}, ErrInvalidAdminStatus
	}
	if item.PublishedIncidents == nil {
		item.PublishedIncidents = []string{}
	}
	return item, nil
}

func (r *Repository) PublicAdminStatusPage(ctx context.Context) (domain.AdminPublicStatusPage, error) {
	var page domain.AdminPublicStatusPage
	var components, published []byte
	var publishedIDs []uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT name,description,components,published_incidents,updated_at FROM admin_status_page_config WHERE id=TRUE AND is_public`).Scan(&page.Name, &page.Description, &components, &published, &page.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AdminPublicStatusPage{}, ErrNotFound
	}
	if err != nil {
		return domain.AdminPublicStatusPage{}, err
	}
	if !json.Valid(components) || json.Unmarshal(components, &page.Components) != nil {
		return domain.AdminPublicStatusPage{}, ErrInvalidAdminStatus
	}
	if page.Components == nil {
		page.Components = []map[string]any{}
	}
	var publishedStrings []string
	if !json.Valid(published) || json.Unmarshal(published, &publishedStrings) != nil {
		return domain.AdminPublicStatusPage{}, ErrInvalidAdminStatus
	}
	for _, value := range publishedStrings {
		id, parseErr := uuid.Parse(strings.TrimSpace(value))
		if parseErr != nil || id == uuid.Nil {
			return domain.AdminPublicStatusPage{}, ErrInvalidAdminStatus
		}
		publishedIDs = append(publishedIDs, id)
	}
	page.Incidents = []domain.AdminPublicIncident{}
	if len(publishedIDs) > 0 {
		rows, queryErr := r.pool.Query(ctx, `
			SELECT id::text,title,severity,status,services,started_at,resolved_at
			FROM admin_incidents WHERE id=ANY($1::uuid[])
			ORDER BY started_at DESC,id DESC`, publishedIDs)
		if queryErr != nil {
			return domain.AdminPublicStatusPage{}, queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var incident domain.AdminPublicIncident
			if scanErr := rows.Scan(&incident.ID, &incident.Title, &incident.Severity, &incident.Status, &incident.Services, &incident.StartedAt, &incident.ResolvedAt); scanErr != nil {
				return domain.AdminPublicStatusPage{}, scanErr
			}
			if incident.Services == nil {
				incident.Services = []string{}
			}
			page.Incidents = append(page.Incidents, incident)
		}
		if err := rows.Err(); err != nil {
			return domain.AdminPublicStatusPage{}, err
		}
	}
	return page, nil
}

func (r *Repository) UpdateAdminStatusPage(ctx context.Context, accountID uuid.UUID, input AdminStatusPageInput) (domain.AdminStatusPage, error) {
	normalized, err := normalizeAdminStatusPageInput(input)
	if err != nil {
		return domain.AdminStatusPage{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.AdminStatusPage{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireInstanceAdminTx(ctx, tx, accountID); err != nil {
		return domain.AdminStatusPage{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE admin_status_page_config SET name=$2,description=$3,is_public=$4,components=$5,published_incidents=$6,updated_by_account_id=$7,updated_at=now() WHERE id=TRUE`, normalized.Name, normalized.Description, normalized.IsPublic, normalized.Components, normalized.PublishedIncidents, accountID)
	if err != nil {
		return domain.AdminStatusPage{}, mapError(err)
	}
	statusID := uuid.Nil
	if err := writeInstanceAuditTx(ctx, tx, accountID, "admin.status_page.update", "admin_status_page", statusID, map[string]any{"is_public": normalized.IsPublic}); err != nil {
		return domain.AdminStatusPage{}, err
	}
	var item domain.AdminStatusPage
	var components, published []byte
	if err := tx.QueryRow(ctx, `SELECT name,description,is_public,components,published_incidents,updated_at FROM admin_status_page_config WHERE id=TRUE`).Scan(&item.Name, &item.Description, &item.IsPublic, &components, &published, &item.UpdatedAt); err != nil {
		return domain.AdminStatusPage{}, err
	}
	if err := json.Unmarshal(components, &item.Components); err != nil {
		return domain.AdminStatusPage{}, err
	}
	if err := json.Unmarshal(published, &item.PublishedIncidents); err != nil {
		return domain.AdminStatusPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.AdminStatusPage{}, err
	}
	return item, nil
}

func normalizeAdminStatusPageInput(input AdminStatusPageInput) (AdminStatusPageInput, error) {
	name, err := normalizeAdminControlText(input.Name, 1, 160)
	if err != nil {
		return AdminStatusPageInput{}, ErrInvalidAdminStatus
	}
	if len(input.Description) > 4000 || strings.ContainsAny(input.Description, "\x00\r\n") {
		return AdminStatusPageInput{}, ErrInvalidAdminStatus
	}
	if len(input.Components) == 0 || len(input.Components) > 128<<10 || !json.Valid(input.Components) {
		return AdminStatusPageInput{}, ErrInvalidAdminStatus
	}
	var components []map[string]any
	if err := json.Unmarshal(input.Components, &components); err != nil || len(components) > 64 {
		return AdminStatusPageInput{}, ErrInvalidAdminStatus
	}
	for _, component := range components {
		if err := validatePublicStatusComponent(component); err != nil {
			return AdminStatusPageInput{}, err
		}
	}
	if len(input.PublishedIncidents) > 64 {
		return AdminStatusPageInput{}, ErrInvalidAdminStatus
	}
	seen := make(map[string]struct{}, len(input.PublishedIncidents))
	published := make([]string, 0, len(input.PublishedIncidents))
	for _, value := range input.PublishedIncidents {
		parsed, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil || parsed == uuid.Nil {
			return AdminStatusPageInput{}, ErrInvalidAdminStatus
		}
		normalizedID := parsed.String()
		if _, exists := seen[normalizedID]; exists {
			return AdminStatusPageInput{}, ErrInvalidAdminStatus
		}
		seen[normalizedID] = struct{}{}
		published = append(published, normalizedID)
	}
	return AdminStatusPageInput{Name: name, Description: strings.TrimSpace(input.Description), IsPublic: input.IsPublic, Components: append(json.RawMessage(nil), input.Components...), PublishedIncidents: published}, nil
}

func validatePublicStatusComponent(component map[string]any) error {
	if component == nil || !conditionHasBoundedString(component, "name", 120) || !conditionHasBoundedString(component, "status", 32) {
		return ErrInvalidAdminStatus
	}
	allowed := map[string]bool{"name": true, "status": true, "description": true, "url": true}
	for key, value := range component {
		if !allowed[key] || strings.Contains(strings.ToLower(key), "secret") || strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "password") {
			return ErrInvalidAdminStatus
		}
		text, ok := value.(string)
		if !ok || !validAdminControlText(text, 0, 512) {
			return ErrInvalidAdminStatus
		}
		if key == "status" {
			switch text {
			case "operational", "degraded", "partial_outage", "major_outage", "maintenance":
			default:
				return ErrInvalidAdminStatus
			}
		}
		if key == "url" {
			parsed, err := url.ParseRequestURI(text)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
				return ErrInvalidAdminStatus
			}
		}
	}
	return nil
}
