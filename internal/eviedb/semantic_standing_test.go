package eviedb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type standingFixture struct {
	store  *Store
	clock  time.Time
	next   int
	leases map[memory.SessionID]memory.TurnLease
}

func newStandingFixture(t *testing.T) *standingFixture {
	t.Helper()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &standingFixture{store: NewStore(db), clock: time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC), leases: map[memory.SessionID]memory.TurnLease{}}
	f.store.now = func() time.Time { f.clock = f.clock.Add(time.Second); return f.clock }
	return f
}

func (f *standingFixture) workspaceSession(t *testing.T, name string) (memory.Workspace, memory.Session) {
	t.Helper()
	workspace, err := f.store.RegisterWorkspace(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return workspace, f.sessionIn(t, workspace)
}

func (f *standingFixture) sessionIn(t *testing.T, workspace memory.Workspace) memory.Session {
	t.Helper()
	session, err := f.store.CreateWorkspaceSessionWithComposition(context.Background(), workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func (f *standingFixture) globalSession(t *testing.T) memory.Session {
	t.Helper()
	session, err := f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func (f *standingFixture) lease(t *testing.T, session memory.Session) memory.TurnLease {
	t.Helper()
	if lease, ok := f.leases[session.ID]; ok {
		return lease
	}
	f.next++
	lease, err := f.store.AcquireTurnLease(context.Background(), session.ID, memory.LeaseHolderID(fmt.Sprintf("standing-%d", f.next)), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.leases[session.ID] = lease
	return lease
}

func (f *standingFixture) say(t *testing.T, session memory.Session, lease memory.TurnLease, content string) memory.Event {
	t.Helper()
	event, err := f.store.AppendEventWithLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func (f *standingFixture) key() string {
	f.next++
	return fmt.Sprintf("idem:v1:97000000-0000-4000-8000-%012d", f.next)
}

// prepareRemember prepares one owner literal with the given kind ("" is the
// pre-standing request) in a fresh turn.
func (f *standingFixture) prepareRemember(t *testing.T, session memory.Session, predicate, label, value string, kind memory.ClaimKind, destination memory.MemoryDestination, ratio int64) (memory.TurnLease, memory.RememberLiteralProposal) {
	t.Helper()
	lease := f.lease(t, session)
	event := f.say(t, session, lease, fmt.Sprintf("Remember that my %s is %s.", label, value))
	proposal, err := f.store.PrepareRememberLiteral(context.Background(), session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: f.key(), SourceEventID: event.ID, Destination: destination,
		Predicate: predicate, PredicateLabel: label, PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed,
		Kind: kind, StandingBytesPerTokenMilli: ratio,
	})
	if err != nil {
		t.Fatal(err)
	}
	return lease, proposal
}

func (f *standingFixture) remember(t *testing.T, session memory.Session, predicate, label, value string, kind memory.ClaimKind, destination memory.MemoryDestination) memory.RememberLiteralProposal {
	t.Helper()
	lease, proposal := f.prepareRemember(t, session, predicate, label, value, kind, destination, 0)
	if _, err := f.store.ApplyRememberLiteral(context.Background(), lease, proposal); err != nil {
		t.Fatal(err)
	}
	return proposal
}

// switchStanding is what the approval card's "Always applies" switch
// approves: the same proposal with the standing flag flipped and its
// prepared alternative hashes.
func switchStanding(t *testing.T, proposal memory.RememberLiteralProposal) memory.RememberLiteralProposal {
	t.Helper()
	if proposal.Classification == nil || proposal.StandingAlternative == nil {
		t.Fatalf("proposal has no standing alternative: %+v", proposal.Classification)
	}
	switched := proposal
	classification := *proposal.Classification
	classification.Standing = !classification.Standing
	switched.Classification = &classification
	switched.ProposalSHA256, switched.PreparedSHA256 = proposal.StandingAlternative.ProposalSHA256, proposal.StandingAlternative.PreparedSHA256
	switched.StandingAlternative = nil
	return switched
}

func standingTexts(t *testing.T, store *Store, session memory.Session) []string {
	t.Helper()
	items, err := store.StandingMemory(context.Background(), session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = memory.StandingScopeLabel(item.ScopeKey) + " " + string(item.Kind) + " " + item.Text
	}
	return texts
}

func assertStandingTexts(t *testing.T, store *Store, session memory.Session, want ...string) {
	t.Helper()
	got := standingTexts(t, store, session)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("standing memory =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func assertProjectionReplays(t *testing.T, store *Store) {
	t.Helper()
	verification, err := store.VerifySemanticProjection(context.Background())
	if err != nil || !verification.Valid {
		t.Fatalf("replay verification = %+v, %v", verification, err)
	}
}

func TestStandingKindDefaultsPersistAndReplay(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)

	preference := f.remember(t, session, "answer_style", "answer style", "concise answers", memory.ClaimKindPreference, "")
	if preference.Classification == nil || *preference.Classification != (memory.ClaimClassification{Kind: memory.ClaimKindPreference, Standing: true}) ||
		preference.StandingAlternative == nil {
		t.Fatalf("preference classification = %+v alternative=%v", preference.Classification, preference.StandingAlternative)
	}
	fact := f.remember(t, session, "favorite_color", "favorite color", "teal", memory.ClaimKindFact, "")
	if fact.Classification == nil || fact.Classification.Standing {
		t.Fatalf("fact classification = %+v", fact.Classification)
	}
	legacy := f.remember(t, session, "home_city", "home city", "Boston", "", "")
	if legacy.Classification != nil || legacy.StandingAlternative != nil {
		t.Fatalf("pre-standing request was classified: %+v", legacy.Classification)
	}
	lease, instruction := f.prepareRemember(t, session, "reply_language", "reply language", "French", memory.ClaimKindInstruction, "", 0)
	if _, err := f.store.ApplyRememberLiteral(context.Background(), lease, switchStanding(t, instruction)); err != nil {
		t.Fatalf("approving with Always applies switched off: %v", err)
	}
	if _, err := f.store.PrepareRememberLiteral(context.Background(), session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: f.key(), SourceEventID: preference.Source.EventID, Predicate: "answer_style", PredicateLabel: "answer style",
		PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "concise answers"},
		Polarity: memory.PolarityAffirmed, Kind: "habit",
	}); err == nil {
		t.Fatal("unknown memory kind was accepted")
	}

	want := "Global preference owner — answer style: concise answers"
	assertStandingTexts(t, f.store, session, want)
	assertStandingTexts(t, f.store, f.globalSession(t), want)
	_, workspaceSession := f.workspaceSession(t, "Elsewhere")
	assertStandingTexts(t, f.store, workspaceSession, want)
	assertProjectionReplays(t, f.store)
}

func TestStandingMemoryOrdersScopesAndIsolatesWorkspaces(t *testing.T) {
	f := newStandingFixture(t)
	global := f.globalSession(t)
	kitchen, kitchenSession := f.workspaceSession(t, "Kitchen")
	_, garden := f.workspaceSession(t, "Garden")

	f.remember(t, kitchenSession, "reply_language", "reply language", "French", memory.ClaimKindInstruction, memory.MemorySession)
	f.remember(t, kitchenSession, "kitchen_brief", "kitchen brief", "brunch on weekends", memory.ClaimKindWorkspaceBrief, "")
	f.remember(t, global, "answer_style", "answer style", "concise answers", memory.ClaimKindPreference, "")

	assertStandingTexts(t, f.store, kitchenSession,
		"Global preference owner — answer style: concise answers",
		"Workspace workspace_brief owner — kitchen brief: brunch on weekends",
		"This conversation instruction owner — reply language: French")
	assertStandingTexts(t, f.store, f.sessionIn(t, kitchen),
		"Global preference owner — answer style: concise answers",
		"Workspace workspace_brief owner — kitchen brief: brunch on weekends")
	assertStandingTexts(t, f.store, garden, "Global preference owner — answer style: concise answers")
	assertStandingTexts(t, f.store, global, "Global preference owner — answer style: concise answers")
}

func TestStandingMemoryFullRefusesApprovalAndListsItems(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)
	long := func(prefix string) string { return prefix + " " + strings.Repeat("detail ", 55) }
	var current []memory.RememberLiteralProposal
	for i := 0; i < 3; i++ {
		current = append(current, f.remember(t, session, fmt.Sprintf("style_%d", i), fmt.Sprintf("style %d", i), long(fmt.Sprintf("rule %d", i)), memory.ClaimKindPreference, ""))
	}
	before := standingTexts(t, f.store, session)
	if len(before) != 3 {
		t.Fatalf("standing memory before the limit = %v", before)
	}
	var claims int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims`).Scan(&claims); err != nil {
		t.Fatal(err)
	}

	lease, over := f.prepareRemember(t, session, "style_3", "style 3", long("rule 3"), memory.ClaimKindPreference, "", 0)
	_, err := f.store.ApplyRememberLiteral(context.Background(), lease, over)
	var full *memory.StandingMemoryFullError
	if !errors.Is(err, memory.ErrStandingMemoryFull) || !errors.As(err, &full) {
		t.Fatalf("approval past the budget = %v", err)
	}
	if full.Tier != memory.StandingTierProfile || full.BudgetTokens != 500 || len(full.Current) != 3 {
		t.Fatalf("memory full report = %+v", full)
	}
	for _, proposal := range current {
		if !strings.Contains(err.Error(), string(proposal.ClaimID)) {
			t.Fatalf("memory full message omits %s: %v", proposal.ClaimID, err)
		}
	}
	var after int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims`).Scan(&after); err != nil || after != claims {
		t.Fatalf("refused approval wrote Claims: %d -> %d (%v)", claims, after, err)
	}
	assertStandingTexts(t, f.store, session, before...)

	if _, err := f.store.ApplyRememberLiteral(context.Background(), lease, switchStanding(t, over)); err != nil {
		t.Fatalf("the same memory without Always applies: %v", err)
	}
	assertStandingTexts(t, f.store, session, before...)

	lease, calibrated := f.prepareRemember(t, session, "style_4", "style 4", long("rule 4"), memory.ClaimKindPreference, "", 4600)
	if _, err := f.store.ApplyRememberLiteral(context.Background(), lease, calibrated); err != nil {
		t.Fatalf("a calibrated ratio measures fewer tokens: %v", err)
	}
	_, workspaceSession := f.workspaceSession(t, "Brief")
	f.remember(t, workspaceSession, "brief", "brief", long("workspace"), memory.ClaimKindWorkspaceBrief, "")
	assertProjectionReplays(t, f.store)
}

func TestRetiredSupersededAndPromotedStandingMemory(t *testing.T) {
	f := newStandingFixture(t)
	general, session := f.workspaceSession(t, "General")
	preference := f.remember(t, session, "answer_style", "answer style", "concise answers", memory.ClaimKindPreference, "")
	assertStandingTexts(t, f.store, session, "Workspace preference owner — answer style: concise answers")

	lease := f.lease(t, session)
	retire := f.say(t, session, lease, "Forget the answer style for now.")
	retirement, err := f.store.PrepareMemoryLifecycle(context.Background(), session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: f.key(), SourceEventID: retire.ID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: preference.ClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendExactSemanticApproval(t, context.Background(), f.store, lease, retire.ID, retirement.OperationID, retirement.ProposalSHA256, retirement.PreparedSHA256)
	if _, err := f.store.ApplyMemoryLifecycle(context.Background(), lease, retirement); err != nil {
		t.Fatal(err)
	}
	assertStandingTexts(t, f.store, session)

	second := f.remember(t, session, "answer_length", "answer length", "short", memory.ClaimKindPreference, "")
	correctionEvent := f.say(t, session, lease, "Correction: my answer length is very short.")
	correction, err := f.store.PrepareCorrectClaim(context.Background(), session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: f.key(), SourceEventID: correctionEvent.ID, OldClaimID: second.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: second.Subject.ID, PredicateID: second.Predicate.ID,
			Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "very short"}}, Polarity: memory.PolarityAffirmed},
	})
	if err != nil {
		t.Fatal(err)
	}
	if correction.Classification == nil || *correction.Classification != (memory.ClaimClassification{Kind: memory.ClaimKindPreference, Standing: true}) {
		t.Fatalf("replacement did not inherit its standing classification: %+v", correction.Classification)
	}
	if _, err := f.store.ApplyCorrectClaim(context.Background(), lease, correction); err != nil {
		t.Fatal(err)
	}
	assertStandingTexts(t, f.store, session, "Workspace preference owner — answer length: very short")

	promotionEvent := f.say(t, session, lease, "Make my answer length apply everywhere.")
	promotion, err := f.store.PreparePromotion(context.Background(), session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: f.key(), SourceEventID: promotionEvent.ID, SourceClaimID: correction.ReplacementClaim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if promotion.Classification == nil || !promotion.Classification.Standing || promotion.StandingAlternative == nil {
		t.Fatalf("promotion classification = %+v", promotion.Classification)
	}
	appendExactSemanticApproval(t, context.Background(), f.store, lease, promotionEvent.ID, promotion.OperationID, promotion.ProposalSHA256, promotion.PreparedSHA256)
	if _, err := f.store.ApplyPromotion(context.Background(), lease, promotion); err != nil {
		t.Fatal(err)
	}
	assertStandingTexts(t, f.store, f.globalSession(t), "Global preference owner — answer length: very short")
	items, err := f.store.StandingMemory(context.Background(), f.sessionIn(t, general).ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if rendered := memory.RenderStandingMemory(items); strings.Count(rendered, "very short") != 1 || len(items) != 2 {
		t.Fatalf("promoted copy rendered %d items: %s", len(items), rendered)
	}
	assertProjectionReplays(t, f.store)
}

func TestSetStandingMarksAnExistingClaimThroughApproval(t *testing.T) {
	f := newStandingFixture(t)
	_, session := f.workspaceSession(t, "General")
	concise := f.remember(t, session, "answer_style", "answer style", "concise answers", "", "")
	f.remember(t, session, "answer_format", "answer format", "bullet lists", "", "")
	assertStandingTexts(t, f.store, session)

	lease := f.lease(t, session)
	ask := f.say(t, session, lease, "Always apply my concise answers preference.")
	prepare := func(standing bool, kind memory.ClaimKind) memory.ClaimStandingProposal {
		t.Helper()
		proposal, err := f.store.PrepareClaimStanding(context.Background(), session.ScopeContext(), memory.ClaimStandingRequest{
			IdempotencyKey: f.key(), SourceEventID: ask.ID, ClaimID: concise.ClaimID, Kind: kind, Standing: standing,
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	proposal := prepare(true, memory.ClaimKindPreference)
	if proposal.SchemaVersion != 7 || proposal.Kind != "classify_claim" || proposal.PriorOperationID != "" ||
		proposal.Prior != (memory.ClaimClassification{Kind: memory.ClaimKindFact}) ||
		proposal.Classification != (memory.ClaimClassification{Kind: memory.ClaimKindPreference, Standing: true}) ||
		proposal.ClaimText != "owner — answer style: concise answers" {
		t.Fatalf("standing proposal = %+v", proposal)
	}
	if _, err := f.store.ApplyClaimStanding(context.Background(), lease, proposal); err == nil {
		t.Fatal("standing change applied without its approval")
	}
	stale := prepare(true, memory.ClaimKindInstruction)
	appendExactSemanticApproval(t, context.Background(), f.store, lease, ask.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	if _, err := f.store.ApplyClaimStanding(context.Background(), lease, proposal); err != nil {
		t.Fatal(err)
	}
	assertStandingTexts(t, f.store, session, "Workspace preference owner — answer style: concise answers")
	assertStandingTexts(t, f.store, f.globalSession(t))

	appendExactSemanticApproval(t, context.Background(), f.store, lease, ask.ID, stale.OperationID, stale.ProposalSHA256, stale.PreparedSHA256)
	if _, err := f.store.ApplyClaimStanding(context.Background(), lease, stale); err == nil {
		t.Fatal("a standing change prepared against an older classification applied")
	}

	off := prepare(false, "")
	if off.Prior != (memory.ClaimClassification{Kind: memory.ClaimKindPreference, Standing: true}) || off.PriorOperationID != proposal.OperationID ||
		off.Classification != (memory.ClaimClassification{Kind: memory.ClaimKindPreference}) {
		t.Fatalf("switch-off proposal = %+v", off)
	}
	appendExactSemanticApproval(t, context.Background(), f.store, lease, ask.ID, off.OperationID, off.ProposalSHA256, off.PreparedSHA256)
	if _, err := f.store.ApplyClaimStanding(context.Background(), lease, off); err != nil {
		t.Fatal(err)
	}
	assertStandingTexts(t, f.store, session)
	var version int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM semantic_operations WHERE schema_version = 7 AND operation_kind = 'classify_claim'`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("classify operations = %d (%v)", version, err)
	}
	assertProjectionReplays(t, f.store)
}

func TestSetStandingRefusesPastTheWorkspaceBudget(t *testing.T) {
	f := newStandingFixture(t)
	_, session := f.workspaceSession(t, "General")
	long := strings.Repeat("detail ", 115)
	f.remember(t, session, "brief", "brief", long, memory.ClaimKindWorkspaceBrief, "")
	other := f.remember(t, session, "second_brief", "second brief", long, "", "")
	lease := f.lease(t, session)
	ask := f.say(t, session, lease, "Always apply my second brief too.")
	proposal, err := f.store.PrepareClaimStanding(context.Background(), session.ScopeContext(), memory.ClaimStandingRequest{
		IdempotencyKey: f.key(), SourceEventID: ask.ID, ClaimID: other.ClaimID, Kind: memory.ClaimKindWorkspaceBrief, Standing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendExactSemanticApproval(t, context.Background(), f.store, lease, ask.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	if _, err := f.store.ApplyClaimStanding(context.Background(), lease, proposal); !errors.Is(err, memory.ErrStandingMemoryFull) {
		t.Fatalf("standing past the Workspace budget = %v", err)
	}
}
