package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// Stage 14 sub-stage 10 (2026-10-04): Evie starts a run of a Workspace's
// active workflow from the owner's chat in that Workspace. The run records
// origin surface "chat" and its session, and the store admits it only for
// a top-level session of the run's Workspace whose pinned composition
// exposes workflows.start, while that session's turn lease is live and its
// committed workflow_start call has no outcome yet. Owner-only operations
// stay owner-only.

func workflowsReceipt(capabilities ...string) composition.Receipt {
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	receipt := composition.Receipt{
		FormatVersion: composition.FormatVersion,
		Preset:        composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion:   "1.0.0",
		Providers:     []composition.Provider{{ID: "workflows", ImplementationVersion: "1.0.0"}},
	}
	for _, id := range capabilities {
		receipt.Capabilities = append(receipt.Capabilities, composition.Capability{ID: id, ProviderID: "workflows", ContractVersion: "1.0.0", SchemaSHA256: hash})
	}
	return receipt
}

// chatTurn is a Workspace chat whose live turn committed a call of tool.
func chatTurn(t *testing.T, s *Store, w memory.Workspace, receipt composition.Receipt, tool string) (memory.Session, WorkflowChatOrigin) {
	t.Helper()
	ctx := context.Background()
	session, err := s.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(uuid.NewString()), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := s.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "research this for me"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-1", Name: tool, Arguments: `{}`}})
	intent, err := s.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: root.ID, ExecutionID: memory.ExecutionID("invocation-" + uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return session, WorkflowChatOrigin{Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}
}

func chatStart(a WorkflowActivation, key string, session memory.Session, origin WorkflowChatOrigin) WorkflowRunStart {
	start := runStart(a, key)
	start.Surface, start.SessionID, start.Chat = "chat", session.ID, &origin
	return start
}

func TestWorkflowChatStartNeedsTheSessionsLiveTurnAndCommittedCall(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, a := splitBillActivation(t, r.s)
	session, origin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID, WorkflowStatusCapabilityID), WorkflowStartToolName)
	run, existing, err := r.s.StartWorkflowRun(ctx, chatStart(a, "split-bill:chat", session, origin))
	if err != nil || existing {
		t.Fatalf("chat start = %+v, %v, %v", run, existing, err)
	}
	r.reopen()
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || d.Run.Surface != "chat" || d.Run.SessionID != session.ID || d.Run.State != WorkflowRunQueued {
		t.Fatalf("chat run = %+v, %v", d.Run, err)
	}
	// The same key returns the run that holds it.
	if again, existing, err := r.s.StartWorkflowRun(ctx, chatStart(a, "split-bill:chat", session, origin)); err != nil || !existing || again.ID != run.ID {
		t.Fatalf("duplicate chat start = %+v, %v, %v", again, existing, err)
	}

	refused := func(name string, start WorkflowRunStart, want error) {
		t.Helper()
		if _, _, err := r.s.StartWorkflowRun(ctx, start); !errors.Is(err, want) {
			t.Fatalf("%s: %v, want %v", name, err, want)
		}
	}
	noOrigin := runStart(a, "k-no-origin")
	noOrigin.Surface, noOrigin.SessionID = "chat", session.ID
	refused("chat without its turn", noOrigin, ErrWorkflowRunSurface)
	ownerWithTurn := chatStart(a, "k-owner", session, origin)
	ownerWithTurn.Surface = "cli"
	refused("an owner surface claiming a chat turn", ownerWithTurn, ErrWorkflowRunSurface)
	noSession := chatStart(a, "k-no-session", session, origin)
	noSession.SessionID = ""
	refused("chat without its session", noSession, ErrWorkflowRunSurface)
	rerun := chatStart(a, "k-rerun", session, origin)
	rerun.RerunOf, rerun.LogicalKey, rerun.Reason = run.ID, "split-bill:chat", "again"
	refused("a rerun override from chat", rerun, ErrWorkflowRunSurface)

	stale := origin
	stale.Lease.FencingToken++
	refused("a stale turn lease", chatStart(a, "k-stale", session, stale), ErrWorkflowChatStart)
	_, other := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), "web_fetch")
	otherSession, _ := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), WorkflowStartToolName)
	refused("another session's turn", chatStart(a, "k-other-turn", otherSession, other), ErrWorkflowChatStart)
	wrongTool, wrongOrigin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), "web_fetch")
	refused("a call of another tool", chatStart(a, "k-tool", wrongTool, wrongOrigin), ErrWorkflowChatStart)
	lacking, lackingOrigin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStatusCapabilityID), WorkflowStartToolName)
	refused("a composition without workflows.start", chatStart(a, "k-capability", lacking, lackingOrigin), ErrWorkflowChatStart)

	// A call that already has an outcome cannot start another run.
	done, doneOrigin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), WorkflowStartToolName)
	var intent memory.Event
	if err := r.db.QueryRowContext(ctx, `SELECT execution_id FROM events WHERE id=?`, doneOrigin.IntentEventID).Scan(&intent.ExecutionID); err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: "call-1"})
	if _, err := r.s.AppendEventWithLease(ctx, done.ID, doneOrigin.Lease.HolderID, doneOrigin.Lease.FencingToken, memory.EventInput{Type: memory.EventToolSucceeded, Role: memory.RoleTool, ParentID: doneOrigin.IntentEventID, ExecutionID: intent.ExecutionID, Content: "{}", Payload: result}); err != nil {
		t.Fatal(err)
	}
	refused("a call that already has an outcome", chatStart(a, "k-done", done, doneOrigin), ErrWorkflowChatStart)

	// A session of another Workspace cannot start this Workspace's run.
	elsewhere := notesWorkspace(t, r.s)
	foreign, foreignOrigin := chatTurn(t, r.s, elsewhere, workflowsReceipt(WorkflowStartCapabilityID), WorkflowStartToolName)
	refused("a session of another Workspace", chatStart(a, "k-foreign", foreign, foreignOrigin), ErrWorkflowRunSession)

	// The owner's cancellation and other owner records refuse a chat surface.
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: run.ID, Surface: "chat"}); err == nil {
		t.Fatal("a chat surface cancelled a run")
	}
}

// A database from before sub-stage 10 refuses origin "chat" in its table's
// check; the upgrade rebuilds workflow_runs once with every row (and rowid),
// index and trigger kept.
func TestWorkflowRunsTableUpgradesToChatOrigins(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:before"))
	if err != nil {
		t.Fatal(err)
	}
	objects := func() string {
		t.Helper()
		rows, err := r.db.QueryContext(ctx, `SELECT type||' '||name||' '||COALESCE(sql,'') FROM sqlite_master WHERE tbl_name='workflow_runs' AND type IN ('index','trigger') ORDER BY type,name`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return strings.Join(out, "\n")
	}
	before := objects()
	if !strings.Contains(before, "workflow_runs_finish_after_workers") || !strings.Contains(before, "workflow_runs_cancelled_by_owner") {
		t.Fatalf("triggers before = %s", before)
	}
	var rowid int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, run.ID).Scan(&rowid); err != nil {
		t.Fatal(err)
	}
	// Put the table back in its sub-stage 9 shape.
	if err := rebuildWorkflowRuns(ctx, r.db, workflowRunSurfaces, workflowRunSurfacesBeforeChat); err != nil {
		t.Fatal(err)
	}
	session, origin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), WorkflowStartToolName)
	if _, _, err := r.s.StartWorkflowRun(ctx, chatStart(a, "split-bill:chat", session, origin)); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("the earlier table accepted a chat run: %v", err)
	}
	r.reopen()
	if after := objects(); after != before {
		t.Fatalf("indexes and triggers changed:\n%s\nwant\n%s", after, before)
	}
	var kept int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, run.ID).Scan(&kept); err != nil || kept != rowid {
		t.Fatalf("rowid = %d, %v; want %d", kept, err, rowid)
	}
	rows, err := r.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("the upgrade left a foreign key violation")
	}
	rows.Close()
	session, origin = chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID), WorkflowStartToolName)
	if _, _, err := r.s.StartWorkflowRun(ctx, chatStart(a, "split-bill:chat", session, origin)); err != nil {
		t.Fatalf("chat start after the upgrade: %v", err)
	}
	// The terminal-history and pinning triggers still hold.
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET run_key='other' WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("pinned run key changed: %v", err)
	}
	// A chat run always names its session.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO workflow_runs SELECT 'orphan',workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,composition,'other-key',inputs,state,outputs,error,'chat',NULL,created_at,started_at,finished_at,updated_at,not_before,source FROM workflow_runs WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("a chat run without its session was stored: %v", err)
	}
}
