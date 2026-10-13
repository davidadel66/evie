// Package web is the evie web frontend: an HTTP server that drives an
// agent.Session and streams each turn to the browser as server-sent
// events. This file is the encoding half — sseEvents is the web twin of
// the REPL's replEvents: same agent.Events contract, but the methods
// write SSE blocks into the HTTP response instead of printing.
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/tools"
)

// sseEvents renders a turn onto one open HTTP response. Every emit is
// flushed immediately — an unflushed event sits in Go's response buffer
// and the stream stops being a stream.
type sseEvents struct {
	// w is nil for a detached turn, whose events reach the browser only
	// through its chat's feed.
	w     http.ResponseWriter
	flush func() error
	// feed, when set, receives every event of the turn of generation, and
	// message is the owner's text it reports when the turn starts.
	feed       *chatFeed
	generation uint64
	message    string
	// err keeps the first failed write/flush through TurnDone. A later successful
	// event cannot establish completion of a stream whose earlier output failed.
	err error
	// wrote flips on the first emit. Until then the response is
	// uncommitted and the handler may still abandon streaming for a
	// plain status reply (the 409-busy path).
	wrote bool
}

// newSSEEvents claims the response as an event stream: sets the SSE
// headers and returns the writer. Errors if the ResponseWriter can't
// flush (no streaming through it means no point continuing).
func newSSEEvents(w http.ResponseWriter) (*sseEvents, error) {
	_, flushes := w.(http.Flusher)
	_, flushErrors := w.(interface{ FlushError() error })
	if !flushes && !flushErrors {
		return nil, fmt.Errorf("response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	return &sseEvents{w: w, flush: http.NewResponseController(w).Flush}, nil
}

// emit writes one SSE block: event name, one line of JSON, blank line,
// flush. json.Marshal escapes newlines inside strings, which is what
// keeps any payload a single data: line — the format's one hard rule.
func (e *sseEvents) emit(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		// Payloads are our own flat structs; this cannot fail. Guard
		// anyway so a future payload type can't wedge the stream.
		if e.err == nil {
			e.err = err
		}
		return
	}
	// The feed never blocks and outlives this response, so a tab that went
	// away does not stop other followers from seeing the turn.
	if e.feed != nil {
		e.feed.publish(e.generation, event, data)
	}
	e.write(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

// publishOnly sends an event to the feed alone: the turn's own response
// keeps its existing vocabulary.
func (e *sseEvents) publishOnly(event string, payload any) {
	if e.feed == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err == nil {
		e.feed.publish(e.generation, event, data)
	}
}

func (e *sseEvents) write(block string) {
	if e.err != nil || e.w == nil {
		return
	}
	e.wrote = true
	n, err := e.w.Write([]byte(block))
	if err == nil && n != len(block) {
		err = io.ErrShortWrite
	}
	if err != nil {
		e.err = err
		return
	}
	e.err = e.flush()
}

func (e *sseEvents) Delta(text string) {
	e.emit("delta", struct {
		Text string `json:"text"`
	}{text})
}

func (e *sseEvents) Reasoning(text string) {
	e.emit("reasoning", struct {
		Text string `json:"text"`
	}{text})
}

// ReasoningDone carries no payload: the client already has every fragment,
// so echoing the blob back would double the bytes for nothing.
func (e *sseEvents) ReasoningDone() {
	e.emit("reasoning_done", struct{}{})
}

func (e *sseEvents) AssistantDone(content string) {
	e.emit("assistant_done", struct {
		Content string `json:"content"`
	}{content})
}

func (e *sseEvents) ToolCall(id, name, args string) {
	e.emit("tool_call", struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Args string `json:"args"`
	}{id, name, args})
}

func (e *sseEvents) ToolResult(id, content string, isErr bool) {
	e.emit("tool_result", struct {
		ID      string `json:"id"`
		Content string `json:"content"`
		IsError bool   `json:"isError"`
	}{id, content, isErr})
}

func (e *sseEvents) ResponseDiscarded(reason agent.DiscardReason, message string) {
	e.emit("response_discarded", struct {
		Reason  agent.DiscardReason `json:"reason"`
		Message string              `json:"message"`
	}{reason, message})
}

// The four below aren't part of agent.Events — they're the server's own
// vocabulary around a turn (see serve.spec.md).

func (e *sseEvents) ApprovalRequest(id, name, args string, preview *tools.FileChangePreview) {
	e.emit("approval_request", struct {
		ID      string                   `json:"id"`
		Name    string                   `json:"name"`
		Args    string                   `json:"args"`
		Preview *tools.FileChangePreview `json:"preview,omitempty"`
	}{id, name, args, preview})
}

func (e *sseEvents) Error(message string) {
	e.emit("error", struct {
		Message string `json:"message"`
	}{message})
}

// TurnStopped reports a turn the owner stopped through /api/cancel. It is an
// error event, so any client still ends the turn, with a stable code that lets
// the browser present the owner's own action instead of a failure banner.
func (e *sseEvents) TurnStopped() {
	e.emit("error", struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}{"Turn stopped.", "turn_stopped"})
}

// Refused reports a turn the agent would not start (another turn holds the
// conversation, or its model changed), with a code the browser keeps.
func (e *sseEvents) Refused(message string) {
	e.emit("error", struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}{message, "turn_refused"})
}

func (e *sseEvents) TurnDone() error {
	e.emit("turn_done", struct{}{})
	return e.err
}
