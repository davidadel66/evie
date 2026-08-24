package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingReadCloser struct {
	reader *bytes.Reader
	err    error
}

func (r *failingReadCloser) Read(p []byte) (int, error) {
	if r.reader.Len() > 0 {
		return r.reader.Read(p)
	}
	return 0, r.err
}
func (*failingReadCloser) Close() error { return nil }

// serveFixture builds a local SSE server replaying one captured stream,
// and a Client pointed at it.
func serveFixture(t *testing.T, path string) *Client {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = srv.URL
	return c
}

func TestChatStreamAssemblesReasoning(t *testing.T) {
	c := serveFixture(t, "testdata/kimi-reasoning-stream.txt")

	var contentFrags, reasoningFrags []string
	res, err := c.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{
		OnContent:   func(s string) { contentFrags = append(contentFrags, s) },
		OnReasoning: func(s string) { reasoningFrags = append(reasoningFrags, s) },
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	msg := res.Choices[0].Message

	const wantReasoning = "Compute 17*23. 17*20 = 340, 17*3 = 51, total 391."
	const wantContent = "17 × 23:\n\n- 17 × 20 = 340\n- 17 × 3 = 51\n- 340 + 51 = **391**"

	if msg.Reasoning != wantReasoning {
		t.Errorf("assembled reasoning = %q, want %q", msg.Reasoning, wantReasoning)
	}
	if msg.Content != wantContent {
		t.Errorf("assembled content = %q, want %q", msg.Content, wantContent)
	}
	// Callbacks see every fragment, in order — joining them reproduces
	// the assembled fields exactly.
	if got := strings.Join(reasoningFrags, ""); got != msg.Reasoning {
		t.Errorf("joined OnReasoning fragments = %q, want %q", got, msg.Reasoning)
	}
	if got := strings.Join(contentFrags, ""); got != msg.Content {
		t.Errorf("joined OnContent fragments = %q, want %q", got, msg.Content)
	}

	// ReasoningDetails round-trips as one array of every part sent.
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.ReasoningDetails, &parts); err != nil {
		t.Fatalf("ReasoningDetails not a JSON array: %v", err)
	}
	if len(parts) != 26 {
		t.Errorf("ReasoningDetails has %d parts, want 26", len(parts))
	}
	if parts[0].Type != "reasoning.text" || parts[0].Text != "Compute" {
		t.Errorf("first part = %+v, want {reasoning.text Compute}", parts[0])
	}

	if got := res.Choices[0].FinishReason; got != "stop" {
		t.Errorf("finish reason = %q, want stop", got)
	}
}

func TestChatStreamWithoutReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(": keepalive\n" +
			"\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n" +
			"\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" there\"},\"finish_reason\":\"stop\"}]}\n" +
			"\n" +
			"data: [DONE]\n"))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = srv.URL

	reasoningCalled := false
	res, err := c.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{
		OnReasoning: func(string) { reasoningCalled = true },
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	msg := res.Choices[0].Message

	if msg.Content != "Hello there" {
		t.Errorf("content = %q, want %q", msg.Content, "Hello there")
	}
	// A stream with no reasoning must behave exactly as before the
	// feature: empty fields, key omitted, callback never fired.
	if msg.Reasoning != "" {
		t.Errorf("reasoning = %q, want empty", msg.Reasoning)
	}
	if msg.ReasoningDetails != nil {
		t.Errorf("ReasoningDetails = %s, want nil", msg.ReasoningDetails)
	}
	if reasoningCalled {
		t.Error("OnReasoning fired on a stream with no reasoning")
	}
}

func TestChatStreamSafelyAssemblesContiguousToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"echo\",\"arguments\":\"{\\\"x\\\":\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"1}\"}}]}}]}\n\n" +
				"data: [DONE]\n",
		))
	}))
	defer srv.Close()
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = srv.URL
	response, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
	if err != nil {
		t.Fatal(err)
	}
	calls := response.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Type != "function" ||
		calls[0].Function.Name != "echo" || calls[0].Function.Arguments != `{"x":1}` {
		t.Fatalf("tool calls=%+v", calls)
	}
}

func TestChatStreamContiguousToolCallCountHasNoUnapprovedCap(t *testing.T) {
	for _, count := range []int{127, 128, 129} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			toolCalls := make([]map[string]any, count)
			for i := range toolCalls {
				toolCalls[i] = map[string]any{
					"index": i,
					"id":    fmt.Sprintf("call-%d", i),
					"type":  "function",
					"function": map[string]any{
						"name": "echo",
					},
				}
			}
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": toolCalls},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n", chunk)
			}))
			defer srv.Close()
			client, err := NewClient("key")
			if err != nil {
				t.Fatal(err)
			}
			client.baseURL = srv.URL
			response, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(response.Choices[0].Message.ToolCalls); got != count {
				t.Fatalf("tool calls=%d, want %d", got, count)
			}
		})
	}
}

func TestChatStreamLeavesProviderNeutralValidityToAgent(t *testing.T) {
	for _, body := range []string{
		"data: {\"choices\":[{\"delta\":{}}]}\n\ndata: [DONE]\n",
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"type\":\"custom\",\"function\":{}}]}}]}\n\ndata: [DONE]\n",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		client, err := NewClient("key")
		if err != nil {
			t.Fatal(err)
		}
		client.baseURL = srv.URL
		if _, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{}); err != nil {
			t.Fatalf("provider-neutral validity rejected in transport: %v", err)
		}
		srv.Close()
	}
}

func TestChatStreamCancellationStopsHTTPRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = srv.URL

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.ChatStream(ctx, ChatRequest{Model: "test"}, StreamHandlers{})
		done <- err
	}()

	<-started
	cancel()

	var gotErr error
	select {
	case err := <-done:
		gotErr = err
	case <-time.After(time.Second):
		close(release)
		t.Fatal("ChatStream did not stop after context cancellation")
	}
	close(release)
	if !errors.Is(gotErr, context.Canceled) {
		t.Fatalf("ChatStream error = %v, want context.Canceled", gotErr)
	}
}

func TestChatStreamClassifiesProviderAndInvalidResponseFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantKind   StreamErrorKind
		wantStatus int
	}{
		{name: "non-2xx", status: http.StatusBadGateway, body: "raw provider body", wantKind: StreamProviderError, wantStatus: http.StatusBadGateway},
		{name: "2xx with no usable choice", status: http.StatusNoContent, wantKind: StreamProviderResponseInvalid},
		{name: "malformed streamed JSON", status: http.StatusOK, body: "data: {not-json}\n", wantKind: StreamProviderResponseInvalid},
		{name: "no chunks", status: http.StatusOK, body: ": keepalive\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
		{name: "missing completion sentinel", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n", wantKind: StreamProviderResponseInvalid},
		{name: "negative tool index", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":-1,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"echo\"}}]}}]}\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
		{name: "sparse tool index", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"echo\"}}]}}]}\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
		{name: "extreme tool index", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1000000000,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"echo\"}}]}}]}\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			client, err := NewClient("key")
			if err != nil {
				t.Fatal(err)
			}
			client.baseURL = srv.URL
			_, err = client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.Kind != tt.wantKind || streamErr.HTTPStatus != tt.wantStatus {
				t.Fatalf("error=%v typed=%+v want kind=%q status=%d", err, streamErr, tt.wantKind, tt.wantStatus)
			}
		})
	}
}

func TestChatStreamClassifiesRequestConstructionFailure(t *testing.T) {
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "://invalid-url"
	_, err = client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
	var streamErr *StreamError
	if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError {
		t.Fatalf("error=%v typed=%+v", err, streamErr)
	}
}

func TestChatStreamClassifiesTransportAndBodyIOAsProviderErrors(t *testing.T) {
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "http://provider.test"

	for _, tt := range []struct {
		name      string
		transport roundTripFunc
		status    int
	}{
		{
			name: "transport",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			},
		},
		{
			name: "stream scanner IO",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: &failingReadCloser{
					reader: bytes.NewReader([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n")),
					err:    errors.New("scanner read failed"),
				}}, nil
			},
		},
		{
			name: "non-2xx body IO",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: &failingReadCloser{
					reader: bytes.NewReader(nil), err: errors.New("body read failed"),
				}}, nil
			},
			status: http.StatusServiceUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			http.DefaultClient = &http.Client{Transport: tt.transport}
			_, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError || streamErr.HTTPStatus != tt.status {
				t.Fatalf("error=%v typed=%+v", err, streamErr)
			}
		})
	}
}
