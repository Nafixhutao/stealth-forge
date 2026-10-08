package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
	"github.com/go-chi/chi/v5"
)

// adminOAuthProvider is the safe Console projection for one sign-in provider.
// The client secret is never part of this shape.
type adminOAuthProvider struct {
	Provider    string `json:"provider"`
	Configured  bool   `json:"configured"`
	ClientID    string `json:"client_id,omitempty"`
	CallbackURL string `json:"callback_url"`
	// Source is where the active credentials come from: "database" when the
	// operator saved them in the Console, "environment" when they come from the
	// deployment environment, "none" when the provider is unavailable.
	Source    string     `json:"source"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type adminOAuthProvidersResponse struct {
	Providers []adminOAuthProvider `json:"providers"`
}

type updateAdminOAuthProviderRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// oauthProviderNames is the fixed order the Console renders.
var oauthProviderNames = []string{"github", "google"}

func (s *Server) listAdminOAuthProviders(w http.ResponseWriter, r *http.Request) {
	stored := map[string]repository.OAuthProviderStatus{}
	if s.repo != nil {
		statuses, err := s.repo.ListOAuthProviders(r.Context())
		if err != nil {
			internalError(s, w, err)
			return
		}
		for _, status := range statuses {
			stored[status.Provider] = status
		}
	}
	providers := make([]adminOAuthProvider, 0, len(oauthProviderNames))
	for _, name := range oauthProviderNames {
		providers = append(providers, s.adminOAuthProvider(r, name, stored[name]))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminOAuthProvidersResponse{Providers: providers})
}

// adminOAuthProvider builds one provider row from stored credentials with a
// fallback to the deployment environment.
func (s *Server) adminOAuthProvider(r *http.Request, name string, stored repository.OAuthProviderStatus) adminOAuthProvider {
	callbackURL, err := s.oauthCallbackURL(r, name)
	if err != nil {
		callbackURL = ""
	}
	entry := adminOAuthProvider{Provider: name, CallbackURL: callbackURL, Source: "none"}
	if stored.Configured {
		entry.Configured = true
		entry.ClientID = stored.ClientID
		entry.Source = "database"
		updated := stored.UpdatedAt
		entry.UpdatedAt = &updated
		return entry
	}
	if provider, ok := s.oauthProviderFromEnvironment(name); ok {
		entry.Configured = true
		entry.ClientID = provider.ClientID
		entry.Source = "environment"
	}
	return entry
}

func (s *Server) updateAdminOAuthProvider(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if !repository.ValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "not_found", "unknown sign-in provider")
		return
	}
	var request updateAdminOAuthProviderRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	status, err := s.repo.UpsertOAuthProvider(r.Context(), provider, request.ClientID, request.ClientSecret)
	if err != nil {
		if errors.Is(err, repository.ErrOAuthProviderUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "oauth_storage_unavailable", "sign-in provider storage is unavailable")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.adminOAuthProvider(r, provider, status))
}

func (s *Server) deleteAdminOAuthProvider(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if !repository.ValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "not_found", "unknown sign-in provider")
		return
	}
	if err := s.repo.DeleteOAuthProvider(r.Context(), provider); err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
