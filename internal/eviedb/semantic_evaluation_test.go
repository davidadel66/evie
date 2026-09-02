package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/memoryeval"
)

const semanticEvaluationRepetitions = 5

func TestSemanticEvaluationStage3Corpus(t *testing.T) {
	ctx := context.Background()
	manifestPath := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory", "evaluation", "v1", "manifest.json")
	manifest, err := memoryeval.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load Stage 3 evaluation manifest: %v", err)
	}
	if os.Getenv("EVIE_EVALUATION_TAMPER_EXPECTATION") == "1" {
		for index := range manifest.Cases {
			if manifest.Cases[index].ID == "graph-replay-recovery" {
				manifest.Cases[index].Expected.Paths[0].ExpectedGraphLinkIDs[0] = "66000000-0000-4000-8000-000000000499"
			}
		}
		manifest.DatasetSHA256, err = manifest.ContentSHA256()
		if err != nil {
			t.Fatal(err)
		}
	}
	report := memoryeval.Report{
		ReportSchemaVersion: 1,
		Run:                 memoryeval.RunIdentity{ID: "go-test-" + time.Now().UTC().Format("20060102T150405.000000000Z"), Commit: evaluationCommit(), StartedAt: time.Now().UTC()},
		Fixture:             memoryeval.FixtureIdentity{ManifestVersion: manifest.ManifestVersion, FixtureVersion: manifest.FixtureVersion, DatasetSHA256: manifest.DatasetSHA256},
		Components:          map[string]memoryeval.ComponentIdentity{"semantic_kernel": {Name: "eviedb", Version: "stage-3", Revision: evaluationCommit(), ConfigHash: manifest.DatasetSHA256}},
		Panels:              memoryeval.EmptyPanels(),
		Cardinality:         manifestCardinality(manifest),
	}
	t.Cleanup(func() {
		report.Summarize()
		if validateErr := report.Validate(); validateErr != nil {
			t.Errorf("validate machine-readable evaluation report: %v", validateErr)
			return
		}
		encoded, encodeErr := json.Marshal(report)
		if encodeErr != nil {
			t.Errorf("encode machine-readable evaluation report: %v", encodeErr)
			return
		}
		t.Logf("semantic-evaluation-json=%s", encoded)
		t.Logf("\n%s", report.Markdown())
	})

	runGate(t, &report, "manifest-and-frozen-operations", memoryeval.FailureOperationOutcome, func(t *testing.T) {
		if len(manifest.Imports) != 7 || len(manifest.Cases) != 6 {
			t.Fatalf("manifest imports/cases = %d/%d, want 7/6", len(manifest.Imports), len(manifest.Cases))
		}
		for _, imported := range manifest.Imports {
			path := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), imported))
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("frozen operation fixture %q: %v", imported, err)
			}
		}
	})
	for _, fixtureCase := range manifest.Cases {
		fixtureCase := fixtureCase
		runGate(t, &report, "fixture/"+fixtureCase.ID, fixtureCase.FailureTaxonomy, func(t *testing.T) {
			runExecutableFixtureCase(t, ctx, fixtureCase)
		})
	}

	runGate(t, &report, "exact-outcomes-revisions-idempotence-provenance-restart", memoryeval.FailureOperationOutcome, func(t *testing.T) {
		fixtureCase := manifest.Cases[1]
		fixtureSource := fixtureCase.Sources[0]
		fixtureOperation := fixtureCase.Operations[0]
		path := filepath.Join(t.TempDir(), "evie.db")
		db, err := OpenDBAt(path)
		if err != nil {
			t.Fatal(err)
		}
		store := NewStore(db)
		clock, err := time.Parse(time.RFC3339Nano, fixtureOperation.Clock)
		if err != nil {
			t.Fatal(err)
		}
		store.now = func() time.Time { return clock }
		sourceRecordedAt, err := time.Parse(time.RFC3339Nano, fixtureSource.RecordedAt)
		if err != nil {
			t.Fatal(err)
		}
		session := memory.Session{ID: memory.SessionID(fixtureSource.SessionID), Status: memory.SessionActive, CreatedAt: sourceRecordedAt, UpdatedAt: sourceRecordedAt}
		if _, err := db.ExecContext(ctx, `INSERT INTO sessions (id, status, created_at, updated_at) VALUES (?, 'active', ?, ?)`, session.ID, fixtureSource.RecordedAt, fixtureSource.RecordedAt); err != nil {
			t.Fatal(err)
		}
		lease, err := store.AcquireTurnLease(ctx, session.ID, "evaluation-exact", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO events (id, session_id, sequence, event_type, role, content, payload_json, recorded_at, format_version) VALUES (?, ?, 1, 'user_message', 'user', ?, '{}', ?, 1)`, fixtureSource.EventID, session.ID, fixtureSource.Content, fixtureSource.RecordedAt); err != nil {
			t.Fatal(err)
		}
		firstEvent := memory.Event{ID: memory.EventID(fixtureSource.EventID), SessionID: session.ID, Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fixtureSource.Content, RecordedAt: sourceRecordedAt}
		first, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: fixtureOperation.IdempotencyKey, SourceEventID: firstEvent.ID,
			Predicate: "timezone_name", PredicateLabel: "timezone name", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
		})
		if err != nil {
			t.Fatal(err)
		}
		first = freezeLiteralFixtureProposal(t, first, fixtureOperation)
		staleEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Chicago is the current timezone")
		stale, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: evaluationKey(2), SourceEventID: staleEvent.ID,
			Predicate: "timezone_name", PredicateLabel: "timezone name", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ApplyRememberLiteral(ctx, lease, first)
		if err != nil || result.ScopeRevision != fixtureOperation.ExpectedRevisions["global"] || result.OperationID != memory.SemanticID(fixtureOperation.OperationID) || result.ClaimID != memory.SemanticID(fixtureOperation.GeneratedIDs["claim_id"]) || result.SourceLinkID != memory.SemanticID(fixtureOperation.GeneratedIDs["source_link_id"]) || !result.TransactionTime.Equal(clock) {
			t.Fatalf("exact accepted result = %+v, error=%v", result, err)
		}
		retry, err := store.ApplyRememberLiteral(ctx, lease, first)
		if err != nil || !semanticTestJSONEqual(t, retry, result) {
			t.Fatalf("idempotent result = %+v, error=%v, want %+v", retry, err, result)
		}
		if _, err := store.ApplyRememberLiteral(ctx, lease, stale); !errors.Is(err, ErrStaleScopeRevision) {
			t.Fatalf("stale proposal error = %v", err)
		}
		interruptedEvent := appendLifecycleEvent(t, ctx, store, session, lease, "This operation must roll back")
		interrupted, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: evaluationKey(3), SourceEventID: interruptedEvent.ID, Predicate: "timezone_name", PredicateLabel: "timezone name", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Interrupted"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER evaluation_interrupt_semantic_claim BEFORE INSERT ON semantic_claims WHEN NEW.claim_id = '%s' BEGIN SELECT RAISE(ABORT, 'evaluation interruption'); END`, interrupted.ClaimID)); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyRememberLiteral(ctx, lease, interrupted); err == nil {
			t.Fatal("interrupted operation unexpectedly committed")
		}
		if _, err := db.ExecContext(ctx, `DROP TRIGGER evaluation_interrupt_semantic_claim`); err != nil {
			t.Fatal(err)
		}
		var interruptedOperations, revision int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`, interrupted.OperationID).Scan(&interruptedOperations); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = 'global'`).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		if interruptedOperations != 0 || revision != 1 {
			t.Fatalf("interrupted operation left operation_count=%d revision=%d", interruptedOperations, revision)
		}
		inspection, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, result.ClaimID)
		if err != nil || inspection.Status != memory.SemanticStatusActive || len(inspection.Sources) != 1 || inspection.Sources[0].Source.EventID != firstEvent.ID || len(inspection.Operations) != 1 {
			t.Fatalf("exact provenance = %+v, error=%v", inspection, err)
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
		afterRestart, err := reopened.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, result.ClaimID)
		if err != nil || afterRestart.ObjectID != inspection.ObjectID || afterRestart.Status != inspection.Status || afterRestart.Metadata.ScopeRevisions[0].Revision != 1 {
			t.Fatalf("restart result = %+v, error=%v", afterRestart, err)
		}
	})

	runGate(t, &report, "scope-isolation-same-name-multiple-sources", memoryeval.FailureScopeLeakage, func(t *testing.T) {
		db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := NewStore(db)
		global, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := store.AcquireTurnLease(ctx, global.ID, "evaluation-alias", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		prepare := func(key, content, canonical string, object memory.EntitySelector) memory.RememberEntityProposal {
			event := appendLifecycleEvent(t, ctx, store, global, lease, content)
			proposal, err := store.PrepareRememberEntity(ctx, global.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: key, SourceEventID: event.ID, Predicate: "knows", PredicateLabel: "knows", PredicateCardinality: memory.CardinalityMany, Subject: memory.EntitySelector{Create: true, CanonicalName: canonical, EntityType: "person", Alias: "Alex"}, Object: object})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ApplyRememberEntity(ctx, lease, proposal); err != nil {
				t.Fatal(err)
			}
			return proposal
		}
		first := prepare(evaluationKey(10), "Alex North knows Casey", "Alex North", memory.EntitySelector{Create: true, CanonicalName: "Casey", EntityType: "person", Alias: "Casey"})
		_ = prepare(evaluationKey(11), "Alex South knows Casey", "Alex South", memory.EntitySelector{EntityID: first.Claim.ObjectEntityID})
		corroboration := appendLifecycleEvent(t, ctx, store, global, lease, "Alex North knows Casey (independent confirmation)")
		corroborated, err := store.PrepareRememberEntity(ctx, global.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: evaluationKey(12), SourceEventID: corroboration.ID, Predicate: "knows", PredicateLabel: "knows", PredicateCardinality: memory.CardinalityMany, Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID}, Object: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyRememberEntity(ctx, lease, corroborated); err != nil {
			t.Fatal(err)
		}
		firstInspection, err := store.InspectSemanticObject(ctx, global.ScopeContext(), memory.SemanticObjectClaim, first.Claim.ID)
		if err != nil || len(firstInspection.Sources) != 2 {
			t.Fatalf("independent provenance = %+v, error=%v", firstInspection.Sources, err)
		}
		matches, err := store.LookupEntitiesByAlias(ctx, global.ScopeContext(), " alex ")
		if err != nil || len(matches) != 2 || matches[0].Entity.ID == matches[1].Entity.ID {
			t.Fatalf("same-name identities = %+v, error=%v", matches, err)
		}

		workspaceA, err := store.RegisterWorkspace(ctx, "Evaluation A")
		if err != nil {
			t.Fatal(err)
		}
		workspaceB, err := store.RegisterWorkspace(ctx, "Evaluation B")
		if err != nil {
			t.Fatal(err)
		}
		sessionA, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceA.ID, workspaceA.CurrentRevisionID, standardReceipt(t))
		if err != nil {
			t.Fatal(err)
		}
		sessionB, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceB.ID, workspaceB.CurrentRevisionID, standardReceipt(t))
		if err != nil {
			t.Fatal(err)
		}
		workspaceClaim := rememberScopeClaim(t, ctx, store, sessionA, false, 11301)
		visibleA, err := store.InspectEntityClaims(ctx, sessionA.ScopeContext())
		if err != nil {
			t.Fatal(err)
		}
		visibleB, err := store.InspectEntityClaims(ctx, sessionB.ScopeContext())
		if err != nil {
			t.Fatal(err)
		}
		if !containsEntityClaim(visibleA, workspaceClaim.Claim.ID) || containsEntityClaim(visibleB, workspaceClaim.Claim.ID) {
			t.Fatalf("Workspace isolation A=%+v B=%+v", visibleA.Claims, visibleB.Claims)
		}

		projectRoot := t.TempDir()
		project, err := store.RegisterProject(ctx, "Evaluation project", projectRoot)
		if err != nil {
			t.Fatal(err)
		}
		projectSession, err := store.CreateProjectSessionWithComposition(ctx, project.ID, standardReceipt(t))
		if err != nil {
			t.Fatal(err)
		}
		projectClaim := rememberScopeClaim(t, ctx, store, projectSession, false, 11302)
		sessionClaim := rememberScopeClaim(t, ctx, store, projectSession, true, 11303)
		projectView, err := store.InspectClaims(ctx, projectSession.ScopeContext(), memory.ClaimQuery{ScopeKey: "project:" + string(project.ID)})
		if err != nil || !containsClaim(projectView, projectClaim.Claim.ID) || containsClaim(projectView, sessionClaim.Claim.ID) {
			t.Fatalf("project/session default view = %+v, error=%v", projectView.Claims, err)
		}
		sessionView, err := store.InspectClaims(ctx, projectSession.ScopeContext(), memory.ClaimQuery{ScopeKey: "session:" + string(projectSession.ID)})
		if err != nil || !containsClaim(sessionView, sessionClaim.Claim.ID) {
			t.Fatalf("explicit session view = %+v, error=%v", sessionView.Claims, err)
		}
	})

	runGate(t, &report, "temporal-correction-lifecycle", memoryeval.FailureTemporal, func(t *testing.T) {
		db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := NewStore(db)
		acceptedAt := time.Date(2026, 9, 2, 13, 0, 1, 0, time.UTC)
		store.now = func() time.Time { return acceptedAt }
		session, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := store.AcquireTurnLease(ctx, session.ID, "evaluation-temporal", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		validTo := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		old := prepareLiteralForCorrection(t, ctx, store, session, lease, evaluationKey(20), "Detroit was recorded", "Detroit", memory.ValidTime{From: &validFrom, To: &validTo})
		oldResult, err := store.ApplyRememberLiteral(ctx, lease, old)
		if err != nil {
			t.Fatal(err)
		}
		correctedAt := acceptedAt.Add(time.Second)
		store.now = func() time.Time { return correctedAt }
		correctionEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Correct Detroit to Chicago")
		correction, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{IdempotencyKey: evaluationKey(21), SourceEventID: correctionEvent.ID, OldClaimID: oldResult.ClaimID, Mode: memory.CorrectionError, Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}}, Polarity: memory.PolarityAffirmed}})
		if err != nil {
			t.Fatal(err)
		}
		corrected, err := store.ApplyCorrectClaim(ctx, lease, correction)
		if err != nil {
			t.Fatal(err)
		}
		before := validFrom.Add(-time.Nanosecond)
		inside := validFrom.Add(time.Hour)
		assertLiteralClaimAt(t, ctx, store, session, before, corrected.TransactionTime, "")
		assertLiteralClaimAt(t, ctx, store, session, validFrom, corrected.TransactionTime, "Chicago")
		assertLiteralClaimAt(t, ctx, store, session, inside, corrected.TransactionTime, "Chicago")
		assertLiteralClaimAt(t, ctx, store, session, validTo, corrected.TransactionTime, "")
		assertLiteralClaimAt(t, ctx, store, session, validTo.Add(time.Nanosecond), corrected.TransactionTime, "")
		assertLiteralClaimAt(t, ctx, store, session, inside, oldResult.TransactionTime, "Detroit")

		store.now = func() time.Time { return correctedAt.Add(time.Second) }
		retractEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Retract correction source")
		retract, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: evaluationKey(22), SourceEventID: retractEvent.ID, Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: corrected.SourceLinkID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, retract); err != nil {
			t.Fatal(err)
		}
		unsupported, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, corrected.ReplacementClaimID)
		if err != nil || unsupported.Status != memory.SemanticStatusUnsupported {
			t.Fatalf("unsupported Claim = %+v, error=%v", unsupported, err)
		}
		store.now = func() time.Time { return correctedAt.Add(2 * time.Second) }
		restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Restore correction source")
		restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: evaluationKey(23), SourceEventID: restoreEvent.ID, Action: memory.LifecycleRestoreSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: corrected.SourceLinkID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
			t.Fatal(err)
		}
		recovered, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, corrected.ReplacementClaimID)
		if err != nil || recovered.Status != memory.SemanticStatusActive || len(recovered.Sources[0].Lifecycle) != 3 {
			t.Fatalf("restored Claim = %+v, error=%v", recovered, err)
		}

		changedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		store.now = func() time.Time { return correctedAt.Add(3 * time.Second) }
		changedEvent := appendLifecycleEvent(t, ctx, store, session, lease, "The city changed to New York on July 1")
		changed, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{IdempotencyKey: evaluationKey(24), SourceEventID: changedEvent.ID, OldClaimID: corrected.ReplacementClaimID, Mode: memory.CorrectionChanged, EffectiveTime: &changedAt, Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "New York"}}, Polarity: memory.PolarityAffirmed}})
		if err != nil {
			t.Fatal(err)
		}
		changedResult, err := store.ApplyCorrectClaim(ctx, lease, changed)
		if err != nil {
			t.Fatal(err)
		}
		assertLiteralClaimAt(t, ctx, store, session, changedAt.Add(-time.Nanosecond), changedResult.TransactionTime, "Chicago")
		assertLiteralClaimAt(t, ctx, store, session, changedAt, changedResult.TransactionTime, "New York")

		store.now = func() time.Time { return correctedAt.Add(4 * time.Second) }
		retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Retire the current city Claim")
		retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: evaluationKey(25), SourceEventID: retireEvent.ID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: changedResult.ReplacementClaimID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
			t.Fatal(err)
		}
		store.now = func() time.Time { return correctedAt.Add(5 * time.Second) }
		restoreClaimEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Restore the current city Claim")
		restoreClaim, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: evaluationKey(26), SourceEventID: restoreClaimEvent.ID, Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectClaim, ObjectID: changedResult.ReplacementClaimID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, restoreClaim); err != nil {
			t.Fatal(err)
		}
		restoredClaim, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, changedResult.ReplacementClaimID)
		if err != nil || restoredClaim.Status != memory.SemanticStatusActive || len(restoredClaim.Lifecycle) != 3 {
			t.Fatalf("retired/restored Claim = %+v, error=%v", restoredClaim, err)
		}
	})

	runGate(t, &report, "promotion-paths-replay-recovery", memoryeval.FailureRecovery, func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "evie.db")
		db, err := OpenDBAt(path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := NewStore(db)
		clock := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
		store.now = func() time.Time { return clock }
		global, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		workspace, err := store.RegisterWorkspace(ctx, "Evaluation promotion")
		if err != nil {
			t.Fatal(err)
		}
		workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
		if err != nil {
			t.Fatal(err)
		}
		workspaceClaim := rememberScopeClaim(t, ctx, store, workspaceSession, false, 11400)
		promotionLease, err := store.AcquireTurnLease(ctx, workspaceSession.ID, "evaluation-promotion", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		promotionEvent := appendLifecycleEvent(t, ctx, store, workspaceSession, promotionLease, "Promote the exact workspace Claim")
		promotion, err := store.PreparePromotion(ctx, workspaceSession.ScopeContext(), memory.PromotionRequest{IdempotencyKey: evaluationKey(29), SourceEventID: promotionEvent.ID, SourceClaimID: workspaceClaim.Claim.ID, DestinationScopeKey: "global"})
		if err != nil {
			t.Fatal(err)
		}
		approvePromotion(t, ctx, store, promotionLease, promotion, memory.ApprovalApproved)
		promoted, err := store.ApplyPromotion(ctx, promotionLease, promotion)
		if err != nil || promoted.SourceClaimID != workspaceClaim.Claim.ID || promoted.DestinationClaimID == promoted.SourceClaimID || promoted.DestinationRevision != promotion.DestinationScope.Revision+1 {
			t.Fatalf("Promotion result = %+v, error=%v", promoted, err)
		}
		if err := store.ReleaseTurnLease(ctx, promotionLease.SessionID, promotionLease.HolderID, promotionLease.FencingToken); err != nil {
			t.Fatal(err)
		}
		first := rememberScopeClaim(t, ctx, store, global, false, 11401)
		second := rememberScopeClaim(t, ctx, store, global, false, 11402)
		third := rememberScopeClaim(t, ctx, store, global, false, 11403)
		lease, err := store.AcquireTurnLease(ctx, global.ID, "evaluation-graph", time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		linkOne := prepareAndApplyGraphLink(t, ctx, store, global, lease, evaluationKey(30), "Recognize contradiction", memory.GraphRelationContradiction, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID})
		linkTwo := prepareAndApplyGraphLink(t, ctx, store, global, lease, evaluationKey(31), "Record derivation", memory.GraphRelationDerivation, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID})
		oneHop, err := store.TraverseSemanticNeighborhood(ctx, global.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 1})
		if err != nil || len(oneHop.Paths) != 1 || len(oneHop.Paths[0].Links) != 1 || oneHop.Paths[0].Links[0].ID != linkOne.GraphLinkID {
			t.Fatalf("one-hop paths = %+v, error=%v", oneHop.Paths, err)
		}
		twoHop, err := store.TraverseSemanticNeighborhood(ctx, global.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
		if err != nil || len(twoHop.Paths) != 2 || !neighborhoodHasLink(twoHop, linkTwo.GraphLinkID) {
			t.Fatalf("two-hop paths = %+v, error=%v", twoHop.Paths, err)
		}
		directIDs := queryEvaluationGraphLinkIDs(t, ctx, db, first.Claim.ID)
		typedIDs := semanticNeighborhoodLinkIDs(twoHop)
		if !semanticTestJSONEqual(t, directIDs, typedIDs) {
			t.Fatalf("direct SQL / recursive CTE / typed traversal differ: direct=%v typed=%v", directIDs, typedIDs)
		}
		verification, err := store.VerifySemanticProjection(ctx)
		if err != nil || !verification.Valid {
			t.Fatalf("replay verification = %+v, error=%v", verification, err)
		}
		if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_entities_append_only_update`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE semantic_entities SET canonical_name = 'corrupted projection' WHERE entity_id = ?`, first.Claim.SubjectEntityID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_entities_append_only_update BEFORE UPDATE ON semantic_entities BEGIN SELECT RAISE(ABORT, 'semantic entities are append-only'); END`); err != nil {
			t.Fatal(err)
		}
		divergent, err := store.VerifySemanticProjection(ctx)
		if err != nil || divergent.Valid || !semanticVerificationQuarantined(divergent, "global") {
			t.Fatalf("divergent projection verification = %+v, error=%v", divergent, err)
		}
		rebuild, err := store.OwnerRebuildSemanticProjection(ctx, "evaluation-rebuild")
		if err != nil || !rebuild.Valid || rebuild.FencingToken < 1 {
			t.Fatalf("projection rebuild = %+v, error=%v", rebuild, err)
		}
		after, err := store.TraverseSemanticNeighborhood(ctx, global.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
		if err != nil || !semanticTestJSONEqual(t, twoHop, after) {
			t.Fatalf("query/rebuild equivalence before=%+v after=%+v error=%v", twoHop, after, err)
		}
	})

	runPerformanceBaseline(t, ctx, &report)
	if report.Environment.SQLiteVersion == "" || report.Environment.JournalMode != "wal" || len(report.Metrics) < 12 {
		t.Fatalf("performance/environment report incomplete: %+v", report)
	}
	for _, metric := range report.Metrics {
		if metric.Threshold != nil || metric.Baseline != nil || metric.Delta != nil {
			t.Fatalf("initial baseline invented threshold or comparison: %+v", metric)
		}
	}
}

func TestSemanticEvaluationRejectsTamperedExpectationWithRefreshedDigest(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestSemanticEvaluationStage3Corpus$", "-test.count=1")
	command.Env = append(os.Environ(), "EVIE_EVALUATION_TAMPER_EXPECTATION=1")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "path one-hop") {
		t.Fatalf("tampered expectation with refreshed digest was not rejected: error=%v\n%s", err, output)
	}
}

func runGate(t *testing.T, report *memoryeval.Report, id string, taxonomy memoryeval.FailureTaxonomy, test func(*testing.T)) {
	t.Helper()
	started := time.Now()
	passed := t.Run(id, test)
	result := memoryeval.CaseResult{ID: id, Panel: memoryeval.PanelSemanticConformance, Gate: true, Status: memoryeval.StatusPassed, DurationNS: time.Since(started).Nanoseconds()}
	if !passed {
		result.Status = memoryeval.StatusFailed
		result.Failure = &memoryeval.Failure{Taxonomy: taxonomy, Message: "exact deterministic gate failed; see test output"}
	}
	report.Cases = append(report.Cases, result)
}

type executableFixtureRequest struct {
	SessionID            string                       `json:"session_id"`
	Predicate            string                       `json:"predicate"`
	PredicateLabel       string                       `json:"predicate_label"`
	PredicateCardinality memory.PredicateCardinality  `json:"predicate_cardinality"`
	Literal              memory.TypedLiteral          `json:"literal"`
	ValidFrom            string                       `json:"valid_from"`
	ValidTo              string                       `json:"valid_to"`
	Subject              executableEntitySelector     `json:"subject"`
	Object               executableEntitySelector     `json:"object"`
	OldClaimID           string                       `json:"old_claim_id"`
	Mode                 memory.CorrectionMode        `json:"mode"`
	ReplacementLiteral   memory.TypedLiteral          `json:"replacement_literal"`
	EffectiveTime        string                       `json:"effective_time"`
	Action               memory.MemoryLifecycleAction `json:"action"`
	ObjectKind           memory.SemanticObjectKind    `json:"object_kind"`
	ObjectID             string                       `json:"object_id"`
	SourceClaimID        string                       `json:"source_claim_id"`
	DestinationScopeKey  string                       `json:"destination_scope_key"`
	Relation             memory.GraphRelation         `json:"relation"`
	SourceKind           memory.SemanticObjectKind    `json:"source_kind"`
	SourceID             string                       `json:"source_id"`
	TargetKind           memory.SemanticObjectKind    `json:"target_kind"`
	TargetID             string                       `json:"target_id"`
	ForcePriorRevision   *int64                       `json:"force_prior_revision"`
	UseSessionScope      bool                         `json:"use_session_scope"`
}

type executableEntitySelector struct {
	EntityID      string `json:"entity_id"`
	Create        bool   `json:"create"`
	CanonicalName string `json:"canonical_name"`
	EntityType    string `json:"entity_type"`
	Alias         string `json:"alias"`
}

func runExecutableFixtureCase(t *testing.T, ctx context.Context, fixtureCase memoryeval.FixtureCase) {
	t.Helper()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	seedExecutableFixtureRegistry(t, ctx, db, fixtureCase)
	if len(fixtureCase.Operations) > 0 {
		leaseClock, err := time.Parse(time.RFC3339Nano, fixtureCase.Operations[0].Clock)
		if err != nil {
			t.Fatal(err)
		}
		store.now = func() time.Time { return leaseClock }
	}
	sessions := make(map[string]memory.Session)
	leases := make(map[string]memory.TurnLease)
	rejectedOperations := make(map[string]error)
	rejectedProjectionStable := make(map[string]bool)
	acceptedResultIDs := make(map[string]map[memory.SemanticID]struct{})
	for _, record := range fixtureCase.Registry.Sessions {
		session, err := store.GetSession(ctx, memory.SessionID(record.SessionID))
		if err != nil {
			t.Fatal(err)
		}
		sessions[record.SessionID] = session
		lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID("fixture-"+fixtureCase.ID), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		leases[record.SessionID] = lease
	}
	for _, operation := range fixtureCase.Operations {
		var request executableFixtureRequest
		if err := json.Unmarshal(operation.Request, &request); err != nil {
			t.Fatalf("%s request: %v", operation.Name, err)
		}
		session, ok := sessions[request.SessionID]
		if !ok {
			t.Fatalf("%s references unregistered session %q", operation.Name, request.SessionID)
		}
		lease := leases[request.SessionID]
		clock, err := time.Parse(time.RFC3339Nano, operation.Clock)
		if err != nil {
			t.Fatal(err)
		}
		store.now = func() time.Time { return clock }
		eventID := memory.EventID(operation.SourceEventIDs[0])
		projectionBefore := ""
		if operation.ExpectedOutcome == "rejected" {
			projectionBefore = fixtureProjectionFingerprint(t, ctx, store)
		}
		var applyErr error
		var resultOperationID memory.SemanticID
		var resultTransactionTime time.Time
		var resultRevisions []memory.ScopeRevision
		preparedEvidenceScope := ""
		resultIDs := make(map[string]memory.SemanticID)
		switch operation.Kind {
		case "remember_literal_claim":
			validTime := parseFixtureValidTime(t, request.ValidFrom, request.ValidTo)
			proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, Predicate: request.Predicate, PredicateLabel: request.PredicateLabel, PredicateCardinality: request.PredicateCardinality, Literal: request.Literal, ValidTime: validTime})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezeLiteralFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Source.ScopeKey
			if request.ForcePriorRevision != nil {
				proposal.Scope.Revision = *request.ForcePriorRevision
				for index := range proposal.Scopes {
					if proposal.Scopes[index].Key == proposal.Scope.Key {
						proposal.Scopes[index].Revision = *request.ForcePriorRevision
					}
				}
				for index := range proposal.PriorRevisions {
					if proposal.PriorRevisions[index].ScopeKey == proposal.Scope.Key {
						proposal.PriorRevisions[index].Revision = *request.ForcePriorRevision
					}
				}
				proposal.ExpectedRevision = *request.ForcePriorRevision
				proposal.ProposalSHA256, _, _ = semanticHash(canonicalRememberLiteralProposal(proposal))
				proposal.PreparedSHA256, _, _ = semanticHash(proposal)
			}
			result, err := store.ApplyRememberLiteral(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			resultIDs["claim_id"], resultIDs["source_link_id"] = result.ClaimID, result.SourceLinkID
		case "remember_entity_claim":
			proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, Predicate: request.Predicate, PredicateLabel: request.PredicateLabel, PredicateCardinality: request.PredicateCardinality, Subject: fixtureEntitySelector(request.Subject), Object: fixtureEntitySelector(request.Object), UseSessionScope: request.UseSessionScope})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezeEntityFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Source.ScopeKey
			result, err := store.ApplyRememberEntity(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			resultIDs["claim_id"], resultIDs["source_link_id"] = result.ClaimID, result.SourceLinkID
		case "correct_claim":
			old, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, memory.SemanticID(request.OldClaimID))
			if err != nil || old.Claim == nil {
				t.Fatalf("%s old claim: %+v %v", operation.Name, old, err)
			}
			var effectiveTime *time.Time
			if request.EffectiveTime != "" {
				value, parseErr := time.Parse(time.RFC3339Nano, request.EffectiveTime)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				effectiveTime = &value
			}
			proposal, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, OldClaimID: memory.SemanticID(request.OldClaimID), Mode: request.Mode, EffectiveTime: effectiveTime, Replacement: memory.ClaimProposition{SubjectEntityID: old.Claim.SubjectEntityID, PredicateID: old.Claim.Predicate.ID, Object: memory.ClaimObject{Literal: &request.ReplacementLiteral}, Polarity: old.Claim.Polarity}})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezeCorrectionFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Source.ScopeKey
			result, err := store.ApplyCorrectClaim(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			resultIDs["claim_id"], resultIDs["source_link_id"] = result.ReplacementClaimID, result.SourceLinkID
			if applyErr == nil && result.OldClaimID != memory.SemanticID(request.OldClaimID) {
				t.Fatalf("%s result old_claim_id=%s want=%s", operation.Name, result.OldClaimID, request.OldClaimID)
			}
		case "retract_source", "restore_source", "retire_memory", "restore_memory":
			proposal, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, Action: request.Action, ObjectKind: request.ObjectKind, ObjectID: memory.SemanticID(request.ObjectID)})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezeLifecycleFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Evidence.ScopeKey
			result, err := store.ApplyMemoryLifecycle(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			if applyErr == nil && (result.ObjectKind != request.ObjectKind || result.ObjectID != memory.SemanticID(request.ObjectID)) {
				t.Fatalf("%s result object=%s/%s want=%s/%s", operation.Name, result.ObjectKind, result.ObjectID, request.ObjectKind, request.ObjectID)
			}
		case "promote_claim":
			proposal, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, SourceClaimID: memory.SemanticID(request.SourceClaimID), DestinationScopeKey: request.DestinationScopeKey})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezePromotionFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Evidence.ScopeKey
			approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalApproved)
			result, err := store.ApplyPromotion(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			resultIDs["claim_id"] = result.DestinationClaimID
			if applyErr == nil && result.SourceClaimID != memory.SemanticID(request.SourceClaimID) {
				t.Fatalf("%s result source_claim_id=%s want=%s", operation.Name, result.SourceClaimID, request.SourceClaimID)
			}
		case "create_graph_link":
			proposal, err := store.PrepareCreateGraphLink(ctx, session.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: operation.IdempotencyKey, SourceEventID: eventID, Relation: request.Relation, Source: memory.GraphEndpoint{Kind: request.SourceKind, ID: memory.SemanticID(request.SourceID)}, Target: memory.GraphEndpoint{Kind: request.TargetKind, ID: memory.SemanticID(request.TargetID)}})
			if err != nil {
				t.Fatalf("%s prepare: %v", operation.Name, err)
			}
			proposal = freezeGraphFixtureProposal(t, proposal, operation)
			preparedEvidenceScope = proposal.Evidence.ScopeKey
			appendExactSemanticApproval(t, ctx, store, lease, eventID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
			result, err := store.ApplyCreateGraphLink(ctx, lease, proposal)
			applyErr = err
			resultOperationID, resultTransactionTime, resultRevisions = result.OperationID, result.TransactionTime, result.ResultingRevisions
			resultIDs["graph_link_id"] = result.GraphLinkID
		default:
			t.Fatalf("%s unsupported fixture operation kind %q", operation.Name, operation.Kind)
		}
		expectedEvidenceScope := ""
		for _, source := range fixtureCase.Sources {
			if source.EventID == string(eventID) {
				expectedEvidenceScope = source.ScopeKey
				break
			}
		}
		if preparedEvidenceScope != expectedEvidenceScope {
			t.Fatalf("%s prepared evidence scope=%q want manifest source scope=%q", operation.Name, preparedEvidenceScope, expectedEvidenceScope)
		}
		if operation.ExpectedOutcome == "rejected" {
			if !errors.Is(applyErr, ErrStaleScopeRevision) {
				t.Fatalf("%s error = %v, want stale scope revision", operation.Name, applyErr)
			}
			rejectedOperations[operation.Name] = applyErr
			rejectedProjectionStable[operation.Name] = projectionBefore == fixtureProjectionFingerprint(t, ctx, store)
		} else if applyErr != nil {
			t.Fatalf("%s apply: %v", operation.Name, applyErr)
		} else {
			assertFixtureApplyResult(t, operation, clock, resultOperationID, resultTransactionTime, resultRevisions, resultIDs, acceptedResultIDs)
			for field, id := range resultIDs {
				if id == "" {
					continue
				}
				if acceptedResultIDs[field] == nil {
					acceptedResultIDs[field] = make(map[memory.SemanticID]struct{})
				}
				acceptedResultIDs[field][id] = struct{}{}
			}
		}
		if operation.ExpectedOutcome == "accepted" {
			var persistedSchemaVersion int
			var persistedKind string
			if err := db.QueryRowContext(ctx, `SELECT schema_version, operation_kind FROM semantic_operations WHERE operation_id = ?`, operation.OperationID).Scan(&persistedSchemaVersion, &persistedKind); err != nil {
				t.Fatalf("%s persisted operation: %v", operation.Name, err)
			}
			if persistedSchemaVersion != operation.SchemaVersion || persistedKind != operation.Kind {
				t.Fatalf("%s persisted envelope schema/kind=%d/%s want=%d/%s", operation.Name, persistedSchemaVersion, persistedKind, operation.SchemaVersion, operation.Kind)
			}
		} else {
			var persisted int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`, operation.OperationID).Scan(&persisted); err != nil || persisted != 0 {
				t.Fatalf("%s rejected operation persisted=%d error=%v", operation.Name, persisted, err)
			}
		}
		assertFixtureRevisions(t, ctx, db, operation.ExpectedRevisions)
	}
	assertExecutableFixtureExpectations(t, ctx, store, db, sessions, rejectedOperations, rejectedProjectionStable, fixtureCase)
}

func assertFixtureApplyResult(t *testing.T, operation memoryeval.FixtureOperation, clock time.Time, operationID memory.SemanticID, transactionTime time.Time, revisions []memory.ScopeRevision, resultIDs map[string]memory.SemanticID, acceptedResultIDs map[string]map[memory.SemanticID]struct{}) {
	t.Helper()
	if operationID != memory.SemanticID(operation.OperationID) {
		t.Fatalf("%s result operation_id=%s want=%s", operation.Name, operationID, operation.OperationID)
	}
	if !transactionTime.Equal(clock) {
		t.Fatalf("%s result transaction_time=%s want=%s", operation.Name, transactionTime.Format(time.RFC3339Nano), clock.Format(time.RFC3339Nano))
	}
	gotRevisions := make(map[string]int64, len(revisions))
	for _, revision := range revisions {
		if _, duplicate := gotRevisions[revision.ScopeKey]; duplicate {
			t.Fatalf("%s result duplicates scope revision %q", operation.Name, revision.ScopeKey)
		}
		gotRevisions[revision.ScopeKey] = revision.Revision
	}
	if len(gotRevisions) != len(operation.ExpectedRevisions) {
		t.Fatalf("%s result revisions=%v want=%v", operation.Name, gotRevisions, operation.ExpectedRevisions)
	}
	for scopeKey, want := range operation.ExpectedRevisions {
		if got, ok := gotRevisions[scopeKey]; !ok || got != want {
			t.Fatalf("%s result scope %s revision=%d present=%t want=%d", operation.Name, scopeKey, got, ok, want)
		}
	}
	for field, got := range resultIDs {
		want, declared := operation.GeneratedIDs[field]
		if !declared {
			if _, previouslyAccepted := acceptedResultIDs[field][got]; previouslyAccepted {
				continue
			}
		}
		if !declared || got != memory.SemanticID(want) {
			t.Fatalf("%s result %s=%s want=%s declared=%t", operation.Name, field, got, want, declared)
		}
	}
}

func seedExecutableFixtureRegistry(t *testing.T, ctx context.Context, db *sql.DB, fixtureCase memoryeval.FixtureCase) {
	t.Helper()
	created := "2026-09-02T00:00:00.000000000Z"
	for _, workspace := range fixtureCase.Registry.Workspaces {
		if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, display_name, lifecycle_state, current_revision_id, created_at, updated_at) VALUES (?, ?, 'active', ?, ?, ?)`, workspace.WorkspaceID, workspace.DisplayName, workspace.CurrentRevisionID, created, created); err != nil {
			t.Fatal(err)
		}
	}
	for _, project := range fixtureCase.Registry.Projects {
		if _, err := db.ExecContext(ctx, `INSERT INTO projects (id, display_name, canonical_root, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, project.ProjectID, project.DisplayName, project.Root, created, created); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range fixtureCase.Registry.Sessions {
		var root any
		if session.ProjectID != "" {
			for _, project := range fixtureCase.Registry.Projects {
				if project.ProjectID == session.ProjectID {
					root = project.Root
				}
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO sessions (id, project_id, project_root_snapshot, title, status, created_at, updated_at, workspace_id, workspace_revision_snapshot) VALUES (?, NULLIF(?,''), ?, ?, ?, ?, ?, NULLIF(?,''), NULLIF(?,''))`, session.SessionID, session.ProjectID, root, session.Title, session.Status, created, created, session.WorkspaceID, session.WorkspaceRevisionID); err != nil {
			t.Fatal(err)
		}
	}
	for _, scope := range fixtureCase.Registry.Scopes {
		kind, registryID := fixtureScopeParts(scope.ScopeKey)
		if _, err := db.ExecContext(ctx, `INSERT INTO semantic_scopes (scope_id, scope_key, scope_kind, registry_id, revision) VALUES (?, ?, ?, NULLIF(?,''), ?)`, scope.ScopeID, scope.ScopeKey, kind, registryID, scope.Revision); err != nil {
			t.Fatal(err)
		}
	}
	sequence := make(map[string]int)
	for _, source := range fixtureCase.Sources {
		sequence[source.SessionID]++
		var projectID, workspaceID any
		for _, session := range fixtureCase.Registry.Sessions {
			if session.SessionID == source.SessionID {
				if session.ProjectID != "" {
					projectID = session.ProjectID
				}
				if session.WorkspaceID != "" {
					workspaceID = session.WorkspaceID
				}
			}
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO events (id, session_id, sequence, project_id, event_type, role, content, payload_json, recorded_at, format_version, workspace_id) VALUES (?, ?, ?, ?, 'user_message', 'user', ?, '{}', ?, 1, ?)`, source.EventID, source.SessionID, sequence[source.SessionID], projectID, source.Content, source.RecordedAt, workspaceID); err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureScopeParts(key string) (string, string) {
	if key == "global" {
		return "global", ""
	}
	parts := strings.SplitN(key, ":", 2)
	return parts[0], parts[1]
}

func parseFixtureValidTime(t *testing.T, from, to string) memory.ValidTime {
	t.Helper()
	result := memory.ValidTime{}
	if from != "" {
		value, err := time.Parse(time.RFC3339Nano, from)
		if err != nil {
			t.Fatal(err)
		}
		result.From = &value
	}
	if to != "" {
		value, err := time.Parse(time.RFC3339Nano, to)
		if err != nil {
			t.Fatal(err)
		}
		result.To = &value
	}
	return result
}

func fixtureEntitySelector(value executableEntitySelector) memory.EntitySelector {
	return memory.EntitySelector{EntityID: memory.SemanticID(value.EntityID), Create: value.Create, CanonicalName: value.CanonicalName, EntityType: value.EntityType, Alias: value.Alias}
}

func assertFixtureRevisions(t *testing.T, ctx context.Context, db *sql.DB, expected map[string]int64) {
	t.Helper()
	for key, want := range expected {
		var got int64
		if err := db.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = ?`, key).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("scope %s revision = %d, want %d", key, got, want)
		}
	}
}

func assertExecutableFixtureExpectations(t *testing.T, ctx context.Context, store *Store, db *sql.DB, sessions map[string]memory.Session, rejectedOperations map[string]error, rejectedProjectionStable map[string]bool, fixtureCase memoryeval.FixtureCase) {
	t.Helper()
	assertSnapshotIDSet := func(field, table, column string, want []string) {
		if want == nil {
			return
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s`, column, table, column))
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0)
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			got = append(got, value)
		}
		rows.Close()
		sort.Strings(want)
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("fixture %s snapshot %s=%v want=%v", fixtureCase.ID, field, got, want)
		}
	}
	snapshot := fixtureCase.Expected.Snapshot
	assertSnapshotIDSet("scope_keys", "semantic_scopes", "scope_key", snapshot.ScopeKeys)
	assertSnapshotIDSet("predicate_ids", "semantic_predicates", "predicate_id", snapshot.PredicateIDs)
	assertSnapshotIDSet("entity_ids", "semantic_entities", "entity_id", snapshot.EntityIDs)
	assertSnapshotIDSet("alias_ids", "semantic_aliases", "alias_id", snapshot.AliasIDs)
	assertSnapshotIDSet("claim_ids", "semantic_claims", "claim_id", snapshot.ClaimIDs)
	assertSnapshotIDSet("source_link_ids", "semantic_source_links", "source_link_id", snapshot.SourceLinkIDs)
	assertSnapshotIDSet("graph_link_ids", "semantic_graph_links", "graph_link_id", snapshot.GraphLinkIDs)
	for _, scalar := range []struct{ field, table, column, value string }{{"workspace_claim_id", "semantic_claims", "claim_id", snapshot.WorkspaceClaimID}, {"global_claim_id", "semantic_claims", "claim_id", snapshot.GlobalClaimID}, {"promotion_operation_id", "semantic_operations", "operation_id", snapshot.PromotionOperationID}} {
		value := scalar.value
		if value == "" {
			continue
		}
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, scalar.table, scalar.column), value).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("fixture %s snapshot %s=%s missing", fixtureCase.ID, scalar.field, value)
		}
	}
	if len(fixtureCase.Registry.Sessions) > 0 {
		session := sessions[fixtureCase.Registry.Sessions[0].SessionID]
		assertStatuses := func(field string, ids []string, status memory.SemanticObjectStatus) {
			for _, value := range ids {
				id := memory.SemanticID(value)
				inspection, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, id)
				if err != nil {
					t.Fatal(err)
				}
				if inspection.Status != status {
					t.Fatalf("fixture %s snapshot %s claim %s status=%s want=%s", fixtureCase.ID, field, id, inspection.Status, status)
				}
			}
		}
		assertStatuses("active_claim_ids", snapshot.ActiveClaimIDs, memory.SemanticStatusActive)
		assertStatuses("superseded_claim_ids", snapshot.SupersededClaimIDs, memory.SemanticStatusSuperseded)
	}
	assertFixtureRevisions(t, ctx, db, snapshot.ScopeRevisions)
	verification, err := store.VerifySemanticProjection(ctx)
	if err != nil || !verification.Valid {
		t.Fatalf("fixture %s canonical replay verification=%+v error=%v", fixtureCase.ID, verification, err)
	}
	if len(verification.Scopes) != len(snapshot.ScopeHashes) || len(verification.Scopes) != len(snapshot.ScopeFrontiers) {
		t.Fatalf("fixture %s canonical scope count=%d hashes=%d frontiers=%d", fixtureCase.ID, len(verification.Scopes), len(snapshot.ScopeHashes), len(snapshot.ScopeFrontiers))
	}
	for _, scope := range verification.Scopes {
		if got, ok := snapshot.ScopeHashes[scope.ScopeKey]; !ok || got != scope.LiveHash {
			t.Fatalf("fixture %s scope %s canonical hash=%s want=%s present=%t", fixtureCase.ID, scope.ScopeKey, scope.LiveHash, got, ok)
		}
		if got, ok := snapshot.ScopeFrontiers[scope.ScopeKey]; !ok || got != scope.LiveFrontier {
			t.Fatalf("fixture %s scope %s operation frontier=%s want=%s present=%t", fixtureCase.ID, scope.ScopeKey, scope.LiveFrontier, got, ok)
		}
	}
	for _, query := range fixtureCase.Expected.Queries {
		session, ok := sessions[query.SessionID]
		if !ok {
			t.Fatalf("fixture %s query %s has unknown session", fixtureCase.ID, query.ID)
		}
		got := make([]string, 0)
		switch query.Kind {
		case "current", "valid_at", "as_known_at":
			claimQuery := memory.ClaimQuery{ScopeKey: query.ScopeKey}
			if query.ValidAt != "" {
				value, err := time.Parse(time.RFC3339Nano, query.ValidAt)
				if err != nil {
					t.Fatal(err)
				}
				claimQuery.ValidAt = &value
			}
			if query.AsKnownAt != "" {
				value, err := time.Parse(time.RFC3339Nano, query.AsKnownAt)
				if err != nil {
					t.Fatal(err)
				}
				claimQuery.AsKnownAt = &value
			}
			inspection, err := store.InspectClaims(ctx, session.ScopeContext(), claimQuery)
			if err != nil {
				t.Fatal(err)
			}
			for _, claim := range inspection.Claims {
				got = append(got, string(claim.ID))
			}
		case "alias":
			matches, err := store.LookupEntitiesByAlias(ctx, session.ScopeContext(), query.Alias)
			if err != nil {
				t.Fatal(err)
			}
			for _, match := range matches {
				got = append(got, string(match.Entity.ID))
			}
		case "replay":
			verification, err := store.VerifySemanticProjection(ctx)
			if err != nil || !verification.Valid {
				t.Fatalf("fixture replay = %+v error=%v", verification, err)
			}
			rows, err := db.QueryContext(ctx, `SELECT graph_link_id FROM semantic_graph_links ORDER BY graph_link_id`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				got = append(got, id)
			}
			rows.Close()
		default:
			t.Fatalf("fixture %s unsupported query kind %q", fixtureCase.ID, query.Kind)
		}
		sort.Strings(got)
		want := append([]string(nil), query.ExpectedIDs...)
		sort.Strings(want)
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("fixture %s query %s ids=%v want=%v", fixtureCase.ID, query.ID, got, want)
		}
	}
	for _, path := range fixtureCase.Expected.Paths {
		if len(fixtureCase.Registry.Sessions) == 0 {
			continue
		}
		session := sessions[fixtureCase.Registry.Sessions[0].SessionID]
		start := memory.SemanticID(path.ExpectedObjectIDs[0])
		neighborhood, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: start}, Depth: path.Depth})
		if err != nil {
			t.Fatal(err)
		}
		got := semanticNeighborhoodLinkIDs(neighborhood)
		want := make([]memory.SemanticID, len(path.ExpectedGraphLinkIDs))
		for index, id := range path.ExpectedGraphLinkIDs {
			want[index] = memory.SemanticID(id)
		}
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if semanticIDListKey(got) != semanticIDListKey(want) {
			t.Fatalf("fixture %s path %s links=%v want=%v", fixtureCase.ID, path.ID, got, want)
		}
		gotObjects := make([]string, 0, len(neighborhood.Objects))
		for _, object := range neighborhood.Objects {
			gotObjects = append(gotObjects, string(object.ObjectID))
		}
		sort.Strings(gotObjects)
		wantObjects := append([]string(nil), path.ExpectedObjectIDs...)
		sort.Strings(wantObjects)
		if strings.Join(gotObjects, "\x00") != strings.Join(wantObjects, "\x00") {
			t.Fatalf("fixture %s path %s objects=%v want=%v", fixtureCase.ID, path.ID, gotObjects, wantObjects)
		}
	}
	for _, rejection := range fixtureCase.Expected.Rejections {
		var rejectionErr error
		beforeProjection := fixtureProjectionFingerprint(t, ctx, store)
		projectionStable := true
		switch rejection.Kind {
		case "inspect_claims":
			var request struct {
				SessionID       string `json:"session_id"`
				ScopeKey        string `json:"scope_key"`
				ForeignScopeKey string `json:"foreign_scope_key"`
			}
			if err := json.Unmarshal(rejection.Request, &request); err != nil {
				t.Fatal(err)
			}
			session := sessions[request.SessionID]
			scopeKey := request.ScopeKey
			if scopeKey == "" {
				scopeKey = request.ForeignScopeKey
			}
			_, rejectionErr = store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{ScopeKey: scopeKey})
		case "remember_literal_claim":
			var request struct {
				OperationName string `json:"operation_name"`
			}
			if err := json.Unmarshal(rejection.Request, &request); err != nil {
				t.Fatal(err)
			}
			rejectionErr = rejectedOperations[request.OperationName]
			projectionStable = rejectedProjectionStable[request.OperationName]
		case "replay_operation":
			var request struct {
				SchemaVersion int `json:"schema_version"`
			}
			if err := json.Unmarshal(rejection.Request, &request); err != nil {
				t.Fatal(err)
			}
			_, rejectionErr = semanticReplayHandler(semanticAcceptedReplayOperation{SchemaVersion: request.SchemaVersion, Kind: "fixture_unknown"})
		default:
			t.Fatalf("fixture %s unsupported rejection kind %q", fixtureCase.ID, rejection.Kind)
		}
		if rejectionErr == nil {
			t.Fatalf("fixture %s rejection %s unexpectedly succeeded", fixtureCase.ID, rejection.ID)
		}
		if rejection.Kind != "remember_literal_claim" {
			projectionStable = beforeProjection == fixtureProjectionFingerprint(t, ctx, store)
		}
		if !projectionStable {
			t.Fatalf("fixture %s rejection %s changed canonical projection", fixtureCase.ID, rejection.ID)
		}
		if got := fixtureErrorCode(rejectionErr); got != rejection.ErrorCode {
			t.Fatalf("fixture %s rejection %s error_code=%s want=%s: %v", fixtureCase.ID, rejection.ID, got, rejection.ErrorCode, rejectionErr)
		}
		assertFixtureRevisions(t, ctx, db, rejection.UnchangedRevisions)
	}
}

func fixtureErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrStaleScopeRevision):
		return "stale_scope_revision"
	case strings.Contains(err.Error(), "unknown operation kind"):
		return "unsupported_operation_schema"
	case strings.Contains(err.Error(), "not available to this session"), strings.Contains(err.Error(), "outside") || strings.Contains(err.Error(), "not allowed"):
		return "scope_isolation"
	default:
		return "unknown"
	}
}

func fixtureProjectionFingerprint(t *testing.T, ctx context.Context, store *Store) string {
	t.Helper()
	verification, err := store.VerifySemanticProjection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parts := make([]string, 0, len(verification.Scopes))
	for _, scope := range verification.Scopes {
		parts = append(parts, scope.ScopeKey+"="+scope.LiveHash+"@"+fmt.Sprint(scope.LiveRevision))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func semanticIDListKey(ids []memory.SemanticID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, "\x00")
}

func runPerformanceBaseline(t *testing.T, ctx context.Context, report *memoryeval.Report) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	openStarted := time.Now()
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	coldOpen := time.Since(openStarted).Nanoseconds()
	store := NewStore(db)
	store.now = func() time.Time { return time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC) }
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	lease, err := store.AcquireTurnLease(ctx, session.ID, "evaluation-performance", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var commits []int64
	var claims []memory.RememberEntityProposal
	for index := 0; index < semanticEvaluationRepetitions; index++ {
		event := appendLifecycleEvent(t, ctx, store, session, lease, fmt.Sprintf("fixed performance claim %d", index))
		proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: evaluationKey(100 + index), SourceEventID: event.ID, Predicate: "performance_marker", PredicateLabel: "performance marker", Subject: memory.EntitySelector{Create: true, CanonicalName: fmt.Sprintf("performance subject %d", index), EntityType: "concept", Alias: fmt.Sprintf("performance subject %d", index)}, Object: memory.EntitySelector{Create: true, CanonicalName: fmt.Sprintf("performance object %d", index), EntityType: "concept", Alias: fmt.Sprintf("performance object %d", index)}})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if _, err := store.ApplyRememberEntity(ctx, lease, proposal); err != nil {
			t.Fatal(err)
		}
		commits = append(commits, time.Since(started).Nanoseconds())
		claims = append(claims, proposal)
	}
	prepareAndApplyGraphLink(t, ctx, store, session, lease, evaluationKey(40), "first performance link", memory.GraphRelationDerivation, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[0].Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[1].Claim.ID})
	prepareAndApplyGraphLink(t, ctx, store, session, lease, evaluationKey(41), "second performance link", memory.GraphRelationDerivation, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[1].Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[2].Claim.ID})
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	measure := func(action func() error) []int64 {
		values := make([]int64, 0, semanticEvaluationRepetitions)
		for index := 0; index < semanticEvaluationRepetitions; index++ {
			started := time.Now()
			if err := action(); err != nil {
				t.Fatal(err)
			}
			values = append(values, time.Since(started).Nanoseconds())
		}
		return values
	}
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("operation_commit", "ns", "warm", commits))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("current_query", "ns", "warm", measure(func() error {
		_, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
		return err
	})))
	validAt := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("historical_query", "ns", "warm", measure(func() error {
		_, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{ValidAt: &validAt, AsKnownAt: &validAt})
		return err
	})))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("provenance_query", "ns", "warm", measure(func() error {
		_, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, claims[0].Claim.ID)
		return err
	})))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("one_hop_traversal", "ns", "warm", measure(func() error {
		_, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[0].Claim.ID}, Depth: 1})
		return err
	})))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("two_hop_traversal", "ns", "warm", measure(func() error {
		_, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: claims[0].Claim.ID}, Depth: 2})
		return err
	})))

	replayDurations := measure(func() error {
		verification, err := store.VerifySemanticProjection(ctx)
		if err == nil && !verification.Valid {
			return errors.New("replay verification mismatch")
		}
		return err
	})
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("replay", "ns", "warm", replayDurations))
	operationCount := int64(len(claims) + 2)
	throughput := make([]int64, len(replayDurations))
	for index, duration := range replayDurations {
		if duration > 0 {
			throughput[index] = operationCount * int64(time.Second) / duration
		}
	}
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("replay_throughput", "operations_per_second", "warm", throughput))
	rebuildDurations := measure(func() error {
		result, err := store.OwnerRebuildSemanticProjection(ctx, fmt.Sprintf("evaluation-rebuild-%d", time.Now().UnixNano()))
		if err == nil && !result.Valid {
			return errors.New("rebuild mismatch")
		}
		return err
	})
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("rebuild", "ns", "warm", rebuildDurations))

	var sqliteVersion, journalMode string
	if err := db.QueryRowContext(ctx, `SELECT sqlite_version()`).Scan(&sqliteVersion); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	walBytes := int64(0)
	if walInfo, err := os.Stat(path + "-wal"); err == nil {
		walBytes = walInfo.Size()
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("database_growth", "bytes", "fixed_fixture", []int64{info.Size()}))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("wal_growth", "bytes", "fixed_fixture", []int64{walBytes}))

	warmOpenValues := make([]int64, 0, semanticEvaluationRepetitions)
	for index := 0; index < semanticEvaluationRepetitions; index++ {
		started := time.Now()
		reopened, err := OpenDBAt(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := reopened.PingContext(ctx); err != nil {
			t.Fatal(err)
		}
		warmOpenValues = append(warmOpenValues, time.Since(started).Nanoseconds())
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("cold_open", "ns", "cold", []int64{coldOpen}))
	report.Metrics = append(report.Metrics, memoryeval.SummarizeMetric("warm_open", "ns", "warm", warmOpenValues))
	report.Environment = memoryeval.RuntimeEnvironment(evaluationHardware(), sqliteVersion, journalMode, "busy_timeout=5000;foreign_keys=on;journal_mode=wal", semanticEvaluationRepetitions, []string{"new temporary database for cold open", "same fixed-size database for warm queries", "performance fixture: 10 entities, 5 claims, 2 links", "OS cache not flushed"})
}

func containsEntityClaim(inspection memory.EntityClaimsInspection, id memory.SemanticID) bool {
	for _, claim := range inspection.Claims {
		if claim.Claim.ID == id {
			return true
		}
	}
	return false
}

func containsClaim(inspection memory.ClaimsInspection, id memory.SemanticID) bool {
	for _, claim := range inspection.Claims {
		if claim.ID == id {
			return true
		}
	}
	return false
}

func neighborhoodHasLink(neighborhood memory.SemanticNeighborhood, id memory.SemanticID) bool {
	for _, path := range neighborhood.Paths {
		for _, link := range path.Links {
			if link.ID == id {
				return true
			}
		}
	}
	return false
}

func semanticVerificationQuarantined(verification memory.SemanticProjectionVerification, scopeKey string) bool {
	for _, scope := range verification.Scopes {
		if scope.ScopeKey == scopeKey {
			return scope.Quarantined
		}
	}
	return false
}

func manifestCardinality(manifest memoryeval.Manifest) memoryeval.Cardinality {
	result := memoryeval.Cardinality{Cases: len(manifest.Cases)}
	entities := make(map[string]bool)
	claims := make(map[string]bool)
	links := make(map[string]bool)
	for _, fixtureCase := range manifest.Cases {
		result.Sources += len(fixtureCase.Sources)
		result.Operations += len(fixtureCase.Operations)
		for _, operation := range fixtureCase.Operations {
			for name, id := range operation.GeneratedIDs {
				switch {
				case strings.Contains(name, "entity"):
					entities[id] = true
				case strings.Contains(name, "claim"):
					claims[id] = true
				case strings.Contains(name, "graph_link"):
					links[id] = true
				}
			}
		}
	}
	result.Entities, result.Claims, result.Links = len(entities), len(claims), len(links)
	return result
}

func evaluationCommit() string {
	if value := os.Getenv("EVIE_EVALUATION_COMMIT"); value != "" {
		return value
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		var revision string
		modified := false
		for _, setting := range build.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		if revision != "" {
			if modified {
				revision += "-dirty"
			}
			return revision
		}
	}
	return "working-tree"
}

func evaluationHardware() string {
	value := os.Getenv("EVIE_EVALUATION_HARDWARE")
	if value == "" {
		value = fmt.Sprintf("arch=%s;logical_cpus=%d", runtime.GOARCH, runtime.NumCPU())
	}
	return value
}

func evaluationKey(sequence int) string {
	return fmt.Sprintf("idem:v1:70000000-0000-4000-8000-%012d", sequence)
}

func freezeLiteralFixtureProposal(t *testing.T, proposal memory.RememberLiteralProposal, operation memoryeval.FixtureOperation) memory.RememberLiteralProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	if id := operation.GeneratedIDs["predicate_id"]; id != "" {
		proposal.Predicate.ID = memory.SemanticID(id)
	}
	if id := operation.GeneratedIDs["owner_entity_id"]; id != "" {
		proposal.Subject.ID = memory.SemanticID(id)
	}
	if id := operation.GeneratedIDs["evie_entity_id"]; id != "" {
		proposal.Evie.ID = memory.SemanticID(id)
	}
	proposal.ClaimID = memory.SemanticID(operation.GeneratedIDs["claim_id"])
	proposal.SourceLinkID = memory.SemanticID(operation.GeneratedIDs["source_link_id"])
	proposal.Source.ID = proposal.SourceLinkID
	proposal.Source.OperationID = proposal.OperationID
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberLiteralProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func freezeEntityFixtureProposal(t *testing.T, proposal memory.RememberEntityProposal, operation memoryeval.FixtureOperation) memory.RememberEntityProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	if id := operation.GeneratedIDs["predicate_id"]; id != "" {
		proposal.Predicate.ID = memory.SemanticID(id)
	}
	remap := make(map[memory.SemanticID]memory.SemanticID)
	for index := range proposal.Entities {
		key := ""
		switch {
		case proposal.Entities[index].AnchorKind == "owner":
			key = "owner_entity_id"
		case proposal.Entities[index].AnchorKind == "evie":
			key = "evie_entity_id"
		case proposal.Entities[index].AnchorKind == "context":
			key = "context_entity_id"
		case proposal.Entities[index].ID == proposal.Claim.SubjectEntityID:
			key = "subject_entity_id"
		case proposal.Entities[index].ID == proposal.Claim.ObjectEntityID:
			key = "object_entity_id"
		}
		if id := operation.GeneratedIDs[key]; id != "" {
			old := proposal.Entities[index].ID
			proposal.Entities[index].ID = memory.SemanticID(id)
			remap[old] = proposal.Entities[index].ID
		}
	}
	for index := range proposal.Aliases {
		key := "subject_alias_id"
		if proposal.Aliases[index].EntityID == proposal.Claim.ObjectEntityID {
			key = "object_alias_id"
		}
		if id := operation.GeneratedIDs[key]; id != "" {
			proposal.Aliases[index].ID = memory.SemanticID(id)
		}
		if id, ok := remap[proposal.Aliases[index].EntityID]; ok {
			proposal.Aliases[index].EntityID = id
		}
		proposal.Aliases[index].OperationID = proposal.OperationID
	}
	if id := operation.GeneratedIDs["subject_entity_id"]; id != "" {
		proposal.Claim.SubjectEntityID = memory.SemanticID(id)
	}
	if id := operation.GeneratedIDs["object_entity_id"]; id != "" {
		proposal.Claim.ObjectEntityID = memory.SemanticID(id)
	}
	if id := operation.GeneratedIDs["claim_id"]; id != "" {
		proposal.Claim.ID = memory.SemanticID(id)
	}
	proposal.Claim.PredicateID = proposal.Predicate.ID
	if id := operation.GeneratedIDs["source_link_id"]; id != "" {
		proposal.Source.ID = memory.SemanticID(id)
	}
	proposal.Source.OperationID = proposal.OperationID
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func freezeCorrectionFixtureProposal(t *testing.T, proposal memory.CorrectClaimProposal, operation memoryeval.FixtureOperation) memory.CorrectClaimProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	proposal.ReplacementClaim.ID = memory.SemanticID(operation.GeneratedIDs["claim_id"])
	proposal.ReplacementClaim.CreatedOperationID = proposal.OperationID
	proposal.Source.ID = memory.SemanticID(operation.GeneratedIDs["source_link_id"])
	proposal.Source.OperationID = proposal.OperationID
	for index := range proposal.Transitions {
		switch proposal.Transitions[index].ObjectKind {
		case "claim":
			if proposal.Transitions[index].ObjectID != proposal.OldClaim.ID {
				proposal.Transitions[index].ObjectID = proposal.ReplacementClaim.ID
			}
		case "source_link":
			proposal.Transitions[index].ObjectID = proposal.Source.ID
		}
	}
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalCorrectClaimProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func freezeLifecycleFixtureProposal(t *testing.T, proposal memory.MemoryLifecycleProposal, operation memoryeval.FixtureOperation) memory.MemoryLifecycleProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalMemoryLifecycleProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func freezePromotionFixtureProposal(t *testing.T, proposal memory.PromotionProposal, operation memoryeval.FixtureOperation) memory.PromotionProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	proposal.DestinationClaim.ID = memory.SemanticID(operation.GeneratedIDs["claim_id"])
	proposal.DestinationClaim.CreatedOperationID = proposal.OperationID
	for index := range proposal.Sources {
		if id := operation.GeneratedIDs["source_link_id"]; id != "" {
			proposal.Sources[index].ID = memory.SemanticID(id)
		}
		proposal.Sources[index].OperationID = proposal.OperationID
	}
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func freezeGraphFixtureProposal(t *testing.T, proposal memory.CreateGraphLinkProposal, operation memoryeval.FixtureOperation) memory.CreateGraphLinkProposal {
	t.Helper()
	proposal.OperationID = memory.SemanticID(operation.OperationID)
	proposal.Link.ID = memory.SemanticID(operation.GeneratedIDs["graph_link_id"])
	proposal.Link.CreatedOperationID = proposal.OperationID
	var err error
	proposal.ProposalSHA256, _, err = semanticHash(canonicalCreateGraphLinkProposal(proposal))
	if err != nil {
		t.Fatal(err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func queryEvaluationGraphLinkIDs(t *testing.T, ctx context.Context, db *sql.DB, start memory.SemanticID) []memory.SemanticID {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE walk(object_id, depth, graph_link_id) AS (
			SELECT target_id, 1, graph_link_id FROM semantic_graph_links
			WHERE source_kind = 'claim' AND source_id = ? AND lifecycle = 'active'
			UNION ALL
			SELECT links.target_id, walk.depth + 1, links.graph_link_id
			FROM walk JOIN semantic_graph_links AS links
			  ON links.source_kind = 'claim' AND links.source_id = walk.object_id AND links.lifecycle = 'active'
			WHERE walk.depth < 2
		)
		SELECT DISTINCT graph_link_id FROM walk ORDER BY graph_link_id
	`, start)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func semanticNeighborhoodLinkIDs(neighborhood memory.SemanticNeighborhood) []memory.SemanticID {
	seen := make(map[memory.SemanticID]bool)
	for _, path := range neighborhood.Paths {
		for _, link := range path.Links {
			seen[link.ID] = true
		}
	}
	ids := make([]memory.SemanticID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
