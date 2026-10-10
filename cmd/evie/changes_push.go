package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// pushVerifyTimeout bounds Verify before a push, as the merge gate bounds it.
const pushVerifyTimeout = 20 * time.Minute

// changePusher pushes Evie's draft Changes and opens their pull requests
// for `evie changes push` and the push route (changes.spec.md story 4a).
// Without a token a push answers that GitHub is not connected. syncer
// records the new pull request at once; nil makes one for this push.
func changePusher(store *eviedb.Store, repositories *changes.Service, token string, syncer *changes.Syncer) *changes.Pusher {
	p := &changes.Pusher{Lister: changeLister(repositories, store), Service: repositories, Verifier: storeVerifier{store: store}}
	if token == "" {
		return p
	}
	client := github.New(github.API, token, nil)
	p.GitHub = client
	if syncer == nil {
		syncer = changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store})
	}
	p.Syncer = syncer
	return p
}

// storeVerifier runs the Workspace's Verify command on a build branch's
// worktree before a push, sandboxed without network as the merge gate runs
// it.
type storeVerifier struct{ store *eviedb.Store }

func (v storeVerifier) Verify(ctx context.Context, ws memory.WorkspaceID, c changes.Change) (*changes.VerifyOutcome, error) {
	commands, err := v.store.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: ws})
	if err != nil {
		return nil, err
	}
	if commands.Verify == "" {
		return nil, nil
	}
	builds, err := v.store.WorkspaceBuilds(ctx, ws)
	if err != nil {
		return nil, err
	}
	for _, a := range builds {
		if a.ID != c.ExecutionID || a.Worktree == nil {
			continue
		}
		w := a.Worktree
		result, err := tools.RunGateCommand(ctx, tools.GateCommand{Fence: tools.BuildFence{Worktree: w.Path, Temp: w.Temp}, Command: commands.Verify,
			Timeout: pushVerifyTimeout, Log: filepath.Join(w.Temp, "evie-push-verify.log")})
		if err != nil {
			return nil, err
		}
		return &changes.VerifyOutcome{Command: commands.Verify, Passed: !result.TimedOut && result.ExitStatus == 0, Output: result.Tail}, nil
	}
	return nil, fmt.Errorf("no build of this Workspace's has execution %q", c.ExecutionID)
}

// githubFolders tells the Subagents plugin which folders' Repositories are
// on GitHub, read from their configured origin without the token: a chat's
// build there becomes a draft Change to push instead of a local merge
// (story 4a).
type githubFolders struct{ repositories *changes.Service }

func (g githubFolders) OnGitHub(ctx context.Context, folder string) (bool, error) {
	repo, err := g.repositories.Repository(ctx, folder)
	return repo.Kind == changes.GitHub, err
}
