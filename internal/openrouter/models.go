package openrouter

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Model is the public, credential-free chat catalog entry. Provider is the
// model author's namespace, not the inference host OpenRouter routes to.
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

func (c *Client) ListChatModels(ctx context.Context) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var response struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Architecture struct {
				Input  []string `json:"input_modalities"`
				Output []string `json:"output_modalities"`
			} `json:"architecture"`
			Parameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := c.getMetadata(ctx, "/models", &response, 8<<20); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(response.Data))
	seen := make(map[string]bool)
	for _, entry := range response.Data {
		provider, _, err := modelSegments(entry.ID)
		if err != nil || seen[entry.ID] || !containsParameter(entry.Architecture.Input, "text") || !containsParameter(entry.Architecture.Output, "text") || !containsParameter(entry.Parameters, "tools") {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = entry.ID
		}
		models = append(models, Model{ID: entry.ID, Name: name, Provider: provider})
		seen[entry.ID] = true
	}
	if len(models) == 0 {
		return nil, errors.New("chat model catalog is empty")
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		if models[i].Name != models[j].Name {
			return models[i].Name < models[j].Name
		}
		return models[i].ID < models[j].ID
	})
	return models, nil
}

// ModelPricing is a model's catalog price in USD per token.
type ModelPricing struct {
	Prompt     float64
	Completion float64
}

// ModelPricing reads one model's per-token prices from the public model
// catalog. It fails when the model is not listed or its prices are not
// non-negative numbers, so a caller can fall back to a conservative price.
func (c *Client) ModelPricing(ctx context.Context, model string) (ModelPricing, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := c.getMetadata(ctx, "/models", &response, 8<<20); err != nil {
		return ModelPricing{}, err
	}
	for _, entry := range response.Data {
		if entry.ID != model {
			continue
		}
		prompt, promptErr := strconv.ParseFloat(entry.Pricing.Prompt, 64)
		completion, completionErr := strconv.ParseFloat(entry.Pricing.Completion, 64)
		if promptErr != nil || completionErr != nil || prompt < 0 || completion < 0 || math.IsNaN(prompt) || math.IsNaN(completion) {
			return ModelPricing{}, errors.New("model catalog price is not a non-negative number")
		}
		return ModelPricing{Prompt: prompt, Completion: completion}, nil
	}
	return ModelPricing{}, errors.New("model is not in the catalog")
}

// ResolveChatContextProfile keeps explicit limits strict. The default working
// ceiling may shrink to a selected model's route-safe window.
func (c *Client) ResolveChatContextProfile(ctx context.Context, model string) (ContextProfile, error) {
	return c.resolveContextProfile(ctx, model, true)
}
