// Package openrouter is a minimal client for OpenRouter's OpenAI-compatible
// chat-completions API. It owns the wire format: every type in schema.go
// mirrors the JSON OpenRouter sends or expects, field for field. Nothing in
// here knows about the agent harness — the harness imports this package,
// never the reverse. If a second provider is ever added, this package is
// the template: a sibling package translating the same ideas to different
// wire types.
package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type StreamErrorKind string

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
	Err        error
}

func (e *StreamError) Error() string { return e.Err.Error() }
func (e *StreamError) Unwrap() error { return e.Err }

func streamError(kind StreamErrorKind, err error) error {
	return &StreamError{Kind: kind, Err: err}
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
		httpClient:              &http.Client{},
		contextDiscoveryTimeout: 3 * time.Second,
	}, nil
}

// ChatStream sends one chat-completions request with streaming enabled,
// invoking onDelta with each content fragment as it arrives (print it
// for live output), and returns the fully assembled ChatResponse — the
// same shape Chat returns, so callers' downstream logic (history
// append, tool-call dispatch) is identical for both methods. Tool-call
// fragments are reassembled by index; OpenRouter's ": ..." keepalive
// comment lines are skipped per the SSE spec; the stream ends at the
// "[DONE]" sentinel.
// StreamHandlers carries the live callbacks ChatStream invokes as fragments
// arrive. A zero StreamHandlers streams nothing and assembles normally.
type StreamHandlers struct {
	OnContent   func(string)
	OnReasoning func(string)
}

func (c *Client) ChatStream(ctx context.Context, r ChatRequest, h StreamHandlers) (ChatResponse, error) {
	r.Stream = true
	jsonBody, err := json.Marshal(r)
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to marshal json: %w", err))
	}

	req, err := http.NewRequestWithContext(
		ctx,
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
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to get response: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return ChatResponse{}, &StreamError{
				Kind: StreamProviderError, HTTPStatus: resp.StatusCode,
				Err: fmt.Errorf("api returned status %d and response body could not be read: %w", resp.StatusCode, readErr),
			}
		}
		return ChatResponse{}, &StreamError{
			Kind: StreamProviderError, HTTPStatus: resp.StatusCode,
			Err: fmt.Errorf("api returned status %d: %s", resp.StatusCode, bodyBytes),
		}
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
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, streamError(StreamProviderError, fmt.Errorf("failed to read stream: %w", err))
	}
	if !completed {
		return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New("stream ended before [DONE]"))
	}
	if !gotChunk {
		return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New("stream contained no chunks"))
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
	jsonBody, err := json.Marshal(r)
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("api returned status %d: %s", resp.StatusCode, bodyBytes)
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
		return ChatResponse{}, fmt.Errorf("response contained no choices: %s", bodyBytes)
	}

	return chatResp, nil
}
