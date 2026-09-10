package openrouter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const AstraModel = "openai/gpt-6-astra"

// UsesResponses deliberately selects only the verified model. Custom models
// retain the existing Chat protocol rather than guessing their capabilities.
func UsesResponses(model string) bool { return model == AstraModel }

type responseEncoding struct {
	body   []byte
	stream bool
	parts  RequestPartSizes
}

// RequestPartSizes measures protocol-encoded message groups and settings.
// Message groups may contain several Responses items. Envelope punctuation is
// included in the full request byte count rather than these component sizes.
type RequestPartSizes struct {
	Messages []int64
	Tools    int64
	Settings int64
}

// PrepareRequest freezes the Responses wire body before accounting/admission.
// The prepared body is authoritative; callers must prepare a new request when
// changing inputs. Legacy Chat requests retain their existing representation.
func PrepareRequest(r ChatRequest) (ChatRequest, error) {
	if !UsesResponses(r.Model) {
		return r, nil
	}
	encoded, err := encodeResponsesRequest(r)
	if err != nil {
		return ChatRequest{}, err
	}
	r.prepared = encoded
	return r, nil
}

// RequestBytes is shared by accounting and HTTP dispatch. Return a copy so a
// consumer cannot mutate a prepared body after its snapshot has been committed.
func RequestBytes(r ChatRequest) ([]byte, error) {
	if r.prepared != nil {
		return bytes.Clone(r.prepared.body), nil
	}
	if !UsesResponses(r.Model) {
		return json.Marshal(r)
	}
	encoded, err := encodeResponsesRequest(r)
	if err != nil {
		return nil, err
	}
	return encoded.body, nil
}

func ResponseRequestPartSizes(r ChatRequest) (RequestPartSizes, error) {
	encoded := r.prepared
	if encoded == nil {
		var err error
		encoded, err = encodeResponsesRequest(r)
		if err != nil {
			return RequestPartSizes{}, err
		}
	}
	parts := encoded.parts
	parts.Messages = append([]int64(nil), parts.Messages...)
	return parts, nil
}

type responsesRequest struct {
	Model           string             `json:"model"`
	Input           []json.RawMessage  `json:"input"`
	Tools           []responseFunction `json:"tools,omitempty"`
	Stream          bool               `json:"stream"`
	Store           bool               `json:"store"`
	Reasoning       responseReasoning  `json:"reasoning"`
	MaxOutputTokens int64              `json:"max_output_tokens,omitempty"`
	Include         []string           `json:"include"`
	Provider        responseProvider   `json:"provider"`
}

type responseProvider struct {
	RequireParameters bool `json:"require_parameters"`
}

type responseReasoning struct {
	Effort  string `json:"effort"`
	Summary string `json:"summary,omitempty"`
}
type responseFunction struct {
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Parameters  Parameter `json:"parameters"`
	Strict      bool      `json:"strict"`
}

func encodeResponsesRequest(r ChatRequest) (*responseEncoding, error) {
	if r.Temperature != nil {
		return nil, errors.New("Astra does not support temperature")
	}
	effort := "low"
	if r.Reasoning != nil && r.Reasoning.Effort != "" {
		effort = r.Reasoning.Effort
	}
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return nil, errors.New("unsupported Astra reasoning effort")
	}
	request := responsesRequest{Model: r.Model, Input: []json.RawMessage{}, Stream: r.Stream,
		Reasoning: responseReasoning{Effort: effort}, MaxOutputTokens: r.MaxTokens,
		Include: []string{"reasoning.encrypted_content"}, Provider: responseProvider{RequireParameters: true}}
	if r.Reasoning != nil {
		request.Reasoning.Summary = r.Reasoning.Summary
	}
	parts := RequestPartSizes{}
	for i, msg := range r.Messages {
		items, err := responseInputItems(msg, i)
		if err != nil {
			return nil, err
		}
		group, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		parts.Messages = append(parts.Messages, int64(len(group)))
		request.Input = append(request.Input, items...)
	}
	for _, tool := range r.Tools {
		if tool.Type != "function" {
			return nil, errors.New("Responses supports only Evie's function tools")
		}
		request.Tools = append(request.Tools, responseFunction{Type: "function", Name: tool.Function.Name,
			Description: tool.Function.Description, Parameters: tool.Function.Parameters, Strict: false})
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(request.Tools) > 0 {
		tools, err := json.Marshal(request.Tools)
		if err != nil {
			return nil, err
		}
		parts.Tools = int64(len(tools))
	}
	// Settings use the same encoder with no input or tool items.
	request.Input, request.Tools = nil, nil
	settings, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	parts.Settings = int64(len(settings))
	return &responseEncoding{body: encoded, stream: r.Stream, parts: parts}, nil
}

func responseInputItems(msg Message, position int) ([]json.RawMessage, error) {
	if len(msg.ResponseItems) > 0 {
		if msg.Role != "assistant" {
			return nil, errors.New("continuation items require an assistant message")
		}
		return msg.ResponseItems, nil
	}
	var items []json.RawMessage
	appendItem := func(item any) error {
		encoded, err := json.Marshal(item)
		if err == nil {
			items = append(items, encoded)
		}
		return err
	}
	switch msg.Role {
	case "system", "developer", "user":
		if err := appendItem(map[string]any{"type": "message", "role": msg.Role, "content": msg.Content}); err != nil {
			return nil, err
		}
	case "assistant":
		parts := msg.TextParts
		if len(parts) == 0 && msg.Content != "" {
			parts = []TextPart{{Text: msg.Content}}
		}
		var text strings.Builder
		previous := 0
		for _, part := range parts {
			if !utf8.ValidString(part.Text) || (part.Phase != "" && part.Phase != "commentary" && part.Phase != "final_answer") || part.AfterToolCalls < previous || part.AfterToolCalls > len(msg.ToolCalls) {
				return nil, errors.New("invalid assistant text part")
			}
			previous = part.AfterToolCalls
			text.WriteString(part.Text)
		}
		if text.String() != msg.Content {
			return nil, errors.New("assistant text parts differ from content")
		}
		partIndex := 0
		for callIndex := 0; callIndex <= len(msg.ToolCalls); callIndex++ {
			for partIndex < len(parts) && parts[partIndex].AfterToolCalls == callIndex {
				part := parts[partIndex]
				item := map[string]any{"type": "message", "role": "assistant", "status": "completed",
					"id":      responseHistoryID("msg", position, partIndex, part.Text),
					"content": []map[string]any{{"type": "output_text", "text": part.Text, "annotations": []any{}}}}
				if part.Phase != "" {
					item["phase"] = part.Phase
				}
				if err := appendItem(item); err != nil {
					return nil, err
				}
				partIndex++
			}
			if callIndex < len(msg.ToolCalls) {
				call := msg.ToolCalls[callIndex]
				if err := appendItem(map[string]any{"type": "function_call", "id": responseHistoryID("fc", position, callIndex, call.ID),
					"call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments, "status": "completed"}); err != nil {
					return nil, err
				}
			}
		}
	case "tool":
		if msg.ToolCallID == "" {
			return nil, errors.New("tool result is missing call identity")
		}
		if err := appendItem(map[string]any{"type": "function_call_output", "call_id": msg.ToolCallID, "output": msg.Content}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported message role %q", msg.Role)
	}
	return items, nil
}

// Legacy events have no provider item IDs. Give their projected items stable,
// request-local identities; tool pairing always uses the original call_id.
func responseHistoryID(prefix string, message, part int, content string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", message, part, content)))
	return prefix + "_" + hex.EncodeToString(digest[:16])
}
