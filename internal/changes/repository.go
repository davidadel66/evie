// Package changes is a Workspace's Repository and the Changes made in it
// (changes.spec.md). Story 1: the Repository is read from the Workspace
// folder each time, never stored. A folder at the top of a git work tree is
// a local Repository; an origin naming github.com/<owner>/<name> makes it a
// GitHub Repository, whose token access is checked against GitHub.
package changes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/worktree"
)

// Kind is what a Workspace has: no Repository, a local one, or one on GitHub.
type Kind string

const (
	None   Kind = "none"
	Local  Kind = "local"
	GitHub Kind = "github"
)

// TokenState is what GitHub said about the token for this Repository.
type TokenState string

const (
	Connected    TokenState = "connected"
	NoToken      TokenState = "no_token"
	InvalidToken TokenState = "invalid_token"
	NoAccess     TokenState = "no_access"
	Unreachable  TokenState = "unreachable"
)

// Repository is a Workspace's Repository as its folder shows it now.
type Repository struct {
	Kind   Kind   `json:"kind"`
	Folder string `json:"folder,omitempty"`
	// Problem says why there is no Repository.
	Problem string `json:"problem,omitempty"`
	// BaseBranch is GitHub's default branch when GitHub was read, else the
	// folder's current branch.
	BaseBranch string `json:"base_branch,omitempty"`
	// RemoteURL is origin as configured, with any credentials removed.
	RemoteURL string        `json:"remote_url,omitempty"`
	Owner     string        `json:"owner,omitempty"`
	Name      string        `json:"name,omitempty"`
	GitHub    *GitHubStatus `json:"github,omitempty"`
}

// GitHubStatus is the token check for a GitHub Repository.
type GitHubStatus struct {
	State   TokenState `json:"state"`
	Login   string     `json:"login,omitempty"`
	Private *bool      `json:"private,omitempty"`
	URL     string     `json:"url,omitempty"`
	Message string     `json:"message,omitempty"`
}

// GitHubClient is what the changes module needs from GitHub.
type GitHubClient interface {
	User(context.Context) (github.User, error)
	Repository(ctx context.Context, owner, name string) (github.Repository, error)
	CreateRepository(ctx context.Context, name string, private, autoInit bool) (github.Repository, error)
}

// Config is how startup composition builds a Service. Without GitHub (no
// token configured) local Repositories work the same and GitHub actions
// are refused.
type Config struct {
	GitHub GitHubClient
	// Token reaches git only through a per-command environment, for HTTPS
	// to github.com; never a remote URL, git config or output.
	Token string
	// AllowFileRemotes lets git push to local paths; tests stand bare
	// repositories in for GitHub with it. Production allows HTTPS only.
	AllowFileRemotes bool
}

// Service reads and changes Workspace Repositories.
type Service struct {
	github    GitHubClient
	token     string
	allowFile bool
	mu        sync.Mutex
	busy      map[string]bool
}

func NewService(c Config) *Service {
	return &Service{github: c.GitHub, token: c.Token, allowFile: c.AllowFileRemotes, busy: map[string]bool{}}
}

// Repository reads the Repository at a Workspace folder ("" for none). Only
// a failure to run git at all is an error; a folder that is not a
// Repository is a Repository of kind none with the reason.
func (s *Service) Repository(ctx context.Context, folder string) (Repository, error) {
	if folder == "" {
		return Repository{Kind: None, Problem: "This Workspace has no folder."}, nil
	}
	real, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return Repository{Kind: None, Folder: folder, Problem: "The folder cannot be read."}, nil
	}
	top, err := worktree.Git(ctx, real, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{Kind: None, Folder: folder, Problem: "The folder is not a git repository."}, nil
	}
	if top, err = filepath.EvalSymlinks(top); err != nil {
		return Repository{}, fmt.Errorf("read the repository's top folder: %w", err)
	}
	if top != real {
		return Repository{Kind: None, Folder: folder, Problem: fmt.Sprintf("The folder is inside the repository at %s; choose its top folder.", top)}, nil
	}
	if bare, _ := worktree.Git(ctx, real, "rev-parse", "--is-bare-repository"); bare == "true" {
		return Repository{Kind: None, Folder: folder, Problem: "The folder is a bare repository."}, nil
	}
	branch, _ := worktree.Git(ctx, real, "symbolic-ref", "--short", "-q", "HEAD")
	r := Repository{Kind: Local, Folder: folder, BaseBranch: branch}
	// origin as configured: get-url would apply insteadOf rewrites.
	origin, err := worktree.Git(ctx, real, "config", "--get", "remote.origin.url")
	if err != nil || origin == "" {
		return r, nil
	}
	r.RemoteURL = withoutCredentials(origin)
	owner, name, ok := gitHubRepository(origin)
	if !ok {
		return r, nil
	}
	r.Kind, r.Owner, r.Name = GitHub, owner, name
	r.GitHub = s.check(ctx, &r)
	return r, nil
}

// check reads the token's owner and the repository; GitHub's default branch
// replaces the local one when the repository could be read.
func (s *Service) check(ctx context.Context, r *Repository) *GitHubStatus {
	if s.github == nil {
		return &GitHubStatus{State: NoToken, Message: "GitHub is not connected. Add GITHUB_TOKEN to ~/.evie/.env and restart evie serve."}
	}
	user, err := s.github.User(ctx)
	if err != nil {
		return refusal(err, r)
	}
	repo, err := s.github.Repository(ctx, r.Owner, r.Name)
	if err != nil {
		status := refusal(err, r)
		status.Login = user.Login
		return status
	}
	if repo.DefaultBranch != "" {
		r.BaseBranch = repo.DefaultBranch
	}
	private := repo.Private
	return &GitHubStatus{State: Connected, Login: user.Login, Private: &private, URL: repo.HTMLURL}
}

func refusal(err error, r *Repository) *GitHubStatus {
	switch {
	case errors.Is(err, github.ErrUnauthorized):
		return &GitHubStatus{State: InvalidToken, Message: "GitHub did not accept the token in GITHUB_TOKEN. Create a new fine-grained token and restart evie serve."}
	case errors.Is(err, github.ErrNotFound):
		return &GitHubStatus{State: NoAccess, Message: fmt.Sprintf("The token cannot read %s/%s. Give it access to this repository in its settings on GitHub.", r.Owner, r.Name)}
	case errors.Is(err, github.ErrForbidden):
		return &GitHubStatus{State: NoAccess, Message: fmt.Sprintf("GitHub refused to show %s/%s: %s", r.Owner, r.Name, strings.TrimPrefix(err.Error(), github.ErrForbidden.Error()+": "))}
	}
	return &GitHubStatus{State: Unreachable, Message: "GitHub could not be reached; this shows what git knows."}
}

var (
	scpLike    = regexp.MustCompile(`^[A-Za-z0-9._-]+@([A-Za-z0-9.-]+):([^/]+)/([^/]+?)(?:\.git)?/?$`)
	repoSuffix = regexp.MustCompile(`\.git$`)
)

// gitHubRepository reads owner and name from an origin on github.com:
// https://github.com/o/n(.git), ssh://git@github.com/o/n.git or
// git@github.com:o/n.git.
func gitHubRepository(origin string) (owner, name string, ok bool) {
	host, path := "", ""
	if m := scpLike.FindStringSubmatch(origin); m != nil && !strings.Contains(origin, "://") {
		host, path = m[1], m[2]+"/"+m[3]
	} else if u, err := url.Parse(origin); err == nil && (u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "ssh" || u.Scheme == "git") {
		host, path = u.Hostname(), strings.Trim(u.Path, "/")
	} else {
		return "", "", false
	}
	host = strings.ToLower(host)
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	parts := strings.Split(repoSuffix.ReplaceAllString(strings.TrimSuffix(path, "/"), ""), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// withoutCredentials removes a URL's user and password for http(s); an ssh
// user such as git@ is not a secret and stays.
func withoutCredentials(origin string) string {
	u, err := url.Parse(origin)
	if err != nil || u.User == nil || (u.Scheme != "https" && u.Scheme != "http") {
		return origin
	}
	u.User = nil
	return u.String()
}
