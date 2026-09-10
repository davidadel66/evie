package web

import (
	"encoding/json"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type activityTurn struct {
	ID         string `json:"id"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	Status     string `json:"status"`
}

type publicAssistantPart struct {
	Text  string `json:"text"`
	Phase string `json:"phase"`
}

// Both live commitment and replay use this projection. A provider's phase
// cannot establish turn completion: only a tool-free accepted assistant can.
func assistantActivity(event memory.Event) ([]publicAssistantPart, bool, error) {
	var payload memory.AssistantMessagePayload
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, false, err
		}
	}
	if err := payload.ValidateTextParts(event.Content); err != nil {
		return nil, false, err
	}
	terminal := len(payload.ToolCalls) == 0
	parts := payload.TextParts
	if len(parts) == 0 && event.Content != "" {
		parts = []memory.AssistantTextPart{{Text: event.Content}}
	}
	public := make([]publicAssistantPart, 0, len(parts))
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		phase := "commentary"
		if terminal && part.Phase != "commentary" {
			phase = "final_answer"
		}
		public = append(public, publicAssistantPart{Text: part.Text, Phase: phase})
	}
	return public, terminal, nil
}

func activityTimestamp(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.UnixMilli()
}

func (e *sseEvents) TurnStarted(id memory.EventID, at time.Time) {
	e.emit("turn_started", activityTurn{ID: string(id), StartedAt: activityTimestamp(at), Status: "working"})
}

func (e *sseEvents) AssistantCommitted(event memory.Event) {
	parts, terminal, err := assistantActivity(event)
	if err != nil {
		// Stored payloads are validated at acceptance; fail visibly if corrupt.
		e.Error("The committed response could not be displayed. Reload the conversation.")
		return
	}
	finishedAt := int64(0)
	if terminal {
		finishedAt = activityTimestamp(event.RecordedAt)
	}
	e.emit("assistant_done", struct {
		Content    string                `json:"content"`
		Parts      []publicAssistantPart `json:"parts"`
		Terminal   bool                  `json:"terminal"`
		FinishedAt int64                 `json:"finishedAt,omitempty"`
	}{event.Content, parts, terminal, finishedAt})
}
