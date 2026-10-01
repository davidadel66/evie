package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/repoinstructions"
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
	recall := s.newRetrievalTurn()
	modelTools := s.modelToolset()
	workingFolder := s.scope.ProjectRoot
	if provider, ok := s.history.(interface {
		WorkingDirectory(context.Context) (string, error)
	}); ok && s.workerInstructions == "" {
		var err error
		workingFolder, err = provider.WorkingDirectory(coordinator.ctx)
		if err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("load working folder: %w", err))
		}
	}
	s.directory.SetRoot(workingFolder)
	coordinator.setStage(memory.StageContextCompose)
	var repository memory.RepositoryInstructionSnapshot
	if provider, ok := s.history.(interface {
		RepositoryInstructions(context.Context, memory.TurnLease, memory.EventID) (memory.RepositoryInstructionSnapshot, error)
	}); ok && s.workerInstructions == "" {
		var err error
		repository, err = provider.RepositoryInstructions(coordinator.ctx, lease, rootTurnID)
		if err != nil {
			if errors.Is(err, memory.ErrRepositoryInstructionsChanged) {
				return s.classifyRepositoryInstructionError(coordinator, err)
			}
			return s.classifyLocalError(coordinator, err)
		}
		if repository.WorkspaceID != "" && repository.Folder.Path != workingFolder {
			return s.classifyRepositoryInstructionError(coordinator, errors.New("Workspace folder changed while preparing instructions; try again"))
		}
		if repository.Status == "error" {
			return s.classifyRepositoryInstructionError(coordinator, fmt.Errorf("repository instructions: %s", repository.Detail))
		}
	}
	// Opaque transport state belongs only to this live turn. Durable events
	// remain sufficient to start a new turn after restart or cancellation.
	continuation := make(map[memory.EventID][]json.RawMessage)
	// A failed automatic compaction is remembered for the rest of this turn
	// and never retried in it; each attempt may wait out the compactor bound.
	var compactionFailure *automaticCompactionFailure
	// A provider context-length rejection gets exactly one compact-and-retry
	// per turn: the same iteration is composed again for its trigger.
	var rejectedRequestBytes int64
	contextRetryUsed := false
	for {
		if !coordinator.transitionIfActive(memory.StageContextCompose, func() {
			progress.requestParentID = requestParentID
		}) {
			return s.observeTurnContext(coordinator)
		}
		rendered.begin()
		retrying := rejectedRequestBytes > 0
		if !retrying {
			iteration++
		}
		// The last permitted model response is requested without tools, so
		// a runaway tool loop ends with an answer from what the turn has. A
		// worker's wrap-up signal can make an earlier response the last.
		finalStepNote, finalStep := s.finalStepNotice(iteration)

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
		if provider, ok := s.history.(workingContextProvider); ok && s.workerInstructions == "" {
			workingContext, err = provider.WorkingContext(coordinator.ctx)
			if err != nil {
				return s.classifyLocalError(coordinator, fmt.Errorf("load working context: %w", err))
			}
		}
		if iteration == 1 && !retrying && !s.automaticRecallDisabled {
			recall.automatic(coordinator.ctx, events, summary, rootTurnID, s.toolset.Schemas())
		}
		if workingFolder != "" {
			workingContext += fmt.Sprintf("\nLocal working folder: %q. Relative file paths and shell commands start in this session's working directory.\n", workingFolder)
		}
		memoryData, memoryReceipt := recall.renderProjection()
		composeInput := ContextComposeInput{
			MemoryData: memoryData, MemoryReceipt: memoryReceipt,
			RepositoryInstructions: repoinstructions.Render(repository), RepositoryInstructionsTurnID: repository.TurnID,
			Profile: s.profile, Summary: summary, Events: events, ActiveRootID: rootTurnID,
			TriggerEventID: requestParentID, Iteration: iteration,
			Tools: modelTools.Schemas(), Reasoning: s.reasoning, WorkingContext: workingContext, WorkerInstructions: s.workerInstructions,
			Continuation: continuation, RejectedRequestBytes: rejectedRequestBytes,
		}
		rejectedRequestBytes = 0
		if finalStep {
			composeInput.ToolChoice = "none"
			composeInput.FinalStepNote = finalStepNote
		}
		composeInput, err = recall.fitContext(composeInput, s.composer)
		if err != nil {
			if IsContextOverflow(err) {
				coordinator.selectCause(causeContextOverflow, err, 0)
				return err
			}
			return s.classifyLocalError(coordinator, err)
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
		if retrying && !required {
			// The retry is only ever a compacted request.
			overflow := fmt.Errorf("%w: no automatic compaction can shrink the request the provider rejected", ErrContextOverflow)
			coordinator.selectCause(causeContextOverflow, overflow, 0)
			return overflow
		}
		failureCategory := memory.ContextCompactionFailureNone
		if required {
			failure := compactionFailure
			if failure == nil {
				if s.compactor == nil {
					return s.classifyLocalError(coordinator, errors.New("agent: compactor is not configured"))
				}
				if !coordinator.setStage(memory.StageContextCompaction) {
					return s.observeTurnContext(coordinator)
				}
				var newSummary *ContextSummary
				var compacted memory.Event
				newSummary, compacted, failure = s.performAutomaticCompaction(coordinator, lease, plan)
				if failure == nil {
					summary = newSummary
					events = append(events, compacted)
					composeInput.Summary = summary
					composeInput.Events = events
					if err := s.observeTurnContext(coordinator); err != nil {
						return err
					}
				} else if coordinator.result().kind != causeNone {
					return failure.err
				} else {
					compactionFailure = failure
				}
			}
			if failure != nil {
				fits, fitErr := completeAutomaticProjectionFits(composeInput, s.composer)
				if fitErr != nil {
					return s.classifyLocalError(coordinator, fitErr)
				}
				if !fits || retrying {
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
		composeInput.MemoryData, composeInput.MemoryReceipt = recall.projection(coordinator.ctx)
		composeInput, err = recall.fitContext(composeInput, s.composer)
		if err != nil {
			if IsContextOverflow(err) {
				coordinator.selectCause(causeContextOverflow, err, 0)
				return err
			}
			return s.classifyLocalError(coordinator, err)
		}
		composed, err := s.composer.Compose(composeInput)
		if err != nil {
			if IsContextOverflow(err) {
				coordinator.selectCause(causeContextOverflow, err, 0)
				return err
			}
			return s.classifyLocalError(coordinator, err)
		}
		boundedData, boundedReceipt, reduced, err := recall.admitRequest(composed.Request)
		if err != nil {
			coordinator.selectCause(causeContextOverflow, err, 0)
			return err
		}
		if reduced {
			composeInput.MemoryData, composeInput.MemoryReceipt = boundedData, boundedReceipt
			composed, err = s.composer.Compose(composeInput)
			if err != nil {
				return s.classifyLocalError(coordinator, err)
			}
		}
		if required && failureCategory == memory.ContextCompactionFailureNone &&
			composed.Snapshot.RetainedFirstEventID != plan.FirstRetained.ID {
			overflow := fmt.Errorf("%w: accepted automatic summary did not preserve its retained frontier", ErrContextOverflow)
			coordinator.selectCause(causeContextOverflow, overflow, 0)
			return overflow
		}
		composed.Snapshot.CompactionFailureCategory = failureCategory
		recall.recordAccounting(composed.Snapshot.Memory)
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
		snapshotEvent, err := s.history.Append(coordinator.ctx, lease, memory.EventInput{
			ParentID: requestParentID, Type: memory.EventContextSnapshot, Payload: snapshotPayload,
		})
		if err != nil {
			coordinator.abortCommitBoundary()
			return s.classifyLocalError(coordinator, fmt.Errorf("persist context snapshot: %w", err))
		}
		if memoryReceipt != nil {
			if activity, ok := ev.(MemoryActivityEvents); ok {
				activity.MemoryRetrieved(snapshotEvent)
			}
		}
		coordinator.finishCommitBoundary(memory.StageProvider)
		req := composed.Request
		if err := s.owner.Authorize(coordinator.ctx, lease); err != nil {
			return s.classifyLocalError(coordinator, fmt.Errorf("authorize provider start: %w", err))
		}
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
		}

		res, err := s.callProvider(coordinator, lease, req, ev, rendered)
		if err != nil {
			var rejection *providerContextRejection
			if !errors.As(err, &rejection) {
				return err
			}
			if contextRetryUsed {
				return s.failContextRejection(coordinator, rejection)
			}
			contextRetryUsed = true
			rejectedRequestBytes = composed.Snapshot.SerializedBytes
			continue
		}
		if err := s.observeTurnContext(coordinator); err != nil {
			return err
		}
		if len(res.Choices) == 0 {
			err := errors.New("agent: provider returned no choices")
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}
		switch res.Choices[0].FinishReason {
		case "length":
			// Truncated text or tool arguments are never durable success.
			err := errors.New("agent: provider stopped at the output token limit")
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		case "error":
			err := errors.New("agent: provider reported a generation error")
			coordinator.selectCause(causeProviderError, err, 0)
			return err
		}

		msg := res.Choices[0].Message
		if err := validateAssistantResponse(msg); err != nil {
			coordinator.selectCause(causeProviderInvalid, err, 0)
			return err
		}
		if finalStep && len(msg.ToolCalls) != 0 {
			// Nothing from a response that ignored the withheld tools is
			// committed or executed.
			err := fmt.Errorf("%w (%d model responses)", ErrStepLimitExceeded, iteration)
			coordinator.selectCause(causeStepLimit, err, 0)
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
			if openrouter.UsesResponses(req.Model) {
				continuation[assistantEvent.ID] = msg.ResponseItems
			}
			coordinator.finishCommitBoundary(memory.StageToolPrepare)
		}
		rendered.mu.Lock()
		rendered.assistantCommitted = true
		rendered.mu.Unlock()

		if len(msg.ToolCalls) == 0 {
			s.emitCommittedAssistant(ev, rendered, assistantEvent)
			return nil
		}

		// This is an exactly-once durable acceptance notification, not a live
		// provider/tool callback. Once the append commits it is delivered even
		// when a terminal cause was reserved at the commit boundary. The call is
		// synchronous after provider callback lifetime closure, so it completes
		// before Send returns and before any frontend error/turn_done wrapper.
		s.emitCommittedAssistant(ev, rendered, assistantEvent)
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
				if s.timing.beforeToolPhaseCallback != nil {
					s.timing.beforeToolPhaseCallback(memory.StageToolApproval)
				}
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
					return coordinator.toolPhaseInterruption()
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
				if s.timing.beforeToolPhaseCallback != nil {
					stage := memory.StageToolExecute
					if boundary == tools.AuthorizePreparation {
						stage = memory.StageToolPrepare
					}
					s.timing.beforeToolPhaseCallback(stage)
				}
				// Lifecycle callbacks run inside the tool phase, so a rejected
				// entry reports without selecting a cause (see
				// toolPhaseInterruption); classification follows abortToolPhase.
				switch boundary {
				case tools.AuthorizePreparation:
					if !coordinator.setStage(memory.StageToolPrepare) {
						return coordinator.toolPhaseInterruption()
					}
				case tools.AuthorizeExecution:
					if !coordinator.setStage(memory.StageToolExecute) {
						return coordinator.toolPhaseInterruption()
					}
				}
				return s.owner.Authorize(authorizeCtx, lease)
			}

			if !coordinator.beginToolPhase() {
				return s.observeTurnContext(coordinator)
			}
			invocationCtx := tools.WithInvocationContext(coordinator.ctx, tools.InvocationContext{
				Profile:   s.profile,
				Directory: &s.directory,
				Scope:     s.scope, Lease: lease, SourceEventID: rootTurnID, IntentEventID: intentEvent.ID, SearchMemory: recall.searchForTool(call.ID),
			})
			toolCtx := task.WithMutationAttribution(invocationCtx, task.MutationAttribution{
				ActorID: string(s.scope.OwnerID), SessionID: string(s.scope.SessionID), RunID: string(executionID),
				ParentSessionID: string(s.scope.ParentSessionID), LeaseHolderID: string(lease.HolderID),
				WorkspaceID: string(s.scope.WorkspaceID), ProjectID: string(s.scope.ProjectID),
				LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation),
			})
			result, isErr, err := modelTools.ExecuteWithApprovalAuthorizedCompletion(
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

const (
	maxProviderRetries = 2
	// maxProviderRetryDelay bounds any single backoff, including a provider's
	// Retry-After. A longer requested wait fails the turn instead of stalling it.
	maxProviderRetryDelay = 30 * time.Second
)

// callProvider streams one admitted request and returns either a response or
// an already classified turn error. A transient failure is retried with the
// identical request only while no live callback has fired, so nothing has
// reached the user and no tool can have run. Each attempt is lease-authorized
// immediately before it starts.
func (s *Session) callProvider(
	coordinator *turnCoordinator,
	lease memory.TurnLease,
	req openrouter.ChatRequest,
	ev Events,
	rendered *renderedOutput,
) (openrouter.ChatResponse, error) {
	for attempt := 0; ; attempt++ {
		res, callbackFired, err := s.streamProviderAttempt(coordinator, req, ev, rendered)
		if err == nil {
			return res, nil
		}
		delay, retry := providerRetryDelay(err, attempt, s.timing.providerRetryBase)
		if !retry || callbackFired || coordinator.result().kind != causeNone || coordinator.ctx.Err() != nil {
			if rejection := contextRejection(coordinator, err, callbackFired); rejection != nil {
				return openrouter.ChatResponse{}, rejection
			}
			return openrouter.ChatResponse{}, s.classifyProviderError(coordinator, err)
		}
		if waitErr := s.timing.waitProviderRetry(coordinator.ctx, delay); waitErr != nil {
			if observed := s.observeTurnContext(coordinator); observed != nil {
				return openrouter.ChatResponse{}, observed
			}
			return openrouter.ChatResponse{}, s.classifyProviderError(coordinator, err)
		}
		if err := s.owner.Authorize(coordinator.ctx, lease); err != nil {
			return openrouter.ChatResponse{}, s.classifyLocalError(coordinator, fmt.Errorf("authorize provider retry: %w", err))
		}
		if err := s.observeTurnContext(coordinator); err != nil {
			return openrouter.ChatResponse{}, err
		}
	}
}

// streamProviderAttempt runs one provider call and reports whether any live
// callback was admitted during it. The callback lifetime is closed and joined
// before returning, so the report is final.
func (s *Session) streamProviderAttempt(
	coordinator *turnCoordinator,
	req openrouter.ChatRequest,
	ev Events,
	rendered *renderedOutput,
) (openrouter.ChatResponse, bool, error) {
	callbackLifetime := &providerCallbackLifetime{}
	var callbackFired atomic.Bool
	handlers := openrouter.StreamHandlers{
		OnReasoning: func(text string) {
			callbackLifetime.invoke(func() {
				callbackFired.Store(true)
				coordinator.emitIfActive(func() {
					rendered.mu.Lock()
					if rendered.content {
						rendered.mu.Unlock()
						return
					}
					rendered.reasoning = rendered.reasoning || text != ""
					rendered.reasoningOpen = true
					rendered.mu.Unlock()
					ev.Reasoning(text)
				})
			})
		},
		OnContent: func(text string) {
			callbackLifetime.invoke(func() {
				callbackFired.Store(true)
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
	return res, callbackFired.Load(), err
}

// providerRetryDelay decides whether a failed provider attempt may repeat and
// after how long. Only failures that prove nothing was generated qualify: no
// HTTP response at all, or 429/502/503/504. Backoff doubles from base and
// honors a longer Retry-After up to maxProviderRetryDelay.
func providerRetryDelay(err error, attempt int, base time.Duration) (time.Duration, bool) {
	var streamErr *openrouter.StreamError
	if attempt >= maxProviderRetries || !errors.As(err, &streamErr) || streamErr.Kind != openrouter.StreamProviderError {
		return 0, false
	}
	switch streamErr.HTTPStatus {
	case 0:
		if !streamErr.NoResponse {
			return 0, false
		}
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return 0, false
	}
	delay := max(base<<attempt, streamErr.RetryAfter)
	if delay > maxProviderRetryDelay {
		return 0, false
	}
	return delay, true
}

func waitProviderRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
	event memory.Event,
) {
	rendered.mu.Lock()
	closeReasoning := rendered.reasoningOpen
	rendered.reasoningOpen = false
	rendered.mu.Unlock()
	if closeReasoning {
		ev.ReasoningDone()
	}
	if activity, ok := ev.(ActivityEvents); ok {
		activity.AssistantCommitted(event)
	} else {
		ev.AssistantDone(event.Content)
	}
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

func (s *Session) classifyRepositoryInstructionError(coordinator *turnCoordinator, err error) error {
	if cause := s.observeTurnContext(coordinator); cause != nil {
		return cause
	}
	coordinator.selectCause(causeRepositoryInstructions, err, 0)
	return err
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
		kind == causeContextOverflow || kind == causeRepositoryInstructions ||
		kind == causeStepLimit
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
	case causeRepositoryInstructions:
		input.Type = memory.EventTurnFailed
		payload.Classification = memory.ClassificationRepositoryInstructions
	case causeStepLimit:
		input.Type = memory.EventTurnFailed
		payload.Classification = memory.ClassificationStepLimitExceeded
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
