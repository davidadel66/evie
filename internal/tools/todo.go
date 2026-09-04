package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/davidadel66/evie/internal/openrouter"
)

// todoListTool describes todo_list to the model: read the current task
// list, no parameters.
var todoListTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_list",
		Description: "This is a CLI command to run get the list of the current todo",
		Parameters: openrouter.Parameter{
			Type:       "object",
			Properties: map[string]openrouter.Property{},
		},
	},
}

var todoLifecycleListTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_list",
		Description: "List durable tasks; defaults to open tasks and can explicitly include lifecycle history",
		Parameters: openrouter.Parameter{
			Type: "object",
			Properties: map[string]openrouter.Property{
				"statuses": {
					Type: "array", Description: "Optional lifecycle statuses to return",
					Items: &openrouter.Property{Type: "string", Enum: []string{"open", "in_progress", "blocked", "completed", "cancelled"}},
				},
				"include_history": {Type: "boolean", Description: "Return every retained lifecycle state when statuses are omitted"},
			},
		},
	},
}

// todoAddTool describes todo_add to the model: create a task with a
// required title and optional priority, description, and due date —
// mirroring the flags of the todo CLI it shells out to.
var todoAddTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_add",
		Description: "This is a CLI command to run and add a task to the todo list",
		Parameters: openrouter.Parameter{
			Type: "object",
			Required: []string{
				"title",
			},
			Properties: map[string]openrouter.Property{
				"title": {
					Type:        "string",
					Description: "The title of the task. Should be concise and self explanatory. One sentence",
				},
				"priority": {
					Type:        "integer",
					Description: "[Optional] How important is this task. Integer between 0-5",
				},
				"description": {
					Type:        "string",
					Description: "[Optional] A long description explaining the task and potentially the steps needed or subtasks",
				},
				"due": {
					Type:        "string",
					Description: "[Optional] Date of task to be due. Format [YYYY-MM-DD]",
				},
			},
		},
	},
}

var todoGetTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_get",
		Description: "Retrieve one durable task by its stable identity",
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"task_id"},
			Properties: map[string]openrouter.Property{
				"task_id": {
					Type:        "string",
					Description: "The opaque task identity returned by todo_add or todo_list",
				},
			},
		},
	},
}

var todoUpdateTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_update",
		Description: "Update task metadata or lifecycle state using its current revision",
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"task_id", "expected_revision"},
			Properties: map[string]openrouter.Property{
				"task_id":           {Type: "string", Description: "The opaque durable task identity"},
				"expected_revision": {Type: "integer", Description: "The exact current revision returned by Todo"},
				"title":             {Type: "string", Description: "Replacement non-blank title"},
				"description":       {Type: "string", Description: "Replacement description; empty clears it"},
				"priority":          {Type: "integer", Description: "Replacement priority from one to five; zero clears it"},
				"due":               {Type: "string", Description: "Replacement YYYY-MM-DD due date; empty clears it"},
				"status": {
					Type: "string", Description: "Replacement lifecycle status",
					Enum: []string{"open", "in_progress", "blocked", "completed", "cancelled"},
				},
			},
		},
	},
}

var todoIdempotentUpdateTool = func() openrouter.Tool {
	tool := cloneSchema(todoUpdateTool)
	tool.Function.Parameters.Required = append(tool.Function.Parameters.Required, "idempotency_key")
	tool.Function.Parameters.Properties["idempotency_key"] = openrouter.Property{
		Type: "string", Description: "Caller-provided identity reused only when retrying this exact mutation",
	}
	return tool
}()

var todoClaimedUpdateTool = func() openrouter.Tool {
	tool := cloneSchema(todoIdempotentUpdateTool)
	tool.Function.Description = "Update claimed Task progress, result, metadata, or lifecycle state using its current revision"
	tool.Function.Parameters.Properties["result_summary"] = openrouter.Property{
		Type: "string", Description: "Concise durable result or progress summary; requires the caller's active Task claim",
	}
	return tool
}()

var todoClaimTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_claim",
		Description: "Claim at most one Task for this active execution before reporting progress or completing it",
		Parameters: openrouter.Parameter{
			Type: "object", Required: []string{"task_id", "idempotency_key"},
			Properties: map[string]openrouter.Property{
				"task_id":         {Type: "string", Description: "The opaque durable Task identity"},
				"idempotency_key": {Type: "string", Description: "Caller identity reused only when retrying this exact claim"},
			},
		},
	},
}

var todoReleaseTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_release",
		Description: "Release this active execution's Task claim without changing Task lifecycle state",
		Parameters: openrouter.Parameter{
			Type: "object", Required: []string{"task_id", "idempotency_key"},
			Properties: map[string]openrouter.Property{
				"task_id":         {Type: "string", Description: "The opaque durable Task identity"},
				"idempotency_key": {Type: "string", Description: "Caller identity reused only when retrying this exact release"},
			},
		},
	},
}

var todoTreeListTool = func() openrouter.Tool {
	tool := cloneSchema(todoLifecycleListTool)
	tool.Function.Description = "List durable Tasks with deterministic Task Tree filters"
	tool.Function.Parameters.Properties["root_id"] = openrouter.Property{
		Type: "string", Description: "Optional Task Tree root identity; returns that root and its descendants",
	}
	tool.Function.Parameters.Properties["parent_id"] = openrouter.Property{
		Type: "string", Description: "Optional parent Task identity; returns its direct children",
	}
	return tool
}()

var todoTreeAddTool = func() openrouter.Tool {
	tool := TodoIdempotentAddTool().Schema
	tool.Function.Description = "Create a durable Task Tree root or one child beneath an existing Task"
	tool.Function.Parameters.Properties["parent_id"] = openrouter.Property{
		Type: "string", Description: "Parent Task identity when creating a child",
	}
	tool.Function.Parameters.Properties["expected_parent_revision"] = openrouter.Property{
		Type: "integer", Description: "Exact parent revision required when parent_id is supplied",
	}
	return tool
}()

var todoScopedListTool = func() openrouter.Tool {
	tool := cloneSchema(todoTreeListTool)
	tool.Function.Description = "List durable Tasks in the current session's authorized Context Scopes"
	tool.Function.Parameters.Properties["scope"] = openrouter.Property{
		Type: "string", Description: "Optional scope view: context selects the active Workspace/project; global selects owner-wide work",
		Enum: []string{"context", "global"},
	}
	return tool
}()

var todoScopedAddTool = func() openrouter.Tool {
	tool := cloneSchema(todoTreeAddTool)
	tool.Function.Description = "Create a Task Tree in the active Context Scope, an explicitly global root, or a child inheriting its parent scope"
	tool.Function.Parameters.Properties["scope"] = openrouter.Property{
		Type: "string", Description: "Optional root scope: defaults to context; use global only for genuinely owner-wide or personal work",
		Enum: []string{"context", "global"},
	}
	return tool
}()

var todoFocusedAddTool = func() openrouter.Tool {
	tool := cloneSchema(todoScopedAddTool)
	tool.Function.Parameters.Properties["focus"] = openrouter.Property{
		Type: "boolean", Description: "Select the created Task as this session's Task Focus for subsequent turns",
	}
	return tool
}()

var todoTreeGetTool = func() openrouter.Tool {
	tool := cloneSchema(todoGetTool)
	tool.Function.Description = "Retrieve one durable Task or a bounded recursive Task Tree"
	tool.Function.Parameters.Properties["include_tree"] = openrouter.Property{
		Type: "boolean", Description: "Return the selected Task and bounded descendants as a recursive tree",
	}
	tool.Function.Parameters.Properties["max_depth"] = openrouter.Property{
		Type: "integer", Description: "Maximum descendant depth from one to 64; defaults to eight",
	}
	tool.Function.Parameters.Properties["include_history"] = openrouter.Property{
		Type: "boolean", Description: "Include completed and cancelled descendants in a tree result",
	}
	return tool
}()

var todoDecomposeTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name:        "todo_decompose",
		Description: "Atomically create an ordered batch of child Tasks beneath one parent",
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"task_id", "expected_revision", "children", "idempotency_key"},
			Properties: map[string]openrouter.Property{
				"task_id":           {Type: "string", Description: "Parent Task identity"},
				"expected_revision": {Type: "integer", Description: "Exact current parent revision"},
				"idempotency_key":   {Type: "string", Description: "Caller identity reused only when retrying this exact decomposition"},
				"children": {
					Type: "array", Description: "Ordered child Tasks to create",
					Items: &openrouter.Property{
						Type: "object", Required: []string{"title"},
						Properties: map[string]openrouter.Property{
							"title":       {Type: "string", Description: "Concise child Task title"},
							"description": {Type: "string", Description: "Optional child Task description"},
							"priority":    {Type: "integer", Description: "Optional priority from zero to five"},
							"due":         {Type: "string", Description: "Optional YYYY-MM-DD due date"},
						},
					},
				},
			},
		},
	},
}

// TodoTools returns fresh definitions for the existing model-facing Todo
// tools. The Todo first-party plugin attaches canonical Capability identities
// while preserving these schemas and execution functions unchanged.
func TodoTools() []Tool {
	return []Tool{
		{Schema: cloneSchema(todoListTool), Execute: toDoList},
		{Schema: cloneSchema(todoAddTool), Execute: toDoAdd},
	}
}

// TodoGetTool returns the durable task identity lookup schema. Unlike the two
// legacy Todo definitions, its execution behavior is supplied only by the
// first-party Todo plugin.
func TodoGetTool() Tool {
	return Tool{Schema: cloneSchema(todoGetTool)}
}

// TodoLifecycleListTool is the current history-aware Todo list definition.
// TodoTools remains frozen for legacy receipts and CLI compatibility.
func TodoLifecycleListTool() Tool { return Tool{Schema: cloneSchema(todoLifecycleListTool)} }

// TodoUpdateTool returns the lifecycle mutation schema. Execution is supplied
// only by the first-party Todo plugin.
func TodoUpdateTool() Tool { return Tool{Schema: cloneSchema(todoUpdateTool)} }

// TodoIdempotentAddTool is the current add schema. The identity remains
// optional so existing valid add calls continue to create independent Tasks.
func TodoIdempotentAddTool() Tool {
	tool := TodoTools()[1]
	tool.Schema.Function.Parameters.Properties["idempotency_key"] = openrouter.Property{
		Type: "string", Description: "Optional caller identity reused only when retrying this exact creation",
	}
	return tool
}

// TodoIdempotentUpdateTool is the current revision-checked mutation schema.
func TodoIdempotentUpdateTool() Tool { return Tool{Schema: cloneSchema(todoIdempotentUpdateTool)} }

// TodoClaimedUpdateTool is the current claim-aware progress and result schema.
func TodoClaimedUpdateTool() Tool { return Tool{Schema: cloneSchema(todoClaimedUpdateTool)} }

// TodoClaimTool acquires or confirms one Task claim for the trusted execution.
func TodoClaimTool() Tool { return Tool{Schema: cloneSchema(todoClaimTool)} }

// TodoReleaseTool releases only the trusted execution's active Task claim.
func TodoReleaseTool() Tool { return Tool{Schema: cloneSchema(todoReleaseTool)} }

// TodoTreeListTool is the hierarchy-aware current list schema.
func TodoTreeListTool() Tool { return Tool{Schema: cloneSchema(todoTreeListTool)} }

// TodoTreeAddTool is the hierarchy-aware current add schema.
func TodoTreeAddTool() Tool { return Tool{Schema: cloneSchema(todoTreeAddTool)} }

func TodoScopedListTool() Tool { return Tool{Schema: cloneSchema(todoScopedListTool)} }

func TodoScopedAddTool() Tool { return Tool{Schema: cloneSchema(todoScopedAddTool)} }

// TodoFocusedAddTool is the current create-and-focus schema. TodoScopedAddTool
// remains frozen for sessions pinned before focus was added.
func TodoFocusedAddTool() Tool { return Tool{Schema: cloneSchema(todoFocusedAddTool)} }

// TodoTreeGetTool is the bounded recursive current get schema.
func TodoTreeGetTool() Tool { return Tool{Schema: cloneSchema(todoTreeGetTool)} }

// TodoDecomposeTool is the atomic ordered child-batch schema.
func TodoDecomposeTool() Tool { return Tool{Schema: cloneSchema(todoDecomposeTool)} }

// toDoList shells out to `todo list` and returns its output verbatim for
// the model to read. Ignores args — the tool has no parameters.
func toDoList(ctx context.Context, _ string) (string, error) {
	out, err := exec.CommandContext(ctx, "todo", "list").CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("command failed: %w: %s", err, out)
	}

	return string(out), nil
}

// toDoAdd unmarshals the model's arguments and shells out to `todo add`,
// translating each optional field into the matching CLI flag. Title is
// required; a missing title or bad JSON comes back as an error for the
// dispatcher to relay to the model.
func toDoAdd(ctx context.Context, args string) (string, error) {
	var params struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Due         string `json:"due"`
		Priority    int    `json:"priority"`
	}

	err := json.Unmarshal([]byte(args), &params)
	if err != nil {
		return "", fmt.Errorf("error returning the todo add json: %w", err)
	}

	cmds := []string{"todo", "add"}
	if params.Title == "" {
		return "", fmt.Errorf("title is required to call the tool")
	}
	cmds = append(cmds, params.Title)

	if params.Priority != 0 {
		cmds = append(cmds, "--priority", fmt.Sprint(params.Priority))
	}

	if params.Description != "" {
		cmds = append(cmds, "--desc", params.Description)
	}

	if params.Due != "" {
		cmds = append(cmds, "--due", params.Due)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, cmds[0], cmds[1:]...).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("tool call failed: %w: %s", err, out)
	}

	return string(out), nil
}
