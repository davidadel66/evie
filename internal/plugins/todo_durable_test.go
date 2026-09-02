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
	updates []task.UpdateInput
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

func TestTodoPluginUpdatesLifecycleListsHistoryAndReportsStaleRevision(t *testing.T) {
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
	createdOutcome := executeTodoTool(t, toolset, "todo_add", `{"title":"progress me"}`)
	if createdOutcome.IsErr {
		t.Fatalf("create outcome = %+v", createdOutcome)
	}
	var created task.Task
	if err := json.Unmarshal([]byte(createdOutcome.Content), &created); err != nil {
		t.Fatal(err)
	}
	updatedOutcome := executeTodoTool(t, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"status":"completed","description":"retained"}`)
	if updatedOutcome.IsErr {
		t.Fatalf("update outcome = %+v", updatedOutcome)
	}
	var updated task.Task
	if err := json.Unmarshal([]byte(updatedOutcome.Content), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status != task.StatusCompleted || updated.Revision != 2 || updated.Description != "retained" {
		t.Fatalf("updated Task = %+v", updated)
	}
	if outcome := executeTodoTool(t, toolset, "todo_list", `{}`); outcome.IsErr || outcome.Content != `[]` {
		t.Fatalf("default list outcome = %+v", outcome)
	}
	history := executeTodoTool(t, toolset, "todo_list", `{"statuses":["completed"]}`)
	var listed []task.Task
	if history.IsErr || json.Unmarshal([]byte(history.Content), &listed) != nil || !reflect.DeepEqual(listed, []task.Task{updated}) {
		t.Fatalf("history outcome = %+v decoded=%+v", history, listed)
	}
	allHistory := executeTodoTool(t, toolset, "todo_list", `{"include_history":true}`)
	if allHistory.IsErr || allHistory.Content != history.Content {
		t.Fatalf("all-history outcome = %+v, want %+v", allHistory, history)
	}
	invalidFilter := executeTodoTool(t, toolset, "todo_list", `{"statuses":["forged"]}`)
	if !invalidFilter.IsErr || !strings.Contains(invalidFilter.Content, "invalid status") {
		t.Fatalf("invalid filter outcome = %+v", invalidFilter)
	}
	stale := executeTodoTool(t, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"title":"lost"}`)
	if !stale.IsErr || !strings.Contains(stale.Content, "expected 1, current 2") {
		t.Fatalf("stale outcome = %+v", stale)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, updated) {
		t.Fatalf("stale update changed Task: %+v, %v", got, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 3 || events[2].Outcome != task.MutationRejected ||
		events[2].DiagnosticCode != task.DiagnosticRevisionConflict {
		t.Fatalf("Task events = %+v, %v", events, err)
	}
	if deleted := executeTodoTool(t, toolset, "todo_delete", `{"task_id":"`+string(created.ID)+`"}`); !deleted.IsErr || !strings.Contains(deleted.Content, "Unknown Tool Call") {
		t.Fatalf("delete capability unexpectedly exists: %+v", deleted)
	}
}

func (s *taskServiceFixture) CreateGlobalTask(_ context.Context, input task.CreateInput) (task.Task, error) {
	s.create = append(s.create, input)
	return s.created, nil
}

func (s *taskServiceFixture) ListOpenGlobalTasks(context.Context) ([]task.Task, error) {
	return append([]task.Task(nil), s.listed...), nil
}

func (s *taskServiceFixture) ListGlobalTasks(context.Context, task.ListFilter) ([]task.Task, error) {
	return append([]task.Task(nil), s.listed...), nil
}

func (s *taskServiceFixture) GetGlobalTask(_ context.Context, id task.ID) (task.Task, error) {
	s.get = append(s.get, id)
	return s.created, nil
}

func (s *taskServiceFixture) UpdateGlobalTask(_ context.Context, _ task.ID, input task.UpdateInput) (task.Task, error) {
	s.updates = append(s.updates, input)
	return s.created, nil
}

func (s *taskServiceFixture) ListTaskEvents(context.Context, task.ID) ([]task.Event, error) {
	return nil, nil
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
		{name: "update actor", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"actor_id":"forged","title":"x"}`},
		{name: "update run", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"run_id":"forged","title":"x"}`},
		{name: "update event", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"event_id":"forged","title":"x"}`},
		{name: "list run", tool: "todo_list", args: `{"run_id":"forged"}`},
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

func (missingTaskService) ListGlobalTasks(context.Context, task.ListFilter) ([]task.Task, error) {
	return nil, nil
}

func (missingTaskService) GetGlobalTask(_ context.Context, id task.ID) (task.Task, error) {
	return task.Task{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) UpdateGlobalTask(_ context.Context, id task.ID, _ task.UpdateInput) (task.Task, error) {
	return task.Task{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) ListTaskEvents(context.Context, task.ID) ([]task.Event, error) {
	return nil, nil
}
