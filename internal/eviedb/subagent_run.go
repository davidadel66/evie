package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/google/uuid"
)

// Run-owned workers (Stage 14 sub-stage 9 of docs/memory-architecture.md,
// 2026-10-04; workflow-runtime.spec.md, subagents.spec.md, ADR 0016, 0017,
// 0019, 0025, 0027). An agent step runs an existing worker preset with the
// Workflow Run's fenced lease in place of a parent turn:
//
//   - Admission (AdmitRunSubagent) is one immediate transaction that
//     requires the run's current, unexpired lease to be the owner's (holder
//     and token), the run running, the node attempt the worker runs under
//     in flight under that lease with its worker call recorded, run
//     admission (deactivation and every block, including the Workspace's
//     worker allowance), the Workspace's allowance for the worker preset in
//     the run's pinned and current revision with its revocations (the same
//     check chat delegation uses), the worker's plugins enabled, and the
//     worker's own rules (an explore root inside the Workspace folder; a
//     build worktree in the Workspace repository, an open Task in the run's
//     Workspace, no running work on it, the Task Tree's dependency and
//     conflict rules). It reserves a hidden child session, its pinned
//     receipt and the execution, links them to the worker call, and issues
//     a build worker's Task Access Grant for this attempt.
//   - The worker holds authority (AuthorizeSubagent, every fenced child
//     write, the supervisor's watchdog) exactly while all of that still
//     holds: losing the lease, the owner cancelling the run (cancelling is
//     not running), a pause, the node attempt ending, a deactivation, a
//     block or a revocation ends it.
//   - A run's workers are children of the run's hidden worker session
//     (workflow_worker_sessions): a session whose parent is itself, so
//     like every child session it appears in no owner session list and no
//     memory path, it can never be opened as a conversation, and the
//     run's attempts share one idempotency space and one per-parent
//     capacity count.
//   - Recovery settles a run-owned attempt interrupted once its run lease
//     ended or its node attempt ended; a claim of the run settles its
//     earlier holder's workers in the claim's own transaction
//     (settleRunWorkers). Recovery never restarts a worker; the workflow,
//     whose reviewed definition authorizes the node, decides what runs
//     again.
//   - Two triggers protect this across versions: an Evie that predates
//     run-owned workers cannot rewrite their records (its recovery would
//     drop the owner), and no Evie can finish a run while one of its
//     worker calls is open, that is while a worker it started has not been
//     settled and recorded (workflow_runs_finish_after_workers, with the
//     worker call table).

const workflowWorkerSessionSchema = `CREATE TABLE IF NOT EXISTS workflow_worker_sessions (
 run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id),
 session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id),
 created_at TEXT NOT NULL);
 CREATE TRIGGER IF NOT EXISTS workflow_worker_sessions_immutable BEFORE UPDATE ON workflow_worker_sessions
 BEGIN SELECT RAISE(ABORT,'a run''s worker session is immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_worker_sessions_no_delete BEFORE DELETE ON workflow_worker_sessions
 BEGIN SELECT RAISE(ABORT,'a run''s worker session is kept'); END;
 CREATE TRIGGER IF NOT EXISTS subagent_executions_run_owner_kept BEFORE UPDATE OF record_json ON subagent_executions
 WHEN json_valid(OLD.record_json) AND json_type(OLD.record_json,'$.run') = 'object'
 AND (NOT json_valid(NEW.record_json) OR json_type(NEW.record_json,'$.run') IS NOT 'object' OR json_extract(NEW.record_json,'$.run.run_id') IS NOT json_extract(OLD.record_json,'$.run.run_id'))
 BEGIN SELECT RAISE(ABORT,'a run-owned subagent execution is written only by an Evie that runs workflow agent steps'); END;`

// ensureWorkflowWorkerSchema creates the run worker sessions and the
// triggers that protect run-owned executions; it runs after both the
// workflow run and subagent execution tables exist.
func ensureWorkflowWorkerSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, workflowWorkerSessionSchema); err != nil {
		return fmt.Errorf("create workflow worker session schema: %w", err)
	}
	return nil
}

// runWorkerSession returns the run's hidden worker session, creating it the
// first time: a session of the run's Workspace at the run's pinned
// Workspace revision, whose parent is itself.
func runWorkerSession(ctx context.Context, conn *sql.Conn, run WorkflowRun, now time.Time) (memory.Session, error) {
	session := memory.Session{WorkspaceID: run.WorkspaceID, WorkspaceRevisionSnapshot: run.WorkspaceRevision, Status: memory.SessionActive}
	var id string
	err := conn.QueryRowContext(ctx, `SELECT session_id FROM workflow_worker_sessions WHERE run_id=?`, run.ID).Scan(&id)
	switch {
	case err == nil:
		session.ID, session.ParentSessionID = memory.SessionID(id), memory.SessionID(id)
		return session, nil
	case !errors.Is(err, sql.ErrNoRows):
		return session, err
	}
	session.ID = memory.SessionID(uuid.NewString())
	session.ParentSessionID, session.CreatedAt, session.UpdatedAt = session.ID, now, now
	if _, err := conn.ExecContext(ctx, `INSERT INTO sessions(id,workspace_id,workspace_revision_snapshot,parent_session_id,title,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		session.ID, run.WorkspaceID, run.WorkspaceRevision, session.ID, "workers of workflow run "+run.ID, memory.SessionActive,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		return session, fmt.Errorf("create the run's worker session: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_worker_sessions(run_id,session_id,created_at) VALUES(?,?,?)`, run.ID, session.ID, formatTime(now)); err != nil {
		return session, fmt.Errorf("record the run's worker session: %w", err)
	}
	return session, nil
}

// runParent is the parent identity of a run-owned attempt: the run's
// worker session as its scope, with no turn lease.
func runParent(session memory.Session) delegation.Parent {
	return delegation.Parent{Scope: session.ScopeContext()}
}

// authorityRefused is a run-owned worker's refusal: delegation.ErrAuthority
// with why.
func authorityRefused(format string, args ...any) error {
	return fmt.Errorf("%w: %s", delegation.ErrAuthority, fmt.Sprintf(format, args...))
}

// authorizeRunOwner proves a run-owned worker's authority inside a
// transaction (see the package note above). Every refusal wraps
// delegation.ErrAuthority.
func authorizeRunOwner(ctx context.Context, conn *sql.Conn, o delegation.RunOwner, worker string, now time.Time) (WorkflowRun, error) {
	if err := fenceWorkflowRun(ctx, conn, WorkflowRunLease{RunID: o.RunID, Holder: o.Holder, Token: o.Token}, now); err != nil {
		return WorkflowRun{}, fmt.Errorf("%w: %w", delegation.ErrAuthority, err)
	}
	run, err := readWorkflowRun(ctx, conn, o.RunID)
	if err != nil {
		return run, err
	}
	switch {
	case run.WorkspaceID != o.WorkspaceID || run.Workflow != o.Workflow:
		return run, authorityRefused("workflow run %s is not a run of %s in Workspace %s", run.ID, o.Workflow, o.WorkspaceID)
	case run.State != WorkflowRunRunning:
		return run, authorityRefused("workflow run %s is %s", run.ID, run.State)
	}
	var state, step string
	var token int64
	var item sql.NullInt64
	err = conn.QueryRowContext(ctx, `SELECT state,step_id,lease_token,item_index FROM workflow_node_attempts WHERE run_id=? AND seq=?`, o.RunID, o.AttemptSeq).
		Scan(&state, &step, &token, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return run, authorityRefused("workflow run %s has no attempt %d", run.ID, o.AttemptSeq)
	}
	if err != nil {
		return run, err
	}
	sameItem := (o.Item == nil) == !item.Valid && (o.Item == nil || int64(*o.Item) == item.Int64)
	if state != string(WorkflowAttemptRunning) || token != o.Token || step != o.Step || !sameItem {
		return run, authorityRefused("its attempt %d of steps.%s is no longer in flight under lease %d", o.AttemptSeq, o.Step, o.Token)
	}
	if err := authorizeWorkflowRun(ctx, conn, run); err != nil {
		return run, fmt.Errorf("%w: %w", delegation.ErrAuthority, err)
	}
	scope := memory.ScopeContext{OwnerID: memory.LocalOwnerID, WorkspaceID: run.WorkspaceID, WorkspaceRevision: run.WorkspaceRevision}
	if err := authorizeWorkspacePreset(ctx, conn, scope, worker); err != nil {
		return run, err
	}
	return run, requireWorkerPlugins(ctx, conn, worker)
}

// runOwnerLive reports whether a run-owned attempt's owner can still hold
// it: its lease is the run's current, unexpired lease and its node attempt
// is in flight under it. Recovery settles an attempt whose owner is gone.
func runOwnerLive(ctx context.Context, conn *sql.Conn, o delegation.RunOwner, now time.Time) (bool, error) {
	err := fenceWorkflowRun(ctx, conn, WorkflowRunLease{RunID: o.RunID, Holder: o.Holder, Token: o.Token}, now)
	if errors.Is(err, ErrWorkflowRunLeaseLost) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var running bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_node_attempts WHERE run_id=? AND seq=? AND state='running' AND lease_token=?)`,
		o.RunID, o.AttemptSeq, o.Token).Scan(&running); err != nil {
		return false, err
	}
	return running, nil
}

// authorizeAttemptOwner proves the authority an attempt runs under: its
// Workflow Run's for a run-owned attempt, its parent turn's otherwise.
func (s *Store) authorizeAttemptOwner(ctx context.Context, conn *sql.Conn, a delegation.Attempt, fence subagentLeaseFence) error {
	if a.Run != nil {
		_, err := authorizeRunOwner(ctx, conn, *a.Run, a.Worker(), s.now().UTC())
		return err
	}
	return s.authorizeSubagentParentWith(ctx, conn, a.Parent, a.Receipt, fence)
}

// authorizeRunTask requires a run's build Task to exist in the run's
// Workspace or the global scope.
func authorizeRunTask(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, id string) error {
	var scope task.Scope
	err := conn.QueryRowContext(ctx, `SELECT scope FROM tasks WHERE id=?`, id).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return &task.NotFoundError{ID: task.ID(id)}
	}
	if err != nil {
		return err
	}
	if scope != task.ScopeGlobal && scope != task.WorkspaceScope(string(ws)) {
		return fmt.Errorf("%w: Task %s is outside the run's Workspace", delegation.ErrAuthority, id)
	}
	return nil
}

// RunWorkerPluginsEnabled reports, changing nothing, whether the plugins a
// worker uses are enabled: what a run checks before an agent step starts.
func (s *Store) RunWorkerPluginsEnabled(ctx context.Context, worker string) error {
	return s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error { return requireWorkerPlugins(ctx, conn, worker) })
}

// AdmitRunSubagent admits one run-owned worker attempt under its owner (see
// the package note above), or returns the attempt already admitted for the
// same node attempt. root is an explore worker's resolved folder, plan a
// fresh build worker's planned worktree; a continuation (ContinueFrom)
// takes the interrupted build attempt's child session, worktree and pinned
// receipt instead of receipt.
func (s *Store) AdmitRunSubagent(ctx context.Context, req delegation.RunAssignment, receipt composition.Receipt, policy delegation.Policy, root string, plan *delegation.Worktree, dispatchID string) (delegation.Attempt, error) {
	o := req.Owner
	switch {
	case dispatchID == "":
		return delegation.Attempt{}, errors.New("subagent dispatch identity is required")
	case req.Worker != delegation.WorkerResearch && req.Worker != delegation.WorkerExplore && req.Worker != delegation.WorkerBuild:
		return delegation.Attempt{}, fmt.Errorf("unknown worker preset %q", req.Worker)
	case receipt.Preset.ID != req.Worker:
		return delegation.Attempt{}, fmt.Errorf("the %s worker's receipt is preset %q", req.Worker, receipt.Preset.ID)
	case o.RunID == "" || o.Holder == "" || o.Token <= 0 || o.AttemptSeq <= 0 || o.Step == "" || o.WorkspaceID == "":
		return delegation.Attempt{}, errors.New("a run-owned worker needs its run, lease and node attempt")
	case req.Worker == delegation.WorkerBuild && req.TaskID == "":
		return delegation.Attempt{}, errors.New("a build worker needs the Task it works on")
	case req.ContinueFrom != "" && req.Worker != delegation.WorkerBuild:
		return delegation.Attempt{}, errors.New("only a build worker resumes an interrupted attempt")
	}
	if err := policy.Validate(); err != nil {
		return delegation.Attempt{}, err
	}
	encoded, err := composition.Marshal(receipt)
	if err != nil {
		return delegation.Attempt{}, err
	}
	key := delegation.RunKey(o)
	assignment := delegation.Assignment{Key: key, Objective: req.Objective, Context: req.Data, TaskID: req.TaskID}
	digest := delegation.Digest(struct {
		Assignment   delegation.Assignment
		Worker       string
		ContinueFrom string
	}{assignment, req.Worker, req.ContinueFrom})
	var a delegation.Attempt
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		run, err := authorizeRunOwner(ctx, conn, o, req.Worker, now)
		if err != nil {
			return err
		}
		anchor, err := runWorkerSession(ctx, conn, run, now)
		if err != nil {
			return err
		}
		retained, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE parent_session_id=? AND idempotency_key=?`, anchor.ID, key))
		if err == nil {
			if retained.Digest != digest || retained.Worker() != req.Worker {
				return fmt.Errorf("%w: attempt %d of workflow run %s already admitted execution %s with a different assignment", delegation.ErrConflict, o.AttemptSeq, o.RunID, retained.ID)
			}
			a = retained
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		parent := runParent(anchor)
		id := uuid.NewString()
		var child memory.Session
		var worktree *delegation.Worktree
		var continues *delegation.Continues
		switch req.Worker {
		case delegation.WorkerExplore:
			if err := authorizeExploreRoot(ctx, conn, parent.Scope, root); err != nil {
				return err
			}
		case delegation.WorkerBuild:
			if err := authorizeRunTask(ctx, conn, run.WorkspaceID, req.TaskID); err != nil {
				return err
			}
			if req.ContinueFrom != "" {
				prior, err := resumableRunBuild(ctx, conn, o, req, anchor.ID)
				if err != nil {
					return err
				}
				if err := authorizeBuildWorktree(ctx, conn, parent.Scope, *prior.Worktree); err != nil {
					return err
				}
				if err := checkContinuationConflicts(ctx, conn, prior, now); err != nil {
					return err
				}
				var after int64
				if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, prior.Child.ID).Scan(&after); err != nil {
					return err
				}
				child, worktree, receipt = prior.Child, prior.Worktree, prior.Receipt
				continues = &delegation.Continues{ExecutionID: prior.ID, AfterSequence: after}
				break
			}
			if plan == nil {
				return errors.New("a fresh build worker needs its planned worktree")
			}
			if err := authorizeBuildWorktree(ctx, conn, parent.Scope, *plan); err != nil {
				return err
			}
			if err := prepareBuildTask(ctx, conn, req.TaskID, now); err != nil {
				return err
			}
			// Task Tree rules (Stage 13), conservatively: dependencies done
			// and no overlap with running builds in the repository.
			if err := checkBuildOrchestration(ctx, conn, task.ID(req.TaskID), *plan, nil, true, now); err != nil {
				return err
			}
			planned := *plan
			id, worktree = filepath.Base(planned.Path), &planned
		}
		if child.ID == "" {
			child = memory.Session{ID: memory.SessionID(uuid.NewString()), ParentSessionID: anchor.ID, Status: memory.SessionActive, CreatedAt: now, UpdatedAt: now,
				WorkspaceID: run.WorkspaceID, WorkspaceRevisionSnapshot: run.WorkspaceRevision}
			if _, err := conn.ExecContext(ctx, `INSERT INTO sessions(id,workspace_id,workspace_revision_snapshot,parent_session_id,status,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
				child.ID, child.WorkspaceID, child.WorkspaceRevisionSnapshot, child.ParentSessionID, child.Status, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				return err
			}
			if err := insertCompositionReceipt(ctx, conn, child.ID, encoded, now); err != nil {
				return err
			}
		}
		owner := o
		a = delegation.Attempt{ID: id, DispatchID: dispatchID, Parent: parent, Assignment: assignment, Digest: digest, Child: child, Receipt: receipt, Policy: policy,
			PolicyID: delegation.Digest(policy), State: "admitted", CreatedAt: now, Root: root, Worktree: worktree, Continues: continues, Run: &owner}
		data, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
			a.ID, anchor.ID, key, digest, child.ID, a.State, string(data)); err != nil {
			return err
		}
		if req.Worker == delegation.WorkerBuild {
			// The worker's authority over its Task: that Task's subtree, for
			// this attempt only (Stage 11).
			if err := issueBuildGrant(ctx, conn, parent, child.ID, req.TaskID, a.ID, now); err != nil {
				return err
			}
		}
		// The attempt's worker call names the execution it ran as.
		res, err := conn.ExecContext(ctx, `UPDATE workflow_worker_calls SET execution_id=?,child_session_id=? WHERE run_id=? AND attempt_seq=? AND execution_id IS NULL AND finished_at IS NULL AND lease_token=?`,
			a.ID, child.ID, o.RunID, o.AttemptSeq, o.Token)
		if err != nil {
			return fmt.Errorf("link the worker call: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			if err != nil {
				return err
			}
			return authorityRefused("attempt %d of workflow run %s has no open worker call to run under", o.AttemptSeq, o.RunID)
		}
		_, err = authorizeRunOwner(ctx, conn, o, req.Worker, s.now().UTC())
		return err
	})
	return a, err
}

// resumableRunBuild reads the interrupted build attempt a run-owned
// continuation resumes: an attempt of the same run, step and item, under
// the run's worker session, that started, ended interrupted or cancelled
// without being merged, and is its child's latest, with its worktree kept.
func resumableRunBuild(ctx context.Context, conn *sql.Conn, o delegation.RunOwner, req delegation.RunAssignment, anchor memory.SessionID) (delegation.Attempt, error) {
	prior, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=? AND parent_session_id=?`, req.ContinueFrom, anchor))
	if errors.Is(err, sql.ErrNoRows) {
		return prior, fmt.Errorf("%w: %q", delegation.ErrNotFound, req.ContinueFrom)
	}
	if err != nil {
		return prior, err
	}
	sameItem := (prior.Run != nil) && (prior.Run.Item == nil) == (o.Item == nil) && (o.Item == nil || *prior.Run.Item == *o.Item)
	switch {
	case prior.Run == nil || prior.Run.RunID != o.RunID || prior.Run.Step != o.Step || !sameItem:
		return prior, fmt.Errorf("%w: execution %s is not an attempt of this step", delegation.ErrNotResumable, prior.ID)
	case prior.Worker() != delegation.WorkerBuild || prior.Worktree == nil || prior.Worktree.RemovedAt != nil || prior.StartedAt == nil:
		return prior, fmt.Errorf("%w: execution %s has no kept worktree to resume", delegation.ErrNotResumable, prior.ID)
	case prior.State != "interrupted" && prior.State != "cancelled":
		return prior, fmt.Errorf("%w: execution %s ended %s, not interrupted", delegation.ErrNotResumable, prior.ID, prior.State)
	case prior.Assignment.TaskID != req.TaskID:
		return prior, fmt.Errorf("%w: execution %s worked on Task %s, not %s", delegation.ErrNotResumable, prior.ID, prior.Assignment.TaskID, req.TaskID)
	}
	var latest string
	if err := conn.QueryRowContext(ctx, `SELECT id FROM subagent_executions WHERE child_session_id=? ORDER BY rowid DESC LIMIT 1`, prior.Child.ID).Scan(&latest); err != nil {
		return prior, err
	}
	if latest != prior.ID {
		return prior, fmt.Errorf("%w: execution %s was already resumed by %s", delegation.ErrNotResumable, prior.ID, latest)
	}
	return prior, nil
}

// RunWorkerExecution reads one run-owned execution of a run and its full
// stored report (empty without one).
func (s *Store) RunWorkerExecution(ctx context.Context, runID, id string) (delegation.Attempt, string, error) {
	a, err := readSubagent(s.db.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, "", fmt.Errorf("%w: %q", delegation.ErrNotFound, id)
	}
	if err != nil {
		return a, "", err
	}
	if a.Run == nil || a.Run.RunID != runID {
		return delegation.Attempt{}, "", fmt.Errorf("%w: %q", delegation.ErrNotFound, id)
	}
	if a.FinalEventID == "" {
		return a, "", nil
	}
	var report string
	err = s.db.QueryRowContext(ctx, `SELECT content FROM events WHERE id=? AND session_id=?`, a.FinalEventID, a.Child.ID).Scan(&report)
	return a, report, err
}
