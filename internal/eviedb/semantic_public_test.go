package eviedb_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestSemanticMemoryAcceptsOneSourcedLiteralClaimAndSurvivesReopen(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-test", time.Minute)
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
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000001",
		SourceEventID:  source.ID,
		Predicate:      "timezone_name",
		PredicateLabel: "timezone name",
		Literal:        memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatalf("prepare remember: %v", err)
	}
	if proposal.Scope.Key != "global" || proposal.ExpectedRevision != 0 || proposal.Source.EventID != source.ID {
		t.Fatalf("proposal = %+v", proposal)
	}
	if proposal.OperationID == "" || proposal.Predicate.ID == "" || proposal.Subject.ID == "" ||
		proposal.ClaimID == "" || proposal.SourceLinkID == "" {
		t.Fatalf("proposal omitted generated IDs: %+v", proposal)
	}

	result, err := store.ApplyRememberLiteral(ctx, lease, proposal)
	if err != nil {
		t.Fatalf("apply remember: %v", err)
	}
	if result.OperationID != proposal.OperationID || result.ScopeRevision != 1 {
		t.Fatalf("result = %+v", result)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatalf("inspect memory: %v", err)
	}
	if inspection.ScopeRevision != 1 || len(inspection.Claims) != 1 {
		t.Fatalf("inspection = %+v", inspection)
	}
	claim := inspection.Claims[0]
	if claim.ID != proposal.ClaimID || claim.OperationID != proposal.OperationID ||
		claim.Source.EventID != source.ID || claim.Source.ID != proposal.SourceLinkID ||
		claim.Scope.Key != "global" || claim.EffectiveAt.IsZero() {
		t.Fatalf("claim inspection = %+v", claim)
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
	if err != nil {
		t.Fatalf("inspect reopened memory: %v", err)
	}
	if len(inspection.Claims) != 1 || inspection.Claims[0].ID != claim.ID ||
		inspection.Claims[0].OperationID != claim.OperationID || inspection.Claims[0].Source.ID != claim.Source.ID ||
		inspection.ScopeRevision != 1 {
		t.Fatalf("reopened inspection = %+v, want identities from %+v", inspection, claim)
	}
}

func TestSemanticMemoryIdempotencyAndStaleProposalChangeNothing(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendSource := func(content string) memory.Event {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	prepare := func(key string, source memory.Event, value string) memory.RememberLiteralProposal {
		t.Helper()
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: "timezone_name",
			PredicateLabel: "timezone name", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}

	firstSource := appendSource("/remember timezone_name Detroit")
	first := prepare("idem:v1:70000000-0000-4000-8000-000000000010", firstSource, "Detroit")
	staleSource := appendSource("/remember timezone_name Chicago")
	stale := prepare("idem:v1:70000000-0000-4000-8000-000000000011", staleSource, "Chicago")

	original, err := store.ApplyRememberLiteral(ctx, lease, first)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.ApplyRememberLiteral(ctx, lease, first)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if replayed.OperationID != original.OperationID || replayed.ClaimID != original.ClaimID ||
		replayed.SourceLinkID != original.SourceLinkID || replayed.ScopeRevision != original.ScopeRevision ||
		!replayed.TransactionTime.Equal(original.TransactionTime) {
		t.Fatalf("replayed result = %+v, want %+v", replayed, original)
	}

	changed := first
	changed.Literal.Value = "changed under same key"
	if _, err := store.ApplyRememberLiteral(ctx, lease, changed); !errors.Is(err, eviedb.ErrIdempotencyConflict) {
		t.Fatalf("changed idempotency replay error = %v, want ErrIdempotencyConflict", err)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, stale); !errors.Is(err, eviedb.ErrStaleScopeRevision) {
		t.Fatalf("stale apply error = %v, want ErrStaleScopeRevision", err)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 1 || len(inspection.Claims) != 1 || inspection.Claims[0].ID != original.ClaimID {
		t.Fatalf("failed/replayed operations changed projection: %+v", inspection)
	}
}

func TestSemanticMemoryUsesTheSessionBoundWorkspaceScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{
		ID: "40000000-0000-4000-8000-000000000020", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-01T12:00:00Z', '2026-09-01T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-workspace", time.Minute)
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
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000020", SourceEventID: source.ID,
		Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantScope := "workspace:" + string(workspace.ID)
	if proposal.Scope.Key != wantScope || len(proposal.PriorRevisions) != 2 ||
		proposal.PriorRevisions[0].ScopeKey != "global" || proposal.PriorRevisions[1].ScopeKey != wantScope {
		t.Fatalf("Workspace proposal scopes = %+v", proposal)
	}
	result, err := store.ApplyRememberLiteral(ctx, lease, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScopeRevision != 1 || len(result.ResultingRevisions) != 2 ||
		result.ResultingRevisions[0].Revision != 1 || result.ResultingRevisions[1].Revision != 1 {
		t.Fatalf("Workspace operation result = %+v", result)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Scope.Key != wantScope || inspection.ScopeRevision != 1 || len(inspection.Claims) != 1 {
		t.Fatalf("Workspace inspection = %+v", inspection)
	}
}

func TestSemanticMemoryRevalidatesSessionScopeAndApprovedFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Bound")
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := store.RegisterWorkspace(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{
		ID: "40000000-0000-4000-8000-000000000030", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-01T12:00:00Z', '2026-09-01T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-boundary", time.Minute)
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
		IdempotencyKey: "idem:v1:70000000-0000-4000-8000-000000000030", SourceEventID: source.ID,
		Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}

	otherKey := "workspace:" + string(otherWorkspace.ID)
	wrongScope := proposal
	wrongScope.Scopes = append([]memory.SemanticScope(nil), proposal.Scopes...)
	wrongScope.PriorRevisions = append([]memory.ScopeRevision(nil), proposal.PriorRevisions...)
	wrongScope.ProposalSHA256 = ""
	wrongScope.Scope = memory.SemanticScope{ID: "10000000-0000-4000-8000-000000000030", Key: otherKey, RegistryID: string(otherWorkspace.ID)}
	wrongScope.Source.ScopeKey = otherKey
	wrongScope.Scopes[1] = wrongScope.Scope
	wrongScope.PriorRevisions[1] = memory.ScopeRevision{ScopeKey: otherKey, Revision: 0}
	if _, err := store.ApplyRememberLiteral(ctx, lease, wrongScope); err == nil {
		t.Fatal("proposal escaped its session-bound Workspace")
	}

	tampered := []struct {
		name   string
		mutate func(*memory.RememberLiteralProposal)
	}{
		{name: "duplicate scope", mutate: func(p *memory.RememberLiteralProposal) {
			p.Scopes[0] = p.Scopes[1]
		}},
		{name: "unsorted scopes", mutate: func(p *memory.RememberLiteralProposal) {
			p.Scopes[0], p.Scopes[1] = p.Scopes[1], p.Scopes[0]
		}},
		{name: "duplicate prior revision", mutate: func(p *memory.RememberLiteralProposal) {
			p.PriorRevisions[0] = p.PriorRevisions[1]
		}},
		{name: "unsorted prior revisions", mutate: func(p *memory.RememberLiteralProposal) {
			p.PriorRevisions[0], p.PriorRevisions[1] = p.PriorRevisions[1], p.PriorRevisions[0]
		}},
		{name: "polarity", mutate: func(p *memory.RememberLiteralProposal) { p.Polarity = "denied" }},
		{name: "valid time", mutate: func(p *memory.RememberLiteralProposal) {
			value := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
			p.ValidTime.From = &value
		}},
		{name: "source actor", mutate: func(p *memory.RememberLiteralProposal) { p.Source.Actor = "assistant" }},
	}
	for _, test := range tampered {
		t.Run(test.name, func(t *testing.T) {
			attempt := proposal
			attempt.Scopes = append([]memory.SemanticScope(nil), proposal.Scopes...)
			attempt.PriorRevisions = append([]memory.ScopeRevision(nil), proposal.PriorRevisions...)
			attempt.ProposalSHA256 = ""
			test.mutate(&attempt)
			if _, err := store.ApplyRememberLiteral(ctx, lease, attempt); err == nil {
				t.Fatalf("tampered %s proposal applied", test.name)
			}
		})
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 0 || len(inspection.Claims) != 0 {
		t.Fatalf("rejected proposals changed Semantic Memory: %+v", inspection)
	}
}

func TestSemanticMemoryReusesStableOwnerAndEvieAnchors(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "semantic-anchors", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(key, command, predicate, value string) memory.RememberLiteralProposal {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: command,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: predicate,
			PredicateLabel: strings.ReplaceAll(predicate, "_", " "),
			Literal:        memory.TypedLiteral{Kind: memory.LiteralText, Value: value},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	first := prepare("idem:v1:70000000-0000-4000-8000-000000000040", "/remember timezone_name Detroit", "timezone_name", "Detroit")
	if !first.Subject.Create || !first.Evie.Create || first.Subject.ID == "" || first.Evie.ID == "" || first.Subject.ID == first.Evie.ID {
		t.Fatalf("first anchor proposal = owner %+v Evie %+v", first.Subject, first.Evie)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, first); err != nil {
		t.Fatal(err)
	}
	second := prepare("idem:v1:70000000-0000-4000-8000-000000000041", "/remember home_city Detroit", "home_city", "Detroit")
	if second.Subject.Create || second.Evie.Create || second.Subject.ID != first.Subject.ID || second.Evie.ID != first.Evie.ID {
		t.Fatalf("anchors were not stable: first owner/Evie=%+v/%+v second=%+v/%+v", first.Subject, first.Evie, second.Subject, second.Evie)
	}
}
