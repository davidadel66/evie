package changes

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// PullRequestRecords reads a Workspace's pull requests as the last sync
// recorded them.
type PullRequestRecords interface {
	ChangePullRequests(context.Context, memory.WorkspaceID) ([]eviedb.ChangePullRequest, error)
}

// PullRequestSource is the Changes of a GitHub Repository's pull requests
// (changes.spec.md story 3b), read from what sync recorded; it never calls
// GitHub. Every pull request is the owner's for now: Evie opens its own
// with the owner's token, and linking one to Evie's build branch comes
// after this story.
type PullRequestSource struct {
	Store PullRequestRecords
	// Arms, when set, says which pull requests have "Merge when checks
	// pass" (story 5c).
	Arms MergeArms
}

// MergeArms reads "Merge when checks pass".
type MergeArms interface {
	ChangeMergeArms(context.Context, memory.WorkspaceID) (map[int]time.Time, error)
}

// Changes lists the Workspace's synced pull requests of repo. A Repository
// not on GitHub has none, and records of a repository the folder no longer
// points at are left out.
func (s PullRequestSource) Changes(ctx context.Context, ws memory.WorkspaceID, repo Repository) ([]Change, error) {
	list := []Change{}
	if repo.Kind != GitHub {
		return list, nil
	}
	records, err := s.Store.ChangePullRequests(ctx, ws)
	if err != nil {
		return nil, err
	}
	arms := map[int]time.Time{}
	if s.Arms != nil {
		if arms, err = s.Arms.ChangeMergeArms(ctx, ws); err != nil {
			return nil, err
		}
	}
	for _, p := range records {
		if sameRepository(p, repo) {
			c := pullRequestChange(ws, p)
			if at, ok := arms[p.Number]; ok && c.State == Open {
				c.ArmedAt = &at
			}
			list = append(list, c)
		}
	}
	return list, nil
}

// sameRepository says whether a record is of repo; GitHub matches owner
// and repository names in any case.
func sameRepository(p eviedb.ChangePullRequest, repo Repository) bool {
	return strings.EqualFold(p.Owner, repo.Owner) && strings.EqualFold(p.Repository, repo.Name)
}

func pullRequestChange(ws memory.WorkspaceID, p eviedb.ChangePullRequest) Change {
	state := Open
	switch p.State {
	case "merged":
		state = Merged
	case "closed":
		state = Closed
	}
	var mergeable *bool
	if p.Mergeable != nil {
		value := *p.Mergeable
		mergeable = &value
	}
	var checks []Check
	for _, c := range p.Checks {
		checks = append(checks, Check{Name: c.Name, State: CheckState(c.State), URL: c.URL})
	}
	return Change{
		ID:          "pr:" + strconv.Itoa(p.Number),
		WorkspaceID: ws,
		Branch:      p.Branch,
		Base:        p.Base,
		State:       state,
		Author:      ByOwner,
		Title:       p.Title,
		Notes:       p.Body,
		Head:        p.HeadSHA,
		PullRequest: &PullRequest{Number: p.Number, URL: p.URL, State: p.State, Draft: p.Draft, HeadSHA: p.HeadSHA,
			Author: p.Author, Mergeable: mergeable, UpdatedAt: p.GitHubUpdatedAt},
		Checks: checks,
		// GitHub found the head conflicts when the owner merged (story 5b).
		NeedsRebase: state == Open && mergeable != nil && !*mergeable,
		UpdatedAt:   p.GitHubUpdatedAt,
	}
}
