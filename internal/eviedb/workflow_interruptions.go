package eviedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Durable interruptions (Stage 14 sub-stage 7 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0009, 0029, 0044). A run waits for its owner without holding a lease:
//
//   - workflow_interruptions: one row per question the run asks or per
//     Effect Intent whose outcome became unknown. A question (an interrupt
//     step) records its step, the checkpoint it waits at, the message, the
//     typed request (the step's declared input) and the answer's schema;
//     its run is waiting_for_human. An effect_reconciliation names the
//     intent and presents its evidence (provider, key, input, why its
//     outcome is unknown) with the owner's possible answers; its run needs
//     reconciliation. Each carries the run's originating session so that
//     session can show it. Immutable.
//   - workflow_interruption_resolutions: how an interruption closed,
//     recorded once: the owner's answer (with the answering surface and,
//     when answered from a session, that session), the provider's
//     reconciliation of an unknown effect, or a withdrawal when the owner
//     cancels the run (sub-stage 8). Immutable. An effect_reconciliation
//     closes only in the transaction that records its Effect Receipt, so
//     the owner's reconciliation and the receipt are one durable record.
//
// While a question is open nothing advances, pauses, resumes or fails its
// run, and no scheduler finds it due (ADR 0029); any later session's
// surface may answer it, under its own claim of the run's lease. The
// runtime validates an answer against the step's schema before it is
// recorded, and again before the graph advances.

var (
	// ErrWorkflowInterruptionOpen refuses what an open question blocks.
	ErrWorkflowInterruptionOpen = errors.New("the workflow run waits for the owner's answer to an interruption")
	// ErrWorkflowInterruptionClosed refuses answering an interruption that
	// is not open, or not a question.
	ErrWorkflowInterruptionClosed = errors.New("the workflow interruption is not an open question")
)

// WorkflowInterruptionKind is what an interruption waits for.
type WorkflowInterruptionKind string

const (
	// WorkflowInterruptionQuestion is an interrupt step's typed question.
	WorkflowInterruptionQuestion WorkflowInterruptionKind = "question"
	// WorkflowInterruptionEffect is an Effect Intent whose outcome is
	// unknown, presented for reconciliation.
	WorkflowInterruptionEffect WorkflowInterruptionKind = "effect_reconciliation"
)

// WorkflowInterruptionResolutionKind is how an interruption closed.
type WorkflowInterruptionResolutionKind string

const (
	// WorkflowInterruptionAnswered: the owner answered.
	WorkflowInterruptionAnswered WorkflowInterruptionResolutionKind = "answered"
	// WorkflowInterruptionReconciled: the provider's reconciliation settled
	// the unknown effect first.
	WorkflowInterruptionReconciled WorkflowInterruptionResolutionKind = "reconciled"
	// WorkflowInterruptionWithdrawn: the question was withdrawn unanswered
	// (cancellation, sub-stage 8).
	WorkflowInterruptionWithdrawn WorkflowInterruptionResolutionKind = "withdrawn"
)

// WorkflowInterruption is one interruption with how it closed, if it did.
type WorkflowInterruption struct {
	RunID       string                   `json:"run_id"`
	Seq         int64                    `json:"seq"`
	WorkspaceID memory.WorkspaceID       `json:"workspace_id"`
	Workflow    string                   `json:"workflow"`
	Kind        WorkflowInterruptionKind `json:"kind"`
	Step        string                   `json:"step"`
	Checkpoint  int64                    `json:"checkpoint"`
	// IntentSeq is the Effect Intent an effect_reconciliation concerns.
	IntentSeq      int64           `json:"intent_seq,omitempty"`
	Message        string          `json:"message"`
	Request        json.RawMessage `json:"request"`
	ResponseSchema json.RawMessage `json:"response_schema"`
	// SessionID is the run's originating session, which shows it.
	SessionID   memory.SessionID                `json:"session_id,omitempty"`
	LeaseHolder string                          `json:"lease_holder"`
	LeaseToken  int64                           `json:"lease_token"`
	OpenedAt    time.Time                       `json:"opened_at"`
	Resolution  *WorkflowInterruptionResolution `json:"resolution,omitempty"`
}

// Open reports an interruption still waiting.
func (i WorkflowInterruption) Open() bool { return i.Resolution == nil }

// WorkflowInterruptionResolution is how an interruption closed.
type WorkflowInterruptionResolution struct {
	Kind   WorkflowInterruptionResolutionKind `json:"kind"`
	Answer json.RawMessage                    `json:"answer,omitempty"`
	// Actor is "owner" or "runtime"; Surface the owner's surface and
	// SessionID the session the owner answered from, if any.
	Actor       string           `json:"actor"`
	Surface     string           `json:"surface,omitempty"`
	SessionID   memory.SessionID `json:"session_id,omitempty"`
	Detail      string           `json:"detail,omitempty"`
	LeaseHolder string           `json:"lease_holder"`
	LeaseToken  int64            `json:"lease_token"`
	RecordedAt  time.Time        `json:"recorded_at"`
}

// maxInterruptionText bounds an interruption's message and a resolution's
// detail; maxInterruptionJSON its request, schema and answer.
const (
	maxInterruptionText = 4000
	maxInterruptionJSON = 256 << 10
)

// effectAnswerSchema is the owner's answer to an unknown effect: whether
// it happened, the evidence and, when it happened, its output under the
// step's Capability Contract.
const effectAnswerSchema = `{"additionalProperties":false,"properties":{"evidence":{"maxLength":2000,"type":"string"},` +
	`"outcome":{"enum":["happened","did-not-happen"],"type":"string"},` +
	`"output":{"description":"the effect's output under its Capability Contract, when it happened","type":"object"}},"required":["outcome"],"type":"object"}`

func ensureWorkflowInterruptionSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_interruptions (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 kind TEXT NOT NULL CHECK(kind IN ('question','effect_reconciliation')),
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 checkpoint INTEGER NOT NULL CHECK(checkpoint >= 0),
 intent_seq INTEGER,
 message TEXT NOT NULL CHECK(length(message) BETWEEN 1 AND 4000),
 request TEXT NOT NULL CHECK(json_valid(request) AND json_type(request) = 'object'),
 response_schema TEXT NOT NULL CHECK(json_valid(response_schema) AND json_type(response_schema) = 'object'),
 session_id TEXT REFERENCES sessions(id),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 opened_at TEXT NOT NULL,
 PRIMARY KEY(run_id, seq),
 FOREIGN KEY(run_id, checkpoint) REFERENCES workflow_run_checkpoints(run_id, seq),
 FOREIGN KEY(run_id, intent_seq) REFERENCES workflow_effect_intents(run_id, seq),
 CHECK((kind = 'effect_reconciliation') = (intent_seq IS NOT NULL)));
 CREATE UNIQUE INDEX IF NOT EXISTS workflow_interruptions_one_per_intent ON workflow_interruptions(run_id, intent_seq) WHERE intent_seq IS NOT NULL;
 CREATE UNIQUE INDEX IF NOT EXISTS workflow_interruptions_one_question_per_step ON workflow_interruptions(run_id, checkpoint, step_id) WHERE kind = 'question';
 CREATE INDEX IF NOT EXISTS workflow_interruptions_by_session ON workflow_interruptions(session_id) WHERE session_id IS NOT NULL;
 CREATE TRIGGER IF NOT EXISTS workflow_interruptions_no_update BEFORE UPDATE ON workflow_interruptions
 BEGIN SELECT RAISE(ABORT,'workflow interruptions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_interruptions_no_delete BEFORE DELETE ON workflow_interruptions
 BEGIN SELECT RAISE(ABORT,'workflow interruptions are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_interruption_resolutions (
 run_id TEXT NOT NULL,
 interruption_seq INTEGER NOT NULL,
 resolution TEXT NOT NULL CHECK(resolution IN ('answered','reconciled','withdrawn')),
 answer TEXT CHECK(answer IS NULL OR (json_valid(answer) AND json_type(answer) = 'object')),
 actor TEXT NOT NULL CHECK(actor IN ('owner','runtime')),
 surface TEXT NOT NULL CHECK(surface IN ('','cli','web')),
 session_id TEXT REFERENCES sessions(id),
 detail TEXT NOT NULL CHECK(length(detail) <= 4000),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(run_id, interruption_seq),
 FOREIGN KEY(run_id, interruption_seq) REFERENCES workflow_interruptions(run_id, seq),
 CHECK((actor = 'owner') = (surface <> '')),
 CHECK(resolution <> 'answered' OR (actor = 'owner' AND answer IS NOT NULL)),
 CHECK(resolution <> 'reconciled' OR (actor = 'runtime' AND answer IS NOT NULL)),
 CHECK(resolution <> 'withdrawn' OR answer IS NULL),
 CHECK(session_id IS NULL OR actor = 'owner'));
 CREATE TRIGGER IF NOT EXISTS workflow_interruption_resolutions_no_update BEFORE UPDATE ON workflow_interruption_resolutions
 BEGIN SELECT RAISE(ABORT,'workflow interruption resolutions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_interruption_resolutions_no_delete BEFORE DELETE ON workflow_interruption_resolutions
 BEGIN SELECT RAISE(ABORT,'workflow interruption resolutions are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow interruption schema: %w", err)
	}
	return nil
}

const interruptionColumns = `i.run_id,i.seq,r.workspace_id,r.workflow,i.kind,i.step_id,i.checkpoint,i.intent_seq,i.message,i.request,i.response_schema,i.session_id,
 i.lease_holder,i.lease_token,i.opened_at,
 x.resolution,x.answer,x.actor,x.surface,x.session_id,x.detail,x.lease_holder,x.lease_token,x.recorded_at`

const interruptionFrom = ` FROM workflow_interruptions i JOIN workflow_runs r ON r.id=i.run_id
 LEFT JOIN workflow_interruption_resolutions x ON x.run_id=i.run_id AND x.interruption_seq=i.seq`

func scanWorkflowInterruption(scanner rowScanner) (WorkflowInterruption, error) {
	var i WorkflowInterruption
	var intent, xToken sql.NullInt64
	var request, schema, opened string
	var session, xKind, xAnswer, xActor, xSurface, xSession, xDetail, xHolder, xRecorded sql.NullString
	if err := scanner.Scan(&i.RunID, &i.Seq, &i.WorkspaceID, &i.Workflow, &i.Kind, &i.Step, &i.Checkpoint, &intent, &i.Message, &request, &schema, &session,
		&i.LeaseHolder, &i.LeaseToken, &opened,
		&xKind, &xAnswer, &xActor, &xSurface, &xSession, &xDetail, &xHolder, &xToken, &xRecorded); err != nil {
		return i, err
	}
	i.IntentSeq, i.Request, i.ResponseSchema, i.SessionID = intent.Int64, json.RawMessage(request), json.RawMessage(schema), memory.SessionID(session.String)
	var err error
	if i.OpenedAt, err = parseTime(opened); err != nil {
		return i, err
	}
	if xKind.Valid {
		res := &WorkflowInterruptionResolution{Kind: WorkflowInterruptionResolutionKind(xKind.String), Actor: xActor.String, Surface: xSurface.String,
			SessionID: memory.SessionID(xSession.String), Detail: xDetail.String, LeaseHolder: xHolder.String, LeaseToken: xToken.Int64}
		if xAnswer.Valid {
			res.Answer = json.RawMessage(xAnswer.String)
		}
		if res.RecordedAt, err = parseTime(xRecorded.String); err != nil {
			return i, err
		}
		i.Resolution = res
	}
	return i, nil
}

func queryInterruptions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, where string, args ...any) ([]WorkflowInterruption, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+interruptionColumns+interruptionFrom+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowInterruption{}
	for rows.Next() {
		i, err := scanWorkflowInterruption(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func workflowInterruptions(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowInterruption, error) {
	return queryInterruptions(ctx, q, `i.run_id=? ORDER BY i.seq`, runID)
}

func readWorkflowInterruption(ctx context.Context, q queryRower, runID string, seq int64) (WorkflowInterruption, error) {
	i, err := scanWorkflowInterruption(q.QueryRowContext(ctx, `SELECT `+interruptionColumns+interruptionFrom+` WHERE i.run_id=? AND i.seq=?`, runID, seq))
	if errors.Is(err, sql.ErrNoRows) {
		return i, fmt.Errorf("%w: run %s has no interruption #%d", ErrWorkflowInterruptionClosed, runID, seq)
	}
	return i, err
}

// WorkflowInterruptions lists a run's interruptions in order, each with
// how it closed.
func (s *Store) WorkflowInterruptions(ctx context.Context, runID string) ([]WorkflowInterruption, error) {
	return workflowInterruptions(ctx, s.db, runID)
}

// WorkflowInterruptionFilter narrows a listing: one Workspace, one
// originating session, one run; open ones only unless All; at most Limit
// (default 50, at most 200), in the order they were opened.
type WorkflowInterruptionFilter struct {
	WorkspaceID memory.WorkspaceID
	SessionID   memory.SessionID
	RunID       string
	All         bool
	Limit       int
}

// ListWorkflowInterruptions lists interruptions across runs: by default
// the open ones, which are what the owner is asked now.
func (s *Store) ListWorkflowInterruptions(ctx context.Context, f WorkflowInterruptionFilter) ([]WorkflowInterruption, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	where, args := `1=1`, []any{}
	if f.WorkspaceID != "" {
		where += ` AND r.workspace_id=?`
		args = append(args, f.WorkspaceID)
	}
	if f.SessionID != "" {
		where += ` AND i.session_id=?`
		args = append(args, f.SessionID)
	}
	if f.RunID != "" {
		where += ` AND i.run_id=?`
		args = append(args, f.RunID)
	}
	if !f.All {
		where += ` AND x.run_id IS NULL`
	}
	return queryInterruptions(ctx, s.db, where+` ORDER BY i.rowid LIMIT ?`, append(args, limit)...)
}

// openQuestion is the run's open question, if any.
func openQuestion(ctx context.Context, q queryRower, runID string) (int64, string, bool, error) {
	var seq int64
	var step string
	err := q.QueryRowContext(ctx, `SELECT i.seq,i.step_id FROM workflow_interruptions i WHERE i.run_id=? AND i.kind='question'
 AND NOT EXISTS(SELECT 1 FROM workflow_interruption_resolutions x WHERE x.run_id=i.run_id AND x.interruption_seq=i.seq) ORDER BY i.seq LIMIT 1`, runID).Scan(&seq, &step)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, "", false, nil
	case err != nil:
		return 0, "", false, err
	}
	return seq, step, true, nil
}

// requireNoOpenQuestion refuses while the run waits for the owner's answer.
func requireNoOpenQuestion(ctx context.Context, q queryRower, runID string) error {
	seq, step, open, err := openQuestion(ctx, q, runID)
	if err != nil || !open {
		return err
	}
	return fmt.Errorf("%w (steps.%s, interruption #%d)", ErrWorkflowInterruptionOpen, step, seq)
}

func clipText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit - len("…")
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

// interruptionRow is one interruption to open.
type interruptionRow struct {
	kind       WorkflowInterruptionKind
	step       string
	checkpoint int64
	intent     int64
	message    string
	request    json.RawMessage
	schema     json.RawMessage
}

// openInterruption records an interruption of the run under the lease,
// carrying the run's originating session.
func openInterruption(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, row interruptionRow, now time.Time) (int64, error) {
	var session sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT session_id FROM workflow_runs WHERE id=?`, lease.RunID).Scan(&session); err != nil {
		return 0, err
	}
	var intent any
	if row.intent > 0 {
		intent = row.intent
	}
	var sessionID any
	if session.Valid {
		sessionID = session.String
	}
	var seq int64
	err := conn.QueryRowContext(ctx, `INSERT INTO workflow_interruptions(run_id,seq,kind,step_id,checkpoint,intent_seq,message,request,response_schema,session_id,lease_holder,lease_token,opened_at)
 VALUES(?,(SELECT COALESCE(MAX(seq),0)+1 FROM workflow_interruptions WHERE run_id=?),?,?,?,?,?,?,?,?,?,?,?) RETURNING seq`,
		lease.RunID, lease.RunID, string(row.kind), row.step, row.checkpoint, intent, clipText(row.message, maxInterruptionText), string(row.request), string(row.schema),
		sessionID, lease.Holder, lease.Token, formatTime(now)).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("record workflow interruption: %w", err)
	}
	return seq, nil
}

// resolutionRow is how one interruption closes.
type resolutionRow struct {
	kind    WorkflowInterruptionResolutionKind
	answer  json.RawMessage
	actor   string
	surface string
	session memory.SessionID
	detail  string
}

func resolveInterruption(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, seq int64, r resolutionRow, now time.Time) error {
	var answer, session any
	if len(r.answer) > 0 {
		answer = string(r.answer)
	}
	if r.session != "" {
		session = string(r.session)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_interruption_resolutions(run_id,interruption_seq,resolution,answer,actor,surface,session_id,detail,lease_holder,lease_token,recorded_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?)`, lease.RunID, seq, string(r.kind), answer, r.actor, r.surface, session, clipText(r.detail, maxInterruptionText),
		lease.Holder, lease.Token, formatTime(now)); err != nil {
		return fmt.Errorf("record workflow interruption resolution: %w", err)
	}
	return nil
}

// requireWorkspaceSession refuses an answering session outside the run's
// Workspace.
func requireWorkspaceSession(ctx context.Context, q queryRower, run WorkflowRun, session memory.SessionID) error {
	if session == "" {
		return nil
	}
	var ws sql.NullString
	err := q.QueryRowContext(ctx, `SELECT workspace_id FROM sessions WHERE id=?`, session).Scan(&ws)
	if errors.Is(err, sql.ErrNoRows) || err == nil && ws.String != string(run.WorkspaceID) {
		return ErrWorkflowRunSession
	}
	return err
}

func validInterruptionJSON(data json.RawMessage) bool {
	return len(data) <= maxInterruptionJSON && workflowJSONObject(data)
}

// WorkflowQuestion is an interrupt step's question: the step at the run's
// latest checkpoint, what the owner is asked, the typed request (the
// step's declared input) and the answer's schema.
type WorkflowQuestion struct {
	RunID          string
	Checkpoint     int64
	Step           string
	Message        string
	Request        json.RawMessage
	ResponseSchema json.RawMessage
}

// AskWorkflowQuestion records, under the run's current lease, that the run
// waits for the owner's answer at its latest checkpoint: the question is
// recorded and the run is waiting_for_human, which no scheduler resumes.
// No attempt may be in flight and no effect unresolved; the step must be
// the one the checkpoint names, asked once.
func (s *Store) AskWorkflowQuestion(ctx context.Context, lease WorkflowRunLease, q WorkflowQuestion) (WorkflowInterruption, error) {
	switch {
	case q.RunID != lease.RunID:
		return WorkflowInterruption{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	case strings.TrimSpace(q.Message) == "" || q.Step == "":
		return WorkflowInterruption{}, errors.New("a question names its step and what the owner is asked")
	case !validInterruptionJSON(q.Request) || !validInterruptionJSON(q.ResponseSchema):
		return WorkflowInterruption{}, errors.New("a question needs its request and answer schema as JSON objects")
	}
	var seq int64
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, q.RunID)
		if err != nil {
			return err
		}
		if run.State == WorkflowRunCancelling {
			return fmt.Errorf("%w (steps.%s was not asked)", ErrWorkflowRunCancelling, q.Step)
		}
		if run.State != WorkflowRunQueued && run.State != WorkflowRunRunning {
			return fmt.Errorf("%w: the run is %s", ErrWorkflowRunConflict, run.State)
		}
		var inFlight int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'`, q.RunID).Scan(&inFlight); err != nil {
			return err
		}
		if inFlight > 0 {
			return fmt.Errorf("%w: an attempt is in flight", ErrWorkflowRunConflict)
		}
		if err := requireEffectsResolved(ctx, conn, q.RunID); err != nil {
			return err
		}
		c, err := latestCheckpoint(ctx, conn, q.RunID)
		if err != nil {
			return err
		}
		if c.Seq != q.Checkpoint || c.Next != q.Step {
			return ErrWorkflowRunConflict
		}
		var asked int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_interruptions WHERE run_id=? AND checkpoint=? AND step_id=? AND kind='question'`,
			q.RunID, q.Checkpoint, q.Step).Scan(&asked); err != nil {
			return err
		}
		if asked > 0 {
			return fmt.Errorf("%w: steps.%s was already asked at checkpoint %d", ErrWorkflowRunConflict, q.Step, q.Checkpoint)
		}
		// A question is a node: it applies run admission like any other
		// (sub-stage 8).
		if err := authorizeWorkflowRun(ctx, conn, run); err != nil {
			return err
		}
		if seq, err = openInterruption(ctx, conn, lease, interruptionRow{kind: WorkflowInterruptionQuestion, step: q.Step, checkpoint: q.Checkpoint,
			message: q.Message, request: q.Request, schema: q.ResponseSchema}, now); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='waiting_for_human',error='',not_before=NULL,started_at=COALESCE(started_at,?),updated_at=? WHERE id=?`,
			formatTime(now), formatTime(now), q.RunID)
		return err
	})
	if err != nil {
		return WorkflowInterruption{}, err
	}
	return readWorkflowInterruption(ctx, s.db, q.RunID, seq)
}

// WorkflowAnswer is the owner's answer to a question: the run, the
// interruption, the answer (validated against the step's schema by the
// runtime), the surface it came from and, when answered from a session,
// that session (any session of the run's Workspace).
type WorkflowAnswer struct {
	RunID     string
	Seq       int64
	Answer    json.RawMessage
	Surface   string
	SessionID memory.SessionID
}

// AnswerWorkflowQuestion records the owner's answer once, under the
// answering surface's claim of the run's lease, and lets the run continue:
// it is running again, and the runtime turns the answer into the step's
// output after validating it once more.
func (s *Store) AnswerWorkflowQuestion(ctx context.Context, lease WorkflowRunLease, a WorkflowAnswer) (WorkflowRun, error) {
	switch {
	case a.Surface != "cli" && a.Surface != "web":
		return WorkflowRun{}, ErrWorkflowApprover
	case a.RunID != lease.RunID:
		return WorkflowRun{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	case !validInterruptionJSON(a.Answer):
		return WorkflowRun{}, errors.New("an answer is one JSON object")
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, a.RunID)
		if err != nil {
			return err
		}
		i, err := readWorkflowInterruption(ctx, conn, a.RunID, a.Seq)
		if err != nil {
			return err
		}
		if i.Kind != WorkflowInterruptionQuestion || !i.Open() || run.State != WorkflowRunWaitingForHuman {
			return fmt.Errorf("%w: interruption #%d is a %s that is %s, and the run is %s", ErrWorkflowInterruptionClosed, i.Seq, i.Kind, interruptionStatus(i), run.State)
		}
		if err := requireWorkspaceSession(ctx, conn, run, a.SessionID); err != nil {
			return err
		}
		if err := resolveInterruption(ctx, conn, lease, i.Seq, resolutionRow{kind: WorkflowInterruptionAnswered, answer: a.Answer, actor: "owner",
			surface: a.Surface, session: a.SessionID}, now); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='running',error='',not_before=NULL,updated_at=? WHERE id=?`, formatTime(now), a.RunID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, a.RunID)
}

func interruptionStatus(i WorkflowInterruption) string {
	if i.Resolution == nil {
		return "open"
	}
	return string(i.Resolution.Kind)
}

// WithdrawWorkflowQuestion records, under the run's current lease, that an
// open question of a run the owner cancelled (it is cancelling) is
// withdrawn unanswered; FinishWorkflowCancellation does the same for
// every open question in the transaction that ends the run cancelled
// (sub-stage 8). An effect whose outcome is unknown cannot be withdrawn:
// it closes only with its receipt (ErrWorkflowEffectUnresolved).
func (s *Store) WithdrawWorkflowQuestion(ctx context.Context, lease WorkflowRunLease, runID string, seq int64, reason string) (WorkflowRun, error) {
	if strings.TrimSpace(reason) == "" {
		return WorkflowRun{}, errors.New("a withdrawal needs its reason")
	}
	if runID != lease.RunID {
		return WorkflowRun{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, runID)
		if err != nil {
			return err
		}
		i, err := readWorkflowInterruption(ctx, conn, runID, seq)
		if err != nil {
			return err
		}
		if i.Kind == WorkflowInterruptionEffect {
			return fmt.Errorf("%w (steps.%s, Effect Intent #%d): an unknown effect is reconciled, never withdrawn", ErrWorkflowEffectUnresolved, i.Step, i.IntentSeq)
		}
		if !i.Open() || run.State != WorkflowRunCancelling {
			return fmt.Errorf("%w: interruption #%d is %s, and the run is %s, not cancelling", ErrWorkflowInterruptionClosed, i.Seq, interruptionStatus(i), run.State)
		}
		return resolveInterruption(ctx, conn, lease, i.Seq, resolutionRow{kind: WorkflowInterruptionWithdrawn, actor: "runtime", detail: reason}, now)
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, runID)
}

// effectInterruption is the interruption that presents an Effect Intent
// whose outcome is unknown to the owner, with its evidence.
func effectInterruption(i WorkflowEffectIntent, reason string) (interruptionRow, error) {
	request, err := json.Marshal(map[string]any{
		"intent": i.Seq, "capability": i.Capability, "contract": i.Contract, "plugin": i.Plugin, "version": i.Version,
		"connection": i.Connection, "resource": i.Resource, "idempotency_key": i.IdempotencyKey, "input": i.Input,
		"reason": reason, "reconciled_by": string(i.Reconcile),
	})
	if err != nil {
		return interruptionRow{}, err
	}
	message := fmt.Sprintf("Did steps.%s's effect happen? Effect Intent #%d (%s on %s %s) has an unknown outcome; answer whether it happened, with your evidence and, if it happened, its output.",
		i.Step, i.Seq, i.Capability, i.Connection, i.Resource)
	if i.Reconcile == WorkflowEffectReconcileByProvider {
		message += " Evie asks the provider first; you may answer meanwhile."
	}
	return interruptionRow{kind: WorkflowInterruptionEffect, step: i.Step, checkpoint: i.Checkpoint, intent: i.Seq, message: message,
		request: request, schema: json.RawMessage(effectAnswerSchema)}, nil
}

// effectInterruptionSeq is the interruption of an Effect Intent, opening
// it when an earlier Evie recorded the intent unknown without one.
func effectInterruptionSeq(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, i WorkflowEffectIntent, now time.Time) (int64, error) {
	var seq int64
	err := conn.QueryRowContext(ctx, `SELECT seq FROM workflow_interruptions WHERE run_id=? AND intent_seq=?`, i.RunID, i.Seq).Scan(&seq)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return seq, err
	}
	reason := "its outcome is unknown"
	if i.Unknown != nil {
		reason = i.Unknown.Reason
	}
	row, err := effectInterruption(i, reason)
	if err != nil {
		return 0, err
	}
	return openInterruption(ctx, conn, lease, row, now)
}

// ensureEffectInterruptions opens the interruption of every unknown
// Effect Intent of the run that has none (recorded unknown by an Evie
// before sub-stage 7), so the owner sees each.
func ensureEffectInterruptions(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, now time.Time) error {
	intents, err := workflowEffectIntents(ctx, conn, lease.RunID)
	if err != nil {
		return err
	}
	for _, i := range intents {
		if i.State() != WorkflowEffectUnknown {
			continue
		}
		if _, err := effectInterruptionSeq(ctx, conn, lease, i, now); err != nil {
			return err
		}
	}
	return nil
}

// effectAnswer is the answer document a reconciliation records: whether
// the effect happened, the evidence and, when it happened, its output.
func effectAnswer(outcome WorkflowEffectState, evidence string, response json.RawMessage) (json.RawMessage, error) {
	doc := map[string]any{"outcome": "did-not-happen"}
	if outcome == WorkflowEffectSucceeded {
		doc["outcome"] = "happened"
		doc["output"] = response
	}
	if evidence != "" {
		doc["evidence"] = evidence
	}
	// Canonical like the owner's answers: sorted keys, no HTML escaping.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte("\n"))), nil
}

// questionWaits is the time a run spent waiting for answers to its
// questions, from each question's opening to its answer.
func questionWaits(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) (time.Duration, error) {
	rows, err := q.QueryContext(ctx, `SELECT i.opened_at,x.recorded_at FROM workflow_interruptions i JOIN workflow_interruption_resolutions x
 ON x.run_id=i.run_id AND x.interruption_seq=i.seq WHERE i.run_id=? AND i.kind='question' AND x.resolution='answered'`, runID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total time.Duration
	for rows.Next() {
		var opened, answered string
		if err := rows.Scan(&opened, &answered); err != nil {
			return 0, err
		}
		from, err := parseTime(opened)
		if err != nil {
			return 0, err
		}
		to, err := parseTime(answered)
		if err != nil {
			return 0, err
		}
		total += max(to.Sub(from), 0)
	}
	return total, rows.Err()
}
