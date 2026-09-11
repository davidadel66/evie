package subagents

import (
	"context"
	"time"
)

// RunRecovery reconciles execution metadata throughout the host's lifetime,
// independently of Plugin enablement. It never starts a child or model call.
// A lease that is still live at startup is checked again after it expires.
func (s *Supervisor) RunRecovery(ctx context.Context) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.store.RecoverSubagents(ctx); err != nil {
				return err
			}
		}
	}
}
