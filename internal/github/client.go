// Package github is the small GitHub REST client the Changes feature reads
// and later writes through (changes.spec.md). One fine-grained personal
// access token, read from GITHUB_TOKEN by startup composition, is sent only
// in the Authorization header; nothing here places it in a URL, a log or a
// returned error.
package github

import (
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
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// User reads the token's owner, which also proves the token works.
func (c *Client) User(ctx context.Context) (User, error) {
	var user User
	return user, c.get(ctx, "/user", &user)
}

// Repository reads one repository the token can see.
func (c *Client) Repository(ctx context.Context, owner, name string) (Repository, error) {
	var repo Repository
	if !validName.MatchString(owner) || !validName.MatchString(name) || strings.Trim(owner, ".") == "" || strings.Trim(name, ".") == "" {
		return repo, ErrInvalidName
	}
	return repo, c.get(ctx, "/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), &repo)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("github: read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrForbidden, message(body))
	case resp.StatusCode >= 300:
		return fmt.Errorf("github: %s: %s", resp.Status, message(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("github: unexpected response: %w", err)
	}
	return nil
}

// message is GitHub's own explanation of a refusal, bounded.
func message(body []byte) string {
	var value struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &value) != nil || value.Message == "" {
		return "no explanation given"
	}
	if len(value.Message) > 200 {
		return value.Message[:200]
	}
	return value.Message
}
