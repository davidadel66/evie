package subagents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
)

// G7: a failed child's reason names the limit it reached instead of the
// former catch-all policy_limit, however the agent loop wraps the error.
func TestExecutionFailureReasonsNameTheLimit(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	for err, want := range map[error]outcome{
		errTokenBudgetSpent:                               {"failed", delegation.ReasonTokenBudgetSpent},
		fmt.Errorf("send: %w", errRequestTooLarge):        {"failed", delegation.ReasonContextLimit},
		fmt.Errorf("build: %w", agent.ErrContextOverflow): {"failed", delegation.ReasonContextLimit},
		fmt.Errorf("stream: %w", errResponseTooLarge):     {"failed", delegation.ReasonResponseTooLarge},
		agent.ErrStepLimitExceeded:                        {"failed", delegation.ReasonWrapUpFailed},
		errors.New("unclassified"):                        {"failed", "infrastructure_failure"},
	} {
		if got := executionFailure(context.Background(), err, deadline); got != want {
			t.Errorf("%v: outcome %+v, want %+v", err, got, want)
		}
	}
}

// G7: a queued child the Kernel refused to start is either an ended parent
// scope or a store failure, no longer one "authority_or_cancellation".
func TestRefusedStartNamesItsCause(t *testing.T) {
	for err, want := range map[error]outcome{
		delegation.ErrAuthority:                       {"interrupted", "authority_ended"},
		eviedb.ErrTurnLeaseLost:                       {"interrupted", "authority_ended"},
		eviedb.ErrProjectNotActive:                    {"interrupted", "authority_ended"},
		fmt.Errorf("load session: %w", sql.ErrNoRows): {"interrupted", "authority_ended"},
		errors.New("disk I/O error"):                  {"failed", "infrastructure_failure"},
	} {
		if got := refused(err); got != want {
			t.Errorf("%v: outcome %+v, want %+v", err, got, want)
		}
	}
}
