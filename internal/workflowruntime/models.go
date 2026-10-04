package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// The model seam AI steps call (Stage 14 sub-stage 7, 2026-10-04; ADR
// 0010, 0015, 0019). The runtime builds each request itself from the step's
// reviewed prompt and declared input only, and sends it to the exact model
// its pinned Model Policy names, with that policy's parameters and no
// tools: the provider sees nothing else of the run, its session, memory or
// credentials. The production implementation is OpenRouter's Chat
// Completions API, reachable only when the owner allowed remote workflow
// models (EVIE_WORKFLOW_REMOTE_MODELS=on in cmd/evie).

// Models is the model provider AI steps call.
type Models interface {
	// Complete sends one request and returns the model's text answer and
	// usage. An error that proves nothing was generated is a *ModelError
	// with Retryable set; one that may have been billed sets
	// MayHaveBilled.
	Complete(ctx context.Context, req ModelRequest) (ModelResponse, error)
	// Pricing is the model's price for cost records, never failing: an
	// unknown price is a conservative fallback.
	Pricing(ctx context.Context, model string) ModelPricing
}

// ModelRequest is everything one AI attempt sends: the exact model and
// parameters of its pinned Model Policy, the reviewed prompt as the system
// message, and the step's declared input with its output schema, framed as
// data, as the user message.
type ModelRequest struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int64    `json:"max_output_tokens"`
	System          string   `json:"system"`
	User            string   `json:"user"`
}

// ModelResponse is the model's text answer and its usage, when the
// provider reported it.
type ModelResponse struct {
	Text          string
	InputTokens   int64
	OutputTokens  int64
	UsageReported bool
}

// ModelError classifies a failed model request (the Stage 2 provider
// robustness rule): Retryable only when nothing was generated (no response
// at all, or 429, 502, 503, 504), with the provider's Retry-After;
// MayHaveBilled unless the request provably generated nothing.
type ModelError struct {
	Err           error
	Retryable     bool
	RetryAfter    time.Duration
	MayHaveBilled bool
}

func (e *ModelError) Error() string { return e.Err.Error() }
func (e *ModelError) Unwrap() error { return e.Err }

// ModelPricing is a model's price in USD per million tokens and where it
// came from.
type ModelPricing struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
	Source           string  `json:"source"`
}

// MicroUSD is the cost of a token count in millionths of a dollar, rounded
// up so a cost is never under-counted.
func (p ModelPricing) MicroUSD(input, output int64) int64 {
	return int64(math.Ceil(float64(input)*p.InputPerMillion + float64(output)*p.OutputPerMillion))
}

// fallbackPricing is used when the catalog price cannot be read: above the
// usual small-model prices, so an unknown price only over-counts.
var fallbackPricing = ModelPricing{InputPerMillion: 2, OutputPerMillion: 8, Source: "fallback"}

// OpenRouterClient is the OpenRouter client surface the adapter needs.
type OpenRouterClient interface {
	ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error)
	ModelPricing(context.Context, string) (openrouter.ModelPricing, error)
}

// OpenRouterModels calls pinned OpenRouter models through Chat Completions
// with JSON output and no tools.
type OpenRouterModels struct {
	client OpenRouterClient
	mu     sync.Mutex
	prices map[string]ModelPricing
}

// NewOpenRouterModels is the production Models over an OpenRouter client.
func NewOpenRouterModels(client OpenRouterClient) *OpenRouterModels {
	return &OpenRouterModels{client: client, prices: map[string]ModelPricing{}}
}

// Complete sends exactly the request's two messages to its model, with its
// temperature and output bound, asking for a JSON object; it sends no
// tools, no reasoning settings and nothing else.
func (m *OpenRouterModels) Complete(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if req.Provider != "openrouter" {
		return ModelResponse{}, &ModelError{Err: fmt.Errorf("provider %s is not one this Evie calls", req.Provider)}
	}
	if openrouter.UsesResponses(req.Model) {
		return ModelResponse{}, &ModelError{Err: fmt.Errorf("model %s needs the Responses API, which AI steps do not use", req.Model)}
	}
	response, err := m.client.ChatStream(ctx, openrouter.ChatRequest{
		Model:          req.Model,
		Messages:       []openrouter.Message{{Role: "system", Content: req.System}, {Role: "user", Content: req.User}},
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxOutputTokens,
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}, openrouter.StreamHandlers{})
	result := ModelResponse{}
	if response.Usage != nil && response.Usage.InputTokens != nil && response.Usage.OutputTokens != nil {
		result.InputTokens, result.OutputTokens, result.UsageReported = *response.Usage.InputTokens, *response.Usage.OutputTokens, true
	}
	if err != nil {
		return result, classifyModelError(err)
	}
	if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 0 {
		return result, errors.New("the model returned no single text answer")
	}
	result.Text = response.Choices[0].Message.Content
	return result, nil
}

// Pricing reads the model's catalog price once and keeps it; an unknown
// price is the conservative fallback, read again next time.
func (m *OpenRouterModels) Pricing(ctx context.Context, model string) ModelPricing {
	m.mu.Lock()
	price, ok := m.prices[model]
	m.mu.Unlock()
	if ok {
		return price
	}
	catalog, err := m.client.ModelPricing(ctx, model)
	if err != nil {
		return fallbackPricing
	}
	price = ModelPricing{InputPerMillion: catalog.Prompt * 1e6, OutputPerMillion: catalog.Completion * 1e6, Source: "catalog"}
	m.mu.Lock()
	m.prices[model] = price
	m.mu.Unlock()
	return price
}

// classifyModelError applies the Stage 2 provider robustness rule: only a
// failure that proved nothing was generated may be retried (no HTTP
// response at all, or 429/502/503/504), honoring Retry-After. A provider's
// error body is never kept.
func classifyModelError(err error) error {
	var stream *openrouter.StreamError
	if !errors.As(err, &stream) {
		return &ModelError{Err: err, MayHaveBilled: true}
	}
	classified := &ModelError{Err: errors.New(string(stream.Kind)), RetryAfter: stream.RetryAfter, MayHaveBilled: true}
	switch stream.HTTPStatus {
	case 0:
		classified.Retryable = stream.NoResponse && stream.Kind == openrouter.StreamProviderError
		classified.MayHaveBilled = !classified.Retryable
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		classified.Retryable, classified.MayHaveBilled = true, false
	default:
		classified.MayHaveBilled = false
	}
	if stream.HTTPStatus != 0 {
		classified.Err = fmt.Errorf("%s: HTTP %d %s", stream.Kind, stream.HTTPStatus, http.StatusText(stream.HTTPStatus))
	}
	return classified
}
