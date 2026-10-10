package changes

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/worktree"
)

// Publish to GitHub (changes.spec.md story 2a): a local Repository becomes
// a new repository on the token owner's account, origin is set to it, and
// only the base branch is pushed. Unmerged branches stay local.

var (
	// ErrNoToken is a GitHub action with no GITHUB_TOKEN configured.
	ErrNoToken = errors.New("GitHub is not connected: add GITHUB_TOKEN to ~/.evie/.env and restart evie serve")
	// ErrNotPublishable is a folder that is not a local Repository ready to
	// publish; the error says why.
	ErrNotPublishable = errors.New("cannot publish this folder")
	// ErrPushFailed is a repository GitHub created whose first push failed;
	// publishing again with the same name retries the push.
	ErrPushFailed = errors.New("the push to GitHub failed")
	// ErrBusy is a second action on a folder while one is running.
	ErrBusy = errors.New("another change to this Repository is running")
)

// Publish is what the owner chose in Settings → Repository.
type Publish struct {
	Folder  string
	Name    string
	Private bool
}

// Publish creates the GitHub repository, sets origin and pushes the base
// branch. A publish whose push failed left origin pointing at the new
// repository; publishing again with the same name pushes to it instead of
// creating another.
func (s *Service) Publish(ctx context.Context, p Publish) (Repository, error) {
	release, err := s.claim(p.Folder)
	if err != nil {
		return Repository{}, err
	}
	defer release()
	r, err := s.Repository(ctx, p.Folder)
	if err != nil {
		return r, err
	}
	switch {
	case r.Kind == None:
		return r, fmt.Errorf("%w: %s", ErrNotPublishable, r.Problem)
	case r.BaseBranch == "":
		return r, fmt.Errorf("%w: the folder has no branch checked out; check out the branch to publish", ErrNotPublishable)
	case s.github == nil:
		return r, ErrNoToken
	case !github.ValidName(p.Name):
		return r, github.ErrInvalidName
	}
	user, err := s.github.User(ctx)
	if err != nil {
		return r, err
	}
	var repo github.Repository
	if r.RemoteURL != "" {
		// Only a retry may find origin already set: it must name the
		// repository GitHub has under this name.
		existing, err := s.github.Repository(ctx, user.Login, p.Name)
		if err != nil || existing.CloneURL != r.RemoteURL {
			return r, fmt.Errorf("%w: origin already points at %s; publishing would replace it", ErrNotPublishable, r.RemoteURL)
		}
		repo = existing
	} else {
		if repo, err = s.github.CreateRepository(ctx, p.Name, p.Private, false); err != nil {
			if errors.Is(err, github.ErrForbidden) {
				return r, fmt.Errorf("%w. Creating repositories needs the token's Administration permission set to read and write", err)
			}
			return r, err
		}
		if _, err := worktree.Git(ctx, p.Folder, "remote", "add", "origin", repo.CloneURL); err != nil {
			return r, fmt.Errorf("created %s, but origin could not be set: %w", repo.HTMLURL, err)
		}
	}
	if _, err := s.remoteGit(ctx, p.Folder, "push", "--set-upstream", "origin", r.BaseBranch); err != nil {
		return r, fmt.Errorf("%w: created %s and set origin, but pushing %s failed: %v. Publish again with the same name to retry the push",
			ErrPushFailed, repo.HTMLURL, r.BaseBranch, err)
	}
	return s.Repository(ctx, p.Folder)
}

// claim lets one action at a time change a folder's Repository.
func (s *Service) claim(folder string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy[folder] {
		return nil, ErrBusy
	}
	s.busy[folder] = true
	return func() {
		s.mu.Lock()
		delete(s.busy, folder)
		s.mu.Unlock()
	}, nil
}

// remoteGit runs a git command that talks to GitHub. It is hardened like
// worktree.Git, with no credential helper, and allows only HTTPS (and local
// paths in tests). The token rides in git's per-command config environment
// as an auth header for github.com: never in the arguments, git config, the
// remote URL or the output.
func (s *Service) remoteGit(ctx context.Context, dir string, args ...string) (string, error) {
	config := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.pager=cat", "-c", "commit.gpgSign=false",
		"-c", "submodule.recurse=false", "-c", "credential.helper=", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always"}
	if s.allowFile {
		config = append(config, "-c", "protocol.file.allow=always")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append(config, args...)...)
	cmd.Dir = dir
	cmd.Env = s.remoteEnv()
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		message := s.scrub(strings.TrimSpace(stderr.String()))
		if len(message) > 400 {
			message = message[:400]
		}
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s *Service) remoteEnv() []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}
	home, _ := os.UserHomeDir()
	env := []string{"PATH=" + path, "HOME=" + home, "LANG=C", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_EDITOR=true", "GIT_ASKPASS=/usr/bin/false"}
	for _, name := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "XDG_CONFIG_HOME"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	if s.token != "" {
		header := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+s.token))
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_0="+header)
	}
	return env
}

// scrub removes the token in either form from text that may be shown.
func (s *Service) scrub(text string) string {
	if s.token == "" {
		return text
	}
	text = strings.ReplaceAll(text, s.token, "[token]")
	return strings.ReplaceAll(text, base64.StdEncoding.EncodeToString([]byte("x-access-token:"+s.token)), "[token]")
}

// Account checks the token alone, for actions that create repositories.
func (s *Service) Account(ctx context.Context) GitHubStatus {
	if s.github == nil {
		return GitHubStatus{State: NoToken, Message: ErrNoToken.Error() + "."}
	}
	user, err := s.github.User(ctx)
	switch {
	case err == nil:
		return GitHubStatus{State: Connected, Login: user.Login}
	case errors.Is(err, github.ErrUnauthorized):
		return GitHubStatus{State: InvalidToken, Message: "GitHub did not accept the token in GITHUB_TOKEN. Create a new fine-grained token and restart evie serve."}
	}
	return GitHubStatus{State: Unreachable, Message: "GitHub could not be reached."}
}
