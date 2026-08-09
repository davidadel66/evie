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

// Decision is an approver's answer for one gated call. Declined and
// Expired differ in what the model is told: Declined means David said
// no; Expired means no human ever saw the request (the frontend went
// away) — the model must not be told David refused something he never
// saw. The zero value is Declined, so approvals fail closed.
type Decision int

const (
	Declined Decision = iota
	Approved
	Expired
)

// Tool binds what the model sees (Schema, the wire-format tool
// definition) to what runs when the model calls it (Execute).
// NeedsApproval marks tools whose calls must be confirmed by the user
// before executing — the confirmation itself is supplied by the caller
// of Execute, because this package never touches the terminal.
type Tool struct {
	Schema        openrouter.Tool
	Execute       func(args string) (string, error)
	NeedsApproval bool
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
	{Schema: financeRulesTool, Execute: financeRules},
	{Schema: financeCategorizeTool, Execute: financeCategorize},
	{Schema: queryDBTool, Execute: queryDB},
	{Schema: editDBTool, Execute: editDB, NeedsApproval: true},
	{Schema: readFileTool, Execute: readFile},
	{Schema: editFileTool, Execute: editFile, NeedsApproval: true},
	{Schema: bashTool, Execute: runBash},
	{Schema: webFetchTool, Execute: webFetch},
	{Schema: webSearchTool, Execute: webSearch},
	{Schema: cronAddTool, Execute: cronAdd},
	{Schema: cronListTool, Execute: cronList},
	{Schema: cronRemoveTool, Execute: cronRemove},
}

// Schemas extracts just the wire-format schemas from the registry for the
// chat request — the model sees every tool's definition but none of its
// behavior. Derived from the registry each call so the two can never
// disagree.
func Schemas() []openrouter.Tool {
	return SchemasWith(nil)
}

// SchemasWith is Schemas plus extra — per-turn tools a frontend supplies
// beyond the registry (a tool built at turn time can capture state the
// package-level registry can't, like a live connection to a frontend).
// Extras are appended after the base list.
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

// Execute dispatches one model tool call to the matching registry entry
// and wraps the outcome into the Role:"tool" message the API requires for
// every tool_call_id. Failures — a tool error, an unknown tool name, or
// a declined approval — are returned as message content rather than
// dropped, so the model can read what went wrong and correct itself on
// the next turn.
//
// approve is asked before any NeedsApproval tool runs; the frontend owns
// how (terminal y/n today). Passing nil means "no approver available",
// which declines every gated call rather than silently allowing it.
func Execute(call openrouter.ToolCall, approve func(name, args string) Decision) openrouter.Message {
	msg, _ := ExecuteWith(nil, call, approve)
	return msg
}

// ExecuteWith is Execute plus extra — the same per-turn tools handed to
// SchemasWith, so a call the model makes against an advertised extra can
// actually dispatch. The base registry is searched first: an extra can
// never shadow a built-in tool.
//
// The bool reports whether the outcome was a failure (tool error or
// unknown tool) so frontends can mark it without parsing the content. A
// decline is not a failure — it's the gate working as intended.
func ExecuteWith(extra []Tool, call openrouter.ToolCall, approve func(name, args string) Decision) (openrouter.Message, bool) {
	for _, list := range [][]Tool{all, extra} {
		for _, tool := range list {
			if tool.Schema.Function.Name == call.Function.Name {
				if tool.NeedsApproval {
					decision := Declined
					if approve != nil {
						decision = approve(call.Function.Name, call.Function.Arguments)
					}
					switch decision {
					case Approved:
						// fall through to execute
					case Expired:
						return openrouter.Message{
							Role:       "tool",
							Content:    "The approval request expired before David saw it — the call was not run. Ask again if it still matters.",
							ToolCallID: call.ID,
						}, false
					default:
						return openrouter.Message{
							Role:       "tool",
							Content:    "David declined this tool call. Do not retry it unless he asks for something different.",
							ToolCallID: call.ID,
						}, false
					}
				}
				resp, err := tool.Execute(call.Function.Arguments)
				if err != nil {
					return openrouter.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("tool call came back with error %v", err),
						ToolCallID: call.ID,
					}, true
				}
				return openrouter.Message{
					Role:       "tool",
					Content:    resp,
					ToolCallID: call.ID,
				}, false
			}
		}
	}
	return openrouter.Message{
		ToolCallID: call.ID,
		Role:       "tool",
		Content:    fmt.Sprintf("Unknown Tool Call: %s", call.Function.Name),
	}, true
}
