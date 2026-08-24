package eviedb

import (
	"context"

	"github.com/davidadel66/evie/internal/memory"
)

type SessionHistory struct {
	store     *Store
	sessionID memory.SessionID
}

func (s *Store) BindHistory(sessionID memory.SessionID) *SessionHistory {
	return &SessionHistory{
		store:     s,
		sessionID: sessionID,
	}
}

func (h *SessionHistory) Append(
	ctx context.Context,
	lease memory.TurnLease,
	input memory.EventInput,
) (memory.Event, error) {
	return h.store.AppendEventWithLease(
		ctx,
		h.sessionID,
		lease.HolderID,
		lease.FencingToken,
		input,
	)
}

func (h *SessionHistory) Events(ctx context.Context) ([]memory.Event, error) {
	return h.store.LoadEvents(ctx, h.sessionID)
}
