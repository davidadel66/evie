package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

func TestTodoManifestAndToolContractsAreStable(t *testing.T) {
	todo := NewTodo(nil)
	want := Manifest{
		ID:                    TodoPluginID,
		ImplementationVersion: "1.1.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: "1.0.0"},
			{ID: TodoAddCapabilityID, Version: "1.0.0"},
			{ID: TodoGetCapabilityID, Version: "1.0.0"},
		},
		ResumableFrom: []ImplementationCompatibility{{
			ImplementationVersion: "1.0.0",
			Capabilities: []CapabilityCompatibility{
				{ID: TodoListCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: "1067181de346c6ec2da9f9fe91b365d9502bfa559e9edc31e9a40c22efcd41ca"},
				{ID: TodoAddCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: "b96146bbfb232ae6217946e7149c0781d8f1bbe923456d7230ed5fc8f270655c"},
			},
		}},
	}
	if got := todo.Manifest(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Todo manifest\n got: %+v\nwant: %+v", got, want)
	}
	if err := todo.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "service is unavailable") {
		t.Fatalf("Todo without Task service started: %v", err)
	}
	type association struct {
		ID              CapabilityID
		ContractVersion string
		SchemaName      string
	}
	capabilities := todo.ToolCapabilities()
	gotAssociations := make([]association, len(capabilities))
	for i, capability := range capabilities {
		gotAssociations[i] = association{
			ID: capability.ID, ContractVersion: capability.ContractVersion,
			SchemaName: capability.Tool.Schema.Function.Name,
		}
	}
	wantAssociations := []association{
		{ID: TodoListCapabilityID, ContractVersion: "1.0.0", SchemaName: "todo_list"},
		{ID: TodoAddCapabilityID, ContractVersion: "1.0.0", SchemaName: "todo_add"},
		{ID: TodoGetCapabilityID, ContractVersion: "1.0.0", SchemaName: "todo_get"},
	}
	if !reflect.DeepEqual(gotAssociations, wantAssociations) {
		t.Fatalf("Todo Capability associations\n got: %+v\nwant: %+v", gotAssociations, wantAssociations)
	}

	manager, err := NewManager(tools.NewToolset(nil), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(TodoPluginID, true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	wantDefinitions := append(tools.TodoTools(), tools.TodoGetTool())
	if got, wantSchemas := toolset.Schemas(), tools.NewToolset(wantDefinitions).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Todo plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"todo_list", "todo_add", "todo_get"}) {
		t.Fatalf("Todo schema names = %v", got)
	}
}

func TestStandardPresetRequiresTodoAndRestoresItAfterReenable(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}

	resolved, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"todo_list", "todo_add"} {
		if countSchema(resolved.Toolset, name) != 1 {
			t.Fatalf("standard exposes %q %d times", name, countSchema(resolved.Toolset, name))
		}
	}

	if err := manager.SetEnabled(TodoPluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePreset(StandardPresetID); err == nil ||
		!strings.Contains(err.Error(), `required Capability "todo.list" is unavailable`) {
		t.Fatalf("disabled Todo resolved standard: %v", err)
	}
	if err := manager.SetEnabled(TodoPluginID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePreset(StandardPresetID); err != nil {
		t.Fatalf("re-enabled Todo did not restore standard: %v", err)
	}
}

func TestTodoManagerToolsetPreservesCancellation(t *testing.T) {
	for _, call := range []openrouter.ToolCall{
		{ID: "list", Type: "function", Function: openrouter.FunctionCall{Name: "todo_list", Arguments: `{}`}},
		{ID: "add", Type: "function", Function: openrouter.FunctionCall{Name: "todo_add", Arguments: `{"title":"cancel me"}`}},
		{ID: "get", Type: "function", Function: openrouter.FunctionCall{Name: "todo_get", Arguments: `{"task_id":"cancel-me"}`}},
	} {
		t.Run(call.ID, func(t *testing.T) {
			service := cancelingTaskService{started: make(chan struct{})}
			manager, err := NewManager(tools.NewToolset(nil), NewTodo(service))
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.SetEnabled(TodoPluginID, true); err != nil {
				t.Fatal(err)
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				_, _, err := toolset.ExecuteWithApprovalAuthorizedCompletion(ctx, call, nil, nil, nil, nil)
				done <- err
			}()

			<-service.started

			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Todo dispatcher error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Todo service did not stop after parent cancellation")
			}
		})
	}
}

type cancelingTaskService struct {
	started chan struct{}
}

func (s cancelingTaskService) wait(ctx context.Context) error {
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func (s cancelingTaskService) CreateGlobalTask(ctx context.Context, _ task.CreateInput) (task.Task, error) {
	return task.Task{}, s.wait(ctx)
}

func (s cancelingTaskService) ListOpenGlobalTasks(ctx context.Context) ([]task.Task, error) {
	return nil, s.wait(ctx)
}

func (s cancelingTaskService) GetGlobalTask(ctx context.Context, _ task.ID) (task.Task, error) {
	return task.Task{}, s.wait(ctx)
}

type todoToolOutcome struct {
	Content string
	IsErr   bool
}

func TestFrozenLegacyTodoToolsetPreservesCLIBehavior(t *testing.T) {
	legacyOutcomes, legacyCalls := exerciseTodoToolset(t, tools.BuiltinToolset())
	wantOutcomes := map[string]todoToolOutcome{
		"list": {
			Content: "[ ] #7 preserve Todo behavior\n",
		},
		"add": {
			Content: "Added: preserve Todo behavior\n",
		},
		"malformed": {
			Content: "tool call came back with error error returning the todo add json: unexpected end of JSON input",
			IsErr:   true,
		},
		"missing title": {
			Content: "tool call came back with error title is required to call the tool",
			IsErr:   true,
		},
		"command failure": {
			Content: "tool call came back with error tool call failed: exit status 9: cannot add\n",
			IsErr:   true,
		},
	}
	if !reflect.DeepEqual(legacyOutcomes, wantOutcomes) {
		t.Fatalf("legacy Todo behavior\n got: %#v\nwant: %#v", legacyOutcomes, wantOutcomes)
	}
	wantCalls := strings.Join([]string{
		"list",
		"add|preserve Todo behavior|--priority|3|--desc|two steps|--due|2026-09-03",
		"add|fail",
		"",
	}, "\n")
	if legacyCalls != wantCalls {
		t.Fatalf("legacy Todo CLI calls = %q, want %q", legacyCalls, wantCalls)
	}
}

func exerciseTodoToolset(t *testing.T, toolset tools.Toolset) (map[string]todoToolOutcome, string) {
	t.Helper()
	dir := t.TempDir()
	command := filepath.Join(dir, "todo")
	script := `#!/bin/sh
command=$1
printf '%s' "$command" >> "$TODO_CALLS"
shift
for arg in "$@"; do
	printf '|%s' "$arg" >> "$TODO_CALLS"
done
printf '\n' >> "$TODO_CALLS"
case "$command" in
	list)
		printf '[ ] #7 preserve Todo behavior\n'
		;;
	add)
		if [ "$1" = fail ]; then
			printf 'cannot add\n' >&2
			exit 9
		fi
		printf 'Added: %s\n' "$1"
		;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	callsPath := filepath.Join(dir, "calls")
	t.Setenv("TODO_CALLS", callsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	outcomes := map[string]todoToolOutcome{
		"list": executeTodoTool(t, toolset, "todo_list", `{}`),
		"add": executeTodoTool(t, toolset, "todo_add",
			`{"title":"preserve Todo behavior","priority":3,"description":"two steps","due":"2026-09-03"}`),
		"malformed":     executeTodoTool(t, toolset, "todo_add", `{"title":`),
		"missing title": executeTodoTool(t, toolset, "todo_add", `{}`),
		"command failure": executeTodoTool(t, toolset, "todo_add",
			`{"title":"fail"}`),
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	return outcomes, string(calls)
}

func executeTodoTool(t *testing.T, toolset tools.Toolset, name, arguments string) todoToolOutcome {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), openrouter.ToolCall{
			ID: "call-todo", Type: "function",
			Function: openrouter.FunctionCall{Name: name, Arguments: arguments},
		}, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("execute %q: %v", name, err)
	}
	return todoToolOutcome{Content: message.Content, IsErr: isErr}
}

func todoSchemasNamed(toolset tools.Toolset, names ...string) []openrouter.Tool {
	selected := make([]openrouter.Tool, 0, len(names))
	for _, name := range names {
		for _, schema := range toolset.Schemas() {
			if schema.Function.Name == name {
				selected = append(selected, schema)
				break
			}
		}
	}
	return selected
}
