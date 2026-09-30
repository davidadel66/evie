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

type fixtureModelCatalog struct{}

func (fixtureModelCatalog) ListChatModels(context.Context) ([]openrouter.Model, error) {
	return []openrouter.Model{{ID: "openai/test", Name: "GPT Test", Provider: "openai"}, {ID: "anthropic/test", Name: "Claude Test", Provider: "anthropic"}, {ID: "broken/test", Name: "Unavailable Test", Provider: "broken"}}, nil
}

func modelFixtureController(t *testing.T, store *eviedb.Store, client *resumeCaptureClient) *webContextSessionController {
	t.Helper()
	c := newWebContextSessionController(store, sessionCompositionManager(t), nil)
	c.defaultModel, c.modelClient = "openai/test", fixtureModelCatalog{}
	c.newModelAgent = func(_ context.Context, session memory.Session, composition plugins.ResolvedComposition, model string, revision int64) (*agent.Session, error) {
		if model == "broken/test" {
			return nil, errors.New("provider metadata unavailable")
		}
		profile, err := openrouter.NewExplicitContextProfile(model, 300000, 200000, 12000)
		if err != nil {
			return nil, err
		}
		holder := memory.LeaseHolderID("model-test-" + session.ID)
		return agent.NewWithToolset(client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwnerWithModelRevision(session.ID, holder, revision), composition.Toolset), nil
	}
	return c
}

func TestChatModelSelectionRetainsHistoryPersistsAndFencesOldRuntime(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := eviedb.NewStore(db)
	client := &resumeCaptureClient{}
	c := modelFixtureController(t, store, client)
	opened, err := c.SelectSession(ctx, web.ContextSessionSelection{Unscoped: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Agent.Send(ctx, "remember the first message", &replEvents{out: io.Discard}, nil); err != nil {
		t.Fatal(err)
	}
	receipt, err := store.GetCompositionReceipt(ctx, opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	staleRuntime, err := c.SelectSession(ctx, web.ContextSessionSelection{SessionID: opened.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := c.SelectModel(ctx, opened.Session.ID, 0, "anthropic/test")
	if err != nil {
		t.Fatal(err)
	}
	if changed.Session.ID != opened.Session.ID || changed.ModelRevision != 1 {
		t.Fatalf("changed=%+v", changed)
	}
	if err := staleRuntime.Agent.Send(ctx, "stale message", &replEvents{out: io.Discard}, nil); !errors.Is(err, agent.ErrLeaseConflict) {
		t.Fatalf("stale runtime send=%v", err)
	}
	if err := changed.Agent.Send(ctx, "second message", &replEvents{out: io.Discard}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 || client.requests[1].Model != "anthropic/test" {
		t.Fatalf("requests=%+v", client.requests)
	}
	found := false
	for _, message := range client.requests[1].Messages {
		if message.Content == "remember the first message" {
			found = true
		}
	}
	if !found {
		t.Fatal("model switch lost history")
	}
	for _, candidate := range []string{"broken/test", "unknown/model"} {
		if _, err := c.SelectModel(ctx, opened.Session.ID, 1, candidate); err == nil {
			t.Fatalf("accepted %s", candidate)
		}
	}
	if _, err := c.SelectModel(ctx, opened.Session.ID, 0, "openai/test"); !errors.Is(err, eviedb.ErrSessionModelChanged) {
		t.Fatalf("stale selection=%v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	c = modelFixtureController(t, store, client)
	resumed, err := c.SelectSession(ctx, web.ContextSessionSelection{SessionID: opened.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Agent.ContextProfile().ConfiguredModel != "anthropic/test" || resumed.ModelRevision != 1 {
		t.Fatalf("resumed=%+v", resumed)
	}
	restoredReceipt, err := store.GetCompositionReceipt(ctx, opened.Session.ID)
	if err != nil || !reflect.DeepEqual(receipt, restoredReceipt) {
		t.Fatalf("composition changed: %v", err)
	}
}
