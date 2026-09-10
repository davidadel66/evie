package plugins

import (
	"context"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func (p *Memory) searchTool() tools.Tool {
	return p.retrievalTool("memory_search", "Search relevant accepted memory. Evidence is supplied as attributed EVIE_MEMORY_DATA in the next request. Empty, failed, unavailable and exhausted are different outcomes; absence is not proof of a negative answer.", memory.RetrievalAcceptedMemory)
}

func (p *Memory) searchConversationsTool() tools.Tool {
	return p.retrievalTool("memory_search_conversations", "Find attributed short excerpts from eligible conversations in this same area. Excerpts establish what was recorded, not accepted current facts. Retired corresponding evidence is excluded. Evidence appears in EVIE_MEMORY_DATA; failed or partial search does not establish absence.", memory.RetrievalConversationExcerpt)
}

func (p *Memory) retrievalTool(name, description, kind string) tools.Tool {
	return tools.Tool{Schema: toolSchema(name, description, map[string]openrouter.Property{
		"query": stringProperty("Focused words, exact identifiers, or accepted aliases."),
	}, "query"), Execute: func(ctx context.Context, raw string) (string, error) {
		var args struct {
			Query string `json:"query"`
		}
		if err := decodeMemoryArgs(raw, &args); err != nil {
			return "", err
		}
		invocation, err := modelReadInvocation(ctx)
		if err != nil {
			return "", err
		}
		if invocation.SearchMemory == nil {
			return "", errors.New("memory search requires an active turn")
		}
		result, err := invocation.SearchMemory(ctx, memory.RetrievalQuery{Text: args.Query, Kind: kind})
		if err != nil {
			return "", err
		}
		return renderRetrievalOutcome(result)
	}}
}

func renderRetrievalOutcome(result memory.RetrievalResult) (string, error) {
	// Persist counts/status only. Source text and references reach the provider
	// only after revalidation in the next synthetic request projection.
	return renderMemoryRead(struct {
		Status    string                   `json:"status"`
		Matches   int                      `json:"matches"`
		Coverage  memory.RetrievalCoverage `json:"coverage"`
		Truncated bool                     `json:"truncated"`
	}{result.Status, len(result.Evidence), result.Coverage, result.Truncated})
}

func (p *Memory) expandConversationTool() tools.Tool {
	return tools.Tool{Schema: toolSchema("memory_expand_conversation", "Read bounded original context around an eligible Conversation excerpt already supplied this turn. Use its exact evidence_id. Request at most two public messages before and after; repeated ranges add no new evidence. This is attributed history, not accepted truth.", map[string]openrouter.Property{
		"evidence_id": stringProperty("Exact ID of a Conversation excerpt already supplied this turn."),
		"before":      {Type: "integer", Description: "Previous public messages, 0 through 2."},
		"after":       {Type: "integer", Description: "Following public messages, 0 through 2."},
	}, "evidence_id", "before", "after"), Execute: func(ctx context.Context, raw string) (string, error) {
		var args struct {
			EvidenceID string `json:"evidence_id"`
			Before     int    `json:"before"`
			After      int    `json:"after"`
		}
		if err := decodeMemoryArgs(raw, &args); err != nil {
			return "", err
		}
		invocation, err := modelReadInvocation(ctx)
		if err != nil {
			return "", err
		}
		if invocation.SearchMemory == nil {
			return "", errors.New("memory expansion requires an active turn")
		}
		result, err := invocation.SearchMemory(ctx, memory.RetrievalQuery{Kind: memory.RetrievalConversationExpansion, AnchorID: args.EvidenceID, Before: args.Before, After: args.After})
		if err != nil {
			return "", err
		}
		return renderRetrievalOutcome(result)
	}}
}
