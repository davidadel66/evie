package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AI step model calls (Stage 14 sub-stage 7 of
// docs/memory-architecture.md, 2026-10-04; ADR 0010, 0019):
//
//   - workflow_model_calls: one row per AI node attempt (joined to it by its
//     sequence number, like Effect Intents, so the attempt table can still
//     be rebuilt), recorded with the
//     attempt in one fenced transaction before the model is called: the
//     pinned Model Policy (name, version, provider, exact model and
//     parameters), the digests of the reviewed prompt, the output schema
//     and the exact request, and the estimate of its tokens and cost. Its
//     usage (provider requests sent, tokens, whether the provider reported
//     them, the cost and where its price came from), the model's text
//     answer and any error are recorded once, with the attempt's outcome
//     and next checkpoint, in one transaction. A call whose attempt ended
//     without them (a crash, a lost lease, a stopped runtime) may still
//     have been billed: it counts at its estimate.
//
// The proposal itself is the attempt's output, appended like every
// attempt's and never rewritten; a correction is a later attempt or an
// answer, never an edit (ADR 0044).

// ErrWorkflowModelCallOpen refuses ending an AI attempt without recording
// its model call.
var ErrWorkflowModelCallOpen = errors.New("the AI attempt's model call has not been recorded")

// WorkflowModelCallState is a model call's state, derived from its row and
// its attempt.
type WorkflowModelCallState string

const (
	WorkflowModelCallPending     WorkflowModelCallState = "pending"
	WorkflowModelCallFinished    WorkflowModelCallState = "finished"
	WorkflowModelCallInterrupted WorkflowModelCallState = "interrupted"
)

// maxModelResponse bounds the model's text answer a call keeps.
const maxModelResponse = 64 << 10

// WorkflowModelCall is one AI attempt's call to its pinned model.
type WorkflowModelCall struct {
	RunID                 string               `json:"run_id"`
	AttemptSeq            int64                `json:"attempt_seq"`
	AttemptState          WorkflowAttemptState `json:"attempt_state"`
	Step                  string               `json:"step"`
	Policy                string               `json:"policy"`
	PolicyVersion         int                  `json:"policy_version"`
	Provider              string               `json:"provider"`
	Model                 string               `json:"model"`
	Parameters            json.RawMessage      `json:"parameters"`
	PromptSHA256          string               `json:"prompt_sha256"`
	SchemaSHA256          string               `json:"schema_sha256"`
	RequestSHA256         string               `json:"request_sha256"`
	EstimatedInputTokens  int64                `json:"estimated_input_tokens"`
	EstimatedOutputTokens int64                `json:"estimated_output_tokens"`
	EstimatedMicroUSD     int64                `json:"estimated_micro_usd"`
	LeaseHolder           string               `json:"lease_holder"`
	LeaseToken            int64                `json:"lease_token"`
	StartedAt             time.Time            `json:"started_at"`
	FinishedAt            *time.Time           `json:"finished_at,omitempty"`
	// Requests is how many provider requests the attempt sent (bounded
	// transport retries included).
	Requests      int    `json:"requests,omitempty"`
	InputTokens   int64  `json:"input_tokens,omitempty"`
	OutputTokens  int64  `json:"output_tokens,omitempty"`
	UsageReported bool   `json:"usage_reported,omitempty"`
	MicroUSD      int64  `json:"micro_usd,omitempty"`
	PricingSource string `json:"pricing_source,omitempty"`
	Response      string `json:"response,omitempty"`
	Error         string `json:"error,omitempty"`
}

// State derives the call's state.
func (c WorkflowModelCall) State() WorkflowModelCallState {
	switch {
	case c.FinishedAt != nil:
		return WorkflowModelCallFinished
	case c.AttemptState == WorkflowAttemptRunning:
		return WorkflowModelCallPending
	}
	return WorkflowModelCallInterrupted
}

// Tokens is what the call counts against the run's limits.max_tokens: its
// usage once recorded, its estimate otherwise (it may have been billed).
func (c WorkflowModelCall) Tokens() int64 {
	if c.FinishedAt != nil {
		return c.InputTokens + c.OutputTokens
	}
	return c.EstimatedInputTokens + c.EstimatedOutputTokens
}

// Cost is the call's cost in millionths of a dollar, at its estimate until
// its usage is recorded.
func (c WorkflowModelCall) Cost() int64 {
	if c.FinishedAt != nil {
		return c.MicroUSD
	}
	return c.EstimatedMicroUSD
}

func ensureWorkflowModelCallSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_model_calls (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 attempt_seq INTEGER NOT NULL CHECK(attempt_seq > 0),
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 policy TEXT NOT NULL CHECK(length(policy) BETWEEN 1 AND 64),
 policy_version INTEGER NOT NULL CHECK(policy_version > 0),
 provider TEXT NOT NULL CHECK(length(provider) BETWEEN 1 AND 64),
 model TEXT NOT NULL CHECK(length(model) BETWEEN 1 AND 256),
 parameters TEXT NOT NULL CHECK(json_valid(parameters) AND json_type(parameters) = 'object'),
 prompt_sha256 TEXT NOT NULL CHECK(length(prompt_sha256) = 71),
 schema_sha256 TEXT NOT NULL CHECK(length(schema_sha256) = 71),
 request_sha256 TEXT NOT NULL CHECK(length(request_sha256) = 71),
 estimated_input_tokens INTEGER NOT NULL CHECK(estimated_input_tokens >= 0),
 estimated_output_tokens INTEGER NOT NULL CHECK(estimated_output_tokens >= 0),
 estimated_micro_usd INTEGER NOT NULL CHECK(estimated_micro_usd >= 0),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 started_at TEXT NOT NULL,
 finished_at TEXT,
 requests INTEGER NOT NULL CHECK(requests >= 0),
 input_tokens INTEGER NOT NULL CHECK(input_tokens >= 0),
 output_tokens INTEGER NOT NULL CHECK(output_tokens >= 0),
 usage_reported INTEGER NOT NULL CHECK(usage_reported IN (0,1)),
 micro_usd INTEGER NOT NULL CHECK(micro_usd >= 0),
 pricing_source TEXT NOT NULL CHECK(length(pricing_source) <= 64),
 response TEXT NOT NULL CHECK(length(response) <= 65536),
 error TEXT NOT NULL CHECK(length(error) <= 4000),
 PRIMARY KEY(run_id, attempt_seq),
 CHECK(finished_at IS NOT NULL OR (requests = 0 AND input_tokens = 0 AND output_tokens = 0 AND micro_usd = 0 AND response = '' AND error = '')));
 CREATE TRIGGER IF NOT EXISTS workflow_model_calls_finished BEFORE UPDATE ON workflow_model_calls
 WHEN OLD.finished_at IS NOT NULL
 BEGIN SELECT RAISE(ABORT,'finished workflow model calls are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_model_calls_identity BEFORE UPDATE OF run_id,attempt_seq,step_id,policy,policy_version,provider,model,parameters,
 prompt_sha256,schema_sha256,request_sha256,estimated_input_tokens,estimated_output_tokens,estimated_micro_usd,lease_holder,lease_token,started_at ON workflow_model_calls
 BEGIN SELECT RAISE(ABORT,'a workflow model call''s identity is immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_model_calls_no_delete BEFORE DELETE ON workflow_model_calls
 BEGIN SELECT RAISE(ABORT,'workflow model calls are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow model call schema: %w", err)
	}
	return nil
}

const modelCallColumns = `m.run_id,m.attempt_seq,a.state,m.step_id,m.policy,m.policy_version,m.provider,m.model,m.parameters,m.prompt_sha256,m.schema_sha256,m.request_sha256,
 m.estimated_input_tokens,m.estimated_output_tokens,m.estimated_micro_usd,m.lease_holder,m.lease_token,m.started_at,m.finished_at,
 m.requests,m.input_tokens,m.output_tokens,m.usage_reported,m.micro_usd,m.pricing_source,m.response,m.error`

const modelCallFrom = ` FROM workflow_model_calls m JOIN workflow_node_attempts a ON a.run_id=m.run_id AND a.seq=m.attempt_seq`

func scanWorkflowModelCall(scanner rowScanner) (WorkflowModelCall, error) {
	var c WorkflowModelCall
	var parameters, started string
	var finished sql.NullString
	if err := scanner.Scan(&c.RunID, &c.AttemptSeq, &c.AttemptState, &c.Step, &c.Policy, &c.PolicyVersion, &c.Provider, &c.Model, &parameters,
		&c.PromptSHA256, &c.SchemaSHA256, &c.RequestSHA256, &c.EstimatedInputTokens, &c.EstimatedOutputTokens, &c.EstimatedMicroUSD,
		&c.LeaseHolder, &c.LeaseToken, &started, &finished, &c.Requests, &c.InputTokens, &c.OutputTokens, &c.UsageReported, &c.MicroUSD,
		&c.PricingSource, &c.Response, &c.Error); err != nil {
		return c, err
	}
	c.Parameters = json.RawMessage(parameters)
	var err error
	if c.StartedAt, err = parseTime(started); err != nil {
		return c, err
	}
	c.FinishedAt, err = parseOptionalTime(finished)
	return c, err
}

func workflowModelCalls(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowModelCall, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+modelCallColumns+modelCallFrom+` WHERE m.run_id=? ORDER BY m.attempt_seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowModelCall{}
	for rows.Next() {
		c, err := scanWorkflowModelCall(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// modelSpend is the run's AI spend so far: recorded usage, and the
// estimate of every call without it.
func modelSpend(ctx context.Context, q queryRower, runID string) (tokens, micro int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_input_tokens+estimated_output_tokens ELSE input_tokens+output_tokens END),0),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_micro_usd ELSE micro_usd END),0)
 FROM workflow_model_calls WHERE run_id=?`, runID).Scan(&tokens, &micro)
	return tokens, micro, err
}

// requireAttemptModelCallRecorded refuses ending an AI attempt whose model
// call has no recorded outcome other than through FinishWorkflowModelCall.
func requireAttemptModelCallRecorded(ctx context.Context, q queryRower, runID string, attemptSeq int64) error {
	var open int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_model_calls WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL`, runID, attemptSeq).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return fmt.Errorf("%w (attempt %d)", ErrWorkflowModelCallOpen, attemptSeq)
	}
	return nil
}

// WorkflowModelCallStart is an AI attempt about to call its model: the
// node attempt and the call the runtime derived for it.
type WorkflowModelCallStart struct {
	Attempt               WorkflowAttemptStart
	Policy                string
	PolicyVersion         int
	Provider              string
	Model                 string
	Parameters            json.RawMessage
	PromptSHA256          string
	SchemaSHA256          string
	RequestSHA256         string
	EstimatedInputTokens  int64
	EstimatedOutputTokens int64
	EstimatedMicroUSD     int64
}

func validDigest(d string) bool {
	return len(d) == 71 && strings.HasPrefix(d, "sha256:")
}

// BeginWorkflowModelCall durably records an AI node attempt and its model
// call, in one transaction under the run's current lease, before the
// model is called.
func (s *Store) BeginWorkflowModelCall(ctx context.Context, lease WorkflowRunLease, m WorkflowModelCallStart) (WorkflowNodeAttempt, error) {
	if err := validAttemptStart(lease, m.Attempt); err != nil {
		return WorkflowNodeAttempt{}, err
	}
	switch {
	case m.Policy == "" || m.PolicyVersion < 1 || m.Provider == "" || m.Model == "" || !workflowJSONObject(m.Parameters):
		return WorkflowNodeAttempt{}, errors.New("a model call names its Model Policy, provider, model and parameters")
	case !validDigest(m.PromptSHA256) || !validDigest(m.SchemaSHA256) || !validDigest(m.RequestSHA256):
		return WorkflowNodeAttempt{}, errors.New("a model call needs the digests of its prompt, schema and request")
	case m.EstimatedInputTokens < 0 || m.EstimatedOutputTokens < 0 || m.EstimatedMicroUSD < 0:
		return WorkflowNodeAttempt{}, errors.New("a model call's estimate is not negative")
	}
	var out WorkflowNodeAttempt
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		var err error
		if out, err = beginWorkflowAttempt(ctx, conn, lease, m.Attempt, now); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO workflow_model_calls(run_id,attempt_seq,step_id,policy,policy_version,provider,model,parameters,prompt_sha256,schema_sha256,request_sha256,
 estimated_input_tokens,estimated_output_tokens,estimated_micro_usd,lease_holder,lease_token,started_at,finished_at,requests,input_tokens,output_tokens,usage_reported,micro_usd,pricing_source,response,error)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,0,0,0,0,0,'','','')`, out.RunID, out.Seq, out.Step, m.Policy, m.PolicyVersion, m.Provider, m.Model, string(m.Parameters),
			m.PromptSHA256, m.SchemaSHA256, m.RequestSHA256, m.EstimatedInputTokens, m.EstimatedOutputTokens, m.EstimatedMicroUSD, lease.Holder, lease.Token, formatTime(now))
		if err != nil {
			return fmt.Errorf("record workflow model call: %w", err)
		}
		return nil
	})
	return out, err
}

// WorkflowModelCallEnd is how an AI attempt's call ended: its usage, cost,
// the model's text answer and any error, with how the attempt ends
// (Attempt; its RunID and Seq are the call's).
type WorkflowModelCallEnd struct {
	RunID         string
	AttemptSeq    int64
	Requests      int
	InputTokens   int64
	OutputTokens  int64
	UsageReported bool
	MicroUSD      int64
	PricingSource string
	Response      string
	Error         string
	Attempt       WorkflowAttemptEnd
}

// FinishWorkflowModelCall records an AI attempt's call and its outcome (the
// validated proposal with the next checkpoint, or its failure with the
// retry time or the run's failure) in one transaction under the run's
// current lease.
func (s *Store) FinishWorkflowModelCall(ctx context.Context, lease WorkflowRunLease, m WorkflowModelCallEnd) (WorkflowRun, error) {
	switch {
	case m.Requests < 0 || m.InputTokens < 0 || m.OutputTokens < 0 || m.MicroUSD < 0:
		return WorkflowRun{}, errors.New("a model call's usage is not negative")
	case len(m.Response) > maxModelResponse || len(m.Error) > maxEffectText || len(m.PricingSource) > 64:
		return WorkflowRun{}, errors.New("a model call's response, error or pricing source is too long")
	}
	m.Attempt.RunID, m.Attempt.Seq = m.RunID, m.AttemptSeq
	if err := validAttemptEnd(lease, m.Attempt); err != nil {
		return WorkflowRun{}, err
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		res, err := conn.ExecContext(ctx, `UPDATE workflow_model_calls SET finished_at=?,requests=?,input_tokens=?,output_tokens=?,usage_reported=?,micro_usd=?,pricing_source=?,response=?,error=?
 WHERE run_id=? AND attempt_seq=? AND finished_at IS NULL AND lease_token=?`, formatTime(now), m.Requests, m.InputTokens, m.OutputTokens, m.UsageReported,
			m.MicroUSD, m.PricingSource, m.Response, m.Error, m.RunID, m.AttemptSeq, lease.Token)
		if err != nil {
			return fmt.Errorf("record workflow model call usage: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			if err != nil {
				return err
			}
			return fmt.Errorf("%w: attempt %d has no open model call under lease %d", ErrWorkflowRunConflict, m.AttemptSeq, lease.Token)
		}
		return finishWorkflowAttempt(ctx, conn, lease, m.Attempt, now)
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, m.RunID)
}
