package procedural

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func newRepo(t *testing.T) *Repo {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "procedural"))
}

func definitionFiles(t *testing.T, name string) map[string][]byte {
	t.Helper()
	src := filepath.Join("..", "workflow", "testdata", name)
	files := map[string][]byte{}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--git-dir=" + root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// The procedural repository is a private bare Git repository created on
// first use: owner-only, on master, with Evie's local identity. Nothing is
// created before then.
func TestProceduralRepositoryIsPrivateAndBare(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if _, err := os.Stat(repo.Root()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("New created the repository: %v", err)
	}
	commit, changed, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", definitionFiles(t, "notes-digest"), "Propose notes-digest")
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !fullCommit.MatchString(commit) {
		t.Fatalf("commit %q changed %v", commit, changed)
	}
	info, err := os.Stat(repo.Root())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("repository mode = %v, want 0700", info.Mode().Perm())
	}
	if got := gitOutput(t, repo.Root(), "rev-parse", "--is-bare-repository"); got != "true" {
		t.Fatalf("bare = %s", got)
	}
	if got := gitOutput(t, repo.Root(), "rev-parse", "refs/heads/master"); got != commit {
		t.Fatalf("master = %s, want %s", got, commit)
	}
	if got := gitOutput(t, repo.Root(), "log", "-1", "--format=%an <%ae>|%s", commit); got != "Evie <evie@localhost>|Propose notes-digest" {
		t.Fatalf("commit = %s", got)
	}
	head, err := repo.Head(ctx)
	if err != nil || head != commit {
		t.Fatalf("head = %q, %v", head, err)
	}
}

// A commit replaces the definition's directory with exactly the given files;
// every earlier commit keeps its content, and an unchanged tree makes no
// commit.
func TestProceduralCommitsReplaceExactlyOneDirectory(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	notes := definitionFiles(t, "notes-digest")
	first, _, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", notes, "first")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := repo.Commit(ctx, "workspaces/w2/workflows/deep-research", definitionFiles(t, "deep-research"), "other Workspace")
	if err != nil {
		t.Fatal(err)
	}
	if again, changed, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", notes, "same"); err != nil || changed || again != other {
		t.Fatalf("unchanged commit = %s changed %v err %v, want %s", again, changed, err, other)
	}
	edited := map[string][]byte{}
	for name, data := range notes {
		if name != "schemas/ask.schema.json" {
			edited[name] = data
		}
	}
	edited["README.md"] = []byte("# Notes digest\n\nRewritten.\n")
	second, changed, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", edited, "second")
	if err != nil || !changed {
		t.Fatalf("second commit: %v changed %v", err, changed)
	}
	if _, err := repo.ReadFile(ctx, second, "workspaces/w1/workflows/notes-digest/schemas/ask.schema.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a file left out of the commit survived: %v", err)
	}
	if data, err := repo.ReadFile(ctx, first, "workspaces/w1/workflows/notes-digest/README.md"); err != nil || string(data) != string(notes["README.md"]) {
		t.Fatalf("first commit's README = %q, %v", data, err)
	}
	if data, err := repo.ReadFile(ctx, second, "workspaces/w1/workflows/notes-digest/README.md"); err != nil || !strings.Contains(string(data), "Rewritten") {
		t.Fatalf("second commit's README = %q, %v", data, err)
	}
	// The other Workspace's definition is untouched by later commits.
	if _, err := repo.ReadFile(ctx, second, "workspaces/w2/workflows/deep-research/workflow.yaml"); err != nil {
		t.Fatal(err)
	}
	if got := gitOutput(t, repo.Root(), "rev-list", "--count", "master"); got != "3" {
		t.Fatalf("history has %s commits, want 3", got)
	}
}

// A Source at one commit reads that commit's tree, so validating it yields
// the same executable hash as the directory it came from.
func TestProceduralSourceValidatesAtOneCommit(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	env := workflow.Environment{Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}}
	dir := filepath.Join("..", "workflow", "testdata", "deep-research")
	fromDir, err := workflow.Validate(workflow.DirSource{Dir: dir}, env)
	if err != nil {
		t.Fatal(err)
	}
	commit, _, err := repo.Commit(ctx, "workspaces/w1/workflows/deep-research", fromDir.Files(), "propose")
	if err != nil {
		t.Fatal(err)
	}
	fromGit, err := workflow.Validate(repo.Source(ctx, commit, "workspaces/w1/workflows/deep-research"), env)
	if err != nil {
		t.Fatal(err)
	}
	if fromGit.Hash() != fromDir.Hash() {
		t.Fatalf("Git hash %s, directory hash %s", fromGit.Hash(), fromDir.Hash())
	}
	// A later commit does not change what the earlier commit's Source reads.
	changed := fromDir.Files()
	changed["prompts/plan.md"] = []byte("Plan differently.\n")
	if _, _, err := repo.Commit(ctx, "workspaces/w1/workflows/deep-research", changed, "edit"); err != nil {
		t.Fatal(err)
	}
	again, err := workflow.Validate(repo.Source(ctx, commit, "workspaces/w1/workflows/deep-research"), env)
	if err != nil || again.Hash() != fromDir.Hash() {
		t.Fatalf("earlier commit re-read as %v, %v", again, err)
	}
	// An empty README is still present, with the empty digest.
	empty := fromDir.Files()
	empty["README.md"] = []byte{}
	emptyCommit, _, err := repo.Commit(ctx, "workspaces/w1/workflows/deep-research", empty, "empty README")
	if err != nil {
		t.Fatal(err)
	}
	blank, err := workflow.Validate(repo.Source(ctx, emptyCommit, "workspaces/w1/workflows/deep-research"), env)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range blank.Report().Files {
		if f.Path == "README.md" && f.Digest != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			t.Fatalf("empty README digest = %q", f.Digest)
		}
	}
}

// Diffs compare one directory between two commits, or against nothing.
func TestProceduralDiffShowsOneDirectory(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	notes := definitionFiles(t, "notes-digest")
	first, _, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", notes, "first")
	if err != nil {
		t.Fatal(err)
	}
	notes["prompts/summarize.md"] = []byte("Summarize in three bullets.\n")
	second, _, err := repo.Commit(ctx, "workspaces/w1/workflows/notes-digest", notes, "second")
	if err != nil {
		t.Fatal(err)
	}
	patch, err := repo.Diff(ctx, first, second, "workspaces/w1/workflows/notes-digest")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(patch, "a/prompts/summarize.md") || !strings.Contains(patch, "+Summarize in three bullets.") || strings.Contains(patch, "workflow.yaml") {
		t.Fatalf("patch:\n%s", patch)
	}
	created, err := repo.Diff(ctx, "", first, "workspaces/w1/workflows/notes-digest")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created, "b/workflow.yaml") || !strings.Contains(created, "new file") {
		t.Fatalf("creation patch:\n%s", created)
	}
}

// Paths stay inside the repository's tree, the root must be a real
// directory, and commits are full hashes of commits.
func TestProceduralRefusesUnsafePathsAndRoots(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	for _, dir := range []string{"", "/abs", "../up", "a/../b", "a//b", "a/./b", ".git/x", "a/.GIT", "a\\b", "a\x00b"} {
		if _, _, err := repo.Commit(ctx, dir, map[string][]byte{"x.md": []byte("x")}, "bad"); err == nil {
			t.Errorf("directory %q was accepted", dir)
		}
	}
	for _, name := range []string{"../x.md", "/x.md", "a/../../x.md", ".git/config"} {
		if _, _, err := repo.Commit(ctx, "d", map[string][]byte{name: []byte("x")}, "bad"); err == nil {
			t.Errorf("file %q was accepted", name)
		}
	}
	commit, _, err := repo.Commit(ctx, "d", map[string][]byte{"x.md": []byte("x")}, "good")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"master", "HEAD", "--all", commit[:12], strings.Repeat("0", 40)} {
		if _, err := repo.ReadFile(ctx, c, "d/x.md"); err == nil {
			t.Errorf("commit %q was accepted", c)
		}
	}
	if _, err := repo.ReadFile(ctx, commit, "d"); err == nil {
		t.Error("a directory read as a file")
	}
	if _, err := repo.ReadFile(ctx, commit, "d/../d/x.md"); err == nil {
		t.Error("an unclean path was read")
	}

	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "procedural")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(link).Commit(ctx, "d", map[string][]byte{"x.md": []byte("x")}, "via link"); err == nil {
		t.Fatal("a symlinked repository root was accepted")
	}
	notRepo := t.TempDir()
	if err := os.WriteFile(filepath.Join(notRepo, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(notRepo).Commit(ctx, "d", map[string][]byte{"x.md": []byte("x")}, "foreign"); err == nil {
		t.Fatal("a folder that is not Evie's repository was used")
	}
	if _, err := New("relative/path").Head(ctx); err == nil {
		t.Fatal("a relative root was accepted")
	}
}

// Git commands are built, not inherited: a GIT_DIR or hook in the
// environment cannot redirect or run anything.
func TestProceduralIgnoresGitEnvironment(t *testing.T) {
	ctx := context.Background()
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.git")
	if out, err := exec.Command("git", "init", "--bare", "-q", elsewhere).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	t.Setenv("GIT_DIR", elsewhere)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
	repo := newRepo(t)
	commit, _, err := repo.Commit(ctx, "d", map[string][]byte{"x.md": []byte("x")}, "isolated")
	if err != nil {
		t.Fatal(err)
	}
	if got := gitOutput(t, repo.Root(), "rev-parse", "refs/heads/master"); got != commit {
		t.Fatalf("master = %s", got)
	}
	cmd := exec.Command("git", "--git-dir="+elsewhere, "rev-parse", "--verify", "-q", "refs/heads/master")
	cmd.Env = append(os.Environ(), "GIT_DIR=")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("GIT_DIR received a commit: %s", out)
	}
}

// Concurrent commits from several handles on one repository all land, each
// on top of the last: the master update is a compare-and-swap.
func TestProceduralConcurrentCommitsAllLand(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "procedural")
	if _, _, err := New(root).Commit(ctx, "seed", map[string][]byte{"x.md": []byte("seed")}, "seed"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := "workspaces/w/workflows/def-" + string(rune('a'+i))
			_, _, err := New(root).Commit(ctx, dir, map[string][]byte{"x.md": []byte(dir)}, dir)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := gitOutput(t, root, "rev-list", "--count", "master"); got != "7" {
		t.Fatalf("history has %s commits, want 7", got)
	}
	head, err := New(root).Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 6 {
		path := "workspaces/w/workflows/def-" + string(rune('a'+i)) + "/x.md"
		if _, err := New(root).ReadFile(ctx, head, path); err != nil {
			t.Fatalf("%s lost: %v", path, err)
		}
	}
}
