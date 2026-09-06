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
	"github.com/davidadel66/evie/internal/task"
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
	foreground      *foregroundObservation
	rendered        renderedOutput
	rootTurnID      memory.EventID
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
) error {
	requestParentID := progress.requestParentID
	rootTurnID := progress.rootTurnID
	if rootTurnID == "" {
		rootTurnID = requestParentID
	}
	rendered := &progress.rendered
	iteration := 0
	for {
		if !coordinator.transitionIfActive(memory.StageContextCompose, func() {
			progress.requestParentID = requestParentID
		}) {
			return s.observeTurnContext(coordinator)
		}
		rendered.begin()
		iteration++

		events, err := s.history.Events(coordinator.ctx)
		if err == nil {
			if ctxErr := coordinator.ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
		}
		if err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("load durable history: %w", err))
		}
		summary, _, err := reconstructCompactionChain(events)
		if err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("reconstruct durable compaction chain: %w", err))
		}
		workingContext := ""
		if provider, ok := s.history.(workingContextProvider); ok {
			workingContext, err = provider.WorkingContext(coordinator.ctx)
			if err != nil {
				return s.classifyLocalError(coordinator, fmt.Errorf("load working context: %w", err))
			}
		}
		composeInput := ContextComposeInput{
			Profile: s.profile, Summary: summary, Events: events, ActiveRootID: rootTurnID,
			TriggerEventID: requestParentID, Iteration: iteration,
			Tools: s.toolset.Schemas(), Reasoning: s.reasoning, WorkingContext: workingContext,
		}
		plan, required, err := selectAutomaticCompaction(composeInput, s.composer)
		if err != nil {
			if errors.Is(err, ErrNoLegalAutomaticCompaction) || IsContextOverflow(err) {
				overflow := fmt.Errorf("%w: %v", ErrContextOverflow, err)
				coordinator.selectCause(causeContextOverflow, overflow, 0)
				return overflow
			}
			return s.classifyLocalError(coordinator, err)
		}
		failureCategory := memory.ContextCompactionFailureNone
		if required {
			if s.compactor == nil {
				return s.classifyLocalError(coordinator, errors.New("agent: compactor is not configured"))
			}
			if !coordinator.setStage(memory.StageContextCompaction) {
				return s.observeTurnContext(coordinator)
			}
			newSummary, compacted, failure := s.performAutomaticCompaction(coordinator, lease, plan)
			if failure == nil {
				summary = newSummary
				events = append(events, compacted)
				composeInput.Summary = summary
				composeInput.Events = events
				if err := s.observeTurnContext(coordinator); err != nil {
					return err
				}
			} else {
				if coordinator.result().kind != causeNone {
					return failure.err
				}
				fits, fitErr := completeAutomaticProjectionFits(composeInput, s.composer)
				if fitErr != nil {
					return s.classifyLocalError(coordinator, fitErr)
				}
				if !fits {
					if failure.cause != causeNone {
						coordinator.selectCause(failure.cause, failure.err, failure.httpStatus)
						return failure.err
					}
					return s.classifyLocalError(coordinator, failure.err)
				}
				failureCategory = failure.category
				if !coordinator.setStage(memory.StageContextCompose) {
					return s.observeTurnContext(coordinator)
				}
			}
		}
		composed, err := s.composer.Compose(composeInput)
		if err != nil {
			if IsContextOverflow(err) {
				coordinator.selectCause(causeContextOverflow, err, 0)
				return err
			}
			return s.classifyLocalError(coordinator, err)
		}
		if required && failureCategory == memory.ContextCompactionFailureNone &&
			composed.Snapshot.RetainedFirstEventID != plan.FirstRetained.ID {
			overflow := fmt.Errorf("%w: accepted automatic summary did not preserve its retained frontier", ErrContextOverflow)
			coordinator.selectCause(causeContextOverflow, overflow, 0)
			return overflow
		}
		if required && failureCategory == memory.ContextCompactionFailureNone &&
			composed.Snapshot.SerializedBytes > percentageFloor(composed.Snapshot.WorkingCeilingTokens, automaticCompactionTargetPercent) {
			overflow := fmt.Errorf("%w: accepted automatic summary did not satisfy its target", ErrContextOverflow)
			coordinator.selectCause(causeContextOverflow, overflow, 0)
			return overflow
		}
		composed.Snapshot.CompactionFailureCategory = failureCategory
		if err := composed.Snapshot.Validate(); err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("validate final context snapshot: %w", err))
		}
		snapshotPayload, err := json.Marshal(composed.Snapshot)
		if err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("encode context snapshot: %w", err))
		}
		if !coordinator.beginCommitBoundary() {
			return s.observeTurnContext(coordinator)
		}
		_, err = s.history.Append(coordinator.ctx, lease, memory.EventInput{
			ParentID: requestParentID, Type: memory.EventContextSnapshot, Payload: snapshotPayload,
		})
		if err != nil {
			coordinator.abortCommitBoundary()
			return s.classifyLocalError(coordinator, fmt.Errorf("persist context snapshot: %w", err))
		}
		coordinator.finishCommitBoundary(memory.StageProvider)
		req := composed.Request
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
						if rendered.content {
							rendered.mu.Unlock()
							return
						}
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
		assistantInput, err := assistantEventInput(msg, res.Usage)
		if err != nil {
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}
		assistantInput.ParentID = requestParentID
		if !coordinator.beginCommitBoundary() {
			return s.observeTurnContext(coordinator)
		}
		assistantCommitStarted := time.Now()
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
			progress.foreground.terminal(assistantCommitStarted, "success")
		} else {
			coordinator.finishCommitBoundary(memory.StageToolPrepare)
		}
		rendered.mu.Lock()
		rendered.assistantCommitted = true
		rendered.mu.Unlock()

		if len(msg.ToolCalls) == 0 {
			s.emitCommittedAssistant(ev, rendered, msg.Content)
			return nil
		}

		// This is an exactly-once durable acceptance notification, not a live
		// provider/tool callback. Once the append commits it is delivered even
		// when a terminal cause was reserved at the commit boundary. The call is
		// synchronous after provider callback lifetime closure, so it completes
		// before Send returns and before any frontend error/turn_done wrapper.
		s.emitCommittedAssistant(ev, rendered, msg.Content)
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
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
			observeApproval := func(observeCtx context.Context, decision tools.Decision, metadata tools.ApprovalMetadata) error {
				input, err := approvalEventInput(intentEvent.ID, executionID, decision)
				var semanticInput memory.EventInput
				hasSemanticInput := metadata != (tools.ApprovalMetadata{})
				if hasSemanticInput {
					semanticInput, err = semanticApprovalEventInput(
						metadata.ParentEventID, metadata.ExecutionID, decision,
						metadata.ProposalSHA256, metadata.PreparedSHA256,
					)
				}
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
				if hasSemanticInput {
					if _, err := s.history.Append(observeCtx, lease, semanticInput); err != nil {
						coordinator.abortCommitBoundary()
						return fmt.Errorf("persist semantic approval: %w", err)
					}
				}
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
			invocationCtx := tools.WithInvocationContext(coordinator.ctx, tools.InvocationContext{
				Scope: s.scope, Lease: lease, SourceEventID: rootTurnID,
			})
			toolCtx := task.WithMutationAttribution(invocationCtx, task.MutationAttribution{
				ActorID: string(s.scope.OwnerID), SessionID: string(s.scope.SessionID), RunID: string(executionID),
				ParentSessionID: string(s.scope.ParentSessionID), LeaseHolderID: string(lease.HolderID),
				WorkspaceID: string(s.scope.WorkspaceID), ProjectID: string(s.scope.ProjectID),
				LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation),
			})
			result, isErr, err := s.toolset.ExecuteWithApprovalAuthorizedCompletion(
				toolCtx, call, wrappedApprover, observeApproval, authorize,
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
			result.Content = admitToolResult(result.Content)

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

func (s *Session) emitCommittedAssistant(
	ev Events,
	rendered *renderedOutput,
	content string,
) {
	rendered.mu.Lock()
	closeReasoning := rendered.reasoningOpen
	rendered.reasoningOpen = false
	rendered.mu.Unlock()
	if closeReasoning {
		ev.ReasoningDone()
	}
	ev.AssistantDone(content)
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
		kind == causeCallerCancelled || kind == causeCallerDeadline ||
		kind == causeContextOverflow
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
	case causeContextOverflow:
		input.Type = memory.EventTurnFailed
		payload.Classification = memory.ClassificationContextOverflow
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
