package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// A provider that keeps searching never receives more than the cumulative
// memory budget of distinct delivery (harness review C2: resent evidence is
// not new delivery), sees the exhausted state once the search limit is spent,
// and is stopped by a turn bound rather than running unbounded.
func TestMemoryInvestigationCannotDispatchPastBudgetWhenProviderContinues(t *testing.T) {
	t.Setenv(TurnStepLimitEnv, "40")
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	f.remember(source, memory.MemoryEverywhere, "peridot records")
	f.refresh()
	var delivery memoryDeliveryTally
	warnings := 0
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		cumulative := delivery.add(t, request, func(message openrouter.Message) bool {
			return strings.HasPrefix(message.Content, "[begin untrusted semantic memory")
		})
		if cumulative > 36*1024 {
			t.Fatalf("an over-budget provider request was dispatched: %d", cumulative)
		}
		if call > 0 && strings.Contains(retrievalData(t, request), `"status":"exhausted"`) {
			warnings++
		}
		if call > 40 {
			t.Fatal("provider continuation escaped the turn bound")
		}
		return assistantStep("", nil, toolCall(fmt.Sprintf("again-%d", call), "memory_search", `{"query":"peridot"}`))
	}}
	err := f.session(reader, client).Send(context.Background(), "Investigate the saved records.", &recorder{}, nil)
	if !(IsContextOverflow(err) || errors.Is(err, ErrStepLimitExceeded)) || warnings == 0 {
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
