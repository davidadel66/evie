package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

type workspaceSetupNoModelClient struct{}

func (workspaceSetupNoModelClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	return openrouter.ChatResponse{}, errors.New("model requests are disabled in the browser fixture")
}

// Opt-in real HTTP/SQLite fixture. Reopening preserves exactly the fixture data;
// no configured Evie database, owner folder, or provider is contacted.
func TestWorkspaceSetupBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_SETUP_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in Workspace creation, archive and Settings browser demonstration")
	}
	output := os.Getenv("EVIE_SETUP_BROWSER_OUTPUT")
	if !filepath.IsAbs(output) {
		t.Fatal("EVIE_SETUP_BROWSER_OUTPUT must be an absolute NEW directory")
	}
	reopen := os.Getenv("EVIE_SETUP_BROWSER_REOPEN") == "1"
	if reopen {
		if _, err := os.Stat(filepath.Join(output, "ready.json")); err != nil {
			t.Fatalf("reopening requires a previous browser fixture: %v", err)
		}
	} else if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	database := filepath.Join(output, "browser.db")
	db, err := eviedb.OpenDBAt(database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	standard, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if !reopen {
		workspace, err := store.RegisterWorkspace(ctx, "General")
		if err != nil {
			t.Fatal(err)
		}
		for _, title := range []string{"Plan the week", "Explore reinforcement learning"} {
			session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standard.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := store.AcquireTurnLease(ctx, session.ID, "fixture-seed", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
				Type: memory.EventUserMessage, Role: memory.RoleUser, Content: title,
			}); err != nil {
				t.Fatal(err)
			}
			if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(output, "existing-folder"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	controller := newWebContextSessionController(store, manager, func(session memory.Session, composition plugins.ResolvedComposition) (*agent.Session, error) {
		holder := memory.LeaseHolderID("browser-" + session.ID)
		return agent.NewWithToolset(workspaceSetupNoModelClient{}, profile, store.BindHistory(session.ID, holder), session.ScopeContext(),
			store.BindTurnOwner(session.ID, holder), composition.Toolset), nil
	})
	server := web.NewContextServer(nil, manager, store, controller)
	handler := server.Handler()
	snapshot, err := controller.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"database": database, "pid": os.Getpid(), "existingFolder": filepath.Join(output, "existing-folder")}
	if len(snapshot.Sessions) > 0 {
		session := snapshot.Sessions[0]
		body, _ := json.Marshal(map[string]any{"sessionId": session.ID})
		request := httptest.NewRequest(http.MethodPost, "http://localhost/api/context-sessions/select", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		selected := httptest.NewRecorder()
		handler.ServeHTTP(selected, request)
		if selected.Code != http.StatusOK {
			t.Fatalf("select: %d %s", selected.Code, selected.Body)
		}
		metadata["sessionId"], metadata["workspaceId"] = session.ID, session.WorkspaceID
	}
	host := httptest.NewServer(handler)
	defer func() { server.Close(); host.Close() }()
	metadata["url"] = host.URL
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "ready.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("SETUP_BROWSER_READY=" + string(raw))
	if os.Getenv("EVIE_SETUP_BROWSER_VALIDATE_ONLY") == "1" {
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
