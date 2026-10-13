package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workspacetabs"
)

// fakeTabsController serves Workspace folders from a map; an absent ID is
// a missing or archived Workspace.
type fakeTabsController struct {
	fakeContextSessionController
	folders map[memory.WorkspaceID]string
}

func (c *fakeTabsController) ActiveWorkspaceFolder(_ context.Context, id memory.WorkspaceID) (string, error) {
	folder, ok := c.folders[id]
	if !ok {
		return "", fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, id)
	}
	return folder, nil
}

func writeTabFiles(t *testing.T, folder, id string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(folder, workspacetabs.Dir, id)
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func tabsFixture(t *testing.T) (http.Handler, string) {
	t.Helper()
	folder := t.TempDir()
	dir := writeTabFiles(t, folder, "reports", map[string]string{
		"tab.json": `{"title":"Reports","order":20}`, "index.html": "<p>Reports</p>", "app.js": "window.ran = true", ".env": "SECRET=1",
	})
	writeTabFiles(t, folder, "budget", map[string]string{"tab.json": `{"title":"Budget","order":10}`, "index.html": "<p>Budget</p>"})
	writeTabFiles(t, folder, "broken", map[string]string{"tab.json": `{"title":"Broken","extra":1}`, "index.html": "x"})
	if err := os.WriteFile(filepath.Join(folder, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(folder, "secret.txt"), filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	controller := &fakeTabsController{folders: map[memory.WorkspaceID]string{"finance": folder, "general": ""}}
	return NewContextServer(nil, nil, nil, controller).Handler(), folder
}

func TestWorkspaceTabsListShowsTabsInOrderWithProblems(t *testing.T) {
	handler, _ := tabsFixture(t)
	list := func(body string) *httptest.ResponseRecorder {
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, managementRequest("/api/workspaces/tabs", body))
		return result
	}
	result := list(`{"workspaceId":"finance"}`)
	var page struct {
		Tabs []workspacetabs.Tab `json:"tabs"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &page); err != nil || result.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s err=%v", result.Code, result.Body.String(), err)
	}
	var order []string
	for _, tab := range page.Tabs {
		order = append(order, tab.ID+"="+tab.Problem)
	}
	if len(order) != 3 || order[0] != "broken="+page.Tabs[0].Problem || !strings.Contains(page.Tabs[0].Problem, `unknown field "extra"`) ||
		order[1] != "budget=" || order[2] != "reports=" {
		t.Fatalf("tabs = %v", order)
	}
	if result := list(`{"workspaceId":"general"}`); result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"tabs":[]`) {
		t.Fatalf("folderless Workspace status=%d body=%s", result.Code, result.Body.String())
	}
	for body, status := range map[string]int{`{"workspaceId":"archived"}`: http.StatusNotFound, `{}`: http.StatusBadRequest, `{"workspaceId":"finance","x":1}`: http.StatusBadRequest} {
		if result := list(body); result.Code != status {
			t.Errorf("body %s status=%d, want %d", body, result.Code, status)
		}
	}
}

func getTab(handler http.Handler, method, path, host string, header http.Header) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1:6687"+path, nil)
	if host != "" {
		request.Host = host
	}
	for name, values := range header {
		request.Header[name] = values
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

// assertSealed checks the headers every tab response carries; a served file
// may load only from its own folder, an error page runs nothing.
func assertSealed(t *testing.T, path string, result *httptest.ResponseRecorder, folder string) {
	t.Helper()
	policy := result.Header().Get("Content-Security-Policy")
	want := []string{"default-src 'none'", "frame-ancestors 'self'"}
	if folder == "" {
		want = append(want, "sandbox;")
	} else {
		want = append(want, "sandbox allow-scripts;", "connect-src 'none'", "form-action 'none'", "frame-src 'none'",
			"script-src "+folder+" 'unsafe-inline'", "img-src "+folder+" data:")
	}
	for _, part := range want {
		if !strings.Contains(policy, part) {
			t.Errorf("%s policy %q lacks %q", path, policy, part)
		}
	}
	if strings.Contains(policy, "allow-same-origin") || strings.Contains(policy, "'self' 'unsafe-inline'") ||
		result.Header().Get("X-Content-Type-Options") != "nosniff" || result.Header().Get("Cache-Control") != "no-store" ||
		result.Header().Get("X-DNS-Prefetch-Control") != "off" {
		t.Errorf("%s headers = %v", path, result.Header())
	}
}

func TestWorkspaceTabFilesAreSealedAndConfined(t *testing.T) {
	handler, _ := tabsFixture(t)
	reports := "http://127.0.0.1:6687/workspace-tabs/finance/reports/"
	for path, want := range map[string]string{
		"/workspace-tabs/finance/reports/":           "text/html; charset=utf-8|<p>Reports</p>",
		"/workspace-tabs/finance/reports/index.html": "text/html; charset=utf-8|<p>Reports</p>",
		"/workspace-tabs/finance/reports/app.js":     "text/javascript; charset=utf-8|window.ran = true",
	} {
		result := getTab(handler, http.MethodGet, path, "", nil)
		contentType, body, _ := strings.Cut(want, "|")
		if result.Code != http.StatusOK || result.Header().Get("Content-Type") != contentType || result.Body.String() != body {
			t.Errorf("%s status=%d type=%q body=%q", path, result.Code, result.Header().Get("Content-Type"), result.Body.String())
		}
		assertSealed(t, path, result, reports)
	}
	head := getTab(handler, http.MethodHead, "/workspace-tabs/finance/reports/app.js", "", nil)
	partial := getTab(handler, http.MethodGet, "/workspace-tabs/finance/reports/app.js", "", http.Header{"Range": {"bytes=0-5"}})
	if head.Code != http.StatusOK || partial.Code != http.StatusPartialContent || partial.Body.String() != "window" {
		t.Errorf("HEAD status=%d, Range status=%d body=%q", head.Code, partial.Code, partial.Body.String())
	}
	assertSealed(t, "HEAD", head, reports)
	assertSealed(t, "Range", partial, reports)
	if result := getTab(handler, http.MethodGet, "/workspace-tabs/finance/reports", "", nil); result.Code != http.StatusFound || result.Header().Get("Location") != "/workspace-tabs/finance/reports/" {
		t.Errorf("no trailing slash status=%d location=%q", result.Code, result.Header().Get("Location"))
	}
	for path, want := range map[string]string{
		"/workspace-tabs/finance/reports/.env":                     "not part of the tab",
		"/workspace-tabs/finance/reports/escape.txt":               "not part of the tab",
		"/workspace-tabs/finance/reports/missing.js":               "not part of the tab",
		"/workspace-tabs/finance/reports/..%2F..%2Fsecret.txt":     "not part of the tab",
		"/workspace-tabs/finance/reports/%2e%2e/%2e%2e/secret.txt": "not part of the tab",
		"/workspace-tabs/finance/missing/":                         "not part of the tab",
		"/workspace-tabs/finance/broken/":                          `This tab has a problem: tab.json is not valid: json: unknown field "extra"`,
		"/workspace-tabs/archived/reports/":                        "not available",
		"/workspace-tabs/general/reports/":                         "not part of the tab",
		"/workspace-tabs/fin%2Fance/reports/":                      "not part of the tab",
	} {
		result := getTab(handler, http.MethodGet, path, "", nil)
		if result.Code != http.StatusNotFound || !strings.Contains(result.Body.String(), want) || strings.Contains(result.Body.String(), "outside") {
			t.Errorf("%s status=%d body=%q", path, result.Code, result.Body.String())
		}
		assertSealed(t, path, result, "")
	}
	for _, host := range []string{"evil.example:6687", "127.0.0.1:6687;script-src *", "localhost:x"} {
		if result := getTab(handler, http.MethodGet, "/workspace-tabs/finance/reports/", host, nil); result.Code != http.StatusForbidden {
			t.Errorf("host %q status=%d", host, result.Code)
		}
	}
}

// The trailing-slash redirect builds its Location only from checked IDs, so
// Evie's origin cannot bounce a browser to another site.
func TestWorkspaceTabRedirectStaysOnEvie(t *testing.T) {
	handler, _ := tabsFixture(t)
	for _, path := range []string{"/workspace-tabs/..%2F..%2F%5Cevil.com/x", "/workspace-tabs/finance/%5Cevil.com", "/workspace-tabs/a%2F%2Fevil.com/reports"} {
		result := getTab(handler, http.MethodGet, path, "", nil)
		if result.Code == http.StatusFound || result.Header().Get("Location") != "" {
			t.Errorf("%s status=%d location=%q", path, result.Code, result.Header().Get("Location"))
		}
	}
}

func TestWorkspaceTabsListRefusesAnOverfullTabsFolder(t *testing.T) {
	folder := t.TempDir()
	for i := range 257 {
		if err := os.MkdirAll(filepath.Join(folder, workspacetabs.Dir, fmt.Sprintf("t%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	handler := NewContextServer(nil, nil, nil, &fakeTabsController{folders: map[memory.WorkspaceID]string{"crowded": folder}}).Handler()
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, managementRequest("/api/workspaces/tabs", `{"workspaceId":"crowded"}`))
	if result.Code != http.StatusUnprocessableEntity || !strings.Contains(result.Body.String(), "over 256 entries") {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
}

// Evie's own page allows frames only from itself, so a tab cannot navigate
// its frame to another site.
func TestAppShellFramesOnlyEvie(t *testing.T) {
	handler, _ := tabsFixture(t)
	result := getTab(handler, http.MethodGet, "/", "", nil)
	if result.Code != http.StatusOK || result.Header().Get("Content-Security-Policy") != "frame-src 'self'" {
		t.Fatalf("status=%d policy=%q", result.Code, result.Header().Get("Content-Security-Policy"))
	}
}

// A tab's page has an opaque origin, so its requests carry Origin: null;
// the management-route guard refuses them before any handler runs.
func TestManagementRoutesRefuseOpaqueOriginRequests(t *testing.T) {
	handler, _ := tabsFixture(t)
	request := managementRequest("/api/workspaces/tabs", `{"workspaceId":"finance"}`)
	request.Header.Set("Origin", "null")
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	if result.Code != http.StatusForbidden || !strings.Contains(result.Body.String(), "cross-origin request rejected") {
		t.Fatalf("opaque origin status=%d body=%s", result.Code, result.Body.String())
	}
}

// storeTabsController reads Workspace folders from a real store.
type storeTabsController struct {
	fakeContextSessionController
	store *eviedb.Store
}

func (c *storeTabsController) ActiveWorkspaceFolder(ctx context.Context, id memory.WorkspaceID) (string, error) {
	return c.store.ActiveWorkspaceFolder(ctx, id)
}

func TestWorkspaceTabsAgainstARealStore(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Finance")
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	writeTabFiles(t, folder, "reports", map[string]string{"tab.json": `{"title":"Reports"}`, "index.html": "<p>Reports</p>"})
	if _, err := store.SetWorkspaceFolder(ctx, workspace.ID, 0, folder); err != nil {
		t.Fatal(err)
	}
	handler := NewContextServer(nil, nil, nil, &storeTabsController{store: store}).Handler()
	list := func() *httptest.ResponseRecorder {
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, managementRequest("/api/workspaces/tabs", `{"workspaceId":"`+string(workspace.ID)+`"}`))
		return result
	}
	page := "/workspace-tabs/" + string(workspace.ID) + "/reports/"
	if result := list(); result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"title":"Reports"`) {
		t.Fatalf("list status=%d body=%s", result.Code, result.Body.String())
	}
	if result := getTab(handler, http.MethodGet, page, "", nil); result.Code != http.StatusOK || result.Body.String() != "<p>Reports</p>" {
		t.Fatalf("page status=%d body=%q", result.Code, result.Body.String())
	}
	if _, err := store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if result := list(); result.Code != http.StatusNotFound {
		t.Fatalf("archived list status=%d", result.Code)
	}
	if result := getTab(handler, http.MethodGet, page, "", nil); result.Code != http.StatusNotFound {
		t.Fatalf("archived page status=%d", result.Code)
	}
}
