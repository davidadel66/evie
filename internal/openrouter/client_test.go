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
	usageJSON, err := json.Marshal(res.Usage)
	if err != nil {
		t.Fatal(err)
	}
	const wantUsage = `{"input_tokens":135,"output_tokens":74,"total_tokens":209,"reasoning_output_tokens":29,"cached_input_tokens":0,"cache_write_input_tokens":0}`
	if string(usageJSON) != wantUsage {
		t.Fatalf("captured Kimi usage = %s, want %s", usageJSON, wantUsage)
	}
	for _, excluded := range []string{"cost", "is_byok", "audio_tokens", "video_tokens", "image_tokens", "provider", "model", "service_tier"} {
		if strings.Contains(string(usageJSON), excluded) {
			t.Errorf("provider-only field %q crossed usage boundary: %s", excluded, usageJSON)
		}
	}
}

func TestChatStreamAndChatNormalizeUsageIdentically(t *testing.T) {
	tests := []struct {
		name         string
		usageMembers string
		want         string
	}{
		{
			name: "complete",
			usageMembers: `"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,` +
				`"completion_tokens_details":{"reasoning_tokens":4},` +
				`"prompt_tokens_details":{"cached_tokens":5,"cache_write_tokens":6}}`,
			want: `{"input_tokens":1,"output_tokens":2,"total_tokens":3,"reasoning_output_tokens":4,"cached_input_tokens":5,"cache_write_input_tokens":6}`,
		},
		{
			name:         "partial preserves reported zero",
			usageMembers: `"usage":{"prompt_tokens":0,"completion_tokens":null,"total_tokens":7}`,
			want:         `{"input_tokens":0,"total_tokens":7}`,
		},
		{
			name:         "maximum signed 64-bit counter",
			usageMembers: `"usage":{"total_tokens":9223372036854775807}`,
			want:         `{"total_tokens":9223372036854775807}`,
		},
		{
			name:         "arithmetic inconsistency is reported evidence",
			usageMembers: `"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":1,"completion_tokens_details":{"reasoning_tokens":30}}`,
			want:         `{"input_tokens":10,"output_tokens":20,"total_tokens":1,"reasoning_output_tokens":30}`,
		},
		{
			name:         "malformed counters preserve valid siblings",
			usageMembers: `"usage":{"prompt_tokens":-1,"completion_tokens":1.5,"total_tokens":9,"completion_tokens_details":{"reasoning_tokens":1e2},"prompt_tokens_details":{"cached_tokens":"8","cache_write_tokens":9223372036854775808}}`,
			want:         `{"total_tokens":9}`,
		},
		{
			name:         "duplicate direct counter omits only destination",
			usageMembers: `"usage":{"prompt_tokens":1,"prompt_tokens":2,"completion_tokens":3}`,
			want:         `{"output_tokens":3}`,
		},
		{
			name:         "repeated detail containers track full paths",
			usageMembers: `"usage":{"prompt_tokens_details":{"cached_tokens":4},"prompt_tokens_details":{"cache_write_tokens":5}}`,
			want:         `{"cached_input_tokens":4,"cache_write_input_tokens":5}`,
		},
		{
			name:         "duplicate nested path preserves sibling",
			usageMembers: `"usage":{"prompt_tokens_details":{"cached_tokens":4,"cache_write_tokens":5},"prompt_tokens_details":{"cached_tokens":6}}`,
			want:         `{"cache_write_input_tokens":5}`,
		},
		{name: "null", usageMembers: `"usage":null`, want: `null`},
		{name: "empty", usageMembers: `"usage":{}`, want: `null`},
		{name: "unknown only", usageMembers: `"usage":{"future_tokens":8}`, want: `null`},
		{name: "excluded only", usageMembers: `"usage":{"cost":1.2,"is_byok":true,"prompt_tokens_details":{"audio_tokens":4}}`, want: `null`},
		{name: "non-object", usageMembers: `"usage":7`, want: `null`},
		{name: "duplicate top-level usage", usageMembers: `"usage":{"prompt_tokens":1},"usage":{"total_tokens":2}`, want: `null`},
		{name: "non-object detail containers", usageMembers: `"usage":{"prompt_tokens":1,"prompt_tokens_details":[],"completion_tokens_details":null}`, want: `{"input_tokens":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamBody := `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}],` + tt.usageMembers + `}`
			streamResponse := streamResponseForTest(t, streamBody)
			chatBody := `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` + tt.usageMembers + `}`
			chatResponse := chatResponseForTest(t, chatBody)

			streamJSON, err := json.Marshal(streamResponse.Usage)
			if err != nil {
				t.Fatal(err)
			}
			chatJSON, err := json.Marshal(chatResponse.Usage)
			if err != nil {
				t.Fatal(err)
			}
			if string(streamJSON) != tt.want || string(chatJSON) != tt.want {
				t.Fatalf("stream usage=%s chat usage=%s, want %s", streamJSON, chatJSON, tt.want)
			}
		})
	}
}

func TestChatStreamUsesLastNonNullUsageOccurrenceWithoutMerging(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   string
	}{
		{
			name: "trailing usage-only chunk replaces earlier usage",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
				`{"choices":[],"usage":null}`,
				`{"choices":[],"usage":{"total_tokens":9}}`,
			},
			want: `{"total_tokens":9}`,
		},
		{
			name: "partial final replaces rather than merges",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
				`{"choices":[],"usage":{"completion_tokens":4}}`,
			},
			want: `{"output_tokens":4}`,
		},
		{
			name: "trailing null is ignored",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1}}`,
				`{"choices":[],"usage":null}`,
			},
			want: `{"input_tokens":1}`,
		},
		{
			name: "invalid-only final removes earlier usage",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1}}`,
				`{"choices":[],"usage":{"prompt_tokens":-1}}`,
			},
			want: `null`,
		},
		{
			name: "duplicate final removes earlier usage",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1}}`,
				`{"choices":[],"usage":{"prompt_tokens":2},"usage":{"total_tokens":3}}`,
			},
			want: `null`,
		},
		{
			name: "empty final removes earlier usage",
			chunks: []string{
				`{"choices":[{"delta":{"content":"ok"}}],"usage":{"prompt_tokens":1}}`,
				`{"choices":[],"usage":{}}`,
			},
			want: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := streamChunksResponseForTest(t, tt.chunks)
			got, err := json.Marshal(response.Usage)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("usage=%s, want %s", got, tt.want)
			}
			if response.Choices[0].Message.Content != "ok" {
				t.Fatalf("assistant content=%q", response.Choices[0].Message.Content)
			}
		})
	}
}

func TestChatRequestsDoNotAskProviderForUsage(t *testing.T) {
	assertRequest := func(t *testing.T, request *http.Request) {
		t.Helper()
		var body map[string]json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"stream_options", "include_usage", "usage"} {
			if _, ok := body[forbidden]; ok {
				t.Errorf("request unexpectedly contains %q: %v", forbidden, body)
			}
		}
	}

	t.Run("streaming", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertRequest(t, r)
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n"))
		}))
		defer srv.Close()
		client, _ := NewClient("key")
		client.baseURL = srv.URL
		if _, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("non-streaming", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertRequest(t, r)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
		}))
		defer srv.Close()
		client, _ := NewClient("key")
		client.baseURL = srv.URL
		if _, err := client.Chat(ChatRequest{Model: "test"}); err != nil {
			t.Fatal(err)
		}
	})
}

func streamResponseForTest(t *testing.T, chunk string) ChatResponse {
	t.Helper()
	return streamChunksResponseForTest(t, []string{chunk})
}

func streamChunksResponseForTest(t *testing.T, chunks []string) ChatResponse {
	t.Helper()
	var body strings.Builder
	for _, chunk := range chunks {
		fmt.Fprintf(&body, "data: %s\n\n", chunk)
	}
	body.WriteString("data: [DONE]\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body.String()))
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
	return response
}

func chatResponseForTest(t *testing.T, body string) ChatResponse {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = srv.URL
	response, err := client.Chat(ChatRequest{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestChatStreamEmitsSameChunkReasoningBeforeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"think\",\"content\":\"answer\"}}]}\n\n" +
			"data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient("test-key")
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = srv.URL
	var callbacks []string
	res, err := c.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{
		OnReasoning: func(text string) { callbacks = append(callbacks, "reasoning:"+text) },
		OnContent:   func(text string) { callbacks = append(callbacks, "content:"+text) },
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"reasoning:think", "content:answer"}
	if fmt.Sprint(callbacks) != fmt.Sprint(want) {
		t.Fatalf("callbacks=%v, want %v", callbacks, want)
	}
	if msg := res.Choices[0].Message; msg.Reasoning != "think" || msg.Content != "answer" {
		t.Fatalf("assembled message=%+v", msg)
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
		{name: "missing tool index", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"echo\"}}]}}]}\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
		{name: "missing tool index on later fragment", status: http.StatusOK, body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"echo\"}}]}}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"arguments\":\"{}\"}}]}}]}\n\ndata: [DONE]\n", wantKind: StreamProviderResponseInvalid},
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
			t.Parallel()
			client, err := NewClient("key")
			if err != nil {
				t.Fatal(err)
			}
			client.baseURL = "http://provider.test"
			client.httpClient = &http.Client{Transport: tt.transport}
			_, err = client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError || streamErr.HTTPStatus != tt.status {
				t.Fatalf("error=%v typed=%+v", err, streamErr)
			}
		})
	}
}
