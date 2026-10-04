package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// flushRequest is one compaction frontier offered to the background pass.
type flushRequest struct {
	sessionID memory.SessionID
	through   int64
}

// recordingFlush records every flush request; onRequest observes the
// session's state at the moment of the request.
type recordingFlush struct {
	requests  []flushRequest
	onRequest func(flushRequest)
}

func (f *recordingFlush) RequestCompactionFlush(sessionID memory.SessionID, through int64) {
	request := flushRequest{sessionID: sessionID, through: through}
	if f.onRequest != nil {
		f.onRequest(request)
	}
	f.requests = append(f.requests, request)
}

func compactedPayloadOf(t *testing.T, event memory.Event) memory.ContextCompactedPayload {
	t.Helper()
	var payload memory.ContextCompactedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// Stage 10: a committed whole-turn automatic compaction offers the owner
// messages it summarized (everything through its last covered event) to the
// background memory pass, once the summary is durable, and the turn goes on
// without waiting for the pass.
func TestAutomaticCompactionOffersItsSummarizedRangeToTheMemoryFlush(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(180_000)}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	flush := &recordingFlush{}
	flush.onRequest = func(flushRequest) {
		if eventsOfType(history.allEvents(), memory.EventContextCompacted) == nil {
			t.Error("the flush was requested before the compaction was durable")
		}
	}
	session := withByteBudgets(NewWithCompactorAndToolset(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner(), tools.BuiltinToolset(),
		WithCompactionMemoryFlush(flush)))

	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	compactions := eventsOfType(history.allEvents(), memory.EventContextCompacted)
	if len(compactions) != 1 || len(conversation.reqs) != 1 {
		t.Fatalf("compactions=%d conversation requests=%d", len(compactions), len(conversation.reqs))
	}
	payload := compactedPayloadOf(t, compactions[0])
	if len(flush.requests) != 1 || flush.requests[0] != (flushRequest{sessionID: "test-session", through: payload.CoveredLastSequence}) {
		t.Fatalf("flush requests=%+v, want the compaction's covered frontier %d", flush.requests, payload.CoveredLastSequence)
	}
}

// A failed compaction summarized nothing, so it offers nothing.
func TestFailedAutomaticCompactionRequestsNoMemoryFlush(t *testing.T) {
	history := &fakeHistory{events: automaticPressureHistory(190_000)}
	compactor := &fakeClient{steps: []step{{err: errors.New("summary unavailable")}}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	flush := &recordingFlush{}
	session := withByteBudgets(NewWithCompactorAndToolset(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner(), tools.BuiltinToolset(),
		WithCompactionMemoryFlush(flush)))
	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(flush.requests) != 0 {
		t.Fatalf("compactor requests=%d flush requests=%+v", len(compactor.reqs), flush.requests)
	}
}

// An in-turn compaction offers its frontier too: the owner message of the
// active turn (its root) precedes every covered tool group, so it is
// covered. Each generation offers its own frontier.
func TestInTurnCompactionOffersTheActiveTurnsOwnerMessageToTheMemoryFlush(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	compactor := &turnSummaryClient{}
	flush := &recordingFlush{}
	session := longTurnSession(t, history, conversation, compactor, WithCompactionMemoryFlush(flush))
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	events := history.allEvents()
	compactions := eventsOfType(events, memory.EventContextTurnCompacted)
	if len(compactions) < 2 || len(flush.requests) != len(compactions) {
		t.Fatalf("compactions=%d flush requests=%+v", len(compactions), flush.requests)
	}
	root := eventsOfType(events, memory.EventUserMessage)
	if len(root) != 1 || root[0].Content != longTurnRequest {
		t.Fatalf("owner messages=%+v", root)
	}
	for i, compaction := range compactions {
		payload := turnCompactedPayloadOf(t, compaction)
		if flush.requests[i] != (flushRequest{sessionID: "test-session", through: payload.CoveredLastSequence}) ||
			flush.requests[i].through <= root[0].Sequence {
			t.Fatalf("generation %d flush=%+v, want the covered frontier %d past the owner message %d",
				i+1, flush.requests[i], payload.CoveredLastSequence, root[0].Sequence)
		}
	}
}

// releaseRecordingOwner reports whether the manual compaction's lease was
// released when the flush was requested.
type releaseRecordingOwner struct {
	*fakeTurnOwner
	released bool
}

func (o *releaseRecordingOwner) Release(ctx context.Context, lease memory.TurnLease) error {
	o.released = true
	return o.fakeTurnOwner.Release(ctx, lease)
}

// The manual /compact path (REPL and web) offers its frontier once its short
// lease is released, so the pass finds the conversation free; a compaction
// that summarized nothing or failed offers nothing.
func TestManualCompactionOffersItsSummarizedRangeAfterReleasingTheConversation(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events}
	compactor := &fakeClient{steps: []step{
		{err: errors.New("summary unavailable")},
		{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: validCompactionSummary()}}}}},
	}}
	owner := &releaseRecordingOwner{fakeTurnOwner: newFakeTurnOwner()}
	flush := &recordingFlush{}
	flush.onRequest = func(flushRequest) {
		if !owner.released {
			t.Error("the flush was requested while the compaction still held the conversation")
		}
	}
	session := NewWithCompactorAndToolset(compactor, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, owner, tools.BuiltinToolset(),
		WithCompactionMemoryFlush(flush))

	if _, err := session.Compact(context.Background()); err == nil || len(flush.requests) != 0 {
		t.Fatalf("failed compaction error=%v flush requests=%+v", err, flush.requests)
	}
	owner.released = false
	result, err := session.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var compacted memory.Event
	for _, event := range history.allEvents() {
		if event.ID == result.CompactionEventID {
			compacted = event
		}
	}
	payload := compactedPayloadOf(t, compacted)
	if payload.CoveredLastSequence != 2 || len(flush.requests) != 1 ||
		flush.requests[0] != (flushRequest{sessionID: "test-session", through: 2}) {
		t.Fatalf("payload=%+v flush requests=%+v", payload, flush.requests)
	}
	if _, err := session.Compact(context.Background()); !errors.Is(err, ErrNothingEligibleForCompaction) || len(flush.requests) != 1 {
		t.Fatalf("nothing eligible error=%v flush requests=%+v", err, flush.requests)
	}
}

// Delegated child conversations never flush: their "user" messages are the
// orchestrator's task text, not the owner's words.
func TestDelegatedCompactionNeverRequestsAMemoryFlush(t *testing.T) {
	events := append(completedCompactionTurn("turn-1", 1, "one", "answer one"), completedCompactionTurn("turn-2", 3, "two", "answer two")...)
	events = append(events, completedCompactionTurn("turn-3", 5, "three", "answer three")...)
	history := &fakeHistory{events: events}
	client := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	flush := &recordingFlush{}
	session := NewDelegatedWithToolset(client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session", ParentSessionID: "parent-session"},
		newFakeTurnOwner(), tools.BuiltinToolset(), "Investigate.", WithCompactionMemoryFlush(flush))
	if _, err := session.Compact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if eventsOfType(history.allEvents(), memory.EventContextCompacted) == nil || len(flush.requests) != 0 {
		t.Fatalf("delegated compaction flush requests=%+v", flush.requests)
	}
}
