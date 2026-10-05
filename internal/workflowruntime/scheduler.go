package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// The background scheduler (Stage 14 sub-stage 4, 2026-10-04). Long-lived
// Evie processes (`evie serve` and the REPL) run one, like the memory
// background pass. Each poll lists the runs that are due (queued, released,
// past an expired lease, or past a retry's or pause's not-before time) and
// starts an execution pass for each, at most SchedulerConfig.Workers at
// once. The first poll happens at start, so every startup is a recovery
// scan, and every later poll is one too: a run whose holder died is due as
// soon as its lease expires. Several processes may run schedulers over one
// database; the fenced lease lets exactly one of them advance a run.

// stopWait bounds how long stop waits for passes to finish their node in
// flight and release their leases.
const stopWait = 5 * time.Second

// Start runs the scheduler until ctx ends or the returned stop is called.
// Stop ends every pass between nodes (a node in flight finishes and is
// recorded), releases their leases and waits at most five seconds; a pass
// still inside a node then keeps its lease until it expires and the next
// process recovers it. Start may be called once at a time.
func (r *Runtime) Start(ctx context.Context) (stop func() error) {
	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.scheduling = true
	r.mu.Unlock()
	slots := make(chan struct{}, r.sched.Workers)
	var passes sync.WaitGroup
	loop := make(chan struct{})
	go func() {
		defer close(loop)
		ticker := time.NewTicker(r.sched.Poll)
		defer ticker.Stop()
		for {
			r.schedule(ctx, slots, &passes)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-r.wakeup:
			}
		}
	}()
	// The schedule ticker (sub-stage 11) decides due schedule slots under
	// its single runner lease; the first tick is at start, so every startup
	// applies the Catch-up Policies.
	ticks := make(chan struct{})
	go func() {
		defer close(ticks)
		ticker := time.NewTicker(r.sched.Schedules)
		defer ticker.Stop()
		for {
			r.reportError("schedules", r.tickSchedules(ctx))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	// The retention sweep (sub-stage 12) runs under its own single runner
	// lease, first at start.
	sweeps := make(chan struct{})
	go func() {
		defer close(sweeps)
		ticker := time.NewTicker(r.sched.Retention)
		defer ticker.Stop()
		for {
			r.reportError("retention", r.tickRetention(ctx))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return sync.OnceValue(func() error {
		cancel()
		<-loop
		<-ticks
		<-sweeps
		if err := r.store.ReleaseWorkflowScheduleRunner(context.WithoutCancel(ctx), r.holder); err != nil {
			r.reportError("schedules", err)
		}
		done := make(chan struct{})
		go func() {
			passes.Wait()
			close(done)
		}()
		timer := time.NewTimer(stopWait)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			return fmt.Errorf("workflow scheduler shutdown exceeded %s; unreleased leases expire and the next Evie process recovers their runs", stopWait)
		}
		r.mu.Lock()
		r.scheduling = false
		r.mu.Unlock()
		return nil
	})
}

// schedule starts a pass for each due run while worker slots are free.
func (r *Runtime) schedule(ctx context.Context, slots chan struct{}, passes *sync.WaitGroup) {
	free := cap(slots) - len(slots)
	if free <= 0 || ctx.Err() != nil {
		return
	}
	// Runs this process is finishing may still be listed; ask for enough.
	ids, err := r.store.DueWorkflowRuns(ctx, free+cap(slots))
	if err != nil {
		if ctx.Err() == nil {
			r.reportError("", fmt.Errorf("workflow scheduler: list due runs: %w", err))
		}
		return
	}
	r.reportError("", nil)
	for _, id := range ids {
		r.mu.Lock()
		busy := r.active[id]
		r.mu.Unlock()
		if busy {
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			return
		}
		r.mu.Lock()
		r.active[id] = true
		r.mu.Unlock()
		passes.Add(1)
		go func() {
			defer passes.Done()
			end, err := r.execute(ctx, id)
			<-slots
			r.passEnded(id, end, err)
		}()
	}
}

// passEnded records how a scheduled pass ended for waiters and reports its
// error once, then lets the scheduler use the freed slot at once. A pass
// that failed is retried at the next poll instead, so a persistent error
// does not spin.
func (r *Runtime) passEnded(id string, end passEnd, err error) {
	if err != nil {
		err = fmt.Errorf("workflow run %s: %w", id, err)
	}
	r.reportError(id, err)
	r.mu.Lock()
	delete(r.active, id)
	if err != nil {
		r.failures[id] = err
	} else {
		delete(r.failures, id)
	}
	close(r.changed)
	r.changed = make(chan struct{})
	r.mu.Unlock()
	if err == nil && end != passHeld {
		r.wake()
	}
}

// reportError reports err for key once while it persists; nil clears it.
func (r *Runtime) reportError(key string, err error) {
	r.mu.Lock()
	if err == nil {
		delete(r.reported, key)
		r.mu.Unlock()
		return
	}
	if r.reported[key] == err.Error() {
		r.mu.Unlock()
		return
	}
	r.reported[key] = err.Error()
	report := r.report
	r.mu.Unlock()
	if report != nil && !errors.Is(err, context.Canceled) {
		report(err)
	}
}

// wake prompts the scheduler to look for due runs now.
func (r *Runtime) wake() {
	select {
	case r.wakeup <- struct{}{}:
	default:
	}
}

// schedulerRunning reports whether this process's scheduler executes runs.
func (r *Runtime) schedulerRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.scheduling
}

// changes is closed when the next pass in this process ends.
func (r *Runtime) changes() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changed
}

// failure is the error of the run's latest pass in this process, if it
// failed.
func (r *Runtime) failure(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failures[id]
}
