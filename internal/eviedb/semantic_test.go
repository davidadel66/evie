package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
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
			Label: "timezone name", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText), Cardinality: "one", Create: true,
		},
		Subject: memory.SemanticEntity{
			ID: "60000000-0000-4000-8000-000000000003", ScopeKey: "global", CanonicalName: "owner",
			EntityType: "person", AnchorKind: "owner", Create: true,
		},
		ClaimID:      "60000000-0000-4000-8000-000000000004",
		ClaimCreate:  true,
		SourceLinkID: "60000000-0000-4000-8000-000000000005",
		Literal:      memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
		Polarity:     "affirmed",
		Source: memory.SemanticSource{
			EventID: "50000000-0000-4000-8000-000000000001", EventPart: "content",
			LocatorKind: "utf8_byte_range", LocatorValue: "0:22",
			EvidenceSHA256: "sha256:a8e9e3c075dababbd40194c8d04a04c9fd21baf1823afeb58f8ae3fade598929",
			Actor:          "owner", SourceType: "user_message", Authority: "owner_statement",
			ObservedAt: "2026-09-01T12:00:00.000000000Z",
			Create:     true,
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
	tamperedSource := proposal
	tamperedSource.Source.OperationID = "60000000-0000-4000-8000-000000000099"
	if _, err := store.ApplyRememberLiteral(ctx, lease, tamperedSource); err == nil {
		t.Fatal("literal proposal with fabricated Source creating operation unexpectedly applied")
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

func TestRememberEntityCompoundSQLFailureRollsBackEverySemanticWrite(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Compound rollback")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{ID: "42000000-0000-4000-8000-000000000099", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-compound-rollback", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice mentors Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000001", SourceEventID: source.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignWorkspace, err := store.RegisterWorkspace(ctx, "Foreign")
	if err != nil {
		t.Fatal(err)
	}
	foreignScopeID, err := newSemanticID()
	if err != nil {
		t.Fatal(err)
	}
	foreignKey := "workspace:" + string(foreignWorkspace.ID)
	crossScope := proposal
	crossScope.Scopes = append(append([]memory.SemanticScope(nil), proposal.Scopes...), memory.SemanticScope{
		ID: foreignScopeID, Key: foreignKey, RegistryID: string(foreignWorkspace.ID),
	})
	crossScope.PriorRevisions = append(append([]memory.ScopeRevision(nil), proposal.PriorRevisions...), memory.ScopeRevision{
		ScopeKey: foreignKey, Revision: 0,
	})
	crossScope.ResultingRevisions = append(append([]memory.ScopeRevision(nil), proposal.ResultingRevisions...), memory.ScopeRevision{
		ScopeKey: foreignKey, Revision: 0,
	})
	crossScope.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(crossScope))
	if err != nil {
		t.Fatal(err)
	}
	crossScope.PreparedSHA256, _, err = semanticHash(crossScope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, crossScope); err == nil {
		t.Fatal("cross-scope Entity proposal unexpectedly applied")
	}
	globalWrite := proposal
	globalWrite.Entities = append([]memory.SemanticEntity(nil), proposal.Entities...)
	for index := range globalWrite.Entities {
		if globalWrite.Entities[index].AnchorKind == "" {
			globalWrite.Entities[index].ScopeKey = "global"
		}
	}
	globalWrite.Aliases = append([]memory.SemanticAlias(nil), proposal.Aliases...)
	for index := range globalWrite.Aliases {
		globalWrite.Aliases[index].ScopeKey = "global"
	}
	globalWrite.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(globalWrite))
	if err != nil {
		t.Fatal(err)
	}
	globalWrite.PreparedSHA256, _, err = semanticHash(globalWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, globalWrite); err == nil {
		t.Fatal("scoped proposal widened ordinary Entity and Alias creation to global")
	}
	invalidUTF8 := string([]byte{0xff})
	for name, mutate := range map[string]func(*memory.RememberEntityProposal){
		"Predicate label": func(candidate *memory.RememberEntityProposal) { candidate.Predicate.Label = invalidUTF8 },
		"Entity name": func(candidate *memory.RememberEntityProposal) {
			candidate.Entities[2].CanonicalName = invalidUTF8
		},
		"Entity type": func(candidate *memory.RememberEntityProposal) { candidate.Entities[2].EntityType = invalidUTF8 },
		"Alias": func(candidate *memory.RememberEntityProposal) {
			candidate.Aliases[0].Value = invalidUTF8
			candidate.Aliases[0].NormalizedValue = invalidUTF8
		},
		"Source authority": func(candidate *memory.RememberEntityProposal) {
			candidate.Source.Actor = "fabricated"
			candidate.Source.Authority = "fabricated"
		},
		"Predicate cardinality": func(candidate *memory.RememberEntityProposal) {
			candidate.Predicate.Cardinality = memory.CardinalityOne
		},
		"blank Entity name": func(candidate *memory.RememberEntityProposal) {
			for index := range candidate.Entities {
				if candidate.Entities[index].AnchorKind == "" {
					candidate.Entities[index].CanonicalName = "  "
					break
				}
			}
		},
		"blank Alias": func(candidate *memory.RememberEntityProposal) {
			candidate.Aliases[0].Value = "  "
			candidate.Aliases[0].NormalizedValue = ""
		},
		"mismatched idempotency key": func(candidate *memory.RememberEntityProposal) {
			candidate.IdempotencyKey = "idem:v1:72000000-0000-4000-8000-000000000099"
		},
	} {
		t.Run("rehashed "+name, func(t *testing.T) {
			candidate := proposal
			candidate.Entities = append([]memory.SemanticEntity(nil), proposal.Entities...)
			candidate.Aliases = append([]memory.SemanticAlias(nil), proposal.Aliases...)
			mutate(&candidate)
			candidate.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(candidate))
			if err != nil {
				t.Fatal(err)
			}
			candidate.PreparedSHA256, _, err = semanticHash(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ApplyRememberEntity(ctx, lease, candidate); err == nil {
				t.Fatalf("proposal with rehashed %s unexpectedly applied", name)
			}
		})
	}
	proposal.Aliases[1].ID = proposal.Aliases[0].ID
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, proposal); err == nil {
		t.Fatal("duplicate Alias ID unexpectedly committed")
	}
	for _, table := range []string{
		"semantic_scopes", "semantic_operations", "semantic_operation_scopes", "semantic_predicates",
		"semantic_entities", "semantic_aliases", "semantic_claims", "semantic_source_links", "semantic_state_events",
	} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("failed compound operation left %d rows in %s", count, table)
		}
	}
}

func TestRememberEntityRejectsSiblingWorkspaceEntityAfterRehash(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	createWorkspaceSession := func(id memory.SessionID, name string) memory.Session {
		t.Helper()
		workspace, err := store.RegisterWorkspace(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		session := memory.Session{ID: id, WorkspaceID: workspace.ID,
			WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
			VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
		`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
			t.Fatal(err)
		}
		return session
	}
	prepare := func(session memory.Session, lease memory.TurnLease, key, subject, object string) memory.RememberEntityProposal {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: subject + " mentors " + object,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: "mentors", PredicateLabel: "mentors",
			Subject: memory.EntitySelector{Create: true, CanonicalName: subject, EntityType: "person", Alias: subject},
			Object:  memory.EntitySelector{Create: true, CanonicalName: object, EntityType: "person", Alias: object},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}

	sibling := createWorkspaceSession("42000000-0000-4000-8000-000000000081", "Sibling")
	siblingLease, err := store.AcquireTurnLease(ctx, sibling.ID, "sibling", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	siblingProposal := prepare(sibling, siblingLease, "idem:v1:72000000-0000-4000-8000-000000000081", "Bea", "Bo")
	if _, err := store.ApplyRememberEntity(ctx, siblingLease, siblingProposal); err != nil {
		t.Fatal(err)
	}
	reusedSource, err := store.PrepareRememberEntity(ctx, sibling.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000084",
		SourceEventID:  siblingProposal.Source.EventID, Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{EntityID: siblingProposal.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: siblingProposal.Claim.ObjectEntityID},
	})
	if err != nil || reusedSource.Source.Create || reusedSource.Source.OperationID != siblingProposal.OperationID {
		t.Fatalf("prepare reused Source Link = %+v, error = %v", reusedSource.Source, err)
	}
	reusedSource.Source.OperationID = reusedSource.OperationID
	reusedSource.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(reusedSource))
	if err != nil {
		t.Fatal(err)
	}
	reusedSource.PreparedSHA256, _, err = semanticHash(reusedSource)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, siblingLease, reusedSource); err == nil {
		t.Fatal("rehashed proposal fabricated reused Source Link provenance")
	}
	reusedSource.Source.OperationID = siblingProposal.OperationID
	reusedSource.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(reusedSource))
	if err != nil {
		t.Fatal(err)
	}
	reusedSource.PreparedSHA256, _, err = semanticHash(reusedSource)
	if err != nil {
		t.Fatal(err)
	}
	setSourceEligibility := func(eligibility string) {
		t.Helper()
		if err := withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
			if _, err := conn.ExecContext(ctx, `DROP TRIGGER semantic_source_links_append_only_update`); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `UPDATE semantic_source_links SET eligibility = ? WHERE source_link_id = ?`, eligibility, reusedSource.Source.ID); err != nil {
				return err
			}
			_, err := conn.ExecContext(ctx, `
				CREATE TRIGGER semantic_source_links_append_only_update BEFORE UPDATE ON semantic_source_links
				BEGIN SELECT RAISE(ABORT, 'semantic source links are append-only'); END;
			`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	setSourceEligibility("retracted")
	if _, err := store.ApplyRememberEntity(ctx, siblingLease, reusedSource); err == nil {
		t.Fatal("proposal reused a retracted Source Link")
	}
	setSourceEligibility("eligible")
	aliasSource, err := store.AppendEventWithLease(ctx, sibling.ID, siblingLease.HolderID, siblingLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Bea still mentors Bo",
	})
	if err != nil {
		t.Fatal(err)
	}
	reusedAlias, err := store.PrepareRememberEntity(ctx, sibling.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000083", SourceEventID: aliasSource.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Alias: "Bea"},
		Object:  memory.EntitySelector{EntityID: siblingProposal.Claim.ObjectEntityID},
	})
	if err != nil || len(reusedAlias.Aliases) != 1 || reusedAlias.Aliases[0].Create {
		t.Fatalf("prepare reused Alias = %+v, error = %v", reusedAlias.Aliases, err)
	}
	reusedAlias.Aliases[0].Value = " BEA "
	reusedAlias.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(reusedAlias))
	if err != nil {
		t.Fatal(err)
	}
	reusedAlias.PreparedSHA256, _, err = semanticHash(reusedAlias)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, siblingLease, reusedAlias); err == nil {
		t.Fatal("rehashed proposal fabricated reused Alias text/provenance")
	}
	if err := store.ReleaseTurnLease(ctx, siblingLease.SessionID, siblingLease.HolderID, siblingLease.FencingToken); err != nil {
		t.Fatal(err)
	}

	target := createWorkspaceSession("42000000-0000-4000-8000-000000000082", "Target")
	targetLease, err := store.AcquireTurnLease(ctx, target.ID, "target", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	targetProposal := prepare(target, targetLease, "idem:v1:72000000-0000-4000-8000-000000000082", "Ada", "Al")
	targetProposal.Claim.ObjectEntityID = siblingProposal.Claim.ObjectEntityID
	targetProposal.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(targetProposal))
	if err != nil {
		t.Fatal(err)
	}
	targetProposal.PreparedSHA256, _, err = semanticHash(targetProposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, targetLease, targetProposal); err == nil {
		t.Fatal("rehashed Claim referencing a sibling Workspace Entity unexpectedly applied")
	}
	for table, want := range map[string]int{"semantic_operations": 1, "semantic_claims": 1, "semantic_aliases": 2} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count after sibling-scope rejection = %d, want %d, error = %v", table, count, want, err)
		}
	}
	var targetScopeCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_scopes WHERE scope_key = ?`, "workspace:"+string(target.WorkspaceID)).Scan(&targetScopeCount); err != nil || targetScopeCount != 0 {
		t.Fatalf("target scope writes after sibling-scope rejection = %d, error = %v", targetScopeCount, err)
	}
}

func TestSemanticSchemaUpgradesLiteralClaimsFromIssue104AndAcceptsEntityClaims(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-upgrade", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "My timezone is Detroit",
	})
	if err != nil {
		t.Fatal(err)
	}
	literal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000010", SourceEventID: source.ID,
		Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	literalResult, err := store.ApplyRememberLiteral(ctx, lease, literal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	downgradeSemanticClaimsToIssue104(t, ctx, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("reopen and migrate issue #104 database: %v", err)
	}
	defer db.Close()
	store = NewStore(db)
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil || len(inspection.Claims) != 1 || inspection.Claims[0].ID != literal.ClaimID {
		t.Fatalf("migrated literal inspection = %+v, error = %v", inspection, err)
	}
	var predicateScope, sourceScope, claimScope string
	if err := db.QueryRowContext(ctx, `
		SELECT predicate_scopes.scope_key, source_links.scope_id, claims.scope_id
		FROM semantic_predicates AS predicates
		JOIN semantic_scopes AS predicate_scopes ON predicate_scopes.scope_id = predicates.scope_id
		JOIN semantic_claims AS claims ON claims.predicate_id = predicates.predicate_id
		JOIN semantic_source_links AS source_links ON source_links.claim_id = claims.claim_id
		WHERE claims.claim_id = ?
	`, literal.ClaimID).Scan(&predicateScope, &sourceScope, &claimScope); err != nil {
		t.Fatal(err)
	}
	if predicateScope != "global" || sourceScope != claimScope {
		t.Fatalf("migrated object scopes: Predicate=%q Source Link=%q Claim=%q", predicateScope, sourceScope, claimScope)
	}
	lease, err = store.AcquireTurnLease(ctx, session.ID, "semantic-upgraded", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	legacyRetry, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000010", SourceEventID: source.ID,
		Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatalf("prepare genuinely legacy-format literal retry: %v", err)
	}
	legacyResult, err := store.ApplyRememberLiteral(ctx, lease, legacyRetry)
	if err != nil {
		t.Fatalf("apply genuinely legacy-format literal retry: %v", err)
	}
	if legacyResult.OperationID != literalResult.OperationID || legacyResult.ClaimID != literalResult.ClaimID ||
		legacyResult.SourceLinkID != literalResult.SourceLinkID || legacyResult.ScopeRevision != literalResult.ScopeRevision {
		t.Fatalf("legacy retry result = %+v, original = %+v", legacyResult, literalResult)
	}
	var operationCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations`).Scan(&operationCount); err != nil || operationCount != 1 {
		t.Fatalf("legacy retry operation count = %d, error = %v", operationCount, err)
	}
	entitySource, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice mentors Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	entityProposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:72000000-0000-4000-8000-000000000011", SourceEventID: entitySource.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, entityProposal); err != nil {
		t.Fatalf("apply Entity Claim after migration: %v", err)
	}
}

func TestSemanticObjectScopeMigrationSupportsConcurrentLegacyOpens(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "concurrent-scope-migration", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Remember the legacy migration value",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8004-000000000198", SourceEventID: event.ID,
		Predicate: "legacy_scope", PredicateLabel: "legacy scope",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	downgradeSemanticClaimsToIssue104(t, ctx, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	dbs := make([]*sql.DB, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for index := range dbs {
		go func() {
			defer wait.Done()
			<-start
			dbs[index], errs[index] = OpenDBAt(path)
		}()
	}
	close(start)
	wait.Wait()
	for index, openErr := range errs {
		if openErr != nil {
			t.Fatalf("concurrent legacy open[%d]: %v", index, openErr)
		}
		defer dbs[index].Close()
	}
	var invalidScopes int
	if err := dbs[0].QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM semantic_predicates AS predicates
		        JOIN semantic_scopes AS scopes ON scopes.scope_id = predicates.scope_id
		        WHERE scopes.scope_key != 'global') +
		       (SELECT COUNT(*) FROM semantic_source_links AS links
		        JOIN semantic_claims AS claims ON claims.claim_id = links.claim_id
		        WHERE links.scope_id != claims.scope_id)
	`).Scan(&invalidScopes); err != nil {
		t.Fatal(err)
	}
	if invalidScopes != 0 {
		t.Fatalf("concurrent migration left %d invalid canonical object scopes", invalidScopes)
	}
}

func downgradeSemanticClaimsToIssue104(t *testing.T, ctx context.Context, db *sql.DB) {
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
	if _, err := conn.ExecContext(ctx, semanticClaimsIssue104Downgrade); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		t.Fatalf("build issue #104 schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
}

const semanticClaimsIssue104Downgrade = `
DROP TRIGGER semantic_operations_append_only_update;
UPDATE semantic_operations
SET prepared_proposal_json = json_remove(prepared_proposal_json, '$.claim_create', '$.source.create');
CREATE TRIGGER semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;

DROP TRIGGER semantic_predicates_append_only_update;
DROP TRIGGER semantic_predicates_append_only_delete;
DROP TRIGGER semantic_predicates_global_scope_insert;
CREATE TABLE semantic_predicates_v0 (
    predicate_id TEXT PRIMARY KEY NOT NULL,
    token TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    label TEXT NOT NULL,
    object_constraint TEXT NOT NULL CHECK (object_constraint IN ('entity', 'text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
    cardinality TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
    created_operation_id TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    UNIQUE (token, version)
);
INSERT INTO semantic_predicates_v0 (
    predicate_id, token, version, label, object_constraint, cardinality, created_operation_id
)
SELECT predicate_id, token, version, label, object_constraint, cardinality, created_operation_id
FROM semantic_predicates;
DROP TABLE semantic_predicates;
ALTER TABLE semantic_predicates_v0 RENAME TO semantic_predicates;

DROP TRIGGER semantic_source_links_append_only_update;
DROP TRIGGER semantic_source_links_append_only_delete;
DROP TRIGGER semantic_claims_append_only_update;
DROP TRIGGER semantic_claims_append_only_delete;
CREATE TABLE semantic_claims_v0 (
    claim_id TEXT PRIMARY KEY NOT NULL,
    scope_id TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    subject_entity_id TEXT NOT NULL REFERENCES semantic_entities(entity_id),
    predicate_id TEXT NOT NULL REFERENCES semantic_predicates(predicate_id),
    predicate_token TEXT NOT NULL,
    predicate_version INTEGER NOT NULL CHECK (predicate_version > 0),
    literal_kind TEXT NOT NULL CHECK (literal_kind IN ('text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
    literal_value TEXT NOT NULL,
    polarity TEXT NOT NULL CHECK (polarity IN ('affirmed', 'denied')),
    valid_from TEXT,
    valid_to TEXT,
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired', 'superseded')),
    created_operation_id TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    transaction_time TEXT NOT NULL,
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to)
);
INSERT INTO semantic_claims_v0 (
    claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
    literal_kind, literal_value, polarity, valid_from, valid_to, lifecycle, created_operation_id, transaction_time
)
SELECT claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
       literal_kind, literal_value, polarity, valid_from, valid_to, lifecycle, created_operation_id, transaction_time
FROM semantic_claims;
CREATE TABLE semantic_source_links_v0 (
    source_link_id TEXT PRIMARY KEY NOT NULL,
    claim_id TEXT NOT NULL REFERENCES semantic_claims_v0(claim_id),
    event_id TEXT NOT NULL REFERENCES events(id),
    source_session_id TEXT NOT NULL REFERENCES sessions(id),
    source_scope_key TEXT NOT NULL,
    event_part TEXT NOT NULL CHECK (event_part IN ('content', 'payload')),
    locator_kind TEXT NOT NULL CHECK (locator_kind IN ('whole', 'utf8_byte_range', 'json_pointer')),
    locator_value TEXT NOT NULL,
    evidence_sha256 TEXT NOT NULL,
    source_actor TEXT NOT NULL,
    source_type TEXT NOT NULL,
    authority TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    eligibility TEXT NOT NULL CHECK (eligibility IN ('eligible', 'retracted')),
    created_operation_id TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    UNIQUE (claim_id, event_id, event_part, locator_kind, locator_value, evidence_sha256)
);
INSERT INTO semantic_source_links_v0 (
    source_link_id, claim_id, event_id, source_session_id, source_scope_key, event_part,
    locator_kind, locator_value, evidence_sha256, source_actor, source_type, authority,
    observed_at, eligibility, created_operation_id
)
SELECT source_link_id, claim_id, event_id, source_session_id, source_scope_key, event_part,
       locator_kind, locator_value, evidence_sha256, source_actor, source_type, authority,
       observed_at, eligibility, created_operation_id
FROM semantic_source_links;
DROP TABLE semantic_source_links;
DROP TABLE semantic_claims;
ALTER TABLE semantic_claims_v0 RENAME TO semantic_claims;
ALTER TABLE semantic_source_links_v0 RENAME TO semantic_source_links;
`
