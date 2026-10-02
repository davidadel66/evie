package subagents

import (
	"context"
	"errors"
	"time"
)

const (
	recoveryInterval   = 250 * time.Millisecond
	recoveryMaxBackoff = 10 * time.Second
)

// RunRecovery reconciles execution metadata throughout the host's lifetime,
// independently of Plugin enablement. It never starts a child or model call.
// A lease that is still live at startup is checked again after it expires.
// Each pass first records the outcomes this process's owners decided but
// could not record, so such an attempt settles while its parent turn is still
// live. A failed pass is reported and retried with exponential backoff
// bounded by recoveryMaxBackoff; only cancellation stops recovery. Each
// distinct failure is reported once while it persists. report may be nil.
func (s *Supervisor) RunRecovery(ctx context.Context, report func(error)) error {
	delay := recoveryInterval
	timer := time.NewTimer(delay)
	defer timer.Stop()
	var reported string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		pendingErr := s.settlePending()
		_, err := s.store.RecoverSubagents(ctx)
		if ctx.Err() == nil {
			err = errors.Join(pendingErr, err)
		}
		switch {
		case err == nil:
			delay, reported = recoveryInterval, ""
		case ctx.Err() != nil:
			return ctx.Err()
		default:
			if message := err.Error(); message != reported {
				reported = message
				if report != nil {
					report(err)
				}
			}
			delay = min(2*delay, recoveryMaxBackoff)
		}
		timer.Reset(delay)
	}
}
