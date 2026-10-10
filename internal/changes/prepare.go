package changes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/worktree"
)

// A Repository at Workspace creation (changes.spec.md story 2b). The
// Repository is set up first and the Workspace registered after, so a
// failure never leaves a Workspace pointing at a half-made folder.

// Choice is the Repository a new Workspace starts with.
type Choice string

const (
	// NewLocal makes the folder, a git repository on main, and one empty
	// first commit.
	NewLocal Choice = "local"
	// NewGitHub creates a repository on the token owner's account with
	// GitHub's first commit and clones it into the folder.
	NewGitHub Choice = "github_new"
	// ExistingGitHub clones one of the owner's repositories into the
	// folder, or adopts a clone already there whose origin is that
	// repository.
	ExistingGitHub Choice = "github_existing"
)

var (
	// ErrFolderUnavailable is a folder that cannot hold the new Repository.
	ErrFolderUnavailable = errors.New("the folder cannot be used")
	// ErrCloneFailed is a clone that failed; when Evie created the GitHub
	// repository first, the error says where it is.
	ErrCloneFailed = errors.New("the clone failed")
)

// Prepare is the Repository chosen for a new Workspace's folder.
type Prepare struct {
	Choice Choice
	Folder string
	// Name is the new repository's name, or the existing one's.
	Name string
	// Owner is the existing repository's owner.
	Owner   string
	Private bool
}

// Prepare sets up the Repository a new Workspace's folder will hold and
// returns it as the folder now shows it.
func (s *Service) Prepare(ctx context.Context, p Prepare) (Repository, error) {
	if !filepath.IsAbs(p.Folder) {
		return Repository{}, fmt.Errorf("%w: enter the full path of the folder, starting with /", ErrFolderUnavailable)
	}
	folder := filepath.Clean(p.Folder)
	release, err := s.claim(folder)
	if err != nil {
		return Repository{}, err
	}
	defer release()
	switch p.Choice {
	case NewLocal:
		err = s.newLocal(ctx, folder)
	case NewGitHub:
		err = s.newGitHub(ctx, folder, p.Name, p.Private)
	case ExistingGitHub:
		err = s.existingGitHub(ctx, folder, p.Owner, p.Name)
	default:
		err = fmt.Errorf("unknown repository choice %q", p.Choice)
	}
	if err != nil {
		return Repository{}, err
	}
	return s.Repository(ctx, folder)
}

// GitHubRepositories lists the token owner's repositories for Connect
// existing.
func (s *Service) GitHubRepositories(ctx context.Context) ([]github.Repository, error) {
	if s.github == nil {
		return nil, ErrNoToken
	}
	return s.github.Repositories(ctx)
}

// free checks that folder does not exist and that its parent is a folder.
func free(folder string) error {
	if _, err := os.Lstat(folder); err == nil {
		return fmt.Errorf("%w: %s already exists; choose a new folder", ErrFolderUnavailable, folder)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrFolderUnavailable, err)
	}
	if info, err := os.Stat(filepath.Dir(folder)); err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s does not exist; create it first", ErrFolderUnavailable, filepath.Dir(folder))
	}
	return nil
}

func (s *Service) newLocal(ctx context.Context, folder string) error {
	if err := free(folder); err != nil {
		return err
	}
	if err := os.Mkdir(folder, 0o755); err != nil {
		return fmt.Errorf("%w: %v", ErrFolderUnavailable, err)
	}
	if err := initialize(ctx, folder); err != nil {
		// The folder is Evie's own and new: nothing of the owner's is in it.
		os.RemoveAll(folder)
		return err
	}
	return nil
}

// initialize makes folder a repository on main with one empty commit, by
// the owner's git identity, or Evie's when none is configured.
func initialize(ctx context.Context, folder string) error {
	if _, err := worktree.Git(ctx, folder, "init", "-q", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	args := []string{"commit", "-q", "--allow-empty", "-m", "Initial commit"}
	if name, _ := worktree.Git(ctx, folder, "config", "user.name"); name == "" {
		args = append([]string{"-c", "user.name=Evie", "-c", "user.email=evie@localhost"}, args...)
	} else if email, _ := worktree.Git(ctx, folder, "config", "user.email"); email == "" {
		args = append([]string{"-c", "user.email=evie@localhost"}, args...)
	}
	if _, err := worktree.Git(ctx, folder, args...); err != nil {
		return fmt.Errorf("first commit: %w", err)
	}
	return nil
}

func (s *Service) newGitHub(ctx context.Context, folder, name string, private bool) error {
	switch {
	case s.github == nil:
		return ErrNoToken
	case !github.ValidName(name):
		return github.ErrInvalidName
	}
	if err := free(folder); err != nil {
		return err
	}
	repo, err := s.github.CreateRepository(ctx, name, private, true)
	if errors.Is(err, github.ErrForbidden) {
		return fmt.Errorf("%w. Creating repositories needs the token's Administration permission set to read and write", err)
	}
	if err != nil {
		return err
	}
	if err := s.clone(ctx, repo.CloneURL, folder); err != nil {
		return fmt.Errorf("%w: created %s, but cloning it into %s failed: %v. Choose Connect existing to clone it", ErrCloneFailed, repo.HTMLURL, folder, err)
	}
	return nil
}

func (s *Service) existingGitHub(ctx context.Context, folder, owner, name string) error {
	switch {
	case s.github == nil:
		return ErrNoToken
	case !github.ValidName(owner) || !github.ValidName(name):
		return github.ErrInvalidName
	}
	repo, err := s.github.Repository(ctx, owner, name)
	if errors.Is(err, github.ErrNotFound) {
		return fmt.Errorf("%w: the token cannot see %s/%s", err, owner, name)
	}
	if err != nil {
		return err
	}
	if _, err := os.Lstat(folder); err == nil {
		r, err := s.Repository(ctx, folder)
		if err != nil {
			return err
		}
		if r.Kind == GitHub && strings.EqualFold(r.Owner, owner) && strings.EqualFold(r.Name, name) {
			return nil
		}
		origin := r.RemoteURL
		if origin == "" {
			origin = "no GitHub origin"
		}
		return fmt.Errorf("%w: %s already exists and is not a clone of %s/%s (%s)", ErrFolderUnavailable, folder, owner, name, origin)
	}
	if err := free(folder); err != nil {
		return err
	}
	if err := s.clone(ctx, repo.CloneURL, folder); err != nil {
		return fmt.Errorf("%w: cloning %s into %s failed: %v", ErrCloneFailed, repo.FullName, folder, err)
	}
	return nil
}

// clone clones url into folder from its parent; a failed clone leaves no
// folder behind.
func (s *Service) clone(ctx context.Context, url, folder string) error {
	_, err := s.remoteGit(ctx, filepath.Dir(folder), "clone", "-q", "--", url, folder)
	if err != nil {
		os.RemoveAll(folder)
	}
	return err
}
