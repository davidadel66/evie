package main

import (
	"context"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

type compilerHostShutdownResult struct{ err error }

func (h compilerHostShutdownResult) RunCompilerHost(ctx context.Context, _ eviedb.CompilerSupervisorConfig) error {
	<-ctx.Done()
	return h.err
}

func TestCompilerHostShutdownPreservesJoinedCleanupFailure(t *testing.T) {
	cleanup := errors.New("fixture cleanup deadline exceeded")
	for _, test := range []struct {
		name        string
		hostError   error
		wantFailure bool
	}{
		{"plain cancellation", context.Canceled, false},
		{"cancellation and cleanup failure", errors.Join(context.Canceled, cleanup), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			stop := startCompilerHost(context.Background(), compilerHostShutdownResult{err: test.hostError}, eviedb.CompilerSupervisorConfig{})
			err := stop()
			if test.wantFailure {
				if !errors.Is(err, cleanup) {
					t.Fatalf("cleanup error suppressed: %v", err)
				}
			} else if err != nil {
				t.Fatalf("plain cancellation reported failure: %v", err)
			}
		})
	}
}
