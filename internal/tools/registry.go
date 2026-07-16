// Package tools is the agent's toolbox: every capability the model can
// invoke, one file per tool, plus the registry and dispatcher that wire
// them into the chat loop. A tool binds a wire-format schema to an
// execute function that takes the model's raw arguments JSON and returns
// plain text — tools never see or build wire-format messages; the
// dispatcher owns that boundary.
package tools

import (
	"fmt"

	"github.com/davidadel66/moussa/internal/openrouter"
)

// Tool binds what the model sees (Schema, the wire-format tool
// definition) to what runs when the model calls it (Execute).
type Tool struct {
	Schema  openrouter.Tool
	Execute func(args string) (string, error)
}

// all is the single registry of every tool the agent can use. Adding a
// tool means adding one entry here — the schema reaches the model via
// Schemas and the behavior reaches the dispatcher via Execute, so no
// other file needs to change and a tool's name exists in exactly one
// place.
var all = []Tool{
	{Schema: getTimeTool, Execute: getTime},
	{Schema: todoListTool, Execute: toDoList},
	{Schema: todoAddTool, Execute: toDoAdd},
	{Schema: financeSyncTool, Execute: financeSync},
}

// Schemas extracts just the wire-format schemas from the registry for the
// chat request — the model sees every tool's definition but none of its
// behavior. Derived from the registry each call so the two can never
// disagree.
func Schemas() []openrouter.Tool {
	schemas := make([]openrouter.Tool, 0, len(all))
	for _, t := range all {
		schemas = append(schemas, t.Schema)
	}

	return schemas
}

// Execute dispatches one model tool call to the matching registry entry
// and wraps the outcome into the Role:"tool" message the API requires for
// every tool_call_id. Failures — a tool error or an unknown tool name —
// are returned as message content rather than dropped, so the model can
// read what went wrong and correct itself on the next turn.
func Execute(call openrouter.ToolCall) openrouter.Message {
	for _, tool := range all {
		if tool.Schema.Function.Name == call.Function.Name {
			resp, err := tool.Execute(call.Function.Arguments)
			if err != nil {
				return openrouter.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("tool call came back with error %v", err),
					ToolCallID: call.ID,
				}
			}
			return openrouter.Message{
				Role:       "tool",
				Content:    resp,
				ToolCallID: call.ID,
			}
		}
	}
	return openrouter.Message{
		ToolCallID: call.ID,
		Role:       "tool",
		Content:    fmt.Sprintf("Unknown Tool Call: %s", call.Function.Name),
	}
}
