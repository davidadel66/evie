package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCorrectClaimErrorPreservesBitemporalHistoryAndExactBoundaries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, acceptedAt)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "correct-error", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	oldProposal := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000001", "My timezone was Detroit", "Detroit",
		memory.ValidTime{From: &from, To: &to})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, oldProposal)
	if err != nil {
		t.Fatal(err)
	}

	correctedAt := acceptedAt.Add(time.Hour)
	setTurnLeaseTime(store, correctedAt)
	correctionSource, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Correction: my timezone was Chicago",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000002",
		SourceEventID:  correctionSource.ID,
		OldClaimID:     oldResult.ClaimID,
		Mode:           memory.CorrectionError,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: oldProposal.Subject.ID,
			PredicateID:     oldProposal.Predicate.ID,
			Object:          memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity:        memory.PolarityAffirmed,
		},
	})
	if err != nil {
		t.Fatalf("prepare correction: %v", err)
	}
	if proposal.OldClaim.ID != oldResult.ClaimID || proposal.Mode != memory.CorrectionError ||
		proposal.ExpectedRevision != 1 || proposal.Source.EventID != correctionSource.ID ||
		!validTimesEqual(proposal.ValidTimeEffect.OldBefore, memory.ValidTime{From: &from, To: &to}) ||
		!validTimesEqual(proposal.ValidTimeEffect.OldAfter, memory.ValidTime{From: &from, To: &to}) ||
		!validTimesEqual(proposal.ValidTimeEffect.Replacement, memory.ValidTime{From: &from, To: &to}) {
		t.Fatalf("correction proposal omitted a typed effect: %+v", proposal)
	}
	if len(proposal.Transitions) != 3 || proposal.Transitions[0].ObjectID != oldResult.ClaimID ||
		proposal.Transitions[0].State != memory.SemanticStateSuperseded ||
		proposal.Transitions[1].ObjectID != proposal.ReplacementClaim.ID ||
		proposal.Transitions[1].State != memory.SemanticStateActive ||
		proposal.Transitions[2].ObjectID != proposal.Source.ID ||
		proposal.Transitions[2].State != memory.SemanticStateEligible {
		t.Fatalf("correction transitions = %+v", proposal.Transitions)
	}
	result, err := store.ApplyCorrectClaim(ctx, lease, proposal)
	if err != nil {
		t.Fatalf("apply correction: %v", err)
	}
	if result.OldClaimID != oldResult.ClaimID || result.ReplacementClaimID != proposal.ReplacementClaim.ID ||
		result.ScopeRevision != 2 || !result.TransactionTime.Equal(correctedAt) {
		t.Fatalf("correction result = %+v", result)
	}
	reprepared, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), proposal.Request)
	if err != nil {
		t.Fatalf("prepare idempotent correction retry: %v", err)
	}
	replayed, err := store.ApplyCorrectClaim(ctx, lease, reprepared)
	if err != nil {
		t.Fatalf("apply idempotent correction retry: %v", err)
	}
	if replayed.OperationID != result.OperationID || replayed.ReplacementClaimID != result.ReplacementClaimID ||
		replayed.ScopeRevision != result.ScopeRevision || !replayed.TransactionTime.Equal(result.TransactionTime) {
		t.Fatalf("idempotent correction result = %+v, want %+v", replayed, result)
	}

	assertLiteralClaimAt(t, ctx, store, session, from.Add(-time.Nanosecond), correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, from, correctedAt, "Chicago")
	assertLiteralClaimAt(t, ctx, store, session, from.Add(time.Hour), correctedAt, "Chicago")
	assertLiteralClaimAt(t, ctx, store, session, to, correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, to.Add(time.Nanosecond), correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, from.Add(time.Hour), oldResult.TransactionTime, "Detroit")

	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if !current.ValidAt.Equal(correctedAt) || !current.AsKnownAt.Equal(correctedAt) {
		t.Fatalf("current query effective times = %s / %s", current.ValidAt, current.AsKnownAt)
	}
	rolledBack := correctedAt.Add(-30 * time.Minute)
	setTurnLeaseTime(store, rolledBack)
	current, err = store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if !current.ValidAt.Equal(rolledBack) || !current.AsKnownAt.Equal(correctedAt) ||
		len(current.Claims) != 1 || current.Claims[0].Object.Literal.Value != "Chicago" {
		t.Fatalf("current query did not capture distinct world/accepted times: %+v", current)
	}
	validOnly := from.Add(time.Hour)
	validQuery, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{ValidAt: &validOnly})
	if err != nil {
		t.Fatal(err)
	}
	if !validQuery.ValidAt.Equal(validOnly) || !validQuery.AsKnownAt.Equal(correctedAt) ||
		len(validQuery.Claims) != 1 || validQuery.Claims[0].Object.Literal.Value != "Chicago" {
		t.Fatalf("Valid-Time-only query = %+v", validQuery)
	}
	knownQuery, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{AsKnownAt: &oldResult.TransactionTime})
	if err != nil {
		t.Fatal(err)
	}
	if !knownQuery.ValidAt.Equal(rolledBack) || !knownQuery.AsKnownAt.Equal(oldResult.TransactionTime) ||
		len(knownQuery.Claims) != 1 || knownQuery.Claims[0].Object.Literal.Value != "Detroit" {
		t.Fatalf("Transaction-Time-only query = %+v", knownQuery)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened := NewStore(db)
	setTurnLeaseTime(reopened, correctedAt)
	assertLiteralClaimAt(t, ctx, reopened, session, from, correctedAt, "Chicago")
}

func TestCorrectClaimErrorUsesExplicitReplacementIntervalWithoutRewritingOldClaim(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	correctedAt := acceptedAt.Add(time.Hour)
	setTurnLeaseTime(store, acceptedAt)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "correct-explicit-interval", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oldFrom := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	oldTo := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	old := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000006", "Detroit", "Detroit",
		memory.ValidTime{From: &oldFrom, To: &oldTo})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	replacementFrom := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	replacementTo := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, correctedAt)
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Correction: Chicago only during 2024",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000007", SourceEventID: source.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
		ReplacementValidTime: &memory.ValidTime{From: &replacementFrom, To: &replacementTo},
		Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed},
	}
	illegal := request
	illegal.IdempotencyKey = "idem:v1:77000000-0000-4000-8000-000000000008"
	illegal.ReplacementValidTime = &memory.ValidTime{From: &replacementFrom, To: &replacementFrom}
	if _, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), illegal); err == nil {
		t.Fatal("empty explicit replacement interval unexpectedly prepared")
	}
	var revision, operationCount, correctionCount int
	if err := db.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = 'global'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations`).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_claim_corrections`).Scan(&correctionCount); err != nil {
		t.Fatal(err)
	}
	if revision != 1 || operationCount != 1 || correctionCount != 0 {
		t.Fatalf("illegal explicit interval changed semantic state: revision=%d operations=%d corrections=%d",
			revision, operationCount, correctionCount)
	}
	proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !validTimesEqual(proposal.ValidTimeEffect.Replacement, *request.ReplacementValidTime) {
		t.Fatalf("explicit replacement interval = %+v", proposal.ValidTimeEffect.Replacement)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	assertLiteralClaimAt(t, ctx, store, session, replacementFrom.Add(-time.Nanosecond), correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, replacementFrom, correctedAt, "Chicago")
	assertLiteralClaimAt(t, ctx, store, session, replacementFrom.Add(time.Hour), correctedAt, "Chicago")
	assertLiteralClaimAt(t, ctx, store, session, replacementTo, correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, replacementTo.Add(time.Nanosecond), correctedAt, "")
	assertLiteralClaimAt(t, ctx, store, session, replacementFrom, oldResult.TransactionTime, "Detroit")
	var persistedFrom, persistedTo string
	if err := db.QueryRowContext(ctx, `SELECT valid_from, valid_to FROM semantic_claims WHERE claim_id = ?`, oldResult.ClaimID).
		Scan(&persistedFrom, &persistedTo); err != nil {
		t.Fatal(err)
	}
	if persistedFrom != formatSemanticTime(oldFrom) || persistedTo != formatSemanticTime(oldTo) {
		t.Fatalf("old immutable interval changed to [%s,%s)", persistedFrom, persistedTo)
	}
}

func TestCorrectClaimChangedSplitsWorldTimeWithoutRewritingOldClaim(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, acceptedAt)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "correct-changed", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	old := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000011", "I live in Detroit", "Detroit",
		memory.ValidTime{From: &from})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	effective := time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC)
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I moved to Chicago on April 1, 2025",
	})
	if err != nil {
		t.Fatal(err)
	}
	illegalEffective := from
	if _, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000013", SourceEventID: source.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionChanged, EffectiveTime: &illegalEffective,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed,
		},
	}); err == nil {
		t.Fatal("changed correction that creates an empty old interval unexpectedly prepared")
	}
	proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000012", SourceEventID: source.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionChanged, EffectiveTime: &effective,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ValidTimeEffect.OldAfter.To == nil || !proposal.ValidTimeEffect.OldAfter.To.Equal(effective) ||
		proposal.ValidTimeEffect.Replacement.From == nil || !proposal.ValidTimeEffect.Replacement.From.Equal(effective) {
		t.Fatalf("changed correction did not split at effective time: %+v", proposal.ValidTimeEffect)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	assertLiteralClaimAt(t, ctx, store, session, effective.Add(-time.Nanosecond), acceptedAt, "Detroit")
	assertLiteralClaimAt(t, ctx, store, session, effective, acceptedAt, "Chicago")

	var literalValue string
	var validTo *string
	if err := db.QueryRowContext(ctx, `SELECT literal_value, valid_to FROM semantic_claims WHERE claim_id = ?`, oldResult.ClaimID).Scan(&literalValue, &validTo); err != nil {
		t.Fatal(err)
	}
	if literalValue != "Detroit" || validTo != nil {
		t.Fatalf("old immutable Claim was rewritten: literal=%q valid_to=%v", literalValue, validTo)
	}
}

func TestCorrectClaimPreservesEntityObjectAndPredicateSemantics(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 13, 30, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "correct-entity", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice mentors Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000015", SourceEventID: source.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldResult, err := store.ApplyRememberEntity(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	correctionSource, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Correction: Alice does not mentor Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000016", SourceEventID: correctionSource.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: old.Claim.SubjectEntityID, PredicateID: old.Predicate.ID,
			Object: memory.ClaimObject{EntityID: old.Claim.ObjectEntityID}, Polarity: memory.PolarityDenied,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ReplacementClaim.Object.EntityID != old.Claim.ObjectEntityID ||
		proposal.ReplacementClaim.Predicate.ID != old.Predicate.ID ||
		proposal.ReplacementClaim.Predicate.Token != old.Predicate.Token ||
		proposal.ReplacementClaim.Predicate.Version != old.Predicate.Version ||
		proposal.ReplacementClaim.Polarity != memory.PolarityDenied {
		t.Fatalf("Entity correction proposition = %+v", proposal.ReplacementClaim)
	}
	readTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	visibleBefore, err := store.inspectClaimsSnapshot(ctx, readTx, session.ScopeContext(), false, memory.ClaimQuery{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	diagnosticsBefore, err := store.inspectClaimsSnapshot(ctx, readTx, session.ScopeContext(), false, memory.ClaimQuery{
		ValidAt: &visibleBefore.ValidAt, AsKnownAt: &visibleBefore.AsKnownAt,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	visibleEntity, diagnosticEntity := entityClaimsFromInspection(visibleBefore), entityClaimsFromInspection(diagnosticsBefore)
	if visibleBefore.ScopeRevision != 1 || diagnosticsBefore.ScopeRevision != 1 ||
		len(visibleEntity) != 1 || len(diagnosticEntity) != 1 ||
		visibleEntity[0].Claim.Polarity != memory.PolarityAffirmed ||
		diagnosticEntity[0].Claim.Polarity != memory.PolarityAffirmed {
		t.Fatalf("focused Entity inspection mixed a same-time correction: visible=%+v diagnostics=%+v",
			visibleBefore, diagnosticsBefore)
	}
	if err := readTx.Commit(); err != nil {
		t.Fatal(err)
	}
	inspection, err := store.InspectEntityClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 1 || inspection.Claims[0].Claim.ID != proposal.ReplacementClaim.ID ||
		inspection.Claims[0].Claim.ObjectEntityID != old.Claim.ObjectEntityID ||
		inspection.Claims[0].Claim.Polarity != memory.PolarityDenied || inspection.Claims[0].Object.CanonicalName != "Bob" {
		t.Fatalf("corrected Entity Claim inspection = %+v", inspection)
	}
}

func TestCorrectClaimRejectsInvalidAndStaleProposalsWithoutSemanticWrites(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	setTurnLeaseTime(store, time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC))
	lease, err := store.AcquireTurnLease(ctx, session.ID, "correct-invalid", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	old := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000021", "I live in Detroit", "Detroit", memory.ValidTime{})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	makeRequest := func(key, content string) memory.CorrectClaimRequest {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		return memory.CorrectClaimRequest{
			IdempotencyKey: key, SourceEventID: source.ID, OldClaimID: oldResult.ClaimID,
			Mode: memory.CorrectionError, Replacement: memory.ClaimProposition{
				SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
				Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: content}},
				Polarity: memory.PolarityAffirmed,
			},
		}
	}
	invalid := makeRequest("idem:v1:77000000-0000-4000-8000-000000000022", "invalid")
	invalid.Mode = "fabricated"
	if _, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), invalid); err == nil {
		t.Fatal("invalid correction mode unexpectedly prepared")
	}
	missingEffective := makeRequest("idem:v1:77000000-0000-4000-8000-000000000023", "missing effective")
	missingEffective.Mode = memory.CorrectionChanged
	if _, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), missingEffective); err == nil {
		t.Fatal("changed correction without effective time unexpectedly prepared")
	}

	first, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), makeRequest(
		"idem:v1:77000000-0000-4000-8000-000000000024", "Chicago"))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), makeRequest(
		"idem:v1:77000000-0000-4000-8000-000000000025", "New York"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, stale); !errors.Is(err, ErrStaleScopeRevision) {
		t.Fatalf("stale correction error = %v, want ErrStaleScopeRevision", err)
	}
	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if current.ScopeRevision != 2 || len(current.Claims) != 1 || current.Claims[0].Object.Literal.Value != "Chicago" {
		t.Fatalf("invalid/stale correction changed state: %+v", current)
	}
	for table, want := range map[string]int{
		"semantic_operations": 2, "semantic_claims": 2, "semantic_source_links": 2,
		"semantic_claim_corrections": 1, "semantic_state_events": 5,
	} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s rows = %d, want %d, error = %v", table, count, want, err)
		}
	}
}

func TestSemanticOperationEncodingV2MigratesV1AndPreservesLegacyRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	acceptedAt := time.Date(2026, 9, 2, 14, 30, 0, 0, time.UTC)
	setTurnLeaseTime(store, acceptedAt)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "v2-migration", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	legacy := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000026", "Detroit", "Detroit", memory.ValidTime{})
	legacyResult, err := store.ApplyRememberLiteral(ctx, lease, legacy)
	if err != nil {
		t.Fatal(err)
	}
	downgradeSemanticOperationsToV1(t, ctx, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("migrate v1 operation table: %v", err)
	}
	defer db.Close()
	store = NewStore(db)
	setTurnLeaseTime(store, acceptedAt.Add(time.Hour))
	retry, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), legacy.Request)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.ApplyRememberLiteral(ctx, lease, retry)
	if err != nil {
		t.Fatal(err)
	}
	if retried.OperationID != legacyResult.OperationID || retried.ClaimID != legacyResult.ClaimID ||
		retried.ScopeRevision != legacyResult.ScopeRevision {
		t.Fatalf("v1 retry changed after v2 migration: got %+v, want %+v", retried, legacyResult)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Correction: Chicago",
	})
	if err != nil {
		t.Fatal(err)
	}
	correction, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000027", SourceEventID: source.ID,
		OldClaimID: legacyResult.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: legacy.Subject.ID, PredicateID: legacy.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed},
	})
	if err != nil {
		t.Fatal(err)
	}
	if correction.SchemaVersion != 2 {
		t.Fatalf("correction schema version = %d, want 2", correction.SchemaVersion)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, correction); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, `SELECT schema_version, operation_kind FROM semantic_operations ORDER BY schema_version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version int
		var kind string
		if err := rows.Scan(&version, &kind); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, fmt.Sprintf("%d:%s", version, kind))
	}
	if len(versions) != 2 || versions[0] != "1:remember_literal_claim" || versions[1] != "2:correct_claim" {
		t.Fatalf("migrated operation versions = %v", versions)
	}
}

func downgradeSemanticOperationsToV1(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, semanticOperationsV1Downgrade); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
}

const semanticOperationsV1Downgrade = `
DROP TRIGGER semantic_operations_append_only_update;
DROP TRIGGER semantic_operations_append_only_delete;
CREATE TABLE semantic_operations_v1 (
    operation_id TEXT PRIMARY KEY NOT NULL,
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    operation_kind TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    actor TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    target_scope_id TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    source_event_id TEXT NOT NULL REFERENCES events(id),
    proposal_sha256 TEXT NOT NULL,
    effect_sha256 TEXT NOT NULL,
    proposal_json TEXT NOT NULL CHECK (json_valid(proposal_json) AND json_type(proposal_json) = 'object'),
    prepared_proposal_json TEXT NOT NULL CHECK (json_valid(prepared_proposal_json) AND json_type(prepared_proposal_json) = 'object'),
    result_json TEXT NOT NULL CHECK (json_valid(result_json) AND json_type(result_json) = 'object'),
    transaction_time TEXT NOT NULL
);
INSERT INTO semantic_operations_v1 SELECT * FROM semantic_operations;
DROP TABLE semantic_operations;
ALTER TABLE semantic_operations_v1 RENAME TO semantic_operations;
CREATE TRIGGER semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
CREATE TRIGGER semantic_operations_append_only_delete BEFORE DELETE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
`

func TestCorrectClaimConcurrentApplyAndCollidingTransactionTimesUseScopeRevision(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeA, storeB := NewStore(dbA), NewStore(dbB)
	collision := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	setTurnLeaseTime(storeA, collision)
	setTurnLeaseTime(storeB, collision)
	session, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := storeA.AcquireTurnLease(ctx, session.ID, "correct-race", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	old := prepareLiteralForCorrection(t, ctx, storeA, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000031", "Detroit", "Detroit", memory.ValidTime{})
	oldResult, err := storeA.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(store *Store, key, value string) memory.CorrectClaimProposal {
		t.Helper()
		source, err := storeA.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: value,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
			IdempotencyKey: key, SourceEventID: source.ID, OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
			Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
				Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}, Polarity: memory.PolarityAffirmed},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	left := prepare(storeA, "idem:v1:77000000-0000-4000-8000-000000000032", "Chicago")
	right := prepare(storeB, "idem:v1:77000000-0000-4000-8000-000000000033", "New York")
	readTx, err := dbA.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	visibleBefore, err := storeA.inspectClaimsSnapshot(ctx, readTx, session.ScopeContext(), false, memory.ClaimQuery{}, false)
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	wait.Add(2)
	errs := make([]error, 2)
	go func() { defer wait.Done(); _, errs[0] = storeA.ApplyCorrectClaim(ctx, lease, left) }()
	go func() { defer wait.Done(); _, errs[1] = storeB.ApplyCorrectClaim(ctx, lease, right) }()
	wait.Wait()
	var successes, stale int
	for _, applyErr := range errs {
		if applyErr == nil {
			successes++
		} else if errors.Is(applyErr, ErrStaleScopeRevision) {
			stale++
		} else {
			t.Fatalf("concurrent correction error = %v", applyErr)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent results = %v", errs)
	}
	diagnosticsBefore, err := storeA.inspectClaimsSnapshot(ctx, readTx, session.ScopeContext(), false, memory.ClaimQuery{
		ValidAt: &visibleBefore.ValidAt, AsKnownAt: &visibleBefore.AsKnownAt,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	visibleLiteral, diagnosticLiteral := literalClaimsFromInspection(visibleBefore), literalClaimsFromInspection(diagnosticsBefore)
	if visibleBefore.ScopeRevision != 1 || diagnosticsBefore.ScopeRevision != 1 ||
		len(visibleLiteral) != 1 || len(diagnosticLiteral) != 1 ||
		visibleLiteral[0].Literal.Value != "Detroit" || diagnosticLiteral[0].Literal.Value != "Detroit" {
		t.Fatalf("focused Literal inspection mixed a same-time correction: visible=%+v diagnostics=%+v",
			visibleBefore, diagnosticsBefore)
	}
	if err := readTx.Commit(); err != nil {
		t.Fatal(err)
	}
	inspection, err := storeA.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{AsKnownAt: &collision})
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 2 || len(inspection.Claims) != 1 || len(inspection.Claims[0].Lifecycle) != 1 ||
		inspection.Claims[0].Lifecycle[0].ScopeRevision != 2 {
		t.Fatalf("colliding Transaction Times were not ordered by Scope Revision: %+v", inspection)
	}
}

func TestInspectClaimsUsesOneSQLiteSnapshotDuringConcurrentCorrection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	reader, writer := NewStore(dbA), NewStore(dbB)
	acceptedAt := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	correctedAt := acceptedAt.Add(time.Hour)
	setTurnLeaseTime(reader, acceptedAt)
	setTurnLeaseTime(writer, correctedAt)
	session, err := reader.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := reader.AcquireTurnLease(ctx, session.ID, "inspect-snapshot", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	old := prepareLiteralForCorrection(t, ctx, reader, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000041", "Detroit", "Detroit", memory.ValidTime{})
	oldResult, err := reader.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	source, err := reader.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Correction: Chicago",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := writer.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000042", SourceEventID: source.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed},
	})
	if err != nil {
		t.Fatal(err)
	}

	readTx, err := dbA.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Rollback()
	if err := validateSessionScope(ctx, readTx, session.ScopeContext()); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ApplyCorrectClaim(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	before, err := reader.inspectClaimsSnapshot(ctx, readTx, session.ScopeContext(), false,
		memory.ClaimQuery{ValidAt: &acceptedAt, AsKnownAt: &correctedAt}, false)
	if err != nil {
		t.Fatal(err)
	}
	if before.ScopeRevision != 1 || len(before.Claims) != 1 ||
		before.Claims[0].Object.Literal == nil || before.Claims[0].Object.Literal.Value != "Detroit" {
		t.Fatalf("inspection mixed the concurrent correction into its pinned snapshot: %+v", before)
	}
	if err := readTx.Commit(); err != nil {
		t.Fatal(err)
	}
	after, err := reader.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{ValidAt: &acceptedAt, AsKnownAt: &correctedAt})
	if err != nil {
		t.Fatal(err)
	}
	if after.ScopeRevision != 2 || len(after.Claims) != 1 ||
		after.Claims[0].Object.Literal == nil || after.Claims[0].Object.Literal.Value != "Chicago" {
		t.Fatalf("fresh inspection did not observe the committed correction: %+v", after)
	}
}

func prepareLiteralForCorrection(t *testing.T, ctx context.Context, store *Store, session memory.Session,
	lease memory.TurnLease, key, content, value string, validTime memory.ValidTime,
) memory.RememberLiteralProposal {
	t.Helper()
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: key, SourceEventID: source.ID, Predicate: "home_city", PredicateLabel: "home city",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, ValidTime: validTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func assertLiteralClaimAt(t *testing.T, ctx context.Context, store *Store, session memory.Session,
	validAt, asKnownAt time.Time, want string,
) {
	t.Helper()
	inspection, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{
		ValidAt: &validAt, AsKnownAt: &asKnownAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.ValidAt.Equal(validAt) || !inspection.AsKnownAt.Equal(asKnownAt) {
		t.Fatalf("query did not echo times: %+v", inspection)
	}
	if want == "" {
		if len(inspection.Claims) != 0 {
			t.Fatalf("claims at valid=%s known=%s = %+v, want none", validAt, asKnownAt, inspection.Claims)
		}
		return
	}
	if len(inspection.Claims) != 1 || inspection.Claims[0].Object.Literal == nil || inspection.Claims[0].Object.Literal.Value != want {
		t.Fatalf("claims at valid=%s known=%s = %+v, want %q", validAt, asKnownAt, inspection.Claims, want)
	}
}
