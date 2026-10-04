package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow Schedules (Stage 14 sub-stage 11, 2026-10-04; ADR 0020, 0021 and
// 0023; workflow-runtime.spec.md "Scheduled start and bounded Catch-up
// Policy follow as a separate slice using the same run-start interface").
//
// The schedule an approval recorded is active with it. In long-lived
// processes (`evie serve`, the REPL) the scheduler's schedule ticker runs
// every SchedulerConfig.Schedules, under a single cross-process runner
// lease like the memory background pass's. Each tick reads the current
// version of every definition that declares a schedule and, for each that
// is active and not paused by the owner, the slots due since its cursor
// (the latest of its approval, the owner's latest resume and its latest
// decided slot), computed on the schedule's IANA timezone's wall clock
// (workflow.Timetable documents the daylight-saving rules). Its Catch-up
// Policy then decides:
//
//   - skip: only slots first seen within SchedulerConfig.ScheduleGrace of
//     their time start; older ones (Evie was not running, or could not
//     start them) are skipped.
//   - each_missed with max_runs N: the latest N due slots start, one run
//     each, oldest first; older ones are skipped.
//
// Skipped slots are recorded as one "skipped" slot (the latest of them)
// with how many there were and a notification, which moves the cursor
// past them. A due slot of a blocked definition is skipped the same way;
// a deactivated or owner-paused schedule decides nothing, and slots that
// came due while it was paused are never caught up.
//
// A slot starts through StartRun with surface "schedule": its nominal
// local date and time feed the run's inputs ($schedule.date and
// $schedule.time) and so its Run Key, and the store records the slot with
// the run in one transaction, so no two processes ever start one slot
// (and a slot whose logical work a run already holds records that run). A
// slot this Evie cannot start (its AI steps or workers are off here, a
// pinned provider is unavailable) stays undecided for an Evie that can:
// the ticker reports it and gives the runner lease up for a while. Any
// other refusal records the slot "failed" with a notification.

const (
	defaultScheduleTick  = 15 * time.Second
	defaultScheduleGrace = 5 * time.Minute
	// scheduleRunnerLease is how many ticks the runner lease outlasts the
	// tick that took it; scheduleYieldTicks how many ticks a runtime that
	// could not start a slot leaves the lease to others.
	scheduleRunnerLease = 4
	scheduleYieldTicks  = 4
	// scheduleKeep bounds the latest slots a tick looks at: more than any
	// max_runs (31) and any skip grace's slots.
	scheduleKeep = 64
)

// errScheduleChanged refuses a scheduled start whose definition was
// approved again since the tick read it: the next tick reads the new
// schedule.
var errScheduleChanged = errors.New("the workflow was approved again since its schedule was read")

// ScheduledStart is the slot a start with surface "schedule" decides: the
// approved version whose schedule it belongs to and the slot.
type ScheduledStart struct {
	Version  int64
	Timezone string
	Slot     workflow.Slot
}

// ScheduleStatus is a schedule as the owner sees it: its state ("active",
// "paused", "inactive" or "blocked") and its next slot after its cursor
// (overdue when that is already due), which is nil for an inactive
// schedule or a cron that never matches.
type ScheduleStatus struct {
	eviedb.WorkflowSchedule
	State   string         `json:"state"`
	Next    *workflow.Slot `json:"next,omitempty"`
	Overdue bool           `json:"overdue,omitempty"`
}

// ScheduleControl is the owner pausing or resuming a schedule.
type ScheduleControl struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Workflow    string             `json:"name"`
	Version     int64              `json:"version,omitempty"`
	Action      string             `json:"action"`
	Reason      string             `json:"reason,omitempty"`
	Surface     string             `json:"-"`
}

// Schedules lists the schedules of a Workspace's definitions, or of every
// Workspace when ws is "".
func (r *Runtime) Schedules(ctx context.Context, ws memory.WorkspaceID) ([]ScheduleStatus, error) {
	schedules, err := r.store.WorkflowSchedules(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]ScheduleStatus, 0, len(schedules))
	now := r.now()
	for _, s := range schedules {
		st := ScheduleStatus{WorkflowSchedule: s, State: scheduleState(s)}
		if st.State != "inactive" {
			if tt, err := workflow.NewTimetable(s.Schedule.Cron, s.Schedule.Timezone); err == nil {
				// A paused schedule's slots until its resume never run, so its
				// next slot is the first after now.
				cursor := s.Cursor
				if s.Paused && now.After(cursor.At) {
					cursor = workflow.SlotCursor{At: now}
				}
				if next, ok := tt.Next(cursor); ok {
					st.Next, st.Overdue = &next, !next.Due.After(now)
				}
			}
		}
		out = append(out, st)
	}
	return out, nil
}

func scheduleState(s eviedb.WorkflowSchedule) string {
	switch {
	case s.Status == eviedb.WorkflowInactive:
		return "inactive"
	case s.Paused:
		return "paused"
	case s.Status == eviedb.WorkflowBlocked:
		return "blocked"
	}
	return "active"
}

// ControlSchedule records the owner pausing or resuming the current
// version's schedule; only the owner's CLI and web surfaces may.
func (r *Runtime) ControlSchedule(ctx context.Context, c ScheduleControl) (eviedb.WorkflowScheduleControl, error) {
	if !ownerSurface(c.Surface) {
		return eviedb.WorkflowScheduleControl{}, eviedb.ErrWorkflowApprover
	}
	return r.store.ControlWorkflowSchedule(ctx, eviedb.WorkflowScheduleControlRequest{WorkspaceID: c.WorkspaceID, Workflow: c.Workflow, Version: c.Version,
		Action: c.Action, Reason: c.Reason, Surface: c.Surface})
}

// tickSchedules is one schedule tick: under the runner lease, decide every
// active schedule's due slots. It returns the errors it met, each schedule
// apart from the others.
func (r *Runtime) tickSchedules(ctx context.Context) error {
	now := r.now()
	r.mu.Lock()
	yielding := now.Before(r.scheduleYield)
	r.mu.Unlock()
	if yielding {
		return nil
	}
	held, err := r.store.AcquireWorkflowScheduleRunner(ctx, r.holder, scheduleRunnerLease*r.sched.Schedules)
	if err != nil || !held {
		return err
	}
	schedules, err := r.store.WorkflowSchedules(ctx, "")
	if err != nil {
		return err
	}
	var errs []error
	yield := false
	for _, s := range schedules {
		if ctx.Err() != nil {
			break
		}
		if err := r.tickSchedule(ctx, s, now); err != nil {
			yield = yield || cannotStartHere(err)
			errs = append(errs, fmt.Errorf("schedule of %s in Workspace %s: %w", s.Workflow, s.WorkspaceID, err))
		}
	}
	if yield {
		// Another Evie may be able to start what this one cannot.
		r.mu.Lock()
		r.scheduleYield = now.Add(scheduleYieldTicks * r.sched.Schedules)
		r.mu.Unlock()
		if err := r.store.ReleaseWorkflowScheduleRunner(context.WithoutCancel(ctx), r.holder); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// cannotStartHere reports a refusal that another Evie might not meet: this
// Evie's AI steps or workers are off, its runtime lacks a step kind, or a
// pinned provider is unavailable here now.
func cannotStartHere(err error) bool {
	return errors.Is(err, ErrModelsUnavailable) || errors.Is(err, ErrAgentsUnavailable) || errors.Is(err, ErrUnsupportedSteps) || errors.Is(err, ErrCapabilityUnavailable)
}

// tickSchedule decides one schedule's due slots.
func (r *Runtime) tickSchedule(ctx context.Context, s eviedb.WorkflowSchedule, now time.Time) error {
	if s.Status == eviedb.WorkflowInactive || s.Paused {
		return nil
	}
	tt, err := workflow.NewTimetable(s.Schedule.Cron, s.Schedule.Timezone)
	if err != nil {
		return fmt.Errorf("its approved schedule cannot be read: %w", err)
	}
	if next, ok := tt.Next(s.Cursor); !ok || next.Due.After(now) {
		return nil
	}
	window := tt.Due(s.Cursor, now, scheduleKeep)
	if window.Total == 0 {
		return nil
	}
	latest := window.Latest
	var start []workflow.Slot
	var why string
	switch {
	case s.Status == eviedb.WorkflowBlocked:
		why = fmt.Sprintf("the workflow is blocked (%s), so no run started", strings.Join(s.Blocked, "; "))
	case s.Schedule.CatchUp == workflow.CatchUpEachMissed:
		n := min(max(s.Schedule.MaxRuns, 1), len(latest))
		start = latest[len(latest)-n:]
		why = fmt.Sprintf("catch-up policy each_missed starts at most the latest %d", s.Schedule.MaxRuns)
	default:
		i := len(latest)
		for i > 0 && now.Sub(latest[i-1].Due) <= r.sched.ScheduleGrace {
			i--
		}
		start = latest[i:]
		why = fmt.Sprintf("they missed their time by more than %s (no Evie that could start them was running), and catch-up policy skip starts none of them", r.sched.ScheduleGrace)
	}
	if skipped := window.Total - len(start); skipped > 0 && len(latest) > len(start) {
		last := latest[len(latest)-len(start)-1]
		first := last
		if window.First != nil {
			first = *window.First
		}
		count := fmt.Sprintf("%d slots from %s to %s", skipped, first.Key, last.Key)
		if skipped == 1 {
			count = fmt.Sprintf("1 slot (%s)", last.Key)
		}
		if window.Truncated {
			count = fmt.Sprintf("at least %d slots up to %s", skipped, last.Key)
		}
		slot := eviedb.WorkflowScheduleSlot{WorkspaceID: s.WorkspaceID, Workflow: s.Workflow, Version: s.Version, Timezone: s.Schedule.Timezone, Key: last.Key,
			Due: last.Due, Shifted: last.Shifted, Outcome: eviedb.WorkflowSlotSkipped, Detail: clipText(count+" skipped: "+why, 4000), DecidedBy: r.holder}
		if err := r.store.RecordWorkflowScheduleSlot(ctx, slot); err != nil && !errors.Is(err, eviedb.ErrWorkflowScheduleSlotTaken) {
			return err
		}
	}
	for _, slot := range start {
		if _, err := r.startSlot(ctx, s, slot); err != nil {
			return err
		}
	}
	return nil
}

// startSlot starts one slot's run through StartRun, or records why it
// could not. It reports whether this call decided the slot by starting
// (or finding) its run; a slot another process decided is not an error.
func (r *Runtime) startSlot(ctx context.Context, s eviedb.WorkflowSchedule, slot workflow.Slot) (bool, error) {
	_, err := r.StartRun(ctx, RunRequest{WorkspaceID: s.WorkspaceID, Workflow: s.Workflow, Surface: "schedule",
		Schedule: &ScheduledStart{Version: s.Version, Timezone: s.Schedule.Timezone, Slot: slot}})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, eviedb.ErrWorkflowScheduleSlotTaken), errors.Is(err, errScheduleChanged), errors.Is(err, eviedb.ErrWorkflowChanged), admissionRefused(err):
		// Decided elsewhere, or the definition changed since the tick read
		// it: the next tick reads it again.
		return false, nil
	case cannotStartHere(err):
		return false, err
	}
	failed := eviedb.WorkflowScheduleSlot{WorkspaceID: s.WorkspaceID, Workflow: s.Workflow, Version: s.Version, Timezone: s.Schedule.Timezone, Key: slot.Key,
		Due: slot.Due, Shifted: slot.Shifted, Outcome: eviedb.WorkflowSlotFailed, Detail: clipText("its run could not start: "+err.Error(), 4000), DecidedBy: r.holder}
	if recordErr := r.store.RecordWorkflowScheduleSlot(ctx, failed); recordErr != nil && !errors.Is(recordErr, eviedb.ErrWorkflowScheduleSlotTaken) {
		return false, errors.Join(err, recordErr)
	}
	return false, nil
}

// SchedulesText renders schedules for the terminal.
func SchedulesText(list []ScheduleStatus) string {
	if len(list) == 0 {
		return "No workflow declares a schedule.\n"
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	for _, s := range list {
		catchUp := s.Schedule.CatchUp
		if s.Schedule.MaxRuns > 0 {
			catchUp += fmt.Sprintf(" (at most %d)", s.Schedule.MaxRuns)
		}
		line("%s  %s v%d  %s %s  catch-up %s  %s", s.WorkspaceID, s.Workflow, s.Version, s.Schedule.Cron, s.Schedule.Timezone, catchUp, s.State)
		if len(s.Blocked) > 0 {
			line("  blocked: %s", strings.Join(s.Blocked, "; "))
		}
		if c := s.Control; c != nil && c.Action == "pause" {
			line("  paused by the %s via %s at %s%s; resume with evie workflows schedule resume %s %s", c.Actor, c.Surface, c.At.Format(timeLayout), reasonText(c.Reason), s.WorkspaceID, s.Workflow)
		}
		if n := s.Next; n != nil {
			when := "next"
			if s.Overdue {
				when = "due now"
			}
			shifted := ""
			if n.Shifted {
				shifted = " (that time does not exist that day; it runs when the clocks change)"
			}
			line("  %s: %s %s, %s%s", when, n.Key, s.Schedule.Timezone, n.Due.UTC().Format(timeLayout), shifted)
		}
		if l := s.LastSlot; l != nil {
			text := fmt.Sprintf("  last slot: %s %s", l.Key, l.Outcome)
			if l.RunID != "" {
				text += " run " + l.RunID
			}
			if l.Detail != "" {
				text += ": " + l.Detail
			}
			line("%s", text)
		}
	}
	return b.String()
}
