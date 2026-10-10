package changes_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/github"
)

// changes.spec.md story 2b: a new Workspace's Repository is set up before
// the Workspace is registered: a new local repository, a new GitHub
// repository cloned into the folder, or an existing one cloned or adopted.

func author(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%an %s")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func freeFolder(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "project")
}

func TestPrepareMakesANewLocalRepositoryWithOneEmptyCommit(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	folder := freeFolder(t)
	r, err := changes.NewService(changes.Config{}).Prepare(context.Background(), changes.Prepare{Choice: changes.NewLocal, Folder: folder})
	if err != nil || r.Kind != changes.Local || r.BaseBranch != "main" || r.Folder != folder || r.RemoteURL != "" {
		t.Fatalf("new local = %+v, %v", r, err)
	}
	// No git identity is configured here, so the first commit is Evie's.
	if got := author(t, folder); got != "Evie Initial commit" {
		t.Fatalf("first commit = %q", got)
	}
}

func TestPrepareRefusesAFolderThatIsTakenOrHasNoParent(t *testing.T) {
	ctx := context.Background()
	service := changes.NewService(changes.Config{})
	taken := t.TempDir()
	if _, err := service.Prepare(ctx, changes.Prepare{Choice: changes.NewLocal, Folder: taken}); !errors.Is(err, changes.ErrFolderUnavailable) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("taken: %v", err)
	}
	orphan := filepath.Join(t.TempDir(), "missing", "project")
	if _, err := service.Prepare(ctx, changes.Prepare{Choice: changes.NewLocal, Folder: orphan}); !errors.Is(err, changes.ErrFolderUnavailable) {
		t.Fatalf("no parent: %v", err)
	}
	if _, err := service.Prepare(ctx, changes.Prepare{Choice: changes.NewLocal, Folder: "relative/project"}); !errors.Is(err, changes.ErrFolderUnavailable) {
		t.Fatalf("relative: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("a refusal made %s", orphan)
	}
}

func TestPrepareCreatesAGitHubRepositoryAndClonesIt(t *testing.T) {
	fake, server := newFakeGitHub(t)
	folder := freeFolder(t)
	r, err := publishService(server).Prepare(context.Background(), changes.Prepare{Choice: changes.NewGitHub, Folder: folder, Name: "notes", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	created := fake.created["notes"]
	if created == nil || created["private"] != true || created["auto_init"] != true {
		t.Fatalf("created = %v", created)
	}
	if r.Kind != changes.GitHub || r.Owner != "davidadel66" || r.Name != "notes" || r.RemoteURL != cloneURL("notes") || r.BaseBranch != "main" {
		t.Fatalf("prepared = %+v", r)
	}
	if got := author(t, folder); got != "GitHub Initial commit" {
		t.Fatalf("clone's first commit = %q", got)
	}
	noToken(t, folder, r.RemoteURL)
	if _, err := changes.NewService(changes.Config{}).Prepare(context.Background(), changes.Prepare{Choice: changes.NewGitHub, Folder: freeFolder(t), Name: "x"}); !errors.Is(err, changes.ErrNoToken) {
		t.Fatalf("no token: %v", err)
	}
	if _, err := publishService(server).Prepare(context.Background(), changes.Prepare{Choice: changes.NewGitHub, Folder: freeFolder(t), Name: "taken"}); !errors.Is(err, github.ErrUnprocessable) {
		t.Fatalf("taken name: %v", err)
	}
}

func TestPrepareClonesAnExistingRepositoryOrAdoptsAMatchingClone(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	fake.seed("evie", false)
	service := publishService(server)

	folder := freeFolder(t)
	r, err := service.Prepare(ctx, changes.Prepare{Choice: changes.ExistingGitHub, Folder: folder, Owner: "davidadel66", Name: "evie"})
	if err != nil || r.Kind != changes.GitHub || r.Name != "evie" || r.RemoteURL != cloneURL("evie") {
		t.Fatalf("clone = %+v, %v", r, err)
	}
	// The same folder again is a clone of that repository: adopted as is.
	if r, err := service.Prepare(ctx, changes.Prepare{Choice: changes.ExistingGitHub, Folder: folder, Owner: "DavidAdel66", Name: "evie"}); err != nil || r.Name != "evie" {
		t.Fatalf("adopt = %+v, %v", r, err)
	}
	if len(fake.created) != 1 {
		t.Fatalf("created %v", fake.created)
	}
	other := repository(t, "main")
	gitIn(t, other, "remote", "add", "origin", "https://github.com/davidadel66/other.git")
	if _, err := service.Prepare(ctx, changes.Prepare{Choice: changes.ExistingGitHub, Folder: other, Owner: "davidadel66", Name: "evie"}); !errors.Is(err, changes.ErrFolderUnavailable) || !strings.Contains(err.Error(), "other") {
		t.Fatalf("a clone of another repository: %v", err)
	}
	if _, err := service.Prepare(ctx, changes.Prepare{Choice: changes.ExistingGitHub, Folder: freeFolder(t), Owner: "davidadel66", Name: "missing"}); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("a repository the token cannot see: %v", err)
	}
	list, err := service.GitHubRepositories(ctx)
	if err != nil || len(list) != 1 || list[0].FullName != "davidadel66/evie" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
