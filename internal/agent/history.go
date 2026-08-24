package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type History interface {
	Append(
		ctx context.Context,
		lease memory.TurnLease,
		input memory.EventInput,
	) (memory.Event, error)
	Events(ctx context.Context) ([]memory.Event, error)
}

func messagesFromEvents(events []memory.Event) ([]openrouter.Message, error) {
	var messages []openrouter.Message
	omit, err := incompleteToolGroupEvents(events)
	if err != nil {
		return nil, err
	}

	for i, event := range events {
		if omit[i] {
			continue
		}
		switch event.Type {
		case memory.EventUserMessage:
			if event.Role != memory.RoleUser {
				return nil, fmt.Errorf("user event %q has role %q", event.ID, event.Role)
			}
			messages = append(messages, openrouter.Message{
				Role:    "user",
				Content: event.Content,
			})
		case memory.EventAssistantMessage:
			if event.Role != memory.RoleAssistant {
				return nil, fmt.Errorf("assistant event %q has role %q", event.ID, event.Role)
			}

			var payload memory.AssistantMessagePayload
			if err := decodeEventPayload(event, &payload); err != nil {
				return nil, err
			}

			var toolCalls []openrouter.ToolCall
			for _, call := range payload.ToolCalls {
				if call.ID == "" || call.Name == "" {
					return nil, fmt.Errorf("assistant event %q contains an incomplete tool call", event.ID)
				}
				toolCalls = append(toolCalls, openrouter.ToolCall{
					ID:   call.ID,
					Type: "function",
					Function: openrouter.FunctionCall{
						Name:      call.Name,
						Arguments: call.Arguments,
					},
				})
			}

			messages = append(messages, openrouter.Message{
				Role:      "assistant",
				Content:   event.Content,
				ToolCalls: toolCalls,
			})

		case memory.EventToolSucceeded,
			memory.EventToolFailed,
			memory.EventToolCancelled:
			if event.Role != memory.RoleTool {
				return nil, fmt.Errorf("tool event %q has role %q", event.ID, event.Role)
			}

			var payload memory.ToolResultPayload
			if err := decodeEventPayload(event, &payload); err != nil {
				return nil, err
			}

			if payload.ToolCallID == "" {
				return nil, fmt.Errorf("tool event %q has no tool call ID", event.ID)
			}

			messages = append(messages, openrouter.Message{
				Role:       "tool",
				Content:    event.Content,
				ToolCallID: payload.ToolCallID,
			})

		case memory.EventToolIntent,
			memory.EventApproval,
			memory.EventExecutionResolved,
			memory.EventTurnFailed,
			memory.EventTurnInterrupted:
			continue

		default:
			return nil, fmt.Errorf("unsupported history event type %q", event.Type)

		}
	}
	return messages, nil
}

func incompleteToolGroupEvents(events []memory.Event) (map[int]bool, error) {
	omit := make(map[int]bool)
	for i, event := range events {
		if event.Type != memory.EventAssistantMessage {
			continue
		}
		var payload memory.AssistantMessagePayload
		if err := decodeEventPayload(event, &payload); err != nil {
			return nil, err
		}
		if len(payload.ToolCalls) == 0 {
			continue
		}

		requested := make(map[string]bool, len(payload.ToolCalls))
		for _, call := range payload.ToolCalls {
			if call.ID == "" || call.Name == "" {
				return nil, fmt.Errorf("assistant event %q contains an incomplete tool call", event.ID)
			}
			if _, exists := requested[call.ID]; exists {
				return nil, fmt.Errorf("assistant event %q repeats tool call ID %q", event.ID, call.ID)
			}
			requested[call.ID] = false
		}
		groupEnd := len(events)
		for j := i + 1; j < len(events); j++ {
			if events[j].Type == memory.EventAssistantMessage || events[j].Type == memory.EventUserMessage {
				groupEnd = j
				break
			}
			if events[j].Type != memory.EventToolSucceeded &&
				events[j].Type != memory.EventToolFailed &&
				events[j].Type != memory.EventToolCancelled {
				continue
			}
			var result memory.ToolResultPayload
			if err := decodeEventPayload(events[j], &result); err != nil {
				return nil, err
			}
			if _, ok := requested[result.ToolCallID]; ok {
				requested[result.ToolCallID] = true
			}
		}
		complete := true
		for _, terminal := range requested {
			complete = complete && terminal
		}
		if complete {
			continue
		}
		omit[i] = true
		for j := i + 1; j < groupEnd; j++ {
			if events[j].Type != memory.EventToolSucceeded &&
				events[j].Type != memory.EventToolFailed &&
				events[j].Type != memory.EventToolCancelled {
				continue
			}
			var result memory.ToolResultPayload
			if err := decodeEventPayload(events[j], &result); err != nil {
				return nil, err
			}
			if _, ok := requested[result.ToolCallID]; ok {
				omit[j] = true
			}
		}
	}
	return omit, nil
}

func decodeEventPayload(event memory.Event, destination any) error {
	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("decode %s event %q payload: %w", event.Type, event.ID, err)
	}
	return nil
}
