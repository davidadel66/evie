package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow Schedules (Stage 14 sub-stage 11; ADR 0020, 0021, 0023). The
// schedule an approval activated starts runs through StartRun with surface
// "schedule": its slot's nominal local date and time feed the run's
// inputs and so its Run Key, each slot is decided once (with the run, in
// one transaction), the declared Catch-up Policy bounds what runs after
// downtime, and the owner's pause, a deactivation or a block stops it.
// Every test uses a real temporary SQLite file and a fake clock that the
// ticks read; the harness starts at 2026-10-04 09:00 UTC (05:00 in New
// York, a Sunday).

var scheduledSplitDir = filepath.Join(testdata, "scheduled-split")

func (h *runHarness) approveScheduled(dir string) (memory.Workspace, eviedb.WorkflowActivation) {
	h.t.Helper()
	w := h.workspace()
	f := fixtures(h.t, readFile(h.t, filepath.Join(testdata, "scheduled-split.fixtures.json")))
	return w, h.approve(h.propose(w.ID, dir, f))
}

// scheduledVariant is scheduled-split with its schedule block's lines
// replaced.
func scheduledVariant(t *testing.T, replacements ...string) string {
	t.Helper()
	dir := copyDefinition(t, "scheduled-split")
	path := filepath.Join(dir, "workflow.yaml")
	text := readFile(t, path)
	for i := 0; i+1 < len(replacements); i += 2 {
		if !strings.Contains(text, replacements[i]) {
			t.Fatalf("workflow.yaml has no %q", replacements[i])
		}
		text = strings.Replace(text, replacements[i], replacements[i+1], 1)
	}
	writeFile(t, path, text)
	return dir
}

func (h *runHarness) setClock(at string) {
	h.t.Helper()
	target, err := time.Parse(time.RFC3339, at)
	if err != nil {
		h.t.Fatal(err)
	}
	h.clock.Advance(target.Sub(h.clock.Now()))
}

func (h *runHarness) tick(rt *Runtime) {
	h.t.Helper()
	if err := rt.tickSchedules(context.Background()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *runHarness) schedules(rt *Runtime, ws memory.WorkspaceID) []ScheduleStatus {
	h.t.Helper()
	out, err := rt.Schedules(context.Background(), ws)
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

func (h *runHarness) slots(ws memory.WorkspaceID, name string) []eviedb.WorkflowScheduleSlot {
	h.t.Helper()
	out, err := h.store.WorkflowScheduleSlots(context.Background(), ws, name, 50)
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

// runInputs is a run's accepted input as name=value.
func runInputs(run eviedb.WorkflowRun) string { return valuesLine(run.Inputs) }

func TestWorkflowScheduleStartsADueSlotOnceWithItsRunKey(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, a := h.approveScheduled(scheduledSplitDir)
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("a run started before its slot: %+v", runs)
	}
	status := h.schedules(h.rt, w.ID)
	if len(status) != 1 || status[0].State != "active" || status[0].Next == nil || status[0].Next.Key != "2026-10-04T07:00" ||
		!status[0].Next.Due.Equal(time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)) || status[0].Version != a.Version {
		t.Fatalf("schedule = %+v", status)
	}
	h.setClock("2026-10-04T11:00:10Z")
	h.tick(h.rt)
	h.tick(h.rt)
	runs := h.runs(w.ID)
	if len(runs) != 1 || runs[0].Surface != "schedule" || runs[0].SessionID != "" || runs[0].State != eviedb.WorkflowRunQueued {
		t.Fatalf("runs = %+v", runs)
	}
	run := runs[0]
	if got := runInputs(run); got != `at="07:00", day="2026-10-04", people=3, total_cents=1000` {
		t.Fatalf("inputs = %s", got)
	}
	// The Run Key comes from $schedule: a manual start of the same logical
	// slot is the scheduled run, not a second one.
	manual, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "scheduled-split", Surface: "cli", Detach: true,
		TextInputs: map[string]string{"day": "2026-10-04", "at": "07:00", "total_cents": "1000", "people": "3"}})
	if err != nil || !manual.Existing || manual.Run.Run.ID != run.ID {
		t.Fatalf("manual start of the same slot = %+v, %v", manual.Run.Run, err)
	}
	d, err := h.rt.await(ctx, run.ID)
	if err != nil || d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("scheduled run = %+v, %v", d.Run, err)
	}
	if d.Slot == nil || d.Slot.Key != "2026-10-04T07:00" || d.Slot.Outcome != eviedb.WorkflowSlotStarted || d.Slot.DecidedBy != h.rt.Holder() {
		t.Fatalf("slot = %+v", d.Slot)
	}
	if text := RunText(d); !strings.Contains(text, "Started by its schedule for 2026-10-04T07:00 America/New_York") {
		t.Fatalf("run text:\n%s", text)
	}
	if !strings.Contains(string(d.Run.Outputs), `"day":"2026-10-04"`) {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	// The run finished; its notice is durable.
	notes, err := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{RunID: run.ID})
	if err != nil || len(notes) != 1 || notes[0].Kind != eviedb.WorkflowNotifySucceeded || notes[0].Surface != "schedule" ||
		!strings.Contains(notes[0].Text, "scheduled-split run "+run.ID+" succeeded") {
		t.Fatalf("notifications = %+v, %v", notes, err)
	}
	// Another process: the slot is decided; deciding it again starts
	// nothing.
	other := h.process()
	h.setClock("2026-10-04T12:00:00Z")
	h.tick(other.rt)
	if again, err := other.rt.startSlot(ctx, h.schedules(other.rt, w.ID)[0].WorkflowSchedule, slotOf(t, "0 7 * * *", "2026-10-04T07:00")); err != nil || again {
		t.Fatalf("deciding a decided slot = %v, %v", again, err)
	}
	if runs := h.runs(w.ID); len(runs) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
}

// slotOf is a New York slot of the timetable by key.
func slotOf(t *testing.T, cron, key string) workflow.Slot {
	t.Helper()
	tt, err := workflow.NewTimetable(cron, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.ParseInLocation("2006-01-02T15:04", key, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := tt.Next(workflow.SlotCursor{At: at.Add(-24 * time.Hour)})
	for ok && s.Key != key {
		s, ok = tt.Next(workflow.SlotCursor{At: s.Due, Key: s.Key})
	}
	if !ok {
		t.Fatalf("no slot %s", key)
	}
	return s
}

// Two processes deciding the same slots at the same time start each once:
// the runner lease lets one tick at a time, and the slot record refuses a
// second decision even when both try one slot together.
func TestWorkflowScheduleNeverStartsASlotTwiceAcrossProcesses(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveScheduled(scheduledSplitDir)
	a, b := h.process(), h.process()
	h.setClock("2026-10-04T11:00:05Z")
	var wg sync.WaitGroup
	for _, p := range []*process{a, b, a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.rt.tickSchedules(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if runs := h.runs(w.ID); len(runs) != 1 {
		t.Fatalf("runs after concurrent ticks = %+v", runs)
	}
	// Both decide the next day's slot directly, at once.
	h.setClock("2026-10-05T11:00:05Z")
	sched := h.schedules(a.rt, w.ID)[0].WorkflowSchedule
	slot := slotOf(t, "0 7 * * *", "2026-10-05T07:00")
	started := make(chan bool, 2)
	for _, p := range []*process{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := p.rt.startSlot(ctx, sched, slot)
			if err != nil {
				t.Error(err)
			}
			started <- ok
		}()
	}
	wg.Wait()
	close(started)
	n := 0
	for ok := range started {
		if ok {
			n++
		}
	}
	if runs := h.runs(w.ID); n != 1 || len(runs) != 2 {
		t.Fatalf("started %d; runs = %+v", n, runs)
	}
	if slots := h.slots(w.ID, "scheduled-split"); len(slots) != 2 || slots[0].Key != "2026-10-05T07:00" {
		t.Fatalf("slots = %+v", slots)
	}
}

// each_missed with max_runs 3: after five days of downtime the three latest
// slots run, one per business date, and the older ones are recorded
// skipped with a notification; nothing runs twice on the next tick.
func TestWorkflowScheduleCatchUpEachMissedIsBounded(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveScheduled(scheduledSplitDir)
	h.setClock("2026-10-09T12:30:00Z")
	h.tick(h.rt)
	h.tick(h.rt)
	runs := h.runs(w.ID)
	var days []string
	for i := len(runs) - 1; i >= 0; i-- {
		days = append(days, runInputs(runs[i]))
	}
	if len(runs) != 3 || !strings.Contains(days[0], `day="2026-10-07"`) || !strings.Contains(days[1], `day="2026-10-08"`) || !strings.Contains(days[2], `day="2026-10-09"`) {
		t.Fatalf("catch-up runs = %v", days)
	}
	slots := h.slots(w.ID, "scheduled-split")
	if len(slots) != 4 || slots[3].Key != "2026-10-06T07:00" || slots[3].Outcome != eviedb.WorkflowSlotSkipped ||
		!strings.Contains(slots[3].Detail, "3 slots from 2026-10-04T07:00 to 2026-10-06T07:00") || !strings.Contains(slots[3].Detail, "each_missed") {
		t.Fatalf("slots = %+v", slots)
	}
	notes, err := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{WorkspaceID: w.ID})
	if err != nil || len(notes) != 1 || notes[0].Kind != eviedb.WorkflowNotifyScheduleSkipped || !strings.Contains(notes[0].Text, "skipped") {
		t.Fatalf("notifications = %+v, %v", notes, err)
	}
	// Each recovery run progresses independently.
	for _, run := range runs {
		if d, err := h.rt.await(ctx, run.ID); err != nil || d.Run.State != eviedb.WorkflowRunSucceeded {
			t.Fatalf("run %s = %+v, %v", run.ID, d.Run, err)
		}
	}
}

// skip: missed slots run never; a slot first seen within the grace period
// runs, one seen later is skipped.
func TestWorkflowScheduleCatchUpSkipRunsOnlyOnTimeSlots(t *testing.T) {
	h := newLeaseHarness(t)
	w, _ := h.approveScheduled(scheduledVariant(t, "policy: each_missed\n    max_runs: 3", "policy: skip"))
	h.setClock("2026-10-09T11:02:00Z")
	h.tick(h.rt)
	runs := h.runs(w.ID)
	if len(runs) != 1 || !strings.Contains(runInputs(runs[0]), `day="2026-10-09"`) {
		t.Fatalf("runs = %+v", runs)
	}
	slots := h.slots(w.ID, "scheduled-split")
	if len(slots) != 2 || slots[1].Key != "2026-10-08T07:00" || slots[1].Outcome != eviedb.WorkflowSlotSkipped || !strings.Contains(slots[1].Detail, "5 slots") ||
		!strings.Contains(slots[1].Detail, "skip") {
		t.Fatalf("slots = %+v", slots)
	}
	// The next slot is first seen twenty minutes late: skipped.
	h.setClock("2026-10-10T11:20:00Z")
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 1 {
		t.Fatalf("a late slot ran under skip: %+v", runs)
	}
	if slots := h.slots(w.ID, "scheduled-split"); slots[0].Key != "2026-10-10T07:00" || slots[0].Outcome != eviedb.WorkflowSlotSkipped {
		t.Fatalf("slots = %+v", slots)
	}
}

// Daylight saving: a 01:30 slot on the day New York falls back runs once,
// at the first 01:30; a 02:30 slot on the day it springs forward runs when
// the gap ends, with its nominal time as $schedule.time.
func TestWorkflowScheduleDaylightSavingIsDeterministic(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	h.setClock("2026-10-31T12:00:00Z")
	w, _ := h.approveScheduled(scheduledVariant(t, `cron: "0 7 * * *"`, `cron: "30 1 * * *"`))
	h.setClock("2026-11-01T05:29:00Z")
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("runs before 01:30 EDT = %+v", runs)
	}
	h.setClock("2026-11-01T05:30:05Z") // 01:30:05 EDT
	h.tick(h.rt)
	h.setClock("2026-11-01T06:30:05Z") // 01:30:05 EST, the repeated hour
	h.tick(h.rt)
	runs := h.runs(w.ID)
	if len(runs) != 1 || runInputs(runs[0]) != `at="01:30", day="2026-11-01", people=3, total_cents=1000` {
		t.Fatalf("fall back runs = %+v", runs)
	}
	h.setClock("2026-11-02T06:30:05Z") // 01:30:05 EST
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 2 || !strings.Contains(runInputs(runs[0]), `day="2026-11-02"`) {
		t.Fatalf("the next day's run = %+v", runs)
	}

	spring := newLeaseHarness(t)
	spring.setClock("2027-03-13T12:00:00Z")
	sw, _ := spring.approveScheduled(scheduledVariant(t, `cron: "0 7 * * *"`, `cron: "30 2 * * *"`))
	spring.setClock("2027-03-14T06:59:00Z") // 01:59 EST: 02:30 does not exist today
	spring.tick(spring.rt)
	if runs := spring.runs(sw.ID); len(runs) != 0 {
		t.Fatalf("runs before the gap ends = %+v", runs)
	}
	spring.setClock("2027-03-14T07:00:05Z") // 03:00:05 EDT
	spring.tick(spring.rt)
	runs = spring.runs(sw.ID)
	if len(runs) != 1 || runInputs(runs[0]) != `at="02:30", day="2027-03-14", people=3, total_cents=1000` {
		t.Fatalf("spring forward runs = %+v", runs)
	}
	d, err := spring.rt.await(ctx, runs[0].ID)
	if err != nil || d.Slot == nil || !d.Slot.Shifted || !d.Slot.Due.Equal(time.Date(2027, 3, 14, 7, 0, 0, 0, time.UTC)) {
		t.Fatalf("shifted slot = %+v, %v", d.Slot, err)
	}
}

// The owner pauses and resumes a schedule without a new approval: no slot
// starts while it is paused, and slots that came due meanwhile are not
// caught up.
func TestWorkflowSchedulePauseAndResume(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveScheduled(scheduledSplitDir)
	control := func(action, surface string) (eviedb.WorkflowScheduleControl, error) {
		return h.rt.ControlSchedule(ctx, ScheduleControl{WorkspaceID: w.ID, Workflow: "scheduled-split", Action: action, Reason: "travelling", Surface: surface})
	}
	if _, err := control("pause", "chat"); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("a chat paused the schedule: %v", err)
	}
	if _, err := control("pause", "cli"); err != nil {
		t.Fatal(err)
	}
	if s := h.schedules(h.rt, w.ID); s[0].State != "paused" {
		t.Fatalf("paused schedule = %+v", s[0])
	}
	h.setClock("2026-10-05T11:00:10Z")
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("a paused schedule started %+v", runs)
	}
	h.setClock("2026-10-05T12:00:00Z")
	if _, err := control("resume", "web"); err != nil {
		t.Fatal(err)
	}
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("slots from the pause were caught up: %+v", runs)
	}
	h.setClock("2026-10-06T11:00:10Z")
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 1 || !strings.Contains(runInputs(runs[0]), `day="2026-10-06"`) {
		t.Fatalf("runs after resume = %+v", runs)
	}
	if d := h.show(w.ID, "scheduled-split"); d.Current.Version != 1 || len(d.Versions) != 1 {
		t.Fatalf("pausing changed the approval: %+v", d)
	}
}

// Deactivation stops the schedule; a block skips its slots visibly.
func TestWorkflowScheduleStopsWhenDeactivatedOrBlocked(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, a := h.approveScheduled(scheduledSplitDir)
	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: a.Name, Version: a.Version, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	h.setClock("2026-10-05T11:00:10Z")
	h.tick(h.rt)
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("a deactivated schedule started %+v", runs)
	}
	if s := h.schedules(h.rt, w.ID); len(s) != 1 || s[0].State != "inactive" || s[0].Next != nil {
		t.Fatalf("deactivated schedule = %+v", s)
	}

	b := newLeaseHarness(t)
	bw, _ := b.approveScheduled(scheduledSplitDir)
	if _, err := b.store.ArchiveWorkspace(ctx, bw.ID); err != nil {
		t.Fatal(err)
	}
	b.setClock("2026-10-04T11:00:10Z")
	b.tick(b.rt)
	if runs := b.runs(bw.ID); len(runs) != 0 {
		t.Fatalf("a blocked schedule started %+v", runs)
	}
	slots := b.slots(bw.ID, "scheduled-split")
	if len(slots) != 1 || slots[0].Outcome != eviedb.WorkflowSlotSkipped || !strings.Contains(slots[0].Detail, "blocked") {
		t.Fatalf("blocked slots = %+v", slots)
	}
}

// A slot whose start this Evie cannot make (its AI steps are off here) is
// left undecided for an Evie that can; the error is reported, not
// recorded as the slot's outcome.
func TestWorkflowScheduleLeavesASlotItCannotStartForAnotherEvie(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	h.modelsOff = "AI steps are off: set EVIE_WORKFLOW_REMOTE_MODELS=on"
	h.reopen()
	dir := copyDefinition(t, "receipt-split")
	path := filepath.Join(dir, "workflow.yaml")
	text := strings.Replace(readFile(t, path), "retention:", `schedule:
  cron: "0 7 * * *"
  timezone: America/New_York
  catch_up:
    policy: each_missed
    max_runs: 1
  inputs:
    receipt_id: $schedule.date
    receipt_text: "TOTAL DUE 12.00"
    people: 3

retention:`, 1)
	writeFile(t, path, text)
	w := h.workspace()
	h.approve(h.propose(w.ID, dir, receiptFixtures(t)))
	h.setClock("2026-10-04T11:00:10Z")
	err := h.rt.tickSchedules(ctx)
	if err == nil || !errors.Is(err, ErrModelsUnavailable) {
		t.Fatalf("tick without AI steps: %v", err)
	}
	if runs, slots := h.runs(w.ID), h.slots(w.ID, "receipt-split"); len(runs) != 0 || len(slots) != 0 {
		t.Fatalf("runs %+v slots %+v", runs, slots)
	}
	chat := newFakeChat(reply(`{"total_cents":1200,"confidence":"high"}`, 100, 20))
	h.models, h.modelsOff = NewOpenRouterModels(chat), ""
	able := h.process()
	h.tick(able.rt)
	runs := h.runs(w.ID)
	if len(runs) != 1 || runs[0].Surface != "schedule" || !strings.Contains(runInputs(runs[0]), `receipt_id="2026-10-04"`) {
		t.Fatalf("runs = %+v", runs)
	}
	_ = json.RawMessage(nil)
}
