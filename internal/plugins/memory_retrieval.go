package plugins

import (
	"context"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func (p *Memory) searchTool() tools.Tool {
	return tools.Tool{Schema: toolSchema("memory_search", "Search relevant accepted memory. Evidence is supplied as attributed EVIE_MEMORY_DATA in the next request. Empty, failed, unavailable and exhausted are different outcomes; absence is not proof of a negative answer.", map[string]openrouter.Property{
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
		result, err := invocation.SearchMemory(ctx, memory.RetrievalQuery{Text: args.Query})
		if err != nil {
			return "", err
		}
		// Durable tool replay contains no source identifiers or copied evidence.
		// The next request receives only revalidated evidence and records its
		// references in the context snapshot. Revoking egress after this tool
		// succeeds therefore cannot disclose sources through old tool outcomes.
		return renderMemoryRead(struct {
			Status    string                   `json:"status"`
			Matches   int                      `json:"matches"`
			Coverage  memory.RetrievalCoverage `json:"coverage"`
			Truncated bool                     `json:"truncated"`
		}{result.Status, len(result.Evidence), result.Coverage, result.Truncated})
	}}
}
