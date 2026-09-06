package agent

import (
	"context"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type foregroundRecorder interface {
	RecordCompilerForeground(context.Context, memory.CompilerForegroundMeasurement) error
}

type foregroundMeasurementKey struct{}
type foregroundFinalizer struct {
	mu          sync.Mutex
	observation *foregroundObservation
	finished    bool
}
type foregroundObservation struct {
	started   time.Time
	record    memory.CompilerForegroundMeasurement
	recorder  foregroundRecorder
	finalizer *foregroundFinalizer
}

// BeginResponseMeasurement lets a host mark completion after its actual final
// response write (SSE turn_done or REPL output). The returned function is called
// exactly once after Send and output handling, with the host's write/flush error.
// A failed output leaves finalization unavailable while preserving the terminal
// commit observation. Only storage failures are returned; neither failure may
// turn an already committed assistant response into a failed turn.
func BeginResponseMeasurement(ctx context.Context) (context.Context, func(error) error) {
	f := &foregroundFinalizer{}
	return context.WithValue(ctx, foregroundMeasurementKey{}, f), func(outputErr error) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.finished {
			return nil
		}
		f.finished = true
		if f.observation == nil {
			return nil
		}
		o := f.observation
		if outputErr == nil {
			now := time.Now()
			at, duration := now.UnixMilli(), now.Sub(o.started).Nanoseconds()
			o.record.ResponseFinalizedAtUnixMS = &at
			o.record.ResponseFinalizationNanos = &duration
		}
		return o.persist()
	}
}
func beginForegroundObservation(ctx context.Context, h History) *foregroundObservation {
	recorder, ok := h.(foregroundRecorder)
	if !ok {
		return nil
	}
	started := time.Now()
	o := &foregroundObservation{started: started, recorder: recorder, record: memory.CompilerForegroundMeasurement{StartedAtUnixMS: started.UnixMilli(), Outcome: "incomplete"}}
	o.finalizer, _ = ctx.Value(foregroundMeasurementKey{}).(*foregroundFinalizer)
	return o
}
func (o *foregroundObservation) terminal(started time.Time, outcome string) {
	if o == nil {
		return
	}
	now := time.Now()
	at, duration := now.UnixMilli(), now.Sub(started).Nanoseconds()
	o.record.TerminalCommittedAtUnixMS = &at
	o.record.TerminalCommitNanos = &duration
	o.record.Outcome = outcome
}
func (o *foregroundObservation) finish() {
	if o == nil || o.record.RootID == "" {
		return
	}
	if o.finalizer != nil {
		o.finalizer.mu.Lock()
		defer o.finalizer.mu.Unlock()
		if !o.finalizer.finished {
			o.finalizer.observation = o
		}
		return
	}
	_ = o.persist()
}
func (o *foregroundObservation) persist() error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return o.recorder.RecordCompilerForeground(ctx, o.record)
}
