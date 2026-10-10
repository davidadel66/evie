package changes_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// changes.spec.md story 4a: the owner pushes one of Evie's draft Changes
// in a GitHub Repository: Verify first, then the branch, then its pull
// request into the branch it was built from, with Evie's notes.

// fixedVerifier answers every Verify with one outcome.
type fixedVerifier struct{ outcome *changes.VerifyOutcome }

func (f fixedVerifier) Verify(context.Context, memory.WorkspaceID, changes.Change) (*changes.VerifyOutcome, error) {
	return f.outcome, nil
}

// pushRepository is a clone of the fake's "notes" repository whose origin
// is its github.com URL, with an Evie branch carrying one commit.
func pushRepository(t *testing.T, fake *fakeGitHub) (string, string) {
	t.Helper()
	fake.seed("notes", true)
	dir := filepath.Join(t.TempDir(), "notes")
	gitIn(t, fake.root, "clone", "-q", fake.bare("notes"), dir)
	gitIn(t, dir, "remote", "set-url", "origin", cloneURL("notes"))
	gitIn(t, dir, "switch", "-q", "-c", "evie/1a2b3c4d-fix-footer")
	gitIn(t, dir, "-c", "user.name=Evie", "-c", "user.email=evie@localhost", "commit", "-q", "--allow-empty", "-m", "Fix the footer")
	gitIn(t, dir, "switch", "-q", "main")
	return dir, head(t, dir, "evie/1a2b3c4d-fix-footer")
}

func draftOn(dir string) changes.Change {
	return changes.Change{ID: "build:evie/1a2b3c4d-fix-footer", Branch: "evie/1a2b3c4d-fix-footer", Base: "main", State: changes.Draft,
		Author: changes.ByEvie, Title: "Fix the footer", Notes: "The footer overlapped the page.", Worktree: dir, ExecutionID: "e1",
		Origin: &changes.Origin{SessionID: "s1"}}
}

func newPusher(server *httptest.Server, builds fixedSource, outcome *changes.VerifyOutcome) *changes.Pusher {
	svc := publishService(server)
	return &changes.Pusher{Lister: &changes.Lister{Repositories: svc, Builds: builds}, Service: svc,
		GitHub: github.New(server.URL, token, server.Client()), Verifier: fixedVerifier{outcome}}
}

func TestPushSendsTheBranchAndOpensItsPullRequestWithEviesNotes(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	dir, branchHead := pushRepository(t, fake)
	passed := &changes.VerifyOutcome{Command: "go test ./...", Passed: true}
	p := newPusher(server, fixedSource{draftOn(dir)}, passed)

	result, err := p.Push(ctx, "w", dir, "evie/1a2b3c4d-fix-footer")
	if err != nil {
		t.Fatal(err)
	}
	if result.PullRequest != 1 || result.URL != "https://github.com/davidadel66/notes/pull/1" || result.Existing || result.Verify != passed {
		t.Fatalf("result = %+v", result)
	}
	if got := refs(t, fake.bare("notes")); !strings.Contains(got, "refs/heads/evie/1a2b3c4d-fix-footer "+branchHead) {
		t.Fatalf("GitHub has %q", got)
	}
	pulls := fake.pulls["notes"]
	if len(pulls) != 1 {
		t.Fatalf("opened %d pull requests", len(pulls))
	}
	pr := pulls[0]
	body, _ := pr["body"].(string)
	if pr["title"] != "Fix the footer" || pr["head"].(map[string]any)["ref"] != "evie/1a2b3c4d-fix-footer" || pr["base"].(map[string]any)["ref"] != "main" ||
		!strings.Contains(body, "The footer overlapped the page.") || !strings.Contains(body, "Verify passed: `go test ./...`") {
		t.Fatalf("pull request = %+v", pr)
	}
	noToken(t, dir, body, result.URL)

	// Pushing again sends any new commits and keeps the pull request.
	again, err := p.Push(ctx, "w", dir, "evie/1a2b3c4d-fix-footer")
	if err != nil || !again.Existing || again.PullRequest != 1 || len(fake.pulls["notes"]) != 1 {
		t.Fatalf("again = %+v, %v; pull requests %d", again, err, len(fake.pulls["notes"]))
	}
}

func TestAnOwnersPushGoesAheadAfterVerifyFailsAndSaysSo(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	dir, _ := pushRepository(t, fake)
	failed := &changes.VerifyOutcome{Command: "go test ./...", Passed: false, Output: "--- FAIL: TestFooter\nFAIL"}
	result, err := newPusher(server, fixedSource{draftOn(dir)}, failed).Push(ctx, "w", dir, "evie/1a2b3c4d-fix-footer")
	if err != nil || result.PullRequest != 1 || result.Verify == nil || result.Verify.Passed {
		t.Fatalf("result = %+v, %v", result, err)
	}
	body, _ := fake.pulls["notes"][0]["body"].(string)
	if !strings.Contains(body, "Verify failed: `go test ./...`") || !strings.Contains(body, "--- FAIL: TestFooter") {
		t.Fatalf("body = %q", body)
	}
	// Without a Verify command there is nothing to report.
	fake2, server2 := newFakeGitHub(t)
	dir2, _ := pushRepository(t, fake2)
	if _, err := newPusher(server2, fixedSource{draftOn(dir2)}, nil).Push(ctx, "w", dir2, "evie/1a2b3c4d-fix-footer"); err != nil {
		t.Fatal(err)
	}
	if body, _ := fake2.pulls["notes"][0]["body"].(string); strings.Contains(body, "Verify") {
		t.Fatalf("body = %q", body)
	}
}

func TestPushRefusesWhatItCannotPush(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	dir, _ := pushRepository(t, fake)
	branch := "evie/1a2b3c4d-fix-footer"
	refused := func(name string, p *changes.Pusher, folder, branch string, want error, says string) {
		t.Helper()
		if _, err := p.Push(ctx, "w", folder, branch); !errors.Is(err, want) || !strings.Contains(err.Error(), says) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	elsewhere := draftOn(dir)
	elsewhere.Base = "release"
	refused("a base GitHub lacks", newPusher(server, fixedSource{elsewhere}, nil), dir, branch, changes.ErrNotPushable, "release")
	merged := draftOn(dir)
	merged.State = changes.Merged
	refused("merged", newPusher(server, fixedSource{merged}, nil), dir, branch, changes.ErrNotPushable, "merged")
	refused("unknown branch", newPusher(server, fixedSource{draftOn(dir)}, nil), dir, "evie/ffffffff-other", changes.ErrNotPushable, "evie/ffffffff-other")
	local := repository(t, "main")
	refused("local", newPusher(server, fixedSource{draftOn(local)}, nil), local, branch, changes.ErrNotPushable, "not on GitHub")
	noGitHub := newPusher(server, fixedSource{draftOn(dir)}, nil)
	noGitHub.GitHub = nil
	refused("no token", noGitHub, dir, branch, changes.ErrNoToken, "GITHUB_TOKEN")
	if got := refs(t, fake.bare("notes")); strings.Contains(got, branch) || len(fake.pulls["notes"]) != 0 {
		t.Fatalf("a refused push reached GitHub: %q, %d pull requests", got, len(fake.pulls["notes"]))
	}
}
