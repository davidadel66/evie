package changes_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/localfiles"
)

// changes.spec.md story 6a: the review page shows a Change with its diff,
// from local git: Evie's branch against the base it left, a local merge as
// what it brought in, and a pull request fetched from GitHub when no branch
// of Evie's holds it, the owner's own included.
func TestShowGivesAChangeWithItsDiff(t *testing.T) {
	ctx := context.Background()
	dir := repository(t, "main")
	commit := func(branch, path, content string) string {
		t.Helper()
		gitIn(t, dir, "switch", "-q", branch)
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "add", path)
		gitIn(t, dir, "-c", "user.name=Evie", "-c", "user.email=evie@localhost", "commit", "-q", "-m", "Change "+path)
		return head(t, dir, "HEAD")
	}
	gitIn(t, dir, "branch", "evie/1a2b3c4d-fix")
	commit("evie/1a2b3c4d-fix", "fix.txt", "fixed\n")
	before := head(t, dir, "main")
	merged := commit("main", "merged.txt", "merged\n")
	draft := changes.Change{ID: "build:evie/1a2b3c4d-fix", Branch: "evie/1a2b3c4d-fix", Base: "main", State: changes.Draft, Author: changes.ByEvie, ExecutionID: "e1"}
	done := changes.Change{ID: "build:evie/2b3c4d5e-done", Branch: "evie/2b3c4d5e-done", Base: "main", State: changes.Merged, Author: changes.ByEvie,
		ExecutionID: "e2", MergedFrom: before, Head: merged}
	gone := changes.Change{ID: "build:evie/3c4d5e6f-gone", Branch: "evie/3c4d5e6f-gone", Base: "main", State: changes.Closed, Author: changes.ByEvie, ExecutionID: "e3"}
	l := &changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: fixedSource{draft, done, gone}}
	files := func(d changes.ChangeDetail) map[string]localfiles.ComparedFile {
		out := map[string]localfiles.ComparedFile{}
		for _, f := range d.Files {
			out[f.Path] = f
		}
		return out
	}

	detail, err := l.Show(ctx, "w", dir, draft.ID)
	if err != nil || detail.Change.ID != draft.ID || detail.Repository.Kind != changes.Local || detail.Problem != "" {
		t.Fatalf("draft = %+v, %v", detail, err)
	}
	if f := files(detail); len(f) != 1 || f["fix.txt"].Status != "added" || !strings.Contains(f["fix.txt"].Patch, "+fixed") {
		t.Fatalf("draft files = %+v", detail.Files)
	}
	detail, err = l.Show(ctx, "w", dir, done.ID)
	if f := files(detail); err != nil || len(f) != 1 || !strings.Contains(f["merged.txt"].Patch, "+merged") {
		t.Fatalf("merged = %+v, %v", detail, err)
	}
	if detail, err := l.Show(ctx, "w", dir, gone.ID); err != nil || len(detail.Files) != 0 || !strings.Contains(detail.Problem, "gone") {
		t.Fatalf("gone = %+v, %v", detail, err)
	}
	if _, err := l.Show(ctx, "w", dir, "build:evie/ffffffff-none"); !errors.Is(err, changes.ErrUnknownChange) {
		t.Fatalf("unknown = %v", err)
	}

	// The owner's pull request on GitHub, fetched by its number.
	fake, server := newFakeGitHub(t)
	gh, built := trackedRepository(t, fake, server, "evie/4d5e6f7a-docs")
	gitIn(t, gh, "switch", "-q", "-c", "owner-idea", "main")
	if err := os.WriteFile(filepath.Join(gh, "idea.md"), []byte("an idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, gh, "add", "idea.md")
	gitIn(t, gh, "-c", "user.name=Owner", "-c", "user.email=owner@example.com", "commit", "-q", "-m", "Idea")
	idea := head(t, gh, "owner-idea")
	gitIn(t, gh, "push", "-q", fake.bare("notes"), "owner-idea:refs/pull/3/head")
	gitIn(t, gh, "switch", "-q", "main")
	gitIn(t, gh, "branch", "-q", "-D", "owner-idea")
	owner := changes.Change{ID: "pr:3", Branch: "owner-idea", Base: "main", State: changes.Open, Author: changes.ByOwner,
		PullRequest: &changes.PullRequest{Number: 3, State: "open", HeadSHA: idea}}
	l = &changes.Lister{Repositories: publishService(server), Builds: fixedSource{built["evie/4d5e6f7a-docs"], owner}}
	detail, err = l.Show(ctx, "w", gh, owner.ID)
	if f := files(detail); err != nil || detail.Problem != "" || len(f) != 1 || !strings.Contains(f["idea.md"].Patch, "+an idea") {
		t.Fatalf("owner pull request = %+v, %v", detail, err)
	}
}
