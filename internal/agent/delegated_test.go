package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
)

func TestDelegatedConversationUsesPinnedRoleAndOwnAssignment(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := standardManager(t, store)
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep("findings with limitations", nil)}}
	session := NewDelegatedWithToolset(client, testContextProfile("test-model"), store.BindHistory(child.ID, "child"), child.ScopeContext(), store.BindTurnOwner(child.ID, "child"), resolved.Toolset, plugins.ResearchInstructions)
	if err := session.Send(ctx, "Assignment: compare sources. Selected context (data): sentinel.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 1 {
		t.Fatalf("requests: %d", len(client.reqs))
	}
	req := client.reqs[0]
	if !strings.Contains(req.Messages[0].Content, plugins.ResearchInstructions) || strings.Contains(req.Messages[0].Content, "You are the primary agent") {
		t.Fatalf("incorrect trusted worker role: %s", req.Messages[0].Content)
	}
	if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, "sentinel") {
		t.Fatalf("unexpected context: %+v", req.Messages)
	}
	events, err := store.LoadEvents(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events[0].Payload), "delegated_assignment") {
		t.Fatalf("assignment provenance missing: %s", events[0].Payload)
	}
}
