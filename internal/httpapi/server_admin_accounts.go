package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/domain"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
)

type adminAccountsResponse struct {
	Items []adminAccountItem `json:"items"`
}

type adminAccountItem struct {
	ID               string   `json:"id"`
	Email            string   `json:"email"`
	EmailVerified    bool     `json:"email_verified"`
	InstanceRole     string   `json:"instance_role,omitempty"`
	Plan             string   `json:"plan"`
	CreatedAt        string   `json:"created_at"`
	Providers        []string `json:"providers"`
	AvatarURL        string   `json:"avatar_url,omitempty"`
	DisplayName      string   `json:"display_name,omitempty"`
	LastSignInAt     string   `json:"last_sign_in_at,omitempty"`
	LastSignInMethod string   `json:"last_sign_in_method,omitempty"`
}

type adminAccountResponse struct {
	Account adminAccountItem `json:"account"`
}

type adminAccountPlanRequest struct {
	Plan string `json:"plan"`
}

func adminAccountItemFrom(item domain.AdminAccount) adminAccountItem {
	providers := item.Providers
	if providers == nil {
		providers = []string{}
	}
	return adminAccountItem{
		ID:               item.ID,
		Email:            item.Email,
		EmailVerified:    item.EmailVerified,
		InstanceRole:     item.InstanceRole,
		Plan:             item.Plan,
		CreatedAt:        item.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Providers:        providers,
		AvatarURL:        item.AvatarURL,
		DisplayName:      item.DisplayName,
		LastSignInAt:     item.LastSignInAt,
		LastSignInMethod: item.LastSignInMethod,
	}
}

// updateAdminAccountPlan assigns one Console account's subscription plan
// (free, go, plus, pro). The change is audit-logged with the acting admin.
func (s *Server) updateAdminAccountPlan(w http.ResponseWriter, r *http.Request) {
	accountID, ok := pathUUID(w, r, "accountID")
	if !ok {
		return
	}
	if s.repo == nil {
		internalError(s, w, errors.New("repository is unavailable"))
		return
	}
	var request adminAccountPlanRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	item, err := s.repo.UpdateInstanceAccountPlan(
		r.Context(), mustUUID(accountFrom(r).ID), accountID, strings.ToLower(strings.TrimSpace(request.Plan)),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "account does not exist")
		case errors.Is(err, repository.ErrInvalidBackup):
			writeError(w, http.StatusUnprocessableEntity, "invalid_plan", "plan must be one of free, go, plus, pro")
		default:
			internalError(s, w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, adminAccountResponse{Account: adminAccountItemFrom(item)})
}

// listAdminAccounts serves the Admin Console's user directory from real
// accounts. Passwords and provider secrets are never part of the projection;
// the Console masks the email before rendering it.
func (s *Server) listAdminAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.repo.ListInstanceAccounts(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	items := make([]adminAccountItem, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, adminAccountItemFrom(account))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminAccountsResponse{Items: items})
}
