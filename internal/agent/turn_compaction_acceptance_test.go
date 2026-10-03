package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// sqliteLongTurnSession binds a session to real SQLite persistence on a
// 128k-token model whose working ceiling is the whole window. With no
// recorded usage the calibrated estimator assumes 3 bytes per token: usable
// input is 331,776 request bytes and the compaction target 235,929.
func sqliteLongTurnSession(t *testing.T, store *eviedb.Store, sessionRecord memory.Session, conversation, compactor Client) *Session {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile("test/model", 131_072, 131_072, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("long-turn")
	session := NewWithCompactor(conversation, compactor, profile, store.BindHistory(sessionRecord.ID, holder),
		sessionRecord.ScopeContext(), store.BindTurnOwner(sessionRecord.ID, holder))
	session.automaticRecallDisabled = true
	return session
}

// resolveLegacyTools pins extra the way a first Send would, so a session
// opened after a restart composes with the same tools.
func resolveLegacyTools(session *Session, extra ...tools.Tool) {
	session.toolset = session.toolset.WithTools(extra)
	session.legacyToolSignatures = legacyToolSignatures(extra)
	session.legacyToolsetPending = false
}

// Stage 8 acceptance: on a 128k model one long agentic turn keeps working
// past the point where projection alone would overflow, against real SQLite
// persistence, and after the database is closed and reopened every durable
// request of the turn is reconstructed exactly and the next turn sees the
// compacted turn as the conversation did.
func TestLongAgenticTurnOn128kKeepsWorkingAndReplaysAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conversation := &agenticClient{calls: 40, argBytes: 12_000}
	compactor := &turnSummaryClient{}
	session := sqliteLongTurnSession(t, store, sessionRecord, conversation, compactor)
	if err := session.Send(ctx, longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatalf("long turn: %v", err)
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	compactions := eventsOfType(events, memory.EventContextTurnCompacted)
	if len(conversation.reqs) != 41 || len(compactions) < 2 || len(compactor.reqs) != len(compactions) {
		t.Fatalf("conversation=%d compactions=%d compactor=%d", len(conversation.reqs), len(compactions), len(compactor.reqs))
	}
	for _, snapshot := range snapshotPayloads(t, events) {
		if snapshot.HardWindowTokens != 131_072 || snapshot.WorkingCeilingTokens != 131_072 {
			t.Fatalf("snapshot profile=%+v", snapshot)
		}

	}
	for i, request := range conversation.reqs {
		checkTurnRequest(t, i+1, request)
	}

	// Projection alone cannot send the final request.
	snapshots := eventsOfType(events, memory.EventContextSnapshot)
	trigger := snapshots[len(snapshots)-1].ParentID
	unprojected := withoutTurnCompactions(events)
	for i, event := range unprojected {
		if event.ID == trigger {
			unprojected = unprojected[:i+1]
			break
		}
	}
	_, err = session.composer.Compose(ContextComposeInput{
		Profile: session.profile, Events: unprojected, ActiveRootID: events[0].ID, TriggerEventID: trigger,
		Iteration: 41, Tools: session.modelToolset().Schemas(),
	})
	if !IsContextOverflow(err) {
		t.Fatalf("projection alone would have fit the final request: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	reopenedEvents, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopenedEvents) != len(events) {
		t.Fatalf("reopened events=%d, want %d", len(reopenedEvents), len(events))
	}
	nextConversation := &agenticClient{calls: 0}
	reopened := sqliteLongTurnSession(t, store, sessionRecord, nextConversation, &turnSummaryClient{})
	resolveLegacyTools(reopened, workTool(3_000))
	if replayed := replayTurnSnapshots(t, reopened, reopenedEvents); replayed != 41 {
		t.Fatalf("replayed %d snapshots after reopen", replayed)
	}
	diagnostics, err := reopened.InspectContext(ctx)
	if err != nil {
		t.Fatalf("/context after reopen: %v", err)
	}
	latest := compactions[len(compactions)-1]
	if diagnostics.LatestSnapshot == nil || diagnostics.LatestSnapshot.Manifest.ActiveTurnCompactionEventID != latest.ID {
		t.Fatalf("latest snapshot does not name the active in-turn compaction: %+v", diagnostics.LatestSnapshot)
	}
	// A summarized result is still re-readable by the event ID the
	// compactor saw.
	covered := turnCompactedPayloadOf(t, compactions[0]).CoveredLastEventID
	reread, err := reopened.readStoredToolResult(ctx, covered)
	if err != nil || reread.ToolName != "work" || reread.Status != "succeeded" ||
		!strings.HasPrefix(reread.Content, "RESULT-") || len(reread.Content) != 3_000 {
		t.Fatalf("covered result %s reread=%+v error=%v", covered, reread.ToolName, err)
	}
	if !strings.Contains(compactor.reqs[0].Messages[1].Content, `"result_event_id":"`+string(covered)+`"`) {
		t.Fatal("the compactor did not see the covered result's event ID")
	}

	// The next turn renders the compacted turn as its last request did: the
	// user request verbatim, the latest summary, then the retained groups.
	if err := reopened.Send(ctx, "What changed?", &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatalf("next turn: %v", err)
	}
	next := nextConversation.reqs[0]
	_, results := checkTurnRequest(t, 1, next)
	summaryBlock := ""
	for i, message := range next.Messages {
		if message.Content == longTurnRequest {
			summaryBlock = next.Messages[i+1].Content
		}
	}
	payload := turnCompactedPayloadOf(t, latest)
	if summaryBlock != turnSummaryMessage(latest.Content) || len(results) == 0 ||
		results[len(results)-1] != "RESULT-040" || results[0] == "RESULT-001" {
		t.Fatalf("next turn does not render the compacted turn: results=%v", results)
	}
	retainedFirst := ""
	for _, event := range reopenedEvents {
		if event.ID == payload.FirstRetainedEventID {
			var assistant memory.AssistantMessagePayload
			if err := json.Unmarshal(event.Payload, &assistant); err != nil {
				t.Fatal(err)
			}
			retainedFirst = assistant.ToolCalls[0].ID
		}
	}
	firstCall := ""
	for _, message := range next.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) != 0 {
			firstCall = message.ToolCalls[0].ID
			break
		}
	}
	if firstCall != retainedFirst {
		t.Fatalf("first rendered group=%s, want the first retained group %s", firstCall, retainedFirst)
	}
}

// C8 against real SQLite: a provider context-length rejection in a long
// turn recovers through an in-turn compaction, and both validators accept
// trigger, rejected snapshot, in-turn compaction, retry snapshot.
func TestInTurnContextRejectionRecoveryIsDurableInSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	sessionRecord, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conversation := &agenticClient{calls: 10, argBytes: 12_000, reject: map[int]bool{8: true}}
	compactor := &turnSummaryClient{}
	session := sqliteLongTurnSession(t, store, sessionRecord, conversation, compactor)
	if err := session.Send(ctx, longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	events, err := store.LoadEvents(ctx, sessionRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	compactions := eventsOfType(events, memory.EventContextTurnCompacted)
	if len(compactions) != 1 || len(conversation.reqs) != 12 {
		t.Fatalf("compactions=%d conversation=%d", len(compactions), len(conversation.reqs))
	}
	var shape []string
	for i, event := range events {
		if event.ID == compactions[0].ID {
			for _, around := range events[i-2 : i+3] {
				shape = append(shape, string(around.Type))
			}
		}
	}
	if strings.Join(shape, ",") != fmt.Sprint("tool_succeeded,context_snapshot,context_turn_compacted,context_snapshot,assistant_message") {
		t.Fatalf("durable recovery shape=%v", shape)
	}
	if _, err := session.InspectContext(ctx); err != nil {
		t.Fatalf("/context: %v", err)
	}
}
