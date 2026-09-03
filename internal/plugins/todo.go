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
	TodoPluginID              PluginID     = "todo"
	TodoListCapabilityID      CapabilityID = "todo.list"
	TodoAddCapabilityID       CapabilityID = "todo.add"
	TodoGetCapabilityID       CapabilityID = "todo.get"
	TodoUpdateCapabilityID    CapabilityID = "todo.update"
	TodoDecomposeCapabilityID CapabilityID = "todo.decompose"
	TodoClaimCapabilityID     CapabilityID = "todo.claim"
	TodoReleaseCapabilityID   CapabilityID = "todo.release"

	todoImplementationVersion    = "1.6.0"
	todoContractVersion          = "1.0.0"
	todoListContractVersion      = "1.3.0"
	todoAddContractVersion       = "1.3.0"
	todoGetContractVersion       = "1.1.0"
	todoUpdateContractVersion    = "1.3.0"
	todoDecomposeContractVersion = "1.0.0"
	todoClaimContractVersion     = "1.0.0"
	todoReleaseContractVersion   = "1.0.0"
)

type Todo struct {
	service task.Service
}

type taskManagementService interface {
	ManagementUpdateGlobalTask(context.Context, task.ID, task.UpdateInput, string) (task.Task, error)
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
	idempotentAdd := tools.TodoIdempotentAddTool()
	idempotentUpdate := tools.TodoIdempotentUpdateTool()
	return Manifest{
		ID:                    TodoPluginID,
		ImplementationVersion: todoImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: todoListContractVersion},
			{ID: TodoAddCapabilityID, Version: todoAddContractVersion},
			{ID: TodoGetCapabilityID, Version: todoGetContractVersion},
			{ID: TodoUpdateCapabilityID, Version: todoUpdateContractVersion},
			{ID: TodoDecomposeCapabilityID, Version: todoDecomposeContractVersion},
			{ID: TodoClaimCapabilityID, Version: todoClaimContractVersion},
			{ID: TodoReleaseCapabilityID, Version: todoReleaseContractVersion},
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
					{ID: TodoListCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(lifecycleList.Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyTools[1].Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyGet.Schema)},
					{ID: TodoUpdateCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyUpdate.Schema)},
				},
			},
			{
				ImplementationVersion: "1.3.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(lifecycleList.Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(idempotentAdd.Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, SchemaSHA256: schemaHash(legacyGet.Schema)},
					{ID: TodoUpdateCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(idempotentUpdate.Schema)},
				},
			},
			{
				ImplementationVersion: "1.4.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: "1.2.0", SchemaSHA256: schemaHash(tools.TodoTreeListTool().Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: "1.2.0", SchemaSHA256: schemaHash(tools.TodoTreeAddTool().Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(tools.TodoTreeGetTool().Schema)},
					{ID: TodoUpdateCapabilityID, ContractVersion: "1.2.0", SchemaSHA256: schemaHash(tools.TodoIdempotentUpdateTool().Schema)},
					{ID: TodoDecomposeCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.TodoDecomposeTool().Schema)},
				},
			},
			{
				ImplementationVersion: "1.5.0",
				Capabilities: []CapabilityCompatibility{
					{ID: TodoListCapabilityID, ContractVersion: "1.2.0", SchemaSHA256: schemaHash(tools.TodoTreeListTool().Schema)},
					{ID: TodoAddCapabilityID, ContractVersion: "1.2.0", SchemaSHA256: schemaHash(tools.TodoTreeAddTool().Schema)},
					{ID: TodoGetCapabilityID, ContractVersion: "1.1.0", SchemaSHA256: schemaHash(tools.TodoTreeGetTool().Schema)},
					{ID: TodoUpdateCapabilityID, ContractVersion: "1.3.0", SchemaSHA256: schemaHash(tools.TodoClaimedUpdateTool().Schema)},
					{ID: TodoDecomposeCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.TodoDecomposeTool().Schema)},
					{ID: TodoClaimCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.TodoClaimTool().Schema)},
					{ID: TodoReleaseCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.TodoReleaseTool().Schema)},
				},
			},
		},
	}
}

func (t Todo) ToolCapabilities() []ToolCapability {
	addTool := tools.TodoScopedAddTool()
	listTool := tools.TodoScopedListTool()
	getTool := tools.TodoTreeGetTool()
	addTool.Execute = t.add
	listTool.Execute = t.list
	getTool.Execute = t.get
	updateTool := tools.TodoClaimedUpdateTool()
	updateTool.Execute = t.update
	decomposeTool := tools.TodoDecomposeTool()
	decomposeTool.Execute = t.decompose
	claimTool := tools.TodoClaimTool()
	claimTool.Execute = t.claim
	releaseTool := tools.TodoReleaseTool()
	releaseTool.Execute = t.release
	return []ToolCapability{
		{ID: TodoListCapabilityID, ContractVersion: todoListContractVersion, Tool: listTool},
		{ID: TodoAddCapabilityID, ContractVersion: todoAddContractVersion, Tool: addTool},
		{ID: TodoGetCapabilityID, ContractVersion: todoGetContractVersion, Tool: getTool},
		{ID: TodoUpdateCapabilityID, ContractVersion: todoUpdateContractVersion, Tool: updateTool},
		{ID: TodoDecomposeCapabilityID, ContractVersion: todoDecomposeContractVersion, Tool: decomposeTool},
		{ID: TodoClaimCapabilityID, ContractVersion: todoClaimContractVersion, Tool: claimTool},
		{ID: TodoReleaseCapabilityID, ContractVersion: todoReleaseContractVersion, Tool: releaseTool},
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
		capabilities[0] = ToolCapability{ID: TodoListCapabilityID, ContractVersion: "1.1.0", Tool: listTool}
		getTool := tools.TodoGetTool()
		getTool.Execute = t.get
		updateTool := tools.TodoUpdateTool()
		updateTool.Execute = t.updateLegacy
		capabilities = append(capabilities,
			ToolCapability{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, Tool: getTool},
			ToolCapability{ID: TodoUpdateCapabilityID, ContractVersion: todoContractVersion, Tool: updateTool},
		)
	}
	if version == "1.3.0" {
		listTool := tools.TodoLifecycleListTool()
		listTool.Execute = t.list
		addTool := tools.TodoIdempotentAddTool()
		addTool.Execute = t.add
		getTool := tools.TodoGetTool()
		getTool.Execute = t.get
		updateTool := tools.TodoIdempotentUpdateTool()
		updateTool.Execute = t.updateCompatibility
		capabilities = []ToolCapability{
			{ID: TodoListCapabilityID, ContractVersion: "1.1.0", Tool: listTool},
			{ID: TodoAddCapabilityID, ContractVersion: "1.1.0", Tool: addTool},
			{ID: TodoGetCapabilityID, ContractVersion: todoContractVersion, Tool: getTool},
			{ID: TodoUpdateCapabilityID, ContractVersion: "1.1.0", Tool: updateTool},
		}
	}
	if version == "1.4.0" {
		listTool := tools.TodoTreeListTool()
		listTool.Execute = t.list
		addTool := tools.TodoTreeAddTool()
		addTool.Execute = t.add
		getTool := tools.TodoTreeGetTool()
		getTool.Execute = t.get
		updateTool := tools.TodoIdempotentUpdateTool()
		updateTool.Execute = t.updateCompatibility
		decomposeTool := tools.TodoDecomposeTool()
		decomposeTool.Execute = t.decompose
		capabilities = []ToolCapability{
			{ID: TodoListCapabilityID, ContractVersion: "1.2.0", Tool: listTool},
			{ID: TodoAddCapabilityID, ContractVersion: "1.2.0", Tool: addTool},
			{ID: TodoGetCapabilityID, ContractVersion: "1.1.0", Tool: getTool},
			{ID: TodoUpdateCapabilityID, ContractVersion: "1.2.0", Tool: updateTool},
			{ID: TodoDecomposeCapabilityID, ContractVersion: "1.0.0", Tool: decomposeTool},
		}
	}
	if version == "1.5.0" {
		listTool := tools.TodoTreeListTool()
		listTool.Execute = t.list
		addTool := tools.TodoTreeAddTool()
		addTool.Execute = t.add
		getTool := tools.TodoTreeGetTool()
		getTool.Execute = t.get
		updateTool := tools.TodoClaimedUpdateTool()
		updateTool.Execute = t.update
		decomposeTool := tools.TodoDecomposeTool()
		decomposeTool.Execute = t.decompose
		claimTool := tools.TodoClaimTool()
		claimTool.Execute = t.claim
		releaseTool := tools.TodoReleaseTool()
		releaseTool.Execute = t.release
		capabilities = []ToolCapability{
			{ID: TodoListCapabilityID, ContractVersion: "1.2.0", Tool: listTool},
			{ID: TodoAddCapabilityID, ContractVersion: "1.2.0", Tool: addTool},
			{ID: TodoGetCapabilityID, ContractVersion: "1.1.0", Tool: getTool},
			{ID: TodoUpdateCapabilityID, ContractVersion: "1.3.0", Tool: updateTool},
			{ID: TodoDecomposeCapabilityID, ContractVersion: "1.0.0", Tool: decomposeTool},
			{ID: TodoClaimCapabilityID, ContractVersion: "1.0.0", Tool: claimTool},
			{ID: TodoReleaseCapabilityID, ContractVersion: "1.0.0", Tool: releaseTool},
		}
	}
	if version != "1.0.0" && version != "1.1.0" && version != "1.2.0" && version != "1.3.0" && version != "1.4.0" && version != "1.5.0" {
		return nil
	}
	for i := range capabilities {
		execute := capabilities[i].Tool.Execute
		capabilities[i].Tool.Execute = func(ctx context.Context, arguments string) (string, error) {
			return execute(task.WithGlobalScopeCompatibility(ctx), arguments)
		}
	}
	return capabilities
}

func (t Todo) add(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Title                  string              `json:"title"`
		Description            string              `json:"description"`
		DueDate                string              `json:"due"`
		Priority               int                 `json:"priority"`
		IdempotencyKey         string              `json:"idempotency_key"`
		ParentID               string              `json:"parent_id"`
		ExpectedParentRevision uint64              `json:"expected_parent_revision"`
		Scope                  task.ScopeSelection `json:"scope"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_add arguments: %w", err)
	}
	if err := task.ValidateScopeSelection(input.Scope); err != nil {
		return "", err
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	created, err := t.service.CreateGlobalTask(ctx, task.CreateInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
		ParentID: task.ID(input.ParentID), ExpectedParentRevision: input.ExpectedParentRevision,
		Scope:          input.Scope,
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
		Statuses       []task.Status       `json:"statuses"`
		IncludeHistory bool                `json:"include_history"`
		RootID         task.ID             `json:"root_id"`
		ParentID       task.ID             `json:"parent_id"`
		Scope          task.ScopeSelection `json:"scope"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_list arguments: %w", err)
	}
	if err := task.ValidateScopeSelection(input.Scope); err != nil {
		return "", err
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	tasks, err := t.service.ListGlobalTasks(ctx, task.ListFilter{
		Statuses: input.Statuses, IncludeHistory: input.IncludeHistory, RootID: input.RootID, ParentID: input.ParentID, Scope: input.Scope,
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
		TaskID         string `json:"task_id"`
		IncludeTree    bool   `json:"include_tree"`
		MaxDepth       int    `json:"max_depth"`
		IncludeHistory bool   `json:"include_history"`
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
	if input.IncludeTree {
		value, err := t.service.GetGlobalTaskTree(ctx, task.ID(input.TaskID), task.TreeQuery{
			MaxDepth: input.MaxDepth, IncludeHistory: input.IncludeHistory,
		})
		if err != nil {
			return "", err
		}
		return encodeTodoResult(value)
	}
	value, err := t.service.GetGlobalTask(ctx, task.ID(input.TaskID))
	if err != nil {
		return "", err
	}
	return encodeTodoResult(value)
}

func (t Todo) decompose(ctx context.Context, arguments string) (string, error) {
	var input struct {
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
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_decompose arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	children := make([]task.ChildInput, len(input.Children))
	for i, child := range input.Children {
		children[i] = task.ChildInput{
			Title: child.Title, Description: child.Description, Priority: child.Priority, DueDate: child.DueDate,
		}
	}
	result, err := t.service.DecomposeGlobalTask(ctx, task.ID(input.TaskID), task.DecomposeInput{
		ExpectedRevision: input.ExpectedRevision, Children: children,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(result)
}

func (t Todo) update(ctx context.Context, arguments string) (string, error) {
	return t.updateWithReason(ctx, arguments, "")
}

func (t Todo) updateCompatibility(ctx context.Context, arguments string) (string, error) {
	return t.updateWithReason(ctx, arguments, "legacy_receipt")
}

func (t Todo) updateWithReason(ctx context.Context, arguments, managementReason string) (string, error) {
	var input struct {
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
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_update arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	updateInput := task.UpdateInput{
		ExpectedRevision: input.ExpectedRevision, Title: input.Title, Description: input.Description,
		Priority: input.Priority, DueDate: input.DueDate, ResultSummary: input.ResultSummary, Status: input.Status,
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	}
	var updated task.Task
	var err error
	if managementReason == "" || (input.Status == nil && input.ResultSummary == nil) {
		updated, err = t.service.UpdateGlobalTask(ctx, task.ID(input.TaskID), updateInput)
	} else if management, ok := t.service.(taskManagementService); ok {
		updated, err = management.ManagementUpdateGlobalTask(ctx, task.ID(input.TaskID), updateInput, managementReason)
	} else {
		return "", fmt.Errorf("Todo Task management service is unavailable")
	}
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
	management, ok := t.service.(taskManagementService)
	if !ok {
		return "", fmt.Errorf("Todo Task management service is unavailable")
	}
	updated, err := management.ManagementUpdateGlobalTask(ctx, task.ID(input.TaskID), task.UpdateInput{
		ExpectedRevision: input.ExpectedRevision, Title: input.Title, Description: input.Description,
		Priority: input.Priority, DueDate: input.DueDate, Status: input.Status,
		IdempotencyKey: task.IdempotencyKey(uuid.NewString()),
	}, "legacy_receipt")
	if err != nil {
		return "", err
	}
	return encodeTodoResult(updated)
}

func (t Todo) claim(ctx context.Context, arguments string) (string, error) {
	var input struct {
		TaskID         string `json:"task_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_claim arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	claimed, err := t.service.ClaimGlobalTask(ctx, task.ID(input.TaskID), task.ClaimInput{
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(claimed)
}

func (t Todo) release(ctx context.Context, arguments string) (string, error) {
	var input struct {
		TaskID         string `json:"task_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := decodeTodoArguments(arguments, &input); err != nil {
		return "", fmt.Errorf("decode todo_release arguments: %w", err)
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return "", &task.InputError{Field: "task_id", Message: "must not be blank"}
	}
	if t.service == nil {
		return "", fmt.Errorf("Todo Task service is unavailable")
	}
	released, err := t.service.ReleaseGlobalTaskClaim(ctx, task.ID(input.TaskID), task.ReleaseInput{
		IdempotencyKey: task.IdempotencyKey(input.IdempotencyKey),
	})
	if err != nil {
		return "", err
	}
	return encodeTodoResult(released)
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
