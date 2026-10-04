package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	receipts "github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/google/uuid"
)

// Stage 14 sub-stage 10 (2026-10-04): a chat in a Workspace starts a run of
// that Workspace's ACTIVE workflow through workflow_start and reads it
// through workflow_status. The runtime takes the Workspace and session from
// the invoking turn, applies every gate an owner's start has, records the
// run with origin "chat" in the background, and never lets the chat
// propose, approve, deactivate, cancel, answer, reconcile, substitute or
// rerun.

func chatReceipt() receipts.Receipt {
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	return receipts.Receipt{FormatVersion: receipts.FormatVersion, Preset: receipts.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion: "1.0.0", Providers: []receipts.Provider{{ID: "workflows", ImplementationVersion: "1.0.0"}},
		Capabilities: []receipts.Capability{
			{ID: string(plugins.WorkflowsStartCapabilityID), ProviderID: "workflows", ContractVersion: "1.0.0", SchemaSHA256: hash},
			{ID: string(plugins.WorkflowsStatusCapabilityID), ProviderID: "workflows", ContractVersion: "1.0.0", SchemaSHA256: hash},
		}}
}

// chatCall is a Workspace chat whose live turn committed a workflow_start
// call; it returns the call's request without a workflow.
func chatCall(t *testing.T, store *eviedb.Store, w memory.Workspace) plugins.WorkflowChatStart {
	t.Helper()
	ctx := context.Background()
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, chatReceipt())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(uuid.NewString()), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "split the bill"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-1", Name: plugins.WorkflowStartToolName, Arguments: `{}`}})
	intent, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: root.ID,
		ExecutionID: memory.ExecutionID("invocation-" + uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return plugins.WorkflowChatStart{Scope: session.ScopeContext(), Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}
}

func (h *harness) runs(ws memory.WorkspaceID) []eviedb.WorkflowRun {
	h.t.Helper()
	list, err := h.rt.ListRuns(context.Background(), RunFilter{WorkspaceID: ws})
	if err != nil {
		h.t.Fatal(err)
	}
	return list
}

func TestWorkflowChatStartsOnlyActiveWorkflowsThroughEveryGate(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace()
	split := h.approve(h.propose(w.ID, filepath.Join(testdata, "split-bill"), fixtures(t, `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`)))
	call := chatCall(t, h.store, w)
	call.Workflow = "split-bill"
	call.Inputs = map[string]json.RawMessage{"bill_id": json.RawMessage(`"dinner-2"`), "total_cents": json.RawMessage(`1000`), "people": json.RawMessage(`4`)}
	started, err := h.rt.StartFromChat(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if started.RunID == "" || started.State != "queued" || started.StartedFrom != "chat" || started.Existing || started.MaxTokens != 1 {
		t.Fatalf("started = %+v", started)
	}
	d, err := h.rt.InspectRun(ctx, started.RunID)
	if err != nil || d.Run.Surface != "chat" || d.Run.SessionID != call.Scope.SessionID || d.Run.WorkspaceID != w.ID {
		t.Fatalf("run = %+v, %v", d.Run, err)
	}
	if text := RunText(d); !strings.Contains(text, "Started by Evie in the owner's chat (workflow_start)") || !strings.Contains(text, "from session "+string(call.Scope.SessionID)) {
		t.Fatalf("inspection:\n%s", text)
	}
	// Starting it again returns the run that holds the key.
	again, err := h.rt.StartFromChat(ctx, call)
	if err != nil || !again.Existing || again.RunID != started.RunID {
		t.Fatalf("again = %+v, %v", again, err)
	}
	// It runs in the background like any run; its status carries outputs.
	if _, err := h.rt.execute(ctx, started.RunID); err != nil {
		t.Fatal(err)
	}
	status, err := h.rt.ChatRunStatus(ctx, call.Scope, started.RunID)
	if err != nil || status.State != "succeeded" || !strings.Contains(status.OutputsJSON, `"shares"`) || len(status.Steps) == 0 {
		t.Fatalf("status = %+v, %v", status, err)
	}
	catalog, err := h.rt.ChatWorkflows(ctx, call.Scope)
	if err != nil || len(catalog.Workflows) != 1 || catalog.Workflows[0].Name != "split-bill" || catalog.Workflows[0].Version != split.Version ||
		!strings.Contains(strings.Join(catalog.Workflows[0].Inputs, " "), "total_cents") || len(catalog.Runs) != 1 || catalog.Runs[0].RunID != started.RunID {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}

	before := len(h.runs(w.ID))
	refused := func(name string, req plugins.WorkflowChatStart, fragment string) {
		t.Helper()
		_, err := h.rt.StartFromChat(ctx, req)
		if err == nil || !strings.Contains(err.Error(), fragment) {
			t.Fatalf("%s: %v, want %q", name, err, fragment)
		}
		if n := len(h.runs(w.ID)); n != before {
			t.Fatalf("%s recorded a run", name)
		}
	}
	bad := call
	bad.Inputs = map[string]json.RawMessage{"bill_id": json.RawMessage(`"x"`), "people": json.RawMessage(`"four"`)}
	refused("invalid inputs", bad, "invalid workflow run input")
	unknown := call
	unknown.Workflow = "notes-digest"
	refused("a workflow that was never approved", unknown, "not found")
	global := call
	global.Scope.WorkspaceID, global.Scope.WorkspaceRevision = "", ""
	refused("a chat outside a Workspace", global, "Workspace")
	stale := chatCall(t, h.store, w)
	stale.Workflow, stale.Inputs = "split-bill", map[string]json.RawMessage{"bill_id": json.RawMessage(`"dinner-3"`), "total_cents": json.RawMessage(`10`), "people": json.RawMessage(`1`)}
	stale.Lease.FencingToken++
	refused("a stale turn", stale, "chat")
	// Deactivated: no chat start, nothing recorded.
	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: "split-bill", Version: split.Version, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	fresh := chatCall(t, h.store, w)
	fresh.Workflow, fresh.Inputs = "split-bill", map[string]json.RawMessage{"bill_id": json.RawMessage(`"dinner-4"`), "total_cents": json.RawMessage(`10`), "people": json.RawMessage(`1`)}
	refused("an inactive workflow", fresh, "not active")
	if catalog, err := h.rt.ChatWorkflows(ctx, fresh.Scope); err != nil || len(catalog.Workflows) != 0 {
		t.Fatalf("catalog after deactivation = %+v, %v", catalog, err)
	}
	// Another Workspace's chat cannot read this run.
	other := h.workspace()
	foreign := chatCall(t, h.store, other)
	if _, err := h.rt.ChatRunStatus(ctx, foreign.Scope, started.RunID); !errors.Is(err, eviedb.ErrWorkflowRunNotFound) {
		t.Fatalf("foreign status: %v", err)
	}
}

// AI steps and workers send run data to the model provider only with the
// owner's EVIE_WORKFLOW_REMOTE_MODELS=on; a chat start respects that too.
func TestWorkflowChatStartRespectsRemoteModels(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.modelsOff = "AI steps are off: set EVIE_WORKFLOW_REMOTE_MODELS=on"
	h.rt = h.runtime()
	w := h.workspace()
	h.approve(h.propose(w.ID, filepath.Join(testdata, "receipt-split"), receiptFixtures(t)))
	call := chatCall(t, h.store, w)
	call.Workflow = "receipt-split"
	call.Inputs = map[string]json.RawMessage{"receipt_id": json.RawMessage(`"r-1"`), "receipt_text": json.RawMessage(`"TOTAL 12.00"`), "people": json.RawMessage(`3`), "tip_cents": json.RawMessage(`300`)}
	if _, err := h.rt.StartFromChat(ctx, call); !errors.Is(err, ErrModelsUnavailable) || !strings.Contains(err.Error(), "EVIE_WORKFLOW_REMOTE_MODELS") {
		t.Fatalf("start without remote models: %v", err)
	}
}

// Every owner operation refuses the chat surface: a model can start and
// read approved workflows, nothing more.
func TestWorkflowChatCannotProposeApproveCancelAnswerOrReconcile(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace()
	card := h.propose(w.ID, filepath.Join(testdata, "split-bill"), fixtures(t, `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`))
	if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Template: "deep-research", Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("chat proposal: %v", err)
	}
	approval := approvalFor(card)
	approval.Surface = "chat"
	if _, err := h.rt.Approve(ctx, approval); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("chat approval: %v", err)
	}
	a := h.approve(card)
	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: a.Name, Version: a.Version, Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("chat deactivation: %v", err)
	}
	call := chatCall(t, h.store, w)
	call.Workflow = "split-bill"
	call.Inputs = map[string]json.RawMessage{"bill_id": json.RawMessage(`"dinner-9"`), "total_cents": json.RawMessage(`10`), "people": json.RawMessage(`1`)}
	run, err := h.rt.StartFromChat(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.Cancel(ctx, CancelRequest{RunID: run.RunID, Surface: "chat"}); err == nil {
		t.Fatal("a chat cancelled a run")
	}
	if _, err := h.rt.Answer(ctx, AnswerRequest{RunID: run.RunID, Step: "split", Answer: json.RawMessage(`{}`), Surface: "chat"}); err == nil {
		t.Fatal("a chat answered a run")
	}
	if _, err := h.rt.Reconcile(ctx, Reconciliation{RunID: run.RunID, Step: "split", Outcome: "happened", Surface: "chat"}); err == nil {
		t.Fatal("a chat reconciled a run")
	}
	if _, err := h.rt.Substitute(ctx, Substitution{RunID: run.RunID, Capability: "notes.list", Version: "1.0.0", Surface: "chat"}); err == nil {
		t.Fatal("a chat substituted a provider")
	}
	// A chat run request cannot carry the owner's rerun override.
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", RerunOf: run.RunID, Reason: "again", Surface: "chat", SessionID: call.Scope.SessionID}); !errors.Is(err, eviedb.ErrWorkflowRunSurface) {
		t.Fatalf("chat rerun: %v", err)
	}
}
