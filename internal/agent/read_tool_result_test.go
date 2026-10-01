package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

func TestModelRereadsAProjectedToolResultByEventIDWithoutRerunningTheTool(t *testing.T) {
	original := "HEAD " + strings.Repeat("stored output ", 900) + " TAIL"
	b := &contextEvents{t: t}
	b.user("earlier", "fetch the page")
	resultID := b.toolGroup("earlier", original)
	b.answer("earlier", "earlier-answer", "fetched")
	history := &fakeHistory{events: b.events}
	reran := false
	client := &fakeClient{steps: []step{
		assistantStep("", nil,
			toolCall("read-1", "read_tool_result", `{"event_id":"`+string(resultID)+`","offset":0,"limit":64}`),
			toolCall("read-2", "read_tool_result", `{"event_id":"earlier"}`),
			toolCall("read-3", "read_tool_result", `{"event_id":"missing"}`),
		),
		assistantStep("done", nil),
	}}
	session := NewWithToolset(client, testContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner(), tools.NewToolset([]tools.Tool{tools.ReadToolResultTool(), echoTool("lookup", false, &reran)}))
	if err := session.Send(context.Background(), "show me the start of that page again", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if reran {
		t.Fatal("rereading ran the original tool")
	}
	outcomes := map[string]memory.Event{}
	for _, event := range history.allEvents() {
		if event.Type != memory.EventToolSucceeded && event.Type != memory.EventToolFailed {
			continue
		}
		var payload memory.ToolResultPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		outcomes[payload.ToolCallID] = event
	}
	read := outcomes["read-1"]
	if read.Type != memory.EventToolSucceeded || !strings.Contains(read.Content, "event_id="+string(resultID)+" tool=lookup status=succeeded") ||
		!strings.Contains(read.Content, "\nHEAD stored output") || !strings.Contains(read.Content, "returned_bytes=64 ") {
		t.Fatalf("reread outcome = %+v", read)
	}
	if outcomes["read-2"].Type != memory.EventToolFailed || !strings.Contains(outcomes["read-2"].Content, "not a tool result") {
		t.Fatalf("non-tool event outcome = %+v", outcomes["read-2"])
	}
	if outcomes["read-3"].Type != memory.EventToolFailed || !strings.Contains(outcomes["read-3"].Content, "no tool result") {
		t.Fatalf("unknown event outcome = %+v", outcomes["read-3"])
	}
}

func TestStoredToolResultReaderStaysInsideTheSession(t *testing.T) {
	b := &contextEvents{t: t}
	b.user("earlier", "run it")
	resultID := b.toolGroup("earlier", "own output")
	b.add(memory.Event{ID: "foreign-result", SessionID: "other-session", ParentID: "assistant-1",
		Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: "another session's output",
		Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: "call-1"})})
	session := NewWithToolset(nil, testContextProfile("test/model"), &fakeHistory{events: b.events}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner(), tools.NewToolset(nil))
	own, err := session.readStoredToolResult(context.Background(), resultID)
	if err != nil || own.Content != "own output" || own.ToolName != "lookup" || own.Status != "succeeded" {
		t.Fatalf("own result = %+v, %v", own, err)
	}
	if foreign, err := session.readStoredToolResult(context.Background(), "foreign-result"); err == nil {
		t.Fatalf("read another session's result: %+v", foreign)
	}
}
