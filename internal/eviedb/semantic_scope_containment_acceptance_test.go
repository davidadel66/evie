package eviedb

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type semanticContainmentActor struct {
	name       string
	session    memory.Session
	contextKey string
}

type semanticContainmentObject struct {
	name     string
	scopeKey string
	claim    memory.RememberEntityProposal
}

type semanticContainmentPromotion struct {
	sourceScope string
	evidence    string
	claimID     memory.SemanticID
	sourceID    memory.SemanticID
}

func TestSemanticScopeContainmentAcceptanceMatrix(t *testing.T) {
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
	workspaceA, err := store.RegisterWorkspace(ctx, "Containment Workspace A")
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := store.RegisterWorkspace(ctx, "Containment Workspace B")
	if err != nil {
		t.Fatal(err)
	}
	workspaceA1, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceA.ID, workspaceA.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	workspaceA2, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceA.ID, workspaceA.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	workspaceB1, err := store.CreateWorkspaceSessionWithComposition(ctx, workspaceB.ID, workspaceB.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	projectA, err := store.RegisterProject(ctx, "Containment Project A", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := store.RegisterProject(ctx, "Containment Project B", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectA1, err := store.CreateProjectSession(ctx, projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectA2, err := store.CreateProjectSession(ctx, projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	projectB1, err := store.CreateProjectSession(ctx, projectB.ID)
	if err != nil {
		t.Fatal(err)
	}

	actors := []semanticContainmentActor{
		{name: "global", session: global, contextKey: "global"},
		{name: "workspace-a-1", session: workspaceA1, contextKey: "workspace:" + string(workspaceA.ID)},
		{name: "workspace-a-2", session: workspaceA2, contextKey: "workspace:" + string(workspaceA.ID)},
		{name: "workspace-b-1", session: workspaceB1, contextKey: "workspace:" + string(workspaceB.ID)},
		{name: "project-a-1", session: projectA1, contextKey: "project:" + string(projectA.ID)},
		{name: "project-a-2", session: projectA2, contextKey: "project:" + string(projectA.ID)},
		{name: "project-b-1", session: projectB1, contextKey: "project:" + string(projectB.ID)},
	}
	actorByScope := make(map[string]semanticContainmentActor)
	for _, actor := range actors {
		actorByScope["session:"+string(actor.session.ID)] = actor
		if actor.contextKey != "global" {
			if _, exists := actorByScope[actor.contextKey]; !exists {
				actorByScope[actor.contextKey] = actor
			}
		}
	}

	sequence := 301
	anchor := semanticContainmentObject{name: "global-anchor", scopeKey: "global", claim: rememberScopeClaim(t, ctx, store, global, false, sequence)}
	sequence++
	globalLeaf := semanticContainmentObject{name: "global-leaf", scopeKey: "global", claim: rememberScopeClaim(t, ctx, store, global, false, sequence)}
	sequence++
	objects := []semanticContainmentObject{anchor, globalLeaf}
	contextObjects := make(map[string]semanticContainmentObject)
	for _, actor := range actors {
		if actor.contextKey == "global" {
			continue
		}
		if _, exists := contextObjects[actor.contextKey]; exists {
			continue
		}
		object := semanticContainmentObject{
			name: actor.name + "-context", scopeKey: actor.contextKey,
			claim: rememberScopeClaim(t, ctx, store, actor.session, false, sequence),
		}
		sequence++
		contextObjects[actor.contextKey] = object
		objects = append(objects, object)
	}
	contextObjectCount := len(contextObjects)
	sessionObjects := make(map[memory.SessionID]semanticContainmentObject)
	for _, actor := range actors {
		object := semanticContainmentObject{
			name: actor.name + "-session", scopeKey: "session:" + string(actor.session.ID),
			claim: rememberScopeClaim(t, ctx, store, actor.session, true, sequence),
		}
		sequence++
		sessionObjects[actor.session.ID] = object
		objects = append(objects, object)
	}
	objectsByScope := make(map[string][]semanticContainmentObject)
	for _, object := range objects {
		objectsByScope[object.scopeKey] = append(objectsByScope[object.scopeKey], object)
	}

	for _, actor := range actors {
		actor := actor
		t.Run(actor.name+"/reads", func(t *testing.T) {
			allowed := semanticContainmentAllowedScopes(actor)
			wantIDs := semanticContainmentAllowedClaimIDs(objects, allowed)
			inspection, err := store.InspectClaims(ctx, actor.session.ScopeContext(), memory.ClaimQuery{PredicateToken: "scope_marker"})
			if err != nil {
				t.Fatal(err)
			}
			semanticContainmentAssertIDs(t, "Claim inspection", wantIDs, semanticContainmentInspectionIDs(inspection))
			page, err := store.ListSemanticObjects(ctx, actor.session.ScopeContext(), memory.SemanticObjectListQuery{
				ClaimQuery: memory.ClaimQuery{PredicateToken: "scope_marker"},
				Kinds:      []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 100,
			})
			if err != nil {
				t.Fatal(err)
			}
			semanticContainmentAssertIDs(t, "object listing", wantIDs, semanticContainmentSummaryIDs(page))

			for _, object := range objects {
				targets := []struct {
					kind memory.SemanticObjectKind
					id   memory.SemanticID
				}{
					{kind: memory.SemanticObjectClaim, id: object.claim.Claim.ID},
					{kind: memory.SemanticObjectEntity, id: object.claim.Claim.SubjectEntityID},
					{kind: memory.SemanticObjectSourceLink, id: object.claim.Source.ID},
				}
				for _, target := range targets {
					_, err := store.InspectSemanticObject(ctx, actor.session.ScopeContext(), target.kind, target.id)
					_, shouldRead := allowed[object.scopeKey]
					if shouldRead && err != nil {
						t.Fatalf("allowed %s %s inspection failed: %v", object.name, target.kind, err)
					}
					if !shouldRead && err == nil {
						t.Fatalf("forbidden %s %s was directly inspectable", object.name, target.kind)
					}
				}
			}
			for scopeKey, scoped := range objectsByScope {
				selected, err := store.ListSemanticObjects(ctx, actor.session.ScopeContext(), memory.SemanticObjectListQuery{
					ClaimQuery: memory.ClaimQuery{ScopeKey: scopeKey, PredicateToken: "scope_marker"},
					Kinds:      []memory.SemanticObjectKind{memory.SemanticObjectClaim}, PageSize: 100,
				})
				_, shouldRead := allowed[scopeKey]
				if !shouldRead {
					if err == nil {
						t.Fatalf("forbidden explicit scope %q was listable", scopeKey)
					}
					continue
				}
				if err != nil {
					t.Fatalf("allowed explicit scope %q failed: %v", scopeKey, err)
				}
				semanticContainmentAssertIDs(t, scopeKey+" listing", semanticContainmentObjectIDs(scoped), semanticContainmentSummaryIDs(selected))
			}
		})
	}

	keySequence := 1
	for _, actor := range actors {
		actor := actor
		t.Run(actor.name+"/writes-and-references", func(t *testing.T) {
			before := semanticContainmentScopeRevisions(t, ctx, store, actor.session)
			lease, err := store.AcquireTurnLease(ctx, actor.session.ID, memory.LeaseHolderID("containment-"+actor.name), time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
					t.Errorf("release containment Turn Lease: %v", err)
				}
			})
			event := appendLifecycleEvent(t, ctx, store, actor.session, lease, "exercise the exact scope containment matrix")
			for _, object := range objects {
				useSession := strings.HasPrefix(object.scopeKey, "session:")
				_, err := store.PrepareMemoryLifecycle(ctx, actor.session.ScopeContext(), memory.MemoryLifecycleRequest{
					IdempotencyKey: semanticContainmentKey(&keySequence), SourceEventID: event.ID,
					Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim,
					ObjectID: object.claim.Claim.ID, UseSessionScope: useSession,
				})
				shouldWrite := object.scopeKey == actor.contextKey || object.scopeKey == "session:"+string(actor.session.ID)
				if shouldWrite && err != nil {
					t.Fatalf("allowed %s mutation was not preparable: %v", object.name, err)
				}
				if !shouldWrite && err == nil {
					t.Fatalf("forbidden %s mutation was prepared", object.name)
				}
			}

			for _, useSession := range []bool{false, true} {
				for _, object := range objects {
					other := anchor.claim.Claim.SubjectEntityID
					if object.claim.Claim.SubjectEntityID == other {
						other = globalLeaf.claim.Claim.SubjectEntityID
					}
					proposal, err := store.PrepareRememberEntity(ctx, actor.session.ScopeContext(), memory.RememberEntityRequest{
						IdempotencyKey: semanticContainmentKey(&keySequence), SourceEventID: event.ID,
						Predicate: "containment_reference", PredicateLabel: "containment reference",
						Subject: memory.EntitySelector{EntityID: object.claim.Claim.SubjectEntityID},
						Object:  memory.EntitySelector{EntityID: other}, UseSessionScope: useSession,
					})
					shouldReference := semanticContainmentReferenceAllowed(actor, object.scopeKey, useSession)
					if shouldReference && err != nil {
						t.Fatalf("allowed %s Entity reference (session=%t) failed: %v", object.name, useSession, err)
					}
					if shouldReference && proposal.Claim.ScopeKey != semanticContainmentTargetScope(actor, useSession) {
						t.Fatalf("%s Entity reference targeted %q", object.name, proposal.Claim.ScopeKey)
					}
					if !shouldReference && err == nil {
						t.Fatalf("forbidden %s Entity reference (session=%t) was prepared", object.name, useSession)
					}
				}
				for _, object := range objects[1:] {
					proposal, err := store.PrepareCreateGraphLink(ctx, actor.session.ScopeContext(), memory.CreateGraphLinkRequest{
						IdempotencyKey: semanticContainmentKey(&keySequence), SourceEventID: event.ID,
						Relation:        memory.GraphRelationGeneralization,
						Source:          memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: anchor.claim.Claim.ID},
						Target:          memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: object.claim.Claim.ID},
						UseSessionScope: useSession,
					})
					shouldReference := semanticContainmentReferenceAllowed(actor, object.scopeKey, useSession)
					if shouldReference && err != nil {
						t.Fatalf("allowed %s Graph Link reference (session=%t) failed: %v", object.name, useSession, err)
					}
					if shouldReference && proposal.Link.ScopeKey != semanticContainmentTargetScope(actor, useSession) {
						t.Fatalf("%s Graph Link targeted %q", object.name, proposal.Link.ScopeKey)
					}
					if !shouldReference && err == nil {
						t.Fatalf("forbidden %s Graph Link reference (session=%t) was prepared", object.name, useSession)
					}
				}
			}
			after := semanticContainmentScopeRevisions(t, ctx, store, actor.session)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("proposal-only containment checks changed Scope Revisions: before=%v after=%v", before, after)
			}
		})
	}

	semanticContainmentApplyLink(t, ctx, store, global, anchor.claim.Claim.ID, globalLeaf.claim.Claim.ID, false, &keySequence)
	for _, object := range objects[2 : 2+contextObjectCount] {
		owner := actorByScope[object.scopeKey]
		semanticContainmentApplyLink(t, ctx, store, owner.session, anchor.claim.Claim.ID, object.claim.Claim.ID, false, &keySequence)
	}
	for _, actor := range actors {
		object := sessionObjects[actor.session.ID]
		semanticContainmentApplyLink(t, ctx, store, actor.session, anchor.claim.Claim.ID, object.claim.Claim.ID, true, &keySequence)
	}
	for _, actor := range actors {
		actor := actor
		t.Run(actor.name+"/traversal", func(t *testing.T) {
			allowed := semanticContainmentAllowedScopes(actor)
			neighborhood, err := store.TraverseSemanticNeighborhood(ctx, actor.session.ScopeContext(), memory.SemanticTraversalQuery{
				Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: anchor.claim.Claim.ID},
				Depth: 1, Relations: []memory.GraphRelation{memory.GraphRelationGeneralization},
			})
			if err != nil {
				t.Fatal(err)
			}
			wantNeighbors := []memory.SemanticID{globalLeaf.claim.Claim.ID, sessionObjects[actor.session.ID].claim.Claim.ID}
			if actor.contextKey != "global" {
				wantNeighbors = append(wantNeighbors, contextObjects[actor.contextKey].claim.Claim.ID)
			}
			gotNeighbors := make([]memory.SemanticID, 0, len(neighborhood.Paths))
			for _, path := range neighborhood.Paths {
				if len(path.Nodes) != 2 || len(path.Links) != 1 {
					t.Fatalf("unexpected one-hop path: %+v", path)
				}
				gotNeighbors = append(gotNeighbors, path.Nodes[1].ID)
				if _, ok := allowed[path.Links[0].ScopeKey]; !ok {
					t.Fatalf("traversal exposed Link in forbidden scope %q", path.Links[0].ScopeKey)
				}
			}
			semanticContainmentAssertIDs(t, "traversal neighbors", wantNeighbors, gotNeighbors)
			for _, object := range objects {
				if _, ok := allowed[object.scopeKey]; ok {
					continue
				}
				if _, err := store.TraverseSemanticNeighborhood(ctx, actor.session.ScopeContext(), memory.SemanticTraversalQuery{
					Start: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: object.claim.Claim.ID}, Depth: 1,
				}); err == nil {
					t.Fatalf("traversal started from forbidden %s Claim", object.name)
				}
			}
		})
	}

	var promotions []semanticContainmentPromotion
	for _, object := range objects[2:] {
		owner := actorByScope[object.scopeKey]
		promotions = append(promotions, semanticContainmentApplyPromotion(t, ctx, store, owner.session, object, &keySequence))
	}
	for _, actor := range actors {
		actor := actor
		t.Run(actor.name+"/source-expansion", func(t *testing.T) {
			allowed := semanticContainmentAllowedScopes(actor)
			for _, promotion := range promotions {
				claim, err := store.InspectSemanticObject(ctx, actor.session.ScopeContext(), memory.SemanticObjectClaim, promotion.claimID)
				if err != nil {
					t.Fatalf("inspect globally promoted Claim from %q: %v", promotion.sourceScope, err)
				}
				if len(claim.Sources) != 1 || claim.Sources[0].Source.ID != promotion.sourceID ||
					claim.Sources[0].Source.EventID == "" || claim.Sources[0].Source.EvidenceSHA256 == "" {
					t.Fatalf("promoted provenance from %q is not ID-auditable: %+v", promotion.sourceScope, claim.Sources)
				}
				_, mayExpand := allowed[promotion.sourceScope]
				wantEvidence := ""
				if mayExpand {
					wantEvidence = promotion.evidence
				}
				if claim.Sources[0].Source.Evidence != wantEvidence {
					t.Fatalf("Claim source expansion from %q = %q, want %q", promotion.sourceScope, claim.Sources[0].Source.Evidence, wantEvidence)
				}
				semanticContainmentAssertOperationRedaction(t, claim.Operations, mayExpand)

				source, err := store.InspectSemanticObject(ctx, actor.session.ScopeContext(), memory.SemanticObjectSourceLink, promotion.sourceID)
				if err != nil {
					t.Fatalf("inspect global Source Link from %q: %v", promotion.sourceScope, err)
				}
				if source.Source == nil || source.Source.ID != promotion.sourceID || source.Source.EventID == "" ||
					source.Source.EvidenceSHA256 == "" || source.Source.Evidence != wantEvidence {
					t.Fatalf("direct Source Link expansion from %q = %+v, want evidence %q", promotion.sourceScope, source.Source, wantEvidence)
				}
				semanticContainmentAssertOperationRedaction(t, source.Operations, mayExpand)
			}
		})
	}
}

func semanticContainmentAllowedScopes(actor semanticContainmentActor) map[string]struct{} {
	allowed := map[string]struct{}{"global": {}, "session:" + string(actor.session.ID): {}}
	allowed[actor.contextKey] = struct{}{}
	return allowed
}

func semanticContainmentTargetScope(actor semanticContainmentActor, useSession bool) string {
	if useSession {
		return "session:" + string(actor.session.ID)
	}
	return actor.contextKey
}

func semanticContainmentReferenceAllowed(actor semanticContainmentActor, candidateScope string, useSession bool) bool {
	if candidateScope == "global" {
		return true
	}
	if candidateScope == actor.contextKey && actor.contextKey != "global" {
		return true
	}
	return useSession && candidateScope == "session:"+string(actor.session.ID)
}

func semanticContainmentKey(sequence *int) string {
	key := fmt.Sprintf("idem:v1:ab000000-0000-4000-8000-%012d", *sequence)
	(*sequence)++
	return key
}

func semanticContainmentAllowedClaimIDs(objects []semanticContainmentObject, allowed map[string]struct{}) []memory.SemanticID {
	var ids []memory.SemanticID
	for _, object := range objects {
		if _, ok := allowed[object.scopeKey]; ok {
			ids = append(ids, object.claim.Claim.ID)
		}
	}
	return ids
}

func semanticContainmentObjectIDs(objects []semanticContainmentObject) []memory.SemanticID {
	ids := make([]memory.SemanticID, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, object.claim.Claim.ID)
	}
	return ids
}

func semanticContainmentInspectionIDs(inspection memory.ClaimsInspection) []memory.SemanticID {
	ids := make([]memory.SemanticID, 0, len(inspection.Claims))
	for _, claim := range inspection.Claims {
		ids = append(ids, claim.ID)
	}
	return ids
}

func semanticContainmentSummaryIDs(page memory.SemanticObjectPage) []memory.SemanticID {
	ids := make([]memory.SemanticID, 0, len(page.Objects))
	for _, object := range page.Objects {
		ids = append(ids, object.ObjectID)
	}
	return ids
}

func semanticContainmentAssertIDs(t *testing.T, label string, want, got []memory.SemanticID) {
	t.Helper()
	wantStrings, gotStrings := make([]string, len(want)), make([]string, len(got))
	for index := range want {
		wantStrings[index] = string(want[index])
	}
	for index := range got {
		gotStrings[index] = string(got[index])
	}
	sort.Strings(wantStrings)
	sort.Strings(gotStrings)
	if !reflect.DeepEqual(wantStrings, gotStrings) {
		t.Fatalf("%s IDs = %v, want %v", label, gotStrings, wantStrings)
	}
}

func semanticContainmentScopeRevisions(t *testing.T, ctx context.Context, store *Store, session memory.Session) []memory.ScopeRevision {
	t.Helper()
	page, err := store.ListSemanticScopes(ctx, session.ScopeContext(), memory.SemanticScopeListQuery{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	revisions := make([]memory.ScopeRevision, 0, len(page.Scopes))
	for _, scope := range page.Scopes {
		revisions = append(revisions, memory.ScopeRevision{ScopeKey: scope.Key, Revision: scope.Revision})
	}
	return revisions
}

func semanticContainmentApplyLink(t *testing.T, ctx context.Context, store *Store, session memory.Session, source, target memory.SemanticID, useSession bool, sequence *int) {
	t.Helper()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(fmt.Sprintf("containment-link-%d", *sequence)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event := appendLifecycleEvent(t, ctx, store, session, lease, "create an allowed containment Graph Link")
	proposal, err := store.PrepareCreateGraphLink(ctx, session.ScopeContext(), memory.CreateGraphLinkRequest{
		IdempotencyKey: semanticContainmentKey(sequence), SourceEventID: event.ID,
		Relation: memory.GraphRelationGeneralization,
		Source:   memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: source},
		Target:   memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: target}, UseSessionScope: useSession,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendExactSemanticApproval(t, ctx, store, lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	if _, err := store.ApplyCreateGraphLink(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
}

func semanticContainmentApplyPromotion(t *testing.T, ctx context.Context, store *Store, session memory.Session, object semanticContainmentObject, sequence *int) semanticContainmentPromotion {
	t.Helper()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(fmt.Sprintf("containment-promotion-%d", *sequence)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event := appendLifecycleEvent(t, ctx, store, session, lease, "promote one containment Claim globally")
	proposal, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: semanticContainmentKey(sequence), SourceEventID: event.ID,
		SourceClaimID: object.claim.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalApproved)
	result, err := store.ApplyPromotion(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if len(proposal.Sources) != 1 || proposal.Sources[0].Evidence == "" {
		t.Fatalf("authorized Promotion source from %q = %+v", object.scopeKey, proposal.Sources)
	}
	return semanticContainmentPromotion{
		sourceScope: object.scopeKey, evidence: proposal.Sources[0].Evidence,
		claimID: result.DestinationClaimID, sourceID: proposal.Sources[0].ID,
	}
}

func semanticContainmentAssertOperationRedaction(t *testing.T, operations []memory.SemanticOperationInspection, mayExpand bool) {
	t.Helper()
	if len(operations) == 0 {
		t.Fatal("exact inspection omitted accepted operation identity")
	}
	for _, operation := range operations {
		hasPayload := operation.ProposalJSON != "" || operation.PreparedJSON != "" || operation.ResultJSON != ""
		if mayExpand && !hasPayload {
			t.Fatalf("allowed source operation %s was redacted", operation.OperationID)
		}
		if !mayExpand && hasPayload {
			t.Fatalf("forbidden source operation %s exposed payload", operation.OperationID)
		}
	}
}
