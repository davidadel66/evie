package localextractor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"strings"
	"time"
)

// VerifyCompilerConfiguration performs bounded metadata-only verification. It
// never loads a model or sends evidence to the generation endpoint.
func (o *Ollama) VerifyCompilerConfiguration(ctx context.Context, g memory.CompilerGeneration) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	id, _, err := memory.CompilerGenerationIdentity(g)
	if err != nil || id != o.generationID {
		return fmt.Errorf("%w: compiler generation identity mismatch", eviedb.ErrCompilerConfiguration)
	}
	var version struct {
		Version string `json:"version"`
	}
	if err := o.call(ctx, "GET", "/api/version", nil, &version); err != nil {
		return err
	}
	if version.Version != g.RuntimeVersion {
		return fmt.Errorf("%w: pinned runtime version mismatch", eviedb.ErrCompilerConfiguration)
	}
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := o.call(ctx, "GET", "/api/tags", nil, &tags); err != nil {
		return err
	}
	matched := 0
	for _, model := range tags.Models {
		if model.Name == g.ModelArtifact && strings.TrimPrefix(model.Digest, "sha256:") == g.ModelManifestSHA256 {
			matched++
		}
	}
	if matched != 1 {
		return fmt.Errorf("%w: pinned model artifact mismatch", eviedb.ErrCompilerConfiguration)
	}
	var show struct {
		Template string `json:"template"`
		System   string `json:"system"`
		Details  struct {
			Quantization string `json:"quantization_level"`
		} `json:"details"`
		ModelInfo map[string]json.RawMessage `json:"model_info"`
	}
	if err := o.call(ctx, "POST", "/api/show", map[string]any{"model": g.ModelArtifact, "verbose": true}, &show); err != nil {
		return err
	}
	tokenizer := map[string]json.RawMessage{}
	for key, value := range show.ModelInfo {
		if strings.HasPrefix(key, "tokenizer.") {
			tokenizer[key] = value
		}
	}
	tokenizerJSON, err := json.Marshal(tokenizer)
	if err != nil {
		return err
	}
	if show.Template != g.Template || show.System != "" || show.Details.Quantization != g.Quantization || len(tokenizer) == 0 || memory.CompilerHash(tokenizerJSON) != g.TokenizerSHA256 {
		return fmt.Errorf("%w: pinned template, system, tokenizer or quantization mismatch", eviedb.ErrCompilerConfiguration)
	}
	return nil
}
