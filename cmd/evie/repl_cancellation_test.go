package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type replCancellationClient struct{ calls atomic.Int32 }

func evieTestContextProfile(model string) openrouter.ContextProfile {
	profile, err := openrouter.NewExplicitContextProfile(model, 300000, 262144, 16384)
	if err != nil {
		panic(err)
	}
	return profile
}

type scanReadBarrier struct {
	reader  io.Reader
	entered chan int
	reads   atomic.Int32
}

func newScanReadBarrier(reader io.Reader) *scanReadBarrier {
	return &scanReadBarrier{reader: reader, entered: make(chan int, 8)}
}

func (r *scanReadBarrier) Read(p []byte) (int, error) {
	read := int(r.reads.Add(1))
	r.entered <- read
	return r.reader.Read(p)
}

func waitForScannerRead(t *testing.T, reader *scanReadBarrier, want int) {
	t.Helper()
	for {
		select {
		case got := <-reader.entered:
			if got >= want {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("scanner did not enter read %d", want)
		}
	}
}

func (c *replCancellationClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls.Add(1)
	return openrouter.ChatResponse{}, nil
}

type replCancellationHistory struct{}

func (replCancellationHistory) Append(context.Context, memory.TurnLease, memory.EventInput) (memory.Event, error) {
	return memory.Event{ID: "event", SessionID: "session", FormatVersion: 1}, nil
}
func (replCancellationHistory) Events(context.Context) ([]memory.Event, error) { return nil, nil }

type replApprovalClient struct {
	called chan struct{}
	path   string
}

func (c *replApprovalClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	close(c.called)
	args := fmt.Sprintf(`{"path":%q,"old_string":"world","new_string":"changed"}`, c.path)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "edit_file", Arguments: args}}},
	}}}}, nil
}

type recordingREPLHistory struct {
	mu     sync.Mutex
	events []memory.Event
}

func (h *recordingREPLHistory) Append(_ context.Context, _ memory.TurnLease, input memory.EventInput) (memory.Event, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e := memory.Event{ID: memory.EventID(fmt.Sprintf("event-%d", len(h.events)+1)), SessionID: "session", Sequence: int64(len(h.events) + 1), Type: input.Type, Role: input.Role, ParentID: input.ParentID, ExecutionID: input.ExecutionID, Content: input.Content, Payload: input.Payload, FormatVersion: 1}
	h.events = append(h.events, e)
	return e, nil
}
func (h *recordingREPLHistory) Events(context.Context) ([]memory.Event, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]memory.Event(nil), h.events...), nil
}

func TestRunREPLContextDiscardsInputThatUnblocksScannerAfterCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	barrier := newScanReadBarrier(reader)
	client := &replCancellationClient{}
	session := agent.New(client, evieTestContextProfile("test"), replCancellationHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "session",
	}, testTurnOwner{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runREPLContext(ctx, session, bufio.NewScanner(barrier))
	}()

	// The REPL intentionally keeps one synchronous scanner. Cancellation does
	// not add a competing reader; one final line releases the blocked Scan.
	waitForScannerRead(t, barrier, 1)
	cancel()
	if _, err := io.WriteString(writer, "late input\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("REPL did not return after its final input")
	}
	if got := client.calls.Load(); got != 0 {
		t.Fatalf("provider calls = %d, cancelled late input must be discarded", got)
	}
}

func TestRunREPLContextDiscardsLateApprovalInputAfterCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	barrier := newScanReadBarrier(reader)
	client := &replApprovalClient{called: make(chan struct{}), path: path}
	history := &recordingREPLHistory{}
	session := agent.New(client, evieTestContextProfile("test"), history, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runREPLContext(ctx, session, bufio.NewScanner(barrier)) }()
	waitForScannerRead(t, barrier, 1)
	if _, err := io.WriteString(writer, "edit it\n"); err != nil {
		t.Fatal(err)
	}
	<-client.called
	waitForScannerRead(t, barrier, 2)
	cancel()
	if _, err := io.WriteString(writer, "yes\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("REPL did not return after final approval input")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("file changed after cancelled approval: %q", data)
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	for _, event := range history.events {
		if event.Type == memory.EventApproval || event.Type == memory.EventToolSucceeded {
			t.Fatalf("unexpected durable event after cancelled approval: %+v", event)
		}
	}
}

type testTurnOwner struct{}

func (testTurnOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	return memory.TurnLease{SessionID: "session", HolderID: "holder", FencingToken: 1}, nil
}
func (testTurnOwner) Heartbeat(context.Context, memory.TurnLease, time.Duration) (memory.TurnLease, error) {
	return memory.TurnLease{SessionID: "session", HolderID: "holder", FencingToken: 1}, nil
}
func (testTurnOwner) Authorize(context.Context, memory.TurnLease) error { return nil }
func (testTurnOwner) Release(context.Context, memory.TurnLease) error   { return nil }
func (testTurnOwner) IsConflict(error) bool                             { return false }
func (testTurnOwner) IsSessionInactive(error) bool                      { return false }
func (testTurnOwner) IsLeaseLost(error) bool                            { return false }
