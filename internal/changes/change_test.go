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
