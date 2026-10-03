package agent

import (
	"context"
	"os"

	"github.com/davidadel66/evie/internal/memory"
)

// standingMemoryProvider is implemented by durable histories that can read
// the owner's standing set for their bound session: active, approved,
// standing Claims in Global, the session's Workspace or project, and the
// session itself.
type standingMemoryProvider interface {
	StandingMemory(context.Context) ([]memory.StandingMemoryItem, error)
}

// standingMemory renders the standing block for the session's next request
// and its content-free receipt. It is selected by rule, never by relevance.
// Standing memory is memory egress: with EVIE_REMOTE_MEMORY off nothing is
// sent, and delegated workers, which never read the owner's memory, carry none.
func (s *Session) standingMemory(ctx context.Context) (string, *memory.StandingMemoryReceipt, error) {
	if s.workerInstructions != "" || os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		return "", nil, nil
	}
	provider, ok := s.history.(standingMemoryProvider)
	if !ok {
		return "", nil, nil
	}
	items, err := provider.StandingMemory(ctx)
	if err != nil {
		return "", nil, err
	}
	rendered, shown := memory.RenderStandingMemoryBlock(items)
	return rendered, memory.NewStandingMemoryReceipt(rendered, shown), nil
}
