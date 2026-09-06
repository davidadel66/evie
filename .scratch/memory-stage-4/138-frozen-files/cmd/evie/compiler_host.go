package main

import (
	"context"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type compilerHostKernel interface {
	RunCompilerHost(context.Context, eviedb.CompilerSupervisorConfig) error
}

// Only long-lived actual runtime entries call this adapter. Empty configuration
// leaves conversation and explicit memory available without background jobs.
func startConfiguredCompilerHost(ctx context.Context, path string, kernel compilerHostKernel) (func() error, error) {
	if path == "" {
		return func() error { return nil }, nil
	}
	config, extractor, err := readCompilerHostConfiguration(path)
	if err != nil {
		return nil, err
	}
	id, _, err := memory.CompilerGenerationIdentity(config.Generation)
	if err != nil {
		return nil, err
	}
	return startCompilerHost(ctx, kernel, eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: extractor}}), nil
}

func startCompilerHost(ctx context.Context, kernel compilerHostKernel, config eviedb.CompilerSupervisorConfig) func() error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- kernel.RunCompilerHost(ctx, config) }()
	return func() error {
		cancel()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case err := <-done:
			if err == context.Canceled {
				return nil
			}
			return err
		case <-timer.C:
			return errors.New("compiler shutdown cleanup exceeded five seconds")
		}
	}
}
