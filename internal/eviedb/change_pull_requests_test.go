package eviedb

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// changes.spec.md story 3b: each sync records a Workspace's pull requests,
// replacing what the last sync knew about each one; a list reads them
// back newest first, one Workspace at a time.
func TestChangePullRequestsRoundTripNewestFirstPerWorkspace(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	clock := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	store.UseClock(func() time.Time { return clock }, time.UTC)
	evie, err := store.RegisterWorkspace(ctx, "evie")
	if err != nil {
		t.Fatal(err)
	}
	notes, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	at := func(hour int) time.Time { return time.Date(2026, 10, 10, hour, 0, 0, 0, time.UTC) }
	no := false
	older := ChangePullRequest{Number: 3, Owner: "davidadel66", Repository: "evie", Branch: "mine", Base: "master", Title: "Older",
		Author: "davidadel66", State: "open", HeadSHA: "abcdef1", URL: "https://github.com/davidadel66/evie/pull/3", GitHubUpdatedAt: at(8)}
	newer := ChangePullRequest{Number: 7, Owner: "davidadel66", Repository: "evie", Branch: "feature/sync", Base: "master", Title: "Add sync",
		Body: "Why.", Author: "davidadel66", State: "open", Draft: true, HeadSHA: "0123456789abcdef0123456789abcdef01234567", Mergeable: &no,
		URL: "https://github.com/davidadel66/evie/pull/7", GitHubUpdatedAt: at(9),
		Checks: []ChangePullRequestCheck{{Name: "go test", State: "passed", URL: "https://github.com/davidadel66/evie/runs/1"}, {Name: "ui", State: "running"}}}
	if err := store.SaveChangePullRequests(ctx, evie.ID, []ChangePullRequest{older, newer}); err != nil {
		t.Fatal(err)
	}
	other := ChangePullRequest{Number: 3, Owner: "davidadel66", Repository: "notes", Branch: "b", Base: "main", Title: "Notes", Author: "davidadel66",
		State: "open", HeadSHA: "abcdef2", URL: "https://github.com/davidadel66/notes/pull/3", GitHubUpdatedAt: at(10)}
	// Half a second later sorts first, though GitHub's times are whole seconds.
	later := other
	later.Number, later.GitHubUpdatedAt = 4, at(10).Add(500*time.Millisecond)
	if err := store.SaveChangePullRequests(ctx, notes.ID, []ChangePullRequest{other, later}); err != nil {
		t.Fatal(err)
	}

	list, err := store.ChangePullRequests(ctx, evie.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	want := newer
	want.WorkspaceID, want.SyncedAt = evie.ID, clock
	if !reflect.DeepEqual(list[0], want) {
		t.Fatalf("newest = %+v\nwant     %+v", list[0], want)
	}
	if list[1].Number != 3 || list[1].Mergeable != nil || list[1].Checks != nil || list[1].Title != "Older" {
		t.Fatalf("older = %+v", list[1])
	}

	// The next sync sees #3 merged on GitHub: the record is replaced.
	clock = clock.Add(time.Minute)
	merged := older
	merged.State, merged.GitHubUpdatedAt = "merged", at(11)
	if err := store.SaveChangePullRequests(ctx, evie.ID, []ChangePullRequest{merged}); err != nil {
		t.Fatal(err)
	}
	list, err = store.ChangePullRequests(ctx, evie.ID)
	if err != nil || len(list) != 2 || list[0].Number != 3 || list[0].State != "merged" || !list[0].SyncedAt.Equal(clock) || list[1].Number != 7 {
		t.Fatalf("after merge = %+v, %v", list, err)
	}
	// The other Workspace's #3 is its own.
	if list, err := store.ChangePullRequests(ctx, notes.ID); err != nil || len(list) != 2 || list[0].Number != 4 || list[1].Number != 3 ||
		list[1].State != "open" || list[1].Repository != "notes" {
		t.Fatalf("notes = %+v, %v", list, err)
	}
	if list, err := store.ChangePullRequests(ctx, memory.WorkspaceID("missing")); err != nil || len(list) != 0 {
		t.Fatalf("missing = %+v, %v", list, err)
	}
}

func TestChangePullRequestsRefuseWhatGitHubCouldNotHaveSent(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "evie")
	if err != nil {
		t.Fatal(err)
	}
	valid := ChangePullRequest{Number: 1, Owner: "o", Repository: "r", Branch: "b", Base: "main", State: "open", HeadSHA: "abcdef1"}
	for name, change := range map[string]func(*ChangePullRequest){
		"state":  func(p *ChangePullRequest) { p.State = "draft" },
		"number": func(p *ChangePullRequest) { p.Number = 0 },
	} {
		bad := valid
		change(&bad)
		// One bad record refuses the whole sync's results.
		if err := store.SaveChangePullRequests(ctx, ws.ID, []ChangePullRequest{valid, bad}); err == nil {
			t.Fatalf("%s: saved %+v", name, bad)
		}
	}
	if err := store.SaveChangePullRequests(ctx, "missing", []ChangePullRequest{valid}); err == nil {
		t.Fatal("saved a pull request for a Workspace that does not exist")
	}
	if list, err := store.ChangePullRequests(ctx, ws.ID); err != nil || len(list) != 0 {
		t.Fatalf("after refusals = %+v, %v", list, err)
	}
}
