package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSourceLifecycleControlsSupportWithoutChangingClaimLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	clock := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claim := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:91000000-0000-4000-8000-000000000001", "Detroit")

	retractEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retract that source")
	retract, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000002",
		SourceEventID:  retractEvent.ID, Action: memory.LifecycleRetractSource,
		ObjectKind: memory.SemanticObjectSourceLink, ObjectID: claim.SourceLinkID,
	})
	if err != nil {
		t.Fatalf("prepare source retraction: %v", err)
	}
	if retract.SchemaVersion != 3 || retract.Kind != "retract_source" ||
		retract.ExpectedState != memory.SemanticStateEligible || len(retract.Transitions) != 1 ||
		retract.Transitions[0] != (memory.SemanticTransition{ObjectKind: "source_link", ObjectID: claim.SourceLinkID, State: memory.SemanticStateRetracted}) ||
		retract.Evidence.EventID != retractEvent.ID || retract.Scope.Revision != 1 {
		t.Fatalf("source retraction proposal = %+v", retract)
	}
	clock = clock.Add(time.Second)
	retracted, err := store.ApplyMemoryLifecycle(ctx, lease, retract)
	if err != nil {
		t.Fatalf("apply source retraction: %v", err)
	}
	knownBefore, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{AsKnownAt: &claim.TransactionTime})
	if err != nil || len(knownBefore.Claims) != 1 {
		t.Fatalf("Claim before source retraction: result=%+v error=%v", knownBefore, err)
	}
	knownRetracted, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{AsKnownAt: &retracted.TransactionTime})
	if err != nil || len(knownRetracted.Claims) != 0 {
		t.Fatalf("Claim as known after source retraction: result=%+v error=%v", knownRetracted, err)
	}
	illegalRetractEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retract the same source again")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000005", SourceEventID: illegalRetractEvent.ID,
		Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: claim.SourceLinkID,
	}); err == nil {
		t.Fatal("second Source Link retraction unexpectedly prepared")
	}
	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Claims) != 0 || current.ScopeRevision != 2 {
		t.Fatalf("unsupported Claim remained current: %+v", current)
	}
	exact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, claim.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Status != memory.SemanticStatusUnsupported || len(exact.Lifecycle) != 1 ||
		exact.Lifecycle[0].State != memory.SemanticStateActive || len(exact.Sources) != 1 ||
		len(exact.Sources[0].Lifecycle) != 2 || exact.Sources[0].Lifecycle[1].State != memory.SemanticStateRetracted ||
		exact.Sources[0].Source.Eligibility != memory.EligibilityRetracted ||
		len(exact.Operations) != 2 {
		t.Fatalf("unsupported exact inspection = %+v", exact)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	clock = clock.Add(time.Second)
	store.now = func() time.Time { return clock }
	exact, err = store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectSourceLink, claim.SourceLinkID)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Status != memory.SemanticStatusSourceRetracted || len(exact.Lifecycle) != 2 || exact.Source == nil ||
		exact.Source.Eligibility != memory.EligibilityRetracted {
		t.Fatalf("reopened source inspection = %+v", exact)
	}
	staleEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire unsupported claim")
	stale, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000004", SourceEventID: staleEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: claim.ClaimID,
	})
	if err != nil {
		t.Fatalf("prepare stale lifecycle basis: %v", err)
	}

	restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore that source")
	restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000003",
		SourceEventID:  restoreEvent.ID, Action: memory.LifecycleRestoreSource,
		ObjectKind: memory.SemanticObjectSourceLink, ObjectID: claim.SourceLinkID,
	})
	if err != nil {
		t.Fatalf("prepare source restoration: %v", err)
	}
	restored, err := store.ApplyMemoryLifecycle(ctx, lease, restore)
	if err != nil {
		t.Fatalf("apply source restoration: %v", err)
	}
	knownRestored, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{AsKnownAt: &restored.TransactionTime})
	if err != nil || len(knownRestored.Claims) != 1 || knownRestored.Claims[0].ID != claim.ClaimID {
		t.Fatalf("Claim as known after source restoration: result=%+v error=%v", knownRestored, err)
	}
	current, err = store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Claims) != 1 || current.Claims[0].ID != claim.ClaimID || len(current.Claims[0].Lifecycle) != 1 {
		t.Fatalf("restored source did not recover unchanged Claim: %+v", current)
	}
	restoredExact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectSourceLink, claim.SourceLinkID)
	if err != nil || restoredExact.Source == nil || restoredExact.Source.ID != claim.SourceLinkID ||
		restoredExact.Source.Eligibility != memory.EligibilityEligible ||
		restoredExact.Source.EventID != current.Claims[0].Sources[0].EventID ||
		restoredExact.Source.EvidenceSHA256 != current.Claims[0].Sources[0].EvidenceSHA256 ||
		len(restoredExact.Lifecycle) != 3 {
		t.Fatalf("restored Source Link identity/history: result=%+v error=%v", restoredExact, err)
	}
	illegalRestoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore the same source again")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000006", SourceEventID: illegalRestoreEvent.ID,
		Action: memory.LifecycleRestoreSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: claim.SourceLinkID,
	}); err == nil {
		t.Fatal("second Source Link restoration unexpectedly prepared")
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, stale); !errors.Is(err, ErrStaleScopeRevision) {
		t.Fatalf("stale lifecycle apply error = %v, want ErrStaleScopeRevision", err)
	}
}

func TestMultipleSourcesKeepClaimSupportedWhenOneIsRetracted(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-multi-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:92000000-0000-4000-8000-000000000001", "Detroit")
	second := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:92000000-0000-4000-8000-000000000002", "Detroit")
	if second.ClaimID != first.ClaimID || second.SourceLinkID == first.SourceLinkID {
		t.Fatalf("equivalent evidence did not reuse Claim: first=%+v second=%+v", first, second)
	}
	event := appendLifecycleEvent(t, ctx, store, session, lease, "retract first source only")
	proposal, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:92000000-0000-4000-8000-000000000003", SourceEventID: event.ID,
		Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: first.SourceLinkID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Claims) != 1 || len(current.Claims[0].Sources) != 1 || current.Claims[0].Sources[0].ID != second.SourceLinkID {
		t.Fatalf("independently supported Claim = %+v", current)
	}
}

func TestEntityRetirementEnumeratesDependentsAtomicallyAndRestoresLatestState(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-entity", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rememberEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Alice mentors Bob")
	remember, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93000000-0000-4000-8000-000000000001", SourceEventID: rememberEvent.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := store.ApplyRememberEntity(ctx, lease, remember)
	if err != nil {
		t.Fatal(err)
	}
	var alice memory.SemanticEntity
	var aliceAlias memory.SemanticAlias
	for _, entity := range remember.Entities {
		if entity.CanonicalName == "Alice" {
			alice = entity
		}
	}
	for _, alias := range remember.Aliases {
		if alias.EntityID == alice.ID {
			aliceAlias = alias
		}
	}
	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire Alice and her dependents")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93000000-0000-4000-8000-000000000002", SourceEventID: retireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity, ObjectID: alice.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantTransitions := []memory.SemanticTransition{
		{ObjectKind: "entity", ObjectID: alice.ID, State: memory.SemanticStateRetired},
		{ObjectKind: "alias", ObjectID: aliceAlias.ID, State: memory.SemanticStateRetired},
		{ObjectKind: "claim", ObjectID: accepted.ClaimID, State: memory.SemanticStateRetired},
	}
	if len(retire.Transitions) != len(wantTransitions) {
		t.Fatalf("Entity retirement transitions = %+v, want %+v", retire.Transitions, wantTransitions)
	}
	for i := range wantTransitions {
		if retire.Transitions[i] != wantTransitions[i] {
			t.Fatalf("Entity retirement transitions = %+v, want %+v", retire.Transitions, wantTransitions)
		}
	}

	omitted := retire
	omitted.Transitions = append([]memory.SemanticTransition(nil), retire.Transitions[:1]...)
	omitted.ProposalSHA256, _, err = semanticHash(canonicalMemoryLifecycleProposal(omitted))
	if err != nil {
		t.Fatal(err)
	}
	omitted.PreparedSHA256, _, err = semanticHash(omitted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, omitted); err == nil {
		t.Fatal("Entity retirement with omitted dependencies unexpectedly applied")
	}
	before, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil || len(before.Claims) != 1 || before.ScopeRevision != 1 {
		t.Fatalf("omitted retirement changed state: result=%+v error=%v", before, err)
	}

	if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
		t.Fatalf("apply Entity retirement: %v", err)
	}
	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil || len(current.Claims) != 0 {
		t.Fatalf("retired dependent Claim remained current: result=%+v error=%v", current, err)
	}
	if matches, err := store.LookupEntitiesByAlias(ctx, session.ScopeContext(), "Alice"); err != nil || len(matches) != 0 {
		t.Fatalf("retired Alias remained resolvable: matches=%+v error=%v", matches, err)
	}

	aliasRestoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore only Alice alias")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93000000-0000-4000-8000-000000000003", SourceEventID: aliasRestoreEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectAlias, ObjectID: aliceAlias.ID,
	}); err == nil {
		t.Fatal("Alias restored while its Entity remained retired")
	}
	restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore Alice and latest eligible dependents")
	restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93000000-0000-4000-8000-000000000004", SourceEventID: restoreEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectEntity, ObjectID: alice.ID,
	})
	if err != nil {
		t.Fatalf("prepare Entity restoration: %v", err)
	}
	if len(restore.Transitions) != 3 || restore.Transitions[0].State != memory.SemanticStateActive ||
		restore.Transitions[1].State != memory.SemanticStateActive || restore.Transitions[2].State != memory.SemanticStateActive {
		t.Fatalf("Entity restoration transitions = %+v", restore.Transitions)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
		t.Fatalf("apply Entity restoration: %v", err)
	}
	current, err = store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil || len(current.Claims) != 1 || current.Claims[0].ID != accepted.ClaimID {
		t.Fatalf("restored current Claim: result=%+v error=%v", current, err)
	}
	exact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectEntity, alice.ID)
	if err != nil || exact.Status != memory.SemanticStatusActive || len(exact.Lifecycle) != 3 || len(exact.Operations) != 3 {
		t.Fatalf("restored Entity inspection: result=%+v error=%v", exact, err)
	}
}

func TestContextEntityRetirementCarriesSessionDependentClaimsInOneRevisionVector(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Lifecycle dependencies")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{
		ID: "93500000-0000-4000-8000-000000000002", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-cross-scope", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	contextEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Alice knows Bob in this workspace")
	contextRemember, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000001", SourceEventID: contextEvent.ID,
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, contextRemember); err != nil {
		t.Fatal(err)
	}
	sessionEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Alice knows Bob for only this session")
	sessionRemember, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000003", SourceEventID: sessionEvent.ID,
		Predicate: "knows", PredicateLabel: "knows", UseSessionScope: true,
		Subject: memory.EntitySelector{EntityID: contextRemember.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: contextRemember.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionResult, err := store.ApplyRememberEntity(ctx, lease, sessionRemember)
	if err != nil {
		t.Fatal(err)
	}
	if sessionRemember.Claim.ScopeKey != "session:"+string(session.ID) {
		t.Fatalf("session dependent Claim = %+v", sessionRemember.Claim)
	}

	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire workspace Alice and every dependent")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000004", SourceEventID: retireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity, ObjectID: contextRemember.Claim.SubjectEntityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceKey := "workspace:" + string(workspace.ID)
	sessionKey := "session:" + string(session.ID)
	if got := semanticScopeKeys(retire.Scopes); !stringSlicesEqual(got, []string{"global", sessionKey, workspaceKey}) {
		t.Fatalf("cross-scope retirement scopes = %v", got)
	}
	if !stringSlicesEqual(retire.EffectScopes, []string{sessionKey, workspaceKey}) ||
		!stringSlicesEqual(canonicalMemoryLifecycleProposal(retire).Effect.Scopes, retire.EffectScopes) {
		t.Fatalf("cross-scope retirement effect scopes = proposal %v effect %v", retire.EffectScopes,
			canonicalMemoryLifecycleProposal(retire).Effect.Scopes)
	}
	foundScopedClaim := false
	for _, transition := range retire.Transitions {
		if transition.ObjectKind == "claim" && transition.ObjectID == sessionResult.ClaimID {
			foundScopedClaim = true
		}
	}
	if !foundScopedClaim {
		t.Fatalf("cross-scope retirement omitted scoped Claim: %+v", retire.Transitions)
	}
	retired, err := store.ApplyMemoryLifecycle(ctx, lease, retire)
	if err != nil {
		t.Fatal(err)
	}
	if len(retired.ResultingRevisions) != 3 || retired.ResultingRevisions[0].Revision != 1 ||
		retired.ResultingRevisions[1].Revision != 2 || retired.ResultingRevisions[2].Revision != 2 {
		t.Fatalf("cross-scope resulting revisions = %+v", retired.ResultingRevisions)
	}
	current, err := store.InspectEntityClaimsAtScope(ctx, session.ScopeContext(), true)
	if err != nil || len(current.Claims) != 0 {
		t.Fatalf("retired cross-scope Claim remained current: result=%+v error=%v", current, err)
	}

	restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore Alice and the same exact dependents")
	restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000005", SourceEventID: restoreEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectEntity, ObjectID: contextRemember.Claim.SubjectEntityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
		t.Fatal(err)
	}
	current, err = store.InspectEntityClaimsAtScope(ctx, session.ScopeContext(), true)
	if err != nil || len(current.Claims) != 1 || current.Claims[0].Claim.ID != sessionResult.ClaimID {
		t.Fatalf("restored cross-scope Claims: result=%+v error=%v", current, err)
	}
	contextExact, err := store.InspectSemanticObjectAtScope(ctx, session.ScopeContext(), memory.SemanticObjectEntity,
		contextRemember.Claim.SubjectEntityID, true)
	if err != nil || contextExact.Scope.Key != workspaceKey || contextExact.Status != memory.SemanticStatusActive {
		t.Fatalf("session-view Context Entity inspection: result=%+v error=%v", contextExact, err)
	}
	var globalEntityID memory.SemanticID
	for _, entity := range contextRemember.Entities {
		if entity.ScopeKey == "global" {
			globalEntityID = entity.ID
			break
		}
	}
	globalExact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectEntity, globalEntityID)
	if err != nil || globalExact.Scope.Key != "global" {
		t.Fatalf("Context-view global Entity inspection: result=%+v error=%v", globalExact, err)
	}
	if _, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, sessionResult.ClaimID); err == nil {
		t.Fatal("default Context inspection unexpectedly read current-session Claim")
	}
	sessionExact, err := store.InspectSemanticObjectAtScope(ctx, session.ScopeContext(), memory.SemanticObjectClaim,
		sessionResult.ClaimID, true)
	if err != nil || sessionExact.Scope.Key != sessionKey || sessionExact.Status != memory.SemanticStatusActive {
		t.Fatalf("current-session Claim inspection: result=%+v error=%v", sessionExact, err)
	}

	sessionRetireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire only the current-session Claim")
	sessionRetire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000006", SourceEventID: sessionRetireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: sessionResult.ClaimID,
		UseSessionScope: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sessionRetire.Scope.Key != sessionKey || !stringSlicesEqual(sessionRetire.EffectScopes, []string{sessionKey}) {
		t.Fatalf("current-session retirement scope = %+v / %v", sessionRetire.Scope, sessionRetire.EffectScopes)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, sessionRetire); err != nil {
		t.Fatal(err)
	}
	sessionExact, err = store.InspectSemanticObjectAtScope(ctx, session.ScopeContext(), memory.SemanticObjectClaim,
		sessionResult.ClaimID, true)
	if err != nil || sessionExact.Status != memory.SemanticStatusRetired {
		t.Fatalf("retired current-session Claim inspection: result=%+v error=%v", sessionExact, err)
	}
	sessionRestoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore the current-session Claim")
	sessionRestore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000007", SourceEventID: sessionRestoreEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectClaim, ObjectID: sessionResult.ClaimID,
		UseSessionScope: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, sessionRestore); err != nil {
		t.Fatal(err)
	}

	sibling := memory.Session{
		ID: "93500000-0000-4000-8000-000000000009", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, sibling.ID, sibling.WorkspaceID, sibling.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	siblingLease, err := store.AcquireTurnLease(ctx, sibling.ID, "lifecycle-sibling-session", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	siblingEvent := appendLifecycleEvent(t, ctx, store, sibling, siblingLease, "Alice knows Bob in a sibling private session")
	siblingRemember, err := store.PrepareRememberEntity(ctx, sibling.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000010", SourceEventID: siblingEvent.ID,
		Predicate: "knows", PredicateLabel: "knows", UseSessionScope: true,
		Subject: memory.EntitySelector{EntityID: contextRemember.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: contextRemember.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, siblingLease, siblingRemember); err != nil {
		t.Fatal(err)
	}
	beforeCurrent, err := store.InspectEntityClaimsAtScope(ctx, session.ScopeContext(), true)
	if err != nil {
		t.Fatal(err)
	}
	beforeSibling, err := store.InspectEntityClaimsAtScope(ctx, sibling.ScopeContext(), true)
	if err != nil {
		t.Fatal(err)
	}
	blockedEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire Alice without exposing sibling session memory")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93500000-0000-4000-8000-000000000011", SourceEventID: blockedEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity,
		ObjectID: contextRemember.Claim.SubjectEntityID,
	}); err == nil {
		t.Fatal("Entity retirement unexpectedly derived authority from a sibling session dependency")
	}
	afterCurrent, err := store.InspectEntityClaimsAtScope(ctx, session.ScopeContext(), true)
	if err != nil {
		t.Fatal(err)
	}
	afterSibling, err := store.InspectEntityClaimsAtScope(ctx, sibling.ScopeContext(), true)
	if err != nil {
		t.Fatal(err)
	}
	if afterCurrent.ScopeRevision != beforeCurrent.ScopeRevision || len(afterCurrent.Claims) != len(beforeCurrent.Claims) ||
		afterSibling.ScopeRevision != beforeSibling.ScopeRevision || len(afterSibling.Claims) != len(beforeSibling.Claims) {
		t.Fatalf("blocked sibling dependency changed state: current=%+v sibling=%+v", afterCurrent, afterSibling)
	}
	entityAfterBlock, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectEntity,
		contextRemember.Claim.SubjectEntityID)
	if err != nil || entityAfterBlock.Status != memory.SemanticStatusActive {
		t.Fatalf("blocked sibling dependency retired Entity: result=%+v error=%v", entityAfterBlock, err)
	}
}

func TestGlobalEntityRetirementFailsWithoutReadingCrossContextDependencies(t *testing.T) {
	ctx := context.Background()
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
	globalLease, err := store.AcquireTurnLease(ctx, global.ID, "lifecycle-global-isolation", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	globalEvent := appendLifecycleEvent(t, ctx, store, global, globalLease, "Alice knows Bob globally")
	globalRemember, err := store.PrepareRememberEntity(ctx, global.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93600000-0000-4000-8000-000000000001", SourceEventID: globalEvent.ID,
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, globalLease, globalRemember); err != nil {
		t.Fatal(err)
	}

	workspace, err := store.RegisterWorkspace(ctx, "Private context")
	if err != nil {
		t.Fatal(err)
	}
	private := memory.Session{
		ID: "93600000-0000-4000-8000-000000000002", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, private.ID, private.WorkspaceID, private.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	privateLease, err := store.AcquireTurnLease(ctx, private.ID, "lifecycle-private-context", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	privateEvent := appendLifecycleEvent(t, ctx, store, private, privateLease, "Alice knows Bob in a private workspace")
	privateRemember, err := store.PrepareRememberEntity(ctx, private.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:93600000-0000-4000-8000-000000000003", SourceEventID: privateEvent.ID,
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{EntityID: globalRemember.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: globalRemember.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, privateLease, privateRemember); err != nil {
		t.Fatal(err)
	}
	beforeGlobal, err := store.InspectEntityClaims(ctx, global.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	beforePrivate, err := store.InspectEntityClaims(ctx, private.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	blockedEvent := appendLifecycleEvent(t, ctx, store, global, globalLease, "retire Alice without reading private Context memory")
	if _, err := store.PrepareMemoryLifecycle(ctx, global.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:93600000-0000-4000-8000-000000000004", SourceEventID: blockedEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity,
		ObjectID: globalRemember.Claim.SubjectEntityID,
	}); err == nil {
		t.Fatal("global Entity retirement unexpectedly derived authority from private Context memory")
	}
	afterGlobal, err := store.InspectEntityClaims(ctx, global.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	afterPrivate, err := store.InspectEntityClaims(ctx, private.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if afterGlobal.ScopeRevision != beforeGlobal.ScopeRevision || len(afterGlobal.Claims) != len(beforeGlobal.Claims) ||
		afterPrivate.ScopeRevision != beforePrivate.ScopeRevision || len(afterPrivate.Claims) != len(beforePrivate.Claims) {
		t.Fatalf("blocked cross-Context dependency changed state: global=%+v private=%+v", afterGlobal, afterPrivate)
	}
	exact, err := store.InspectSemanticObject(ctx, global.ScopeContext(), memory.SemanticObjectEntity,
		globalRemember.Claim.SubjectEntityID)
	if err != nil || exact.Status != memory.SemanticStatusActive {
		t.Fatalf("blocked cross-Context dependency retired global Entity: result=%+v error=%v", exact, err)
	}
}

func TestAliasAndClaimLifecycleRejectIllegalTransitionsAndSupersededRestore(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-legality", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	event := appendLifecycleEvent(t, ctx, store, session, lease, "Alice mentors Bob")
	remember, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000001", SourceEventID: event.ID,
		Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := store.ApplyRememberEntity(ctx, lease, remember)
	if err != nil {
		t.Fatal(err)
	}
	aliasID := remember.Aliases[0].ID
	for index, target := range []struct {
		kind memory.SemanticObjectKind
		id   memory.SemanticID
	}{{memory.SemanticObjectAlias, aliasID}, {memory.SemanticObjectClaim, accepted.ClaimID}} {
		retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire one object")
		retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: semanticLifecycleKey(10 + index*3), SourceEventID: retireEvent.ID,
			Action: memory.LifecycleRetire, ObjectKind: target.kind, ObjectID: target.id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
			t.Fatal(err)
		}
		illegalEvent := appendLifecycleEvent(t, ctx, store, session, lease, "repeat illegal retirement")
		if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: semanticLifecycleKey(11 + index*3), SourceEventID: illegalEvent.ID,
			Action: memory.LifecycleRetire, ObjectKind: target.kind, ObjectID: target.id,
		}); err == nil {
			t.Fatalf("second %s retirement unexpectedly prepared", target.kind)
		}
		restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore one object")
		restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: semanticLifecycleKey(12 + index*3), SourceEventID: restoreEvent.ID,
			Action: memory.LifecycleRestore, ObjectKind: target.kind, ObjectID: target.id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
			t.Fatal(err)
		}
		if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: semanticLifecycleKey(30 + index), SourceEventID: restoreEvent.ID,
			Action: memory.LifecycleRestore, ObjectKind: target.kind, ObjectID: target.id,
		}); err == nil {
			t.Fatalf("second %s restoration unexpectedly prepared", target.kind)
		}
	}

	correctionEvent := appendLifecycleEvent(t, ctx, store, session, lease, "correct the restored Claim")
	correction, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000040", SourceEventID: correctionEvent.ID,
		OldClaimID: accepted.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: remember.Claim.SubjectEntityID, PredicateID: remember.Predicate.ID,
			Object: memory.ClaimObject{EntityID: remember.Claim.ObjectEntityID}, Polarity: remember.Claim.Polarity,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCorrectClaim(ctx, lease, correction); err != nil {
		t.Fatal(err)
	}
	restoreSupersededEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore obsolete Claim")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:94000000-0000-4000-8000-000000000041", SourceEventID: restoreSupersededEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectClaim, ObjectID: accepted.ClaimID,
	}); err == nil {
		t.Fatal("superseded Claim restoration unexpectedly prepared")
	}
	exact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectClaim, accepted.ClaimID)
	if err != nil || exact.Status != memory.SemanticStatusSuperseded {
		t.Fatalf("superseded exact inspection: result=%+v error=%v", exact, err)
	}
}

func TestLiteralClaimRetiresAndRestoresWithFrozenV1AnchorDependencies(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-literal", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claim := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:94500000-0000-4000-8000-000000000001", "Detroit")
	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire literal Claim")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:94500000-0000-4000-8000-000000000002", SourceEventID: retireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: claim.ClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
		t.Fatal(err)
	}
	restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore literal Claim")
	restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:94500000-0000-4000-8000-000000000003", SourceEventID: restoreEvent.ID,
		Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectClaim, ObjectID: claim.ClaimID,
	})
	if err != nil {
		t.Fatalf("prepare literal Claim restore: %v", err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
		t.Fatalf("apply literal Claim restore: %v", err)
	}
	current, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil || len(current.Claims) != 1 || current.Claims[0].ID != claim.ClaimID || len(current.Claims[0].Lifecycle) != 3 {
		t.Fatalf("restored literal Claim: result=%+v error=%v", current, err)
	}
}

func TestLifecycleSQLFailureRollsBackOperationTransitionsAndRevision(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-rollback", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claim := acceptLifecycleLiteral(t, ctx, store, session, lease,
		"idem:v1:95000000-0000-4000-8000-000000000001", "Detroit")
	event := appendLifecycleEvent(t, ctx, store, session, lease, "retire this Claim")
	proposal, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:95000000-0000-4000-8000-000000000002", SourceEventID: event.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: claim.ClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER reject_lifecycle_retirement BEFORE INSERT ON semantic_state_events
		WHEN NEW.state = 'retired' BEGIN SELECT RAISE(ABORT, 'injected lifecycle failure'); END
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, proposal); err == nil {
		t.Fatal("injected lifecycle failure unexpectedly committed")
	}
	var operations, transitions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`, proposal.OperationID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_state_events WHERE operation_id = ?`, proposal.OperationID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	inspection, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if operations != 0 || transitions != 0 || inspection.ScopeRevision != 1 || len(inspection.Claims) != 1 {
		t.Fatalf("failed lifecycle write was not atomic: operations=%d transitions=%d inspection=%+v", operations, transitions, inspection)
	}
}

func TestSemanticOperationEncodingV3PreservesV1AndV2HistoryAcrossUpgrade(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "lifecycle-v3-migration", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	legacy := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:96000000-0000-4000-8000-000000000001", "Detroit", "Detroit", memory.ValidTime{})
	legacyResult, err := store.ApplyRememberLiteral(ctx, lease, legacy)
	if err != nil {
		t.Fatal(err)
	}
	correctionEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Correction: Chicago")
	correctionRequest := memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:96000000-0000-4000-8000-000000000002", SourceEventID: correctionEvent.ID,
		OldClaimID: legacyResult.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: legacy.Subject.ID, PredicateID: legacy.Predicate.ID,
			Object:   memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity: memory.PolarityAffirmed,
		},
	}
	correction, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), correctionRequest)
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := store.ApplyCorrectClaim(ctx, lease, correction)
	if err != nil {
		t.Fatal(err)
	}
	before := loadAcceptedOperationBytes(t, ctx, db)
	downgradeSemanticOperationsToV2(t, ctx, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("upgrade v2 operation table to v3: %v", err)
	}
	defer db.Close()
	after := loadAcceptedOperationBytes(t, ctx, db)
	if len(after) != len(before) {
		t.Fatalf("operation count after v3 upgrade = %d, want %d", len(after), len(before))
	}
	for id, want := range before {
		if got := after[id]; got != want {
			t.Fatalf("accepted operation %s changed during v3 upgrade\ngot  %+v\nwant %+v", id, got, want)
		}
	}
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'semantic_operations'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "schema_version IN (1, 2, 3)") {
		t.Fatalf("upgraded operation schema = %s", definition)
	}
	store = NewStore(db)
	legacyRetry, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), legacy.Request)
	if err != nil {
		t.Fatal(err)
	}
	legacyRetried, err := store.ApplyRememberLiteral(ctx, lease, legacyRetry)
	if err != nil || legacyRetried.OperationID != legacyResult.OperationID {
		t.Fatalf("v1 retry after v3 upgrade: result=%+v error=%v", legacyRetried, err)
	}
	correctionRetry, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), correctionRequest)
	if err != nil {
		t.Fatal(err)
	}
	correctedRetry, err := store.ApplyCorrectClaim(ctx, lease, correctionRetry)
	if err != nil || correctedRetry.OperationID != corrected.OperationID {
		t.Fatalf("v2 retry after v3 upgrade: result=%+v error=%v", correctedRetry, err)
	}
	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire corrected Claim")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:96000000-0000-4000-8000-000000000003", SourceEventID: retireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: corrected.ReplacementClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if retire.SchemaVersion != 3 {
		t.Fatalf("lifecycle schema version = %d, want 3", retire.SchemaVersion)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
		t.Fatal(err)
	}
}

type acceptedOperationBytes struct {
	SchemaVersion                                                                     int
	Kind, Key, Actor, SessionID, ScopeID, SourceEventID                               string
	ProposalHash, EffectHash, ProposalJSON, PreparedJSON, ResultJSON, TransactionTime string
}

func loadAcceptedOperationBytes(t *testing.T, ctx context.Context, db *sql.DB) map[string]acceptedOperationBytes {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT operation_id, schema_version, operation_kind, idempotency_key, actor, session_id,
		       target_scope_id, source_event_id, proposal_sha256, effect_sha256,
		       proposal_json, prepared_proposal_json, result_json, transaction_time
		FROM semantic_operations ORDER BY operation_id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[string]acceptedOperationBytes)
	for rows.Next() {
		var id string
		var operation acceptedOperationBytes
		if err := rows.Scan(&id, &operation.SchemaVersion, &operation.Kind, &operation.Key, &operation.Actor,
			&operation.SessionID, &operation.ScopeID, &operation.SourceEventID, &operation.ProposalHash,
			&operation.EffectHash, &operation.ProposalJSON, &operation.PreparedJSON, &operation.ResultJSON,
			&operation.TransactionTime); err != nil {
			t.Fatal(err)
		}
		result[id] = operation
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func downgradeSemanticOperationsToV2(t *testing.T, ctx context.Context, db *sql.DB) {
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
	if _, err := conn.ExecContext(ctx, semanticOperationsV2Downgrade); err != nil {
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

const semanticOperationsV2Downgrade = `
DROP TRIGGER semantic_operations_append_only_update;
DROP TRIGGER semantic_operations_append_only_delete;
CREATE TABLE semantic_operations_v2_only (
    operation_id TEXT PRIMARY KEY NOT NULL,
    schema_version INTEGER NOT NULL CHECK (schema_version IN (1, 2)),
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
INSERT INTO semantic_operations_v2_only SELECT * FROM semantic_operations;
DROP TABLE semantic_operations;
ALTER TABLE semantic_operations_v2_only RENAME TO semantic_operations;
CREATE TRIGGER semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
CREATE TRIGGER semantic_operations_append_only_delete BEFORE DELETE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
`

func semanticLifecycleKey(index int) string {
	return fmt.Sprintf("idem:v1:94000000-0000-4000-8000-%012d", index)
}

func acceptLifecycleLiteral(t *testing.T, ctx context.Context, store *Store, session memory.Session,
	lease memory.TurnLease, key, value string,
) memory.RememberLiteralResult {
	t.Helper()
	event := appendLifecycleEvent(t, ctx, store, session, lease, "remember home city "+value)
	proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: key, SourceEventID: event.ID, Predicate: "home_city", PredicateLabel: "home city",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
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

func appendLifecycleEvent(t *testing.T, ctx context.Context, store *Store, session memory.Session,
	lease memory.TurnLease, content string,
) memory.Event {
	t.Helper()
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
