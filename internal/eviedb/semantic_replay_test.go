package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSemanticProjectionReplayCoversFrozenV1ThroughV5Operations(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	clock := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, clock)
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, globalSession.ID, "replay-v1-v3", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oldProposal := prepareLiteralForCorrection(t, ctx, store, globalSession, lease,
		"idem:v1:77000000-0000-4000-8000-000000000110", "Detroit was wrong", "Detroit", memory.ValidTime{})
	oldResult, err := store.ApplyRememberLiteral(ctx, lease, oldProposal)
	if err != nil {
		t.Fatal(err)
	}
	correctionEvent := appendLifecycleEvent(t, ctx, store, globalSession, lease, "Correct it to Chicago")
	correction, err := store.PrepareCorrectClaim(ctx, globalSession.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:77000000-0000-4000-8000-000000000111", SourceEventID: correctionEvent.ID,
		OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: oldProposal.Subject.ID, PredicateID: oldProposal.Predicate.ID,
			Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}}, Polarity: memory.PolarityAffirmed},
	})
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := store.ApplyCorrectClaim(ctx, lease, correction)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleEvent := appendLifecycleEvent(t, ctx, store, globalSession, lease, "Retract corrected evidence")
	lifecycle, err := store.PrepareMemoryLifecycle(ctx, globalSession.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000112", SourceEventID: lifecycleEvent.ID,
		Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: corrected.SourceLinkID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	firstClaim := rememberScopeClaim(t, ctx, store, globalSession, false, 911)
	secondClaim := rememberScopeClaim(t, ctx, store, globalSession, false, 912)
	graphLease, err := store.AcquireTurnLease(ctx, globalSession.ID, "replay-v5", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	graphResult := prepareAndApplyGraphLink(t, ctx, store, globalSession, graphLease,
		"idem:v1:96000000-0000-4000-8000-000000000110", "Record contradiction", memory.GraphRelationContradiction,
		memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: firstClaim.Claim.ID},
		memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: secondClaim.Claim.ID})
	if graphResult.GraphLinkID == "" {
		t.Fatal("Graph Link v5 operation was not accepted")
	}
	if err := store.ReleaseTurnLease(ctx, graphLease.SessionID, graphLease.HolderID, graphLease.FencingToken); err != nil {
		t.Fatal(err)
	}

	workspace, err := store.RegisterWorkspace(ctx, "Replay promotion")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	source := rememberScopeClaim(t, ctx, store, workspaceSession, false, 913)
	promotionLease, err := store.AcquireTurnLease(ctx, workspaceSession.ID, "replay-v4", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	promotionEvent := appendLifecycleEvent(t, ctx, store, workspaceSession, promotionLease, "Promote exact claim")
	promotion, err := store.PreparePromotion(ctx, workspaceSession.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8001-000000000110", SourceEventID: promotionEvent.ID,
		SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, promotionLease, promotion, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, promotionLease, promotion); err != nil {
		t.Fatal(err)
	}

	liveClaims, err := store.InspectClaims(ctx, globalSession.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := store.buildSemanticProjectionShadow(ctx)
	if err != nil {
		t.Fatalf("replay v1-v5 stream: %v", err)
	}
	defer shadow.close()
	shadowStore := NewStore(shadow.db)
	setTurnLeaseTime(shadowStore, clock)
	shadowClaims, err := shadowStore.InspectClaims(ctx, globalSession.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if !semanticTestJSONEqual(t, liveClaims, shadowClaims) {
		t.Fatalf("replay exact reads differ: claims live=%+v shadow=%+v", liveClaims, shadowClaims)
	}
	verification := compareSemanticProjectionSnapshots(shadow.live, shadow.shadow)
	if !verification.Valid {
		t.Fatalf("v1-v5 canonical replay = %+v", verification)
	}
}

func semanticTestJSONEqual(t *testing.T, left, right any) bool {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	equal, err := semanticJSONEqual(string(leftJSON), string(rightJSON))
	if err != nil {
		t.Fatal(err)
	}
	return equal
}

func TestSemanticProjectionReplayPreservesExactGraphPaths(t *testing.T) {
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
	first := rememberScopeClaim(t, ctx, store, session, false, 921)
	second := rememberScopeClaim(t, ctx, store, session, false, 922)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "path-replay", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	graph := prepareAndApplyGraphLink(t, ctx, store, session, lease,
		"idem:v1:96000000-0000-4000-8000-000000000120", "Record exact derivation", memory.GraphRelationDerivation,
		memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID},
		memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID})
	query := memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 1,
		ClaimQuery: memory.ClaimQuery{ValidAt: &graph.TransactionTime, AsKnownAt: &graph.TransactionTime}}
	live, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), query)
	if err != nil {
		t.Fatal(err)
	}
	shadow, err := store.buildSemanticProjectionShadow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer shadow.close()
	shadowStore := NewStore(shadow.db)
	replayed, err := shadowStore.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), query)
	if err != nil {
		t.Fatal(err)
	}
	if !semanticTestJSONEqual(t, live, replayed) || len(live.Paths) != 1 {
		t.Fatalf("exact path replay live=%+v shadow=%+v", live, replayed)
	}
}

func TestSemanticProjectionReplayFailsClosedAtExactMalformedOrUnknownOperation(t *testing.T) {
	tests := []struct {
		name        string
		before      string
		update      string
		after       string
		wantVersion int
		args        func(memory.SemanticID) []any
	}{
		{name: "unknown schema version", before: `PRAGMA ignore_check_constraints = ON`, update: `UPDATE semantic_operations SET schema_version = 99 WHERE operation_id = ?`, after: `PRAGMA ignore_check_constraints = OFF`, wantVersion: 99, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "malformed noninteger schema version", before: `PRAGMA ignore_check_constraints = ON`, update: `UPDATE semantic_operations SET schema_version = 'future' WHERE operation_id = ?`, after: `PRAGMA ignore_check_constraints = OFF`, wantVersion: 0, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "unknown kind", update: `UPDATE semantic_operations SET operation_kind = 'future_operation' WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "malformed prepared proposal", update: `UPDATE semantic_operations SET prepared_proposal_json = '{"schema_version":1,"unknown":true}' WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "accepted envelope differs", update: `UPDATE semantic_operations SET actor = 'model' WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "proposal JSON differs", update: `UPDATE semantic_operations SET proposal_json = '{}' WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "effect hash differs", update: `UPDATE semantic_operations SET effect_sha256 = 'sha256:tampered' WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "result JSON is not canonical", update: `UPDATE semantic_operations SET result_json = json_set(result_json, '$.unknown', true) WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
		{name: "result identity differs", update: `UPDATE semantic_operations SET result_json = json_set(result_json, '$.claim_id', 'ffffffff-ffff-4fff-8fff-ffffffffffff') WHERE operation_id = ?`, wantVersion: 1, args: func(id memory.SemanticID) []any { return []any{id} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			lease, err := store.AcquireTurnLease(ctx, session.ID, "malformed-replay", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			accepted := acceptLifecycleLiteral(t, ctx, store, session, lease,
				"idem:v1:94000000-0000-4000-8000-000000000130", "Detroit")
			if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_operations_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if test.before != "" {
				if _, err := db.ExecContext(ctx, test.before); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, test.update, test.args(accepted.OperationID)...); err != nil {
				t.Fatal(err)
			}
			if test.after != "" {
				if _, err := db.ExecContext(ctx, test.after); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END`); err != nil {
				t.Fatal(err)
			}
			_, err = store.VerifySemanticProjection(ctx)
			var replayErr *SemanticReplayError
			if !errors.As(err, &replayErr) || replayErr.OperationID != accepted.OperationID || replayErr.SchemaVersion != test.wantVersion {
				t.Fatalf("replay error = %#v, want exact operation %s", err, accepted.OperationID)
			}
			if _, err := store.InspectLiteralClaims(ctx, session.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
				t.Fatalf("quarantine after replay failure = %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := NewStore(db).InspectLiteralClaims(ctx, session.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
				t.Fatalf("reopened quarantine = %v", err)
			}
		})
	}
}

func TestSemanticProjectionMissingOperationFrontierQuarantinesTargetScope(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "missing-frontier", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	accepted := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:94000000-0000-4000-8000-000000000139", "Detroit")
	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_operation_scopes_append_only_delete`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM semantic_operation_scopes WHERE operation_id = ?`, accepted.OperationID); err != nil {
		t.Fatal(err)
	}
	_, err = store.VerifySemanticProjection(ctx)
	var replayErr *SemanticReplayError
	if !errors.As(err, &replayErr) || replayErr.OperationID != accepted.OperationID {
		t.Fatalf("missing-frontier replay error = %#v, want exact operation %s", err, accepted.OperationID)
	}
	if _, err := store.InspectLiteralClaims(ctx, session.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("target scope after missing frontier = %v", err)
	}
}

func TestSemanticProjectionMaintenanceIsFencedAcrossStores(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	otherDB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	store, other := NewStore(db), NewStore(otherDB)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedLease, err := store.AcquireTurnLease(ctx, session.ID, "maintenance-seed", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	acceptLifecycleLiteral(t, ctx, store, session, seedLease,
		"idem:v1:94000000-0000-4000-8000-000000000140", "Detroit")
	if err := store.ReleaseTurnLease(ctx, seedLease.SessionID, seedLease.HolderID, seedLease.FencingToken); err != nil {
		t.Fatal(err)
	}
	writeLease, err := other.AcquireTurnLease(ctx, session.ID, "maintenance-writer", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event := appendLifecycleEvent(t, ctx, other, session, writeLease, "Remember Chicago too")
	proposal, err := other.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000141", SourceEventID: event.ID,
		Predicate: "home_city", PredicateLabel: "home city", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"},
	})
	if err != nil {
		t.Fatal(err)
	}

	locked := make(chan struct{})
	release := make(chan struct{})
	store.semanticMaintenance.afterLock = func() error {
		close(locked)
		<-release
		return nil
	}
	type rebuildResult struct {
		value memory.SemanticProjectionRebuild
		err   error
	}
	resultChannel := make(chan rebuildResult, 1)
	go func() {
		value, err := store.OwnerRebuildSemanticProjection(ctx, "maintenance-one")
		resultChannel <- rebuildResult{value: value, err: err}
	}()
	<-locked
	if _, err := other.ApplyRememberLiteral(ctx, writeLease, proposal); !errors.Is(err, ErrSemanticMaintenanceHeld) {
		t.Fatalf("semantic write during maintenance = %v", err)
	}
	if _, err := other.OwnerRebuildSemanticProjection(ctx, "maintenance-two"); !errors.Is(err, ErrSemanticMaintenanceHeld) {
		t.Fatalf("second store maintenance race = %v", err)
	}
	close(release)
	first := <-resultChannel
	if first.err != nil || first.value.FencingToken != 1 {
		t.Fatalf("first rebuild = %+v err=%v", first.value, first.err)
	}
	store.semanticMaintenance.afterLock = nil
	second, err := other.OwnerRebuildSemanticProjection(ctx, "maintenance-two")
	if err != nil || second.FencingToken != 2 {
		t.Fatalf("second fenced rebuild = %+v err=%v", second, err)
	}
	if _, err := other.ApplyRememberLiteral(ctx, writeLease, proposal); err != nil {
		t.Fatalf("semantic write after maintenance = %v", err)
	}
}

func TestSemanticProjectionFailedOrInterruptedRebuildPreservesLiveState(t *testing.T) {
	tests := []struct {
		name string
		hook func(*Store)
	}{
		{name: "interrupted before swap", hook: func(store *Store) {
			store.semanticMaintenance.beforeSwap = func() error { return context.Canceled }
		}},
		{name: "malformed shadow", hook: func(store *Store) {
			store.semanticMaintenance.beforeShadowValidation = func(db *sql.DB) error {
				if _, err := db.Exec(`DROP TRIGGER semantic_claims_append_only_update`); err != nil {
					return err
				}
				_, err := db.Exec(`UPDATE semantic_claims SET literal_value = 'Malformed shadow'`)
				return err
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
			lease, err := store.AcquireTurnLease(ctx, session.ID, "failed-rebuild", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			accepted := acceptLifecycleLiteral(t, ctx, store, session, lease,
				"idem:v1:94000000-0000-4000-8000-000000000150", "Detroit")
			if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET literal_value = 'Live projection before failure' WHERE claim_id = ?`, accepted.ClaimID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END`); err != nil {
				t.Fatal(err)
			}
			test.hook(store)
			if _, err := store.OwnerRebuildSemanticProjection(ctx, "failed-rebuild"); err == nil {
				t.Fatal("failed rebuild unexpectedly succeeded")
			}
			inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
			if err != nil || len(inspection.Claims) != 1 || inspection.Claims[0].Literal.Value != "Live projection before failure" {
				t.Fatalf("failed rebuild changed live projection: %+v err=%v", inspection, err)
			}
			var operations int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations`).Scan(&operations); err != nil || operations != 1 {
				t.Fatalf("failed rebuild changed operation stream: count=%d err=%v", operations, err)
			}
		})
	}
}

func TestSemanticProjectionStartupUsesCheapFrontierChecks(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "startup-check", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	accepted := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:94000000-0000-4000-8000-000000000160", "Detroit")
	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET literal_value = 'Chicago' WHERE claim_id = ?`, accepted.ClaimID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(db)
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil || len(inspection.Claims) != 1 || inspection.Claims[0].Literal.Value != "Chicago" {
		t.Fatalf("startup unexpectedly performed full replay: %+v err=%v", inspection, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_scopes SET revision = 9 WHERE scope_key = 'global'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewStore(db).InspectLiteralClaims(ctx, session.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("startup frontier mismatch was not quarantined: %v", err)
	}
}

func TestSemanticProjectionQuarantinesOnlyTheDivergentScope(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalClaim := rememberScopeClaim(t, ctx, store, globalSession, false, 931)
	workspace, err := store.RegisterWorkspace(ctx, "Quarantine scope")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	workspaceClaim := rememberScopeClaim(t, ctx, store, workspaceSession, false, 932)
	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET polarity = 'denied' WHERE claim_id = ?`, workspaceClaim.Claim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END`); err != nil {
		t.Fatal(err)
	}
	report, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid {
		t.Fatalf("divergent Workspace unexpectedly verified: %+v", report)
	}
	quarantined := make(map[string]bool)
	for _, scope := range report.Scopes {
		if scope.Quarantined {
			quarantined[scope.ScopeKey] = true
		}
	}
	workspaceKey := "workspace:" + string(workspace.ID)
	if !quarantined[workspaceKey] || quarantined["global"] {
		t.Fatalf("quarantine scope set = %+v, report=%+v", quarantined, report)
	}
	globalInspection, err := store.InspectEntityClaims(ctx, globalSession.ScopeContext())
	if err != nil || len(globalInspection.Claims) != 1 || globalInspection.Claims[0].Claim.ID != globalClaim.Claim.ID {
		t.Fatalf("unrelated global scope unavailable: %+v err=%v", globalInspection, err)
	}
	if _, err := store.InspectEntityClaims(ctx, workspaceSession.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("divergent Workspace read = %v", err)
	}
	lease, err := store.AcquireTurnLease(ctx, workspaceSession.ID, "quarantined-episodic", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, workspaceSession.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Episodic remains available in quarantined scope",
	})
	if err != nil || event.ID == "" {
		t.Fatalf("quarantined Workspace Episodic append = %+v err=%v", event, err)
	}
	if _, err := store.PrepareRememberEntity(ctx, workspaceSession.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8000-000000000933", SourceEventID: event.ID,
		Predicate: "blocked", PredicateLabel: "blocked",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Blocked", EntityType: "concept"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Write", EntityType: "concept"},
	}); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("divergent Workspace write preparation = %v", err)
	}
}

func TestSemanticProjectionForeignKeyFailuresAreReportedAndQuarantinedPerScope(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalClaim := rememberScopeClaim(t, ctx, store, globalSession, false, 941)
	workspace, err := store.RegisterWorkspace(ctx, "Foreign-key quarantine")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	workspaceClaim := rememberScopeClaim(t, ctx, store, workspaceSession, false, 942)
	workspaceKey := "workspace:" + string(workspace.ID)
	corruptScopeForeignKey := func(database *sql.DB) {
		t.Helper()
		connection, err := database.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if _, err := connection.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, `UPDATE semantic_claims SET scope_id = 'ffffffff-ffff-4fff-8fff-ffffffffffff' WHERE claim_id = ?`, workspaceClaim.Claim.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
			t.Fatal(err)
		}
		if _, err := connection.ExecContext(ctx, semanticProjectionAppendOnlyTriggerSQL()); err != nil {
			t.Fatal(err)
		}
	}

	corruptScopeForeignKey(db)
	report, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid {
		t.Fatalf("foreign-key corruption verified: %+v", report)
	}
	var workspaceReport *memory.SemanticProjectionScopeVerification
	for index := range report.Scopes {
		if report.Scopes[index].ScopeKey == workspaceKey {
			workspaceReport = &report.Scopes[index]
		}
		if report.Scopes[index].ScopeKey == "global" && report.Scopes[index].Quarantined {
			t.Fatalf("unrelated global scope quarantined: %+v", report.Scopes[index])
		}
	}
	if workspaceReport == nil || !workspaceReport.Quarantined || !projectionMismatchNames(workspaceReport.Mismatches)["foreign_key:semantic_claims"] {
		t.Fatalf("workspace foreign-key report = %+v", workspaceReport)
	}
	foundExactFailure := false
	for _, mismatch := range workspaceReport.Mismatches {
		if mismatch.Table == "foreign_key:semantic_claims" && len(mismatch.LiveCanonicalRows) == 1 &&
			len(mismatch.ShadowCanonicalRows) == 0 {
			foundExactFailure = true
		}
	}
	if !foundExactFailure {
		t.Fatalf("exact foreign-key mismatch omitted: %+v", workspaceReport.Mismatches)
	}
	globalInspection, err := store.InspectEntityClaims(ctx, globalSession.ScopeContext())
	if err != nil || len(globalInspection.Claims) != 1 || globalInspection.Claims[0].Claim.ID != globalClaim.Claim.ID {
		t.Fatalf("global scope after Workspace FK failure = %+v err=%v", globalInspection, err)
	}
	if _, err := store.OwnerRebuildSemanticProjection(ctx, "foreign-key-recovery"); err != nil {
		t.Fatalf("rebuild foreign-key corruption: %v", err)
	}
	corruptScopeForeignKey(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	if _, err := store.InspectEntityClaims(ctx, workspaceSession.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("startup did not quarantine affected Workspace: %v", err)
	}
	globalInspection, err = store.InspectEntityClaims(ctx, globalSession.ScopeContext())
	if err != nil || len(globalInspection.Claims) != 1 {
		t.Fatalf("startup quarantined unrelated global scope: %+v err=%v", globalInspection, err)
	}
}

func TestSemanticProjectionVerifyQuarantineAndOwnerRebuild(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "replay-owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	accepted := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:94000000-0000-4000-8000-000000000110", "Detroit")

	first, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatalf("first verification: %v", err)
	}
	second, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatalf("repeated verification: %v", err)
	}
	if !first.Valid || !second.Valid || len(first.Scopes) != 1 || !reflect.DeepEqual(first.Scopes[0], second.Scopes[0]) {
		t.Fatalf("verification first=%+v second=%+v", first, second)
	}
	if first.Scopes[0].ScopeKey != "global" || first.Scopes[0].LiveRevision != 1 ||
		first.Scopes[0].ShadowRevision != 1 || first.Scopes[0].LiveHash != first.Scopes[0].ShadowHash ||
		first.Scopes[0].LiveFrontier != first.Scopes[0].ShadowFrontier || len(first.Scopes[0].Mismatches) != 0 {
		t.Fatalf("valid scope report = %+v", first.Scopes[0])
	}

	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET literal_value = 'Chicago' WHERE claim_id = ?`, accepted.ClaimID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END`); err != nil {
		t.Fatal(err)
	}

	divergent, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatalf("verification of divergent projection: %v", err)
	}
	if divergent.Valid || len(divergent.Scopes) != 1 || !divergent.Scopes[0].Quarantined ||
		!projectionMismatchNames(divergent.Scopes[0].Mismatches)["semantic_claims"] {
		t.Fatalf("divergent report = %+v", divergent)
	}
	for _, mismatch := range divergent.Scopes[0].Mismatches {
		if mismatch.Table == "semantic_claims" && (len(mismatch.LiveCanonicalRows) != 1 || len(mismatch.ShadowCanonicalRows) != 1 ||
			mismatch.LiveCanonicalRows[0] == mismatch.ShadowCanonicalRows[0]) {
			t.Fatalf("semantic claim mismatch omitted exact canonical rows: %+v", mismatch)
		}
	}
	if _, err := store.InspectLiteralClaims(ctx, session.ScopeContext()); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("quarantined read error = %v", err)
	}
	requestEvent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Episodic Memory remains available",
	})
	if err != nil || requestEvent.ID == "" {
		t.Fatalf("append Episodic event while quarantined: event=%+v err=%v", requestEvent, err)
	}
	if _, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000111", SourceEventID: requestEvent.ID,
		Predicate: "home_city", PredicateLabel: "home city", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"},
	}); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("quarantined write preparation error = %v", err)
	}

	rebuilt, err := store.OwnerRebuildSemanticProjection(ctx, "replay-owner")
	if err != nil {
		t.Fatalf("owner rebuild: %v", err)
	}
	if !rebuilt.Valid || rebuilt.FencingToken < 1 {
		t.Fatalf("rebuild result = %+v", rebuilt)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatalf("inspect rebuilt projection: %v", err)
	}
	if len(inspection.Claims) != 1 || inspection.Claims[0].ID != accepted.ClaimID || inspection.Claims[0].Literal.Value != "Detroit" {
		t.Fatalf("rebuilt inspection = %+v", inspection)
	}
	var operations, events int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if operations != 1 || events != 2 {
		t.Fatalf("canonical history changed: operations=%d events=%d", operations, events)
	}
}

func projectionMismatchNames(mismatches []memory.SemanticProjectionMismatch) map[string]bool {
	names := make(map[string]bool, len(mismatches))
	for _, mismatch := range mismatches {
		names[mismatch.Table] = true
	}
	return names
}
