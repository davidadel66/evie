package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

// A delegated worker's supervisor can end the turn early through the same
// tool-free final call as the step limit, with its own harness notice. The
// signal is consulted before every model response and also replaces the
// step-limit notice when the limit is reached.
func TestWrapUpSignalRequestsTheFinalToolFreeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, limit string
		wrapAt      int
	}{{"budget", "100", 1}, {"step_limit", "1", 100}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(TurnStepLimitEnv, tc.limit)
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			parent, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := standardManager(t, store).ResolvePreset(plugins.ResearchPresetID)
			if err != nil {
				t.Fatal(err)
			}
			child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, resolved.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			const notice = "Evie harness notice: wrap up now (test)."
			var asked [][2]int
			signal := func(step, limit int) (string, bool) {
				asked = append(asked, [2]int{step, limit})
				if step >= tc.wrapAt || step >= limit {
					return notice, true
				}
				return "", false
			}
			client := &fakeClient{steps: []step{assistantStep("## Summary\nwrapped", nil)}}
			session := NewDelegatedWithToolset(client, testContextProfile("test-model"), store.BindHistory(child.ID, "child"), child.ScopeContext(), store.BindTurnOwner(child.ID, "child"), resolved.Toolset, plugins.ResearchInstructions, WithWrapUp(signal))
			if err := session.Send(ctx, "Assignment: research.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			if len(client.reqs) != 1 || client.reqs[0].ToolChoice != "none" || len(client.reqs[0].Tools) == 0 {
				t.Fatalf("requests=%+v", client.reqs)
			}
			last := client.reqs[0].Messages[len(client.reqs[0].Messages)-1]
			if last.Role != "user" || last.Content != notice || len(asked) != 1 || asked[0][0] != 1 {
				t.Fatalf("final message=%+v asked=%v", last, asked)
			}
			events, err := store.LoadEvents(ctx, child.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final := events[len(events)-1]; final.Type != memory.EventAssistantMessage || final.Content != "## Summary\nwrapped" {
				t.Fatalf("final event=%+v", final)
			}
		})
	}
}
