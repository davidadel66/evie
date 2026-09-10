package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/davidadel66/evie/internal/memory"
)

type historyApproval struct {
	State     string `json:"state"`
	RequestID string `json:"reqId"`
}
type historyItem struct {
	Turn      *activityTurn    `json:"turn,omitempty"`
	Phase     string           `json:"phase,omitempty"`
	Kind      string           `json:"kind"`
	Key       string           `json:"key"`
	Text      string           `json:"text,omitempty"`
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Args      string           `json:"args,omitempty"`
	Result    *string          `json:"result,omitempty"`
	IsError   bool             `json:"isErr,omitempty"`
	Approval  *historyApproval `json:"approval,omitempty"`
	Streaming bool             `json:"streaming"`
	StartedAt int64            `json:"startedAt"`
	Tone      string           `json:"tone,omitempty"`
	Memory    *memoryActivity  `json:"memory,omitempty"`
}

func (s *Server) handleContextSessionHistory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID memory.SessionID `json:"sessionId"`
		Before    string           `json:"before"`
	}
	if status, err := decodeManagementJSON(w, r, &req); err != nil || req.SessionID == "" {
		if status == 0 {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "A selected session ID is required")
		return
	}
	before := 0
	if req.Before != "" {
		var err error
		before, err = strconv.Atoi(req.Before)
		if err != nil || before <= 0 {
			jsonError(w, 400, "Invalid history cursor")
			return
		}
	}
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	if s.selectingSession || s.activeTurns > 0 || s.activeSession.ID != req.SessionID || s.session == nil {
		managementJSONError(w, 409, "history_session_changed", "Conversation changed or a turn is still running. Try again.")
		return
	}
	events, err := s.session.HistoryEvents(r.Context())
	if err != nil {
		managementJSONError(w, 409, "history_unavailable", "Conversation history could not be loaded. Try again.")
		return
	}
	items, err := projectHistory(events)
	if err != nil {
		managementJSONError(w, 500, "history_unavailable", "Conversation history could not be loaded.")
		return
	}
	end := len(items)
	if before > 0 {
		if before > end {
			jsonError(w, 400, "Invalid history cursor")
			return
		}
		end = before
	}
	start := max(0, end-100)
	next := ""
	if start > 0 {
		next = strconv.Itoa(start)
	}
	writeJSON(w, 200, struct {
		SessionID memory.SessionID `json:"sessionId"`
		Items     []historyItem    `json:"items"`
		Before    string           `json:"before,omitempty"`
	}{req.SessionID, items[start:end], next})
}

// Project persisted events, not the compacted model view. Tool outcomes and
// decisions attach only to their durable execution; old approvals cannot act.
func projectHistory(events []memory.Event) ([]historyItem, error) {
	items := make([]historyItem, 0)
	tools := map[memory.ExecutionID]int{}
	roots := map[memory.EventID]*activityTurn{}
	var current *activityTurn
	for _, e := range events {
		if e.Type == memory.EventUserMessage {
			current = &activityTurn{ID: string(e.ID), StartedAt: activityTimestamp(e.RecordedAt), Status: "incomplete"}
		}
		turn := roots[e.ParentID]
		if turn == nil {
			turn = current
		} // Legacy records without parent links.
		roots[e.ID] = turn
		first := len(items)
		switch e.Type {
		case memory.EventContextSnapshot:
			activity, err := projectMemoryActivity(e)
			if err != nil {
				return nil, err
			}
			if activity != nil {
				items = append(items, historyItem{Kind: "memory", Key: string(e.ID), Memory: activity})
			}
		case memory.EventUserMessage:
			items = append(items, historyItem{Kind: "user", Key: string(e.ID), Text: e.Content})
		case memory.EventAssistantMessage:
			parts, terminal, err := assistantActivity(e)
			if err != nil {
				return nil, err
			}
			for i, part := range parts {
				items = append(items, historyItem{Kind: "assistant", Key: string(e.ID) + "-part-" + strconv.Itoa(i), Text: part.Text, Phase: part.Phase})
			}
			if terminal && turn != nil {
				turn.Status = "complete"
				turn.FinishedAt = activityTimestamp(e.RecordedAt)
			}
		case memory.EventToolIntent:
			var p memory.ToolIntentPayload
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return nil, err
			}
			tools[e.ExecutionID] = len(items)
			items = append(items, historyItem{Kind: "tool", Key: string(e.ID), ID: p.Call.ID, Name: p.Call.Name, Args: p.Call.Arguments, StartedAt: activityTimestamp(e.RecordedAt)})
		case memory.EventToolSucceeded, memory.EventToolFailed, memory.EventToolCancelled:
			if i, ok := tools[e.ExecutionID]; ok {
				result := e.Content
				items[i].Result = &result
				items[i].IsError = e.Type != memory.EventToolSucceeded
			}
		case memory.EventApproval:
			if i, ok := tools[e.ExecutionID]; ok {
				var p memory.ApprovalPayload
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return nil, err
				}
				state := string(p.Decision)
				if state != "approved" && state != "declined" {
					state = "expired"
				}
				items[i].Approval = &historyApproval{State: state}
			}
		case memory.EventTurnFailed, memory.EventTurnInterrupted:
			if turn != nil {
				turn.Status = "incomplete"
				turn.FinishedAt = activityTimestamp(e.RecordedAt)
			}
			items = append(items, historyItem{Kind: "notice", Key: string(e.ID), Text: "This turn did not complete.", Tone: "warning"})
		}
		for i := first; i < len(items); i++ {
			items[i].Turn = turn
		}
	}
	for i := range items {
		if items[i].Kind == "tool" && items[i].Result == nil {
			text := "No completed result was recorded."
			items[i].Result = &text
			items[i].IsError = true
		}
	}
	return items, nil
}
