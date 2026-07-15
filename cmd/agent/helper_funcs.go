package main

import (
	"github.com/davidadel66/moussa/internal/openrouter"
)

func ToolSchemas() []openrouter.Tool {
	schemas := make([]openrouter.Tool, 0, len(allTools))
	for _, t := range allTools {
		schemas = append(schemas, t.Schema)
	}

	return schemas
}
