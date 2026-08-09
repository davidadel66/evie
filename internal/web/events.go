// Package web is the evie web frontend: an HTTP server that drives an
// agent.Session and streams each turn to the browser as server-sent
// events. This file is the encoding half — sseEvents is the web twin of
// the REPL's replEvents: same agent.Events contract, but the methods
// write SSE blocks into the HTTP response instead of printing.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sseEvents renders a turn onto one open HTTP response. Every emit is
// flushed immediately — an unflushed event sits in Go's response buffer
// and the stream stops being a stream.
type sseEvents struct {
	w http.ResponseWriter
	f http.Flusher
	// wrote flips on the first emit. Until then the response is
	// uncommitted and the handler may still abandon streaming for a
	// plain status reply (the 409-busy path).
	wrote bool
}

// newSSEEvents claims the response as an event stream: sets the SSE
// headers and returns the writer. Errors if the ResponseWriter can't
// flush (no streaming through it means no point continuing).
func newSSEEvents(w http.ResponseWriter) (*sseEvents, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	return &sseEvents{w: w, f: f}, nil
}

// emit writes one SSE block: event name, one line of JSON, blank line,
// flush. json.Marshal escapes newlines inside strings, which is what
// keeps any payload a single data: line — the format's one hard rule.
func (e *sseEvents) emit(event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		// Payloads are our own flat structs; this cannot fail. Guard
		// anyway so a future payload type can't wedge the stream.
		data = []byte("{}")
	}
	e.wrote = true
	fmt.Fprintf(e.w, "event: %s\ndata: %s\n\n", event, data)
	e.f.Flush()
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

// The three below aren't part of agent.Events — they're the server's own
// vocabulary around a turn (see serve.spec.md).

func (e *sseEvents) ApprovalRequest(id, name, args string) {
	e.emit("approval_request", struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Args string `json:"args"`
	}{id, name, args})
}

func (e *sseEvents) Error(message string) {
	e.emit("error", struct {
		Message string `json:"message"`
	}{message})
}

func (e *sseEvents) TurnDone() {
	e.emit("turn_done", struct{}{})
}
