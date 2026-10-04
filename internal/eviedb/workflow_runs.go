package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// Workflow Runs (Stage 14 sub-stage 3 of docs/memory-architecture.md,
// 2026-10-04; workflow-runtime.spec.md, ADR 0007, 0016, 0017, 0021, 0025).
// SQLite is authoritative for what has happened:
//
//   - workflow_runs: one row per run, pinned at start to the approved
//     version, its commit, both hashes, the Workspace revision and access
//     revision it started under, its Execution Composition, Run Key and
//     accepted input. Pinned fields never change, and a terminal run is
//     never rewritten. At most one run per Workflow and Run Key may be
//     active or succeeded (a partial unique index), so a logical execution
//     cannot silently happen twice; failed and cancelled runs leave the key
//     free.
//   - workflow_run_checkpoints: the durable boundary between nodes. Seq 0
//     is the accepted input with the first step; each succeeded attempt
//     appends the next one (the step to run next, "" when the graph ended,
//     and every step output so far). Immutable.
//   - workflow_node_attempts: one row per node attempt, recorded (with its
//     step, attempt number, input checkpoint and resolved input) before the
//     node runs, and finished once with its normalized output or error, in
//     the same transaction as the next checkpoint. Only one attempt of a run
//     may be in flight, and a finished attempt is immutable.
//
// Every write is a compare-and-set on the run's latest checkpoint and, since
// sub-stage 4, fenced by the run's lease (workflow_run_leases.go): only the
// runtime holding the current, unexpired lease can begin or finish an
// attempt, write a checkpoint or fail the run.

var (
	ErrWorkflowRunNotFound = errors.New("workflow run not found")
	ErrWorkflowRunConflict = errors.New("the workflow run moved on: its checkpoint or attempt changed")
	ErrWorkflowRunFinished = errors.New("the workflow run has finished")
	ErrWorkflowRunSurface  = errors.New("only the owner starts workflow runs, from the CLI or the web")
	ErrWorkflowRunSession  = errors.New("a run's originating session must be a session of its Workspace")
)

// WorkflowRunState is a run's lifecycle state. The states are explicit and
// non-overlapping; sub-stage 3 reaches queued, running, failed and
// succeeded, and later sub-stages use the rest without a schema change.
type WorkflowRunState string

const (
	WorkflowRunQueued               WorkflowRunState = "queued"
	WorkflowRunRunning              WorkflowRunState = "running"
	WorkflowRunWaitingForHuman      WorkflowRunState = "waiting_for_human"
	WorkflowRunWaitingForDependency WorkflowRunState = "waiting_for_dependency"
	WorkflowRunNeedsReconciliation  WorkflowRunState = "needs_reconciliation"
	WorkflowRunCancelling           WorkflowRunState = "cancelling"
	WorkflowRunCancelled            WorkflowRunState = "cancelled"
	WorkflowRunFailed               WorkflowRunState = "failed"
	WorkflowRunSucceeded            WorkflowRunState = "succeeded"
)

// Terminal reports a state whose history is final.
func (s WorkflowRunState) Terminal() bool {
	return s == WorkflowRunSucceeded || s == WorkflowRunFailed || s == WorkflowRunCancelled
}

// WorkflowAttemptState is one node attempt's state. An attempt is
// interrupted when the runtime holding the run stopped before recording its
// outcome (a crash, a lost lease or a failed write); the next holder records
// it so before anything else happens (sub-stage 4).
type WorkflowAttemptState string

const (
	WorkflowAttemptRunning     WorkflowAttemptState = "running"
	WorkflowAttemptSucceeded   WorkflowAttemptState = "succeeded"
	WorkflowAttemptFailed      WorkflowAttemptState = "failed"
	WorkflowAttemptInterrupted WorkflowAttemptState = "interrupted"
)

// WorkflowRun is one run with its pins, input and progress.
type WorkflowRun struct {
	ID                string                     `json:"id"`
	WorkspaceID       memory.WorkspaceID         `json:"workspace_id"`
	Workflow          string                     `json:"workflow"`
	Version           int64                      `json:"version"`
	Commit            string                     `json:"commit"`
	Path              string                     `json:"path"`
	ExecutableHash    string                     `json:"executable_hash"`
	AuthorityHash     string                     `json:"authority_hash"`
	WorkspaceRevision memory.WorkspaceRevisionID `json:"workspace_revision"`
	AccessRevision    int64                      `json:"access_revision"`
	Composition       json.RawMessage            `json:"composition"`
	RunKey            string                     `json:"run_key"`
	Inputs            json.RawMessage            `json:"inputs"`
	State             WorkflowRunState           `json:"state"`
	Outputs           json.RawMessage            `json:"outputs,omitempty"`
	Error             string                     `json:"error,omitempty"`
	Surface           string                     `json:"surface"`
	SessionID         memory.SessionID           `json:"session_id,omitempty"`
	// Checkpoint is the latest checkpoint's sequence number and NextStep
	// the step it names ("" once the graph ended).
	Checkpoint int64      `json:"checkpoint"`
	NextStep   string     `json:"next_step,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	// NotBefore is the durable time before which no runtime advances the
	// run: a retry's backoff, or a paused run's next check. While the run
	// waits for a dependency, Error says why.
	NotBefore *time.Time `json:"not_before,omitempty"`
}

// WorkflowNodeAttempt is one node attempt.
type WorkflowNodeAttempt struct {
	RunID           string               `json:"run_id"`
	Seq             int64                `json:"seq"`
	Step            string               `json:"step"`
	Kind            string               `json:"kind"`
	Attempt         int                  `json:"attempt"`
	InputCheckpoint int64                `json:"input_checkpoint"`
	Input           json.RawMessage      `json:"input"`
	State           WorkflowAttemptState `json:"state"`
	Output          json.RawMessage      `json:"output,omitempty"`
	Error           string               `json:"error,omitempty"`
	Next            string               `json:"next,omitempty"`
	Note            string               `json:"note,omitempty"`
	StartedAt       time.Time            `json:"started_at"`
	FinishedAt      *time.Time           `json:"finished_at,omitempty"`
	// LeaseHolder and LeaseToken are the lease the attempt ran under (empty
	// and 0 for attempts recorded before leases existed).
	LeaseHolder string `json:"lease_holder,omitempty"`
	LeaseToken  int64  `json:"lease_token,omitempty"`
}

// WorkflowCheckpoint is the durable state between nodes.
type WorkflowCheckpoint struct {
	RunID string `json:"run_id"`
	Seq   int64  `json:"seq"`
	// AfterAttempt is the attempt that produced it; 0 for the accepted input.
	AfterAttempt int64 `json:"after_attempt,omitempty"`
	// Next is the step to run next; "" when the graph ended.
	Next string `json:"next"`
	// State holds every step output so far, by step ID.
	State     json.RawMessage `json:"state"`
	CreatedAt time.Time       `json:"created_at"`
}

// WorkflowRunDetail is a run with its full attempt and checkpoint history,
// its lease (nil before any runtime claimed it), its recovery events and
// the compatible provider substitutions the owner approved for it.
type WorkflowRunDetail struct {
	Run           WorkflowRun               `json:"run"`
	Attempts      []WorkflowNodeAttempt     `json:"attempts"`
	Checkpoints   []WorkflowCheckpoint      `json:"checkpoints"`
	Lease         *WorkflowRunLease         `json:"lease,omitempty"`
	Events        []WorkflowRunEvent        `json:"events"`
	Substitutions []WorkflowRunSubstitution `json:"substitutions"`
}

// WorkflowRunProgress is what an executor needs to take the next step.
type WorkflowRunProgress struct {
	Run        WorkflowRun
	Checkpoint WorkflowCheckpoint
	// Made is every attempt of the checkpoint's next step from this
	// checkpoint (attempts are numbered by it). Attempts are those counted
	// against the step's Retry Policy and Interrupted those a stopped
	// runtime left without an outcome, which are not (sub-stage 5: a rerun
	// from the same checkpoint is safe; the runtime's interruption bound
	// counts them instead). LastError is the latest attempt's error.
	Made        int
	Attempts    int
	Interrupted int
	LastError   string
	// Counted is every attempt counted against limits.max_attempts
	// (neither branch decisions nor interrupted attempts are counted).
	Counted int
	// InFlight is an attempt recorded but not finished.
	InFlight *WorkflowNodeAttempt
}

// workflowNodeAttemptsTable is the node attempt table since sub-stage 4:
// an attempt records the lease it ran under, and a crash leaves it
// interrupted rather than failed.
const workflowNodeAttemptsTable = `CREATE TABLE IF NOT EXISTS workflow_node_attempts (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 kind TEXT NOT NULL CHECK(kind IN ('deterministic','capability','ai','agent','map','branch','interrupt')),
 attempt INTEGER NOT NULL CHECK(attempt > 0),
 input_checkpoint INTEGER NOT NULL,
 input TEXT NOT NULL CHECK(json_valid(input)),
 state TEXT NOT NULL CHECK(state IN ('running','succeeded','failed','interrupted')),
 output TEXT CHECK(output IS NULL OR json_valid(output)),
 error TEXT NOT NULL,
 next_step TEXT NOT NULL,
 note TEXT NOT NULL,
 started_at TEXT NOT NULL,
 finished_at TEXT,
 lease_holder TEXT NOT NULL,
 lease_token INTEGER NOT NULL CHECK(lease_token >= 0),
 PRIMARY KEY(run_id, seq),
 FOREIGN KEY(run_id, input_checkpoint) REFERENCES workflow_run_checkpoints(run_id, seq),
 CHECK((state = 'running') = (finished_at IS NULL)),
 CHECK(state <> 'succeeded' OR output IS NOT NULL),
 CHECK(state NOT IN ('failed','interrupted') OR (error <> '' AND output IS NULL)))`

const workflowNodeAttemptsIndexes = `CREATE UNIQUE INDEX IF NOT EXISTS workflow_node_attempts_one_in_flight ON workflow_node_attempts(run_id) WHERE state = 'running';
 CREATE UNIQUE INDEX IF NOT EXISTS workflow_node_attempts_numbered ON workflow_node_attempts(run_id, input_checkpoint, step_id, attempt);
 CREATE TRIGGER IF NOT EXISTS workflow_node_attempts_finished BEFORE UPDATE ON workflow_node_attempts
 WHEN OLD.state <> 'running'
 BEGIN SELECT RAISE(ABORT,'finished workflow node attempts are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_node_attempts_identity BEFORE UPDATE OF run_id,seq,step_id,kind,attempt,input_checkpoint,input,started_at,lease_holder,lease_token ON workflow_node_attempts
 BEGIN SELECT RAISE(ABORT,'a workflow node attempt''s identity is immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_node_attempts_no_delete BEFORE DELETE ON workflow_node_attempts
 BEGIN SELECT RAISE(ABORT,'workflow node attempts are immutable'); END;`

// subStage3Attempts identifies the sub-stage 3 attempt table: no lease
// columns and no interrupted state.
const subStage3Attempts = `state TEXT NOT NULL CHECK(state IN ('running','succeeded','failed'))`

func ensureWorkflowRunSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_runs (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 workflow TEXT NOT NULL CHECK(length(workflow) BETWEEN 1 AND 64),
 version INTEGER NOT NULL CHECK(version > 0),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 executable_hash TEXT NOT NULL,
 authority_hash TEXT NOT NULL,
 workspace_revision_id TEXT NOT NULL,
 access_revision INTEGER NOT NULL CHECK(access_revision >= 0),
 composition TEXT NOT NULL CHECK(json_valid(composition) AND json_type(composition) = 'object'),
 run_key TEXT NOT NULL CHECK(length(run_key) BETWEEN 1 AND 256),
 inputs TEXT NOT NULL CHECK(json_valid(inputs) AND json_type(inputs) = 'object'),
 state TEXT NOT NULL CHECK(state IN ('queued','running','waiting_for_human','waiting_for_dependency','needs_reconciliation','cancelling','cancelled','failed','succeeded')),
 outputs TEXT CHECK(outputs IS NULL OR (json_valid(outputs) AND json_type(outputs) = 'object')),
 error TEXT NOT NULL,
 origin_surface TEXT NOT NULL CHECK(origin_surface IN ('cli','web')),
 session_id TEXT REFERENCES sessions(id),
 created_at TEXT NOT NULL,
 started_at TEXT,
 finished_at TEXT,
 updated_at TEXT NOT NULL,
 not_before TEXT,
 FOREIGN KEY(workspace_id, workflow, version) REFERENCES workflow_activations(workspace_id, name, version),
 CHECK((state IN ('succeeded','failed','cancelled')) = (finished_at IS NOT NULL)),
 CHECK((state = 'succeeded') = (outputs IS NOT NULL)),
 CHECK(state <> 'failed' OR error <> ''));
 CREATE UNIQUE INDEX IF NOT EXISTS workflow_runs_one_per_key ON workflow_runs(workspace_id, workflow, run_key)
 WHERE state NOT IN ('failed','cancelled');
 CREATE INDEX IF NOT EXISTS workflow_runs_by_workflow ON workflow_runs(workspace_id, workflow);
 CREATE TRIGGER IF NOT EXISTS workflow_runs_no_delete BEFORE DELETE ON workflow_runs
 BEGIN SELECT RAISE(ABORT,'workflow runs are immutable history'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_terminal BEFORE UPDATE ON workflow_runs
 WHEN OLD.state IN ('succeeded','failed','cancelled')
 BEGIN SELECT RAISE(ABORT,'terminal workflow runs are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_pinned BEFORE UPDATE OF id,workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,
 workspace_revision_id,access_revision,composition,run_key,inputs,origin_surface,session_id,created_at ON workflow_runs
 BEGIN SELECT RAISE(ABORT,'a workflow run''s pinned fields are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_no_requeue BEFORE UPDATE OF state ON workflow_runs
 WHEN NEW.state = 'queued' AND OLD.state <> 'queued'
 BEGIN SELECT RAISE(ABORT,'a started workflow run cannot be queued again'); END;
 CREATE TABLE IF NOT EXISTS workflow_run_checkpoints (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq >= 0),
 after_attempt INTEGER,
 next_step TEXT NOT NULL,
 state TEXT NOT NULL CHECK(json_valid(state) AND json_type(state) = 'object'),
 created_at TEXT NOT NULL,
 PRIMARY KEY(run_id, seq),
 CHECK((seq = 0) = (after_attempt IS NULL)));
 CREATE TRIGGER IF NOT EXISTS workflow_run_checkpoints_no_update BEFORE UPDATE ON workflow_run_checkpoints
 BEGIN SELECT RAISE(ABORT,'workflow checkpoints are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_checkpoints_no_delete BEFORE DELETE ON workflow_run_checkpoints
 BEGIN SELECT RAISE(ABORT,'workflow checkpoints are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow run schema: %w", err)
	}
	// Sub-stage 4 adds the durable retry time to runs and the lease and
	// interrupted state to attempts. A sub-stage 3 database is upgraded once,
	// under the write lock, so competing startups migrate it once; an
	// unknown shape fails closed.
	err = withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		var hasNotBefore bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info('workflow_runs') WHERE name='not_before')`).Scan(&hasNotBefore); err != nil {
			return err
		}
		if !hasNotBefore {
			if _, err := conn.ExecContext(ctx, `ALTER TABLE workflow_runs ADD COLUMN not_before TEXT`); err != nil {
				return fmt.Errorf("add the workflow run retry time: %w", err)
			}
		}
		var definition string
		err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_node_attempts'`).Scan(&definition)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			_, err = conn.ExecContext(ctx, workflowNodeAttemptsTable)
			return err
		case err != nil:
			return err
		case strings.Contains(definition, "lease_token") && strings.Contains(definition, "'interrupted'"):
			return nil
		case !strings.Contains(definition, subStage3Attempts):
			return errors.New("unsupported workflow node attempt table schema")
		}
		var references int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master AS m JOIN pragma_foreign_key_list(m.name) AS f WHERE m.type='table' AND f."table"='workflow_node_attempts'`).Scan(&references); err != nil {
			return err
		}
		if references != 0 {
			return errors.New("unsupported workflow node attempt foreign reference")
		}
		replacement := strings.Replace(workflowNodeAttemptsTable, "IF NOT EXISTS workflow_node_attempts", "workflow_node_attempts_leased", 1)
		if _, err := conn.ExecContext(ctx, replacement); err != nil {
			return fmt.Errorf("create workflow node attempt table: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_node_attempts_leased(rowid,run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at,lease_holder,lease_token)
 SELECT rowid,run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at,'',0 FROM workflow_node_attempts;
 DROP TABLE workflow_node_attempts;
 ALTER TABLE workflow_node_attempts_leased RENAME TO workflow_node_attempts;`); err != nil {
			return fmt.Errorf("migrate workflow node attempts: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("upgrade workflow run schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, workflowNodeAttemptsIndexes); err != nil {
		return fmt.Errorf("create workflow node attempt indexes: %w", err)
	}
	if err := ensureWorkflowRunLeaseSchema(ctx, db); err != nil {
		return err
	}
	return ensureWorkflowRunSubstitutionSchema(ctx, db)
}

// WorkflowRunStart is a run the runtime has validated: the exact approved
// version it read from procedural Git and re-hashed, the accepted input,
// its Run Key and Execution Composition, and where it was started.
type WorkflowRunStart struct {
	WorkspaceID    memory.WorkspaceID
	Workflow       string
	Version        int64
	ExecutableHash string
	AuthorityHash  string
	Composition    json.RawMessage
	RunKey         string
	Inputs         json.RawMessage
	// Start is the first step, named by the initial checkpoint.
	Start     string
	Surface   string
	SessionID memory.SessionID
}

const runColumns = `r.id,r.workspace_id,r.workflow,r.version,r.commit_id,r.tree_path,r.executable_hash,r.authority_hash,r.workspace_revision_id,r.access_revision,
 r.composition,r.run_key,r.inputs,r.state,r.outputs,r.error,r.origin_surface,r.session_id,r.created_at,r.started_at,r.finished_at,r.updated_at,
 r.not_before,c.seq,c.next_step`

const runFrom = ` FROM workflow_runs r JOIN workflow_run_checkpoints c ON c.run_id=r.id
 AND c.seq=(SELECT MAX(seq) FROM workflow_run_checkpoints WHERE run_id=r.id)`

func parseOptionalTime(text sql.NullString) (*time.Time, error) {
	if !text.Valid {
		return nil, nil
	}
	t, err := parseTime(text.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanWorkflowRun(scanner rowScanner) (WorkflowRun, error) {
	var r WorkflowRun
	var composition, inputs, created, updated string
	var outputs, session, started, finished, notBefore sql.NullString
	if err := scanner.Scan(&r.ID, &r.WorkspaceID, &r.Workflow, &r.Version, &r.Commit, &r.Path, &r.ExecutableHash, &r.AuthorityHash, &r.WorkspaceRevision, &r.AccessRevision,
		&composition, &r.RunKey, &inputs, &r.State, &outputs, &r.Error, &r.Surface, &session, &created, &started, &finished, &updated,
		&notBefore, &r.Checkpoint, &r.NextStep); err != nil {
		return r, err
	}
	if notBefore.Valid {
		t, err := parseLeaseTime(notBefore.String)
		if err != nil {
			return r, err
		}
		r.NotBefore = &t
	}
	r.Composition, r.Inputs = json.RawMessage(composition), json.RawMessage(inputs)
	if outputs.Valid {
		r.Outputs = json.RawMessage(outputs.String)
	}
	r.SessionID = memory.SessionID(session.String)
	var err error
	if r.CreatedAt, err = parseTime(created); err != nil {
		return r, err
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return r, err
	}
	if r.StartedAt, err = parseOptionalTime(started); err != nil {
		return r, err
	}
	r.FinishedAt, err = parseOptionalTime(finished)
	return r, err
}

func readWorkflowRun(ctx context.Context, q queryRower, id string) (WorkflowRun, error) {
	r, err := scanWorkflowRun(q.QueryRowContext(ctx, `SELECT `+runColumns+runFrom+` WHERE r.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("%w: %s", ErrWorkflowRunNotFound, id)
	}
	return r, err
}

func workflowJSONObject(data json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(data, &m) == nil && m != nil
}

// StartWorkflowRun records a new run, or returns the run that already holds
// its Run Key (existing is then true and nothing is recorded). In one
// transaction it applies run admission (authorizeWorkflowActivation: the
// definition's latest version, not deactivated and not blocked), requires
// that version and both hashes to be the ones the runtime validated, checks
// the originating session, and pins the current Workspace revision and
// access revision with the first checkpoint.
func (s *Store) StartWorkflowRun(ctx context.Context, req WorkflowRunStart) (run WorkflowRun, existing bool, err error) {
	if req.Surface != "cli" && req.Surface != "web" {
		return WorkflowRun{}, false, ErrWorkflowRunSurface
	}
	if !workflowJSONObject(req.Inputs) || !workflowJSONObject(req.Composition) || req.RunKey == "" || req.Start == "" {
		return WorkflowRun{}, false, errors.New("a workflow run needs its accepted input and composition as JSON objects, a Run Key and a first step")
	}
	id := uuid.NewString()
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		a, err := authorizeWorkflowActivation(ctx, conn, req.WorkspaceID, req.Workflow)
		if err != nil {
			return err
		}
		if a.Version != req.Version || a.ExecutableHash != req.ExecutableHash || a.AuthorityHash != req.AuthorityHash {
			return ErrWorkflowChanged
		}
		var held string
		err = conn.QueryRowContext(ctx, `SELECT id FROM workflow_runs WHERE workspace_id=? AND workflow=? AND run_key=? AND state NOT IN ('failed','cancelled')`,
			req.WorkspaceID, req.Workflow, req.RunKey).Scan(&held)
		switch {
		case err == nil:
			run, existing = WorkflowRun{ID: held}, true
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if req.SessionID != "" {
			var sessionWorkspace sql.NullString
			err := conn.QueryRowContext(ctx, `SELECT workspace_id FROM sessions WHERE id=?`, req.SessionID).Scan(&sessionWorkspace)
			if errors.Is(err, sql.ErrNoRows) || err == nil && sessionWorkspace.String != string(req.WorkspaceID) {
				return ErrWorkflowRunSession
			}
			if err != nil {
				return err
			}
		}
		w, err := readWorkspaceRow(ctx, conn, req.WorkspaceID)
		if err != nil {
			return err
		}
		access, err := workspaceAccess(ctx, conn, req.WorkspaceID)
		if err != nil {
			return err
		}
		now := formatTime(s.now())
		var session any
		if req.SessionID != "" {
			session = string(req.SessionID)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_runs(id,workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,
 composition,run_key,inputs,state,outputs,error,origin_surface,session_id,created_at,started_at,finished_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'queued',NULL,'',?,?,?,NULL,NULL,?)`, id, req.WorkspaceID, req.Workflow, a.Version, a.Commit, a.Path, a.ExecutableHash, a.AuthorityHash,
			w.revision, access.Revision, string(req.Composition), req.RunKey, string(req.Inputs), req.Surface, session, now, now); err != nil {
			return fmt.Errorf("record workflow run: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_checkpoints(run_id,seq,after_attempt,next_step,state,created_at) VALUES(?,0,NULL,?,'{}',?)`,
			id, req.Start, now); err != nil {
			return fmt.Errorf("record workflow checkpoint: %w", err)
		}
		run = WorkflowRun{ID: id}
		return nil
	})
	if err != nil {
		return WorkflowRun{}, false, err
	}
	run, err = readWorkflowRun(ctx, s.db, run.ID)
	return run, existing, err
}

// WorkflowAttemptStart is a node attempt about to run.
type WorkflowAttemptStart struct {
	RunID string
	// Checkpoint is the run's latest checkpoint, whose next step this is.
	Checkpoint int64
	Step       string
	Kind       string
	Attempt    int
	Input      json.RawMessage
}

// latestCheckpoint reads a run's latest checkpoint.
func latestCheckpoint(ctx context.Context, q queryRower, runID string) (WorkflowCheckpoint, error) {
	c := WorkflowCheckpoint{RunID: runID}
	var after sql.NullInt64
	var state, created string
	err := q.QueryRowContext(ctx, `SELECT seq,after_attempt,next_step,state,created_at FROM workflow_run_checkpoints WHERE run_id=? ORDER BY seq DESC LIMIT 1`, runID).
		Scan(&c.Seq, &after, &c.Next, &state, &created)
	if err != nil {
		return c, err
	}
	c.AfterAttempt, c.State = after.Int64, json.RawMessage(state)
	c.CreatedAt, err = parseTime(created)
	return c, err
}

// runRunnable requires a run that has not finished.
func runRunnable(ctx context.Context, q queryRower, runID string) (WorkflowRun, error) {
	r, err := readWorkflowRun(ctx, q, runID)
	if err != nil {
		return r, err
	}
	if r.State.Terminal() {
		return r, fmt.Errorf("%w: %s", ErrWorkflowRunFinished, r.State)
	}
	return r, nil
}

// BeginWorkflowAttempt durably records a node attempt before it runs, under
// the run's current lease: the step the run's latest checkpoint names, its
// next attempt number from that checkpoint, its resolved input and the
// lease. No other attempt of the run may be in flight, a paused run must be
// resumed first, and a retry waits for its not-before time. The run is
// running from its first attempt.
func (s *Store) BeginWorkflowAttempt(ctx context.Context, lease WorkflowRunLease, a WorkflowAttemptStart) (WorkflowNodeAttempt, error) {
	if len(a.Input) == 0 || !json.Valid(a.Input) {
		return WorkflowNodeAttempt{}, errors.New("a node attempt needs its resolved input as JSON")
	}
	if a.RunID != lease.RunID {
		return WorkflowNodeAttempt{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	}
	var out WorkflowNodeAttempt
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, a.RunID)
		if err != nil {
			return err
		}
		if run.State != WorkflowRunQueued && run.State != WorkflowRunRunning {
			return fmt.Errorf("%w: the run is %s", ErrWorkflowRunConflict, run.State)
		}
		if run.NotBefore != nil && run.NotBefore.After(now) {
			return &WorkflowRunNotDueError{NotBefore: *run.NotBefore}
		}
		c, err := latestCheckpoint(ctx, conn, a.RunID)
		if err != nil {
			return err
		}
		if c.Seq != a.Checkpoint || c.Next != a.Step {
			return ErrWorkflowRunConflict
		}
		var inFlight, made int
		var next int64
		if err := conn.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'),
 (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=?),
 (SELECT COALESCE(MAX(seq),0)+1 FROM workflow_node_attempts WHERE run_id=?)`, a.RunID, a.RunID, c.Seq, a.Step, a.RunID).Scan(&inFlight, &made, &next); err != nil {
			return err
		}
		if inFlight > 0 || a.Attempt != made+1 {
			return ErrWorkflowRunConflict
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_node_attempts(run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at,lease_holder,lease_token)
 VALUES(?,?,?,?,?,?,?,'running',NULL,'','','',?,NULL,?,?)`, a.RunID, next, a.Step, a.Kind, a.Attempt, c.Seq, string(a.Input), formatTime(now), lease.Holder, lease.Token); err != nil {
			return fmt.Errorf("record workflow node attempt: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='running',started_at=COALESCE(started_at,?),not_before=NULL,updated_at=? WHERE id=?`,
			formatTime(now), formatTime(now), a.RunID); err != nil {
			return fmt.Errorf("start workflow run: %w", err)
		}
		out = WorkflowNodeAttempt{RunID: a.RunID, Seq: next, Step: a.Step, Kind: a.Kind, Attempt: a.Attempt, InputCheckpoint: c.Seq,
			Input: a.Input, State: WorkflowAttemptRunning, StartedAt: now.UTC(), LeaseHolder: lease.Holder, LeaseToken: lease.Token}
		return nil
	})
	return out, err
}

// WorkflowAttemptEnd is how a node attempt ended. An empty Error means it
// succeeded: Output is its normalized output, and Next and State form the
// next checkpoint; Outputs, given when Next is "", are the run's outputs
// and the run succeeds. A failed attempt records Error; RunError, when set,
// fails the run (no attempt is left); otherwise RetryAfter, when positive,
// is the retry's backoff, recorded as the run's durable not-before time.
type WorkflowAttemptEnd struct {
	RunID      string
	Seq        int64
	Error      string
	Output     json.RawMessage
	Note       string
	Next       string
	State      json.RawMessage
	Outputs    json.RawMessage
	RunError   string
	RetryAfter time.Duration
}

// FinishWorkflowAttempt records how an in-flight attempt ended and, when it
// succeeded, the next checkpoint, in one transaction under the run's
// current lease, before any other attempt can start. The attempt's output
// and its checkpoint commit together or not at all.
func (s *Store) FinishWorkflowAttempt(ctx context.Context, lease WorkflowRunLease, e WorkflowAttemptEnd) (WorkflowRun, error) {
	succeeded := e.Error == ""
	switch {
	case e.RunID != lease.RunID:
		return WorkflowRun{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	case e.RetryAfter < 0 || e.RetryAfter > 0 && (succeeded || e.RunError != ""):
		return WorkflowRun{}, errors.New("only a failed attempt that leaves its run running is retried after a backoff")
	case succeeded && (len(e.Output) == 0 || !json.Valid(e.Output) || !workflowJSONObject(e.State)):
		return WorkflowRun{}, errors.New("a succeeded attempt needs its output and the next checkpoint's state")
	case succeeded && e.Next == "" && !workflowJSONObject(e.Outputs):
		return WorkflowRun{}, errors.New("an attempt that ends the graph needs the run's outputs")
	case succeeded && e.Next != "" && len(e.Outputs) > 0:
		return WorkflowRun{}, errors.New("only an attempt that ends the graph gives the run's outputs")
	case !succeeded && (len(e.Output) > 0 || len(e.State) > 0 || len(e.Outputs) > 0):
		return WorkflowRun{}, errors.New("a failed attempt has no output or checkpoint")
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, clock time.Time) error {
		if _, err := runRunnable(ctx, conn, e.RunID); err != nil {
			return err
		}
		var state, step string
		var input, token int64
		var attempt int
		err := conn.QueryRowContext(ctx, `SELECT state,input_checkpoint,lease_token,step_id,attempt FROM workflow_node_attempts WHERE run_id=? AND seq=?`, e.RunID, e.Seq).
			Scan(&state, &input, &token, &step, &attempt)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (state != string(WorkflowAttemptRunning) || token != lease.Token) {
			return ErrWorkflowRunConflict
		}
		if err != nil {
			return err
		}
		c, err := latestCheckpoint(ctx, conn, e.RunID)
		if err != nil {
			return err
		}
		if c.Seq != input {
			return ErrWorkflowRunConflict
		}
		now := formatTime(clock)
		if !succeeded {
			if _, err := conn.ExecContext(ctx, `UPDATE workflow_node_attempts SET state='failed',error=?,note=?,finished_at=? WHERE run_id=? AND seq=?`,
				e.Error, e.Note, now, e.RunID, e.Seq); err != nil {
				return fmt.Errorf("record failed workflow node attempt: %w", err)
			}
			if e.RunError == "" && e.RetryAfter > 0 {
				retry := clock.Add(e.RetryAfter)
				if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET not_before=?,updated_at=? WHERE id=?`, formatLeaseTime(retry), now, e.RunID); err != nil {
					return fmt.Errorf("record workflow retry time: %w", err)
				}
				return recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: e.RunID, Kind: WorkflowEventRetryScheduled, Holder: lease.Holder, Token: lease.Token, Attempt: e.Seq,
					Detail: fmt.Sprintf("steps.%s attempt %d failed; it is retried from checkpoint %d no earlier than %s", step, attempt, input, formatLeaseTime(retry)), At: clock})
			}
			if e.RunError == "" {
				_, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, now, e.RunID)
				return err
			}
			_, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='failed',error=?,finished_at=?,updated_at=? WHERE id=?`, e.RunError, now, now, e.RunID)
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_node_attempts SET state='succeeded',output=?,next_step=?,note=?,finished_at=? WHERE run_id=? AND seq=?`,
			string(e.Output), e.Next, e.Note, now, e.RunID, e.Seq); err != nil {
			return fmt.Errorf("record workflow node output: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_checkpoints(run_id,seq,after_attempt,next_step,state,created_at) VALUES(?,?,?,?,?,?)`,
			e.RunID, c.Seq+1, e.Seq, e.Next, string(e.State), now); err != nil {
			return fmt.Errorf("record workflow checkpoint: %w", err)
		}
		if e.Next != "" {
			_, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, now, e.RunID)
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='succeeded',outputs=?,finished_at=?,updated_at=? WHERE id=?`, string(e.Outputs), now, now, e.RunID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, e.RunID)
}

// FailWorkflowRun fails a run between attempts (a run limit, or a
// checkpoint the definition cannot continue from), under the run's current
// lease, when checkpoint is still its latest and no attempt is in flight.
func (s *Store) FailWorkflowRun(ctx context.Context, lease WorkflowRunLease, checkpoint int64, reason string) (WorkflowRun, error) {
	if reason == "" {
		return WorkflowRun{}, errors.New("a failed run needs its reason")
	}
	runID := lease.RunID
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, clock time.Time) error {
		if _, err := runRunnable(ctx, conn, runID); err != nil {
			return err
		}
		c, err := latestCheckpoint(ctx, conn, runID)
		if err != nil {
			return err
		}
		var inFlight int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'`, runID).Scan(&inFlight); err != nil {
			return err
		}
		if c.Seq != checkpoint || inFlight > 0 {
			return ErrWorkflowRunConflict
		}
		now := formatTime(clock)
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='failed',error=?,not_before=NULL,finished_at=?,updated_at=? WHERE id=?`, reason, now, now, runID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, runID)
}

const attemptColumns = `run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at,lease_holder,lease_token`

func scanWorkflowAttempt(scanner rowScanner) (WorkflowNodeAttempt, error) {
	var a WorkflowNodeAttempt
	var input, started string
	var output, finished sql.NullString
	if err := scanner.Scan(&a.RunID, &a.Seq, &a.Step, &a.Kind, &a.Attempt, &a.InputCheckpoint, &input, &a.State, &output, &a.Error, &a.Next, &a.Note, &started, &finished,
		&a.LeaseHolder, &a.LeaseToken); err != nil {
		return a, err
	}
	a.Input = json.RawMessage(input)
	if output.Valid {
		a.Output = json.RawMessage(output.String)
	}
	var err error
	if a.StartedAt, err = parseTime(started); err != nil {
		return a, err
	}
	a.FinishedAt, err = parseOptionalTime(finished)
	return a, err
}

// WorkflowRunProgress reads what an executor needs to take the run's next
// step: the run, its latest checkpoint, the attempts made from it and any
// attempt in flight.
func (s *Store) WorkflowRunProgress(ctx context.Context, id string) (WorkflowRunProgress, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowRunProgress{}, err
	}
	defer conn.Close()
	var p WorkflowRunProgress
	if p.Run, err = readWorkflowRun(ctx, conn, id); err != nil {
		return p, err
	}
	if p.Checkpoint, err = latestCheckpoint(ctx, conn, id); err != nil {
		return p, err
	}
	var lastError sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=?),
 (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=? AND state='interrupted'),
 (SELECT error FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=? ORDER BY seq DESC LIMIT 1),
 (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND kind<>'branch' AND state<>'interrupted')`,
		id, p.Checkpoint.Seq, p.Checkpoint.Next, id, p.Checkpoint.Seq, p.Checkpoint.Next, id, p.Checkpoint.Seq, p.Checkpoint.Next, id).
		Scan(&p.Made, &p.Interrupted, &lastError, &p.Counted); err != nil {
		return p, err
	}
	p.Attempts = p.Made - p.Interrupted
	p.LastError = lastError.String
	a, err := scanWorkflowAttempt(conn.QueryRowContext(ctx, `SELECT `+attemptColumns+` FROM workflow_node_attempts WHERE run_id=? AND state='running'`, id))
	switch {
	case err == nil:
		p.InFlight = &a
	case !errors.Is(err, sql.ErrNoRows):
		return p, err
	}
	return p, nil
}

// WorkflowRun reads one run with every attempt and checkpoint, in order.
func (s *Store) WorkflowRun(ctx context.Context, id string) (WorkflowRunDetail, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowRunDetail{}, err
	}
	defer conn.Close()
	d := WorkflowRunDetail{Attempts: []WorkflowNodeAttempt{}, Checkpoints: []WorkflowCheckpoint{}, Events: []WorkflowRunEvent{}}
	if d.Run, err = readWorkflowRun(ctx, conn, id); err != nil {
		return d, err
	}
	rows, err := conn.QueryContext(ctx, `SELECT `+attemptColumns+` FROM workflow_node_attempts WHERE run_id=? ORDER BY seq`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		a, err := scanWorkflowAttempt(rows)
		if err != nil {
			rows.Close()
			return d, err
		}
		d.Attempts = append(d.Attempts, a)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	rows, err = conn.QueryContext(ctx, `SELECT seq,after_attempt,next_step,state,created_at FROM workflow_run_checkpoints WHERE run_id=? ORDER BY seq`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		c := WorkflowCheckpoint{RunID: id}
		var after sql.NullInt64
		var state, created string
		if err := rows.Scan(&c.Seq, &after, &c.Next, &state, &created); err != nil {
			rows.Close()
			return d, err
		}
		c.AfterAttempt, c.State = after.Int64, json.RawMessage(state)
		if c.CreatedAt, err = parseTime(created); err != nil {
			rows.Close()
			return d, err
		}
		d.Checkpoints = append(d.Checkpoints, c)
	}
	if err := rows.Close(); err != nil {
		return d, err
	}
	if err := rows.Err(); err != nil {
		return d, err
	}
	lease, held, err := readWorkflowRunLease(ctx, conn, id)
	if err != nil {
		return d, err
	}
	if held {
		d.Lease = &lease
	}
	if d.Events, err = workflowRunEvents(ctx, conn, id); err != nil {
		return d, err
	}
	d.Substitutions, err = workflowRunSubstitutions(ctx, conn, id)
	return d, err
}

// WorkflowRunFilter narrows a run listing: one Workspace, one workflow in
// it, and at most Limit runs (default 50, at most 200), newest first.
type WorkflowRunFilter struct {
	WorkspaceID memory.WorkspaceID
	Workflow    string
	Limit       int
}

// ListWorkflowRuns lists runs, newest first.
func (s *Store) ListWorkflowRuns(ctx context.Context, f WorkflowRunFilter) ([]WorkflowRun, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	query, args := `SELECT `+runColumns+runFrom+` WHERE 1=1`, []any{}
	if f.WorkspaceID != "" {
		query += ` AND r.workspace_id=?`
		args = append(args, f.WorkspaceID)
	}
	if f.Workflow != "" {
		query += ` AND r.workflow=?`
		args = append(args, f.Workflow)
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY r.rowid DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRun{}
	for rows.Next() {
		r, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
