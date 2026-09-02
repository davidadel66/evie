package eviedb

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func appendExactSemanticApproval(t *testing.T, ctx context.Context, store *Store, lease memory.TurnLease, parent memory.EventID, operation memory.SemanticID, proposalHash, preparedHash string) {
	t.Helper()
	payload, err := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: proposalHash, PreparedSHA256: preparedHash})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEventWithLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: parent, Type: memory.EventApproval, ExecutionID: memory.ExecutionID(operation), Payload: payload}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id = ? AND event_type = 'approval' AND execution_id = ? AND parent_id = ?`, lease.SessionID, operation, parent).Scan(&count); err != nil || count != 1 {
		t.Fatalf("persisted exact approval count=%d error=%v", count, err)
	}
}

func prepareAndApplyGraphLink(t *testing.T, ctx context.Context, store *Store, session memory.Session, lease memory.TurnLease, key, command string, relation memory.GraphRelation, source, target memory.GraphEndpoint) memory.CreateGraphLinkResult {
	t.Helper()
	event := appendLifecycleEvent(t, ctx, store, session, lease, command)
	proposal, err := store.PrepareCreateGraphLink(ctx, session.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: key, SourceEventID: event.ID, Relation: relation, Source: source, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SchemaVersion != 5 || proposal.Kind != "create_graph_link" || proposal.Link.CreatedOperationID != proposal.OperationID || proposal.Link.ScopeKey != scopeKeyForContext(session.ScopeContext()) {
		t.Fatalf("Graph Link proposal = %+v", proposal)
	}
	appendExactSemanticApproval(t, ctx, store, lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	result, err := store.ApplyCreateGraphLink(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestGraphLinksUseClosedStructuralRelationsAndAppendOnlyApprovedLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	clock := time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := rememberScopeClaim(t, ctx, store, session, false, 201)
	second := rememberScopeClaim(t, ctx, store, session, false, 202)
	third := rememberScopeClaim(t, ctx, store, session, false, 203)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "graph-links", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	badEvent := appendLifecycleEvent(t, ctx, store, session, lease, "Alice mentors Bob")
	if _, err := store.PrepareCreateGraphLink(ctx, session.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000001", SourceEventID: badEvent.ID, Relation: "mentors", Source: memory.GraphEndpoint{Kind: memory.SemanticObjectEntity, ID: first.Claim.SubjectEntityID}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectEntity, ID: first.Claim.ObjectEntityID}}); err == nil {
		t.Fatal("ordinary Entity relationship unexpectedly became a Graph Link")
	}

	event := appendLifecycleEvent(t, ctx, store, session, lease, "recognize an explicit contradiction")
	proposal, err := store.PrepareCreateGraphLink(ctx, session.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000002", SourceEventID: event.ID, Relation: memory.GraphRelationContradiction, Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyCreateGraphLink(ctx, lease, proposal); err == nil {
		t.Fatal("unapproved Graph Link unexpectedly applied")
	}
	appendExactSemanticApproval(t, ctx, store, lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	created, err := store.ApplyCreateGraphLink(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	derived := prepareAndApplyGraphLink(t, ctx, store, session, lease, "idem:v1:99000000-0000-4000-8000-000000000003", "derive another claim", memory.GraphRelationDerivation, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID})

	exact, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, created.GraphLinkID)
	if err != nil {
		t.Fatal(err)
	}
	if exact.GraphLink == nil || exact.GraphLink.Relation != memory.GraphRelationContradiction || exact.Status != memory.SemanticStatusActive || len(exact.Lifecycle) != 1 || len(exact.Operations) != 1 || exact.Metadata.ValidAt.IsZero() || len(exact.Metadata.AllowedScopes) != 2 {
		t.Fatalf("created Graph Link inspection = %+v", exact)
	}
	entityRetireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire the linked Entity")
	entityRetire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000006", SourceEventID: entityRetireEvent.ID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity, ObjectID: first.Claim.SubjectEntityID})
	if err != nil {
		t.Fatal(err)
	}
	if entityRetire.Transitions[len(entityRetire.Transitions)-1].ObjectKind != "graph_link" || entityRetire.Transitions[len(entityRetire.Transitions)-1].ObjectID != created.GraphLinkID {
		t.Fatalf("Entity retirement omitted dependent Graph Link: %+v", entityRetire.Transitions)
	}
	if entityRetire.SchemaVersion != 5 {
		t.Fatalf("compound Graph Link retirement schema = %d", entityRetire.SchemaVersion)
	}
	appendExactSemanticApproval(t, ctx, store, lease, entityRetireEvent.ID, entityRetire.OperationID, entityRetire.ProposalSHA256, entityRetire.PreparedSHA256)
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, entityRetire); err != nil {
		t.Fatal(err)
	}
	retiredByEntity, err := store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, created.GraphLinkID)
	if err != nil || retiredByEntity.Status != memory.SemanticStatusRetired {
		t.Fatalf("Entity-dependent Graph Link retirement: result=%+v error=%v", retiredByEntity, err)
	}
	entityRestoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore the linked Entity")
	entityRestore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000007", SourceEventID: entityRestoreEvent.ID, Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectEntity, ObjectID: first.Claim.SubjectEntityID})
	if err != nil {
		t.Fatal(err)
	}
	if entityRestore.SchemaVersion != 5 {
		t.Fatalf("compound Graph Link restoration schema = %d", entityRestore.SchemaVersion)
	}
	appendExactSemanticApproval(t, ctx, store, lease, entityRestoreEvent.ID, entityRestore.OperationID, entityRestore.ProposalSHA256, entityRestore.PreparedSHA256)
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, entityRestore); err != nil {
		t.Fatal(err)
	}

	clock = clock.Add(time.Second)
	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire the derivation")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000004", SourceEventID: retireEvent.ID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectGraphLink, ObjectID: derived.GraphLinkID})
	if err != nil {
		t.Fatal(err)
	}
	if retire.SchemaVersion != 5 {
		t.Fatalf("Graph Link lifecycle schema = %d", retire.SchemaVersion)
	}
	appendExactSemanticApproval(t, ctx, store, lease, retireEvent.ID, retire.OperationID, retire.ProposalSHA256, retire.PreparedSHA256)
	retired, err := store.ApplyMemoryLifecycle(ctx, lease, retire)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	restoreEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore the derivation")
	restore, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000005", SourceEventID: restoreEvent.ID, Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectGraphLink, ObjectID: derived.GraphLinkID})
	if err != nil {
		t.Fatal(err)
	}
	appendExactSemanticApproval(t, ctx, store, lease, restoreEvent.ID, restore.OperationID, restore.ProposalSHA256, restore.PreparedSHA256)
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, restore); err != nil {
		t.Fatal(err)
	}
	createdAt := derived.TransactionTime
	activeAtCreation, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, derived.GraphLinkID, memory.ClaimQuery{AsKnownAt: &createdAt})
	if err != nil || activeAtCreation.Status != memory.SemanticStatusActive || len(activeAtCreation.Lifecycle) != 1 {
		t.Fatalf("Graph Link inspection at creation: result=%+v error=%v", activeAtCreation, err)
	}
	retiredAt := retired.TransactionTime
	retiredAtTime, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, derived.GraphLinkID, memory.ClaimQuery{AsKnownAt: &retiredAt})
	if err != nil || retiredAtTime.Status != memory.SemanticStatusRetired || len(retiredAtTime.Lifecycle) != 2 {
		t.Fatalf("Graph Link inspection at retirement: result=%+v error=%v", retiredAtTime, err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
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
	store = NewStore(db)
	store.now = func() time.Time { return clock.Add(time.Second) }
	exact, err = store.InspectSemanticObject(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, derived.GraphLinkID)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Status != memory.SemanticStatusActive || len(exact.Lifecycle) != 3 || len(exact.Operations) != 3 {
		t.Fatalf("reopened Graph Link lifecycle = %+v", exact)
	}
	if !retired.TransactionTime.Before(exact.Lifecycle[2].TransactionTime) {
		t.Fatalf("restore history not ordered: %+v", exact.Lifecycle)
	}
	reopenedRetired, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectGraphLink, derived.GraphLinkID, memory.ClaimQuery{AsKnownAt: &retiredAt})
	if err != nil || reopenedRetired.Status != retiredAtTime.Status || len(reopenedRetired.Lifecycle) != len(retiredAtTime.Lifecycle) {
		t.Fatalf("reopened historical Graph Link inspection: before=%+v after=%+v error=%v", retiredAtTime, reopenedRetired, err)
	}
}

func TestExactSemanticPaginationFiltersTraversalTemporalLifecycleAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	clock := time.Date(2026, 9, 2, 21, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first := rememberScopeClaim(t, ctx, store, session, false, 211)
	second := rememberScopeClaim(t, ctx, store, session, false, 212)
	third := rememberScopeClaim(t, ctx, store, session, false, 213)
	_ = rememberScopeClaim(t, ctx, store, session, true, 214)
	page, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objects) != 1 || page.NextCursor == "" || page.Metadata.ValidAt.IsZero() || len(page.Metadata.ScopeRevisions) != 2 {
		t.Fatalf("first exact page = %+v", page)
	}
	globalOnly, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{
		ClaimQuery: memory.ClaimQuery{ScopeKey: "global", PredicateToken: "scope_marker"},
		Kinds:      []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 10,
	})
	if err != nil || globalOnly.Metadata.SelectedScope != "global" || len(globalOnly.Metadata.AllowedScopes) != 1 ||
		len(globalOnly.Objects) != 3 {
		t.Fatalf("global-only exact page: result=%+v error=%v", globalOnly, err)
	}
	sessionKey := "session:" + string(session.ID)
	sessionOnly, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{
		ClaimQuery: memory.ClaimQuery{ScopeKey: sessionKey, PredicateToken: "scope_marker"},
		Kinds:      []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 10,
	})
	if err != nil || sessionOnly.Metadata.SelectedScope != sessionKey || len(sessionOnly.Objects) != 1 {
		t.Fatalf("session-only exact page: result=%+v error=%v", sessionOnly, err)
	}
	if _, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectClaim, first.Claim.ID, memory.ClaimQuery{ScopeKey: sessionKey}); err == nil {
		t.Fatal("selected session scope inspected a global object")
	}
	var sessionScopeID string
	if err := db.QueryRowContext(ctx, `SELECT scope_id FROM semantic_scopes WHERE scope_key = ?`, sessionKey).Scan(&sessionScopeID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO semantic_projection_quarantine (scope_id, reason, verified_at) VALUES (?, 'test sibling quarantine', ?)`, sessionScopeID, formatSemanticTime(clock)); err != nil {
		t.Fatal(err)
	}
	globalDetail, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectClaim, first.Claim.ID, memory.ClaimQuery{ScopeKey: "global"})
	if err != nil || globalDetail.ObjectID != first.Claim.ID || globalDetail.Metadata.SelectedScope != "global" {
		t.Fatalf("global exact inspection with quarantined sibling: result=%+v error=%v", globalDetail, err)
	}
	if _, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{ScopeKey: sessionKey}}); !errors.Is(err, ErrSemanticScopeQuarantined) {
		t.Fatalf("selected quarantined session scope error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM semantic_projection_quarantine WHERE scope_id = ?`, sessionScopeID); err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "Selected exact-read quarantine")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	_ = rememberScopeClaim(t, ctx, store, workspaceSession, false, 215)
	workspaceKey := "workspace:" + string(workspace.ID)
	var workspaceScopeID string
	if err := db.QueryRowContext(ctx, `SELECT scope_id FROM semantic_scopes WHERE scope_key = ?`, workspaceKey).Scan(&workspaceScopeID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO semantic_projection_quarantine (scope_id, reason, verified_at) VALUES (?, 'test context quarantine', ?)`, workspaceScopeID, formatSemanticTime(clock)); err != nil {
		t.Fatal(err)
	}
	globalDetail, err = store.InspectSemanticObjectAt(ctx, workspaceSession.ScopeContext(), memory.SemanticObjectClaim, first.Claim.ID, memory.ClaimQuery{ScopeKey: "global"})
	if err != nil || globalDetail.ObjectID != first.Claim.ID || globalDetail.Metadata.SelectedScope != "global" {
		t.Fatalf("global exact inspection with quarantined Context sibling: result=%+v error=%v", globalDetail, err)
	}
	entities, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{
		ClaimQuery: memory.ClaimQuery{ScopeKey: "global"}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectEntity}, PageSize: 10,
	})
	if err != nil || len(entities.Objects) == 0 || entities.Objects[0].Entity == nil || entities.Objects[0].Entity.CanonicalName == "" {
		t.Fatalf("owner-facing Entity summaries: result=%+v error=%v", entities, err)
	}
	scopePage, err := store.ListSemanticScopes(ctx, session.ScopeContext(), memory.SemanticScopeListQuery{PageSize: 1})
	if err != nil || len(scopePage.Scopes) != 1 || scopePage.NextCursor == "" || len(scopePage.Metadata.AllowedScopes) != 2 {
		t.Fatalf("paginated scope listing: result=%+v error=%v", scopePage, err)
	}
	nextScopePage, err := store.ListSemanticScopes(ctx, session.ScopeContext(), memory.SemanticScopeListQuery{PageSize: 1, Cursor: scopePage.NextCursor})
	if err != nil || len(nextScopePage.Scopes) != 1 || nextScopePage.Scopes[0].Key == scopePage.Scopes[0].Key {
		t.Fatalf("second scope page: result=%+v error=%v", nextScopePage, err)
	}
	secondPage, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Objects) != 1 || secondPage.Objects[0].ObjectID == page.Objects[0].ObjectID {
		t.Fatalf("second exact page = %+v", secondPage)
	}
	if _, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor/query mismatch error = %v", err)
	}
	tamperedCursor := "A" + page.NextCursor[1:]
	if _, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: tamperedCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("tampered cursor error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(db)
	store.now = func() time.Time { return clock }
	reopenedPage, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: page.NextCursor})
	if err != nil || len(reopenedPage.Objects) != 1 || reopenedPage.Objects[0].ObjectID != secondPage.Objects[0].ObjectID {
		t.Fatalf("reopened authenticated cursor page: result=%+v error=%v", reopenedPage, err)
	}

	lease, err := store.AcquireTurnLease(ctx, session.ID, "graph-traversal", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Second)
	firstLink := prepareAndApplyGraphLink(t, ctx, store, session, lease, "idem:v1:99000000-0000-4000-8000-000000000011", "generalize first", memory.GraphRelationGeneralization, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID})
	clock = clock.Add(time.Second)
	secondLink := prepareAndApplyGraphLink(t, ctx, store, session, lease, "idem:v1:99000000-0000-4000-8000-000000000012", "generalize second", memory.GraphRelationGeneralization, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: second.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID})
	validFrom := clock.Add(time.Hour)
	timedEvent := appendLifecycleEvent(t, ctx, store, session, lease, "remember a future structural Claim")
	timedProposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000016", SourceEventID: timedEvent.ID, Predicate: "scope_marker", PredicateLabel: "scope marker", ValidTime: memory.ValidTime{From: &validFrom}, Subject: memory.EntitySelector{Create: true, CanonicalName: "future subject", EntityType: "concept", Alias: "future subject"}, Object: memory.EntitySelector{Create: true, CanonicalName: "future object", EntityType: "concept", Alias: "future object"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, timedProposal); err != nil {
		t.Fatal(err)
	}
	_ = prepareAndApplyGraphLink(t, ctx, store, session, lease, "idem:v1:99000000-0000-4000-8000-000000000017", "derive the future Claim", memory.GraphRelationDerivation, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID}, memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: timedProposal.Claim.ID})
	currentTemporal, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID}, Depth: 1, Relations: []memory.GraphRelation{memory.GraphRelationDerivation}})
	if err != nil || len(currentTemporal.Paths) != 0 {
		t.Fatalf("pre-validity traversal: result=%+v error=%v", currentTemporal, err)
	}
	futureTemporal, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{ClaimQuery: memory.ClaimQuery{ValidAt: &validFrom}, Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID}, Depth: 1, Relations: []memory.GraphRelation{memory.GraphRelationDerivation}})
	if err != nil || len(futureTemporal.Paths) != 1 || futureTemporal.Paths[0].Nodes[1].ID != timedProposal.Claim.ID {
		t.Fatalf("valid-time traversal: result=%+v error=%v", futureTemporal, err)
	}
	if _, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: page.NextCursor}); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("changed-revision cursor error = %v", err)
	}
	forgedRaw, err := base64.RawURLEncoding.DecodeString(page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var forged semanticCursorEnvelope
	if err := json.Unmarshal(forgedRaw, &forged); err != nil {
		t.Fatal(err)
	}
	for index := range forged.Payload.ScopeRevisions {
		if err := store.db.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = ?`, forged.Payload.ScopeRevisions[index].ScopeKey).Scan(&forged.Payload.ScopeRevisions[index].Revision); err != nil {
			t.Fatal(err)
		}
	}
	forged.Payload.LastKey = ""
	payloadRaw, err := json.Marshal(forged.Payload)
	if err != nil {
		t.Fatal(err)
	}
	forgedDigest := sha256.Sum256(payloadRaw)
	forged.MACSHA256 = "hmac-sha256:" + fmt.Sprintf("%x", forgedDigest)
	forgedRaw, err = json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	forgedCursor := base64.RawURLEncoding.EncodeToString(forgedRaw)
	if _, err := store.ListSemanticObjects(ctx, session.ScopeContext(), memory.SemanticObjectListQuery{ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker", Polarity: memory.PolarityAffirmed}, Kinds: []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 1, Cursor: forgedCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("forged cursor error = %v", err)
	}
	neighborhood, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2, Relations: []memory.GraphRelation{memory.GraphRelationGeneralization}})
	if err != nil {
		t.Fatal(err)
	}
	if len(neighborhood.Paths) != 2 || len(neighborhood.Objects) != 3 || len(neighborhood.Paths[1].Links) != 2 {
		t.Fatalf("two-hop neighborhood = %+v", neighborhood)
	}
	reverse, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: third.Claim.ID}, Depth: 2, Relations: []memory.GraphRelation{memory.GraphRelationGeneralization}})
	if err != nil || len(reverse.Paths) != 2 || reverse.Paths[1].Links[0].ID != secondLink.GraphLinkID || reverse.Paths[1].Links[1].ID != firstLink.GraphLinkID {
		t.Fatalf("reverse equivalent path: result=%+v error=%v", reverse, err)
	}
	retractEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retract the third Claim source")
	retract, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000014", SourceEventID: retractEvent.ID, Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: third.Source.ID})
	if err != nil {
		t.Fatal(err)
	}
	retracted, err := store.ApplyMemoryLifecycle(ctx, lease, retract)
	if err != nil {
		t.Fatal(err)
	}
	retractedAt := retracted.TransactionTime
	retractedInspection, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectClaim, third.Claim.ID, memory.ClaimQuery{AsKnownAt: &retractedAt})
	if err != nil || retractedInspection.Status != memory.SemanticStatusUnsupported {
		t.Fatalf("source-retracted historical inspection: result=%+v error=%v", retractedInspection, err)
	}
	unsupportedHop, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
	if err != nil || len(unsupportedHop.Paths) != 1 {
		t.Fatalf("unsupported endpoint traversal: result=%+v error=%v", unsupportedHop, err)
	}
	restoreSourceEvent := appendLifecycleEvent(t, ctx, store, session, lease, "restore the third Claim source")
	clock = clock.Add(time.Second)
	restoreSource, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000015", SourceEventID: restoreSourceEvent.ID, Action: memory.LifecycleRestoreSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: third.Source.ID})
	if err != nil {
		t.Fatal(err)
	}
	restoredSource, err := store.ApplyMemoryLifecycle(ctx, lease, restoreSource)
	if err != nil {
		t.Fatal(err)
	}
	restoredAt := restoredSource.TransactionTime
	restoredInspection, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectClaim, third.Claim.ID, memory.ClaimQuery{AsKnownAt: &restoredAt})
	if err != nil || restoredInspection.Status != memory.SemanticStatusActive {
		t.Fatalf("source-restored historical inspection: result=%+v error=%v", restoredInspection, err)
	}
	beforeRetirement := clock
	if _, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 3}); err == nil {
		t.Fatal("depth three traversal unexpectedly accepted")
	}
	clock = clock.Add(time.Second)
	retireEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retire second path")
	retire, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000013", SourceEventID: retireEvent.ID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectGraphLink, ObjectID: secondLink.GraphLinkID})
	if err != nil {
		t.Fatal(err)
	}
	appendExactSemanticApproval(t, ctx, store, lease, retireEvent.ID, retire.OperationID, retire.ProposalSHA256, retire.PreparedSHA256)
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
		t.Fatal(err)
	}
	current, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(current.Paths) != 1 || current.Paths[0].Links[0].ID != firstLink.GraphLinkID {
		t.Fatalf("retired-link neighborhood = %+v", current)
	}
	historical, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{ClaimQuery: memory.ClaimQuery{AsKnownAt: &beforeRetirement}, Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(historical.Paths) != 2 {
		t.Fatalf("historical neighborhood = %+v", historical)
	}
	retractAgainEvent := appendLifecycleEvent(t, ctx, store, session, lease, "retract the third Claim source before restoring its Graph Link")
	retractAgain, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000018", SourceEventID: retractAgainEvent.ID, Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: third.Source.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, lease, retractAgain); err != nil {
		t.Fatal(err)
	}
	restoreLinkEvent := appendLifecycleEvent(t, ctx, store, session, lease, "try to restore a Graph Link with an unsupported Claim endpoint")
	if _, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000019", SourceEventID: restoreLinkEvent.ID, Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectGraphLink, ObjectID: secondLink.GraphLinkID}); err == nil {
		t.Fatal("Graph Link restoration unexpectedly accepted an unsupported Claim endpoint")
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
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
	store = NewStore(db)
	store.now = func() time.Time { return clock.Add(time.Second) }
	reopened, err := store.TraverseSemanticNeighborhood(ctx, session.ScopeContext(), memory.SemanticTraversalQuery{ClaimQuery: memory.ClaimQuery{AsKnownAt: &beforeRetirement}, Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: first.Claim.ID}, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Paths) != len(historical.Paths) || pathKey(reopened.Paths[1]) != pathKey(historical.Paths[1]) {
		t.Fatalf("reopened path order changed: before=%+v after=%+v", historical.Paths, reopened.Paths)
	}
	reopenedRetracted, err := store.InspectSemanticObjectAt(ctx, session.ScopeContext(), memory.SemanticObjectClaim, third.Claim.ID, memory.ClaimQuery{AsKnownAt: &retractedAt})
	if err != nil || reopenedRetracted.Status != retractedInspection.Status || len(reopenedRetracted.Lifecycle) != len(retractedInspection.Lifecycle) {
		t.Fatalf("reopened source-retracted inspection: before=%+v after=%+v error=%v", retractedInspection, reopenedRetracted, err)
	}
}

func TestGraphLinkEndpointScopeMatrixAndProvenanceRedaction(t *testing.T) {
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
	workspace, err := store.RegisterWorkspace(ctx, "Graph scope")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := store.RegisterWorkspace(ctx, "Sibling graph scope")
	if err != nil {
		t.Fatal(err)
	}
	siblingSession, err := store.CreateWorkspaceSessionWithComposition(ctx, sibling.ID, sibling.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	globalClaim := rememberScopeClaim(t, ctx, store, global, false, 221)
	localClaim := rememberScopeClaim(t, ctx, store, workspaceSession, false, 222)
	sessionClaim := rememberScopeClaim(t, ctx, store, workspaceSession, true, 223)
	siblingClaim := rememberScopeClaim(t, ctx, store, siblingSession, false, 224)
	lease, err := store.AcquireTurnLease(ctx, workspaceSession.ID, "graph-scope", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event := appendLifecycleEvent(t, ctx, store, workspaceSession, lease, "link allowed global and Workspace records")
	if _, err := store.PrepareCreateGraphLink(ctx, workspaceSession.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000021", SourceEventID: event.ID, Relation: memory.GraphRelationGeneralization, Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: localClaim.Claim.ID}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: globalClaim.Claim.ID}}); err != nil {
		t.Fatalf("Workspace/global endpoints rejected: %v", err)
	}
	siblingEvent := appendLifecycleEvent(t, ctx, store, workspaceSession, lease, "try sibling link")
	if _, err := store.PrepareCreateGraphLink(ctx, workspaceSession.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000022", SourceEventID: siblingEvent.ID, Relation: memory.GraphRelationGeneralization, Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: localClaim.Claim.ID}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: siblingClaim.Claim.ID}}); err == nil {
		t.Fatal("sibling Workspace endpoint unexpectedly prepared")
	}
	sessionEvent := appendLifecycleEvent(t, ctx, store, workspaceSession, lease, "link current session to Workspace")
	if _, err := store.PrepareCreateGraphLink(ctx, workspaceSession.ScopeContext(), memory.CreateGraphLinkRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000023", SourceEventID: sessionEvent.ID, Relation: memory.GraphRelationGeneralization, Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: sessionClaim.Claim.ID}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: localClaim.Claim.ID}, UseSessionScope: true}); err != nil {
		t.Fatalf("session/Context endpoints rejected: %v", err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	pctx, pstore, promotionDB, scopedSession, _, promotionLease, promotion := preparePromotionTest(t, 229)
	defer promotionDB.Close()
	approvePromotion(t, pctx, pstore, promotionLease, promotion, memory.ApprovalApproved)
	result, err := pstore.ApplyPromotion(pctx, promotionLease, promotion)
	if err != nil {
		t.Fatal(err)
	}
	globalInspector, err := pstore.CreateGlobalSession(pctx)
	if err != nil {
		t.Fatal(err)
	}
	redactionLease, err := pstore.AcquireTurnLease(pctx, globalInspector.ID, "redacted-source-lifecycle", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	promotedSourceID := promotion.Sources[0].ID
	retractSourceEvent := appendLifecycleEvent(t, pctx, pstore, globalInspector, redactionLease, "retract promoted source evidence")
	retractSource, err := pstore.PrepareMemoryLifecycle(pctx, globalInspector.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000024", SourceEventID: retractSourceEvent.ID, Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: promotedSourceID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pstore.ApplyMemoryLifecycle(pctx, redactionLease, retractSource); err != nil {
		t.Fatal(err)
	}
	restoreSourceEvent := appendLifecycleEvent(t, pctx, pstore, globalInspector, redactionLease, "restore promoted source evidence")
	restoreSource, err := pstore.PrepareMemoryLifecycle(pctx, globalInspector.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: "idem:v1:99000000-0000-4000-8000-000000000025", SourceEventID: restoreSourceEvent.ID, Action: memory.LifecycleRestoreSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: promotedSourceID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pstore.ApplyMemoryLifecycle(pctx, redactionLease, restoreSource); err != nil {
		t.Fatal(err)
	}
	if err := pstore.ReleaseTurnLease(pctx, redactionLease.SessionID, redactionLease.HolderID, redactionLease.FencingToken); err != nil {
		t.Fatal(err)
	}
	exact, err := pstore.InspectSemanticObject(pctx, globalInspector.ScopeContext(), memory.SemanticObjectClaim, result.DestinationClaimID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Sources) == 0 || exact.Sources[0].Source.ScopeKey != "workspace:"+string(scopedSession.WorkspaceID) || exact.Sources[0].Source.ID == "" || exact.Sources[0].Source.EventID == "" || exact.Sources[0].Source.Evidence != "" {
		t.Fatalf("redacted promoted provenance = %+v", exact.Sources)
	}
	if len(exact.Operations) != 3 {
		t.Fatalf("disallowed source operation history = %+v", exact.Operations)
	}
	for _, operation := range exact.Operations {
		if operation.ProposalJSON != "" || operation.PreparedJSON != "" || operation.ResultJSON != "" {
			t.Fatalf("disallowed nested Source operation text was not redacted: %+v", exact.Operations)
		}
	}
	directSource, err := pstore.InspectSemanticObject(pctx, globalInspector.ScopeContext(), memory.SemanticObjectSourceLink, promotedSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if directSource.Source == nil || directSource.Source.ID == "" || directSource.Source.EventID == "" || directSource.Source.Evidence != "" || len(directSource.Operations) != 3 {
		t.Fatalf("direct redacted Source inspection = %+v", directSource)
	}
	for _, operation := range directSource.Operations {
		if operation.ProposalJSON != "" || operation.PreparedJSON != "" || operation.ResultJSON != "" {
			t.Fatalf("disallowed direct Source operation text was not redacted: %+v", directSource.Operations)
		}
	}
}
