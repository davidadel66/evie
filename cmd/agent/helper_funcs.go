package main

import (
	"github.com/davidadel66/moussa/internal/openrouter"
)

// ToolSchemas extracts just the wire-format schemas from the registry for
// the chat request — the model sees every tool's definition but none of its
// behavior. Derived from allTools each call so the two can never disagree.
func ToolSchemas() []openrouter.Tool {
	schemas := make([]openrouter.Tool, 0, len(allTools))
	for _, t := range allTools {
		schemas = append(schemas, t.Schema)
	}

	return schemas
}
