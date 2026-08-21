// Package tools is the agent's toolbox: every capability the model can
// invoke, one file per tool, plus the registry and dispatcher that wire
// them into the chat loop. A tool binds a wire-format schema to an
// execute function that takes the model's raw arguments JSON and returns
// plain text — tools never see or build wire-format messages; the
// dispatcher owns that boundary.
package tools

import (
	"fmt"

	"github.com/davidadel66/evie/internal/openrouter"
)

type Decision int

const (
	Declined Decision = iota
	Approved
	Expired
)

type Tool struct {
	Schema        openrouter.Tool
	Execute       func(args string) (string, error)
	Prepare       func(args string) (PreparedTool, error)
	NeedsApproval bool
}

type ApprovalObserver func(decision Decision) error

type PreparedTool struct {
	Preview *FileChangePreview
	Execute func() (string, error)
}

type Approver func(name, args string, preview *FileChangePreview) Decision

var all = []Tool{
	{Schema: getTimeTool, Execute: getTime},
	{Schema: todoListTool, Execute: toDoList},
	{Schema: todoAddTool, Execute: toDoAdd},
	{Schema: financeSyncTool, Execute: financeSync},
	{Schema: financeRulesTool, Execute: financeRules},
	{Schema: financeCategorizeTool, Execute: financeCategorize},
	{Schema: youtubeTranscriptTool, Execute: youtubeTranscript},
	{Schema: youtubeScrapeChannelTool, Execute: youtubeScrapeChannel},
	{Schema: queryDBTool, Execute: queryDB},
	{Schema: editDBTool, Execute: editDB, NeedsApproval: true},
	{Schema: readFileTool, Execute: readFile},
	{Schema: editFileTool, Execute: editFile, Prepare: prepareEditFileTool, NeedsApproval: true},
	{Schema: bashTool, Execute: runBash},
	{Schema: webFetchTool, Execute: webFetch},
	{Schema: webSearchTool, Execute: webSearch},
	{Schema: cronAddTool, Execute: cronAdd},
	{Schema: cronListTool, Execute: cronList},
	{Schema: cronRemoveTool, Execute: cronRemove},
}

func Schemas() []openrouter.Tool {
	return SchemasWith(nil)
}

func SchemasWith(extra []Tool) []openrouter.Tool {
	schemas := make([]openrouter.Tool, 0, len(all)+len(extra))
	for _, t := range all {
		schemas = append(schemas, t.Schema)
	}
	for _, t := range extra {
		schemas = append(schemas, t.Schema)
	}

	return schemas
}

func Execute(call openrouter.ToolCall, approve Approver) openrouter.Message {
	msg, _ := ExecuteWith(nil, call, approve)
	return msg
}

func ExecuteWith(
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
) (openrouter.Message, bool) {
	msg, isErr, _ := ExecuteWithApproval(extra, call, approve, nil)
	return msg, isErr
}

func ExecuteWithApproval(
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
	observe ApprovalObserver,
) (openrouter.Message, bool, error) {
	for _, list := range [][]Tool{all, extra} {
		for _, tool := range list {
			if tool.Schema.Function.Name != call.Function.Name {
				continue
			}

			var prepared *PreparedTool
			if tool.NeedsApproval {
				decision := Declined
				if approve != nil {
					if tool.Prepare != nil {
						p, err := tool.Prepare(call.Function.Arguments)
						if err != nil {
							msg, isErr := toolError(call.ID, err)
							return msg, isErr, nil
						}
						prepared = &p
					}

					var preview *FileChangePreview
					if prepared != nil {
						preview = prepared.Preview
					}

					decision = approve(
						call.Function.Name,
						call.Function.Arguments,
						preview,
					)
				}

				if observe != nil {
					if err := observe(decision); err != nil {
						return openrouter.Message{}, false, fmt.Errorf("observe approval: %w", err)
					}
				}

				switch decision {
				case Approved:
					// Continue to execution.
				case Expired:
					return openrouter.Message{
						Role:       "tool",
						Content:    "The approval request expired before David saw it - the call was not run. Ask again if it still matters.",
						ToolCallID: call.ID,
					}, false, nil
				default:
					return openrouter.Message{
						Role:       "tool",
						Content:    "David declined this tool call. Do not retry it unless he asks for something different.",
						ToolCallID: call.ID,
					}, false, nil
				}
			}

			var response string
			var err error
			if prepared != nil {
				response, err = prepared.Execute()
			} else {
				response, err = tool.Execute(call.Function.Arguments)
			}
			if err != nil {
				msg, isErr := toolError(call.ID, err)
				return msg, isErr, nil
			}

			return openrouter.Message{
				Role:       "tool",
				Content:    response,
				ToolCallID: call.ID,
			}, false, nil
		}
	}

	return openrouter.Message{
		Role:       "tool",
		Content:    fmt.Sprintf("Unknown Tool Call: %s", call.Function.Name),
		ToolCallID: call.ID,
	}, true, nil
}

func toolError(id string, err error) (openrouter.Message, bool) {
	return openrouter.Message{
		Role:       "tool",
		Content:    fmt.Sprintf("tool call came back with error %v", err),
		ToolCallID: id,
	}, true
}
