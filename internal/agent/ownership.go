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
	ErrLeaseConflict        = errors.New("agent: session turn lease is held")
	ErrLeaseLost            = errors.New("agent: session turn lease was lost")
	ErrLeaseHeartbeatFailed = errors.New("agent: session turn lease heartbeat failed")
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

	// callbackMu serializes live callback admission with terminal selection.
	// A callback admitted before a cause may finish; after selection, no new
	// live callback can begin.
	callbackMu sync.RWMutex
	mu         sync.Mutex
	stage      memory.TurnStage
	cause      terminalCause
	pending    *terminalCause
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
	if c.cause.kind != causeNone || c.pending != nil {
		c.mu.Unlock()
		return false
	}
	pending := &terminalCause{kind: kind, err: err, httpStatus: httpStatus}
	c.pending = pending
	c.mu.Unlock()

	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.mu.Lock()
	if c.cause.kind != causeNone || c.pending != pending {
		c.mu.Unlock()
		return false
	}
	pending.stage = c.stage
	c.cause = *pending
	c.pending = nil
	c.mu.Unlock()
	c.cancel()
	return true
}

func (c *turnCoordinator) result() terminalCause {
	c.callbackMu.RLock()
	defer c.callbackMu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind == causeNone && c.pending != nil {
		pending := *c.pending
		pending.stage = c.stage
		return pending
	}
	return c.cause
}

func (c *turnCoordinator) emitIfActive(emit func()) bool {
	c.callbackMu.RLock()
	defer c.callbackMu.RUnlock()
	c.mu.Lock()
	active := c.cause.kind == causeNone && c.pending == nil && c.ctx.Err() == nil
	c.mu.Unlock()
	if !active {
		return false
	}
	emit()
	return true
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
