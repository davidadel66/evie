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
// any output streamed while the turn is still active.
func contextRejection(coordinator *turnCoordinator, err error, outputStreamed bool) *providerContextRejection {
	var streamErr *openrouter.StreamError
	if outputStreamed || coordinator.result().kind != causeNone || coordinator.ctx.Err() != nil ||
		!errors.As(err, &streamErr) || !streamErr.ContextLengthExceeded {
		return nil
	}
	return &providerContextRejection{err: err}
}

// failContextRejection ends a turn whose context-length rejection cannot be
// recovered by compacting: the compacted retry was also rejected, or the
// rejected request was already compacted in its iteration. It is a
// context_overflow at the provider stage, distinct from provider_error.
func (s *Session) failContextRejection(coordinator *turnCoordinator, rejection *providerContextRejection) error {
	if cause := s.observeTurnContext(coordinator); cause != nil {
		return cause
	}
	overflow := fmt.Errorf("%w: the provider rejected an already compacted request for context length (%v)", ErrContextOverflow, rejection.err)
	coordinator.selectCause(causeContextOverflow, overflow, 0)
	return overflow
}

// closeReasoningPhase ends a visible reasoning phase left open by a rejected
// attempt, as assistant acceptance and turn failure do, so the retry or the
// failure that replaces it starts from a closed phase.
func closeReasoningPhase(ev Events, rendered *renderedOutput) {
	rendered.mu.Lock()
	open := rendered.reasoningOpen
	rendered.reasoningOpen = false
	rendered.mu.Unlock()
	if open {
		ev.ReasoningDone()
	}
}
