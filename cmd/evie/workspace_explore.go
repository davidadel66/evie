package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

const workspacesUsage = "usage: evie workspaces list | evie workspaces explore <workspace-id> on|off|default"

// runWorkspaceManagement answers `evie workspaces list`, which shows each
// Workspace with its folder and explore setting, and `evie workspaces
// explore <workspace-id> on|off|default`, which records the owner's explore
// setting as a new revision of the Workspace's current configuration
// (2026-10-03). It contacts no provider.
func runWorkspaceManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) (bool, error) {
	if len(args) == 0 || args[0] != "workspaces" {
		return false, nil
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	switch {
	case len(args) == 2 && args[1] == "list":
		workspaces, err := store.ListWorkspaces(ctx, true)
		if err != nil {
			return true, err
		}
		if workspaces == nil {
			workspaces = []memory.Workspace{}
		}
		return true, encoder.Encode(struct {
			Workspaces []memory.Workspace `json:"workspaces"`
		}{workspaces})
	case len(args) == 4 && args[1] == "explore":
		setting := memory.WorkspaceExploreSetting(args[3])
		if !setting.Valid() {
			return true, errors.New(workspacesUsage)
		}
		workspaces, err := store.ListWorkspaces(ctx, true)
		if err != nil {
			return true, err
		}
		for _, workspace := range workspaces {
			if string(workspace.ID) != args[2] {
				continue
			}
			changed, err := store.SetWorkspaceExplore(ctx, workspace.ID, workspace.CurrentRevisionID, setting)
			if err != nil {
				return true, fmt.Errorf("set Workspace explore: %w", err)
			}
			return true, encoder.Encode(changed)
		}
		return true, fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, args[2])
	}
	return true, errors.New(workspacesUsage)
}
