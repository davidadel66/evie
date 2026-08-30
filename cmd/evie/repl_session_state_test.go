package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

var (
	errREPLHeld     = errors.New("held")
	errREPLInactive = errors.New("inactive")
)

type unavailableREPLClient struct{ calls int }

func (c *unavailableREPLClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls++
	return openrouter.ChatResponse{}, nil
}

type unavailableREPLHistory struct{ appends int }

func (h *unavailableREPLHistory) Append(context.Context, memory.TurnLease, memory.EventInput) (memory.Event, error) {
	h.appends++
	return memory.Event{}, nil
}
func (*unavailableREPLHistory) Events(context.Context) ([]memory.Event, error) { return nil, nil }

type unavailableREPLOwner struct {
	err      error
	acquire  int
	releases int
}

func (o *unavailableREPLOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	o.acquire++
	return memory.TurnLease{}, o.err
}
func (*unavailableREPLOwner) Heartbeat(context.Context, memory.TurnLease, time.Duration) (memory.TurnLease, error) {
	panic("heartbeat must not run")
}
func (*unavailableREPLOwner) Authorize(context.Context, memory.TurnLease) error {
	panic("authorize must not run")
}
func (o *unavailableREPLOwner) Release(context.Context, memory.TurnLease) error {
	o.releases++
	return nil
}
func (*unavailableREPLOwner) IsConflict(err error) bool {
	return errors.Is(err, errREPLHeld)
}
func (*unavailableREPLOwner) IsSessionInactive(err error) bool {
	return errors.Is(err, errREPLInactive)
}
func (*unavailableREPLOwner) IsLeaseLost(error) bool { return false }

func TestRunREPLPresentsBusyAndInactiveWithoutTurnWork(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{name: "busy", err: errREPLHeld, message: "Session busy; message not sent."},
		{name: "inactive", err: errREPLInactive, message: "Session unavailable; message not sent."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &unavailableREPLClient{}
			history := &unavailableREPLHistory{}
			owner := &unavailableREPLOwner{err: tt.err}
			session := agent.New(client, evieTestContextProfile("test"), history, memory.ScopeContext{
				OwnerID: memory.LocalOwnerID, SessionID: "session",
			}, owner)
			var out bytes.Buffer
			runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("hello\n")), &out)

			if strings.Count(out.String(), tt.message) != 1 || strings.Count(out.String(), "< ") != 2 {
				t.Fatalf("output=%q, want exact state message and selected prompt retained", out.String())
			}
			if strings.Contains(out.String(), "request failed:") {
				t.Fatalf("friendly state message leaked generic error: %q", out.String())
			}
			if owner.acquire != 1 || owner.releases != 0 || history.appends != 0 || client.calls != 0 {
				t.Fatalf("turn work: acquire=%d release=%d append=%d provider=%d", owner.acquire, owner.releases, history.appends, client.calls)
			}
		})
	}
}

func TestRunREPLCompactPresentsBusyAndInactiveWithoutCompactionWork(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{name: "busy", err: errREPLHeld, message: "Session busy; message not sent."},
		{name: "inactive", err: errREPLInactive, message: "Session unavailable; message not sent."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &unavailableREPLClient{}
			history := &unavailableREPLHistory{}
			owner := &unavailableREPLOwner{err: test.err}
			session := agent.New(client, evieTestContextProfile("test"), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, owner)
			var out bytes.Buffer
			runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/compact\n")), &out)

			if strings.Count(out.String(), test.message) != 1 || strings.Contains(out.String(), "compaction failed:") {
				t.Fatalf("output=%q", out.String())
			}
			if owner.acquire != 1 || owner.releases != 0 || history.appends != 0 || client.calls != 0 {
				t.Fatalf("compaction work: acquire=%d release=%d append=%d provider=%d", owner.acquire, owner.releases, history.appends, client.calls)
			}
		})
	}
}

type blockingREPLCompactor struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (c *blockingREPLCompactor) ChatStream(
	context.Context,
	openrouter.ChatRequest,
	openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	c.calls++
	close(c.entered)
	<-c.release
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		summary.WriteString("## ")
		summary.WriteString(heading)
		summary.WriteString("\nkept\n\n")
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: summary.String(),
	}}}}, nil
}

func TestRunREPLCompactMapsInProcessContentionToExistingBusyMessage(t *testing.T) {
	history := &recordingREPLHistory{events: []memory.Event{
		{ID: "turn-1", SessionID: "session", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser},
		{ID: "assistant-1", SessionID: "session", Sequence: 2, ParentID: "turn-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant},
		{ID: "turn-2", SessionID: "session", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser},
		{ID: "assistant-2", SessionID: "session", Sequence: 4, ParentID: "turn-2", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant},
		{ID: "turn-3", SessionID: "session", Sequence: 5, Type: memory.EventUserMessage, Role: memory.RoleUser},
		{ID: "assistant-3", SessionID: "session", Sequence: 6, ParentID: "turn-3", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant},
	}}
	client := &blockingREPLCompactor{entered: make(chan struct{}), release: make(chan struct{})}
	session := agent.NewWithCompactor(client, client, evieTestContextProfile("test"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session"}, testTurnOwner{})
	done := make(chan error, 1)
	go func() {
		_, err := session.Compact(context.Background())
		done <- err
	}()
	<-client.entered

	var out bytes.Buffer
	runREPLContextIO(context.Background(), session, bufio.NewScanner(strings.NewReader("/compact\n")), &out)
	if strings.Count(out.String(), "Session busy; message not sent.") != 1 || strings.Contains(out.String(), "compaction failed:") {
		t.Fatalf("output=%q", out.String())
	}
	close(client.release)
	if err := <-done; err != nil {
		t.Fatalf("first compaction: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("provider calls=%d, want only the in-flight compaction", client.calls)
	}
}
