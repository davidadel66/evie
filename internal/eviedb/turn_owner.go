package eviedb

import (
	"context"
	"errors"
	"fmt"
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
	lease, err := o.store.AcquireTurnLease(ctx, o.sessionID, o.holderID, duration)
	if err != nil {
		return memory.TurnLease{}, err
	}
	if err := validateBoundTurnLease(lease, o.sessionID, o.holderID); err != nil {
		return memory.TurnLease{}, err
	}
	return lease, nil
}

func (o *SessionTurnOwner) Heartbeat(
	ctx context.Context,
	lease memory.TurnLease,
	duration time.Duration,
) (memory.TurnLease, error) {
	if err := validateBoundTurnLease(lease, o.sessionID, o.holderID); err != nil {
		return memory.TurnLease{}, err
	}
	renewed, err := o.store.HeartbeatTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken, duration)
	if err != nil {
		return memory.TurnLease{}, err
	}
	if err := validateBoundTurnLease(renewed, o.sessionID, o.holderID); err != nil {
		return memory.TurnLease{}, err
	}
	return renewed, nil
}

func (o *SessionTurnOwner) Authorize(ctx context.Context, lease memory.TurnLease) error {
	if err := validateBoundTurnLease(lease, o.sessionID, o.holderID); err != nil {
		return err
	}
	return o.store.AuthorizeTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken)
}

func (o *SessionTurnOwner) Release(ctx context.Context, lease memory.TurnLease) error {
	if err := validateBoundTurnLease(lease, o.sessionID, o.holderID); err != nil {
		return err
	}
	return o.store.ReleaseTurnLease(ctx, o.sessionID, o.holderID, lease.FencingToken)
}

func validateBoundTurnLease(
	lease memory.TurnLease,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
) error {
	if lease.SessionID != sessionID {
		return fmt.Errorf("%w: lease session %q does not match bound session %q", ErrTurnLeaseLost, lease.SessionID, sessionID)
	}
	if lease.HolderID != holderID {
		return fmt.Errorf("%w: lease holder %q does not match bound holder %q", ErrTurnLeaseLost, lease.HolderID, holderID)
	}
	if lease.FencingToken <= 0 || lease.Generation <= 0 ||
		lease.Generation != memory.LeaseGeneration(lease.FencingToken) {
		return fmt.Errorf("%w: invalid lease token/generation %d/%d", ErrTurnLeaseLost, lease.FencingToken, lease.Generation)
	}
	return nil
}

func (*SessionTurnOwner) IsConflict(err error) bool {
	return errors.Is(err, ErrTurnLeaseHeld)
}

func (*SessionTurnOwner) IsLeaseLost(err error) bool {
	return errors.Is(err, ErrTurnLeaseLost)
}
