package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	BuiltinModel                      = "moonshotai/kimi-k3"
	defaultContextWorkingTokens int64 = 262144
	defaultContextOutputTokens  int64 = 16384
	contextEstimationMargin     int64 = 4096
	builtinContextFallback      int64 = 262144
	maxContextMetadataBody            = 1 << 20
)

type ContextProfileSource string

const (
	ContextProfileRemoteMetadata   ContextProfileSource = "remote_metadata"
	ContextProfileExplicitOverride ContextProfileSource = "explicit_override"
	ContextProfileBuiltinFallback  ContextProfileSource = "builtin_fallback"
)

// ContextProfile is constructed only after all Stage 2 token limits have been
// validated. Its fields are private so a live session cannot mutate them.
type ContextProfile struct {
	configuredModel        string
	advertisedModel        string
	canonicalModel         string
	advertisedWindowTokens int64
	hardWindowTokens       int64
	workingTokens          int64
	outputReserveTokens    int64
	estimationMarginTokens int64
	source                 ContextProfileSource
}

type ContextProfileDiagnostics struct {
	ConfiguredModel        string
	AdvertisedModel        string
	CanonicalModel         string
	AdvertisedWindowTokens int64
	HardWindowTokens       int64
	WorkingTokens          int64
	OutputReserveTokens    int64
	EstimationMarginTokens int64
	Source                 ContextProfileSource
}

type contextProfileValidationError struct{ err error }

func (e *contextProfileValidationError) Error() string { return e.err.Error() }
func (e *contextProfileValidationError) Unwrap() error { return e.err }

func invalidContextProfile(message string) error {
	return &contextProfileValidationError{err: errors.New(message)}
}

func (p ContextProfile) Diagnostics() ContextProfileDiagnostics {
	return ContextProfileDiagnostics{
		ConfiguredModel:        p.configuredModel,
		AdvertisedModel:        p.advertisedModel,
		CanonicalModel:         p.canonicalModel,
		AdvertisedWindowTokens: p.advertisedWindowTokens,
		HardWindowTokens:       p.hardWindowTokens,
		WorkingTokens:          p.workingTokens,
		OutputReserveTokens:    p.outputReserveTokens,
		EstimationMarginTokens: p.estimationMarginTokens,
		Source:                 p.source,
	}
}

func (p ContextProfile) Model() string { return p.configuredModel }

func (p ContextProfile) OutputReserveTokens() int64 { return p.outputReserveTokens }

// NewExplicitContextProfile constructs the same validated profile used by the
// environment hard-window override. It is useful to startup adapters that
// obtain an explicit hard limit from a source other than process environment.
func NewExplicitContextProfile(
	model string,
	hardWindowTokens int64,
	workingTokens int64,
	outputReserveTokens int64,
) (ContextProfile, error) {
	return newContextProfile(ContextProfileDiagnostics{
		ConfiguredModel:        model,
		HardWindowTokens:       hardWindowTokens,
		WorkingTokens:          workingTokens,
		OutputReserveTokens:    outputReserveTokens,
		EstimationMarginTokens: contextEstimationMargin,
		Source:                 ContextProfileExplicitOverride,
	})
}

type contextProfileConfig struct {
	hardOverride int64
	working      int64
	output       int64
}

type modelMetadataResponse struct {
	Data struct {
		ID            string `json:"id"`
		CanonicalSlug string `json:"canonical_slug"`
		ContextLength *int64 `json:"context_length"`
	} `json:"data"`
}

type endpointMetadataResponse struct {
	Data struct {
		ID        string `json:"id"`
		Endpoints []struct {
			ContextLength       *int64   `json:"context_length"`
			MaxCompletionTokens *int64   `json:"max_completion_tokens"`
			Status              *int     `json:"status"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"endpoints"`
	} `json:"data"`
}

func (c *Client) ResolveContextProfile(ctx context.Context, model string) (ContextProfile, error) {
	if err := ctx.Err(); err != nil {
		return ContextProfile{}, err
	}
	if _, _, err := modelSegments(model); err != nil {
		return ContextProfile{}, err
	}
	config, err := loadContextProfileConfig()
	if err != nil {
		return ContextProfile{}, err
	}
	if config.hardOverride > 0 {
		return newContextProfile(ContextProfileDiagnostics{
			ConfiguredModel:        model,
			HardWindowTokens:       config.hardOverride,
			WorkingTokens:          config.working,
			OutputReserveTokens:    config.output,
			EstimationMarginTokens: contextEstimationMargin,
			Source:                 ContextProfileExplicitOverride,
		})
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, c.contextDiscoveryTimeout)
	defer cancel()
	profile, discoveryErr := c.discoverContextProfile(discoveryCtx, model, config)
	if discoveryErr == nil {
		return profile, nil
	}
	var validationErr *contextProfileValidationError
	if errors.As(discoveryErr, &validationErr) {
		return ContextProfile{}, validationErr
	}
	if err := ctx.Err(); err != nil {
		return ContextProfile{}, err
	}
	if model != BuiltinModel {
		return ContextProfile{}, fmt.Errorf("discover context profile for %q: %w", model, discoveryErr)
	}
	return newContextProfile(ContextProfileDiagnostics{
		ConfiguredModel:        model,
		CanonicalModel:         BuiltinModel,
		HardWindowTokens:       builtinContextFallback,
		WorkingTokens:          config.working,
		OutputReserveTokens:    config.output,
		EstimationMarginTokens: contextEstimationMargin,
		Source:                 ContextProfileBuiltinFallback,
	})
}

func (c *Client) discoverContextProfile(
	ctx context.Context,
	model string,
	config contextProfileConfig,
) (ContextProfile, error) {
	author, slug, _ := modelSegments(model)
	var modelResponse modelMetadataResponse
	if err := c.getContextMetadata(ctx, "/model/"+url.PathEscape(author)+"/"+url.PathEscape(slug), &modelResponse); err != nil {
		return ContextProfile{}, fmt.Errorf("look up focused model: %w", err)
	}
	modelData := modelResponse.Data
	if modelData.ID == "" || modelData.CanonicalSlug == "" || modelData.ContextLength == nil || *modelData.ContextLength <= 0 {
		return ContextProfile{}, errors.New("focused model metadata is incomplete")
	}
	canonicalAuthor, canonicalSlug, err := modelSegments(modelData.CanonicalSlug)
	if err != nil {
		return ContextProfile{}, fmt.Errorf("canonical model metadata: %w", err)
	}

	var endpointsResponse endpointMetadataResponse
	if err := c.getContextMetadata(
		ctx,
		"/models/"+url.PathEscape(canonicalAuthor)+"/"+url.PathEscape(canonicalSlug)+"/endpoints",
		&endpointsResponse,
	); err != nil {
		return ContextProfile{}, fmt.Errorf("look up model endpoints: %w", err)
	}
	if endpointsResponse.Data.ID != modelData.CanonicalSlug {
		return ContextProfile{}, fmt.Errorf(
			"endpoint model identity %q does not match canonical model %q",
			endpointsResponse.Data.ID,
			modelData.CanonicalSlug,
		)
	}
	hardWindow, err := routeSafeWindow(endpointsResponse, config.output)
	if err != nil {
		return ContextProfile{}, err
	}
	return newContextProfile(ContextProfileDiagnostics{
		ConfiguredModel:        model,
		AdvertisedModel:        modelData.ID,
		CanonicalModel:         modelData.CanonicalSlug,
		AdvertisedWindowTokens: *modelData.ContextLength,
		HardWindowTokens:       hardWindow,
		WorkingTokens:          config.working,
		OutputReserveTokens:    config.output,
		EstimationMarginTokens: contextEstimationMargin,
		Source:                 ContextProfileRemoteMetadata,
	})
}

func routeSafeWindow(response endpointMetadataResponse, outputReserve int64) (int64, error) {
	var hardWindow int64
	for i, endpoint := range response.Data.Endpoints {
		if endpoint.Status == nil {
			return 0, fmt.Errorf("endpoint %d is missing status", i)
		}
		if *endpoint.Status != 0 || !containsParameter(endpoint.SupportedParameters, "max_tokens") ||
			endpoint.MaxCompletionTokens == nil || *endpoint.MaxCompletionTokens < outputReserve {
			continue
		}
		if endpoint.ContextLength == nil || *endpoint.ContextLength <= 0 || *endpoint.MaxCompletionTokens <= 0 {
			return 0, fmt.Errorf("eligible endpoint %d has invalid token limits", i)
		}
		if hardWindow == 0 || *endpoint.ContextLength < hardWindow {
			hardWindow = *endpoint.ContextLength
		}
	}
	if hardWindow == 0 {
		return 0, errors.New("model has no endpoint eligible for the configured output reserve")
	}
	return hardWindow, nil
}

func containsParameter(parameters []string, want string) bool {
	for _, parameter := range parameters {
		if parameter == want {
			return true
		}
	}
	return false
}

func (c *Client) getContextMetadata(ctx context.Context, path string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.apiBaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxContextMetadataBody))
		return fmt.Errorf("metadata API returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxContextMetadataBody+1))
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}
	if len(body) > maxContextMetadataBody {
		return errors.New("metadata response exceeds size limit")
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}
	return nil
}

func loadContextProfileConfig() (contextProfileConfig, error) {
	hard, err := optionalPositiveTokenEnv("EVIE_CONTEXT_WINDOW_TOKENS")
	if err != nil {
		return contextProfileConfig{}, err
	}
	working, err := positiveTokenEnv("EVIE_CONTEXT_WORKING_TOKENS", defaultContextWorkingTokens)
	if err != nil {
		return contextProfileConfig{}, err
	}
	output, err := positiveTokenEnv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", defaultContextOutputTokens)
	if err != nil {
		return contextProfileConfig{}, err
	}
	if output > math.MaxInt64-contextEstimationMargin || output+contextEstimationMargin >= working {
		return contextProfileConfig{}, errors.New(
			"EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS plus the estimation margin must be below EVIE_CONTEXT_WORKING_TOKENS",
		)
	}
	return contextProfileConfig{hardOverride: hard, working: working, output: output}, nil
}

func optionalPositiveTokenEnv(name string) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, nil
	}
	return parsePositiveTokenEnv(name, raw)
}

func positiveTokenEnv(name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	return parsePositiveTokenEnv(name, raw)
}

func parsePositiveTokenEnv(name, raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func newContextProfile(d ContextProfileDiagnostics) (ContextProfile, error) {
	if d.ConfiguredModel == "" || d.HardWindowTokens <= 0 || d.WorkingTokens <= 0 ||
		d.OutputReserveTokens <= 0 || d.EstimationMarginTokens != contextEstimationMargin {
		return ContextProfile{}, invalidContextProfile("context profile contains non-positive or inconsistent values")
	}
	if d.WorkingTokens > d.HardWindowTokens {
		return ContextProfile{}, &contextProfileValidationError{err: fmt.Errorf(
			"context working ceiling %d exceeds hard window %d",
			d.WorkingTokens,
			d.HardWindowTokens,
		)}
	}
	if d.OutputReserveTokens > math.MaxInt64-d.EstimationMarginTokens ||
		d.OutputReserveTokens+d.EstimationMarginTokens >= d.WorkingTokens {
		return ContextProfile{}, invalidContextProfile("context output reserve plus estimation margin must be below the working ceiling")
	}
	switch d.Source {
	case ContextProfileRemoteMetadata:
		if d.AdvertisedModel == "" || d.CanonicalModel == "" || d.AdvertisedWindowTokens <= 0 {
			return ContextProfile{}, invalidContextProfile("remote context profile is missing model diagnostics")
		}
	case ContextProfileExplicitOverride:
		if d.AdvertisedModel != "" || d.CanonicalModel != "" || d.AdvertisedWindowTokens != 0 {
			return ContextProfile{}, invalidContextProfile("explicit context profile contains undiscovered model diagnostics")
		}
	case ContextProfileBuiltinFallback:
		if d.CanonicalModel != BuiltinModel || d.AdvertisedModel != "" || d.AdvertisedWindowTokens != 0 {
			return ContextProfile{}, invalidContextProfile("built-in fallback context profile has invalid model diagnostics")
		}
	default:
		return ContextProfile{}, invalidContextProfile("context profile source is invalid")
	}
	return ContextProfile{
		configuredModel:        d.ConfiguredModel,
		advertisedModel:        d.AdvertisedModel,
		canonicalModel:         d.CanonicalModel,
		advertisedWindowTokens: d.AdvertisedWindowTokens,
		hardWindowTokens:       d.HardWindowTokens,
		workingTokens:          d.WorkingTokens,
		outputReserveTokens:    d.OutputReserveTokens,
		estimationMarginTokens: d.EstimationMarginTokens,
		source:                 d.Source,
	}, nil
}

func modelSegments(model string) (string, string, error) {
	if strings.TrimSpace(model) != model || strings.Count(model, "/") != 1 {
		return "", "", fmt.Errorf("model %q must have author/slug form", model)
	}
	author, slug, _ := strings.Cut(model, "/")
	if author == "" || slug == "" {
		return "", "", fmt.Errorf("model %q must have author/slug form", model)
	}
	return author, slug, nil
}
