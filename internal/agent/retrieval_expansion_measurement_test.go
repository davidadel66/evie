package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// This development experiment measures actual request costs and whole-turn
// latency. The scripted provider is not an answer-quality evaluator.
func TestConversationExpansionWindowMeasurements(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	long := strings.Repeat("The été planning discussion remains tentative. ", 24)
	for _, text := range []string{long, "windowanchor: She might go, but nothing is booked.", long} {
		client := &fakeClient{steps: []step{assistantStep(long, nil)}}
		if err := f.session(source, client).Send(context.Background(), text, &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	f.refresh()
	type sample struct {
		Window       int    `json:"messages_each_side"`
		TurnNanos    int64  `json:"whole_turn_nanoseconds"`
		Evidence     int    `json:"supplied_evidence"`
		RequestBytes int    `json:"complete_request_bytes"`
		MemoryBytes  int    `json:"serialized_memory_message_bytes"`
		Outcome      string `json:"expansion_outcome"`
	}
	var samples []sample
	for _, window := range []int{0, 1, 2} {
		for repeat := 0; repeat < 10; repeat++ {
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				switch call {
				case 0:
					return assistantStep("", nil, toolCall("lookup", "memory_search_conversations", `{"query":"windowanchor"}`))
				case 1:
					evidence := expansionBoundaryEvidence(t, request)
					if len(evidence) != 1 {
						t.Fatalf("anchor count=%d", len(evidence))
					}
					return expansionBoundaryCall(t, "expand", evidence[0].ID, window, window)
				default:
					return assistantStep("Recorded tentative context received.", nil)
				}
			}}
			started := time.Now()
			if err := f.session(f.global(), client).Send(context.Background(), "Read the context of the tentative plan.", &recorder{}, nil); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started).Nanoseconds()
			req := client.reqs[2]
			encoded, _ := json.Marshal(req)
			memoryMessage, _ := json.Marshal(openrouter.Message{Role: "user", Content: retrievalData(t, req)})
			evidence := expansionBoundaryEvidence(t, req)
			outcome := expansionBoundaryToolResult(t, req, "expand")
			if len(evidence) > 5 || len(memoryMessage) > retrievalTurnBytes {
				t.Fatalf("window escaped bound: evidence=%d memory=%d", len(evidence), len(memoryMessage))
			}
			samples = append(samples, sample{window, elapsed, len(evidence), len(encoded), len(memoryMessage), outcome})
		}
	}
	encoded, err := json.Marshal(struct {
		Version     string   `json:"version"`
		Purpose     string   `json:"purpose"`
		Repetitions int      `json:"repetitions_per_window"`
		Samples     []sample `json:"samples"`
	}{"expansion-window-v1", "development request-cost experiment with real SQLite and scripted provider; no model-quality measurement", 10, samples})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MEMORY_EXPANSION_WINDOW_REPORT=%s", encoded)
}
