package web

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	controller := &fakeTabsController{folders: map[memory.WorkspaceID]string{"lab": folder}}
	server := NewContextServer(nil, nil, nil, controller)
	defer server.Close()
	host := httptest.NewServer(server.Handler())
	defer host.Close()
	fmt.Printf("WORKSPACE_TABS_BROWSER_READY=%s\n", host.URL)
	var stop [1]byte
	_, _ = os.Stdin.Read(stop[:])
}
