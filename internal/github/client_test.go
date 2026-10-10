package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidadel66/evie/internal/github"
)

// changes.spec.md: one fine-grained token, sent only as a bearer token;
// GitHub's refusals are told apart so Settings can say which one it was.
func TestClientReadsTheTokenOwnerAndARepository(t *testing.T) {
	var seen http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		switch r.URL.Path {
		case "/user":
			w.Write([]byte(`{"login":"davidadel66","id":1}`))
		case "/repos/davidadel66/evie":
			w.Write([]byte(`{"name":"evie","full_name":"davidadel66/evie","private":false,"default_branch":"master","html_url":"https://github.com/davidadel66/evie","owner":{"login":"davidadel66"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := github.New(server.URL, "ghp_secret", server.Client())
	ctx := context.Background()

	user, err := client.User(ctx)
	if err != nil || user.Login != "davidadel66" {
		t.Fatalf("user = %+v, %v", user, err)
	}
	if seen.Get("Authorization") != "Bearer ghp_secret" || seen.Get("Accept") != "application/vnd.github+json" || seen.Get("X-GitHub-Api-Version") == "" || seen.Get("User-Agent") == "" {
		t.Fatalf("headers = %v", seen)
	}
	repo, err := client.Repository(ctx, "davidadel66", "evie")
	if err != nil || repo.FullName != "davidadel66/evie" || repo.DefaultBranch != "master" || repo.Private || repo.HTMLURL != "https://github.com/davidadel66/evie" {
		t.Fatalf("repository = %+v, %v", repo, err)
	}
}

func TestClientTellsGitHubsRefusalsApart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials"}`))
		case "/repos/someone/private":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"Not Found"}`))
		case "/repos/someone/forbidden":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
		}
	}))
	ctx := context.Background()
	client := github.New(server.URL, "ghp_bad", server.Client())
	if _, err := client.User(ctx); !errors.Is(err, github.ErrUnauthorized) {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := client.Repository(ctx, "someone", "private"); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("no access: %v", err)
	}
	_, err := client.Repository(ctx, "someone", "forbidden")
	if !errors.Is(err, github.ErrForbidden) || err.Error() != "github: forbidden: Resource not accessible by personal access token" {
		t.Fatalf("forbidden: %v", err)
	}
	server.Close()
	if _, err := client.User(ctx); err == nil || errors.Is(err, github.ErrUnauthorized) || errors.Is(err, github.ErrNotFound) {
		t.Fatalf("unreachable: %v", err)
	}
	// Names are path segments: nothing can steer the request elsewhere.
	if _, err := client.Repository(ctx, "a/../../user", "x"); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted name: %v", err)
	}
}

func TestClientCreatesARepositoryOnTheTokensAccount(t *testing.T) {
	var method, path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body: %v", err)
		}
		if body["name"] == "taken" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"Repository creation failed.","errors":[{"message":"name already exists on this account"}]}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"name":"notes","full_name":"davidadel66/notes","private":true,"default_branch":"main","html_url":"https://github.com/davidadel66/notes","clone_url":"https://github.com/davidadel66/notes.git","owner":{"login":"davidadel66"}}`))
	}))
	defer server.Close()
	client := github.New(server.URL, "ghp_secret", server.Client())
	repo, err := client.CreateRepository(context.Background(), "notes", true, false)
	if err != nil || method != http.MethodPost || path != "/user/repos" || body["name"] != "notes" || body["private"] != true || body["auto_init"] != false {
		t.Fatalf("create = %+v, %v; %s %s %v", repo, err, method, path, body)
	}
	if repo.CloneURL != "https://github.com/davidadel66/notes.git" || repo.Owner.Login != "davidadel66" || !repo.Private {
		t.Fatalf("repository = %+v", repo)
	}
	_, err = client.CreateRepository(context.Background(), "taken", true, false)
	if !errors.Is(err, github.ErrUnprocessable) || err.Error() != "github: refused: name already exists on this account" {
		t.Fatalf("taken: %v", err)
	}
	if _, err := client.CreateRepository(context.Background(), "../user", true, false); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("crafted name: %v", err)
	}
}
