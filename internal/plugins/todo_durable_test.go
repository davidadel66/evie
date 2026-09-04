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
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

type taskServiceFixture struct {
	created   task.Task
	listed    []task.Task
	create    []task.CreateInput
	get       []task.ID
	updates   []task.UpdateInput
	decompose []task.DecomposeInput
	claims    []task.ClaimInput
	releases  []task.ReleaseInput
}

func todoClaimTestContext(t *testing.T, store *eviedb.Store, holder string) context.Context {
	t.Helper()
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, memory.LeaseHolderID(holder), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: holder + "-run",
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken),
		LeaseGeneration: uint64(lease.Generation),
	})
}

func TestTodoPluginClaimsProgressResultsAndReleasesThroughSQLite(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "plugin-claim-holder", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: "plugin-claim-run",
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation),
	})
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
	createdOutcome := executeTodoToolContext(t, ctx, toolset, "todo_add", `{"title":"claimed work","idempotency_key":"claimed-root"}`)
	var created task.Task
	if createdOutcome.IsErr || json.Unmarshal([]byte(createdOutcome.Content), &created) != nil {
		t.Fatalf("created outcome = %+v", createdOutcome)
	}
	withoutClaim := executeTodoToolContext(t, ctx, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"status":"in_progress","idempotency_key":"without-claim"}`)
	if !withoutClaim.IsErr || !strings.Contains(withoutClaim.Content, "active claim is required") {
		t.Fatalf("unclaimed progress outcome = %+v", withoutClaim)
	}
	claimArgs := `{"task_id":"` + string(created.ID) + `","idempotency_key":"claim-task"}`
	claimedOutcome := executeTodoToolContext(t, ctx, toolset, "todo_claim", claimArgs)
	claimedRetry := executeTodoToolContext(t, ctx, toolset, "todo_claim", claimArgs)
	var claimed task.Claim
	if claimedOutcome.IsErr || claimedRetry.IsErr || claimedOutcome.Content != claimedRetry.Content ||
		json.Unmarshal([]byte(claimedOutcome.Content), &claimed) != nil || claimed.TaskID != created.ID {
		t.Fatalf("claim outcomes = first %+v retry %+v decoded=%+v", claimedOutcome, claimedRetry, claimed)
	}
	progressedOutcome := executeTodoToolContext(t, ctx, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"status":"in_progress","result_summary":"focused checks passing","idempotency_key":"claimed-progress"}`)
	var progressed task.Task
	if progressedOutcome.IsErr || json.Unmarshal([]byte(progressedOutcome.Content), &progressed) != nil ||
		progressed.ResultSummary != "focused checks passing" || progressed.Status != task.StatusInProgress {
		t.Fatalf("claimed progress outcome = %+v decoded=%+v", progressedOutcome, progressed)
	}
	releaseArgs := `{"task_id":"` + string(created.ID) + `","idempotency_key":"release-task"}`
	releasedOutcome := executeTodoToolContext(t, ctx, toolset, "todo_release", releaseArgs)
	releasedRetry := executeTodoToolContext(t, ctx, toolset, "todo_release", releaseArgs)
	if releasedOutcome.IsErr || releasedRetry.IsErr || releasedOutcome.Content != releasedRetry.Content {
		t.Fatalf("release outcomes = first %+v retry %+v", releasedOutcome, releasedRetry)
	}
	for _, forged := range []struct{ name, arguments string }{
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","actor_id":"forged"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","session_id":"forged"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","run_id":"forged"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","claimant":"forged"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","duration":60}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","expires_at":"never"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","holder_id":"forged"}`},
		{"todo_claim", `{"task_id":"x","idempotency_key":"x","fencing_token":99}`},
		{"todo_release", `{"task_id":"x","idempotency_key":"x","override":true}`},
		{"todo_release", `{"task_id":"x","idempotency_key":"x","claim_id":"forged"}`},
		{"todo_add", `{"title":"x","grant_id":"forged"}`},
		{"todo_add", `{"title":"x","access_level":"manage"}`},
		{"todo_list", `{"grantee_session_id":"forged"}`},
	} {
		outcome := executeTodoToolContext(t, ctx, toolset, forged.name, forged.arguments)
		if !outcome.IsErr || !strings.Contains(outcome.Content, "unknown field") {
			t.Fatalf("forged %s outcome = %+v", forged.name, outcome)
		}
	}
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
	ctx := todoClaimTestContext(t, store, "lifecycle-holder")
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
	createdOutcome := executeTodoToolContext(t, ctx, toolset, "todo_add", `{"title":"progress me"}`)
	if createdOutcome.IsErr {
		t.Fatalf("create outcome = %+v", createdOutcome)
	}
	var created task.Task
	if err := json.Unmarshal([]byte(createdOutcome.Content), &created); err != nil {
		t.Fatal(err)
	}
	claim := executeTodoToolContext(t, ctx, toolset, "todo_claim",
		`{"task_id":"`+string(created.ID)+`","idempotency_key":"claim-completion"}`)
	if claim.IsErr {
		t.Fatalf("claim outcome = %+v", claim)
	}
	updatedOutcome := executeTodoToolContext(t, ctx, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"status":"completed","description":"retained","idempotency_key":"complete-task"}`)
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
	stale := executeTodoToolContext(t, ctx, toolset, "todo_update",
		`{"task_id":"`+string(created.ID)+`","expected_revision":1,"title":"lost","idempotency_key":"stale-task"}`)
	if !stale.IsErr || !strings.Contains(stale.Content, "expected 1, current 2") {
		t.Fatalf("stale outcome = %+v", stale)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, updated) {
		t.Fatalf("stale update changed Task: %+v, %v", got, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 6 || events[5].Outcome != task.MutationRejected ||
		events[5].DiagnosticCode != task.DiagnosticRevisionConflict {
		t.Fatalf("Task events = %+v, %v", events, err)
	}
	if deleted := executeTodoTool(t, toolset, "todo_delete", `{"task_id":"`+string(created.ID)+`"}`); !deleted.IsErr || !strings.Contains(deleted.Content, "Unknown Tool Call") {
		t.Fatalf("delete capability unexpectedly exists: %+v", deleted)
	}
}

func TestTodoPluginDispatchesIdempotentMutationsThroughSQLite(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ctx := todoClaimTestContext(t, store, "idempotency-holder")
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

	createArgs := `{"title":"one effect","idempotency_key":"plugin-create"}`
	first := executeTodoToolContext(t, ctx, toolset, "todo_add", createArgs)
	retry := executeTodoToolContext(t, ctx, toolset, "todo_add", createArgs)
	if first.IsErr || retry.IsErr || first.Content != retry.Content {
		t.Fatalf("create outcomes = first %+v retry %+v", first, retry)
	}
	var created task.Task
	if err := json.Unmarshal([]byte(first.Content), &created); err != nil {
		t.Fatal(err)
	}
	conflictingCreate := executeTodoToolContext(t, ctx, toolset, "todo_add",
		`{"title":"different","idempotency_key":"plugin-create"}`)
	if !conflictingCreate.IsErr || !strings.Contains(conflictingCreate.Content, "idempotency identity was reused") {
		t.Fatalf("conflicting create outcome = %+v", conflictingCreate)
	}

	updateArgs := `{"task_id":"` + string(created.ID) + `","expected_revision":1,"status":"in_progress","idempotency_key":"plugin-update"}`
	claimed := executeTodoToolContext(t, ctx, toolset, "todo_claim",
		`{"task_id":"`+string(created.ID)+`","idempotency_key":"plugin-claim"}`)
	if claimed.IsErr {
		t.Fatalf("claim outcome = %+v", claimed)
	}
	updated := executeTodoToolContext(t, ctx, toolset, "todo_update", updateArgs)
	updatedRetry := executeTodoToolContext(t, ctx, toolset, "todo_update", updateArgs)
	if updated.IsErr || updatedRetry.IsErr || updated.Content != updatedRetry.Content {
		t.Fatalf("update outcomes = first %+v retry %+v", updated, updatedRetry)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 4 {
		t.Fatalf("idempotent dispatcher events = %+v, %v", events, err)
	}
}

func TestTodoPluginDispatchesOrderedTaskTreesThroughSQLite(t *testing.T) {
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
	rootOutcome := executeTodoTool(t, toolset, "todo_add", `{"title":"tree root","idempotency_key":"tree-root"}`)
	var root task.Task
	if rootOutcome.IsErr || json.Unmarshal([]byte(rootOutcome.Content), &root) != nil {
		t.Fatalf("root outcome = %+v", rootOutcome)
	}
	childOutcome := executeTodoTool(t, toolset, "todo_add",
		`{"title":"single child","parent_id":"`+string(root.ID)+`","expected_parent_revision":1,"idempotency_key":"single-child"}`)
	var child task.Task
	if childOutcome.IsErr || json.Unmarshal([]byte(childOutcome.Content), &child) != nil || child.ParentID != root.ID {
		t.Fatalf("child outcome = %+v child=%+v", childOutcome, child)
	}
	arguments := `{"task_id":"` + string(root.ID) + `","expected_revision":2,"children":[{"title":"research"},{"title":"verify"}],"idempotency_key":"tree-batch"}`
	first := executeTodoTool(t, toolset, "todo_decompose", arguments)
	retry := executeTodoTool(t, toolset, "todo_decompose", arguments)
	var decomposed task.Decomposition
	if first.IsErr || retry.IsErr || first.Content != retry.Content || json.Unmarshal([]byte(first.Content), &decomposed) != nil ||
		pluginTaskTitles(decomposed.Children) != "research,verify" || decomposed.Parent.Revision != 3 {
		t.Fatalf("decomposition outcomes = first %+v retry %+v decoded=%+v", first, retry, decomposed)
	}
	list := executeTodoTool(t, toolset, "todo_list", `{"root_id":"`+string(root.ID)+`"}`)
	var listed []task.Task
	if list.IsErr || json.Unmarshal([]byte(list.Content), &listed) != nil || pluginTaskTitles(listed) != "tree root,single child,research,verify" {
		t.Fatalf("tree list = %+v decoded=%+v", list, listed)
	}
	treeOutcome := executeTodoTool(t, toolset, "todo_get",
		`{"task_id":"`+string(root.ID)+`","include_tree":true,"max_depth":4}`)
	var tree task.Tree
	if treeOutcome.IsErr || json.Unmarshal([]byte(treeOutcome.Content), &tree) != nil || len(tree.Children) != 3 || tree.Truncated {
		t.Fatalf("tree get = %+v decoded=%+v", treeOutcome, tree)
	}
}

func pluginTaskTitles(values []task.Task) string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = value.Title
	}
	return strings.Join(result, ",")
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

func (s *taskServiceFixture) GetGlobalTaskTree(_ context.Context, _ task.ID, _ task.TreeQuery) (task.Tree, error) {
	return task.Tree{Task: s.created}, nil
}

func (s *taskServiceFixture) UpdateGlobalTask(_ context.Context, _ task.ID, input task.UpdateInput) (task.Task, error) {
	s.updates = append(s.updates, input)
	return s.created, nil
}

func (s *taskServiceFixture) DecomposeGlobalTask(_ context.Context, _ task.ID, input task.DecomposeInput) (task.Decomposition, error) {
	s.decompose = append(s.decompose, input)
	return task.Decomposition{}, nil
}

func (s *taskServiceFixture) ClaimGlobalTask(_ context.Context, id task.ID, input task.ClaimInput) (task.Claim, error) {
	s.claims = append(s.claims, input)
	return task.Claim{ID: "claim", TaskID: id}, nil
}

func (s *taskServiceFixture) ReleaseGlobalTaskClaim(_ context.Context, id task.ID, input task.ReleaseInput) (task.ClaimRelease, error) {
	s.releases = append(s.releases, input)
	return task.ClaimRelease{Claim: task.Claim{ID: "claim", TaskID: id}, Reason: "explicit"}, nil
}

func (s *taskServiceFixture) GetGlobalTaskClaim(_ context.Context, id task.ID) (task.Claim, bool, error) {
	return task.Claim{ID: "claim", TaskID: id}, true, nil
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
	if len(service.create) == 1 {
		service.create[0].IdempotencyKey = ""
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
		name    string
		tool    string
		args    string
		message string
	}{
		{name: "persistence identity", tool: "todo_add", args: `{"title":"x","id":"forged"}`},
		{name: "owner identity", tool: "todo_add", args: `{"title":"x","owner_id":"forged"}`},
		{name: "actor identity", tool: "todo_add", args: `{"title":"x","actor_id":"forged"}`},
		{name: "session identity", tool: "todo_add", args: `{"title":"x","session_id":"forged"}`},
		{name: "scope", tool: "todo_add", args: `{"title":"x","scope":"project"}`, message: "invalid task scope"},
		{name: "list identity", tool: "todo_list", args: `{"owner_id":"forged"}`},
		{name: "get extra identity", tool: "todo_get", args: `{"task_id":"x","session_id":"forged"}`},
		{name: "update actor", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"actor_id":"forged","title":"x"}`},
		{name: "update run", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"run_id":"forged","title":"x"}`},
		{name: "update event", tool: "todo_update", args: `{"task_id":"x","expected_revision":1,"event_id":"forged","title":"x"}`},
		{name: "list run", tool: "todo_list", args: `{"run_id":"forged"}`},
		{name: "decompose actor", tool: "todo_decompose", args: `{"task_id":"x","expected_revision":1,"children":[{"title":"child"}],"idempotency_key":"key","actor_id":"forged"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := executeTodoTool(t, toolset, tt.tool, tt.args)
			message := tt.message
			if message == "" {
				message = "unknown field"
			}
			if !outcome.IsErr || !strings.Contains(outcome.Content, message) {
				t.Fatalf("forged input outcome = %+v", outcome)
			}
		})
	}
	if len(service.create) != 0 || len(service.get) != 0 || len(service.decompose) != 0 {
		t.Fatalf("forged input reached service: create %#v get %#v decompose %#v", service.create, service.get, service.decompose)
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

func (missingTaskService) GetGlobalTaskTree(_ context.Context, id task.ID, _ task.TreeQuery) (task.Tree, error) {
	return task.Tree{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) UpdateGlobalTask(_ context.Context, id task.ID, _ task.UpdateInput) (task.Task, error) {
	return task.Task{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) DecomposeGlobalTask(_ context.Context, id task.ID, _ task.DecomposeInput) (task.Decomposition, error) {
	return task.Decomposition{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) ClaimGlobalTask(_ context.Context, id task.ID, _ task.ClaimInput) (task.Claim, error) {
	return task.Claim{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) ReleaseGlobalTaskClaim(_ context.Context, id task.ID, _ task.ReleaseInput) (task.ClaimRelease, error) {
	return task.ClaimRelease{}, &task.NotFoundError{ID: id}
}

func (missingTaskService) GetGlobalTaskClaim(_ context.Context, id task.ID) (task.Claim, bool, error) {
	return task.Claim{}, false, &task.NotFoundError{ID: id}
}

func (missingTaskService) ListTaskEvents(context.Context, task.ID) ([]task.Event, error) {
	return nil, nil
}
