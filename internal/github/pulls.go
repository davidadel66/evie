package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pull requests and checks (changes.spec.md story 3b): what sync reads to
// list a GitHub Repository's open pull requests, notice merges and closes
// done on GitHub, and show the checks on each pull request's latest commit.

var (
	// ErrInvalidNumber is a pull request number GitHub could not have.
	ErrInvalidNumber = errors.New("github: invalid pull request number")
	// ErrInvalidSHA is a commit that is not 7 to 40 hexadecimal characters.
	ErrInvalidSHA = errors.New("github: invalid commit")
)

var validSHA = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// PullRequest is what Evie reads about a pull request. Merged and
// Mergeable come only from reading one pull request; a list leaves them
// unset.
type PullRequest struct {
	Number int `json:"number"`
	// State is open or closed; a merged pull request is closed and Merged.
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	MergedAt  *time.Time `json:"merged_at"`
	Mergeable *bool      `json:"mergeable"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	HTMLURL   string     `json:"html_url"`
	UpdatedAt time.Time  `json:"updated_at"`
	Head      Ref        `json:"head"`
	Base      Ref        `json:"base"`
	User      User       `json:"user"`
}

// Ref is a pull request's head or base: its branch and commit.
type Ref struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// WorkflowRun is one GitHub Actions workflow run on a commit. Conclusion is
// empty until Status is completed. Evie reads CI from workflow runs because
// GitHub's Checks API does not accept fine-grained personal access tokens;
// workflow runs need the token's Actions permission (read).
type WorkflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

// CombinedStatus is a commit's statuses, the latest one for each context.
type CombinedStatus struct {
	State    string   `json:"state"`
	Statuses []Status `json:"statuses"`
}

// Status is one commit status: pending, success, failure or error.
type Status struct {
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"target_url"`
}

// PullRequests lists a repository's open pull requests, up to 100.
func (c *Client) PullRequests(ctx context.Context, owner, name string) ([]PullRequest, error) {
	var pulls []PullRequest
	path, err := repoPath(owner, name)
	if err != nil {
		return nil, err
	}
	return pulls, c.do(ctx, http.MethodGet, path+"/pulls?state=open&per_page=100", nil, &pulls)
}

// PullRequestsOnBranch lists the pull requests whose head is branch in the
// repository itself, open or not, up to 100, newest first (story 4b).
func (c *Client) PullRequestsOnBranch(ctx context.Context, owner, name, branch string) ([]PullRequest, error) {
	var pulls []PullRequest
	path, err := repoPath(owner, name)
	if err != nil {
		return nil, err
	}
	if !validRef(branch) {
		return nil, ErrInvalidRef
	}
	query := url.Values{"state": {"all"}, "head": {owner + ":" + branch}, "per_page": {"100"}}
	return pulls, c.do(ctx, http.MethodGet, path+"/pulls?"+query.Encode(), nil, &pulls)
}

// PullRequest reads one pull request, open or not.
func (c *Client) PullRequest(ctx context.Context, owner, name string, number int) (PullRequest, error) {
	var pull PullRequest
	path, err := repoPath(owner, name)
	if err != nil {
		return pull, err
	}
	if number < 1 {
		return pull, ErrInvalidNumber
	}
	return pull, c.do(ctx, http.MethodGet, path+"/pulls/"+strconv.Itoa(number), nil, &pull)
}

// WorkflowRuns lists the GitHub Actions workflow runs on a commit, up to
// 100, newest first.
func (c *Client) WorkflowRuns(ctx context.Context, owner, name, sha string) ([]WorkflowRun, error) {
	var runs struct {
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	path, err := repoPath(owner, name)
	if err != nil {
		return nil, err
	}
	if !validSHA.MatchString(sha) {
		return nil, ErrInvalidSHA
	}
	return runs.WorkflowRuns, c.do(ctx, http.MethodGet, path+"/actions/runs?head_sha="+sha+"&per_page=100", nil, &runs)
}

// CombinedStatus reads a commit's statuses, up to 100 contexts.
func (c *Client) CombinedStatus(ctx context.Context, owner, name, sha string) (CombinedStatus, error) {
	var status CombinedStatus
	path, err := commitPath(owner, name, sha)
	if err != nil {
		return status, err
	}
	return status, c.do(ctx, http.MethodGet, path+"/status?per_page=100", nil, &status)
}

func repoPath(owner, name string) (string, error) {
	if !ValidName(owner) || !ValidName(name) {
		return "", ErrInvalidName
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name), nil
}

func commitPath(owner, name, sha string) (string, error) {
	path, err := repoPath(owner, name)
	if err != nil {
		return "", err
	}
	if !validSHA.MatchString(sha) {
		return "", ErrInvalidSHA
	}
	return path + "/commits/" + sha, nil
}

// CreatePullRequest opens a pull request from head into base: a ready one,
// not a GitHub draft.
func (c *Client) CreatePullRequest(ctx context.Context, owner, name, head, base, title, body string) (PullRequest, error) {
	var pull PullRequest
	path, err := repoPath(owner, name)
	if err != nil {
		return pull, err
	}
	if !validRef(head) || !validRef(base) {
		return pull, ErrInvalidRef
	}
	if strings.TrimSpace(title) == "" || len(title) > 256 {
		return pull, errors.New("github: a pull request needs a title of at most 256 bytes")
	}
	payload := map[string]any{"title": title, "head": head, "base": base, "body": body, "draft": false}
	return pull, c.do(ctx, http.MethodPost, path+"/pulls", payload, &pull)
}

// ErrInvalidRef is a branch name git would refuse.
var ErrInvalidRef = errors.New("github: invalid branch name")

// validRef is a plausible branch name: no control characters, spaces or
// the characters git forbids, and no "..".
func validRef(ref string) bool {
	if ref == "" || len(ref) > 255 || strings.Contains(ref, "..") || strings.HasPrefix(ref, "-") {
		return false
	}
	for _, r := range ref {
		if r <= ' ' || r == 0x7f || strings.ContainsRune("~^:?*[\\", r) {
			return false
		}
	}
	return true
}
