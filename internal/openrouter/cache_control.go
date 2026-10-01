package openrouter

import (
	"encoding/json"
	"strings"
)

// UsesExplicitCacheBreakpoints reports whether OpenRouter caches a model's
// prompt only at cache_control breakpoints placed in message content.
// Anthropic and Alibaba Qwen require explicit breakpoints; OpenAI, DeepSeek,
// Moonshot, Grok, and Gemini 2.5+ cache prefixes automatically. A leading "~"
// is OpenRouter's latest-model alias.
func UsesExplicitCacheBreakpoints(model string) bool {
	model = strings.TrimPrefix(model, "~")
	return strings.HasPrefix(model, "anthropic/") || strings.HasPrefix(model, "qwen/")
}

// cacheTextPart is one OpenAI-compatible text content part. OpenRouter carries
// its cache_control marker to providers that use explicit breakpoints.
type cacheTextPart struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control"`
}

// wireMessage mirrors Message's JSON fields and order with content widened to
// a string or a marked text-part array.
type wireMessage struct {
	Role             string          `json:"role"`
	Reasoning        string          `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
	Content          any             `json:"content,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

// cacheMarked reports whether a message is encoded with a breakpoint. Blank
// text cannot carry one, because providers reject empty text blocks.
func cacheMarked(message Message) bool {
	return message.CacheControl != nil && strings.TrimSpace(message.Content) != ""
}

// encodeChatRequest is the one Chat Completions encoding shared by accounting
// and dispatch. A request without breakpoints is the ordinary JSON encoding.
func encodeChatRequest(r ChatRequest) ([]byte, error) {
	marked := false
	for _, message := range r.Messages {
		marked = marked || cacheMarked(message)
	}
	if !marked {
		return json.Marshal(r)
	}
	messages := make([]wireMessage, len(r.Messages))
	for i, message := range r.Messages {
		wire := wireMessage{
			Role: message.Role, Reasoning: message.Reasoning, ReasoningDetails: message.ReasoningDetails,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		}
		switch {
		case cacheMarked(message):
			wire.Content = []cacheTextPart{{Type: "text", Text: message.Content, CacheControl: message.CacheControl}}
		case message.Content != "":
			wire.Content = message.Content
		}
		messages[i] = wire
	}
	// The outer Messages field shadows the embedded request's, so every other
	// request field keeps its ordinary encoding.
	return json.Marshal(struct {
		ChatRequest
		Messages []wireMessage `json:"messages"`
	}{r, messages})
}
