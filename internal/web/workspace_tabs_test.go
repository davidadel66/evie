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

func TestWorkspaceTabFilesAreSealedAndConfined(t *testing.T) {
	handler, _ := tabsFixture(t)
	get := func(path string, host string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		if host != "" {
			request.Host = host
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		return result
	}
	sealed := func(path string, result *httptest.ResponseRecorder) {
		t.Helper()
		policy := result.Header().Get("Content-Security-Policy")
		for _, want := range []string{"sandbox allow-scripts;", "connect-src 'none'", "frame-ancestors 'self'", "form-action 'none'"} {
			if !strings.Contains(policy, want) {
				t.Errorf("%s policy %q lacks %q", path, policy, want)
			}
		}
		if strings.Contains(policy, "allow-same-origin") || result.Header().Get("X-Content-Type-Options") != "nosniff" || result.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s headers = %v", path, result.Header())
		}
	}
	for path, want := range map[string]string{
		"/workspace-tabs/finance/reports/":           "text/html; charset=utf-8|<p>Reports</p>",
		"/workspace-tabs/finance/reports/index.html": "text/html; charset=utf-8|<p>Reports</p>",
		"/workspace-tabs/finance/reports/app.js":     "text/javascript; charset=utf-8|window.ran = true",
	} {
		result := get(path, "")
		contentType, body, _ := strings.Cut(want, "|")
		if result.Code != http.StatusOK || result.Header().Get("Content-Type") != contentType || result.Body.String() != body {
			t.Errorf("%s status=%d type=%q body=%q", path, result.Code, result.Header().Get("Content-Type"), result.Body.String())
		}
		sealed(path, result)
	}
	if result := get("/workspace-tabs/finance/reports", ""); result.Code != http.StatusFound || result.Header().Get("Location") != "/workspace-tabs/finance/reports/" {
		t.Errorf("no trailing slash status=%d location=%q", result.Code, result.Header().Get("Location"))
	}
	for path, want := range map[string]string{
		"/workspace-tabs/finance/reports/.env":       "not part of the tab",
		"/workspace-tabs/finance/reports/escape.txt": "not part of the tab",
		"/workspace-tabs/finance/reports/missing.js": "not part of the tab",
		"/workspace-tabs/finance/missing/":           "not part of the tab",
		"/workspace-tabs/finance/broken/":            `This tab has a problem: tab.json is not valid: json: unknown field "extra"`,
		"/workspace-tabs/archived/reports/":          "not available",
		"/workspace-tabs/general/reports/":           "not part of the tab",
	} {
		result := get(path, "")
		if result.Code != http.StatusNotFound || !strings.Contains(result.Body.String(), want) || strings.Contains(result.Body.String(), "outside") {
			t.Errorf("%s status=%d body=%q", path, result.Code, result.Body.String())
		}
		sealed(path, result)
	}
	if result := get("/workspace-tabs/finance/reports/", "evil.example:6687"); result.Code != http.StatusForbidden {
		t.Errorf("foreign host status=%d", result.Code)
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
