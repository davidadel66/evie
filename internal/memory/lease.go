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

// UnexpiredAt reports whether this snapshot's local expiry is after the supplied
// instant. It does not prove durable ownership; a newer fencing token may have
// replaced the snapshot. Expiry is a half-open boundary.
func (l TurnLease) UnexpiredAt(now time.Time) bool {
	return l.HolderID != "" && l.ExpiresAt.After(now)
}
