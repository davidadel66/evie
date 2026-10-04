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

const workspacesUsage = "usage: evie workspaces list | evie workspaces explore <workspace-id> on|off|default | evie workspaces build <workspace-id> on|off|default | evie workspaces verify <workspace-id> [--setup <command>] [--verify <command>]" +
	" | evie workspaces access <workspace-id> [--expect <revision> (--set <connection>=<resource>[,<resource>...]... | --clear)]"

// runWorkspaceManagement answers `evie workspaces list`, which shows each
// Workspace with its folder and explore and build settings, `evie workspaces
// explore <workspace-id> on|off|default`, which records the owner's explore
// setting as a new revision of the Workspace's current configuration
// (2026-10-03), and `evie workspaces build <workspace-id> on|off|default`,
// which does the same for build workers, off unless turned on (Stage 11). It
// contacts no provider.
func runWorkspaceManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) (bool, error) {
	if len(args) == 0 || args[0] != "workspaces" {
		return false, nil
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	switch {
	case len(args) >= 3 && args[1] == "verify":
		// The merge gate's setup and verify commands (Stage 12).
		return true, workspaceVerify(ctx, args, out, store)
	case len(args) >= 3 && args[1] == "access":
		// Workspace Access, the ceiling of workflow authority (Stage 14).
		return true, workspaceAccess(ctx, args, out, store)
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
	case len(args) == 4 && (args[1] == "explore" || args[1] == "build"):
		var set func(memory.Workspace) (memory.Workspace, error)
		switch args[1] {
		case "explore":
			setting := memory.WorkspaceExploreSetting(args[3])
			if !setting.Valid() {
				return true, errors.New(workspacesUsage)
			}
			set = func(w memory.Workspace) (memory.Workspace, error) {
				return store.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, setting)
			}
		default:
			setting := memory.WorkspaceBuildSetting(args[3])
			if !setting.Valid() {
				return true, errors.New(workspacesUsage)
			}
			set = func(w memory.Workspace) (memory.Workspace, error) {
				return store.SetWorkspaceBuild(ctx, w.ID, w.CurrentRevisionID, setting)
			}
		}
		workspaces, err := store.ListWorkspaces(ctx, true)
		if err != nil {
			return true, err
		}
		for _, workspace := range workspaces {
			if string(workspace.ID) != args[2] {
				continue
			}
			changed, err := set(workspace)
			if err != nil {
				return true, fmt.Errorf("set Workspace %s: %w", args[1], err)
			}
			return true, encoder.Encode(changed)
		}
		return true, fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, args[2])
	}
	return true, errors.New(workspacesUsage)
}
