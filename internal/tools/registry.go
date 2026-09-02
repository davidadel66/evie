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

	"github.com/davidadel66/evie/internal/memory"
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

// ApprovalMetadata is prepared, immutable evidence that a consequential tool
// needs the harness to place in the durable Action Approval record. Ordinary
// tools leave it empty. Semantic tools bind one exact prepared proposal to the
// owner-visible approval without gaining access to event persistence.
type ApprovalMetadata struct {
	Arguments      string
	ParentEventID  memory.EventID
	ExecutionID    memory.ExecutionID
	ProposalSHA256 string
	PreparedSHA256 string
}

func (m ApprovalMetadata) empty() bool {
	return m == (ApprovalMetadata{})
}

func (m ApprovalMetadata) validate() error {
	if m.empty() {
		return nil
	}
	if m.Arguments == "" || m.ParentEventID == "" || m.ExecutionID == "" ||
		m.ProposalSHA256 == "" || m.PreparedSHA256 == "" {
		return errors.New("prepared approval metadata must bind arguments, parent, execution, and both hashes")
	}
	return nil
}

type ApprovalObserver func(ctx context.Context, decision Decision, metadata ApprovalMetadata) error

type AuthorizationBoundary int

const (
	AuthorizePreparation AuthorizationBoundary = iota + 1
	AuthorizeExecution
)

type LifecycleAuthorizer func(context.Context, AuthorizationBoundary) error

type PreparedTool struct {
	Preview  *FileChangePreview
	Approval ApprovalMetadata
	Execute  func(ctx context.Context) (string, error)
}

// InvocationContext is the harness-owned authority supplied to a tool call.
// It is deliberately absent from model arguments: a Capability can use the
// current scope, source event, and live turn fence but cannot choose them.
type InvocationContext struct {
	Scope         memory.ScopeContext
	Lease         memory.TurnLease
	SourceEventID memory.EventID
}

type invocationContextKey struct{}

func WithInvocationContext(ctx context.Context, invocation InvocationContext) context.Context {
	return context.WithValue(ctx, invocationContextKey{}, invocation)
}

func InvocationFromContext(ctx context.Context) (InvocationContext, bool) {
	invocation, ok := ctx.Value(invocationContextKey{}).(InvocationContext)
	return invocation, ok
}

type Approver func(ctx context.Context, name, args string, preview *FileChangePreview) Decision

var kernelToolsBeforeFinance = []Tool{
	{Schema: getTimeTool, Execute: getTime},
	{Schema: todoListTool, Execute: toDoList},
	{Schema: todoAddTool, Execute: toDoAdd},
}

var kernelToolsAfterYouTube = []Tool{
	{Schema: queryDBTool, Execute: queryDB},
	{Schema: editDBTool, Execute: editDB, NeedsApproval: true},
	{Schema: readFileTool, Execute: readFile},
	{Schema: editFileTool, Execute: editFile, Prepare: prepareEditFileTool, NeedsApproval: true},
	{Schema: bashTool, Execute: runBash},
	{Schema: cronAddTool, Execute: cronAdd},
	{Schema: cronListTool, Execute: cronList},
	{Schema: cronRemoveTool, Execute: cronRemove},
}

var kernelTools = append(append([]Tool(nil), kernelToolsBeforeFinance...), kernelToolsAfterYouTube...)

var legacyBuiltinTools = func() []Tool {
	youtube := YouTubeTools()
	definitions := make([]Tool, 0, len(kernelTools)+len(youtube)+len(FinanceTools())+len(WebTools()))
	definitions = append(definitions, kernelToolsBeforeFinance...)
	definitions = append(definitions, FinanceTools()...)
	definitions = append(definitions, youtube...)
	definitions = append(definitions, kernelToolsAfterYouTube...)
	definitions = append(definitions, WebTools()...)
	return definitions
}()

// KernelToolset returns the non-plugin capability surface. Production session
// composition adds enabled plugin contributions to this immutable base.
func KernelToolset() Toolset {
	return NewToolset(kernelTools)
}

// LegacyKernelToolset returns the frozen pre-extraction Kernel surface used
// only to reconstruct Composition Receipts created before Todo and YouTube
// had provider identities. New sessions must use KernelToolset.
func LegacyKernelToolset() Toolset {
	youtube := YouTubeTools()
	definitions := make([]Tool, 0, len(kernelToolsBeforeFinance)+len(youtube)+len(kernelToolsAfterYouTube))
	definitions = append(definitions, kernelToolsBeforeFinance...)
	definitions = append(definitions, youtube...)
	definitions = append(definitions, kernelToolsAfterYouTube...)
	return NewToolset(definitions)
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

// FinanceTools returns fresh definitions for the existing model-facing
// Finance tools. The Finance first-party plugin attaches canonical Capability
// identities while preserving these schemas and execution functions unchanged.
func FinanceTools() []Tool {
	return []Tool{
		FinanceSyncTool(),
		FinanceRulesTool(),
		FinanceCategorizeTool(),
	}
}

// FinanceSyncTool returns the Finance synchronization definition.
func FinanceSyncTool() Tool {
	return Tool{Schema: cloneSchema(financeSyncTool), Execute: financeSync}
}

// FinanceRulesTool returns the Finance rule-loading definition.
func FinanceRulesTool() Tool {
	return Tool{Schema: cloneSchema(financeRulesTool), Execute: financeRules}
}

// FinanceCategorizeTool returns the Finance categorization definition.
func FinanceCategorizeTool() Tool {
	return Tool{Schema: cloneSchema(financeCategorizeTool), Execute: financeCategorize}
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
				if prepared != nil {
					if err := prepared.Approval.validate(); err != nil {
						msg, isErr := toolError(call.ID, err)
						return complete(msg, isErr)
					}
					if !prepared.Approval.empty() && observe == nil {
						msg, isErr := toolError(call.ID, errors.New("prepared semantic mutation requires a durable approval observer"))
						return complete(msg, isErr)
					}
				}

				var preview *FileChangePreview
				if prepared != nil {
					preview = prepared.Preview
				}

				if err := ctx.Err(); err != nil {
					return openrouter.Message{}, false, err
				}
				approvalArguments := call.Function.Arguments
				if prepared != nil && prepared.Approval.Arguments != "" {
					approvalArguments = prepared.Approval.Arguments
				}
				decision = approve(
					ctx,
					call.Function.Name,
					approvalArguments,
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
				metadata := ApprovalMetadata{}
				if prepared != nil {
					metadata = prepared.Approval
				}
				if err := observe(ctx, decision, metadata); err != nil {
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
