package eviedb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// turnCompactionFixture appends durable-shaped events for one session.
type turnCompactionFixture struct {
	t       *testing.T
	store   *Store
	session memory.Session
	root    memory.Event
	trigger memory.Event
}

func newTurnCompactionFixture(t *testing.T) *turnCompactionFixture {
	t.Helper()
	store := NewStore(newTestDB(t))
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return &turnCompactionFixture{t: t, store: store, session: session}
}

func (f *turnCompactionFixture) append(input memory.EventInput) (memory.Event, error) {
	return f.store.appendEventForTest(context.Background(), f.session.ID, input)
}

func (f *turnCompactionFixture) mustAppend(input memory.EventInput) memory.Event {
	f.t.Helper()
	event, err := f.append(input)
	if err != nil {
		f.t.Fatalf("append %s: %v", input.Type, err)
	}
	return event
}

func (f *turnCompactionFixture) rootTurn(content string) memory.Event {
	f.root = f.mustAppend(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
	f.trigger = f.root
	return f.root
}

// group appends one assistant tool-call event with calls calls, each with its
// intent and result, and returns the assistant and its results.
func (f *turnCompactionFixture) group(label string, calls int) (memory.Event, []memory.Event) {
	f.t.Helper()
	var toolCalls []memory.ToolCall
	for i := 0; i < calls; i++ {
		toolCalls = append(toolCalls, memory.ToolCall{ID: fmt.Sprintf("%s-call-%d", label, i), Name: "work", Arguments: `{}`})
	}
	payload, err := json.Marshal(memory.AssistantMessagePayload{ToolCalls: toolCalls})
	if err != nil {
		f.t.Fatal(err)
	}
	assistant := f.mustAppend(memory.EventInput{ParentID: f.trigger.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: payload})
	var results []memory.Event
	for i, call := range toolCalls {
		execution := memory.ExecutionID(fmt.Sprintf("%s-%s-execution-%d", f.session.ID, label, i))
		intentPayload, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
		intent := f.mustAppend(memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: execution, Payload: intentPayload})
		resultPayload, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: call.ID})
		results = append(results, f.mustAppend(memory.EventInput{ParentID: intent.ID, Type: memory.EventToolSucceeded,
			Role: memory.RoleTool, ExecutionID: execution, Content: label + " result", Payload: resultPayload}))
	}
	f.trigger = results[len(results)-1]
	return assistant, results
}

func turnCompactionInput(t *testing.T, parent memory.EventID, turn memory.EventID, generation int64, prior memory.EventID, first, last, retained memory.Event) memory.EventInput {
	t.Helper()
	summary := validContextCompactionSummary()
	digest := sha256.Sum256([]byte(summary))
	payload, err := json.Marshal(memory.ContextTurnCompactedPayload{
		SchemaVersion: memory.ContextTurnCompactedSchemaVersion, TurnID: turn, Generation: generation,
		PriorCompactionEventID: prior,
		CoveredFirstEventID:    first.ID, CoveredFirstSequence: first.Sequence,
		CoveredLastEventID: last.ID, CoveredLastSequence: last.Sequence,
		FirstRetainedEventID: retained.ID, FirstRetainedSequence: retained.Sequence,
		CanonicalModel: "test/model", PromptVersion: memory.ContextTurnCompactionPromptVersion,
		SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	})
	if err != nil {
		t.Fatal(err)
	}
	return memory.EventInput{ParentID: parent, Type: memory.EventContextTurnCompacted, Content: summary, Payload: payload}
}

func (f *turnCompactionFixture) snapshotInput(activeTurnCompaction memory.EventID) memory.EventInput {
	f.t.Helper()
	snapshot := validContextSnapshotPayload(f.root, f.trigger)
	snapshot.ActiveTurnCompactionEventID = activeTurnCompaction
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		f.t.Fatal(err)
	}
	return memory.EventInput{ParentID: f.trigger.ID, Type: memory.EventContextSnapshot, Payload: encoded}
}

// fourGroupTurn builds a root turn with four two-call groups.
func fourGroupTurn(t *testing.T) (*turnCompactionFixture, []memory.Event, [][]memory.Event) {
	t.Helper()
	f := newTurnCompactionFixture(t)
	f.rootTurn("earlier")
	f.mustAppend(memory.EventInput{ParentID: f.root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "answer", Payload: json.RawMessage(`{}`)})
	f.rootTurn("do the work")
	f.mustAppend(f.snapshotInput(""))
	var assistants []memory.Event
	var results [][]memory.Event
	for i := 0; i < 4; i++ {
		assistant, groupResults := f.group(fmt.Sprintf("g%d", i+1), 2)
		assistants = append(assistants, assistant)
		results = append(results, groupResults)
		// The newest trigger has no snapshot yet: automatic compaction runs
		// before the request is composed.
		if i < 3 {
			f.mustAppend(f.snapshotInput(""))
		}
	}
	return f, assistants, results
}

// Stage 8: SQLite admits an in-turn compaction only of the active turn, as a
// contiguous prefix of whole tool groups that keeps the group of the trigger
// it immediately follows, and only as an exact advance of the turn's chain.
func TestContextTurnCompactedAppendValidatesActiveTurnFrontier(t *testing.T) {
	f, a, r := fourGroupTurn(t)
	for _, test := range []struct {
		name  string
		input memory.EventInput
	}{
		{name: "splits a call from its group", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[1][0], a[2])},
		{name: "retained frontier skips a group", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[1][1], a[3])},
		{name: "first generation skips the first group", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[1], r[1][1], a[2])},
		{name: "does not follow its trigger", input: turnCompactionInput(t, r[2][1].ID, f.root.ID, 1, "", a[0], r[1][1], a[2])},
		{name: "parent is not a tool result", input: turnCompactionInput(t, a[3].ID, f.root.ID, 1, "", a[0], r[1][1], a[2])},
		{name: "advances a missing prior", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 2, "missing", a[2], r[2][1], a[3])},
		{name: "covers the triggering group", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[3][1],
			memory.Event{ID: "future-assistant", Sequence: r[3][1].Sequence + 2})},
		{name: "retained group precedes the covered frontier", input: turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[3][0], a[3])},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.append(test.input); err == nil {
				t.Fatal("invalid in-turn compaction accepted")
			}
		})
	}
	var earlier memory.Event
	for _, event := range mustLoadEvents(t, f) {
		if event.Content == "earlier" {
			earlier = event
		}
	}
	if _, err := f.append(turnCompactionInput(t, f.trigger.ID, earlier.ID, 1, "", a[0], r[1][1], a[2])); err == nil {
		t.Fatal("in-turn compaction of an earlier turn accepted")
	}

	compacted := f.mustAppend(turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[1][1], a[2]))
	if compacted.ParentID != f.trigger.ID || compacted.Role != "" || compacted.ExecutionID != "" {
		t.Fatalf("compaction envelope=%+v", compacted)
	}
	if _, err := f.append(f.snapshotInput("")); err == nil {
		t.Fatal("snapshot after an in-turn compaction accepted without naming it")
	}
	f.mustAppend(f.snapshotInput(compacted.ID))
	if _, err := f.append(turnCompactionInput(t, f.trigger.ID, f.root.ID, 2, compacted.ID, a[2], r[2][1], a[3])); err == nil {
		t.Fatal("second compaction for one trigger accepted after its request snapshot")
	}

	// The next generation advances the chain exactly.
	f.group("g5", 1)
	if _, err := f.append(turnCompactionInput(t, f.trigger.ID, f.root.ID, 2, compacted.ID, a[0], r[2][1], a[3])); err == nil {
		t.Fatal("generation two that does not start at the retained frontier accepted")
	}
	if _, err := f.append(f.snapshotInput("")); err == nil {
		t.Fatal("later snapshot accepted without the turn's active in-turn compaction")
	}
	second := f.mustAppend(turnCompactionInput(t, f.trigger.ID, f.root.ID, 2, compacted.ID, a[2], r[2][1], a[3]))
	if _, err := f.append(f.snapshotInput(compacted.ID)); err == nil {
		t.Fatal("snapshot naming a superseded in-turn compaction accepted")
	}
	f.mustAppend(f.snapshotInput(second.ID))

	// A new turn starts without an active in-turn compaction.
	f.mustAppend(memory.EventInput{ParentID: f.trigger.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "done", Payload: json.RawMessage(`{}`)})
	f.rootTurn("next")
	if _, err := f.append(f.snapshotInput(second.ID)); err == nil {
		t.Fatal("next turn's snapshot accepted an earlier turn's in-turn compaction")
	}
	f.mustAppend(f.snapshotInput(""))

	// In-turn summaries are not session episodes.
	var episodes int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM memory_session_episodes WHERE session_id=?`, f.session.ID).Scan(&episodes); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 {
		t.Fatalf("in-turn compaction created %d session episodes", episodes)
	}
}

// C8: trigger, rejected request snapshot, in-turn compaction, retry snapshot
// is the one recovery shape; no further snapshot follows for the trigger.
func TestContextTurnCompactedAcceptsCompactAndRetryShape(t *testing.T) {
	f, a, r := fourGroupTurn(t)
	f.mustAppend(f.snapshotInput(""))
	compacted := f.mustAppend(turnCompactionInput(t, f.trigger.ID, f.root.ID, 1, "", a[0], r[1][1], a[2]))
	f.mustAppend(f.snapshotInput(compacted.ID))
	if _, err := f.append(f.snapshotInput(compacted.ID)); err == nil {
		t.Fatal("a second retry snapshot for the same trigger was accepted")
	}
}

func mustLoadEvents(t *testing.T, f *turnCompactionFixture) []memory.Event {
	t.Helper()
	events, err := f.store.LoadEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	return events
}
