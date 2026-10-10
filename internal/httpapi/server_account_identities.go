package httpapi

import (
	"net/http"
	"strings"

	"github.com/Stealth-deplover/stealth/internal/repository"
)

type accountIdentitiesResponse struct {
	Items []accountIdentityItem `json:"items"`
	// Providers lists the sign-in providers the instance has configured, so
	// the Console offers a link button only for providers that can complete.
	Providers []string `json:"providers"`
}

type accountIdentityItem struct {
	Provider      string `json:"provider"`
	ProviderLogin string `json:"provider_login,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// listAccountIdentities returns the external sign-in identities linked to the
// signed-in account plus the providers the instance can complete a round trip
// with. It never exposes provider secrets or provider user ids.
func (s *Server) listAccountIdentities(w http.ResponseWriter, r *http.Request) {
	account := accountFrom(r)
	identities, err := s.repo.ListAccountIdentities(r.Context(), mustUUID(account.ID))
	if err != nil {
		internalError(s, w, err)
		return
	}
	items := make([]accountIdentityItem, 0, len(identities))
	for _, identity := range identities {
		items = append(items, accountIdentityItem{
			Provider:      identity.Provider,
			ProviderLogin: identity.ProviderLogin,
			DisplayName:   identity.DisplayName,
			AvatarURL:     identity.AvatarURL,
			CreatedAt:     identity.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	providers := make([]string, 0, 2)
	for _, name := range []string{"github", "google"} {
		if _, configured := s.oauthProvider(r.Context(), name); configured {
			providers = append(providers, name)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, accountIdentitiesResponse{Items: items, Providers: providers})
}

// unlinkAccountIdentity removes one provider identity from the signed-in
// account. The account must keep at least one identity so a provider-only
// account cannot lock itself out of every sign-in method.
func (s *Server) unlinkAccountIdentity(w http.ResponseWriter, r *http.Request) {
	account := accountFrom(r)
	provider := strings.ToLower(strings.TrimSpace(chiURLParam(r, "provider")))
	if !repository.ValidOAuthProvider(provider) {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", "provider is not supported")
		return
	}
	identities, err := s.repo.ListAccountIdentities(r.Context(), mustUUID(account.ID))
	if err != nil {
		internalError(s, w, err)
		return
	}
	remaining := 0
	for _, identity := range identities {
		if identity.Provider != provider {
			remaining++
		}
	}
	if remaining == 0 {
		writeError(
			w,
			http.StatusConflict,
			"last_sign_in_method",
			"The last sign-in identity cannot be unlinked",
		)
		return
	}
	if err := s.repo.DeleteAccountIdentity(r.Context(), mustUUID(account.ID), provider); err != nil {
		internalError(s, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
