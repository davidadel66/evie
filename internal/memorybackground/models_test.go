package memorybackground

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

type fakeChat struct {
	request  openrouter.ChatRequest
	response openrouter.ChatResponse
	err      error
}

func (f *fakeChat) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	f.request = request
	return f.response, f.err
}

func TestOpenRouterModelSendsOneStructuredRequestToThePinnedModel(t *testing.T) {
	input, output := int64(812), int64(64)
	chat := &fakeChat{response: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: `{"memories":[]}`}}},
		Usage: &openrouter.TokenUsage{InputTokens: &input, OutputTokens: &output}}}
	model, err := NewOpenRouterModel(chat, DefaultModel)
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.Complete(context.Background(), ModelRequest{System: "system", User: "user", MaxOutputTokens: 4000})
	if err != nil || response.Text != `{"memories":[]}` || response.InputTokens != 812 || !response.UsageReported {
		t.Fatalf("response = %+v, %v", response, err)
	}
	request := chat.request
	if request.Model != DefaultModel || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Content != "user" ||
		len(request.Tools) != 0 || request.MaxTokens != 4000 || request.Temperature == nil || *request.Temperature != 0 ||
		request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" || request.Reasoning == nil || request.Reasoning.Effort != "low" {
		t.Fatalf("request = %+v", request)
	}
	if model.Identity() != "openrouter:"+DefaultModel {
		t.Fatalf("identity = %s", model.Identity())
	}
	if _, err := NewOpenRouterModel(chat, DefaultModel+":batch"); err == nil {
		t.Fatal("the :batch variant was accepted for synchronous calls")
	}
}

func TestOpenRouterFailuresAreClassifiedByTheStage2RetryRule(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		retryable bool
		billed    bool
		malformed bool
	}{
		{"no response", &openrouter.StreamError{Kind: openrouter.StreamProviderError, NoResponse: true, Err: errors.New("reset")}, true, false, false},
		{"rate limited", &openrouter.StreamError{Kind: openrouter.StreamProviderError, HTTPStatus: http.StatusTooManyRequests, RetryAfter: 3 * time.Second, Err: errors.New("429")}, true, false, false},
		{"unavailable", &openrouter.StreamError{Kind: openrouter.StreamProviderError, HTTPStatus: http.StatusServiceUnavailable, Err: errors.New("503")}, true, false, false},
		{"bad request", &openrouter.StreamError{Kind: openrouter.StreamProviderError, HTTPStatus: http.StatusBadRequest, Err: errors.New("400 secret body")}, false, false, false},
		{"idle stream", &openrouter.StreamError{Kind: openrouter.StreamProviderError, Err: openrouter.ErrStreamIdle}, false, true, false},
		{"truncated", &openrouter.StreamError{Kind: openrouter.StreamProviderResponseInvalid, Err: errors.New("length")}, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, _ := NewOpenRouterModel(&fakeChat{err: tc.err}, DefaultModel)
			_, err := model.Complete(context.Background(), ModelRequest{})
			var provider *ProviderError
			if !errors.As(err, &provider) || provider.Retryable != tc.retryable || provider.MayHaveBilled != tc.billed || errors.Is(err, ErrMalformed) != tc.malformed {
				t.Fatalf("classified = %+v (%v)", provider, err)
			}
			if tc.name == "rate limited" && provider.RetryAfter != 3*time.Second {
				t.Fatalf("Retry-After lost: %+v", provider)
			}
			if tc.name == "bad request" && provider.Error() == "400 secret body" {
				t.Fatal("a provider error body was kept")
			}
		})
	}
	model, _ := NewOpenRouterModel(&fakeChat{response: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{ToolCalls: []openrouter.ToolCall{{ID: "x"}}}}}}}, DefaultModel)
	if _, err := model.Complete(context.Background(), ModelRequest{}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("a tool call answer = %v", err)
	}
}
