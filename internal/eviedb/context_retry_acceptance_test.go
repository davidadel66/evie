package eviedb_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type contextRejectingClient struct {
	rejections int
	requests   []openrouter.ChatRequest
}

func (c *contextRejectingClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if len(c.requests) <= c.rejections {
		return openrouter.ChatResponse{}, &openrouter.StreamError{
			Kind: openrouter.StreamProviderError, HTTPStatus: 400, ContextLengthExceeded: true,
			Err: errors.New("api returned status 400"),
		}
	}
	return compactionAcceptanceResponse("answered after compaction"), nil
}

// C8: a provider context-length rejection compacts once and retries against
// real SQLite persistence, and the resulting history stays valid.
func TestProviderContextRejectionCompactsAndRetriesDurably(t *testing.T) {
	ctx := context.Background()
	store := eviedb.NewStore(openAcceptanceDB(t))
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("context-rejection-acceptance")
	history := store.BindHistory(sessionRecord.ID, holder)
	lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		root, err := history.Append(ctx, lease, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser,
			Content: fmt.Sprintf("source-%d:", i+1) + strings.Repeat(string(rune('a'+i)), 60_000),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := history.Append(ctx, lease, memory.EventInput{
			ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "accepted", Payload: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	profile, err := openrouter.NewExplicitContextProfile("retry/model", 262_144, 262_144, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	conversation := &contextRejectingClient{rejections: 1}
	compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(compactionAcceptanceSummary("after provider rejection")),
	}}
	session := agent.NewWithCompactor(conversation, compactor, profile, history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	if err := session.Send(ctx, "continue", nilAgentEvents{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(conversation.requests) != 2 || len(compactor.requests) != 1 {
		t.Fatalf("conversation requests=%d compactor requests=%d", len(conversation.requests), len(compactor.requests))
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	tail := acceptanceEventTypes(events[6:])
	if fmt.Sprint(tail) != fmt.Sprint([]memory.EventType{
		memory.EventUserMessage, memory.EventContextSnapshot, memory.EventContextCompacted,
		memory.EventContextSnapshot, memory.EventAssistantMessage,
	}) {
		t.Fatalf("durable shape=%v", tail)
	}
	if _, err := session.InspectContext(ctx); err != nil {
		t.Fatalf("/context after retry: %v", err)
	}
}

// appendCompletedAcceptanceTurns appends n completed root turns whose user
// messages are size bytes each, under one released lease.
func appendCompletedAcceptanceTurns(t *testing.T, store *eviedb.Store, sessionID memory.SessionID, prefix string, n, size int) {
	t.Helper()
	ctx := context.Background()
	const holder = memory.LeaseHolderID("acceptance-setup")
	history := store.BindHistory(sessionID, holder)
	lease, err := store.AcquireTurnLease(ctx, sessionID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		root, err := history.Append(ctx, lease, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser,
			Content: fmt.Sprintf("%s-%d:", prefix, i+1) + strings.Repeat(string(rune('a'+i%26)), size),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := history.Append(ctx, lease, memory.EventInput{
			ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "accepted", Payload: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ReleaseTurnLease(ctx, sessionID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
}

// C8 final pass: when the iteration already ran automatic compaction, a
// provider context-length rejection ends the turn with durable
// context_overflow evidence. Compacting again appended a second compaction
// for one trigger, which SQLite snapshot correlation rejects, and that
// storage failure left the turn without a terminal.
func TestProviderContextRejectionAfterSameIterationCompactionEndsDurably(t *testing.T) {
	ctx := context.Background()
	store := eviedb.NewStore(openAcceptanceDB(t))
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendCompletedAcceptanceTurns(t, store, sessionRecord.ID, "source", 12, 60_000)
	const holder = memory.LeaseHolderID("same-iteration-rejection")
	history := store.BindHistory(sessionRecord.ID, holder)
	profile, err := openrouter.NewExplicitContextProfile("retry/model", 262_144, 262_144, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	conversation := &contextRejectingClient{rejections: 1}
	compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(compactionAcceptanceSummary("first")),
		compactionAcceptanceResponse(compactionAcceptanceSummary("second")),
	}}
	session := agent.NewWithCompactor(conversation, compactor, profile, history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	if err := session.Send(ctx, "continue", nilAgentEvents{}, nil); !agent.IsContextOverflow(err) {
		t.Fatalf("Send error=%v, want context overflow", err)
	}
	if len(conversation.requests) != 1 || len(compactor.requests) != 1 {
		t.Fatalf("conversation requests=%d compactor requests=%d", len(conversation.requests), len(compactor.requests))
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	tail := events[24:]
	if fmt.Sprint(acceptanceEventTypes(tail)) != fmt.Sprint([]memory.EventType{
		memory.EventUserMessage, memory.EventContextCompacted, memory.EventContextSnapshot, memory.EventTurnFailed,
	}) {
		t.Fatalf("durable shape=%v", acceptanceEventTypes(tail))
	}
	var terminal memory.TurnTerminalPayload
	if err := json.Unmarshal(tail[3].Payload, &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.TurnID != tail[0].ID || terminal.Classification != memory.ClassificationContextOverflow ||
		terminal.Stage != memory.StageProvider || tail[3].ParentID != tail[0].ID {
		t.Fatalf("terminal=%+v parent=%s", terminal, tail[3].ParentID)
	}
	if _, err := session.InspectContext(ctx); err != nil {
		t.Fatalf("/context after rejection: %v", err)
	}
	next := agent.NewWithCompactor(&contextRejectingClient{}, &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
		compactionAcceptanceResponse(compactionAcceptanceSummary("third")),
	}}, profile, history, sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	if err := next.Send(ctx, "next", nilAgentEvents{}, nil); err != nil {
		t.Fatalf("next turn: %v", err)
	}
}

// snapshotFailingHistory fails every context snapshot append as a full disk
// would, so a turn ends locally after its root without terminal evidence.
type snapshotFailingHistory struct{ *eviedb.SessionHistory }

func (h snapshotFailingHistory) Append(ctx context.Context, lease memory.TurnLease, input memory.EventInput) (memory.Event, error) {
	if input.Type == memory.EventContextSnapshot {
		return memory.Event{}, errors.New("disk full")
	}
	return h.SessionHistory.Append(ctx, lease, input)
}

// abandonTurn runs one turn that fails locally after its root user message,
// leaving a root turn that no owner will ever terminate.
func abandonTurn(t *testing.T, store *eviedb.Store, sessionRecord memory.Session, profile openrouter.ContextProfile) {
	t.Helper()
	const holder = memory.LeaseHolderID("abandoning-owner")
	history := snapshotFailingHistory{store.BindHistory(sessionRecord.ID, holder)}
	session := agent.NewWithCompactor(&contextRejectingClient{}, &compactionAcceptanceClient{}, profile, history,
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	if err := session.Send(context.Background(), "abandoned request", nilAgentEvents{}, nil); err == nil {
		t.Fatal("abandoned turn unexpectedly succeeded")
	}
}

// A turn that ended without terminal evidence (a local storage failure, lease
// loss, or a crash) is closed once a later root turn exists. It must not
// block automatic or manual compaction for the rest of the session.
func TestAbandonedTurnDoesNotBlockLaterCompaction(t *testing.T) {
	ctx := context.Background()
	t.Run("automatic", func(t *testing.T) {
		store := eviedb.NewStore(openAcceptanceDB(t))
		sessionRecord, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		profile, err := openrouter.NewExplicitContextProfile("retry/model", 262_144, 262_144, 16_384)
		if err != nil {
			t.Fatal(err)
		}
		abandonTurn(t, store, sessionRecord, profile)
		appendCompletedAcceptanceTurns(t, store, sessionRecord.ID, "later", 14, 60_000)
		const holder = memory.LeaseHolderID("later-owner")
		history := store.BindHistory(sessionRecord.ID, holder)
		compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
			compactionAcceptanceResponse(compactionAcceptanceSummary("past abandoned turn")),
		}}
		conversation := &contextRejectingClient{}
		session := agent.NewWithCompactor(conversation, compactor, profile, history,
			sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
		if err := session.Send(ctx, "go on", nilAgentEvents{}, nil); err != nil {
			t.Fatalf("Send after abandoned turn: %v", err)
		}
		if len(compactor.requests) != 1 || len(conversation.requests) != 1 {
			t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.requests), len(conversation.requests))
		}
		events, err := store.LoadEvents(ctx, sessionRecord.ID)
		if err != nil {
			t.Fatal(err)
		}
		if events[0].Content != "abandoned request" || events[1].Type != memory.EventUserMessage {
			t.Fatalf("setup shape=%v", acceptanceEventTypes(events[:2]))
		}
		var compacted memory.ContextCompactedPayload
		for _, event := range events {
			if event.Type == memory.EventContextCompacted {
				if err := json.Unmarshal(event.Payload, &compacted); err != nil {
					t.Fatal(err)
				}
			}
		}
		if compacted.CoveredFirstEventID != events[0].ID {
			t.Fatalf("automatic compaction covered from %q, want the abandoned root %q", compacted.CoveredFirstEventID, events[0].ID)
		}
		if _, err := session.InspectContext(ctx); err != nil {
			t.Fatalf("/context after compaction: %v", err)
		}
	})
	t.Run("manual", func(t *testing.T) {
		store := eviedb.NewStore(openAcceptanceDB(t))
		sessionRecord, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		profile, err := openrouter.NewExplicitContextProfile("retry/model", 262_144, 262_144, 16_384)
		if err != nil {
			t.Fatal(err)
		}
		appendCompletedAcceptanceTurns(t, store, sessionRecord.ID, "first", 1, 10)
		abandonTurn(t, store, sessionRecord, profile)
		appendCompletedAcceptanceTurns(t, store, sessionRecord.ID, "later", 2, 10)
		const holder = memory.LeaseHolderID("manual-compactor")
		history := store.BindHistory(sessionRecord.ID, holder)
		compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{
			compactionAcceptanceResponse(compactionAcceptanceSummary("through abandoned turn")),
		}}
		session := agent.NewWithCompactor(&contextRejectingClient{}, compactor, profile, history,
			sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
		if _, err := session.Compact(ctx); err != nil {
			t.Fatalf("manual compaction: %v", err)
		}
		events, err := store.LoadEvents(ctx, sessionRecord.ID)
		if err != nil {
			t.Fatal(err)
		}
		abandoned := events[2]
		if abandoned.Type != memory.EventUserMessage || abandoned.Content != "abandoned request" || events[3].Type != memory.EventUserMessage {
			t.Fatalf("setup shape=%v", acceptanceEventTypes(events))
		}
		last := events[len(events)-1]
		var compacted memory.ContextCompactedPayload
		if err := json.Unmarshal(last.Payload, &compacted); err != nil || last.Type != memory.EventContextCompacted {
			t.Fatalf("last event=%+v error=%v", last, err)
		}
		if compacted.CoveredLastEventID != abandoned.ID || compacted.FirstRetainedEventID != events[3].ID {
			t.Fatalf("manual compaction covered through %q retaining %q, want through abandoned %q",
				compacted.CoveredLastEventID, compacted.FirstRetainedEventID, abandoned.ID)
		}
		if _, err := session.InspectContext(ctx); err != nil {
			t.Fatalf("/context after compaction: %v", err)
		}
	})
}

// Covering an abandoned turn never permits a cut inside a turn: the covered
// frontier must still be the final event before the retained root.
func TestCompactionFrontierMustEndItsRootTurn(t *testing.T) {
	ctx := context.Background()
	store := eviedb.NewStore(openAcceptanceDB(t))
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendCompletedAcceptanceTurns(t, store, sessionRecord.ID, "turn", 3, 10)
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("cutting-compactor")
	lease, err := store.AcquireTurnLease(ctx, sessionRecord.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseTurnLease(ctx, sessionRecord.ID, holder, lease.FencingToken)
	summary := compactionAcceptanceSummary("mid-turn cut")
	digest := sha256.Sum256([]byte(summary))
	payload, err := json.Marshal(memory.ContextCompactedPayload{
		SchemaVersion: memory.ContextCompactedSchemaVersion, Generation: 1,
		Trigger:             memory.ContextCompactionManual,
		CoveredFirstEventID: events[0].ID, CoveredFirstSequence: events[0].Sequence,
		CoveredLastEventID: events[2].ID, CoveredLastSequence: events[2].Sequence,
		FirstRetainedEventID: events[4].ID, CanonicalModel: "model", PromptVersion: agent.CompactionPromptVersion,
		SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.BindHistory(sessionRecord.ID, holder).Append(ctx, lease, memory.EventInput{
		Type: memory.EventContextCompacted, Content: summary, Payload: payload,
	})
	if err == nil || !strings.Contains(err.Error(), "does not end its root turn") {
		t.Fatalf("mid-turn compaction append error=%v", err)
	}
}
