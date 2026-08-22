package memory

import "time"

type (
	LeaseHolderID   string
	FencingToken    int64
	LeaseGeneration int64
	TurnLease       struct {
		SessionID    SessionID
		HolderID     LeaseHolderID
		FencingToken FencingToken
		Generation   LeaseGeneration
		ExpiresAt    time.Time
	}
)

// ActiveAt reports whether the lease owns its session at the supplied instant.
// Expiry is a half-open boundary: a lease is no longer active at ExpiresAt.
func (l TurnLease) ActiveAt(now time.Time) bool {
	return l.HolderID != "" && l.ExpiresAt.After(now)
}
