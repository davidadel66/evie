package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// C8: a provider context-length rejection is a structural signal, detected
// from the bounded error body without ever surfacing that body.
func TestStreamErrorMarksProviderContextLengthRejections(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		body      string
		responses bool
		want      bool
	}{
		{name: "openrouter maximum context length", status: http.StatusBadRequest, want: true,
			body: `{"error":{"code":400,"message":"This endpoint's maximum context length is 131072 tokens. However, you requested about 140000 tokens (secret-token). Please reduce the length of either one, or use the \"middle-out\" transform."}}`},
		{name: "openai context_length_exceeded code", status: http.StatusBadRequest, want: true,
			body: `{"error":{"message":"secret-token","type":"invalid_request_error","code":"context_length_exceeded"}}`},
		{name: "anthropic prompt too long", status: http.StatusBadRequest, want: true,
			body: `{"error":{"message":"prompt is too long: 210000 tokens > 200000 maximum (secret-token)"}}`},
		{name: "context window wording", status: http.StatusBadRequest, want: true,
			body: `{"error":{"message":"Input exceeds the Context Window of this model (secret-token)"}}`},
		{name: "payload too large", status: http.StatusRequestEntityTooLarge, want: true, body: `secret-token`},
		{name: "responses path", status: http.StatusBadRequest, responses: true, want: true,
			body: `{"error":{"code":"context_length_exceeded","message":"secret-token"}}`},
		{name: "unrelated bad request", status: http.StatusBadRequest, want: false,
			body: `{"error":{"message":"tools[0].function.name is invalid (secret-token)"}}`},
		{name: "context wording on server error", status: http.StatusBadGateway, want: false,
			body: `{"error":{"message":"maximum context length upstream (secret-token)"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient("key")
			if err != nil {
				t.Fatal(err)
			}
			client.baseURL = "http://provider.test/chat/completions"
			client.apiBaseURL = "http://provider.test"
			client.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(test.body))}, nil
			})}
			request := ChatRequest{Model: "test"}
			if test.responses {
				request = ChatRequest{Model: AstraModel, MaxTokens: 1024, Messages: []Message{{Role: "user", Content: "hello"}}}
			}
			_, err = client.ChatStream(context.Background(), request, StreamHandlers{})
			var streamErr *StreamError
			if !errors.As(err, &streamErr) || streamErr.HTTPStatus != test.status {
				t.Fatalf("error=%v typed=%+v", err, streamErr)
			}
			if streamErr.ContextLengthExceeded != test.want {
				t.Fatalf("ContextLengthExceeded=%v, want %v", streamErr.ContextLengthExceeded, test.want)
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "context length") {
				t.Fatalf("error leaked provider detail: %v", err)
			}
		})
	}
}
