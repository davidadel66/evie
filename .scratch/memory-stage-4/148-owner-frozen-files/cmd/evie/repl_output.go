package main

import (
	"io"
	"sync"
)

// replResponseWriter retains failures from both the smooth printer goroutine
// and synchronous host output until that turn's finalization measurement.
type replResponseWriter struct {
	mu  sync.Mutex
	out io.Writer
	err error
}

func (w *replResponseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.out.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if w.err == nil {
		w.err = err
	}
	return n, err
}

func (w *replResponseWriter) begin() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = nil
}

// complete runs after the smooth printer has drained and all terminal output
// has been handled. A buffered host writer must flush before completion can be
// observed. Errors affect telemetry, not the already committed agent turn.
func (w *replResponseWriter) complete() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.out.(interface{ Flush() error }); ok {
		if err := f.Flush(); w.err == nil {
			w.err = err
		}
	}
	return w.err
}
