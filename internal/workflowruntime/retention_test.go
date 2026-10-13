package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Retention (Stage 14 sub-stage 12; ADR 0045): each retention
// classification has a policy. Sensitive values are masked on every owner
// surface once a run ended (by default), and never reach a chat or a
// notice; the periodic sweep, under one runner lease across processes,
// redacts them from storage, compacts finished runs' checkpoints and
// deletes read notices, superseded slots and controls only after the
// policy's windows, each action recorded as a tombstone, and never touches
// evidence an unfinished run may need. By default nothing is redacted or
// deleted. Every test uses a real temporary SQLite file and a fake clock.

const secretBill = "alice-dinner-2026"

func days(n int) *time.Duration {
	d := time.Duration(n) * 24 * time.Hour
	return &d
}

// secretSplit is split-bill whose bill_id is a sensitive input that one
// output names, under the given classification and name.
func secretSplit(t *testing.T, name, classification string) string {
	t.Helper()
	dir := copyDefinition(t, "split-bill")
	path := filepath.Join(dir, "workflow.yaml")
	text := readFile(t, path)
	for _, r := range [][2]string{
		{"name: split-bill", "name: " + name},
		{"    max_length: 64\n", "    max_length: 64\n    sensitive: true\n"},
		{"  classification: ordinary", "  classification: " + classification},
		{"  leftover_cents: $steps.leftover.output.share", "  leftover_cents: $steps.leftover.output.share\n  bill: $inputs.bill_id"},
	} {
		if !strings.Contains(text, r[0]) {
			t.Fatalf("split-bill has no %q", r[0])
		}
		text = strings.Replace(text, r[0], r[1], 1)
	}
	writeFile(t, path, text)
	return dir
}

func (h *runHarness) approveDir(ws memory.WorkspaceID, dir string) {
	h.t.Helper()
	f := splitBillFixtures(h.t)
	h.approve(h.propose(ws, dir, f))
}

func (h *runHarness) startNamed(ws memory.WorkspaceID, name, bill string) eviedb.WorkflowRunDetail {
	h.t.Helper()
	res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: name, Inputs: billInputs(bill, 1000, 3), Surface: "cli"})
	if err != nil {
		h.t.Fatal(err)
	}
	return res.Run
}

func (h *runHarness) sweep(rt *Runtime, dryRun bool) SweepResult {
	h.t.Helper()
	res, err := rt.Sweep(context.Background(), SweepRequest{DryRun: dryRun, Surface: "cli"})
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

func encodedDetail(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWorkflowRetentionMasksSensitiveValuesOnOwnerSurfacesOnceARunEnds(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	policy := DefaultRetention()
	h.retention = &policy
	h.reopen()
	w := h.workspace()
	h.approveDir(w.ID, secretSplit(t, "split-bill", workflow.ClassificationOrdinary))
	h.approveDir(w.ID, secretSplit(t, "secret-split", workflow.ClassificationSensitive))

	done := h.startNamed(w.ID, "split-bill", secretBill)
	if done.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v", done.Run)
	}
	shown := h.inspect(done.Run.ID)
	if strings.Contains(encodedDetail(t, shown), secretBill) || strings.Contains(RunText(shown), secretBill) {
		t.Fatalf("the sensitive input reached inspection:\n%s", RunText(shown))
	}
	if !strings.Contains(string(shown.Run.Inputs), `"bill_id":"[sensitive]"`) || !strings.Contains(string(shown.Run.Inputs), `"total_cents":1000`) ||
		!strings.Contains(string(shown.Run.Outputs), `"bill":"[sensitive]"`) || !strings.Contains(string(shown.Run.Outputs), `"leftover_cents":1`) {
		t.Fatalf("masked run = %s %s", shown.Run.Inputs, shown.Run.Outputs)
	}
	runs := h.runs(w.ID)
	if strings.Contains(encodedDetail(t, runs), secretBill) {
		t.Fatalf("the sensitive input reached the run list: %s", encodedDetail(t, runs))
	}
	// Storage keeps it until a policy redacts it.
	if stored, _ := h.store.WorkflowRun(ctx, done.Run.ID); !strings.Contains(string(stored.Run.Inputs), secretBill) {
		t.Fatalf("masking changed storage: %s", stored.Run.Inputs)
	}
	// A chat never sees it, finished or not.
	view, err := h.rt.ChatRunStatus(ctx, memory.ScopeContext{WorkspaceID: w.ID}, done.Run.ID)
	if err != nil || strings.Contains(encodedDetail(t, view), secretBill) || !strings.Contains(view.OutputsJSON, `"leftover_cents":1`) {
		t.Fatalf("chat view = %+v, %v", view, err)
	}

	// Under the sensitive classification every value is masked once the
	// run ended.
	hidden := h.inspect(h.startNamed(w.ID, "secret-split", secretBill).Run.ID)
	if !strings.Contains(string(hidden.Run.Inputs), `"total_cents":"[sensitive]"`) || strings.Contains(string(hidden.Run.Outputs), "334") ||
		strings.Contains(string(hidden.Checkpoints[1].State), "334") || strings.Contains(string(hidden.Attempts[0].Output), "334") {
		t.Fatalf("sensitive run = %s %s %s", hidden.Run.Inputs, hidden.Run.Outputs, hidden.Checkpoints[1].State)
	}

	// A notice's detail never carries it: a run that fails quoting the
	// bill.
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		return nil, fmt.Errorf("the ledger locked %s", "carol-tea-22")
	}
	failedC := h.startNamed(w.ID, "split-bill", "carol-tea-22")
	if failedC.Run.State != eviedb.WorkflowRunFailed {
		t.Fatalf("failing run = %+v", failedC.Run)
	}
	notes, err := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: failedC.Run.ID, All: true})
	if err != nil || len(notes) != 1 || strings.Contains(encodedDetail(t, notes), "carol-tea-22") || !strings.Contains(notes[0].Detail, "the ledger locked [sensitive]") {
		t.Fatalf("notices = %+v, %v", notes, err)
	}
	h.rt.call = nil

	// Before a run ends, the owner sees its input as given.
	h.pauseAfterSplit(w.ID)
	paused := h.startNamed(w.ID, "split-bill", "dave-brunch-3")
	if paused.Run.State.Terminal() || !strings.Contains(string(h.inspect(paused.Run.ID).Run.Inputs), "dave-brunch-3") {
		t.Fatalf("unfinished run = %+v", paused.Run)
	}

	// A policy can turn masking off for a classification.
	off := DefaultRetention()
	off.Ordinary.Mask = false
	h.retention = &off
	h.reopen()
	if shown := h.inspect(done.Run.ID); !strings.Contains(string(shown.Run.Inputs), secretBill) {
		t.Fatalf("with masking off = %s", shown.Run.Inputs)
	}
}

func TestWorkflowRetentionSweepRedactsAfterEachClassificationsWindow(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	policy := DefaultRetention()
	// By default nothing is redacted or deleted.
	if policy.Ordinary.Redact != nil || policy.Sensitive.Redact != nil || policy.Ordinary.Notices != nil || policy.Sensitive.Notices != nil {
		t.Fatalf("default policy = %+v", policy)
	}
	h.retention = &policy
	h.reopen()
	w := h.workspace()
	h.approveDir(w.ID, secretSplit(t, "split-bill", workflow.ClassificationOrdinary))
	h.approveDir(w.ID, secretSplit(t, "secret-split", workflow.ClassificationSensitive))
	plain := h.approve(h.propose(w.ID, func() string {
		dir := copyDefinition(t, "split-bill")
		path := filepath.Join(dir, "workflow.yaml")
		writeFile(t, path, strings.Replace(readFile(t, path), "name: split-bill", "name: plain-split", 1))
		return dir
	}(), splitBillFixtures(t)))
	_ = plain
	ordinary := h.startNamed(w.ID, "split-bill", secretBill).Run.ID
	sensitive := h.startNamed(w.ID, "secret-split", secretBill).Run.ID
	other := h.startNamed(w.ID, "plain-split", "plain-1").Run.ID
	h.clock.Advance(60 * 24 * time.Hour)
	if res := h.sweep(h.rt, false); len(redactions(res)) != 0 {
		t.Fatalf("the default policy redacted: %+v", res.Tombstones)
	}

	policy.Ordinary.Redact, policy.Sensitive.Redact = days(90), days(30)
	h.retention = &policy
	h.reopen()
	res := h.sweep(h.rt, false)
	got := redactions(res)
	if len(got) != 1 || got[0].RunID != sensitive || got[0].DataClass != eviedb.WorkflowDataRunValues || got[0].Classification != workflow.ClassificationSensitive ||
		got[0].Actor != "owner" || got[0].Surface != "cli" || got[0].Records == 0 || !strings.Contains(got[0].Policy, "30 days") {
		t.Fatalf("after 60 days: %+v", res.Tombstones)
	}
	stored, err := h.store.WorkflowRun(ctx, sensitive)
	if err != nil || strings.Contains(encodedDetail(t, stored), secretBill) || strings.Contains(string(stored.Run.Outputs), "334") ||
		string(stored.Run.Inputs) != `{"bill_id":"[redacted]","people":"[redacted]","total_cents":"[redacted]"}` || stored.Run.RunKey == "" {
		t.Fatalf("redacted storage = %s %s, %v", stored.Run.Inputs, stored.Run.Outputs, err)
	}
	if kept, _ := h.store.WorkflowRun(ctx, ordinary); !strings.Contains(string(kept.Run.Inputs), secretBill) {
		t.Fatalf("the ordinary run was redacted inside its window: %s", kept.Run.Inputs)
	}
	h.clock.Advance(31 * 24 * time.Hour)
	got = redactions(h.sweep(h.rt, false))
	if len(got) != 1 || got[0].RunID != ordinary || got[0].DataClass != eviedb.WorkflowDataSensitiveInputs {
		t.Fatalf("after 91 days: %+v", got)
	}
	redacted, _ := h.store.WorkflowRun(ctx, ordinary)
	if strings.Contains(encodedDetail(t, redacted), secretBill) || !strings.Contains(string(redacted.Run.Inputs), `"bill_id":"[redacted]"`) ||
		!strings.Contains(string(redacted.Run.Inputs), `"total_cents":1000`) || !strings.Contains(string(redacted.Run.Outputs), `"leftover_cents":1`) {
		t.Fatalf("ordinary redaction = %s %s", redacted.Run.Inputs, redacted.Run.Outputs)
	}
	// Inspection shows what was redacted, when and why; a run without
	// sensitive values is never touched.
	if text := RunText(h.inspect(ordinary)); !strings.Contains(text, "Redacted sensitive_inputs") {
		t.Fatalf("inspection:\n%s", text)
	}
	if untouched, _ := h.store.WorkflowRun(ctx, other); !strings.Contains(string(untouched.Run.Inputs), "plain-1") {
		t.Fatalf("a run without sensitive values = %s", untouched.Run.Inputs)
	} else {
		for _, tomb := range untouched.Retention {
			if tomb.Action == eviedb.WorkflowRetentionRedact {
				t.Fatalf("a run without sensitive values was redacted: %+v", tomb)
			}
		}
	}
	// A rerun that would take the redacted input is refused.
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", RerunOf: ordinary, Reason: "again", Surface: "cli"}); !errors.Is(err, ErrRerunRefused) ||
		!strings.Contains(err.Error(), "redacted") {
		t.Fatalf("rerun of a redacted run: %v", err)
	}
	// Nothing is redacted twice.
	if again := redactions(h.sweep(h.rt, false)); len(again) != 0 {
		t.Fatalf("a second sweep redacted again: %+v", again)
	}
}

func redactions(res SweepResult) []eviedb.WorkflowRetentionTombstone {
	var out []eviedb.WorkflowRetentionTombstone
	for _, t := range res.Tombstones {
		if t.Action == eviedb.WorkflowRetentionRedact {
			out = append(out, t)
		}
	}
	return out
}

func tombstonesOf(res SweepResult, action, class string) []eviedb.WorkflowRetentionTombstone {
	var out []eviedb.WorkflowRetentionTombstone
	for _, t := range res.Tombstones {
		if t.Action == action && t.DataClass == class {
			out = append(out, t)
		}
	}
	return out
}

func TestWorkflowRetentionCompactsCheckpointsWithoutLosingThem(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	policy := DefaultRetention()
	policy.Ordinary.Mask = false
	h.retention = &policy
	h.reopen()
	w, _ := h.approveSplitBill(splitBillDir)
	id := h.start(w.ID, billInputs("dinner-1", 1000, 3)).Run.Run.ID
	before := h.inspect(id)
	if len(before.Checkpoints) != 4 {
		t.Fatalf("checkpoints = %+v", before.Checkpoints)
	}
	// Inside the default seven days nothing is compacted.
	h.clock.Advance(24 * time.Hour)
	if res := h.sweep(h.rt, false); len(tombstonesOf(res, eviedb.WorkflowRetentionCompact, eviedb.WorkflowDataCheckpointStates)) != 0 {
		t.Fatalf("compacted inside the window: %+v", res.Tombstones)
	}
	h.clock.Advance(7 * 24 * time.Hour)
	res := h.sweep(h.rt, false)
	compacted := tombstonesOf(res, eviedb.WorkflowRetentionCompact, eviedb.WorkflowDataCheckpointStates)
	if len(compacted) != 1 || compacted[0].RunID != id || compacted[0].Records != 2 || compacted[0].Bytes == 0 {
		t.Fatalf("compaction = %+v", res.Tombstones)
	}
	var stored []string
	rows, err := h.db.QueryContext(ctx, `SELECT state FROM workflow_run_checkpoints WHERE run_id=? ORDER BY seq`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, s)
	}
	rows.Close()
	if len(stored) != 4 || stored[1] != "{}" || stored[2] != "{}" || stored[3] != string(before.Checkpoints[3].State) || stored[0] != string(before.Checkpoints[0].State) {
		t.Fatalf("stored states = %v", stored)
	}
	after := h.inspect(id)
	for i := range before.Checkpoints {
		if string(after.Checkpoints[i].State) != string(before.Checkpoints[i].State) || after.Checkpoints[i].Compacted != (i == 1 || i == 2) {
			t.Fatalf("checkpoint %d: %s (compacted %v), want %s", i, after.Checkpoints[i].State, after.Checkpoints[i].Compacted, before.Checkpoints[i].State)
		}
	}
	if !reflect.DeepEqual(after.Attempts, before.Attempts) || string(after.Run.Outputs) != string(before.Run.Outputs) {
		t.Fatal("compaction changed more than checkpoint states")
	}
	if again := h.sweep(h.rt, false); len(again.Tombstones) != 0 {
		t.Fatalf("a second sweep acted again: %+v", again.Tombstones)
	}
}

func TestWorkflowRetentionDeletesReadNoticesAndSupersededScheduleRecordsWithTombstones(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	policy := DefaultRetention()
	policy.Ordinary.Notices, policy.Ordinary.Compact = days(7), nil
	h.retention = &policy
	h.reopen()
	w, a := h.approveScheduled(scheduledSplitDir)
	for _, day := range []string{"2026-09-01", "2026-09-02", "2026-09-03"} {
		due, _ := time.Parse("2006-01-02T15:04", day+"T07:00")
		if err := h.store.RecordWorkflowScheduleSlot(ctx, eviedb.WorkflowScheduleSlot{WorkspaceID: w.ID, Workflow: "scheduled-split", Version: a.Version,
			Timezone: "America/New_York", Key: day + "T07:00", Due: due, Outcome: eviedb.WorkflowSlotSkipped, Detail: "the Evie was off", DecidedBy: "holder-a"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"pause", "resume", "pause"} {
		if _, err := h.rt.ControlSchedule(ctx, ScheduleControl{WorkspaceID: w.ID, Workflow: "scheduled-split", Action: action, Surface: "cli"}); err != nil {
			t.Fatal(err)
		}
	}
	read := h.start2(w.ID, "2026-10-01")
	unread := h.start2(w.ID, "2026-10-02")
	notes, err := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: read, All: true})
	if err != nil || len(notes) != 1 {
		t.Fatalf("notices = %+v, %v", notes, err)
	}
	if _, err := h.rt.MarkNotifications(ctx, NotificationMark{IDs: []int64{notes[0].ID}, Read: true, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	// Schedule notices read too.
	all, _ := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{WorkspaceID: w.ID, All: true})
	for _, n := range all {
		if n.RunID == "" {
			if _, err := h.rt.MarkNotifications(ctx, NotificationMark{IDs: []int64{n.ID}, Read: true, Surface: "cli"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	h.clock.Advance(6 * 24 * time.Hour)
	if res := h.sweep(h.rt, false); len(res.Tombstones) != 0 {
		t.Fatalf("deleted inside the window: %+v", res.Tombstones)
	}
	h.clock.Advance(2 * 24 * time.Hour)
	dry := h.sweep(h.rt, true)
	res := h.sweep(h.rt, false)
	notices := tombstonesOf(res, eviedb.WorkflowRetentionDelete, eviedb.WorkflowDataNotifications)
	slots := tombstonesOf(res, eviedb.WorkflowRetentionDelete, eviedb.WorkflowDataScheduleSlots)
	controls := tombstonesOf(res, eviedb.WorkflowRetentionDelete, eviedb.WorkflowDataScheduleControls)
	// The read run notice and the three schedule notices; the unread one
	// stays.
	if len(notices) != 1 || notices[0].Records != 4 || len(slots) != 1 || slots[0].Records != 2 || len(controls) != 1 || controls[0].Records != 2 ||
		notices[0].OldestAt == nil || notices[0].Reason == "" || !strings.Contains(notices[0].Policy, "7 days") {
		t.Fatalf("deletions = %+v skipped %+v problems %v", res.Tombstones, res.Skipped, res.Problems)
	}
	if len(dry.Tombstones) != len(res.Tombstones) || !dry.DryRun || !dry.Tombstones[0].DryRun || dry.Tombstones[0].ID != 0 {
		t.Fatalf("dry run = %+v, real %+v", dry.Tombstones, res.Tombstones)
	}
	left, _ := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{WorkspaceID: w.ID, All: true})
	if len(left) != 1 || left[0].RunID != unread {
		t.Fatalf("notices left = %+v", left)
	}
	if got := h.slots(w.ID, "scheduled-split"); len(got) != 1 || got[0].Key != "2026-09-03T07:00" {
		t.Fatalf("slots left = %+v", got)
	}
	if s := h.schedules(h.rt, w.ID); len(s) != 1 || !s[0].Paused {
		t.Fatalf("the schedule after deletion = %+v", s)
	}
	status, err := h.rt.RetentionStatus(ctx)
	if err != nil || status.State.LastSweepAt == nil || len(status.State.Totals) != 3 {
		t.Fatalf("status = %+v, %v", status, err)
	}
}

// start2 runs scheduled-split for a day by hand and returns the run.
func (h *runHarness) start2(ws memory.WorkspaceID, day string) string {
	h.t.Helper()
	res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "scheduled-split", Surface: "cli",
		TextInputs: map[string]string{"day": day, "at": "07:00", "total_cents": "1000", "people": "3"}})
	if err != nil {
		h.t.Fatal(err)
	}
	return res.Run.Run.ID
}

// Evidence an unfinished run may need is never touched: a superseded
// predecessor whose successor has not finished, and the notices of a run
// that is not finished.
func TestWorkflowRetentionNeverTouchesEvidenceOfUnfinishedWork(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	zero := time.Duration(0)
	policy := DefaultRetention()
	policy.Ordinary.Redact, policy.Ordinary.Compact, policy.Ordinary.Notices = &zero, &zero, &zero
	h.retention = &policy
	h.reopen()
	w := h.workspace()
	h.approveDir(w.ID, secretSplit(t, "split-bill", workflow.ClassificationOrdinary))
	h.pauseAfterSplit(w.ID)
	pred := h.startNamed(w.ID, "split-bill", secretBill).Run
	h.rt.call = nil
	h.approve(h.propose(w.ID, func() string {
		dir := secretSplit(t, "split-bill", workflow.ClassificationOrdinary)
		path := filepath.Join(dir, "workflow.yaml")
		writeFile(t, path, strings.Replace(readFile(t, path), "  - id: leftover\n    kind: deterministic\n", "  - id: leftover\n    kind: deterministic\n    retry:\n      max_attempts: 2\n", 1))
		return dir
	}(), splitBillFixtures(t)))
	succ := h.migrate(MigrationRequest{RunID: pred.ID, ToVersion: 2, Detach: true})
	if succ.Successor.Run.State.Terminal() {
		t.Fatalf("successor = %+v", succ.Successor.Run)
	}
	if _, err := h.rt.MarkNotifications(ctx, NotificationMark{All: true, Read: true, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Hour)
	res := h.sweep(h.rt, false)
	for _, tomb := range res.Tombstones {
		if tomb.RunID == pred.ID {
			t.Fatalf("the predecessor of an unfinished successor was touched: %+v", tomb)
		}
	}
	if len(res.Skipped) == 0 || !strings.Contains(fmt.Sprint(res.Skipped), "successor run "+succ.Successor.Run.ID) {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
	if stored, _ := h.store.WorkflowRun(ctx, pred.ID); !strings.Contains(string(stored.Run.Inputs), secretBill) {
		t.Fatalf("predecessor = %s", stored.Run.Inputs)
	}
	// Once the successor finished, the whole chain may go.
	if _, err := h.rt.await(ctx, succ.Successor.Run.ID); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Hour)
	res = h.sweep(h.rt, false)
	redacted := map[string]bool{}
	for _, tomb := range redactions(res) {
		redacted[tomb.RunID] = true
	}
	if !redacted[pred.ID] || !redacted[succ.Successor.Run.ID] {
		t.Fatalf("after the chain finished: %+v", res.Tombstones)
	}
}

// One sweep runs at a time across processes: another Evie's sweep is
// refused while one holds the runner lease, and a dry run needs none.
func TestWorkflowRetentionSweepHasOneRunnerAcrossProcesses(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	zero := time.Duration(0)
	policy := DefaultRetention()
	policy.Ordinary.Redact = &zero
	h.retention = &policy
	h.reopen()
	w := h.workspace()
	h.approveDir(w.ID, secretSplit(t, "split-bill", workflow.ClassificationOrdinary))
	id := h.startNamed(w.ID, "split-bill", secretBill).Run.ID
	first, second := h.process(), h.process()
	// The first Evie is sweeping: it holds the runner lease.
	if ok, err := h.store.AcquireWorkflowRetentionRunner(ctx, first.rt.Holder(), 10*time.Minute); err != nil || !ok {
		t.Fatalf("runner lease = %v, %v", ok, err)
	}
	if _, err := second.rt.Sweep(ctx, SweepRequest{Surface: "cli"}); !errors.Is(err, ErrRetentionBusy) {
		t.Fatalf("a second runner: %v", err)
	}
	if err := second.rt.tickRetention(ctx); err != nil {
		t.Fatalf("a periodic sweep beside the runner: %v", err)
	}
	if tombs, _ := h.store.WorkflowRetentionTombstones(ctx, eviedb.WorkflowRetentionTombstoneFilter{RunID: id}); len(tombs) != 0 {
		t.Fatalf("the second Evie swept beside the runner: %+v", tombs)
	}
	if _, err := second.rt.Sweep(ctx, SweepRequest{Surface: "cli", DryRun: true}); err != nil {
		t.Fatalf("a dry run beside the runner: %v", err)
	}
	if err := first.rt.tickRetention(ctx); err != nil {
		t.Fatal(err)
	}
	tombs, err := h.store.WorkflowRetentionTombstones(ctx, eviedb.WorkflowRetentionTombstoneFilter{RunID: id})
	if err != nil || len(tombs) != 1 || tombs[0].Actor != "runtime" || tombs[0].Surface != "" {
		t.Fatalf("tombstones = %+v, %v", tombs, err)
	}
	// The periodic sweep gave the lease up: the owner's command runs.
	if _, err := second.rt.Sweep(ctx, SweepRequest{Surface: "cli"}); err != nil {
		t.Fatalf("after the first runner finished: %v", err)
	}
	if tombs, _ := h.store.WorkflowRetentionTombstones(ctx, eviedb.WorkflowRetentionTombstoneFilter{RunID: id}); len(tombs) != 1 {
		t.Fatalf("a run was redacted twice: %+v", tombs)
	}
}

func TestWorkflowRetentionPolicyParsing(t *testing.T) {
	base := DefaultRetention().Ordinary
	p, err := ParseRetentionPolicy("redact=30d, notices=12h,compact=never,mask=off", base)
	if err != nil || p.Mask || p.Redact == nil || *p.Redact != 30*24*time.Hour || p.Notices == nil || *p.Notices != 12*time.Hour || p.Compact != nil {
		t.Fatalf("policy = %+v, %v", p, err)
	}
	if text := p.String(); text != "mask off, redact 30 days after a run ended, delete notices 12h0m0s after they were recorded, compact never" {
		t.Fatalf("text = %q", text)
	}
	for _, bad := range []string{"redact=soon", "keep=forever", "mask=maybe", "redact=-1d", "redact"} {
		if _, err := ParseRetentionPolicy(bad, base); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	if same, err := ParseRetentionPolicy("", base); err != nil || !reflect.DeepEqual(same, base) {
		t.Fatalf("an empty policy = %+v, %v", same, err)
	}
}

// A run whose effect's outcome is unknown keeps every record and notice,
// whatever the policy: the owner needs them to reconcile it.
func TestWorkflowRetentionKeepsEvidenceOfAnUnknownEffect(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, nil)
	zero := time.Duration(0)
	policy := DefaultRetention()
	policy.Ordinary.Redact, policy.Ordinary.Compact, policy.Ordinary.Notices = &zero, &zero, &zero
	h.retention = &policy
	h.reopen()
	h.advanceOnSleep(h.rt)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	if res.Run.Run.State != eviedb.WorkflowRunNeedsReconciliation {
		t.Fatalf("run = %+v", res.Run.Run)
	}
	if _, err := h.rt.MarkNotifications(ctx, NotificationMark{All: true, Read: true, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	before, err := h.store.WorkflowRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(24 * time.Hour)
	swept := h.sweep(h.rt, false)
	for _, tomb := range swept.Tombstones {
		if tomb.RunID == id || tomb.DataClass == eviedb.WorkflowDataNotifications {
			t.Fatalf("the unreconciled run's evidence was touched: %+v", tomb)
		}
	}
	after, err := h.store.WorkflowRun(ctx, id)
	if err != nil || !reflect.DeepEqual(after.Effects, before.Effects) || !reflect.DeepEqual(after.Checkpoints, before.Checkpoints) || len(after.Retention) != 0 {
		t.Fatalf("after the sweep = %+v, %v", after.Effects, err)
	}
	if notes, _ := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: id, All: true}); len(notes) == 0 {
		t.Fatal("the needs-reconciliation notice was deleted")
	}
}

// A tab sees runs as a chat does: a sensitive input's value never reaches
// it, finished or not and whatever the masking policy, and it gets no inputs.
func TestTabRunsNeverCarrySensitiveInputs(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	off := DefaultRetention()
	off.Ordinary.Mask = false
	h.retention = &off
	h.reopen()
	w := h.workspace()
	h.approveDir(w.ID, secretSplit(t, "split-bill", workflow.ClassificationOrdinary))
	done := h.startNamed(w.ID, "split-bill", secretBill)
	if !strings.Contains(string(h.inspect(done.Run.ID).Run.Inputs), secretBill) {
		t.Fatal("with masking off the owner should see the input")
	}
	h.pauseAfterSplit(w.ID)
	paused := h.startNamed(w.ID, "split-bill", "dave-brunch-3")
	runs, err := h.rt.TabRuns(ctx, w.ID, 100)
	if err != nil || len(runs) != 2 {
		t.Fatalf("tab runs = %d, %v", len(runs), err)
	}
	encoded := encodedDetail(t, runs)
	if strings.Contains(encoded, secretBill) || strings.Contains(encoded, "dave-brunch-3") {
		t.Fatalf("a sensitive input reached a tab: %s", encoded)
	}
	for _, run := range runs {
		if len(run.Inputs) != 0 {
			t.Fatalf("a tab got inputs: %s", run.Inputs)
		}
		if run.ID == done.Run.ID && !strings.Contains(string(run.Outputs), `"leftover_cents":1`) {
			t.Fatalf("the finished run's outputs = %s", run.Outputs)
		}
	}
	_ = paused
}
