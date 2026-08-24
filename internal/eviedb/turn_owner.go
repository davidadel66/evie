package eviedb

import (
	"context"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// SessionTurnOwner binds immutable harness-owned session and holder identity.
// The model and tool arguments never receive either value.
type SessionTurnOwner struct {
	store     *Store
	sessionID memory.SessionID
	holderID  memory.LeaseHolderID
}

func (s *Store) BindTurnOwner(
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
) *SessionTurnOwner {
	return &SessionTurnOwner{store: s, sessionID: sessionID, holderID: holderID}
}

func (o *SessionTurnOwner) Acquire(ctx context.Context, duration time.Duration) (memory.TurnLease, error) {
	return o.store.AcquireTurnLease(ctx, o.sessionID, o.holderID, duration)
}

func (o *SessionTurnOwner) Heartbeat(
	ctx context.Context,
	lease memory.TurnLease,
	duration time.Duration,
) (memory.TurnLease, error) {
	return o.store.HeartbeatTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken, duration)
}

func (o *SessionTurnOwner) Authorize(ctx context.Context, lease memory.TurnLease) error {
	return o.store.AuthorizeTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken)
}

func (o *SessionTurnOwner) Release(ctx context.Context, lease memory.TurnLease) error {
	return o.store.ReleaseTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken)
}

func (*SessionTurnOwner) IsConflict(err error) bool {
	return errors.Is(err, ErrTurnLeaseHeld)
}

func (*SessionTurnOwner) IsLeaseLost(err error) bool {
	return errors.Is(err, ErrTurnLeaseLost)
}
