package eviedb

import (
	"context"

	"github.com/davidadel66/evie/internal/memory"
)

type SessionHistory struct {
	store     *Store
	sessionID memory.SessionID
	holderID  memory.LeaseHolderID
}

func (s *Store) BindHistory(sessionID memory.SessionID, holderID memory.LeaseHolderID) *SessionHistory {
	return &SessionHistory{
		store:     s,
		sessionID: sessionID,
		holderID:  holderID,
	}
}

func (h *SessionHistory) Append(
	ctx context.Context,
	lease memory.TurnLease,
	input memory.EventInput,
) (memory.Event, error) {
	if err := validateBoundTurnLease(lease, h.sessionID, h.holderID); err != nil {
		return memory.Event{}, err
	}
	return h.store.AppendEventWithLease(
		ctx,
		h.sessionID,
		h.holderID,
		lease.FencingToken,
		input,
	)
}

func (h *SessionHistory) Events(ctx context.Context) ([]memory.Event, error) {
	return h.store.LoadEvents(ctx, h.sessionID)
}

func (h *SessionHistory) WorkingContext(ctx context.Context) (string, error) {
	return h.store.workingTaskContext(ctx, h.sessionID)
}
