package memorybackground

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// ChatClient is the OpenRouter client surface the remote model needs.
type ChatClient interface {
	ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error)
}

// OpenRouterModel calls one pinned OpenRouter model through Chat
// Completions with JSON output, temperature zero, low reasoning effort and
// no tools. Only the pass's own request (owner messages and Claim text)
// leaves the machine.
type OpenRouterModel struct {
	client ChatClient
	model  string
}

func NewOpenRouterModel(client ChatClient, model string) (*OpenRouterModel, error) {
	if err := ValidateRemoteModel(model); err != nil {
		return nil, err
	}
	if openrouter.UsesResponses(model) {
		return nil, errors.New("the background pass needs a Chat Completions model")
	}
	return &OpenRouterModel{client: client, model: model}, nil
}

func (m *OpenRouterModel) Identity() string { return ProviderOpenRouter + ":" + m.model }

func (m *OpenRouterModel) Complete(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	temperature := 0.0
	response, err := m.client.ChatStream(ctx, openrouter.ChatRequest{
		Model:          m.model,
		Messages:       []openrouter.Message{{Role: "system", Content: request.System}, {Role: "user", Content: request.User}},
		Temperature:    &temperature,
		MaxTokens:      request.MaxOutputTokens,
		Reasoning:      &openrouter.ReasoningConfig{Effort: "low"},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}, openrouter.StreamHandlers{})
	if err != nil {
		return ModelResponse{}, classifyProviderError(err)
	}
	result := ModelResponse{}
	if response.Usage != nil && response.Usage.InputTokens != nil && response.Usage.OutputTokens != nil {
		result.InputTokens, result.OutputTokens, result.UsageReported = *response.Usage.InputTokens, *response.Usage.OutputTokens, true
	}
	if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 0 {
		return result, malformed("the model returned no single text answer")
	}
	result.Text = response.Choices[0].Message.Content
	return result, nil
}

// classifyProviderError applies the Stage 2 provider robustness rule: only
// a failure that proved nothing was generated may be retried (no HTTP
// response at all, or 429/502/503/504), honoring Retry-After. A provider
// error body is never kept.
func classifyProviderError(err error) error {
	var stream *openrouter.StreamError
	if !errors.As(err, &stream) {
		return &ProviderError{Err: err, MayHaveBilled: true}
	}
	classified := &ProviderError{Err: errors.New(string(stream.Kind)), RetryAfter: stream.RetryAfter, MayHaveBilled: true}
	switch stream.HTTPStatus {
	case 0:
		classified.Retryable = stream.NoResponse
		classified.MayHaveBilled = !stream.NoResponse
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		classified.Retryable, classified.MayHaveBilled = true, false
	default:
		classified.MayHaveBilled = false
	}
	if stream.HTTPStatus != 0 {
		classified.Err = errors.New(string(stream.Kind) + ": HTTP " + http.StatusText(stream.HTTPStatus))
	}
	if stream.Kind == openrouter.StreamProviderResponseInvalid {
		// A truncated or invalid generation was billed and is malformed.
		return errors.Join(classified, ErrMalformed)
	}
	return classified
}

// LocalCompleter is the local extractor's chat surface (Ollama).
type LocalCompleter interface {
	Chat(context.Context, memory.CompilerGeneration, string, string, int64) (string, int64, int64, error)
}

// LocalModel runs the pass on the explicitly configured loopback extractor
// (EVIE_COMPILER_CONFIG): nothing leaves the machine and nothing is billed.
type LocalModel struct {
	extractor  LocalCompleter
	generation memory.CompilerGeneration
}

func NewLocalModel(extractor LocalCompleter, generation memory.CompilerGeneration) *LocalModel {
	return &LocalModel{extractor: extractor, generation: generation}
}

func (m *LocalModel) Identity() string { return ProviderLocal + ":" + m.generation.ModelArtifact }

func (m *LocalModel) Complete(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	text, input, output, err := m.extractor.Chat(ctx, m.generation, request.System, request.User, request.MaxOutputTokens)
	if errors.Is(err, localextractor.ErrRuntimeOutputInvalid) {
		// Deterministic decoding repeats a truncated reply: count it as
		// malformed so a stuck batch is eventually skipped.
		return ModelResponse{}, errors.Join(&ProviderError{Err: err}, ErrMalformed)
	}
	if err != nil {
		return ModelResponse{}, &ProviderError{Err: err}
	}
	return ModelResponse{Text: text, InputTokens: input, OutputTokens: output, UsageReported: input > 0 || output > 0}, nil
}
