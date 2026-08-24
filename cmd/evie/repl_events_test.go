package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

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

func TestREPLFinishSendAfterIncompleteProviderStreamDoesNotContaminateNextTurn(t *testing.T) {
	for _, failure := range []string{"transport failure", "provider stream failure"} {
		t.Run(failure, func(t *testing.T) {
			var out bytes.Buffer
			events := &replEvents{out: &out}

			// The provider streamed partial speculative content, then failed before
			// any assistant response committed or its normal callbacks finalized.
			events.Delta("partial provider response")
			events.finishSend()
			if events.deltaIn != nil || events.flush != nil || events.reasoningOpen || events.streamedContent.Len() != 0 {
				t.Fatalf("pending REPL state survived %s", failure)
			}

			events.Delta("next ")
			events.AssistantDone("next answer")
			events.finishSend()
			want := "partial provider response\nnext answer\n"
			if got := out.String(); got != want {
				t.Fatalf("REPL output after %s=%q, want %q", failure, got, want)
			}
			if strings.Contains(out.String(), replUnsavedStreamCorrection) {
				t.Fatalf("finishSend invented an authoritative-content correction after %s", failure)
			}
		})
	}
}

func TestREPLFinishSendClosesOpenReasoningOnceBeforeNextTurn(t *testing.T) {
	var out bytes.Buffer
	events := &replEvents{out: &out}
	events.Reasoning("unfinished reasoning")
	events.finishSend()
	events.finishSend() // completed printers are not closed twice
	events.Delta("next ")
	events.AssistantDone("next answer")
	events.finishSend()

	want := "\x1b[90mthinking…\nunfinished reasoning\x1b[0m\nnext answer\n"
	if got := out.String(); got != want {
		t.Fatalf("REPL output=%q, want safely reset reasoning then next turn %q", got, want)
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
		"test",
		history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"},
		testTurnOwner{},
	)
	if err := session.Send(context.Background(), "go", events, nil); err != nil {
		t.Fatal(err)
	}
	events.finishSend()
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
					history.appendErrAt = 3 // tool intent, before ToolCall presentation
				}
				client := &fakeREPLBoundaryClient{deltas: presentation.deltas}
				var out bytes.Buffer
				events := &replEvents{out: &out}
				session := agent.New(client, "test", history, memory.ScopeContext{
					OwnerID: memory.LocalOwnerID, SessionID: "session",
				}, leaseAwareREPLTurnOwner{})
				if err := session.Send(ctx, "go", events, nil); err == nil {
					t.Fatal("Send succeeded after terminal cause")
				}
				events.finishSend()
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
