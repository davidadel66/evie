package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

const (
	TodoPluginID           PluginID     = "todo"
	TodoListCapabilityID   CapabilityID = "todo.list"
	TodoAddCapabilityID    CapabilityID = "todo.add"
	TodoGetCapabilityID    CapabilityID = "todo.get"
	TodoUpdateCapabilityID CapabilityID = "todo.update"

	todoImplementationVersion   = "1.3.0"
	todoContractVersion         = "1.0.0"
	todoListContractVersion     = "1.1.0"
	todoMutationContractVersion = "1.1.0"
)

type Todo struct {
	service task.Service
}

// NewTodo binds the Kernel Task service used by every current Todo capability.
func NewTodo(service task.Service) Todo { return Todo{service: service} }

func (t Todo) Start(context.Context) error {
	if t.service == nil {
		return fmt.Errorf("Todo Task service is unavailable")
	}
	return nil
}

func (Todo) Stop(context.Context) error { return nil }

func (Todo) Manifest() Manifest {
	legacyTools := tools.TodoTools()
	legacyGet := tools.TodoGetTool()
	lifecycleList := tools.TodoLifecycleListTool()
	legacyUpdate := tools.TodoUpdateTool()
	return Manifest{
		ID:                    TodoPluginID,
		ImplementationVersion: todoImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: todoListContractVersion},
			{ID: TodoAddCapabilityID, Version: todoMutationContractVersion},
			{ID: TodoGetCapabilityID, Version: todoContractVersion},
			{ID: TodoUpdateCapabilityID, Version: todoMutationContractVersion},
		},
		ResumableFrom: []ImplementationCompatibility{
			{
				ImplementationVersion: "1.0.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[0].Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[1].Schema)},
				},
			},
			{
				ImplementationVersion: "1.1.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[0].Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[1].Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyGet.Schema)},
				},
			},
			{
				ImplementationVersion: "1.2.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: todoListContractVersion, SchemaSHA256: schemaHash(lifecycleList.Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[1].Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyGet.Schema)},
					{ID: TodoUpdateCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyUpdate.Schema)},
				},
			},
		},
	}
}

func (t Todo) ToolCapabilities() []ToolCapability {
	addTool := tools.TodoIdempotentAddTool()
	listTool := tools.TodoLifecycleListTool()
	getTool := tools.TodoGetTool()
	addTool.Execute = t.add
	listTool.Execute = t.list
	getTool.Execute = t.get
	updateTool := tools.TodoIdempotentUpdateTool()
	updateTool.Execute = t.update
	return []ToolCapability{
		{ID: TodoListCapabilityID, ContractVersion: todoListContractVersion, Tool: listTool},
		{ID: TodoAddCapabilityID, ContractVersion: todoMutationContractVersion, Tool: addTool},
		{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, Tool: getTool},
		{ID: TodoUpdateCapabilityID, ContractVersion: todoMutationContractVersion, Tool: updateTool},
	}
}

// ResumableToolCapabilities supplies exact frozen schemas with current
// plugin-owned execution for receipts pinned before an intentional evolution.
func (t Todo) ResumableToolCapabilities(version string) []ToolCapability {
	legacy := tools.TodoTools()
	legacy[0].Execute = t.listLegacy
	legacy[1].Execute = t.addLegacy
	capabilities := []ToolCapability{
		{ID: TodoListCapabilityID, ContractVersion: todoContractVersion, Tool: legacy[0]},
		{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, Tool: legacy[1]},
	}
	if version == "1.1.0" {
		getTool := tools.TodoGetTool()
		getTool.Execute = t.get
		capabilities = append(capabilities, ToolCapability{
			ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, Tool: getTool,
		})
	}
	if version == "1.2.0" {
		listTool := tools.TodoLifecycleListTool()
		listTool.Execute = t.list
		capabilities[0] = ToolCapability{ID: TodoListCapabilityID, ContractVersion: todoListContractVersion, Tool: listTool}
		getTool := tools.TodoGetTool()
		getTool.Execute = t.get
		updateTool := tools.TodoUpdateTool()
		updateTool.Execute = t.updateLegacy
		capabilities = append(capabilities,
			ToolCapability{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, Tool: getTool},
			ToolCapability{ID: TodoUpdateCapabilityID, ContractVersion: todoContractVersion, Tool: updateTool},
		)
	}
	if version != "1.0.0" && version != "1.1.0" && version != "1.2.0" {
		return nil
	}
	return capabilities
}

func (t Todo) add(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Title          string `json:"title"`
		Description    string `json:"description"`
		DueDate        string `json:"due"`
		Priority       int    `json:"priority"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_add arguments: %w", err)
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	created, err := t.service.CreateGlobalTask(ctx, task.CreateInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(created)
}

func (t Todo) addLegacy(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		DueDate     string `json:"due"`
		Priority    int    `json:"priority"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_add arguments: %w", err)
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	created, err := t.service.CreateGlobalTask(ctx, task.CreateInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
		IdempotencyKey: task.IdempotencyKey(uuid.NewString()),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(created)
}

func (t Todo) list(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Statuses       []task.Status `json:"statuses"`
		IncludeHistory bool          `json:"include_history"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_list arguments: %w", err)
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	tasks, err := t.service.ListGlobalTasks(ctx, task.ListFilter{
		Statuses: input.Statuses, IncludeHistory: input.IncludeHistory,
	})
	if err != nil {
		return "", err
	}
	if tasks == nil {
		tasks = []task.Task{}
	}
	return encodeTodoResult(tasks)
}

func (t Todo) listLegacy(ctx context.Context, arguments string) (string, error) {
	var input struct{}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_list arguments: %w", err)
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	values, err := t.service.ListOpenGlobalTasks(ctx)
	if err != nil {
		return "", err
	}
	if values == nil {
		values = []task.Task{}
	}
	return encodeTodoResult(values)
}

func (t Todo) get(ctx context.Context, arguments string) (string, error) {
	var input struct {
		TaskID string `json:"task_id"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_get arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	value, err := t.service.GetGlobalTask(ctx, task.ID(input.TaskID))
	if err != nil {
		return "", err
	}
	return encodeTodoResult(value)
}

func (t Todo) update(ctx context.Context, arguments string) (string, error) {
	var input struct {
		TaskID           string       `json:"task_id"`
		ExpectedRevision uint64       `json:"expected_revision"`
		Title            *string      `json:"title"`
		Description      *string      `json:"description"`
		Priority         *int         `json:"priority"`
		DueDate          *string      `json:"due"`
		Status           *task.Status `json:"status"`
		IdempotencyKey   string       `json:"idempotency_key"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_update arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	updated, err := t.service.UpdateGlobalTask(ctx, task.ID(input.TaskID), task.UpdateInput{
		ExpectedRevision: input.ExpectedRevision, Title: input.Title, Description: input.Description,
		Priority: input.Priority, DueDate: input.DueDate, Status: input.Status,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(updated)
}

func (t Todo) updateLegacy(ctx context.Context, arguments string) (string, error) {
	var input struct {
		TaskID           string       `json:"task_id"`
		ExpectedRevision uint64       `json:"expected_revision"`
		Title            *string      `json:"title"`
		Description      *string      `json:"description"`
		Priority         *int         `json:"priority"`
		DueDate          *string      `json:"due"`
		Status           *task.Status `json:"status"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_update arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	updated, err := t.service.UpdateGlobalTask(ctx, task.ID(input.TaskID), task.UpdateInput{
		ExpectedRevision: input.ExpectedRevision, Title: input.Title, Description: input.Description,
		Priority: input.Priority, DueDate: input.DueDate, Status: input.Status,
		IdempotencyKey: task.IdempotencyKey(uuid.NewString()),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(updated)
}

func decodeTodoArguments(arguments string, destination any) error {
	if !strings.HasPrefix(strings.TrimSpace(arguments), "{") {
		return &task.InputError{Field: "arguments", Message: "must be a JSON object"}
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return &task.InputError{Field: "arguments", Message: err.Error()}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return &task.InputError{Field: "arguments", Message: "must contain one JSON object"}
		}
		return &task.InputError{Field: "arguments", Message: err.Error()}
	}
	return nil
}

func encodeTodoResult(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Todo result: %w", err)
	}
	return string(encoded), nil
}
