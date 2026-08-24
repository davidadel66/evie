package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

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

type providerCallbackLifetime struct {
	mu     sync.Mutex
	serial sync.Mutex
	closed bool
	active sync.WaitGroup
}

func (l *providerCallbackLifetime) invoke(callback func()) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.active.Add(1)
	// Holding the admission lock while waiting for serial preserves the order
	// in which concurrent provider callbacks enter this lifetime. Frontend
	// event sinks are intentionally allowed to be non-thread-safe.
	l.serial.Lock()
	l.mu.Unlock()
	defer func() {
		l.serial.Unlock()
		l.active.Done()
	}()
	callback()
}

func (l *providerCallbackLifetime) closeAndWait() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.active.Wait()
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
		ticker := s.timing.newTicker(s.timing.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-caller.Done():
				coordinator.selectCause(callerCause(caller.Err()), caller.Err(), 0)
				return
			case <-ticker.C():
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
					coordinator.selectCause(causeHeartbeatFailed, fmt.Errorf("%w: %v", ErrLeaseHeartbeatFailed, err), 0)
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
		if !coordinator.transitionIfActive(memory.StageProvider, func() {
			progress.requestParentID = requestParentID
		}) {
			return s.observeTurnContext(coordinator)
		}
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

		callbackLifetime := &providerCallbackLifetime{}
		handlers := openrouter.StreamHandlers{
			OnReasoning: func(text string) {
				callbackLifetime.invoke(func() {
					coordinator.emitIfActive(func() {
						rendered.mu.Lock()
						rendered.reasoning = true
						rendered.reasoningOpen = true
						rendered.mu.Unlock()
						ev.Reasoning(text)
					})
				})
			},
			OnContent: func(text string) {
				callbackLifetime.invoke(func() {
					coordinator.emitIfActive(func() {
						rendered.mu.Lock()
						closeReasoning := rendered.reasoningOpen
						rendered.reasoningOpen = false
						rendered.content = true
						rendered.mu.Unlock()
						if closeReasoning {
							ev.ReasoningDone()
						}
						ev.Delta(text)
					})
				})
			},
		}

		res, err := s.client.ChatStream(coordinator.ctx, req, handlers)
		callbackLifetime.closeAndWait()
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

		if s.timing.beforeAssistantConstruction != nil {
			s.timing.beforeAssistantConstruction()
		}
		if !coordinator.setStage(memory.StageAssistantCommit) {
			return s.observeTurnContext(coordinator)
		}
		assistantInput, err := assistantEventInput(msg)
		if err != nil {
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}
		assistantInput.ParentID = requestParentID
		if !coordinator.beginCommitBoundary() {
			return s.observeTurnContext(coordinator)
		}
		assistantEvent, err := s.history.Append(coordinator.ctx, lease, assistantInput)
		if err != nil {
			coordinator.abortCommitBoundary()
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
		if len(msg.ToolCalls) == 0 {
			// A committed final assistant is durable success. This deliberately
			// wins over cancellation reserved while its append was in flight.
			coordinator.finishSuccessBoundary()
		} else {
			coordinator.finishCommitBoundary(memory.StageToolPrepare)
		}
		rendered.mu.Lock()
		rendered.assistantCommitted = true
		rendered.mu.Unlock()

		if len(msg.ToolCalls) == 0 {
			s.emitAssistantAccepted(coordinator, ev, rendered, msg.Content, true)
			return nil
		}

		if !s.emitAssistantAccepted(coordinator, ev, rendered, msg.Content, false) {
			return s.observeTurnContext(coordinator)
		}

		var lastOutcomeID memory.EventID
		for callIndex, call := range msg.ToolCalls {
			if !coordinator.setStage(memory.StageToolPrepare) {
				return s.observeTurnContext(coordinator)
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
			if !coordinator.emitIfActive(func() {
				ev.ToolCall(call.ID, call.Function.Name, call.Function.Arguments)
			}) {
				return s.observeTurnContext(coordinator)
			}

			var approvalEventID memory.EventID
			var approvalDecision tools.Decision
			wrappedApprover := func(
				approvalCtx context.Context,
				name, args string,
				preview *tools.FileChangePreview,
			) tools.Decision {
				if s.timing.beforeApprovalInvocation != nil {
					s.timing.beforeApprovalInvocation()
				}
				if !coordinator.setStage(memory.StageToolApproval) {
					return tools.Expired
				}
				return admitApproval(coordinator, approve, approvalCtx, name, args, preview)
			}
			observeApproval := func(observeCtx context.Context, decision tools.Decision) error {
				input, err := approvalEventInput(intentEvent.ID, executionID, decision)
				if err != nil {
					return err
				}
				if !coordinator.beginCommitBoundary() {
					return s.observeTurnContext(coordinator)
				}
				approvalEvent, err := s.history.Append(observeCtx, lease, input)
				if err != nil {
					coordinator.abortCommitBoundary()
					return fmt.Errorf("persist approval: %w", err)
				}
				approvalEventID = approvalEvent.ID
				approvalDecision = decision
				if decision == tools.Approved {
					coordinator.finishCommitBoundary(memory.StageToolExecute)
				} else {
					coordinator.finishCommitBoundary(memory.StageToolCommit)
				}
				return nil
			}
			authorize := func(authorizeCtx context.Context, boundary tools.AuthorizationBoundary) error {
				switch boundary {
				case tools.AuthorizePreparation:
					if !coordinator.setStage(memory.StageToolPrepare) {
						return s.observeTurnContext(coordinator)
					}
				case tools.AuthorizeExecution:
					if !coordinator.setStage(memory.StageToolExecute) {
						return s.observeTurnContext(coordinator)
					}
				}
				return s.owner.Authorize(authorizeCtx, lease)
			}

			if !coordinator.beginToolPhase() {
				return s.observeTurnContext(coordinator)
			}
			result, isErr, err := tools.ExecuteWithApprovalAuthorizedCompletion(
				coordinator.ctx, extra, call, wrappedApprover, observeApproval, authorize,
				func() {
					if s.timing.beforeToolResultHandoff != nil {
						s.timing.beforeToolResultHandoff()
					}
					coordinator.completeToolPhase()
				},
			)
			if err != nil {
				coordinator.abortToolPhase()
				return s.classifyLocalError(coordinator, fmt.Errorf("execute tool lifecycle: %w", err))
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
			if !coordinator.emitIfActive(func() {
				ev.ToolResult(call.ID, result.Content, isErr)
				if callIndex+1 < len(msg.ToolCalls) {
					coordinator.finishAdmittedCallbackStage(memory.StageToolPrepare)
				}
			}) {
				return s.observeTurnContext(coordinator)
			}
			if err := s.observeTurnContext(coordinator); err != nil {
				return err
			}
		}
		requestParentID = lastOutcomeID
	}
}

func admitApproval(
	coordinator *turnCoordinator,
	approve tools.Approver,
	ctx context.Context,
	name, args string,
	preview *tools.FileChangePreview,
) tools.Decision {
	decision := tools.Expired
	coordinator.emitIfActive(func() {
		if approve == nil {
			decision = tools.Declined
			return
		}
		decision = approve(ctx, name, args, preview)
	})
	return decision
}

func (s *Session) emitAssistantAccepted(
	coordinator *turnCoordinator,
	ev Events,
	rendered *renderedOutput,
	content string,
	committedFinal bool,
) bool {
	emit := func() {
		rendered.mu.Lock()
		closeReasoning := rendered.reasoningOpen
		rendered.reasoningOpen = false
		rendered.mu.Unlock()
		if closeReasoning {
			ev.ReasoningDone()
		}
		ev.AssistantDone(content)
	}
	if committedFinal {
		emit()
		return true
	}
	return coordinator.emitIfActive(emit)
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
		payload.Classification = memory.ClassificationProviderError
		if cause.httpStatus != 0 {
			status := cause.httpStatus
			payload.HTTPStatus = &status
		}
	case causeProviderInvalid:
		input.Type = memory.EventTurnFailed
		payload.Classification = memory.ClassificationProviderResponseInvalid
	case causeCallerCancelled:
		input.Type = memory.EventTurnInterrupted
		payload.Classification = memory.ClassificationCallerCancelled
	case causeCallerDeadline:
		input.Type = memory.EventTurnInterrupted
		payload.Classification = memory.ClassificationCallerDeadlineExceeded
	default:
		return nil
	}
	input.Content = payload.SafeContent()
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode terminal payload: %w", err)
	}
	input.Payload = payloadJSON
	_, err = s.history.Append(ctx, lease, input)
	return err
}
