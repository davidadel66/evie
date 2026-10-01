package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func recordProviderRetryWaits(s *Session) *[]time.Duration {
	waits := &[]time.Duration{}
	s.timing.waitProviderRetry = func(ctx context.Context, delay time.Duration) error {
		*waits = append(*waits, delay)
		return ctx.Err()
	}
	return waits
}

func providerFailure(status int) *openrouter.StreamError {
	return &openrouter.StreamError{Kind: openrouter.StreamProviderError, HTTPStatus: status, Err: fmt.Errorf("status %d", status)}
}

func connectionFailure() *openrouter.StreamError {
	return &openrouter.StreamError{Kind: openrouter.StreamProviderError, NoResponse: true, Err: errors.New("connection reset by peer")}
}

func terminalPayloadOf(t *testing.T, event memory.Event) memory.TurnTerminalPayload {
	t.Helper()
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode terminal %+v: %v", event, err)
	}
	return payload
}

func finishStep(content, reason string, deltas []string, calls ...openrouter.ToolCall) step {
	s := assistantStep(content, deltas, calls...)
	s.res.Choices[0].FinishReason = reason
	return s
}

// L2: a generation the provider cut short is not durable success, and its
// truncated tool arguments are never stored or executed.
func TestTruncatedOrErroredGenerationIsNeverCommitted(t *testing.T) {
	for _, test := range []struct {
		name string
		step step
		want memory.TurnClassification
	}{
		{name: "length-truncated answer", step: finishStep("The answer is", "length", []string{"The answer is"}), want: memory.ClassificationProviderResponseInvalid},
		{name: "length-truncated tool call", step: finishStep("", "length", nil, toolCall("call-1", "echo", `{"path":"/fo`)), want: memory.ClassificationProviderResponseInvalid},
		{name: "provider error finish", step: finishStep("partial", "error", []string{"partial"}), want: memory.ClassificationProviderError},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{}
			owner := &scriptedOwner{}
			ran := false
			client := &fakeClient{steps: []step{test.step, assistantStep("must not run", nil)}}
			s := ownedSession(client, history, owner)
			waits := recordProviderRetryWaits(s)
			err := s.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, &ran))
			if err == nil || ran || len(client.reqs) != 1 || len(*waits) != 0 {
				t.Fatalf("Send error=%v tool ran=%v provider calls=%d waits=%v", err, ran, len(client.reqs), *waits)
			}
			if len(history.events) != 2 || history.events[1].Type != memory.EventTurnFailed {
				t.Fatalf("durable events=%+v, want root and failure only", history.events)
			}
			payload := terminalPayloadOf(t, history.events[1])
			if payload.Classification != test.want || payload.Stage != memory.StageProvider || payload.HTTPStatus != nil {
				t.Fatalf("payload=%+v", payload)
			}
			if _, _, _, releases := owner.counts(); releases != 1 {
				t.Fatalf("releases=%d", releases)
			}
		})
	}
}

// L3: once the transport gives up on a silent stream, the turn fails cleanly,
// is not retried, and the lease is released.
func TestIdleProviderStreamFailsTurnWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		deltas []string
	}{
		{name: "silent before output"},
		{name: "silent after partial output", deltas: []string{"partial"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			idle := &openrouter.StreamError{Kind: openrouter.StreamProviderError, Err: fmt.Errorf("read stream: %w", openrouter.ErrStreamIdle)}
			history := &fakeHistory{}
			owner := &scriptedOwner{}
			client := &fakeClient{steps: []step{{deltas: test.deltas, err: idle}, assistantStep("must not run", nil)}}
			s := ownedSession(client, history, owner)
			waits := recordProviderRetryWaits(s)
			err := s.Send(context.Background(), "go", &recorder{}, nil)
			if !errors.Is(err, openrouter.ErrStreamIdle) || len(client.reqs) != 1 || len(*waits) != 0 {
				t.Fatalf("Send error=%v provider calls=%d waits=%v", err, len(client.reqs), *waits)
			}
			if len(history.events) != 2 || terminalPayloadOf(t, history.events[1]).Classification != memory.ClassificationProviderError {
				t.Fatalf("events=%+v", history.events)
			}
			if _, _, _, releases := owner.counts(); releases != 1 {
				t.Fatalf("releases=%d", releases)
			}
		})
	}
}

// L4: transient failures before any output retry the identical request.
func TestTransientProviderFailureBeforeOutputIsRetried(t *testing.T) {
	for _, test := range []struct {
		name string
		err  *openrouter.StreamError
	}{
		{name: "no response", err: connectionFailure()},
		{name: "429", err: providerFailure(http.StatusTooManyRequests)},
		{name: "502", err: providerFailure(http.StatusBadGateway)},
		{name: "503", err: providerFailure(http.StatusServiceUnavailable)},
		{name: "504", err: providerFailure(http.StatusGatewayTimeout)},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{}
			owner := &scriptedOwner{}
			events := &recorder{}
			client := &fakeClient{steps: []step{{err: test.err}, assistantStep("recovered", []string{"recovered"})}}
			s := ownedSession(client, history, owner)
			waits := recordProviderRetryWaits(s)
			if err := s.Send(context.Background(), "go", events, nil); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if !reflect.DeepEqual(*waits, []time.Duration{time.Second}) || len(client.reqs) != 2 {
				t.Fatalf("waits=%v provider calls=%d", *waits, len(client.reqs))
			}
			first, _ := json.Marshal(client.reqs[0])
			second, _ := json.Marshal(client.reqs[1])
			if string(first) != string(second) {
				t.Fatal("retry did not resend the identical admitted request")
			}
			if len(history.events) != 2 || history.events[1].Type != memory.EventAssistantMessage || history.events[1].Content != "recovered" {
				t.Fatalf("events=%+v", history.events)
			}
			if history.snapshotCount != 1 {
				t.Fatalf("context snapshots=%d, want the one admitted request", history.snapshotCount)
			}
			if _, _, authorizations, releases := owner.counts(); authorizations != 2 || releases != 1 {
				t.Fatalf("authorizations=%d releases=%d, want one authorization per provider start", authorizations, releases)
			}
			if !containsString(events.events, "done:recovered") {
				t.Fatalf("events=%v", events.events)
			}
		})
	}
}

func TestProviderRetriesStopAfterTwoWithExponentialBackoff(t *testing.T) {
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		{err: providerFailure(http.StatusServiceUnavailable)},
		{err: providerFailure(http.StatusServiceUnavailable)},
		{err: providerFailure(http.StatusServiceUnavailable)},
		assistantStep("must not run", nil),
	}}
	s := ownedSession(client, history, &scriptedOwner{})
	waits := recordProviderRetryWaits(s)
	err := s.Send(context.Background(), "go", &recorder{}, nil)
	if err == nil || len(client.reqs) != 3 || !reflect.DeepEqual(*waits, []time.Duration{time.Second, 2 * time.Second}) {
		t.Fatalf("Send error=%v provider calls=%d waits=%v", err, len(client.reqs), *waits)
	}
	payload := terminalPayloadOf(t, history.events[1])
	if len(history.events) != 2 || payload.Classification != memory.ClassificationProviderError ||
		payload.HTTPStatus == nil || *payload.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("events=%+v payload=%+v", history.events, payload)
	}
}

func TestProviderRetryHonorsBoundedRetryAfter(t *testing.T) {
	for _, test := range []struct {
		name       string
		retryAfter time.Duration
		wantWaits  []time.Duration
		wantCalls  int
	}{
		{name: "longer than backoff", retryAfter: 7 * time.Second, wantWaits: []time.Duration{7 * time.Second}, wantCalls: 2},
		{name: "shorter than backoff", retryAfter: 200 * time.Millisecond, wantWaits: []time.Duration{time.Second}, wantCalls: 2},
		{name: "beyond retry bound fails fast", retryAfter: time.Minute, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			limited := providerFailure(http.StatusTooManyRequests)
			limited.RetryAfter = test.retryAfter
			client := &fakeClient{steps: []step{{err: limited}, assistantStep("recovered", nil)}}
			s := ownedSession(client, &fakeHistory{}, &scriptedOwner{})
			waits := recordProviderRetryWaits(s)
			_ = s.Send(context.Background(), "go", &recorder{}, nil)
			if len(client.reqs) != test.wantCalls || !reflect.DeepEqual(*waits, append([]time.Duration{}, test.wantWaits...)) {
				t.Fatalf("provider calls=%d waits=%v", len(client.reqs), *waits)
			}
		})
	}
}

func TestProviderFailureAfterAnyLiveCallbackIsNeverRetried(t *testing.T) {
	for _, test := range []struct {
		name string
		step step
	}{
		{name: "content delta", step: step{deltas: []string{"partial"}, err: providerFailure(http.StatusServiceUnavailable)}},
		{name: "reasoning", step: step{reasoning: []string{"thinking"}, err: connectionFailure()}},
		{name: "reasoning activity start", step: step{reasoning: []string{""}, err: providerFailure(http.StatusBadGateway)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{}
			client := &fakeClient{steps: []step{test.step, assistantStep("must not run", nil)}}
			s := ownedSession(client, history, &scriptedOwner{})
			waits := recordProviderRetryWaits(s)
			err := s.Send(context.Background(), "go", &recorder{}, nil)
			if err == nil || len(client.reqs) != 1 || len(*waits) != 0 {
				t.Fatalf("Send error=%v provider calls=%d waits=%v", err, len(client.reqs), *waits)
			}
			if len(history.events) != 2 || terminalPayloadOf(t, history.events[1]).Classification != memory.ClassificationProviderError {
				t.Fatalf("events=%+v", history.events)
			}
		})
	}
}

func TestNonTransientProviderFailuresAreNotRetried(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "500", err: providerFailure(http.StatusInternalServerError)},
		{name: "400", err: providerFailure(http.StatusBadRequest)},
		{name: "401", err: providerFailure(http.StatusUnauthorized)},
		{name: "408", err: providerFailure(http.StatusRequestTimeout)},
		{name: "transport failure after a response", err: &openrouter.StreamError{Kind: openrouter.StreamProviderError, Err: errors.New("read stream: reset")}},
		{name: "invalid response", err: &openrouter.StreamError{Kind: openrouter.StreamProviderResponseInvalid, HTTPStatus: http.StatusServiceUnavailable, NoResponse: true, Err: errors.New("bad")}},
		{name: "unclassified", err: errors.New("delegation policy")},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{steps: []step{{err: test.err}, assistantStep("must not run", nil)}}
			s := ownedSession(client, &fakeHistory{}, &scriptedOwner{})
			waits := recordProviderRetryWaits(s)
			if err := s.Send(context.Background(), "go", &recorder{}, nil); err == nil {
				t.Fatal("Send succeeded")
			}
			if len(client.reqs) != 1 || len(*waits) != 0 {
				t.Fatalf("provider calls=%d waits=%v", len(client.reqs), *waits)
			}
		})
	}
}

func TestProviderRetryBackoffStopsOnCallerCancellation(t *testing.T) {
	history := &fakeHistory{}
	owner := &scriptedOwner{}
	client := &fakeClient{steps: []step{{err: providerFailure(http.StatusServiceUnavailable)}, assistantStep("must not run", nil)}}
	s := ownedSession(client, history, owner)
	waiting := make(chan struct{})
	s.timing.waitProviderRetry = func(ctx context.Context, _ time.Duration) error {
		close(waiting)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Send(ctx, "go", &recorder{}, nil) }()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("turn never entered retry backoff")
	}
	cancel()
	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("caller cancellation did not interrupt retry backoff")
	}
	if !errors.Is(err, context.Canceled) || len(client.reqs) != 1 {
		t.Fatalf("Send error=%v provider calls=%d", err, len(client.reqs))
	}
	if len(history.events) != 2 || history.events[1].Type != memory.EventTurnInterrupted {
		t.Fatalf("events=%+v", history.events)
	}
	payload := terminalPayloadOf(t, history.events[1])
	if payload.Classification != memory.ClassificationCallerCancelled || payload.Stage != memory.StageProvider {
		t.Fatalf("payload=%+v", payload)
	}
	if _, _, _, releases := owner.counts(); releases != 1 {
		t.Fatalf("releases=%d", releases)
	}
}

func TestProviderRetryReauthorizesLeaseBeforeStarting(t *testing.T) {
	history := &fakeHistory{}
	owner := &scriptedOwner{authorizeErrAt: 2, authorizeErr: errFakeLeaseLost}
	client := &fakeClient{steps: []step{{err: providerFailure(http.StatusBadGateway)}, assistantStep("must not run", nil)}}
	s := ownedSession(client, history, owner)
	recordProviderRetryWaits(s)
	err := s.Send(context.Background(), "go", &recorder{}, nil)
	if !errors.Is(err, ErrLeaseLost) || len(client.reqs) != 1 {
		t.Fatalf("Send error=%v provider calls=%d", err, len(client.reqs))
	}
	if len(history.events) != 1 {
		t.Fatalf("stale owner appended evidence: %+v", history.events)
	}
}

func TestDefaultProviderRetryTimingWaitsAndHonorsCancellation(t *testing.T) {
	if defaultTurnTiming.providerRetryBase != time.Second || defaultTurnTiming.waitProviderRetry == nil {
		t.Fatalf("timing=%+v", defaultTurnTiming)
	}
	if err := defaultTurnTiming.waitProviderRetry(context.Background(), time.Millisecond); err != nil {
		t.Fatalf("short wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := defaultTurnTiming.waitProviderRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) || time.Since(started) > time.Second {
		t.Fatalf("cancelled wait err=%v after %s", err, time.Since(started))
	}
	if maxProviderRetries != 2 {
		t.Fatalf("maxProviderRetries=%d", maxProviderRetries)
	}
}
