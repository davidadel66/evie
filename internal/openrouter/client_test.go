package openrouter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// serveFixture builds a local SSE server replaying one captured stream,
// and a Client pointed at it.
func serveFixture(t *testing.T, path string) *Client {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = srv.URL
	return c
}

func TestChatStreamAssemblesReasoning(t *testing.T) {
	c := serveFixture(t, "testdata/kimi-reasoning-stream.txt")

	var contentFrags, reasoningFrags []string
	res, err := c.ChatStream(ChatRequest{Model: "test"}, StreamHandlers{
		OnContent:   func(s string) { contentFrags = append(contentFrags, s) },
		OnReasoning: func(s string) { reasoningFrags = append(reasoningFrags, s) },
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	msg := res.Choices[0].Message

	const wantReasoning = "Compute 17*23. 17*20 = 340, 17*3 = 51, total 391."
	const wantContent = "17 × 23:\n\n- 17 × 20 = 340\n- 17 × 3 = 51\n- 340 + 51 = **391**"

	if msg.Reasoning != wantReasoning {
		t.Errorf("assembled reasoning = %q, want %q", msg.Reasoning, wantReasoning)
	}
	if msg.Content != wantContent {
		t.Errorf("assembled content = %q, want %q", msg.Content, wantContent)
	}
	// Callbacks see every fragment, in order — joining them reproduces
	// the assembled fields exactly.
	if got := strings.Join(reasoningFrags, ""); got != msg.Reasoning {
		t.Errorf("joined OnReasoning fragments = %q, want %q", got, msg.Reasoning)
	}
	if got := strings.Join(contentFrags, ""); got != msg.Content {
		t.Errorf("joined OnContent fragments = %q, want %q", got, msg.Content)
	}

	// ReasoningDetails round-trips as one array of every part sent.
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.ReasoningDetails, &parts); err != nil {
		t.Fatalf("ReasoningDetails not a JSON array: %v", err)
	}
	if len(parts) != 26 {
		t.Errorf("ReasoningDetails has %d parts, want 26", len(parts))
	}
	if parts[0].Type != "reasoning.text" || parts[0].Text != "Compute" {
		t.Errorf("first part = %+v, want {reasoning.text Compute}", parts[0])
	}

	if got := res.Choices[0].FinishReason; got != "stop" {
		t.Errorf("finish reason = %q, want stop", got)
	}
}

func TestChatStreamWithoutReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(": keepalive\n" +
			"\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n" +
			"\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" there\"},\"finish_reason\":\"stop\"}]}\n" +
			"\n" +
			"data: [DONE]\n"))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.baseURL = srv.URL

	reasoningCalled := false
	res, err := c.ChatStream(ChatRequest{Model: "test"}, StreamHandlers{
		OnReasoning: func(string) { reasoningCalled = true },
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	msg := res.Choices[0].Message

	if msg.Content != "Hello there" {
		t.Errorf("content = %q, want %q", msg.Content, "Hello there")
	}
	// A stream with no reasoning must behave exactly as before the
	// feature: empty fields, key omitted, callback never fired.
	if msg.Reasoning != "" {
		t.Errorf("reasoning = %q, want empty", msg.Reasoning)
	}
	if msg.ReasoningDetails != nil {
		t.Errorf("ReasoningDetails = %s, want nil", msg.ReasoningDetails)
	}
	if reasoningCalled {
		t.Error("OnReasoning fired on a stream with no reasoning")
	}
}
