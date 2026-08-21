// Package agent is the harness core: the model↔tools loop, extracted from
// any particular frontend. A Session holds one conversation; frontends
// (terminal REPL, web server) drive it through Send and receive everything
// worth rendering through the Events interface. This package never prints —
// the repo's "domain layer silent, frontends own output" convention applied
// to the agent itself.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

const DefaultModel = "moonshotai/kimi-k3"

var ErrBusy = errors.New("agent: a turn is already in progress")

type Session struct {
	mu        sync.Mutex
	client    Client
	model     string
	reasoning *openrouter.ReasoningConfig
	history   History
	scope     memory.ScopeContext
}
type Client interface {
	ChatStream(
		ctx context.Context,
		req openrouter.ChatRequest,
		h openrouter.StreamHandlers,
	) (openrouter.ChatResponse, error)
}
type Events interface {
	Delta(text string)                         // streaming assistant text
	Reasoning(text string)                     // streaming thinking text
	ReasoningDone()                            // thinking ended for this assistant message
	AssistantDone(content string)              // every assistant message, even empty (tool-only)
	ToolCall(id, name, args string)            // emitted immediately before executing
	ToolResult(id, content string, isErr bool) // tool finished (includes declines)
}

func (s *Session) requestMessages(
	ctx context.Context,
) ([]openrouter.Message, error) {
	events, err := s.history.Events(ctx)
	if err != nil {
		return nil, fmt.Errorf("load durable history: %w", err)
	}

	conversation, err := messagesFromEvents(events)
	if err != nil {
		return nil, fmt.Errorf("project durable history: %w", err)
	}

	messages := make([]openrouter.Message, 0, len(conversation)+1)
	messages = append(messages, openrouter.Message{
		Role:    "system",
		Content: systemPrompt,
	})
	messages = append(messages, conversation...)

	return messages, nil
}

func (s *Session) Send(
	ctx context.Context,
	input string,
	ev Events,
	approve tools.Approver,
	extra ...tools.Tool,
) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()

	if _, err := s.history.Append(ctx, memory.EventInput{
		Type:    memory.EventUserMessage,
		Role:    memory.RoleUser,
		Content: input,
	}); err != nil {
		return fmt.Errorf("persist user message: %w", err)
	}

	for {
		messages, err := s.requestMessages(ctx)
		if err != nil {
			return err
		}

		req := openrouter.ChatRequest{
			Model:     s.model,
			Messages:  messages,
			Tools:     tools.SchemasWith(extra),
			Reasoning: s.reasoning,
		}
		thinking := false
		h := openrouter.StreamHandlers{
			OnReasoning: func(text string) {
				thinking = true
				ev.Reasoning(text)
			},
			OnContent: func(text string) {
				if thinking {
					thinking = false
					ev.ReasoningDone()
				}
				ev.Delta(text)
			},
		}

		res, err := s.client.ChatStream(ctx, req, h)
		if err != nil {
			return fmt.Errorf("chat request failed: %w", err)
		}
		if len(res.Choices) == 0 {
			return errors.New("agent: provider returned no choices")
		}

		msg := res.Choices[0].Message
		assistantInput, err := assistantEventInput(msg)
		if err != nil {
			return err
		}

		assistantEvent, err := s.history.Append(
			ctx,
			assistantInput,
		)
		if err != nil {
			return fmt.Errorf("persist assistant message: %w", err)
		}

		if thinking {
			ev.ReasoningDone()
		}
		ev.AssistantDone(msg.Content)

		if len(msg.ToolCalls) == 0 {
			return nil
		}

		for _, call := range msg.ToolCalls {
			executionUUID, err := uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("generate execution ID: %w", err)
			}
			executionID := memory.ExecutionID(executionUUID.String())
			intentInput, err := toolIntentInput(assistantEvent.ID, executionID, call)
			if err != nil {
				return err
			}
			intentEvent, err := s.history.Append(ctx, intentInput)
			if err != nil {
				return fmt.Errorf("persist tool intent: %w", err)
			}

			ev.ToolCall(call.ID, call.Function.Name, call.Function.Arguments)
			var approvalEventID memory.EventID
			var approvalDecision tools.Decision

			observeApproval := func(decision tools.Decision) error {
				input, err := approvalEventInput(
					intentEvent.ID,
					executionID,
					decision,
				)
				if err != nil {
					return err
				}
				approvalEvent, err := s.history.Append(ctx, input)
				if err != nil {
					return fmt.Errorf("persist approval: %w", err)
				}

				approvalEventID = approvalEvent.ID
				approvalDecision = decision
				return nil
			}

			result, isErr, err := tools.ExecuteWithApproval(
				extra,
				call,
				approve,
				observeApproval,
			)
			if err != nil {
				return fmt.Errorf("execute tool lifecycle: %w", err)
			}

			outcomeParentID := intentEvent.ID
			outcomeType := memory.EventToolSucceeded
			if isErr {
				outcomeType = memory.EventToolFailed
			}
			if approvalEventID != "" {
				outcomeParentID = approvalEventID
				if approvalDecision != tools.Approved {
					outcomeType = memory.EventToolCancelled
				}
			}

			outcomeInput, err := toolOutcomeInput(
				outcomeParentID,
				executionID,
				result,
				outcomeType,
			)
			if err != nil {
				return err
			}
			if _, err := s.history.Append(ctx, outcomeInput); err != nil {
				return fmt.Errorf("persist tool outcome: %w", err)
			}
			ev.ToolResult(call.ID, result.Content, isErr)
		}
	}
}

func resolveReasoning(v string) *openrouter.ReasoningConfig {
	switch v {
	case "off":
		return nil
	case "high", "medium", "low":
		return &openrouter.ReasoningConfig{Effort: v}
	default:
		return &openrouter.ReasoningConfig{Enabled: true}
	}
}

func New(
	client Client,
	model string,
	history History,
	scope memory.ScopeContext,
) *Session {
	if model == "" {
		model = os.Getenv("EVIE_MODEL")
	}
	if model == "" {
		model = DefaultModel
	}
	return &Session{
		client: client,
		model:  model,
		reasoning: resolveReasoning(
			os.Getenv("EVIE_REASONING"),
		),
		scope:   scope,
		history: history,
	}
}

func assistantEventInput(msg openrouter.Message) (
	memory.EventInput, error,
) {
	payload := memory.AssistantMessagePayload{
		ToolCalls: make([]memory.ToolCall, len(msg.ToolCalls)),
	}

	for i, call := range msg.ToolCalls {
		payload.ToolCalls[i] = memory.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		}
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return memory.EventInput{}, fmt.Errorf("encode assistant payload: %w", err)
	}

	return memory.EventInput{
		Type:    memory.EventAssistantMessage,
		Role:    memory.RoleAssistant,
		Content: msg.Content,
		Payload: payloadJSON,
	}, nil
}

func toolIntentInput(
	parentID memory.EventID,
	executionID memory.ExecutionID,
	call openrouter.ToolCall,
) (memory.EventInput, error) {
	payloadJSON, err := json.Marshal(memory.ToolIntentPayload{
		Call: memory.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		},
	})
	if err != nil {
		return memory.EventInput{}, fmt.Errorf("encode tool intent payload: %w", err)
	}

	return memory.EventInput{
		ParentID:    parentID,
		Type:        memory.EventToolIntent,
		ExecutionID: executionID,
		Payload:     payloadJSON,
	}, nil
}

func toolOutcomeInput(
	parentID memory.EventID,
	executionID memory.ExecutionID,
	result openrouter.Message,
	eventType memory.EventType,
) (memory.EventInput, error) {
	switch eventType {
	case memory.EventToolSucceeded,
		memory.EventToolFailed,
		memory.EventToolCancelled:
	default:
		return memory.EventInput{}, fmt.Errorf(
			"invalid tool outcome type %q",
			eventType,
		)
	}

	payloadJSON, err := json.Marshal(memory.ToolResultPayload{
		ToolCallID: result.ToolCallID,
		IsError:    eventType == memory.EventToolFailed,
	})
	if err != nil {
		return memory.EventInput{}, fmt.Errorf("encode tool outcome payload: %w", err)
	}

	return memory.EventInput{
		ParentID:    parentID,
		Type:        eventType,
		Role:        memory.RoleTool,
		ExecutionID: executionID,
		Content:     result.Content,
		Payload:     payloadJSON,
	}, nil
}

func approvalEventInput(
	parentID memory.EventID,
	executionID memory.ExecutionID,
	decision tools.Decision,
) (memory.EventInput, error) {
	var storedDecision memory.ApprovalDecision

	switch decision {
	case tools.Approved:
		storedDecision = memory.ApprovalApproved
	case tools.Declined:
		storedDecision = memory.ApprovalDeclined
	case tools.Expired:
		storedDecision = memory.ApprovalExpired
	default:
		return memory.EventInput{}, fmt.Errorf(
			"unsupported approval decision %d",
			decision,
		)
	}

	payloadJSON, err := json.Marshal(memory.ApprovalPayload{
		Decision: storedDecision,
	})
	if err != nil {
		return memory.EventInput{}, fmt.Errorf("encode approval payload: %w", err)
	}

	return memory.EventInput{
		ParentID:    parentID,
		Type:        memory.EventApproval,
		ExecutionID: executionID,
		Payload:     payloadJSON,
	}, nil
}
