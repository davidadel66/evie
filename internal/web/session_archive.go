package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type contextSessionArchiveController interface {
	ArchiveSession(context.Context, memory.SessionID) (memory.Session, error)
	RestoreSession(context.Context, memory.SessionID) (memory.Session, error)
}

func (s *Server) handleContextSessionArchive(w http.ResponseWriter, r *http.Request) {
	s.handleContextSessionLifecycle(w, r, true)
}

func (s *Server) handleContextSessionRestore(w http.ResponseWriter, r *http.Request) {
	s.handleContextSessionLifecycle(w, r, false)
}

func (s *Server) handleContextSessionLifecycle(w http.ResponseWriter, r *http.Request, archived bool) {
	var request struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil || strings.TrimSpace(string(request.SessionID)) == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must be one JSON object with a nonblank sessionId field")
		return
	}
	controller := s.contextSessions.(contextSessionArchiveController)
	s.sessionMu.Lock()
	if s.selectingSession || s.activeTurns > 0 {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "context_session_busy", "finish the current turn before changing session archival")
		return
	}
	s.selectingSession = true
	s.sessionMu.Unlock()
	defer func() {
		s.sessionMu.Lock()
		s.selectingSession = false
		s.sessionMu.Unlock()
	}()
	change := controller.RestoreSession
	if archived {
		change = controller.ArchiveSession
	}
	session, err := change(r.Context(), request.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, eviedb.ErrTurnLeaseHeld):
			managementJSONError(w, http.StatusConflict, "context_session_busy", "finish the session's current work before changing archival")
		case errors.Is(err, eviedb.ErrSessionNotActive):
			managementJSONError(w, http.StatusConflict, "context_session_state_changed", "Session choices changed; refresh and try again")
		case errors.Is(err, eviedb.ErrSessionDelegated):
			managementJSONError(w, http.StatusUnprocessableEntity, "context_session_delegated", "Delegated sessions are managed by their parent conversation")
		default:
			managementJSONError(w, http.StatusInternalServerError, "context_session_archive_failed", "Session archival could not be changed")
		}
		return
	}
	if archived {
		s.sessionMu.Lock()
		if s.activeSession.ID == request.SessionID {
			s.session = nil
			s.activeSession = memory.Session{}
		}
		s.sessionMu.Unlock()
	}
	writeJSON(w, http.StatusOK, struct {
		Session memory.Session `json:"session"`
	}{session})
}
