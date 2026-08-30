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
)

const DefaultModel = openrouter.BuiltinModel

var ErrBusy = errors.New("agent: a turn is already in progress")

type sessionUnavailableError struct{ cause error }

func (e sessionUnavailableError) Error() string {
	return fmt.Sprintf("acquire turn lease: %v", e.cause)
}

func (e sessionUnavailableError) Unwrap() []error {
	return []error{ErrSessionUnavailable, e.cause}
}

type Session struct {
	mu        sync.Mutex
	client    Client
	profile   openrouter.ContextProfile
	reasoning *openrouter.ReasoningConfig
	history   History
	scope     memory.ScopeContext
	owner     TurnOwnership
	composer  *ContextComposer
	timing    turnTiming
}
type Client interface {
	// ChatStream callbacks need not be synchronous with its return. Session
	// owns a per-call lifetime gate that closes admission and joins admitted
	// callbacks before it inspects the response or advances turn state.
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
	AssistantDone(content string)              // authoritative committed content for every assistant message, even empty (tool-only)
	ToolCall(id, name, args string)            // emitted immediately before executing
	ToolResult(id, content string, isErr bool) // tool finished (includes declines)
	ResponseDiscarded(reason DiscardReason, message string)
}

func (s *Session) Send(
	ctx context.Context,
	input string,
	ev Events,
	approve tools.Approver,
	extra ...tools.Tool,
) (retErr error) {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.owner == nil {
		return errors.New("agent: turn ownership is not configured")
	}

	lease, err := s.owner.Acquire(ctx, s.timing.leaseDuration)
	if err != nil {
		if s.owner.IsConflict(err) {
			return fmt.Errorf("%w: %v", ErrLeaseConflict, err)
		}
		if s.owner.IsSessionInactive(err) {
			return sessionUnavailableError{cause: err}
		}
		return fmt.Errorf("acquire turn lease: %w", err)
	}

	coordinator := newTurnCoordinator(ctx)
	coordinator.setStage(memory.StageTurnStart)
	defer coordinator.cancel()
	stopHeartbeat := s.startHeartbeat(ctx, coordinator, lease)
	defer func() {
		stopHeartbeat()
		cleanupCtx, cancel := s.timing.newCleanupContext(ctx, s.timing.cleanupTimeout)
		defer cancel()
		if releaseErr := s.owner.Release(cleanupCtx, lease); releaseErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release turn lease: %w", releaseErr))
		}
	}()

	if err := ctx.Err(); err != nil {
		coordinator.selectCause(callerCause(err), err, 0)
		return err
	}

	rootEvent, err := s.history.Append(coordinator.ctx, lease, memory.EventInput{
		Type:    memory.EventUserMessage,
		Role:    memory.RoleUser,
		Content: input,
	})
	if err != nil {
		if cause := coordinator.result(); cause.kind != causeNone {
			return cause.err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			coordinator.selectCause(callerCause(ctxErr), ctxErr, 0)
			return ctxErr
		}
		if s.owner.IsLeaseLost(err) {
			coordinator.selectCause(causeLeaseLost, err, 0)
			return fmt.Errorf("%w: %v", ErrLeaseLost, err)
		}
		coordinator.selectCause(causeStorage, err, 0)
		return fmt.Errorf("persist user message: %w", err)
	}
	progress := &turnProgress{rootTurnID: rootEvent.ID, requestParentID: rootEvent.ID}
	turnErr := s.runOwnedTurn(coordinator, lease, ev, approve, progress, extra)
	if turnErr != nil && coordinator.result().kind == causeNone {
		coordinator.selectCause(causeStorage, turnErr, 0)
	}

	cause := coordinator.result()
	if cause.kind == causeSuccess {
		return nil
	}
	if cause.err != nil {
		turnErr = cause.err
	}

	stopHeartbeat()
	if rootEvent.ID != "" && causeHasDurableTerminal(cause.kind) {
		terminalCtx, cancel := s.timing.newCleanupContext(ctx, s.timing.cleanupTimeout)
		terminalErr := s.appendTerminal(terminalCtx, lease, rootEvent.ID, progress.requestParentID, cause)
		cancel()
		if terminalErr != nil {
			turnErr = errors.Join(turnErr, fmt.Errorf("persist terminal event: %w", terminalErr))
		}
	}
	wasRendered, reasoningOpen, assistantCommitted := progress.rendered.discardState()
	if wasRendered && !assistantCommitted {
		if reason := cause.discardReason(); reason != "" {
			if reasoningOpen {
				ev.ReasoningDone()
			}
			ev.ResponseDiscarded(reason, DiscardedResponseMessage)
		}
	}
	return turnErr
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
	profile openrouter.ContextProfile,
	history History,
	scope memory.ScopeContext,
	owner TurnOwnership,
) *Session {
	return &Session{
		client:  client,
		profile: profile,
		reasoning: resolveReasoning(
			os.Getenv("EVIE_REASONING"),
		),
		scope:    scope,
		history:  history,
		owner:    owner,
		composer: NewContextComposer(CanonicalRequestEstimator{}),
		timing:   defaultTurnTiming,
	}
}

func (s *Session) ContextProfile() openrouter.ContextProfileDiagnostics {
	return s.profile.Diagnostics()
}

func assistantEventInput(msg openrouter.Message, usage *openrouter.TokenUsage) (
	memory.EventInput, error,
) {
	payload := memory.AssistantMessagePayload{
		ToolCalls: make([]memory.ToolCall, len(msg.ToolCalls)),
		Usage:     memoryUsage(usage),
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

func memoryUsage(usage *openrouter.TokenUsage) *memory.TokenUsage {
	if usage == nil {
		return nil
	}
	mapped := &memory.TokenUsage{
		InputTokens:           nonNegativeInt64(usage.InputTokens),
		OutputTokens:          nonNegativeInt64(usage.OutputTokens),
		TotalTokens:           nonNegativeInt64(usage.TotalTokens),
		ReasoningOutputTokens: nonNegativeInt64(usage.ReasoningOutputTokens),
		CachedInputTokens:     nonNegativeInt64(usage.CachedInputTokens),
		CacheWriteInputTokens: nonNegativeInt64(usage.CacheWriteInputTokens),
	}
	if mapped.InputTokens == nil && mapped.OutputTokens == nil && mapped.TotalTokens == nil &&
		mapped.ReasoningOutputTokens == nil && mapped.CachedInputTokens == nil &&
		mapped.CacheWriteInputTokens == nil {
		return nil
	}
	return mapped
}

func nonNegativeInt64(value *int64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	copy := *value
	return &copy
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
