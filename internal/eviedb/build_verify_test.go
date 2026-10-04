package eviedb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// Stage 12: a Workspace's or project's setup and verify commands are the
// owner's, recorded as immutable revisions with a compare-and-set on the
// expected revision; the latest applies at once, every listing shows it, a
// stale writer is refused, bad commands are refused, and the store never
// lets a row change.
func TestBuildVerifyCommandsAreOwnerSetAndVersioned(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w, err := s.RegisterWorkspace(ctx, "Code")
	if err != nil {
		t.Fatal(err)
	}
	if w.Verify != (memory.BuildVerify{}) {
		t.Fatalf("a new Workspace has commands: %+v", w.Verify)
	}
	set, err := s.SetWorkspaceVerify(ctx, w.ID, 0, "  npm --prefix internal/web/ui ci ", "./scripts/verify-change.sh")
	if err != nil {
		t.Fatal(err)
	}
	if set.Verify != (memory.BuildVerify{Setup: "npm --prefix internal/web/ui ci", Verify: "./scripts/verify-change.sh", Revision: 1}) {
		t.Fatalf("set = %+v", set.Verify)
	}
	if listed := listedWorkspace(t, s, w.ID); listed.Verify != set.Verify {
		t.Fatalf("listed = %+v", listed.Verify)
	}
	if _, err := s.SetWorkspaceVerify(ctx, w.ID, 0, "", "make check"); !errors.Is(err, ErrBuildVerifyChanged) {
		t.Fatalf("stale writer = %v", err)
	}
	cleared, err := s.SetWorkspaceVerify(ctx, w.ID, 1, "", "make check")
	if err != nil || cleared.Verify != (memory.BuildVerify{Verify: "make check", Revision: 2}) {
		t.Fatalf("second revision = %+v %v", cleared.Verify, err)
	}
	for _, bad := range []string{"make\x00check", "make\rcheck", strings.Repeat("x", memory.MaxBuildCommandBytes+1)} {
		if _, err := s.SetWorkspaceVerify(ctx, w.ID, 2, "", bad); !errors.Is(err, ErrBuildVerifyCommand) {
			t.Fatalf("command %q = %v", bad[:min(len(bad), 12)], err)
		}
	}
	got, err := s.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: w.ID})
	if err != nil || got != cleared.Verify {
		t.Fatalf("scope lookup = %+v %v", got, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE build_verify_revisions SET verify_command='true'`); err == nil {
		t.Fatal("a recorded revision changed")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM build_verify_revisions`); err == nil {
		t.Fatal("a recorded revision was deleted")
	}

	project, err := s.RegisterProject(ctx, "Repo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.BuildVerifyFor(ctx, memory.ScopeContext{ProjectID: project.ID, ProjectRoot: project.CanonicalRoot}); err != nil || got != (memory.BuildVerify{}) {
		t.Fatalf("unset project = %+v %v", got, err)
	}
	p, err := s.SetProjectVerify(ctx, project.ID, 0, "", "go test ./...")
	if err != nil || p != (memory.BuildVerify{Verify: "go test ./...", Revision: 1}) {
		t.Fatalf("project = %+v %v", p, err)
	}
	if got, err := s.BuildVerifyFor(ctx, memory.ScopeContext{ProjectID: project.ID}); err != nil || got != p {
		t.Fatalf("project lookup = %+v %v", got, err)
	}
	if got, err := s.BuildVerifyFor(ctx, memory.ScopeContext{}); err != nil || got != (memory.BuildVerify{}) {
		t.Fatalf("a Global chat has commands: %+v %v", got, err)
	}
	if _, err := s.SetProjectVerify(ctx, "missing", 0, "", "true"); err == nil {
		t.Fatal("an unknown project was configured")
	}
	if _, err := s.SetWorkspaceVerify(ctx, "missing", 0, "", "true"); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("an unknown Workspace = %v", err)
	}
}
