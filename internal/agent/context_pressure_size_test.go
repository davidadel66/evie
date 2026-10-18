package agent

import (
	"fmt"
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
