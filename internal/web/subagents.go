package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// SubagentReader is the read-only view of the children a conversation
// delegated (chat-activity.spec.md, subagent inspector amendment). Child
// sessions stay fenced from being opened as conversations; the owner sees
// them only through the parent conversation that started them.
type SubagentReader interface {
	SubagentsOfCall(ctx context.Context, session memory.SessionID, turn memory.EventID, callID string) ([]eviedb.SubagentProgress, error)
	SubagentEvents(ctx context.Context, session memory.SessionID, executionID string) (eviedb.SubagentProgress, []memory.Event, error)
}

// WithSubagents adds the subagent progress routes.
func WithSubagents(server *Server, reader SubagentReader) *Server {
	server.subagents = reader
	return server
}

// A child's trace is for watching it work, not for reading every page it
// fetched: tool arguments and results are cut, and the child's own text
// (its notes and report) gets a larger bound.
const (
	subagentTraceToolBytes = 4 << 10
	subagentTraceTextBytes = 64 << 10
)

func (s *Server) registerSubagentRoutes(mux *http.ServeMux) {
	if s.subagents == nil {
		return
	}
	mux.Handle("/api/subagents/list", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionID memory.SessionID `json:"sessionId"`
			TurnID    memory.EventID   `json:"turnId"`
			CallID    string           `json:"callId"`
		}
		if status, err := decodeManagementJSON(w, r, &req); err != nil || !boundedID(string(req.SessionID)) || !boundedID(req.CallID) || len(req.TurnID) > 128 {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, "A conversation and tool call are required")
			return
		}
		if !s.isActiveSession(req.SessionID) {
			managementJSONError(w, http.StatusConflict, "subagents_session_changed", "Conversation changed; reopen the subagents")
			return
		}
		list, err := s.subagents.SubagentsOfCall(r.Context(), req.SessionID, req.TurnID, req.CallID)
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "subagents_unavailable", "Subagents could not be read")
			return
		}
		if list == nil {
			list = []eviedb.SubagentProgress{}
		}
		writeJSON(w, http.StatusOK, struct {
			Subagents []eviedb.SubagentProgress `json:"subagents"`
		}{list})
	}))
	mux.Handle("/api/subagents/trace", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SessionID   memory.SessionID `json:"sessionId"`
			ExecutionID string           `json:"executionId"`
		}
		if status, err := decodeManagementJSON(w, r, &req); err != nil || !boundedID(string(req.SessionID)) || !boundedID(req.ExecutionID) {
			if err == nil {
				status = http.StatusBadRequest
			}
			jsonError(w, status, "A conversation and subagent are required")
			return
		}
		if !s.isActiveSession(req.SessionID) {
			managementJSONError(w, http.StatusConflict, "subagents_session_changed", "Conversation changed; reopen the subagent")
			return
		}
		progress, events, err := s.subagents.SubagentEvents(r.Context(), req.SessionID, req.ExecutionID)
		if errors.Is(err, sql.ErrNoRows) {
			managementJSONError(w, http.StatusNotFound, "subagent_not_found", "No such subagent in this conversation")
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "subagents_unavailable", "The subagent could not be read")
			return
		}
		live := progress.State == "admitted" || progress.State == "running"
		items, err := projectEvents(events, !live)
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "subagents_unavailable", "The subagent could not be read")
			return
		}
		for i := range items {
			item := &items[i]
			item.Turn = nil
			if item.Kind == "tool" {
				item.Args = eviedb.BoundToolArguments(item.Args, subagentTraceToolBytes)
				if item.Result != nil {
					result := boundTraceText(*item.Result, subagentTraceToolBytes)
					item.Result = &result
				}
				continue
			}
			item.Text = boundTraceText(item.Text, subagentTraceTextBytes)
		}
		writeJSON(w, http.StatusOK, struct {
			Subagent eviedb.SubagentProgress `json:"subagent"`
			Items    []historyItem           `json:"items"`
		}{progress, items})
	}))
}

// isActiveSession reports whether id is the owner's selected conversation.
// Subagent reads follow the other per-conversation reads: only the open one.
func (s *Server) isActiveSession(id memory.SessionID) bool {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return !s.selectingSession && s.activeSession.ID != "" && s.activeSession.ID == id
}

func boundTraceText(value string, n int) string {
	if len(value) <= n {
		return value
	}
	for n > 0 && !utf8.RuneStart(value[n]) {
		n--
	}
	return value[:n] + "\n… (cut for display)"
}
