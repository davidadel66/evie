package changes_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// storedPulls answers with the records it was given.
type storedPulls map[memory.WorkspaceID][]eviedb.ChangePullRequest

func (s storedPulls) ChangePullRequests(_ context.Context, ws memory.WorkspaceID) ([]eviedb.ChangePullRequest, error) {
	return s[ws], nil
}

// changes.spec.md story 3b: every synced pull request of the Workspace's
// GitHub Repository is a Change: open, merged or closed as GitHub says,
// GitHub's draft marking kept, its body as the notes and its checks.
func TestPullRequestSourceMapsSyncedPullRequestsToChanges(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	no := false
	open := eviedb.ChangePullRequest{WorkspaceID: "w", Number: 7, Owner: "davidadel66", Repository: "evie", Branch: "feature/sync", Base: "master",
		Title: "Add sync", Body: "Why it changed.", Author: "davidadel66", State: "open", Draft: true, HeadSHA: "0123456789abcdef0123456789abcdef01234567",
		Mergeable: &no, URL: "https://github.com/davidadel66/evie/pull/7", GitHubUpdatedAt: at,
		Checks: []eviedb.ChangePullRequestCheck{{Name: "go test", State: "passed", URL: "https://github.com/davidadel66/evie/runs/1"}, {Name: "ui", State: "failed"}}}
	merged := eviedb.ChangePullRequest{WorkspaceID: "w", Number: 5, Owner: "DavidAdel66", Repository: "Evie", Branch: "mine", Base: "master",
		Title: "Merged on GitHub", Author: "davidadel66", State: "merged", HeadSHA: "abcdef1", GitHubUpdatedAt: at.Add(-time.Hour)}
	closed := merged
	closed.Number, closed.State, closed.Title = 4, "closed", "Closed on GitHub"
	// A pull request of the Repository the folder used to have is not this
	// Repository's Change.
	elsewhere := merged
	elsewhere.Number, elsewhere.Repository = 3, "notes"
	source := changes.PullRequestSource{Store: storedPulls{"w": {open, merged, closed, elsewhere}}}
	repo := changes.Repository{Kind: changes.GitHub, Owner: "davidadel66", Name: "evie", BaseBranch: "master"}

	list, err := source.Changes(ctx, "w", repo)
	if err != nil || len(list) != 3 {
		t.Fatalf("changes = %+v, %v", list, err)
	}
	want := changes.Change{ID: "pr:7", WorkspaceID: "w", Branch: "feature/sync", Base: "master", State: changes.Open, Author: changes.ByOwner,
		Title: "Add sync", Notes: "Why it changed.", Head: "0123456789abcdef0123456789abcdef01234567",
		PullRequest: &changes.PullRequest{Number: 7, URL: "https://github.com/davidadel66/evie/pull/7", State: "open", Draft: true,
			HeadSHA: "0123456789abcdef0123456789abcdef01234567", Author: "davidadel66", Mergeable: &no, UpdatedAt: at},
		Checks: []changes.Check{{Name: "go test", State: changes.CheckPassed, URL: "https://github.com/davidadel66/evie/runs/1"}, {Name: "ui", State: changes.CheckFailed}},
		// GitHub found it cannot merge this head (story 5b).
		NeedsRebase: true,
		UpdatedAt:   at}
	if !reflect.DeepEqual(list[0], want) {
		t.Fatalf("open = %+v %+v\nwant   %+v %+v", list[0], list[0].PullRequest, want, want.PullRequest)
	}
	if list[1].ID != "pr:5" || list[1].State != changes.Merged || list[1].PullRequest.State != "merged" || list[1].Checks != nil || list[1].PullRequest.Draft {
		t.Fatalf("merged = %+v", list[1])
	}
	if list[2].ID != "pr:4" || list[2].State != changes.Closed || list[2].Title != "Closed on GitHub" {
		t.Fatalf("closed = %+v", list[2])
	}

	// A local Repository has no pull requests, nor has another Workspace.
	if list, err := source.Changes(ctx, "w", changes.Repository{Kind: changes.Local}); err != nil || len(list) != 0 {
		t.Fatalf("local = %+v, %v", list, err)
	}
	if list, err := source.Changes(ctx, "other", repo); err != nil || len(list) != 0 {
		t.Fatalf("other Workspace = %+v, %v", list, err)
	}
}
