package eviedb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/workspacetabs"
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

func TestWorkspaceTabProblemsKeepTheLatestPerTab(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newTestDB(t))
	ws, err := store.RegisterWorkspace(ctx, "Lab")
	if err != nil {
		t.Fatal(err)
	}
	for i := range keptTabProblems + 5 {
		if err := store.RecordWorkspaceTabProblem(ctx, ws.ID, workspacetabs.Problem{TabID: "reports", Kind: "error", Message: "boom " + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordWorkspaceTabProblem(ctx, ws.ID, workspacetabs.Problem{TabID: "budget", Kind: "blocked", Message: strings.Repeat("x", 900)}); err != nil {
		t.Fatal(err)
	}
	problems, err := store.WorkspaceTabProblems(ctx, ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != keptTabProblems+1 || problems[0].TabID != "budget" || len(problems[0].Message) != 500 || problems[1].Message != "boom y" {
		t.Fatalf("problems = %d, first = %+v, second = %+v", len(problems), problems[0], problems[1])
	}
	if err := store.ClearWorkspaceTabProblems(ctx, ws.ID, "reports"); err != nil {
		t.Fatal(err)
	}
	if problems, _ := store.WorkspaceTabProblems(ctx, ws.ID); len(problems) != 1 {
		t.Fatalf("after clearing reports = %v", problems)
	}
	if _, err := store.ArchiveWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordWorkspaceTabProblem(ctx, ws.ID, workspacetabs.Problem{TabID: "reports", Kind: "error", Message: "x"}); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("record in an archived Workspace: %v", err)
	}
}
