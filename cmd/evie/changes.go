package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

const changesUsage = "usage: evie changes list <workspace-id>"

// changeLister lists a Workspace's Changes for `evie changes` and the web
// routes: Evie's build branches, read from the build records (story 3a),
// joined with the pull requests the last sync recorded (story 3b).
func changeLister(repositories *changes.Service, store *eviedb.Store) *changes.Lister {
	return &changes.Lister{Repositories: repositories, Builds: changes.NewBuildSource(store), PullRequests: changes.PullRequestSource{Store: store}}
}

// runChangeManagement answers `evie changes list <workspace-id>`, which
// prints the Workspace's Repository and its Changes as the Changes tab
// lists them. repositories makes the Repository reader only when the
// command is this one.
func runChangeManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store, repositories func() *changes.Service) (bool, error) {
	if len(args) == 0 || args[0] != "changes" {
		return false, nil
	}
	if len(args) != 3 || args[1] != "list" || args[2] == "" {
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
	repository, list, err := changeLister(repositories(), store).List(ctx, ws, folder.Path)
	if err != nil {
		return true, err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return true, encoder.Encode(struct {
		Repository changes.Repository `json:"repository"`
		Changes    []changes.Change   `json:"changes"`
	}{repository, list})
}
