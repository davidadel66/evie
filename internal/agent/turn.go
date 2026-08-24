package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type renderedOutput struct {
	mu                 sync.Mutex
	content            bool
	reasoning          bool
	reasoningOpen      bool
	assistantCommitted bool
}

type turnProgress struct {
	rendered        renderedOutput
	requestParentID memory.EventID
}

func (r *renderedOutput) discardState() (rendered, reasoningOpen, assistantCommitted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.content || r.reasoning, r.reasoningOpen, r.assistantCommitted
}

func (r *renderedOutput) begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.content = false
	r.reasoning = false
	r.reasoningOpen = false
	r.assistantCommitted = false
}

func (s *Session) startHeartbeat(
	caller context.Context,
	coordinator *turnCoordinator,
	lease memory.TurnLease,
) func() {
	heartbeatCtx, cancelHeartbeat := context.WithCancel(coordinator.ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.timing.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-caller.Done():
				coordinator.selectCause(callerCause(caller.Err()), caller.Err(), 0)
				return
			case <-ticker.C:
				_, err := s.owner.Heartbeat(heartbeatCtx, lease, s.timing.leaseDuration)
				if err == nil {
					continue
				}
				if heartbeatCtx.Err() != nil {
					if caller.Err() != nil && coordinator.result().kind == causeNone {
						coordinator.selectCause(callerCause(caller.Err()), caller.Err(), 0)
					}
					return
				}
				if coordinator.result().kind != causeNone {
					return
				}
				if caller.Err() != nil {
					coordinator.selectCause(callerCause(caller.Err()), caller.Err(), 0)
				} else if s.owner.IsLeaseLost(err) {
					coordinator.selectCause(causeLeaseLost, fmt.Errorf("%w: %v", ErrLeaseLost, err), 0)
				} else {
					coordinator.selectCause(causeHeartbeatFailed, fmt.Errorf("heartbeat turn lease: %w", err), 0)
				}
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancelHeartbeat()
			close(stop)
			<-done
		})
	}
}

func (s *Session) runOwnedTurn(
	coordinator *turnCoordinator,
	lease memory.TurnLease,
	ev Events,
	approve tools.Approver,
	progress *turnProgress,
	extra []tools.Tool,
) error {
	requestParentID := progress.requestParentID
	rendered := &progress.rendered
	for {
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
		}
		coordinator.setStage(memory.StageProvider)
		progress.requestParentID = requestParentID
		rendered.begin()

		messages, err := s.requestMessages(coordinator.ctx)
		if err != nil {
			return s.classifyLocalError(coordinator, err)
		}
		req := openrouter.ChatRequest{
			Model:     s.model,
			Messages:  messages,
			Tools:     tools.SchemasWith(extra),
			Reasoning: s.reasoning,
		}
		if err := s.owner.Authorize(coordinator.ctx, lease); err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("authorize provider start: %w", err))
		}
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
		}

		handlers := openrouter.StreamHandlers{
			OnReasoning: func(text string) {
				if !coordinator.active() || coordinator.ctx.Err() != nil {
					return
				}
				rendered.mu.Lock()
				rendered.reasoning = true
				rendered.reasoningOpen = true
				rendered.mu.Unlock()
				ev.Reasoning(text)
			},
			OnContent: func(text string) {
				if !coordinator.active() || coordinator.ctx.Err() != nil {
					return
				}
				rendered.mu.Lock()
				closeReasoning := rendered.reasoningOpen
				rendered.reasoningOpen = false
				rendered.content = true
				rendered.mu.Unlock()
				if closeReasoning {
					ev.ReasoningDone()
					if !coordinator.active() || coordinator.ctx.Err() != nil {
						return
					}
				}
				ev.Delta(text)
			},
		}

		res, err := s.client.ChatStream(coordinator.ctx, req, handlers)
		if err != nil {
			return s.classifyProviderError(coordinator, err)
		}
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
		}
		if len(res.Choices) == 0 {
			err := errors.New("agent: provider returned no choices")
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}

		msg := res.Choices[0].Message
		if err := validateAssistantResponse(msg); err != nil {
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}

		coordinator.setStage(memory.StageAssistantCommit)
		assistantInput, err := assistantEventInput(msg)
		if err != nil {
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}
		assistantInput.ParentID = requestParentID
		assistantEvent, err := s.history.Append(coordinator.ctx, lease, assistantInput)
		if err != nil {
			if coordinator.result().kind != causeNone {
				return coordinator.result().err
			}
			if ctxErr := coordinator.ctx.Err(); ctxErr != nil {
				coordinator.selectCause(callerCause(ctxErr), ctxErr, 0)
				return ctxErr
			}
			if s.owner.IsLeaseLost(err) {
				wrapped := fmt.Errorf("%w: %v", ErrLeaseLost, err)
				coordinator.selectCause(causeLeaseLost, wrapped, 0)
				return wrapped
			}
			wrapped := fmt.Errorf("persist assistant message: %w", err)
			coordinator.selectCause(causeAssistantPersistence, wrapped, 0)
			return wrapped
		}
		rendered.mu.Lock()
		rendered.assistantCommitted = true
		rendered.mu.Unlock()

		if len(msg.ToolCalls) == 0 {
			// A committed final assistant is durable success. This deliberately
			// wins over cancellation observed by a post-commit callback.
			coordinator.commitSuccess()
			s.emitAssistantAccepted(coordinator, ev, rendered, msg.Content, true)
			return nil
		}

		coordinator.setStage(memory.StageToolPrepare)
		if !s.emitAssistantAccepted(coordinator, ev, rendered, msg.Content, false) {
			return s.observeTurnContext(coordinator)
		}

		var lastOutcomeID memory.EventID
		for _, call := range msg.ToolCalls {
			coordinator.setStage(memory.StageToolPrepare)
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}
			executionUUID, err := uuid.NewRandom()
			if err != nil {
				coordinator.selectCause(causeStorage, err, 0)
				return fmt.Errorf("generate execution ID: %w", err)
			}
			executionID := memory.ExecutionID(executionUUID.String())
			intentInput, err := toolIntentInput(assistantEvent.ID, executionID, call)
			if err != nil {
				coordinator.selectCause(causeStorage, err, 0)
				return err
			}
			intentEvent, err := s.history.Append(coordinator.ctx, lease, intentInput)
			if err != nil {
				return s.classifyLocalError(coordinator, fmt.Errorf("persist tool intent: %w", err))
			}
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}
			ev.ToolCall(call.ID, call.Function.Name, call.Function.Arguments)
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}

			var approvalEventID memory.EventID
			var approvalDecision tools.Decision
			wrappedApprover := func(
				approvalCtx context.Context,
				name, args string,
				preview *tools.FileChangePreview,
			) tools.Decision {
				coordinator.setStage(memory.StageToolApproval)
				if approve == nil {
					return tools.Declined
				}
				return approve(approvalCtx, name, args, preview)
			}
			observeApproval := func(observeCtx context.Context, decision tools.Decision) error {
				input, err := approvalEventInput(intentEvent.ID, executionID, decision)
				if err != nil {
					return err
				}
				approvalEvent, err := s.history.Append(observeCtx, lease, input)
				if err != nil {
					return fmt.Errorf("persist approval: %w", err)
				}
				approvalEventID = approvalEvent.ID
				approvalDecision = decision
				if decision == tools.Approved {
					coordinator.setStage(memory.StageToolExecute)
				} else {
					coordinator.setStage(memory.StageToolCommit)
				}
				return nil
			}
			authorize := func(authorizeCtx context.Context, boundary tools.AuthorizationBoundary) error {
				switch boundary {
				case tools.AuthorizePreparation:
					coordinator.setStage(memory.StageToolPrepare)
				case tools.AuthorizeExecution:
					coordinator.setStage(memory.StageToolExecute)
				}
				return s.owner.Authorize(authorizeCtx, lease)
			}

			result, isErr, err := tools.ExecuteWithApprovalAuthorized(
				coordinator.ctx, extra, call, wrappedApprover, observeApproval, authorize,
			)
			if err != nil {
				return s.classifyLocalError(coordinator, fmt.Errorf("execute tool lifecycle: %w", err))
			}
			coordinator.setStage(memory.StageToolCommit)

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
			outcomeInput, err := toolOutcomeInput(outcomeParentID, executionID, result, outcomeType)
			if err != nil {
				coordinator.selectCause(causeStorage, err, 0)
				return err
			}
			outcomeEvent, err := s.history.Append(coordinator.ctx, lease, outcomeInput)
			if err != nil {
				return s.classifyLocalError(coordinator, fmt.Errorf("persist tool outcome: %w", err))
			}
			lastOutcomeID = outcomeEvent.ID
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}
			ev.ToolResult(call.ID, result.Content, isErr)
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}
		}
		requestParentID = lastOutcomeID
		progress.requestParentID = requestParentID
	}
}

func (c *turnCoordinator) commitSuccess() {
	c.mu.Lock()
	c.cause = terminalCause{kind: causeSuccess, stage: c.stage}
	c.mu.Unlock()
}

func (s *Session) emitAssistantAccepted(
	coordinator *turnCoordinator,
	ev Events,
	rendered *renderedOutput,
	content string,
	committedFinal bool,
) bool {
	if !committedFinal && (!coordinator.active() || coordinator.ctx.Err() != nil) {
		return false
	}
	rendered.mu.Lock()
	closeReasoning := rendered.reasoningOpen
	rendered.reasoningOpen = false
	rendered.mu.Unlock()
	if closeReasoning {
		ev.ReasoningDone()
		if !committedFinal && (!coordinator.active() || coordinator.ctx.Err() != nil) {
			return false
		}
	}
	if !committedFinal && (!coordinator.active() || coordinator.ctx.Err() != nil) {
		return false
	}
	ev.AssistantDone(content)
	return committedFinal || (coordinator.active() && coordinator.ctx.Err() == nil)
}

func (s *Session) observeTurnContext(coordinator *turnCoordinator) error {
	if cause := coordinator.result(); cause.kind != causeNone {
		return cause.err
	}
	if err := coordinator.ctx.Err(); err != nil {
		coordinator.selectCause(callerCause(err), err, 0)
		return err
	}
	return nil
}

func (s *Session) classifyLocalError(coordinator *turnCoordinator, err error) error {
	if cause := coordinator.result(); cause.kind != causeNone {
		return cause.err
	}
	if s.owner.IsLeaseLost(err) {
		wrapped := fmt.Errorf("%w: %v", ErrLeaseLost, err)
		coordinator.selectCause(causeLeaseLost, wrapped, 0)
		return wrapped
	}
	if coordinator.ctx.Err() != nil {
		ctxErr := coordinator.ctx.Err()
		coordinator.selectCause(callerCause(ctxErr), ctxErr, 0)
		return ctxErr
	}
	coordinator.selectCause(causeStorage, err, 0)
	return err
}

func (s *Session) classifyProviderError(coordinator *turnCoordinator, err error) error {
	if cause := coordinator.result(); cause.kind != causeNone {
		return cause.err
	}
	if coordinator.ctx.Err() != nil {
		ctxErr := coordinator.ctx.Err()
		coordinator.selectCause(callerCause(ctxErr), ctxErr, 0)
		return ctxErr
	}
	var streamErr *openrouter.StreamError
	if errors.As(err, &streamErr) && streamErr.Kind == openrouter.StreamProviderResponseInvalid {
		coordinator.selectCause(causeProviderInvalid, err, 0)
		return fmt.Errorf("chat response invalid: %w", err)
	}
	httpStatus := 0
	if errors.As(err, &streamErr) {
		httpStatus = streamErr.HTTPStatus
	}
	coordinator.selectCause(causeProviderError, err, httpStatus)
	return fmt.Errorf("chat request failed: %w", err)
}

func validateAssistantResponse(msg openrouter.Message) error {
	if msg.Content == "" && len(msg.ToolCalls) == 0 {
		return errors.New("agent: provider returned no usable assistant choice")
	}
	seen := make(map[string]struct{}, len(msg.ToolCalls))
	for i, call := range msg.ToolCalls {
		if call.ID == "" {
			return fmt.Errorf("agent: provider tool call %d has no ID", i)
		}
		if _, exists := seen[call.ID]; exists {
			return fmt.Errorf("agent: provider tool call ID %q is duplicated", call.ID)
		}
		seen[call.ID] = struct{}{}
		if call.Type != "function" || call.Function.Name == "" {
			return fmt.Errorf("agent: provider tool call %q is structurally incomplete", call.ID)
		}
	}
	return nil
}

func causeHasDurableTerminal(kind causeKind) bool {
	return kind == causeProviderError || kind == causeProviderInvalid ||
		kind == causeCallerCancelled || kind == causeCallerDeadline
}

func (s *Session) appendTerminal(
	ctx context.Context,
	lease memory.TurnLease,
	turnID memory.EventID,
	parentID memory.EventID,
	cause terminalCause,
) error {
	payload := memory.TurnTerminalPayload{TurnID: turnID, Stage: cause.stage}
	input := memory.EventInput{ParentID: parentID}
	switch cause.kind {
	case causeProviderError:
		input.Type = memory.EventTurnFailed
		input.Content = "The provider request failed."
		payload.Classification = memory.ClassificationProviderError
		if cause.httpStatus != 0 {
			status := cause.httpStatus
			payload.HTTPStatus = &status
		}
	case causeProviderInvalid:
		input.Type = memory.EventTurnFailed
		input.Content = "The provider response was invalid."
		payload.Classification = memory.ClassificationProviderResponseInvalid
	case causeCallerCancelled:
		input.Type = memory.EventTurnInterrupted
		input.Content = "The turn was cancelled by the caller."
		payload.Classification = memory.ClassificationCallerCancelled
	case causeCallerDeadline:
		input.Type = memory.EventTurnInterrupted
		input.Content = "The caller deadline was exceeded."
		payload.Classification = memory.ClassificationCallerDeadlineExceeded
	default:
		return nil
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode terminal payload: %w", err)
	}
	input.Payload = payloadJSON
	_, err = s.history.Append(ctx, lease, input)
	return err
}
