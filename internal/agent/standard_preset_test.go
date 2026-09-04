package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

type claimThenBlockClient struct {
	taskID  task.ID
	calls   int
	entered chan struct{}
}

type delegatedSubtaskClient struct {
	parentID       task.ID
	parentRevision uint64
	label          string
	calls          int
	reqs           []openrouter.ChatRequest
	created        task.Task
}

func (c *delegatedSubtaskClient) ChatStream(
	_ context.Context,
	req openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.reqs = append(c.reqs, req)
	c.calls++
	switch c.calls {
	case 1:
		arguments := fmt.Sprintf(
			`{"title":"%s subtask","parent_id":"%s","expected_parent_revision":%d,"idempotency_key":"create-%s"}`,
			c.label, c.parentID, c.parentRevision, c.label,
		)
		return assistantStep("", nil, toolCall("create-subtask", "todo_add", arguments)).res, nil
	case 2:
		for i := len(req.Messages) - 1; i >= 0; i-- {
			message := req.Messages[i]
			if message.Role == "tool" && message.ToolCallID == "create-subtask" {
				if err := json.Unmarshal([]byte(message.Content), &c.created); err != nil {
					return openrouter.ChatResponse{}, fmt.Errorf("decode created delegated Subtask: %w", err)
				}
				arguments := fmt.Sprintf(`{"task_id":"%s","idempotency_key":"claim-%s"}`, c.created.ID, c.label)
				return assistantStep("", nil, toolCall("claim-subtask", "todo_claim", arguments)).res, nil
			}
		}
		return openrouter.ChatResponse{}, errors.New("created delegated Subtask result missing")
	case 3:
		arguments := fmt.Sprintf(
			`{"task_id":"%s","expected_revision":1,"status":"completed","result_summary":"%s done","idempotency_key":"complete-%s"}`,
			c.created.ID, c.label, c.label,
		)
		return assistantStep("", nil, toolCall("complete-subtask", "todo_update", arguments)).res, nil
	case 4:
		return assistantStep("done", nil).res, nil
	default:
		return openrouter.ChatResponse{}, errors.New("unexpected delegated Subtask provider call")
	}
}

func TestStandardGuidanceSupportsVisibleDurableCreationAndSkipsOneShotWork(t *testing.T) {
	t.Run("durable multi-step work", func(t *testing.T) {
		ctx := context.Background()
		db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := eviedb.NewStore(db)
		resolved, err := standardManager(t, store).ResolvePreset(plugins.StandardPresetID)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("create-tree", "todo_add",
				`{"title":"Plan the release","description":"Track implementation and verification","focus":true,"idempotency_key":"natural-tree"}`)),
			assistantStep(`I created the Task Tree "Plan the release" so you can inspect its durable progress.`, nil),
			assistantStep("I am continuing from the focused release tree.", nil),
		}}
		recorded := &recorder{}
		session := NewWithToolset(
			client, testContextProfile("test-model"), store.BindHistory(stored.ID, "natural-holder"),
			stored.ScopeContext(), store.BindTurnOwner(stored.ID, "natural-holder"), resolved.Toolset,
		)
		if err := session.Send(ctx, "Plan and carry out this multi-step release across several turns.", recorded, nil); err != nil {
			t.Fatal(err)
		}
		if len(client.reqs) != 2 || len(client.reqs[0].Messages) == 0 {
			t.Fatalf("scripted requests = %+v", client.reqs)
		}
		guidance := client.reqs[0].Messages[0].Content
		for _, want := range []string{
			"# Durable Task Trees", "multi-step", "span turns", "ordinary one-shot", "without a separate approval",
			"mention", "active Workspace or project", "Task Focus", "Task Access Grant", "does not grant authority",
		} {
			if !strings.Contains(guidance, want) {
				t.Errorf("system guidance missing %q", want)
			}
		}
		created, err := store.ListOpenGlobalTasks(ctx)
		if err != nil || len(created) != 1 || created[0].Title != "Plan the release" {
			t.Fatalf("durable creation = %+v, %v", created, err)
		}
		if !slicesContainSubstring(recorded.events, `done:I created the Task Tree "Plan the release"`) {
			t.Fatalf("owner-visible creation = %#v", recorded.events)
		}
		durable, err := store.LoadEvents(ctx, stored.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range durable {
			if event.Type == memory.EventApproval {
				t.Fatalf("autonomous Task creation requested separate approval: %+v", event)
			}
		}
		if err := session.Send(ctx, "Continue the tracked release.", &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
		if len(client.reqs) != 3 || !requestContains(client.reqs[2], "<task-focus-data>") ||
			!requestContains(client.reqs[2], "Plan the release") {
			t.Fatalf("next-turn Task Focus = %+v", client.reqs)
		}
	})

	t.Run("ordinary one-shot work", func(t *testing.T) {
		ctx := context.Background()
		db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		store := eviedb.NewStore(db)
		resolved, err := standardManager(t, store).ResolvePreset(plugins.StandardPresetID)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{steps: []step{assistantStep("Four.", nil)}}
		session := NewWithToolset(
			client, testContextProfile("test-model"), store.BindHistory(stored.ID, "simple-holder"),
			stored.ScopeContext(), store.BindTurnOwner(stored.ID, "simple-holder"), resolved.Toolset,
		)
		if err := session.Send(ctx, "What is two plus two?", &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
		created, err := store.ListOpenGlobalTasks(ctx)
		if err != nil || len(created) != 0 {
			t.Fatalf("one-shot request created Tasks = %+v, %v", created, err)
		}
	})
}

func (c *claimThenBlockClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant", ToolCalls: []openrouter.ToolCall{toolCall("claim-call", "todo_claim",
				`{"task_id":"`+string(c.taskID)+`","idempotency_key":"interrupted-claim"}`)},
		}}}}, nil
	}
	close(c.entered)
	<-ctx.Done()
	return openrouter.ChatResponse{}, ctx.Err()
}

func TestStandardPresetReceiptReopensIntoExactScriptedAgentSchemas(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstManager := standardManager(t, store)
	first, err := firstManager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCompositionReceipt(ctx, storedSession.ID, first.Receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	receipt, err := store.GetCompositionReceipt(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, first.Receipt) {
		t.Fatalf("reopened receipt = %#v, want %#v", receipt, first.Receipt)
	}
	secondManager := standardManager(t, store)
	resumed, err := secondManager.ResumeComposition(receipt)
	if err != nil {
		t.Fatal(err)
	}

	wantTodo := []tools.Tool{
		tools.TodoScopedListTool(), tools.TodoFocusedAddTool(), tools.TodoTreeGetTool(), tools.TodoClaimedUpdateTool(),
		tools.TodoDecomposeTool(), tools.TodoClaimTool(), tools.TodoReleaseTool(),
	}
	wantSchemas := tools.KernelToolset().WithTools(tools.FinanceTools()).WithTools(tools.WebTools()).WithTools(tools.YouTubeTools()).WithTools(wantTodo).Schemas()
	if !reflect.DeepEqual(resumed.Toolset.Schemas(), wantSchemas) {
		t.Fatalf("resumed schemas = %#v, want exact standard schemas %#v", resumed.Toolset.Schemas(), wantSchemas)
	}
	wantProviders := []plugins.ProviderReceipt{
		{ID: "finance", ImplementationVersion: "1.0.0"},
		{ID: "todo", ImplementationVersion: "1.7.0"},
		{ID: "web", ImplementationVersion: "1.0.0"},
		{ID: "youtube", ImplementationVersion: "1.0.0"},
	}
	if !reflect.DeepEqual(receipt.Providers, wantProviders) {
		t.Fatalf("receipt providers = %#v, want %#v", receipt.Providers, wantProviders)
	}
	wantCapabilities := []string{
		"finance.sync@1.0.0", "finance.rules@1.0.0", "finance.categorize@1.0.0",
		"web.fetch@1.0.0", "web.search@1.0.0",
		"youtube.transcript@1.0.0", "youtube.scrape_channel@1.0.0",
		"todo.list@1.3.0", "todo.add@1.4.0", "todo.get@1.1.0", "todo.update@1.3.0", "todo.decompose@1.0.0",
		"todo.claim@1.0.0", "todo.release@1.0.0",
	}
	gotCapabilities := make([]string, len(receipt.Capabilities))
	for i, capability := range receipt.Capabilities {
		gotCapabilities[i] = capability.ID + "@" + capability.ContractVersion
	}
	if !reflect.DeepEqual(gotCapabilities, wantCapabilities) {
		t.Fatalf("receipt capabilities = %v, want %v", gotCapabilities, wantCapabilities)
	}

	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("selected-call", "youtube_transcript", `{}`)),
		assistantStep("", nil, toolCall("todo-call", "todo_add", `{"title":"created through standard","priority":5,"due":"2026-09-03"}`)),
		assistantStep("", nil, toolCall("absent-call", "absent_standard_tool", `{}`)),
		assistantStep("done", nil),
	}}
	recorded := &recorder{}
	session := NewWithToolset(
		client,
		testContextProfile("test-model"),
		store.BindHistory(storedSession.ID, "holder"),
		storedSession.ScopeContext(),
		store.BindTurnOwner(storedSession.ID, "holder"),
		resumed.Toolset,
	)
	if err := session.Send(ctx, "hello", recorded, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 4 {
		t.Fatalf("scripted requests = %d, want four", len(client.reqs))
	}
	for i, request := range client.reqs {
		if !reflect.DeepEqual(request.Tools, wantSchemas) {
			t.Fatalf("scripted request %d schemas = %#v, want %#v", i, request.Tools, wantSchemas)
		}
	}
	created, err := store.ListOpenGlobalTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || created[0].Title != "created through standard" || created[0].Priority != 5 ||
		created[0].DueDate != "2026-09-03" || created[0].Scope != task.ScopeGlobal {
		t.Fatalf("standard Todo Task = %+v", created)
	}
	encodedTask, err := json.Marshal(created[0])
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"done:",
		`call:selected-call:youtube_transcript:{}`,
		"result:selected-call:false:deterministic youtube.transcript result",
		"done:",
		`call:todo-call:todo_add:{"title":"created through standard","priority":5,"due":"2026-09-03"}`,
		"result:todo-call:false:" + string(encodedTask),
		"done:",
		`call:absent-call:absent_standard_tool:{}`,
		"result:absent-call:true:Unknown Tool Call: absent_standard_tool",
		"done:done",
	}
	if !reflect.DeepEqual(recorded.events, wantEvents) {
		t.Fatalf("scripted public events = %#v, want %#v", recorded.events, wantEvents)
	}
	if len(client.reqs) != 4 || !reflect.DeepEqual(client.reqs[0].Tools, wantSchemas) {
		t.Fatalf("scripted request schemas = %#v, want %#v", client.reqs, wantSchemas)
	}
	durableEvents, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	var todoIntent, todoOutcome bool
	var todoExecutionID memory.ExecutionID
	for _, event := range durableEvents {
		if event.Type == memory.EventToolIntent {
			var payload memory.ToolIntentPayload
			if json.Unmarshal(event.Payload, &payload) == nil && payload.Call.Name == "todo_add" {
				todoIntent = true
				todoExecutionID = event.ExecutionID
			}
		}
		if event.Type == memory.EventToolSucceeded && event.Content == string(encodedTask) {
			todoOutcome = true
		}
	}
	if !todoIntent || !todoOutcome {
		t.Fatalf("Todo episodic evidence missing: intent=%v outcome=%v events=%+v", todoIntent, todoOutcome, durableEvents)
	}
	taskEvents, err := store.ListTaskEvents(ctx, created[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(taskEvents) != 1 || taskEvents[0].ActorID != string(memory.LocalOwnerID) ||
		taskEvents[0].SessionID != string(storedSession.ID) || taskEvents[0].RunID != string(todoExecutionID) {
		t.Fatalf("Task event attribution = %+v, episodic execution = %q", taskEvents, todoExecutionID)
	}
}

func TestStandardWorkspaceTodoDefaultsScopeAndProjectsDurableFocus(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "Scoped agent")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("add", "todo_add", `{"title":"workspace work","focus":true,"idempotency_key":"workspace-work"}`)),
		assistantStep("created", nil), assistantStep("continuing", nil),
	}}
	session := NewWithToolset(client, testContextProfile("test-model"), store.BindHistory(stored.ID, "holder"),
		stored.ScopeContext(), store.BindTurnOwner(stored.ID, "holder"), resolved.Toolset)
	if err := session.Send(ctx, "track this", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	bound := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(stored.ID), RunID: "inspect", WorkspaceID: string(workspace.ID),
	})
	values, err := store.ListGlobalTasks(bound, task.ListFilter{Scope: task.ScopeSelectionContext})
	if err != nil || len(values) != 1 || values[0].Scope != task.WorkspaceScope(string(workspace.ID)) {
		t.Fatalf("Workspace Tasks = %+v, %v", values, err)
	}
	if err := session.Send(ctx, "continue", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 || len(client.reqs[2].Messages) < 2 ||
		client.reqs[2].Messages[1].Role != "user" ||
		!strings.Contains(client.reqs[2].Messages[1].Content, "<task-focus-data>") ||
		!strings.Contains(client.reqs[2].Messages[1].Content, "workspace work") {
		t.Fatalf("focused request = %+v", client.reqs)
	}
	if stored.ScopeContext().WorkspaceID != workspace.ID || stored.ScopeContext().ProjectID != "" {
		t.Fatalf("Task Focus changed session Context Scope: %+v", stored.ScopeContext())
	}
}

func TestStandardAgentClaimsAndCompletesTaskThroughOneFencedTurn(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: "local", SessionID: string(storedSession.ID), RunID: "seed",
	})
	created, err := store.CreateGlobalTask(seedCtx, task.CreateInput{Title: "claim through standard", IdempotencyKey: "claim-standard-root"})
	if err != nil {
		t.Fatal(err)
	}
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("claim-call", "todo_claim",
			`{"task_id":"`+string(created.ID)+`","idempotency_key":"standard-claim"}`)),
		assistantStep("", nil, toolCall("complete-call", "todo_update",
			`{"task_id":"`+string(created.ID)+`","expected_revision":1,"status":"completed","result_summary":"implemented and verified","idempotency_key":"standard-complete"}`)),
		assistantStep("done", nil),
	}}
	session := NewWithToolset(
		client, testContextProfile("test-model"), store.BindHistory(storedSession.ID, "claim-holder"),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, "claim-holder"), resolved.Toolset,
	)
	if err := session.Send(ctx, "complete the task", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetGlobalTask(ctx, created.ID)
	if err != nil || got.Status != task.StatusCompleted || got.ResultSummary != "implemented and verified" {
		t.Fatalf("completed Task = %+v, %v", got, err)
	}
	if _, found, err := store.GetGlobalTaskClaim(ctx, created.ID); err != nil || found {
		t.Fatalf("terminal Task retained claim: found=%v err=%v", found, err)
	}
	durable, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	executions := map[string]memory.ExecutionID{}
	for _, event := range durable {
		if event.Type != memory.EventToolIntent {
			continue
		}
		var payload memory.ToolIntentPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
			executions[payload.Call.Name] = event.ExecutionID
		}
	}
	events, err := store.ListTaskEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 || events[1].Operation != task.OperationClaim ||
		events[1].RunID != string(executions["todo_claim"]) ||
		events[2].Operation != task.OperationUpdate || events[2].RunID != string(executions["todo_update"]) ||
		events[3].Operation != task.OperationUpdate || events[3].ClaimReason != "authorized" ||
		events[3].ClaimID == "" || events[3].RunID != string(executions["todo_update"]) ||
		events[4].Operation != task.OperationRelease || events[4].ClaimReason != "task_completed" ||
		events[4].RunID != string(executions["todo_update"]) {
		t.Fatalf("claim lifecycle evidence = executions %+v events %+v", executions, events)
	}
}

func TestInterruptedStandardTurnReleasesTaskClaim(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seedCtx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: string(storedSession.ID), RunID: "seed",
	})
	created, err := store.CreateGlobalTask(seedCtx, task.CreateInput{Title: "interrupt claim", IdempotencyKey: "interrupt-root"})
	if err != nil {
		t.Fatal(err)
	}
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	client := &claimThenBlockClient{taskID: created.ID, entered: make(chan struct{})}
	session := NewWithToolset(
		client, testContextProfile("test-model"), store.BindHistory(storedSession.ID, "interrupt-holder"),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, "interrupt-holder"), resolved.Toolset,
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Send(ctx, "start work", &recorder{}, nil) }()
	<-client.entered
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || !found {
		t.Fatalf("claim was not active before interruption: found=%v err=%v", found, err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted Send error = %v", err)
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || found {
		t.Fatalf("interrupted claim remained active: found=%v err=%v", found, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || events[len(events)-1].Operation != task.OperationRelease ||
		events[len(events)-1].ClaimReason != "execution_ended" {
		t.Fatalf("interruption claim events = %+v, %v", events, err)
	}
}

func TestStandardAgentRecordsRejectedTaskMutationWithoutInventingTransition(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: "local", SessionID: string(storedSession.ID), RunID: "seed",
	})
	created, err := store.CreateGlobalTask(seedCtx, task.CreateInput{Title: "protect revision", IdempotencyKey: "seed-create"})
	if err != nil {
		t.Fatal(err)
	}
	seedTitle := "revision two"
	created, err = store.UpdateGlobalTask(seedCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Title: &seedTitle, IdempotencyKey: "seed-update",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("stale-call", "todo_update",
			`{"task_id":"`+string(created.ID)+`","expected_revision":1,"title":"lost","idempotency_key":"stale-agent"}`)),
		assistantStep("done", nil),
	}}
	session := NewWithToolset(
		client, testContextProfile("test-model"), store.BindHistory(storedSession.ID, "holder"),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, "holder"), resolved.Toolset,
	)
	if err := session.Send(ctx, "stale update", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetGlobalTask(ctx, created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("rejected update changed Task: %+v, %v", got, err)
	}
	durable, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	var failedExecution memory.ExecutionID
	for _, event := range durable {
		if event.Type == memory.EventToolFailed && event.ExecutionID != "" {
			failedExecution = event.ExecutionID
		}
	}
	taskEvents, err := store.ListTaskEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := taskEvents[len(taskEvents)-1]
	if failedExecution == "" || len(taskEvents) != 3 || last.Outcome != task.MutationRejected ||
		last.DiagnosticCode != task.DiagnosticRevisionConflict || last.PreviousRevision != 2 ||
		last.ResultingRevision != 2 || last.RunID != string(failedExecution) {
		t.Fatalf("rejected evidence = episodic execution %q Task events %+v", failedExecution, taskEvents)
	}
}

func TestStandardAgentDecomposesTaskTreeThroughNormalDispatcher(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seedCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: "local", SessionID: string(storedSession.ID), RunID: "seed",
	})
	root, err := store.CreateGlobalTask(seedCtx, task.CreateInput{Title: "agent tree", IdempotencyKey: "agent-tree-root"})
	if err != nil {
		t.Fatal(err)
	}
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("decompose-call", "todo_decompose",
			`{"task_id":"`+string(root.ID)+`","expected_revision":1,"children":[{"title":"research"},{"title":"verify"}],"idempotency_key":"agent-decompose"}`)),
		assistantStep("done", nil),
	}}
	session := NewWithToolset(
		client, testContextProfile("test-model"), store.BindHistory(storedSession.ID, "holder"),
		storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, "holder"), resolved.Toolset,
	)
	if err := session.Send(ctx, "decompose it", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	children, err := store.ListGlobalTasks(ctx, task.ListFilter{ParentID: root.ID})
	if err != nil || len(children) != 2 || children[0].Title != "research" || children[1].Title != "verify" {
		t.Fatalf("agent decomposition children = %+v, %v", children, err)
	}
	durable, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	var executionID memory.ExecutionID
	var succeeded bool
	for _, event := range durable {
		if event.Type == memory.EventToolIntent {
			var payload memory.ToolIntentPayload
			if json.Unmarshal(event.Payload, &payload) == nil && payload.Call.Name == "todo_decompose" {
				executionID = event.ExecutionID
			}
		}
		if event.Type == memory.EventToolSucceeded && event.ExecutionID == executionID {
			succeeded = true
		}
	}
	taskEvents, err := store.ListTaskEvents(ctx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := taskEvents[len(taskEvents)-1]
	if executionID == "" || !succeeded || last.Operation != task.OperationDecompose ||
		last.ActorID != string(memory.LocalOwnerID) || last.SessionID != string(storedSession.ID) || last.RunID != string(executionID) {
		t.Fatalf("decomposition evidence = execution %q succeeded=%v events=%+v", executionID, succeeded, taskEvents)
	}
}

func TestStandardDelegatedSessionsIntersectTodoCompositionWithTaskGrants(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := standardManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := task.WithMutationAttribution(ctx, task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(owner.ID), RunID: "delegate",
	})
	root, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "shared root", IdempotencyKey: "shared-root"})
	if err != nil {
		t.Fatal(err)
	}
	decomposed, err := store.DecomposeGlobalTask(ownerCtx, root.ID, task.DecomposeInput{
		ExpectedRevision: root.Revision, IdempotencyKey: "shared-children",
		Children: []task.ChildInput{{Title: "child A"}, {Title: "child B"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	type delegatedFixture struct {
		session memory.Session
		grant   task.AccessGrant
	}
	makeDelegated := func(level task.AccessLevel, grantedRoot task.ID) delegatedFixture {
		t.Helper()
		stored, err := store.CreateDelegatedSessionWithComposition(ctx, owner.ID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		grant, err := store.IssueFocusedTaskAccessGrant(ownerCtx, task.GrantInput{
			GranteeSessionID: string(stored.ID), RootID: grantedRoot, Level: level,
		})
		if err != nil {
			t.Fatal(err)
		}
		return delegatedFixture{session: stored, grant: grant}
	}
	workerA := makeDelegated(task.AccessContribute, decomposed.Children[0].ID)
	workerB := makeDelegated(task.AccessContribute, decomposed.Children[1].ID)
	reader := makeDelegated(task.AccessRead, decomposed.Children[0].ID)
	withoutGrant, err := store.CreateDelegatedSessionWithComposition(ctx, owner.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}

	run := func(stored memory.Session, holder, expectedFocus string, forbiddenFocus []string, steps []step) *recorder {
		t.Helper()
		client := &fakeClient{steps: steps}
		recorded := &recorder{}
		session := NewWithToolset(
			client, testContextProfile("test-model"), store.BindHistory(stored.ID, memory.LeaseHolderID(holder)),
			stored.ScopeContext(), store.BindTurnOwner(stored.ID, memory.LeaseHolderID(holder)), resolved.Toolset,
		)
		if err := session.Send(ctx, "work the granted Task", recorded, nil); err != nil {
			t.Fatal(err)
		}
		if expectedFocus != "" {
			if len(client.reqs) == 0 || !requestContains(client.reqs[0], "<task-focus-data>") ||
				!requestContains(client.reqs[0], expectedFocus) {
				t.Fatalf("delegated focus %q missing from request: %+v", expectedFocus, client.reqs)
			}
			for _, forbidden := range forbiddenFocus {
				if requestContains(client.reqs[0], forbidden) {
					t.Fatalf("delegated focus leaked %q in request: %+v", forbidden, client.reqs[0])
				}
			}
		} else if len(client.reqs) > 0 && requestContains(client.reqs[0], "<task-focus-data>") {
			t.Fatalf("session without grant received Task Focus: %+v", client.reqs[0])
		}
		return recorded
	}
	readOnlyEvents := run(reader.session, "reader", decomposed.Children[0].Title, []string{root.Title, decomposed.Children[1].Title}, []step{
		assistantStep("", nil, toolCall("forbidden", "todo_add",
			`{"title":"forbidden","parent_id":"`+string(decomposed.Children[0].ID)+`","expected_parent_revision":2,"idempotency_key":"reader-add"}`)),
		assistantStep("done", nil),
	}).events
	if !slicesContainSubstring(readOnlyEvents, "result:forbidden:true:") ||
		!slicesContainSubstring(readOnlyEvents, "task: delegated Task access denied") {
		t.Fatalf("read-only dispatcher events = %#v", readOnlyEvents)
	}
	noGrantEvents := run(withoutGrant, "without-grant", "", nil, []step{
		assistantStep("", nil, toolCall("withheld", "todo_get",
			`{"task_id":"`+string(decomposed.Children[0].ID)+`"}`)),
		assistantStep("", nil, toolCall("withheld-mutation", "todo_add",
			`{"title":"forbidden","parent_id":"`+string(decomposed.Children[0].ID)+`","expected_parent_revision":2,"idempotency_key":"without-grant-add"}`)),
		assistantStep("done", nil),
	}).events
	if !slicesContainSubstring(noGrantEvents, "result:withheld:true:") ||
		!slicesContainSubstring(noGrantEvents, "result:withheld-mutation:true:") ||
		!slicesContainSubstring(noGrantEvents, "task: delegated Task access denied") {
		t.Fatalf("no-grant dispatcher events = %#v", noGrantEvents)
	}

	workers := []delegatedFixture{workerA, workerB}
	type workerResult struct {
		client *delegatedSubtaskClient
		err    error
	}
	results := make([]workerResult, len(workers))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i, worker := range workers {
		grantedRoot := decomposed.Children[i]
		label := string(rune('a' + i))
		client := &delegatedSubtaskClient{
			parentID: grantedRoot.ID, parentRevision: grantedRoot.Revision, label: label,
		}
		results[i].client = client
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			session := NewWithToolset(
				client, testContextProfile("test-model"), store.BindHistory(worker.session.ID, memory.LeaseHolderID("worker-"+label)),
				worker.session.ScopeContext(), store.BindTurnOwner(worker.session.ID, memory.LeaseHolderID("worker-"+label)), resolved.Toolset,
			)
			results[i].err = session.Send(ctx, "create and complete a Subtask", &recorder{}, nil)
		}()
	}
	close(start)
	wait.Wait()

	for i, worker := range workers {
		grantedRoot := decomposed.Children[i]
		other := decomposed.Children[1-i]
		label := string(rune('a' + i))
		client := results[i].client
		if results[i].err != nil {
			t.Fatalf("worker %d: %v", i, results[i].err)
		}
		if len(client.reqs) != 4 || !requestContains(client.reqs[0], grantedRoot.Title) ||
			requestContains(client.reqs[0], root.Title) || requestContains(client.reqs[0], other.Title) {
			t.Fatalf("worker %d bounded requests = %+v", i, client.reqs)
		}
		value, err := store.GetGlobalTask(ctx, client.created.ID)
		if err != nil || value.ParentID != grantedRoot.ID || value.Status != task.StatusCompleted || value.ResultSummary != label+" done" {
			t.Fatalf("worker %d Subtask result = %+v, %v", i, value, err)
		}
		events, err := store.ListTaskEvents(ctx, value.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.SessionID == string(worker.session.ID) && event.GrantID != worker.grant.ID {
				t.Fatalf("worker %d event missing grant: %+v", i, event)
			}
		}
	}
}

func slicesContainSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func requestContains(request openrouter.ChatRequest, needle string) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func standardManager(t *testing.T, taskService task.Service) *plugins.Manager {
	t.Helper()
	web := plugins.NewWeb()
	webCapabilities := web.ToolCapabilities()
	for i := range webCapabilities {
		capabilityID := webCapabilities[i].ID
		webCapabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "deterministic " + string(capabilityID) + " result", nil
		}
	}
	finance := plugins.NewFinance()
	financeCapabilities := finance.ToolCapabilities()
	for i := range financeCapabilities {
		capabilityID := financeCapabilities[i].ID
		financeCapabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "deterministic " + string(capabilityID) + " result", nil
		}
	}
	youtube := plugins.NewYouTube()
	youtubeCapabilities := youtube.ToolCapabilities()
	for i := range youtubeCapabilities {
		capabilityID := youtubeCapabilities[i].ID
		youtubeCapabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "deterministic " + string(capabilityID) + " result", nil
		}
	}
	todo := plugins.NewTodo(taskService)
	manager, err := plugins.NewManager(
		tools.KernelToolset(),
		deterministicToolPlugin{manifest: web.Manifest(), capabilities: webCapabilities},
		deterministicToolPlugin{manifest: finance.Manifest(), capabilities: financeCapabilities},
		deterministicToolPlugin{manifest: youtube.Manifest(), capabilities: youtubeCapabilities},
		todo,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

type deterministicToolPlugin struct {
	manifest     plugins.Manifest
	capabilities []plugins.ToolCapability
}

func (p deterministicToolPlugin) Manifest() plugins.Manifest { return p.manifest }

func (deterministicToolPlugin) Start(context.Context) error { return nil }

func (deterministicToolPlugin) Stop(context.Context) error { return nil }

func (p deterministicToolPlugin) ToolCapabilities() []plugins.ToolCapability {
	return append([]plugins.ToolCapability(nil), p.capabilities...)
}
