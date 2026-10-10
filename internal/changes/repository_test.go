package changes_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/github"
)

// changes.spec.md story 1: a Workspace's Repository is its folder, read
// from git, never stored: none, local, or on GitHub by its origin.

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repository makes a repository with one commit on branch.
func repository(t *testing.T, branch string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", branch)
	gitIn(t, dir, "-c", "user.name=Test", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "first")
	return dir
}

func TestRepositoryIsNoneWithoutAGitFolderAtItsTop(t *testing.T) {
	ctx := context.Background()
	service := changes.NewService(changes.Config{})
	if r, err := service.Repository(ctx, ""); err != nil || r.Kind != changes.None || !strings.Contains(r.Problem, "no folder") {
		t.Fatalf("no folder = %+v, %v", r, err)
	}
	plain := t.TempDir()
	if r, err := service.Repository(ctx, plain); err != nil || r.Kind != changes.None || !strings.Contains(r.Problem, "not a git repository") {
		t.Fatalf("plain folder = %+v, %v", r, err)
	}
	repo := repository(t, "main")
	inner := filepath.Join(repo, "docs")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if r, err := service.Repository(ctx, inner); err != nil || r.Kind != changes.None || !strings.Contains(r.Problem, repo) {
		t.Fatalf("inner folder = %+v, %v", r, err)
	}
}

func TestRepositoryIsLocalWithoutAGitHubOrigin(t *testing.T) {
	ctx := context.Background()
	service := changes.NewService(changes.Config{})
	repo := repository(t, "main")
	r, err := service.Repository(ctx, repo)
	if err != nil || r.Kind != changes.Local || r.BaseBranch != "main" || r.Folder != repo || r.GitHub != nil {
		t.Fatalf("no origin = %+v, %v", r, err)
	}
	gitIn(t, repo, "remote", "add", "origin", "https://gitlab.com/someone/project.git")
	if r, err := service.Repository(ctx, repo); err != nil || r.Kind != changes.Local || r.RemoteURL != "https://gitlab.com/someone/project.git" {
		t.Fatalf("another host = %+v, %v", r, err)
	}
}

func TestRepositoryReadsEveryGitHubOriginForm(t *testing.T) {
	ctx := context.Background()
	service := changes.NewService(changes.Config{})
	for _, origin := range []string{
		"https://github.com/davidadel66/evie.git",
		"https://github.com/davidadel66/evie",
		"git@github.com:davidadel66/evie.git",
		"ssh://git@github.com/davidadel66/evie.git",
		"https://www.github.com/davidadel66/evie/",
	} {
		repo := repository(t, "master")
		gitIn(t, repo, "remote", "add", "origin", origin)
		r, err := service.Repository(ctx, repo)
		if err != nil || r.Kind != changes.GitHub || r.Owner != "davidadel66" || r.Name != "evie" || r.RemoteURL != origin {
			t.Fatalf("%s = %+v, %v", origin, r, err)
		}
		// Without a token the local branch stands in for GitHub's default.
		if r.BaseBranch != "master" || r.GitHub == nil || r.GitHub.State != changes.NoToken {
			t.Fatalf("%s without a token = %+v", origin, r)
		}
	}
	for _, origin := range []string{"https://github.com/davidadel66", "https://github.com.evil.example/a/b", "https://notgithub.com/a/b"} {
		repo := repository(t, "main")
		gitIn(t, repo, "remote", "add", "origin", origin)
		if r, _ := service.Repository(ctx, repo); r.Kind != changes.Local {
			t.Fatalf("%s = %+v, want local", origin, r)
		}
	}
}

func TestRepositoryNeverEchoesCredentialsInItsOrigin(t *testing.T) {
	repo := repository(t, "main")
	gitIn(t, repo, "remote", "add", "origin", "https://x-access-token:ghp_secret@github.com/davidadel66/evie.git")
	r, err := changes.NewService(changes.Config{}).Repository(context.Background(), repo)
	if err != nil || r.Kind != changes.GitHub || r.Owner != "davidadel66" || r.Name != "evie" {
		t.Fatalf("repository = %+v, %v", r, err)
	}
	if strings.Contains(r.RemoteURL, "ghp_secret") || strings.Contains(r.RemoteURL, "x-access-token") || r.RemoteURL != "https://github.com/davidadel66/evie.git" {
		t.Fatalf("remote URL = %q", r.RemoteURL)
	}
}

func TestRepositoryChecksTheTokenAgainstGitHub(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case token == "bad":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/user":
			w.Write([]byte(`{"login":"davidadel66"}`))
		case r.URL.Path == "/repos/davidadel66/evie" && token == "good":
			w.Write([]byte(`{"name":"evie","full_name":"davidadel66/evie","private":true,"default_branch":"trunk","html_url":"https://github.com/davidadel66/evie"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	repo := repository(t, "master")
	gitIn(t, repo, "remote", "add", "origin", "git@github.com:davidadel66/evie.git")
	check := func(token string) changes.Repository {
		t.Helper()
		r, err := changes.NewService(changes.Config{GitHub: github.New(server.URL, token, server.Client())}).Repository(ctx, repo)
		if err != nil || r.Kind != changes.GitHub || r.GitHub == nil {
			t.Fatalf("%s: %+v, %v", token, r, err)
		}
		return r
	}
	r := check("good")
	if s := r.GitHub; s.State != changes.Connected || s.Login != "davidadel66" || s.Private == nil || !*s.Private || s.URL != "https://github.com/davidadel66/evie" || r.BaseBranch != "trunk" {
		t.Fatalf("connected = %+v %+v", r, s)
	}
	if s := check("bad").GitHub; s.State != changes.InvalidToken || s.Message == "" {
		t.Fatalf("bad token = %+v", s)
	}
	if s := check("other").GitHub; s.State != changes.NoAccess || !strings.Contains(s.Message, "davidadel66/evie") {
		t.Fatalf("no access = %+v", s)
	}
	server.Close()
	if r := check("good"); r.GitHub.State != changes.Unreachable || r.BaseBranch != "master" {
		t.Fatalf("unreachable = %+v %+v", r, r.GitHub)
	}
}

func TestAccountReportsTheTokenAlone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"login":"davidadel66"}`))
	}))
	ctx := context.Background()
	account := func(token string) changes.GitHubStatus {
		return changes.NewService(changes.Config{GitHub: github.New(server.URL, token, server.Client()), Token: token}).Account(ctx)
	}
	if s := account("good"); s.State != changes.Connected || s.Login != "davidadel66" {
		t.Fatalf("good = %+v", s)
	}
	if s := account("bad"); s.State != changes.InvalidToken {
		t.Fatalf("bad = %+v", s)
	}
	if s := changes.NewService(changes.Config{}).Account(ctx); s.State != changes.NoToken || !strings.Contains(s.Message, "GITHUB_TOKEN") {
		t.Fatalf("none = %+v", s)
	}
	server.Close()
	if s := account("good"); s.State != changes.Unreachable {
		t.Fatalf("unreachable = %+v", s)
	}
}
