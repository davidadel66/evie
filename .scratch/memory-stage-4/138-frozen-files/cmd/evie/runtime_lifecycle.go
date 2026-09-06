package main

import (
	"context"
	"io"
)

// The process owns its REPL input. Closing it on shutdown releases the one
// scanner whether it is waiting at the chooser, prompt, or approval gate.
func closeInputOnCancellation(ctx context.Context, input io.Closer) func() bool {
	return context.AfterFunc(ctx, func() { _ = input.Close() })
}
