package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

type resumeOnlyCompositionManager struct {
	*plugins.Manager
}

func (m resumeOnlyCompositionManager) ResolvePresetContext(context.Context, plugins.PresetID) (plugins.ResolvedComposition, error) {
	return plugins.ResolvedComposition{}, errors.New("current Standard preset is invalid")
}

func TestWebContextControllerCreatesAndResumesPinnedWorkspaceCompositionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	workspace, err := store.RegisterWorkspace(context.Background(), "Cairo's Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	var createdCompositions []plugins.ResolvedComposition
	var clients []*resumeCaptureClient
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	controller := newWebContextSessionController(store, manager, func(
		session memory.Session, composition plugins.ResolvedComposition,
	) (*agent.Session, error) {
		if session.ScopeContext().WorkspaceID != workspace.ID {
			t.Fatalf("factory session scope=%+v", session.ScopeContext())
		}
		createdCompositions = append(createdCompositions, composition)
		client := &resumeCaptureClient{}
		clients = append(clients, client)
		holder := memory.LeaseHolderID("web-" + session.ID)
		return agent.NewWithToolset(
			client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(),
			store.BindTurnOwner(session.ID, holder), composition.Toolset,
		), nil
	})

	opened, err := controller.SelectSession(context.Background(), web.ContextSessionSelection{
		WorkspaceID: workspace.ID, WorkspaceRevision: workspace.CurrentRevisionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Session.WorkspaceID != workspace.ID || opened.Session.WorkspaceRevisionSnapshot != workspace.CurrentRevisionID ||
		len(createdCompositions) != 1 {
		t.Fatalf("opened=%+v compositions=%d", opened.Session, len(createdCompositions))
	}
	if err := opened.Agent.Send(context.Background(), "prepare dinner", &replEvents{out: io.Discard}, nil); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.GetCompositionReceipt(context.Background(), opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, createdCompositions[0].Receipt) || receipt.Preset.ID != string(plugins.StandardPresetID) {
		t.Fatalf("stored receipt=%+v composition=%+v", receipt, createdCompositions[0].Receipt)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = eviedb.NewStore(db)
	controller = newWebContextSessionController(store, resumeOnlyCompositionManager{Manager: manager}, func(
		session memory.Session, composition plugins.ResolvedComposition,
	) (*agent.Session, error) {
		createdCompositions = append(createdCompositions, composition)
		client := &resumeCaptureClient{}
		clients = append(clients, client)
		holder := memory.LeaseHolderID("web-restarted-" + session.ID)
		return agent.NewWithToolset(
			client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(),
			store.BindTurnOwner(session.ID, holder), composition.Toolset,
		), nil
	})
	resumed, err := controller.SelectSession(context.Background(), web.ContextSessionSelection{SessionID: opened.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Session.ID != opened.Session.ID || resumed.Session.ScopeContext() != opened.Session.ScopeContext() || len(createdCompositions) != 2 ||
		!reflect.DeepEqual(createdCompositions[1].Receipt, receipt) {
		t.Fatalf("resumed=%+v composition count=%d", resumed.Session, len(createdCompositions))
	}
	if err := resumed.Agent.Send(context.Background(), "continue dinner", &replEvents{out: io.Discard}, nil); err != nil {
		t.Fatal(err)
	}
	if len(clients) != 2 || len(clients[1].requests) != 1 {
		t.Fatalf("restarted clients=%+v", clients)
	}
	messages := clients[1].requests[0].Messages
	if len(messages) < 4 || messages[1].Content != "prepare dinner" ||
		messages[2].Content != "resumed" || messages[len(messages)-1].Content != "continue dinner" {
		t.Fatalf("resumed provider history=%+v", messages)
	}
	snapshot, err := controller.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || len(snapshot.Sessions) != 1 || snapshot.Sessions[0].WorkspaceID != workspace.ID {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestWebContextControllerPreservesProjectAndUnscopedCreation(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	project, err := store.RegisterProject(context.Background(), "Evie", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	controller := newWebContextSessionController(store, manager, func(memory.Session, plugins.ResolvedComposition) (*agent.Session, error) {
		return &agent.Session{}, nil
	})
	projectOpened, err := controller.SelectSession(context.Background(), web.ContextSessionSelection{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	unscoped, err := controller.SelectSession(context.Background(), web.ContextSessionSelection{Unscoped: true})
	if err != nil {
		t.Fatal(err)
	}
	if projectOpened.Session.ProjectID != project.ID || projectOpened.Session.WorkspaceID != "" ||
		unscoped.Session.ProjectID != "" || unscoped.Session.WorkspaceID != "" {
		t.Fatalf("project=%+v unscoped=%+v", projectOpened.Session, unscoped.Session)
	}
}
