package changes_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/github"
)

// changes.spec.md story 2a: a local Repository is published to GitHub from
// Settings: a new repository on the owner's account, origin set, and only
// the base branch pushed. The token never reaches git config, the remote
// URL or any output.

const token = "ghp_publishsecret"

// fakeGitHub answers as the token's owner and creates each repository as a
// bare repository on disk. Its clone URLs are real github.com URLs; a
// test-only global git config rewrites each one to its bare repository, and
// every other github.com URL to a path that does not exist, so no test
// reaches the real GitHub.
type fakeGitHub struct {
	t       *testing.T
	root    string
	config  string
	mu      sync.Mutex
	created map[string]map[string]any
	// unpushable creates repositories without their bare repository, so a
	// push to one fails until the test makes it.
	unpushable bool
	// pulls are the pull requests opened on each repository.
	pulls map[string][]map[string]any
	// openFails answers every request to open a pull request with this
	// status when set.
	openFails int
	// pullDraft and notMergeable describe every pull request read alone;
	// mergeFails answers every merge with this status when set.
	pullDraft, notMergeable bool
	mergeFails              int
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *httptest.Server) {
	f := &fakeGitHub{t: t, root: t.TempDir(), created: map[string]map[string]any{}, pulls: map[string][]map[string]any{}}
	f.config = filepath.Join(f.root, "gitconfig")
	f.rewrite("https://github.com/", filepath.Join(f.root, "no-network")+"/")
	t.Setenv("GIT_CONFIG_GLOBAL", f.config)
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fakeGitHub) rewrite(from, to string) {
	file, err := os.OpenFile(f.config, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		f.t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("[url \"" + to + "\"]\n\tinsteadOf = " + from + "\n"); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeGitHub) bare(name string) string { return filepath.Join(f.root, name+".git") }

// init makes a repository's bare repository; withCommit gives it GitHub's
// first commit, as auto_init does.
func (f *fakeGitHub) init(name string, withCommit bool) {
	gitIn(f.t, f.root, "init", "-q", "--bare", "-b", "main", f.bare(name))
	if !withCommit {
		return
	}
	seed := f.t.TempDir()
	gitIn(f.t, seed, "init", "-q", "-b", "main")
	gitIn(f.t, seed, "-c", "user.name=GitHub", "-c", "user.email=noreply@github.com", "commit", "-q", "--allow-empty", "-m", "Initial commit")
	gitIn(f.t, seed, "push", "-q", f.bare(name), "main")
}

// seed is a repository the owner already has on GitHub.
func (f *fakeGitHub) seed(name string, private bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.init(name, true)
	f.rewrite(cloneURL(name), f.bare(name))
	f.created[name] = f.repo(name, private)
}

func cloneURL(name string) string { return "https://github.com/davidadel66/" + name + ".git" }

func (f *fakeGitHub) repo(name string, private bool) map[string]any {
	return map[string]any{"name": name, "full_name": "davidadel66/" + name, "private": private, "default_branch": "main",
		"html_url": "https://github.com/davidadel66/" + name, "clone_url": cloneURL(name), "owner": map[string]any{"login": "davidadel66"}}
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.URL.Path == "/user":
		w.Write([]byte(`{"login":"davidadel66"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/user/repos":
		list := []map[string]any{}
		for _, repo := range f.created {
			list = append(list, repo)
		}
		json.NewEncoder(w).Encode(list)
	case r.Method == http.MethodPost && r.URL.Path == "/user/repos":
		var body struct {
			Name     string `json:"name"`
			Private  bool   `json:"private"`
			AutoInit bool   `json:"auto_init"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if _, taken := f.created[body.Name]; taken || body.Name == "taken" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"Repository creation failed.","errors":[{"message":"name already exists on this account"}]}`))
			return
		}
		if !f.unpushable {
			f.init(body.Name, body.AutoInit)
		}
		f.rewrite(cloneURL(body.Name), f.bare(body.Name))
		repo := f.repo(body.Name, body.Private)
		repo["auto_init"] = body.AutoInit
		f.created[body.Name] = repo
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(repo)
	case strings.HasPrefix(r.URL.Path, "/repos/davidadel66/") && strings.Contains(r.URL.Path, "/pulls/"):
		f.servePull(w, r)
	case strings.HasSuffix(r.URL.Path, "/pulls") && strings.HasPrefix(r.URL.Path, "/repos/davidadel66/"):
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/davidadel66/"), "/pulls")
		if r.Method == http.MethodGet {
			// state=open by default, or state=all for one head branch.
			listed := []map[string]any{}
			head := strings.TrimPrefix(r.URL.Query().Get("head"), "davidadel66:")
			for _, p := range f.pulls[name] {
				if r.URL.Query().Get("state") == "all" && p["head"].(map[string]any)["ref"] == head || r.URL.Query().Get("state") != "all" && p["state"] == "open" {
					listed = append(listed, p)
				}
			}
			json.NewEncoder(w).Encode(listed)
			return
		}
		if f.openFails != 0 {
			w.WriteHeader(f.openFails)
			w.Write([]byte(`{"message":"refused by the fake"}`))
			return
		}
		var body struct{ Title, Head, Base, Body string }
		json.NewDecoder(r.Body).Decode(&body)
		number := len(f.pulls[name]) + 1
		pull := map[string]any{"number": number, "state": "open", "title": body.Title, "body": body.Body,
			"html_url": fmt.Sprintf("https://github.com/davidadel66/%s/pull/%d", name, number),
			"head":     map[string]any{"ref": body.Head, "sha": head(f.t, f.bare(name), "refs/heads/"+body.Head)}, "base": map[string]any{"ref": body.Base}, "user": map[string]any{"login": "davidadel66"}}
		f.pulls[name] = append(f.pulls[name], pull)
		// GitHub keeps every pull request's head at refs/pull/<number>/head.
		gitIn(f.t, f.bare(name), "update-ref", fmt.Sprintf("refs/pull/%d/head", number), pull["head"].(map[string]any)["sha"].(string))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(pull)
	case strings.HasPrefix(strings.ToLower(r.URL.Path), "/repos/davidadel66/"):
		// GitHub matches owner and repository names in any case.
		name := strings.ToLower(r.URL.Path[len("/repos/davidadel66/"):])
		if repo, ok := f.created[name]; ok {
			json.NewEncoder(w).Encode(repo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// servePull reads, merges or closes one pull request. A merge is GitHub's
// rebase merge of a branch already on its base, so the base moves to the
// pull request's head.
func (f *fakeGitHub) servePull(w http.ResponseWriter, r *http.Request) {
	name, tail, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/repos/davidadel66/"), "/pulls/")
	numberText, merge := strings.CutSuffix(tail, "/merge")
	number, _ := strconv.Atoi(numberText)
	var pull map[string]any
	for _, p := range f.pulls[name] {
		if p["number"] == number {
			pull = p
		}
	}
	if pull == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	headSHA := pull["head"].(map[string]any)["sha"].(string)
	switch {
	case merge && r.Method == http.MethodPut:
		var body struct {
			SHA         string `json:"sha"`
			MergeMethod string `json:"merge_method"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch {
		case f.mergeFails != 0:
			w.WriteHeader(f.mergeFails)
			w.Write([]byte(`{"message":"Rebase merges are not allowed on this repository."}`))
			return
		case body.SHA != headSHA:
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"message":"Head branch was modified. Review and try the merge again."}`))
			return
		case body.MergeMethod != "rebase":
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		gitIn(f.t, f.bare(name), "update-ref", "refs/heads/"+pull["base"].(map[string]any)["ref"].(string), headSHA)
		pull["state"], pull["merged"] = "closed", true
		json.NewEncoder(w).Encode(map[string]any{"sha": headSHA, "merged": true})
	case r.Method == http.MethodPatch:
		pull["state"] = "closed"
		json.NewEncoder(w).Encode(pull)
	default:
		read := map[string]any{}
		for k, v := range pull {
			read[k] = v
		}
		read["draft"], read["mergeable"] = f.pullDraft, !f.notMergeable
		json.NewEncoder(w).Encode(read)
	}
}

func publishService(server *httptest.Server) *changes.Service {
	return changes.NewService(changes.Config{GitHub: github.New(server.URL, token, server.Client()), Token: token, AllowFileRemotes: true})
}

func refs(t *testing.T, bare string) string {
	t.Helper()
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname) %(objectname)")
	cmd.Dir = bare
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("refs: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func head(t *testing.T, repo, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

func noToken(t *testing.T, repo string, texts ...string) {
	t.Helper()
	config, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range append(texts, string(config)) {
		if strings.Contains(text, token) || strings.Contains(strings.ToLower(text), "extraheader") {
			t.Fatalf("the token reached %q", text)
		}
	}
}

func TestPublishCreatesTheRepositorySetsOriginAndPushesOnlyTheBaseBranch(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	repo := repository(t, "main")
	gitIn(t, repo, "branch", "experiment")
	r, err := publishService(server).Publish(ctx, changes.Publish{Folder: repo, Name: "notes", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	created := fake.created["notes"]
	if created == nil || created["private"] != true || created["auto_init"] != false {
		t.Fatalf("created = %v", created)
	}
	if r.Kind != changes.GitHub || r.Owner != "davidadel66" || r.Name != "notes" || r.RemoteURL != cloneURL("notes") ||
		r.GitHub == nil || r.GitHub.State != changes.Connected || r.GitHub.Private == nil || !*r.GitHub.Private {
		t.Fatalf("published = %+v %+v", r, r.GitHub)
	}
	if got, want := refs(t, fake.bare("notes")), "refs/heads/main "+head(t, repo, "main"); got != want {
		t.Fatalf("pushed %q, want only %q", got, want)
	}
	noToken(t, repo, r.RemoteURL)
}

func TestPublishRefusesWhatItCannotPublish(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	service := publishService(server)

	plain := t.TempDir()
	if _, err := service.Publish(ctx, changes.Publish{Folder: plain, Name: "x", Private: true}); !errors.Is(err, changes.ErrNotPublishable) {
		t.Fatalf("not a repository: %v", err)
	}
	elsewhere := repository(t, "main")
	gitIn(t, elsewhere, "remote", "add", "origin", "https://gitlab.com/someone/notes.git")
	if _, err := service.Publish(ctx, changes.Publish{Folder: elsewhere, Name: "notes", Private: true}); !errors.Is(err, changes.ErrNotPublishable) || !strings.Contains(err.Error(), "gitlab.com") {
		t.Fatalf("origin elsewhere: %v", err)
	}
	repo := repository(t, "main")
	if _, err := service.Publish(ctx, changes.Publish{Folder: repo, Name: "../user", Private: true}); !errors.Is(err, github.ErrInvalidName) {
		t.Fatalf("invalid name: %v", err)
	}
	if _, err := service.Publish(ctx, changes.Publish{Folder: repo, Name: "taken", Private: true}); !errors.Is(err, github.ErrUnprocessable) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("taken: %v", err)
	}
	if _, err := changes.NewService(changes.Config{}).Publish(ctx, changes.Publish{Folder: repo, Name: "notes", Private: true}); !errors.Is(err, changes.ErrNoToken) {
		t.Fatalf("no token: %v", err)
	}
	detached := repository(t, "main")
	gitIn(t, detached, "checkout", "-q", "--detach")
	if _, err := service.Publish(ctx, changes.Publish{Folder: detached, Name: "detached", Private: true}); !errors.Is(err, changes.ErrNotPublishable) {
		t.Fatalf("detached HEAD: %v", err)
	}
	// A refused publish leaves the folder as it was.
	if r, _ := service.Repository(ctx, repo); r.Kind != changes.Local || r.RemoteURL != "" {
		t.Fatalf("after refusals = %+v", r)
	}
	if len(fake.created) != 0 {
		t.Fatalf("refusals created %v", fake.created)
	}
}

func TestAPublishWhosePushFailedIsRetriedWithoutASecondRepository(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	service := publishService(server)
	repo := repository(t, "main")
	fake.unpushable = true
	_, err := service.Publish(ctx, changes.Publish{Folder: repo, Name: "notes", Private: true})
	if !errors.Is(err, changes.ErrPushFailed) || !strings.Contains(err.Error(), "https://github.com/davidadel66/notes") {
		t.Fatalf("failed push: %v", err)
	}
	noToken(t, repo, err.Error())
	// GitHub now has the repository and origin points at it; publishing
	// again pushes to it instead of creating another.
	if r, _ := service.Repository(ctx, repo); r.Kind != changes.GitHub || r.RemoteURL != cloneURL("notes") {
		t.Fatalf("after the failed push = %+v", r)
	}
	gitIn(t, fake.root, "init", "-q", "--bare", "-b", "main", fake.bare("notes"))
	r, err := service.Publish(ctx, changes.Publish{Folder: repo, Name: "notes", Private: true})
	if err != nil || r.Name != "notes" {
		t.Fatalf("retry = %+v, %v", r, err)
	}
	if got, want := refs(t, fake.bare("notes")), "refs/heads/main "+head(t, repo, "main"); got != want {
		t.Fatalf("retried push %q, want %q", got, want)
	}
	if len(fake.created) != 1 {
		t.Fatalf("created %d repositories", len(fake.created))
	}
}
