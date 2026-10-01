package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func chatStreamAgainst(t *testing.T, handler http.HandlerFunc) (*Client, func(context.Context) (ChatResponse, error)) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	return client, func(ctx context.Context) (ChatResponse, error) {
		return client.ChatStream(ctx, ChatRequest{Model: "test"}, StreamHandlers{})
	}
}

// L2: a truncated or errored generation must never look like a completed one.
func TestChatStreamRejectsTruncatedAndErroredFinishReasons(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		wantKind StreamErrorKind
	}{
		{
			name:     "length-truncated answer",
			body:     "data: {\"choices\":[{\"delta\":{\"content\":\"The answer is\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n",
			wantKind: StreamProviderResponseInvalid,
		},
		{
			name: "length-truncated tool arguments",
			body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"/fo\"}}]},\"finish_reason\":\"length\"}]}\n\n" +
				"data: [DONE]\n",
			wantKind: StreamProviderResponseInvalid,
		},
		{
			name:     "mid-stream provider error without sentinel",
			body:     "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"error\":{\"message\":\"upstream secret\"},\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"error\"}]}\n\n",
			wantKind: StreamProviderError,
		},
		{
			name:     "mid-stream provider error with sentinel",
			body:     "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"error\"}]}\n\ndata: [DONE]\n",
			wantKind: StreamProviderError,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, stream := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, test.body)
			})
			_, err := stream(context.Background())
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.Kind != test.wantKind {
				t.Fatalf("error=%v typed=%+v want kind %q", err, streamErr, test.wantKind)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked provider detail: %v", err)
			}
		})
	}
}

func TestChatStreamAcceptsCompletedFinishReasons(t *testing.T) {
	for _, reason := range []string{"stop", "tool_calls", ""} {
		_, stream := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\""+reason+"\"}]}\n\ndata: [DONE]\n")
		})
		response, err := stream(context.Background())
		if err != nil || response.Choices[0].Message.Content != "done" {
			t.Fatalf("reason=%q response=%+v err=%v", reason, response, err)
		}
	}
}

// L3: a stream that stops sending bytes fails instead of blocking forever.
func TestChatStreamFailsWhenProviderGoesIdle(t *testing.T) {
	for _, test := range []struct {
		name       string
		firstChunk string
	}{
		{name: "silent before headers"},
		{name: "silent after partial output", firstChunk: "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"},
		{name: "silent after keepalive", firstChunk: ": OPENROUTER PROCESSING\n\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			release := make(chan struct{})
			client, _ := chatStreamAgainst(t, func(w http.ResponseWriter, r *http.Request) {
				if test.firstChunk != "" {
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, test.firstChunk)
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			})
			t.Cleanup(func() { close(release) })
			client.streamIdleTimeout = 50 * time.Millisecond
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var content strings.Builder
			started := time.Now()
			_, err := client.ChatStream(ctx, ChatRequest{Model: "test"}, StreamHandlers{OnContent: func(s string) { content.WriteString(s) }})
			elapsed := time.Since(started)
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError || streamErr.NoResponse {
				t.Fatalf("error=%v typed=%+v", err, streamErr)
			}
			if !errors.Is(err, ErrStreamIdle) || ctx.Err() != nil || elapsed > 2*time.Second {
				t.Fatalf("error=%v ctx=%v elapsed=%s, want idle failure well before caller deadline", err, ctx.Err(), elapsed)
			}
		})
	}
}

func TestChatStreamIdleTimerResetsOnEveryLine(t *testing.T) {
	client, _ := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		for range 8 {
			_, _ = io.WriteString(w, ": OPENROUTER PROCESSING\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(25 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\ndata: [DONE]\n")
	})
	client.streamIdleTimeout = 150 * time.Millisecond
	response, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
	if err != nil || response.Choices[0].Message.Content != "done" {
		t.Fatalf("a slow but live stream failed: response=%+v err=%v", response, err)
	}
}

func TestResponsesStreamFailsWhenProviderGoesIdle(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client, _ := NewClient("key")
	client.apiBaseURL = server.URL
	client.streamIdleTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.ChatStream(ctx, ChatRequest{Model: AstraModel}, StreamHandlers{})
	var streamErr *StreamError
	if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError || !errors.Is(err, ErrStreamIdle) || ctx.Err() != nil {
		t.Fatalf("error=%v typed=%+v ctx=%v", err, streamErr, ctx.Err())
	}
}

func TestNewClientBoundsConnectionPhases(t *testing.T) {
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil || transport.Proxy == nil ||
		transport.TLSHandshakeTimeout != providerTLSHandshakeTimeout ||
		transport.ResponseHeaderTimeout != providerResponseHeaderTimeout {
		t.Fatalf("transport=%+v", client.httpClient.Transport)
	}
	if client.httpClient.Timeout != 0 {
		t.Fatalf("total client timeout=%s would cut off long legitimate streams", client.httpClient.Timeout)
	}
	if client.streamIdleTimeout != defaultStreamIdleTimeout || defaultStreamIdleTimeout <= 0 {
		t.Fatalf("stream idle timeout=%s", client.streamIdleTimeout)
	}
}

func TestNewClientRoutesThroughInstrumentedDefaultTransport(t *testing.T) {
	prior := http.DefaultTransport
	var used atomic.Bool
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		used.Store(true)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = prior })
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = "http://provider.test"
	if _, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{}); err == nil || !used.Load() {
		t.Fatalf("error=%v instrumented transport used=%v", err, used.Load())
	}
}

// L4 transport contract: retry policy lives in the agent, but only the
// transport can tell whether any response arrived and what Retry-After said.
func TestChatStreamMarksFailuresBeforeAnyResponse(t *testing.T) {
	t.Run("connection closed before headers", func(t *testing.T) {
		_, stream := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		})
		_, err := stream(context.Background())
		var streamErr *StreamError
		if !errors.As(err, &streamErr) || streamErr.Kind != StreamProviderError || streamErr.HTTPStatus != 0 || !streamErr.NoResponse {
			t.Fatalf("error=%v typed=%+v", err, streamErr)
		}
	})
	t.Run("connection refused", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		url := server.URL
		server.Close()
		client, _ := NewClient("key")
		client.baseURL = url
		_, err := client.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
		var streamErr *StreamError
		if !errors.As(err, &streamErr) || !streamErr.NoResponse {
			t.Fatalf("error=%v typed=%+v", err, streamErr)
		}
	})
	t.Run("caller cancellation is not a missing response", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		client, _ := chatStreamAgainst(t, func(w http.ResponseWriter, r *http.Request) {
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		})
		t.Cleanup(func() { close(release) })
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := client.ChatStream(ctx, ChatRequest{Model: "test"}, StreamHandlers{})
			done <- err
		}()
		<-started
		cancel()
		err := <-done
		var streamErr *StreamError
		if !errors.Is(err, context.Canceled) || (errors.As(err, &streamErr) && streamErr.NoResponse) {
			t.Fatalf("error=%v typed=%+v", err, streamErr)
		}
	})
	t.Run("HTTP status is a response", func(t *testing.T) {
		_, stream := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		_, err := stream(context.Background())
		var streamErr *StreamError
		if !errors.As(err, &streamErr) || streamErr.NoResponse || streamErr.HTTPStatus != http.StatusBadGateway {
			t.Fatalf("error=%v typed=%+v", err, streamErr)
		}
	})
}

func TestChatStreamReportsRetryAfter(t *testing.T) {
	for _, test := range []struct {
		name, header string
		want         time.Duration
		atLeast      bool
	}{
		{name: "seconds", header: "7", want: 7 * time.Second},
		{name: "zero", header: "0", want: 0},
		{name: "absent", header: "", want: 0},
		{name: "malformed", header: "soon", want: 0},
		{name: "negative", header: "-3", want: 0},
		{name: "http date", header: time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat), want: 15 * time.Second, atLeast: true},
		{name: "past http date", header: time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), want: 0},
		{name: "huge", header: "999999999999999999999", want: maxRetryAfter},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, stream := chatStreamAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
				if test.header != "" {
					w.Header().Set("Retry-After", test.header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			})
			_, err := stream(context.Background())
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.HTTPStatus != http.StatusTooManyRequests {
				t.Fatalf("error=%v typed=%+v", err, streamErr)
			}
			if test.atLeast {
				if streamErr.RetryAfter < test.want || streamErr.RetryAfter > 21*time.Second {
					t.Fatalf("RetryAfter=%s want about %s", streamErr.RetryAfter, test.want)
				}
				return
			}
			if streamErr.RetryAfter != test.want {
				t.Fatalf("RetryAfter=%s want %s", streamErr.RetryAfter, test.want)
			}
		})
	}
}

// L7: provider error bodies are untrusted, may be huge, and may echo content.
type countingBody struct {
	read      atomic.Int64
	remaining int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > b.remaining {
		n = b.remaining
	}
	chunk := strings.Repeat("secret-token ", int(n)/13+1)
	copy(p, chunk[:n])
	b.remaining -= n
	b.read.Add(n)
	return int(n), nil
}

func (*countingBody) Close() error { return nil }

func TestChatPathWithholdsAndBoundsProviderErrorBodies(t *testing.T) {
	const bodySize = 8 << 20
	for _, test := range []struct {
		name   string
		status int
		call   func(*Client) error
	}{
		{name: "stream non-2xx", status: http.StatusBadGateway, call: func(c *Client) error {
			_, err := c.ChatStream(context.Background(), ChatRequest{Model: "test"}, StreamHandlers{})
			return err
		}},
		{name: "non-stream non-2xx", status: http.StatusBadRequest, call: func(c *Client) error {
			_, err := c.Chat(ChatRequest{Model: "test"})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &countingBody{remaining: bodySize}
			client, err := NewClient("key")
			if err != nil {
				t.Fatal(err)
			}
			client.baseURL = "http://provider.test"
			client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{}, Body: body}, nil
			})}
			err = test.call(client)
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error=%v", err)
			}
			if !strings.Contains(err.Error(), "status") {
				t.Fatalf("error=%v lacks the HTTP status summary", err)
			}
			if read := body.read.Load(); read > maxProviderErrorBody+64<<10 {
				t.Fatalf("read %d bytes of an error body, want at most about %d", read, maxProviderErrorBody)
			}
		})
	}
}

func TestChatWithholdsBodyWhenResponseHasNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[],"echo":"secret-token"}`)
	}))
	t.Cleanup(server.Close)
	client, _ := NewClient("key")
	client.baseURL = server.URL
	_, err := client.Chat(ChatRequest{Model: "test"})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error=%v", err)
	}
}

// C7: the application default model survives a failed or slow metadata lookup.
func TestResolveContextProfileDefaultModelFallback(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")
	const model = "deepseek/deepseek-v4.1-flash"
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "metadata unavailable", handler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}},
		{name: "metadata slower than discovery deadline", handler: func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := contextProfileClient(t, test.handler)
			client.contextDiscoveryTimeout = 20 * time.Millisecond
			for _, resolve := range []func(context.Context, string) (ContextProfile, error){
				client.ResolveContextProfile, client.ResolveChatContextProfile,
			} {
				profile, err := resolve(context.Background(), model)
				if err != nil {
					t.Fatal(err)
				}
				d := profile.Diagnostics()
				if d.Source != ContextProfileBuiltinFallback || d.ConfiguredModel != model || d.CanonicalModel != model ||
					d.HardWindowTokens != 262144 || d.WorkingTokens != 262144 || d.AdvertisedWindowTokens != 0 || d.AdvertisedModel != "" {
					t.Fatalf("diagnostics=%+v", d)
				}
			}
		})
	}
}

func TestBuiltinFallbackProfilesRejectUnlistedModels(t *testing.T) {
	for _, model := range []string{"custom/model", AstraModel, "deepseek/deepseek-v4.1"} {
		_, err := newContextProfile(ContextProfileDiagnostics{
			ConfiguredModel: model, CanonicalModel: model, HardWindowTokens: 262144, WorkingTokens: 262144,
			OutputReserveTokens: 16384, EstimationMarginTokens: contextEstimationMargin, Source: ContextProfileBuiltinFallback,
		})
		if err == nil {
			t.Fatalf("model %q accepted a built-in fallback profile", model)
		}
	}
}
