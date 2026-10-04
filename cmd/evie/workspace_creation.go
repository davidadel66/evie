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

// SetWorkspaceExplore records the owner's explore setting. The explore
// preset is built in and selects no plugin, so there is nothing to resolve.
func (c *webContextSessionController) SetWorkspaceExplore(ctx context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, setting memory.WorkspaceExploreSetting) (memory.Workspace, error) {
	return c.store.SetWorkspaceExplore(ctx, id, revision, setting)
}

// SetWorkspaceBuild records the owner's build setting (Stage 11). Turning it
// on needs the build preset to resolve, as research does; turning it off
// never does.
func (c *webContextSessionController) SetWorkspaceBuild(ctx context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, setting memory.WorkspaceBuildSetting) (memory.Workspace, error) {
	if setting == memory.WorkspaceBuildOn {
		if _, err := c.manager.ResolvePresetContext(ctx, plugins.BuildPresetID); err != nil {
			return memory.Workspace{}, fmt.Errorf("validate build allowance: %w", err)
		}
	}
	return c.store.SetWorkspaceBuild(ctx, id, revision, setting)
}
