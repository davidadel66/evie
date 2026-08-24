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
	leaseDuration               time.Duration
	heartbeatInterval           time.Duration
	cleanupTimeout              time.Duration
	newTicker                   func(time.Duration) heartbeatTicker
	beforeAssistantConstruction func()
	beforeApprovalInvocation    func()
	// beforeToolResultHandoff is a deterministic test seam at the zero-work
	// boundary after ordinary tool return and before lifecycle-stage handoff.
	beforeToolResultHandoff func()
}

var defaultTurnTiming = turnTiming{
	leaseDuration:     30 * time.Second,
	heartbeatInterval: 10 * time.Second,
	cleanupTimeout:    5 * time.Second,
	newTicker: func(interval time.Duration) heartbeatTicker {
		return realHeartbeatTicker{Ticker: time.NewTicker(interval)}
	},
}

type heartbeatTicker interface {
	C() <-chan time.Time
	Stop()
}

type realHeartbeatTicker struct {
	*time.Ticker
}

func (t realHeartbeatTicker) C() <-chan time.Time { return t.Ticker.C }

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
	toolDone   chan struct{}
}

func newTurnCoordinator(parent context.Context) *turnCoordinator {
	ctx, cancel := context.WithCancel(parent)
	return &turnCoordinator{ctx: ctx, cancel: cancel}
}

// setStage admits an ordinary phase entry only while the coordinator remains
// live. Mandatory transitions after a durable commit or ordinary tool return
// use their dedicated handoff methods because those transitions intentionally
// finish even when a cause was reserved concurrently.
func (c *turnCoordinator) setStage(stage memory.TurnStage) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind != causeNone || c.pending != nil || c.ctx.Err() != nil {
		return false
	}
	c.stage = stage
	return true
}

func (c *turnCoordinator) finishAdmittedCallbackStage(stage memory.TurnStage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind == causeNone {
		c.stage = stage
	}
}

// transitionIfActive changes lifecycle stage and its corresponding agent-owned
// state as one boundary with terminal-cause finalization. A cause may reserve
// and cancel concurrently, but it cannot capture the new stage without also
// observing the completed state transition.
func (c *turnCoordinator) transitionIfActive(stage memory.TurnStage, transition func()) bool {
	c.callbackMu.RLock()
	defer c.callbackMu.RUnlock()
	c.mu.Lock()
	active := c.cause.kind == causeNone && c.pending == nil && c.ctx.Err() == nil
	if active {
		c.stage = stage
	}
	c.mu.Unlock()
	if !active {
		return false
	}
	transition()
	return true
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
	// Cancellation must not wait for an admitted frontend callback. The
	// callback may rely on this context to finish; pending prevents any new
	// callback from being admitted while lifecycle-stage finalization waits.
	c.cancel()
	c.mu.Lock()
	toolDone := c.toolDone
	c.mu.Unlock()
	if toolDone != nil {
		<-toolDone
	}

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
	return true
}

// beginCommitBoundary prevents terminal-cause stage capture from completing
// between a durable append and its mandatory post-commit stage transition.
func (c *turnCoordinator) beginCommitBoundary() bool {
	c.callbackMu.RLock()
	c.mu.Lock()
	active := c.cause.kind == causeNone && c.pending == nil && c.ctx.Err() == nil
	c.mu.Unlock()
	if !active {
		c.callbackMu.RUnlock()
	}
	return active
}

func (c *turnCoordinator) abortCommitBoundary() {
	c.callbackMu.RUnlock()
}

func (c *turnCoordinator) finishCommitBoundary(stage memory.TurnStage) {
	c.mu.Lock()
	if c.cause.kind == causeNone {
		c.stage = stage
	}
	c.mu.Unlock()
	c.callbackMu.RUnlock()
}

func (c *turnCoordinator) finishSuccessBoundary() {
	c.mu.Lock()
	c.cause = terminalCause{kind: causeSuccess, stage: c.stage}
	c.pending = nil
	c.mu.Unlock()
	c.callbackMu.RUnlock()
}

func (c *turnCoordinator) beginToolPhase() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cause.kind != causeNone || c.pending != nil || c.ctx.Err() != nil {
		return false
	}
	c.toolDone = make(chan struct{})
	return true
}

// completeToolPhase resolves the consumer-owned result handoff before a
// pending cause can capture its lifecycle stage. No coordinator lock is held
// while preparation or execution itself runs.
func (c *turnCoordinator) completeToolPhase() {
	c.mu.Lock()
	if c.cause.kind == causeNone {
		c.stage = memory.StageToolCommit
	}
	if c.toolDone != nil {
		close(c.toolDone)
		c.toolDone = nil
	}
	c.mu.Unlock()
}

func (c *turnCoordinator) abortToolPhase() {
	c.mu.Lock()
	if c.toolDone != nil {
		close(c.toolDone)
		c.toolDone = nil
	}
	c.mu.Unlock()
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
