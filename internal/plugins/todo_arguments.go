package plugins

import (
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/task"
)

type todoAddArguments struct {
	Title                  string              `json:"title"`
	Description            string              `json:"description"`
	DueDate                string              `json:"due"`
	Priority               int                 `json:"priority"`
	IdempotencyKey         string              `json:"idempotency_key"`
	ParentID               string              `json:"parent_id"`
	ExpectedParentRevision uint64              `json:"expected_parent_revision"`
	Scope                  task.ScopeSelection `json:"scope"`
	Focus                  bool                `json:"focus"`
}

func decodeTodoAddArguments(arguments string) (todoAddArguments, error) {
	var input todoAddArguments
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return todoAddArguments{}, err
	}
	return input, nil
}

func (input todoAddArguments) taskInput() task.CreateInput {
	return task.CreateInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
		ParentID: task.ID(input.ParentID), ExpectedParentRevision: input.ExpectedParentRevision,
		Scope: input.Scope, IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey), Focus: input.Focus,
	}
}

type todoUpdateArguments struct {
	TaskID           string       `json:"task_id"`
	ExpectedRevision uint64       `json:"expected_revision"`
	Title            *string      `json:"title"`
	Description      *string      `json:"description"`
	Priority         *int         `json:"priority"`
	DueDate          *string      `json:"due"`
	ResultSummary    *string      `json:"result_summary"`
	Status           *task.Status `json:"status"`
	IdempotencyKey   string       `json:"idempotency_key"`
}

func decodeTodoUpdateArguments(arguments string) (todoUpdateArguments, error) {
	var input todoUpdateArguments
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return todoUpdateArguments{}, err
	}
	return input, nil
}

func (input todoUpdateArguments) taskInput() task.UpdateInput {
	return task.UpdateInput{
		ExpectedRevision: input.ExpectedRevision, Title: input.Title, Description: input.Description,
		Priority: input.Priority, DueDate: input.DueDate, ResultSummary: input.ResultSummary, Status: input.Status,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	}
}

type todoDecomposeArguments struct {
	TaskID           string `json:"task_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Children         []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    int    `json:"priority"`
		DueDate     string `json:"due"`
	} `json:"children"`
	IdempotencyKey string `json:"idempotency_key"`
}

func decodeTodoDecomposeArguments(arguments string) (todoDecomposeArguments, error) {
	var input todoDecomposeArguments
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return todoDecomposeArguments{}, err
	}
	return input, nil
}

func (input todoDecomposeArguments) taskInput() task.DecomposeInput {
	children := make([]task.ChildInput, len(input.Children))
	for index, child := range input.Children {
		children[index] = task.ChildInput{
			Title: child.Title, Description: child.Description, Priority: child.Priority, DueDate: child.DueDate,
		}
	}
	return task.DecomposeInput{
		ExpectedRevision: input.ExpectedRevision, Children: children,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	}
}

type todoCoordinationArguments struct {
	TaskID         string `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func decodeTodoCoordinationArguments(arguments string) (todoCoordinationArguments, error) {
	var input todoCoordinationArguments
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return todoCoordinationArguments{}, err
	}
	return input, nil
}

// TodoMutationAttempt is the content-free canonical identity of one current
// Todo mutation invocation. It can be retained in evaluation fixtures without
// retaining the caller's idempotency key or tool arguments.
type TodoMutationAttempt struct {
	IdempotencySHA256 string
	RequestSHA256     string
}

// CanonicalTodoMutationAttempt interprets the current Todo wire contract with
// the same strict decoders and Task-input translations used by execution.
func CanonicalTodoMutationAttempt(
	name string,
	arguments string,
	contextScope task.Scope,
) (TodoMutationAttempt, bool, error) {
	var (
		key     task.IdempotencyKey
		request string
		err     error
	)
	switch name {
	case "todo_add":
		input, decodeErr := decodeTodoAddArguments(arguments)
		if decodeErr != nil || task.ValidateScopeSelection(input.Scope) != nil {
			return TodoMutationAttempt{}, false, nil
		}
		targetScope := contextScope
		if input.Scope == task.ScopeSelectionGlobal {
			targetScope = task.ScopeGlobal
		}
		if targetScope == "" {
			return TodoMutationAttempt{}, false, fmt.Errorf("Todo evaluation context scope is required")
		}
		key = task.IdempotencyKey(input.IdempotencyKey)
		request, err = task.CanonicalCreateRequestSHA256(input.taskInput(), targetScope)
	case "todo_update":
		input, decodeErr := decodeTodoUpdateArguments(arguments)
		if decodeErr != nil {
			return TodoMutationAttempt{}, false, nil
		}
		key = task.IdempotencyKey(input.IdempotencyKey)
		request, err = task.CanonicalUpdateRequestSHA256(task.ID(input.TaskID), input.taskInput())
	case "todo_decompose":
		input, decodeErr := decodeTodoDecomposeArguments(arguments)
		if decodeErr != nil {
			return TodoMutationAttempt{}, false, nil
		}
		key = task.IdempotencyKey(input.IdempotencyKey)
		request, err = task.CanonicalDecomposeRequestSHA256(task.ID(input.TaskID), input.taskInput())
	case "todo_claim", "todo_release":
		input, decodeErr := decodeTodoCoordinationArguments(arguments)
		if decodeErr != nil {
			return TodoMutationAttempt{}, false, nil
		}
		key = task.IdempotencyKey(input.IdempotencyKey)
		operation := task.OperationClaim
		if name == "todo_release" {
			operation = task.OperationRelease
		}
		request, err = task.CanonicalCoordinationRequestSHA256(task.ID(input.TaskID), operation)
	default:
		return TodoMutationAttempt{}, false, nil
	}
	if strings.TrimSpace(string(key)) == "" {
		return TodoMutationAttempt{}, false, nil
	}
	if err != nil {
		return TodoMutationAttempt{}, false, err
	}
	return TodoMutationAttempt{
		IdempotencySHA256: task.IdempotencySHA256(key),
		RequestSHA256:     request,
	}, true, nil
}
