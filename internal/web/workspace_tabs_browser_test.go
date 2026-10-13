package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workspacetabs"
)

// TestWorkspaceTabsBrowserFixture serves Evie with one Workspace whose folder
// holds the hostile test tab, for scripts/workspace-tabs-browser.mjs. It
// prints the server URL and waits for stdin to close.
func TestWorkspaceTabsBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_WORKSPACE_TABS_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in workspace tabs browser fixture")
	}
	folder := t.TempDir()
	source := filepath.Join("testdata", "workspace-tabs", "hostile")
	target := filepath.Join(folder, workspacetabs.Dir, "hostile")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tab.json", "index.html", "app.js"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Benign tabs with long titles, so a phone-width header needs More.
	for i, title := range []string{"Evaluation metrics", "Reading map", "Budget"} {
		dir := filepath.Join(folder, workspacetabs.Dir, fmt.Sprintf("plain-%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf(`{"title":%q,"order":%d}`, title, i+2)
		if err := os.WriteFile(filepath.Join(dir, "tab.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>"+title+"</h1>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	controller := &fakeTabsController{folders: map[memory.WorkspaceID]string{"lab": folder}}
	controller.snapshot = ContextSessionSnapshot{Workspaces: []memory.Workspace{{
		ID: "lab", DisplayName: "Lab", State: memory.WorkspaceActive, CurrentRevisionID: "revision-1",
		Folder: memory.WorkspaceFolder{Path: folder, Revision: 1},
	}}}
	server := NewContextServer(nil, nil, nil, controller)
	defer server.Close()
	// /leak stands in for the outside world: every request that reaches it
	// is counted, and /fixture/leaks reports the count to the browser check.
	var leaks atomic.Int64
	evie := server.Handler()
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/leak"):
			leaks.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/fixture/leaks":
			fmt.Fprint(w, leaks.Load())
		default:
			evie.ServeHTTP(w, r)
		}
	}))
	defer host.Close()
	fmt.Printf("WORKSPACE_TABS_BROWSER_READY=%s\n", host.URL)
	var stop [1]byte
	_, _ = os.Stdin.Read(stop[:])
}
