package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Stealth-deplover/stealth/internal/setupconfig"
	"github.com/Stealth-deplover/stealth/internal/setupstate"
	"github.com/google/uuid"
)

func (s *Server) startSetupInstall(w http.ResponseWriter, r *http.Request) {
	var ignored map[string]any
	if !decodeJSON(w, r, &ignored) {
		return
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if !s.setupStateReady() {
		writeError(w, http.StatusServiceUnavailable, "setup_unavailable", "the setup service is not ready")
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	if state.Phase == setupstate.PhaseInstallRequested || state.Phase == setupstate.PhaseInstalling {
		status := "accepted"
		if state.Phase == setupstate.PhaseInstalling {
			status = "installing"
		}
		writeJSON(w, http.StatusAccepted, setupInstallResponse{Status: status, State: state.Public()})
		return
	}
	if state.Phase == setupstate.PhaseComplete || state.Phase == setupstate.PhaseHandoff {
		writeError(w, http.StatusConflict, "setup_complete", "installation has already been handed off")
		return
	}
	bootstrapStatus, err := s.bootstrap.BootstrapStatus(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	if bootstrapStatus.SetupRequired {
		writeError(w, http.StatusConflict, "setup_owner_required", "finish GitHub first-owner verification before installing")
		return
	}
	if err := setupconfig.ValidateInstallableSetup(state); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
		return
	}
	runID := uuid.NewString()
	state, err = s.setupState.Update(r.Context(), func(state *setupstate.State) error {
		if err := setupconfig.ValidateInstallableSetup(*state); err != nil {
			return &setupInstallValidationError{err: err}
		}
		if err := setupstate.RequestInstallation(state, runID); err != nil {
			return err
		}
		state.Step = "Configuration and secrets"
		return nil
	})
	if err != nil {
		var validationErr *setupInstallValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusUnprocessableEntity, "validation_error", validationErr.Error())
			return
		}
		writeError(w, http.StatusConflict, "setup_state_conflict", "the installation could not be started")
		return
	}
	writeJSON(w, http.StatusAccepted, setupInstallResponse{Status: "accepted", State: state.Public()})
}

func (s *Server) setupInstallEvents(w http.ResponseWriter, r *http.Request) {
	if s.setupState == nil {
		writeError(w, http.StatusServiceUnavailable, "setup_unavailable", "setup events are not available")
		return
	}
	state, err := s.setupState.Load(r.Context())
	if err != nil {
		internalError(s, w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unavailable", "install event streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	writeSSE(w, "snapshot", state.LastEventID, state.Public())
	flusher.Flush()
	poll := time.NewTicker(setupEventPoll)
	defer poll.Stop()
	heartbeat := time.NewTicker(setupEventHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			latest, loadErr := s.setupState.Load(r.Context())
			if loadErr != nil || latest.LastEventID <= state.LastEventID {
				continue
			}
			state = latest
			writeSSE(w, "progress", state.LastEventID, map[string]string{
				"step":  state.Step,
				"error": state.ErrorMessage,
			})
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
func writeSSE(w io.Writer, eventName string, id uint64, payload any) {
	contents, err := jsonMarshal(payload)
	if err != nil {
		return
	}
	if id > 0 {
		_, _ = io.WriteString(w, "id: "+strconv.FormatUint(id, 10)+"\n")
	}
	_, _ = io.WriteString(w, "event: "+eventName+"\n")
	_, _ = io.WriteString(w, "data: "+contents+"\n\n")
}

// jsonMarshal is kept local to the SSE writer so the event path has no
// dependency on the HTTP JSON response headers or on request-scoped state.
func jsonMarshal(value any) (string, error) {
	contents, err := json.Marshal(value)
	return string(contents), err
}
