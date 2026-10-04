package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
)

// Agent steps and map fan-out (Stage 14 sub-stage 9 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0016, 0017, 0019, 0025, 0027):
//
//   - Map items are node attempts of their map step with an item index and
//     the map step's attempt they ran under (workflow_node_attempts gains
//     item_index and map_attempt). Any number of item attempts may be in
//     flight beside the map's own attempt; attempt numbers are per item. An
//     item attempt is recorded before it runs and its output (the item's
//     durable boundary) when it ends, under the run's lease; the map's
//     attempt ends, with the map's output and the next checkpoint, only
//     after every item it started ended. Items that finished are never run
//     again: a map attempt that runs again from the same checkpoint (after
//     a crash, a stop or a pause) takes their recorded outcomes.
//   - workflow_worker_calls: one row per agent attempt (an agent step's or
//     an agent item's), joined to it by its sequence number like model
//     calls, recorded with the attempt in one fenced transaction before the
//     worker is admitted: the worker preset, the pinned Model Policy, the
//     digests of the reviewed objective and the result schema, the budget,
//     the model's price, and the worker execution it ran as (linked in the
//     admission transaction). Its outcome and usage are recorded once, with
//     the attempt's end. An open call counts at its whole token budget,
//     since the worker may spend up to it; a finished one counts its
//     reported usage, or its budget when some usage went unreported.

var (
	// ErrWorkflowWorkerCallOpen refuses ending an agent attempt without
	// recording its worker call.
	ErrWorkflowWorkerCallOpen = errors.New("the agent attempt's worker call has not been recorded")
	// ErrWorkflowItemsInFlight refuses ending a map step's attempt while
	// items it started are still in flight.
	ErrWorkflowItemsInFlight = errors.New("the map step's items are still in flight")
)

// WorkflowWorkerCall is one agent attempt's worker.
type WorkflowWorkerCall struct {
	RunID           string               `json:"run_id"`
	AttemptSeq      int64                `json:"attempt_seq"`
	AttemptState    WorkflowAttemptState `json:"attempt_state"`
	Step            string               `json:"step"`
	Item            *int                 `json:"item,omitempty"`
	Worker          string               `json:"worker"`
	Policy          string               `json:"policy"`
	PolicyVersion   int                  `json:"policy_version"`
	Provider        string               `json:"provider"`
	Model           string               `json:"model"`
	ObjectiveSHA256 string               `json:"objective_sha256"`
	SchemaSHA256    string               `json:"schema_sha256,omitempty"`
	BudgetMS        int64                `json:"budget_ms"`
	BudgetTokens    int64                `json:"budget_tokens"`
	// InputPerMillion and OutputPerMillion are the model's price in USD
	// per million tokens when the call began, from PricingSource.
	InputPerMillion   float64 `json:"input_per_million"`
	OutputPerMillion  float64 `json:"output_per_million"`
	PricingSource     string  `json:"pricing_source"`
	EstimatedMicroUSD int64   `json:"estimated_micro_usd"`
	// AdoptedFrom is the execution whose finished result this attempt took
	// without running a worker: it finished before an earlier attempt's
	// outcome was recorded. Continues is the interrupted build execution
	// whose kept worktree this attempt's worker resumed.
	AdoptedFrom    string     `json:"adopted_from,omitempty"`
	Continues      string     `json:"continues,omitempty"`
	ExecutionID    string     `json:"execution_id,omitempty"`
	ChildSessionID string     `json:"child_session_id,omitempty"`
	LeaseHolder    string     `json:"lease_holder"`
	LeaseToken     int64      `json:"lease_token"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	// Status is the worker's terminal state (or not_admitted when no
	// worker ran), Reason its safe reason.
	Status        string `json:"status,omitempty"`
	Reason        string `json:"reason,omitempty"`
	InputTokens   int64  `json:"input_tokens,omitempty"`
	OutputTokens  int64  `json:"output_tokens,omitempty"`
	UsageComplete bool   `json:"usage_complete,omitempty"`
	CountedTokens int64  `json:"counted_tokens,omitempty"`
	MicroUSD      int64  `json:"micro_usd,omitempty"`
}

// Tokens is what the call counts against the run's limits.max_tokens.
func (c WorkflowWorkerCall) Tokens() int64 {
	if c.FinishedAt != nil {
		return c.CountedTokens
	}
	return c.BudgetTokens
}

// Cost is the call's cost in millionths of a dollar.
func (c WorkflowWorkerCall) Cost() int64 {
	if c.FinishedAt != nil {
		return c.MicroUSD
	}
	return c.EstimatedMicroUSD
}

func ensureWorkflowWorkerCallSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_worker_calls (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 attempt_seq INTEGER NOT NULL CHECK(attempt_seq > 0),
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 item_index INTEGER CHECK(item_index IS NULL OR item_index BETWEEN 0 AND 999),
 worker TEXT NOT NULL CHECK(worker IN ('research','explore','build')),
 policy TEXT NOT NULL CHECK(length(policy) BETWEEN 1 AND 64),
 policy_version INTEGER NOT NULL CHECK(policy_version > 0),
 provider TEXT NOT NULL CHECK(length(provider) BETWEEN 1 AND 64),
 model TEXT NOT NULL CHECK(length(model) BETWEEN 1 AND 256),
 objective_sha256 TEXT NOT NULL CHECK(length(objective_sha256) = 71),
 schema_sha256 TEXT NOT NULL CHECK(schema_sha256 = '' OR length(schema_sha256) = 71),
 budget_ms INTEGER NOT NULL CHECK(budget_ms > 0),
 budget_tokens INTEGER NOT NULL CHECK(budget_tokens > 0),
 input_per_million REAL NOT NULL CHECK(input_per_million >= 0),
 output_per_million REAL NOT NULL CHECK(output_per_million >= 0),
 pricing_source TEXT NOT NULL CHECK(length(pricing_source) <= 64),
 estimated_micro_usd INTEGER NOT NULL CHECK(estimated_micro_usd >= 0),
 adopted_from TEXT NOT NULL CHECK(length(adopted_from) <= 128),
 continues TEXT NOT NULL CHECK(length(continues) <= 128),
 execution_id TEXT UNIQUE CHECK(execution_id IS NULL OR length(execution_id) BETWEEN 1 AND 128),
 child_session_id TEXT REFERENCES sessions(id),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 started_at TEXT NOT NULL,
 finished_at TEXT,
 status TEXT NOT NULL CHECK(status IN ('','succeeded','partial','failed','cancelled','interrupted','not_admitted')),
 reason TEXT NOT NULL CHECK(length(reason) <= 1000),
 input_tokens INTEGER NOT NULL CHECK(input_tokens >= 0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens >= 0),
 usage_complete INTEGER NOT NULL CHECK(usage_complete IN (0,1)),
 counted_tokens INTEGER NOT NULL CHECK(counted_tokens >= 0),
 micro_usd INTEGER NOT NULL CHECK(micro_usd >= 0),
 PRIMARY KEY(run_id, attempt_seq),
 CHECK((finished_at IS NULL) = (status = '')),
 CHECK(finished_at IS NOT NULL OR (input_tokens = 0 AND output_tokens = 0 AND counted_tokens = 0 AND micro_usd = 0)));
 CREATE TRIGGER IF NOT EXISTS workflow_worker_calls_finished BEFORE UPDATE ON workflow_worker_calls
 WHEN OLD.finished_at IS NOT NULL
 BEGIN SELECT RAISE(ABORT,'finished workflow worker calls are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_worker_calls_identity BEFORE UPDATE OF run_id,attempt_seq,step_id,item_index,worker,policy,policy_version,provider,model,
 objective_sha256,schema_sha256,budget_ms,budget_tokens,input_per_million,output_per_million,pricing_source,estimated_micro_usd,adopted_from,continues,
 lease_holder,lease_token,started_at ON workflow_worker_calls
 BEGIN SELECT RAISE(ABORT,'a workflow worker call''s identity is immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_worker_calls_linked_once BEFORE UPDATE OF execution_id,child_session_id ON workflow_worker_calls
 WHEN OLD.execution_id IS NOT NULL
 BEGIN SELECT RAISE(ABORT,'a workflow worker call is linked to one execution'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_worker_calls_no_delete BEFORE DELETE ON workflow_worker_calls
 BEGIN SELECT RAISE(ABORT,'workflow worker calls are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_finish_after_workers BEFORE UPDATE OF state ON workflow_runs
 WHEN NEW.state IN ('succeeded','failed','cancelled') AND EXISTS(SELECT 1 FROM workflow_worker_calls c WHERE c.run_id=NEW.id AND c.finished_at IS NULL)
 BEGIN SELECT RAISE(ABORT,'a workflow run finishes only after every worker it started ended and was recorded'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow worker call schema: %w", err)
	}
	return nil
}

const workerCallColumns = `w.run_id,w.attempt_seq,a.state,w.step_id,w.item_index,w.worker,w.policy,w.policy_version,w.provider,w.model,w.objective_sha256,w.schema_sha256,
 w.budget_ms,w.budget_tokens,w.input_per_million,w.output_per_million,w.pricing_source,w.estimated_micro_usd,w.adopted_from,w.continues,
 w.execution_id,w.child_session_id,w.lease_holder,w.lease_token,w.started_at,w.finished_at,w.status,w.reason,
 w.input_tokens,w.output_tokens,w.usage_complete,w.counted_tokens,w.micro_usd`

const workerCallFrom = ` FROM workflow_worker_calls w JOIN workflow_node_attempts a ON a.run_id=w.run_id AND a.seq=w.attempt_seq`

func scanWorkflowWorkerCall(scanner rowScanner) (WorkflowWorkerCall, error) {
	var c WorkflowWorkerCall
	var item sql.NullInt64
	var execution, child, finished sql.NullString
	var started string
	if err := scanner.Scan(&c.RunID, &c.AttemptSeq, &c.AttemptState, &c.Step, &item, &c.Worker, &c.Policy, &c.PolicyVersion, &c.Provider, &c.Model,
		&c.ObjectiveSHA256, &c.SchemaSHA256, &c.BudgetMS, &c.BudgetTokens, &c.InputPerMillion, &c.OutputPerMillion, &c.PricingSource, &c.EstimatedMicroUSD,
		&c.AdoptedFrom, &c.Continues, &execution, &child, &c.LeaseHolder, &c.LeaseToken, &started, &finished, &c.Status, &c.Reason,
		&c.InputTokens, &c.OutputTokens, &c.UsageComplete, &c.CountedTokens, &c.MicroUSD); err != nil {
		return c, err
	}
	if item.Valid {
		index := int(item.Int64)
		c.Item = &index
	}
	c.ExecutionID, c.ChildSessionID = execution.String, child.String
	var err error
	if c.StartedAt, err = parseTime(started); err != nil {
		return c, err
	}
	c.FinishedAt, err = parseOptionalTime(finished)
	return c, err
}

func queryWorkerCalls(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, where string, args ...any) ([]WorkflowWorkerCall, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+workerCallColumns+workerCallFrom+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowWorkerCall{}
	for rows.Next() {
		c, err := scanWorkflowWorkerCall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func workflowWorkerCalls(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowWorkerCall, error) {
	return queryWorkerCalls(ctx, q, `w.run_id=? ORDER BY w.attempt_seq`, runID)
}

// WorkflowStepWorkerCalls lists the worker calls of one agent step, or of
// one map item (item set), at one checkpoint, newest first: what an
// attempt that runs again from that checkpoint takes over or resumes.
func (s *Store) WorkflowStepWorkerCalls(ctx context.Context, runID string, checkpoint int64, step string, item *int) ([]WorkflowWorkerCall, error) {
	if item == nil {
		return queryWorkerCalls(ctx, s.db, `w.run_id=? AND a.input_checkpoint=? AND w.step_id=? AND w.item_index IS NULL ORDER BY w.attempt_seq DESC`, runID, checkpoint, step)
	}
	return queryWorkerCalls(ctx, s.db, `w.run_id=? AND a.input_checkpoint=? AND w.step_id=? AND w.item_index=? ORDER BY w.attempt_seq DESC`, runID, checkpoint, step, *item)
}

// workerSpend is the run's worker spend so far: what each finished call
// counted, and the budget of each call still open.
func workerSpend(ctx context.Context, q queryRower, runID string) (tokens, micro int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN budget_tokens ELSE counted_tokens END),0),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_micro_usd ELSE micro_usd END),0)
 FROM workflow_worker_calls WHERE run_id=?`, runID).Scan(&tokens, &micro)
	return tokens, micro, err
}

// WorkflowRunSpend is what a map counts before each item it starts: the
// run's attempts counted against limits.max_attempts and its tokens and
// cost so far (AI calls and workers, open calls at their bound).
type WorkflowRunSpend struct {
	Counted  int
	Tokens   int64
	MicroUSD int64
}

// WorkflowRunSpend reads the run's counted attempts and spend.
func (s *Store) WorkflowRunSpend(ctx context.Context, runID string) (WorkflowRunSpend, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return WorkflowRunSpend{}, err
	}
	defer conn.Close()
	var out WorkflowRunSpend
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND kind NOT IN ('branch','map') AND state<>'interrupted'`, runID).Scan(&out.Counted); err != nil {
		return out, err
	}
	modelTokens, modelMicro, err := modelSpend(ctx, conn, runID)
	if err != nil {
		return out, err
	}
	workerTokens, workerMicro, err := workerSpend(ctx, conn, runID)
	if err != nil {
		return out, err
	}
	out.Tokens, out.MicroUSD = modelTokens+workerTokens, modelMicro+workerMicro
	return out, nil
}

// requireAttemptWorkerCallRecorded refuses ending an agent attempt whose
// worker call has no recorded outcome other than with it.
func requireAttemptWorkerCallRecorded(ctx context.Context, q queryRower, runID string, attemptSeq int64) error {
	var open int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_worker_calls WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL`, runID, attemptSeq).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return fmt.Errorf("%w (attempt %d)", ErrWorkflowWorkerCallOpen, attemptSeq)
	}
	return nil
}

// requireItemsEnded refuses ending a map step's attempt while an item it
// started is in flight.
func requireItemsEnded(ctx context.Context, q queryRower, runID string, mapSeq int64) error {
	var running int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND map_attempt=? AND state='running'`, runID, mapSeq).Scan(&running); err != nil {
		return err
	}
	if running > 0 {
		return fmt.Errorf("%w: %d of attempt %d", ErrWorkflowItemsInFlight, running, mapSeq)
	}
	return nil
}

// WorkflowWorkerCallStart is an agent attempt about to run its worker.
type WorkflowWorkerCallStart struct {
	Attempt          WorkflowAttemptStart
	Worker           string
	Policy           string
	PolicyVersion    int
	Provider         string
	Model            string
	ObjectiveSHA256  string
	SchemaSHA256     string
	Budget           time.Duration
	BudgetTokens     int64
	InputPerMillion  float64
	OutputPerMillion float64
	PricingSource    string
	// EstimatedMicroUSD is the cost of the whole token budget, priced as
	// output: what an open call counts.
	EstimatedMicroUSD int64
	AdoptedFrom       string
	Continues         string
	// Cap, when set, is the operator's daily cap (sub-stage 11): a worker
	// that would run is refused, with nothing recorded, when its whole
	// budget would take today's spend past it. An adopted result runs
	// nothing and is not checked.
	Cap *WorkflowDailyCap
}

func validWorkerCallStart(w WorkflowWorkerCallStart) error {
	switch {
	case w.Worker != delegation.WorkerResearch && w.Worker != delegation.WorkerExplore && w.Worker != delegation.WorkerBuild:
		return fmt.Errorf("unknown worker %q", w.Worker)
	case w.Policy == "" || w.PolicyVersion < 1 || w.Provider == "" || w.Model == "":
		return errors.New("a worker call names its Model Policy, provider and model")
	case !validDigest(w.ObjectiveSHA256) || w.SchemaSHA256 != "" && !validDigest(w.SchemaSHA256):
		return errors.New("a worker call needs the digests of its objective and result schema")
	case w.Budget <= 0 || w.BudgetTokens <= 0:
		return errors.New("a worker call needs its time and token budget")
	case w.InputPerMillion < 0 || w.OutputPerMillion < 0 || math.IsNaN(w.InputPerMillion) || math.IsNaN(w.OutputPerMillion) || w.EstimatedMicroUSD < 0 || len(w.PricingSource) > 64:
		return errors.New("a worker call's price is not negative")
	case w.AdoptedFrom != "" && w.Continues != "":
		return errors.New("a worker call adopts a result or resumes a worker, not both")
	}
	return validDailyCap(w.Cap)
}

// capNeed is what a worker call counts toward the daily cap before it
// ends: nothing when it adopts a finished worker's result.
func (w WorkflowWorkerCallStart) capNeed() (tokens, micro int64, checked bool) {
	if w.Cap == nil || w.AdoptedFrom != "" {
		return 0, 0, false
	}
	return w.BudgetTokens, w.EstimatedMicroUSD, true
}

// insertWorkerCall records an agent attempt's worker call.
func insertWorkerCall(ctx context.Context, conn *sql.Conn, a WorkflowNodeAttempt, w WorkflowWorkerCallStart, lease WorkflowRunLease, now time.Time) error {
	var item any
	if a.Item != nil {
		item = *a.Item
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO workflow_worker_calls(run_id,attempt_seq,step_id,item_index,worker,policy,policy_version,provider,model,objective_sha256,schema_sha256,
 budget_ms,budget_tokens,input_per_million,output_per_million,pricing_source,estimated_micro_usd,adopted_from,continues,execution_id,child_session_id,
 lease_holder,lease_token,started_at,finished_at,status,reason,input_tokens,output_tokens,usage_complete,counted_tokens,micro_usd)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,NULL,?,?,?,NULL,'','',0,0,0,0,0)`, a.RunID, a.Seq, a.Step, item, w.Worker, w.Policy, w.PolicyVersion, w.Provider, w.Model,
		w.ObjectiveSHA256, w.SchemaSHA256, w.Budget.Milliseconds(), w.BudgetTokens, w.InputPerMillion, w.OutputPerMillion, w.PricingSource, w.EstimatedMicroUSD,
		w.AdoptedFrom, w.Continues, lease.Holder, lease.Token, formatTime(now))
	if err != nil {
		return fmt.Errorf("record workflow worker call: %w", err)
	}
	return nil
}

// BeginWorkflowWorkerCall durably records an agent step's attempt and its
// worker call, in one transaction under the run's current lease, before
// the worker is admitted (or, adopting, before the earlier worker's result
// is taken).
func (s *Store) BeginWorkflowWorkerCall(ctx context.Context, lease WorkflowRunLease, w WorkflowWorkerCallStart) (WorkflowNodeAttempt, error) {
	if err := validAttemptStart(lease, w.Attempt); err != nil {
		return WorkflowNodeAttempt{}, err
	}
	if err := validWorkerCallStart(w); err != nil {
		return WorkflowNodeAttempt{}, err
	}
	var out WorkflowNodeAttempt
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		if tokens, micro, checked := w.capNeed(); checked {
			if err := checkDailyCap(ctx, conn, now, *w.Cap, tokens, micro); err != nil {
				return err
			}
		}
		var err error
		if out, err = beginWorkflowAttempt(ctx, conn, lease, w.Attempt, now); err != nil {
			return err
		}
		return insertWorkerCall(ctx, conn, out, w, lease, now)
	})
	return out, err
}

// WorkflowWorkerCallEnd is how an agent attempt's worker ended: its
// terminal status (not_admitted when no worker ran) and reason, and its
// reported usage. Attempt is how a step's attempt ends (FinishWorkflowWorkerCall).
type WorkflowWorkerCallEnd struct {
	RunID         string
	AttemptSeq    int64
	Status        string
	Reason        string
	InputTokens   int64
	OutputTokens  int64
	UsageComplete bool
	Attempt       WorkflowAttemptEnd
}

func validWorkerCallEnd(w WorkflowWorkerCallEnd) error {
	switch {
	case !slices.Contains([]string{"succeeded", "partial", "failed", "cancelled", "interrupted", "not_admitted"}, w.Status):
		return fmt.Errorf("unknown worker call status %q", w.Status)
	case w.InputTokens < 0 || w.OutputTokens < 0:
		return errors.New("a worker call's usage is not negative")
	}
	return nil
}

// finishWorkerCall records a worker call's outcome under the lease that
// began it, or any later one (settlement at a claim): what it counts is
// its reported usage when complete, otherwise at least its budget; nothing
// when it adopted an earlier worker's result or no worker ran. Its cost
// prices the counted tokens at the prices recorded when it began, any
// unreported remainder as output.
func finishWorkerCall(ctx context.Context, conn *sql.Conn, w WorkflowWorkerCallEnd, token int64, now time.Time) error {
	if err := validWorkerCallEnd(w); err != nil {
		return err
	}
	var budget int64
	var adopted string
	var inPrice, outPrice float64
	err := conn.QueryRowContext(ctx, `SELECT budget_tokens,adopted_from,input_per_million,output_per_million FROM workflow_worker_calls
 WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL AND lease_token<=?`, w.RunID, w.AttemptSeq, token).Scan(&budget, &adopted, &inPrice, &outPrice)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: attempt %d has no open worker call under lease %d", ErrWorkflowRunConflict, w.AttemptSeq, token)
	}
	if err != nil {
		return err
	}
	counted, unreported := WorkerCallCounted(w, budget, adopted != "")
	if counted == 0 {
		w.InputTokens, w.OutputTokens = 0, 0
	}
	micro := int64(0)
	if counted > 0 {
		micro = int64(math.Ceil(float64(w.InputTokens)*inPrice + float64(w.OutputTokens+unreported)*outPrice))
	}
	_, err = conn.ExecContext(ctx, `UPDATE workflow_worker_calls SET finished_at=?,status=?,reason=?,input_tokens=?,output_tokens=?,usage_complete=?,counted_tokens=?,micro_usd=?
 WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL`, formatTime(now), w.Status, clipText(w.Reason, 1000), w.InputTokens, w.OutputTokens, w.UsageComplete,
		counted, micro, w.RunID, w.AttemptSeq)
	if err != nil {
		return fmt.Errorf("record workflow worker call outcome: %w", err)
	}
	return nil
}

// WorkerCallCounted is what a finished worker call counts against the
// run's limits.max_tokens: nothing when it adopted an earlier worker's
// result or no worker ran, its reported usage when every response
// reported it, and otherwise at least its whole budget (unreported is
// the part above what was reported, priced as output).
func WorkerCallCounted(w WorkflowWorkerCallEnd, budget int64, adopted bool) (counted, unreported int64) {
	reported := w.InputTokens + w.OutputTokens
	switch {
	case adopted || w.Status == "not_admitted":
		return 0, 0
	case !w.UsageComplete && reported < budget:
		return budget, budget - reported
	}
	return reported, 0
}

// FinishWorkflowWorkerCall records an agent step's worker call and its
// attempt's outcome (the worker result with the next checkpoint, or its
// failure with the retry time or the run's failure) in one transaction
// under the run's current lease.
func (s *Store) FinishWorkflowWorkerCall(ctx context.Context, lease WorkflowRunLease, w WorkflowWorkerCallEnd) (WorkflowRun, error) {
	w.Attempt.RunID, w.Attempt.Seq = w.RunID, w.AttemptSeq
	if err := validAttemptEnd(lease, w.Attempt); err != nil {
		return WorkflowRun{}, err
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		if err := requireOwnWorkerCall(ctx, conn, lease, w.RunID, w.AttemptSeq); err != nil {
			return err
		}
		if err := finishWorkerCall(ctx, conn, w, lease.Token, now); err != nil {
			return err
		}
		return finishWorkflowAttempt(ctx, conn, lease, w.Attempt, now)
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, w.RunID)
}

// InterruptWorkflowWorkerCall records the holder's own agent attempt
// interrupted (the runtime stopped, or the owner cancelled the run, while
// its worker ran), with the worker's outcome, in one transaction under the
// run's current lease.
func (s *Store) InterruptWorkflowWorkerCall(ctx context.Context, lease WorkflowRunLease, w WorkflowWorkerCallEnd, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("an interrupted attempt needs its reason")
	}
	return s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		if err := requireOwnWorkerCall(ctx, conn, lease, w.RunID, w.AttemptSeq); err != nil {
			return err
		}
		if err := finishWorkerCall(ctx, conn, w, lease.Token, now); err != nil {
			return err
		}
		return interruptAttempt(ctx, conn, lease, w.AttemptSeq, reason, now)
	})
}

// requireOwnWorkerCall requires the worker call to have begun under lease.
func requireOwnWorkerCall(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, runID string, seq int64) error {
	var token int64
	err := conn.QueryRowContext(ctx, `SELECT lease_token FROM workflow_worker_calls WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL`, runID, seq).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) || err == nil && token != lease.Token {
		return fmt.Errorf("%w: attempt %d has no open worker call under lease %d", ErrWorkflowRunConflict, seq, lease.Token)
	}
	return err
}

// WorkflowItemStart is one map item attempt about to run: its index, the
// item step's kind, its attempt number for that item, its resolved input,
// and an agent item's worker call or an AI item's model call (their
// Attempt fields are not used).
type WorkflowItemStart struct {
	Index   int
	Kind    string
	Attempt int
	Input   json.RawMessage
	Worker  *WorkflowWorkerCallStart
	Model   *WorkflowModelCallStart
}

// BeginWorkflowItemAttempts durably records item attempts of the map step
// attempt mapSeq, in one transaction under the run's current lease, before
// they run: run admission is applied once for the batch. The run must be
// running (a cancelling run starts nothing), the map's attempt in flight
// under this lease from the run's latest checkpoint, each item's attempt
// number its next one, and no attempt of the item in flight.
func (s *Store) BeginWorkflowItemAttempts(ctx context.Context, lease WorkflowRunLease, mapSeq int64, items []WorkflowItemStart) ([]WorkflowNodeAttempt, error) {
	if len(items) == 0 {
		return nil, errors.New("begin at least one item attempt")
	}
	for _, it := range items {
		switch {
		case len(it.Input) == 0 || !json.Valid(it.Input):
			return nil, errors.New("an item attempt needs its resolved input as JSON")
		case it.Index < 0 || it.Index > 999 || it.Attempt < 1:
			return nil, fmt.Errorf("item %d attempt %d is not an item attempt", it.Index, it.Attempt)
		case it.Kind != "deterministic" && it.Kind != "ai" && it.Kind != "agent":
			return nil, fmt.Errorf("a map item is a deterministic, AI or agent step, not %s", it.Kind)
		case (it.Kind == "agent") != (it.Worker != nil) || (it.Kind == "ai") != (it.Model != nil):
			return nil, errors.New("an agent item records its worker call and an AI item its model call")
		}
		if it.Worker != nil {
			if err := validWorkerCallStart(*it.Worker); err != nil {
				return nil, err
			}
		}
		if it.Model != nil {
			if err := validDailyCap(it.Model.Cap); err != nil {
				return nil, err
			}
		}
	}
	// The batch is checked against the daily cap as a whole (sub-stage 11).
	var capLimit *WorkflowDailyCap
	var needTokens, needMicro int64
	for _, it := range items {
		switch {
		case it.Worker != nil:
			if tokens, micro, checked := it.Worker.capNeed(); checked {
				capLimit, needTokens, needMicro = it.Worker.Cap, needTokens+tokens, needMicro+micro
			}
		case it.Model != nil && it.Model.Cap != nil:
			capLimit = it.Model.Cap
			needTokens, needMicro = needTokens+it.Model.EstimatedInputTokens+it.Model.EstimatedOutputTokens, needMicro+it.Model.EstimatedMicroUSD
		}
	}
	var out []WorkflowNodeAttempt
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if run.State == WorkflowRunCancelling {
			return fmt.Errorf("%w (no item was started)", ErrWorkflowRunCancelling)
		}
		if run.State != WorkflowRunRunning {
			return fmt.Errorf("%w: the run is %s", ErrWorkflowRunConflict, run.State)
		}
		var state, kind, step string
		var token, checkpoint int64
		var item sql.NullInt64
		err = conn.QueryRowContext(ctx, `SELECT state,kind,step_id,lease_token,input_checkpoint,item_index FROM workflow_node_attempts WHERE run_id=? AND seq=?`, lease.RunID, mapSeq).
			Scan(&state, &kind, &step, &token, &checkpoint, &item)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (state != string(WorkflowAttemptRunning) || kind != "map" || token != lease.Token || item.Valid) {
			return fmt.Errorf("%w: attempt %d is not a map step's attempt in flight under lease %d", ErrWorkflowRunConflict, mapSeq, lease.Token)
		}
		if err != nil {
			return err
		}
		c, err := latestCheckpoint(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if c.Seq != checkpoint {
			return ErrWorkflowRunConflict
		}
		// Run admission once for the whole batch (sub-stage 8 applies it
		// to every node; sub-stage 9 batches a map's items).
		if err := authorizeWorkflowRun(ctx, conn, run); err != nil {
			return err
		}
		if capLimit != nil {
			if err := checkDailyCap(ctx, conn, now, *capLimit, needTokens, needMicro); err != nil {
				return err
			}
		}
		var next int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM workflow_node_attempts WHERE run_id=?`, lease.RunID).Scan(&next); err != nil {
			return err
		}
		for _, it := range items {
			var made, running int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(state='running'),0) FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=? AND item_index=?`,
				lease.RunID, checkpoint, step, it.Index).Scan(&made, &running); err != nil {
				return err
			}
			if running > 0 || it.Attempt != made+1 {
				return fmt.Errorf("%w: steps.%s item %d attempt %d", ErrWorkflowRunConflict, step, it.Index, it.Attempt)
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_node_attempts(run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at,lease_holder,lease_token,item_index,map_attempt)
 VALUES(?,?,?,?,?,?,?,'running',NULL,'','','',?,NULL,?,?,?,?)`, lease.RunID, next, step, it.Kind, it.Attempt, checkpoint, string(it.Input), formatTime(now), lease.Holder, lease.Token, it.Index, mapSeq); err != nil {
				return fmt.Errorf("record workflow map item attempt: %w", err)
			}
			index, mapped := it.Index, mapSeq
			a := WorkflowNodeAttempt{RunID: lease.RunID, Seq: next, Step: step, Kind: it.Kind, Attempt: it.Attempt, InputCheckpoint: checkpoint, Input: it.Input,
				State: WorkflowAttemptRunning, StartedAt: now.UTC(), LeaseHolder: lease.Holder, LeaseToken: lease.Token, Item: &index, MapAttempt: &mapped}
			switch {
			case it.Worker != nil:
				if err := insertWorkerCall(ctx, conn, a, *it.Worker, lease, now); err != nil {
					return err
				}
			case it.Model != nil:
				if err := insertModelCall(ctx, conn, a, *it.Model, lease, now); err != nil {
					return err
				}
			}
			out = append(out, a)
			next++
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, formatTime(now), lease.RunID)
		return err
	})
	return out, err
}

// WorkflowItemEnd is how one map item attempt ended: a succeeded item's
// normalized output, or its failure, or (Interrupted) the holder's
// interruption of it with Error as the reason (the runtime stopped, the
// owner cancelled the run, or an item that failed stopped the map). An
// agent item ends with its worker call and an AI item with its model
// call.
type WorkflowItemEnd struct {
	RunID       string
	Seq         int64
	Output      json.RawMessage
	Error       string
	Note        string
	Interrupted bool
	Worker      *WorkflowWorkerCallEnd
	Model       *WorkflowModelCallEnd
}

// FinishWorkflowItemAttempt records how a map item attempt ended, with its
// worker or model call, in one transaction under the run's current lease.
// It writes no checkpoint: the item's recorded outcome is its durable
// boundary, and the map's attempt writes the next checkpoint once every
// item ended.
func (s *Store) FinishWorkflowItemAttempt(ctx context.Context, lease WorkflowRunLease, e WorkflowItemEnd) error {
	succeeded := e.Error == "" && !e.Interrupted
	switch {
	case e.RunID != lease.RunID:
		return fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	case succeeded && (len(e.Output) == 0 || !json.Valid(e.Output)):
		return errors.New("a succeeded item attempt needs its output")
	case !succeeded && (e.Error == "" || len(e.Output) > 0):
		return errors.New("a failed or interrupted item attempt has its error and no output")
	}
	return s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		var state string
		var token int64
		var item sql.NullInt64
		err := conn.QueryRowContext(ctx, `SELECT state,lease_token,item_index FROM workflow_node_attempts WHERE run_id=? AND seq=?`, e.RunID, e.Seq).Scan(&state, &token, &item)
		if errors.Is(err, sql.ErrNoRows) || err == nil && (state != string(WorkflowAttemptRunning) || token != lease.Token || !item.Valid) {
			return fmt.Errorf("%w: attempt %d is not a map item attempt in flight under lease %d", ErrWorkflowRunConflict, e.Seq, lease.Token)
		}
		if err != nil {
			return err
		}
		if e.Worker != nil {
			e.Worker.RunID, e.Worker.AttemptSeq = e.RunID, e.Seq
			if err := requireOwnWorkerCall(ctx, conn, lease, e.RunID, e.Seq); err != nil {
				return err
			}
			if err := finishWorkerCall(ctx, conn, *e.Worker, lease.Token, now); err != nil {
				return err
			}
		}
		if e.Model != nil {
			e.Model.RunID, e.Model.AttemptSeq = e.RunID, e.Seq
			if err := finishModelCallRow(ctx, conn, *e.Model, lease, now); err != nil {
				return err
			}
		}
		if err := requireAttemptWorkerCallRecorded(ctx, conn, e.RunID, e.Seq); err != nil {
			return err
		}
		if err := requireAttemptModelCallRecorded(ctx, conn, e.RunID, e.Seq); err != nil {
			return err
		}
		switch {
		case e.Interrupted:
			return interruptAttempt(ctx, conn, lease, e.Seq, e.Error, now)
		case succeeded:
			_, err = conn.ExecContext(ctx, `UPDATE workflow_node_attempts SET state='succeeded',output=?,note=?,finished_at=? WHERE run_id=? AND seq=?`,
				string(e.Output), e.Note, formatTime(now), e.RunID, e.Seq)
		default:
			_, err = conn.ExecContext(ctx, `UPDATE workflow_node_attempts SET state='failed',error=?,note=?,finished_at=? WHERE run_id=? AND seq=?`,
				clipText(e.Error, maxEffectText), e.Note, formatTime(now), e.RunID, e.Seq)
		}
		if err != nil {
			return fmt.Errorf("record workflow map item outcome: %w", err)
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, formatTime(now), e.RunID)
		return err
	})
}

// WorkflowItemAttempts lists the item attempts of one map step at one
// checkpoint, in the order they were recorded: what a map attempt that
// runs again from that checkpoint continues from.
func (s *Store) WorkflowItemAttempts(ctx context.Context, runID string, checkpoint int64, step string) ([]WorkflowNodeAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+attemptColumns+` FROM workflow_node_attempts WHERE run_id=? AND input_checkpoint=? AND step_id=? AND item_index IS NOT NULL ORDER BY seq`,
		runID, checkpoint, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowNodeAttempt{}
	for rows.Next() {
		a, err := scanWorkflowAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// workerUsage is what a worker reported: its input and output tokens and
// whether every response reported them.
func workerUsage(r *delegation.Result) (in, out int64, complete bool) {
	if r == nil || r.Usage == nil {
		return 0, 0, false
	}
	u := r.Usage
	if u.InputTokens != nil {
		in = max(*u.InputTokens, 0)
	}
	if u.OutputTokens != nil {
		out = max(*u.OutputTokens, 0)
	}
	return in, out, !u.Incomplete && u.InputTokens != nil && u.OutputTokens != nil
}

// WorkerCallEndOf is a worker call's end from its settled execution.
func WorkerCallEndOf(a delegation.Attempt) WorkflowWorkerCallEnd {
	in, out, complete := workerUsage(a.Result)
	status, reason := a.State, ""
	if a.Result != nil {
		status, reason = a.Result.Status, a.Result.Reason
	}
	return WorkflowWorkerCallEnd{Status: status, Reason: reason, InputTokens: in, OutputTokens: out, UsageComplete: complete}
}

// settleRunWorkers is the part of a claim that settles what an earlier
// holder's workers left: every run-owned execution of the run admitted
// under an older lease that is still admitted or running ends interrupted
// (its lease ended, so its authority did), and every worker call an
// earlier lease left open is recorded with its execution's outcome and
// usage, or as not admitted when no worker was admitted for it.
func (s *Store) settleRunWorkers(ctx context.Context, conn *sql.Conn, runID string, token int64, now time.Time) error {
	rows, err := conn.QueryContext(ctx, `SELECT e.record_json FROM subagent_executions e JOIN workflow_worker_sessions w ON e.parent_session_id=w.session_id
 WHERE w.run_id=? AND e.state IN ('admitted','running') ORDER BY e.rowid`, runID)
	if err != nil {
		return err
	}
	var unfinished []delegation.Attempt
	for rows.Next() {
		a, err := readSubagent(rows)
		if err != nil {
			rows.Close()
			return err
		}
		unfinished = append(unfinished, a)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, a := range unfinished {
		if a.Run == nil || a.Run.Token >= token {
			continue
		}
		if err := s.finishSubagent(ctx, conn, &a, "interrupted", delegation.ReasonRunOwnershipEnded, nil); err != nil {
			return fmt.Errorf("settle worker execution %s: %w", a.ID, err)
		}
	}
	open, err := queryWorkerCalls(ctx, conn, `w.run_id=? AND w.finished_at IS NULL AND w.lease_token<? ORDER BY w.attempt_seq`, runID, token)
	if err != nil {
		return err
	}
	for _, c := range open {
		end := WorkflowWorkerCallEnd{RunID: runID, AttemptSeq: c.AttemptSeq, Status: "not_admitted", Reason: "the holder stopped before a worker was admitted"}
		if c.ExecutionID != "" {
			a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, c.ExecutionID))
			if err != nil {
				return fmt.Errorf("read worker execution %s: %w", c.ExecutionID, err)
			}
			end = WorkerCallEndOf(a)
			end.RunID, end.AttemptSeq = runID, c.AttemptSeq
		} else if c.AdoptedFrom != "" {
			end.Status, end.Reason = "succeeded", "adopted "+c.AdoptedFrom
		}
		if err := finishWorkerCall(ctx, conn, end, token, now); err != nil {
			return err
		}
	}
	return nil
}
