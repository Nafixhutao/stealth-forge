package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/domain"
	"github.com/Stealth-deplover/stealth/internal/repository"
)

type adminAlertRuleRequest struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Condition  map[string]any `json:"condition"`
	Severity   string         `json:"severity"`
	ForSeconds int            `json:"for_seconds"`
	Enabled    *bool          `json:"enabled"`
}

type adminAlertRuleResponse struct {
	Rule   domain.AdminAlertRule    `json:"rule"`
	Events []domain.AdminAlertEvent `json:"events,omitempty"`
}

type adminAlertRulesResponse struct {
	Items []domain.AdminAlertRule `json:"items"`
}

type adminAlertEventsResponse struct {
	Items      []domain.AdminAlertEvent `json:"items"`
	NextCursor *string                  `json:"next_cursor,omitempty"`
}

type adminNotificationChannelRequest struct {
	Name    string         `json:"name"`
	Kind    string         `json:"kind"`
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

type adminNotificationChannelsResponse struct {
	Items []domain.AdminNotificationChannel `json:"items"`
}

type adminNotificationTestResponse struct {
	DeliveryID string `json:"delivery_id"`
	Status     string `json:"status"`
}

type adminIncidentRequest struct {
	Title    string   `json:"title"`
	Severity string   `json:"severity"`
	Status   string   `json:"status"`
	Services []string `json:"services"`
	Message  string   `json:"message"`
}

type adminIncidentPatchRequest struct {
	Title    *string   `json:"title"`
	Severity *string   `json:"severity"`
	Status   *string   `json:"status"`
	Services *[]string `json:"services"`
	Message  *string   `json:"message"`
}

type adminIncidentEventRequest struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type adminIncidentsResponse struct {
	Items []domain.AdminIncident `json:"items"`
}

type adminIncidentResponse struct {
	Incident domain.AdminIncident `json:"incident"`
}

type adminDashboardRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Definition  map[string]any `json:"definition"`
}

type adminDashboardsResponse struct {
	Items []domain.AdminDashboard `json:"items"`
}

type adminDashboardResponse struct {
	Dashboard domain.AdminDashboard `json:"dashboard"`
}

type adminStatusPageRequest struct {
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	IsPublic           bool             `json:"is_public"`
	Components         []map[string]any `json:"components"`
	PublishedIncidents []string         `json:"published_incidents"`
}

func adminConfigLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "validation_error", "limit must be an integer between 1 and 100")
			return 0, false
		}
		limit = parsed
	}
	return limit, true
}

func adminControlError(s *Server, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrAdminAlertRuleConflict):
		writeError(w, http.StatusConflict, "admin_alert_rule_conflict", "the alert rule changed concurrently")
	case errors.Is(err, repository.ErrInvalidAdminAlert), errors.Is(err, repository.ErrInvalidAdminAlertHistory), errors.Is(err, repository.ErrInvalidAdminNotification), errors.Is(err, repository.ErrInvalidAdminIncident), errors.Is(err, repository.ErrInvalidAdminDashboard), errors.Is(err, repository.ErrInvalidAdminStatus):
		writeError(w, http.StatusBadRequest, "validation_error", "admin configuration is invalid")
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "admin resource was not found")
	case errors.Is(err, repository.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "instance owner or admin permission is required")
	default:
		internalError(s, w, err)
	}
}
