package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type ModelController interface {
	ListChatModels(context.Context) ([]openrouter.Model, error)
	SelectModel(context.Context, memory.SessionID, int64, string) (OpenedContextSession, error)
}

func (s *Server) handleModelList(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a session")
		return
	}
	controller := s.contextSessions.(ModelController)
	models, err := controller.ListChatModels(r.Context())
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	if request.SessionID == "" || request.SessionID != s.activeSession.ID || s.session == nil || s.selectingSession {
		managementJSONError(w, http.StatusConflict, "model_session_changed", "Chat changed; refresh and try again")
		return
	}
	if models == nil {
		models = []openrouter.Model{}
	}
	problem := ""
	if err != nil {
		problem = "Models could not be loaded. Your current model is still available."
	}
	writeJSON(w, http.StatusOK, struct {
		Models   []openrouter.Model `json:"models"`
		Model    string             `json:"model"`
		Revision int64              `json:"revision"`
		Problem  string             `json:"problem,omitempty"`
	}{models, s.session.ContextProfile().ConfiguredModel, s.modelRevision, problem})
}

func (s *Server) handleModelSelect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID memory.SessionID `json:"sessionId"`
		Model     string           `json:"model"`
		Revision  *int64           `json:"revision"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a session, model and revision")
		return
	}
	if request.Model == "" || request.Revision == nil || *request.Revision < 0 {
		jsonError(w, http.StatusBadRequest, "model and revision are required")
		return
	}
	s.sessionMu.Lock()
	if s.selectingSession || s.activeTurns > 0 || s.session == nil || request.SessionID == "" || request.SessionID != s.activeSession.ID || *request.Revision != s.modelRevision {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "model_selection_conflict", "Chat is busy or changed; refresh and try again")
		return
	}
	s.selectingSession = true
	s.sessionMu.Unlock()
	defer func() { s.sessionMu.Lock(); s.selectingSession = false; s.sessionMu.Unlock() }()
	opened, err := s.contextSessions.(ModelController).SelectModel(r.Context(), request.SessionID, *request.Revision, request.Model)
	if err != nil {
		status := http.StatusUnprocessableEntity
		message := "This model could not be selected. Check model availability and context or reasoning settings, then try again."
		if errors.Is(err, eviedb.ErrSessionModelChanged) || errors.Is(err, eviedb.ErrTurnLeaseHeld) || errors.Is(err, eviedb.ErrSessionNotActive) {
			status, message = http.StatusConflict, "Chat is busy or changed; refresh and try again"
		}
		managementJSONError(w, status, "model_selection_failed", message)
		return
	}
	s.sessionMu.Lock()
	s.session, s.activeSession, s.modelRevision = opened.Agent, opened.Session, opened.ModelRevision
	s.sessionMu.Unlock()
	writeJSON(w, http.StatusOK, eviedb.SessionModel{Model: request.Model, Revision: opened.ModelRevision})
}
