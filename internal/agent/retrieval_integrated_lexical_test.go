package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// TestMemoryStage5IntegratedLexicalRecall reruns the frozen 24-case
// development and held-out workloads through the scripted local probe with
// dense retrieval disabled, and reports how many gold source obligations each
// automatic condition delivers. It makes no reader-model call and claims no
// answer quality; it exists so a recall-policy change can show whether it
// loses any source obligation on the frozen cases. Set
// EVIE_MEMORY_INTEGRATED_LEXICAL=1 to run it.
func TestMemoryStage5IntegratedLexicalRecall(t *testing.T) {
	if os.Getenv("EVIE_MEMORY_INTEGRATED_LEXICAL") != "1" {
		t.Skip("set EVIE_MEMORY_INTEGRATED_LEXICAL=1 to measure lexical source recall on the frozen workloads")
	}
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	for _, workload := range []struct{ name, path string }{
		{"development-v1", integratedWorkloadDefault},
		{"heldout-v2", "../../cmd/evie/docs/fixtures/memory-stage5-integrated/heldout/v2/workload.json"},
	} {
		t.Run(workload.name, func(t *testing.T) {
			loaded := integratedLoadWorkload(t, workload.path)
			type tally struct{ initial, union, required, delivered, unwanted int }
			totals := map[string]*tally{}
			var lines []string
			for _, test := range loaded.Cases {
				directory := filepath.Join(t.TempDir(), test.ID)
				seed := integratedBuildSeed(t, test, directory)
				for _, condition := range []string{"automatic", "automatic_deeper"} {
					t.Setenv("EVIE_REMOTE_MEMORY", "on")
					f := integratedCloneSeed(t, directory, seed)
					client := &integratedClient{t: t, f: f, seed: seed, condition: condition}
					if condition == "automatic_deeper" {
						client.scripted = func(d integratedDispatch) openrouter.ChatResponse {
							return integratedLocalResponse(condition, seed.Question, d)
						}
					}
					session, _, reader := integratedSession(t, f, seed, condition, client, testContextProfile("test-model"))
					client.reader = reader
					if test.MemoryMode == "unavailable" {
						t.Setenv("EVIE_REMOTE_MEMORY", "off")
					}
					if err := session.Send(context.Background(), seed.Question, &recorder{}, nil); err != nil {
						t.Fatalf("%s/%s: %v", test.ID, condition, err)
					}
					has := func(evidence []memory.RetrievalEvidence, id string) bool {
						b := seed.Bindings[id]
						for _, e := range evidence {
							if b.ClaimID != "" && e.ClaimID == b.ClaimID || b.ClaimID == "" && integratedHasEvent(e, b.Source.EventID) {
								return true
							}
						}
						return false
					}
					var all []memory.RetrievalEvidence
					for _, d := range client.dispatches {
						all = append(all, d.Evidence...)
					}
					sum := totals[condition]
					if sum == nil {
						sum = &tally{}
						totals[condition] = sum
					}
					var missing, unionMissing []string
					for _, id := range test.Gold.SupportSets[0] {
						sum.required++
						if has(client.dispatches[0].Evidence, id) {
							sum.initial++
						} else {
							missing = append(missing, id)
						}
						if has(all, id) {
							sum.union++
						} else {
							unionMissing = append(unionMissing, id)
						}
					}
					var unwanted []string
					for _, e := range client.dispatches[0].Evidence {
						sum.delivered++
						wanted := false
						for _, id := range append(append([]string(nil), test.Gold.SupportSets[0]...), test.Gold.AcceptableContext...) {
							wanted = wanted || has([]memory.RetrievalEvidence{e}, id)
						}
						if !wanted {
							sum.unwanted++
							label := e.Kind + ":" + e.Text
							for id, b := range seed.Bindings {
								if b.ClaimID != "" && e.ClaimID == b.ClaimID || b.ClaimID == "" && integratedHasEvent(e, b.Source.EventID) {
									label = id
								}
							}
							unwanted = append(unwanted, label)
						}
					}
					lines = append(lines, fmt.Sprintf("%s %s initial-missing=%v union-missing=%v non-gold=%q", test.ID, condition, missing, unionMissing, unwanted))
				}
			}
			for _, condition := range []string{"automatic", "automatic_deeper"} {
				sum := totals[condition]
				lines = append(lines, fmt.Sprintf("TOTAL %s initial=%d/%d union=%d/%d first-dispatch items=%d non-gold=%d", condition, sum.initial, sum.required, sum.union, sum.required, sum.delivered, sum.unwanted))
			}
			t.Logf("\n%s", strings.Join(lines, "\n"))
		})
	}
}
