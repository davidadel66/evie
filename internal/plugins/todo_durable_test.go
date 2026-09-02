package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

type taskServiceFixture struct {
	created task.Task
	listed  []task.Task
	create  []task.CreateInput
	get     []task.ID
}

func TestTodoPluginRealSQLiteManagerPathCreatesListsAndGetsExactlyOnce(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager, err := NewManager(tools.NewToolset(nil), NewTodo(store))
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

	invalid := executeTodoTool(t, toolset, "todo_add", `{"title":"invalid","priority":6}`)
	if !invalid.IsErr || !strings.Contains(invalid.Content, "invalid task priority") {
		t.Fatalf("invalid Todo outcome = %+v", invalid)
	}
	createdOutcome := executeTodoTool(t, toolset, "todo_add", `{"title":"durable","description":"one row","priority":3,"due":"2026-09-03"}`)
	if createdOutcome.IsErr {
		t.Fatalf("create Todo outcome = %+v", createdOutcome)
	}
	var created task.Task
	if err := json.Unmarshal([]byte(createdOutcome.Content), &created); err != nil {
		t.Fatal(err)
	}
	listOutcome := executeTodoTool(t, toolset, "todo_list", `{}`)
	getOutcome := executeTodoTool(t, toolset, "todo_get", `{"task_id":"`+string(created.ID)+`"}`)
	if listOutcome.IsErr || getOutcome.IsErr {
		t.Fatalf("read Todo outcomes = list %+v get %+v", listOutcome, getOutcome)
	}
	var listed []task.Task
	var got task.Task
	if err := json.Unmarshal([]byte(listOutcome.Content), &listed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(getOutcome.Content), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listed, []task.Task{created}) || !reflect.DeepEqual(got, created) {
		t.Fatalf("durable Todo reads = listed %#v got %#v created %#v", listed, got, created)
	}
}

func (s *taskServiceFixture) CreateGlobalTask(_ context.Context, input task.CreateInput) (task.Task, error) {
	s.create = append(s.create, input)
	return s.created, nil
}

func (s *taskServiceFixture) ListOpenGlobalTasks(context.Context) ([]task.Task, error) {
	return append([]task.Task(nil), s.listed...), nil
}

func (s *taskServiceFixture) GetGlobalTask(_ context.Context, id task.ID) (task.Task, error) {
	s.get = append(s.get, id)
	return s.created, nil
}

func TestTodoPluginDispatchesDurableTaskServiceWithoutShell(t *testing.T) {
	wantTask := task.Task{
		ID: "opaque-task", Scope: task.ScopeGlobal, Title: "durable task",
		Description: "through the plugin", Priority: 4, DueDate: "2026-09-03",
		Status: task.StatusOpen, Revision: 1,
		CreatedAt: time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC),
	}
	service := &taskServiceFixture{created: wantTask, listed: []task.Task{wantTask}}
	t.Setenv("PATH", t.TempDir())
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

	add := executeTodoTool(t, toolset, "todo_add", `{"title":"durable task","priority":4,"description":"through the plugin","due":"2026-09-03"}`)
	list := executeTodoTool(t, toolset, "todo_list", `{}`)
	get := executeTodoTool(t, toolset, "todo_get", `{"task_id":"opaque-task"}`)
	if add.IsErr || list.IsErr || get.IsErr {
		t.Fatalf("durable Todo outcomes = add %+v list %+v get %+v", add, list, get)
	}
	encodedTask, _ := json.Marshal(wantTask)
	encodedList, _ := json.Marshal([]task.Task{wantTask})
	if add.Content != string(encodedTask) || get.Content != string(encodedTask) || list.Content != string(encodedList) {
		t.Fatalf("durable Todo output = add %q list %q get %q", add.Content, list.Content, get.Content)
	}
	wantInput := task.CreateInput{
		Title: "durable task", Description: "through the plugin", Priority: 4, DueDate: "2026-09-03",
	}
	if !reflect.DeepEqual(service.create, []task.CreateInput{wantInput}) ||
		!reflect.DeepEqual(service.get, []task.ID{"opaque-task"}) {
		t.Fatalf("service calls = create %#v get %#v", service.create, service.get)
	}
}

func TestTodoPluginRejectsForgedIdentitiesAndUnknownInputBeforeService(t *testing.T) {
	service := &taskServiceFixture{}
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

	tests := []struct {
		name string
		tool string
		args string
	}{
		{name: "persistence identity", tool: "todo_add", args: `{"title":"x","id":"forged"}`},
		{name: "owner identity", tool: "todo_add", args: `{"title":"x","owner_id":"forged"}`},
		{name: "actor identity", tool: "todo_add", args: `{"title":"x","actor_id":"forged"}`},
		{name: "session identity", tool: "todo_add", args: `{"title":"x","session_id":"forged"}`},
		{name: "scope", tool: "todo_add", args: `{"title":"x","scope":"project"}`},
		{name: "list identity", tool: "todo_list", args: `{"owner_id":"forged"}`},
		{name: "get extra identity", tool: "todo_get", args: `{"task_id":"x","session_id":"forged"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := executeTodoTool(t, toolset, tt.tool, tt.args)
			if !outcome.IsErr || !strings.Contains(outcome.Content, "unknown field") {
				t.Fatalf("forged input outcome = %+v", outcome)
			}
		})
	}
	if len(service.create) != 0 || len(service.get) != 0 {
		t.Fatalf("forged input reached service: create %#v get %#v", service.create, service.get)
	}
}

func TestTodoGetMissingIdentityIsModelVisibleTypedError(t *testing.T) {
	service := missingTaskService{}
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
	outcome := executeTodoTool(t, toolset, "todo_get", `{"task_id":"missing"}`)
	if !outcome.IsErr || !strings.Contains(outcome.Content, `task "missing" not found`) {
		t.Fatalf("missing Task outcome = %+v", outcome)
	}
}

type missingTaskService struct{}

func (missingTaskService) CreateGlobalTask(context.Context, task.CreateInput) (task.Task, error) {
	return task.Task{}, nil
}

func (missingTaskService) ListOpenGlobalTasks(context.Context) ([]task.Task, error) { return nil, nil }

func (missingTaskService) GetGlobalTask(_ context.Context, id task.ID) (task.Task, error) {
	return task.Task{}, &task.NotFoundError{ID: id}
}
