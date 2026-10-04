package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	receipts "github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Stage 14 sub-stage 10 (2026-10-04): a workflow still cannot merge, but
// the owner's chat in the run's Workspace takes a finished run's build
// branch through the Stage 12 gate with propose_build_merge: Evie verifies
// the branch, rebases it and verifies again, and only the owner's approval
// of the card fast-forwards the target.

func requireGateSandbox(t *testing.T) {
	t.Helper()
	if _, err := tools.DetectWorkerSandbox(); err != nil {
		t.Skipf("no OS sandbox on this host: %v", err)
	}
}

// mergeChat is a Standard chat in w whose live turn committed a
// propose_build_merge call for execution.
func mergeChat(t *testing.T, store *eviedb.Store, w memory.Workspace, execution string) (delegation.Parent, string) {
	t.Helper()
	ctx := context.Background()
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	receipt := receipts.Receipt{FormatVersion: receipts.FormatVersion, Preset: receipts.PresetIdentity{ID: "standard", Version: "sha256:" + hash}, EvieVersion: "1.0.0",
		Providers: []receipts.Provider{{ID: "subagents", ImplementationVersion: "1.0.0"}, {ID: "todo", ImplementationVersion: "1.8.0"}},
		Capabilities: []receipts.Capability{
			{ID: delegation.MergeCapabilityID, ProviderID: "subagents", ContractVersion: "1.0.0", SchemaSHA256: hash},
			{ID: "todo.get", ProviderID: "todo", ContractVersion: "1.1.0", SchemaSHA256: hash},
		}}
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(uuid.NewString()), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "merge the run's build"})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"execution_id": execution})
	call := "call-" + uuid.NewString()
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: call, Name: delegation.MergeToolName, Arguments: string(args)}})
	intent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: root.ID,
		ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return delegation.Parent{Scope: session.ScopeContext(), Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}, call
}

// proposeRunMerge runs propose_build_merge through the Subagents plugin and
// the approval dispatcher, which decide answers.
func proposeRunMerge(t *testing.T, supervisor *subagents.Supervisor, p delegation.Parent, call, execution string, approve bool) (string, bool, int) {
	t.Helper()
	var merge tools.Tool
	for _, c := range plugins.NewSubagents(supervisor).ToolCapabilities() {
		if c.Tool.Schema.Function.Name == delegation.MergeToolName {
			merge = c.Tool
		}
	}
	asked := 0
	decide := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
		asked++
		if approve {
			return tools.Approved
		}
		return tools.Declined
	}
	args, _ := json.Marshal(map[string]string{"execution_id": execution})
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: p.Scope, Lease: p.Lease, SourceEventID: p.SourceEventID, IntentEventID: p.IntentEventID})
	message, isErr, err := tools.NewToolset([]tools.Tool{merge}).ExecuteWithApprovalAuthorizedCompletion(ctx,
		openrouter.ToolCall{ID: call, Type: "function", Function: openrouter.FunctionCall{Name: delegation.MergeToolName, Arguments: string(args)}}, decide, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return message.Content, isErr, asked
}

func TestWorkflowBuildBranchMergesThroughTheGateFromTheOwnersChat(t *testing.T) {
	requireGateSandbox(t)
	ctx := context.Background()
	repo := ownerRepo(t)
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if toolResults(r) == 0 {
			return toolCall("write_file", map[string]string{"path": "hello.txt", "content": "hello\n"}), nil
		}
		return finalReport("Added hello.txt.", map[string]any{"answer": "added"}), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.folderWorkspace(repo, true)
	if _, err := h.store.SetWorkspaceVerify(ctx, w.ID, 0, "", "test -f hello.txt"); err != nil {
		t.Fatal(err)
	}
	h.approve(h.propose(w.ID, localDefinition(t, "build"), fixtures(t, localFixtures("build"))))
	master := gitIn(t, repo, "rev-parse", "master")
	work := h.buildTask("Add hello")
	res, err := h.startRun(h.rt, w.ID, "local-build", map[string]string{"note": "add hello.txt", "task_id": string(work.ID)})
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v, %v", res.Run.Run, err)
	}
	execution := res.Run.Workers[0].ExecutionID
	supervisor, err := subagents.New(h.store, delegation.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	// The run's status names the branch it left for the gate.
	status, err := h.rt.ChatRunStatus(ctx, memory.ScopeContext{WorkspaceID: w.ID}, res.Run.Run.ID)
	if err != nil || len(status.Builds) != 1 || status.Builds[0].ExecutionID != execution {
		t.Fatalf("status builds = %+v, %v", status.Builds, err)
	}

	// A chat in another Workspace cannot take it to the gate.
	other := h.folderWorkspace(repo, true)
	foreign, _ := mergeChat(t, h.store, other, execution)
	if _, err := supervisor.ProposeMerge(ctx, foreign, execution, ""); !errors.Is(err, delegation.ErrNotFound) {
		t.Fatalf("foreign chat: %v", err)
	}

	// Declined: verified and shown, nothing merged.
	p, call := mergeChat(t, h.store, w, execution)
	content, isErr, asked := proposeRunMerge(t, supervisor, p, call, execution, false)
	if asked != 1 || isErr || !strings.Contains(content, "declined") || gitIn(t, repo, "rev-parse", "master") != master {
		t.Fatalf("declined merge = %q (error %v, asked %d)", content, isErr, asked)
	}
	// Approved: the owner's approval fast-forwards master.
	p, call = mergeChat(t, h.store, w, execution)
	content, isErr, asked = proposeRunMerge(t, supervisor, p, call, execution, true)
	if asked != 1 || isErr {
		t.Fatalf("approved merge = %q (error %v, asked %d)", content, isErr, asked)
	}
	if files := gitIn(t, repo, "show", "--name-only", "--format=", "master"); files != "hello.txt" {
		t.Fatalf("master's last commit changed %q", files)
	}
	merged, _, err := h.store.RunWorkerExecution(ctx, res.Run.Run.ID, execution)
	if err != nil || merged.Gate == nil || merged.Gate.Merged == nil || merged.Run == nil || merged.Run.RunID != res.Run.Run.ID {
		t.Fatalf("merged execution = %+v, %v", merged, err)
	}
}

// A run that has not finished keeps its build branch out of the gate: its
// workflow may still continue that worker.
func TestWorkflowUnfinishedRunsBuildStaysOutOfTheGate(t *testing.T) {
	requireGateSandbox(t)
	ctx := context.Background()
	repo := ownerRepo(t)
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if toolResults(r) == 0 {
			return toolCall("write_file", map[string]string{"path": "hello.txt", "content": "hello\n"}), nil
		}
		return finalReport("Added hello.txt.", map[string]any{"answer": "added"}), nil
	}}
	h, _ := newAgentHarness(t, nil, model)
	w := h.folderWorkspace(repo, true)
	if _, err := h.store.SetWorkspaceVerify(ctx, w.ID, 0, "", "test -f hello.txt"); err != nil {
		t.Fatal(err)
	}
	// A build step followed by a question keeps the run waiting after its
	// worker finished.
	h.approve(h.propose(w.ID, writeDefinition(t, "build-then-ask", strings.Replace(localStep("build"), "outputs:\n  answer: $steps.work.output.result.answer\n",
		"    next: ask\n  - id: ask\n    kind: interrupt\n    message: Merge it?\n    response_schema: schemas/ack.schema.json\noutputs:\n  answer: $steps.work.output.result.answer\n", 1),
		map[string]string{"prompts/work.md": "Do the work.\n", "schemas/finding.schema.json": findingSchema,
			"schemas/ack.schema.json": `{"type":"object","additionalProperties":false,"required":["ok"],"properties":{"ok":{"type":"boolean"}}}`}),
		fixtures(t, `{"inputs": {"note": "n", "task_id": "t"}, "steps": {"work": {"output": {"status": "succeeded", "summary": "s", "sources": [], "result": {"answer": "a"}}, "tokens": 1000}, "ask": {"response": {"ok": true}}}}`)))
	work := h.buildTask("Add hello")
	res, err := h.startRun(h.rt, w.ID, "local-build", map[string]string{"note": "add hello.txt", "task_id": string(work.ID)})
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunWaitingForHuman {
		t.Fatalf("run = %+v, %v", res.Run.Run, err)
	}
	execution := res.Run.Workers[0].ExecutionID
	supervisor, err := subagents.New(h.store, delegation.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := mergeChat(t, h.store, w, execution)
	if _, err := supervisor.ProposeMerge(ctx, p, execution, ""); err == nil || !strings.Contains(err.Error(), "once the run has finished") {
		t.Fatalf("merge of a waiting run's build: %v", err)
	}
}
