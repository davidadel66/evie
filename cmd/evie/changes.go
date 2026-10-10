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

const changesUsage = "usage: evie changes list <workspace-id> | evie changes push <workspace-id> <branch>"

// changeLister lists a Workspace's Changes for `evie changes` and the web
// routes: Evie's build branches, read from the build records (story 3a),
// joined with the pull requests the last sync recorded (story 3b).
func changeLister(repositories *changes.Service, store *eviedb.Store) *changes.Lister {
	return &changes.Lister{Repositories: repositories, Builds: changes.NewBuildSource(store), PullRequests: changes.PullRequestSource{Store: store}}
}

// runChangeManagement answers `evie changes list <workspace-id>`, which
// prints the Workspace's Repository and its Changes as the Changes tab
// lists them, and `evie changes push <workspace-id> <branch>`, which pushes
// one of Evie's draft Changes and opens its pull request (story 4a).
// repositories makes the Repository reader only when the command is this
// one; getenv reads GITHUB_TOKEN.
func runChangeManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store, repositories func() *changes.Service, getenv func(string) string) (bool, error) {
	if len(args) == 0 || args[0] != "changes" {
		return false, nil
	}
	list := len(args) == 3 && args[1] == "list" && args[2] != ""
	push := len(args) == 4 && args[1] == "push" && args[2] != "" && args[3] != ""
	if !list && !push {
		return true, errors.New(changesUsage)
	}
	ws := memory.WorkspaceID(args[2])
	folder, err := store.WorkspaceFolder(ctx, ws)
	if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
		return true, fmt.Errorf("no Workspace %q", args[2])
	}
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if push {
		result, err := changePusher(store, repositories(), strings.TrimSpace(getenv("GITHUB_TOKEN")), nil).Push(ctx, ws, folder.Path, args[3])
		if err != nil {
			return true, err
		}
		return true, encoder.Encode(result)
	}
	repository, changeList, err := changeLister(repositories(), store).List(ctx, ws, folder.Path)
	if err != nil {
		return true, err
	}
	return true, encoder.Encode(struct {
		Repository changes.Repository `json:"repository"`
		Changes    []changes.Change   `json:"changes"`
	}{repository, changeList})
}
