package eviedb_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestSemanticMemoryAcceptsCompoundEntityClaimAndAddsEvidenceToEquivalentClaim(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "entity-claim", time.Minute)
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

	firstSource := appendSource("Remember that Alice mentors Bob")
	first, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000001",
		SourceEventID:  firstSource.ID, Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Alice Example", EntityType: "person", Alias: "Alice"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Bob Example", EntityType: "person", Alias: "Bob"},
	})
	if err != nil {
		t.Fatalf("prepare compound entity Claim: %v", err)
	}
	if first.OperationID == "" || first.Predicate.ID == "" || first.Claim.ID == "" || first.Source.ID == "" {
		t.Fatalf("proposal omitted generated operation effects: %+v", first)
	}
	if len(first.Entities) != 4 || len(first.Aliases) != 2 || !first.Claim.Create || first.ResultingRevision != 1 ||
		len(first.ResultingRevisions) != 1 || first.ResultingRevisions[0].Revision != 1 {
		t.Fatalf("compound preview = %+v", first)
	}
	for _, entity := range first.Entities {
		if entity.ID == "" || !entity.Create {
			t.Fatalf("first proposal did not enumerate created Entity: %+v", entity)
		}
	}
	firstResult, err := store.ApplyRememberEntity(ctx, lease, first)
	if err != nil {
		t.Fatalf("apply compound entity Claim: %v", err)
	}
	if firstResult.ScopeRevision != 1 || firstResult.ClaimID != first.Claim.ID || firstResult.SourceLinkID != first.Source.ID {
		t.Fatalf("first result = %+v", firstResult)
	}

	secondSource := appendSource("Alice also mentors Bob")
	second, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000002",
		SourceEventID:  secondSource.ID, Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatalf("prepare equivalent entity Claim: %v", err)
	}
	if second.Claim.Create || second.Claim.ID != first.Claim.ID || second.Source.ID == first.Source.ID {
		t.Fatalf("equivalent Claim preview = %+v, first = %+v", second, first)
	}
	if len(second.Entities) != 4 {
		t.Fatalf("reused Entity preview omitted effects: %+v", second.Entities)
	}
	for _, entity := range second.Entities {
		if entity.Create {
			t.Fatalf("equivalent proposal recreated Entity: %+v", entity)
		}
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, second); err != nil {
		t.Fatalf("apply equivalent entity Claim: %v", err)
	}

	inspection, err := store.InspectEntityClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 2 || len(inspection.Claims) != 1 || len(inspection.Claims[0].Sources) != 2 {
		t.Fatalf("entity Claim inspection = %+v", inspection)
	}
	var firstOwner, firstEvie memory.SemanticID
	for _, entity := range first.Entities {
		switch entity.AnchorKind {
		case "owner":
			firstOwner = entity.ID
		case "evie":
			firstEvie = entity.ID
		}
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	inspection, err = store.InspectEntityClaims(ctx, session.ScopeContext())
	if err != nil || len(inspection.Claims) != 1 || len(inspection.Claims[0].Sources) != 2 {
		t.Fatalf("reopened entity Claim inspection = %+v, error = %v", inspection, err)
	}
	lease, err = store.AcquireTurnLease(ctx, session.ID, "entity-claim-reopen", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	reopenSource, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alice continues to mentor Bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000003",
		SourceEventID:  reopenSource.ID, Predicate: "mentors", PredicateLabel: "mentors",
		Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range reopened.Entities {
		if entity.AnchorKind == "owner" && (entity.Create || entity.ID != firstOwner) {
			t.Fatalf("owner anchor changed after reopen: %+v", entity)
		}
		if entity.AnchorKind == "evie" && (entity.Create || entity.ID != firstEvie) {
			t.Fatalf("Evie anchor changed after reopen: %+v", entity)
		}
	}
}

func TestSemanticMemoryLiteralAndEntityClaimsShareEquivalentEvidenceRules(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "literal-evidence", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(key, content string) memory.RememberLiteralProposal {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: "timezone_name", PredicateLabel: "timezone name",
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	first := prepare("idem:v1:71000000-0000-4000-8000-000000000030", "My timezone is Detroit")
	if _, err := store.ApplyRememberLiteral(ctx, lease, first); err != nil {
		t.Fatal(err)
	}
	second := prepare("idem:v1:71000000-0000-4000-8000-000000000031", "Detroit remains my timezone")
	if second.ClaimCreate || second.ClaimID != first.ClaimID || !second.Source.Create {
		t.Fatalf("equivalent literal preview = %+v, first = %+v", second, first)
	}
	if _, err := store.ApplyRememberLiteral(ctx, lease, second); err != nil {
		t.Fatal(err)
	}
	inspection, err := store.InspectLiteralClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 2 || len(inspection.Claims) != 1 || len(inspection.Claims[0].Sources) != 2 {
		t.Fatalf("literal evidence inspection = %+v", inspection)
	}
	reusedSource, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000032",
		SourceEventID:  first.Source.EventID, Predicate: "timezone_name", PredicateLabel: "timezone name",
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reusedSource.Source.Create || reusedSource.SourceLinkID != first.SourceLinkID || reusedSource.ClaimCreate {
		t.Fatalf("exact reused evidence preview = %+v", reusedSource)
	}
}

func TestSemanticMemoryKeepsAmbiguousAliasesAndRequiresStableIDForMutation(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "ambiguous-alias", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(key, content, subjectName, subjectAlias string, object memory.EntitySelector) memory.RememberEntityProposal {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: "knows", PredicateLabel: "knows",
			Subject: memory.EntitySelector{Create: true, CanonicalName: subjectName, EntityType: "person", Alias: subjectAlias},
			Object:  object,
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	first := prepare(
		"idem:v1:71000000-0000-4000-8000-000000000010", "Alex One knows Casey", "Alex One", " Alex ",
		memory.EntitySelector{Create: true, CanonicalName: "Casey", EntityType: "person", Alias: "Casey"},
	)
	if _, err := store.ApplyRememberEntity(ctx, lease, first); err != nil {
		t.Fatal(err)
	}
	second := prepare(
		"idem:v1:71000000-0000-4000-8000-000000000011", "Alex Two knows Casey", "Alex Two", "alex",
		memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	)
	if _, err := store.ApplyRememberEntity(ctx, lease, second); err != nil {
		t.Fatal(err)
	}

	matches, err := store.LookupEntitiesByAlias(ctx, session.ScopeContext(), "  ALEX  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Entity.ID == matches[1].Entity.ID {
		t.Fatalf("ambiguous exact Alias lookup = %+v", matches)
	}
	for _, match := range matches {
		if match.Entity.ID == "" || match.Entity.EntityType != "person" || match.Entity.ScopeKey != "global" ||
			match.Alias.ID == "" || match.Alias.OperationID == "" || match.Alias.SourceEventID == "" {
			t.Fatalf("Alias result omitted identity, type, scope, or provenance: %+v", match)
		}
	}

	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Alex knows Casey again",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000012",
		SourceEventID:  source.ID, Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{Alias: "alex"}, Object: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	})
	if !errors.Is(err, eviedb.ErrAmbiguousAlias) {
		t.Fatalf("ambiguous mutation error = %v, want ErrAmbiguousAlias", err)
	}
	inspection, inspectErr := store.InspectEntityClaims(ctx, session.ScopeContext())
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if inspection.ScopeRevision != 2 || len(inspection.Claims) != 2 {
		t.Fatalf("ambiguous mutation changed Semantic Memory: %+v", inspection)
	}
}

func TestSemanticMemoryScopedAliasLookupIncludesEligibleGlobalEntities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	globalLease, err := store.AcquireTurnLease(ctx, globalSession.ID, "global-alias", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	globalSource, err := store.AppendEventWithLease(ctx, globalSession.ID, globalLease.HolderID, globalLease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Global Alex knows Global Casey",
	})
	if err != nil {
		t.Fatal(err)
	}
	globalProposal, err := store.PrepareRememberEntity(ctx, globalSession.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000013", SourceEventID: globalSource.ID,
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Global Alex", EntityType: "person", Alias: "Alex"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Global Casey", EntityType: "person", Alias: "Casey"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, globalLease, globalProposal); err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "Scoped")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{ID: "44000000-0000-4000-8000-000000000001", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "scoped-alias", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Local Alex knows Local Casey",
	})
	if err != nil {
		t.Fatal(err)
	}
	localProposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000014", SourceEventID: source.ID,
		Predicate: "knows", PredicateLabel: "knows",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Local Alex", EntityType: "person", Alias: "alex"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Local Casey", EntityType: "person", Alias: "Local Casey"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, localProposal); err != nil {
		t.Fatal(err)
	}
	matches, err := store.LookupEntitiesByAlias(ctx, session.ScopeContext(), "ALEX")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].Entity.ScopeKey != "global" ||
		matches[1].Entity.ScopeKey != "workspace:"+string(workspace.ID) {
		t.Fatalf("scoped exact Alias lookup omitted global/local ambiguity: %+v", matches)
	}
}

func TestSemanticMemoryContextEntitySurvivesWorkspaceArchiveAndCompoundFailureRollsBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Memory Lab")
	if err != nil {
		t.Fatal(err)
	}
	session := memory.Session{
		ID: "41000000-0000-4000-8000-000000000001", WorkspaceID: workspace.ID,
		WorkspaceRevisionSnapshot: workspace.CurrentRevisionID, Status: memory.SessionActive,
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO sessions (id, workspace_id, workspace_revision_snapshot, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
	`, session.ID, session.WorkspaceID, session.WorkspaceRevisionSnapshot); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "context-entity", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	prepare := func(key, content string) memory.RememberEntityProposal {
		t.Helper()
		source, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
			IdempotencyKey: key, SourceEventID: source.ID, Predicate: "collaborates_with", PredicateLabel: "collaborates with",
			Subject: memory.EntitySelector{Create: true, CanonicalName: "Dana", EntityType: "person", Alias: "Dana"},
			Object:  memory.EntitySelector{Create: true, CanonicalName: "Eli", EntityType: "person", Alias: "Eli"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return proposal
	}
	failed := prepare("idem:v1:71000000-0000-4000-8000-000000000020", "Dana collaborates with Eli")
	var contextID memory.SemanticID
	for _, entity := range failed.Entities {
		if entity.AnchorKind == "context" {
			contextID = entity.ID
		}
	}
	if contextID == "" || failed.ResultingRevision != 1 {
		t.Fatalf("proposal omitted Context Entity or resulting revision: %+v", failed)
	}
	tamperedOperation := failed
	tamperedOperation.OperationID = "43000000-0000-4000-8000-000000000001"
	if _, err := store.ApplyRememberEntity(ctx, lease, tamperedOperation); err == nil {
		t.Fatal("proposal with an unapproved operation ID unexpectedly applied")
	}
	tamperedAnchor := failed
	tamperedAnchor.Entities = append([]memory.SemanticEntity(nil), failed.Entities...)
	for index := range tamperedAnchor.Entities {
		if tamperedAnchor.Entities[index].AnchorKind == "context" {
			tamperedAnchor.Entities[index].AnchorKind = "owner"
		}
	}
	if _, err := store.ApplyRememberEntity(ctx, lease, tamperedAnchor); err == nil {
		t.Fatal("proposal with an unapproved anchor kind unexpectedly applied")
	}
	failed.Aliases[1].ID = failed.Aliases[0].ID
	if _, err := store.ApplyRememberEntity(ctx, lease, failed); err == nil {
		t.Fatal("compound proposal with duplicate Alias ID unexpectedly applied")
	}
	inspection, err := store.InspectEntityClaims(ctx, session.ScopeContext())
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ScopeRevision != 0 || len(inspection.Claims) != 0 {
		t.Fatalf("failed compound operation left writes: %+v", inspection)
	}

	accepted := prepare("idem:v1:71000000-0000-4000-8000-000000000021", "Dana collaborates with Eli")
	if _, err := store.ApplyRememberEntity(ctx, lease, accepted); err != nil {
		t.Fatal(err)
	}
	acceptedContextID := memory.SemanticID("")
	for _, entity := range accepted.Entities {
		if entity.AnchorKind == "context" {
			acceptedContextID = entity.ID
		}
	}
	if _, err := store.RenameWorkspace(ctx, workspace.ID, "Renamed Memory Lab"); err != nil {
		t.Fatal(err)
	}
	renameSource, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Dana still collaborates with Eli",
	})
	if err != nil {
		t.Fatal(err)
	}
	afterRename, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000022", SourceEventID: renameSource.ID,
		Predicate: "collaborates_with", PredicateLabel: "collaborates with",
		Subject: memory.EntitySelector{EntityID: accepted.Claim.SubjectEntityID},
		Object:  memory.EntitySelector{EntityID: accepted.Claim.ObjectEntityID},
	})
	if err != nil {
		t.Fatalf("prepare after Workspace rename: %v", err)
	}
	for _, entity := range afterRename.Entities {
		if entity.AnchorKind == "context" && (entity.ID != acceptedContextID || entity.Create) {
			t.Fatalf("Workspace rename changed Context Entity: %+v", entity)
		}
	}
	if _, err := store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	matches, err := store.LookupEntitiesByAlias(ctx, session.ScopeContext(), "Dana")
	if err != nil || len(matches) != 1 {
		t.Fatalf("archived/reopened Workspace Alias lookup = %+v, error = %v", matches, err)
	}
	contextEntity, err := store.InspectSemanticEntity(ctx, session.ScopeContext(), acceptedContextID)
	if err != nil || contextEntity.ID != acceptedContextID || contextEntity.AnchorKind != "context" {
		t.Fatalf("archived Context Entity = %+v, error = %v", contextEntity, err)
	}
}

func TestSemanticMemoryCreatesOneContextEntityForProjectAndSessionRegistryScopes(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"project", "session"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			var session memory.Session
			useSessionScope := kind == "session"
			if kind == "project" {
				project, err := store.RegisterProject(ctx, "Context Project", t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				session = memory.Session{ID: "42000000-0000-4000-8000-000000000001", ProjectID: project.ID,
					ProjectRootSnapshot: project.CanonicalRoot, Status: memory.SessionActive}
				if _, err := db.ExecContext(ctx, `
					INSERT INTO sessions (id, project_id, project_root_snapshot, status, created_at, updated_at)
					VALUES (?, ?, ?, 'active', '2026-09-02T12:00:00Z', '2026-09-02T12:00:00Z')
				`, session.ID, session.ProjectID, session.ProjectRootSnapshot); err != nil {
					t.Fatal(err)
				}
			} else {
				session, err = store.CreateGlobalSession(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID("context-"+kind), time.Minute)
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
			firstSource := appendSource("A relates to B")
			first, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
				IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000040", SourceEventID: firstSource.ID,
				Predicate: "relates_to", PredicateLabel: "relates to", UseSessionScope: useSessionScope,
				Subject: memory.EntitySelector{Create: true, CanonicalName: "A", EntityType: "concept", Alias: "A"},
				Object:  memory.EntitySelector{Create: true, CanonicalName: "B", EntityType: "concept", Alias: "B"},
			})
			if err != nil {
				t.Fatal(err)
			}
			var firstContext memory.SemanticEntity
			for _, entity := range first.Entities {
				if entity.AnchorKind == "context" {
					firstContext = entity
				}
			}
			if firstContext.ID == "" || !firstContext.Create {
				t.Fatalf("first %s Context Entity = %+v", kind, firstContext)
			}
			if _, err := store.ApplyRememberEntity(ctx, lease, first); err != nil {
				t.Fatal(err)
			}
			secondSource := appendSource("A still relates to B")
			second, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
				IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000041", SourceEventID: secondSource.ID,
				Predicate: "relates_to", PredicateLabel: "relates to", UseSessionScope: useSessionScope,
				Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID},
				Object:  memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
			})
			if err != nil {
				t.Fatal(err)
			}
			var secondContext memory.SemanticEntity
			for _, entity := range second.Entities {
				if entity.AnchorKind == "context" {
					secondContext = entity
				}
			}
			if secondContext.ID != firstContext.ID || secondContext.Create {
				t.Fatalf("%s Context Entity was not reused: first=%+v second=%+v", kind, firstContext, secondContext)
			}
			if kind == "project" {
				sessionSource := appendSource("A relates to B for this session")
				sessionProposal, err := store.PrepareRememberEntity(ctx, session.ScopeContext(), memory.RememberEntityRequest{
					IdempotencyKey: "idem:v1:71000000-0000-4000-8000-000000000042", SourceEventID: sessionSource.ID,
					Predicate: "relates_to", PredicateLabel: "relates to", UseSessionScope: true,
					Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID},
					Object:  memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
				})
				if err != nil {
					t.Fatalf("prepare session Claim using project Entities: %v", err)
				}
				if len(sessionProposal.Scopes) != 3 || sessionProposal.Scope.Key != "session:"+string(session.ID) {
					t.Fatalf("session proposal scope vector = %+v", sessionProposal.Scopes)
				}
				if _, err := store.ApplyRememberEntity(ctx, lease, sessionProposal); err != nil {
					t.Fatalf("apply session Claim using project Entities: %v", err)
				}
				sessionClaims, err := store.InspectEntityClaimsAtScope(ctx, session.ScopeContext(), true)
				if err != nil || sessionClaims.Scope.Key != "session:"+string(session.ID) || len(sessionClaims.Claims) != 1 {
					t.Fatalf("session exact Claim read = %+v, error = %v", sessionClaims, err)
				}
				matches, err := store.LookupEntitiesByAliasAtScope(ctx, session.ScopeContext(), "A", true)
				if err != nil || len(matches) != 1 || matches[0].Entity.ID != first.Claim.SubjectEntityID ||
					matches[0].Entity.ScopeKey != "project:"+string(session.ProjectID) {
					t.Fatalf("session Alias lookup across project Context = %+v, error = %v", matches, err)
				}
				entity, err := store.InspectSemanticEntityAtScope(ctx, session.ScopeContext(), first.Claim.SubjectEntityID, true)
				if err != nil || entity.ID != first.Claim.SubjectEntityID || entity.ScopeKey != "project:"+string(session.ProjectID) {
					t.Fatalf("session stable-ID read across project Context = %+v, error = %v", entity, err)
				}
			}
		})
	}
}
