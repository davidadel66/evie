package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSemanticClaimsUseExactAllowedScopeMatrix(t *testing.T) {
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
	workspace, err := store.RegisterWorkspace(ctx, "Primary")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := store.CreateWorkspaceSessionWithComposition(
		ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	siblingWorkspace, err := store.RegisterWorkspace(ctx, "Sibling")
	if err != nil {
		t.Fatal(err)
	}
	siblingWorkspaceSession, err := store.CreateWorkspaceSessionWithComposition(
		ctx, siblingWorkspace.ID, siblingWorkspace.CurrentRevisionID, standardReceipt(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.RegisterProject(ctx, "Primary project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	siblingProject, err := store.RegisterProject(ctx, "Sibling project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	siblingProjectSession, err := store.CreateProjectSession(ctx, siblingProject.ID)
	if err != nil {
		t.Fatal(err)
	}

	sessions := []memory.Session{global, workspaceSession, siblingWorkspaceSession, projectSession, siblingProjectSession}
	accepted := make(map[memory.SessionID][]memory.RememberEntityProposal)
	for index, session := range sessions {
		accepted[session.ID] = append(accepted[session.ID], rememberScopeClaim(t, ctx, store, session, false, index*2+1))
		accepted[session.ID] = append(accepted[session.ID], rememberScopeClaim(t, ctx, store, session, true, index*2+2))
	}

	tests := []struct {
		name    string
		session memory.Session
		want    map[string]int
	}{
		{name: "global", session: global, want: map[string]int{"global": 1, "session:" + string(global.ID): 1}},
		{name: "workspace", session: workspaceSession, want: map[string]int{
			"global": 1, "workspace:" + string(workspace.ID): 1, "session:" + string(workspaceSession.ID): 1,
		}},
		{name: "sibling workspace", session: siblingWorkspaceSession, want: map[string]int{
			"global": 1, "workspace:" + string(siblingWorkspace.ID): 1, "session:" + string(siblingWorkspaceSession.ID): 1,
		}},
		{name: "project", session: projectSession, want: map[string]int{
			"global": 1, "project:" + string(project.ID): 1, "session:" + string(projectSession.ID): 1,
		}},
		{name: "sibling project", session: siblingProjectSession, want: map[string]int{
			"global": 1, "project:" + string(siblingProject.ID): 1, "session:" + string(siblingProjectSession.ID): 1,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inspection, err := store.InspectClaims(ctx, test.session.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]int)
			for _, claim := range inspection.Claims {
				got[claim.Scope.Key]++
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("visible Claim scopes = %v, want %v", got, test.want)
			}
		})
	}
	var predicatesWithoutCanonicalGlobalScope int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM semantic_predicates AS predicates
		LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = predicates.scope_id
		WHERE scopes.scope_key IS NULL OR scopes.scope_key != 'global'
	`).Scan(&predicatesWithoutCanonicalGlobalScope); err != nil {
		t.Fatal(err)
	}
	if predicatesWithoutCanonicalGlobalScope != 0 {
		t.Fatalf("Predicates without canonical global scope = %d", predicatesWithoutCanonicalGlobalScope)
	}
	var sourceLinksWithoutClaimScope int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM semantic_source_links AS source_links
		LEFT JOIN semantic_scopes AS scopes ON scopes.scope_id = source_links.scope_id
		LEFT JOIN semantic_claims AS claims ON claims.claim_id = source_links.claim_id
		WHERE scopes.scope_id IS NULL OR source_links.scope_id != claims.scope_id
	`).Scan(&sourceLinksWithoutClaimScope); err != nil {
		t.Fatal(err)
	}
	if sourceLinksWithoutClaimScope != 0 {
		t.Fatalf("Source Links without their Claim's canonical scope = %d", sourceLinksWithoutClaimScope)
	}
	localSessionEntity := accepted[workspaceSession.ID][1].Claim.SubjectEntityID
	if _, err := store.InspectSemanticEntity(ctx, workspaceSession.ScopeContext(), localSessionEntity); err != nil {
		t.Fatalf("current-session Entity was not readable through the allowed scope matrix: %v", err)
	}
	siblingEntity := accepted[siblingWorkspaceSession.ID][0].Claim.SubjectEntityID
	if _, err := store.InspectSemanticEntity(ctx, workspaceSession.ScopeContext(), siblingEntity); err == nil {
		t.Fatal("sibling Workspace Entity was readable")
	}
	lease, err := store.AcquireTurnLease(ctx, workspaceSession.ID, "forbidden-reference", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, workspaceSession.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Use a sibling Entity",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareRememberEntity(ctx, workspaceSession.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8000-000000000099", SourceEventID: event.ID,
		Predicate: "scope_marker", PredicateLabel: "scope marker",
		Subject: memory.EntitySelector{EntityID: siblingEntity},
		Object:  memory.EntitySelector{EntityID: accepted[workspaceSession.ID][0].Claim.ObjectEntityID},
	}); err == nil {
		t.Fatal("write with sibling Workspace Entity reference was prepared")
	}
}

func TestArchivedSessionSemanticMemoryIsExplicitlyInspectableButNotShared(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	archived, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claim := rememberScopeClaim(t, ctx, store, archived, true, 80)
	if _, err := db.ExecContext(ctx, `UPDATE sessions SET status = 'closed' WHERE id = ?`, archived.ID); err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := store.InspectClaims(ctx, other.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, inspected := range visible.Claims {
		if inspected.ID == claim.Claim.ID {
			t.Fatal("archived session Claim leaked to another session")
		}
	}
	archivedView, err := store.InspectArchivedSessionClaims(ctx, archived.ID, memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if archivedView.Scope.Key != "session:"+string(archived.ID) || len(archivedView.Claims) != 1 || archivedView.Claims[0].ID != claim.Claim.ID {
		t.Fatalf("archived session inspection = %+v", archivedView)
	}
	reopenedDB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopenedView, err := NewStore(reopenedDB).InspectArchivedSessionClaims(ctx, archived.ID, memory.ClaimQuery{})
	if err != nil || len(reopenedView.Claims) != 1 || reopenedView.Claims[0].ID != claim.Claim.ID {
		t.Fatalf("reopened archived session inspection = %+v, error = %v", reopenedView, err)
	}
}

func TestPromotionCreatesBroaderClaimWithoutChangingNarrowerMemory(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Promotion")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(
		ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceClaim := rememberScopeClaim(t, ctx, store, session, false, 81)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "promotion", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	promotionEvent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote this memory globally",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey:      "idem:v1:85000000-0000-4000-8000-000000000181",
		SourceEventID:       promotionEvent.ID,
		SourceClaimID:       sourceClaim.Claim.ID,
		DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SchemaVersion != 4 || proposal.SourceClaim.ID != sourceClaim.Claim.ID ||
		proposal.SourceScope.Key != "workspace:"+string(workspace.ID) || proposal.DestinationScope.Key != "global" ||
		len(proposal.PriorRevisions) != 2 {
		t.Fatalf("Promotion preview = %+v", proposal)
	}
	approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalApproved)
	changedSourceID := proposal
	changedSourceID.Sources = append([]memory.SemanticSource(nil), proposal.Sources...)
	changedSourceID.Sources[0].ID = "85000000-0000-4000-8006-000000000181"
	changedSourceID.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(changedSourceID))
	if err != nil {
		t.Fatal(err)
	}
	changedSourceID.PreparedSHA256, _, err = semanticHash(changedSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyPromotion(ctx, lease, changedSourceID); err == nil {
		t.Fatal("rehashed Promotion changed a prepared Source Link ID")
	}
	changedOperationID := proposal
	changedOperationID.OperationID = "85000000-0000-4000-8006-000000000182"
	changedOperationID.Sources = append([]memory.SemanticSource(nil), proposal.Sources...)
	for index := range changedOperationID.Sources {
		if changedOperationID.Sources[index].Create {
			changedOperationID.Sources[index].OperationID = changedOperationID.OperationID
		}
	}
	changedOperationID.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(changedOperationID))
	if err != nil {
		t.Fatal(err)
	}
	changedOperationID.PreparedSHA256, _, err = semanticHash(changedOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyPromotion(ctx, lease, changedOperationID); err == nil {
		t.Fatal("rehashed Promotion changed the prepared operation ID")
	}
	var changedIdentityOperations int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id IN (?, ?)`,
		changedSourceID.OperationID, changedOperationID.OperationID).Scan(&changedIdentityOperations); err != nil || changedIdentityOperations != 0 {
		t.Fatalf("identity-tampered Promotion writes = %d, error = %v", changedIdentityOperations, err)
	}
	tampered := proposal
	tampered.PromotedEntities = append([]memory.PromotedEntity(nil), proposal.PromotedEntities...)
	tampered.PromotedEntities[0].DestinationEntity.CanonicalName = "Fabricated"
	tampered.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(tampered))
	if err != nil {
		t.Fatal(err)
	}
	tampered.PreparedSHA256, _, err = semanticHash(tampered)
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, lease, tampered, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, lease, tampered); err == nil {
		t.Fatal("rehashed Promotion changed the broader identity")
	}
	extraMapping := proposal
	extraMapping.PromotedEntities = append([]memory.PromotedEntity(nil), proposal.PromotedEntities...)
	extraMapping.PromotedEntities = append(extraMapping.PromotedEntities, memory.PromotedEntity{
		SourceEntityID: proposal.PromotedEntities[0].DestinationEntity.ID,
		DestinationEntity: memory.SemanticEntity{
			ID: "85000000-0000-4000-8000-000000000199", ScopeKey: "global",
			CanonicalName: "unrelated", EntityType: "concept", Create: true,
		},
	})
	extraMapping.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(extraMapping))
	if err != nil {
		t.Fatal(err)
	}
	extraMapping.PreparedSHA256, _, err = semanticHash(extraMapping)
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, lease, extraMapping, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, lease, extraMapping); err == nil {
		t.Fatal("rehashed Promotion added an unrelated identity mapping")
	}
	suppressedSource := proposal
	suppressedSource.Sources = append([]memory.SemanticSource(nil), proposal.Sources...)
	suppressedSource.Sources[0].Create = false
	suppressedSource.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(suppressedSource))
	if err != nil {
		t.Fatal(err)
	}
	suppressedSource.PreparedSHA256, _, err = semanticHash(suppressedSource)
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, lease, suppressedSource, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, lease, suppressedSource); err == nil {
		t.Fatal("rehashed Promotion suppressed destination provenance")
	}
	approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalApproved)
	result, err := store.ApplyPromotion(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceClaimID != sourceClaim.Claim.ID || result.DestinationClaimID == sourceClaim.Claim.ID {
		t.Fatalf("Promotion result = %+v", result)
	}

	workspaceView, err := store.InspectClaims(ctx, session.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceView.Claims) != 2 {
		t.Fatalf("Workspace view after Promotion = %+v", workspaceView)
	}
	var narrower, broader *memory.ClaimInspection
	for index := range workspaceView.Claims {
		claim := &workspaceView.Claims[index]
		switch claim.ID {
		case sourceClaim.Claim.ID:
			narrower = claim
		case result.DestinationClaimID:
			broader = claim
		}
	}
	if narrower == nil || broader == nil || narrower.Scope.Key != "workspace:"+string(workspace.ID) ||
		broader.Scope.Key != "global" || broader.Sources[0].Evidence == "" {
		t.Fatalf("Promotion did not preserve labeled source and destination Claims: %+v", workspaceView.Claims)
	}

	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalView, err := store.InspectClaims(ctx, global.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(globalView.Claims) != 1 || globalView.Claims[0].ID != result.DestinationClaimID ||
		len(globalView.Claims[0].Sources) == 0 || globalView.Claims[0].Sources[0].ID == "" ||
		globalView.Claims[0].Sources[0].Evidence != "" {
		t.Fatalf("global Promotion provenance was not ID-auditable and text-redacted: %+v", globalView)
	}
	exact, err := store.InspectSemanticObject(ctx, global.ScopeContext(), memory.SemanticObjectClaim, result.DestinationClaimID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Sources) != 1 || exact.Sources[0].Source.ID == "" || exact.Sources[0].Source.Evidence != "" {
		t.Fatalf("exact global inspection expanded disallowed narrower source text: %+v", exact.Sources)
	}
	for _, operation := range exact.Operations {
		if strings.Contains(operation.PreparedJSON, "scope claim 81") {
			t.Fatalf("exact global operation history leaked narrower source text: %+v", operation)
		}
	}
	reuseEvent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Confirm the same Promotion",
	})
	if err != nil {
		t.Fatal(err)
	}
	reuse, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8000-000000000184", SourceEventID: reuseEvent.ID,
		SourceClaimID: sourceClaim.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reuse.DestinationClaimCreate || reuse.DestinationClaim.ID != result.DestinationClaimID {
		t.Fatalf("repeated Promotion did not reuse broader Claim: %+v", reuse)
	}
	for _, promoted := range reuse.PromotedEntities {
		if promoted.DestinationEntity.Create {
			t.Fatalf("repeated Promotion recreated broader Entity: %+v", promoted)
		}
	}
	if effect := canonicalPromoteClaimProposal(reuse).Effect; len(effect.Claims) != 0 || len(effect.Entities) != 0 || len(effect.SourceLinks) != 0 {
		t.Fatalf("all-reuse Promotion encoded unexpected creates: %+v", effect)
	}
	approvePromotion(t, ctx, store, lease, reuse, memory.ApprovalApproved)
	if repeated, err := store.ApplyPromotion(ctx, lease, reuse); err != nil || repeated.DestinationClaimID != result.DestinationClaimID {
		t.Fatalf("apply repeated Promotion: result=%+v error=%v", repeated, err)
	}
}

func TestPromotionRejectsDuplicateSourcePlanWithoutWrites(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Duplicate Promotion source")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	source := rememberScopeClaim(t, ctx, store, session, false, 192)
	attachLease, err := store.AcquireTurnLease(ctx, session.ID, "attach-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	secondEvidence, err := store.AppendEventWithLease(ctx, session.ID, attachLease.HolderID, attachLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "second eligible source",
	})
	if err != nil {
		t.Fatal(err)
	}
	attach, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8006-000000000192", SourceEventID: secondEvidence.ID,
		Predicate: source.Predicate.Token, PredicateLabel: source.Predicate.Label,
		Subject: memory.EntitySelector{EntityID: source.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: source.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, attachLease, attach); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, attachLease.SessionID, attachLease.HolderID, attachLease.FencingToken); err != nil {
		t.Fatal(err)
	}
	promotionLease, err := store.AcquireTurnLease(ctx, session.ID, "duplicate-source-promotion", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	promotionEvent, err := store.AppendEventWithLease(ctx, session.ID, promotionLease.HolderID, promotionLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "promote both sources",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8006-000000000193", SourceEventID: promotionEvent.ID,
		SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Sources) != 2 {
		t.Fatalf("prepared source plans = %d, want 2", len(proposal.Sources))
	}
	duplicate := proposal
	duplicate.Sources = append([]memory.SemanticSource(nil), proposal.Sources...)
	duplicate.Sources[1] = duplicate.Sources[0]
	duplicate.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(duplicate))
	if err != nil {
		t.Fatal(err)
	}
	duplicate.PreparedSHA256, _, err = semanticHash(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, promotionLease, duplicate, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, promotionLease, duplicate); err == nil {
		t.Fatal("rehashed Promotion replaced one source with a duplicate")
	}
	var operations int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`, proposal.OperationID).Scan(&operations); err != nil || operations != 0 {
		t.Fatalf("duplicate-source Promotion operations = %d, error = %v", operations, err)
	}
}

func TestPromotionRequiresExactSessionApprovalWithoutWrites(t *testing.T) {
	tests := []struct {
		name    string
		approve func(*testing.T, context.Context, *Store, memory.TurnLease, memory.PromotionProposal)
	}{
		{name: "missing approval"},
		{
			name: "declined approval",
			approve: func(t *testing.T, ctx context.Context, store *Store, lease memory.TurnLease, proposal memory.PromotionProposal) {
				approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalDeclined)
			},
		},
		{
			name: "changed approved proposal hash",
			approve: func(t *testing.T, ctx context.Context, store *Store, lease memory.TurnLease, proposal memory.PromotionProposal) {
				payload, err := json.Marshal(memory.ApprovalPayload{
					Decision:       memory.ApprovalApproved,
					ProposalSHA256: "sha256:attacker-recomputed-effect",
					PreparedSHA256: proposal.PreparedSHA256,
				})
				if err != nil {
					t.Fatal(err)
				}
				appendPromotionApproval(t, ctx, store, lease, proposal, payload)
			},
		},
		{
			name: "changed approved prepared hash",
			approve: func(t *testing.T, ctx context.Context, store *Store, lease memory.TurnLease, proposal memory.PromotionProposal) {
				payload, err := json.Marshal(memory.ApprovalPayload{
					Decision:       memory.ApprovalApproved,
					ProposalSHA256: proposal.ProposalSHA256,
					PreparedSHA256: "sha256:attacker-recomputed-preparation",
				})
				if err != nil {
					t.Fatal(err)
				}
				appendPromotionApproval(t, ctx, store, lease, proposal, payload)
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, store, db, _, _, lease, proposal := prepareUnapprovedPromotionTest(t, 193+index)
			defer db.Close()
			if test.approve != nil {
				test.approve(t, ctx, store, lease, proposal)
			}
			if _, err := store.ApplyPromotion(ctx, lease, proposal); err == nil {
				t.Fatal("Promotion without its exact approved event was applied")
			}
			assertPromotionLeavesNoWrites(t, ctx, store, proposal)
		})
	}
}

func TestSemanticSchemaHasNoPromotionPreparationTable(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'semantic_promotion_preparations'
	`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("Promotion proposals must remain Episodic Memory, not a semantic preparation table")
	}
}

func TestPromotionRejectsRetractedDestinationSourceLink(t *testing.T) {
	ctx, store, db, workspaceSession, source, workspaceLease, first := preparePromotionTest(t, 185)
	defer db.Close()
	_, err := store.ApplyPromotion(ctx, workspaceLease, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sources) != 1 || !first.Sources[0].Create {
		t.Fatalf("initial Promotion source plan = %+v", first.Sources)
	}
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalLease, err := store.AcquireTurnLease(ctx, globalSession.ID, "retract-promoted-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	retractEvent, err := store.AppendEventWithLease(ctx, globalSession.ID, globalLease.HolderID, globalLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Retract the promoted Source Link",
	})
	if err != nil {
		t.Fatal(err)
	}
	retraction, err := store.PrepareMemoryLifecycle(ctx, globalSession.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8005-000000000185", SourceEventID: retractEvent.ID,
		Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: first.Sources[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, globalLease, retraction); err != nil {
		t.Fatal(err)
	}
	retryEvent, err := store.AppendEventWithLease(ctx, workspaceSession.ID, workspaceLease.HolderID, workspaceLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote with eligible provenance again",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.PreparePromotion(ctx, workspaceSession.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8005-000000000186", SourceEventID: retryEvent.ID,
		SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err == nil || !strings.Contains(err.Error(), "restore it explicitly") {
		t.Fatalf("Promotion with retracted destination provenance error = %v", err)
	}
	var retryOperations int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE idempotency_key = ?`,
		"idem:v1:85000000-0000-4000-8005-000000000186").Scan(&retryOperations); err != nil || retryOperations != 0 {
		t.Fatalf("rejected Promotion operations = %d, error = %v", retryOperations, err)
	}
}

func TestPromotionDoesNotReuseRetiredDestinationClaimOrEntity(t *testing.T) {
	ctx, store, db, workspaceSession, source, workspaceLease, first := preparePromotionTest(t, 186)
	defer db.Close()
	firstResult, err := store.ApplyPromotion(ctx, workspaceLease, first)
	if err != nil {
		t.Fatal(err)
	}
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalLease, err := store.AcquireTurnLease(ctx, globalSession.ID, "retire-promotion-targets", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	retireClaimEvent, err := store.AppendEventWithLease(ctx, globalSession.ID, globalLease.HolderID, globalLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Retire the promoted Claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	retireClaim, err := store.PrepareMemoryLifecycle(ctx, globalSession.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000186", SourceEventID: retireClaimEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: firstResult.DestinationClaimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, globalLease, retireClaim); err != nil {
		t.Fatal(err)
	}
	secondEvent, err := store.AppendEventWithLease(ctx, workspaceSession.ID, workspaceLease.HolderID, workspaceLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote without reusing the retired Claim",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PreparePromotion(ctx, workspaceSession.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000187", SourceEventID: secondEvent.ID,
		SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !second.DestinationClaimCreate || second.DestinationClaim.ID == firstResult.DestinationClaimID {
		t.Fatalf("Promotion reused retired destination Claim: %+v", second.DestinationClaim)
	}
	approvePromotion(t, ctx, store, workspaceLease, second, memory.ApprovalApproved)
	secondResult, err := store.ApplyPromotion(ctx, workspaceLease, second)
	if err != nil {
		t.Fatal(err)
	}
	retiredEntityID := second.PromotedEntities[0].DestinationEntity.ID
	retireEntityEvent, err := store.AppendEventWithLease(ctx, globalSession.ID, globalLease.HolderID, globalLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Retire the promoted Entity",
	})
	if err != nil {
		t.Fatal(err)
	}
	retireEntity, err := store.PrepareMemoryLifecycle(ctx, globalSession.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000188", SourceEventID: retireEntityEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectEntity, ObjectID: retiredEntityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMemoryLifecycle(ctx, globalLease, retireEntity); err != nil {
		t.Fatal(err)
	}
	thirdEvent, err := store.AppendEventWithLease(ctx, workspaceSession.ID, workspaceLease.HolderID, workspaceLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote without reusing the retired Entity",
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.PreparePromotion(ctx, workspaceSession.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000189", SourceEventID: thirdEvent.ID,
		SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	foundReplacement := false
	for _, promoted := range third.PromotedEntities {
		if promoted.SourceEntityID == second.PromotedEntities[0].SourceEntityID {
			foundReplacement = promoted.DestinationEntity.Create && promoted.DestinationEntity.ID != retiredEntityID
		}
	}
	if !foundReplacement || !third.DestinationClaimCreate || third.DestinationClaim.ID == secondResult.DestinationClaimID {
		t.Fatalf("Promotion reused retired broader identity: %+v", third)
	}
	approvePromotion(t, ctx, store, workspaceLease, third, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, workspaceLease, third); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionPreparationRedactsSiblingSessionSourceTextButStillApplies(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Promotion redaction")
	if err != nil {
		t.Fatal(err)
	}
	sourceSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	sessionClaim := rememberScopeClaim(t, ctx, store, sourceSession, true, 190)
	sourceLease, err := store.AcquireTurnLease(ctx, sourceSession.ID, "session-to-workspace", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	workspaceEvent, err := store.AppendEventWithLease(ctx, sourceSession.ID, sourceLease.HolderID, sourceLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote to the Workspace",
	})
	if err != nil {
		t.Fatal(err)
	}
	workspacePromotion, err := store.PreparePromotion(ctx, sourceSession.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000190", SourceEventID: workspaceEvent.ID,
		SourceClaimID: sessionClaim.Claim.ID, DestinationScopeKey: "workspace:" + string(workspace.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	approvePromotion(t, ctx, store, sourceLease, workspacePromotion, memory.ApprovalApproved)
	workspaceResult, err := store.ApplyPromotion(ctx, sourceLease, workspacePromotion)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, sourceLease.SessionID, sourceLease.HolderID, sourceLease.FencingToken); err != nil {
		t.Fatal(err)
	}
	sibling, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	siblingLease, err := store.AcquireTurnLease(ctx, sibling.ID, "workspace-to-global", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	globalEvent, err := store.AppendEventWithLease(ctx, sibling.ID, siblingLease.HolderID, siblingLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote Workspace memory globally",
	})
	if err != nil {
		t.Fatal(err)
	}
	globalPromotion, err := store.PreparePromotion(ctx, sibling.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: "idem:v1:85000000-0000-4000-8003-000000000191", SourceEventID: globalEvent.ID,
		SourceClaimID: workspaceResult.DestinationClaimID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(globalPromotion.Sources) == 0 || globalPromotion.Sources[0].ID == "" || globalPromotion.Sources[0].EvidenceSHA256 == "" || globalPromotion.Sources[0].Evidence != "" {
		t.Fatalf("sibling-session Promotion source was not ID-auditable and text-redacted: %+v", globalPromotion.Sources)
	}
	approvePromotion(t, ctx, store, siblingLease, globalPromotion, memory.ApprovalApproved)
	if _, err := store.ApplyPromotion(ctx, siblingLease, globalPromotion); err != nil {
		t.Fatalf("apply Promotion with redacted source text: %v", err)
	}
}

func TestPromotionRejectsStaleSourceAndDestinationWithoutWrites(t *testing.T) {
	t.Run("source retraction", func(t *testing.T) {
		ctx, store, db, session, sourceClaim, lease, proposal := preparePromotionTest(t, 182)
		defer db.Close()
		retractionEvent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Retract the source",
		})
		if err != nil {
			t.Fatal(err)
		}
		retraction, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: "idem:v1:85000000-0000-4000-8000-000000000282", SourceEventID: retractionEvent.ID,
			Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink,
			ObjectID: sourceClaim.Source.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, retraction); err != nil {
			t.Fatal(err)
		}
		assertStalePromotionLeavesNoDestination(t, ctx, store, lease, proposal)
	})

	t.Run("destination revision", func(t *testing.T) {
		ctx, store, db, _, _, lease, proposal := preparePromotionTest(t, 183)
		defer db.Close()
		global, err := store.CreateGlobalSession(ctx)
		if err != nil {
			t.Fatal(err)
		}
		rememberScopeClaim(t, ctx, store, global, false, 283)
		assertStalePromotionLeavesNoDestination(t, ctx, store, lease, proposal)
	})
}

func TestPromotionTwoStoreRaceCommitsOneRevisionAndSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	storeA := NewStore(dbA)
	workspaces := make([]memory.Workspace, 2)
	sessions := make([]memory.Session, 2)
	claims := make([]memory.RememberEntityProposal, 2)
	leases := make([]memory.TurnLease, 2)
	proposals := make([]memory.PromotionProposal, 2)
	for index := range sessions {
		workspaces[index], err = storeA.RegisterWorkspace(ctx, fmt.Sprintf("Race %d", index))
		if err != nil {
			t.Fatal(err)
		}
		sessions[index], err = storeA.CreateWorkspaceSessionWithComposition(
			ctx, workspaces[index].ID, workspaces[index].CurrentRevisionID, standardReceipt(t),
		)
		if err != nil {
			t.Fatal(err)
		}
		claims[index] = rememberScopeClaim(t, ctx, storeA, sessions[index], false, 300+index)
		leases[index], err = storeA.AcquireTurnLease(
			ctx, sessions[index].ID, memory.LeaseHolderID(fmt.Sprintf("race-%d", index)), time.Minute,
		)
		if err != nil {
			t.Fatal(err)
		}
		event, err := storeA.AppendEventWithLease(ctx, sessions[index].ID, leases[index].HolderID, leases[index].FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Race Promotion",
		})
		if err != nil {
			t.Fatal(err)
		}
		proposals[index], err = storeA.PreparePromotion(ctx, sessions[index].ScopeContext(), memory.PromotionRequest{
			IdempotencyKey: fmt.Sprintf("idem:v1:85000000-0000-4000-8002-%012d", index+1),
			SourceEventID:  event.ID, SourceClaimID: claims[index].Claim.ID, DestinationScopeKey: "global",
		})
		if err != nil {
			t.Fatal(err)
		}
		approvePromotion(t, ctx, storeA, leases[index], proposals[index], memory.ApprovalApproved)
	}
	if proposals[0].DestinationScope.Revision != proposals[1].DestinationScope.Revision {
		t.Fatalf("Promotion race did not start at one destination revision: %d != %d",
			proposals[0].DestinationScope.Revision, proposals[1].DestinationScope.Revision)
	}
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	storeB := NewStore(dbB)
	stores := []*Store{storeA, storeB}
	errs := make([]error, 2)
	results := make([]memory.PromotionResult, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for index := range stores {
		go func() {
			defer wait.Done()
			results[index], errs[index] = stores[index].ApplyPromotion(ctx, leases[index], proposals[index])
		}()
	}
	wait.Wait()
	succeeded, stale := 0, 0
	var accepted memory.PromotionResult
	for index, applyErr := range errs {
		switch {
		case applyErr == nil:
			succeeded++
			accepted = results[index]
		case errors.Is(applyErr, ErrStaleScopeRevision):
			stale++
		default:
			t.Fatalf("Promotion race error[%d] = %v", index, applyErr)
		}
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("Promotion race succeeded=%d stale=%d errors=%v", succeeded, stale, errs)
	}
	for index := range leases {
		if err := storeA.ReleaseTurnLease(ctx, leases[index].SessionID, leases[index].HolderID, leases[index].FencingToken); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dbA.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedDB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := NewStore(reopenedDB)
	global, err := reopened.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	view, err := reopened.InspectClaims(ctx, global.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Claims) != 1 || view.Claims[0].ID != accepted.DestinationClaimID || view.ScopeRevision != accepted.DestinationRevision {
		t.Fatalf("reopened Promotion = %+v, accepted = %+v", view, accepted)
	}
}

func preparePromotionTest(t *testing.T, sequence int) (
	context.Context, *Store, *sql.DB, memory.Session, memory.RememberEntityProposal, memory.TurnLease, memory.PromotionProposal,
) {
	t.Helper()
	ctx, store, db, session, source, lease, proposal := prepareUnapprovedPromotionTest(t, sequence)
	approvePromotion(t, ctx, store, lease, proposal, memory.ApprovalApproved)
	return ctx, store, db, session, source, lease, proposal
}

func prepareUnapprovedPromotionTest(t *testing.T, sequence int) (
	context.Context, *Store, *sql.DB, memory.Session, memory.RememberEntityProposal, memory.TurnLease, memory.PromotionProposal,
) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, fmt.Sprintf("Promotion %d", sequence))
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	source := rememberScopeClaim(t, ctx, store, session, false, sequence)
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(fmt.Sprintf("promotion-%d", sequence)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote explicitly",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PreparePromotion(ctx, session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: fmt.Sprintf("idem:v1:85000000-0000-4000-8001-%012d", sequence),
		SourceEventID:  event.ID, SourceClaimID: source.Claim.ID, DestinationScopeKey: "global",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, db, session, source, lease, proposal
}

func approvePromotion(
	t *testing.T,
	ctx context.Context,
	store *Store,
	lease memory.TurnLease,
	proposal memory.PromotionProposal,
	decision memory.ApprovalDecision,
) {
	t.Helper()
	payload, err := json.Marshal(memory.ApprovalPayload{
		Decision:       decision,
		ProposalSHA256: proposal.ProposalSHA256,
		PreparedSHA256: proposal.PreparedSHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendPromotionApproval(t, ctx, store, lease, proposal, payload)
}

func appendPromotionApproval(
	t *testing.T,
	ctx context.Context,
	store *Store,
	lease memory.TurnLease,
	proposal memory.PromotionProposal,
	payload []byte,
) {
	t.Helper()
	if _, err := store.AppendEventWithLease(ctx, proposal.SessionID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID:    proposal.Evidence.EventID,
		Type:        memory.EventApproval,
		ExecutionID: memory.ExecutionID(proposal.OperationID),
		Payload:     payload,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertStalePromotionLeavesNoDestination(
	t *testing.T,
	ctx context.Context,
	store *Store,
	lease memory.TurnLease,
	proposal memory.PromotionProposal,
) {
	t.Helper()
	if _, err := store.ApplyPromotion(ctx, lease, proposal); !errors.Is(err, ErrStaleScopeRevision) {
		t.Fatalf("stale Promotion error = %v, want ErrStaleScopeRevision", err)
	}
	assertPromotionLeavesNoWrites(t, ctx, store, proposal)
}

func assertPromotionLeavesNoWrites(
	t *testing.T,
	ctx context.Context,
	store *Store,
	proposal memory.PromotionProposal,
) {
	t.Helper()
	var operations, claims int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE operation_id = ?`, proposal.OperationID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_claims WHERE claim_id = ?`, proposal.DestinationClaim.ID).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || claims != 0 {
		t.Fatalf("rejected Promotion wrote operation=%d destination_claim=%d", operations, claims)
	}
}

func rememberScopeClaim(t *testing.T, ctx context.Context, store *Store, session memory.Session, useSessionScope bool, sequence int) memory.RememberEntityProposal {
	t.Helper()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(fmt.Sprintf("scope-%d", sequence)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fmt.Sprintf("scope claim %d", sequence),
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: fmt.Sprintf("idem:v1:85000000-0000-4000-8000-%012d", sequence),
		SourceEventID:  source.ID, Predicate: "scope_marker", PredicateLabel: "scope marker",
		Subject:         memory.EntitySelector{Create: true, CanonicalName: fmt.Sprintf("subject-%d", sequence), EntityType: "concept", Alias: fmt.Sprintf("subject-%d", sequence)},
		Object:          memory.EntitySelector{Create: true, CanonicalName: fmt.Sprintf("object-%d", sequence), EntityType: "concept", Alias: fmt.Sprintf("object-%d", sequence)},
		UseSessionScope: useSessionScope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, proposal); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	return proposal
}
