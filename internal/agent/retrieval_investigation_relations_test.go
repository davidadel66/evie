package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func TestMemoryInvestigationBudgetTrimmingKeepsOnlySupportedRelations(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	readerRemember(t, f, source, "live_in", "Remember that I live in Boston.", "Boston")
	readerRemember(t, f, source, "live_in", "Remember that I live in Portland.", "Portland")
	f.converse(source, "I live in Chicago now.")
	f.refresh()
	check := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) { return "Checked the current request.", nil }}
	sawRelations, sawTrim := false, false
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		if call == 0 {
			return assistantStep("", nil, toolCall("conversation", "memory_search_conversations", `{"query":"Chicago"}`))
		}
		evidence := expansionBoundaryEvidence(t, request)
		selected := map[memory.SemanticID]bool{}
		for _, item := range evidence {
			if item.ClaimID != "" {
				selected[item.ClaimID] = true
			}
		}
		if call > 2 && len(evidence) > 0 && len(evidence) < 3 {
			sawTrim = true
		}
		for _, item := range evidence {
			for _, conflict := range item.Conflicts {
				sawRelations = true
				for _, id := range conflict.ClaimIDs {
					if !selected[id] {
						t.Fatalf("trimmed request advertises an unsupported conflict: %+v", item)
					}
				}
			}
			for _, id := range item.RelatedClaimIDs {
				if !selected[id] {
					t.Fatalf("trimmed request advertises an unsupported newer-statement relation: %+v", item)
				}
			}
		}
		if call == 1 {
			return assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"Boston"}`))
		}
		if call > 40 {
			t.Fatal("unbounded continuation")
		}
		return assistantStep("", nil, toolCall(fmt.Sprintf("check-%d", call), "continue_check", `{}`))
	}}
	err := f.session(f.global(), client, check).Send(context.Background(), "Investigate the conflicting city records.", &recorder{}, nil)
	if !IsContextOverflow(err) || !sawRelations || !sawTrim {
		t.Fatalf("fixture failed to exercise bounded relation trimming: err=%v relations=%v trimmed=%v", err, sawRelations, sawTrim)
	}
}
