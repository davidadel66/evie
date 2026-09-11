package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// This development conformance probe exercises the selected local model. It is
// not a held-out retrieval measurement and never substitutes for one.
func TestDenseSelectedModelHandlesBoundedTokenDenseSources(t *testing.T) {
	endpoint := os.Getenv("EVIE_MEMORY_DENSE_MODEL_ENDPOINT")
	if endpoint == "" {
		t.Skip("requires the selected local MiniLM endpoint")
	}
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, strings.Repeat("a! ", 240)+"runs before sunrise")
	f.refresh()
	client, _ := f.search(f.global(), "sunrise")
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if item.ClaimID == target.ClaimID {
			return
		}
	}
	t.Fatal("bounded source failed retained reconciliation and complete-turn retrieval")
}
