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
