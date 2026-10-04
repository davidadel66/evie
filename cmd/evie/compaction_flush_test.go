package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// replSummaryClient answers the compactor with a valid summary.
type replSummaryClient struct{}

func (replSummaryClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: summary.String()}}}}, nil
}

type replFlush struct{ requests []string }

func (f *replFlush) RequestCompactionFlush(sessionID memory.SessionID, through int64) {
	f.requests = append(f.requests, fmt.Sprintf("%s@%d", sessionID, through))
}

// Stage 10: the REPL's /compact offers the owner messages it summarized to
// the background memory pass (the web /api/compact runs the same
// Session.Compact).
func TestREPLCompactOffersTheSummarizedOwnerMessagesToTheMemoryFlush(t *testing.T) {
	var events []memory.Event
	for i, text := range []string{"one", "two", "three"} {
		root := memory.EventID(fmt.Sprintf("turn-%d", i+1))
		events = append(events,
			memory.Event{ID: root, SessionID: "session", Sequence: int64(2*i + 1), Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text, FormatVersion: 1},
			memory.Event{ID: root + "-assistant", SessionID: "session", Sequence: int64(2*i + 2), ParentID: root, Type: memory.EventAssistantMessage,
				Role: memory.RoleAssistant, Content: "answer " + text, Payload: json.RawMessage(`{}`), FormatVersion: 1})
	}
	history := &recordingREPLHistory{events: events}
	flush := &replFlush{}
	session := agent.NewWithToolset(replSummaryClient{}, evieTestContextProfile("test"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{}, tools.BuiltinToolset(),
		agent.WithCompactionMemoryFlush(flush))
	var out bytes.Buffer
	runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/compact\n")), &out)
	if !strings.Contains(out.String(), "Context compacted.\n") || strings.Join(flush.requests, ",") != "session@2" {
		t.Fatalf("output=%q flush requests=%v, want the first turn's frontier", out.String(), flush.requests)
	}
}
