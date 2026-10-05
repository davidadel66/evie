package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// The retention sweep's store operations (Stage 14 sub-stage 12). Each
// action is one immediate transaction that checks, inside it, that the run
// (or record) may be acted on, opens the retention guard for its purpose,
// rewrites or removes exactly what its policy covers, records its
// tombstone and closes the guard. A dry run does the same and rolls back,
// reporting what it would have recorded.

// WorkflowRetentionCandidate is a finished run the sweep may act on.
type WorkflowRetentionCandidate struct {
	RowID       int64              `json:"-"`
	RunID       string             `json:"run_id"`
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Workflow    string             `json:"workflow"`
	Version     int64              `json:"version"`
	Commit      string             `json:"commit"`
	Path        string             `json:"path"`
	FinishedAt  time.Time          `json:"finished_at"`
	// Redacted: a redaction of it is recorded. Compactable: one of its
	// intermediate checkpoints still holds its state.
	Redacted    bool `json:"redacted"`
	Compactable bool `json:"compactable"`
}

// WorkflowRetentionCandidates lists finished runs after rowID, oldest
// first, at most limit.
func (s *Store) WorkflowRetentionCandidates(ctx context.Context, afterRowID int64, limit int) ([]WorkflowRetentionCandidate, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.rowid,r.id,r.workspace_id,r.workflow,r.version,r.commit_id,r.tree_path,r.finished_at,
 EXISTS(SELECT 1 FROM workflow_retention_tombstones t WHERE t.run_id=r.id AND t.action='redact'),
 EXISTS(SELECT 1 FROM workflow_run_checkpoints c WHERE c.run_id=r.id AND c.seq>0 AND c.compacted_at IS NULL
 AND c.seq<(SELECT MAX(k.seq) FROM workflow_run_checkpoints k WHERE k.run_id=r.id))
 FROM workflow_runs r WHERE r.state IN (`+terminalWorkflowStates+`) AND r.rowid>? ORDER BY r.rowid LIMIT ?`, afterRowID, min(limit, 1000))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkflowRetentionCandidate
	for rows.Next() {
		var c WorkflowRetentionCandidate
		var finished sql.NullString
		if err := rows.Scan(&c.RowID, &c.RunID, &c.WorkspaceID, &c.Workflow, &c.Version, &c.Commit, &c.Path, &finished, &c.Redacted, &c.Compactable); err != nil {
			return nil, err
		}
		if c.FinishedAt, err = parseTime(finished.String); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// WorkflowRetentionRun is one retention action on one finished run.
type WorkflowRetentionRun struct {
	RunID string
	// Sensitivity is the run's pinned version's (redaction only).
	Sensitivity workflow.Sensitivity
	// FinishedBefore: the run must have finished at or before it.
	FinishedBefore time.Time
	SweepID        string
	Policy         string
	Reason         string
	// Actor is "runtime" for the periodic sweep, "owner" for the owner's
	// command with its Surface.
	Actor   string
	Surface string
	DryRun  bool
}

func (req WorkflowRetentionRun) valid() error {
	switch {
	case req.RunID == "" || req.SweepID == "" || strings.TrimSpace(req.Policy) == "" || strings.TrimSpace(req.Reason) == "":
		return errors.New("a retention action names its run, sweep, policy and reason")
	case req.Actor == "runtime" && req.Surface != "", req.Actor == "owner" && req.Surface != "cli" && req.Surface != "web", req.Actor != "runtime" && req.Actor != "owner":
		return errors.New("a retention action is the runtime's sweep or the owner's command from the CLI or the web")
	}
	return nil
}

// WorkflowRetentionSkip is a run the sweep leaves as it is, and why.
type WorkflowRetentionSkip struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

func (e *WorkflowRetentionSkip) Error() string { return "run " + e.RunID + " is kept: " + e.Reason }

// settledForRetention requires a finished run, finished at or before
// cutoff, every Effect Intent of which has its receipt and whose
// successors all finished: evidence an unfinished run may still need is
// never touched.
func settledForRetention(ctx context.Context, conn *sql.Conn, run WorkflowRun, cutoff time.Time) error {
	skip := func(format string, args ...any) error {
		return &WorkflowRetentionSkip{RunID: run.ID, Reason: fmt.Sprintf(format, args...)}
	}
	if !run.State.Terminal() || run.FinishedAt == nil {
		return skip("it has not finished")
	}
	if run.FinishedAt.After(cutoff) {
		return skip("it finished at %s, inside the policy's window", run.FinishedAt.Format(time.RFC3339))
	}
	if err := requireEffectsResolved(ctx, conn, run.ID); err != nil {
		return skip("%v", err)
	}
	id := run.ID
	for range 1000 {
		m, found, err := readMigration(ctx, conn, "predecessor_id", id)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		var state WorkflowRunState
		if err := conn.QueryRowContext(ctx, `SELECT state FROM workflow_runs WHERE id=?`, m.SuccessorID).Scan(&state); err != nil {
			return err
		}
		if !state.Terminal() {
			return skip("its successor run %s is still %s", m.SuccessorID, state)
		}
		id = m.SuccessorID
	}
	return skip("its migration chain is too long")
}

// openRetentionGuard opens the guard of one purpose inside a transaction;
// the returned close removes it before the transaction ends.
func openRetentionGuard(ctx context.Context, conn *sql.Conn, purpose string) (func() error, error) {
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_retention_guard(singleton,purpose) VALUES(1,?)`, purpose); err != nil {
		return nil, fmt.Errorf("open the retention guard: %w", err)
	}
	return func() error {
		_, err := conn.ExecContext(ctx, `DELETE FROM workflow_retention_guard`)
		return err
	}, nil
}

func recordTombstone(ctx context.Context, conn *sql.Conn, t *WorkflowRetentionTombstone) error {
	fields, err := json.Marshal(t.Fields)
	if err != nil || t.Fields == nil {
		fields = []byte("{}")
	}
	optional := func(at *time.Time) any {
		if at == nil {
			return nil
		}
		return formatTime(*at)
	}
	var run any
	if t.RunID != "" {
		run = t.RunID
	}
	err = conn.QueryRowContext(ctx, `INSERT INTO workflow_retention_tombstones(sweep_id,action,data_class,classification,workspace_id,workflow,run_id,records,bytes,fields,
 oldest_at,newest_at,policy,reason,actor,surface,recorded_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) RETURNING id`,
		t.SweepID, t.Action, t.DataClass, t.Classification, t.WorkspaceID, t.Workflow, run, t.Records, t.Bytes, string(fields),
		optional(t.OldestAt), optional(t.NewestAt), t.Policy, t.Reason, t.Actor, t.Surface, formatTime(t.RecordedAt)).Scan(&t.ID)
	if t.DryRun {
		// A dry run's transaction is rolled back: nothing was recorded.
		t.ID = 0
	}
	return err
}

// retentionTransaction runs one action in an immediate transaction; a dry
// run is rolled back and reported.
func (s *Store) retentionTransaction(ctx context.Context, dryRun bool, action func(conn *sql.Conn, now time.Time) error) error {
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := action(conn, s.now().UTC()); err != nil {
			return err
		}
		if dryRun {
			return errRetentionDryRun
		}
		return nil
	})
	if errors.Is(err, errRetentionDryRun) {
		return nil
	}
	return err
}

// rewriter counts what one redaction rewrites.
type rewriter struct {
	ctx    context.Context
	conn   *sql.Conn
	fields map[string]int
	bytes  int64
}

// set updates one record when any of its values changed. pairs are
// column, old value, new value; key is the WHERE clause and its arguments.
func (w *rewriter) set(table, where string, key []any, pairs ...any) error {
	var sets []string
	var args []any
	for i := 0; i+2 < len(pairs); i += 3 {
		column := pairs[i].(string)
		old, next := columnText(pairs[i+1]), columnText(pairs[i+2])
		if old == next || pairs[i+1] == nil {
			continue
		}
		sets = append(sets, column+"=?")
		args = append(args, pairs[i+2])
		w.bytes += int64(len(old))
	}
	if len(sets) == 0 {
		return nil
	}
	if _, err := w.conn.ExecContext(w.ctx, `UPDATE `+table+` SET `+strings.Join(sets, ",")+` WHERE `+where, append(args, key...)...); err != nil {
		return fmt.Errorf("redact %s: %w", table, err)
	}
	w.fields[table]++
	return nil
}

// columnText is a nullable column value as text; nil reads as "".
func columnText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.RawMessage:
		return string(x)
	case []byte:
		return string(x)
	}
	return fmt.Sprint(v)
}

// nullableJSON is a JSON column value: NULL when absent.
func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

// RedactWorkflowRun redacts one finished run's sensitive values from
// storage: under its version's classification, the sensitive inputs'
// values (ordinary) or every value it recorded (sensitive), across the
// run, its checkpoints, attempts, recovery events, Effect Intents, unknown
// outcomes and receipts, interruptions and answers, model and worker
// calls, and its notifications. It returns the tombstone it recorded (nil
// when there was nothing to redact or it was already redacted), or a
// *WorkflowRetentionSkip when the run may not be redacted yet.
func (s *Store) RedactWorkflowRun(ctx context.Context, req WorkflowRetentionRun) (*WorkflowRetentionTombstone, error) {
	if err := req.valid(); err != nil {
		return nil, err
	}
	var out *WorkflowRetentionTombstone
	err := s.retentionTransaction(ctx, req.DryRun, func(conn *sql.Conn, now time.Time) error {
		run, err := readWorkflowRun(ctx, conn, req.RunID)
		if err != nil {
			return err
		}
		if err := settledForRetention(ctx, conn, run, req.FinishedBefore); err != nil {
			return err
		}
		var done bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_retention_tombstones WHERE run_id=? AND action='redact')`, run.ID).Scan(&done); err != nil || done {
			return err
		}
		red := NewWorkflowRedactor(req.Sensitivity, run.Inputs, WorkflowRedactMarker, true)
		if !red.Active() {
			return nil
		}
		closeGuard, err := openRetentionGuard(ctx, conn, WorkflowRetentionRedact)
		if err != nil {
			return err
		}
		w := &rewriter{ctx: ctx, conn: conn, fields: map[string]int{}}
		if err := redactRunRecords(ctx, conn, w, red, run); err != nil {
			return err
		}
		if err := closeGuard(); err != nil {
			return err
		}
		records := 0
		for _, n := range w.fields {
			records += n
		}
		if records == 0 {
			return nil
		}
		class := WorkflowDataSensitiveInputs
		if req.Sensitivity.Classification == workflow.ClassificationSensitive {
			class = WorkflowDataRunValues
		}
		created := run.CreatedAt
		out = &WorkflowRetentionTombstone{SweepID: req.SweepID, Action: WorkflowRetentionRedact, DataClass: class, Classification: req.Sensitivity.Classification,
			WorkspaceID: string(run.WorkspaceID), Workflow: run.Workflow, RunID: run.ID, Records: records, Bytes: w.bytes, Fields: w.fields,
			OldestAt: &created, NewestAt: run.FinishedAt, Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface, RecordedAt: now, DryRun: req.DryRun}
		return recordTombstone(ctx, conn, out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// redactRunRecords rewrites every record of run under the open guard.
func redactRunRecords(ctx context.Context, conn *sql.Conn, w *rewriter, red WorkflowRedactor, run WorkflowRun) error {
	var outputs any
	if len(run.Outputs) > 0 {
		outputs = string(run.Outputs)
	}
	if err := w.set("workflow_runs", "id=?", []any{run.ID},
		"inputs", string(run.Inputs), string(red.Inputs(run.Inputs)),
		"outputs", outputs, nullableJSON(red.Outputs(run.Outputs)),
		"error", run.Error, red.Text(run.Error)); err != nil {
		return err
	}
	// each scans one table's rows of the run, then applies the update each
	// row needs once the rows are closed: a connection cannot change a
	// table while its own query of it is open.
	each := func(query string, scan func(*sql.Rows) (func() error, error)) error {
		rows, err := conn.QueryContext(ctx, query, run.ID)
		if err != nil {
			return err
		}
		var updates []func() error
		for rows.Next() {
			update, err := scan(rows)
			if err != nil {
				rows.Close()
				return err
			}
			updates = append(updates, update)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, update := range updates {
			if err := update(); err != nil {
				return err
			}
		}
		return nil
	}
	nullable := func(s sql.NullString) any {
		if s.Valid {
			return s.String
		}
		return nil
	}
	steps := []struct {
		query string
		scan  func(*sql.Rows) (func() error, error)
	}{
		{`SELECT seq,state FROM workflow_run_checkpoints WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var state string
			err := rows.Scan(&seq, &state)
			return func() error {
				return w.set("workflow_run_checkpoints", "run_id=? AND seq=?", []any{run.ID, seq}, "state", state, string(red.Value(json.RawMessage(state))))
			}, err
		}},
		{`SELECT seq,step_id,kind,item_index,input,output,error,note FROM workflow_node_attempts WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var step, kind, input, errText, note string
			var item sql.NullInt64
			var output sql.NullString
			err := rows.Scan(&seq, &step, &kind, &item, &input, &output, &errText, &note)
			return func() error {
				return w.set("workflow_node_attempts", "run_id=? AND seq=?", []any{run.ID, seq},
					"input", input, string(red.StepInput(step, item.Valid, kind, json.RawMessage(input))),
					"output", nullable(output), nullableJSON(red.Value(json.RawMessage(output.String))),
					"error", errText, red.Text(errText),
					"note", note, red.Text(note))
			}, err
		}},
		{`SELECT seq,detail FROM workflow_run_events WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var detail string
			err := rows.Scan(&seq, &detail)
			return func() error {
				return w.set("workflow_run_events", "run_id=? AND seq=?", []any{run.ID, seq}, "detail", detail, red.Text(detail))
			}, err
		}},
		{`SELECT seq,step_id,input FROM workflow_effect_intents WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var step, input string
			err := rows.Scan(&seq, &step, &input)
			return func() error {
				return w.set("workflow_effect_intents", "run_id=? AND seq=?", []any{run.ID, seq},
					"input", input, string(red.StepInput(step, false, workflow.KindCapability, json.RawMessage(input))))
			}, err
		}},
		{`SELECT intent_seq,reason,response FROM workflow_effect_unknowns WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var reason string
			var response sql.NullString
			err := rows.Scan(&seq, &reason, &response)
			return func() error {
				return w.set("workflow_effect_unknowns", "run_id=? AND intent_seq=?", []any{run.ID, seq},
					"reason", reason, red.Text(reason), "response", nullable(response), nullableJSON(red.Value(json.RawMessage(response.String))))
			}, err
		}},
		{`SELECT intent_seq,response,error,evidence FROM workflow_effect_receipts WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var response sql.NullString
			var errText, evidence string
			err := rows.Scan(&seq, &response, &errText, &evidence)
			return func() error {
				return w.set("workflow_effect_receipts", "run_id=? AND intent_seq=?", []any{run.ID, seq},
					"response", nullable(response), nullableJSON(red.Value(json.RawMessage(response.String))),
					"error", errText, red.Text(errText), "evidence", evidence, red.Text(evidence))
			}, err
		}},
		{`SELECT seq,step_id,message,request FROM workflow_interruptions WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var step, message, request string
			err := rows.Scan(&seq, &step, &message, &request)
			return func() error {
				return w.set("workflow_interruptions", "run_id=? AND seq=?", []any{run.ID, seq},
					"message", message, red.Text(message), "request", request, string(red.Request(step, json.RawMessage(request))))
			}, err
		}},
		{`SELECT interruption_seq,answer,detail FROM workflow_interruption_resolutions WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var answer sql.NullString
			var detail string
			err := rows.Scan(&seq, &answer, &detail)
			return func() error {
				return w.set("workflow_interruption_resolutions", "run_id=? AND interruption_seq=?", []any{run.ID, seq},
					"answer", nullable(answer), nullableJSON(red.Value(json.RawMessage(answer.String))), "detail", detail, red.Text(detail))
			}, err
		}},
		{`SELECT attempt_seq,response,error FROM workflow_model_calls WHERE run_id=? AND finished_at IS NOT NULL`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var response, errText string
			err := rows.Scan(&seq, &response, &errText)
			return func() error {
				return w.set("workflow_model_calls", "run_id=? AND attempt_seq=?", []any{run.ID, seq}, "response", response, red.Text(response), "error", errText, red.Text(errText))
			}, err
		}},
		{`SELECT attempt_seq,reason FROM workflow_worker_calls WHERE run_id=? AND finished_at IS NOT NULL`, func(rows *sql.Rows) (func() error, error) {
			var seq int64
			var reason string
			err := rows.Scan(&seq, &reason)
			return func() error {
				return w.set("workflow_worker_calls", "run_id=? AND attempt_seq=?", []any{run.ID, seq}, "reason", reason, red.Text(reason))
			}, err
		}},
		{`SELECT id,detail FROM workflow_notifications WHERE run_id=?`, func(rows *sql.Rows) (func() error, error) {
			var id int64
			var detail string
			err := rows.Scan(&id, &detail)
			return func() error {
				return w.set("workflow_notifications", "id=?", []any{id}, "detail", detail, red.Text(detail))
			}, err
		}},
	}
	for _, s := range steps {
		if err := each(s.query, s.scan); err != nil {
			return err
		}
	}
	return nil
}

// CompactWorkflowRun compacts one finished run's checkpoints: every
// checkpoint between its first and its last, each of which is exactly the
// one before it with the output of the attempt that produced it, keeps its
// row and loses its stored state, which inspection rebuilds from those
// attempts (WorkflowRun). It checks that every compacted state rebuilds to
// what is stored first; a run whose checkpoint does not is kept as it is
// (*WorkflowRetentionSkip). It returns the tombstone it recorded, or nil
// when there was nothing to compact.
func (s *Store) CompactWorkflowRun(ctx context.Context, req WorkflowRetentionRun) (*WorkflowRetentionTombstone, error) {
	if err := req.valid(); err != nil {
		return nil, err
	}
	var out *WorkflowRetentionTombstone
	err := s.retentionTransaction(ctx, req.DryRun, func(conn *sql.Conn, now time.Time) error {
		run, err := readWorkflowRun(ctx, conn, req.RunID)
		if err != nil {
			return err
		}
		if err := settledForRetention(ctx, conn, run, req.FinishedBefore); err != nil {
			return err
		}
		checkpoints, attempts, err := checkpointHistory(ctx, conn, run.ID)
		if err != nil {
			return err
		}
		var compact []WorkflowCheckpoint
		var bytes int64
		var prev any
		for i, c := range checkpoints {
			if i == 0 {
				if prev, err = workflow.DecodeValue(c.State); err != nil {
					return &WorkflowRetentionSkip{RunID: run.ID, Reason: fmt.Sprintf("checkpoint 0 cannot be read: %v", err)}
				}
				continue
			}
			rebuilt, err := rebuildCheckpoint(prev, attempts[c.AfterAttempt])
			if err != nil {
				return &WorkflowRetentionSkip{RunID: run.ID, Reason: fmt.Sprintf("checkpoint %d cannot be rebuilt: %v", c.Seq, err)}
			}
			if !c.Compacted {
				stored, err := workflow.DecodeValue(c.State)
				if err != nil || !reflect.DeepEqual(stored, rebuilt) {
					return &WorkflowRetentionSkip{RunID: run.ID, Reason: fmt.Sprintf("checkpoint %d is not its predecessor with attempt %d's output, so it is kept whole", c.Seq, c.AfterAttempt)}
				}
				if i < len(checkpoints)-1 {
					compact = append(compact, c)
					bytes += int64(len(c.State))
				}
			}
			prev = rebuilt
		}
		if len(compact) == 0 {
			return nil
		}
		closeGuard, err := openRetentionGuard(ctx, conn, WorkflowRetentionCompact)
		if err != nil {
			return err
		}
		for _, c := range compact {
			if _, err := conn.ExecContext(ctx, `UPDATE workflow_run_checkpoints SET state='{}',compacted_at=? WHERE run_id=? AND seq=?`, formatTime(now), run.ID, c.Seq); err != nil {
				return fmt.Errorf("compact workflow checkpoint %d: %w", c.Seq, err)
			}
		}
		if err := closeGuard(); err != nil {
			return err
		}
		oldest, newest := compact[0].CreatedAt, compact[len(compact)-1].CreatedAt
		classification := req.Sensitivity.Classification
		if classification == "" {
			classification = workflow.ClassificationOrdinary
		}
		out = &WorkflowRetentionTombstone{SweepID: req.SweepID, Action: WorkflowRetentionCompact, DataClass: WorkflowDataCheckpointStates, Classification: classification,
			WorkspaceID: string(run.WorkspaceID), Workflow: run.Workflow, RunID: run.ID, Records: len(compact), Bytes: bytes,
			Fields: map[string]int{"workflow_run_checkpoints": len(compact)}, OldestAt: &oldest, NewestAt: &newest,
			Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface, RecordedAt: now, DryRun: req.DryRun}
		return recordTombstone(ctx, conn, out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// compactedAttempt is what rebuilding a checkpoint needs of the attempt
// that produced it.
type compactedAttempt struct {
	step   string
	kind   string
	output sql.NullString
}

// checkpointHistory reads a run's checkpoints in order (with whether each
// is compacted) and its step attempts by sequence number.
func checkpointHistory(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowCheckpoint, map[int64]compactedAttempt, error) {
	rows, err := q.QueryContext(ctx, `SELECT seq,after_attempt,next_step,state,created_at,compacted_at FROM workflow_run_checkpoints WHERE run_id=? ORDER BY seq`, runID)
	if err != nil {
		return nil, nil, err
	}
	var checkpoints []WorkflowCheckpoint
	for rows.Next() {
		c := WorkflowCheckpoint{RunID: runID}
		var after sql.NullInt64
		var state, created string
		var compacted sql.NullString
		if err := rows.Scan(&c.Seq, &after, &c.Next, &state, &created, &compacted); err != nil {
			rows.Close()
			return nil, nil, err
		}
		c.AfterAttempt, c.State, c.Compacted = after.Int64, json.RawMessage(state), compacted.Valid
		if c.CreatedAt, err = parseTime(created); err != nil {
			rows.Close()
			return nil, nil, err
		}
		checkpoints = append(checkpoints, c)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	rows, err = q.QueryContext(ctx, `SELECT seq,step_id,kind,output FROM workflow_node_attempts WHERE run_id=? AND item_index IS NULL`, runID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	attempts := map[int64]compactedAttempt{}
	for rows.Next() {
		var seq int64
		var a compactedAttempt
		if err := rows.Scan(&seq, &a.step, &a.kind, &a.output); err != nil {
			return nil, nil, err
		}
		attempts[seq] = a
	}
	return checkpoints, attempts, rows.Err()
}

// rebuildCheckpoint is the state after an attempt: the state before it
// with its output under its step, or the same state after a branch.
func rebuildCheckpoint(prev any, a compactedAttempt) (any, error) {
	before, ok := prev.(map[string]any)
	if !ok {
		return nil, errors.New("the earlier state is not an object")
	}
	if a.kind == workflow.KindBranch {
		return before, nil
	}
	if a.step == "" || !a.output.Valid {
		return nil, errors.New("the attempt that produced it has no output")
	}
	out, err := workflow.DecodeValue([]byte(a.output.String))
	if err != nil {
		return nil, err
	}
	next := make(map[string]any, len(before)+1)
	for k, v := range before {
		next[k] = v
	}
	next[a.step] = out
	return next, nil
}

// rebuildCompacted fills in the state of a detail's compacted checkpoints
// from the attempts that produced them.
func rebuildCompacted(d *WorkflowRunDetail) {
	attempts := map[int64]compactedAttempt{}
	for _, a := range d.Attempts {
		if a.Item != nil {
			continue
		}
		attempts[a.Seq] = compactedAttempt{step: a.Step, kind: a.Kind, output: sql.NullString{String: string(a.Output), Valid: len(a.Output) > 0}}
	}
	var prev any
	for i := range d.Checkpoints {
		c := &d.Checkpoints[i]
		if i == 0 || !c.Compacted {
			v, err := workflow.DecodeValue(c.State)
			if err != nil {
				return
			}
			prev = v
			continue
		}
		rebuilt, err := rebuildCheckpoint(prev, attempts[c.AfterAttempt])
		if err != nil {
			return
		}
		if state, err := workflow.EncodeValue(rebuilt); err == nil {
			c.State = state
		}
		prev = rebuilt
	}
}

// WorkflowRetentionDeletion is one workflow's deletion of the records a
// policy lets go: read notices of its finished runs and of its schedule,
// skipped and failed schedule slots that are not their schedule's latest,
// and schedule controls a later one replaced, each recorded before Before,
// for the versions of one classification.
type WorkflowRetentionDeletion struct {
	WorkspaceID    memory.WorkspaceID
	Workflow       string
	Versions       []int64
	Classification string
	Before         time.Time
	SweepID        string
	Policy         string
	Reason         string
	Actor          string
	Surface        string
	DryRun         bool
}

// DeleteWorkflowRecords deletes what a deletion covers, under the deletion
// guard, recording one tombstone per class of data it removed.
func (s *Store) DeleteWorkflowRecords(ctx context.Context, req WorkflowRetentionDeletion) ([]WorkflowRetentionTombstone, error) {
	check := WorkflowRetentionRun{RunID: "-", SweepID: req.SweepID, Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface}
	if err := check.valid(); err != nil {
		return nil, err
	}
	if len(req.Versions) == 0 {
		return nil, nil
	}
	var out []WorkflowRetentionTombstone
	err := s.retentionTransaction(ctx, req.DryRun, func(conn *sql.Conn, now time.Time) error {
		versions := strings.TrimSuffix(strings.Repeat("?,", len(req.Versions)), ",")
		args := func(prefix ...any) []any {
			for _, v := range req.Versions {
				prefix = append(prefix, v)
			}
			return prefix
		}
		type found struct {
			key   []any
			at    time.Time
			bytes int64
		}
		collect := func(query string, args []any, scan func(*sql.Rows) (found, error)) ([]found, error) {
			rows, err := conn.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var list []found
			for rows.Next() {
				f, err := scan(rows)
				if err != nil {
					return nil, err
				}
				if f.at.Before(req.Before) {
					list = append(list, f)
				}
			}
			return list, rows.Err()
		}
		notices, err := collect(`SELECT n.id,n.created_at,length(n.detail) FROM workflow_notifications n LEFT JOIN workflow_runs r ON r.id=n.run_id
 LEFT JOIN workflow_schedule_slots s ON n.run_id IS NULL AND n.dedup_key='schedule:'||s.workspace_id||'/'||s.workflow||':'||s.timezone||':'||s.slot_key
 WHERE n.workspace_id=? AND n.workflow=? AND n.read_at IS NOT NULL
 AND (n.run_id IS NOT NULL AND r.state IN (`+terminalWorkflowStates+`) AND r.version IN (`+versions+`) OR n.run_id IS NULL AND s.version IN (`+versions+`))`,
			append(args(req.WorkspaceID, req.Workflow), args()...), func(rows *sql.Rows) (found, error) {
				var id, size int64
				var created string
				if err := rows.Scan(&id, &created, &size); err != nil {
					return found{}, err
				}
				at, err := parseTime(created)
				return found{key: []any{id}, at: at, bytes: size}, err
			})
		if err != nil {
			return err
		}
		slots, err := collect(`SELECT s.timezone,s.slot_key,s.decided_at,length(s.detail) FROM workflow_schedule_slots s
 WHERE s.workspace_id=? AND s.workflow=? AND s.outcome IN ('skipped','failed') AND s.version IN (`+versions+`)
 AND EXISTS(SELECT 1 FROM workflow_schedule_slots l WHERE l.workspace_id=s.workspace_id AND l.workflow=s.workflow AND l.timezone=s.timezone
 AND (l.due_at > s.due_at OR l.due_at = s.due_at AND l.slot_key > s.slot_key))
 AND NOT EXISTS(SELECT 1 FROM workflow_notifications n WHERE n.read_at IS NULL
 AND n.dedup_key='schedule:'||s.workspace_id||'/'||s.workflow||':'||s.timezone||':'||s.slot_key)`, args(req.WorkspaceID, req.Workflow), func(rows *sql.Rows) (found, error) {
			var tz, key, decided string
			var size int64
			if err := rows.Scan(&tz, &key, &decided, &size); err != nil {
				return found{}, err
			}
			at, err := parseTime(decided)
			return found{key: []any{tz, key}, at: at, bytes: size}, err
		})
		if err != nil {
			return err
		}
		controls, err := collect(`SELECT c.seq,c.recorded_at,length(c.reason) FROM workflow_schedule_controls c
 WHERE c.workspace_id=? AND c.workflow=? AND c.version IN (`+versions+`)
 AND EXISTS(SELECT 1 FROM workflow_schedule_controls l WHERE l.workspace_id=c.workspace_id AND l.workflow=c.workflow AND l.version=c.version AND l.seq > c.seq)`,
			args(req.WorkspaceID, req.Workflow), func(rows *sql.Rows) (found, error) {
				var seq, size int64
				var recorded string
				if err := rows.Scan(&seq, &recorded, &size); err != nil {
					return found{}, err
				}
				at, err := parseTime(recorded)
				return found{key: []any{seq}, at: at, bytes: size}, err
			})
		if err != nil {
			return err
		}
		if len(notices)+len(slots)+len(controls) == 0 {
			return nil
		}
		closeGuard, err := openRetentionGuard(ctx, conn, WorkflowRetentionDelete)
		if err != nil {
			return err
		}
		for _, group := range []struct {
			class, table, where string
			list                []found
		}{
			{WorkflowDataNotifications, "workflow_notifications", "id=?", notices},
			{WorkflowDataScheduleSlots, "workflow_schedule_slots", "workspace_id=? AND workflow=? AND timezone=? AND slot_key=?", slots},
			{WorkflowDataScheduleControls, "workflow_schedule_controls", "seq=?", controls},
		} {
			if len(group.list) == 0 {
				continue
			}
			t := WorkflowRetentionTombstone{SweepID: req.SweepID, Action: WorkflowRetentionDelete, DataClass: group.class, Classification: req.Classification,
				WorkspaceID: string(req.WorkspaceID), Workflow: req.Workflow, Records: len(group.list), Fields: map[string]int{group.table: len(group.list)},
				Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface, RecordedAt: now, DryRun: req.DryRun}
			for _, f := range group.list {
				key := f.key
				if group.class == WorkflowDataScheduleSlots {
					key = append([]any{req.WorkspaceID, req.Workflow}, key...)
				}
				if _, err := conn.ExecContext(ctx, `DELETE FROM `+group.table+` WHERE `+group.where, key...); err != nil {
					return fmt.Errorf("delete from %s: %w", group.table, err)
				}
				at := f.at
				if t.OldestAt == nil || at.Before(*t.OldestAt) {
					t.OldestAt = &at
				}
				if t.NewestAt == nil || at.After(*t.NewestAt) {
					t.NewestAt = &at
				}
				t.Bytes += f.bytes
			}
			if err := recordTombstone(ctx, conn, &t); err != nil {
				return err
			}
			out = append(out, t)
		}
		return closeGuard()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RedactWorkflowSlots redacts, for the versions of the sensitive
// classification, the details of a workflow's skipped and failed schedule
// slots (and their notices) recorded before Before: a start the schedule
// could not make can quote run-derived errors.
func (s *Store) RedactWorkflowSlots(ctx context.Context, req WorkflowRetentionDeletion) (*WorkflowRetentionTombstone, error) {
	check := WorkflowRetentionRun{RunID: "-", SweepID: req.SweepID, Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface}
	if err := check.valid(); err != nil {
		return nil, err
	}
	if len(req.Versions) == 0 {
		return nil, nil
	}
	var out *WorkflowRetentionTombstone
	err := s.retentionTransaction(ctx, req.DryRun, func(conn *sql.Conn, now time.Time) error {
		versions := strings.TrimSuffix(strings.Repeat("?,", len(req.Versions)), ",")
		args := []any{req.WorkspaceID, req.Workflow, WorkflowRedactMarker}
		for _, v := range req.Versions {
			args = append(args, v)
		}
		rows, err := conn.QueryContext(ctx, `SELECT s.timezone,s.slot_key,s.decided_at,s.detail FROM workflow_schedule_slots s
 WHERE s.workspace_id=? AND s.workflow=? AND s.outcome IN ('skipped','failed') AND s.detail NOT IN ('',?) AND s.version IN (`+versions+`)`, args...)
		if err != nil {
			return err
		}
		type slot struct {
			tz, key, detail string
			at              time.Time
		}
		var list []slot
		for rows.Next() {
			var s slot
			var decided string
			if err := rows.Scan(&s.tz, &s.key, &decided, &s.detail); err != nil {
				rows.Close()
				return err
			}
			if s.at, err = parseTime(decided); err != nil {
				rows.Close()
				return err
			}
			if s.at.Before(req.Before) {
				list = append(list, s)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(list) == 0 {
			return nil
		}
		closeGuard, err := openRetentionGuard(ctx, conn, WorkflowRetentionRedact)
		if err != nil {
			return err
		}
		t := &WorkflowRetentionTombstone{SweepID: req.SweepID, Action: WorkflowRetentionRedact, DataClass: WorkflowDataScheduleSlots, Classification: req.Classification,
			WorkspaceID: string(req.WorkspaceID), Workflow: req.Workflow, Fields: map[string]int{},
			Policy: req.Policy, Reason: req.Reason, Actor: req.Actor, Surface: req.Surface, RecordedAt: now, DryRun: req.DryRun}
		for _, s := range list {
			if _, err := conn.ExecContext(ctx, `UPDATE workflow_schedule_slots SET detail=? WHERE workspace_id=? AND workflow=? AND timezone=? AND slot_key=?`,
				WorkflowRedactMarker, req.WorkspaceID, req.Workflow, s.tz, s.key); err != nil {
				return fmt.Errorf("redact schedule slot %s: %w", s.key, err)
			}
			t.Fields["workflow_schedule_slots"]++
			t.Bytes += int64(len(s.detail))
			key := fmt.Sprintf("schedule:%s/%s:%s:%s", req.WorkspaceID, req.Workflow, s.tz, s.key)
			res, err := conn.ExecContext(ctx, `UPDATE workflow_notifications SET detail=? WHERE dedup_key=? AND detail<>?`, "slot "+s.key+" "+s.tz+": "+WorkflowRedactMarker, key,
				"slot "+s.key+" "+s.tz+": "+WorkflowRedactMarker)
			if err != nil {
				return fmt.Errorf("redact schedule notice %s: %w", s.key, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				t.Fields["workflow_notifications"] += int(n)
			}
			at := s.at
			if t.OldestAt == nil || at.Before(*t.OldestAt) {
				t.OldestAt = &at
			}
			if t.NewestAt == nil || at.After(*t.NewestAt) {
				t.NewestAt = &at
			}
		}
		for _, n := range t.Fields {
			t.Records += n
		}
		if err := closeGuard(); err != nil {
			return err
		}
		out = t
		return recordTombstone(ctx, conn, t)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

const tombstoneColumns = `id,sweep_id,action,data_class,classification,workspace_id,workflow,run_id,records,bytes,fields,oldest_at,newest_at,policy,reason,actor,surface,recorded_at`

func scanTombstone(scanner rowScanner) (WorkflowRetentionTombstone, error) {
	var t WorkflowRetentionTombstone
	var run, oldest, newest sql.NullString
	var fields, recorded string
	if err := scanner.Scan(&t.ID, &t.SweepID, &t.Action, &t.DataClass, &t.Classification, &t.WorkspaceID, &t.Workflow, &run, &t.Records, &t.Bytes, &fields,
		&oldest, &newest, &t.Policy, &t.Reason, &t.Actor, &t.Surface, &recorded); err != nil {
		return t, err
	}
	t.RunID = run.String
	if err := json.Unmarshal([]byte(fields), &t.Fields); err != nil {
		return t, err
	}
	var err error
	if t.OldestAt, err = parseOptionalTime(oldest); err != nil {
		return t, err
	}
	if t.NewestAt, err = parseOptionalTime(newest); err != nil {
		return t, err
	}
	t.RecordedAt, err = parseTime(recorded)
	return t, err
}

// WorkflowRetentionTombstoneFilter narrows a tombstone listing: one run,
// one sweep, at most Limit (default 100, at most 1000), newest first.
type WorkflowRetentionTombstoneFilter struct {
	RunID   string
	SweepID string
	Limit   int
}

// WorkflowRetentionTombstones lists recorded retention actions.
func (s *Store) WorkflowRetentionTombstones(ctx context.Context, f WorkflowRetentionTombstoneFilter) ([]WorkflowRetentionTombstone, error) {
	return retentionTombstones(ctx, s.db, f)
}

func retentionTombstones(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, f WorkflowRetentionTombstoneFilter) ([]WorkflowRetentionTombstone, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	query, args := `SELECT `+tombstoneColumns+` FROM workflow_retention_tombstones WHERE 1=1`, []any{}
	if f.RunID != "" {
		query, args = query+` AND run_id=?`, append(args, f.RunID)
	}
	if f.SweepID != "" {
		query, args = query+` AND sweep_id=?`, append(args, f.SweepID)
	}
	rows, err := q.QueryContext(ctx, query+` ORDER BY id DESC LIMIT ?`, append(args, min(limit, 1000))...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRetentionTombstone{}
	for rows.Next() {
		t, err := scanTombstone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// WorkflowRetentionTotal is how much one action removed of one class of
// data, over every sweep.
type WorkflowRetentionTotal struct {
	Action    string `json:"action"`
	DataClass string `json:"data_class"`
	Actions   int    `json:"actions"`
	Records   int64  `json:"records"`
	Bytes     int64  `json:"bytes"`
}

// WorkflowRetentionState is the sweep's runner lease, its latest summary
// and the totals of every recorded action.
type WorkflowRetentionState struct {
	Runner          string                   `json:"runner,omitempty"`
	RunnerExpiresAt *time.Time               `json:"runner_expires_at,omitempty"`
	LastSweepAt     *time.Time               `json:"last_sweep_at,omitempty"`
	LastSummary     json.RawMessage          `json:"last_summary,omitempty"`
	Totals          []WorkflowRetentionTotal `json:"totals"`
}

// WorkflowRetentionStatus reads the sweep's state and totals.
func (s *Store) WorkflowRetentionStatus(ctx context.Context) (WorkflowRetentionState, error) {
	var out WorkflowRetentionState
	var runner, expires, last, summary sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT runner_holder,runner_expires_at,last_sweep_at,last_summary FROM workflow_retention_state WHERE singleton=1`).
		Scan(&runner, &expires, &last, &summary); err != nil {
		return out, err
	}
	out.Runner = runner.String
	if expires.Valid {
		t, err := parseLeaseTime(expires.String)
		if err != nil {
			return out, err
		}
		out.RunnerExpiresAt = &t
	}
	var err error
	if out.LastSweepAt, err = parseOptionalTime(last); err != nil {
		return out, err
	}
	if summary.Valid {
		out.LastSummary = json.RawMessage(summary.String)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action,data_class,COUNT(*),SUM(records),SUM(bytes) FROM workflow_retention_tombstones GROUP BY action,data_class ORDER BY action,data_class`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out.Totals = []WorkflowRetentionTotal{}
	for rows.Next() {
		var t WorkflowRetentionTotal
		if err := rows.Scan(&t.Action, &t.DataClass, &t.Actions, &t.Records, &t.Bytes); err != nil {
			return out, err
		}
		out.Totals = append(out.Totals, t)
	}
	return out, rows.Err()
}

// AcquireWorkflowRetentionRunner takes or renews the single cross-process
// lease of the retention sweep; false while another holder's is unexpired.
func (s *Store) AcquireWorkflowRetentionRunner(ctx context.Context, holder string, ttl time.Duration) (bool, error) {
	if holder == "" || ttl <= 0 {
		return false, errors.New("the retention runner lease needs a holder and a duration")
	}
	acquired := false
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		res, err := conn.ExecContext(ctx, `UPDATE workflow_retention_state SET runner_holder=?, runner_expires_at=?
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

// ReleaseWorkflowRetentionRunner gives the lease up if holder still has it.
func (s *Store) ReleaseWorkflowRetentionRunner(ctx context.Context, holder string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_retention_state SET runner_holder=NULL, runner_expires_at=NULL WHERE singleton=1 AND runner_holder=?`, holder)
	return err
}

// RecordWorkflowRetentionSweep records a finished sweep's summary, while
// holder still has the runner lease.
func (s *Store) RecordWorkflowRetentionSweep(ctx context.Context, holder string, summary json.RawMessage) error {
	if !json.Valid(summary) {
		return errors.New("a sweep summary is JSON")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_retention_state SET last_sweep_at=?, last_summary=? WHERE singleton=1 AND runner_holder=?`,
		formatTime(s.now()), string(summary), holder)
	return err
}
