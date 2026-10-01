package openrouter

import (
	"encoding/json"
	"strings"
	"testing"
)

// The step-limit final call keeps tool schemas, because providers that replay
// tool-call history require them, and forbids new calls with tool_choice.
func TestToolChoiceNoneKeepsSchemasOnBothWireFormats(t *testing.T) {
	tool := Tool{Type: "function", Function: Function{Name: "echo", Parameters: Parameter{Type: "object"}}}
	chat, err := json.Marshal(ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}, Tools: []Tool{tool}, ToolChoice: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), `"tool_choice":"none"`) || !strings.Contains(string(chat), `"tools":[`) {
		t.Fatalf("chat request = %s", chat)
	}
	unset, err := json.Marshal(ChatRequest{Model: "m", Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unset), "tool_choice") {
		t.Fatalf("unset tool_choice must be omitted: %s", unset)
	}
	encoded, err := encodeResponsesRequest(ChatRequest{Model: AstraModel, Messages: []Message{{Role: "user", Content: "hi"}}, Tools: []Tool{tool}, ToolChoice: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded.body), `"tool_choice":"none"`) || !strings.Contains(string(encoded.body), `"tools":[`) {
		t.Fatalf("responses request = %s", encoded.body)
	}
}
