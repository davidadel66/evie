package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// Proactive memory proposals (docs/memory-architecture.md, Stage 2): a
// proposal the owner did not explicitly ask for is queued, exactly as
// prepared, in the review inbox and applies only through an inbox approval.

func (f *standingFixture) release(t *testing.T, session memory.Session) {
	t.Helper()
	lease, ok := f.leases[session.ID]
	if !ok {
		return
	}
	if err := f.store.ReleaseTurnLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	delete(f.leases, session.ID)
}

func (f *standingFixture) intent(t *testing.T, session memory.Session, lease memory.TurnLease, parent memory.EventID) memory.Event {
	t.Helper()
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + string(parent), Name: memory.RememberLiteralToolName, Arguments: "{}"}})
	event, err := f.store.AppendEventWithLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID: parent, Type: memory.EventToolIntent, ExecutionID: memory.ExecutionID("70000000-0000-4000-8000-" + fmt.Sprintf("%012d", f.next)), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

// propose queues one owner literal that Evie noticed in message, the way the
// remember tool does when the owner did not ask.
func (f *standingFixture) propose(t *testing.T, session memory.Session, message, predicate, label, value string, kind memory.ClaimKind) (memory.RememberLiteralProposal, memory.MemoryProposalQueued) {
	t.Helper()
	return f.proposeTo(t, session, "", message, predicate, label, value, kind)
}

// proposeTo is propose with an explicit memory destination.
func (f *standingFixture) proposeTo(t *testing.T, session memory.Session, destination memory.MemoryDestination, message, predicate, label, value string, kind memory.ClaimKind) (memory.RememberLiteralProposal, memory.MemoryProposalQueued) {
	t.Helper()
	lease := f.lease(t, session)
	said := f.say(t, session, lease, message)
	proposal, err := f.store.PrepareRememberLiteral(context.Background(), session.ScopeContext(), memory.RememberLiteralRequest{
		Destination: destination, IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: predicate, PredicateLabel: label,
		PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		Polarity: memory.PolarityAffirmed, Kind: kind,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent := f.intent(t, session, lease, said.ID)
	queued, err := f.store.QueueMemoryProposal(context.Background(), lease, memory.QueuedMemoryProposal{
		ToolName: memory.RememberLiteralToolName, IntentEventID: intent.ID, Reason: memory.MemoryProposalNotRequested, Literal: &proposal,
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal, queued
}

func pendingProposals(t *testing.T, store *Store) memory.MemoryProposalPage {
	t.Helper()
	page, err := store.ListMemoryProposals(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func claimCount(t *testing.T, store *Store) int {
	t.Helper()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// An archived Workspace has no memory proposals (workspaces.decisions.md,
// 2026-10-11): the ones targeting it or found in its conversations are
// hidden, uncounted and refused, nothing new is queued for it, and restoring
// it brings the hidden ones back.
func TestArchivedWorkspaceProposalsAreHiddenUntilRestored(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	workspace, session := f.workspaceSession(t, "Old project")
	elsewhere := f.globalSession(t)
	f.proposeTo(t, session, memory.MemoryWorkspace, "Deploys go out on Fridays.", "deploy_day", "deploy day", "Fridays", memory.ClaimKindFact)
	f.proposeTo(t, session, memory.MemoryEverywhere, "I merge my own changes.", "merge_style", "merge style", "merge my own changes", memory.ClaimKindPreference)
	f.propose(t, elsewhere, "Keep it short, I hate long answers.", "answer_style", "answer style", "short answers", memory.ClaimKindPreference)
	before := pendingProposals(t, f.store)
	scopes := map[string]bool{}
	for _, item := range before.Items {
		scopes[item.ScopeKey] = true
	}
	workspaceKey := "workspace:" + string(workspace.ID)
	if before.Pending != 3 || len(before.Items) != 3 || !scopes[workspaceKey] || !scopes["global"] {
		t.Fatalf("inbox before archive = %+v", before)
	}
	// A proposal prepared before the archive is not queued after it.
	lease := f.lease(t, session)
	said := f.say(t, session, lease, "I prefer green tea.")
	late, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		Destination: memory.MemoryEverywhere, IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: "drink", PredicateLabel: "drink",
		PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "green tea"},
		Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindPreference,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent := f.intent(t, session, lease, said.ID)
	f.release(t, session)
	f.release(t, elsewhere)

	if _, err := f.store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	page := pendingProposals(t, f.store)
	if page.Pending != 1 || len(page.Items) != 1 || page.Items[0].SessionID != elsewhere.ID {
		t.Fatalf("inbox while archived = %+v", page)
	}
	if count, err := f.store.PendingMemoryProposalCount(ctx, workspaceKey, "global"); err != nil || count != 1 {
		t.Fatalf("pending count while archived = %d, %v", count, err)
	}
	for _, item := range before.Items {
		if item.SessionID != session.ID {
			continue
		}
		_, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256})
		if !errors.Is(err, ErrMemoryProposalUnavailable) || !strings.Contains(err.Error(), "workspace is archived") {
			t.Fatalf("approving %s while archived = %v", item.ScopeKey, err)
		}
	}
	if claimCount(t, f.store) != 0 {
		t.Fatal("an archived Workspace's proposal saved memory")
	}
	lease = f.lease(t, session)
	if _, err := f.store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{
		ToolName: memory.RememberLiteralToolName, IntentEventID: intent.ID, Reason: memory.MemoryProposalNotRequested, Literal: &late,
	}); !errors.Is(err, ErrMemoryProposalWorkspaceArchived) {
		t.Fatalf("queueing into an archived Workspace = %v", err)
	}
	f.release(t, session)

	if _, err := f.store.RestoreWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	after := pendingProposals(t, f.store)
	if after.Pending != 3 || len(after.Items) != 3 {
		t.Fatalf("inbox after restore = %+v", after)
	}
	for _, item := range after.Items {
		if item.ScopeKey != workspaceKey {
			continue
		}
		if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256}); err != nil {
			t.Fatalf("approving after restore: %v", err)
		}
	}
	if claimCount(t, f.store) != 1 {
		t.Fatal("the restored Workspace's proposal did not apply")
	}
}

func TestInboxShowsAClippedSourceMessage(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)
	message := "I prefer short answers. " + strings.Repeat("é", inboxSourceMessageLimit)
	f.propose(t, session, message, "answer_style", "answer style", "short answers", memory.ClaimKindPreference)
	got := pendingProposals(t, f.store).Items[0].SourceMessage
	if len(got) > inboxSourceMessageLimit || !strings.HasPrefix(got, "I prefer short answers. ") || !strings.HasSuffix(got, "é…") {
		t.Fatalf("source message = %d bytes, %q…%q", len(got), got[:24], got[len(got)-8:])
	}
}

func TestQueuedProposalCreatesNoClaimUntilApprovedFromTheInbox(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	proposal, queued := f.propose(t, session, "Keep it short, I hate long answers.", "answer_style", "answer style", "short answers", memory.ClaimKindPreference)
	if queued.ProposalID == "" || queued.Duplicate {
		t.Fatalf("queued = %+v", queued)
	}
	if claimCount(t, f.store) != 0 || len(standingTexts(t, f.store, session)) != 0 {
		t.Fatal("queueing a proposal created memory")
	}
	page := pendingProposals(t, f.store)
	if page.Pending != 1 || len(page.Items) != 1 {
		t.Fatalf("inbox = %+v", page)
	}
	item := page.Items[0]
	exact, _ := json.Marshal(proposal)
	if item.ProposalID != queued.ProposalID || item.Proposal != string(exact) || item.PreparedSHA256 != proposal.PreparedSHA256 ||
		!item.Approvable || item.ScopeKey != "global" || item.SessionID != session.ID || item.Reason != memory.MemoryProposalNotRequested ||
		item.ToolName != memory.RememberLiteralToolName || item.Revision != 1 || item.SourceMessage != "Keep it short, I hate long answers." {
		t.Fatalf("inbox item = %+v", item)
	}

	approval := memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: true}
	if _, err := f.store.ApproveMemoryProposal(ctx, approval); !errors.Is(err, ErrMemoryProposalBusy) {
		t.Fatalf("approval while the conversation's turn holds its lease = %v", err)
	}
	f.release(t, session)
	result, err := f.store.ApproveMemoryProposal(ctx, approval)
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != proposal.OperationID || result.ClaimID != proposal.ClaimID || !result.Standing ||
		result.ProposalSHA256 != proposal.ProposalSHA256 || result.PreparedSHA256 != proposal.PreparedSHA256 {
		t.Fatalf("inbox approval result = %+v", result)
	}
	assertStandingTexts(t, f.store, session, "Global preference owner — answer style: short answers")
	if page := pendingProposals(t, f.store); page.Pending != 0 || len(page.Items) != 0 {
		t.Fatalf("approved proposal is still pending: %+v", page)
	}

	// The durable approval names exactly the applied hashes, in the source
	// conversation, like an approval card's.
	events, err := f.store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var approvals int
	for _, event := range events {
		if event.Type != memory.EventApproval || event.ExecutionID != memory.ExecutionID(proposal.OperationID) {
			continue
		}
		var payload memory.ApprovalPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Decision != memory.ApprovalApproved ||
			payload.ProposalSHA256 != proposal.ProposalSHA256 || payload.PreparedSHA256 != proposal.PreparedSHA256 || event.ParentID != proposal.Source.EventID {
			t.Fatalf("approval event = %+v %+v (%v)", event, payload, err)
		}
		approvals++
	}
	if approvals != 1 {
		t.Fatalf("approval events = %d", approvals)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, approval); !errors.Is(err, ErrMemoryProposalResolved) {
		t.Fatalf("second approval = %v", err)
	}
	assertProjectionReplays(t, f.store)
}

func TestInboxApprovalHonorsTheAlwaysAppliesSwitch(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)
	proposal, _ := f.propose(t, session, "I always want metric units.", "unit_system", "unit system", "metric", memory.ClaimKindPreference)
	f.release(t, session)
	item := pendingProposals(t, f.store).Items[0]
	result, err := f.store.ApproveMemoryProposal(context.Background(), memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Standing || result.ProposalSHA256 != proposal.StandingAlternative.ProposalSHA256 || result.PreparedSHA256 != proposal.StandingAlternative.PreparedSHA256 {
		t.Fatalf("switched approval = %+v, want the prepared alternative %+v", result, proposal.StandingAlternative)
	}
	if claimCount(t, f.store) != 1 || len(standingTexts(t, f.store, session)) != 0 {
		t.Fatal("the switched-off memory is missing or always applies")
	}
}

func TestStaleInboxProposalIsRevalidatedBeforeItApplies(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	f.propose(t, session, "Use British spelling.", "spelling", "spelling", "British", memory.ClaimKindPreference)
	second, _ := f.propose(t, session, "I'm vegetarian.", "diet", "diet", "vegetarian", memory.ClaimKindProfileFact)
	f.release(t, session)
	page := pendingProposals(t, f.store)
	if len(page.Items) != 2 {
		t.Fatalf("inbox = %+v", page)
	}
	shown := page.Items[1]
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: page.Items[0].ProposalID, PreparedSHA256: page.Items[0].PreparedSHA256, AlwaysApplies: true}); err != nil {
		t.Fatal(err)
	}

	// The first approval moved Global's revision, so the second proposal as
	// shown is stale: it is refreshed, not applied against newer memory.
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: shown.ProposalID, PreparedSHA256: shown.PreparedSHA256, AlwaysApplies: true}); !errors.Is(err, ErrMemoryProposalChanged) {
		t.Fatalf("approving a stale proposal = %v", err)
	}
	if claimCount(t, f.store) != 1 {
		t.Fatal("a stale proposal applied")
	}
	refreshed := pendingProposals(t, f.store)
	if len(refreshed.Items) != 1 || refreshed.Items[0].ProposalID != shown.ProposalID || refreshed.Items[0].Revision != 2 ||
		refreshed.Items[0].PreparedSHA256 == shown.PreparedSHA256 || !refreshed.Items[0].Approvable {
		t.Fatalf("refreshed inbox = %+v", refreshed)
	}
	var current memory.RememberLiteralProposal
	if err := json.Unmarshal([]byte(refreshed.Items[0].Proposal), &current); err != nil || current.Literal != second.Literal ||
		current.Source.EventID != second.Source.EventID || current.Source.Authority != second.Source.Authority ||
		*current.Classification != *second.Classification || current.PriorRevisions[0].Revision == second.PriorRevisions[0].Revision {
		t.Fatalf("refreshed proposal = %+v (%v)", current, err)
	}
	result, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: shown.ProposalID, PreparedSHA256: refreshed.Items[0].PreparedSHA256, AlwaysApplies: true})
	if err != nil || result.OperationID != current.OperationID {
		t.Fatalf("approving the refreshed proposal = %+v %v", result, err)
	}

	// A proposal whose conversation was archived cannot apply; it waits.
	third, _ := f.propose(t, session, "My partner is called Sam.", "partner_name", "partner name", "Sam", memory.ClaimKindProfileFact)
	f.release(t, session)
	if _, err := f.store.ArchiveSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	archived := pendingProposals(t, f.store)
	if len(archived.Items) != 1 || archived.Items[0].Approvable || !strings.Contains(archived.Items[0].Problem, "archived") {
		t.Fatalf("archived conversation's proposal = %+v", archived)
	}
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: archived.Items[0].ProposalID, PreparedSHA256: third.PreparedSHA256, AlwaysApplies: true}); !errors.Is(err, ErrMemoryProposalUnavailable) {
		t.Fatalf("approving an archived conversation's proposal = %v", err)
	}
	if claimCount(t, f.store) != 2 {
		t.Fatal("an archived conversation's proposal applied")
	}
}

func TestInboxApprovalEnforcesTheStandingBudget(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)
	long := func(prefix string) string { return prefix + " " + strings.Repeat("detail ", 55) }
	for i := 0; i < 3; i++ {
		f.remember(t, session, fmt.Sprintf("style_%d", i), fmt.Sprintf("style %d", i), long(fmt.Sprintf("rule %d", i)), memory.ClaimKindPreference, "")
	}
	f.propose(t, session, "Also, never use tables.", "style_3", "style 3", long("rule 3"), memory.ClaimKindPreference)
	f.release(t, session)
	item := pendingProposals(t, f.store).Items[0]
	before := claimCount(t, f.store)
	_, err := f.store.ApproveMemoryProposal(context.Background(), memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: true})
	if !errors.Is(err, memory.ErrStandingMemoryFull) {
		t.Fatalf("inbox approval past the budget = %v", err)
	}
	if claimCount(t, f.store) != before {
		t.Fatal("memory full approval wrote a Claim")
	}
	page := pendingProposals(t, f.store)
	if len(page.Items) != 1 || !page.Items[0].Approvable {
		t.Fatalf("a memory-full proposal left the inbox: %+v", page)
	}
	if _, err := f.store.ApproveMemoryProposal(context.Background(), memory.MemoryProposalApproval{ProposalID: page.Items[0].ProposalID, PreparedSHA256: page.Items[0].PreparedSHA256, AlwaysApplies: false}); err != nil {
		t.Fatalf("the same memory without Always applies: %v", err)
	}
}

func TestQueueDeduplicatesDeclinesAndWithholdsSecrets(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	_, first := f.propose(t, session, "I drink my coffee black.", "coffee_style", "coffee style", "black", memory.ClaimKindPreference)
	_, again := f.propose(t, session, "Black coffee for me, as usual.", "coffee_style", "coffee style", "black", memory.ClaimKindPreference)
	if !again.Duplicate || again.ProposalID != first.ProposalID || pendingProposals(t, f.store).Pending != 1 {
		t.Fatalf("duplicate proposal = %+v (first %+v)", again, first)
	}

	lease := f.lease(t, session)
	said := f.say(t, session, lease, "Here is the deploy key: sk-abcdefghijklmnopqrstuvwx")
	secret, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: f.key(), SourceEventID: said.ID, Predicate: "deploy_key", PredicateLabel: "deploy key",
		PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "sk-abcdefghijklmnopqrstuvwx"},
		Polarity: memory.PolarityAffirmed, Kind: memory.ClaimKindFact,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.QueueMemoryProposal(ctx, lease, memory.QueuedMemoryProposal{ToolName: memory.RememberLiteralToolName,
		IntentEventID: f.intent(t, session, lease, said.ID).ID, Reason: memory.MemoryProposalNotRequested, Literal: &secret}); err == nil {
		t.Fatal("a proposal holding a secret was queued")
	}

	if err := f.store.DeclineMemoryProposal(ctx, first.ProposalID); err != nil {
		t.Fatal(err)
	}
	if page := pendingProposals(t, f.store); page.Pending != 0 {
		t.Fatalf("declined proposal is pending: %+v", page)
	}
	f.release(t, session)
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: first.ProposalID, PreparedSHA256: "sha256:any", AlwaysApplies: true}); !errors.Is(err, ErrMemoryProposalResolved) {
		t.Fatalf("approving a declined proposal = %v", err)
	}
	if claimCount(t, f.store) != 0 {
		t.Fatal("a declined proposal applied")
	}
}

func TestOwnerRequestsMemoryReadsTheBoundOwnerMessage(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	session := f.globalSession(t)
	lease := f.lease(t, session)
	asked := f.say(t, session, lease, "Please remember that I take my tea with lemon.")
	told := f.say(t, session, lease, "I take my tea with lemon.")
	for event, want := range map[memory.EventID]bool{asked.ID: true, told.ID: false} {
		got, err := f.store.OwnerRequestsMemory(ctx, session.ScopeContext(), event)
		if err != nil || got != want {
			t.Fatalf("owner requests memory in %s = %v %v, want %v", event, got, err, want)
		}
	}
	other := f.globalSession(t)
	if got, err := f.store.OwnerRequestsMemory(ctx, other.ScopeContext(), asked.ID); err != nil || got {
		t.Fatalf("another conversation's message counted as a request: %v %v", got, err)
	}
}
