package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

const changesUsage = "usage: evie changes list <workspace-id> | evie changes push <workspace-id> <branch> | evie changes merge <workspace-id> <change-id> | evie changes close <workspace-id> <change-id> --yes"

// changeLister lists a Workspace's Changes for `evie changes` and the web
// routes: Evie's build branches, read from the build records (story 3a),
// joined with the pull requests the last sync recorded (story 3b).
func changeLister(repositories *changes.Service, store *eviedb.Store) *changes.Lister {
	return &changes.Lister{Repositories: repositories, Builds: changes.NewBuildSource(store), PullRequests: changes.PullRequestSource{Store: store}}
}

// changeCommands is what `evie changes` needs; each is made only when a
// command uses it. getenv reads GITHUB_TOKEN; gate is the Kernel's merge
// gate, for merge and close.
type changeCommands struct {
	store        *eviedb.Store
	repositories func() *changes.Service
	getenv       func(string) string
	gate         func() (changes.BuildGate, error)
}

// runChangeManagement answers `evie changes list <workspace-id>`, which
// prints the Workspace's Repository and its Changes as the Changes tab
// lists them; `evie changes push <workspace-id> <branch>`, which pushes one
// of Evie's draft Changes and opens its pull request (story 4a); and
// `evie changes merge|close <workspace-id> <change-id>`, which merge or
// close one of Evie's Changes by the ID the list shows (story 5a). Close
// discards the branch, so it needs --yes.
func runChangeManagement(ctx context.Context, args []string, out io.Writer, c changeCommands) (bool, error) {
	if len(args) == 0 || args[0] != "changes" {
		return false, nil
	}
	command := ""
	if len(args) > 1 {
		command = args[1]
	}
	switch {
	case command == "list" && len(args) == 3 && args[2] != "":
	case (command == "push" || command == "merge") && len(args) == 4 && args[2] != "" && args[3] != "":
	case command == "close" && len(args) == 5 && args[2] != "" && args[3] != "" && args[4] == "--yes":
	case command == "close" && len(args) == 4:
		return true, errors.New("closing removes the Change's branch and worktree; pass --yes to close it. " + changesUsage)
	default:
		return true, errors.New(changesUsage)
	}
	ws := memory.WorkspaceID(args[2])
	folder, err := c.store.WorkspaceFolder(ctx, ws)
	if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
		return true, fmt.Errorf("no Workspace %q", args[2])
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	switch command {
	case "push":
		result, err := changePusher(c.store, c.repositories(), strings.TrimSpace(c.getenv("GITHUB_TOKEN")), nil).Push(ctx, ws, folder.Path, args[3])
		if err != nil {
			return true, err
		}
		return true, encoder.Encode(result)
	case "merge", "close":
		gate, err := c.gate()
		if err != nil {
			return true, err
		}
		merger := &changes.Merger{Lister: changeLister(c.repositories(), c.store), Gate: gate}
		if command == "close" {
			closed, err := merger.Close(ctx, ws, folder.Path, args[3])
			if err != nil {
				return true, err
			}
			return true, encoder.Encode(closed)
		}
		result, err := merger.Merge(ctx, ws, folder.Path, args[3])
		if result.Status != "" {
			// The gate's runs and notes say why nothing merged.
			if encodeErr := encoder.Encode(result); encodeErr != nil && err == nil {
				err = encodeErr
			}
		}
		return true, err
	}
	repository, changeList, err := changeLister(c.repositories(), c.store).List(ctx, ws, folder.Path)
	if err != nil {
		return true, err
	}
	return true, encoder.Encode(struct {
		Repository changes.Repository `json:"repository"`
		Changes    []changes.Change   `json:"changes"`
	}{repository, changeList})
}
