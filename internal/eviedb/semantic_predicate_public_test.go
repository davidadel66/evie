package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestSemanticMemoryAcceptsEveryCanonicalLiteralKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		literal memory.TypedLiteral
	}{
		{name: "text", literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: " Detroit \u00e9 "}},
		{name: "integer zero", literal: memory.TypedLiteral{Kind: memory.LiteralInteger, Value: "0"}},
		{name: "negative integer", literal: memory.TypedLiteral{Kind: memory.LiteralInteger, Value: "-9223372036854775809"}},
		{name: "decimal", literal: memory.TypedLiteral{Kind: memory.LiteralDecimal, Value: "-1234567890.000000001"}},
		{name: "boolean", literal: memory.TypedLiteral{Kind: memory.LiteralBoolean, Value: "false"}},
		{name: "leap date", literal: memory.TypedLiteral{Kind: memory.LiteralDate, Value: "2024-02-29"}},
		{name: "UTC datetime", literal: memory.TypedLiteral{Kind: memory.LiteralDatetime, Value: "2026-09-02T14:03:04.123456789Z"}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			session, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := store.AcquireTurnLease(ctx, session.ID, "typed-literal", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
				Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "remember exact literal",
			})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
				IdempotencyKey: semanticIdempotencyKey(100 + index), SourceEventID: event.ID,
				Predicate: "exact_value", PredicateLabel: "exact value", Literal: test.literal,
				PredicateCardinality: memory.CardinalityMany,
			})
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if proposal.Literal != test.literal || proposal.Predicate.ObjectConstraint != memory.PredicateObjectConstraint(test.literal.Kind) ||
				proposal.Predicate.Cardinality != memory.CardinalityMany {
				t.Fatalf("proposal = %+v", proposal)
			}
			if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); err != nil {
				t.Fatalf("apply: %v", err)
			}
			inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
			if err != nil {
				t.Fatal(err)
			}
			if len(inspection.Claims) != 1 || inspection.Claims[0].Literal != test.literal {
				t.Fatalf("inspection = %+v", inspection)
			}
		})
	}
}

func TestSemanticMemoryRejectsNoncanonicalLiteralValuesWithoutMutation(t *testing.T) {
	t.Parallel()

	tests := []memory.TypedLiteral{
		{Kind: "float", Value: "1.5"},
		{Kind: "json", Value: `{}`},
		{Kind: "money", Value: "USD 1.00"},
		{Kind: "duration", Value: "PT1H"},
		{Kind: "quantity", Value: "1 kg"},
		{Kind: memory.LiteralInteger, Value: "01"},
		{Kind: memory.LiteralInteger, Value: "+1"},
		{Kind: memory.LiteralDecimal, Value: "1.0"},
		{Kind: memory.LiteralDecimal, Value: "1e3"},
		{Kind: memory.LiteralBoolean, Value: "TRUE"},
		{Kind: memory.LiteralDate, Value: "2025-02-29"},
		{Kind: memory.LiteralDatetime, Value: "2026-09-02T10:00:00-04:00"},
		{Kind: memory.LiteralText, Value: string([]byte{0xff})},
	}
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "invalid-literal", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for index, literal := range tests {
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "reject invalid literal",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: semanticIdempotencyKey(200 + index), SourceEventID: event.ID,
			Predicate: "invalid_value", PredicateLabel: "invalid value", Literal: literal,
		})
		if err == nil {
			t.Fatalf("literal %+v was accepted", literal)
		}
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 0 || len(inspection.Claims) != 0 {
		t.Fatalf("invalid values mutated Semantic Memory: %+v", inspection)
	}
}

func TestSemanticMemoryVersionsPredicateDefinitionsAndPinsClaims(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "predicate-version", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	remember := func(key string, label string, cardinality memory.PredicateCardinality, value string) memory.RememberLiteralProposal {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "remember predicate version",
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: key, SourceEventID: event.ID, Predicate: "home_city",
			PredicateLabel: label, PredicateCardinality: cardinality,
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	first := remember(semanticIdempotencyKey(300), "home city", memory.CardinalityOne, "Detroit")
	if !first.Predicate.Create || first.Predicate.Version != 1 {
		t.Fatalf("first proposal did not visibly introduce Predicate v1: %+v", first.Predicate)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, first); err != nil {
		t.Fatal(err)
	}
	second := remember(semanticIdempotencyKey(301), "cities called home", memory.CardinalityMany, "Chicago")
	if !second.Predicate.Create || second.Predicate.Version != 2 || second.Predicate.ID == first.Predicate.ID {
		t.Fatalf("changed definition was not visible as Predicate v2: first=%+v second=%+v", first.Predicate, second.Predicate)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, second); err != nil {
		t.Fatal(err)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 2 || literalPredicateVersions(inspection)["Detroit"] != 1 ||
		literalPredicateVersions(inspection)["Chicago"] != 2 {
		t.Fatalf("Claims did not retain Predicate versions: %+v", inspection.Claims)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inspection, err = eviedb.NewStore(db).InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil || len(inspection.Claims) != 2 || literalPredicateVersions(inspection)["Detroit"] != 1 ||
		literalPredicateVersions(inspection)["Chicago"] != 2 {
		t.Fatalf("Predicate versions did not survive restart: inspection=%+v err=%v", inspection, err)
	}
}

func literalPredicateVersions(inspection memory.LiteralClaimsInspection) map[string]int64 {
	versions := make(map[string]int64, len(inspection.Claims))
	for _, claim := range inspection.Claims {
		versions[claim.Literal.Value] = claim.Predicate.Version
	}
	return versions
}

func TestSemanticMemoryPreservesPolarityAndReportsDeterministicConflicts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "conflicts", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	remember := func(index int, polarity memory.ClaimPolarity, value string) memory.RememberLiteralResult {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "remember conflicting evidence",
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: semanticIdempotencyKey(index), SourceEventID: event.ID,
			Predicate: "home_city", PredicateLabel: "home city", PredicateCardinality: memory.CardinalityOne,
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: polarity,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ApplyRememberLiteral(ctx, lease, proposal)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	affirmed := remember(400, memory.PolarityAffirmed, "Detroit")
	denied := remember(401, memory.PolarityDenied, "Detroit")
	other := remember(402, memory.PolarityAffirmed, "Chicago")
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 3 {
		t.Fatalf("conflicting Claims were not independently inspectable: %+v", inspection)
	}
	want := []memory.ClaimConflictWarning{
		{Code: memory.ConflictOppositePolarity, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(affirmed.ClaimID, denied.ClaimID)},
		{Code: memory.ConflictOneCardinality, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(affirmed.ClaimID, other.ClaimID)},
	}
	if !equalConflictWarnings(inspection.Warnings, want) {
		t.Fatalf("warnings = %+v, want %+v", inspection.Warnings, want)
	}
	for _, claim := range inspection.Claims {
		if len(claim.Sources) != 1 || len(claim.Lifecycle) != 1 || claim.Lifecycle[0].State != memory.SemanticStateActive {
			t.Fatalf("conflicting Claim omitted source or lifecycle: %+v", claim)
		}
	}
}

func sortedSemanticIDs(left, right memory.SemanticID) []memory.SemanticID {
	if right < left {
		left, right = right, left
	}
	return []memory.SemanticID{left, right}
}

func TestSemanticMemoryRejectsTamperedPredicatePolarityAndValidityWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "tamper", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "remember exact semantics",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: semanticIdempotencyKey(500), SourceEventID: event.ID, Predicate: "home_city",
		PredicateLabel: "home city", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"}, Polarity: memory.PolarityDenied,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*memory.RememberLiteralProposal)
	}{
		{name: "cardinality", mutate: func(p *memory.RememberLiteralProposal) { p.Predicate.Cardinality = memory.CardinalityOne }},
		{name: "polarity", mutate: func(p *memory.RememberLiteralProposal) { p.Polarity = memory.PolarityAffirmed }},
		{name: "coordinated request and proposal polarity", mutate: func(p *memory.RememberLiteralProposal) {
			p.Polarity = memory.PolarityAffirmed
			p.Request.Polarity = memory.PolarityAffirmed
		}},
		{name: "predicate version", mutate: func(p *memory.RememberLiteralProposal) { p.Predicate.Version++ }},
		{name: "invalid interval", mutate: func(p *memory.RememberLiteralProposal) {
			from := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
			to := from.Add(-time.Hour)
			p.ValidTime = memory.ValidTime{From: &from, To: &to}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attempt := proposal
			attempt.ProposalSHA256 = ""
			attempt.PreparedSHA256 = ""
			test.mutate(&attempt)
			if _, err := store.ApplyRememberLiteral(ctx, lease, attempt); err == nil || errors.Is(err, eviedb.ErrStaleScopeRevision) {
				t.Fatalf("tampered proposal error = %v", err)
			}
		})
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 0 || len(inspection.Claims) != 0 {
		t.Fatalf("tampered proposals mutated memory: %+v", inspection)
	}
}

func TestSemanticMemoryEntityObjectUsesApprovedPredicatePolarityAndCardinality(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "entity-semantics", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice does not mentor Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: semanticIdempotencyKey(600), SourceEventID: event.ID,
		Predicate: "mentors", PredicateLabel: "mentors", PredicateCardinality: memory.CardinalityOne,
		Polarity: memory.PolarityDenied,
		Subject:  memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:   memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Predicate.ObjectConstraint != memory.ConstraintEntity || proposal.Predicate.Cardinality != memory.CardinalityOne ||
		proposal.Claim.Polarity != memory.PolarityDenied {
		t.Fatalf("proposal = %+v", proposal)
	}
	denied, err := store.ApplyRememberEntity(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	remember := func(index int, object memory.EntitySelector) memory.RememberEntityResult {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice mentors another person",
		})
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
			IdempotencyKey: semanticIdempotencyKey(index), SourceEventID: event.ID,
			Predicate: "mentors", PredicateLabel: "mentors", PredicateCardinality: memory.CardinalityOne,
			Polarity: memory.PolarityAffirmed, Subject: memory.EntitySelector{EntityID: proposal.Claim.SubjectEntityID}, Object: object,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ApplyRememberEntity(ctx, lease, candidate)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	affirmed := remember(601, memory.EntitySelector{EntityID: proposal.Claim.ObjectEntityID})
	other := remember(602, memory.EntitySelector{Create: true, CanonicalName: "Carol", EntityType: "person", Alias: "Carol"})
	inspection, err := store.InspectEntityClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 3 {
		t.Fatalf("inspection = %+v", inspection)
	}
	wantWarnings := []memory.ClaimConflictWarning{
		{Code: memory.ConflictOppositePolarity, PredicateToken: "mentors", ClaimIDs: sortedSemanticIDs(denied.ClaimID, affirmed.ClaimID)},
		{Code: memory.ConflictOneCardinality, PredicateToken: "mentors", ClaimIDs: sortedSemanticIDs(affirmed.ClaimID, other.ClaimID)},
	}
	if !equalConflictWarnings(inspection.Warnings, wantWarnings) {
		t.Fatalf("Entity warnings = %+v, want %+v", inspection.Warnings, wantWarnings)
	}
	for _, claim := range inspection.Claims {
		if claim.Predicate.Cardinality != memory.CardinalityOne || len(claim.Lifecycle) != 1 || len(claim.Sources) != 1 {
			t.Fatalf("Entity conflict lost its definition, lifecycle, or source: %+v", claim)
		}
	}
}

func TestSemanticMemoryNormalizesValidTimeAndKeepsAcceptedStateModelIndependent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "valid-time", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Detroit is home in this interval",
	})
	if err != nil {
		t.Fatal(err)
	}
	detroit := time.FixedZone("Detroit", -4*60*60)
	from := time.Date(2020, 1, 1, 0, 0, 0, 123, detroit)
	to := time.Date(2030, 1, 1, 0, 0, 0, 456, detroit)
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: semanticIdempotencyKey(700), SourceEventID: event.ID,
		Predicate: "home_city", PredicateLabel: "home city",
		Literal:   memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
		ValidTime: memory.ValidTime{From: &from, To: &to},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ValidTime.From.Location() != time.UTC || proposal.ValidTime.To.Location() != time.UTC ||
		proposal.ValidTime.From.Format(time.RFC3339Nano) != "2020-01-01T04:00:00.000000123Z" ||
		proposal.ValidTime.To.Format(time.RFC3339Nano) != "2030-01-01T04:00:00.000000456Z" {
		t.Fatalf("proposal Valid Time was not canonical UTC: %+v", proposal.ValidTime)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 1 || !validTimePointersEqual(inspection.Claims[0].ValidTime, proposal.ValidTime) {
		t.Fatalf("inspection = %+v", inspection)
	}
	encoded, err := json.Marshal(inspection)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"confidence", "model_version", "prompt_version", "extractor_version", "entity_summary"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("accepted semantic state contains forbidden model metadata %q: %s", forbidden, encoded)
		}
	}
}

func TestSemanticMemoryDiagnosesBoundedConflictsOutsideCurrentValidTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "bounded-conflicts", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	bound := func(year, month, day int) *time.Time {
		value := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		return &value
	}
	remember := func(index int, polarity memory.ClaimPolarity, value string, validTime memory.ValidTime) memory.RememberLiteralResult {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "remember bounded conflict",
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: semanticIdempotencyKey(index), SourceEventID: event.ID,
			Predicate: "home_city", PredicateLabel: "home city", PredicateCardinality: memory.CardinalityOne,
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: polarity, ValidTime: validTime,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ApplyRememberLiteral(ctx, lease, proposal)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	affirmed := remember(800, memory.PolarityAffirmed, "Detroit", memory.ValidTime{From: bound(2100, 1, 1), To: bound(2110, 1, 1)})
	denied := remember(801, memory.PolarityDenied, "Detroit", memory.ValidTime{From: bound(2105, 1, 1), To: bound(2106, 1, 1)})
	other := remember(802, memory.PolarityAffirmed, "Chicago", memory.ValidTime{From: bound(2102, 1, 1), To: bound(2103, 1, 1)})
	remember(803, memory.PolarityAffirmed, "Toledo", memory.ValidTime{From: bound(2110, 1, 1), To: bound(2120, 1, 1)})
	pastAffirmed := remember(804, memory.PolarityAffirmed, "Ann Arbor", memory.ValidTime{From: bound(2000, 1, 1), To: bound(2010, 1, 1)})
	pastDenied := remember(805, memory.PolarityDenied, "Ann Arbor", memory.ValidTime{From: bound(2005, 1, 1), To: bound(2006, 1, 1)})

	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Claims) != 0 {
		t.Fatalf("future Claims appeared in the current view: %+v", inspection.Claims)
	}
	want := []memory.ClaimConflictWarning{
		{Code: memory.ConflictOppositePolarity, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(pastAffirmed.ClaimID, pastDenied.ClaimID)},
		{Code: memory.ConflictOppositePolarity, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(affirmed.ClaimID, denied.ClaimID)},
		{Code: memory.ConflictOneCardinality, PredicateToken: "home_city", ClaimIDs: sortedSemanticIDs(affirmed.ClaimID, other.ClaimID)},
	}
	if !equalConflictWarnings(inspection.Warnings, want) || len(inspection.ConflictClaims) != 5 {
		t.Fatalf("bounded conflict inspection = %+v, want warnings %+v and 5 complete conflict Claims", inspection, want)
	}
	for _, claim := range inspection.ConflictClaims {
		if len(claim.Sources) != 1 || len(claim.Lifecycle) != 1 {
			t.Fatalf("bounded conflict omitted sources or lifecycle: %+v", claim)
		}
	}
}

func validTimePointersEqual(left, right memory.ValidTime) bool {
	return left.From != nil && right.From != nil && left.From.Equal(*right.From) &&
		left.To != nil && right.To != nil && left.To.Equal(*right.To)
}

func semanticIdempotencyKey(index int) string {
	return "idem:v1:70000000-0000-4000-8000-" + leftPad12(index)
}

func leftPad12(value int) string {
	const digits = "000000000000"
	text := []byte(digits)
	for index := len(text) - 1; value > 0; index-- {
		text[index] = byte('0' + value%10)
		value /= 10
	}
	return string(text)
}

func equalConflictWarnings(got, want []memory.ClaimConflictWarning) bool {
	if len(got) != len(want) {
		return false
	}
	got = append([]memory.ClaimConflictWarning(nil), got...)
	want = append([]memory.ClaimConflictWarning(nil), want...)
	less := func(values []memory.ClaimConflictWarning, i, j int) bool {
		left, right := values[i], values[j]
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		if left.PredicateToken != right.PredicateToken {
			return left.PredicateToken < right.PredicateToken
		}
		if left.ClaimIDs[0] != right.ClaimIDs[0] {
			return left.ClaimIDs[0] < right.ClaimIDs[0]
		}
		return left.ClaimIDs[1] < right.ClaimIDs[1]
	}
	sort.Slice(got, func(i, j int) bool { return less(got, i, j) })
	sort.Slice(want, func(i, j int) bool { return less(want, i, j) })
	for index := range got {
		if got[index].Code != want[index].Code || got[index].PredicateToken != want[index].PredicateToken ||
			len(got[index].ClaimIDs) != len(want[index].ClaimIDs) {
			return false
		}
		for claimIndex := range got[index].ClaimIDs {
			if got[index].ClaimIDs[claimIndex] != want[index].ClaimIDs[claimIndex] {
				return false
			}
		}
	}
	return true
}
