package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

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
		tools.TodoTreeListTool(), tools.TodoTreeAddTool(), tools.TodoTreeGetTool(), tools.TodoIdempotentUpdateTool(), tools.TodoDecomposeTool(),
	}
	wantSchemas := tools.KernelToolset().WithTools(tools.FinanceTools()).WithTools(tools.WebTools()).WithTools(tools.YouTubeTools()).WithTools(wantTodo).Schemas()
	if !reflect.DeepEqual(resumed.Toolset.Schemas(), wantSchemas) {
		t.Fatalf("resumed schemas = %#v, want exact standard schemas %#v", resumed.Toolset.Schemas(), wantSchemas)
	}
	wantProviders := []plugins.ProviderReceipt{
		{ID: "finance", ImplementationVersion: "1.0.0"},
		{ID: "todo", ImplementationVersion: "1.4.0"},
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
		"todo.list@1.2.0", "todo.add@1.2.0", "todo.get@1.1.0", "todo.update@1.2.0", "todo.decompose@1.0.0",
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
