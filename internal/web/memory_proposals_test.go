package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func proposalPost(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/memory/proposals/"+path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// queueWebProposal queues one preference Evie noticed, the way the remember
// tool does when the owner did not ask, and ends the proposing turn.
func queueWebProposal(t *testing.T, store *eviedb.Store, session memory.Session, holder, message, value string) {
	t.Helper()
	ctx := context.Background()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(holder), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	said, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: message})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-" + holder[len(holder)-12:], SourceEventID: said.ID,
		Predicate: "answer_style", PredicateLabel: "answer style", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindPreference,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: said.ID, Type: memory.EventToolIntent,
		ExecutionID: "91000000-0000-4000-8000-" + memory.ExecutionID(holder[len(holder)-12:]), Payload: json.RawMessage(`{"call":{"id":"call","name":"memory_remember_literal","arguments":"{}"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName, IntentEventID: intent.ID,
		Reason: memory.MemoryProposalNotRequested, Literal: &proposal}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryProposalInboxRoutesApproveAndDecline(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queueWebProposal(t, store, session, "inbox-000000000001", "Short answers suit me best.", "short answers")
	queueWebProposal(t, store, session, "inbox-000000000002", "And skip the pleasantries.", "no pleasantries")
	handler := web.WithCandidateReview(web.NewServer(nil), store).Handler()

	if count := reviewResponse[struct{ Pending int }](t, proposalPost(t, handler, "count", map[string]any{}), 200); count.Pending != 2 {
		t.Fatalf("pending = %d", count.Pending)
	}
	page := reviewResponse[memory.MemoryProposalPage](t, proposalPost(t, handler, "list", map[string]any{}), 200)
	if len(page.Items) != 2 || page.Pending != 2 || !page.Items[0].Approvable || !strings.Contains(page.Items[0].Proposal, `"classification":{"kind":"preference","standing":true`) {
		t.Fatalf("inbox page = %+v", page)
	}
	if w := proposalPost(t, handler, "approve", map[string]any{"proposal_id": page.Items[0].ProposalID}); w.Code != http.StatusBadRequest {
		t.Fatalf("approval without the exact hash = %d %s", w.Code, w.Body.String())
	}
	approved := reviewResponse[memory.MemoryProposalResult](t, proposalPost(t, handler, "approve", memory.MemoryProposalApproval{
		ProposalID: page.Items[0].ProposalID, PreparedSHA256: page.Items[0].PreparedSHA256, AlwaysApplies: false}), 200)
	if approved.Standing || approved.ClaimID == "" {
		t.Fatalf("approved = %+v", approved)
	}
	if w := proposalPost(t, handler, "approve", memory.MemoryProposalApproval{ProposalID: page.Items[0].ProposalID, PreparedSHA256: page.Items[0].PreparedSHA256}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "proposal_resolved") {
		t.Fatalf("second approval = %d %s", w.Code, w.Body.String())
	}
	// The first approval moved Global's revision: the second proposal as
	// first listed is refused, and the refreshed listing approves.
	if w := proposalPost(t, handler, "approve", memory.MemoryProposalApproval{ProposalID: page.Items[1].ProposalID, PreparedSHA256: page.Items[1].PreparedSHA256, AlwaysApplies: true}); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), "proposal_changed") {
		t.Fatalf("stale approval = %d %s", w.Code, w.Body.String())
	}
	refreshed := reviewResponse[memory.MemoryProposalPage](t, proposalPost(t, handler, "list", map[string]any{}), 200)
	if len(refreshed.Items) != 1 || refreshed.Items[0].Revision != 2 {
		t.Fatalf("refreshed page = %+v", refreshed)
	}
	if w := proposalPost(t, handler, "decline", map[string]any{"proposal_id": refreshed.Items[0].ProposalID}); w.Code != http.StatusOK {
		t.Fatalf("decline = %d %s", w.Code, w.Body.String())
	}
	if count := reviewResponse[struct{ Pending int }](t, proposalPost(t, handler, "count", map[string]any{}), 200); count.Pending != 0 {
		t.Fatalf("pending after review = %d", count.Pending)
	}
	if w := proposalPost(t, handler, "decline", map[string]any{"proposal_id": "missing"}); w.Code != http.StatusNotFound {
		t.Fatalf("decline of a missing proposal = %d %s", w.Code, w.Body.String())
	}
}

// A background proposal from a Workspace conversation lists its Global
// version, and "Save everywhere instead" approves it with that version's
// hash (memory.decisions.md, 2026-10-12).
func TestMemoryProposalInboxApprovesTheGlobalVersion(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Finance")
	if err != nil {
		t.Fatal(err)
	}
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	receipt := composition.Receipt{FormatVersion: composition.FormatVersion, Preset: composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion: "1.0.0", Providers: []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities: []composition.Capability{{ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash}},
		ToolSchemas:  []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}}, Instructions: []composition.InstructionReference{{ID: "fixture-instructions", SHA256: hash}}}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AcquireTurnLease(ctx, session.ID, "turn", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	said, err := store.AppendEventWithLease(ctx, session.ID, turn.HolderID, turn.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer short answers."})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, turn.HolderID, turn.FencingToken); err != nil {
		t.Fatal(err)
	}
	request := memory.RememberLiteralRequest{Destination: memory.MemoryWorkspace, IdempotencyKey: "idem:v1:92000000-0000-4000-8000-000000000001",
		SourceEventID: said.ID, Predicate: "answer_style", PredicateLabel: "answer style", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "short answers"}, Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindPreference}
	own, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Destination = memory.MemoryEverywhere
	everywhere, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	background, err := store.AcquireTurnLease(ctx, session.ID, "memory-background:web", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := store.CommitMemoryBackgroundPass(ctx, background, memory.BackgroundCommit{
		Proposals: []memory.QueuedMemoryProposal{{ToolName: memory.RememberLiteralToolName, Reason: memory.MemoryProposalBackground, Literal: &own, Everywhere: &everywhere}},
		Run: memory.BackgroundRun{RunID: "run-web", SessionID: session.ID, Trigger: memory.BackgroundTriggerIdle, Model: "fake",
			ThroughSequence: said.Sequence, StartedAt: now, FinishedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, background.HolderID, background.FencingToken); err != nil {
		t.Fatal(err)
	}
	handler := web.WithCandidateReview(web.NewServer(nil), store).Handler()

	page := reviewResponse[memory.MemoryProposalPage](t, proposalPost(t, handler, "list", map[string]any{}), 200)
	if len(page.Items) != 1 || page.Items[0].Everywhere == nil || page.Items[0].Everywhere.ScopeKey != "global" ||
		page.Items[0].Everywhere.PreparedSHA256 != everywhere.PreparedSHA256 || page.Items[0].ScopeKey != "workspace:"+string(workspace.ID) {
		t.Fatalf("inbox page = %+v", page)
	}
	item := page.Items[0]
	if w := proposalPost(t, handler, "approve", map[string]any{"proposal_id": item.ProposalID, "prepared_sha256": item.PreparedSHA256,
		"always_applies": true, "everywhere": true}); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "proposal_changed") {
		t.Fatalf("the Global version approved under its proposal's hash = %d %s", w.Code, w.Body.String())
	}
	approved := reviewResponse[memory.MemoryProposalResult](t, proposalPost(t, handler, "approve", map[string]any{"proposal_id": item.ProposalID,
		"prepared_sha256": item.Everywhere.PreparedSHA256, "always_applies": true, "everywhere": true}), 200)
	if approved.ClaimID != everywhere.ClaimID || !approved.Standing {
		t.Fatalf("approved = %+v", approved)
	}
}
