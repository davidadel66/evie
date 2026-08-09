package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
)

func TestSSEHeadersAndFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	ev, err := newSSEEvents(rec)
	if err != nil {
		t.Fatalf("newSSEEvents: %v", err)
	}

	ev.Delta("hi")

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if !rec.Flushed {
		t.Fatal("emit did not flush")
	}
}

func TestEventWireFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	ev, _ := newSSEEvents(rec)

	ev.Delta("Hello ")
	ev.AssistantDone("Hello David")
	ev.ToolCall("c1", "get_time", "{}")
	ev.ToolResult("c1", "3pm", false)
	ev.ApprovalRequest("a1", "edit_file", `{"path":"x"}`, &tools.FileChangePreview{
		Path: "x", OldText: "before", NewText: "after",
	})
	ev.Error("boom")
	ev.TurnDone()

	want := "event: delta\n" +
		`data: {"text":"Hello "}` + "\n\n" +
		"event: assistant_done\n" +
		`data: {"content":"Hello David"}` + "\n\n" +
		"event: tool_call\n" +
		`data: {"id":"c1","name":"get_time","args":"{}"}` + "\n\n" +
		"event: tool_result\n" +
		`data: {"id":"c1","content":"3pm","isError":false}` + "\n\n" +
		"event: approval_request\n" +
		`data: {"id":"a1","name":"edit_file","args":"{\"path\":\"x\"}","preview":{"path":"x","oldText":"before","newText":"after","isNew":false}}` + "\n\n" +
		"event: error\n" +
		`data: {"message":"boom"}` + "\n\n" +
		"event: turn_done\n" +
		"data: {}\n\n"

	if got := rec.Body.String(); got != want {
		t.Fatalf("wire bytes:\n got: %q\nwant: %q", got, want)
	}
}

func TestReasoningWireFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	ev, _ := newSSEEvents(rec)

	ev.Reasoning("Compute ")
	ev.Reasoning("17*23")
	ev.ReasoningDone()

	want := "event: reasoning\n" +
		`data: {"text":"Compute "}` + "\n\n" +
		"event: reasoning\n" +
		`data: {"text":"17*23"}` + "\n\n" +
		"event: reasoning_done\n" +
		"data: {}\n\n"

	if got := rec.Body.String(); got != want {
		t.Fatalf("wire bytes:\n got: %q\nwant: %q", got, want)
	}
}

// The SSE format's one hard rule: a payload must stay a single data:
// line. Newlines inside content must arrive JSON-escaped, never raw.
func TestNewlinesStayOneDataLine(t *testing.T) {
	rec := httptest.NewRecorder()
	ev, _ := newSSEEvents(rec)

	ev.Delta("line one\nline two")

	body := rec.Body.String()
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n") {
		if line == "" {
			t.Fatalf("raw newline split the event into multiple blocks: %q", body)
		}
	}
	if !strings.Contains(body, `\n`) {
		t.Fatalf("newline not JSON-escaped: %q", body)
	}
}
