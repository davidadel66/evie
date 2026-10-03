package eviedb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// The Stage 2 inbox table admitted only in-turn remember proposals. Startup
// rebuilds it for background proposals, keeping every row and decline.
const stage2InboxTable = `
CREATE TABLE memory_proposal_inbox (
    proposal_id                 TEXT PRIMARY KEY,
    session_id                  TEXT NOT NULL REFERENCES sessions(id),
    scope_key                   TEXT NOT NULL,
    tool_name                   TEXT NOT NULL CHECK (tool_name IN ('memory_remember_literal', 'memory_remember_entity')),
    reason                      TEXT NOT NULL CHECK (reason IN ('not_requested', 'no_explicit_cue')),
    idempotency_key             TEXT NOT NULL UNIQUE,
    source_event_id             TEXT NOT NULL,
    intent_event_id             TEXT NOT NULL,
    dedupe_key                  TEXT NOT NULL,
    proposal_json               TEXT NOT NULL,
    proposal_sha256             TEXT NOT NULL,
    prepared_sha256             TEXT NOT NULL,
    alternative_proposal_sha256 TEXT,
    alternative_prepared_sha256 TEXT,
    revision                    INTEGER NOT NULL CHECK (revision > 0),
    queued_at                   TEXT NOT NULL,
    refreshed_at                TEXT
);`

func TestStage2InboxIsMigratedForBackgroundProposalsKeepingEveryRow(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	kept, queued := f.propose(t, session, "Keep it short, I hate long answers.", "answer_style", "answer style", "short answers", memory.ClaimKindPreference)
	_, declined := f.propose(t, session, "I like tea.", "drink", "drink", "tea", memory.ClaimKindPreference)
	if err := f.store.DeclineMemoryProposal(ctx, declined.ProposalID); err != nil {
		t.Fatal(err)
	}
	f.release(t, session)
	conn, err := f.store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys = OFF`,
		`CREATE TABLE inbox_copy AS SELECT proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id, intent_event_id,
			dedupe_key, proposal_json, proposal_sha256, prepared_sha256, alternative_proposal_sha256, alternative_prepared_sha256, revision, queued_at, refreshed_at
			FROM memory_proposal_inbox`,
		`DROP TABLE memory_proposal_inbox`,
		stage2InboxTable,
		`INSERT INTO memory_proposal_inbox SELECT * FROM inbox_copy`,
		`DROP TABLE inbox_copy`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	conn.Close()
	if err := ensureMemoryProposalInbox(ctx, f.store.db); err != nil {
		t.Fatal(err)
	}
	if err := ensureMemoryProposalInbox(ctx, f.store.db); err != nil {
		t.Fatalf("the migration is not idempotent: %v", err)
	}
	var definition string
	if err := f.store.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'memory_proposal_inbox'`).Scan(&definition); err != nil ||
		!strings.Contains(definition, "'background'") || !strings.Contains(definition, "target_claim_id") {
		t.Fatalf("definition = %s, %v", definition, err)
	}
	page := pendingProposals(t, f.store)
	if page.Pending != 1 || len(page.Items) != 1 || page.Items[0].ProposalID != queued.ProposalID || page.Items[0].PreparedSHA256 != kept.PreparedSHA256 {
		t.Fatalf("migrated inbox = %+v", page)
	}
	if err := f.store.DeclineMemoryProposal(ctx, declined.ProposalID); err != nil {
		t.Fatalf("the earlier decline was lost: %v", err)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: queued.ProposalID, PreparedSHA256: kept.PreparedSHA256, AlwaysApplies: true}); err != nil {
		t.Fatalf("a migrated proposal could not be approved: %v", err)
	}
}

func TestInTurnQueueingAcceptsOnlyRememberProposalsWithTheirIntent(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	lease := f.lease(t, session)
	said := f.say(t, session, lease, "I live in Boston.")
	proposal, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: "home_city", PredicateLabel: "home city",
		PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Boston"},
		Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindProfileFact,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent := f.intent(t, session, lease, said.ID)
	if _, err := f.store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName, IntentEventID: intent.ID,
		Reason: memory.MemoryProposalBackground, Literal: &proposal}); err == nil {
		t.Fatal("a turn queued a proposal with the background reason")
	}
	if _, err := f.store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{ToolName: memory.CorrectClaimToolName, IntentEventID: intent.ID,
		Reason: memory.MemoryProposalNotRequested, Literal: &proposal}); err == nil {
		t.Fatal("a mismatched tool name was queued")
	}
	if _, err := f.store.db.Exec(`INSERT INTO memory_proposal_inbox (proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id,
		intent_event_id, dedupe_key, proposal_json, proposal_sha256, prepared_sha256, revision, queued_at)
		VALUES ('x', ?, 'global', 'memory_retire', 'not_requested', 'k', 'e', 'i', 'd', '{}', 'p', 'q', 1, 't')`, session.ID); err == nil {
		t.Fatal("the table admitted an in-turn retirement")
	}
	if _, err := f.store.db.Exec(`INSERT INTO memory_proposal_inbox (proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id,
		intent_event_id, dedupe_key, proposal_json, proposal_sha256, prepared_sha256, revision, queued_at)
		VALUES ('y', ?, 'global', 'memory_remember_literal', 'background', 'k2', 'e', 'i', 'd', '{}', 'p', 'q', 1, 't')`, session.ID); err == nil {
		t.Fatal("the table admitted a background proposal citing a tool intent")
	}
}

func backgroundRun(session memory.Session, from, through int64) memory.BackgroundRun {
	now := time.Now()
	return memory.BackgroundRun{RunID: "run-" + time.Now().Format("150405.000000000"), SessionID: session.ID, Trigger: memory.BackgroundTriggerIdle,
		Model: "fake", FromSequence: from, ThroughSequence: through, StartedAt: now, FinishedAt: now}
}

func TestBackgroundCommitIsFencedByTheTurnLeaseAndRefusesSecrets(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	lease := f.lease(t, session)
	said := f.say(t, session, lease, "My password: hunter2hunter2 is what I use.")
	f.release(t, session)
	proposal, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: "password", PredicateLabel: "password",
		PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "password: hunter2hunter2"},
		Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindProfileFact,
	})
	if err != nil {
		t.Fatal(err)
	}
	queued := memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName, Reason: memory.MemoryProposalBackground, Literal: &proposal}
	commit := memory.BackgroundCommit{Proposals: []memory.QueuedMemoryProposal{queued}, Run: backgroundRun(session, 0, said.Sequence)}
	stale := memory.TurnLease{SessionID: session.ID, HolderID: "nobody", FencingToken: 99}
	if _, err := f.store.CommitMemoryBackgroundPass(ctx, stale, commit); err == nil {
		t.Fatal("a commit without the conversation's lease succeeded")
	}
	background, err := f.store.AcquireTurnLease(ctx, session.ID, "memory-background:test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.CommitMemoryBackgroundPass(ctx, background, commit)
	if err != nil || result.Refused != 1 || result.Queued != 0 {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if page := pendingProposals(t, f.store); page.Pending != 0 {
		t.Fatalf("a secret was queued: %+v", page)
	}
	if _, err := f.store.CommitMemoryBackgroundPass(ctx, background, commit); !errors.Is(err, memory.ErrBackgroundCursorMoved) {
		t.Fatalf("a covered batch committed twice: %v", err)
	}
	progress, err := f.store.MemoryBackgroundProgress(ctx, time.Now(), 10)
	if err != nil || progress.LastRun == nil || progress.LastRun.Skipped != 1 || len(progress.Sessions) != 1 || progress.Sessions[0].Cursor != said.Sequence {
		t.Fatalf("progress = %+v, %v", progress, err)
	}
}

func TestBackgroundBatchReadsOnlyOwnerMessagesAfterTheCursorWithinBounds(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	lease := f.lease(t, session)
	first := f.say(t, session, lease, "one")
	if _, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID: first.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "assistant words"}); err != nil {
		t.Fatal(err)
	}
	f.say(t, session, lease, "/compact")
	f.say(t, session, lease, "token sk-abcdefghijklmnopqrstuvwxyz0123")
	f.say(t, session, lease, "two")
	f.say(t, session, lease, "three")
	f.release(t, session)
	batch, err := f.store.MemoryBackgroundBatch(ctx, session.ID, memory.BackgroundBatchLimits{MaxMessages: 2, MaxBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var contents []string
	for _, message := range batch.Messages {
		contents = append(contents, message.Content)
	}
	if strings.Join(contents, "|") != "one|two" || batch.Commands != 1 || batch.Withheld != 1 || !batch.More || batch.FromSequence != 0 {
		t.Fatalf("batch = %+v contents = %v", batch, contents)
	}
	through := batch.Messages[0].Sequence
	limited, err := f.store.MemoryBackgroundBatch(ctx, session.ID, memory.BackgroundBatchLimits{MaxMessages: 5, MaxBytes: 1000, Through: through})
	if err != nil || len(limited.Messages) != 1 || limited.ThroughSequence != through || limited.More {
		t.Fatalf("limited = %+v, %v", limited, err)
	}
}
