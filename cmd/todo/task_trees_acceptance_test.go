package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/taskeval"
	"github.com/davidadel66/evie/internal/tools"
)

type acceptanceStep func(openrouter.ChatRequest) (openrouter.ChatResponse, error)

type acceptanceClient struct {
	steps []acceptanceStep
}

func (c *acceptanceClient) ChatStream(
	_ context.Context,
	req openrouter.ChatRequest,
	_ openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	if len(c.steps) == 0 {
		return openrouter.ChatResponse{}, errors.New("acceptance client has no scripted step")
	}
	step := c.steps[0]
	c.steps = c.steps[1:]
	return step(req)
}

type acceptanceEvents struct {
	mu      sync.Mutex
	results []string
	finals  []string
}

func (*acceptanceEvents) Delta(string)                                  {}
func (*acceptanceEvents) Reasoning(string)                              {}
func (*acceptanceEvents) ReasoningDone()                                {}
func (*acceptanceEvents) ToolCall(string, string, string)               {}
func (*acceptanceEvents) ResponseDiscarded(agent.DiscardReason, string) {}
func (e *acceptanceEvents) AssistantDone(content string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.finals = append(e.finals, content)
}
func (e *acceptanceEvents) ToolResult(id, content string, isErr bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.results = append(e.results, fmt.Sprintf("%s:%t:%s", id, isErr, content))
}

func TestStandardTaskTreeWholeSystemAcceptanceAndEvaluationEvidence(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	dbDir := filepath.Join(home, ".evie")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "evie.db")
	db, err := eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	firstDB := db
	t.Cleanup(func() { _ = firstDB.Close() })
	store := eviedb.NewStore(db)
	manager := acceptanceManager(t, store)
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	assertAcceptanceComposition(t, manager, resolved)

	workspace, err := store.RegisterWorkspace(ctx, "Task Tree acceptance")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	siblingWorkspace, err := store.RegisterWorkspace(ctx, "Sibling private workspace")
	if err != nil {
		t.Fatal(err)
	}
	siblingSession, err := store.CreateWorkspaceSessionWithComposition(ctx, siblingWorkspace.ID, siblingWorkspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	siblingTask := createAcceptanceTask(t, store, siblingSession, "sibling-workspace", "sibling workspace secret")
	projectRoot := t.TempDir()
	project, err := store.RegisterProject(ctx, "Private project", projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	projectSession, err := store.CreateProjectSessionWithComposition(ctx, project.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	projectTask := createAcceptanceTask(t, store, projectSession, "sibling-project", "project secret")
	adjacent := createAcceptanceTask(t, store, owner, "adjacent-tree", "adjacent tree secret")

	rootClient := &acceptanceClient{}
	var root task.Task
	var decomposition task.Decomposition
	var decompositionArguments string
	rootClient.steps = []acceptanceStep{
		toolAcceptanceStep("youtube", "youtube_transcript", `{}`),
		toolAcceptanceStep("create-root", "todo_add", `{"title":"Ship the Task Tree release","description":"credential-marker must stay out of reports","focus":true,"idempotency_key":"acceptance-root"}`),
		func(req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			if err := decodeAcceptanceToolResult(req, "create-root", &root); err != nil {
				return openrouter.ChatResponse{}, err
			}
			decompositionArguments = fmt.Sprintf(
				`{"task_id":"%s","expected_revision":1,"children":[{"title":"implementation"},{"title":"verification"}],"idempotency_key":"acceptance-decompose"}`,
				root.ID,
			)
			return toolAcceptanceStep("decompose", "todo_decompose", decompositionArguments)(req)
		},
		func(req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			if err := decodeAcceptanceToolResult(req, "decompose", &decomposition); err != nil {
				return openrouter.ChatResponse{}, err
			}
			return toolAcceptanceStep("decompose-retry", "todo_decompose", decompositionArguments)(req)
		},
		func(req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			var replay task.Decomposition
			if err := decodeAcceptanceToolResult(req, "decompose-retry", &replay); err != nil {
				return openrouter.ChatResponse{}, err
			}
			if !reflect.DeepEqual(replay, decomposition) {
				return openrouter.ChatResponse{}, errors.New("idempotent decomposition retry changed its result")
			}
			arguments := fmt.Sprintf(
				`{"task_id":"%s","expected_revision":1,"title":"stale overwrite","idempotency_key":"acceptance-stale"}`,
				root.ID,
			)
			return toolAcceptanceStep("stale-update", "todo_update", arguments)(req)
		},
		func(openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			return assistantAcceptanceStep(fmt.Sprintf("I created and focused Task Tree %q (%s).", root.Title, root.ID)), nil
		},
	}
	ownerEvents := &acceptanceEvents{}
	ownerAgent := acceptanceAgent(t, rootClient, store, owner, "acceptance-owner", resolved.Toolset)
	if err := ownerAgent.Send(ctx, "Plan and execute this multi-step release. Full prompt credential-marker.", ownerEvents, nil); err != nil {
		t.Fatal(err)
	}
	if root.ID == "" || len(decomposition.Children) != 2 || root.Scope != task.WorkspaceScope(string(workspace.ID)) {
		t.Fatalf("created scoped tree root=%+v decomposition=%+v", root, decomposition)
	}
	if !containsAcceptanceResult(ownerEvents, "youtube:true:tool call came back with error video must not be empty") ||
		!containsAcceptanceResult(ownerEvents, "stale-update:true:") ||
		len(ownerEvents.finals) == 0 || !strings.Contains(ownerEvents.finals[len(ownerEvents.finals)-1], string(root.ID)) {
		t.Fatalf("owner-visible results=%#v finals=%#v", ownerEvents.results, ownerEvents.finals)
	}

	type delegatedWorker struct {
		session memory.Session
		grant   task.AccessGrant
		events  *acceptanceEvents
	}
	workers := make([]delegatedWorker, len(decomposition.Children))
	ownerCtx := acceptanceTaskContext(owner, "issue-grants")
	for i, child := range decomposition.Children {
		delegated, err := store.CreateDelegatedSessionWithComposition(ctx, owner.ID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		grant, err := store.IssueFocusedTaskAccessGrant(ownerCtx, task.GrantInput{
			GranteeSessionID: string(delegated.ID), RootID: child.ID, Level: task.AccessContribute,
		})
		if err != nil {
			t.Fatal(err)
		}
		workers[i] = delegatedWorker{session: delegated, grant: grant, events: &acceptanceEvents{}}
	}

	workerErrors := make(chan error, len(workers))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i, worker := range workers {
		child := decomposition.Children[i]
		steps := []acceptanceStep{}
		if i == 0 {
			for probe, id := range []task.ID{root.ID, adjacent.ID, siblingTask.ID, projectTask.ID} {
				steps = append(steps, toolAcceptanceStep(
					fmt.Sprintf("forbidden-get-%d", probe), "todo_get", fmt.Sprintf(`{"task_id":"%s"}`, id),
				))
			}
			steps = append(steps,
				toolAcceptanceStep("bounded-list", "todo_list", `{"include_history":true}`),
				toolAcceptanceStep("forbidden-update", "todo_update", fmt.Sprintf(
					`{"task_id":"%s","expected_revision":1,"title":"leaked","idempotency_key":"forbidden-update"}`, adjacent.ID,
				)),
			)
		}
		steps = append(steps,
			toolAcceptanceStep("claim", "todo_claim", fmt.Sprintf(
				`{"task_id":"%s","idempotency_key":"worker-%d-claim"}`, child.ID, i,
			)),
			toolAcceptanceStep("progress", "todo_update", fmt.Sprintf(
				`{"task_id":"%s","expected_revision":1,"status":"in_progress","idempotency_key":"worker-%d-progress"}`, child.ID, i,
			)),
			toolAcceptanceStep("complete", "todo_update", fmt.Sprintf(
				`{"task_id":"%s","expected_revision":2,"status":"completed","result_summary":"worker %d done","idempotency_key":"worker-%d-complete"}`,
				child.ID, i, i,
			)),
			func(openrouter.ChatRequest) (openrouter.ChatResponse, error) {
				return assistantAcceptanceStep("done"), nil
			},
		)
		client := &acceptanceClient{steps: steps}
		workerAgent := acceptanceAgent(
			t,
			client,
			store,
			worker.session,
			memory.LeaseHolderID(fmt.Sprintf("acceptance-worker-%d", i)),
			resolved.Toolset,
		)
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			workerErrors <- workerAgent.Send(ctx, "Progress only the granted Task.", worker.events, nil)
		}()
	}
	close(start)
	wait.Wait()
	close(workerErrors)
	for err := range workerErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	probeOutput := strings.Join(workers[0].events.results, "\n")
	for _, forbidden := range []string{root.Title, adjacent.Title, siblingTask.Title, projectTask.Title} {
		if strings.Contains(probeOutput, forbidden) {
			t.Fatalf("delegated scope probe disclosed %q: %s", forbidden, probeOutput)
		}
	}
	if !strings.Contains(probeOutput, decomposition.Children[0].Title) || !strings.Contains(probeOutput, "not found") {
		t.Fatalf("delegated scope probe did not show bounded branch and denials: %s", probeOutput)
	}

	rootFinish := &acceptanceClient{steps: []acceptanceStep{
		toolAcceptanceStep("claim-root", "todo_claim", fmt.Sprintf(
			`{"task_id":"%s","idempotency_key":"acceptance-root-claim"}`, root.ID,
		)),
		toolAcceptanceStep("complete-root", "todo_update", fmt.Sprintf(
			`{"task_id":"%s","expected_revision":2,"status":"completed","result_summary":"tests pass","idempotency_key":"acceptance-root-complete"}`,
			root.ID,
		)),
		func(openrouter.ChatRequest) (openrouter.ChatResponse, error) {
			return assistantAcceptanceStep("release complete"), nil
		},
	}}
	ownerFinishEvents := &acceptanceEvents{}
	if err := acceptanceAgent(t, rootFinish, store, owner, "acceptance-owner-finish", resolved.Toolset).
		Send(ctx, "Finish the focused Task Tree.", ownerFinishEvents, nil); err != nil {
		t.Fatal(err)
	}

	tree, err := store.GetGlobalTaskTree(acceptanceTaskContext(owner, "inspect"), root.ID, task.TreeQuery{IncludeHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	tasks := flattenAcceptanceTree(tree)
	if len(tasks) != 3 {
		t.Fatalf("completed tree = %+v", tree)
	}
	var taskEvents []task.Event
	for _, value := range tasks {
		if value.Status != task.StatusCompleted {
			t.Fatalf("Task %s status=%s", value.ID, value.Status)
		}
		events, err := store.ListTaskEvents(acceptanceTaskContext(owner, "events"), value.ID)
		if err != nil {
			t.Fatal(err)
		}
		for j, event := range events {
			if event.Sequence != uint64(j+1) {
				t.Fatalf("Task %s unordered events = %+v", value.ID, events)
			}
		}
		taskEvents = append(taskEvents, events...)
	}
	var episodic []memory.Event
	for _, sessionID := range []memory.SessionID{owner.ID, workers[0].session.ID, workers[1].session.ID} {
		events, err := store.LoadEvents(ctx, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		episodic = append(episodic, events...)
	}
	sessionScopes := map[memory.SessionID]task.Scope{owner.ID: root.Scope}
	for i, worker := range workers {
		sessionScopes[worker.session.ID] = decomposition.Children[i].Scope
	}
	report, err := taskeval.Derive(taskeval.Input{
		Tasks: tasks, TaskEvents: taskEvents, EpisodicEvents: episodic, SessionScopes: sessionScopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.TaskCount != 3 || report.CompletionCount != 3 || report.AbandonmentCount != 0 ||
		report.DuplicateAttemptCount != 1 || report.RetryCount != 1 || report.ConflictCount != 1 ||
		report.DecompositionCount != 1 || report.RecoveryCount != 0 || report.EvidenceLinkCount != len(taskEvents) {
		t.Fatalf("whole-system evaluation report = %+v, Task events=%d", report, len(taskEvents))
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	taskEventJSON, err := json.Marshal(taskEvents)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"credential-marker", "Full prompt", "tests pass", "reasoning"} {
		if strings.Contains(string(reportJSON), forbidden) || strings.Contains(string(taskEventJSON), forbidden) {
			t.Fatalf("Task evidence retained source content %q\nreport=%s\nevents=%s", forbidden, reportJSON, taskEventJSON)
		}
	}

	storedReceipt, err := store.GetCompositionReceipt(ctx, owner.ID)
	if err != nil || !reflect.DeepEqual(storedReceipt, resolved.Receipt) {
		t.Fatalf("stored receipt = %+v, %v; want %+v", storedReceipt, err, resolved.Receipt)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	reopenedDB := db
	t.Cleanup(func() { _ = reopenedDB.Close() })
	store = eviedb.NewStore(db)
	reopenedReceipt, err := store.GetCompositionReceipt(ctx, owner.ID)
	if err != nil || !reflect.DeepEqual(reopenedReceipt, resolved.Receipt) {
		t.Fatalf("reopened receipt = %+v, %v", reopenedReceipt, err)
	}
	reopenedManager := acceptanceManager(t, store)
	resumed, err := reopenedManager.ResumeComposition(reopenedReceipt)
	if err != nil {
		t.Fatal(err)
	}
	focused, isError, err := resumed.Toolset.ExecuteWithApprovalAuthorizedCompletion(
		acceptanceTaskContext(owner, "focused-restart"),
		openrouter.ToolCall{ID: "focused-restart", Type: "function", Function: openrouter.FunctionCall{Name: "todo_list", Arguments: `{"include_history":true}`}},
		nil, nil, nil, nil,
	)
	if err != nil || isError || !strings.Contains(focused.Content, string(root.ID)) || strings.Contains(focused.Content, string(adjacent.ID)) {
		t.Fatalf("reopened focus = %q, isError=%v, err=%v", focused.Content, isError, err)
	}
	for i, worker := range workers {
		grant, err := store.GetTaskAccessGrant(ctx, worker.grant.ID)
		if err != nil || grant.GranteeSessionID != string(worker.session.ID) || grant.RootID != decomposition.Children[i].ID || grant.EndedAt != nil {
			t.Fatalf("reopened grant %d = %+v, %v", i, grant, err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	binary := buildTodoBinary(t)
	stdout, stderr, exit := runTodoProcess(t, binary, home, "get", "--session", string(owner.ID), "--tree", "--history", string(root.ID))
	if exit != 0 || stderr != "" || !strings.Contains(stdout, "id="+string(root.ID)) ||
		!strings.Contains(stdout, "id="+string(decomposition.Children[0].ID)) ||
		!strings.Contains(stdout, "id="+string(decomposition.Children[1].ID)) || strings.Count(stdout, "status=completed") != 3 {
		t.Fatalf("restarted CLI exit=%d\nstdout=%s\nstderr=%s", exit, stdout, stderr)
	}
}

func acceptanceManager(t *testing.T, service task.Service) *plugins.Manager {
	t.Helper()
	manager, err := plugins.NewManager(
		tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(service),
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

func assertAcceptanceComposition(t *testing.T, manager *plugins.Manager, resolved plugins.ResolvedComposition) {
	t.Helper()
	for _, name := range []string{"todo_list", "todo_get", "todo_add", "todo_update", "todo_decompose", "todo_claim", "todo_release", "youtube_transcript", "youtube_scrape_channel"} {
		if acceptanceSchemaCount(resolved.Toolset, name) != 1 {
			t.Fatalf("standard schema %q count=%d", name, acceptanceSchemaCount(resolved.Toolset, name))
		}
	}
	for _, schema := range resolved.Toolset.Schemas() {
		if strings.Contains(schema.Function.Name, "grant") || strings.Contains(schema.Function.Name, "delete") {
			t.Fatalf("standard exposes forbidden model surface %q", schema.Function.Name)
		}
	}
	providers := map[string]int{}
	for _, provider := range resolved.Receipt.Providers {
		providers[provider.ID]++
	}
	if providers[string(plugins.TodoPluginID)] != 1 || providers[string(plugins.YouTubePluginID)] != 1 {
		t.Fatalf("current providers = %+v", resolved.Receipt.Providers)
	}
	capabilities := map[string]int{}
	contracts := map[string]string{}
	for _, capability := range resolved.Receipt.Capabilities {
		if len(capability.SchemaSHA256) != 64 {
			t.Fatalf("Capability lacks schema hash: %+v", capability)
		}
		capabilities[capability.ID]++
		contracts[capability.ID] = capability.ContractVersion
	}
	expectedContracts := map[string]string{
		"todo.add":               "1.4.0",
		"todo.decompose":         "1.0.0",
		"youtube.transcript":     "1.0.0",
		"youtube.scrape_channel": "1.0.0",
	}
	for id, contract := range expectedContracts {
		if capabilities[id] != 1 {
			t.Fatalf("current Capability %q count=%d", id, capabilities[id])
		}
		if contracts[id] != contract {
			t.Fatalf("current Capability %q contract=%q, want %q", id, contracts[id], contract)
		}
	}
	presets, err := manager.InspectPresets()
	if err != nil || len(presets) != 1 || presets[0].ID != plugins.StandardPresetID {
		t.Fatalf("preset regressions = %+v, %v", presets, err)
	}
}

func acceptanceSchemaCount(toolset tools.Toolset, name string) int {
	count := 0
	for _, schema := range toolset.Schemas() {
		if schema.Function.Name == name {
			count++
		}
	}
	return count
}

func acceptanceAgent(
	t *testing.T,
	client agent.Client,
	store *eviedb.Store,
	session memory.Session,
	holder memory.LeaseHolderID,
	toolset tools.Toolset,
) *agent.Session {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile("acceptance/model", 300000, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewWithToolset(
		client, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), toolset,
	)
}

func acceptanceTaskContext(session memory.Session, runID string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), RunID: runID,
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID), ParentSessionID: string(session.ParentSessionID),
	})
}

func createAcceptanceTask(t *testing.T, store *eviedb.Store, session memory.Session, key, title string) task.Task {
	t.Helper()
	created, err := store.CreateGlobalTask(acceptanceTaskContext(session, key), task.CreateInput{Title: title, IdempotencyKey: task.IdempotencyKey(key)})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func toolAcceptanceStep(id, name, arguments string) acceptanceStep {
	return func(openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant", ToolCalls: []openrouter.ToolCall{{
				ID: id, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: arguments},
			}},
		}}}}, nil
	}
}

func assistantAcceptanceStep(content string) openrouter.ChatResponse {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: content}}}}
}

func decodeAcceptanceToolResult(req openrouter.ChatRequest, callID string, destination any) error {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		message := req.Messages[i]
		if message.Role == "tool" && message.ToolCallID == callID {
			if err := json.Unmarshal([]byte(message.Content), destination); err != nil {
				return fmt.Errorf("decode %s result: %w", callID, err)
			}
			return nil
		}
	}
	return fmt.Errorf("tool result %q not found", callID)
}

func containsAcceptanceResult(events *acceptanceEvents, needle string) bool {
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, result := range events.results {
		if strings.Contains(result, needle) {
			return true
		}
	}
	return false
}

func flattenAcceptanceTree(tree task.Tree) []task.Task {
	values := []task.Task{tree.Task}
	for _, child := range tree.Children {
		values = append(values, flattenAcceptanceTree(child)...)
	}
	return values
}
