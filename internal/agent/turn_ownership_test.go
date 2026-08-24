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
	afterAcquire   func()
	acquires       int
	heartbeats     int
	authorizations int
	releases       int
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
	o.mu.Unlock()
	if started != nil {
		close(started)
	}
	if block {
		<-ctx.Done()
		return memory.TurnLease{}, ctx.Err()
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
		defaultTurnTiming.cleanupTimeout != 5*time.Second {
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
	s.timing.heartbeatInterval = time.Millisecond
	start := time.Now()
	if err := s.Send(context.Background(), "hello", &recorder{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("Send waited %v for in-flight heartbeat shutdown", elapsed)
	}
	_, heartbeats, _, releases := owner.counts()
	if heartbeats != 1 || releases != 1 {
		t.Fatalf("heartbeats=%d releases=%d, want one each", heartbeats, releases)
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
		start := time.Now()
		err := s.Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, providerErr) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send error=%v, want provider and terminal deadline errors", err)
		}
		if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
			t.Fatalf("bounded terminal append took %v", elapsed)
		}
		if len(history.events) != 1 || history.events[0].Type != memory.EventUserMessage {
			t.Fatalf("accepted state=%+v", history.events)
		}
	})

	t.Run("release timeout", func(t *testing.T) {
		owner := &scriptedOwner{releaseBlock: true}
		s := ownedSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, &fakeHistory{}, owner)
		s.timing.cleanupTimeout = 5 * time.Millisecond
		start := time.Now()
		err := s.Send(context.Background(), "go", &recorder{}, nil)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Send error=%v, want release deadline", err)
		}
		if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
			t.Fatalf("bounded release took %v", elapsed)
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
}

func (c cancelAwareClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if c.streamReasoning != "" && h.OnReasoning != nil {
		h.OnReasoning(c.streamReasoning)
	}
	if c.streamContent != "" && h.OnContent != nil {
		h.OnContent(c.streamContent)
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
			s := ownedSession(cancelAwareClient{streamContent: "partial"}, history, owner)
			s.timing.heartbeatInterval = time.Millisecond
			err := s.Send(context.Background(), "hello", events, nil)
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
