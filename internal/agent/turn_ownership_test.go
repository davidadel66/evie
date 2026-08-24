package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

var (
	errFakeConflict  = errors.New("fake lease conflict")
	errFakeLeaseLost = errors.New("fake lease lost")
)

type scriptedOwner struct {
	mu sync.Mutex

	acquireErr     error
	heartbeatErr   error
	authorizeErrAt int
	authorizeErr   error
	releaseErr     error
	releaseBlock   bool
	heartbeatBlock bool
	heartbeatStart chan struct{}
	heartbeatWait  <-chan struct{}
	afterAcquire   func()
	acquires       int
	heartbeats     int
	authorizations int
	releases       int
}

type manualHeartbeatTicker struct {
	ticks   <-chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (t *manualHeartbeatTicker) C() <-chan time.Time { return t.ticks }
func (t *manualHeartbeatTicker) Stop() {
	t.once.Do(func() { close(t.stopped) })
}

func useManualHeartbeatTicker(s *Session) chan<- time.Time {
	ticks := make(chan time.Time, 8)
	s.timing.newTicker = func(time.Duration) heartbeatTicker {
		return &manualHeartbeatTicker{ticks: ticks, stopped: make(chan struct{})}
	}
	return ticks
}

func (o *scriptedOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	o.mu.Lock()
	o.acquires++
	err := o.acquireErr
	hook := o.afterAcquire
	o.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return memory.TurnLease{}, err
	}
	return memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 7, Generation: 7}, nil
}

func (o *scriptedOwner) Heartbeat(ctx context.Context, _ memory.TurnLease, _ time.Duration) (memory.TurnLease, error) {
	o.mu.Lock()
	o.heartbeats++
	err := o.heartbeatErr
	block := o.heartbeatBlock
	started := o.heartbeatStart
	wait := o.heartbeatWait
	o.mu.Unlock()
	if started != nil {
		close(started)
	}
	if block {
		<-ctx.Done()
		return memory.TurnLease{}, ctx.Err()
	}
	if wait != nil {
		select {
		case <-wait:
		case <-ctx.Done():
			return memory.TurnLease{}, ctx.Err()
		}
	}
	return memory.TurnLease{}, err
}

func (o *scriptedOwner) Authorize(context.Context, memory.TurnLease) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.authorizations++
	if o.authorizeErrAt == o.authorizations {
		return o.authorizeErr
	}
	return nil
}

func (o *scriptedOwner) Release(ctx context.Context, _ memory.TurnLease) error {
	o.mu.Lock()
	o.releases++
	err := o.releaseErr
	block := o.releaseBlock
	o.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

func (*scriptedOwner) IsConflict(err error) bool  { return errors.Is(err, errFakeConflict) }
func (*scriptedOwner) IsLeaseLost(err error) bool { return errors.Is(err, errFakeLeaseLost) }

func (o *scriptedOwner) counts() (acquire, heartbeat, authorize, release int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.acquires, o.heartbeats, o.authorizations, o.releases
}

func ownedSession(client Client, history *fakeHistory, owner *scriptedOwner) *Session {
	return New(client, "test", history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, owner)
}

func TestTurnOwnershipUsesFixedTimingAndExpectedHeartbeatShutdownIsNormal(t *testing.T) {
	if defaultTurnTiming.leaseDuration != 30*time.Second ||
		defaultTurnTiming.heartbeatInterval != 10*time.Second ||
		defaultTurnTiming.cleanupTimeout != 5*time.Second ||
		defaultTurnTiming.newTicker == nil {
		t.Fatalf("turn timing=%+v", defaultTurnTiming)
	}
	owner := &scriptedOwner{}
	err := ownedSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, &fakeHistory{}, owner).
		Send(context.Background(), "hello", &recorder{}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, heartbeats, _, releases := owner.counts()
	if heartbeats != 0 || releases != 1 {
		t.Fatalf("heartbeats=%d releases=%d", heartbeats, releases)
	}
}

type waitForHeartbeatClient struct {
	started <-chan struct{}
}

func (c waitForHeartbeatClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	select {
	case <-c.started:
	case <-ctx.Done():
		return openrouter.ChatResponse{}, ctx.Err()
	case <-time.After(time.Second):
		return openrouter.ChatResponse{}, errors.New("heartbeat did not start")
	}
	return assistantStep("done", nil).res, nil
}

func TestSuccessfulTurnCancelsInflightHeartbeatBeforeBoundedRelease(t *testing.T) {
	started := make(chan struct{})
	owner := &scriptedOwner{heartbeatBlock: true, heartbeatStart: started}
	s := ownedSession(waitForHeartbeatClient{started: started}, &fakeHistory{}, owner)
	ticks := useManualHeartbeatTicker(s)
	ticks <- time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Send(context.Background(), "hello", &recorder{}, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send deadlocked while stopping the in-flight heartbeat")
	}
	_, heartbeats, _, releases := owner.counts()
	if heartbeats != 1 || releases != 1 {
		t.Fatalf("heartbeats=%d releases=%d, want one each", heartbeats, releases)
	}
}

func TestHeartbeatTickSeamStopsTickerOnExpectedShutdown(t *testing.T) {
	owner := &scriptedOwner{}
	s := ownedSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, &fakeHistory{}, owner)
	ticks := make(chan time.Time)
	stopped := make(chan struct{})
	s.timing.newTicker = func(time.Duration) heartbeatTicker {
		return &manualHeartbeatTicker{ticks: ticks, stopped: stopped}
	}
	if err := s.Send(context.Background(), "hello", &recorder{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("heartbeat ticker was not stopped during expected shutdown")
	}
	_, heartbeats, _, releases := owner.counts()
	if heartbeats != 0 || releases != 1 {
		t.Fatalf("heartbeats=%d releases=%d", heartbeats, releases)
	}
}

func TestAlreadyCancelledSendPerformsZeroTurnWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	owner := &scriptedOwner{}
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	err := ownedSession(client, history, owner).Send(ctx, "hello", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context.Canceled", err)
	}
	acquires, heartbeats, authorizes, releases := owner.counts()
	if acquires != 0 || heartbeats != 0 || authorizes != 0 || releases != 0 ||
		history.appendAttempts != 0 || len(client.reqs) != 0 {
		t.Fatalf("acquire=%d heartbeat=%d authorize=%d release=%d appends=%d provider=%d", acquires, heartbeats, authorizes, releases, history.appendAttempts, len(client.reqs))
	}
}

func TestTurnCoordinatorAtomicallySelectsOneFirstCause(t *testing.T) {
	coordinator := newTurnCoordinator(context.Background())
	coordinator.setStage(memory.StageProvider)
	start := make(chan struct{})
	winners := make(chan causeKind, 5)
	var wg sync.WaitGroup
	for _, kind := range []causeKind{
		causeProviderError, causeCallerCancelled, causeLeaseLost,
		causeHeartbeatFailed, causeAssistantPersistence,
	} {
		wg.Add(1)
		go func(kind causeKind) {
			defer wg.Done()
			<-start
			if coordinator.selectCause(kind, errors.New("terminal cause"), 0) {
				winners <- kind
			}
		}(kind)
	}
	close(start)
	wg.Wait()
	close(winners)
	var selected []causeKind
	for kind := range winners {
		selected = append(selected, kind)
	}
	if len(selected) != 1 || coordinator.result().kind != selected[0] ||
		coordinator.result().stage != memory.StageProvider {
		t.Fatalf("winners=%v result=%+v", selected, coordinator.result())
	}
	if coordinator.selectCause(causeCallerDeadline, context.DeadlineExceeded, 0) {
		t.Fatal("later cause replaced the first terminal cause")
	}
}

func TestCallbackAdmissionGateSerializesEveryLiveCallbackWithCauseSelection(t *testing.T) {
	callbackNames := []string{"reasoning", "content", "assistant_done", "tool_call", "tool_result", "approval"}
	causes := []struct {
		name string
		kind causeKind
		err  error
	}{
		{name: "caller", kind: causeCallerCancelled, err: context.Canceled},
		{name: "heartbeat", kind: causeHeartbeatFailed, err: ErrLeaseHeartbeatFailed},
		{name: "lease", kind: causeLeaseLost, err: ErrLeaseLost},
	}
	for _, callbackName := range callbackNames {
		for _, cause := range causes {
			t.Run(callbackName+"_"+cause.name, func(t *testing.T) {
				coordinator := newTurnCoordinator(context.Background())
				coordinator.setStage(memory.StageProvider)
				callbackStarted := make(chan struct{})
				releaseCallback := make(chan struct{})
				callbackDone := make(chan bool, 1)
				go func() {
					callbackDone <- coordinator.emitIfActive(func() {
						close(callbackStarted)
						<-releaseCallback
					})
				}()
				<-callbackStarted

				selectionStarted := make(chan struct{})
				selectionDone := make(chan bool, 1)
				go func() {
					close(selectionStarted)
					selectionDone <- coordinator.selectCause(cause.kind, cause.err, 0)
				}()
				<-selectionStarted
				select {
				case <-coordinator.ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("coordinator context was not cancelled while callback remained admitted")
				}
				select {
				case <-selectionDone:
					t.Fatal("terminal cause finalized while an admitted callback was still running")
				default:
				}
				close(releaseCallback)
				if admitted := <-callbackDone; !admitted {
					t.Fatal("first callback was not admitted")
				}
				if selected := <-selectionDone; !selected {
					t.Fatal("terminal cause was not selected")
				}
				beganAfterSelection := false
				if coordinator.emitIfActive(func() { beganAfterSelection = true }) || beganAfterSelection {
					t.Fatal("callback began after terminal selection")
				}
			})
		}
	}
}

func TestAdmittedCallbackMayWaitForCoordinatorCancellationWithoutDeadlock(t *testing.T) {
	coordinator := newTurnCoordinator(context.Background())
	callbackStarted := make(chan struct{})
	callbackDone := make(chan bool, 1)
	go func() {
		callbackDone <- coordinator.emitIfActive(func() {
			close(callbackStarted)
			<-coordinator.ctx.Done()
		})
	}()
	<-callbackStarted
	selectionDone := make(chan bool, 1)
	go func() {
		selectionDone <- coordinator.selectCause(causeLeaseLost, ErrLeaseLost, 0)
	}()
	select {
	case admitted := <-callbackDone:
		if !admitted {
			t.Fatal("callback was not admitted")
		}
	case <-time.After(time.Second):
		t.Fatal("callback deadlocked waiting for coordinator cancellation")
	}
	select {
	case selected := <-selectionDone:
		if !selected {
			t.Fatal("terminal cause was not finalized")
		}
	case <-time.After(time.Second):
		t.Fatal("terminal selection did not finish after callback returned")
	}
}

func TestApprovalAdmissionSuppressesCallbackAndLaterToolLifecycle(t *testing.T) {
	for _, cause := range []struct {
		name string
		kind causeKind
		err  error
	}{
		{name: "caller cancellation", kind: causeCallerCancelled, err: context.Canceled},
		{name: "heartbeat lease loss", kind: causeLeaseLost, err: ErrLeaseLost},
	} {
		t.Run(cause.name, func(t *testing.T) {
			coordinator := newTurnCoordinator(context.Background())
			coordinator.setStage(memory.StageToolApproval)
			coordinator.callbackMu.Lock()

			admissionAttempted := make(chan struct{})
			admissionDecision := make(chan tools.Decision, 1)
			selectionDone := make(chan bool, 1)
			lifecycleDone := make(chan error, 1)
			frontendCalled := false
			observed := false
			ran := false
			var authorizations []tools.AuthorizationBoundary
			tool := echoTool("gated", true, &ran)
			call := toolCall("call", "gated", `{}`)
			go func() {
				_, _, err := tools.ExecuteWithApprovalAuthorized(
					coordinator.ctx,
					[]tools.Tool{tool},
					call,
					func(ctx context.Context, name, args string, preview *tools.FileChangePreview) tools.Decision {
						close(admissionAttempted)
						decision := admitApproval(
							coordinator,
							func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
								frontendCalled = true
								return tools.Approved
							},
							ctx,
							name,
							args,
							preview,
						)
						admissionDecision <- decision
						return decision
					},
					func(context.Context, tools.Decision) error {
						observed = true
						return nil
					},
					func(_ context.Context, boundary tools.AuthorizationBoundary) error {
						authorizations = append(authorizations, boundary)
						return nil
					},
				)
				lifecycleDone <- err
			}()
			select {
			case <-admissionAttempted:
			case <-time.After(time.Second):
				coordinator.callbackMu.Unlock()
				t.Fatal("tool lifecycle did not reach approval admission")
			}
			go func() {
				selectionDone <- coordinator.selectCause(cause.kind, cause.err, 0)
			}()
			select {
			case <-coordinator.ctx.Done():
			case <-time.After(time.Second):
				coordinator.callbackMu.Unlock()
				t.Fatal("terminal cause did not cancel approval context")
			}
			coordinator.callbackMu.Unlock()

			if selected := <-selectionDone; !selected {
				t.Fatal("terminal cause was not selected")
			}
			if err := <-lifecycleDone; !errors.Is(err, context.Canceled) {
				t.Fatalf("tool lifecycle error=%v, want context.Canceled", err)
			}
			if decision := <-admissionDecision; decision != tools.Expired {
				t.Fatalf("suppressed approval decision=%v, want Expired", decision)
			}
			if frontendCalled || observed || ran {
				t.Fatalf("frontend=%v observed=%v ran=%v", frontendCalled, observed, ran)
			}
			if len(authorizations) != 1 || authorizations[0] != tools.AuthorizePreparation {
				t.Fatalf("authorizations=%v, want preparation only", authorizations)
			}
			if selected := coordinator.result(); selected.kind != cause.kind || selected.stage != memory.StageToolApproval {
				t.Fatalf("cause=%+v, want kind=%v stage=%q", selected, cause.kind, memory.StageToolApproval)
			}
		})
	}
}

func TestLeaseAcquisitionConflictDoesNoTurnWorkOrRelease(t *testing.T) {
	owner := &scriptedOwner{acquireErr: errFakeConflict}
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	err := ownedSession(client, history, owner).Send(context.Background(), "hello", &recorder{}, nil)
	if !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("Send error = %v, want ErrLeaseConflict", err)
	}
	acquires, _, authorizations, releases := owner.counts()
	if acquires != 1 || authorizations != 0 || releases != 0 || len(history.events) != 0 || len(client.reqs) != 0 {
		t.Fatalf("acquire=%d authorize=%d release=%d events=%d requests=%d", acquires, authorizations, releases, len(history.events), len(client.reqs))
	}
}

func TestCancellationAfterAcquisitionBeforeRootReleasesOnly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	owner := &scriptedOwner{afterAcquire: cancel}
	history := &fakeHistory{}
	err := ownedSession(&fakeClient{}, history, owner).Send(ctx, "hello", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	_, _, authorizations, releases := owner.counts()
	if authorizations != 0 || releases != 1 || len(history.events) != 0 {
		t.Fatalf("authorize=%d release=%d events=%+v", authorizations, releases, history.events)
	}
}

type rolledBackRootHistory struct {
	cancel context.CancelFunc
}

func (h rolledBackRootHistory) Append(context.Context, memory.TurnLease, memory.EventInput) (memory.Event, error) {
	h.cancel()
	return memory.Event{}, context.Canceled
}

func (rolledBackRootHistory) Events(context.Context) ([]memory.Event, error) {
	return nil, errors.New("provider history must not be loaded")
}

func TestCancellationDuringRolledBackRootAppendReleasesWithoutLaterWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	owner := &scriptedOwner{}
	client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	s := New(client, "test", rolledBackRootHistory{cancel: cancel}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, owner)
	err := s.Send(ctx, "hello", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context.Canceled", err)
	}
	_, _, authorizations, releases := owner.counts()
	if authorizations != 0 || releases != 1 || len(client.reqs) != 0 {
		t.Fatalf("authorizations=%d releases=%d provider requests=%d", authorizations, releases, len(client.reqs))
	}
}

func TestPreRootBlockedAppendReturnsSelectedHeartbeatOrCallerCause(t *testing.T) {
	for _, tt := range []struct {
		name              string
		heartbeatErr      error
		cancelCaller      bool
		want              error
		wantHeartbeatRuns int
	}{
		{name: "definitive lease loss", heartbeatErr: errFakeLeaseLost, want: ErrLeaseLost, wantHeartbeatRuns: 1},
		{name: "heartbeat storage failure", heartbeatErr: errors.New("heartbeat disk failure"), want: ErrLeaseHeartbeatFailed, wantHeartbeatRuns: 1},
		{name: "caller wins before heartbeat", heartbeatErr: errors.New("later heartbeat failure"), cancelCaller: true, want: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			history := &fakeHistory{appendBlockAt: 1, appendEntered: make(chan struct{})}
			owner := &scriptedOwner{heartbeatErr: tt.heartbeatErr}
			client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
			s := ownedSession(client, history, owner)
			ticks := useManualHeartbeatTicker(s)
			done := make(chan error, 1)
			go func() { done <- s.Send(ctx, "go", &recorder{}, nil) }()
			select {
			case <-history.appendEntered:
			case <-time.After(time.Second):
				t.Fatal("root append did not block")
			}
			if tt.cancelCaller {
				cancel()
			} else {
				ticks <- time.Now()
			}
			err := <-done
			if !errors.Is(err, tt.want) {
				t.Fatalf("Send error=%v, want %v", err, tt.want)
			}
			acquires, heartbeats, authorizes, releases := owner.counts()
			if acquires != 1 || heartbeats != tt.wantHeartbeatRuns || authorizes != 0 || releases != 1 ||
				history.appendAttempts != 1 || len(history.events) != 0 || len(client.reqs) != 0 {
				t.Fatalf("acquire=%d heartbeat=%d authorize=%d release=%d appends=%d events=%d provider=%d", acquires, heartbeats, authorizes, releases, history.appendAttempts, len(history.events), len(client.reqs))
			}
		})
	}
}

func TestEveryAcquiredTurnAttemptsOneReleaseAndJoinsFailure(t *testing.T) {
	releaseErr := errors.New("release storage unavailable")
	owner := &scriptedOwner{releaseErr: releaseErr}
	s := ownedSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, &fakeHistory{}, owner)
	err := s.Send(context.Background(), "hello", &recorder{}, nil)
	if !errors.Is(err, releaseErr) {
		t.Fatalf("Send error = %v, want joined release error", err)
	}
	_, _, _, releases := owner.counts()
	if releases != 1 {
		t.Fatalf("release attempts = %d, want 1", releases)
	}
}

func TestTerminalAndReleaseCleanupAreBoundedAndErrorsAreJoined(t *testing.T) {
	t.Run("terminal append", func(t *testing.T) {
		providerErr := errors.New("provider unavailable")
		terminalErr := errors.New("terminal storage unavailable")
		history := &fakeHistory{appendErrAt: 2, appendErr: terminalErr}
		s := ownedSession(&fakeClient{steps: []step{{err: providerErr}}}, history, &scriptedOwner{})
		err := s.Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, providerErr) || !errors.Is(err, terminalErr) {
			t.Fatalf("Send error=%v, want provider and terminal errors", err)
		}
		if len(history.events) != 1 || history.events[0].Type != memory.EventUserMessage {
			t.Fatalf("accepted state=%+v", history.events)
		}
	})

	t.Run("terminal timeout", func(t *testing.T) {
		providerErr := errors.New("provider unavailable")
		history := &fakeHistory{appendBlockAt: 2}
		s := ownedSession(&fakeClient{steps: []step{{err: providerErr}}}, history, &scriptedOwner{})
		s.timing.cleanupTimeout = 5 * time.Millisecond
		err := s.Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, providerErr) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send error=%v, want provider and terminal deadline errors", err)
		}
		if len(history.events) != 1 || history.events[0].Type != memory.EventUserMessage {
			t.Fatalf("accepted state=%+v", history.events)
		}
	})

	t.Run("release timeout", func(t *testing.T) {
		owner := &scriptedOwner{releaseBlock: true}
		s := ownedSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, &fakeHistory{}, owner)
		s.timing.cleanupTimeout = 5 * time.Millisecond
		err := s.Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send error=%v, want release deadline", err)
		}
		_, _, _, releases := owner.counts()
		if releases != 1 {
			t.Fatalf("release attempts=%d", releases)
		}
	})
}

type cancelAwareClient struct {
	streamContent   string
	streamReasoning string
	streamed        chan<- struct{}
}

type callbacksAfterCoordinatorCancelClient struct{}

func (callbacksAfterCoordinatorCancelClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	<-ctx.Done()
	if h.OnReasoning != nil {
		h.OnReasoning("late reasoning")
	}
	if h.OnContent != nil {
		h.OnContent("late content")
	}
	return openrouter.ChatResponse{}, ctx.Err()
}

func (c cancelAwareClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if c.streamReasoning != "" && h.OnReasoning != nil {
		h.OnReasoning(c.streamReasoning)
	}
	if c.streamContent != "" && h.OnContent != nil {
		h.OnContent(c.streamContent)
	}
	if c.streamed != nil {
		close(c.streamed)
	}
	<-ctx.Done()
	return openrouter.ChatResponse{}, ctx.Err()
}

func TestHeartbeatFailuresCancelAndNeverPersistTerminalEvidence(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		reason DiscardReason
	}{
		{name: "definitive loss", err: errFakeLeaseLost, reason: DiscardLeaseLost},
		{name: "storage uncertainty", err: errors.New("heartbeat disk error"), reason: DiscardLeaseHeartbeatFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			owner := &scriptedOwner{heartbeatErr: tt.err}
			history := &fakeHistory{}
			events := &recorder{}
			streamed := make(chan struct{})
			s := ownedSession(cancelAwareClient{streamContent: "partial", streamed: streamed}, history, owner)
			ticks := useManualHeartbeatTicker(s)
			done := make(chan error, 1)
			go func() { done <- s.Send(context.Background(), "hello", events, nil) }()
			select {
			case <-streamed:
			case <-time.After(time.Second):
				t.Fatal("provider did not stream before heartbeat trigger")
			}
			ticks <- time.Now()
			err := <-done
			if err == nil {
				t.Fatal("Send succeeded after heartbeat failure")
			}
			for _, event := range history.events {
				if event.Type == memory.EventTurnFailed || event.Type == memory.EventTurnInterrupted {
					t.Fatalf("heartbeat failure persisted terminal event %+v", event)
				}
			}
			wantMarker := "discarded:" + string(tt.reason) + ":" + DiscardedResponseMessage
			if !containsString(events.events, wantMarker) {
				t.Fatalf("events = %v, want %q", events.events, wantMarker)
			}
			_, heartbeats, _, releases := owner.counts()
			if heartbeats != 1 || releases != 1 {
				t.Fatalf("heartbeats=%d releases=%d, want 1 each", heartbeats, releases)
			}
		})
	}
}

func TestReasoningAndContentCallbacksCannotBeginAfterCallerOrHeartbeatCause(t *testing.T) {
	t.Run("caller", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakeClient{
			onCall: cancel,
			steps: []step{{
				reasoning: []string{"late reasoning"},
				deltas:    []string{"late content"},
				res:       assistantStep("late content", nil).res,
			}},
		}
		events := &recorder{}
		err := ownedSession(client, &fakeHistory{}, &scriptedOwner{}).Send(ctx, "go", events, nil)
		if !errors.Is(err, context.Canceled) || len(events.events) != 0 {
			t.Fatalf("Send error=%v callbacks=%v", err, events.events)
		}
	})

	t.Run("heartbeat", func(t *testing.T) {
		owner := &scriptedOwner{heartbeatErr: errFakeLeaseLost}
		events := &recorder{}
		s := ownedSession(callbacksAfterCoordinatorCancelClient{}, &fakeHistory{}, owner)
		ticks := useManualHeartbeatTicker(s)
		ticks <- time.Now()
		err := s.Send(context.Background(), "go", events, nil)
		if !errors.Is(err, ErrLeaseLost) || len(events.events) != 0 {
			t.Fatalf("Send error=%v callbacks=%v", err, events.events)
		}
	})
}

func TestProviderAndToolStartsUseDurableAuthorizationFences(t *testing.T) {
	tests := []struct {
		name               string
		tool               tools.Tool
		approve            tools.Approver
		wantAuthorizations int
	}{
		{name: "ungated", tool: echoTool("echo", false, nil), wantAuthorizations: 3},
		{name: "gated approved", tool: echoTool("echo", true, nil), approve: func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }, wantAuthorizations: 4},
		{name: "gated declined", tool: echoTool("echo", true, nil), approve: func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Declined }, wantAuthorizations: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := &scriptedOwner{}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
				assistantStep("done", nil),
			}}
			err := ownedSession(client, &fakeHistory{}, owner).Send(context.Background(), "go", &recorder{}, tt.approve, tt.tool)
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			_, _, got, _ := owner.counts()
			if got != tt.wantAuthorizations {
				t.Fatalf("durable authorizations = %d, want %d", got, tt.wantAuthorizations)
			}
		})
	}
}

func TestAuthorizationFailureSuppressesProviderAndToolStarts(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		owner := &scriptedOwner{authorizeErrAt: 1, authorizeErr: errFakeLeaseLost}
		client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
		err := ownedSession(client, &fakeHistory{}, owner).Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, ErrLeaseLost) || len(client.reqs) != 0 {
			t.Fatalf("Send error=%v provider requests=%d", err, len(client.reqs))
		}
	})

	for _, tt := range []struct {
		name   string
		gated  bool
		failAt int
	}{
		{name: "ungated execution", failAt: 2},
		{name: "gated preparation", gated: true, failAt: 2},
		{name: "gated post-approval execution", gated: true, failAt: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ran := false
			owner := &scriptedOwner{authorizeErrAt: tt.failAt, authorizeErr: errFakeLeaseLost}
			client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call-1", "echo", `{}`))}}
			approve := tools.Approver(nil)
			if tt.gated {
				approve = func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }
			}
			err := ownedSession(client, &fakeHistory{}, owner).Send(context.Background(), "go", &recorder{}, approve, echoTool("echo", tt.gated, &ran))
			if !errors.Is(err, ErrLeaseLost) || ran {
				t.Fatalf("Send error=%v tool ran=%v", err, ran)
			}
		})
	}
}

func TestAssistantPersistenceFailureIsLocalAndMarksRenderedOutput(t *testing.T) {
	for _, tt := range []struct {
		name string
		step step
	}{
		{name: "content", step: assistantStep("partial", []string{"partial"})},
		{name: "reasoning only", step: reasoningStep("unrendered final", []string{"thinking"}, nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			history := &fakeHistory{appendErrAt: 2, appendErr: errors.New("assistant write failed")}
			events := &recorder{}
			err := ownedSession(&fakeClient{steps: []step{tt.step}}, history, &scriptedOwner{}).Send(context.Background(), "go", events, nil)
			if err == nil || !containsString(events.events, "discarded:assistant_persistence_failed:"+DiscardedResponseMessage) {
				t.Fatalf("Send error=%v events=%v", err, events.events)
			}
			if len(history.events) != 1 {
				t.Fatalf("durable events = %+v, want root only", history.events)
			}
		})
	}
}

func TestProviderTerminalPayloadIsSafeAndCorrelated(t *testing.T) {
	status := 503
	providerErr := &openrouter.StreamError{Kind: openrouter.StreamProviderError, HTTPStatus: status, Err: errors.New("secret URL https://provider.invalid body=raw")}
	history := &fakeHistory{}
	err := ownedSession(&fakeClient{steps: []step{{err: providerErr}}}, history, &scriptedOwner{}).Send(context.Background(), "go", &recorder{}, nil)
	if err == nil || len(history.events) != 2 {
		t.Fatalf("Send error=%v events=%+v", err, history.events)
	}
	terminal := history.events[1]
	var payload memory.TurnTerminalPayload
	if decodeErr := json.Unmarshal(terminal.Payload, &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if terminal.Type != memory.EventTurnFailed || terminal.ParentID != history.events[0].ID ||
		payload.TurnID != history.events[0].ID || payload.Classification != memory.ClassificationProviderError ||
		payload.Stage != memory.StageProvider || payload.HTTPStatus == nil || *payload.HTTPStatus != status {
		t.Fatalf("terminal=%+v payload=%+v", terminal, payload)
	}
	if strings.Contains(string(terminal.Payload), "secret") || strings.Contains(terminal.Content, "secret") {
		t.Fatalf("terminal leaked provider detail: content=%q payload=%s", terminal.Content, terminal.Payload)
	}
}

func TestProviderCycleParentageUsesLatestDurableTriggerAndRootTurnID(t *testing.T) {
	providerErr := errors.New("second provider request failed")
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
		{err: providerErr},
	}}
	err := ownedSession(client, history, &scriptedOwner{}).Send(
		context.Background(), "go", &recorder{}, nil, echoTool("echo", false, nil),
	)
	if !errors.Is(err, providerErr) || len(history.events) != 5 {
		t.Fatalf("Send error=%v events=%+v", err, history.events)
	}
	root, assistant, outcome, terminal := history.events[0], history.events[1], history.events[3], history.events[4]
	if assistant.ParentID != root.ID || terminal.ParentID != outcome.ID {
		t.Fatalf("root=%+v assistant=%+v outcome=%+v terminal=%+v", root, assistant, outcome, terminal)
	}
	var payload memory.TurnTerminalPayload
	if decodeErr := json.Unmarshal(terminal.Payload, &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if payload.TurnID != root.ID || payload.Stage != memory.StageProvider ||
		payload.Classification != memory.ClassificationProviderError {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestCallerDeadlinePersistsInterruptedClassification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	history := &fakeHistory{}
	err := ownedSession(cancelAwareClient{}, history, &scriptedOwner{}).Send(ctx, "go", &recorder{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) || len(history.events) != 2 {
		t.Fatalf("Send error=%v events=%+v", err, history.events)
	}
	var payload memory.TurnTerminalPayload
	if decodeErr := json.Unmarshal(history.events[1].Payload, &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if history.events[1].Type != memory.EventTurnInterrupted ||
		payload.Classification != memory.ClassificationCallerDeadlineExceeded || payload.Stage != memory.StageProvider {
		t.Fatalf("terminal=%+v payload=%+v", history.events[1], payload)
	}
}

func TestStructurallyInvalidProviderResponsesPersistInvalidClassification(t *testing.T) {
	tests := []struct {
		name string
		res  openrouter.ChatResponse
	}{
		{name: "no choices", res: openrouter.ChatResponse{}},
		{name: "empty choice", res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant"}}}}},
		{name: "empty call ID", res: assistantStep("", nil, toolCall("", "echo", `{}`)).res},
		{name: "duplicate call ID", res: assistantStep("", nil, toolCall("same", "one", `{}`), toolCall("same", "two", `{}`)).res},
		{name: "wrong call type", res: assistantStep("", nil, openrouter.ToolCall{ID: "call", Type: "custom", Function: openrouter.FunctionCall{Name: "echo"}}).res},
		{name: "empty function name", res: assistantStep("", nil, toolCall("call", "", `{}`)).res},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &fakeHistory{}
			err := ownedSession(&fakeClient{steps: []step{{res: tt.res}}}, history, &scriptedOwner{}).Send(context.Background(), "go", &recorder{}, nil)
			if err == nil || len(history.events) != 2 {
				t.Fatalf("Send error=%v events=%+v", err, history.events)
			}
			var payload memory.TurnTerminalPayload
			if decodeErr := json.Unmarshal(history.events[1].Payload, &payload); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if payload.Classification != memory.ClassificationProviderResponseInvalid || payload.Stage != memory.StageProvider {
				t.Fatalf("payload=%+v", payload)
			}
		})
	}
}

func TestInvalidStreamCompletionAndToolIndexPersistInvalidEvidence(t *testing.T) {
	for _, tt := range []struct {
		name       string
		deltas     []string
		wantMarker bool
	}{
		{name: "missing completion sentinel", deltas: []string{"partial"}, wantMarker: true},
		{name: "unsafe tool index"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			history := &fakeHistory{}
			events := &recorder{}
			err := ownedSession(&fakeClient{steps: []step{{
				deltas: tt.deltas,
				err: &openrouter.StreamError{
					Kind: openrouter.StreamProviderResponseInvalid,
					Err:  errors.New(tt.name),
				},
			}}}, history, &scriptedOwner{}).Send(context.Background(), "go", events, nil)
			if err == nil || len(history.events) != 2 {
				t.Fatalf("Send error=%v events=%+v", err, history.events)
			}
			var payload memory.TurnTerminalPayload
			if decodeErr := json.Unmarshal(history.events[1].Payload, &payload); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if payload.Classification != memory.ClassificationProviderResponseInvalid ||
				payload.Stage != memory.StageProvider {
				t.Fatalf("payload=%+v", payload)
			}
			marker := "discarded:provider_response_invalid:" + DiscardedResponseMessage
			if containsString(events.events, marker) != tt.wantMarker {
				t.Fatalf("events=%v marker wanted=%v", events.events, tt.wantMarker)
			}
		})
	}
}

func TestMissingStreamToolIndexPersistsInvalidWithoutToolWork(t *testing.T) {
	history := &fakeHistory{}
	events := &recorder{}
	owner := &scriptedOwner{}
	ran := false
	client := &fakeClient{steps: []step{{
		err: &openrouter.StreamError{
			Kind: openrouter.StreamProviderResponseInvalid,
			Err:  errors.New("provider tool call fragment is missing its index"),
		},
	}}}
	err := ownedSession(client, history, owner).Send(
		context.Background(),
		"go",
		events,
		nil,
		echoTool("echo", false, &ran),
	)
	if err == nil || ran || len(history.events) != 2 {
		t.Fatalf("Send error=%v tool ran=%v events=%+v", err, ran, history.events)
	}
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(history.events[1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Classification != memory.ClassificationProviderResponseInvalid ||
		payload.Stage != memory.StageProvider {
		t.Fatalf("payload=%+v", payload)
	}
	for _, callback := range events.events {
		if strings.HasPrefix(callback, "call:") || strings.HasPrefix(callback, "result:") {
			t.Fatalf("tool callback emitted: %v", events.events)
		}
	}
	_, heartbeats, authorizations, _ := owner.counts()
	if heartbeats != 0 || authorizations != 1 {
		t.Fatalf("heartbeats=%d authorizations=%d, want provider authorization only", heartbeats, authorizations)
	}
}

func TestAssistantCommitInterruptionPersistsExactEvidenceAndSuppressesLaterWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{appendBlockAt: 2, appendEntered: make(chan struct{})}
	events := &recorder{}
	owner := &scriptedOwner{}
	client := &fakeClient{steps: []step{assistantStep("final", []string{"partial"})}}
	s := ownedSession(client, history, owner)
	done := make(chan error, 1)
	go func() { done <- s.Send(ctx, "go", events, nil) }()
	select {
	case <-history.appendEntered:
	case <-time.After(time.Second):
		t.Fatal("assistant append did not start")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context.Canceled", err)
	}
	if len(history.events) != 2 || history.events[0].Type != memory.EventUserMessage ||
		history.events[1].Type != memory.EventTurnInterrupted {
		t.Fatalf("events=%+v", history.events)
	}
	root, terminal := history.events[0], history.events[1]
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if terminal.ParentID != root.ID || payload.TurnID != root.ID ||
		payload.Classification != memory.ClassificationCallerCancelled ||
		payload.Stage != memory.StageAssistantCommit {
		t.Fatalf("root=%+v terminal=%+v payload=%+v", root, terminal, payload)
	}
	if containsString(events.events, "done:final") ||
		!containsString(events.events, "discarded:caller_cancelled:"+DiscardedResponseMessage) {
		t.Fatalf("callbacks=%v", events.events)
	}
	_, _, authorizes, releases := owner.counts()
	if len(client.reqs) != 1 || authorizes != 1 || releases != 1 {
		t.Fatalf("provider=%d authorizes=%d releases=%d", len(client.reqs), authorizes, releases)
	}
}

func TestToolCallingAssistantCallbackSuppressedWhenAppendReturnsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{}
	history.afterAppend = func(input memory.EventInput) {
		if input.Type == memory.EventAssistantMessage {
			cancel()
		}
	}
	events := &recorder{}
	client := &fakeClient{steps: []step{assistantStep("calling", []string{"calling"}, toolCall("call", "echo", `{}`))}}
	err := ownedSession(client, history, &scriptedOwner{}).Send(ctx, "go", events, nil, echoTool("echo", false, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context.Canceled", err)
	}
	for _, event := range events.events {
		if strings.HasPrefix(event, "done:") || strings.HasPrefix(event, "call:") || strings.HasPrefix(event, "result:") {
			t.Fatalf("stale callback emitted after cancellation: %v", events.events)
		}
	}
	if len(history.events) != 3 || history.events[1].Type != memory.EventAssistantMessage ||
		history.events[2].Type != memory.EventTurnInterrupted {
		t.Fatalf("durable events=%+v", history.events)
	}
}

func TestToolCallingAssistantCallbacksSuppressedWhenAppendLosesLease(t *testing.T) {
	history := &fakeHistory{appendErrAt: 2, appendErr: errFakeLeaseLost}
	events := &recorder{}
	client := &fakeClient{steps: []step{assistantStep("calling", []string{"calling"}, toolCall("call", "echo", `{}`))}}
	err := ownedSession(client, history, &scriptedOwner{}).Send(
		context.Background(), "go", events, nil, echoTool("echo", false, nil),
	)
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Send error=%v, want ErrLeaseLost", err)
	}
	for _, event := range events.events {
		if strings.HasPrefix(event, "done:") || strings.HasPrefix(event, "call:") || strings.HasPrefix(event, "result:") {
			t.Fatalf("stale callback emitted after lease loss: %v", events.events)
		}
	}
	if !containsString(events.events, "discarded:lease_lost:"+DiscardedResponseMessage) ||
		len(history.events) != 1 {
		t.Fatalf("callbacks=%v durable events=%+v", events.events, history.events)
	}
}

func TestMultiToolCallerCancellationAfterFirstResultTransitionsToToolPrepare(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	secondRan := false
	history := &fakeHistory{}
	events := &recorder{onToolResult: cancel}
	client := &fakeClient{steps: []step{assistantStep("", nil,
		toolCall("call-1", "first", `{}`),
		toolCall("call-2", "second", `{}`),
	)}}
	err := ownedSession(client, history, &scriptedOwner{}).Send(
		ctx,
		"go",
		events,
		nil,
		echoTool("first", false, nil),
		echoTool("second", false, &secondRan),
	)
	if !errors.Is(err, context.Canceled) || secondRan {
		t.Fatalf("Send error=%v second tool ran=%v", err, secondRan)
	}
	terminal := history.events[len(history.events)-1]
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if terminal.Type != memory.EventTurnInterrupted || payload.Stage != memory.StageToolPrepare ||
		payload.Classification != memory.ClassificationCallerCancelled {
		t.Fatalf("terminal=%+v payload=%+v", terminal, payload)
	}
	if containsString(events.events, `call:call-2:second:{}`) {
		t.Fatalf("second tool callback emitted: %v", events.events)
	}
}

func TestFinalToolResultCallerCancellationRemainsAtToolCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{}
	events := &recorder{onToolResult: cancel}
	client := &fakeClient{steps: []step{assistantStep("", nil,
		toolCall("call-1", "only", `{}`),
	)}}
	err := ownedSession(client, history, &scriptedOwner{}).Send(
		ctx,
		"go",
		events,
		nil,
		echoTool("only", false, nil),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v, want context.Canceled", err)
	}
	terminal := history.events[len(history.events)-1]
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if terminal.Type != memory.EventTurnInterrupted || payload.Stage != memory.StageToolCommit ||
		payload.Classification != memory.ClassificationCallerCancelled {
		t.Fatalf("terminal=%+v payload=%+v", terminal, payload)
	}
}

func TestMultiToolHeartbeatLossAfterFirstResultTransitionsToToolPrepare(t *testing.T) {
	heartbeatStarted := make(chan struct{})
	releaseHeartbeat := make(chan struct{})
	owner := &scriptedOwner{
		heartbeatErr:   errFakeLeaseLost,
		heartbeatStart: heartbeatStarted,
		heartbeatWait:  releaseHeartbeat,
	}
	history := &fakeHistory{events: []memory.Event{{
		ID: "root", SessionID: "test-session", Sequence: 1,
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go",
	}}}
	secondRan := false
	first := echoTool("first", false, nil)
	first.Execute = func(context.Context, string) (string, error) {
		select {
		case <-heartbeatStarted:
			return "first result", nil
		case <-time.After(time.Second):
			return "", errors.New("heartbeat did not start")
		}
	}
	coordinator := newTurnCoordinator(context.Background())
	coordinator.setStage(memory.StageTurnStart)
	events := &recorder{onToolResult: func() {
		close(releaseHeartbeat)
		select {
		case <-coordinator.ctx.Done():
		case <-time.After(time.Second):
			t.Error("heartbeat did not cancel the coordinator while callback was admitted")
		}
	}}
	client := &fakeClient{steps: []step{assistantStep("", nil,
		toolCall("call-1", "first", `{}`),
		toolCall("call-2", "second", `{}`),
	)}}
	s := ownedSession(client, history, owner)
	ticks := useManualHeartbeatTicker(s)
	ticks <- time.Now()
	lease := memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 7, Generation: 7}
	stopHeartbeat := s.startHeartbeat(context.Background(), coordinator, lease)
	defer stopHeartbeat()
	err := s.runOwnedTurn(
		coordinator,
		lease,
		events,
		nil,
		&turnProgress{requestParentID: "root"},
		[]tools.Tool{first, echoTool("second", false, &secondRan)},
	)
	if !errors.Is(err, ErrLeaseLost) || secondRan {
		t.Fatalf("runOwnedTurn error=%v second tool ran=%v", err, secondRan)
	}
	if cause := coordinator.result(); cause.kind != causeLeaseLost || cause.stage != memory.StageToolPrepare {
		t.Fatalf("cause=%+v", cause)
	}
	if containsString(events.events, `call:call-2:second:{}`) {
		t.Fatalf("second tool callback emitted: %v", events.events)
	}
}

type postCommitCause struct {
	name      string
	kind      causeKind
	err       error
	heartbeat bool
}

var postCommitCauses = []postCommitCause{
	{name: "caller", kind: causeCallerCancelled, err: context.Canceled},
	{name: "heartbeat lease loss", kind: causeLeaseLost, err: ErrLeaseLost, heartbeat: true},
}

func triggerPostCommitCause(
	t *testing.T,
	coordinator *turnCoordinator,
	ticks chan<- time.Time,
	cause postCommitCause,
) {
	t.Helper()
	if cause.heartbeat {
		ticks <- time.Now()
	} else {
		go coordinator.selectCause(cause.kind, cause.err, 0)
	}
	select {
	case <-coordinator.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("terminal cause was not reserved at post-commit boundary")
	}
}

func TestAssistantCommitTransitionsAtomicallyToToolPrepare(t *testing.T) {
	for _, cause := range postCommitCauses {
		t.Run(cause.name, func(t *testing.T) {
			owner := &scriptedOwner{}
			if cause.heartbeat {
				owner.heartbeatErr = errFakeLeaseLost
			}
			coordinator := newTurnCoordinator(context.Background())
			coordinator.setStage(memory.StageTurnStart)
			history := &fakeHistory{events: []memory.Event{{
				ID: "root", SessionID: "test-session", Sequence: 1,
				Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go",
			}}}
			client := &fakeClient{steps: []step{assistantStep("calling", nil,
				toolCall("call", "echo", `{}`),
			)}}
			s := ownedSession(client, history, owner)
			ticks := useManualHeartbeatTicker(s)
			triggered := false
			history.afterAppend = func(input memory.EventInput) {
				if input.Type == memory.EventAssistantMessage && !triggered {
					triggered = true
					triggerPostCommitCause(t, coordinator, ticks, cause)
				}
			}
			lease := memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 7, Generation: 7}
			stopHeartbeat := s.startHeartbeat(context.Background(), coordinator, lease)
			defer stopHeartbeat()
			ran := false
			events := &recorder{}
			err := s.runOwnedTurn(
				coordinator,
				lease,
				events,
				nil,
				&turnProgress{requestParentID: "root"},
				[]tools.Tool{echoTool("echo", false, &ran)},
			)
			if !errors.Is(err, cause.err) || ran {
				t.Fatalf("runOwnedTurn error=%v tool ran=%v", err, ran)
			}
			if selected := coordinator.result(); selected.kind != cause.kind || selected.stage != memory.StageToolPrepare {
				t.Fatalf("cause=%+v, want kind=%v stage=%q", selected, cause.kind, memory.StageToolPrepare)
			}
			_, heartbeats, authorizations, _ := owner.counts()
			wantHeartbeats := 0
			if cause.heartbeat {
				wantHeartbeats = 1
			}
			if !triggered || len(history.events) != 2 || len(events.events) != 0 ||
				len(client.reqs) != 1 || heartbeats != wantHeartbeats || authorizations != 1 {
				t.Fatalf("triggered=%v durable=%+v callbacks=%v", triggered, history.events, events.events)
			}
		})
	}
}

func TestApprovalCommitTransitionsAtomicallyBeforeCauseCapture(t *testing.T) {
	for _, decision := range []struct {
		name      string
		decision  tools.Decision
		wantStage memory.TurnStage
	}{
		{name: "approved", decision: tools.Approved, wantStage: memory.StageToolExecute},
		{name: "declined", decision: tools.Declined, wantStage: memory.StageToolCommit},
		{name: "expired", decision: tools.Expired, wantStage: memory.StageToolCommit},
	} {
		for _, cause := range postCommitCauses {
			t.Run(decision.name+"_"+cause.name, func(t *testing.T) {
				owner := &scriptedOwner{}
				if cause.heartbeat {
					owner.heartbeatErr = errFakeLeaseLost
				}
				coordinator := newTurnCoordinator(context.Background())
				coordinator.setStage(memory.StageTurnStart)
				history := &fakeHistory{events: []memory.Event{{
					ID: "root", SessionID: "test-session", Sequence: 1,
					Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go",
				}}}
				client := &fakeClient{steps: []step{assistantStep("", nil,
					toolCall("call", "gated", `{}`),
				)}}
				s := ownedSession(client, history, owner)
				ticks := useManualHeartbeatTicker(s)
				triggered := false
				history.afterAppend = func(input memory.EventInput) {
					if input.Type == memory.EventApproval && !triggered {
						triggered = true
						triggerPostCommitCause(t, coordinator, ticks, cause)
					}
				}
				lease := memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 7, Generation: 7}
				stopHeartbeat := s.startHeartbeat(context.Background(), coordinator, lease)
				defer stopHeartbeat()
				ran := false
				events := &recorder{}
				err := s.runOwnedTurn(
					coordinator,
					lease,
					events,
					func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
						return decision.decision
					},
					&turnProgress{requestParentID: "root"},
					[]tools.Tool{echoTool("gated", true, &ran)},
				)
				if !errors.Is(err, cause.err) || ran {
					t.Fatalf("runOwnedTurn error=%v tool ran=%v", err, ran)
				}
				if selected := coordinator.result(); selected.kind != cause.kind || selected.stage != decision.wantStage {
					t.Fatalf("cause=%+v, want kind=%v stage=%q", selected, cause.kind, decision.wantStage)
				}
				for _, callback := range events.events {
					if strings.HasPrefix(callback, "result:") {
						t.Fatalf("tool-result callback emitted after approval boundary cause: %v", events.events)
					}
				}
				_, heartbeats, authorizations, _ := owner.counts()
				wantHeartbeats := 0
				if cause.heartbeat {
					wantHeartbeats = 1
				}
				if !triggered || len(history.events) != 4 || len(client.reqs) != 1 ||
					heartbeats != wantHeartbeats || authorizations != 2 {
					t.Fatalf("triggered=%v durable=%+v callbacks=%v", triggered, history.events, events.events)
				}
			})
		}
	}
}

func TestCallerInterruptionCapturesExactLifecycleStage(t *testing.T) {
	tests := []struct {
		name  string
		stage memory.TurnStage
		run   func(context.Context, context.CancelFunc, *fakeHistory, *recorder) error
	}{
		{
			name: "turn start", stage: memory.StageTurnStart,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				history.afterAppend = func(input memory.EventInput) {
					if input.Type == memory.EventUserMessage {
						cancel()
					}
				}
				return ownedSession(&fakeClient{}, history, &scriptedOwner{}).Send(ctx, "go", events, nil)
			},
		},
		{
			name: "provider", stage: memory.StageProvider,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				history.onEvents = cancel
				return ownedSession(&fakeClient{}, history, &scriptedOwner{}).Send(ctx, "go", events, nil)
			},
		},
		{
			name: "assistant commit", stage: memory.StageAssistantCommit,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				history.appendBlockAt = 2
				history.appendEntered = make(chan struct{})
				done := make(chan error, 1)
				go func() {
					done <- ownedSession(
						&fakeClient{steps: []step{assistantStep("final", []string{"partial"})}},
						history,
						&scriptedOwner{},
					).Send(ctx, "go", events, nil)
				}()
				select {
				case <-history.appendEntered:
					cancel()
				case <-time.After(time.Second):
					return errors.New("assistant append did not start")
				}
				return <-done
			},
		},
		{
			name: "tool prepare", stage: memory.StageToolPrepare,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				events.onToolCall = cancel
				client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call", "echo", `{}`))}}
				return ownedSession(client, history, &scriptedOwner{}).Send(ctx, "go", events, nil, echoTool("echo", false, nil))
			},
		},
		{
			name: "tool approval", stage: memory.StageToolApproval,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call", "echo", `{}`))}}
				approve := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
					cancel()
					return tools.Approved
				}
				return ownedSession(client, history, &scriptedOwner{}).Send(ctx, "go", events, approve, echoTool("echo", true, nil))
			},
		},
		{
			name: "tool execute", stage: memory.StageToolExecute,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				tool := echoTool("echo", false, nil)
				tool.Execute = func(context.Context, string) (string, error) {
					cancel()
					return "", context.Canceled
				}
				client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call", "echo", `{}`))}}
				return ownedSession(client, history, &scriptedOwner{}).Send(ctx, "go", events, nil, tool)
			},
		},
		{
			name: "tool commit", stage: memory.StageToolCommit,
			run: func(ctx context.Context, cancel context.CancelFunc, history *fakeHistory, events *recorder) error {
				history.afterAppend = func(input memory.EventInput) {
					if input.Type == memory.EventToolSucceeded {
						cancel()
					}
				}
				client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call", "echo", `{}`))}}
				return ownedSession(client, history, &scriptedOwner{}).Send(ctx, "go", events, nil, echoTool("echo", false, nil))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			history := &fakeHistory{}
			err := tt.run(ctx, cancel, history, &recorder{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Send error=%v, want context.Canceled", err)
			}
			var terminal *memory.Event
			for i := range history.events {
				if history.events[i].Type == memory.EventTurnInterrupted {
					terminal = &history.events[i]
				}
			}
			if terminal == nil {
				t.Fatalf("events=%+v, want interruption", history.events)
			}
			var payload memory.TurnTerminalPayload
			if decodeErr := json.Unmarshal(terminal.Payload, &payload); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if payload.Stage != tt.stage || payload.Classification != memory.ClassificationCallerCancelled ||
				payload.TurnID != history.events[0].ID || terminal.ParentID != history.events[0].ID {
				t.Fatalf("payload=%+v want stage=%q", payload, tt.stage)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
