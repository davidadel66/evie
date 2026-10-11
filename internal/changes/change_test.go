package changes_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/memory"
)

// fixedSource answers with the Changes it was given.
type fixedSource []changes.Change

func (f fixedSource) Changes(context.Context, memory.WorkspaceID, changes.Repository) ([]changes.Change, error) {
	return f, nil
}

// changes.spec.md story 3: a Workspace's list merges Evie's build branches
// and, for a GitHub Repository, its pull requests: unfinished Changes
// first, newest first, then the 20 most recently finished.
func TestListMergesSourcesUnfinishedFirstAndKeepsTwentyFinished(t *testing.T) {
	ctx := context.Background()
	at := func(minutes int) time.Time {
		return time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute)
	}
	builds := fixedSource{
		{ID: "build:a", State: changes.Draft, UpdatedAt: at(1)},
		{ID: "build:b", State: changes.Open, UpdatedAt: at(5)},
	}
	var pulls fixedSource
	pulls = append(pulls, changes.Change{ID: "pr:7", State: changes.Open, UpdatedAt: at(3)})
	for i := 0; i < 25; i++ {
		pulls = append(pulls, changes.Change{ID: fmt.Sprintf("pr:%d", 100+i), State: changes.Merged, UpdatedAt: at(-i)})
	}
	local := repository(t, "main")
	github := repository(t, "main")
	gitIn(t, github, "remote", "add", "origin", "https://github.com/davidadel66/evie.git")
	lister := &changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: builds, PullRequests: pulls}

	repo, list, err := lister.List(ctx, "w", github)
	if err != nil || repo.Kind != changes.GitHub {
		t.Fatalf("list = %+v, %v", repo, err)
	}
	ids := []string{}
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	if len(ids) != 23 || ids[0] != "build:b" || ids[1] != "pr:7" || ids[2] != "build:a" || ids[3] != "pr:100" || ids[22] != "pr:119" {
		t.Fatalf("order = %v", ids)
	}
	// A local Repository has no pull requests.
	if _, list, _ := lister.List(ctx, "w", local); len(list) != 2 {
		t.Fatalf("local = %+v", list)
	}
	// No Repository, no Changes.
	if repo, list, err := lister.List(ctx, "w", t.TempDir()); err != nil || repo.Kind != changes.None || list == nil || len(list) != 0 {
		t.Fatalf("none = %+v %+v %v", repo, list, err)
	}
}

// A pull request on Evie's build branch is that Change on GitHub: one
// Change, Evie's, with the pull request's number, state and checks; any
// other pull request stays the owner's own Change.
func TestListLinksAPullRequestToEviesBuildBranch(t *testing.T) {
	ctx := context.Background()
	builds := fixedSource{
		{ID: "build:evie/1a2b3c4d-fix-x", Branch: "evie/1a2b3c4d-fix-x", State: changes.Draft, Author: changes.ByEvie, Title: "Fix x", Notes: "Fixed x.", ExecutionID: "e1"},
		{ID: "build:evie/5e6f7a8b-add-y", Branch: "evie/5e6f7a8b-add-y", State: changes.Closed, Author: changes.ByEvie, Title: "Add y", ExecutionID: "e2"},
		{ID: "build:evie/9c0d1e2f-local", Branch: "evie/9c0d1e2f-local", State: changes.Draft, Author: changes.ByEvie, Title: "No pull request yet", ExecutionID: "e3"},
	}
	checks := []changes.Check{{Name: "unit", State: changes.CheckPassed}}
	pulls := fixedSource{
		{ID: "pr:12", Branch: "evie/1a2b3c4d-fix-x", State: changes.Open, Author: changes.ByOwner, Title: "Fix x (PR)", Notes: "PR body",
			PullRequest: &changes.PullRequest{Number: 12, State: "open"}, Checks: checks},
		{ID: "pr:13", Branch: "evie/5e6f7a8b-add-y", State: changes.Merged, Author: changes.ByOwner, PullRequest: &changes.PullRequest{Number: 13, State: "merged"}},
		{ID: "pr:14", Branch: "feature/mine", State: changes.Open, Author: changes.ByOwner, Title: "My change", PullRequest: &changes.PullRequest{Number: 14, State: "open"}},
	}
	repo := repository(t, "main")
	gitIn(t, repo, "remote", "add", "origin", "https://github.com/davidadel66/evie.git")
	_, list, err := (&changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: builds, PullRequests: pulls}).List(ctx, "w", repo)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]changes.Change{}
	for _, c := range list {
		byID[c.ID] = c
	}
	if len(list) != 4 || byID["pr:12"].ID != "" || byID["pr:13"].ID != "" {
		t.Fatalf("list = %+v", list)
	}
	fix := byID["build:evie/1a2b3c4d-fix-x"]
	// Evie gave this build no title of its own, so its pull request's title
	// and description are the Change's (story 6a).
	if fix.State != changes.Open || fix.Author != changes.ByEvie || fix.Title != "Fix x (PR)" || fix.Notes != "PR body" || fix.ExecutionID != "e1" ||
		fix.PullRequest == nil || fix.PullRequest.Number != 12 || len(fix.Checks) != 1 {
		t.Fatalf("linked = %+v", fix)
	}
	// GitHub's state wins: merged there, though its worktree was cleaned up.
	if add := byID["build:evie/5e6f7a8b-add-y"]; add.State != changes.Merged || add.PullRequest.Number != 13 {
		t.Fatalf("merged on GitHub = %+v", add)
	}
	if local := byID["build:evie/9c0d1e2f-local"]; local.State != changes.Draft || local.PullRequest != nil {
		t.Fatalf("no pull request = %+v", local)
	}
	if mine := byID["pr:14"]; mine.Author != changes.ByOwner || mine.Title != "My change" {
		t.Fatalf("owner's = %+v", mine)
	}
}

func TestABranchWithSeveralPullRequestsIsItsOpenOneElseItsNewest(t *testing.T) {
	ctx := context.Background()
	at := func(minutes int) time.Time {
		return time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC).Add(time.Duration(minutes) * time.Minute)
	}
	pr := func(n int, branch string, state changes.State, minutes int) changes.Change {
		return changes.Change{ID: fmt.Sprintf("pr:%d", n), Branch: branch, State: state, UpdatedAt: at(minutes), PullRequest: &changes.PullRequest{Number: n}}
	}
	builds := fixedSource{
		{ID: "build:evie/a", Branch: "evie/a", State: changes.Draft},
		{ID: "build:evie/b", Branch: "evie/b", State: changes.Draft},
	}
	pulls := fixedSource{
		pr(1, "evie/a", changes.Closed, 30), pr(2, "evie/a", changes.Open, 10), // the open one, though older
		pr(3, "evie/b", changes.Closed, 10), pr(4, "evie/b", changes.Closed, 20), // the newer of two finished
	}
	repo := repository(t, "main")
	gitIn(t, repo, "remote", "add", "origin", "https://github.com/davidadel66/evie.git")
	_, list, err := (&changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: builds, PullRequests: pulls}).List(ctx, "w", repo)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	for _, c := range list {
		want := map[string]int{"build:evie/a": 2, "build:evie/b": 4}[c.ID]
		if c.PullRequest == nil || c.PullRequest.Number != want {
			t.Fatalf("%s linked %+v, want #%d", c.ID, c.PullRequest, want)
		}
	}
}
