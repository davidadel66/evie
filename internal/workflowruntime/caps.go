package workflowruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
)

// The operator's daily cap (Stage 14 sub-stage 11, 2026-10-04). Schedules
// and chat starts multiply runs, so on top of each run's reviewed
// limits.max_tokens every AI call and worker start counts toward one cap
// on all workflow runs per UTC day, in tokens and in dollars
// (Config.DailyCap; EVIE_WORKFLOW_DAILY_TOKENS and EVIE_WORKFLOW_DAILY_USD,
// by default 10,000,000 tokens and $5). The store refuses, in the
// transaction that would record it, a call whose bound (an AI call's
// estimate, a worker's whole budget priced as output) would take today's
// spend past the cap; nothing is recorded or sent. The run then pauses
// visibly, its reason beginning "daily workflow cap reached" (its
// notification is cap_reached), with its recheck at the next 00:00 UTC,
// when it continues by itself. A map holds back the items that do not
// fit, lets those started finish, and pauses the same way; on resuming it
// keeps its finished items.

// untilReset is how long a run paused at the cap waits: until the UTC day
// the refusal counted ends.
func (r *Runtime) untilReset(capped *eviedb.WorkflowDailyCapError) time.Duration {
	return max(capped.ResetAt.Sub(r.now()), time.Second)
}

// CapStatus is the daily cap as the owner sees it: whether this Evie
// enforces one, today's UTC day and spend against it, when it resets, and
// the runs paused at it.
type CapStatus struct {
	Enforced   bool                      `json:"enforced"`
	Day        string                    `json:"day"`
	ResetAt    time.Time                 `json:"reset_at"`
	Cap        eviedb.WorkflowDailyCap   `json:"cap"`
	Spent      eviedb.WorkflowDailySpend `json:"spent"`
	Reached    bool                      `json:"reached"`
	PausedRuns []string                  `json:"paused_runs"`
}

// Caps reads today's spend against the daily cap.
func (r *Runtime) Caps(ctx context.Context) (CapStatus, error) {
	day, reset := eviedb.WorkflowDay(r.now())
	out := CapStatus{Day: day, ResetAt: reset, PausedRuns: []string{}}
	spent, err := r.store.WorkflowDailySpend(ctx, day)
	if err != nil {
		return out, err
	}
	out.Spent = spent
	if r.cap != nil {
		out.Enforced, out.Cap = true, *r.cap
		out.Reached = spent.Tokens >= r.cap.Tokens || spent.MicroUSD >= r.cap.MicroUSD
	}
	runs, err := r.store.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{Limit: 200})
	if err != nil {
		return out, err
	}
	for _, run := range runs {
		if run.State == eviedb.WorkflowRunWaitingForDependency && strings.HasPrefix(run.Error, eviedb.WorkflowDailyCapReason) {
			out.PausedRuns = append(out.PausedRuns, run.ID)
		}
	}
	return out, nil
}

// Text renders the cap status for the terminal.
func (c CapStatus) Text() string {
	var b strings.Builder
	if !c.Enforced {
		fmt.Fprintf(&b, "Daily cap: not enforced by this Evie; today (%s UTC) workflow runs have used %s tokens and %s\n", c.Day, grouped(c.Spent.Tokens), dollars(c.Spent.MicroUSD))
	} else {
		state := "within the cap"
		if c.Reached {
			state = "the cap is reached"
		}
		fmt.Fprintf(&b, "Daily cap (%s UTC): %s of %s tokens and %s of %s used by %s and %s; %s; it resets at %s\n", c.Day,
			grouped(c.Spent.Tokens), grouped(c.Cap.Tokens), dollars(c.Spent.MicroUSD), dollars(c.Cap.MicroUSD),
			plural(c.Spent.ModelCalls, "AI call"), plural(c.Spent.WorkerCalls, "worker"), state, c.ResetAt.Format(timeLayout))
	}
	if len(c.PausedRuns) > 0 {
		fmt.Fprintf(&b, "  paused at the cap: %s (%s); each continues by itself after the reset\n", plural(len(c.PausedRuns), "run"), strings.Join(c.PausedRuns, ", "))
	}
	return b.String()
}
