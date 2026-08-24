package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

const DiscardedResponseMessage = "Response interrupted; streamed text was not saved."

type DiscardReason string

const (
	DiscardProviderError              DiscardReason = "provider_error"
	DiscardProviderResponseInvalid    DiscardReason = "provider_response_invalid"
	DiscardCallerCancelled            DiscardReason = "caller_cancelled"
	DiscardCallerDeadlineExceeded     DiscardReason = "caller_deadline_exceeded"
	DiscardLeaseLost                  DiscardReason = "lease_lost"
	DiscardLeaseHeartbeatFailed       DiscardReason = "lease_heartbeat_failed"
	DiscardAssistantPersistenceFailed DiscardReason = "assistant_persistence_failed"
)

var (
	ErrLeaseConflict = errors.New("agent: session turn lease is held")
	ErrLeaseLost     = errors.New("agent: session turn lease was lost")
)

// TurnOwnership is owned by the agent consumer and implemented by a bound
// durable adapter. Session and holder identity are deliberately absent from
// every method: the adapter binds them before any model-controlled work.
type TurnOwnership interface {
	Acquire(context.Context, time.Duration) (memory.TurnLease, error)
	Heartbeat(context.Context, memory.TurnLease, time.Duration) (memory.TurnLease, error)
	Authorize(context.Context, memory.TurnLease) error
	Release(context.Context, memory.TurnLease) error
	IsConflict(error) bool
	IsLeaseLost(error) bool
}

type turnTiming struct {
	leaseDuration     time.Duration
	heartbeatInterval time.Duration
	cleanupTimeout    time.Duration
}

var defaultTurnTiming = turnTiming{
	leaseDuration:     30 * time.Second,
	heartbeatInterval: 10 * time.Second,
	cleanupTimeout:    5 * time.Second,
}

type causeKind int

const (
	causeNone causeKind = iota
	causeSuccess
	causeProviderError
	causeProviderInvalid
	causeCallerCancelled
	causeCallerDeadline
	causeLeaseLost
	causeHeartbeatFailed
	causeAssistantPersistence
	causeStorage
)

type terminalCause struct {
	kind       causeKind
	err        error
	stage      memory.TurnStage
	httpStatus int
}

type turnCoordinator struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	stage memory.TurnStage
	cause terminalCause
}

func newTurnCoordinator(parent context.Context) *turnCoordinator {
	ctx, cancel := context.WithCancel(parent)
	return &turnCoordinator{ctx: ctx, cancel: cancel}
}

func (c *turnCoordinator) setStage(stage memory.TurnStage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind == causeNone {
		c.stage = stage
	}
}

func (c *turnCoordinator) selectCause(kind causeKind, err error, httpStatus int) bool {
	c.mu.Lock()
	if c.cause.kind != causeNone {
		c.mu.Unlock()
		return false
	}
	c.cause = terminalCause{kind: kind, err: err, stage: c.stage, httpStatus: httpStatus}
	c.mu.Unlock()
	c.cancel()
	return true
}

func (c *turnCoordinator) succeed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind != causeNone {
		return false
	}
	c.cause = terminalCause{kind: causeSuccess, stage: c.stage}
	return true
}

func (c *turnCoordinator) result() terminalCause {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cause
}

func (c *turnCoordinator) active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cause.kind == causeNone
}

func callerCause(err error) causeKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return causeCallerDeadline
	}
	return causeCallerCancelled
}

func (c terminalCause) discardReason() DiscardReason {
	switch c.kind {
	case causeProviderError:
		return DiscardProviderError
	case causeProviderInvalid:
		return DiscardProviderResponseInvalid
	case causeCallerCancelled:
		return DiscardCallerCancelled
	case causeCallerDeadline:
		return DiscardCallerDeadlineExceeded
	case causeLeaseLost:
		return DiscardLeaseLost
	case causeHeartbeatFailed:
		return DiscardLeaseHeartbeatFailed
	case causeAssistantPersistence:
		return DiscardAssistantPersistenceFailed
	default:
		return ""
	}
}
