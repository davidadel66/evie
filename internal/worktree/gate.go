package worktree

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
)

// The merge gate's git operations (Stage 12 of docs/memory-architecture.md,
// 2026-10-04). Like the rest of this package they run in Evie's process,
// never in a worker, with hooks, fsmonitor, pagers, signing and autostash
// off. A rebase happens only inside the worker's worktree. A merge only
// fast-forwards: in the checkout that has the target checked out, through
// git merge --ff-only, which refuses to overwrite the owner's uncommitted
// changes; otherwise by a compare-and-swap of the target's ref. Nothing here
// stashes, resets, forces or pushes.

// gateHardening turns off what a rebase or merge could otherwise pick up
// from the repository's configuration.
var gateHardening = []string{"-c", "rebase.autoStash=false", "-c", "merge.autoStash=false", "-c", "rebase.updateRefs=false",
	"-c", "rerere.enabled=false", "-c", "rebase.autoSquash=false", "-c", "pull.rebase=false", "-c", "merge.verifySignatures=false"}

// ErrTargetMoved is a target branch that no longer points where the gate
// verified it.
var ErrTargetMoved = errors.New("the target branch moved since it was verified")

// MergeRefusedError is a fast-forward git refused in the owner's checkout,
// in git's words (bounded): typically uncommitted changes the merge would
// overwrite. Nothing was changed.
type MergeRefusedError struct {
	Checkout string
	Message  string
}

func (e *MergeRefusedError) Error() string {
	return fmt.Sprintf("git refused to fast-forward the checkout at %s: %s", e.Checkout, e.Message)
}

// BranchHead is the head of w's branch.
func BranchHead(ctx context.Context, w delegation.Worktree) (string, error) {
	head, err := git(ctx, w.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+w.Branch)
	if err != nil || head == "" {
		return "", fmt.Errorf("branch %s is missing", w.Branch)
	}
	return head, nil
}

// TargetHead resolves a local branch name to its head. Only refs/heads is
// consulted: a merge target is always a local branch.
func TargetHead(ctx context.Context, repo, target string) (string, error) {
	if !validRef(target) || strings.ContainsAny(target, "~^") {
		return "", fmt.Errorf("target %q is not a valid branch name", target)
	}
	head, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+target)
	if err != nil || head == "" {
		return "", fmt.Errorf("target branch %s does not exist", target)
	}
	return head, nil
}

// IsAncestor reports whether ancestor is reachable from descendant.
func IsAncestor(ctx context.Context, repo, ancestor, descendant string) (bool, error) {
	_, err := git(ctx, repo, "merge-base", "--is-ancestor", ancestor, descendant)
	switch {
	case err == nil:
		return true, nil
	case exitCode(err) == 1:
		return false, nil
	}
	return false, err
}

// TrackedChanges lists the tracked files changed in w's worktree, plain
// paths only, and how many there are; untracked files do not count.
func TrackedChanges(ctx context.Context, w delegation.Worktree) ([]string, error) {
	out, err := inWorktree(ctx, w, "-c", "core.quotePath=true", "status", "--porcelain", "--untracked-files=no", "--ignore-submodules=all")
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 3 {
			changed = append(changed, line[3:])
		}
	}
	return changed, nil
}

// rebaseInProgress reports whether w's git directory holds a rebase.
func rebaseInProgress(w delegation.Worktree) bool {
	for _, marker := range []string{"rebase-merge", "rebase-apply"} {
		if _, err := os.Lstat(filepath.Join(w.GitDir, marker)); err == nil {
			return true
		}
	}
	return false
}

// AbortRebase aborts a rebase a crash left in w and reports whether there
// was one. The branch returns to where it was before that rebase.
func AbortRebase(ctx context.Context, w delegation.Worktree) (bool, error) {
	unlock := lockRepo(w.Repo)
	defer unlock()
	return abortRebase(ctx, w)
}

func abortRebase(ctx context.Context, w delegation.Worktree) (bool, error) {
	if err := checkPlanned(w); err != nil {
		return false, err
	}
	if !rebaseInProgress(w) {
		return false, nil
	}
	if _, err := inWorktree(ctx, w, append(append([]string(nil), gateHardening...), "rebase", "--abort")...); err != nil {
		return true, fmt.Errorf("abort the rebase: %w", err)
	}
	return true, nil
}

// RebaseResult is a rebase of a worker's branch.
type RebaseResult struct {
	// Rebased is false when the branch already contained the target head.
	Rebased bool
	Head    string
	// Conflicts lists the conflicting files (plain paths) of an aborted
	// rebase; the branch is then unchanged.
	Conflicts []string
	// Unlisted counts conflicting files whose names are not plain.
	Unlisted int
}

// Rebase replays w's branch onto onto inside its worktree, as the harness:
// the repository's identity (Evie's when it has none), hooks and autostash
// off. A conflict aborts the rebase and is reported in the result, so the
// branch and worktree stay as they were. A rebase a crash left behind is
// aborted first.
func Rebase(ctx context.Context, w delegation.Worktree, onto string) (RebaseResult, error) {
	unlock := lockRepo(w.Repo)
	defer unlock()
	if _, err := abortRebase(ctx, w); err != nil {
		return RebaseResult{}, err
	}
	if err := verify(ctx, w); err != nil {
		return RebaseResult{}, err
	}
	head, err := BranchHead(ctx, w)
	if err != nil {
		return RebaseResult{}, err
	}
	contained, err := IsAncestor(ctx, w.Repo, onto, head)
	if err != nil {
		return RebaseResult{}, err
	}
	if contained {
		return RebaseResult{Head: head}, nil
	}
	identity := []string{}
	if name, _ := inWorktree(ctx, w, "config", "--get", "user.name"); name == "" {
		identity = append(identity, "-c", "user.name=Evie")
	}
	if email, _ := inWorktree(ctx, w, "config", "--get", "user.email"); email == "" {
		identity = append(identity, "-c", "user.email=evie@localhost")
	}
	args := append(append(append([]string(nil), identity...), gateHardening...), "--git-dir="+w.GitDir, "--work-tree="+w.Path,
		"rebase", "--quiet", "--no-autostash", "--no-autosquash", "--no-gpg-sign", onto)
	_, rebaseErr := git(ctx, w.Path, args...)
	if rebaseErr == nil {
		rebased, err := BranchHead(ctx, w)
		return RebaseResult{Rebased: true, Head: rebased}, err
	}
	result := RebaseResult{Head: head}
	if rebaseInProgress(w) {
		if names, err := inWorktree(context.WithoutCancel(ctx), w, "-c", "core.quotePath=true", "diff", "--name-only", "--diff-filter=U"); err == nil {
			for _, name := range strings.Split(names, "\n") {
				switch {
				case name == "":
				case plainPath(name):
					result.Conflicts = append(result.Conflicts, name)
				default:
					result.Unlisted++
				}
			}
		}
		if _, err := abortRebase(context.WithoutCancel(ctx), w); err != nil {
			return result, err
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if len(result.Conflicts) > 0 || result.Unlisted > 0 {
			return result, nil
		}
	}
	return result, fmt.Errorf("rebase: %w", rebaseErr)
}

// Compare summarizes the commits from..to and the files they change, as
// Report does for a branch against its base.
func Compare(ctx context.Context, repo, from, to string) (delegation.BuildReport, error) {
	report := delegation.BuildReport{Head: to}
	log, err := git(ctx, repo, "log", "--no-color", "--format=%H%x1f%s", "-n", strconv.Itoa(maxReportCommits+1), from+".."+to, "--")
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
	numstat, err := git(ctx, repo, "diff", "--numstat", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", from, to, "--")
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
	return report, nil
}

// PlainPath reports whether a path a worker may have named can be shown
// outside an untrusted-data frame: short, with no spaces, controls or
// unusual characters.
func PlainPath(path string) bool { return plainPath(path) }

// ChangedPaths lists every path from..to changes, plain or not.
func ChangedPaths(ctx context.Context, repo, from, to string) ([]string, error) {
	out, err := git(ctx, repo, "diff", "--name-only", "-z", "--no-renames", from, to, "--")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// checkout is one entry of git worktree list.
type checkout struct {
	path, branch string
	bare         bool
}

func checkouts(ctx context.Context, repo string) ([]checkout, error) {
	out, err := git(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var list []checkout
	var current *checkout
	for _, field := range strings.Split(out, "\x00") {
		switch {
		case strings.HasPrefix(field, "worktree "):
			list = append(list, checkout{path: strings.TrimPrefix(field, "worktree ")})
			current = &list[len(list)-1]
		case current == nil:
		case strings.HasPrefix(field, "branch "):
			current.branch = strings.TrimPrefix(field, "branch ")
		case field == "bare":
			current.bare = true
		}
	}
	return list, nil
}

// CheckedOutAt is the checkout that has branch checked out, if any.
func CheckedOutAt(ctx context.Context, repo, branch string) (string, bool, error) {
	list, err := checkouts(ctx, repo)
	if err != nil {
		return "", false, err
	}
	for _, c := range list {
		if c.branch == "refs/heads/"+branch {
			return c.path, true, nil
		}
	}
	return "", false, nil
}

// FastForward moves target from expected to next, which must descend from
// it, and says how. When a checkout has target checked out (the owner's,
// typically), it runs git merge --ff-only there, so that checkout's files
// follow; git refuses when the owner's uncommitted changes would be
// overwritten (MergeRefusedError), and nothing is stashed, reset or
// overwritten. Otherwise the ref moves by a compare-and-swap that fails
// with ErrTargetMoved if it no longer points at expected. Build worktrees
// are never merged into. Nothing is pushed.
func FastForward(ctx context.Context, w delegation.Worktree, target, expected, next string) (string, error) {
	unlock := lockRepo(w.Repo)
	defer unlock()
	current, err := TargetHead(ctx, w.Repo, target)
	if err != nil {
		return "", err
	}
	if current != expected {
		return "", fmt.Errorf("%w: %s is at %s, verified at %s", ErrTargetMoved, target, short(current), short(expected))
	}
	if ok, err := IsAncestor(ctx, w.Repo, expected, next); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("%s does not descend from %s's head %s, so it cannot fast-forward", short(next), target, short(expected))
	}
	at, found, err := CheckedOutAt(ctx, w.Repo, target)
	if err != nil {
		return "", err
	}
	if !found {
		ref := "refs/heads/" + target
		message := fmt.Sprintf("evie: fast-forward %s to %s (%s)", target, short(next), w.Branch)
		if _, err := git(ctx, w.Repo, "update-ref", "-m", message, ref, next, expected); err != nil {
			if now, readErr := TargetHead(ctx, w.Repo, target); readErr == nil && now != expected {
				return "", fmt.Errorf("%w: %s is at %s", ErrTargetMoved, target, short(now))
			}
			return "", fmt.Errorf("update %s: %w", ref, err)
		}
		return fmt.Sprintf("compare-and-swap of %s from %s to %s (not checked out anywhere)", ref, short(expected), short(next)), nil
	}
	canonical := resolved(at)
	if within(filepath.Join(w.Repo, Folder), canonical) {
		return "", fmt.Errorf("%s is checked out in the build worktree %s; Evie does not merge into a build worktree", target, canonical)
	}
	gitDir, err := git(ctx, canonical, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return "", err
	}
	for _, op := range inProgress {
		if _, err := os.Lstat(filepath.Join(gitDir, op.marker)); err == nil {
			return "", &MergeRefusedError{Checkout: canonical, Message: op.operation + " is in progress there; finish or abort it first"}
		}
	}
	if head, err := git(ctx, canonical, "rev-parse", "--verify", "HEAD"); err != nil || head != expected {
		return "", fmt.Errorf("%w: the checkout at %s is at %s", ErrTargetMoved, canonical, short(head))
	}
	args := append(append([]string(nil), gateHardening...), "merge", "--ff-only", "--no-autostash", "--quiet", "--no-edit", "--no-stat", next)
	if _, err := git(ctx, canonical, args...); err != nil {
		var gerr *gitError
		message := err.Error()
		if errors.As(err, &gerr) && gerr.message != "" {
			message = gerr.message
		}
		return "", &MergeRefusedError{Checkout: canonical, Message: message}
	}
	if head, err := git(ctx, canonical, "rev-parse", "--verify", "HEAD"); err != nil || head != next {
		return "", fmt.Errorf("the checkout at %s did not reach %s after the merge", canonical, short(next))
	}
	return fmt.Sprintf("git merge --ff-only in the checkout at %s, where %s is checked out", canonical, target), nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// OwnerHistoryContains reports whether commit is reachable from a local
// branch outside evie/, a tag or a remote-tracking branch: history the owner
// wrote, fetched or approved, as opposed to a commit that exists only on a
// build worker's branch. Only such a commit may run the setup command,
// which has network.
func OwnerHistoryContains(ctx context.Context, repo, commit string) (bool, error) {
	out, err := git(ctx, repo, "for-each-ref", "--contains", commit, "--format=%(refname)", "refs/heads/", "refs/tags/", "refs/remotes/")
	if err != nil {
		return false, err
	}
	for _, ref := range strings.Split(out, "\n") {
		if ref != "" && !strings.HasPrefix(ref, "refs/heads/evie/") {
			return true, nil
		}
	}
	return false, nil
}

// Proposal is the setup and verify commands Evie found in a repository's
// instructions. It is only a suggestion: the owner sets the commands.
type Proposal struct {
	Setup        string `json:"setup,omitempty"`
	SetupSource  string `json:"setupSource,omitempty"`
	Verify       string `json:"verify,omitempty"`
	VerifySource string `json:"verifySource,omitempty"`
}

// instructionFiles are read in this order.
var instructionFiles = []string{"AGENTS.md", "CLAUDE.md"}

const maxInstructionBytes = 256 << 10

var (
	installCommand = regexp.MustCompile(`^(npm|pnpm|yarn|bun)( --prefix \S+)? (ci|install)\b|^go mod download\b|^(pip|pip3) install\b|^uv sync\b|^poetry install\b|^bundle install\b|^cargo fetch\b`)
	verifyCommand  = regexp.MustCompile(`^(\./\S+|bash \S+|sh \S+|make\b|just\b|go (test|vet)\b|(npm|pnpm|yarn|bun) (run |test\b)|cargo test\b|pytest\b|tox\b|uv run\b|bundle exec\b|\./gradlew\b|mvn\b)`)
	verifyHeading  = regexp.MustCompile(`(?i)verif|check|test|\bci\b`)
	inlineCode     = regexp.MustCompile("`([^`\n]+)`")
)

// ProposeCommands reads AGENTS.md and CLAUDE.md at folder's top (regular
// files inside it, bounded) and proposes the first install command as setup
// and the first check command in a verification, check or test section (or
// a command naming verify or check anywhere) as verify.
func ProposeCommands(folder string) Proposal {
	var p Proposal
	root, err := os.OpenRoot(folder)
	if err != nil {
		return p
	}
	defer root.Close()
	for _, name := range instructionFiles {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		file, err := root.Open(name)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(file, maxInstructionBytes))
		file.Close()
		setup, verify := proposeFrom(string(data))
		if p.Setup == "" && setup != "" {
			p.Setup, p.SetupSource = setup, name
		}
		if p.Verify == "" && verify != "" {
			p.Verify, p.VerifySource = verify, name
		}
	}
	return p
}

// proposeFrom scans one instructions file.
func proposeFrom(text string) (setup, verify string) {
	var heading string
	var fenced bool
	var fallback string
	consider := func(command string, block bool) {
		command = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(command), "$ "))
		if command == "" || len(command) > 300 || strings.ContainsAny(command, "\x00\r") {
			return
		}
		if setup == "" && installCommand.MatchString(command) {
			setup = command
			return
		}
		if verify != "" || !verifyCommand.MatchString(command) {
			return
		}
		if block && verifyHeading.MatchString(heading) {
			verify = command
		} else if fallback == "" && (strings.Contains(command, "verify") || strings.Contains(command, "check")) {
			fallback = command
		}
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64<<10), maxInstructionBytes)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			consider(trimmed, true)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			heading = strings.TrimLeft(trimmed, "# ")
			continue
		}
		for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
			consider(m[1], false)
		}
	}
	if verify == "" {
		verify = fallback
	}
	return setup, verify
}

// Exists reports whether w's worktree folder is present.
func Exists(w delegation.Worktree) bool {
	info, err := os.Lstat(w.Path)
	return err == nil && info.IsDir() && info.Mode()&fs.ModeSymlink == 0
}
