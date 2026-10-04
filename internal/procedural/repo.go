// Package procedural is Evie's procedural Git repository: the reviewed,
// versioned home of procedural assets such as Workflow Definitions (Stage 14
// sub-stage 2 of docs/memory-architecture.md; ADR 0007, 0012). Git answers
// "what was proposed and approved?"; SQLite owns approval records and runs.
//
// The repository is a private bare repository at ~/.evie/procedural, inside
// the location every file tool, explore root and sandboxed shell already
// refuses, so no worker can read or write it. Evie writes it only with Git
// plumbing (hash-object, mktree, commit-tree and a compare-and-swap
// update-ref of master): there is no work tree to leave dirty, a crash
// before the ref moves leaves only unreferenced objects, and concurrent
// writers retry on top of each other. Every command runs with a built
// environment (no inherited GIT_DIR, index, global or system configuration),
// no hooks and Evie's own identity. Readers name an exact commit, so later
// commits never change what an earlier commit reads.
package procedural

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	branch = "refs/heads/master"
	// MaxFileBytes bounds one file read from or written to the repository.
	MaxFileBytes = 1 << 20
	// maxPatchBytes bounds a diff.
	maxPatchBytes = 512 << 10
	maxPathBytes  = 1024
	maxCASRetries = 32
)

// hardening is passed to every command: no hooks, fsmonitor, pager,
// signing, external diff, automatic maintenance or transport.
var hardening = []string{
	"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.pager=cat",
	"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false", "-c", "log.showSignature=false",
	"-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "protocol.allow=never",
	"-c", "core.quotePath=false",
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// DefaultRoot is ~/.evie/procedural.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("procedural repository: no home directory")
	}
	return filepath.Join(home, ".evie", "procedural"), nil
}

// Repo is one procedural repository. New touches nothing; the repository is
// created on first use.
type Repo struct {
	root string
	mu   sync.Mutex
	init bool
}

// New returns the repository at root, an absolute path.
func New(root string) *Repo { return &Repo{root: root} }

// Root is the repository's directory.
func (r *Repo) Root() string { return r.root }

// env is built, not inherited: only PATH and HOME come from the process.
func env(extra ...string) []string {
	p := os.Getenv("PATH")
	if p == "" {
		p = "/usr/local/bin:/usr/bin:/bin"
	}
	home, _ := os.UserHomeDir()
	out := []string{
		"PATH=" + p, "HOME=" + home, "LANG=C", "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat", "GIT_EDITOR=true", "GIT_OPTIONAL_LOCKS=0",
	}
	return append(out, extra...)
}

type gitError struct {
	args    []string
	message string
	err     error
}

func (e *gitError) Error() string {
	if e.message != "" {
		return fmt.Sprintf("procedural repository: git %s: %s", e.args[0], e.message)
	}
	return fmt.Sprintf("procedural repository: git %s: %v", e.args[0], e.err)
}

func (e *gitError) Unwrap() error { return e.err }

// git runs one hardened command against the repository with stdin.
func (r *Repo) git(ctx context.Context, stdin []byte, extraEnv []string, args ...string) ([]byte, error) {
	full := append(append([]string{}, hardening...), "--git-dir="+r.root)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.root
	cmd.Env = env(extraEnv...)
	cmd.WaitDelay = 2 * time.Second
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 400 {
			message = message[:400]
		}
		return stdout.Bytes(), &gitError{args: args, message: message, err: err}
	}
	return stdout.Bytes(), nil
}

func (r *Repo) gitText(ctx context.Context, args ...string) (string, error) {
	out, err := r.git(ctx, nil, nil, args...)
	return strings.TrimSpace(string(out)), err
}

// ensure creates the repository on first use, or checks that root is a
// private bare repository and not a link.
func (r *Repo) ensure(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.init {
		return nil
	}
	if !filepath.IsAbs(r.root) || filepath.Clean(r.root) != r.root {
		return fmt.Errorf("procedural repository: %q must be a clean absolute path", r.root)
	}
	info, err := os.Lstat(r.root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(r.root), 0o700); err != nil {
			return fmt.Errorf("procedural repository: %w", err)
		}
		if err := os.Mkdir(r.root, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("procedural repository: %w", err)
		}
		if info, err = os.Lstat(r.root); err != nil {
			return fmt.Errorf("procedural repository: %w", err)
		}
	case err != nil:
		return fmt.Errorf("procedural repository: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("procedural repository: %s must be a directory, not a link or file", r.root)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(r.root, 0o700); err != nil {
			return fmt.Errorf("procedural repository: restrict %s: %w", r.root, err)
		}
	}
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return fmt.Errorf("procedural repository: %w", err)
	}
	if len(entries) == 0 {
		cmd := exec.CommandContext(ctx, "git", append(append([]string{}, hardening...), "init", "--bare", "--quiet", "--initial-branch=master", r.root)...)
		cmd.Dir = r.root
		cmd.Env = env()
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("procedural repository: git init: %v %s", err, strings.TrimSpace(string(out)))
		}
	}
	bare, err := r.gitText(ctx, "rev-parse", "--is-bare-repository")
	if err != nil || bare != "true" {
		return fmt.Errorf("procedural repository: %s is not Evie's bare procedural repository", r.root)
	}
	if gitDir, err := r.gitText(ctx, "rev-parse", "--absolute-git-dir"); err != nil || !sameDir(gitDir, r.root) {
		return fmt.Errorf("procedural repository: %s is not Evie's bare procedural repository", r.root)
	}
	r.init = true
	return nil
}

func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// cleanPath checks a slash-separated path inside the repository's tree:
// relative, clean, no empty, "." or ".." component, no .git component, no
// backslash and no control character.
func cleanPath(p string) error {
	if p == "" || len(p) > maxPathBytes {
		return fmt.Errorf("procedural repository: path %q must be 1 to %d bytes", p, maxPathBytes)
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || path.Clean(p) != p {
		return fmt.Errorf("procedural repository: path %q must be clean and relative", p)
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f {
			return fmt.Errorf("procedural repository: path %q holds a control character", p)
		}
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return fmt.Errorf("procedural repository: path %q has a reserved component", p)
		}
	}
	return nil
}

func checkCommit(commit string) error {
	if !commitPattern.MatchString(commit) || strings.Trim(commit, "0") == "" {
		return fmt.Errorf("procedural repository: %q is not a full commit hash", commit)
	}
	return nil
}

// Head is master's commit, or "" when nothing has been committed.
func (r *Repo) Head(ctx context.Context) (string, error) {
	if err := r.ensure(ctx); err != nil {
		return "", err
	}
	return r.head(ctx)
}

func (r *Repo) head(ctx context.Context) (string, error) {
	out, err := r.gitText(ctx, "rev-parse", "--verify", "--quiet", branch+"^{commit}")
	var exit *exec.ExitError
	if err != nil && errors.As(err, &exit) && exit.ExitCode() == 1 && out == "" {
		return "", nil
	}
	return out, err
}

type treeEntry struct {
	mode, kind, id, name string
}

func (r *Repo) readTree(ctx context.Context, tree string) ([]treeEntry, error) {
	if tree == "" {
		return nil, nil
	}
	out, err := r.git(ctx, nil, nil, "ls-tree", "-z", tree)
	if err != nil {
		return nil, err
	}
	var entries []treeEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, name, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("procedural repository: unexpected tree entry %q", rec)
		}
		entries = append(entries, treeEntry{fields[0], fields[1], fields[2], name})
	}
	return entries, nil
}

func (r *Repo) makeTree(ctx context.Context, entries []treeEntry) (string, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var input bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&input, "%s %s %s\t%s\x00", e.mode, e.kind, e.id, e.name)
	}
	out, err := r.git(ctx, input.Bytes(), nil, "mktree", "-z")
	return strings.TrimSpace(string(out)), err
}

// writeFiles writes files (paths relative to the tree) as one tree.
func (r *Repo) writeFiles(ctx context.Context, files map[string][]byte) (string, error) {
	blobs := map[string][]byte{}
	subdirs := map[string]map[string][]byte{}
	for name, data := range files {
		first, rest, nested := strings.Cut(name, "/")
		if !nested {
			blobs[first] = data
			continue
		}
		if subdirs[first] == nil {
			subdirs[first] = map[string][]byte{}
		}
		subdirs[first][rest] = data
	}
	var entries []treeEntry
	for name, data := range blobs {
		if _, clash := subdirs[name]; clash {
			return "", fmt.Errorf("procedural repository: %s is both a file and a folder", name)
		}
		id, err := r.git(ctx, data, nil, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		entries = append(entries, treeEntry{"100644", "blob", strings.TrimSpace(string(id)), name})
	}
	for name, sub := range subdirs {
		id, err := r.writeFiles(ctx, sub)
		if err != nil {
			return "", err
		}
		entries = append(entries, treeEntry{"040000", "tree", id, name})
	}
	return r.makeTree(ctx, entries)
}

// splice returns base with the tree at components replaced by child (or
// removed when child is "").
func (r *Repo) splice(ctx context.Context, base string, components []string, child string) (string, error) {
	entries, err := r.readTree(ctx, base)
	if err != nil {
		return "", err
	}
	name := components[0]
	var kept []treeEntry
	var current string
	for _, e := range entries {
		if e.name == name {
			if e.kind == "tree" {
				current = e.id
			}
			continue
		}
		kept = append(kept, e)
	}
	replacement := child
	if len(components) > 1 {
		if replacement, err = r.splice(ctx, current, components[1:], child); err != nil {
			return "", err
		}
	}
	if replacement != "" {
		kept = append(kept, treeEntry{"040000", "tree", replacement, name})
	}
	return r.makeTree(ctx, kept)
}

// Commit makes one commit on master whose tree is master's with dir
// replaced by exactly files (paths relative to dir), and returns it. When
// the tree would not change it makes no commit and returns master's commit
// with changed false. A concurrent writer's commit is never lost: the
// master update is a compare-and-swap retried on top of the new commit.
func (r *Repo) Commit(ctx context.Context, dir string, files map[string][]byte, message string) (commit string, changed bool, err error) {
	if err := cleanPath(dir); err != nil {
		return "", false, err
	}
	if len(files) == 0 {
		return "", false, errors.New("procedural repository: a commit needs at least one file")
	}
	for name, data := range files {
		if err := cleanPath(name); err != nil {
			return "", false, err
		}
		if len(data) > MaxFileBytes {
			return "", false, fmt.Errorf("procedural repository: %s is larger than %d KiB", name, MaxFileBytes>>10)
		}
	}
	if strings.TrimSpace(message) == "" || strings.ContainsRune(message, 0) {
		return "", false, errors.New("procedural repository: a commit needs a message")
	}
	if err := r.ensure(ctx); err != nil {
		return "", false, err
	}
	child, err := r.writeFiles(ctx, files)
	if err != nil {
		return "", false, err
	}
	for attempt := 0; ; attempt++ {
		parent, err := r.head(ctx)
		if err != nil {
			return "", false, err
		}
		var parentTree string
		if parent != "" {
			if parentTree, err = r.gitText(ctx, "rev-parse", "--verify", parent+"^{tree}"); err != nil {
				return "", false, err
			}
		}
		tree, err := r.splice(ctx, parentTree, strings.Split(dir, "/"), child)
		if err != nil {
			return "", false, err
		}
		if tree == parentTree {
			return parent, false, nil
		}
		args := []string{"commit-tree", tree, "-F", "-"}
		if parent != "" {
			args = append(args, "-p", parent)
		}
		identity := []string{"GIT_AUTHOR_NAME=Evie", "GIT_AUTHOR_EMAIL=evie@localhost", "GIT_COMMITTER_NAME=Evie", "GIT_COMMITTER_EMAIL=evie@localhost"}
		out, err := r.git(ctx, []byte(message), identity, args...)
		if err != nil {
			return "", false, err
		}
		commit = strings.TrimSpace(string(out))
		old := parent
		if old == "" {
			old = strings.Repeat("0", len(commit))
		}
		if _, err := r.git(ctx, nil, nil, "update-ref", "-m", "evie: "+firstLine(message), branch, commit, old); err == nil {
			return commit, true, nil
		} else if attempt+1 >= maxCASRetries || ctx.Err() != nil {
			return "", false, fmt.Errorf("procedural repository: master kept moving: %w", err)
		}
		select {
		case <-ctx.Done():
			return "", false, ctx.Err()
		case <-time.After(time.Duration(1+rand.IntN(20)) * time.Millisecond):
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// ReadFile reads one regular file at an exact commit.
func (r *Repo) ReadFile(ctx context.Context, commit, name string) ([]byte, error) {
	if err := checkCommit(commit); err != nil {
		return nil, err
	}
	if err := cleanPath(name); err != nil {
		return nil, err
	}
	if err := r.ensure(ctx); err != nil {
		return nil, err
	}
	if kind, err := r.gitText(ctx, "cat-file", "-t", commit); err != nil || kind != "commit" {
		return nil, fmt.Errorf("procedural repository: %s is not a commit", commit)
	}
	out, err := r.git(ctx, nil, nil, "ls-tree", "-l", "-z", commit, "--", name)
	if err != nil {
		return nil, err
	}
	rec := strings.TrimSuffix(string(out), "\x00")
	if rec == "" {
		return nil, fmt.Errorf("procedural repository: %s at %s: %w", name, short(commit), fs.ErrNotExist)
	}
	meta, got, ok := strings.Cut(rec, "\t")
	fields := strings.Fields(meta)
	if !ok || got != name || len(fields) != 4 {
		return nil, fmt.Errorf("procedural repository: %s at %s: %w", name, short(commit), fs.ErrNotExist)
	}
	if fields[0] != "100644" && fields[0] != "100755" || fields[1] != "blob" {
		return nil, fmt.Errorf("procedural repository: %s is not a regular file", name)
	}
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || size > MaxFileBytes {
		return nil, fmt.Errorf("procedural repository: %s is larger than %d KiB", name, MaxFileBytes>>10)
	}
	return r.git(ctx, nil, nil, "cat-file", "blob", fields[2])
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// Source is a workflow definition Source: dir's files at one commit.
type Source struct {
	ctx    context.Context
	repo   *Repo
	commit string
	dir    string
}

// Source reads dir at commit. Reads use ctx.
func (r *Repo) Source(ctx context.Context, commit, dir string) Source {
	return Source{ctx: ctx, repo: r, commit: commit, dir: dir}
}

// ReadFile reads one file of the definition, by slash path relative to its
// directory.
func (s Source) ReadFile(name string) ([]byte, error) {
	if err := cleanPath(name); err != nil {
		return nil, err
	}
	return s.repo.ReadFile(s.ctx, s.commit, s.dir+"/"+name)
}

// Diff is the patch of dir from one commit to another, with paths relative
// to dir. An empty from compares with nothing.
func (r *Repo) Diff(ctx context.Context, from, to, dir string) (string, error) {
	if err := cleanPath(dir); err != nil {
		return "", err
	}
	if err := checkCommit(to); err != nil {
		return "", err
	}
	if err := r.ensure(ctx); err != nil {
		return "", err
	}
	base := from
	if from == "" {
		empty, err := r.git(ctx, []byte{}, nil, "hash-object", "-t", "tree", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		base = strings.TrimSpace(string(empty))
	} else if err := checkCommit(from); err != nil {
		return "", err
	}
	out, err := r.git(ctx, nil, nil, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames",
		"--relative="+dir+"/", base, to, "--", dir)
	if err != nil {
		return "", err
	}
	if len(out) > maxPatchBytes {
		return string(out[:maxPatchBytes]) + "\n[diff truncated]\n", nil
	}
	return string(out), nil
}
