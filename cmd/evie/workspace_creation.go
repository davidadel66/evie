package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

func (c *webContextSessionController) RegisterWorkspaceWithOptions(ctx context.Context, options eviedb.WorkspaceRegistration) (memory.Workspace, error) {
	options.PresetID = strings.TrimSpace(options.PresetID)
	if options.PresetID == "" {
		options.PresetID = string(plugins.StandardPresetID)
	}
	if _, err := c.manager.ResolvePresetContext(ctx, plugins.PresetID(options.PresetID)); err != nil {
		return memory.Workspace{}, fmt.Errorf("validate Workspace Agent Preset: %w", err)
	}
	if options.AllowResearchDelegation {
		if _, err := c.manager.ResolvePresetContext(ctx, plugins.ResearchPresetID); err != nil {
			return memory.Workspace{}, fmt.Errorf("validate research allowance: %w", err)
		}
	}
	return c.store.RegisterWorkspaceWithOptions(ctx, options)
}

func (c *webContextSessionController) SetWorkspaceResearch(ctx context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, enabled bool) (memory.Workspace, error) {
	if enabled {
		if _, err := c.manager.ResolvePresetContext(ctx, plugins.ResearchPresetID); err != nil {
			return memory.Workspace{}, fmt.Errorf("validate research allowance: %w", err)
		}
	}
	return c.store.SetWorkspaceResearch(ctx, id, revision, enabled)
}
