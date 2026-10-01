package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// readStoredToolResult backs read_tool_result. Projection and compaction keep
// a result's event ID in context; this returns its admitted durable content so
// the model can recover the full output without repeating a possibly
// side-effecting call. Only tool outcomes in this session's own durable
// history qualify.
func (s *Session) readStoredToolResult(ctx context.Context, id memory.EventID) (tools.StoredToolResult, error) {
	events, err := s.history.Events(ctx)
	if err != nil {
		return tools.StoredToolResult{}, fmt.Errorf("load conversation history: %w", err)
	}
	index := -1
	for i, event := range events {
		if event.ID == id && (event.SessionID == "" || s.scope.SessionID == "" || event.SessionID == s.scope.SessionID) {
			index = i
			break
		}
	}
	if index < 0 {
		return tools.StoredToolResult{}, fmt.Errorf("no tool result with event_id %q in this conversation", id)
	}
	event := events[index]
	status := map[memory.EventType]string{
		memory.EventToolSucceeded: "succeeded", memory.EventToolFailed: "failed", memory.EventToolCancelled: "cancelled",
	}[event.Type]
	if status == "" {
		return tools.StoredToolResult{}, fmt.Errorf("event_id %q is not a tool result", id)
	}
	var result memory.ToolResultPayload
	if err := decodeEventPayload(event, &result); err != nil {
		return tools.StoredToolResult{}, err
	}
	// Provider call IDs may repeat across turns, so the call is the nearest
	// earlier assistant request that names it.
	name := ""
	for i := index - 1; i >= 0 && name == ""; i-- {
		if events[i].Type != memory.EventAssistantMessage {
			continue
		}
		var assistant memory.AssistantMessagePayload
		if err := json.Unmarshal(events[i].Payload, &assistant); err != nil {
			continue
		}
		for _, call := range assistant.ToolCalls {
			if call.ID == result.ToolCallID {
				name = call.Name
				break
			}
		}
	}
	return tools.StoredToolResult{EventID: event.ID, ToolName: name, Status: status, Content: event.Content}, nil
}
