package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Migration (Stage 14 sub-stage 12; ADR 0025, 0030): only the owner's
// explicit request moves an unfinished run to a newer approved version. It
// creates a linked Successor Run pinned to that version, which resumes from
// the predecessor's checkpoint carrying only the state the new version's
// reads accept and the predecessor's Effect Receipts (referenced, never
// performed again); the predecessor is superseded and its history is never
// rewritten; the Run Key and any schedule slot pass to the successor.
// Every test uses a real temporary SQLite file.

// splitBillV2 is split-bill whose leftover step may retry: a behavior
// change that keeps every read the run's state must satisfy.
func splitBillV2(t *testing.T) string {
	t.Helper()
	dir := copyDefinition(t, "split-bill")
	path := filepath.Join(dir, "workflow.yaml")
	text := readFile(t, path)
	old := "  - id: leftover\n    kind: deterministic\n"
	if !strings.Contains(text, old) {
		t.Fatal("split-bill changed")
	}
	writeFile(t, path, strings.Replace(text, old, old+"    retry:\n      max_attempts: 2\n", 1))
	return dir
}

// pauseAfterSplit deactivates version 1 while its split step runs, so the
// run pauses at its next node boundary.
func (h *runHarness) pauseAfterSplit(ws memory.WorkspaceID) {
	h.t.Helper()
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		out, err := v.CallFunction(step, input)
		if step == "split" {
			if _, derr := h.store.DeactivateWorkflow(context.Background(), ws, "split-bill", 1, "replaced", "cli"); derr != nil {
				h.t.Error(derr)
			}
		}
		return out, err
	}
}

func (h *runHarness) migrate(req MigrationRequest) MigrationResult {
	h.t.Helper()
	if req.Surface == "" {
		req.Surface = "cli"
	}
	res, err := h.rt.Migrate(context.Background(), req)
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func stepsOf(d eviedb.WorkflowRunDetail) []string {
	out := []string{}
	for _, a := range d.Attempts {
		out = append(out, a.Step)
	}
	return out
}

func TestWorkflowMigrationSuccessorResumesAndCompletes(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	h.pauseAfterSplit(w.ID)
	pred := h.start(w.ID, billInputs("dinner-1", 1000, 3)).Run.Run
	if pred.State != eviedb.WorkflowRunWaitingForDependency || pred.NextStep != "check" || !strings.Contains(pred.Error, "deactivated") {
		t.Fatalf("predecessor = %+v", pred)
	}
	h.rt.call = nil
	// Approving a newer version migrates nothing by itself.
	h.approve(h.propose(w.ID, splitBillV2(t), splitBillFixtures(t)))
	before := h.inspect(pred.ID)
	if before.Run.State != eviedb.WorkflowRunWaitingForDependency || before.Run.Version != 1 {
		t.Fatalf("a newer approval changed the run: %+v", before.Run)
	}

	res := h.migrate(MigrationRequest{RunID: pred.ID, ToVersion: 2, Reason: "carry dinner-1 to version 2"})
	succ := res.Successor
	if succ.Run.State != eviedb.WorkflowRunSucceeded || succ.Run.Version != 2 || succ.Run.RunKey != pred.RunKey || succ.Run.Surface != pred.Surface ||
		!reflect.DeepEqual(decoded(t, succ.Run.Outputs), dinnerOutputs) {
		t.Fatalf("successor = %+v", succ.Run)
	}
	if got := stepsOf(succ); !reflect.DeepEqual(got, []string{"check", "leftover"}) {
		t.Fatalf("the successor ran %v; split had already run", got)
	}
	first := succ.Checkpoints[0]
	if first.Next != "check" || !reflect.DeepEqual(decoded(t, first.State), map[string]any{"split": decoded(t, before.Checkpoints[1].State).(map[string]any)["split"]}) {
		t.Fatalf("the successor's first checkpoint = %+v", first)
	}
	m := succ.Predecessor
	if m == nil || m.PredecessorID != pred.ID || m.SuccessorID != succ.Run.ID || m.FromVersion != 1 || m.ToVersion != 2 || m.Checkpoint != 1 || m.Resume != "check" ||
		m.Reason != "carry dinner-1 to version 2" || m.Actor != "owner" || m.Surface != "cli" || m.Carried.Attempts != 1 || m.LogicalKey != pred.RunKey {
		t.Fatalf("migration = %+v", m)
	}
	if res.Migration.SuccessorID != succ.Run.ID || res.Predecessor.State != eviedb.WorkflowRunSuperseded {
		t.Fatalf("result = %+v", res.Migration)
	}

	// The predecessor is superseded; nothing else about it changed.
	after := h.inspect(pred.ID)
	if after.Run.State != eviedb.WorkflowRunSuperseded || after.Run.FinishedAt == nil || !strings.Contains(after.Run.Error, succ.Run.ID) ||
		after.Successor == nil || after.Successor.SuccessorID != succ.Run.ID {
		t.Fatalf("predecessor after = %+v", after.Run)
	}
	if !reflect.DeepEqual(after.Attempts, before.Attempts) || !reflect.DeepEqual(after.Checkpoints, before.Checkpoints) ||
		!reflect.DeepEqual(after.Events, before.Events) || string(after.Run.Inputs) != string(before.Run.Inputs) || after.Run.Version != 1 ||
		after.Run.ExecutableHash != before.Run.ExecutableHash || string(after.Run.Composition) != string(before.Run.Composition) {
		t.Fatalf("the predecessor's history changed")
	}
	// The Run Key moved with the work: a start of the same key is the
	// successor, and the superseded run cannot be migrated again.
	again := h.start(w.ID, billInputs("dinner-1", 1000, 3))
	if !again.Existing || again.Run.Run.ID != succ.Run.ID {
		t.Fatalf("a start of the migrated key = %+v", again.Run.Run)
	}
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 2, Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowRunFinished) {
		t.Fatalf("migrating a superseded run: %v", err)
	}
	text := RunText(after)
	if !strings.Contains(text, "Superseded by run "+succ.Run.ID) || !strings.Contains(RunText(succ), "Successor of run "+pred.ID) {
		t.Fatalf("inspection:\n%s\n%s", text, RunText(succ))
	}
}

func TestWorkflowMigrationRefusals(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	h.pauseAfterSplit(w.ID)
	pred := h.start(w.ID, billInputs("dinner-1", 1000, 3)).Run.Run
	h.rt.call = nil

	// Only the owner migrates: no model or chat surface.
	for _, surface := range []string{"chat", "tool", ""} {
		if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 2, Surface: surface}); !errors.Is(err, eviedb.ErrWorkflowRunSurface) {
			t.Fatalf("surface %q: %v", surface, err)
		}
	}
	// No newer active version yet.
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 2, Surface: "cli"}); !errors.Is(err, ErrMigrationRefused) {
		t.Fatalf("without a newer version: %v", err)
	}
	// A newer version whose reads the run's state does not satisfy: it
	// splits into four, so its outputs read four shares.
	dir := copyDefinition(t, "split-bill")
	path := filepath.Join(dir, "workflow.yaml")
	writeFile(t, path, strings.Replace(readFile(t, path), "      parts: $inputs.people\n    retry:", "      parts: 4\n    retry:", 1))
	h.approve(h.propose(w.ID, dir, splitBillFixtures(t)))
	_, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 2, Surface: "cli"})
	var refused *MigrationRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, ErrMigrationRefused) || !strings.Contains(err.Error(), "outputs.shares") || !strings.Contains(err.Error(), "at least 4") ||
		len(refused.Problems) == 0 {
		t.Fatalf("incompatible reads: %v", err)
	}
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 1, Surface: "cli"}); !errors.Is(err, ErrMigrationRefused) {
		t.Fatalf("to the run's own version: %v", err)
	}
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 3, Surface: "cli"}); !errors.Is(err, ErrMigrationRefused) || !strings.Contains(err.Error(), "version 2 is active") {
		t.Fatalf("to a version that is not active: %v", err)
	}
	if d := h.inspect(pred.ID); d.Run.State != eviedb.WorkflowRunWaitingForDependency || d.Successor != nil {
		t.Fatalf("a refused migration changed the run: %+v", d.Run)
	}
	if runs := h.runs(w.ID); len(runs) != 1 {
		t.Fatalf("a refused migration recorded a run: %+v", runs)
	}
	// A finished run is never migrated.
	done := h.start(w.ID, billInputs("lunch-1", 900, 3)).Run.Run
	if done.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("lunch = %+v", done)
	}
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: done.ID, ToVersion: 2, Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowRunFinished) {
		t.Fatalf("a finished run: %v", err)
	}
}

// postShareV2 is post-share whose effect retries after two minutes: a new
// version with the same reads.
func postShareV2(t *testing.T) string {
	return postShareVariant(t, "      backoff: 1m\n", "      backoff: 2m\n")
}

// Effects are never performed again: a successor resuming after an
// effect carries its output and references its receipt.
func TestWorkflowMigrationCarriesEffectEvidenceWithoutPerformingAgain(t *testing.T) {
	ctx := context.Background()
	var h *runHarness
	var ws memory.WorkspaceID
	ledger := newFakeLedger(func(l *fakeLedger, _ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		// The owner replaces version 1 while its effect is in flight: the
		// effect completes, and the run pauses before the next node.
		if _, err := h.store.DeactivateWorkflow(ctx, ws, "post-share", 1, "replaced", "cli"); err != nil {
			return nil, err
		}
		return l.apply(req), nil
	})
	h, _ = newEffectHarness(t, ledger, true, nil)
	ws = h.approvePostShare(postShareDir).ID
	res, err := h.startPost(h.rt, ws, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	pred := res.Run.Run
	if pred.State != eviedb.WorkflowRunWaitingForDependency || pred.NextStep != "record" {
		t.Fatalf("predecessor = %+v", pred)
	}
	h.approve(h.propose(ws, postShareV2(t), postShareFixtures(t)))
	succ := h.migrate(MigrationRequest{RunID: pred.ID, ToVersion: 2, Reason: "new ledger rules"}).Successor
	if succ.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, succ.Run.Outputs), postedOutputs) ||
		!reflect.DeepEqual(stepsOf(succ), []string{"record"}) || len(succ.Effects) != 0 {
		t.Fatalf("successor = %+v steps %v effects %+v", succ.Run, stepsOf(succ), succ.Effects)
	}
	if performs, _ := ledger.counts(); performs != 1 || ledger.balanceOf("account:household") != 250 {
		t.Fatalf("the effect was performed again: %d performs", performs)
	}
	inherited := succ.Predecessor.Effects
	if len(inherited) != 1 || inherited[0].RunID != pred.ID || inherited[0].Seq != 1 || inherited[0].Step != "post" ||
		inherited[0].Outcome != eviedb.WorkflowEffectSucceeded || !strings.HasPrefix(inherited[0].IdempotencyKey, "sha256:") {
		t.Fatalf("inherited effects = %+v", inherited)
	}
	if text := RunText(succ); !strings.Contains(text, "Inherited Effect Intent #1 of run "+pred.ID+" (steps.post): succeeded") {
		t.Fatalf("inspection:\n%s", text)
	}
}

// An effect whose outcome is unknown blocks migration; once the owner
// reconciled it as having happened, a successor resuming at that step
// takes its output from the receipt and calls nothing.
func TestWorkflowMigrationNeedsUnknownEffectsReconciledAndReusesTheirReceipts(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, nil)
	ws := h.approvePostShare(postShareDir).ID
	res, err := h.startPost(h.rt, ws, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	pred := res.Run.Run
	if pred.State != eviedb.WorkflowRunNeedsReconciliation {
		t.Fatalf("predecessor = %+v", pred)
	}
	if _, err := h.store.DeactivateWorkflow(ctx, ws, "post-share", 1, "replaced", "cli"); err != nil {
		t.Fatal(err)
	}
	h.approve(h.propose(ws, postShareV2(t), postShareFixtures(t)))
	_, err = h.rt.Migrate(ctx, MigrationRequest{RunID: pred.ID, ToVersion: 2, Surface: "cli"})
	if !errors.Is(err, ErrMigrationRefused) || !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("migrating with an unknown effect: %v", err)
	}
	if d := h.inspect(pred.ID); d.Run.State != eviedb.WorkflowRunNeedsReconciliation || d.Successor != nil {
		t.Fatalf("a refused migration changed the run: %+v", d.Run)
	}
	if _, err := h.rt.Reconcile(ctx, Reconciliation{RunID: pred.ID, Step: "post", Outcome: OutcomeHappened, Evidence: "entry-1 is on the ledger",
		Output: json.RawMessage(`{"entry_id":"entry-1","balance_cents":250}`), Detach: true, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	succ := h.migrate(MigrationRequest{RunID: pred.ID, ToVersion: 2}).Successor
	if succ.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, succ.Run.Outputs), postedOutputs) ||
		!reflect.DeepEqual(stepsOf(succ), []string{"post", "record"}) || len(succ.Effects) != 0 {
		t.Fatalf("successor = %+v steps %v effects %+v", succ.Run, stepsOf(succ), succ.Effects)
	}
	if !strings.Contains(succ.Attempts[0].Note, "run "+pred.ID) || !strings.Contains(succ.Attempts[0].Note, "not called again") {
		t.Fatalf("the successor's post attempt = %+v", succ.Attempts[0])
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("the reconciled effect was performed again: %d performs", performs)
	}
}

// What a predecessor used of its limits carries over: its time (less its
// waits at the daily cap), attempts and spend. A run paused at the cap
// overnight is not failed for max_duration once migrated.
func TestWorkflowMigrationKeepsCapWaitsOutOfMaxDuration(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	pause := func(bill, reason string) string {
		run := h.detach(w.ID, billInputs(bill, 1000, 3))
		h.clock.Advance(time.Minute)
		claim, err := h.store.AcquireWorkflowRunLease(ctx, run.ID, "holder-x", testLease)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.store.PauseWorkflowRun(ctx, claim.Lease, reason, 15*time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := h.store.ReleaseWorkflowRunLease(ctx, claim.Lease, ""); err != nil {
			t.Fatal(err)
		}
		return run.ID
	}
	capped := pause("dinner-1", eviedb.WorkflowDailyCapReason+" for 2026-10-04 (UTC): 5,000,000 of 5,000,000 tokens used")
	paused := pause("dinner-2", "steps.split waits for its provider")
	h.clock.Advance(10 * time.Hour)
	h.approve(h.propose(w.ID, splitBillV2(t), splitBillFixtures(t)))

	succ := h.migrate(MigrationRequest{RunID: capped, ToVersion: 2}).Successor
	// It ran one minute before the cap paused it for ten hours.
	if succ.Predecessor.Carried.Elapsed() != time.Minute || succ.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("carried %s, successor %+v", succ.Predecessor.Carried.Elapsed(), succ.Run)
	}
	// Any other pause counts toward max_duration, as it does for the run
	// itself (sub-stage 4): the successor fails at its first node.
	other := h.migrate(MigrationRequest{RunID: paused, ToVersion: 2}).Successor
	if other.Predecessor.Carried.Elapsed() < 10*time.Hour || other.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(other.Run.Error, "limits.max_duration") {
		t.Fatalf("carried %s, successor %+v", other.Predecessor.Carried.Elapsed(), other.Run)
	}
}

// A scheduled run's successor keeps the slot its predecessor decided: the
// schedule never decides it again, and inspection names it.
func TestWorkflowMigrationKeepsTheScheduleSlot(t *testing.T) {
	h := newLeaseHarness(t)
	w, _ := h.approveScheduled(scheduledSplitDir)
	h.setClock("2026-10-04T11:00:10Z")
	h.tick(h.rt)
	runs := h.runs(w.ID)
	if len(runs) != 1 || runs[0].Surface != "schedule" {
		t.Fatalf("runs = %+v", runs)
	}
	pred := runs[0]
	v2 := scheduledVariant(t, "  max_attempts: 10\n", "  max_attempts: 11\n")
	f := fixtures(t, readFile(t, filepath.Join(testdata, "scheduled-split.fixtures.json")))
	h.approve(h.propose(w.ID, v2, f))
	succ := h.migrate(MigrationRequest{RunID: pred.ID, ToVersion: 2}).Successor
	if succ.Run.State != eviedb.WorkflowRunSucceeded || succ.Run.Surface != "schedule" || succ.Slot == nil || succ.Slot.Key != "2026-10-04T07:00" ||
		succ.Predecessor.SlotKey != "2026-10-04T07:00" || succ.Predecessor.SlotTimezone != "America/New_York" {
		t.Fatalf("successor = %+v slot %+v", succ.Run, succ.Slot)
	}
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 2 {
		t.Fatalf("the schedule decided the migrated slot again: %+v", runs)
	}
	if slots := h.slots(w.ID, "scheduled-split"); len(slots) != 1 || slots[0].RunID != pred.ID {
		t.Fatalf("slots = %+v", slots)
	}
	if text := RunText(succ); !strings.Contains(text, "Started by its schedule for 2026-10-04T07:00 America/New_York") {
		t.Fatalf("inspection:\n%s", text)
	}
}
