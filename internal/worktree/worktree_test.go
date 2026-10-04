package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// repository is an owner's checkout with one commit on master and an
// uncommitted change the worker must never see or touch.
func repository(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := filepath.Join(base, "repo")
	write(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	write(t, filepath.Join(repo, "README.md"), "# Repo\n")
	run(t, repo, "init", "-q", "-b", "master")
	run(t, repo, "config", "user.name", "Owner")
	run(t, repo, "config", "user.email", "owner@example.com")
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-q", "-m", "first")
	write(t, filepath.Join(repo, "README.md"), "# Repo\nowner's uncommitted edit\n")
	return repo
}

const attemptID = "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0"

// Stage 11: planning resolves the worktree, branch, base and temp folder
// without changing anything, and refuses a folder that is not a
// repository's top level, a repository mid-merge or mid-rebase, and a base
// it cannot resolve.
func TestPlanResolvesWithoutSideEffectsAndRefusesUnsafeRepositories(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	head := run(t, repo, "rev-parse", "HEAD")
	w, err := Plan(ctx, repo, "", attemptID, "Add a parser for *TOML* files, please!")
	if err != nil {
		t.Fatal(err)
	}
	want := delegation.Worktree{Repo: repo, CommonDir: filepath.Join(repo, ".git"), GitDir: filepath.Join(repo, ".git", "worktrees", attemptID),
		Path: filepath.Join(repo, ".evie", "worktrees", attemptID), Temp: filepath.Join(repo, ".evie", "tmp", attemptID),
		Branch: "evie/0f1e2d3c-add-a-parser-for-toml-files-please", Base: "master", BaseCommit: head}
	if w != want {
		t.Fatalf("plan = %+v\nwant  %+v", w, want)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".evie")); !os.IsNotExist(err) {
		t.Fatal("planning created .evie")
	}
	if out := run(t, repo, "branch", "--list", "evie/*"); out != "" {
		t.Fatalf("planning created a branch: %s", out)
	}
	if w, err := Plan(ctx, repo, "master~0", attemptID, "x"); err != nil || w.BaseCommit != head || w.Base != "master~0" {
		t.Fatalf("named base: %+v %v", w, err)
	}

	refusals := []struct {
		name   string
		folder string
		base   string
		setup  func(t *testing.T, repo string)
		want   string
	}{
		{"not a repository", t.TempDir(), "", nil, "not a git repository"},
		{"subfolder", "", "", func(t *testing.T, repo string) { write(t, filepath.Join(repo, "sub", "x"), "x") }, "top level"},
		{"merge in progress", "", "", func(t *testing.T, repo string) { write(t, filepath.Join(repo, ".git", "MERGE_HEAD"), head+"\n") }, "merge"},
		{"rebase in progress", "", "", func(t *testing.T, repo string) {
			if err := os.MkdirAll(filepath.Join(repo, ".git", "rebase-merge"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, "rebase"},
		{"unknown base", "", "no-such-branch", nil, "cannot be resolved"},
		{"option base", "", "--output=/tmp/x", nil, "not a valid"},
		{"range base", "", "master..evil", nil, "not a valid"},
		{"detached head", "", "", func(t *testing.T, repo string) { run(t, repo, "checkout", "-q", "--detach") }, "detached"},
		{"tracked .evie", "", "", func(t *testing.T, repo string) {
			write(t, filepath.Join(repo, ".evie", "keep"), "x")
			run(t, repo, "add", "-f", ".evie/keep")
		}, "tracks .evie"},
		{"symlinked .evie", "", "", func(t *testing.T, repo string) {
			if err := os.Symlink(t.TempDir(), filepath.Join(repo, ".evie")); err != nil {
				t.Fatal(err)
			}
		}, ".evie"},
	}
	for _, c := range refusals {
		t.Run(c.name, func(t *testing.T) {
			repo := repository(t)
			folder := c.folder
			if folder == "" {
				folder = repo
			}
			if c.setup != nil {
				c.setup(t, repo)
			}
			if c.name == "subfolder" {
				folder = filepath.Join(repo, "sub")
			}
			if _, err := Plan(ctx, folder, c.base, attemptID, "x"); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("plan = %v; want refusal naming %q", err, c.want)
			}
		})
	}
}

// Stage 11: the harness creates the worktree on a fresh branch from the
// pinned base commit, keeps .evie out of the owner's status with the local
// info/exclude, and never touches the owner's checkout. Ensuring again finds
// the same worktree.
func TestEnsureCreatesTheWorktreeOnAFreshBranch(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	before := run(t, repo, "status", "--porcelain")
	w, err := Plan(ctx, repo, "", attemptID, "Add parser")
	if err != nil {
		t.Fatal(err)
	}
	// A post-checkout hook in the owner's repository never runs.
	marker := filepath.Join(filepath.Dir(repo), "hook-ran")
	write(t, filepath.Join(repo, ".git", "hooks", "post-checkout"), "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "post-checkout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the repository's post-checkout hook ran")
	}
	if got := run(t, w.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != w.Branch {
		t.Fatalf("worktree on %q, want %q", got, w.Branch)
	}
	if got := read(t, filepath.Join(w.Path, "README.md")); got != "# Repo\n" {
		t.Fatalf("worktree saw the owner's uncommitted change: %q", got)
	}
	if got := read(t, filepath.Join(w.Path, ".git")); strings.TrimSpace(got) != "gitdir: "+w.GitDir {
		t.Fatalf("worktree .git = %q, want gitdir %s", got, w.GitDir)
	}
	if info, err := os.Stat(w.Temp); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("temp folder: %v %v", info, err)
	}
	if !strings.Contains(read(t, filepath.Join(repo, ".git", "info", "exclude")), "\n/.evie/\n") {
		t.Fatalf("info/exclude lacks /.evie/: %q", read(t, filepath.Join(repo, ".git", "info", "exclude")))
	}
	if after := run(t, repo, "status", "--porcelain"); after != before {
		t.Fatalf("owner's status changed from %q to %q", before, after)
	}
	if got := run(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "master" {
		t.Fatalf("owner's checkout moved to %s", got)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatalf("ensuring an existing worktree: %v", err)
	}
	// A worktree that must already exist (a continuation) is not recreated.
	missing := w
	missing.Path, missing.Temp, missing.GitDir = filepath.Join(repo, ".evie", "worktrees", "gone"), filepath.Join(repo, ".evie", "tmp", "gone"), filepath.Join(w.CommonDir, "worktrees", "gone")
	if err := Ensure(ctx, missing, false); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing worktree: %v", err)
	}
	// A second plan with the same branch cannot be created.
	if err := Ensure(ctx, delegation.Worktree{Repo: w.Repo, CommonDir: w.CommonDir, GitDir: filepath.Join(w.CommonDir, "worktrees", "other"),
		Path: filepath.Join(repo, ".evie", "worktrees", "other"), Temp: filepath.Join(repo, ".evie", "tmp", "other"), Branch: w.Branch, Base: w.Base, BaseCommit: w.BaseCommit}, true); err == nil {
		t.Fatal("a second worktree reused an existing branch")
	}
}

// Stage 11: the harness commits the worker's changes on the worker's branch
// with the owner's identity and a plain message, running no hook or
// fsmonitor and ignoring a tampered .git file; it refuses to commit a
// worktree that contains a nested repository. The report lists the commits
// and files changed against the base.
func TestCommitAndReportUseTheHarnessGitDir(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w, err := Plan(ctx, repo, "", attemptID, "Add parser")
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	if sha, err := Commit(ctx, w, "Nothing yet"); err != nil || sha != "" {
		t.Fatalf("empty commit = %q, %v", sha, err)
	}
	marker := filepath.Join(filepath.Dir(repo), "hook-ran")
	for _, hook := range []string{"pre-commit", "commit-msg", "post-commit"} {
		write(t, filepath.Join(repo, ".git", "hooks", hook), "#!/bin/sh\ntouch "+marker+"\n")
		if err := os.Chmod(filepath.Join(repo, ".git", "hooks", hook), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run(t, repo, "config", "core.fsmonitor", filepath.Join(repo, ".git", "hooks", "pre-commit"))
	write(t, filepath.Join(w.Path, "parser.go"), "package main\n\nfunc parse() {}\n")
	write(t, filepath.Join(w.Path, "main.go"), "package main\n\nfunc main() { parse() }\n")
	// The worker cannot do this (its tools and sandbox refuse), but the
	// harness must not trust the .git file even so.
	decoy := filepath.Join(filepath.Dir(repo), "decoy")
	run(t, filepath.Dir(repo), "init", "-q", decoy)
	write(t, filepath.Join(w.Path, ".git"), "gitdir: "+filepath.Join(decoy, ".git")+"\n")
	sha, err := Commit(ctx, w, "Add parser")
	if err != nil || sha == "" {
		t.Fatalf("commit = %q, %v", sha, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a hook or fsmonitor ran during the harness commit")
	}
	if got := run(t, repo, "rev-parse", "refs/heads/"+w.Branch); got != sha {
		t.Fatalf("branch at %s, want %s", got, sha)
	}
	if got := run(t, repo, "rev-parse", "master"); got != w.BaseCommit {
		t.Fatal("master moved")
	}
	if got := run(t, repo, "log", "-1", "--format=%an <%ae>|%s", sha); got != "Owner <owner@example.com>|Add parser" {
		t.Fatalf("commit = %q", got)
	}
	if out, err := exec.Command("git", "-C", decoy, "rev-parse", "--verify", "--quiet", "HEAD").CombinedOutput(); err == nil {
		t.Fatalf("the decoy repository got a commit: %s", out)
	}
	write(t, filepath.Join(w.Path, ".git"), "gitdir: "+w.GitDir+"\n")

	report, err := Report(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	if report.Branch != w.Branch || report.Base != "master" || report.BaseCommit != w.BaseCommit || report.Head != sha || report.Uncommitted ||
		len(report.Commits) != 1 || report.Commits[0].SHA != sha || report.Commits[0].Subject != "Add parser" ||
		report.FilesChanged != 2 || report.Insertions != 4 || report.Deletions != 1 || len(report.Files) != 2 {
		t.Fatalf("report = %+v", report)
	}
	write(t, filepath.Join(w.Path, "dirty.txt"), "x")
	if report, err := Report(ctx, w); err != nil || !report.Uncommitted {
		t.Fatalf("dirty worktree report = %+v, %v", report, err)
	}

	// A nested repository is never committed.
	if err := os.MkdirAll(filepath.Join(w.Path, "vendor", "lib", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Path, "vendor", "lib", ".git", "config"), "[core]\n\tfsmonitor = "+filepath.Join(repo, ".git", "hooks", "pre-commit")+"\n")
	if sha, err := Commit(ctx, w, "Sneak"); err == nil || !strings.Contains(err.Error(), ".git") {
		t.Fatalf("nested repository committed: %q %v", sha, err)
	}
	if got := run(t, repo, "rev-parse", "refs/heads/"+w.Branch); got != report.Head {
		t.Fatal("the refused commit moved the branch")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a nested repository's fsmonitor ran")
	}
}

// Stage 11: cleanup removes the worktree and its temp folder, keeps the
// branch unless asked, and works when the worktree is already gone.
func TestRemoveCleansTheWorktreeAndOptionallyItsBranch(t *testing.T) {
	ctx := context.Background()
	repo := repository(t)
	w, err := Plan(ctx, repo, "", attemptID, "Add parser")
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(w.Path, "x.txt"), "uncommitted")
	if present, dirty, err := Status(ctx, w); err != nil || !present || !dirty {
		t.Fatalf("status = %v %v %v", present, dirty, err)
	}
	if err := Remove(ctx, w, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{w.Path, w.Temp, w.GitDir} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s survived cleanup", path)
		}
	}
	if got := run(t, repo, "branch", "--list", w.Branch); !strings.Contains(got, w.Branch) {
		t.Fatal("cleanup deleted the branch without being asked")
	}
	if present, _, err := Status(ctx, w); err != nil || present {
		t.Fatalf("removed worktree status = %v %v", present, err)
	}
	if err := Remove(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	if got := run(t, repo, "branch", "--list", w.Branch); got != "" {
		t.Fatalf("branch survived: %s", got)
	}
	if got := run(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "master" || read(t, filepath.Join(repo, "README.md")) != "# Repo\nowner's uncommitted edit\n" {
		t.Fatal("cleanup touched the owner's checkout")
	}
}
