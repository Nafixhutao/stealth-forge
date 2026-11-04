package httpapi

import (
	"net/http"
	"strings"

	"github.com/Nafixhutao/stealth-forge/internal/config"
	"github.com/Nafixhutao/stealth-forge/internal/repository"
)

// agentCatalogExecution describes the current control-plane boundary. The
// API accepts and persists runs, but never claims that a provider call or
// repository mutation happened until a trusted worker is deployed.
type agentCatalogExecution struct {
	Mode    string `json:"mode"`
	Ready   bool   `json:"ready"`
	Message string `json:"message"`
}

func (s *Server) agentCatalog(w http.ResponseWriter, _ *http.Request) {
	providers := config.AgentProviderCatalogOrDefault(s.config.AgentProviderCatalog)
	execution := agentCatalogExecution{
		Mode:    "queue_only",
		Ready:   false,
		Message: "Runs are accepted into the durable queue; provider execution is not enabled on this installation.",
	}
	// The catalog is the source of truth the Console reads. When the worker is
	// configured with a trusted runtime, say so instead of leaving the page on
	// its queue-only copy.
	if strings.TrimSpace(s.config.AgentRuntimeBinary) != "" {
		execution = agentCatalogExecution{
			Mode:    "executing",
			Ready:   true,
			Message: "A trusted Agent runtime is installed; prompts execute on the worker.",
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": providers,
		"roles":     repository.SupportedAgentRoles(),
		"tools":     repository.SupportedAgentTools(),
		"execution": execution,
	})
}
