package agent

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func historyPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal history payload: %v", err)
	}
	return payload
}

func TestMessagesFromEventsProjectsConversationAndOmitsOperations(t *testing.T) {
	call := memory.ToolCall{ID: "call-1", Name: "time", Arguments: `{}`}
	events := []memory.Event{
		{Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "what time is it?"},
		{
			Sequence: 2,
			Type:     memory.EventAssistantMessage,
			Role:     memory.RoleAssistant,
			Payload:  historyPayload(t, memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{call}}),
		},
		{
			Sequence:    3,
			Type:        memory.EventToolIntent,
			ParentID:    "assistant-event",
			Payload:     historyPayload(t, memory.ToolIntentPayload{Call: call}),
			ExecutionID: "execution-1",
		},
		{Sequence: 4, Type: memory.EventApproval, ExecutionID: "execution-1", Payload: json.RawMessage(`{"approved":true}`)},
		{
			Sequence:    5,
			Type:        memory.EventToolSucceeded,
			Role:        memory.RoleTool,
			ExecutionID: "execution-1",
			Content:     "12:00 PM",
			Payload:     historyPayload(t, memory.ToolResultPayload{ToolCallID: "call-1"}),
		},
		{Sequence: 6, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "It is 12:00 PM.", Payload: json.RawMessage(`{}`)},
	}

	got, err := messagesFromEvents(events)
	if err != nil {
		t.Fatalf("messagesFromEvents: %v", err)
	}
	want := []openrouter.Message{
		{Role: "user", Content: "what time is it?"},
		{
			Role: "assistant",
			ToolCalls: []openrouter.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: openrouter.FunctionCall{
					Name:      "time",
					Arguments: `{}`,
				},
			}},
		},
		{Role: "tool", Content: "12:00 PM", ToolCallID: "call-1"},
		{Role: "assistant", Content: "It is 12:00 PM."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("messages = %#v, want %#v", got, want)
	}
}

func TestMessagesFromEventsRejectsInvalidConversationData(t *testing.T) {
	tests := []struct {
		name  string
		event memory.Event
	}{
		{
			name: "malformed assistant payload",
			event: memory.Event{
				Type:    memory.EventAssistantMessage,
				Payload: json.RawMessage(`{`),
			},
		},
		{
			name: "tool result without call ID",
			event: memory.Event{
				Type:    memory.EventToolSucceeded,
				Payload: json.RawMessage(`{}`),
			},
		},
		{
			name:  "unknown event type",
			event: memory.Event{Type: memory.EventType("future_unknown")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if messages, err := messagesFromEvents([]memory.Event{tt.event}); err == nil {
				t.Fatalf("invalid event produced messages %#v", messages)
			}
		})
	}
}

func TestMessagesFromEventsOmitsIncompleteToolGroupsWhole(t *testing.T) {
	for _, tt := range []struct {
		name     string
		calls    []memory.ToolCall
		outcomes []memory.ToolCall
	}{
		{
			name:  "single missing outcome",
			calls: []memory.ToolCall{{ID: "c1", Name: "one", Arguments: `{}`}},
		},
		{
			name: "multi partial outcome",
			calls: []memory.ToolCall{
				{ID: "c1", Name: "one", Arguments: `{}`},
				{ID: "c2", Name: "two", Arguments: `{}`},
			},
			outcomes: []memory.ToolCall{{ID: "c1", Name: "one"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := []memory.Event{
				{ID: "u1", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "first"},
				{ID: "a1", Sequence: 2, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: tt.calls})},
			}
			for i, outcome := range tt.outcomes {
				events = append(events, memory.Event{
					ID: memory.EventID("result-" + outcome.ID), Sequence: int64(3 + i),
					Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: "partial result",
					Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: outcome.ID}),
				})
			}
			events = append(events,
				memory.Event{ID: "u2", Sequence: 10, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "continue"},
				memory.Event{ID: "a2", Sequence: 11, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "continued", Payload: json.RawMessage(`{}`)},
			)
			messages, err := messagesFromEvents(events)
			if err != nil {
				t.Fatal(err)
			}
			want := []openrouter.Message{
				{Role: "user", Content: "first"},
				{Role: "user", Content: "continue"},
				{Role: "assistant", Content: "continued"},
			}
			if !reflect.DeepEqual(messages, want) {
				t.Fatalf("messages=%#v want=%#v", messages, want)
			}
		})
	}
}

func TestMessagesFromEventsRetainsCompleteMultiToolGroupInDurableOrder(t *testing.T) {
	calls := []memory.ToolCall{
		{ID: "c1", Name: "one", Arguments: `{"n":1}`},
		{ID: "c2", Name: "two", Arguments: `{"n":2}`},
	}
	events := []memory.Event{
		{ID: "u1", Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run both"},
		{ID: "a1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: calls})},
		{ID: "r1", Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: "one result", Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: "c1"})},
		{ID: "r2", Type: memory.EventToolFailed, Role: memory.RoleTool, Content: "two result", Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: "c2", IsError: true})},
	}
	messages, err := messagesFromEvents(events)
	if err != nil {
		t.Fatal(err)
	}
	if roles := rolesOf(messages); roles != "user,assistant,tool,tool" {
		t.Fatalf("roles=%s messages=%+v", roles, messages)
	}
	if messages[2].ToolCallID != "c1" || messages[3].ToolCallID != "c2" {
		t.Fatalf("tool order=%+v", messages[2:])
	}
}
