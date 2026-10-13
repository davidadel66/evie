package eviedb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkspaceTabDataApprovalsPinAKeyPerTab(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newTestDB(t))
	ws, err := store.RegisterWorkspace(ctx, "Finance")
	if err != nil {
		t.Fatal(err)
	}
	key, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if err := store.ApproveWorkspaceTabData(ctx, ws.ID, "reports", key); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveWorkspaceTabData(ctx, ws.ID, "reports", other); err != nil {
		t.Fatal(err)
	}
	approvals, err := store.WorkspaceTabDataApprovals(ctx, ws.ID)
	if err != nil || len(approvals) != 1 || approvals["reports"] != other {
		t.Fatalf("approvals = %v, %v", approvals, err)
	}
	if err := store.WithdrawWorkspaceTabData(ctx, ws.ID, "reports"); err != nil {
		t.Fatal(err)
	}
	if approvals, _ := store.WorkspaceTabDataApprovals(ctx, ws.ID); len(approvals) != 0 {
		t.Fatalf("after withdraw = %v", approvals)
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 64)} {
		if err := store.ApproveWorkspaceTabData(ctx, ws.ID, "reports", bad); err == nil {
			t.Errorf("key %q was accepted", bad)
		}
	}
	if _, err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveWorkspaceTabData(ctx, ws.ID, "reports", key); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("approve in an archived Workspace: %v", err)
	}
}
