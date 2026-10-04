package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
)

// worked is a build worktree on its branch with one harness commit that
// changes main.go and adds parser.go.
func worked(t *testing.T, repo string) delegation.Worktree {
	t.Helper()
	ctx := context.Background()
	w, err := Plan(ctx, repo, "", attemptID, "Add a parser")
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Path, "parser.go"), "package main\n\nfunc parse() {}\n")
	write(t, filepath.Join(w.Path, "main.go"), "package main\n\nfunc main() { parse() }\n")
	if _, err := Commit(ctx, w, "Add a parser"); err != nil {
		t.Fatal(err)
	}
	return w
}

// ownerCommit commits path with content on master in the owner's checkout,
// leaving the owner's other uncommitted edits alone.
func ownerCommit(t *testing.T, repo, path, content, message string) string {
	t.Helper()
	write(t, filepath.Join(repo, path), content)
	run(t, repo, "add", path)
	run(t, repo, "commit", "-q", "-m", message, "--", path)
	return run(t, repo, "rev-parse", "HEAD")
}

// Stage 12: the gate rebases the worker's branch onto the target's current
// head inside the worktree with the harness's hardening; a rebase that is
// not needed changes nothing; a conflicting one is aborted, leaves the
// branch where it was and names the conflicting files; a rebase a crash
// left behind is aborted on the next gate.
func TestRebaseOntoTheTargetAbortsConflictsAndLeftovers(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w := worked(t, repo)
	head, err := BranchHead(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	master, err := TargetHead(ctx, repo, "master")
	if err != nil || master != w.BaseCommit {
		t.Fatalf("target head = %q %v", master, err)
	}
	if r, err := Rebase(ctx, w, master); err != nil || r.Rebased || r.Head != head {
		t.Fatalf("rebase onto the base = %+v %v", r, err)
	}

	moved := ownerCommit(t, repo, "docs.md", "docs\n", "Owner docs")
	r, err := Rebase(ctx, w, moved)
	if err != nil || !r.Rebased || r.Head == head || len(r.Conflicts) != 0 {
		t.Fatalf("rebase onto a moved target = %+v %v", r, err)
	}
	if ok, err := IsAncestor(ctx, repo, moved, r.Head); err != nil || !ok {
		t.Fatalf("rebased branch does not contain the target: %v %v", ok, err)
	}
	if got := read(t, filepath.Join(w.Path, "docs.md")); got != "docs\n" {
		t.Fatalf("worktree after rebase lacks the target's file: %q", got)
	}
	summary, err := Compare(ctx, repo, moved, r.Head)
	if err != nil || len(summary.Commits) != 1 || summary.Commits[0].Subject != "Add a parser" || summary.FilesChanged != 2 {
		t.Fatalf("summary after rebase = %+v %v", summary, err)
	}

	// A conflicting change on the target: the rebase is aborted and the
	// branch stays exactly where it was.
	before := r.Head
	conflicting := ownerCommit(t, repo, "main.go", "package main\n\nfunc main() { println(\"owner\") }\n", "Owner main")
	r, err = Rebase(ctx, w, conflicting)
	if err != nil || r.Rebased || !slices.Equal(r.Conflicts, []string{"main.go"}) || r.Head != before {
		t.Fatalf("conflicting rebase = %+v %v", r, err)
	}
	if after, _ := BranchHead(ctx, w); after != before {
		t.Fatalf("branch moved to %s after an aborted rebase", after)
	}
	if aborted, err := AbortRebase(ctx, w); err != nil || aborted {
		t.Fatalf("a rebase was left in progress: %v %v", aborted, err)
	}
	if changed, err := TrackedChanges(ctx, w); err != nil || len(changed) != 0 {
		t.Fatalf("worktree after abort = %v %v", changed, err)
	}

	// A crash in the middle of a rebase leaves its state behind; the next
	// gate aborts it and the branch is intact.
	runAllowFail(t, w.Path, "-c", "core.hooksPath=/dev/null", "--git-dir="+w.GitDir, "--work-tree="+w.Path, "rebase", conflicting)
	if _, err := os.Stat(filepath.Join(w.GitDir, "rebase-merge")); err != nil {
		t.Skipf("git left no rebase state to recover: %v", err)
	}
	if aborted, err := AbortRebase(ctx, w); err != nil || !aborted {
		t.Fatalf("leftover rebase abort = %v %v", aborted, err)
	}
	if after, _ := BranchHead(ctx, w); after != before {
		t.Fatalf("branch after recovering a crashed rebase = %s, want %s", after, before)
	}
}

func runAllowFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_EDITOR=true")
	_ = cmd.Run()
}

func envRun(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com"), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runInput(t *testing.T, dir, input string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// Stage 12: the target is fast-forwarded in the owner's checkout when it is
// checked out there, keeping unrelated uncommitted edits; git refuses when
// the owner's uncommitted edits would be overwritten, and those edits stay
// exactly as they were. A branch that is not checked out moves by a
// compare-and-swap that fails if it moved since verification. The worker
// never runs any of this.
func TestFastForwardKeepsOwnerChangesAndUsesCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w := worked(t, repo)
	head, _ := BranchHead(ctx, w)
	master, _ := TargetHead(ctx, repo, "master")

	// Unrelated dirty file (README.md) and an untracked one: kept.
	write(t, filepath.Join(repo, "notes.txt"), "owner's untracked notes\n")
	where, err := FastForward(ctx, w, "master", master, head)
	if err != nil || !strings.Contains(where, repo) || !strings.Contains(where, "--ff-only") {
		t.Fatalf("fast-forward in the owner's checkout = %q %v", where, err)
	}
	if got := run(t, repo, "rev-parse", "HEAD"); got != head {
		t.Fatalf("owner's HEAD = %s, want %s", got, head)
	}
	if got := read(t, filepath.Join(repo, "README.md")); got != "# Repo\nowner's uncommitted edit\n" {
		t.Fatalf("owner's uncommitted edit changed: %q", got)
	}
	if got := read(t, filepath.Join(repo, "notes.txt")); got != "owner's untracked notes\n" {
		t.Fatalf("owner's untracked file changed: %q", got)
	}
	if got := read(t, filepath.Join(repo, "parser.go")); !strings.Contains(got, "parse") {
		t.Fatalf("merged file missing from the owner's checkout: %q", got)
	}
	if stash := run(t, repo, "stash", "list"); stash != "" {
		t.Fatalf("the merge stashed owner changes: %q", stash)
	}
}

func TestFastForwardRefusesWhenOwnerChangesConflict(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w := worked(t, repo)
	head, _ := BranchHead(ctx, w)
	master, _ := TargetHead(ctx, repo, "master")
	// The owner is editing main.go, which the branch changes.
	write(t, filepath.Join(repo, "main.go"), "package main\n\n// owner's work in progress\nfunc main() {}\n")
	run(t, repo, "config", "merge.autoStash", "true")
	_, err := FastForward(ctx, w, "master", master, head)
	var refused *MergeRefusedError
	if !errors.As(err, &refused) || !strings.Contains(refused.Error(), "main.go") {
		t.Fatalf("conflicting fast-forward = %v", err)
	}
	if got := run(t, repo, "rev-parse", "HEAD"); got != master {
		t.Fatalf("owner's HEAD moved to %s", got)
	}
	if got := read(t, filepath.Join(repo, "main.go")); got != "package main\n\n// owner's work in progress\nfunc main() {}\n" {
		t.Fatalf("owner's edit changed: %q", got)
	}
	if got := read(t, filepath.Join(repo, "README.md")); got != "# Repo\nowner's uncommitted edit\n" {
		t.Fatalf("owner's other edit changed: %q", got)
	}
	if stash := run(t, repo, "stash", "list"); stash != "" {
		t.Fatalf("the merge stashed owner changes: %q", stash)
	}
}

func TestFastForwardByCompareAndSwapWhenNotCheckedOut(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	run(t, repo, "branch", "release")
	w, err := Plan(ctx, repo, "release", attemptID, "Release fix")
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Path, "fix.txt"), "fix\n")
	if _, err := Commit(ctx, w, "Release fix"); err != nil {
		t.Fatal(err)
	}
	head, _ := BranchHead(ctx, w)
	release, _ := TargetHead(ctx, repo, "release")
	if at, found, err := CheckedOutAt(ctx, repo, "release"); err != nil || found {
		t.Fatalf("release checked out at %q %v", at, err)
	}
	masterBefore := run(t, repo, "rev-parse", "master")

	// It moved since verification: the swap fails and nothing changes.
	moved := ownerCommitOn(t, repo, "release", "hotfix.txt", "hotfix\n")
	if _, err := FastForward(ctx, w, "release", release, head); !errors.Is(err, ErrTargetMoved) {
		t.Fatalf("stale swap = %v", err)
	}
	if got := run(t, repo, "rev-parse", "release"); got != moved {
		t.Fatalf("release after a refused swap = %s, want %s", got, moved)
	}
	run(t, repo, "branch", "-f", "release", release)
	where, err := FastForward(ctx, w, "release", release, head)
	if err != nil || !strings.Contains(where, "compare-and-swap") {
		t.Fatalf("swap = %q %v", where, err)
	}
	if got := run(t, repo, "rev-parse", "release"); got != head {
		t.Fatalf("release = %s, want %s", got, head)
	}
	if got := run(t, repo, "rev-parse", "master"); got != masterBefore {
		t.Fatal("master moved")
	}
	// Only fast-forwards: a head that does not descend from the target is
	// refused before anything changes.
	if _, err := FastForward(ctx, w, "release", head, moved); err == nil || !strings.Contains(err.Error(), "cannot fast-forward") {
		t.Fatalf("a non-fast-forward was accepted: %v", err)
	}
	if got := run(t, repo, "rev-parse", "release"); got != head {
		t.Fatalf("release after a refused non-fast-forward = %s", got)
	}
}

// ownerCommitOn commits path on branch without checking it out.
func ownerCommitOn(t *testing.T, repo, branch, path, content string) string {
	t.Helper()
	blob := runInput(t, repo, content, "hash-object", "-w", "--stdin")
	parent := run(t, repo, "rev-parse", branch)
	index := filepath.Join(t.TempDir(), "idx")
	envRun(t, repo, []string{"GIT_INDEX_FILE=" + index}, "read-tree", parent)
	envRun(t, repo, []string{"GIT_INDEX_FILE=" + index}, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	newTree := envRun(t, repo, []string{"GIT_INDEX_FILE=" + index}, "write-tree")
	commit := run(t, repo, "commit-tree", newTree, "-p", parent, "-m", "Owner "+path)
	run(t, repo, "update-ref", "refs/heads/"+branch, commit)
	return commit
}

// Stage 12: setup with network runs only on the owner's history: a base
// reachable from a branch, tag or remote-tracking ref outside evie/, never a
// commit that exists only on a build worker's branch.
func TestOwnerHistoryExcludesUnmergedBuildBranches(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w := worked(t, repo)
	head, _ := BranchHead(ctx, w)
	if ok, err := OwnerHistoryContains(ctx, repo, w.BaseCommit); err != nil || !ok {
		t.Fatalf("base on master = %v %v", ok, err)
	}
	if ok, err := OwnerHistoryContains(ctx, repo, head); err != nil || ok {
		t.Fatalf("a worker-only commit counts as owner history: %v %v", ok, err)
	}
	run(t, repo, "tag", "reviewed", head)
	if ok, err := OwnerHistoryContains(ctx, repo, head); err != nil || !ok {
		t.Fatalf("a tagged commit = %v %v", ok, err)
	}
}

// Stage 12: Evie proposes setup and verify commands it finds in the
// repository's instructions; nothing is set by proposing.
func TestProposeCommandsFromRepositoryInstructions(t *testing.T) {
	repo := t.TempDir()
	write(t, filepath.Join(repo, "AGENTS.md"), "# Guide\n\n## Verification\n\nRun focused checks. Before handing off, run:\n\n```sh\n./scripts/verify-change.sh\n```\n\nInstall UI dependencies first with `npm --prefix internal/web/ui ci` when needed. Format with `gofmt`.\n")
	p := ProposeCommands(repo)
	if p.Verify != "./scripts/verify-change.sh" || p.Setup != "npm --prefix internal/web/ui ci" || p.VerifySource != "AGENTS.md" || p.SetupSource != "AGENTS.md" {
		t.Fatalf("proposal = %+v", p)
	}
	other := t.TempDir()
	write(t, filepath.Join(other, "CLAUDE.md"), "# Notes\n\n## Testing\n\n```bash\n$ make test\n```\n")
	if p := ProposeCommands(other); p.Verify != "make test" || p.Setup != "" || p.VerifySource != "CLAUDE.md" {
		t.Fatalf("CLAUDE.md proposal = %+v", p)
	}
	if p := ProposeCommands(t.TempDir()); p != (Proposal{}) {
		t.Fatalf("no instructions proposed %+v", p)
	}
	// An instructions file that links out of the repository is not read.
	linked := t.TempDir()
	outside := filepath.Join(t.TempDir(), "AGENTS.md")
	write(t, outside, "## Verification\n\n```sh\n./evil.sh\n```\n")
	if err := os.Symlink(outside, filepath.Join(linked, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if p := ProposeCommands(linked); p.Verify != "" {
		t.Fatalf("a linked-out instructions file was read: %+v", p)
	}
}
