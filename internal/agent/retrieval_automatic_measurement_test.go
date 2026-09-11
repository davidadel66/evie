package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Repeated fresh chats share one real growing conversation corpus. Report exact
// original targets, not just any lexical match, and retain misses as failures.
func TestAutomaticMemoryRecallDevelopmentMeasurements(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	saved := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	original := f.converse(source, "The greenhouse trial contains saffron crocuses and remains experimental.")
	for i := 0; i < 20; i++ {
		f.remember(source, memory.MemoryEverywhere, fmt.Sprintf("The blue library shelf numbered %d holds gardening notebooks.", i))
		f.converse(source, fmt.Sprintf("Bicycle chain inspection %d found no damage.", i))
	}
	started := time.Now()
	f.refresh()
	build := time.Since(started).Nanoseconds()
	type sample struct {
		Query          string                         `json:"query"`
		WholeTurnNanos int64                          `json:"whole_turn_ns"`
		RequestBytes   int                            `json:"complete_request_bytes"`
		MemoryBytes    int                            `json:"serialized_memory_message_bytes"`
		Count          int                            `json:"evidence_count"`
		ExpectedFound  bool                           `json:"exact_original_target_found"`
		UnwantedCount  int                            `json:"non_target_evidence_count"`
		Input          memory.RetrievalInterpretation `json:"interpretation"`
	}
	var samples []sample
	hits := 0
	for _, question := range []string{"Suggest dinner for me.", "What was the greenhouse trial?", "What is the molybdenum coating?"} {
		for repeat := 0; repeat < 10; repeat++ {
			reader := f.global()
			client := &fakeClient{steps: []step{assistantStep("The available evidence has been checked.", nil)}}
			start := time.Now()
			if err := f.automaticSession(reader, client).Send(context.Background(), question, &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(start).Nanoseconds()
			request := client.reqs[0]
			wire, err := openrouter.RequestBytes(request)
			if err != nil {
				t.Fatal(err)
			}
			block, err := json.Marshal(openrouter.Message{Role: "user", Content: retrievalData(t, request)})
			if err != nil {
				t.Fatal(err)
			}
			evidence := expansionBoundaryEvidence(t, request)
			observation := sample{Query: question, WholeTurnNanos: elapsed, RequestBytes: len(wire), MemoryBytes: len(block), Count: len(evidence)}
			for _, item := range evidence {
				target := item.ClaimID == saved.ClaimID && question == "Suggest dinner for me."
				for _, ref := range item.Sources {
					target = target || ref.EventID == saved.Source.EventID && question == "Suggest dinner for me." || ref.EventID == original.ID && question == "What was the greenhouse trial?"
				}
				if target {
					observation.ExpectedFound = true
				} else {
					observation.UnwantedCount++
				}
			}
			if question == "What is the molybdenum coating?" {
				observation.ExpectedFound = len(evidence) == 0
			}
			if observation.ExpectedFound {
				hits++
			}
			events, err := f.store.LoadEvents(context.Background(), reader.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.Type == memory.EventContextSnapshot {
					var snapshot memory.ContextSnapshotPayload
					if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
						t.Fatal(err)
					}
					if snapshot.Memory != nil && snapshot.Memory.Interpretation != nil {
						observation.Input = *snapshot.Memory.Interpretation
					}
				}
			}
			samples = append(samples, observation)
		}
	}
	report := map[string]any{"version": "automatic-development-v1", "fixture": "one accepted dinner preference, one uncompiled greenhouse statement, twenty accepted and twenty conversation distractors; thirty sequential fresh chats in the same area", "index_refresh_ns": build, "target_or_empty_cases": hits, "total_cases": len(samples), "model_quality": "not measured; scripted provider", "samples": samples}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AUTOMATIC_RECALL_REPORT=%s", raw)
	if hits != len(samples) {
		t.Fatalf("automatic recall found exact target or correctly empty in only %d/%d cases; raw report retained", hits, len(samples))
	}
}
