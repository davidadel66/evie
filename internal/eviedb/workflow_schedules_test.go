package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow Schedules and the daily cap (Stage 14 sub-stage 11, ADR 0020,
// 0021 and 0023). A scheduled start records its slot with the run in one
// transaction, so a slot is decided once whatever process tries it; the
// owner pauses and resumes a schedule with immutable records; and every AI
// call or worker start is refused once today's spend across all workflow
// runs would pass the operator's daily cap.

// notesActivation approves the scheduled notes-digest example.
func notesActivation(t *testing.T, s *Store) WorkflowActivation {
	t.Helper()
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	return activate(t, s, p, 0)
}

func scheduledStart(a WorkflowActivation, day, holder string) WorkflowRunStart {
	return WorkflowRunStart{
		WorkspaceID: a.WorkspaceID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
		Composition: json.RawMessage(`{"runtime":"test"}`), RunKey: "notes-digest:" + day, Inputs: json.RawMessage(`{"day":"` + day + `"}`), Start: "read", Surface: "schedule",
		Slot: &WorkflowScheduleSlotStart{Timezone: "America/New_York", Key: day + "T07:00", Due: time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC), Holder: holder},
	}
}

func TestWorkflowScheduledStartDecidesItsSlotOnce(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	a := notesActivation(t, r.s)
	run, existing, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-05", "holder-a"))
	if err != nil || existing {
		t.Fatalf("scheduled start = %+v, %v, %v", run, existing, err)
	}
	if run.Surface != "schedule" || run.SessionID != "" {
		t.Fatalf("scheduled run = %+v", run)
	}
	// Another process deciding the same slot starts nothing, even after the
	// run failed and freed its Run Key.
	if _, _, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-05", "holder-b")); !errors.Is(err, ErrWorkflowScheduleSlotTaken) {
		t.Fatalf("second decision of the slot: %v", err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, claim.Lease, 0, "test failure"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-05", "holder-b")); !errors.Is(err, ErrWorkflowScheduleSlotTaken) {
		t.Fatalf("a failed run's slot was decided again: %v", err)
	}
	// A manual start of the same logical day holds the key, so the next
	// slot that names it records the run that already has it.
	manual := scheduledStart(a, "2026-10-06", "")
	manual.Surface, manual.Slot = "cli", nil
	held, _, err := r.s.StartWorkflowRun(ctx, manual)
	if err != nil {
		t.Fatal(err)
	}
	same, existing, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-06", "holder-a"))
	if err != nil || !existing || same.ID != held.ID {
		t.Fatalf("slot for a held key = %+v, %v, %v", same, existing, err)
	}
	r.reopen()
	slots, err := r.s.WorkflowScheduleSlots(ctx, a.WorkspaceID, a.Name, 10)
	if err != nil || len(slots) != 2 {
		t.Fatalf("slots = %+v, %v", slots, err)
	}
	if slots[0].Key != "2026-10-06T07:00" || slots[0].Outcome != WorkflowSlotExisting || slots[0].RunID != held.ID ||
		slots[1].Key != "2026-10-05T07:00" || slots[1].Outcome != WorkflowSlotStarted || slots[1].RunID != run.ID || slots[1].DecidedBy != "holder-a" {
		t.Fatalf("slots = %+v", slots)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || d.Slot == nil || d.Slot.Key != "2026-10-05T07:00" || d.Slot.Timezone != "America/New_York" {
		t.Fatalf("run's slot = %+v, %v", d.Slot, err)
	}
	wantImmutable(t, r.s, "workflow_schedule_slots")
	// The surfaces keep their shapes: a scheduled run names no session and
	// carries its slot; no other surface carries a slot.
	bad := scheduledStart(a, "2026-10-07", "holder-a")
	bad.SessionID = "s"
	if _, _, err := r.s.StartWorkflowRun(ctx, bad); !errors.Is(err, ErrWorkflowRunSurface) {
		t.Fatalf("scheduled start with a session: %v", err)
	}
	bad = scheduledStart(a, "2026-10-07", "holder-a")
	bad.Slot = nil
	if _, _, err := r.s.StartWorkflowRun(ctx, bad); !errors.Is(err, ErrWorkflowRunSurface) {
		t.Fatalf("scheduled start without its slot: %v", err)
	}
	bad = scheduledStart(a, "2026-10-07", "holder-a")
	bad.Surface = "web"
	if _, _, err := r.s.StartWorkflowRun(ctx, bad); !errors.Is(err, ErrWorkflowRunSurface) {
		t.Fatalf("web start with a slot: %v", err)
	}
	bad = scheduledStart(a, "2026-10-07", "holder-a")
	bad.Slot.Timezone = "Europe/London"
	if _, _, err := r.s.StartWorkflowRun(ctx, bad); err == nil || !strings.Contains(err.Error(), "schedule") {
		t.Fatalf("a slot of another timezone: %v", err)
	}
}

// Skipped and failed slots are recorded once, each with a notification.
func TestWorkflowScheduleSlotsRecordSkipsAndFailures(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	a := notesActivation(t, r.s)
	skip := WorkflowScheduleSlot{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Version: a.Version, Timezone: "America/New_York", Key: "2026-10-02T07:00",
		Due: time.Date(2026, 10, 2, 11, 0, 0, 0, time.UTC), Outcome: WorkflowSlotSkipped, Detail: "3 slots from 2026-09-30T07:00 missed their time; catch-up policy skip", DecidedBy: "holder-a"}
	if err := r.s.RecordWorkflowScheduleSlot(ctx, skip); err != nil {
		t.Fatal(err)
	}
	if err := r.s.RecordWorkflowScheduleSlot(ctx, skip); !errors.Is(err, ErrWorkflowScheduleSlotTaken) {
		t.Fatalf("recorded twice: %v", err)
	}
	failed := skip
	failed.Key, failed.Outcome, failed.Detail = "2026-10-03T07:00", WorkflowSlotFailed, "the approved workflow definition no longer matches its approval"
	if err := r.s.RecordWorkflowScheduleSlot(ctx, failed); err != nil {
		t.Fatal(err)
	}
	started := skip
	started.Key, started.Outcome = "2026-10-04T07:00", WorkflowSlotStarted
	if err := r.s.RecordWorkflowScheduleSlot(ctx, started); err == nil {
		t.Fatal("a started slot was recorded without its run")
	}
	notes, err := r.s.ListWorkflowNotifications(ctx, WorkflowNotificationFilter{WorkspaceID: a.WorkspaceID})
	if err != nil || len(notes) != 2 || notes[0].Kind != WorkflowNotifyScheduleFailed || notes[1].Kind != WorkflowNotifyScheduleSkipped || notes[1].RunID != "" ||
		!strings.Contains(notes[1].Detail, "catch-up policy skip") {
		t.Fatalf("notifications = %+v, %v", notes, err)
	}
}

// The owner pauses and resumes the active version's schedule with
// immutable records; neither needs a new approval, and a new approval's
// schedule starts unpaused.
func TestWorkflowSchedulePauseAndResumeAreImmutableOwnerRecords(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	a := notesActivation(t, r.s)
	schedules, err := r.s.WorkflowSchedules(ctx, "")
	if err != nil || len(schedules) != 1 {
		t.Fatalf("schedules = %+v, %v", schedules, err)
	}
	s := schedules[0]
	if s.Version != 1 || s.Paused || s.Status != WorkflowActive || s.Schedule.Cron != "0 7 * * 1-5" || !s.Cursor.At.Equal(a.ApprovedAt) || s.Cursor.Key != "" {
		t.Fatalf("schedule = %+v", s)
	}
	if _, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "resume", Surface: "cli"}); !errors.Is(err, ErrWorkflowScheduleControl) {
		t.Fatalf("resuming a running schedule: %v", err)
	}
	if _, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "pause", Surface: "chat"}); !errors.Is(err, ErrWorkflowApprover) {
		t.Fatalf("a chat paused a schedule: %v", err)
	}
	paused, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "pause", Reason: "on holiday", Surface: "cli"})
	if err != nil || paused.Action != "pause" || paused.Version != 1 || paused.Actor != "owner" {
		t.Fatalf("pause = %+v, %v", paused, err)
	}
	if _, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "pause", Surface: "cli"}); !errors.Is(err, ErrWorkflowScheduleControl) {
		t.Fatalf("pausing twice: %v", err)
	}
	if schedules, _ = r.s.WorkflowSchedules(ctx, a.WorkspaceID); !schedules[0].Paused || schedules[0].Control == nil || schedules[0].Control.Reason != "on holiday" {
		t.Fatalf("paused schedule = %+v", schedules[0])
	}
	clock.Advance(time.Hour)
	resumed, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "resume", Surface: "web", Version: 1})
	if err != nil || resumed.Action != "resume" || resumed.Surface != "web" {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	r.reopen()
	schedules, _ = r.s.WorkflowSchedules(ctx, a.WorkspaceID)
	// Slots that came due while it was paused are not caught up: the cursor
	// is the resume time.
	if schedules[0].Paused || !schedules[0].Cursor.At.Equal(resumed.At) {
		t.Fatalf("resumed schedule = %+v", schedules[0])
	}
	wantImmutable(t, r.s, "workflow_schedule_controls")
	if _, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Action: "pause", Surface: "cli", Version: 2}); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("pausing a version that is not current: %v", err)
	}
	// A manual workflow has no schedule to pause.
	_, manual := splitBillActivation(t, r.s)
	if _, err := r.s.ControlWorkflowSchedule(ctx, WorkflowScheduleControlRequest{WorkspaceID: manual.WorkspaceID, Workflow: manual.Name, Action: "pause", Surface: "cli"}); !errors.Is(err, ErrWorkflowScheduleNotFound) {
		t.Fatalf("pausing a manual workflow: %v", err)
	}
	// Deactivation leaves a schedule listed but inactive.
	if _, err := r.s.DeactivateWorkflow(ctx, a.WorkspaceID, a.Name, 1, "", "cli"); err != nil {
		t.Fatal(err)
	}
	if schedules, _ = r.s.WorkflowSchedules(ctx, a.WorkspaceID); len(schedules) != 1 || schedules[0].Status != WorkflowInactive {
		t.Fatalf("deactivated schedule = %+v", schedules)
	}
}

// Exactly one process runs schedules at a time; another takes over once
// the lease expires.
func TestWorkflowScheduleRunnerLeaseIsSingle(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	if ok, err := r.s.AcquireWorkflowScheduleRunner(ctx, "holder-a", time.Minute); !ok || err != nil {
		t.Fatalf("first runner: %v, %v", ok, err)
	}
	if ok, err := r.s.AcquireWorkflowScheduleRunner(ctx, "holder-b", time.Minute); ok || err != nil {
		t.Fatalf("second runner while the first holds: %v, %v", ok, err)
	}
	if ok, _ := r.s.AcquireWorkflowScheduleRunner(ctx, "holder-a", time.Minute); !ok {
		t.Fatal("the holder could not renew")
	}
	clock.Advance(2 * time.Minute)
	if ok, _ := r.s.AcquireWorkflowScheduleRunner(ctx, "holder-b", time.Minute); !ok {
		t.Fatal("no takeover after expiry")
	}
	if err := r.s.ReleaseWorkflowScheduleRunner(ctx, "holder-a"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.s.AcquireWorkflowScheduleRunner(ctx, "holder-a", time.Minute); ok {
		t.Fatal("a stale holder's release freed the new holder's lease")
	}
}

// The daily cap: every model call and worker start counts toward today's
// spend (UTC) across all runs, open ones at their bound; the transaction
// that records a call refuses it when it would pass the cap, recording
// nothing, and the next UTC day starts from zero.
func TestWorkflowDailyCapRefusesCallsPastIt(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, run := notesRun(t, r.s)
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease := claim.Lease
	advanceTo(t, r.s, lease, run.ID, []string{"read", "check"}, []string{"check", "summarize"})
	call := func(attempt int, cap *WorkflowDailyCap) WorkflowModelCallStart {
		return WorkflowModelCallStart{
			Attempt: WorkflowAttemptStart{RunID: run.ID, Checkpoint: 2, Step: "summarize", Kind: workflow.KindAI, Attempt: attempt, Input: json.RawMessage(`{"notes":[]}`)},
			Policy:  "summarizer", PolicyVersion: 2, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash", Parameters: json.RawMessage(`{"max_output_tokens":2000}`),
			PromptSHA256: "sha256:" + strings.Repeat("1", 64), SchemaSHA256: "sha256:" + strings.Repeat("2", 64), RequestSHA256: "sha256:" + strings.Repeat("3", 64),
			EstimatedInputTokens: 300, EstimatedOutputTokens: 2000, EstimatedMicroUSD: 17_000, Cap: cap,
		}
	}
	first, err := r.s.BeginWorkflowModelCall(ctx, lease, call(1, &WorkflowDailyCap{Tokens: 3000, MicroUSD: 1_000_000}))
	if err != nil {
		t.Fatal(err)
	}
	spend, err := r.s.WorkflowDailySpend(ctx, "2026-10-04")
	if err != nil || spend.Tokens != 2300 || spend.MicroUSD != 17_000 || spend.ModelCalls != 1 {
		t.Fatalf("today's spend with an open call = %+v, %v", spend, err)
	}
	if _, err := r.s.FinishWorkflowModelCall(ctx, lease, WorkflowModelCallEnd{RunID: run.ID, AttemptSeq: first.Seq, Requests: 1, InputTokens: 300, OutputTokens: 400,
		UsageReported: true, MicroUSD: 1900, PricingSource: "catalog", Response: "x", Error: "bad", Attempt: WorkflowAttemptEnd{Error: "invalid output"}}); err != nil {
		t.Fatal(err)
	}
	// 700 used; the next call could use 2,300 more: 3,000 fits exactly,
	// 2,999 does not.
	_, err = r.s.BeginWorkflowModelCall(ctx, lease, call(2, &WorkflowDailyCap{Tokens: 2999, MicroUSD: 1_000_000}))
	var capped *WorkflowDailyCapError
	if !errors.As(err, &capped) || !errors.Is(err, ErrWorkflowDailyCap) || capped.Day != "2026-10-04" || capped.Spent.Tokens != 700 || capped.NeedTokens != 2300 ||
		!capped.ResetAt.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) || !strings.HasPrefix(err.Error(), WorkflowDailyCapReason) {
		t.Fatalf("over the token cap: %+v, %v", capped, err)
	}
	if _, err := r.s.BeginWorkflowModelCall(ctx, lease, call(2, &WorkflowDailyCap{Tokens: 10_000, MicroUSD: 18_000})); !errors.As(err, &capped) || capped.Spent.MicroUSD != 1900 {
		t.Fatalf("over the dollar cap: %+v, %v", capped, err)
	}
	if p, _ := r.s.WorkflowRunProgress(ctx, run.ID); p.Made != 1 || p.InFlight != nil {
		t.Fatalf("a refused call recorded something: %+v", p)
	}
	if _, err := r.s.BeginWorkflowModelCall(ctx, lease, call(2, &WorkflowDailyCap{Tokens: 3000, MicroUSD: 1_000_000})); err != nil {
		t.Fatalf("a call that fits exactly: %v", err)
	}
	// The next UTC day starts from zero.
	clock.Advance(15 * time.Hour)
	if spend, _ := r.s.WorkflowDailySpend(ctx, "2026-10-05"); spend.Tokens != 0 || spend.ModelCalls != 0 {
		t.Fatalf("tomorrow's spend = %+v", spend)
	}
}

// A worker start counts its whole budget toward the cap; adopting a
// finished worker's result runs nothing and is not refused.
func TestWorkflowDailyCapCountsAWorkersWholeBudget(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, run := notesRun(t, r.s)
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease := claim.Lease
	advanceTo(t, r.s, lease, run.ID, []string{"read", "check"}, []string{"check", "summarize"})
	worker := func(cap *WorkflowDailyCap, adopted string) WorkflowWorkerCallStart {
		return WorkflowWorkerCallStart{
			Attempt: WorkflowAttemptStart{RunID: run.ID, Checkpoint: 2, Step: "summarize", Kind: workflow.KindAgent, Attempt: 1, Input: json.RawMessage(`{}`)},
			Worker:  "research", Policy: "summarizer", PolicyVersion: 2, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash",
			ObjectiveSHA256: "sha256:" + strings.Repeat("4", 64), Budget: time.Minute, BudgetTokens: 250_000, OutputPerMillion: 1.2, PricingSource: "catalog",
			EstimatedMicroUSD: 300_000, AdoptedFrom: adopted, Cap: cap,
		}
	}
	if _, err := r.s.BeginWorkflowWorkerCall(ctx, lease, worker(&WorkflowDailyCap{Tokens: 10_000_000, MicroUSD: 299_999}, "")); !errors.Is(err, ErrWorkflowDailyCap) {
		t.Fatalf("a worker past the dollar cap: %v", err)
	}
	if _, err := r.s.BeginWorkflowWorkerCall(ctx, lease, worker(&WorkflowDailyCap{Tokens: 249_999, MicroUSD: 1_000_000}, "")); !errors.Is(err, ErrWorkflowDailyCap) {
		t.Fatalf("a worker past the token cap: %v", err)
	}
	if _, err := r.s.BeginWorkflowWorkerCall(ctx, lease, worker(&WorkflowDailyCap{Tokens: 1, MicroUSD: 1}, "execution-1")); err != nil {
		t.Fatalf("adopting a result under the cap: %v", err)
	}
	if spend, _ := r.s.WorkflowDailySpend(ctx, "2026-10-04"); spend.Tokens != 0 || spend.WorkerCalls != 0 {
		t.Fatalf("an adopted result counted: %+v", spend)
	}
}

// A sub-stage 10 database (origin check with chat, no schedule or
// notification tables) upgrades once: its runs keep their rows and rowids,
// get no notification for anything that happened before, and a scheduled
// start works afterwards.
func TestWorkflowRunsTableUpgradesToScheduledOrigins(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	a := notesActivation(t, r.s)
	manual := scheduledStart(a, "2026-10-01", "")
	manual.Surface, manual.Slot = "cli", nil
	old, _, err := r.s.StartWorkflowRun(ctx, manual)
	if err != nil {
		t.Fatal(err)
	}
	var rowid int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, old.ID).Scan(&rowid); err != nil {
		t.Fatal(err)
	}
	// Put the database back in its sub-stage 10 shape.
	for _, statement := range []string{`DROP TRIGGER workflow_runs_notify`, `DROP TABLE workflow_notifications`, `DROP TABLE workflow_schedule_slots`,
		`DROP TABLE workflow_schedule_controls`, `DROP TABLE workflow_scheduler_state`} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := rebuildWorkflowRuns(ctx, r.db, workflowRunSurfaces, workflowRunSurfacesChat); err != nil {
		t.Fatal(err)
	}
	copyRun := `INSERT INTO workflow_runs SELECT ?,workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,
 composition,?,inputs,state,outputs,error,'schedule',?,created_at,started_at,finished_at,updated_at,not_before,source FROM workflow_runs WHERE id=?`
	if _, err := r.db.ExecContext(ctx, copyRun, "scheduled", "other-key", nil, old.ID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("the sub-stage 10 table accepted a scheduled run: %v", err)
	}
	// The old run fails under the old shape, before notifications exist.
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET state='failed',error='before the upgrade',finished_at=updated_at WHERE id=?`, old.ID); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	var kept int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, old.ID).Scan(&kept); err != nil || kept != rowid {
		t.Fatalf("rowid = %d, %v; want %d", kept, err, rowid)
	}
	if n := notificationsOf(t, r.s, WorkflowNotificationFilter{All: true}); len(n) != 0 {
		t.Fatalf("an earlier transition notified: %+v", n)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-05", "holder-a")); err != nil {
		t.Fatalf("scheduled start after the upgrade: %v", err)
	}
	// A scheduled run never names a session.
	if _, err := r.db.ExecContext(ctx, copyRun, "orphan", "orphan-key", "s", old.ID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("a scheduled run with a session was stored: %v", err)
	}
}
