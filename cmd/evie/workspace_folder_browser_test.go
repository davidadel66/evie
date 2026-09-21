package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

// Opt-in real-HTTP demonstration, using a new disposable SQLite database and
// scripted model responses. No user folders or network providers are touched.
func TestWorkspaceFolderBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_FOLDER_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in folder and inspector browser demonstration")
	}
	output := os.Getenv("EVIE_FOLDER_BROWSER_OUTPUT")
	if !filepath.IsAbs(output) {
		t.Fatal("EVIE_FOLDER_BROWSER_OUTPUT must be an absolute NEW directory")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	t.Setenv("EVIE_REASONING", "")
	f := &stage5BrowserFixture{t: t, ctx: context.Background(), path: filepath.Join(output, "browser.db"), output: output}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.db.Close()
	f.store = eviedb.NewStore(f.db)
	manager := sessionCompositionManager(t)
	composition, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := f.store.RegisterWorkspace(f.ctx, "Local project demo")
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	claim := f.remember(source, "study_format", "Study format", "Use worked examples.", memory.ValidTime{}, memory.CardinalityOne)
	f.refresh()
	f.send(reader, "Which study format do I prefer?", true, stage5BrowserCheckedText("You prefer worked examples.", string(claim.ClaimID)))
	project := filepath.Join(output, "project")
	if err := os.MkdirAll(filepath.Join(project, "notes"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(project, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# Local project\n\nFiles, changes, and memory in one pane.\n")
	write("notes/lecture.md", "# Reinforcement learning\n\nAn agent learns by interacting with an environment.\n\n- Observe the state\n- Choose an action\n- Receive a reward\n")
	write("main.go", "package main\n\nfunc main() { println(\"Hello\") }\n")
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", project}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	run("init", "-q")
	run("add", ".")
	run("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "initial")
	write("main.go", "package main\n\nfunc main() { println(\"Hello, Evie\") }\n")
	write("todo.md", "# Next steps\n\nTry the file tree toggle.\n")
	if os.Getenv("EVIE_INSTRUCTIONS_BROWSER_FIXTURE") == "1" {
		write("AGENTS.md", "# Project guide\n\nUse concise answers. Run the project checks before handoff.\n")
		if _, err := f.store.SetWorkspaceFolder(f.ctx, workspace.ID, 0, project); err != nil {
			t.Fatal(err)
		}
		f.send(reader, "Which repository instructions are active?", false, stage5BrowserText("The project guide asks for concise answers and project checks."))
	}
	controller := newWebContextSessionController(f.store, manager, func(record memory.Session, _ plugins.ResolvedComposition) (*agent.Session, error) {
		return f.runtime(record, &stage5BrowserClient{fixture: f}, true), nil
	})
	server := web.NewContextMemoryServer(nil, nil, nil, controller, f.store)
	handler := server.Handler()
	body, _ := json.Marshal(map[string]any{"sessionId": reader.ID})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/context-sessions/select", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, request)
	if selected.Code != http.StatusOK {
		t.Fatalf("select: %d %s", selected.Code, selected.Body)
	}
	host := httptest.NewServer(handler)
	defer func() { server.Close(); host.Close() }()
	metadata := map[string]any{"url": host.URL, "workspaceId": workspace.ID, "sessionId": reader.ID, "project": project, "database": f.path, "pid": os.Getpid()}
	f.write("ready.json", metadata)
	raw, _ := json.Marshal(metadata)
	fmt.Println("FOLDER_BROWSER_READY=" + string(raw))
	if os.Getenv("EVIE_FOLDER_BROWSER_VALIDATE_ONLY") == "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case <-time.After(time.Hour):
	}
}
