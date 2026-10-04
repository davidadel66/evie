package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Durable notifications (Stage 14 sub-stage 11 of
// docs/memory-architecture.md, 2026-10-04; ADR 0046, workflow-runtime.spec.md
// user story 59): SQLite is the authoritative record of every run
// transition the owner should hear about, so no session has to stay open
// and nothing external owns the event.
//
//   - workflow_notifications: one row per notice. A trigger on
//     workflow_runs writes it in the transaction that changes the run's
//     state, whatever path changes it, for the transitions that finish a
//     run (succeeded, failed, cancelled) or need the owner (waiting for an
//     answer, needing reconciliation, paused with its reason, and paused at
//     the daily cap, which is its own kind). Each is deduplicated by run
//     and transition (its state, the checkpoint it happened at and its
//     reason), so a pause that is rechecked or flaps at the same boundary
//     for the same reason notifies once. The schedule records skipped and
//     failed slots the same way (workflow_schedules.go). A notice carries
//     the run's Workspace, workflow, origin surface and session, so a chat
//     can find the runs it started. Content is immutable and nothing is
//     deleted; only the owner's read state changes.
//
// Runs that changed state before this table existed have no notices: only
// new transitions notify.

// WorkflowNotificationKind is what a notice reports.
type WorkflowNotificationKind string

const (
	WorkflowNotifySucceeded           WorkflowNotificationKind = "succeeded"
	WorkflowNotifyFailed              WorkflowNotificationKind = "failed"
	WorkflowNotifyCancelled           WorkflowNotificationKind = "cancelled"
	WorkflowNotifyWaitingForHuman     WorkflowNotificationKind = "waiting_for_human"
	WorkflowNotifyNeedsReconciliation WorkflowNotificationKind = "needs_reconciliation"
	WorkflowNotifyPaused              WorkflowNotificationKind = "paused"
	WorkflowNotifyCapReached          WorkflowNotificationKind = "cap_reached"
	WorkflowNotifyScheduleSkipped     WorkflowNotificationKind = "schedule_skipped"
	WorkflowNotifyScheduleFailed      WorkflowNotificationKind = "schedule_failed"
)

// WorkflowNotification is one durable notice.
type WorkflowNotification struct {
	ID          int64                    `json:"id"`
	WorkspaceID memory.WorkspaceID       `json:"workspace_id"`
	Workflow    string                   `json:"workflow"`
	RunID       string                   `json:"run_id,omitempty"`
	Kind        WorkflowNotificationKind `json:"kind"`
	RunState    string                   `json:"run_state,omitempty"`
	Detail      string                   `json:"detail,omitempty"`
	Surface     string                   `json:"surface,omitempty"`
	SessionID   memory.SessionID         `json:"session_id,omitempty"`
	CreatedAt   time.Time                `json:"created_at"`
	ReadAt      *time.Time               `json:"read_at,omitempty"`
}

// WorkflowNotificationFilter narrows a listing: one Workspace, the session
// a run came from, or one run; unread only unless All; at most Limit
// (default 50, at most 200), newest first.
type WorkflowNotificationFilter struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id,omitempty"`
	SessionID   memory.SessionID   `json:"session_id,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
	All         bool               `json:"all,omitempty"`
	Limit       int                `json:"limit,omitempty"`
}

// WorkflowNotificationCounts is how many notices a Workspace (or every
// Workspace) has, and how many are unread.
type WorkflowNotificationCounts struct {
	Unread int `json:"unread"`
	Total  int `json:"total"`
}

// workflowNotificationKinds is the kind check.
const workflowNotificationKinds = `'succeeded','failed','cancelled','waiting_for_human','needs_reconciliation','paused','cap_reached','schedule_skipped','schedule_failed'`

func ensureWorkflowNotificationSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_notifications (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 dedup_key TEXT NOT NULL UNIQUE,
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL CHECK(length(workflow) BETWEEN 1 AND 64),
 run_id TEXT REFERENCES workflow_runs(id),
 kind TEXT NOT NULL CHECK(kind IN (`+workflowNotificationKinds+`)),
 run_state TEXT NOT NULL,
 detail TEXT NOT NULL,
 origin_surface TEXT NOT NULL,
 session_id TEXT,
 created_at TEXT NOT NULL,
 read_at TEXT,
 CHECK((kind IN ('schedule_skipped','schedule_failed')) = (run_id IS NULL)));
 CREATE INDEX IF NOT EXISTS workflow_notifications_by_workspace ON workflow_notifications(workspace_id, id);
 CREATE INDEX IF NOT EXISTS workflow_notifications_by_session ON workflow_notifications(session_id, id) WHERE session_id IS NOT NULL;
 CREATE INDEX IF NOT EXISTS workflow_notifications_by_run ON workflow_notifications(run_id, id) WHERE run_id IS NOT NULL;
 CREATE TRIGGER IF NOT EXISTS workflow_notifications_content BEFORE UPDATE OF id,dedup_key,workspace_id,workflow,run_id,kind,run_state,detail,origin_surface,session_id,created_at ON workflow_notifications
 BEGIN SELECT RAISE(ABORT,'workflow notifications are immutable; only their read state changes'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_notifications_no_delete BEFORE DELETE ON workflow_notifications
 BEGIN SELECT RAISE(ABORT,'workflow notifications are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_notify AFTER UPDATE OF state ON workflow_runs
 WHEN NEW.state <> OLD.state AND NEW.state IN ('succeeded','failed','cancelled','waiting_for_human','needs_reconciliation','waiting_for_dependency')
 BEGIN INSERT OR IGNORE INTO workflow_notifications(dedup_key,workspace_id,workflow,run_id,kind,run_state,detail,origin_surface,session_id,created_at,read_at)
 VALUES('run:'||NEW.id||':'||NEW.state||':'||COALESCE((SELECT MAX(seq) FROM workflow_run_checkpoints WHERE run_id=NEW.id),0)||':'||NEW.error,
 NEW.workspace_id,NEW.workflow,NEW.id,
 CASE WHEN NEW.state='waiting_for_dependency' AND substr(NEW.error,1,`+fmt.Sprint(len(WorkflowDailyCapReason))+`)='`+WorkflowDailyCapReason+`' THEN 'cap_reached'
 WHEN NEW.state='waiting_for_dependency' THEN 'paused' ELSE NEW.state END,
 NEW.state,NEW.error,NEW.origin_surface,NEW.session_id,NEW.updated_at,NULL); END;`)
	if err != nil {
		return fmt.Errorf("create workflow notification schema: %w", err)
	}
	return nil
}

// insertScheduleNotification records a schedule's notice (no run) in the
// caller's transaction.
func insertScheduleNotification(ctx context.Context, conn *sql.Conn, key string, ws memory.WorkspaceID, workflow string, kind WorkflowNotificationKind, detail string, at time.Time) error {
	_, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO workflow_notifications(dedup_key,workspace_id,workflow,run_id,kind,run_state,detail,origin_surface,session_id,created_at,read_at)
 VALUES(?,?,?,NULL,?,'',?,'schedule',NULL,?,NULL)`, key, ws, workflow, string(kind), detail, formatTime(at))
	if err != nil {
		return fmt.Errorf("record workflow notification: %w", err)
	}
	return nil
}

const notificationColumns = `id,workspace_id,workflow,run_id,kind,run_state,detail,origin_surface,session_id,created_at,read_at`

func scanWorkflowNotification(scanner rowScanner) (WorkflowNotification, error) {
	var n WorkflowNotification
	var run, session, read sql.NullString
	var created string
	if err := scanner.Scan(&n.ID, &n.WorkspaceID, &n.Workflow, &run, &n.Kind, &n.RunState, &n.Detail, &n.Surface, &session, &created, &read); err != nil {
		return n, err
	}
	n.RunID, n.SessionID = run.String, memory.SessionID(session.String)
	var err error
	if n.CreatedAt, err = parseTime(created); err != nil {
		return n, err
	}
	n.ReadAt, err = parseOptionalTime(read)
	return n, err
}

// ListWorkflowNotifications lists notices, newest first.
func (s *Store) ListWorkflowNotifications(ctx context.Context, f WorkflowNotificationFilter) ([]WorkflowNotification, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	var where []string
	var args []any
	if f.WorkspaceID != "" {
		where, args = append(where, `workspace_id=?`), append(args, f.WorkspaceID)
	}
	if f.SessionID != "" {
		where, args = append(where, `session_id=?`), append(args, f.SessionID)
	}
	if f.RunID != "" {
		where, args = append(where, `run_id=?`), append(args, f.RunID)
	}
	if !f.All {
		where = append(where, `read_at IS NULL`)
	}
	query := `SELECT ` + notificationColumns + ` FROM workflow_notifications`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowNotification{}
	for rows.Next() {
		n, err := scanWorkflowNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// MarkWorkflowNotifications sets the owner's read state of the given
// notices (a notice already in that state keeps its read time) and says
// how many exist.
func (s *Store) MarkWorkflowNotifications(ctx context.Context, ids []int64, read bool) (int, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return 0, errors.New("mark 1 to 200 workflow notifications")
	}
	marked := 0
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := formatTime(s.now())
		for _, id := range ids {
			var res sql.Result
			var err error
			if read {
				res, err = conn.ExecContext(ctx, `UPDATE workflow_notifications SET read_at=COALESCE(read_at,?) WHERE id=?`, now, id)
			} else {
				res, err = conn.ExecContext(ctx, `UPDATE workflow_notifications SET read_at=NULL WHERE id=?`, id)
			}
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			marked += int(n)
		}
		return nil
	})
	return marked, err
}

// MarkAllWorkflowNotificationsRead marks every unread notice of a
// Workspace, or of every Workspace when ws is "", read.
func (s *Store) MarkAllWorkflowNotificationsRead(ctx context.Context, ws memory.WorkspaceID) (int, error) {
	query, args := `UPDATE workflow_notifications SET read_at=? WHERE read_at IS NULL`, []any{formatTime(s.now())}
	if ws != "" {
		query, args = query+` AND workspace_id=?`, append(args, ws)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// WorkflowNotificationCounts counts a Workspace's notices, or every
// Workspace's when ws is "".
func (s *Store) WorkflowNotificationCounts(ctx context.Context, ws memory.WorkspaceID) (WorkflowNotificationCounts, error) {
	var out WorkflowNotificationCounts
	query, args := `SELECT COUNT(*),COALESCE(SUM(read_at IS NULL),0) FROM workflow_notifications`, []any{}
	if ws != "" {
		query, args = query+` WHERE workspace_id=?`, append(args, ws)
	}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&out.Total, &out.Unread)
	return out, err
}
