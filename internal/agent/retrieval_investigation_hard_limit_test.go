package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestMemoryInvestigationCannotDispatchPastBudgetWhenProviderContinues(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	f.remember(source, memory.MemoryEverywhere, "peridot records")
	f.refresh()
	cumulative, warnings := 0, 0
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		for _, message := range request.Messages {
			if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") || message.Role == "tool" && strings.HasPrefix(message.Content, "[begin untrusted semantic memory") {
				raw, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				cumulative += len(raw)
			}
		}
		if cumulative > 36*1024 {
			t.Fatalf("an over-budget provider request was dispatched: %d", cumulative)
		}
		if call > 0 && strings.Contains(retrievalData(t, request), `"status":"exhausted"`) {
			warnings++
		}
		if call > 40 {
			t.Fatal("provider continuation escaped the finite budget")
		}
		return assistantStep("", nil, toolCall(fmt.Sprintf("again-%d", call), "memory_search", `{"query":"peridot"}`))
	}}
	err := f.session(reader, client).Send(context.Background(), "Investigate the saved records.", &recorder{}, nil)
	if !IsContextOverflow(err) || warnings == 0 {
		t.Fatalf("expected supported exhaustion warning followed by bounded stop, got %v warnings=%d", err, warnings)
	}
	receipts := investigationReceipts(t, f, reader)
	if len(receipts) != len(client.reqs)-1 {
		t.Fatalf("unadmitted request produced an extra memory receipt: requests=%d receipts=%d", len(client.reqs), len(receipts))
	}
	for _, receipt := range receipts {
		if receipt.Investigation == nil || receipt.Investigation.CumulativeMemoryBytes > 36*1024 {
			t.Fatal("durable accounting exceeded budget")
		}
	}
}
