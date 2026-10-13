package eviedb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// A background proposal from a Workspace or project conversation targets that
// scope and may carry a prepared Global version, which the inbox offers as
// "Save everywhere instead" (memory.decisions.md, 2026-10-12).

// prepareVersions prepares one owner literal in the session's own scope and
// the same request for Global, under one idempotency key, the way the
// background pass prepares a memory Evie suggests holds everywhere.
func (f *standingFixture) prepareVersions(t *testing.T, session memory.Session, message, value string) (memory.Event, memory.RememberLiteralProposal, memory.RememberLiteralProposal) {
	t.Helper()
	lease := f.lease(t, session)
	said := f.say(t, session, lease, message)
	f.release(t, session)
	request := memory.RememberLiteralRequest{
		Destination: memory.MemoryWorkspace, IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: "diet", PredicateLabel: "diet",
		PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindProfileFact,
	}
	own, err := f.store.PrepareRememberLiteral(context.Background(), session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Destination = memory.MemoryEverywhere
	everywhere, err := f.store.PrepareRememberLiteral(context.Background(), session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	return said, own, everywhere
}

// commitBackground commits one background pass over the session through the
// given owner message, queueing the proposals.
func (f *standingFixture) commitBackground(t *testing.T, session memory.Session, from int64, said memory.Event, proposals ...memory.QueuedMemoryProposal) (memory.BackgroundCommitResult, error) {
	t.Helper()
	lease, err := f.store.AcquireTurnLease(context.Background(), session.ID, "memory-background:test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.store.ReleaseTurnLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken)
	}()
	return f.store.CommitMemoryBackgroundPass(context.Background(), lease, memory.BackgroundCommit{Proposals: proposals, Run: backgroundRun(session, from, said.Sequence)})
}

func withEverywhere(own, everywhere memory.RememberLiteralProposal) memory.QueuedMemoryProposal {
	return memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName, Reason: memory.MemoryProposalBackground, Literal: &own, Everywhere: &everywhere}
}

func claimScopeKeys(t *testing.T, store *Store, value string) []string {
	t.Helper()
	rows, err := store.db.Query(`SELECT scopes.scope_key FROM semantic_claims AS claims
		JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id WHERE claims.literal_value = ? ORDER BY scopes.scope_key`, value)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	return keys
}

func TestABackgroundProposalOffersItsPreparedGlobalVersion(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	workspace, session := f.workspaceSession(t, "Kitchen")
	said, own, everywhere := f.prepareVersions(t, session, "I'm vegetarian.", "vegetarian")
	if result, err := f.commitBackground(t, session, 0, said, withEverywhere(own, everywhere)); err != nil || result.Queued != 1 {
		t.Fatalf("commit = %+v, %v", result, err)
	}
	page := pendingProposals(t, f.store)
	workspaceKey := "workspace:" + string(workspace.ID)
	if len(page.Items) != 1 || page.Items[0].ScopeKey != workspaceKey || page.Items[0].PreparedSHA256 != own.PreparedSHA256 {
		t.Fatalf("inbox = %+v", page)
	}
	item := page.Items[0]
	if item.Everywhere == nil || item.Everywhere.ScopeKey != "global" || item.Everywhere.PreparedSHA256 != everywhere.PreparedSHA256 ||
		!strings.Contains(item.Everywhere.Proposal, `"scope_key":"global"`) || !strings.Contains(item.Everywhere.Proposal, "vegetarian") {
		t.Fatalf("Global version = %+v", item.Everywhere)
	}

	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256,
		AlwaysApplies: true, Everywhere: true}); !errors.Is(err, ErrMemoryProposalChanged) {
		t.Fatalf("approving the Global version under its proposal's hash = %v", err)
	}
	result, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID,
		PreparedSHA256: item.Everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.ClaimID != everywhere.ClaimID || result.OperationID != everywhere.OperationID || result.PreparedSHA256 != everywhere.PreparedSHA256 || !result.Standing {
		t.Fatalf("result = %+v", result)
	}
	if keys := claimScopeKeys(t, f.store, "vegetarian"); len(keys) != 1 || keys[0] != "global" {
		t.Fatalf("the memory was saved in %v, want only global", keys)
	}
	var approvals int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ? AND event_type = ? AND execution_id = ?`,
		session.ID, memory.EventApproval, everywhere.OperationID).Scan(&approvals); err != nil || approvals != 1 {
		t.Fatalf("approval events naming the Global version = %d, %v", approvals, err)
	}
	if page := pendingProposals(t, f.store); page.Pending != 0 {
		t.Fatalf("the approved proposal is still pending: %+v", page)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: true}); !errors.Is(err, ErrMemoryProposalResolved) {
		t.Fatalf("the proposal's own version applied after its Global version: %v", err)
	}
}

func TestApprovingABackgroundProposalInItsOwnScopeLeavesGlobalUntouched(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	workspace, session := f.workspaceSession(t, "Kitchen")
	said, own, everywhere := f.prepareVersions(t, session, "I'm vegetarian.", "vegetarian")
	if _, err := f.commitBackground(t, session, 0, said, withEverywhere(own, everywhere)); err != nil {
		t.Fatal(err)
	}
	item := pendingProposals(t, f.store).Items[0]
	result, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: false})
	if err != nil || result.ClaimID != own.ClaimID || result.Standing {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if keys := claimScopeKeys(t, f.store, "vegetarian"); len(keys) != 1 || keys[0] != "workspace:"+string(workspace.ID) {
		t.Fatalf("the memory was saved in %v, want only the Workspace", keys)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID,
		PreparedSHA256: item.Everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true}); !errors.Is(err, ErrMemoryProposalResolved) {
		t.Fatalf("the Global version applied after its proposal: %v", err)
	}
}

func TestQueueingRefusesAGlobalVersionThatIsNotTheSameMemory(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	_, session := f.workspaceSession(t, "Kitchen")
	said, own, everywhere := f.prepareVersions(t, session, "I'm vegetarian and I cook Egyptian food.", "vegetarian")

	other := everywhere.Request
	other.Literal.Value = "Egyptian food"
	different, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), other)
	if err != nil {
		t.Fatal(err)
	}
	for _, refused := range []struct {
		queued memory.QueuedMemoryProposal
		reason string
	}{
		{withEverywhere(own, different), "states the same memory"},
		{withEverywhere(own, own), "targets Global"},
		{withEverywhere(everywhere, everywhere), "only a Workspace or project memory"},
	} {
		if _, err := f.commitBackground(t, session, 0, said, refused.queued); err == nil || !strings.Contains(err.Error(), refused.reason) {
			t.Fatalf("commit = %v, want a refusal that %s", err, refused.reason)
		}
	}
	if page := pendingProposals(t, f.store); page.Pending != 0 {
		t.Fatalf("a refused commit queued something: %+v", page)
	}

	lease := f.lease(t, session)
	asked := f.say(t, session, lease, "Remember I'm vegetarian.")
	request := own.Request
	request.IdempotencyKey, request.SourceEventID = f.key(), asked.ID
	inTurn, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Destination = memory.MemoryEverywhere
	inTurnGlobal, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	intent := f.intent(t, session, lease, asked.ID)
	if _, err := f.store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName, IntentEventID: intent.ID,
		Reason: memory.MemoryProposalNotRequested, Literal: &inTurn, Everywhere: &inTurnGlobal}); err == nil || !strings.Contains(err.Error(), "only a background proposal") {
		t.Fatalf("an in-turn proposal with a Global version = %v", err)
	}
}

func TestAStaleGlobalVersionIsPreparedAgainWithItsProposal(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	_, session := f.workspaceSession(t, "Kitchen")
	said, own, everywhere := f.prepareVersions(t, session, "I'm vegetarian.", "vegetarian")
	if _, err := f.commitBackground(t, session, 0, said, withEverywhere(own, everywhere)); err != nil {
		t.Fatal(err)
	}
	// Another Global memory moves the Global scope both versions read.
	f.remember(t, f.globalSession(t), "home_city", "home city", "Boston", memory.ClaimKindProfileFact, memory.MemoryEverywhere)

	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: pendingID(t, f.store),
		PreparedSHA256: everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true}); !errors.Is(err, ErrMemoryProposalChanged) {
		t.Fatalf("a stale Global version applied: %v", err)
	}
	item := pendingProposals(t, f.store).Items[0]
	if item.Revision < 2 || !item.Approvable || item.Everywhere == nil || item.Everywhere.PreparedSHA256 == everywhere.PreparedSHA256 {
		t.Fatalf("refreshed item = %+v", item)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID,
		PreparedSHA256: item.Everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true}); err != nil {
		t.Fatalf("the refreshed Global version could not be approved: %v", err)
	}
	if keys := claimScopeKeys(t, f.store, "vegetarian"); len(keys) != 1 || keys[0] != "global" {
		t.Fatalf("the memory was saved in %v", keys)
	}
}

func TestAGlobalVersionThatAlreadyExistsIsDroppedOnRefresh(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	_, session := f.workspaceSession(t, "Kitchen")
	said, own, everywhere := f.prepareVersions(t, session, "I'm vegetarian.", "vegetarian")
	if _, err := f.commitBackground(t, session, 0, said, withEverywhere(own, everywhere)); err != nil {
		t.Fatal(err)
	}
	// The same memory is saved everywhere from another conversation.
	f.remember(t, f.globalSession(t), "diet", "diet", "vegetarian", memory.ClaimKindProfileFact, memory.MemoryEverywhere)
	item := pendingProposals(t, f.store).Items[0]
	if item.Everywhere != nil || !item.Approvable {
		t.Fatalf("a Global version of an existing Global memory was still offered: %+v", item)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID,
		PreparedSHA256: everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true}); !errors.Is(err, ErrMemoryProposalChanged) {
		t.Fatalf("approving a dropped Global version = %v", err)
	}
}

func pendingID(t *testing.T, store *Store) string {
	t.Helper()
	var id string
	if err := store.db.QueryRow(`SELECT proposal_id FROM memory_proposal_inbox ORDER BY queued_at LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// stage4InboxColumns are the inbox columns before the Global version.
const stage4InboxColumns = `proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id, intent_event_id,
	target_claim_id, dedupe_key, proposal_json, proposal_sha256, prepared_sha256, alternative_proposal_sha256,
	alternative_prepared_sha256, revision, queued_at, refreshed_at`

const stage4InboxTable = `
CREATE TABLE memory_proposal_inbox (
    proposal_id                 TEXT PRIMARY KEY,
    session_id                  TEXT NOT NULL REFERENCES sessions(id),
    scope_key                   TEXT NOT NULL,
    tool_name                   TEXT NOT NULL CHECK (tool_name IN ('memory_remember_literal', 'memory_remember_entity', 'memory_correct_claim', 'memory_retire')),
    reason                      TEXT NOT NULL CHECK (reason IN ('not_requested', 'no_explicit_cue', 'background')),
    idempotency_key             TEXT NOT NULL UNIQUE,
    source_event_id             TEXT NOT NULL,
    intent_event_id             TEXT,
    target_claim_id             TEXT,
    dedupe_key                  TEXT NOT NULL,
    proposal_json               TEXT NOT NULL,
    proposal_sha256             TEXT NOT NULL,
    prepared_sha256             TEXT NOT NULL,
    alternative_proposal_sha256 TEXT,
    alternative_prepared_sha256 TEXT,
    revision                    INTEGER NOT NULL CHECK (revision > 0),
    queued_at                   TEXT NOT NULL,
    refreshed_at                TEXT,
    CHECK ((reason = 'background') = (intent_event_id IS NULL)),
    CHECK (reason = 'background' OR tool_name IN ('memory_remember_literal', 'memory_remember_entity'))
);`

func TestTheInboxGainsTheGlobalVersionColumnsInPlace(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	kept, queued := f.propose(t, session, "Keep it short, I hate long answers.", "answer_style", "answer style", "short answers", memory.ClaimKindPreference)
	f.release(t, session)
	conn, err := f.store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys = OFF`,
		`CREATE TABLE inbox_copy AS SELECT ` + stage4InboxColumns + ` FROM memory_proposal_inbox`,
		`DROP TABLE memory_proposal_inbox`,
		stage4InboxTable,
		`INSERT INTO memory_proposal_inbox (` + stage4InboxColumns + `) SELECT ` + stage4InboxColumns + ` FROM inbox_copy`,
		`DROP TABLE inbox_copy`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	conn.Close()
	for range 2 {
		if err := ensureMemoryProposalInbox(ctx, f.store.db); err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"everywhere_proposal_json", "everywhere_proposal_sha256", "everywhere_prepared_sha256",
		"everywhere_alternative_proposal_sha256", "everywhere_alternative_prepared_sha256"} {
		if present, err := tableHasColumn(ctx, f.store.db, "memory_proposal_inbox", column); err != nil || !present {
			t.Fatalf("column %s present=%v err=%v", column, present, err)
		}
	}
	page := pendingProposals(t, f.store)
	if len(page.Items) != 1 || page.Items[0].ProposalID != queued.ProposalID || page.Items[0].Everywhere != nil {
		t.Fatalf("migrated inbox = %+v", page)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: queued.ProposalID, PreparedSHA256: kept.PreparedSHA256, AlwaysApplies: true}); err != nil {
		t.Fatalf("a migrated proposal could not be approved: %v", err)
	}
}
