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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// NewClient is the only way to build a Client: it rejects an empty API key
// up front so a misconfigured environment fails at startup with a clear
// message instead of failing weirdly at the first request.
func NewClient(key string) (*Client, error) {
	if key == "" {
		return nil, errors.New("API key is empty")
	}

	return &Client{apiKey: key}, nil
}

// ChatStream sends one chat-completions request with streaming enabled,
// invoking onDelta with each content fragment as it arrives (print it
// for live output), and returns the fully assembled ChatResponse — the
// same shape Chat returns, so callers' downstream logic (history
// append, tool-call dispatch) is identical for both methods. Tool-call
// fragments are reassembled by index; OpenRouter's ": ..." keepalive
// comment lines are skipped per the SSE spec; the stream ends at the
// "[DONE]" sentinel.
func (c *Client) ChatStream(r ChatRequest, onDelta func(string)) (ChatResponse, error) {
	r.Stream = true
	jsonBody, err := json.Marshal(r)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to marshal json: %w", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to get response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return ChatResponse{}, fmt.Errorf("api returned status %d: %s", resp.StatusCode, bodyBytes)
	}

	var (
		msg          Message
		finishReason string
		gotChunk     bool
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
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return ChatResponse{}, fmt.Errorf("failed to parse stream chunk: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		gotChunk = true
		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			msg.Content += choice.Delta.Content
			if onDelta != nil {
				onDelta(choice.Delta.Content)
			}
		}
		for _, tcd := range choice.Delta.ToolCalls {
			for len(msg.ToolCalls) <= tcd.Index {
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{})
			}
			tc := &msg.ToolCalls[tcd.Index]
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
		return ChatResponse{}, fmt.Errorf("failed to read stream: %w", err)
	}
	if !gotChunk {
		return ChatResponse{}, errors.New("stream contained no chunks")
	}

	return ChatResponse{Choices: []Choice{{Message: msg, FinishReason: finishReason}}}, nil
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

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to send request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
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

	var chatResp ChatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("response contained no choices: %s", bodyBytes)
	}

	return chatResp, nil
}
