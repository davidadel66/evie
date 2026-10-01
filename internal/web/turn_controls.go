// Turn controls: the owner's explicit stop for a running web turn and manual
// context compaction for the active conversation. A browser disconnect still
// never cancels a turn (serve.decisions.md 2026-08-23); only POST /api/cancel
// does, and only for the conversation it names.
package web

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

// errTurnStopped is the cancellation cause of an owner-requested stop. It
// separates a stop from runtime shutdown, which cancels the same turn context
// with a different cause.
var errTurnStopped = errors.New("turn stopped by the owner")

// webTurn is one running chat turn's stop control. The agent turns its
// context cancellation into the ordinary caller-cancel path: durable
// turn_interrupted evidence, cancelled tools, expired approvals, and a
// released lease.
type webTurn struct {
	stop context.CancelCauseFunc
}

// beginWebTurn registers the stop control for key's running turn. It runs
// under sessionMu and reports false when that conversation already has one,
// so a second request cannot replace the control of the turn that is running.
func (s *Server) beginWebTurn(key memory.SessionID, turn *webTurn) bool {
	if _, running := s.turns[key]; running {
		return false
	}
	s.turns[key] = turn
	return true
}

// finishWebTurn removes turn's stop control once its Send has returned. It is
// safe to call more than once.
func (s *Server) finishWebTurn(key memory.SessionID, turn *webTurn) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.turns[key] == turn {
		delete(s.turns, key)
	}
}

// stoppedByOwner reports whether Send ended only because of an owner stop.
// Send returns the turn context's own error for a caller interruption; a
// joined terminal-evidence or lease-release failure is not a clean stop and
// keeps the ordinary error presentation.
func stoppedByOwner(turnCtx context.Context, sendErr error) bool {
	return sendErr == context.Canceled && errors.Is(context.Cause(turnCtx), errTurnStopped)
}

// handleCancel stops the identified conversation's running turn. Repeated
// stops while the turn unwinds are accepted again; a conversation with no
// running turn gets a typed 409 rather than silently succeeding.
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &req); err != nil {
		jsonError(w, status, "body must be one JSON object with an optional sessionId")
		return
	}
	s.sessionMu.Lock()
	turn := s.turns[req.SessionID]
	s.sessionMu.Unlock()
	if turn == nil {
		managementJSONError(w, http.StatusConflict, "no_active_turn", "no turn is running in this conversation")
		return
	}
	turn.stop(errTurnStopped)
	writeJSON(w, http.StatusAccepted, struct {
		Status string `json:"status"`
	}{"stopping"})
}

type compactResponse struct {
	Outcome string         `json:"outcome"`
	EventID memory.EventID `json:"eventId,omitempty"`
}

// handleCompact runs the agent's manual compaction for the active
// conversation. It is refused while a turn is running there and holds the
// conversation like a turn, so session and model selection wait for it.
func (s *Server) handleCompact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID memory.SessionID `json:"sessionId"`
	}
	if status, err := decodeManagementJSON(w, r, &req); err != nil {
		jsonError(w, status, "body must be one JSON object with an optional sessionId")
		return
	}
	s.sessionMu.Lock()
	if s.selectingSession || (s.contextSessions != nil && (req.SessionID == "" || req.SessionID != s.activeSession.ID)) {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "context_session_changed", "the active conversation changed; reload before compacting")
		return
	}
	if _, running := s.turns[s.activeSession.ID]; running {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "turn_in_progress", "finish or stop the current turn before compacting")
		return
	}
	session := s.session
	if session == nil {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusServiceUnavailable, "chat_unavailable", "chat is unavailable because the active Agent Preset is invalid")
		return
	}
	s.activeTurns++
	s.sessionMu.Unlock()
	defer func() {
		s.sessionMu.Lock()
		s.activeTurns--
		s.sessionMu.Unlock()
	}()

	result, err := session.Compact(r.Context())
	var compactionErr *agent.CompactionError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, compactResponse{Outcome: "compacted", EventID: result.CompactionEventID})
	case errors.Is(err, agent.ErrNothingEligibleForCompaction):
		writeJSON(w, http.StatusOK, compactResponse{Outcome: "nothing_to_compact"})
	case errors.Is(err, agent.ErrBusy), errors.Is(err, agent.ErrLeaseConflict):
		managementJSONError(w, http.StatusConflict, "session_busy", "the conversation is busy; try again when its current work finishes")
	case errors.Is(err, agent.ErrSessionUnavailable):
		managementJSONError(w, http.StatusConflict, "session_unavailable", "the conversation is unavailable")
	case errors.As(err, &compactionErr):
		// Only the stable classification crosses to the browser; provider
		// details stay in the local log.
		log.Printf("context compaction failed: %v", err)
		status := http.StatusBadGateway
		if compactionErr.Classification == memory.ClassificationContextOverflow {
			status = http.StatusUnprocessableEntity
		}
		compactionFailure(w, status, string(compactionErr.Classification))
	default:
		log.Printf("context compaction failed: %v", err)
		compactionFailure(w, http.StatusInternalServerError, "local_failure")
	}
}

func compactionFailure(w http.ResponseWriter, status int, classification string) {
	writeJSON(w, status, struct {
		Code           string `json:"code"`
		Classification string `json:"classification"`
		Error          string `json:"error"`
	}{"compaction_failed", classification, "context compaction failed; the conversation is unchanged"})
}
