// Package openrouter is a minimal client for OpenRouter's OpenAI-compatible
// chat-completions API. It owns the wire format: every type in schema.go
// mirrors the JSON OpenRouter sends or expects, field for field. Nothing in
// here knows about the agent harness — the harness imports this package,
// never the reverse. If a second provider is ever added, this package is
// the template: a sibling package translating the same ideas to different
// wire types.
package openrouter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
