// Package github is the small GitHub REST client the Changes feature reads
// and later writes through (changes.spec.md). One fine-grained personal
// access token, read from GITHUB_TOKEN by startup composition, is sent only
// in the Authorization header; nothing here places it in a URL, a log or a
// returned error.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// API is GitHub's REST API base URL; tests point a Client at a fake.
const API = "https://api.github.com"

var (
	// ErrUnauthorized is a missing, expired or revoked token (401).
	ErrUnauthorized = errors.New("github: the token was not accepted")
	// ErrNotFound is a repository that does not exist or that the token
	// cannot see: GitHub answers both with 404.
	ErrNotFound = errors.New("github: not found, or the token cannot see it")
	// ErrForbidden is a refusal the token cannot get past as it is, such as
	// a missing permission or a rate limit (403); the message says which.
	ErrForbidden = errors.New("github: forbidden")
	// ErrInvalidName is an owner or repository name GitHub could not have.
	ErrInvalidName = errors.New("github: invalid owner or repository name")
	// ErrUnprocessable is a request GitHub refused as given (422), such as
	// a repository name the account already has; the message says why.
	ErrUnprocessable = errors.New("github: refused")
)

var validName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

// Client calls GitHub's REST API with one token.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New returns a client for base (API in production). A nil http client
// gets one with a ten-second timeout.
func New(base, token string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: client}
}

// User is the account a token belongs to.
type User struct {
	Login string `json:"login"`
}

// Repository is what Evie reads about a repository.
type Repository struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// User reads the token's owner, which also proves the token works.
func (c *Client) User(ctx context.Context) (User, error) {
	var user User
	return user, c.do(ctx, http.MethodGet, "/user", nil, &user)
}

// Repository reads one repository the token can see.
func (c *Client) Repository(ctx context.Context, owner, name string) (Repository, error) {
	var repo Repository
	if !ValidName(owner) || !ValidName(name) {
		return repo, ErrInvalidName
	}
	return repo, c.do(ctx, http.MethodGet, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), nil, &repo)
}

// CreateRepository creates a repository on the token owner's personal
// account. autoInit gives it GitHub's first commit, so it can be cloned.
func (c *Client) CreateRepository(ctx context.Context, name string, private, autoInit bool) (Repository, error) {
	var repo Repository
	if !ValidName(name) {
		return repo, ErrInvalidName
	}
	body := map[string]any{"name": name, "private": private, "auto_init": autoInit}
	return repo, c.do(ctx, http.MethodPost, "/user/repos", body, &repo)
}

// Repositories lists the token owner's own repositories, most recently
// updated first, up to 100.
func (c *Client) Repositories(ctx context.Context) ([]Repository, error) {
	var repos []Repository
	return repos, c.do(ctx, http.MethodGet, "/user/repos?affiliation=owner&per_page=100&sort=updated", nil, &repos)
}

// ValidName says whether GitHub could have an owner or repository named so.
func ValidName(name string) bool {
	return validName.MatchString(name) && strings.Trim(name, ".") != ""
}

func (c *Client) do(ctx context.Context, method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "evie")
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL is ours and holds no token; the error names only the host.
		return fmt.Errorf("github: cannot reach %s: %w", req.URL.Host, errors.Unwrap(err))
	}
	defer resp.Body.Close()
	// Bounded, yet room for a full page of pull requests (about 20 KB each).
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("github: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrForbidden, message(data))
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s", ErrUnprocessable, message(data))
	case resp.StatusCode >= 300:
		return fmt.Errorf("github: %s: %s", resp.Status, message(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("github: unexpected response: %w", err)
	}
	return nil
}

// message is GitHub's own explanation of a refusal, bounded: the first
// detailed error when it gives one (a 422's "name already exists on this
// account"), else its message.
func message(body []byte) string {
	var value struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &value) != nil {
		return "no explanation given"
	}
	text := value.Message
	if len(value.Errors) > 0 && value.Errors[0].Message != "" {
		text = value.Errors[0].Message
	}
	if text == "" {
		return "no explanation given"
	}
	if len(text) > 200 {
		return text[:200]
	}
	return text
}
