package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/workflow"
)

// workspace-mcp-connections.spec.md: a chat created in a Workspace pins the
// Workspace Access revision current at creation; later additions reach new
// chats only.
func TestWorkspaceSessionsPinTheAccessRevisionAtCreation(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "CairosKitchen")
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)

	before, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if pinnedWS, revision, err := store.SessionWorkspaceAccessPin(ctx, before.ID); err != nil || pinnedWS != ws.ID || revision != 0 {
		t.Fatalf("pin before any access = %q, %d, %v", pinnedWS, revision, err)
	}
	square := workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: "mcp-square", Resources: []string{"make_api_request_read@abc"}}}}
	if _, err := store.SetWorkspaceAccess(ctx, ws.ID, 0, square); err != nil {
		t.Fatal(err)
	}
	after, err := store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, ws.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, revision, err := store.SessionWorkspaceAccessPin(ctx, after.ID); err != nil || revision != 1 {
		t.Fatalf("pin after access = %d, %v", revision, err)
	}
	if _, revision, _ := store.SessionWorkspaceAccessPin(ctx, before.ID); revision != 0 {
		t.Fatalf("an existing chat's pin moved to %d", revision)
	}

	at0, err := store.WorkspaceAccessAt(ctx, ws.ID, 0)
	if err != nil || at0.Revision != 0 || len(at0.Access.Connections) != 0 {
		t.Fatalf("access at 0 = %+v, %v", at0, err)
	}
	at1, err := store.WorkspaceAccessAt(ctx, ws.ID, 1)
	if err != nil || at1.Revision != 1 || len(at1.Access.Connections) != 1 || at1.Access.Connections[0].ID != "mcp-square" {
		t.Fatalf("access at 1 = %+v, %v", at1, err)
	}
	if _, err := store.WorkspaceAccessAt(ctx, ws.ID, 7); !errors.Is(err, ErrWorkspaceAccessRevisionNotFound) {
		t.Fatalf("access at a missing revision error = %v", err)
	}

	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.SessionWorkspaceAccessPin(ctx, global.ID); !errors.Is(err, ErrNoWorkspaceAccessPin) {
		t.Fatalf("a global chat's pin error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE session_workspace_access_pins SET access_revision=5`); err == nil {
		t.Fatal("a pin was updated")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM session_workspace_access_pins`); err == nil {
		t.Fatal("a pin was deleted")
	}
}
