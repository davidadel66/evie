package tools

import (
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// getTimeTool describes get_time to the model: fetch the current local
// time, no parameters.
var getTimeTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "get_time",
		Description: "This tool fetches the current time at the moment of execution",
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

// getTime returns the current local time. It takes the arguments string
// only to satisfy the Execute contract and ignores it — this tool has no
// parameters.
func getTime(_ string) (string, error) {
	return time.Now().Format("2006-01-02 15:04:05"), nil
}
