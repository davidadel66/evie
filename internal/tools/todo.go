package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/davidadel66/moussa/internal/openrouter"
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

// toDoList shells out to `todo list` and returns its output verbatim for
// the model to read. Ignores args — the tool has no parameters.
func toDoList(_ string) (string, error) {
	out, err := exec.Command("todo", "list").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w: %s", err, out)
	}

	return string(out), nil
}

// toDoAdd unmarshals the model's arguments and shells out to `todo add`,
// translating each optional field into the matching CLI flag. Title is
// required; a missing title or bad JSON comes back as an error for the
// dispatcher to relay to the model.
func toDoAdd(args string) (string, error) {
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
	out, err := exec.Command(cmds[0], cmds[1:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tool call failed: %w: %s", err, out)
	}

	return string(out), nil
}
