package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAstraResponsesToolRoundTrip(t *testing.T) {
	var requests []map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request route/auth")
		}
		body, _ := io.ReadAll(r.Body)
		var request map[string]json.RawMessage
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\\\"n\\\":\"}\n\n")
			io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"2}\"}\n\n")
			io.WriteString(w, `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-canary","summary":[]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"double","arguments":"{\"n\":2}","status":"completed"}],"usage":{"input_tokens":20,"output_tokens":10,"total_tokens":30,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":5}}}}`+"\n\n")
		} else {
			io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"4"}`+"\n\n")
			io.WriteString(w, `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"4"}]}]}}`+"\n\n")
		}
	}))
	defer server.Close()
	client, _ := NewClient("test-key")
	client.apiBaseURL = server.URL
	client.baseURL = server.URL + "/chat/completions"
	request := ChatRequest{Model: "openai/gpt-6-astra", MaxTokens: 1024, Messages: []Message{{Role: "user", Content: "Double 2"}}, Tools: []Tool{{Type: "function", Function: Function{Name: "double", Parameters: Parameter{Type: "object", Properties: map[string]Property{"n": {Type: "integer"}}, Required: []string{"n"}}}}}}
	first, err := client.ChatStream(context.Background(), request, StreamHandlers{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Choices[0].Message.ToolCalls) != 1 || first.Choices[0].Message.ToolCalls[0].ID != "call_1" {
		t.Fatalf("missing call identity: %+v", first)
	}
	if first.Usage == nil || first.Usage.InputTokens == nil || *first.Usage.InputTokens != 20 || first.Usage.CachedInputTokens == nil || *first.Usage.CachedInputTokens != 0 {
		t.Fatalf("usage=%+v", first.Usage)
	}
	request.Messages = append(request.Messages, first.Choices[0].Message, Message{Role: "tool", ToolCallID: "call_1", Content: "4"})
	var streamed string
	final, err := client.ChatStream(context.Background(), request, StreamHandlers{OnContent: func(s string) { streamed += s }})
	if err != nil {
		t.Fatal(err)
	}
	if final.Choices[0].Message.Content != "4" || streamed != "4" {
		t.Fatalf("final=%+v streamed=%q", final, streamed)
	}
	for _, req := range requests {
		for _, forbidden := range []string{"messages", "temperature", "max_tokens", "previous_response_id"} {
			if _, ok := req[forbidden]; ok {
				t.Errorf("unsupported parameter %s", forbidden)
			}
		}
		if string(req["store"]) != "false" || string(req["reasoning"]) != `{"effort":"low"}` {
			t.Errorf("settings=%s", mustJSON(t, req))
		}
	}
	secondInput := string(requests[1]["input"])
	for _, want := range []string{"opaque-canary", `"call_id":"call_1"`, `"type":"function_call_output"`} {
		if !strings.Contains(secondInput, want) {
			t.Errorf("input missing %s: %s", want, secondInput)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestResponsesReasoningActivityWithoutPublicSummary(t *testing.T) {
	for _, publicSummary := range []bool{false, true} {
		t.Run(map[bool]string{false: "no summary", true: "public summary"}[publicSummary], func(t *testing.T) {
			var activityStarted atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// OpenRouter may announce the reasoning item only when it has
				// finished reasoning. The visible wait must start at dispatch.
				if !activityStarted.Load() {
					t.Error("thinking indicator did not start before waiting for the provider")
				}
				io.WriteString(w, `data: {"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_1","summary":[]}}`+"\n\n")
				io.WriteString(w, `data: {"type":"response.reasoning_text.delta","delta":"private-canary"}`+"\n\n")
				if publicSummary {
					io.WriteString(w, `data: {"type":"response.reasoning_summary_text.delta","delta":"Checking the arithmetic."}`+"\n\n")
				}
				io.WriteString(w, `data: {"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque-canary"}}`+"\n\n")
				io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"42"}`+"\n\n")
				io.WriteString(w, `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"opaque-canary"},{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"42"}]}]}}`+"\n\n")
			}))
			defer server.Close()
			client, _ := NewClient("test-key")
			client.apiBaseURL = server.URL
			var events []string
			result, err := client.ChatStream(context.Background(), ChatRequest{Model: AstraModel}, StreamHandlers{
				OnReasoning: func(text string) {
					activityStarted.Store(true)
					events = append(events, "reasoning:"+text)
				},
				OnContent: func(text string) { events = append(events, "content:"+text) },
			})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"reasoning:"}
			if publicSummary {
				want = append(want, "reasoning:Checking the arithmetic.")
			}
			want = append(want, "content:42")
			if mustJSON(t, events) != mustJSON(t, want) {
				t.Fatalf("display events=%q, want %q", events, want)
			}
			if result.Choices[0].Message.Reasoning != "" {
				t.Fatal("reasoning leaked into the durable message projection")
			}
		})
	}
}

func TestResponsesUsageUsesLastNonNullObservation(t *testing.T) {
	for _, test := range []struct {
		name, final string
		known       bool
	}{{"null retains prior", "null", true}, {"empty replaces prior", "{}", false}, {"invalid replaces prior", `{"input_tokens":-1}`, false}} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `data: {"type":"response.in_progress","response":{"usage":{"input_tokens":12,"output_tokens":2,"total_tokens":14}}}`+"\n\n")
				io.WriteString(w, `data: {"type":"response.completed","response":{"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":`+test.final+"}}\n\n")
			}))
			defer server.Close()
			client, _ := NewClient("test-key")
			client.apiBaseURL = server.URL
			result, err := client.ChatStream(context.Background(), ChatRequest{Model: AstraModel}, StreamHandlers{})
			if err != nil {
				t.Fatal(err)
			}
			known := result.Usage != nil && result.Usage.InputTokens != nil
			if known != test.known || (known && *result.Usage.InputTokens != 12) {
				t.Fatalf("usage=%+v want known=%v", result.Usage, test.known)
			}
		})
	}
}

func TestResponsesPreservesPublicOrderAndOptionalNestedSchemas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if len(request.Tools) != 1 || request.Tools[0].Strict || len(request.Tools[0].Parameters.Required) != 0 ||
			len(request.Tools[0].Parameters.Properties["options"].Required) != 0 || request.Tools[0].Parameters.Properties["options"].Properties["limit"].Type != "integer" {
			t.Errorf("optional schema changed: %+v", request.Tools)
		}
		io.WriteString(w, `{"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"Before."}]},{"type":"function_call","id":"fc_a","call_id":"call-a","name":"inspect","arguments":"{}"},{"type":"message","id":"msg_2","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"Between."}]},{"type":"function_call","id":"fc_b","call_id":"call-b","name":"inspect","arguments":"{}"}]}`)
	}))
	defer server.Close()
	client, _ := NewClient("test-key")
	client.apiBaseURL = server.URL
	response, err := client.Chat(ChatRequest{Model: AstraModel, Tools: []Tool{{Type: "function", Function: Function{Name: "inspect", Parameters: Parameter{Type: "object", Properties: map[string]Property{
		"options": {Type: "object", Properties: map[string]Property{"limit": {Type: "integer"}}},
	}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	message := response.Choices[0].Message
	if message.Content != "Before.Between." || len(message.TextParts) != 2 || message.TextParts[0].Phase != "commentary" || message.TextParts[1].Phase != "final_answer" || message.TextParts[1].AfterToolCalls != 1 {
		t.Fatalf("public projection=%+v", message)
	}
	message.ResponseItems = nil
	items, err := responseInputItems(message, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sequence []string
	for _, raw := range items {
		var item responseItem
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		sequence = append(sequence, item.Phase+item.CallID)
	}
	if strings.Join(sequence, ",") != "commentary,call-a,final_answer,call-b" {
		t.Fatalf("replay order=%v", sequence)
	}
}

func TestResponsesBoundsBodiesAndSanitizesHTTPErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		stream bool
		status int
	}{
		{"nonstream limit", false, http.StatusOK}, {"stream limit", true, http.StatusOK}, {"HTTP failure", true, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				if test.status != http.StatusOK {
					io.WriteString(w, "secret-provider-body")
					return
				}
				io.WriteString(w, strings.Repeat("x", maxResponsesBody+1))
			}))
			defer server.Close()
			client, _ := NewClient("test-key")
			client.apiBaseURL = server.URL
			var err error
			if test.stream {
				_, err = client.ChatStream(context.Background(), ChatRequest{Model: AstraModel}, StreamHandlers{})
			} else {
				_, err = client.Chat(ChatRequest{Model: AstraModel})
			}
			if err == nil || strings.Contains(err.Error(), "secret-provider-body") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestResponsesRejectsUnfinishedOrInconsistentStreams(t *testing.T) {
	valid := `{"status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"double","arguments":"{}"}]}`
	tests := map[string]string{
		"EOF after text":        `data: {"type":"response.output_text.delta","delta":"partial"}` + "\n\n",
		"sentinel alone":        "data: [DONE]\n\n",
		"incomplete":            `data: {"type":"response.incomplete","response":{"status":"incomplete"}}` + "\n\n",
		"failed":                `data: {"type":"response.failed","response":{"status":"failed"}}` + "\n\n",
		"bad JSON":              "data: {broken\n\n",
		"arguments changed":     `data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"x\":1}"}` + "\n\ndata: " + `{"type":"response.completed","response":` + valid + "}\n\n",
		"call identity changed": `data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"different","name":"double"}}` + "\n\ndata: " + `{"type":"response.completed","response":` + valid + "}\n\n",
		"invalid arguments":     "data: " + `{"type":"response.completed","response":` + strings.Replace(valid, `"arguments":"{}"`, `"arguments":"broken"`, 1) + "}\n\n",
	}
	for name, stream := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, stream) }))
			defer server.Close()
			client, _ := NewClient("test-key")
			client.apiBaseURL = server.URL
			result, err := client.ChatStream(context.Background(), ChatRequest{Model: AstraModel}, StreamHandlers{})
			if err == nil || len(result.Choices) != 0 {
				t.Fatalf("unfinished response accepted: %+v %v", result, err)
			}
			var classified *StreamError
			if !errors.As(err, &classified) {
				t.Fatalf("unclassified error: %v", err)
			}
		})
	}
}

func TestResponsesPreparedBodyIsTheBodyDispatched(t *testing.T) {
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		io.WriteString(w, `data: {"type":"response.done","response":{"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}}`+"\n\n")
	}))
	defer server.Close()
	client, _ := NewClient("test-key")
	client.apiBaseURL = server.URL
	request, err := PrepareRequest(ChatRequest{Model: AstraModel, Stream: true, Messages: []Message{{Role: "user", Content: "original"}}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := RequestBytes(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Messages[0].Content = "mutation after preflight"
	copyBody, _ := RequestBytes(request)
	copyBody[0] = '!'
	if _, err := client.ChatStream(context.Background(), request, StreamHandlers{}); err != nil {
		t.Fatal(err)
	}
	if string(sent) != string(want) {
		t.Fatal("dispatch re-encoded or mutated the admitted body")
	}
}

func TestResponsesCancellationStopsBlockedStream(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, _ := NewClient("test-key")
	client.apiBaseURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.ChatStream(ctx, ChatRequest{Model: AstraModel}, StreamHandlers{})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop stream")
	}
}

func TestResponsesUsagePreservesUnknownAndZero(t *testing.T) {
	cases := []struct{ usage, want string }{
		{`{"input_tokens":0,"output_tokens":12,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":4}}`, `{"input_tokens":0,"output_tokens":12,"reasoning_output_tokens":4,"cached_input_tokens":0}`},
		{`{"input_tokens":1,"input_tokens":2,"output_tokens":3}`, `{"output_tokens":3}`},
		{`{"input_tokens":-1,"output_tokens":1.5,"total_tokens":1e3,"output_tokens_details":{"reasoning_tokens":9223372036854775808}}`, `null`},
		{`{"input_tokens":null,"output_tokens":2,"input_tokens_details":{"cached_tokens":1,"cached_tokens":2,"cache_write_tokens":0}}`, `{"output_tokens":2,"cache_write_input_tokens":0}`},
	}
	for _, test := range cases {
		t.Run(test.usage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":`+test.usage+`}`)
			}))
			defer server.Close()
			client, _ := NewClient("test-key")
			client.apiBaseURL = server.URL
			result, err := client.Chat(ChatRequest{Model: AstraModel})
			if err != nil {
				t.Fatal(err)
			}
			if got := mustJSON(t, result.Usage); got != test.want {
				t.Fatalf("usage=%s want %s", got, test.want)
			}
		})
	}
}
