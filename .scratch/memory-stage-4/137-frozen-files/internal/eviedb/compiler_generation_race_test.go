package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestCompilerWorkerGenerationSchemaRemainsCallerOwned(t *testing.T) {
	for _, method := range []string{"queue", "compile"} {
		t.Run(method, func(t *testing.T) {
			f := newWorkerFixture(t)
			job := f.queue(t, "I prefer tea.")
			generation := f.generation
			generation.Schema = json.RawMessage(` { "type" : "object" } `)
			original := string(generation.Schema)
			var err error
			if method == "queue" {
				_, err = f.store.QueueCandidateUnit(context.Background(), f.owner, job.Window.Selection, generation, &workerScript{})
			} else {
				_, err = f.store.CompileCandidateUnit(context.Background(), f.owner, job.Window.Selection, generation, &workerScript{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(generation.Schema) != original {
				t.Fatalf("%s rewrote caller Schema buffer: got %q want %q", method, generation.Schema, original)
			}
		})
	}
}

func TestCompilerWorkerConcurrentGenerationSchemaIsDetached(t *testing.T) {
	for _, method := range []string{"queue", "compile"} {
		t.Run(method, func(t *testing.T) {
			f := newWorkerFixture(t)
			job := f.queue(t, "I prefer tea.")
			generation := f.generation
			generation.Schema = json.RawMessage(` { "type" : "object" } `)
			original := string(generation.Schema)
			start := make(chan struct{})
			failures := make(chan error, 8)
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					var err error
					if method == "queue" {
						_, err = f.store.QueueCandidateUnit(context.Background(), f.owner, job.Window.Selection, generation, &workerScript{})
					} else {
						_, err = f.store.CompileCandidateUnit(context.Background(), f.owner, job.Window.Selection, generation, &workerScript{})
					}
					if err != nil && !errors.Is(err, ErrCompilerFence) && !errors.Is(err, ErrCompilerCapacityBlocked) {
						failures <- err
					}
				}()
			}
			close(start)
			wg.Wait()
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			if string(generation.Schema) != original {
				t.Error("concurrent calls changed caller Schema")
			}
		})
	}
}
