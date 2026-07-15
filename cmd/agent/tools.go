package main

import (
	"fmt"

	"github.com/davidadel66/moussa/internal/openrouter"
)

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

type AgentTool struct {
	Schema  openrouter.Tool
	Execute func(args string) (string, error)
}

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
