package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// This diagnostic measures the actual public turn and SQLite revalidation, not
// answering quality. It is opt-in so normal verification does not generate data.
func TestMemoryInvestigationDevelopmentMeasurements(t *testing.T) {
	output := os.Getenv("EVIE_MEMORY_INVESTIGATION_MEASURE_DIR")
	if output == "" {
		t.Skip("set EVIE_MEMORY_INVESTIGATION_MEASURE_DIR for frozen development measurements")
	}
	f := newRetrievalFixture(t)
	source := f.global()
	saved := f.remember(source, memory.MemoryEverywhere, "moonstone keepsake")
	original := f.converse(source, "The indigo observatory experiment is provisional.")
	for i := 0; i < 20; i++ {
		f.remember(source, memory.MemoryEverywhere, fmt.Sprintf("Cedar cabinet record %d.", i))
		f.converse(source, fmt.Sprintf("Bicycle inspection number %d found no damage.", i))
	}
	type requestSample struct {
		CompleteBytes  int                      `json:"complete_request_bytes"`
		CompleteSHA256 string                   `json:"complete_request_sha256"`
		MemoryBytes    int                      `json:"memory_message_bytes"`
		Receipt        *memory.RetrievalReceipt `json:"receipt,omitempty"`
	}
	type sample struct {
		IndexRefreshNS int64           `json:"index_refresh_ns"`
		WholeTurnNS    int64           `json:"whole_turn_ns"`
		Requests       []requestSample `json:"requests"`
	}
	var samples []sample
	check := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) { return "Checked the current request.", nil }}
	for repeat := 0; repeat < 30; repeat++ {
		reader := f.global()
		started := time.Now()
		f.refresh()
		refreshNS := time.Since(started).Nanoseconds()
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"moonstone"}`), toolCall("original", "memory_search_conversations", `{"query":"indigo"}`)),
			assistantStep("", nil, toolCall("check-1", "continue_check", `{}`)),
			assistantStep("", nil, toolCall("check-2", "continue_check", `{}`)),
			assistantStep("The supplied keepsake and provisional experiment sources remain valid.", nil),
		}}
		started = time.Now()
		if err := f.session(reader, client, check).Send(context.Background(), "Investigate my saved records, checking the current request twice.", &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
		observation := sample{IndexRefreshNS: refreshNS, WholeTurnNS: time.Since(started).Nanoseconds()}
		receipts := investigationReceipts(t, f, reader)
		if len(receipts) != 3 {
			t.Fatalf("measurement lacks expected continuation receipts: %d", len(receipts))
		}
		cumulative := 0
		for i, request := range client.reqs {
			wire, err := openrouter.RequestBytes(request)
			if err != nil {
				t.Fatal(err)
			}
			measured := requestSample{CompleteBytes: len(wire), CompleteSHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(wire))}
			for _, message := range request.Messages {
				if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") || message.Role == "tool" && (message.ToolCallID == "accepted" || message.ToolCallID == "original") {
					raw, err := json.Marshal(message)
					if err != nil {
						t.Fatal(err)
					}
					measured.MemoryBytes += len(raw)
				}
			}
			cumulative += measured.MemoryBytes
			if i > 0 {
				receipt := receipts[i-1]
				measured.Receipt = &receipt
				accepted, raw := false, false
				for _, ref := range receipt.Evidence {
					accepted = accepted || ref.ClaimID == saved.ClaimID
					for _, source := range ref.Sources {
						raw = raw || source.EventID == original.ID
					}
				}
				reuse := 0
				if i > 1 {
					reuse = 2
				}
				if !accepted || !raw || len(receipt.Evidence) != 2 || receipt.Investigation == nil || receipt.Investigation.SearchAttempts != 2 || receipt.Investigation.RefreshAttempts != 0 || receipt.Investigation.ReusedEvidence != reuse || receipt.Investigation.CumulativeMemoryBytes != cumulative || cumulative > 36*1024 {
					t.Fatalf("measurement violated target/reuse/accounting contract: %+v actual=%d", receipt, cumulative)
				}
			}
			observation.Requests = append(observation.Requests, measured)
		}
		samples = append(samples, observation)
	}
	report := map[string]any{"version": "investigation-development-v1", "fixture": "one accepted keepsake, one uncompiled provisional experiment, twenty accepted and twenty conversation distractors; thirty sequential fresh reader chats in one growing SQLite database", "reader": "scripted provider; model answer quality is not measured", "expected_claim_id": saved.ClaimID, "expected_original_event_id": original.ID, "samples": samples}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "results.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
