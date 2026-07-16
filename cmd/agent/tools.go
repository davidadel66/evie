package main

import (
	"fmt"

	"github.com/davidadel66/moussa/internal/openrouter"
)

// allTools is the single registry of every tool the agent can use. Adding a
// tool means adding one entry here — the schema reaches the model via
// ToolSchemas and the behavior reaches the dispatcher via Execute, so no
// other file needs to change and a tool's name exists in exactly one place.
var allTools = []AgentTool{
	{
		Schema:  getTimeTool,
		Execute: GetTime,
	},
	{
		Schema:  todoListTool,
		Execute: ToDoList,
	},
	{
		Schema:  todoAddTool,
		Execute: ToDoAdd,
	},
}

// AgentTool binds what the model sees (Schema, the wire-format tool
// definition) to what runs when the model calls it (Execute). Execute takes
// the raw arguments JSON string and returns plain text — tools never see or
// build wire-format messages; that stays the dispatcher's job.
type AgentTool struct {
	Schema  openrouter.Tool
	Execute func(args string) (string, error)
}

// ExecuteTool dispatches one model tool call to the matching registry entry
// and wraps the outcome into the Role:"tool" message the API requires for
// every tool_call_id. Failures — a tool error or an unknown tool name — are
// returned as message content rather than dropped, so the model can read
// what went wrong and correct itself on the next turn.
func ExecuteTool(call openrouter.ToolCall) openrouter.Message {
	for _, tool := range allTools {
		if tool.Schema.Function.Name == call.Function.Name {
			resp, err := tool.Execute(call.Function.Arguments)
			if err != nil {
				return openrouter.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("tool call came back with error %v", err),
					ToolCallID: call.ID,
				}
			}
			ans := openrouter.Message{
				Role:       "tool",
				Content:    resp,
				ToolCallID: call.ID,
			}
			return ans
		}
	}
	return openrouter.Message{
		ToolCallID: call.ID,
		Role:       "tool",
		Content:    fmt.Sprintf("Unknown Tool Call: %s", call.Function.Name),
	}
}
