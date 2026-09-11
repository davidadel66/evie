package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func stringHolder(id memory.SessionID) memory.LeaseHolderID { return memory.LeaseHolderID(id) }

// Both conversation and compaction use this same allowance.
type boundedClient struct {
	mu        sync.Mutex
	client    agent.Client
	policy    delegation.Policy
	calls     int
	authorize func(context.Context) error
}

func (c *boundedClient) ChatStream(ctx context.Context, r openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return openrouter.ChatResponse{}, err
	}
	if err := c.authorize(ctx); err != nil {
		return openrouter.ChatResponse{}, err
	}
	c.mu.Lock()
	if c.calls >= c.policy.ModelCalls {
		c.mu.Unlock()
		return openrouter.ChatResponse{}, fmt.Errorf("%w: model-call allowance", delegation.ErrPolicy)
	}
	c.calls++
	c.mu.Unlock()
	if r.MaxTokens > int64(c.policy.OutputTokens) {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: output token allowance", delegation.ErrPolicy)
	}
	b, err := openrouter.RequestBytes(r)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	if len(b) > c.policy.RequestBytes {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: request context", delegation.ErrPolicy)
	}
	result, err := c.client.ChatStream(ctx, r, h)
	if err != nil {
		return result, err
	}
	// Enforce a response byte bound even when a fake or provider ignores max_tokens.
	b, err = json.Marshal(result)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	if len(b) > c.policy.RequestBytes {
		return openrouter.ChatResponse{}, fmt.Errorf("%w: provider output", delegation.ErrPolicy)
	}
	return result, nil
}
