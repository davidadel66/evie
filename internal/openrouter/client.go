// Package openrouter provides the client used for Chat with OpenRouter
package openrouter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func NewClient(key string) (*Client, error) {
	if key == "" {
		return nil, errors.New("API key is empty")
	}

	return &Client{apiKey: key}, nil
}

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
