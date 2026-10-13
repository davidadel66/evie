package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestWorkspaceFolderPersistsWithoutChangingExistingSessionScope(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Learning")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	folder, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, root)
	if err != nil {
		t.Fatal(err)
	}
	if folder.Path == "" || folder.Revision != 1 {
		t.Fatalf("folder = %+v", folder)
	}
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, root); !errors.Is(err, ErrWorkspaceFolderChanged) {
		t.Fatalf("stale update = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Folder != folder || listed[0].CurrentRevisionID != ws.CurrentRevisionID {
		t.Fatalf("reopen = %+v", listed)
	}
	resumed, err := store.GetActiveSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ScopeContext() != session.ScopeContext() {
		t.Fatal("attaching a folder changed memory scope")
	}
	cwd, err := store.BindHistory(session.ID, "worker").WorkingDirectory(ctx)
	if err != nil || cwd != folder.Path {
		t.Fatalf("existing chat folder = %q, %v", cwd, err)
	}
	detached, err := store.SetWorkspaceFolder(ctx, ws.ID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if detached.Path != "" || detached.Revision != 2 {
		t.Fatalf("detach = %+v", detached)
	}
}

func TestActiveWorkspaceFolderServesOnlyActiveWorkspaces(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newTestDB(t))
	ws, err := store.RegisterWorkspace(ctx, "Finance")
	if err != nil {
		t.Fatal(err)
	}
	if path, err := store.ActiveWorkspaceFolder(ctx, ws.ID); err != nil || path != "" {
		t.Fatalf("folderless Workspace = %q, %v", path, err)
	}
	folder, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if path, err := store.ActiveWorkspaceFolder(ctx, ws.ID); err != nil || path != folder.Path {
		t.Fatalf("active Workspace = %q, %v, want %q", path, err, folder.Path)
	}
	if _, err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActiveWorkspaceFolder(ctx, ws.ID); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("archived Workspace: %v", err)
	}
	if _, err := store.ActiveWorkspaceFolder(ctx, "missing"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("missing Workspace: %v", err)
	}
}
