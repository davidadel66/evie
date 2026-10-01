package eviedb_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// Harness review M4: a Predicate's label or cardinality may drift between
// proposals. Labels that differ only in case or spacing are one definition,
// and real drift may still version, but conflict diagnostics span every
// version of a token instead of going silent.
type predicateDriftFixture struct {
	t       *testing.T
	ctx     context.Context
	store   *eviedb.Store
	session memory.Session
	lease   memory.TurnLease
	index   int
}

func newPredicateDriftFixture(t *testing.T) *predicateDriftFixture {
	t.Helper()
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "predicate-drift", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return &predicateDriftFixture{t: t, ctx: ctx, store: store, session: session, lease: lease, index: 900}
}

func (f *predicateDriftFixture) prepare(label string, cardinality memory.PredicateCardinality, value string) memory.RememberLiteralProposal {
	f.t.Helper()
	f.index++
	event, err := f.store.AppendEventWithLease(f.ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Remember that my home city is " + value + ".",
	})
	if err != nil {
		f.t.Fatal(err)
	}
	proposal, err := f.store.PrepareRememberLiteral(f.ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: semanticIdempotencyKey(f.index), SourceEventID: event.ID, Predicate: "home_city",
		PredicateLabel: label, PredicateCardinality: cardinality,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

func (f *predicateDriftFixture) apply(proposal memory.RememberLiteralProposal) memory.RememberLiteralResult {
	f.t.Helper()
	result, err := f.store.ApplyRememberLiteral(f.ctx, f.lease, proposal)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *predicateDriftFixture) warnings() []memory.ClaimConflictWarning {
	f.t.Helper()
	inspection, err := f.store.InspectLiteralClaims(f.ctx, f.session.ScopeContext())
	if err != nil {
		f.t.Fatal(err)
	}
	return inspection.Warnings
}

func TestSemanticMemoryCaseOrSpacingOnlyLabelReusesPredicateDefinition(t *testing.T) {
	t.Parallel()
	f := newPredicateDriftFixture(t)
	first := f.prepare("home city", memory.CardinalityOne, "Detroit")
	detroit := f.apply(first)
	second := f.prepare("  Home   City ", memory.CardinalityOne, "Chicago")
	if second.Predicate.Create || second.Predicate.ID != first.Predicate.ID || second.Predicate.Version != 1 || second.Predicate.Label != "home city" {
		t.Fatalf("a case or spacing difference minted a new Predicate definition: first=%+v second=%+v", first.Predicate, second.Predicate)
	}
	chicago := f.apply(second)
	// An idempotent retry with the caller's own spelling returns the same proposal.
	retry, err := f.store.PrepareRememberLiteral(f.ctx, f.session.ScopeContext(), second.Request)
	if err != nil || retry.OperationID != second.OperationID {
		t.Fatalf("idempotent retry with the original label spelling failed: %+v %v", retry.OperationID, err)
	}
	want := []memory.ClaimConflictWarning{{Code: memory.ConflictOneCardinality, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(detroit.ClaimID, chicago.ClaimID)}}
	if got := f.warnings(); !equalConflictWarnings(got, want) {
		t.Fatalf("warnings = %+v, want %+v", got, want)
	}
}

func TestSemanticMemoryConflictsSpanPredicateVersions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		label       string
		cardinality memory.PredicateCardinality
	}{
		{name: "label drift", label: "city of residence", cardinality: memory.CardinalityOne},
		{name: "cardinality drift", label: "home city", cardinality: memory.CardinalityMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPredicateDriftFixture(t)
			detroit := f.apply(f.prepare("home city", memory.CardinalityOne, "Detroit"))
			drifted := f.prepare(tc.label, tc.cardinality, "Chicago")
			if !drifted.Predicate.Create || drifted.Predicate.Version != 2 {
				t.Fatalf("a real definition change must stay a visible new version: %+v", drifted.Predicate)
			}
			chicago := f.apply(drifted)
			want := []memory.ClaimConflictWarning{{Code: memory.ConflictOneCardinality, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(detroit.ClaimID, chicago.ClaimID)}}
			if got := f.warnings(); !equalConflictWarnings(got, want) {
				t.Fatalf("drift hid the conflict: warnings = %+v, want %+v", got, want)
			}
		})
	}
}
