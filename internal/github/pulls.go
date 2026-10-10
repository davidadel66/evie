package github

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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

// CheckRun is one check run on a commit. Conclusion is empty until Status
// is completed.
type CheckRun struct {
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

// CheckRuns lists the check runs on a commit, up to 100.
func (c *Client) CheckRuns(ctx context.Context, owner, name, sha string) ([]CheckRun, error) {
	var runs struct {
		CheckRuns []CheckRun `json:"check_runs"`
	}
	path, err := commitPath(owner, name, sha)
	if err != nil {
		return nil, err
	}
	return runs.CheckRuns, c.do(ctx, http.MethodGet, path+"/check-runs?per_page=100", nil, &runs)
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
