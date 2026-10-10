package httpapi

import (
	"net/http"
)

type adminAccountsResponse struct {
	Items []adminAccountItem `json:"items"`
}

type adminAccountItem struct {
	ID               string   `json:"id"`
	Email            string   `json:"email"`
	EmailVerified    bool     `json:"email_verified"`
	InstanceRole     string   `json:"instance_role,omitempty"`
	CreatedAt        string   `json:"created_at"`
	Providers        []string `json:"providers"`
	AvatarURL        string   `json:"avatar_url,omitempty"`
	DisplayName      string   `json:"display_name,omitempty"`
	LastSignInAt     string   `json:"last_sign_in_at,omitempty"`
	LastSignInMethod string   `json:"last_sign_in_method,omitempty"`
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
		providers := account.Providers
		if providers == nil {
			providers = []string{}
		}
		items = append(items, adminAccountItem{
			ID:               account.ID,
			Email:            account.Email,
			EmailVerified:    account.EmailVerified,
			InstanceRole:     account.InstanceRole,
			CreatedAt:        account.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Providers:        providers,
			AvatarURL:        account.AvatarURL,
			DisplayName:      account.DisplayName,
			LastSignInAt:     account.LastSignInAt,
			LastSignInMethod: account.LastSignInMethod,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminAccountsResponse{Items: items})
}
