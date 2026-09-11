package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
)

// Runtime maintenance keeps derived coverage moving independently of provider
// dispatch. The Store owns generation activation and durable batch progress.
func startMemoryRetrievalHost(ctx context.Context, store *eviedb.Store) func() error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		unavailable := false
		for ctx.Err() == nil {
			batchCtx, finishBatch := context.WithTimeout(ctx, 5*time.Second)
			coverage, err := store.RefreshMemoryIndex(batchCtx, 256)
			finishBatch()
			if ctx.Err() != nil {
				return
			}
			delay := time.Second
			if err != nil {
				if !unavailable {
					log.Print("memory retrieval index unavailable; maintenance will retry")
				}
				unavailable = true
				delay = 5 * time.Second
			} else {
				if unavailable {
					log.Print("memory retrieval index maintenance recovered")
				}
				unavailable = false
				if coverage.Pending > 0 {
					delay = 100 * time.Millisecond
				}
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return sync.OnceValue(func() error {
		cancel()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
			return nil
		case <-timer.C:
			return errors.New("memory retrieval index shutdown exceeded five seconds")
		}
	})
}
