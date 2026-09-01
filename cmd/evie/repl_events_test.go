package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type pinnedREPLToolClient struct {
	requests []openrouter.ChatRequest
}

func (c *pinnedREPLToolClient) ChatStream(
	_ context.Context,
	req openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, req)
	if len(c.requests) == 1 {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant", ToolCalls: []openrouter.ToolCall{{
				ID: "call-1", Type: "function",
				Function: openrouter.FunctionCall{Name: "repl_only", Arguments: `{}`},
			}},
		}}}}, nil
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "complete",
	}}}}, nil
}

func TestREPLUsesSessionToolsetForEveryProviderIteration(t *testing.T) {
	client := &pinnedREPLToolClient{}
	toolset := tools.NewToolset([]tools.Tool{{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "repl_only", Parameters: openrouter.Parameter{Type: "object"},
		}},
		Execute: func(context.Context, string) (string, error) { return "repl result", nil },
	}})
	session := agent.NewWithToolset(
		client,
		evieTestContextProfile("test"),
		&recordingREPLHistory{},
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"},
		testTurnOwner{},
		toolset,
	)
	var out bytes.Buffer

	runREPLContextIO(
		context.Background(), session, bufio.NewScanner(strings.NewReader("use it\n")), &out,
	)

	if len(client.requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.requests))
	}
	for i, req := range client.requests {
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "repl_only" {
			t.Fatalf("request %d tools = %+v", i, req.Tools)
		}
	}
	if got := client.requests[1].Messages[len(client.requests[1].Messages)-1]; got.Role != "tool" || got.Content != "repl result" {
		t.Fatalf("second request tool result = %+v", got)
	}
	if got := out.String(); !strings.Contains(got, "[calling repl_only]\n") || !strings.Contains(got, "complete\n") {
		t.Fatalf("REPL output = %q", got)
	}
}

func TestREPLDiscardedContentStaysVisibleAndIsMarked(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Delta("partial answer")
	events.ResponseDiscarded(agent.DiscardLeaseLost, agent.DiscardedResponseMessage)
	got := out.String()
	if !strings.Contains(got, "partial answer") || !strings.Contains(got, agent.DiscardedResponseMessage) ||
		strings.Index(got, "partial answer") > strings.Index(got, agent.DiscardedResponseMessage) {
		t.Fatalf("REPL output = %q", got)
	}
}

func TestREPLReasoningOnlyDiscardUsesExactWarning(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Reasoning("unfinished thought")
	events.ReasoningDone()
	events.ResponseDiscarded(agent.DiscardAssistantPersistenceFailed, agent.DiscardedResponseMessage)
	got := out.String()
	if !strings.Contains(got, "unfinished thought") || !strings.Contains(got, agent.DiscardedResponseMessage) ||
		strings.Count(got, agent.DiscardedResponseMessage) != 1 {
		t.Fatalf("REPL output = %q", got)
	}
}

func TestREPLAssistantDonePrintsCommittedAsyncProviderContentWithoutDelta(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.AssistantDone("complete answer")
	if got := out.String(); got != "complete answer\n" {
		t.Fatalf("REPL output=%q, want committed content once", got)
	}
}

func TestREPLCompactIsExactEventlessCommandAndDoubleSlashIsLiteralText(t *testing.T) {
	history := &recordingREPLHistory{events: []memory.Event{
		{ID: "turn-1", SessionID: "session", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "one", FormatVersion: 1},
		{ID: "turn-1-assistant", SessionID: "session", Sequence: 2, ParentID: "turn-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "answer one", Payload: json.RawMessage(`{}`), FormatVersion: 1},
		{ID: "turn-2", SessionID: "session", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "two", FormatVersion: 1},
		{ID: "turn-2-assistant", SessionID: "session", Sequence: 4, ParentID: "turn-2", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "answer two", Payload: json.RawMessage(`{}`), FormatVersion: 1},
	}}
	client := replContentFirstReasoningClient{}
	session := agent.New(client, evieTestContextProfile("test"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{})
	var out bytes.Buffer
	runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/compact\n//compact\n")), &out)

	if !strings.Contains(out.String(), "Nothing eligible for compaction.\n") {
		t.Fatalf("output=%q", out.String())
	}
	var compactCommands, literalMessages int
	for _, event := range history.events {
		if event.Type == memory.EventUserMessage && event.Content == "/compact" {
			compactCommands++
		}
		if event.Type == memory.EventUserMessage && event.Content == "//compact" {
			literalMessages++
		}
	}
	if compactCommands != 0 || literalMessages != 1 {
		t.Fatalf("compact command events=%d literal messages=%d events=%+v", compactCommands, literalMessages, history.events)
	}
}

func TestREPLCompactRejectsSummaryInstructionsWithoutCreatingAnEvent(t *testing.T) {
	history := &recordingREPLHistory{}
	session := agent.New(replContentFirstReasoningClient{}, evieTestContextProfile("test"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{})
	var out bytes.Buffer
	runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/compact preserve this\n")), &out)
	if got := out.String(); !strings.Contains(got, "Usage: /compact\n") {
		t.Fatalf("output=%q", got)
	}
	if len(history.events) != 0 {
		t.Fatalf("instruction created events: %+v", history.events)
	}
}

func TestREPLAssistantDonePrintsOnlyMissingCommittedAsyncProviderSuffix(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Delta("complete ")
	events.AssistantDone("complete answer")
	if got := out.String(); got != "complete answer\n" {
		t.Fatalf("REPL output=%q, want committed content without duplication", got)
	}
}

func TestREPLAssistantDoneCorrectsDivergentStreamWithAuthoritativeContent(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Delta("speculative answer")
	events.AssistantDone("committed answer")
	want := "speculative answer\n" + replUnsavedStreamCorrection + "\n" +
		replCommittedResponseLabel + "\ncommitted answer\n"
	if got := out.String(); got != want {
		t.Fatalf("REPL output=%q, want correction %q", got, want)
	}
}

func TestREPLAssistantDoneCorrectsStreamWhenCommittedContentIsEmpty(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Delta("speculative answer")
	events.AssistantDone("")
	want := "speculative answer\n" + replUnsavedStreamCorrection + "\n" +
		replCommittedResponseLabel + "\n" + replEmptyCommittedResponse + "\n"
	if got := out.String(); got != want {
		t.Fatalf("REPL output=%q, want correction %q", got, want)
	}
}

type replDiscardThenSucceedClient struct {
	reasoning bool
	calls     int
}

func (c *replDiscardThenSucceedClient) ChatStream(
	_ context.Context,
	_ openrouter.ChatRequest,
	h openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		if c.reasoning {
			h.OnReasoning("unfinished thought")
		} else {
			h.OnContent("partial answer")
		}
		return openrouter.ChatResponse{}, &openrouter.StreamError{
			Kind: openrouter.StreamProviderError,
			Err:  errors.New("transport failed"),
		}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "next answer",
	}}}}, nil
}

func TestSessionToREPLDiscardedStreamCleansStateBeforeNextTurn(t *testing.T) {
	for _, tt := range []struct {
		name      string
		reasoning bool
		prefix    string
	}{
		{name: "partial content", prefix: "partial answer\n"},
		{name: "reasoning only", reasoning: true, prefix: "\x1b[90mthinking…\nunfinished thought\x1b[0m\n\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			events := &replEvents{out: &out}
			client := &replDiscardThenSucceedClient{reasoning: tt.reasoning}
			session := agent.New(client, evieTestContextProfile("test"), &recordingREPLHistory{}, memory.ScopeContext{
				OwnerID: memory.LocalOwnerID, SessionID: "session",
			}, testTurnOwner{})

			if err := session.Send(context.Background(), "first", events, nil); err == nil {
				t.Fatal("first Send succeeded, want provider transport failure")
			}
			if events.deltaIn != nil || events.flush != nil || events.reasoningOpen || events.streamedContent.Len() != 0 {
				t.Fatalf("ResponseDiscarded left pending REPL state: %+v", events)
			}
			if err := session.Send(context.Background(), "second", events, nil); err != nil {
				t.Fatalf("second Send: %v", err)
			}

			want := tt.prefix + agent.DiscardedResponseMessage + "\nnext answer\n"
			if got := out.String(); got != want {
				t.Fatalf("REPL output=%q, want %q", got, want)
			}
			if strings.Contains(out.String(), replUnsavedStreamCorrection) || strings.Count(out.String(), "next answer") != 1 {
				t.Fatalf("subsequent turn was corrected or duplicated: %q", out.String())
			}
		})
	}
}

type replContentFirstReasoningClient struct{}

func (replContentFirstReasoningClient) ChatStream(
	_ context.Context,
	_ openrouter.ChatRequest,
	h openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	h.OnContent("answer")
	h.OnReasoning("late reasoning")
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "answer", Reasoning: "late reasoning",
	}}}}, nil
}

func TestREPLRendersContentFirstProviderAsOneAuthoritativeAssistant(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	history := &recordingREPLHistory{}
	session := agent.New(
		replContentFirstReasoningClient{},
		evieTestContextProfile("test"),
		history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"},
		testTurnOwner{},
	)
	if err := session.Send(context.Background(), "go", events, nil); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "answer\n" {
		t.Fatalf("REPL output=%q, want one authoritative assistant", got)
	}
}

var errREPLLeaseLost = errors.New("repl fixture lease lost")

type committedBoundaryREPLHistory struct {
	events      []memory.Event
	attempts    int
	appendErrAt int
	afterAppend func(memory.EventInput)
}

func (h *committedBoundaryREPLHistory) Append(
	_ context.Context,
	_ memory.TurnLease,
	input memory.EventInput,
) (memory.Event, error) {
	h.attempts++
	if h.appendErrAt == h.attempts {
		return memory.Event{}, errREPLLeaseLost
	}
	event := memory.Event{
		ID: memory.EventID(fmt.Sprintf("event-%d", len(h.events)+1)), SessionID: "session",
		Sequence: int64(len(h.events) + 1), ParentID: input.ParentID, Type: input.Type,
		Role: input.Role, Content: input.Content, Payload: input.Payload, FormatVersion: 1,
	}
	h.events = append(h.events, event)
	if h.afterAppend != nil {
		h.afterAppend(input)
	}
	return event, nil
}
func (h *committedBoundaryREPLHistory) Events(context.Context) ([]memory.Event, error) {
	return append([]memory.Event(nil), h.events...), nil
}

type leaseAwareREPLTurnOwner struct{ testTurnOwner }

func (leaseAwareREPLTurnOwner) IsLeaseLost(err error) bool {
	return errors.Is(err, errREPLLeaseLost)
}

func TestSessionToREPLPresentsCommittedAssistantBeforeTerminalCause(t *testing.T) {
	for _, presentation := range []struct {
		name   string
		deltas []string
		want   string
	}{
		{name: "zero delta", want: "committed\n"},
		{name: "matching prefix", deltas: []string{"committed"}, want: "committed\n"},
		{name: "divergent content", deltas: []string{"speculative"}, want: "speculative\n" +
			replUnsavedStreamCorrection + "\n" + replCommittedResponseLabel + "\ncommitted\n"},
	} {
		for _, cause := range []string{"caller cancellation", "lease loss"} {
			t.Run(presentation.name+"_"+cause, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				history := &committedBoundaryREPLHistory{}
				if cause == "caller cancellation" {
					history.afterAppend = func(input memory.EventInput) {
						if input.Type == memory.EventAssistantMessage {
							cancel()
						}
					}
				} else {
					history.appendErrAt = 4 // tool intent, before ToolCall presentation
				}
				client := &fakeREPLBoundaryClient{deltas: presentation.deltas}
				var out bytes.Buffer
				events := &replEvents{out: &out}
				session := agent.New(client, evieTestContextProfile("test"), history, memory.ScopeContext{
					OwnerID: memory.LocalOwnerID, SessionID: "session",
				}, leaseAwareREPLTurnOwner{})
				if err := session.Send(ctx, "go", events, nil); err == nil {
					t.Fatal("Send succeeded after terminal cause")
				}
				if got := out.String(); got != presentation.want {
					t.Fatalf("REPL output=%q, want %q", got, presentation.want)
				}
				if strings.Contains(out.String(), "[calling") {
					t.Fatalf("tool presentation escaped terminal boundary: %q", out.String())
				}
			})
		}
	}
}

type fakeREPLBoundaryClient struct{ deltas []string }

func (c *fakeREPLBoundaryClient) ChatStream(
	_ context.Context,
	_ openrouter.ChatRequest,
	h openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	for _, delta := range c.deltas {
		h.OnContent(delta)
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "committed", ToolCalls: []openrouter.ToolCall{{
			ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "missing", Arguments: `{}`},
		}},
	}}}}, nil
}

func TestContextCommandIsLocalReadOnlyAndEventless(t *testing.T) {
	client := &replCancellationClient{}
	history := &recordingREPLHistory{}
	session := agent.New(client, evieTestContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "session",
	}, testTurnOwner{})
	var out strings.Builder

	runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/context\n")), &out)

	if client.calls.Load() != 0 {
		t.Fatalf("provider calls=%d", client.calls.Load())
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	if len(history.events) != 0 {
		t.Fatalf("/context created events: %+v", history.events)
	}
	got := out.String()
	for _, want := range []string{"Context\n", "profile: explicit_override", "usable input bytes:", "hypothetical projection:", "headroom bytes:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q missing %q", got, want)
		}
	}
}

func TestContextDiagnosticsPrintLatestSnapshotCounts(t *testing.T) {
	var out strings.Builder
	writeContextDiagnostics(&out, agent.ContextDiagnostics{
		Profile: evieTestContextProfile("test/model").Diagnostics(),
		LatestSnapshot: &agent.DurableContextSnapshotDiagnostics{
			EventID: "snapshot-1", Sequence: 2,
			Manifest: memory.ContextSnapshotPayload{
				MessageCount: 7, ToolSchemaCount: 5,
				Placeholders: []memory.ContextPlaceholderManifest{{
					EventID: "result-1", OriginalBytes: 8192, ProjectedBytes: 1200,
					SHA256: strings.Repeat("a", 64),
				}},
			},
		},
	})
	got := out.String()
	for _, want := range []string{
		"latest snapshot counts: messages=7 tools=5 placeholders=1",
		"latest snapshot placeholder: event=result-1 original_bytes=8192 projected_bytes=1200",
		"latest snapshot placeholder bytes: original=8192 projected=1200 saved=6992",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q omitted %q", got, want)
		}
	}
}
