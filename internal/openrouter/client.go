// Package openrouter is a minimal client for OpenRouter's Chat Completions and
// Responses APIs. It owns protocol encoding, streaming, and normalization into
// the shared conversation types. It does not depend on the agent harness.
package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type StreamErrorKind string

const (
	defaultStreamIdleTimeout      = 5 * time.Minute
	providerDialTimeout           = 30 * time.Second
	providerTLSHandshakeTimeout   = 10 * time.Second
	providerResponseHeaderTimeout = 5 * time.Minute
	maxRetryAfter                 = 10 * time.Minute
	maxProviderErrorBody          = 64 << 10
)

// ErrStreamIdle reports a provider stream that sent nothing for longer than
// the client's idle timeout.
var ErrStreamIdle = errors.New("provider stream idle timeout")

const (
	StreamProviderError           StreamErrorKind = "provider_error"
	StreamProviderResponseInvalid StreamErrorKind = "provider_response_invalid"
)

// StreamError exposes only stable structural classification to the harness.
// Err remains local diagnostic detail and must never be copied into durable
// terminal evidence.
type StreamError struct {
	Kind       StreamErrorKind
	HTTPStatus int
	// RetryAfter is the provider's Retry-After hint on a non-2xx response,
	// clamped to maxRetryAfter; zero when absent, malformed, or in the past.
	RetryAfter time.Duration
	// NoResponse marks a transport failure before any HTTP status arrived,
	// such as a refused, reset, or closed connection. Timeouts, idle streams,
	// and caller cancellation are never marked.
	NoResponse bool
	// ContextLengthExceeded marks a non-2xx response whose bounded error body
	// says the request exceeded the model's context window. Only this bit is
	// derived from the untrusted body; the body itself is never retained.
	ContextLengthExceeded bool
	Err                   error
}

func (e *StreamError) Error() string { return e.Err.Error() }
func (e *StreamError) Unwrap() error { return e.Err }

func streamError(kind StreamErrorKind, err error) error {
	return &StreamError{Kind: kind, Err: err}
}

// transportFailure classifies an error from sending a request. Only a failure
// before any response that is neither a timeout nor cancellation is marked
// NoResponse; the harness may retry those because nothing was generated.
func transportFailure(caller context.Context, watchdog *streamWatchdog, err error) error {
	if watchdog.idle() {
		return watchdog.failure()
	}
	var netErr net.Error
	timedOut := errors.As(err, &netErr) && netErr.Timeout()
	return &StreamError{Kind: StreamProviderError, NoResponse: caller.Err() == nil && !timedOut, Err: err}
}

// httpStatusFailure reports a non-2xx response by status alone. Provider error
// bodies are untrusted and may echo credentials or request content, so a
// bounded prefix is drained for connection reuse and never surfaced. A 400 or
// 413 body is only scanned for a context-length rejection marker.
func httpStatusFailure(resp *http.Response, message string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProviderErrorBody))
	return &StreamError{
		Kind:                  StreamProviderError,
		HTTPStatus:            resp.StatusCode,
		RetryAfter:            parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		ContextLengthExceeded: contextLengthRejection(resp.StatusCode, body),
		Err:                   errors.New(message),
	}
}

// contextLengthMarkers are lowercase phrases providers use when a request is
// larger than the model's context window.
var contextLengthMarkers = []string{
	"context_length_exceeded",
	"context length",
	"context window",
	"maximum context",
	"prompt is too long",
	"input is too long",
	"reduce the length",
}

// contextLengthRejection reports whether a client-error response refused the
// request for its size. Server errors never qualify; 413 always does.
func contextLengthRejection(status int, body []byte) bool {
	switch status {
	case http.StatusRequestEntityTooLarge:
		return true
	case http.StatusBadRequest:
		text := strings.ToLower(string(body))
		for _, marker := range contextLengthMarkers {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

// parseRetryAfter accepts delta-seconds or an HTTP date and clamps the result
// to [0, maxRetryAfter]. Anything else is treated as absent.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if strings.Trim(value, "0123456789") == "" {
		seconds, err := strconv.ParseUint(value, 10, 64)
		if err != nil || seconds > uint64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	return min(max(when.Sub(now), 0), maxRetryAfter)
}

// streamWatchdog cancels a streaming request once it has gone streamIdleTimeout
// without a received line, including the wait for response headers. It bounds
// silence, not total duration, so long healthy streams are unaffected.
type streamWatchdog struct {
	timeout time.Duration
	timer   *time.Timer
	ctx     context.Context
	cancel  context.CancelCauseFunc
}

func (c *Client) watchStream(parent context.Context) *streamWatchdog {
	ctx, cancel := context.WithCancelCause(parent)
	w := &streamWatchdog{timeout: c.streamIdleTimeout, ctx: ctx, cancel: cancel}
	if w.timeout > 0 {
		w.timer = time.AfterFunc(w.timeout, func() { cancel(ErrStreamIdle) })
	}
	return w
}

func (w *streamWatchdog) touch() {
	if w != nil && w.timer != nil {
		w.timer.Reset(w.timeout)
	}
}

func (w *streamWatchdog) stop() {
	if w.timer != nil {
		w.timer.Stop()
	}
	w.cancel(nil)
}

// idle reports whether the watchdog, rather than the caller, ended the request.
func (w *streamWatchdog) idle() bool {
	return w != nil && errors.Is(context.Cause(w.ctx), ErrStreamIdle)
}

func (w *streamWatchdog) failure() error {
	return streamError(StreamProviderError, fmt.Errorf("%w: no data for %s", ErrStreamIdle, w.timeout))
}

// NewClient is the only way to build a Client: it rejects an empty API key
// up front so a misconfigured environment fails at startup with a clear
// message instead of failing weirdly at the first request.
func NewClient(key string) (*Client, error) {
	if key == "" {
		return nil, errors.New("API key is empty")
	}

	return &Client{
		apiKey:                  key,
		baseURL:                 "https://openrouter.ai/api/v1/chat/completions",
		apiBaseURL:              "https://openrouter.ai/api/v1",
		httpClient:              newProviderHTTPClient(),
		contextDiscoveryTimeout: 3 * time.Second,
		streamIdleTimeout:       defaultStreamIdleTimeout,
	}, nil
}

// newProviderHTTPClient bounds each connection phase. It deliberately has no
// total timeout: streams may legitimately run long, and stream silence is
// bounded separately by the idle watchdog.
func newProviderHTTPClient() *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// An instrumented process (such as the opt-in production reader
		// capture) replaced the default transport; route through it unchanged.
		return &http.Client{}
	}
	transport := base.Clone()
	transport.DialContext = (&net.Dialer{Timeout: providerDialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = providerTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = providerResponseHeaderTimeout
	return &http.Client{Transport: transport}
}

// StreamHandlers carries the live callbacks ChatStream invokes as fragments
// arrive. A zero StreamHandlers streams nothing and assembles normally.
type StreamHandlers struct {
	OnContent func(string)
	// An empty fragment starts the visible wait before any public summary;
	// it carries no output, so callers may still retry the request after it.
	// Private reasoning and encrypted continuation never enter this callback.
	OnReasoning func(string)
}

// ChatStream selects the model's protocol and returns a complete normalized
// response. Astra requires a successful Responses completion event; legacy
// Chat streams finish at their [DONE] sentinel.
func (c *Client) ChatStream(ctx context.Context, r ChatRequest, h StreamHandlers) (ChatResponse, error) {
	if UsesResponses(r.Model) {
		if r.prepared != nil && !r.prepared.stream {
			return ChatResponse{}, streamError(StreamProviderError, errors.New("prepared request is not streaming"))
		}
		r.Stream = true
		return c.responses(ctx, r, h)
	}
	r.Stream = true
	jsonBody, err := encodeChatRequest(r)
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to marshal json: %w", err))
	}

	watchdog := c.watchStream(ctx)
	defer watchdog.stop()
	req, err := http.NewRequestWithContext(
		watchdog.ctx,
		http.MethodPost,
		c.baseURL,
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to build request: %w", err))
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, transportFailure(ctx, watchdog, fmt.Errorf("failed to get response: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ChatResponse{}, httpStatusFailure(resp, fmt.Sprintf("api returned status %d", resp.StatusCode))
	}

	var (
		msg          Message
		details      []json.RawMessage
		finishReason string
		gotChunk     bool
		completed    bool
		usage        *TokenUsage
		toolCalls    = make(map[int]*ToolCall)
	)
	msg.Role = "assistant"

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		watchdog.touch() // any line, including keepalive comments, is liveness
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue // blank lines and ": keepalive" comments
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			completed = true
			break
		}

		chunkUsage, hasNonNullUsage, err := parseProviderUsage([]byte(data))
		if err != nil {
			return ChatResponse{}, streamError(StreamProviderResponseInvalid, fmt.Errorf("failed to parse stream chunk: %w", err))
		}
		if hasNonNullUsage {
			usage = chunkUsage
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return ChatResponse{}, streamError(StreamProviderResponseInvalid, fmt.Errorf("failed to parse stream chunk: %w", err))
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		gotChunk = true
		choice := chunk.Choices[0]

		if choice.Delta.Reasoning != "" {
			msg.Reasoning += choice.Delta.Reasoning
			if h.OnReasoning != nil {
				h.OnReasoning(choice.Delta.Reasoning)
			}
		}

		// A provider may place the last reasoning fragment and first content
		// fragment in one chunk. Render reasoning first so consumers observe a
		// single monotonic reasoning -> content transition.
		if choice.Delta.Content != "" {
			msg.Content += choice.Delta.Content
			if h.OnContent != nil {
				h.OnContent(choice.Delta.Content)
			}
		}

		if len(choice.Delta.ReasoningDetails) != 0 {
			details = append(details, choice.Delta.ReasoningDetails...)
		}
		for _, tcd := range choice.Delta.ToolCalls {
			if tcd.Index == nil {
				return ChatResponse{}, streamError(
					StreamProviderResponseInvalid,
					errors.New("provider tool call fragment is missing its index"),
				)
			}
			index := *tcd.Index
			if index < 0 {
				return ChatResponse{}, streamError(
					StreamProviderResponseInvalid,
					fmt.Errorf("provider tool call index %d is negative", index),
				)
			}
			tc := toolCalls[index]
			if tc == nil {
				tc = &ToolCall{}
				toolCalls[index] = tc
			}
			if tcd.ID != "" {
				tc.ID = tcd.ID
			}
			if tcd.Type != "" {
				tc.Type = tcd.Type
			}
			if tcd.Function.Name != "" {
				tc.Function.Name = tcd.Function.Name
			}
			tc.Function.Arguments += tcd.Function.Arguments
		}
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
		if finishReason == "error" {
			// OpenRouter reports a failure after streaming began this way, and
			// may not send [DONE]. Its error object is untrusted detail.
			return ChatResponse{}, streamError(StreamProviderError, errors.New("provider reported a generation error"))
		}
	}
	if err := scanner.Err(); err != nil {
		if watchdog.idle() {
			return ChatResponse{}, watchdog.failure()
		}
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to read stream: %w", err))
	}
	if !completed {
		return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New("stream ended before [DONE]"))
	}
	if !gotChunk {
		return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New("stream contained no chunks"))
	}
	if finishReason == "length" {
		// Truncated text or tool arguments must never look like a completed
		// generation, matching the Responses path's incomplete status.
		return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New("provider stopped at the output token limit"))
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = make([]ToolCall, len(toolCalls))
		for i := range msg.ToolCalls {
			call := toolCalls[i]
			if call == nil {
				return ChatResponse{}, streamError(
					StreamProviderResponseInvalid,
					fmt.Errorf("provider tool call indices are not contiguous at index %d", i),
				)
			}
			msg.ToolCalls[i] = *call
		}
	}
	if len(details) > 0 {
		msg.ReasoningDetails, err = json.Marshal(details)
		if err != nil {
			return ChatResponse{}, streamError(StreamProviderResponseInvalid, fmt.Errorf("failed to marshal reasoning details: %w", err))
		}
	}

	return ChatResponse{Choices: []Choice{{Message: msg, FinishReason: finishReason}}, Usage: usage}, nil
}

// Chat sends one chat-completions request and returns the parsed response.
// It normalizes every failure mode — marshal errors, transport errors,
// non-200 statuses, unparseable bodies, and responses with no choices —
// into a single error return, so callers may rely on Choices[0] existing
// whenever err is nil.
func (c *Client) Chat(r ChatRequest) (ChatResponse, error) {
	if UsesResponses(r.Model) {
		if r.prepared != nil && r.prepared.stream {
			return ChatResponse{}, errors.New("prepared request is streaming")
		}
		r.Stream = false
		return c.responses(context.Background(), r, StreamHandlers{})
	}
	jsonBody, err := encodeChatRequest(r)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to marshal json: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to send request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to get response: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, httpStatusFailure(resp, fmt.Sprintf("api returned status %d", resp.StatusCode))
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponsesBody+1))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(bodyBytes) > maxResponsesBody {
		return ChatResponse{}, errors.New("response exceeds byte limit")
	}

	var wireResponse struct {
		Choices []Choice `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &wireResponse); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}
	usage, _, err := parseProviderUsage(bodyBytes)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}
	chatResp := ChatResponse{Choices: wireResponse.Choices, Usage: usage}

	if len(chatResp.Choices) == 0 {
		return ChatResponse{}, errors.New("response contained no choices")
	}

	return chatResp, nil
}
