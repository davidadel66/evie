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
	"unicode/utf8"
)

const maxResponsesBody = 16 << 20

type responseEnvelope struct {
	Status            string            `json:"status"`
	Output            []json.RawMessage `json:"output"`
	Error             json.RawMessage   `json:"error"`
	IncompleteDetails json.RawMessage   `json:"incomplete_details"`
}

type responseItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Role      string `json:"role"`
	Phase     string `json:"phase"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
	} `json:"content"`
}

func (c *Client) responses(ctx context.Context, r ChatRequest, h StreamHandlers) (ChatResponse, error) {
	body, err := RequestBytes(r)
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.apiBaseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if r.Stream && h.OnReasoning != nil {
		// Astra always reasons, but OpenRouter may announce its reasoning
		// item only after that work. Time the visible wait from dispatch.
		h.OnReasoning("")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResponse{}, streamError(StreamProviderError, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Provider error bodies are untrusted and may echo credentials/content.
		return ChatResponse{}, &StreamError{Kind: StreamProviderError, HTTPStatus: resp.StatusCode, Err: fmt.Errorf("Responses API returned status %d", resp.StatusCode)}
	}
	if !r.Stream {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponsesBody+1))
		if err != nil {
			return ChatResponse{}, streamError(StreamProviderError, err)
		}
		if len(data) > maxResponsesBody {
			return invalidResponse("response exceeds byte limit")
		}
		return completedResponse(data)
	}
	reader := &io.LimitedReader{R: resp.Body, N: maxResponsesBody + 1}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxResponsesBody)
	arguments := map[string]string{}
	announced := map[string]responseItem{}
	var usage *TokenUsage
	for scanner.Scan() {
		line := scanner.Text()
		if reader.N <= 0 {
			return invalidResponse("response stream exceeds byte limit")
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return invalidResponse("stream ended without a completed response")
		}
		if !utf8.ValidString(data) {
			return invalidResponse("stream contains invalid UTF-8")
		}
		var event struct {
			Type     string          `json:"type"`
			Delta    string          `json:"delta"`
			ItemID   string          `json:"item_id"`
			Item     responseItem    `json:"item"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return invalidResponse("malformed stream event")
		}
		if len(event.Response) > 0 && !isJSONNull(event.Response) {
			observed, present, err := parseResponsesUsage(event.Response)
			if err != nil {
				return invalidResponse("malformed response usage")
			}
			if present {
				usage = observed
			}
		}
		switch event.Type {
		case "response.output_text.delta", "response.content_part.delta":
			if h.OnContent != nil {
				h.OnContent(event.Delta)
			}
		case "response.reasoning_summary_text.delta":
			if h.OnReasoning != nil {
				h.OnReasoning(event.Delta)
			}
		case "response.function_call_arguments.delta":
			if event.ItemID == "" {
				return invalidResponse("function arguments lack item identity")
			}
			arguments[event.ItemID] += event.Delta
		case "response.output_item.added":
			if event.Item.Type == "function_call" {
				if event.Item.ID == "" {
					return invalidResponse("function item lacks identity")
				}
				if _, exists := announced[event.Item.ID]; exists {
					return invalidResponse("duplicate function item")
				}
				announced[event.Item.ID] = event.Item
			}
		case "response.completed", "response.done":
			result, err := completedResponse(event.Response)
			if err != nil {
				return ChatResponse{}, err
			}
			seen := map[string]bool{}
			for _, raw := range result.Choices[0].Message.ResponseItems {
				var item responseItem
				if err := json.Unmarshal(raw, &item); err != nil {
					return invalidResponse("malformed completed item")
				}
				if item.Type != "function_call" {
					continue
				}
				seen[item.ID] = true
				if fragments, ok := arguments[item.ID]; ok && fragments != item.Arguments {
					return invalidResponse("completed arguments disagree with stream")
				}
				if prior, ok := announced[item.ID]; ok && ((prior.CallID != "" && prior.CallID != item.CallID) || (prior.Name != "" && prior.Name != item.Name)) {
					return invalidResponse("completed tool identity disagrees with stream")
				}
			}
			for id := range arguments {
				if !seen[id] {
					return invalidResponse("completed response dropped streamed function")
				}
			}
			for id := range announced {
				if !seen[id] {
					return invalidResponse("completed response dropped announced function")
				}
			}
			if err := ctx.Err(); err != nil {
				return ChatResponse{}, streamError(StreamProviderError, err)
			}
			result.Usage = usage
			return result, nil
		case "response.failed", "error":
			return ChatResponse{}, streamError(StreamProviderError, errors.New("Responses provider failed"))
		case "response.incomplete":
			return invalidResponse("Responses generation is incomplete")
		case "":
			return invalidResponse("stream event lacks a type")
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, streamError(StreamProviderError, err)
	}
	return invalidResponse("stream ended before response completion")
}

func invalidResponse(message string) (ChatResponse, error) {
	return ChatResponse{}, streamError(StreamProviderResponseInvalid, errors.New(message))
}

func completedResponse(data []byte) (ChatResponse, error) {
	if !utf8.Valid(data) {
		return invalidResponse("response contains invalid UTF-8")
	}
	var response responseEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		return invalidResponse("malformed completed response")
	}
	if response.Status != "completed" || len(response.Output) == 0 ||
		(len(response.Error) > 0 && !isJSONNull(response.Error)) ||
		(len(response.IncompleteDetails) > 0 && !isJSONNull(response.IncompleteDetails)) {
		return invalidResponse("response did not complete successfully")
	}
	msg := Message{Role: "assistant"}
	ids, calls := map[string]bool{}, map[string]bool{}
	for _, raw := range response.Output {
		var item responseItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return invalidResponse("malformed output item")
		}
		if item.ID == "" || ids[item.ID] {
			return invalidResponse("missing or duplicate output item identity")
		}
		ids[item.ID] = true
		if item.Status != "" && item.Status != "completed" {
			return invalidResponse("incomplete output item")
		}
		switch item.Type {
		case "reasoning":
			// Retain opaque reasoning only in the per-turn transport overlay.
		case "message":
			if item.Role != "assistant" || (item.Phase != "" && item.Phase != "commentary" && item.Phase != "final_answer") {
				return invalidResponse("invalid assistant message role or phase")
			}
			part := TextPart{Phase: item.Phase, AfterToolCalls: len(msg.ToolCalls)}
			for _, content := range item.Content {
				switch content.Type {
				case "output_text":
					part.Text += content.Text
				case "refusal":
					part.Text += content.Refusal
				default:
					return invalidResponse("unsupported assistant content type")
				}
			}
			msg.TextParts = append(msg.TextParts, part)
			msg.Content += part.Text
		case "function_call":
			if item.CallID == "" || calls[item.CallID] || item.Name == "" || !json.Valid([]byte(item.Arguments)) {
				return invalidResponse("invalid function call")
			}
			calls[item.CallID] = true
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{ID: item.CallID, Type: "function", Function: FunctionCall{Name: item.Name, Arguments: item.Arguments}})
		default:
			return invalidResponse("unsupported output item type")
		}
		msg.ResponseItems = append(msg.ResponseItems, bytes.Clone(raw))
	}
	if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
		return invalidResponse("response has no assistant content or tools")
	}
	usage, _, err := parseResponsesUsage(data)
	if err != nil {
		return invalidResponse("malformed response usage")
	}
	finish := "stop"
	if len(msg.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	return ChatResponse{Choices: []Choice{{Message: msg, FinishReason: finish}}, Usage: usage}, nil
}
