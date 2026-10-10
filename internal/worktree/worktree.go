// Package worktree is the harness-owned lifecycle of a build worker's git
// worktree (Stage 11 of docs/memory-architecture.md, 2026-10-03): planning
// it before admission, creating it on a fresh branch, committing the
// worker's changes as the harness, reporting the branch against its base,
// and cleaning it up. The worker itself never runs a git command that
// writes; every write here runs in Evie's process with hooks, fsmonitor,
// pagers and signing off, and with the worktree's git directory named
// explicitly, so nothing the worker wrote can steer it.
//
// Worktrees live at <repo>/.evie/worktrees/<attempt>, with the attempt's
// private temp folder at <repo>/.evie/tmp/<attempt>. That is inside the
// Workspace folder (the attempt's authority ceiling) and never inside
// ~/.evie, and the harness adds /.evie/ to the repository's local
// info/exclude rather than to any tracked file, so the owner's status stays
// clean and nothing there is ever committed. The layout mirrors Claude
// Code's <repo>/.claude/worktrees.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/tools"
)

const (
	// Folder is the repository-relative folder holding build worktrees and
	// their temp folders.
	Folder = ".evie"
	// excludeLine keeps Folder out of the repository's status.
	excludeLine = "/.evie/"
	// maxWalkEntries bounds the nested-repository scan before a commit.
	maxWalkEntries = 500_000
	// maxReportCommits and maxReportFiles bound a report.
	maxReportCommits = 50
	maxReportFiles   = 200
	maxPathBytes     = 256
)

// hardening is passed to every harness git command: no hooks, fsmonitor,
// pager, signing, external diff or submodule recursion, and no transport.
var hardening = []string{
	"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.pager=cat",
	"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "log.showSignature=false",
	"-c", "diff.ignoreSubmodules=all", "-c", "status.submoduleSummary=false", "-c", "submodule.recurse=false",
	"-c", "protocol.allow=never", "-c", "core.untrackedCache=false",
}

// gitEnv is built, not inherited, so no GIT_DIR or similar in Evie's own
// environment can redirect a harness command.
func gitEnv() []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	home, _ := os.UserHomeDir()
	env := []string{"PATH=" + path, "HOME=" + home, "LANG=C", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_EDITOR=true"}
	for _, name := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "XDG_CONFIG_HOME"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}

// Git runs one hardened git command in dir (no hooks, fsmonitor, pager,
// signing or transport) and returns its trimmed stdout. The changes
// package inspects Workspace repositories through it.
func Git(ctx context.Context, dir string, args ...string) (string, error) {
	return git(ctx, dir, args...)
}

// git runs one hardened git command in dir and returns its trimmed stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append(append([]string(nil), hardening...), args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 400 {
			message = message[:400]
		}
		return strings.TrimSpace(stdout.String()), &gitError{args: args, err: err, message: message}
	}
	return strings.TrimSpace(stdout.String()), nil
}

type gitError struct {
	args    []string
	err     error
	message string
}

func (e *gitError) Error() string {
	if e.message != "" {
		return fmt.Sprintf("git %s: %s", e.args[0], e.message)
	}
	return fmt.Sprintf("git %s: %v", e.args[0], e.err)
}

func (e *gitError) Unwrap() error { return e.err }

// exitCode is a failed git command's exit status, or -1.
func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

// inWorktree runs git against w with its git directory and work tree named
// explicitly: the worktree's .git file is never consulted.
func inWorktree(ctx context.Context, w delegation.Worktree, args ...string) (string, error) {
	return git(ctx, w.Path, append([]string{"--git-dir=" + w.GitDir, "--work-tree=" + w.Path}, args...)...)
}

// refPattern is a conservative base or branch name: no option, range,
// reflog or revision syntax beyond ~N and ^N suffixes.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/+-]*([~^][0-9]*)*$`)

func validRef(name string) bool {
	return refPattern.MatchString(name) && !strings.Contains(name, "..") && !strings.Contains(name, "//") &&
		!strings.HasSuffix(name, "/") && !strings.HasSuffix(name, ".lock") && !strings.HasSuffix(name, ".") && len(name) <= delegation.MaxBaseBytes
}

// slug turns an objective into a short branch-safe phrase.
func slug(objective string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(objective) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// inProgress names unfinished repository operations by their marker in a
// git directory.
var inProgress = []struct{ marker, operation string }{
	{"MERGE_HEAD", "a merge"}, {"rebase-merge", "a rebase"}, {"rebase-apply", "a rebase"},
	{"CHERRY_PICK_HEAD", "a cherry-pick"}, {"REVERT_HEAD", "a revert"}, {"BISECT_LOG", "a bisect"},
}

// Plan resolves, without changing anything, where attempt id's worktree
// goes for the repository at folder: the folder must be a non-bare
// repository's top level with no merge, rebase or similar in progress, and
// base (default: the branch checked out there) must resolve to a commit,
// which is pinned. The branch is evie/<id prefix>-<slug of objective>.
func Plan(ctx context.Context, folder, base, id, objective string) (delegation.Worktree, error) {
	var w delegation.Worktree
	if folder == "" {
		return w, errors.New("build work needs a folder: this conversation has no Workspace folder or project root")
	}
	canonical, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return w, fmt.Errorf("folder %q is unavailable: %w", folder, err)
	}
	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		return w, fmt.Errorf("%q is not a directory", folder)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`).MatchString(id) {
		return w, fmt.Errorf("invalid attempt identity %q", id)
	}
	out, err := git(ctx, canonical, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir", "--is-bare-repository")
	lines := strings.Split(out, "\n")
	if err != nil || len(lines) != 4 {
		return w, fmt.Errorf("%s is not a git repository, so build work has no repository to branch from", canonical)
	}
	if lines[3] == "true" {
		return w, fmt.Errorf("%s is a bare repository; build work needs a checkout", canonical)
	}
	top, err := filepath.EvalSymlinks(lines[0])
	if err != nil || top != canonical {
		return w, fmt.Errorf("%s is inside the repository %s but is not its top level; attach the repository's top-level folder for build work", canonical, lines[0])
	}
	gitDir, err := filepath.EvalSymlinks(lines[1])
	if err != nil {
		return w, fmt.Errorf("the repository's git directory is unavailable: %w", err)
	}
	commonDir, err := filepath.EvalSymlinks(lines[2])
	if err != nil {
		return w, fmt.Errorf("the repository's common git directory is unavailable: %w", err)
	}
	for _, op := range inProgress {
		if _, err := os.Lstat(filepath.Join(gitDir, op.marker)); err == nil {
			return w, fmt.Errorf("the repository at %s has %s in progress; finish or abort it before delegating build work", canonical, op.operation)
		}
	}
	if base == "" {
		branch, err := git(ctx, canonical, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil || branch == "" {
			return w, fmt.Errorf("the repository at %s has a detached HEAD; name a base branch for the build", canonical)
		}
		base = branch
	}
	if !validRef(base) {
		return w, fmt.Errorf("base %q is not a valid branch or commit name", base)
	}
	commit, err := git(ctx, canonical, "rev-parse", "--verify", "--quiet", "--end-of-options", base+"^{commit}")
	if err != nil || commit == "" {
		return w, fmt.Errorf("base %q cannot be resolved to a commit in %s", base, canonical)
	}
	if err := checkFolder(ctx, canonical); err != nil {
		return w, err
	}
	branch := "evie/" + strings.ToLower(id[:8])
	if s := slug(objective); s != "" {
		branch += "-" + s
	}
	if !validRef(branch) {
		return w, fmt.Errorf("branch %q is not a valid name", branch)
	}
	if _, err := git(ctx, canonical, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return w, fmt.Errorf("branch %s already exists", branch)
	}
	w = delegation.Worktree{Repo: canonical, CommonDir: commonDir, GitDir: filepath.Join(commonDir, "worktrees", id),
		Path: filepath.Join(canonical, Folder, "worktrees", id), Temp: filepath.Join(canonical, Folder, "tmp", id),
		Branch: branch, Base: base, BaseCommit: commit}
	for _, path := range []string{w.Path, w.Temp, w.GitDir} {
		if _, err := os.Lstat(path); err == nil {
			return delegation.Worktree{}, fmt.Errorf("%s already exists", path)
		}
	}
	return w, nil
}

// checkFolder requires repo's .evie folder, and its worktrees and tmp
// folders, to be untracked real directories (or absent), and the folder not
// to be under a location that may hold secrets.
func checkFolder(ctx context.Context, repo string) error {
	evie := filepath.Join(repo, Folder)
	if err := tools.ValidateFileAccess(filepath.Join(evie, "worktrees", "x")); err != nil {
		return fmt.Errorf("build worktrees cannot be placed at %s: %w", evie, err)
	}
	// Never under Evie's own data folder, which explore treats as secret.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, data := range []string{filepath.Join(home, Folder), resolved(filepath.Join(home, Folder))} {
			if within(data, evie) {
				return fmt.Errorf("build worktrees cannot be placed under %s, Evie's own data folder; attach a repository outside it", data)
			}
		}
	}
	for _, dir := range []string{evie, filepath.Join(evie, "worktrees"), filepath.Join(evie, "tmp")} {
		info, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s is not a plain folder (it is a link or a file), so build worktrees cannot be placed under it", dir)
		}
	}
	tracked, err := git(ctx, repo, "ls-files", "--", Folder)
	if err != nil {
		return err
	}
	if tracked != "" {
		return fmt.Errorf("the repository tracks .evie, where build worktrees go; build work cannot use it")
	}
	return nil
}

// resolved is p with symlinks resolved, or p when it does not exist.
func resolved(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// within reports whether p is root or below it.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

var repoLocks sync.Map

// lockRepo serializes this process's worktree changes per repository.
func lockRepo(repo string) func() {
	value, _ := repoLocks.LoadOrStore(repo, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Ensure creates w on its branch from its pinned base commit, or verifies
// the worktree that is already there: a plain folder whose .git names the
// pinned git directory and whose HEAD is the branch. A continuation passes
// create false: its worktree must still exist.
func Ensure(ctx context.Context, w delegation.Worktree, create bool) error {
	unlock := lockRepo(w.Repo)
	defer unlock()
	if err := checkPlanned(w); err != nil {
		return err
	}
	if _, err := os.Lstat(w.Path); err == nil {
		if err := verify(ctx, w); err != nil {
			return err
		}
		return ensureTemp(w)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if !create {
		return fmt.Errorf("worktree %s is missing; it was removed, so this build cannot continue there", w.Path)
	}
	if err := checkFolder(ctx, w.Repo); err != nil {
		return err
	}
	if err := ensureExcluded(w); err != nil {
		return err
	}
	if err := mkdirPlain(filepath.Join(w.Repo, Folder, "worktrees")); err != nil {
		return err
	}
	if _, err := git(ctx, w.Repo, "worktree", "add", "-q", "-b", w.Branch, w.Path, w.BaseCommit); err != nil {
		return fmt.Errorf("create the worktree: %w", err)
	}
	if err := verify(ctx, w); err != nil {
		_, _ = git(context.WithoutCancel(ctx), w.Repo, "worktree", "remove", "--force", w.Path)
		return err
	}
	return ensureTemp(w)
}

// checkPlanned requires w's harness paths to be where Plan puts them.
func checkPlanned(w delegation.Worktree) error {
	id := filepath.Base(w.Path)
	if w.Repo == "" || w.Path != filepath.Join(w.Repo, Folder, "worktrees", id) || w.Temp != filepath.Join(w.Repo, Folder, "tmp", id) ||
		w.GitDir != filepath.Join(w.CommonDir, "worktrees", id) || !validRef(w.Branch) || w.BaseCommit == "" {
		return fmt.Errorf("worktree record for %s is not a harness plan", w.Path)
	}
	return nil
}

// verify proves an existing worktree is w: a real folder whose .git points
// at the pinned git directory, which has the branch checked out.
func verify(ctx context.Context, w delegation.Worktree) error {
	info, err := os.Lstat(w.Path)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("worktree %s is not a plain folder", w.Path)
	}
	if canonical, err := filepath.EvalSymlinks(w.Path); err != nil || canonical != w.Path {
		return fmt.Errorf("worktree %s has moved", w.Path)
	}
	pointer, err := os.ReadFile(filepath.Join(w.Path, ".git"))
	if err != nil || strings.TrimSpace(string(pointer)) != "gitdir: "+w.GitDir {
		return fmt.Errorf("worktree %s is not linked to its pinned git directory %s", w.Path, w.GitDir)
	}
	head, err := inWorktree(ctx, w, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || head != "refs/heads/"+w.Branch {
		return fmt.Errorf("worktree %s is not on its branch %s", w.Path, w.Branch)
	}
	return nil
}

// mkdirPlain creates dir and its parents, refusing any that is a link.
func mkdirPlain(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if canonical, err := filepath.EvalSymlinks(dir); err != nil || canonical != dir {
		return fmt.Errorf("%s is reached through a link", dir)
	}
	return nil
}

// ensureTemp creates the attempt's private temp folder.
func ensureTemp(w delegation.Worktree) error {
	if err := mkdirPlain(filepath.Dir(w.Temp)); err != nil {
		return err
	}
	if err := os.Mkdir(w.Temp, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := os.Lstat(w.Temp)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("temp folder %s is not a plain folder", w.Temp)
	}
	return os.Chmod(w.Temp, 0o700)
}

// ensureExcluded adds /.evie/ to the repository's local info/exclude once.
// It never edits a tracked .gitignore.
func ensureExcluded(w delegation.Worktree) error {
	info := filepath.Join(w.CommonDir, "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		return err
	}
	path := filepath.Join(info, "exclude")
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(current), "\n") {
		if strings.TrimSpace(line) == excludeLine {
			return nil
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	prefix := ""
	if len(current) > 0 && !bytes.HasSuffix(current, []byte("\n")) {
		prefix = "\n"
	}
	_, err = fmt.Fprintf(file, "%s# Evie build worktrees (added by Evie)\n%s\n", prefix, excludeLine)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// ErrNestedRepository refuses to commit a worktree that holds another
// repository's .git: committing it would make git consult that
// repository's own configuration.
var ErrNestedRepository = errors.New("the worktree contains a nested .git")

// nestedRepository finds a .git below the worktree other than its own.
func nestedRepository(w delegation.Worktree) (string, error) {
	entries := 0
	var found string
	err := filepath.WalkDir(w.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries++
		if entries > maxWalkEntries {
			return fmt.Errorf("the worktree has more than %d entries; the harness does not commit it", maxWalkEntries)
		}
		if path == w.Path || path == filepath.Join(w.Path, ".git") {
			return nil
		}
		if tools.GitComponent(entry.Name()) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// Commit stages every change in the worktree and commits it on the
// worker's branch as the harness: with the repository's configured identity
// (or Evie's when it has none), the given plain message, and no hook. It
// returns the new commit, or "" when there was nothing to commit.
func Commit(ctx context.Context, w delegation.Worktree, message string) (string, error) {
	unlock := lockRepo(w.Repo)
	defer unlock()
	if err := checkPlanned(w); err != nil {
		return "", err
	}
	if info, err := os.Lstat(w.Path); err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("worktree %s is missing", w.Path)
	}
	if nested, err := nestedRepository(w); err != nil {
		return "", err
	} else if nested != "" {
		return "", fmt.Errorf("%w at %s; nothing was committed (remove it, or commit by hand after reviewing it)", ErrNestedRepository, nested)
	}
	if head, err := inWorktree(ctx, w, "symbolic-ref", "--quiet", "HEAD"); err != nil || head != "refs/heads/"+w.Branch {
		return "", fmt.Errorf("worktree %s is not on its branch %s", w.Path, w.Branch)
	}
	if _, err := inWorktree(ctx, w, "add", "-A", "--", "."); err != nil {
		return "", err
	}
	if _, err := inWorktree(ctx, w, "diff", "--cached", "--quiet", "--no-ext-diff"); err == nil {
		return "", nil
	} else if exitCode(err) != 1 {
		return "", err
	}
	identity := []string{}
	if name, _ := inWorktree(ctx, w, "config", "--get", "user.name"); name == "" {
		identity = append(identity, "-c", "user.name=Evie")
	}
	if email, _ := inWorktree(ctx, w, "config", "--get", "user.email"); email == "" {
		identity = append(identity, "-c", "user.email=evie@localhost")
	}
	args := append(identity, "--git-dir="+w.GitDir, "--work-tree="+w.Path, "commit", "-q", "--no-verify", "--no-edit", "-m", message)
	if _, err := git(ctx, w.Path, args...); err != nil {
		return "", err
	}
	return inWorktree(ctx, w, "rev-parse", "--verify", "HEAD")
}

// plainPath reports whether a changed path can be listed outside the
// child-output frame: short, with no spaces, controls or unusual characters.
var plainPathPattern = regexp.MustCompile(`^[A-Za-z0-9._/@+-]+$`)

func plainPath(path string) bool {
	return len(path) <= maxPathBytes && plainPathPattern.MatchString(path)
}

// Report summarizes the branch against its pinned base: its head, the
// commits after the base, the files changed with line counts, and whether
// the worktree has changes the harness has not committed.
func Report(ctx context.Context, w delegation.Worktree) (delegation.BuildReport, error) {
	report := delegation.BuildReport{Branch: w.Branch, Base: w.Base, BaseCommit: w.BaseCommit, Worktree: w.Path}
	head, err := git(ctx, w.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+w.Branch)
	if err != nil {
		return report, fmt.Errorf("branch %s is missing", w.Branch)
	}
	report.Head = head
	log, err := git(ctx, w.Repo, "log", "--no-color", "--format=%H%x1f%s", "-n", strconv.Itoa(maxReportCommits+1), w.BaseCommit+".."+head, "--")
	if err != nil {
		return report, err
	}
	for _, line := range strings.Split(log, "\n") {
		sha, subject, ok := strings.Cut(line, "\x1f")
		if !ok || len(report.Commits) == maxReportCommits {
			continue
		}
		report.Commits = append(report.Commits, delegation.BuildCommit{SHA: sha, Subject: subject})
	}
	numstat, err := git(ctx, w.Repo, "diff", "--numstat", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", w.BaseCommit, head, "--")
	if err != nil {
		return report, err
	}
	for _, record := range strings.Split(numstat, "\x00") {
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		report.FilesChanged++
		file := delegation.BuildFile{Path: fields[2]}
		if fields[0] == "-" {
			file.Binary = true
		} else {
			file.Added, _ = strconv.Atoi(fields[0])
			file.Deleted, _ = strconv.Atoi(fields[1])
		}
		report.Insertions += file.Added
		report.Deletions += file.Deleted
		if plainPath(file.Path) && len(report.Files) < maxReportFiles {
			report.Files = append(report.Files, file)
		}
	}
	if _, dirty, err := Status(ctx, w); err == nil && dirty {
		report.Uncommitted = true
	}
	return report, nil
}

// Status reports whether w's worktree exists and has uncommitted changes.
func Status(ctx context.Context, w delegation.Worktree) (present, dirty bool, err error) {
	info, err := os.Lstat(w.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return false, false, fmt.Errorf("worktree %s is not a plain folder", w.Path)
	}
	out, err := inWorktree(ctx, w, "-c", "core.quotePath=true", "status", "--porcelain", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		return true, false, err
	}
	return true, out != "", nil
}

// Remove deletes w's worktree and temp folder, and its branch when
// deleteBranch is set. A worktree that is already gone is pruned.
func Remove(ctx context.Context, w delegation.Worktree, deleteBranch bool) error {
	unlock := lockRepo(w.Repo)
	defer unlock()
	if err := checkPlanned(w); err != nil {
		return err
	}
	if _, err := os.Lstat(w.Path); err == nil {
		if _, err := git(ctx, w.Repo, "worktree", "remove", "--force", "--force", w.Path); err != nil {
			return fmt.Errorf("remove the worktree: %w", err)
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if _, err := git(ctx, w.Repo, "worktree", "prune"); err != nil {
			return err
		}
	} else {
		return err
	}
	if err := os.RemoveAll(w.Temp); err != nil {
		return err
	}
	if deleteBranch {
		if _, err := git(ctx, w.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+w.Branch); err == nil {
			if _, err := git(ctx, w.Repo, "branch", "-D", "--end-of-options", w.Branch); err != nil {
				return fmt.Errorf("delete branch %s: %w", w.Branch, err)
			}
		}
	}
	return nil
}
