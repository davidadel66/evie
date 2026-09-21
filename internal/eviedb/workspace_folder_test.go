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
