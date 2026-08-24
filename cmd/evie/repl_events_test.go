package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
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

func TestREPLFinishSendAfterSuppressedCommittedAssistantDoesNotContaminateNextTurn(t *testing.T) {
	for _, cause := range []string{"caller cancellation", "lease loss"} {
		t.Run(cause, func(t *testing.T) {
			var out bytes.Buffer
			events := &replEvents{out: &out}

			// The assistant content committed with tool calls, then the cause won
			// before AssistantDone admission. Agent ownership tests establish this
			// exact callback suppression for both causes.
			events.Delta("committed tool preface")
			events.finishSend()
			if events.deltaIn != nil || events.flush != nil || events.reasoningOpen || events.streamedContent.Len() != 0 {
				t.Fatalf("pending REPL state survived %s", cause)
			}

			events.Delta("next ")
			events.AssistantDone("next answer")
			events.finishSend()
			want := "committed tool preface\nnext answer\n"
			if got := out.String(); got != want {
				t.Fatalf("REPL output after %s=%q, want %q", cause, got, want)
			}
			if strings.Contains(out.String(), replUnsavedStreamCorrection) {
				t.Fatalf("committed output after %s received a false unsaved label", cause)
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
