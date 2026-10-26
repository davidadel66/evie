package agent

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Shortening an older tool result updates the request's size from the one
// message it changes. With verifyReuse every step is checked against a full
// encoding; this covers escaped characters in every request encoding: plain
// Chat Completions, Chat Completions with cache breakpoints, and Responses.
func TestPressureProjectionRunningSizeMatchesEveryEncoding(t *testing.T) {
	unit := "<a href=\"x\">&amp;   é\t\\ "
	content := strings.Repeat(unit, 6000/len(unit)+1)
	for _, model := range []string{"test/model", "anthropic/claude-sonnet-4", openrouter.AstraModel} {
		t.Run(model, func(t *testing.T) {
			b := &contextEvents{t: t}
			b.user("root", "run")
			var trigger memory.EventID
			for i := 0; i < 12; i++ {
				trigger = b.toolGroup("root", fmt.Sprintf("%d %s", i, content))
			}
			profile, err := openrouter.NewExplicitContextProfile(model, 100000, 100000, 1000)
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
				Profile: profile, Events: b.events, ActiveRootID: "root", TriggerEventID: trigger, Iteration: 13,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Snapshot.Placeholders) < 2 {
				t.Fatalf("pressure shortened %d results; the test needs several", len(result.Snapshot.Placeholders))
			}
			encoded, err := openrouter.RequestBytes(result.Request)
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(encoded)) != result.Snapshot.SerializedBytes {
				t.Fatalf("snapshot size %d, encoded request %d", result.Snapshot.SerializedBytes, len(encoded))
			}
		})
	}
}

// Groups leaving the retained window become eligible for pressure projection
// in whole batches while the request fits, so the projected set, and with it
// the provider's cached history, changes once a batch rather than on every
// tool call. A request that would not fit is projected as far as before.
func TestPressureProjectionWaitsForAWholeBatchWhileTheRequestFits(t *testing.T) {
	b := &contextEvents{t: t}
	b.user("root", "run")
	var results []memory.EventID
	for i := 0; i < retainedCompleteToolResultGroups+pressureProjectionGroupBatch+2; i++ {
		results = append(results, b.toolGroup("root", strings.Repeat(string(rune('a'+i)), 16*1024)))
	}
	projected := func(t *testing.T, usable int64) []memory.EventID {
		t.Helper()
		profile, err := openrouter.NewExplicitContextProfile("test/model", usable+1+4096, usable+1+4096, 1)
		if err != nil {
			t.Fatal(err)
		}
		result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
			Profile: profile, Events: b.events, ActiveRootID: "root", TriggerEventID: results[len(results)-1], Iteration: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		var ids []memory.EventID
		for _, placeholder := range result.Snapshot.Placeholders {
			ids = append(ids, placeholder.EventID)
		}
		return ids
	}
	t.Run("fits with one batch projected", func(t *testing.T) {
		if got := projected(t, 160_000); !slices.Equal(got, results[:pressureProjectionGroupBatch]) {
			t.Fatalf("projected %v, want the oldest batch %v", got, results[:pressureProjectionGroupBatch])
		}
	})
	t.Run("fits only with every eligible group projected", func(t *testing.T) {
		eligible := len(results) - retainedCompleteToolResultGroups
		if got := projected(t, 85_000); !slices.Equal(got, results[:eligible]) {
			t.Fatalf("projected %v, want every eligible group %v", got, results[:eligible])
		}
	})
}
