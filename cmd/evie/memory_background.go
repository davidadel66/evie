package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/memorybackground"
	"github.com/davidadel66/evie/internal/openrouter"
)

// memoryBackgroundHost owns the always-on background pass (memory
// architecture Stage 4) for a long-lived REPL or web process: its resolved
// configuration, model and runner. It also answers status for the web
// Review area. Configuration problems disable the pass with a reason; they
// never stop Evie from starting.
type memoryBackgroundHost struct {
	runner  *memorybackground.Runner
	pricing func(context.Context) (memorybackground.Pricing, error)
}

// The runner is every root conversation's compaction flush (Stage 10).
var _ agent.CompactionMemoryFlush = (*memorybackground.Runner)(nil)

func newMemoryBackgroundHost(getenv func(string) string, store *eviedb.Store) *memoryBackgroundHost {
	config := memorybackground.FromEnv(getenv)
	host := &memoryBackgroundHost{}
	var model memorybackground.Model
	if config.Enabled {
		switch config.Provider {
		case memorybackground.ProviderOpenRouter:
			client, err := openrouter.NewClient(getenv(memorybackground.EnvAPIKey))
			if err == nil {
				model, err = memorybackground.NewOpenRouterModel(client, config.Model)
			}
			if err != nil {
				config.Enabled, config.Reason = false, "the OpenRouter model is unavailable: "+err.Error()
				break
			}
			host.pricing = func(ctx context.Context) (memorybackground.Pricing, error) {
				price, err := client.ModelPricing(ctx, config.Model)
				if err != nil {
					return memorybackground.Pricing{}, err
				}
				return memorybackground.Pricing{InputPerMillion: price.Prompt * 1e6, OutputPerMillion: price.Completion * 1e6, Source: "catalog"}, nil
			}
		case memorybackground.ProviderLocal:
			local, extractor, err := readCompilerHostConfiguration(config.LocalConfigPath)
			if err != nil {
				config.Enabled, config.Reason = false, "the local extractor configuration ("+memorybackground.EnvLocal+") cannot be used: "+err.Error()
				break
			}
			config.Model = local.Generation.ModelArtifact
			model = memorybackground.NewLocalModel(extractor, local.Generation)
		}
	}
	host.runner = memorybackground.NewRunner(store, model, config, memorybackground.WithLogger(log.Printf))
	return host
}

func (h *memoryBackgroundHost) MemoryBackgroundStatus(ctx context.Context) (memory.BackgroundStatus, error) {
	return h.runner.Status(ctx)
}

// Start runs the pass until ctx ends. Shutdown cancels an in-flight model
// call; nothing commits after it, and the durable cursor lets the next
// process redo the batch. The returned stop waits at most five seconds.
func (h *memoryBackgroundHost) Start(ctx context.Context) func() error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		if h.runner.Config().Enabled && h.pricing != nil {
			priceCtx, finish := context.WithTimeout(ctx, 10*time.Second)
			if price, err := h.pricing(priceCtx); err == nil {
				h.runner.UsePricing(price)
			} else if ctx.Err() == nil {
				log.Printf("memory background pass: model price unavailable, using the conservative fallback: %v", err)
			}
			finish()
		}
		done <- h.runner.Run(ctx)
	}()
	return sync.OnceValue(func() error {
		cancel()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case err := <-done:
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-timer.C:
			return errors.New("memory background pass shutdown exceeded five seconds")
		}
	})
}

// runMemoryBackgroundManagement answers `evie memory-background status`: the
// configuration this environment resolves to and the durable progress
// (activation, cursors, last run and problem, today's spend against the
// caps). It starts no pass and contacts no provider.
func runMemoryBackgroundManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store, getenv func(string) string) (bool, error) {
	if len(args) == 0 || args[0] != "memory-background" {
		return false, nil
	}
	if len(args) != 2 || args[1] != "status" {
		return true, errors.New("usage: memory-background status")
	}
	config := memorybackground.FromEnv(getenv)
	if config.Enabled && config.Provider == memorybackground.ProviderLocal {
		local, _, err := readCompilerHostConfiguration(config.LocalConfigPath)
		if err != nil {
			config.Enabled, config.Reason = false, "the local extractor configuration ("+memorybackground.EnvLocal+") cannot be used: "+err.Error()
		} else {
			config.Model = local.Generation.ModelArtifact
		}
	}
	runner := memorybackground.NewRunner(store, statusOnlyModel{config.Provider + ":" + config.Model}, config,
		memorybackground.WithPricing(memorybackground.Pricing{Source: "not read by the status command", InputPerMillion: memorybackground.FallbackPricing.InputPerMillion, OutputPerMillion: memorybackground.FallbackPricing.OutputPerMillion}))
	status, err := runner.Status(ctx)
	if err != nil {
		return true, fmt.Errorf("read memory background status: %w", err)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(status)
}

// statusOnlyModel names the configured model for status; it never runs.
type statusOnlyModel struct{ identity string }

func (m statusOnlyModel) Identity() string { return m.identity }
func (statusOnlyModel) Complete(context.Context, memorybackground.ModelRequest) (memorybackground.ModelResponse, error) {
	return memorybackground.ModelResponse{}, errors.New("the status command never runs a pass")
}
