package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The operator's daily workflow cap (Stage 14 sub-stage 11 of
// docs/memory-architecture.md, 2026-10-04; the sub-stage 7 and 10 entries
// of workflow-runtime.decisions.md asked for it before schedules and chat
// starts multiply runs). Every AI call and every worker start of every
// workflow run counts toward the UTC calendar day it was recorded on: a
// finished call at its recorded usage and cost, an open one at its bound
// (an AI call's estimate, a worker's whole token budget priced as output),
// because it may still spend that much. The transaction that records a
// call refuses it, recording nothing, when today's spend plus the call's
// bound would pass the cap in tokens or in dollars. A run's own reviewed
// limits.max_tokens still applies on top.

// ErrWorkflowDailyCap refuses an AI call or worker start past today's cap.
var ErrWorkflowDailyCap = errors.New(WorkflowDailyCapReason)

// WorkflowDailyCapReason begins every pause reason of a run held at the
// daily cap; the run's notification is then a cap_reached one.
const WorkflowDailyCapReason = "daily workflow cap reached"

// WorkflowDailyCap is the operator's cap on all workflow runs' spend in one
// UTC day.
type WorkflowDailyCap struct {
	Tokens   int64 `json:"tokens"`
	MicroUSD int64 `json:"micro_usd"`
}

// WorkflowDailySpend is what workflow runs have counted on one UTC day.
type WorkflowDailySpend struct {
	Day         string `json:"day"`
	Tokens      int64  `json:"tokens"`
	MicroUSD    int64  `json:"micro_usd"`
	ModelCalls  int    `json:"model_calls"`
	WorkerCalls int    `json:"worker_calls"`
}

// WorkflowDailyCapError is a call refused at the cap: what today has
// counted, what the call could add, the cap, and when the day resets.
type WorkflowDailyCapError struct {
	Day          string             `json:"day"`
	Spent        WorkflowDailySpend `json:"spent"`
	Cap          WorkflowDailyCap   `json:"cap"`
	NeedTokens   int64              `json:"need_tokens"`
	NeedMicroUSD int64              `json:"need_micro_usd"`
	ResetAt      time.Time          `json:"reset_at"`
}

func (e *WorkflowDailyCapError) Error() string {
	return fmt.Sprintf("%s for %s (UTC): the next call could use up to %s tokens and %s, and workflow runs have used %s of %s tokens and %s of %s today; it continues after %s",
		WorkflowDailyCapReason, e.Day, groupDigits(e.NeedTokens), microDollars(e.NeedMicroUSD), groupDigits(e.Spent.Tokens), groupDigits(e.Cap.Tokens),
		microDollars(e.Spent.MicroUSD), microDollars(e.Cap.MicroUSD), e.ResetAt.UTC().Format("2006-01-02 15:04:05Z07:00"))
}

func (e *WorkflowDailyCapError) Is(target error) bool { return target == ErrWorkflowDailyCap }

// WorkflowDay is the UTC calendar day t counts toward, and when it ends.
func WorkflowDay(t time.Time) (string, time.Time) {
	u := t.UTC()
	start := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return start.Format(time.DateOnly), start.AddDate(0, 0, 1)
}

// WorkflowDailySpend reads what workflow runs counted on day
// ("2006-01-02", UTC).
func (s *Store) WorkflowDailySpend(ctx context.Context, day string) (WorkflowDailySpend, error) {
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		return WorkflowDailySpend{}, fmt.Errorf("a day is YYYY-MM-DD: %w", err)
	}
	return dailySpend(ctx, s.db, day)
}

// dailySpend sums one UTC day's model calls and worker calls by the day
// their start was recorded (the date prefix of their UTC timestamp).
func dailySpend(ctx context.Context, q queryRower, day string) (WorkflowDailySpend, error) {
	out := WorkflowDailySpend{Day: day}
	var modelTokens, modelMicro, workerTokens, workerMicro int64
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_input_tokens+estimated_output_tokens ELSE input_tokens+output_tokens END),0),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_micro_usd ELSE micro_usd END),0)
 FROM workflow_model_calls WHERE substr(started_at,1,10)=?`, day).Scan(&out.ModelCalls, &modelTokens, &modelMicro); err != nil {
		return out, err
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN budget_tokens ELSE counted_tokens END),0),
 COALESCE(SUM(CASE WHEN finished_at IS NULL THEN estimated_micro_usd ELSE micro_usd END),0)
 FROM workflow_worker_calls WHERE substr(started_at,1,10)=? AND adopted_from=''`, day).Scan(&out.WorkerCalls, &workerTokens, &workerMicro); err != nil {
		return out, err
	}
	out.Tokens, out.MicroUSD = modelTokens+workerTokens, modelMicro+workerMicro
	return out, nil
}

// checkDailyCap refuses, inside the transaction that would record it, a
// call whose bound would take today's spend past the cap.
func checkDailyCap(ctx context.Context, conn *sql.Conn, now time.Time, limit WorkflowDailyCap, needTokens, needMicro int64) error {
	day, reset := WorkflowDay(now)
	spent, err := dailySpend(ctx, conn, day)
	if err != nil {
		return err
	}
	if spent.Tokens+needTokens > limit.Tokens || spent.MicroUSD+needMicro > limit.MicroUSD {
		return &WorkflowDailyCapError{Day: day, Spent: spent, Cap: limit, NeedTokens: needTokens, NeedMicroUSD: needMicro, ResetAt: reset}
	}
	return nil
}

// capWaits is the time a run spent paused at the daily cap: from each
// pause whose reason is the cap until the run resumed or its pause reason
// changed. limits.max_duration does not count it (sub-stage 11): the
// operator's cap is a wait the reviewed definition could not plan for, and
// a run that failed the moment the cap reset would make the reset useless.
func capWaits(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) (time.Duration, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind,detail,created_at FROM workflow_run_events WHERE run_id=? AND kind IN ('paused','resumed') ORDER BY seq`, runID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total time.Duration
	var pausedAt *time.Time
	for rows.Next() {
		var kind, detail, created string
		if err := rows.Scan(&kind, &detail, &created); err != nil {
			return 0, err
		}
		at, err := parseTime(created)
		if err != nil {
			return 0, err
		}
		if pausedAt != nil && (kind == string(WorkflowEventResumed) || !strings.HasPrefix(detail, WorkflowDailyCapReason)) {
			total += max(at.Sub(*pausedAt), 0)
			pausedAt = nil
		}
		if kind == string(WorkflowEventPaused) && strings.HasPrefix(detail, WorkflowDailyCapReason) && pausedAt == nil {
			pausedAt = &at
		}
	}
	return total, rows.Err()
}

func validDailyCap(c *WorkflowDailyCap) error {
	if c != nil && (c.Tokens < 0 || c.MicroUSD < 0) {
		return errors.New("a daily workflow cap is not negative")
	}
	return nil
}

func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func microDollars(micro int64) string {
	return fmt.Sprintf("$%d.%06d", micro/1_000_000, micro%1_000_000)
}
