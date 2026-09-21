package eviedb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestRepositoryInstructionsPersistenceAndFencedSnapshots(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Repository")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("first instructions"), 0600)
	folder, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, root)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.RepositoryInstructionSettings(ctx, ws.ID)
	if err != nil || !settings.Enabled || settings.Revision != 0 {
		t.Fatalf("default=%+v %v", settings, err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, "test")
	turn, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "one"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := history.RepositoryInstructions(ctx, lease, turn.ID)
	if err != nil || snapshot.Text != "first instructions" {
		t.Fatalf("snapshot=%+v %v", snapshot, err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, session.ID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	childHistory := store.BindHistory(child.ID, "child")
	childLease, err := store.AcquireTurnLease(ctx, child.ID, "child", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	childTurn, err := childHistory.Append(ctx, childLease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "delegated task"})
	if err != nil {
		t.Fatal(err)
	}
	childSnapshot, err := childHistory.RepositoryInstructions(ctx, childLease, childTurn.ID)
	if err != nil || childSnapshot.WorkspaceID != "" || childSnapshot.Text != "" {
		t.Fatalf("child implicitly inherited instructions: %+v %v", childSnapshot, err)
	}
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("changed instructions"), 0600)
	repeated, err := history.RepositoryInstructions(ctx, lease, turn.ID)
	if err != nil || repeated.Text != snapshot.Text || repeated.SHA256 != snapshot.SHA256 {
		t.Fatalf("snapshot changed: %+v %v", repeated, err)
	}
	stale := lease
	stale.FencingToken++
	if _, err := history.RepositoryInstructions(ctx, stale, turn.ID); err == nil {
		t.Fatal("accepted stale lease")
	}
	if _, err := history.RepositoryInstructions(ctx, lease, "foreign"); err == nil {
		t.Fatal("accepted foreign root")
	}
	second, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "two"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := history.RepositoryInstructions(ctx, lease, second.ID)
	if err != nil || next.Text != "changed instructions" {
		t.Fatalf("next=%+v %v", next, err)
	}
	settings, err = store.SetRepositoryInstructionSettings(ctx, ws.ID, 0, false)
	if err != nil || settings.Revision != 1 {
		t.Fatal(err)
	}
	if _, err := store.SetRepositoryInstructionSettings(ctx, ws.ID, 0, true); !errors.Is(err, ErrRepositoryInstructionsChanged) {
		t.Fatalf("stale settings=%v", err)
	}
	currentFolder, err := store.WorkspaceFolder(ctx, ws.ID)
	if err != nil || currentFolder != folder {
		t.Fatal("toggle changed folder identity")
	}
	preview, err := history.PreviewRepositoryInstructions(ctx)
	if err != nil || preview.Status != "disabled" {
		t.Fatal("disabled preview")
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM repository_instruction_snapshots`).Scan(&count)
	if count != 2 {
		t.Fatalf("preview wrote snapshot: %d", count)
	}
	if _, err := db.Exec(`UPDATE repository_instruction_snapshots SET snapshot_json='{}'`); err == nil {
		t.Fatal("mutable snapshot")
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
	settings, err = store.RepositoryInstructionSettings(ctx, ws.ID)
	if err != nil || settings.Enabled || settings.Revision != 1 {
		t.Fatalf("restart setting=%+v %v", settings, err)
	}
	restored, err := store.RepositoryInstructionSnapshot(ctx, session.ID, turn.ID)
	if err != nil || restored.Text != snapshot.Text || restored.SHA256 != snapshot.SHA256 {
		t.Fatal("snapshot lost across restart")
	}
}
