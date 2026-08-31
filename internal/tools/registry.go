// Package tools is the agent's toolbox: every capability the model can
// invoke, one file per tool, plus the registry and dispatcher that wire
// them into the chat loop. A tool binds a wire-format schema to an
// execute function that takes the model's raw arguments JSON and returns
// plain text — tools never see or build wire-format messages; the
// dispatcher owns that boundary.
package tools

import (
	"context"
	"errors"
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

// Toolset is the immutable set of model-facing schemas and execution behavior
// resolved for one agent session. Construction copies tool definitions and
// their schema data so later changes to the source slice cannot alter a live
// session's capability surface.
type Toolset struct {
	tools   []Tool
	schemas []openrouter.Tool
}

func NewToolset(toolDefinitions []Tool) Toolset {
	toolset := Toolset{
		tools:   make([]Tool, len(toolDefinitions)),
		schemas: make([]openrouter.Tool, len(toolDefinitions)),
	}
	for i, definition := range toolDefinitions {
		definition.Schema = cloneSchema(definition.Schema)
		toolset.tools[i] = definition
		toolset.schemas[i] = cloneSchema(definition.Schema)
	}
	return toolset
}

// BuiltinToolset returns the complete legacy built-in tool surface. A fresh
// immutable value is returned for each session.
func BuiltinToolset() Toolset {
	return NewToolset(legacyBuiltinTools)
}

func (t Toolset) Schemas() []openrouter.Tool {
	schemas := make([]openrouter.Tool, len(t.schemas))
	for i, schema := range t.schemas {
		schemas[i] = cloneSchema(schema)
	}
	return schemas
}

// WithTools returns a new Toolset without changing the receiver. It exists for
// transitional callers that formerly supplied per-turn tools.
func (t Toolset) WithTools(extra []Tool) Toolset {
	definitions := make([]Tool, 0, len(t.tools)+len(extra))
	definitions = append(definitions, t.tools...)
	definitions = append(definitions, extra...)
	return NewToolset(definitions)
}

func cloneSchema(schema openrouter.Tool) openrouter.Tool {
	clone := schema
	clone.Function.Parameters.Required = append([]string(nil), schema.Function.Parameters.Required...)
	if schema.Function.Parameters.Properties != nil {
		clone.Function.Parameters.Properties = make(map[string]openrouter.Property, len(schema.Function.Parameters.Properties))
		for name, property := range schema.Function.Parameters.Properties {
			clone.Function.Parameters.Properties[name] = cloneProperty(property)
		}
	}
	return clone
}

func cloneProperty(property openrouter.Property) openrouter.Property {
	clone := property
	clone.Enum = append([]string(nil), property.Enum...)
	if property.Items != nil {
		items := cloneProperty(*property.Items)
		clone.Items = &items
	}
	return clone
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

var kernelTools = []Tool{
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
	{Schema: cronAddTool, Execute: cronAdd},
	{Schema: cronListTool, Execute: cronList},
	{Schema: cronRemoveTool, Execute: cronRemove},
}

var legacyBuiltinTools = append(append([]Tool(nil), kernelTools...), WebTools()...)

// KernelToolset returns the non-plugin capability surface. Production session
// composition adds enabled plugin contributions to this immutable base.
func KernelToolset() Toolset {
	return NewToolset(kernelTools)
}

// WebTools returns fresh definitions for the existing model-facing Web tools.
// The Web first-party plugin attaches canonical Capability identities while
// preserving these schemas and execution functions unchanged.
func WebTools() []Tool {
	return []Tool{
		{Schema: cloneSchema(webFetchTool), Execute: webFetch},
		{Schema: cloneSchema(webSearchTool), Execute: webSearch},
	}
}

func Schemas() []openrouter.Tool {
	return BuiltinToolset().Schemas()
}

func SchemasWith(extra []Tool) []openrouter.Tool {
	definitions := append(append([]Tool(nil), legacyBuiltinTools...), extra...)
	return NewToolset(definitions).Schemas()
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
	return ExecuteWithApprovalAuthorizedCompletion(ctx, extra, call, approve, observe, authorize, nil)
}

// ExecuteWithApprovalAuthorizedCompletion calls ordinaryComplete at the exact
// handoff where preparation failure, non-approved completion, execution result,
// or unknown-tool output becomes an ordinary model-visible tool result.
// Cancellation and authorization failures do not call it.
func ExecuteWithApprovalAuthorizedCompletion(
	ctx context.Context,
	extra []Tool,
	call openrouter.ToolCall,
	approve Approver,
	observe ApprovalObserver,
	authorize LifecycleAuthorizer,
	ordinaryComplete func(),
) (openrouter.Message, bool, error) {
	definitions := append(append([]Tool(nil), legacyBuiltinTools...), extra...)
	return NewToolset(definitions).ExecuteWithApprovalAuthorizedCompletion(
		ctx, call, approve, observe, authorize, ordinaryComplete,
	)
}

// ExecuteWithApprovalAuthorizedCompletion preserves the existing approval,
// authorization, cancellation, completion, and unknown-tool behavior while
// dispatching only within this resolved Toolset.
func (t Toolset) ExecuteWithApprovalAuthorizedCompletion(
	ctx context.Context,
	call openrouter.ToolCall,
	approve Approver,
	observe ApprovalObserver,
	authorize LifecycleAuthorizer,
	ordinaryComplete func(),
) (openrouter.Message, bool, error) {
	complete := func(message openrouter.Message, isErr bool) (openrouter.Message, bool, error) {
		if ordinaryComplete != nil {
			ordinaryComplete()
		}
		return message, isErr, nil
	}
	if err := ctx.Err(); err != nil {
		return openrouter.Message{}, false, err
	}
	for _, tool := range t.tools {
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
						if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
							return openrouter.Message{}, false, ctx.Err()
						}
						msg, isErr := toolError(call.ID, err)
						return complete(msg, isErr)
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
				return complete(openrouter.Message{
					Role:       "tool",
					Content:    "The approval request expired before David saw it - the call was not run. Ask again if it still matters.",
					ToolCallID: call.ID,
				}, false)
			default:
				return complete(openrouter.Message{
					Role:       "tool",
					Content:    "David declined this tool call. Do not retry it unless he asks for something different.",
					ToolCallID: call.ID,
				}, false)
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
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				return openrouter.Message{}, false, ctxErr
			}
			msg, isErr := toolError(call.ID, err)
			return complete(msg, isErr)
		}

		return complete(openrouter.Message{
			Role:       "tool",
			Content:    response,
			ToolCallID: call.ID,
		}, false)
	}

	return complete(openrouter.Message{
		Role:       "tool",
		Content:    fmt.Sprintf("Unknown Tool Call: %s", call.Function.Name),
		ToolCallID: call.ID,
	}, true)
}

func toolError(id string, err error) (openrouter.Message, bool) {
	return openrouter.Message{
		Role:       "tool",
		Content:    fmt.Sprintf("tool call came back with error %v", err),
		ToolCallID: id,
	}, true
}
