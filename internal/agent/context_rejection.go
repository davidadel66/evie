package agent

import (
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/openrouter"
)

// providerContextRejection is a provider refusal of a request for exceeding
// the model context window. It is returned unclassified so the turn can make
// its one compact-and-retry; the provider body never reaches it.
type providerContextRejection struct{ err error }

func (e *providerContextRejection) Error() string { return e.err.Error() }
func (e *providerContextRejection) Unwrap() error { return e.err }

// contextRejection recognizes a context-length rejection that arrived before
// any live output while the turn is still active.
func contextRejection(coordinator *turnCoordinator, err error, callbackFired bool) *providerContextRejection {
	var streamErr *openrouter.StreamError
	if callbackFired || coordinator.result().kind != causeNone || coordinator.ctx.Err() != nil ||
		!errors.As(err, &streamErr) || !streamErr.ContextLengthExceeded {
		return nil
	}
	return &providerContextRejection{err: err}
}

// failContextRejection ends a turn whose compacted retry the provider also
// rejected for context length: a context_overflow at the provider stage,
// distinct from an ordinary provider_error.
func (s *Session) failContextRejection(coordinator *turnCoordinator, rejection *providerContextRejection) error {
	if cause := s.observeTurnContext(coordinator); cause != nil {
		return cause
	}
	overflow := fmt.Errorf("%w: the provider rejected the compacted retry for context length (%v)", ErrContextOverflow, rejection.err)
	coordinator.selectCause(causeContextOverflow, overflow, 0)
	return overflow
}
