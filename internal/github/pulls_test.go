package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/github"
)

// changes.spec.md story 3b: a GitHub Repository's open pull requests, one
// pull request, and the checks on a commit (check runs and the combined
// status) are read through the same client and the same token.

const sha = "0123456789abcdef0123456789abcdef01234567"

func pullsServer(t *testing.T) (*github.Client, *[]string) {
	t.Helper()
	var mu sync.Mutex
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.RequestURI())
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer ghp_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/repos/davidadel66/evie/pulls":
			w.Write([]byte(`[{"number":7,"state":"open","draft":true,"title":"Add sync","body":"Why it changed.","html_url":"https://github.com/davidadel66/evie/pull/7",
				"updated_at":"2026-10-10T09:30:00Z","merged_at":null,"head":{"ref":"feature/sync","sha":"` + sha + `"},"base":{"ref":"master"},"user":{"login":"davidadel66"}},
				{"number":8,"state":"open","draft":false,"title":"No body","body":null,"html_url":"https://github.com/davidadel66/evie/pull/8",
				"updated_at":"2026-10-10T08:00:00Z","head":{"ref":"evie/1234abcd-x","sha":"abcdef1"},"base":{"ref":"master"},"user":{"login":"davidadel66"}}]`))
		case "/repos/davidadel66/evie/pulls/7":
			w.Write([]byte(`{"number":7,"state":"closed","merged":true,"merged_at":"2026-10-10T10:00:00Z","mergeable":null,"draft":false,"title":"Add sync","body":"Why it changed.",
				"html_url":"https://github.com/davidadel66/evie/pull/7","updated_at":"2026-10-10T10:00:01Z","head":{"ref":"feature/sync","sha":"` + sha + `"},"base":{"ref":"master"},"user":{"login":"davidadel66"}}`))
		case "/repos/davidadel66/evie/pulls/9":
			w.Write([]byte(`{"number":9,"state":"open","merged":false,"merged_at":null,"mergeable":false,"draft":false,"title":"Conflicts",
				"html_url":"https://github.com/davidadel66/evie/pull/9","updated_at":"2026-10-10T07:00:00Z","head":{"ref":"mine","sha":"` + sha + `"},"base":{"ref":"master"},"user":{"login":"davidadel66"}}`))
		case "/repos/davidadel66/evie/actions/runs":
			if r.URL.Query().Get("head_sha") != sha {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(`{"total_count":2,"workflow_runs":[{"id":11,"name":"verify","status":"completed","conclusion":"success","html_url":"https://github.com/davidadel66/evie/actions/runs/11"},
				{"id":12,"name":"ui","status":"in_progress","conclusion":null,"html_url":"https://github.com/davidadel66/evie/actions/runs/12"}]}`))
		case "/repos/davidadel66/evie/commits/" + sha + "/status":
			w.Write([]byte(`{"state":"failure","sha":"` + sha + `","statuses":[{"context":"ci/legacy","state":"failure","target_url":"https://ci.example.com/1","description":"broke"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return github.New(server.URL, "ghp_secret", server.Client()), &seen
}

func TestClientListsARepositorysOpenPullRequests(t *testing.T) {
	client, seen := pullsServer(t)
	pulls, err := client.PullRequests(context.Background(), "davidadel66", "evie")
	if err != nil || len(pulls) != 2 {
		t.Fatalf("pulls = %+v, %v", pulls, err)
	}
	if got := (*seen)[0]; got != "/repos/davidadel66/evie/pulls?state=open&per_page=100" {
		t.Fatalf("requested %q", got)
	}
	p := pulls[0]
	if p.Number != 7 || p.State != "open" || !p.Draft || p.Merged || p.MergedAt != nil || p.Mergeable != nil || p.Title != "Add sync" || p.Body != "Why it changed." ||
		p.HTMLURL != "https://github.com/davidadel66/evie/pull/7" || p.Head.Ref != "feature/sync" || p.Head.SHA != sha || p.Base.Ref != "master" ||
		p.User.Login != "davidadel66" || !p.UpdatedAt.Equal(time.Date(2026, 10, 10, 9, 30, 0, 0, time.UTC)) {
		t.Fatalf("pull 7 = %+v", p)
	}
	if pulls[1].Body != "" || pulls[1].Draft {
		t.Fatalf("pull 8 = %+v", pulls[1])
	}
}

func TestClientReadsOnePullRequestWithItsMergeState(t *testing.T) {
	client, seen := pullsServer(t)
	ctx := context.Background()
	merged, err := client.PullRequest(ctx, "davidadel66", "evie", 7)
	if err != nil || !merged.Merged || merged.MergedAt == nil || merged.State != "closed" || merged.Mergeable != nil {
		t.Fatalf("merged = %+v, %v", merged, err)
	}
	if got := (*seen)[0]; got != "/repos/davidadel66/evie/pulls/7" {
		t.Fatalf("requested %q", got)
	}
	conflicted, err := client.PullRequest(ctx, "davidadel66", "evie", 9)
	if err != nil || conflicted.Merged || conflicted.State != "open" || conflicted.Mergeable == nil || *conflicted.Mergeable {
		t.Fatalf("conflicted = %+v, %v", conflicted, err)
	}
	if _, err := client.PullRequest(ctx, "davidadel66", "evie", 404); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
}

func TestClientReadsTheChecksOnACommit(t *testing.T) {
	client, seen := pullsServer(t)
	ctx := context.Background()
	// Fine-grained tokens cannot read the Checks API, so CI is read from
	// the GitHub Actions workflow runs on the commit.
	runs, err := client.WorkflowRuns(ctx, "davidadel66", "evie", sha)
	if err != nil || len(runs) != 2 {
		t.Fatalf("workflow runs = %+v, %v", runs, err)
	}
	if runs[0].ID != 11 || runs[0].Name != "verify" || runs[0].Status != "completed" || runs[0].Conclusion != "success" ||
		runs[0].HTMLURL != "https://github.com/davidadel66/evie/actions/runs/11" || runs[1].Status != "in_progress" || runs[1].Conclusion != "" {
		t.Fatalf("workflow runs = %+v", runs)
	}
	status, err := client.CombinedStatus(ctx, "davidadel66", "evie", sha)
	if err != nil || status.State != "failure" || len(status.Statuses) != 1 {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if s := status.Statuses[0]; s.Context != "ci/legacy" || s.State != "failure" || s.TargetURL != "https://ci.example.com/1" {
		t.Fatalf("status = %+v", s)
	}
	want := []string{"/repos/davidadel66/evie/actions/runs?head_sha=" + sha + "&per_page=100", "/repos/davidadel66/evie/commits/" + sha + "/status?per_page=100"}
	if strings.Join(*seen, " ") != strings.Join(want, " ") {
		t.Fatalf("requested %v, want %v", *seen, want)
	}
}

// A full page of pull requests is larger than most answers: GitHub repeats
// both repositories in every item, so a hundred of them pass a megabyte.
func TestClientReadsAFullPageOfPullRequests(t *testing.T) {
	item := `{"number":%d,"state":"open","title":"t","body":"` + strings.Repeat("x", 20000) + `","head":{"ref":"b","sha":"abcdef1"},"base":{"ref":"main"},"user":{"login":"o"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := make([]string, 100)
		for i := range items {
			items[i] = strings.Replace(item, "%d", strconv.Itoa(i+1), 1)
		}
		w.Write([]byte("[" + strings.Join(items, ",") + "]"))
	}))
	defer server.Close()
	pulls, err := github.New(server.URL, "t", server.Client()).PullRequests(context.Background(), "o", "r")
	if err != nil || len(pulls) != 100 || pulls[99].Number != 100 {
		t.Fatalf("a full page = %d pull requests, %v", len(pulls), err)
	}
}

// Owner, name, number and commit are path segments: nothing crafted can
// steer a request elsewhere, and a refused one never reaches GitHub.
func TestClientRefusesCraftedNamesNumbersAndCommits(t *testing.T) {
	client, seen := pullsServer(t)
	ctx := context.Background()
	if _, err := client.PullRequests(ctx, "a/../../user", "evie"); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted owner: %v", err)
	}
	if _, err := client.PullRequests(ctx, "davidadel66", ".."); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted name: %v", err)
	}
	for _, number := range []int{0, -1} {
		if _, err := client.PullRequest(ctx, "davidadel66", "evie", number); !errors.Is(err, github.ErrInvalidNumber) {
			t.Fatalf("number %d: %v", number, err)
		}
	}
	if _, err := client.PullRequest(ctx, "davidadel66", "../x", 7); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted name for one pull request: %v", err)
	}
	for _, commit := range []string{"", "main", "abc123", "../../user", sha + "0", "0123456z", sha[:20] + "/status"} {
		if _, err := client.WorkflowRuns(ctx, "davidadel66", "evie", commit); !errors.Is(err, github.ErrInvalidSHA) {
			t.Fatalf("workflow runs for %q: %v", commit, err)
		}
		if _, err := client.CombinedStatus(ctx, "davidadel66", "evie", commit); !errors.Is(err, github.ErrInvalidSHA) {
			t.Fatalf("status for %q: %v", commit, err)
		}
	}
	if _, err := client.WorkflowRuns(ctx, "x?y", "evie", sha); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted owner for checks: %v", err)
	}
	if len(*seen) != 0 {
		t.Fatalf("refused requests reached GitHub: %v", *seen)
	}
	// Seven hex characters is the shortest commit GitHub resolves.
	if _, err := client.WorkflowRuns(ctx, "davidadel66", "evie", "abcdef1"); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("short commit: %v", err)
	}
}
