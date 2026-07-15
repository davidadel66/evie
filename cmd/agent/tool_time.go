package main

import (
	"time"

	"github.com/davidadel66/moussa/internal/openrouter"
)

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

func GetTime(_ string) (string, error) {
	return time.Now().Format("2006-01-02 15:04:05"), nil
}
