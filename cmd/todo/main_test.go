package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

func TestTodoCLIProcessImportsOnceAndNeverWritesLegacyJSON(t *testing.T) {
	binary := buildTodoBinary(t)
	home := t.TempDir()
	legacyPath := filepath.Join(home, ".todo", "DayToDay.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"Tasks":[{"ID":7,"Title":"legacy open","Description":"old description","CreatedAt":"2026-01-01T00:00:00Z","Priority":2,"Status":false,"DueDate":"2026-12-01T00:00:00Z"},{"ID":8,"Title":"legacy done","Description":"","CreatedAt":"2026-01-02T00:00:00Z","Priority":0,"Status":true,"DueDate":"0001-01-01T00:00:00Z"}],"NextID":9}`)
	if err := os.WriteFile(legacyPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exit := runTodoProcess(t, binary, home, "list", "--history")
	if exit != 0 || stderr != "" || !strings.Contains(stdout, `status=open`) ||
		!strings.Contains(stdout, `priority=2 due=2026-12-01`) || !strings.Contains(stdout, `title="legacy open"`) ||
		!strings.Contains(stdout, `status=completed`) || !strings.Contains(stdout, `title="legacy done"`) {
		t.Fatalf("first list exit=%d\nstdout=%s\nstderr=%s", exit, stdout, stderr)
	}
	firstIDs := renderedIDs(stdout)
	if len(firstIDs) != 2 || firstIDs[0] == firstIDs[1] {
		t.Fatalf("imported IDs = %#v", firstIDs)
	}
	assertFileBytes(t, legacyPath, legacy)

	second, stderr, exit := runTodoProcess(t, binary, home, "list", "--history")
	if exit != 0 || stderr != "" || second != stdout {
		t.Fatalf("restart list exit=%d\nstdout=%s\nstderr=%s\nwant=%s", exit, second, stderr, stdout)
	}
	added, stderr, exit := runTodoProcess(t, binary, home, "add", "--priority", "4", "--due", "2026-12-31", "new shared task")
	if exit != 0 || stderr != "" || len(renderedIDs(added)) != 1 {
		t.Fatalf("add exit=%d stdout=%s stderr=%s", exit, added, stderr)
	}
	addedID := renderedIDs(added)[0]
	_, stderr, exit = runTodoProcess(t, binary, home, "delete", addedID)
	if exit != 0 || !strings.Contains(stderr, "deprecated") {
		t.Fatalf("delete exit=%d stderr=%s", exit, stderr)
	}
	cancelled, stderr, exit := runTodoProcess(t, binary, home, "list", "--status", "cancelled")
	if exit != 0 || stderr != "" || !strings.Contains(cancelled, "id="+addedID) || !strings.Contains(cancelled, "status=cancelled") {
		t.Fatalf("cancelled list exit=%d stdout=%s stderr=%s", exit, cancelled, stderr)
	}
	assertFileBytes(t, legacyPath, legacy)

	// Once the durable migration record exists, even a later-corrupted source
	// is neither read as a fallback nor rewritten.
	corrupted := []byte("not json")
	if err := os.WriteFile(legacyPath, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "get", addedID)
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "status=cancelled") {
		t.Fatalf("post-migration get exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	assertFileBytes(t, legacyPath, corrupted)
	if _, err := os.Stat(filepath.Join(home, "ignored", "Other.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy override file exists: %v", err)
	}
}

func TestTodoCLIProcessRendersTreesFiltersAndLifecycle(t *testing.T) {
	binary := buildTodoBinary(t)
	home := t.TempDir()
	stdout, stderr, exit := runTodoProcess(t, binary, home, "help")
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "durable Task Trees") || !strings.Contains(stdout, "deprecated") {
		t.Fatalf("help exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".evie")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("help created persistence: %v", err)
	}
	for _, args := range [][]string{{}, {"wat"}, {"add"}, {"list", "extra"}, {"add", "--priority", "nope", "x"},
		{"list", "--history", "--status", "open"}, {"release-claim", "missing"}} {
		stdout, stderr, exit = runTodoProcess(t, binary, home, args...)
		if exit != 2 || stdout != "" || !strings.Contains(stderr, "todo") {
			t.Fatalf("usage %q exit=%d stdout=%s stderr=%s", args, exit, stdout, stderr)
		}
	}

	firstRetry, stderr, exit := runTodoProcess(t, binary, home, "add", "--idempotency-key", "stable-retry", "retry once")
	if exit != 0 || stderr != "" {
		t.Fatalf("first retry add exit=%d stdout=%s stderr=%s", exit, firstRetry, stderr)
	}
	secondRetry, stderr, exit := runTodoProcess(t, binary, home, "add", "--idempotency-key", "stable-retry", "retry once")
	if exit != 0 || stderr != "" || secondRetry != firstRetry {
		t.Fatalf("replayed add exit=%d stdout=%s stderr=%s want=%s", exit, secondRetry, stderr, firstRetry)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--idempotency-key", "stable-retry", "different request")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "idempotency identity was reused") {
		t.Fatalf("conflicting retry exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	retryID := renderedIDs(firstRetry)[0]
	stdout, stderr, exit = runTodoProcess(t, binary, home, "update", "--idempotency-key", "update-retry", "--title", "updated once", retryID)
	if exit != 2 || stdout != "" || !strings.Contains(stderr, "--revision is required with --idempotency-key") {
		t.Fatalf("ambiguous update retry exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	firstUpdate, stderr, exit := runTodoProcess(t, binary, home, "update", "--revision", "1", "--idempotency-key", "update-retry", "--title", "updated once", retryID)
	if exit != 0 || stderr != "" {
		t.Fatalf("first update retry exit=%d stdout=%s stderr=%s", exit, firstUpdate, stderr)
	}
	secondUpdate, stderr, exit := runTodoProcess(t, binary, home, "update", "--revision", "1", "--idempotency-key", "update-retry", "--title", "updated once", retryID)
	if exit != 0 || stderr != "" || secondUpdate != firstUpdate {
		t.Fatalf("replayed update exit=%d stdout=%s stderr=%s want=%s", exit, secondUpdate, stderr, firstUpdate)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "start", "--idempotency-key", "start-retry", retryID)
	if exit != 2 || stdout != "" || !strings.Contains(stderr, "--revision is required with --idempotency-key") {
		t.Fatalf("ambiguous lifecycle retry exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	firstStart, stderr, exit := runTodoProcess(t, binary, home, "start", "--revision", "2", "--idempotency-key", "start-retry", retryID)
	if exit != 0 || stderr != "" {
		t.Fatalf("first lifecycle retry exit=%d stdout=%s stderr=%s", exit, firstStart, stderr)
	}
	secondStart, stderr, exit := runTodoProcess(t, binary, home, "start", "--revision", "2", "--idempotency-key", "start-retry", retryID)
	if exit != 0 || stderr != "" || secondStart != firstStart {
		t.Fatalf("replayed lifecycle exit=%d stdout=%s stderr=%s want=%s", exit, secondStart, stderr, firstStart)
	}

	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--priority", "4", "--due", "2026-10-02", "--desc", "root details", "root")
	if exit != 0 || stderr != "" {
		t.Fatalf("root add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	rootID := renderedIDs(stdout)[0]
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--parent", rootID, "--parent-revision", "1", "child")
	if exit != 0 || stderr != "" {
		t.Fatalf("child add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	childID := renderedIDs(stdout)[0]
	tree, stderr, exit := runTodoProcess(t, binary, home, "get", "--tree", rootID)
	if exit != 0 || stderr != "" || !strings.Contains(tree, "id="+rootID+" parent=- root="+rootID) ||
		!strings.Contains(tree, "\n  - status=open id="+childID+" parent="+rootID+" root="+rootID) ||
		!strings.Contains(tree, `priority=4 due=2026-10-02 claim=- title="root" description="root details"`) {
		t.Fatalf("tree exit=%d\nstdout=%s\nstderr=%s", exit, tree, stderr)
	}
	direct, stderr, exit := runTodoProcess(t, binary, home, "list", "--parent", rootID, "--history")
	if exit != 0 || stderr != "" || strings.Contains(direct, "id="+rootID) || !strings.Contains(direct, "id="+childID) {
		t.Fatalf("parent filter exit=%d stdout=%s stderr=%s", exit, direct, stderr)
	}

	stdout, stderr, exit = runTodoProcess(t, binary, home, "update", "--revision", "1", "--title", "refined", "--priority", "3", childID)
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "revision=2") || !strings.Contains(stdout, `title="refined"`) {
		t.Fatalf("update exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "update", "--revision", "1", "--title", "stale", childID)
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "revision conflict") {
		t.Fatalf("stale update exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "block", "--revision", "2", childID)
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "status=blocked") || !strings.Contains(stdout, "revision=3") {
		t.Fatalf("block exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	blocked, stderr, exit := runTodoProcess(t, binary, home, "list", "--root", rootID, "--status", "blocked")
	if exit != 0 || stderr != "" || strings.Contains(blocked, "id="+rootID) || !strings.Contains(blocked, "id="+childID) {
		t.Fatalf("blocked filter exit=%d stdout=%s stderr=%s", exit, blocked, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "reopen", "--revision", "3", childID)
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "status=open") || !strings.Contains(stdout, "revision=4") {
		t.Fatalf("reopen exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "done", "--revision", "2", rootID)
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "active descendants") {
		t.Fatalf("premature parent completion exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	for _, transition := range []struct {
		command  string
		revision string
		status   string
	}{
		{"start", "4", "in_progress"},
		{"done", "5", "completed"},
		{"reopen", "6", "open"},
		{"cancel", "7", "cancelled"},
		{"reopen", "8", "open"},
	} {
		stdout, stderr, exit = runTodoProcess(t, binary, home, transition.command, "--revision", transition.revision, childID)
		if exit != 0 || stderr != "" || !strings.Contains(stdout, "status="+transition.status) {
			t.Fatalf("%s exit=%d stdout=%s stderr=%s", transition.command, exit, stdout, stderr)
		}
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "get", "missing")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, `task "missing" not found`) {
		t.Fatalf("missing get exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--priority", "6", "bad")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "priority") {
		t.Fatalf("invalid priority exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
}

func TestTodoCLISharesScopeClaimsAndStoreWithPlugin(t *testing.T) {
	binary := buildTodoBinary(t)
	home := t.TempDir()
	dbDir := filepath.Join(home, ".evie")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := eviedb.OpenDBAt(filepath.Join(dbDir, "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ctx := globalOwnerContext("plugin")
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewTodo(store))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(plugins.TodoPluginID, true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	execute := func(name, arguments string) string {
		t.Helper()
		message, isError, err := toolset.ExecuteWithApprovalAuthorizedCompletion(ctx, openrouter.ToolCall{
			ID: "cli-cross-surface", Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: arguments},
		}, nil, nil, nil, nil)
		if err != nil || isError {
			t.Fatalf("execute %s = %q, error=%v, dispatch=%v", name, message, isError, err)
		}
		return message.Content
	}
	pluginJSON := execute("todo_add", `{"title":"from plugin","idempotency_key":"plugin-add"}`)
	pluginID := jsonTaskID(t, pluginJSON)
	stdout, stderr, exit := runTodoProcess(t, binary, home, "get", string(pluginID))
	if exit != 0 || stderr != "" || !strings.Contains(stdout, `title="from plugin"`) {
		t.Fatalf("CLI sees plugin exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}

	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "from CLI")
	if exit != 0 || stderr != "" {
		t.Fatalf("CLI add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	cliID := renderedIDs(stdout)[0]
	pluginJSON = execute("todo_get", `{"task_id":"`+cliID+`"}`)
	if !strings.Contains(pluginJSON, `"title":"from CLI"`) {
		t.Fatalf("plugin sees CLI = %s", pluginJSON)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "update", "--revision", "1", "--title", "from CLI refined", cliID)
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "revision=2") {
		t.Fatalf("CLI update exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	pluginJSON = execute("todo_get", `{"task_id":"`+cliID+`"}`)
	if !strings.Contains(pluginJSON, `"title":"from CLI refined"`) || !strings.Contains(pluginJSON, `"revision":2`) {
		t.Fatalf("open plugin connection sees CLI update = %s", pluginJSON)
	}

	projectAPath := filepath.Join(home, "project-a")
	projectBPath := filepath.Join(home, "project-b")
	if err := os.MkdirAll(projectAPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectBPath, 0o700); err != nil {
		t.Fatal(err)
	}
	projectA, err := store.RegisterProject(context.Background(), "A", projectAPath)
	if err != nil {
		t.Fatal(err)
	}
	projectB, err := store.RegisterProject(context.Background(), "B", projectBPath)
	if err != nil {
		t.Fatal(err)
	}
	sessionA, err := store.CreateProjectSession(context.Background(), projectA.ID)
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := store.CreateProjectSession(context.Background(), projectB.ID)
	if err != nil {
		t.Fatal(err)
	}
	taskA, err := store.CreateGlobalTask(sessionOwnerContext(sessionA, "project-a"), task.CreateInput{
		Title: "project A only", IdempotencyKey: "project-a", Scope: task.ScopeSelectionContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := store.CreateGlobalTask(sessionOwnerContext(sessionB, "project-b"), task.CreateInput{
		Title: "project B secret", IdempotencyKey: "project-b", Scope: task.ScopeSelectionContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "list", "--session", string(sessionA.ID), "--scope", "context")
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "id="+string(taskA.ID)) || strings.Contains(stdout, string(taskB.ID)) || strings.Contains(stdout, taskB.Title) {
		t.Fatalf("scoped list exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "get", "--session", string(sessionA.ID), string(taskB.ID))
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "not found") || strings.Contains(stderr, taskB.Title) {
		t.Fatalf("cross-scope get exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--session", string(sessionA.ID), "--scope", "context", "CLI project A")
	if exit != 0 || stderr != "" {
		t.Fatalf("scoped CLI add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	projectCLI, err := store.GetGlobalTask(sessionOwnerContext(sessionA, "read-cli-project"), task.ID(renderedIDs(stdout)[0]))
	if err != nil || projectCLI.Scope != task.ProjectScope(string(projectA.ID)) {
		t.Fatalf("scoped CLI Task = %+v, %v", projectCLI, err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--session", string(sessionA.ID), "--scope", "global", "CLI explicit Global")
	if exit != 0 || stderr != "" {
		t.Fatalf("global CLI add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	globalCLI, err := store.GetGlobalTask(ctx, task.ID(renderedIDs(stdout)[0]))
	if err != nil || globalCLI.Scope != task.ScopeGlobal {
		t.Fatalf("global CLI Task = %+v, %v", globalCLI, err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "start", "--revision", "1", string(pluginID))
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "revision=2") {
		t.Fatalf("start result Task exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	firstResult, stderr, exit := runTodoProcess(
		t, binary, home, "update", "--revision", "2", "--idempotency-key", "claimed-result-retry", "--result", "durable result", string(pluginID),
	)
	if exit != 0 || stderr != "" || !strings.Contains(firstResult, "revision=3") {
		t.Fatalf("first result update exit=%d stdout=%s stderr=%s", exit, firstResult, stderr)
	}

	workspaceA, err := store.RegisterWorkspace(context.Background(), "Workspace A")
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := store.RegisterWorkspace(context.Background(), "Workspace B")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSessionA, err := store.CreateWorkspaceSessionWithComposition(
		context.Background(), workspaceA.ID, workspaceA.CurrentRevisionID, todoTestReceipt(),
	)
	if err != nil {
		t.Fatal(err)
	}
	workspaceSessionB, err := store.CreateWorkspaceSessionWithComposition(
		context.Background(), workspaceB.ID, workspaceB.CurrentRevisionID, todoTestReceipt(),
	)
	if err != nil {
		t.Fatal(err)
	}
	workspaceTaskA, err := store.CreateGlobalTask(sessionOwnerContext(workspaceSessionA, "workspace-a"), task.CreateInput{
		Title: "Workspace A only", IdempotencyKey: "workspace-a", Scope: task.ScopeSelectionContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspaceTaskB, err := store.CreateGlobalTask(sessionOwnerContext(workspaceSessionB, "workspace-b"), task.CreateInput{
		Title: "Workspace B secret", IdempotencyKey: "workspace-b", Scope: task.ScopeSelectionContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "list", "--session", string(workspaceSessionA.ID), "--scope", "context")
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "id="+string(workspaceTaskA.ID)) ||
		strings.Contains(stdout, string(workspaceTaskB.ID)) || strings.Contains(stdout, workspaceTaskB.Title) {
		t.Fatalf("Workspace scoped list exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "get", "--session", string(workspaceSessionA.ID), string(workspaceTaskB.ID))
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "not found") || strings.Contains(stderr, workspaceTaskB.Title) {
		t.Fatalf("cross-Workspace get exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "add", "--session", string(workspaceSessionA.ID), "--scope", "context", "CLI Workspace A")
	if exit != 0 || stderr != "" {
		t.Fatalf("Workspace CLI add exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	workspaceCLI, err := store.GetGlobalTask(sessionOwnerContext(workspaceSessionA, "read-cli-workspace"), task.ID(renderedIDs(stdout)[0]))
	if err != nil || workspaceCLI.Scope != task.WorkspaceScope(string(workspaceA.ID)) {
		t.Fatalf("Workspace CLI Task = %+v, %v", workspaceCLI, err)
	}

	claimSession, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), claimSession.ID, "claim-holder", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claimCtx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(claimSession.ID), RunID: "claim-run",
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation),
	})
	claim, err := store.ClaimGlobalTask(claimCtx, pluginID, task.ClaimInput{IdempotencyKey: "visible-claim"})
	if err != nil {
		t.Fatal(err)
	}
	eventsBeforeRetry, err := store.ListTaskEvents(ctx, pluginID)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, stderr, exit := runTodoProcess(
		t, binary, home, "update", "--revision", "2", "--idempotency-key", "claimed-result-retry", "--result", "durable result", string(pluginID),
	)
	if exit != 0 || stderr != "" || !strings.Contains(secondResult, "revision=3") ||
		!strings.Contains(secondResult, `result="durable result"`) || !strings.Contains(secondResult, "claim="+claim.ID+"@") {
		t.Fatalf("result replay with foreign claim exit=%d stdout=%s stderr=%s", exit, secondResult, stderr)
	}
	eventsAfterRetry, err := store.ListTaskEvents(ctx, pluginID)
	if err != nil || len(eventsAfterRetry) != len(eventsBeforeRetry) {
		t.Fatalf("result replay events before=%d after=%d err=%v", len(eventsBeforeRetry), len(eventsAfterRetry), err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "get", string(pluginID))
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "claim="+claim.ID+"@") {
		t.Fatalf("claim rendering exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, home, "release-claim", "--reason", "owner recovery", string(pluginID))
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "claim="+claim.ID) || !strings.Contains(stdout, `reason="owner recovery"`) {
		t.Fatalf("claim release exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
	_, found, err := store.GetGlobalTaskClaim(ctx, pluginID)
	if err != nil || found {
		t.Fatalf("claim after recovery found=%v err=%v", found, err)
	}
	events, err := store.ListTaskEvents(ctx, pluginID)
	if err != nil || !events[len(events)-1].ManagementOverride || events[len(events)-1].ManagementReason != "owner recovery" {
		t.Fatalf("recovery audit = %+v, %v", events, err)
	}
}

func TestTodoCLIProcessReportsImportAndPersistenceFailures(t *testing.T) {
	binary := buildTodoBinary(t)
	home := t.TempDir()
	legacyPath := filepath.Join(home, ".todo", "DayToDay.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := runTodoProcess(t, binary, home, "list", "--history")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "todo: import legacy Todo list:") {
		t.Fatalf("malformed import exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}

	brokenHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(brokenHome, ".evie"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit = runTodoProcess(t, binary, brokenHome, "list")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "todo: open Evie database:") {
		t.Fatalf("persistence failure exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
	}
}

func buildTodoBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "todo")
	command := exec.Command("go", "build", "-o", binary, ".")
	command.Dir = "."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build todo: %v\n%s", err, output)
	}
	return binary
}

func runTodoProcess(t *testing.T, binary, home string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = home
	command.Env = []string{
		"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.TempDir(), "TZ=UTC", "TERM=dumb",
		"TODO_DIR=ignored", "TODO_NAME=Other",
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	exit := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run todo %q: %v", args, err)
		}
		exit = exitError.ExitCode()
	}
	return stdout.String(), stderr.String(), exit
}

var renderedIDPattern = regexp.MustCompile(`(?:^|\s)id=([0-9a-f-]+)`)

func renderedIDs(output string) []string {
	matches := renderedIDPattern.FindAllStringSubmatch(output, -1)
	values := make([]string, len(matches))
	for i, match := range matches {
		values[i] = match[1]
	}
	return values
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed: %q; want %q", path, got, want)
	}
}

func globalOwnerContext(run string) context.Context {
	return task.WithGlobalScopeCompatibility(task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: "test-owner", RunID: run,
	}))
}

func sessionOwnerContext(session memory.Session, run string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: run,
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID),
	})
}

func jsonTaskID(t *testing.T, encoded string) task.ID {
	t.Helper()
	match := regexp.MustCompile(`"id":"([0-9a-f-]+)"`).FindStringSubmatch(encoded)
	if len(match) != 2 {
		t.Fatalf("Task ID missing from %s", encoded)
	}
	return task.ID(match[1])
}

func todoTestReceipt() composition.Receipt {
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	return composition.Receipt{
		FormatVersion: composition.FormatVersion,
		Preset:        composition.PresetIdentity{ID: "todo-cli-test", Version: "sha256:" + hash},
		EvieVersion:   "1.0.0",
	}
}
