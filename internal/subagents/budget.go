package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func stringHolder(id memory.SessionID) memory.LeaseHolderID { return memory.LeaseHolderID(id) }

// budget meters one child against its time and token budget. Conversation
// and compaction calls share it. At WrapUpPercent of either budget, or at
// the turn's step limit, the child's next model call is its tool-free
// wrap-up. Once the token budget is spent no further call starts, except
// that single wrap-up.
type budget struct {
	client    agent.Client
	policy    delegation.Policy
	authorize func(context.Context) error
	now       func() time.Time
	started   time.Time
	// record durably marks the wrap-up before its call; see wrapUp.
	record func(reason string)

	mu     sync.Mutex
	tokens int64
	reason string
}

func (b *budget) ChatStream(ctx context.Context, r openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return openrouter.ChatResponse{}, err
	}
	if err := b.authorize(ctx); err != nil {
		return openrouter.ChatResponse{}, err
	}
	b.mu.Lock()
	spent := b.tokens >= b.policy.TokenBudget
	b.mu.Unlock()
	if spent && r.ToolChoice != "none" {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: token budget", delegation.ErrPolicy)
	}
	request, err := openrouter.RequestBytes(r)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	if len(request) > b.policy.RequestBytes {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: request context", delegation.ErrPolicy)
	}
	result, err := b.client.ChatStream(ctx, r, h)
	if err != nil {
		return result, err
	}
	// Enforce a response byte bound even when a fake or provider ignores max_tokens.
	response, err := json.Marshal(result)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	if len(response) > b.policy.RequestBytes {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: provider output", delegation.ErrPolicy)
	}
	b.mu.Lock()
	b.tokens += countedTokens(result.Usage, len(request), len(response))
	b.mu.Unlock()
	return result, nil
}

// countedTokens is the provider's reported input plus output. Whatever it
// does not report is estimated at one token per serialized byte, an
// over-estimate for text, so missing usage cannot hide spending.
func countedTokens(u *openrouter.TokenUsage, requestBytes, responseBytes int) int64 {
	if u != nil && u.InputTokens != nil && u.OutputTokens != nil {
		return max(*u.InputTokens, 0) + max(*u.OutputTokens, 0)
	}
	if u != nil && u.TotalTokens != nil {
		return max(*u.TotalTokens, 0)
	}
	input, output := int64(requestBytes), int64(responseBytes)
	if u != nil && u.InputTokens != nil {
		input = max(*u.InputTokens, 0)
	}
	if u != nil && u.OutputTokens != nil {
		output = max(*u.OutputTokens, 0)
	}
	return input + output
}

// wrapUp is the child's agent.WrapUpSignal. The first time a budget runs
// low it records which one, durably, so an accepted report settles as
// partial even through recovery; the decision then holds for the turn.
func (b *budget) wrapUp(step, limit int) (string, bool) {
	b.mu.Lock()
	reason := b.reason
	if reason == "" {
		switch {
		case step >= limit:
			reason = delegation.WrapUpSteps
		case b.now().Sub(b.started)*100 >= b.policy.Deadline*delegation.WrapUpPercent:
			reason = delegation.WrapUpTime
		case b.tokens*100 >= b.policy.TokenBudget*delegation.WrapUpPercent:
			reason = delegation.WrapUpTokens
		default:
			b.mu.Unlock()
			return "", false
		}
		b.reason = reason
		b.mu.Unlock()
		b.record(reason)
	} else {
		b.mu.Unlock()
	}
	return wrapUpNotice(reason), true
}

// wrapUpReason is the budget that triggered the wrap-up, if any.
func (b *budget) wrapUpReason() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reason
}

var wrapUpCauses = map[string]string{
	delegation.WrapUpTime:   "used 90% of its time budget",
	delegation.WrapUpTokens: "used 90% of its token budget",
	delegation.WrapUpSteps:  "reached its model-response limit",
}

// reportFormat is the report the orchestrator receives: the Summary section
// inline, the rest on request.
const reportFormat = "## Summary\nA self-contained summary of your findings in about 1,000 to 2,000 tokens, citing the source URL for each claim. The orchestrator receives this section directly.\n" +
	"## Details\nSupporting detail and evidence with source URLs. The orchestrator reads this on request.\n" +
	"## Limitations\nOne bullet per gap, conflict, or claim you could not verify."

// wrapUpNotice is the trailing harness message of the tool-free wrap-up call.
func wrapUpNotice(reason string) string {
	return "Evie harness notice: this assignment has " + wrapUpCauses[reason] + ", so tools are no longer available. " +
		"Write your final report now from what you already have, in the required format, and say under Limitations what you could not finish:\n" + reportFormat
}

// assignmentBrief states the child's budget and report format up front.
func assignmentBrief(p delegation.Policy) string {
	return fmt.Sprintf("Budget set by Evie's harness: about %s of wall-clock time and %d model tokens (input plus output) for this assignment. "+
		"When %d%% of either is used, or at the harness's model-response limit, tools are withdrawn and you get one final response to write your report from what you have, so pace your searching and reading.\n\n"+
		"Write your final report in this format:\n%s", p.Deadline, p.TokenBudget, delegation.WrapUpPercent, reportFormat)
}
