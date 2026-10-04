package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow Schedules (Stage 14 sub-stage 11 of docs/memory-architecture.md,
// 2026-10-04; ADR 0020, 0021 and 0023). The schedule a definition declares
// was recorded with its approval (sub-stage 2) and is active with it: an
// active, unblocked latest version's schedule starts runs, and
// deactivation or a block stops it. SQLite records:
//
//   - workflow_schedule_slots: one row per decided slot, keyed by the
//     Workspace, the workflow, the schedule's timezone and the slot's
//     nominal local time ("2006-01-02T15:04"), so a slot is decided once
//     whatever version or process decides it. A scheduled run's slot is
//     recorded in the transaction that records the run (StartWorkflowRun):
//     "started", or "existing" when its Run Key was already held (a manual
//     start of the same logical work). A slot the Catch-up Policy skips or
//     whose start was refused is recorded "skipped" or "failed" with a
//     notification. A second process deciding the same slot is refused
//     (ErrWorkflowScheduleSlotTaken), even after the slot's run failed and
//     freed its key. Immutable.
//   - workflow_schedule_controls: the owner's pause and resume of a
//     version's schedule, which need no new approval (the schedule itself
//     is in the approved hash; changing it needs one). A new approval's
//     schedule starts unpaused, since that approval activates it (ADR
//     0020). Immutable.
//   - workflow_scheduler_state: the single cross-process runner lease of
//     the schedule ticker, like the memory background pass's.
//
// A schedule's cursor, after which slots are due, is the latest of its
// approval, its latest resume and its latest decided slot: slots that came
// due while it was paused are never caught up.

var (
	// ErrWorkflowScheduleSlotTaken refuses deciding a slot already decided.
	ErrWorkflowScheduleSlotTaken = errors.New("this schedule slot was already decided")
	// ErrWorkflowScheduleNotFound refuses controlling a workflow whose
	// current version has no schedule.
	ErrWorkflowScheduleNotFound = errors.New("the workflow's current version has no schedule")
	// ErrWorkflowScheduleControl refuses pausing a paused schedule or
	// resuming one that is not paused.
	ErrWorkflowScheduleControl = errors.New("the schedule is already in that state")
)

// Slot outcomes.
const (
	WorkflowSlotStarted  = "started"
	WorkflowSlotExisting = "existing"
	WorkflowSlotSkipped  = "skipped"
	WorkflowSlotFailed   = "failed"
)

// WorkflowScheduleSlotStart is the slot a scheduled start decides, with the
// runtime instance deciding it.
type WorkflowScheduleSlotStart struct {
	Timezone string
	Key      string
	Due      time.Time
	Shifted  bool
	Holder   string
}

// WorkflowScheduleSlot is one decided slot.
type WorkflowScheduleSlot struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Workflow    string             `json:"workflow"`
	Version     int64              `json:"version"`
	Timezone    string             `json:"timezone"`
	Key         string             `json:"key"`
	Due         time.Time          `json:"due"`
	Shifted     bool               `json:"shifted,omitempty"`
	Outcome     string             `json:"outcome"`
	RunID       string             `json:"run_id,omitempty"`
	Detail      string             `json:"detail,omitempty"`
	DecidedBy   string             `json:"decided_by"`
	DecidedAt   time.Time          `json:"decided_at"`
}

// WorkflowScheduleControl is the owner pausing or resuming a schedule.
type WorkflowScheduleControl struct {
	Seq         int64              `json:"seq"`
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Workflow    string             `json:"workflow"`
	Version     int64              `json:"version"`
	Action      string             `json:"action"`
	Reason      string             `json:"reason,omitempty"`
	Actor       string             `json:"actor"`
	Surface     string             `json:"surface"`
	At          time.Time          `json:"at"`
}

// WorkflowScheduleControlRequest is the owner pausing ("pause") or
// resuming ("resume") the current version's schedule; Version, when set,
// must be the current version.
type WorkflowScheduleControlRequest struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Workflow    string             `json:"name"`
	Version     int64              `json:"version,omitempty"`
	Action      string             `json:"action"`
	Reason      string             `json:"reason,omitempty"`
	Surface     string             `json:"-"`
}

// WorkflowSchedule is a definition's current version's schedule as the
// ticker and the owner see it: the approved schedule, the definition's
// status, the owner's latest control, the latest decided slot and the
// cursor after which slots are due.
type WorkflowSchedule struct {
	WorkspaceID memory.WorkspaceID       `json:"workspace_id"`
	Workflow    string                   `json:"workflow"`
	Version     int64                    `json:"version"`
	Schedule    workflow.ScheduleReport  `json:"schedule"`
	ApprovedAt  time.Time                `json:"approved_at"`
	Status      WorkflowStatus           `json:"status"`
	Blocked     []string                 `json:"blocked,omitempty"`
	Paused      bool                     `json:"paused"`
	Control     *WorkflowScheduleControl `json:"control,omitempty"`
	LastSlot    *WorkflowScheduleSlot    `json:"last_slot,omitempty"`
	Cursor      workflow.SlotCursor      `json:"cursor"`
}

func ensureWorkflowScheduleSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_schedule_slots (
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 64),
 slot_key TEXT NOT NULL CHECK(length(slot_key) = 16),
 due_at TEXT NOT NULL,
 shifted INTEGER NOT NULL CHECK(shifted IN (0,1)),
 outcome TEXT NOT NULL CHECK(outcome IN ('started','existing','skipped','failed')),
 run_id TEXT REFERENCES workflow_runs(id),
 detail TEXT NOT NULL CHECK(length(detail) <= 4000),
 decided_by TEXT NOT NULL CHECK(length(decided_by) BETWEEN 1 AND 128),
 decided_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, workflow, timezone, slot_key),
 FOREIGN KEY(workspace_id, workflow, version) REFERENCES workflow_activations(workspace_id, name, version),
 CHECK((outcome IN ('started','existing')) = (run_id IS NOT NULL)));
 CREATE INDEX IF NOT EXISTS workflow_schedule_slots_by_run ON workflow_schedule_slots(run_id) WHERE run_id IS NOT NULL;
 CREATE TRIGGER IF NOT EXISTS workflow_schedule_slots_no_update BEFORE UPDATE ON workflow_schedule_slots
 BEGIN SELECT RAISE(ABORT,'workflow schedule slots are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_schedule_slots_no_delete BEFORE DELETE ON workflow_schedule_slots
 BEGIN SELECT RAISE(ABORT,'workflow schedule slots are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_schedule_controls (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 action TEXT NOT NULL CHECK(action IN ('pause','resume')),
 reason TEXT NOT NULL CHECK(length(reason) <= 1000),
 actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 recorded_at TEXT NOT NULL,
 FOREIGN KEY(workspace_id, workflow, version) REFERENCES workflow_activations(workspace_id, name, version));
 CREATE TRIGGER IF NOT EXISTS workflow_schedule_controls_no_update BEFORE UPDATE ON workflow_schedule_controls
 BEGIN SELECT RAISE(ABORT,'workflow schedule controls are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_schedule_controls_no_delete BEFORE DELETE ON workflow_schedule_controls
 BEGIN SELECT RAISE(ABORT,'workflow schedule controls are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_scheduler_state (
 singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
 runner_holder TEXT,
 runner_expires_at TEXT);
 INSERT OR IGNORE INTO workflow_scheduler_state(singleton) VALUES(1);`)
	if err != nil {
		return fmt.Errorf("create workflow schedule schema: %w", err)
	}
	return nil
}

func validSlotStart(s *WorkflowScheduleSlotStart) error {
	switch {
	case s.Timezone == "" || len(s.Timezone) > 64:
		return errors.New("a schedule slot names its timezone")
	case len(s.Key) != 16:
		return fmt.Errorf("a schedule slot key is a nominal local time such as 2026-10-05T07:00, not %q", s.Key)
	case s.Due.IsZero():
		return errors.New("a schedule slot needs its due time")
	case s.Holder == "" || len(s.Holder) > 128:
		return errors.New("a schedule slot names the runtime deciding it")
	}
	return nil
}

// slotDecided reports whether a slot of the workflow's schedule in tz was
// decided.
func slotDecided(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, workflow, tz, key string) (bool, error) {
	var decided bool
	err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_schedule_slots WHERE workspace_id=? AND workflow=? AND timezone=? AND slot_key=?)`,
		ws, workflow, tz, key).Scan(&decided)
	return decided, err
}

// recordStartedSlot records, in the run's transaction, the slot a
// scheduled start decided: the activation must have a schedule in the
// slot's timezone, and the slot must not have been decided.
func recordStartedSlot(ctx context.Context, conn *sql.Conn, a WorkflowActivation, slot *WorkflowScheduleSlotStart, outcome, runID string, now time.Time) error {
	if a.Schedule == nil || a.Schedule.Timezone != slot.Timezone {
		return fmt.Errorf("version %d of %s has no schedule in %s", a.Version, a.Name, slot.Timezone)
	}
	decided, err := slotDecided(ctx, conn, a.WorkspaceID, a.Name, slot.Timezone, slot.Key)
	if err != nil {
		return err
	}
	if decided {
		return fmt.Errorf("%w: %s %s", ErrWorkflowScheduleSlotTaken, slot.Key, slot.Timezone)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_schedule_slots(workspace_id,workflow,version,timezone,slot_key,due_at,shifted,outcome,run_id,detail,decided_by,decided_at)
 VALUES(?,?,?,?,?,?,?,?,?,'',?,?)`, a.WorkspaceID, a.Name, a.Version, slot.Timezone, slot.Key, formatLeaseTime(slot.Due), slot.Shifted, outcome, runID,
		slot.Holder, formatTime(now)); err != nil {
		return fmt.Errorf("record workflow schedule slot: %w", err)
	}
	return nil
}

// RecordWorkflowScheduleSlot records a slot the schedule did not start a
// run for: skipped by its Catch-up Policy (or while the definition was
// blocked), or failed because its start was refused, with a notification
// in the same transaction.
func (s *Store) RecordWorkflowScheduleSlot(ctx context.Context, slot WorkflowScheduleSlot) error {
	var kind WorkflowNotificationKind
	switch slot.Outcome {
	case WorkflowSlotSkipped:
		kind = WorkflowNotifyScheduleSkipped
	case WorkflowSlotFailed:
		kind = WorkflowNotifyScheduleFailed
	default:
		return fmt.Errorf("only skipped and failed slots are recorded without their run, not %q", slot.Outcome)
	}
	start := &WorkflowScheduleSlotStart{Timezone: slot.Timezone, Key: slot.Key, Due: slot.Due, Holder: slot.DecidedBy}
	if err := validSlotStart(start); err != nil {
		return err
	}
	if strings.TrimSpace(slot.Detail) == "" || len(slot.Detail) > 4000 {
		return errors.New("a skipped or failed slot says why in at most 4000 bytes")
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		decided, err := slotDecided(ctx, conn, slot.WorkspaceID, slot.Workflow, slot.Timezone, slot.Key)
		if err != nil {
			return err
		}
		if decided {
			return fmt.Errorf("%w: %s %s", ErrWorkflowScheduleSlotTaken, slot.Key, slot.Timezone)
		}
		now := s.now().UTC()
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_schedule_slots(workspace_id,workflow,version,timezone,slot_key,due_at,shifted,outcome,run_id,detail,decided_by,decided_at)
 VALUES(?,?,?,?,?,?,?,?,NULL,?,?,?)`, slot.WorkspaceID, slot.Workflow, slot.Version, slot.Timezone, slot.Key, formatLeaseTime(slot.Due), slot.Shifted, slot.Outcome,
			slot.Detail, slot.DecidedBy, formatTime(now)); err != nil {
			return fmt.Errorf("record workflow schedule slot: %w", err)
		}
		key := fmt.Sprintf("schedule:%s/%s:%s:%s", slot.WorkspaceID, slot.Workflow, slot.Timezone, slot.Key)
		detail := fmt.Sprintf("slot %s %s: %s", slot.Key, slot.Timezone, slot.Detail)
		return insertScheduleNotification(ctx, conn, key, slot.WorkspaceID, slot.Workflow, kind, detail, now)
	})
}

const slotColumns = `workspace_id,workflow,version,timezone,slot_key,due_at,shifted,outcome,run_id,detail,decided_by,decided_at`

func scanScheduleSlot(scanner rowScanner) (WorkflowScheduleSlot, error) {
	var s WorkflowScheduleSlot
	var due, decided string
	var run sql.NullString
	if err := scanner.Scan(&s.WorkspaceID, &s.Workflow, &s.Version, &s.Timezone, &s.Key, &due, &s.Shifted, &s.Outcome, &run, &s.Detail, &s.DecidedBy, &decided); err != nil {
		return s, err
	}
	s.RunID = run.String
	var err error
	if s.Due, err = parseLeaseTime(due); err != nil {
		return s, err
	}
	s.DecidedAt, err = parseTime(decided)
	return s, err
}

// WorkflowScheduleSlots lists a workflow's latest decided slots, newest
// first (at most limit, default 20, at most 200).
func (s *Store) WorkflowScheduleSlots(ctx context.Context, ws memory.WorkspaceID, name string, limit int) ([]WorkflowScheduleSlot, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+slotColumns+` FROM workflow_schedule_slots WHERE workspace_id=? AND workflow=? ORDER BY due_at DESC, slot_key DESC LIMIT ?`,
		ws, name, min(limit, 200))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowScheduleSlot{}
	for rows.Next() {
		slot, err := scanScheduleSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, slot)
	}
	return out, rows.Err()
}

// runSlot is the slot that started a run, if a schedule started it.
func runSlot(ctx context.Context, q queryRower, runID string) (*WorkflowScheduleSlot, error) {
	slot, err := scanScheduleSlot(q.QueryRowContext(ctx, `SELECT `+slotColumns+` FROM workflow_schedule_slots WHERE run_id=? AND outcome='started'`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &slot, nil
}

const controlColumns = `seq,workspace_id,workflow,version,action,reason,actor,surface,recorded_at`

func scanScheduleControl(scanner rowScanner) (WorkflowScheduleControl, error) {
	var c WorkflowScheduleControl
	var at string
	if err := scanner.Scan(&c.Seq, &c.WorkspaceID, &c.Workflow, &c.Version, &c.Action, &c.Reason, &c.Actor, &c.Surface, &at); err != nil {
		return c, err
	}
	var err error
	c.At, err = parseTime(at)
	return c, err
}

// latestScheduleControl is the owner's latest pause or resume of a
// version's schedule, if any.
func latestScheduleControl(ctx context.Context, q queryRower, ws memory.WorkspaceID, name string, version int64) (*WorkflowScheduleControl, error) {
	c, err := scanScheduleControl(q.QueryRowContext(ctx, `SELECT `+controlColumns+` FROM workflow_schedule_controls WHERE workspace_id=? AND workflow=? AND version=? ORDER BY seq DESC LIMIT 1`,
		ws, name, version))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ControlWorkflowSchedule records the owner pausing or resuming the
// current version's schedule. No approval is needed: the schedule itself
// does not change.
func (s *Store) ControlWorkflowSchedule(ctx context.Context, req WorkflowScheduleControlRequest) (WorkflowScheduleControl, error) {
	if req.Surface != "cli" && req.Surface != "web" {
		return WorkflowScheduleControl{}, ErrWorkflowApprover
	}
	if req.Action != "pause" && req.Action != "resume" {
		return WorkflowScheduleControl{}, fmt.Errorf("a schedule is paused or resumed, not %q", req.Action)
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 1000 {
		return WorkflowScheduleControl{}, errors.New("a schedule control reason is at most 1000 bytes")
	}
	var out WorkflowScheduleControl
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		latest, ok, err := latestActivation(ctx, conn, req.WorkspaceID, req.Workflow)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s", ErrWorkflowNotFound, req.Workflow)
		}
		if req.Version != 0 && req.Version != latest.Version {
			return fmt.Errorf("%w: the current version is %d", ErrWorkflowChanged, latest.Version)
		}
		if latest.Schedule == nil {
			return fmt.Errorf("%w: %s version %d", ErrWorkflowScheduleNotFound, req.Workflow, latest.Version)
		}
		if latest.Deactivated != nil {
			return fmt.Errorf("%w: version %d was deactivated", ErrWorkflowNotActive, latest.Version)
		}
		control, err := latestScheduleControl(ctx, conn, req.WorkspaceID, req.Workflow, latest.Version)
		if err != nil {
			return err
		}
		paused := control != nil && control.Action == "pause"
		if paused == (req.Action == "pause") {
			state := "running"
			if paused {
				state = "paused"
			}
			return fmt.Errorf("%w: the schedule of %s version %d is %s", ErrWorkflowScheduleControl, req.Workflow, latest.Version, state)
		}
		res, err := conn.ExecContext(ctx, `INSERT INTO workflow_schedule_controls(workspace_id,workflow,version,action,reason,actor,surface,recorded_at) VALUES(?,?,?,?,?,'owner',?,?)`,
			req.WorkspaceID, req.Workflow, latest.Version, req.Action, reason, req.Surface, formatTime(s.now()))
		if err != nil {
			return fmt.Errorf("record workflow schedule control: %w", err)
		}
		seq, err := res.LastInsertId()
		if err != nil {
			return err
		}
		out, err = scanScheduleControl(conn.QueryRowContext(ctx, `SELECT `+controlColumns+` FROM workflow_schedule_controls WHERE seq=?`, seq))
		return err
	})
	return out, err
}

// WorkflowSchedules lists the schedules of every definition whose current
// version declares one, in a Workspace or in every Workspace when ws is "".
func (s *Store) WorkflowSchedules(ctx context.Context, ws memory.WorkspaceID) ([]WorkflowSchedule, error) {
	defs, err := s.ListWorkflowDefinitions(ctx, ws)
	if err != nil {
		return nil, err
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	out := []WorkflowSchedule{}
	for _, d := range defs {
		a := d.Current
		if a == nil || a.Schedule == nil {
			continue
		}
		sched := WorkflowSchedule{WorkspaceID: d.WorkspaceID, Workflow: d.Name, Version: a.Version, Schedule: *a.Schedule, ApprovedAt: a.ApprovedAt,
			Status: d.Status, Blocked: d.Blocked, Cursor: workflow.SlotCursor{At: a.ApprovedAt}}
		if sched.Control, err = latestScheduleControl(ctx, conn, d.WorkspaceID, d.Name, a.Version); err != nil {
			return nil, err
		}
		if c := sched.Control; c != nil {
			sched.Paused = c.Action == "pause"
			if c.Action == "resume" && c.At.After(sched.Cursor.At) {
				sched.Cursor = workflow.SlotCursor{At: c.At}
			}
		}
		last, err := scanScheduleSlot(conn.QueryRowContext(ctx, `SELECT `+slotColumns+` FROM workflow_schedule_slots WHERE workspace_id=? AND workflow=? AND timezone=?
 ORDER BY due_at DESC, slot_key DESC LIMIT 1`, d.WorkspaceID, d.Name, a.Schedule.Timezone))
		switch {
		case err == nil:
			sched.LastSlot = &last
			if last.Due.After(sched.Cursor.At) {
				sched.Cursor = workflow.SlotCursor{At: last.Due, Key: last.Key}
			}
		case !errors.Is(err, sql.ErrNoRows):
			return nil, err
		}
		out = append(out, sched)
	}
	return out, nil
}

// AcquireWorkflowScheduleRunner takes or renews the single cross-process
// lease of the schedule ticker; false while another holder's is unexpired.
func (s *Store) AcquireWorkflowScheduleRunner(ctx context.Context, holder string, ttl time.Duration) (bool, error) {
	if holder == "" || ttl <= 0 {
		return false, errors.New("the schedule runner lease needs a holder and a duration")
	}
	acquired := false
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		res, err := conn.ExecContext(ctx, `UPDATE workflow_scheduler_state SET runner_holder=?, runner_expires_at=?
 WHERE singleton=1 AND (runner_holder IS NULL OR runner_holder=? OR runner_expires_at<=?)`, holder, formatLeaseTime(now.Add(ttl)), holder, formatLeaseTime(now))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		acquired = n == 1
		return err
	})
	return acquired, err
}

// ReleaseWorkflowScheduleRunner gives the lease up if holder still has it.
func (s *Store) ReleaseWorkflowScheduleRunner(ctx context.Context, holder string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_scheduler_state SET runner_holder=NULL, runner_expires_at=NULL WHERE singleton=1 AND runner_holder=?`, holder)
	return err
}
