package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestUsesExplicitCacheBreakpointsOnlyForProvidersThatNeedThem(t *testing.T) {
	for model, want := range map[string]bool{
		"anthropic/claude-sonnet-4.6":     true,
		"~anthropic/claude-sonnet-latest": true,
		"qwen/qwen3-max":                  true,
		"~qwen/qwen3-coder-plus":          true,
		"deepseek/deepseek-v4.1-flash":    false,
		"moonshotai/kimi-k3":              false,
		"google/gemini-3-pro":             false,
		AstraModel:                        false,
		"anthropicish/model":              false,
	} {
		if got := UsesExplicitCacheBreakpoints(model); got != want {
			t.Errorf("UsesExplicitCacheBreakpoints(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestRequestBytesEncodeMarkedMessagesAsCacheControlledTextParts(t *testing.T) {
	ephemeral := &CacheControl{Type: "ephemeral"}
	request := ChatRequest{
		Model: "anthropic/claude-sonnet-4.6",
		Messages: []Message{
			{Role: "system", Content: "rules", CacheControl: ephemeral},
			{Role: "user", Content: "hello"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "echo", Arguments: "{}"}}}},
			{Role: "tool", ToolCallID: "call-1", Content: "result <b>", CacheControl: ephemeral},
		},
		Stream: true,
	}
	encoded, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
		Stream   bool              `json:"stream"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Model != request.Model || !decoded.Stream || len(decoded.Messages) != 4 {
		t.Fatalf("decoded request = %s", encoded)
	}
	if got, want := string(decoded.Messages[0]),
		`{"role":"system","content":[{"type":"text","text":"rules","cache_control":{"type":"ephemeral"}}]}`; got != want {
		t.Fatalf("marked system message = %s, want %s", got, want)
	}
	for _, index := range []int{1, 2} {
		plain, err := json.Marshal(request.Messages[index])
		if err != nil {
			t.Fatal(err)
		}
		if string(decoded.Messages[index]) != string(plain) {
			t.Fatalf("unmarked message %d = %s, want ordinary encoding %s", index, decoded.Messages[index], plain)
		}
	}
	if got, want := string(decoded.Messages[3]),
		"{\"role\":\"tool\",\"content\":[{\"type\":\"text\",\"text\":\"result \\u003cb\\u003e\",\"cache_control\":{\"type\":\"ephemeral\"}}],\"tool_call_id\":\"call-1\"}"; got != want {
		t.Fatalf("marked tool message = %s, want %s", got, want)
	}
}

func TestRequestBytesWithoutCacheMarkersAreTheOrdinaryEncoding(t *testing.T) {
	request := ChatRequest{
		Model:    "deepseek/deepseek-v4.1-flash",
		Messages: []Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "hello"}},
		Tools: []Tool{{Type: "function", Function: Function{Name: "lookup", Parameters: Parameter{
			Type: "object", Properties: map[string]Property{"q": {Type: "string"}},
		}}}},
		ToolChoice: "none", Stream: true, Reasoning: &ReasoningConfig{Effort: "low"}, MaxTokens: 512,
	}
	encoded, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(plain) {
		t.Fatalf("unmarked request bytes changed:\n%s\n%s", encoded, plain)
	}
}

func TestCacheMarkersKeepEveryRequestField(t *testing.T) {
	temperature := 0.0
	request := ChatRequest{
		Model:      "anthropic/claude-sonnet-4.6",
		Messages:   []Message{{Role: "system", Content: "rules", CacheControl: &CacheControl{Type: "ephemeral"}}},
		Tools:      []Tool{{Type: "function", Function: Function{Name: "lookup", Parameters: Parameter{Type: "object"}}}},
		ToolChoice: "none", Stream: true, Reasoning: &ReasoningConfig{Effort: "low"}, Temperature: &temperature, MaxTokens: 512,
	}
	marked, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var markedFields, plainFields map[string]json.RawMessage
	if err := json.Unmarshal(marked, &markedFields); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(plain, &plainFields); err != nil {
		t.Fatal(err)
	}
	delete(markedFields, "messages")
	delete(plainFields, "messages")
	if !reflect.DeepEqual(markedFields, plainFields) {
		t.Fatalf("marked request fields = %v, want %v", markedFields, plainFields)
	}
}

func TestCacheMarkerIsNotAddedToBlankContent(t *testing.T) {
	request := ChatRequest{Model: "anthropic/claude-sonnet-4.6", Messages: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Type: "function", Function: FunctionCall{Name: "echo", Arguments: "{}"}}},
			CacheControl: &CacheControl{Type: "ephemeral"}},
	}}
	encoded, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "cache_control") || strings.Contains(string(encoded), `"content"`) {
		t.Fatalf("blank content gained a cache marker: %s", encoded)
	}
}

func TestChatStreamSendsTheAccountedCacheMarkedBytes(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	request := ChatRequest{Model: "anthropic/claude-sonnet-4.6", Stream: true, Messages: []Message{
		{Role: "system", Content: "rules", CacheControl: &CacheControl{Type: "ephemeral"}},
		{Role: "user", Content: "hello", CacheControl: &CacheControl{Type: "ephemeral"}},
	}}
	if _, err := client.ChatStream(context.Background(), request, StreamHandlers{}); err != nil {
		t.Fatal(err)
	}
	accounted, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(accounted) || !strings.Contains(string(body), `"cache_control":{"type":"ephemeral"}`) {
		t.Fatalf("sent body = %s, want accounted bytes %s", body, accounted)
	}
}

func TestWireMessageMirrorsMessageJSONFields(t *testing.T) {
	tags := func(value any) []string {
		var fields []string
		typ := reflect.TypeOf(value)
		for i := 0; i < typ.NumField(); i++ {
			tag := typ.Field(i).Tag.Get("json")
			if tag != "" && tag != "-" {
				fields = append(fields, tag)
			}
		}
		return fields
	}
	if got, want := tags(wireMessage{}), tags(Message{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("wire message JSON fields = %v, want Message fields %v", got, want)
	}
}
