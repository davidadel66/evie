package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestActivityReplayPreservesPhasesTurnAndTerminalTime(t *testing.T) {
	start := time.Unix(100, 0)
	events := []memory.Event{
		{ID: "u", Type: memory.EventUserMessage, Content: "inspect", RecordedAt: start},
		{ID: "a", ParentID: "u", Type: memory.EventAssistantMessage, Content: "Checking.", Payload: []byte(`{"tool_calls":[{"id":"c","name":"read_file","arguments":"{}"}]}`)},
		{ID: "t", ParentID: "a", Type: memory.EventToolIntent, ExecutionID: "x", Payload: []byte(`{"call":{"id":"c","name":"read_file","arguments":"{}"}}`)},
		{ID: "r", ParentID: "t", Type: memory.EventToolSucceeded, ExecutionID: "x", Content: "ok"},
		{ID: "f", ParentID: "r", Type: memory.EventAssistantMessage, Content: "Checked.Done.", RecordedAt: start.Add(4 * time.Second), Payload: []byte(`{"text_parts":[{"text":"Checked.","phase":"commentary"},{"text":"Done.","phase":"final_answer"}]}`)},
		{ID: "u2", Type: memory.EventUserMessage, Content: "unfinished", RecordedAt: start.Add(time.Minute)},
	}
	items, err := projectHistory(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 6 {
		t.Fatalf("items = %+v", items)
	}
	for _, item := range items[:5] {
		if item.Turn == nil || item.Turn.ID != "u" || item.Turn.StartedAt != start.UnixMilli() || item.Turn.FinishedAt != start.Add(4*time.Second).UnixMilli() || item.Turn.Status != "complete" {
			t.Fatalf("turn metadata = %+v", item)
		}
	}
	if items[1].Phase != "commentary" || items[3].Phase != "commentary" || items[4].Phase != "final_answer" {
		t.Fatalf("parts = %+v", items)
	}
	if items[5].Turn.Status != "incomplete" || items[5].Turn.FinishedAt != 0 {
		t.Fatalf("unfinished turn = %+v", items[5])
	}
	if items[3].Key == items[4].Key {
		t.Fatal("parts need stable distinct keys")
	}
}

func TestActivitySSEUsesSamePublicPartsAsReplay(t *testing.T) {
	rr := httptest.NewRecorder()
	ev, err := newSSEEvents(rr)
	if err != nil {
		t.Fatal(err)
	}
	ev.TurnStarted("u", time.Unix(100, 0))
	event := memory.Event{ID: "a", Type: memory.EventAssistantMessage, Content: "Progress.Final.", RecordedAt: time.Unix(104, 0), Payload: []byte(`{"text_parts":[{"text":"Progress.","phase":"commentary"},{"text":"Final.","phase":"final_answer"}]}`)}
	ev.AssistantCommitted(event)
	blocks := strings.Split(strings.TrimSpace(rr.Body.String()), "\n\n")
	if len(blocks) != 2 || !strings.HasPrefix(blocks[0], "event: turn_started\n") {
		t.Fatal(rr.Body.String())
	}
	var payload struct {
		Parts      []publicAssistantPart
		Terminal   bool
		FinishedAt int64
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(blocks[1], "event: assistant_done\ndata: ")), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Terminal || payload.FinishedAt != 104000 || len(payload.Parts) != 2 || payload.Parts[0].Phase != "commentary" || payload.Parts[1].Phase != "final_answer" {
		t.Fatalf("payload=%+v", payload)
	}
}
