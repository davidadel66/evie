package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/worktree"
)

const projectsUsage = "usage: evie projects list | evie projects verify <project-id or root> [--setup <command>] [--verify <command>]"

// verifyFlags are the owner's --setup and --verify values; nil leaves a
// command as it is.
type verifyFlags struct{ setup, verify *string }

func parseVerifyFlags(args []string, usage string) (verifyFlags, error) {
	var flags verifyFlags
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return flags, errors.New(usage)
		}
		value := args[i+1]
		switch args[i] {
		case "--setup":
			flags.setup = &value
		case "--verify":
			flags.verify = &value
		default:
			return flags, errors.New(usage)
		}
	}
	return flags, nil
}

// apply is current with the flags the owner gave.
func (f verifyFlags) apply(current memory.BuildVerify) (setup, verify string, changed bool) {
	setup, verify = current.Setup, current.Verify
	if f.setup != nil {
		setup, changed = *f.setup, true
	}
	if f.verify != nil {
		verify, changed = *f.verify, true
	}
	return setup, verify, changed
}

// verifyView is what `verify` shows: the commands in effect and what the
// repository's instructions suggest. A suggestion is never recorded.
type verifyView struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id,omitempty"`
	ProjectID   memory.ProjectID   `json:"project_id,omitempty"`
	Folder      string             `json:"folder,omitempty"`
	Verify      memory.BuildVerify `json:"verify"`
	Proposal    *worktree.Proposal `json:"proposal,omitempty"`
	Note        string             `json:"note,omitempty"`
}

const proposalNote = "Suggested from the repository's instructions; nothing is set until you pass --setup or --verify."

// workspaceVerify answers `evie workspaces verify <workspace-id> [--setup
// <command>] [--verify <command>]` (Stage 12, 2026-10-04).
func workspaceVerify(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) error {
	if len(args) < 3 {
		return errors.New(workspacesUsage)
	}
	flags, err := parseVerifyFlags(args[3:], workspacesUsage)
	if err != nil {
		return err
	}
	workspaces, err := store.ListWorkspaces(ctx, true)
	if err != nil {
		return err
	}
	for _, w := range workspaces {
		if string(w.ID) != args[2] {
			continue
		}
		setup, verify, changed := flags.apply(w.Verify)
		if changed {
			if w, err = store.SetWorkspaceVerify(ctx, w.ID, w.Verify.Revision, setup, verify); err != nil {
				return fmt.Errorf("set the Workspace's setup and verify commands: %w", err)
			}
		}
		view := verifyView{WorkspaceID: w.ID, Folder: w.Folder.Path, Verify: w.Verify}
		if !changed && w.Folder.Path != "" {
			if proposal := worktree.ProposeCommands(w.Folder.Path); proposal != (worktree.Proposal{}) {
				view.Proposal, view.Note = &proposal, proposalNote
			}
		}
		return encodeJSON(out, view)
	}
	return fmt.Errorf("%w: %s", eviedb.ErrWorkspaceNotFound, args[2])
}

// projectListing is one project as `evie projects list` shows it.
type projectListing struct {
	memory.Project
	Verify memory.BuildVerify `json:"verify"`
}

// runProjectManagement answers `evie projects list`, which shows each
// registered project with its setup and verify commands, and `evie projects
// verify <project-id or root> [--setup <command>] [--verify <command>]`,
// which shows them with the repository's suggestions or sets them (Stage
// 12, 2026-10-04). It contacts no provider.
func runProjectManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) (bool, error) {
	if len(args) == 0 || args[0] != "projects" {
		return false, nil
	}
	switch {
	case len(args) == 2 && args[1] == "list":
		projects, err := store.ListProjects(ctx, true)
		if err != nil {
			return true, err
		}
		listed := make([]projectListing, 0, len(projects))
		for _, p := range projects {
			verify, err := store.BuildVerifyFor(ctx, memory.ScopeContext{ProjectID: p.ID})
			if err != nil {
				return true, err
			}
			listed = append(listed, projectListing{Project: p, Verify: verify})
		}
		return true, encodeJSON(out, struct {
			Projects []projectListing `json:"projects"`
		}{listed})
	case len(args) >= 3 && args[1] == "verify":
		flags, err := parseVerifyFlags(args[3:], projectsUsage)
		if err != nil {
			return true, err
		}
		project, err := findProject(ctx, store, args[2])
		if err != nil {
			return true, err
		}
		current, err := store.BuildVerifyFor(ctx, memory.ScopeContext{ProjectID: project.ID})
		if err != nil {
			return true, err
		}
		setup, verify, changed := flags.apply(current)
		if changed {
			if current, err = store.SetProjectVerify(ctx, project.ID, current.Revision, setup, verify); err != nil {
				return true, fmt.Errorf("set the project's setup and verify commands: %w", err)
			}
		}
		view := verifyView{ProjectID: project.ID, Folder: project.CanonicalRoot, Verify: current}
		if !changed {
			if proposal := worktree.ProposeCommands(project.CanonicalRoot); proposal != (worktree.Proposal{}) {
				view.Proposal, view.Note = &proposal, proposalNote
			}
		}
		return true, encodeJSON(out, view)
	}
	return true, errors.New(projectsUsage)
}

// findProject is the registered project with this ID, or at this root.
func findProject(ctx context.Context, store *eviedb.Store, key string) (memory.Project, error) {
	projects, err := store.ListProjects(ctx, true)
	if err != nil {
		return memory.Project{}, err
	}
	for _, p := range projects {
		if string(p.ID) == key {
			return p, nil
		}
	}
	if strings.HasPrefix(key, "/") || strings.HasPrefix(key, ".") || strings.HasPrefix(key, "~") {
		return store.FindProjectByRoot(ctx, key)
	}
	return memory.Project{}, fmt.Errorf("%w: %s", eviedb.ErrProjectNotFound, key)
}

func encodeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
