package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestWorkspaceRegistryPersistsStableIdentityAcrossRenameAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(context.Background(), "Cairo's Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.ID == "" || workspace.CurrentRevisionID == "" || workspace.State != memory.WorkspaceActive ||
		workspace.CreatedAt.IsZero() || workspace.CreatedAt.Location() != time.UTC || workspace.UpdatedAt != workspace.CreatedAt {
		t.Fatalf("registered workspace=%+v", workspace)
	}
	renamed, err := store.RenameWorkspace(context.Background(), workspace.ID, "Cairo Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != workspace.ID || renamed.CurrentRevisionID != workspace.CurrentRevisionID ||
		renamed.DisplayName != "Cairo Kitchen" || renamed.CreatedAt != workspace.CreatedAt || !renamed.UpdatedAt.After(workspace.UpdatedAt) {
		t.Fatalf("renamed workspace=%+v, original=%+v", renamed, workspace)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	listed, err := NewStore(db).ListWorkspaces(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0] != renamed {
		t.Fatalf("reopened workspaces=%+v, want %+v", listed, renamed)
	}
}

func TestWorkspaceDisplayNamesDoNotOwnIdentity(t *testing.T) {
	store := NewStore(newTestDB(t))
	first, err := store.RegisterWorkspace(context.Background(), "Cairo")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.RegisterWorkspace(context.Background(), "Cairo")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.CurrentRevisionID == second.CurrentRevisionID {
		t.Fatalf("same-name workspaces share identity: first=%+v second=%+v", first, second)
	}
}

func TestWorkspaceSessionCreationPinsScopeAndCompositionAtomicallyAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ctx := context.Background()
	workspace, err := store.RegisterWorkspace(ctx, "Cairo's Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if session.WorkspaceID != workspace.ID || session.WorkspaceRevisionSnapshot != workspace.CurrentRevisionID ||
		session.ProjectID != "" || session.ProjectRootSnapshot != "" {
		t.Fatalf("workspace session=%+v", session)
	}
	if scope := session.ScopeContext(); scope.WorkspaceID != workspace.ID ||
		scope.WorkspaceRevision != workspace.CurrentRevisionID || scope.ProjectID != "" {
		t.Fatalf("workspace scope=%+v", scope)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reopened := NewStore(db)
	loaded, err := reopened.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != session {
		t.Fatalf("reopened session=%+v, want %+v", loaded, session)
	}
	gotReceipt, err := reopened.GetCompositionReceipt(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotReceipt.Preset != receipt.Preset || len(gotReceipt.Capabilities) != len(receipt.Capabilities) {
		t.Fatalf("reopened receipt=%+v, want %+v", gotReceipt, receipt)
	}
}

func TestWorkspaceSessionChooserRejectsStaleOrArchivedSelection(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	workspace, err := store.RegisterWorkspace(ctx, "Cairo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspaceSessionForChooserWithComposition(
		ctx, workspace.ID, memory.WorkspaceRevisionID("stale"), standardReceipt(t),
	); !errors.Is(err, ErrChooserStateChanged) {
		t.Fatalf("stale revision error=%v, want ErrChooserStateChanged", err)
	}
	assertSessionCount(t, db, 0)
	if _, err := store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspaceSessionForChooserWithComposition(
		ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t),
	); !errors.Is(err, ErrChooserStateChanged) {
		t.Fatalf("archived selection error=%v, want ErrChooserStateChanged", err)
	}
	assertSessionCount(t, db, 0)
}

func TestWorkspaceSessionScopeIsExclusiveAndImmutableInSQLite(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	workspace, err := store.RegisterWorkspace(ctx, "Cairo")
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.RegisterProject(ctx, "Evie", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for name, values := range map[string][]any{
		"workspace without revision": {"workspace-no-revision", workspace.ID, nil, nil, nil, now, now},
		"revision without workspace": {"revision-no-workspace", nil, workspace.CurrentRevisionID, nil, nil, now, now},
		"workspace and project":      {"both", workspace.ID, workspace.CurrentRevisionID, project.ID, project.CanonicalRoot, now, now},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := db.Exec(`
				INSERT INTO sessions (
					id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
					status, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, 'active', ?, ?)
			`, values...); err == nil {
				t.Fatal("invalid Context Scope insert succeeded")
			}
		})
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET workspace_id = NULL, workspace_revision_snapshot = NULL WHERE id = ?`, session.ID); err == nil {
		t.Fatal("mutating a session Context Scope succeeded")
	}
}

func TestConcurrentWorkspaceSessionCreationKeepsIndependentAtomicSnapshots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbA.Close() })
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dbB.Close() })
	workspace, err := NewStore(dbA).RegisterWorkspace(context.Background(), "Cairo")
	if err != nil {
		t.Fatal(err)
	}

	stores := []*Store{NewStore(dbA), NewStore(dbB)}
	sessions := make([]memory.Session, len(stores))
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			sessions[i], errs[i] = stores[i].CreateWorkspaceSessionForChooserWithComposition(
				context.Background(), workspace.ID, workspace.CurrentRevisionID, standardReceipt(t),
			)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if sessions[i].WorkspaceID != workspace.ID || sessions[i].WorkspaceRevisionSnapshot != workspace.CurrentRevisionID {
			t.Fatalf("session %d=%+v", i, sessions[i])
		}
		if _, err := stores[i].GetCompositionReceipt(context.Background(), sessions[i].ID); err != nil {
			t.Fatalf("receipt %d: %v", i, err)
		}
	}
	if sessions[0].ID == sessions[1].ID {
		t.Fatalf("concurrent creations reused session ID %q", sessions[0].ID)
	}
}

func TestWorkspaceEventScopeFollowsImmutableSession(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	workspace, err := store.RegisterWorkspace(ctx, "Cairo")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, "worker", lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.WorkspaceID != workspace.ID || event.ProjectID != "" {
		t.Fatalf("workspace event=%+v", event)
	}
	loaded, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].WorkspaceID != workspace.ID {
		t.Fatalf("loaded events=%+v", loaded)
	}

	if _, err := db.Exec(`INSERT INTO events (
		id, session_id, sequence, workspace_id, event_type, content, payload_json, recorded_at
	) VALUES ('bad', ?, 2, NULL, 'user_message', 'bad', '{}', ?)`, session.ID, time.Now().UTC().Format(time.RFC3339Nano)); err == nil {
		t.Fatal("event with a widened scope succeeded")
	}
}
