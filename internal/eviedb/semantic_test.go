package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestRememberLiteralCanonicalHashesMatchV1Fixture(t *testing.T) {
	proposal := memory.RememberLiteralProposal{
		SchemaVersion:  1,
		Kind:           "remember_literal_claim",
		OperationID:    "60000000-0000-4000-8000-000000000001",
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000001",
		Actor:          "owner",
		SessionID:      "40000000-0000-4000-8000-000000000001",
		Scope:          memory.SemanticScope{ID: "10000000-0000-4000-8000-000000000001", Key: "global"},
		Scopes:         []memory.SemanticScope{{ID: "10000000-0000-4000-8000-000000000001", Key: "global"}},
		PriorRevisions: []memory.ScopeRevision{{ScopeKey: "global", Revision: 0}},
		Predicate: memory.SemanticPredicate{
			ID: "60000000-0000-4000-8000-000000000002", Token: "timezone_name", Version: 1,
			Label: "timezone name", ObjectConstraint: memory.LiteralText, Cardinality: "one", Create: true,
		},
		Subject: memory.SemanticEntity{
			ID: "60000000-0000-4000-8000-000000000003", ScopeKey: "global", CanonicalName: "owner",
			EntityType: "person", AnchorKind: "owner", Create: true,
		},
		ClaimID:      "60000000-0000-4000-8000-000000000004",
		SourceLinkID: "60000000-0000-4000-8000-000000000005",
		Literal:      memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
		Polarity:     "affirmed",
		Source: memory.SemanticSource{
			EventID: "50000000-0000-4000-8000-000000000001", EventPart: "content",
			LocatorKind: "utf8_byte_range", LocatorValue: "0:22",
			EvidenceSHA256: "sha256:a8e9e3c075dababbd40194c8d04a04c9fd21baf1823afeb58f8ae3fade598929",
			Actor:          "owner", SourceType: "user_message", Authority: "owner_statement",
			ObservedAt: "2026-09-01T12:00:00.000000000Z",
		},
	}
	canonical := canonicalRememberLiteralProposal(proposal)
	proposalHash, _, err := semanticHash(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if want := "sha256:29f1fa4c17966dd1fd22e119b5dcca706e65ad0d066945429579b147bf609d20"; proposalHash != want {
		t.Fatalf("proposal hash = %s, want %s", proposalHash, want)
	}
	effectHash, _, err := semanticHash(canonical.Effect)
	if err != nil {
		t.Fatal(err)
	}
	if want := "sha256:60a41b9acdb5c8e85546097d5fe9a02cd85df67505dc41df828ab321c1520561"; effectHash != want {
		t.Fatalf("effect hash = %s, want %s", effectHash, want)
	}
}

func TestRememberLiteralExpiredTurnLeaseRollsBackSemanticWrites(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-expiry", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember timezone_name Detroit",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000060", SourceEventID: source.ID,
		Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	setTurnLeaseTime(store, lease.ExpiresAt)
	if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("apply at lease expiry error = %v, want ErrTurnLeaseLost", err)
	}
	for _, table := range []string{"semantic_scopes", "semantic_operations", "semantic_predicates", "semantic_entities", "semantic_claims", "semantic_source_links", "semantic_state_events"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("expired lease left %d rows in %s", count, table)
		}
	}
}
