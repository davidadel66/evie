package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestStandardPresetRereadsAnEarlierToolResultFromDurableHistory(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(),
		plugins.NewTodo(store), plugins.NewMemory(store))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID,
		plugins.TodoPluginID, plugins.MemoryPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("time-call", "get_time", `{}`)),
		assistantStep("it is late", nil),
	}}
	session := NewWithToolset(client, testContextProfile("test-model"), store.BindHistory(stored.ID, "holder"),
		stored.ScopeContext(), store.BindTurnOwner(stored.ID, "holder"), resolved.Toolset)
	if err := session.Send(ctx, "what time is it?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := store.BindHistory(stored.ID, "holder").Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var earlier memory.Event
	for _, event := range events {
		if event.Type == memory.EventToolSucceeded {
			earlier = event
		}
	}
	if earlier.ID == "" {
		t.Fatal("first turn stored no tool result")
	}
	client.steps = []step{
		assistantStep("", nil, toolCall("read-call", "read_tool_result", `{"event_id":"`+string(earlier.ID)+`"}`)),
		assistantStep("same as before", nil),
	}
	if err := session.Send(ctx, "what did the clock say earlier?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err = store.BindHistory(stored.ID, "holder").Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reread := events[len(events)-3]
	if reread.Type != memory.EventToolSucceeded || !strings.Contains(reread.Content, "tool=get_time status=succeeded") ||
		!strings.Contains(reread.Content, "\n"+earlier.Content+"\n") {
		t.Fatalf("reread outcome = %+v, earlier = %q", reread, earlier.Content)
	}
}
