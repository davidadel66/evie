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
	"github.com/davidadel66/evie/internal/tools"
)

func TestTodoManifestAndToolContractsAreStable(t *testing.T) {
	todo := NewTodo()
	want := Manifest{
		ID:                    TodoPluginID,
		ImplementationVersion: "1.0.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: "1.0.0"},
			{ID: TodoAddCapabilityID, Version: "1.0.0"},
		},
	}
	if got := todo.Manifest(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Todo manifest\n got: %+v\nwant: %+v", got, want)
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
	}
	if !reflect.DeepEqual(gotAssociations, wantAssociations) {
		t.Fatalf("Todo Capability associations\n got: %+v\nwant: %+v", gotAssociations, wantAssociations)
	}

	manager, err := NewManager(tools.NewToolset(nil), todo)
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
	if got, wantSchemas := toolset.Schemas(), todoSchemasNamed(tools.BuiltinToolset(), "todo_list", "todo_add"); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Todo plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"todo_list", "todo_add"}) {
		t.Fatalf("Todo schema names = %v", got)
	}
}

func TestStandardPresetRequiresTodoAndRestoresItAfterReenable(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo())
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
	dir := t.TempDir()
	command := filepath.Join(dir, "todo")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf started > \"$TODO_STARTED\"\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manager, err := NewManager(tools.NewToolset(nil), NewTodo())
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

	for _, call := range []openrouter.ToolCall{
		{ID: "list", Type: "function", Function: openrouter.FunctionCall{Name: "todo_list", Arguments: `{}`}},
		{ID: "add", Type: "function", Function: openrouter.FunctionCall{Name: "todo_add", Arguments: `{"title":"cancel me"}`}},
	} {
		t.Run(call.ID, func(t *testing.T) {
			started := filepath.Join(t.TempDir(), "started")
			t.Setenv("TODO_STARTED", started)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() {
				_, _, err := toolset.ExecuteWithApprovalAuthorizedCompletion(ctx, call, nil, nil, nil, nil)
				done <- err
			}()

			deadline := time.Now().Add(time.Second)
			for {
				if _, err := os.Stat(started); err == nil {
					break
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if time.Now().After(deadline) {
					t.Fatal("Todo subprocess did not reach the execution barrier")
				}
				time.Sleep(time.Millisecond)
			}

			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Todo dispatcher error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Todo subprocess did not stop after parent cancellation")
			}
		})
	}
}

type todoToolOutcome struct {
	Content string
	IsErr   bool
}

func TestTodoManagerToolsetPreservesLegacyBehavior(t *testing.T) {
	manager, err := NewManager(tools.NewToolset(nil), NewTodo())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(TodoPluginID, true); err != nil {
		t.Fatal(err)
	}
	managerToolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}

	legacyOutcomes, legacyCalls := exerciseTodoToolset(t, tools.BuiltinToolset())
	composedOutcomes, composedCalls := exerciseTodoToolset(t, managerToolset)
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
	if !reflect.DeepEqual(composedOutcomes, legacyOutcomes) {
		t.Fatalf("Manager-composed Todo behavior\n got: %#v\nlegacy: %#v", composedOutcomes, legacyOutcomes)
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
	if composedCalls != legacyCalls {
		t.Fatalf("Manager-composed Todo CLI calls = %q, legacy = %q", composedCalls, legacyCalls)
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
