// Package tools is the agent's toolbox: every capability the model can
// invoke, one file per tool, plus the registry and dispatcher that wire
// them into the chat loop. A tool binds a wire-format schema to an
// execute function that takes the model's raw arguments JSON and returns
// plain text — tools never see or build wire-format messages; the
// dispatcher owns that boundary.
package tools

import (
	"context"
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
	Execute       func(ctx context.Context, args string) (string, error)
	Prepare       func(ctx context.Context, args string) (PreparedTool, error)
	NeedsApproval bool
}

type ApprovalObserver func(ctx context.Context, decision Decision) error

type AuthorizationBoundary int

const (
	AuthorizePreparation AuthorizationBoundary = iota + 1
	AuthorizeExecution
)

type LifecycleAuthorizer func(context.Context, AuthorizationBoundary) error

type PreparedTool struct {
	Preview *FileChangePreview
	Execute func(ctx context.Context) (string, error)
}

type Approver func(ctx context.Context, name, args string, preview *FileChangePreview) Decision

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

func Execute(ctx context.Context, call openrouter.ToolCall, approve Approver) (openrouter.Message, error) {
	msg, _, err := ExecuteWith(ctx, nil, call, approve)
	return msg, err
}

func ExecuteWith(
	ctx context.Context,
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
) (openrouter.Message, bool, error) {
	return ExecuteWithApproval(ctx, extra, call, approve, nil)
}

func ExecuteWithApproval(
	ctx context.Context,
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
	observe ApprovalObserver,
) (openrouter.Message, bool, error) {
	return ExecuteWithApprovalAuthorized(ctx, extra, call, approve, observe, nil)
}

func ExecuteWithApprovalAuthorized(
	ctx context.Context,
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
	observe ApprovalObserver,
	authorize LifecycleAuthorizer,
) (openrouter.Message, bool, error) {
	if err := ctx.Err(); err != nil {
		return openrouter.Message{}, false, err
	}
	for _, list := range [][]Tool{all, extra} {
		for _, tool := range list {
			if tool.Schema.Function.Name != call.Function.Name {
				continue
			}

			var prepared *PreparedTool
			if tool.NeedsApproval {
				if authorize != nil {
					if err := authorize(ctx, AuthorizePreparation); err != nil {
						return openrouter.Message{}, false, fmt.Errorf("authorize tool preparation: %w", err)
					}
				}
				decision := Declined
				if approve != nil {
					if tool.Prepare != nil {
						if err := ctx.Err(); err != nil {
							return openrouter.Message{}, false, err
						}
						p, err := tool.Prepare(ctx, call.Function.Arguments)
						if err != nil {
							if ctx.Err() != nil {
								return openrouter.Message{}, false, ctx.Err()
							}
							msg, isErr := toolError(call.ID, err)
							return msg, isErr, nil
						}
						prepared = &p
					}

					var preview *FileChangePreview
					if prepared != nil {
						preview = prepared.Preview
					}

					if err := ctx.Err(); err != nil {
						return openrouter.Message{}, false, err
					}
					decision = approve(
						ctx,
						call.Function.Name,
						call.Function.Arguments,
						preview,
					)
					if err := ctx.Err(); err != nil {
						return openrouter.Message{}, false, err
					}
				}

				if observe != nil {
					if err := ctx.Err(); err != nil {
						return openrouter.Message{}, false, err
					}
					if err := observe(ctx, decision); err != nil {
						if ctx.Err() != nil {
							return openrouter.Message{}, false, ctx.Err()
						}
						return openrouter.Message{}, false, fmt.Errorf("observe approval: %w", err)
					}
				}
				if err := ctx.Err(); err != nil {
					return openrouter.Message{}, false, err
				}

				switch decision {
				case Approved:
					if authorize != nil {
						if err := authorize(ctx, AuthorizeExecution); err != nil {
							return openrouter.Message{}, false, fmt.Errorf("authorize tool execution: %w", err)
						}
					}
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
			if !tool.NeedsApproval && authorize != nil {
				if err := authorize(ctx, AuthorizeExecution); err != nil {
					return openrouter.Message{}, false, fmt.Errorf("authorize tool execution: %w", err)
				}
			}

			var response string
			var err error
			if ctxErr := ctx.Err(); ctxErr != nil {
				return openrouter.Message{}, false, ctxErr
			}
			if prepared != nil {
				response, err = prepared.Execute(ctx)
			} else {
				response, err = tool.Execute(ctx, call.Function.Arguments)
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return openrouter.Message{}, false, ctxErr
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
