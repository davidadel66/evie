package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
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

// changeMerger merges and closes Changes for the owner (changes.spec.md
// stories 5a and 5b): local ones through the Kernel's merge gate and, with
// a token, GitHub ones through their pull requests. syncer records a merge
// or close done on GitHub at once; nil makes one for this command.
func changeMerger(store *eviedb.Store, repositories *changes.Service, gate changes.BuildGate, token string, syncer *changes.Syncer) *changes.Merger {
	m := &changes.Merger{Lister: changeLister(repositories, store), Gate: gate, Records: store}
	if token == "" {
		return m
	}
	client := github.New(github.API, token, nil)
	m.GitHub = client
	if syncer == nil {
		syncer = changes.NewSyncer(changes.SyncConfig{GitHub: client, Store: store, After: m.AfterSync})
	}
	m.Syncer = syncer
	return m
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

// workflowGitHub is the GitHub Plugin with its github.open_change wired to
// the owner's push (changes.spec.md story 4b): a workflow pushes Evie's
// draft Changes and opens their pull requests in its Workspace's
// Repository. Without GITHUB_TOKEN every request is refused.
func workflowGitHub(store *eviedb.Store, repositories *changes.Service) plugins.GitHub {
	return plugins.NewGitHub(workflowChangeOpener{store: store, pusher: changePusher(store, repositories, strings.TrimSpace(os.Getenv("GITHUB_TOKEN")), nil)})
}

// workflowChangeOpener opens Changes for workflows in the Workspace's
// current folder.
type workflowChangeOpener struct {
	store  *eviedb.Store
	pusher *changes.Pusher
}

func (o workflowChangeOpener) OpenChange(ctx context.Context, ws memory.WorkspaceID, resource, execution string, text delegation.ChangeText) (plugins.OpenedChange, error) {
	folder, err := o.store.WorkspaceFolder(ctx, ws)
	if err != nil {
		return plugins.OpenedChange{}, plugins.EffectNotPerformed(err)
	}
	result, err := o.pusher.PushForWorkflow(ctx, ws, folder.Path, resource, execution, text)
	switch {
	case errors.Is(err, changes.ErrOpenUncertain):
		// GitHub may have opened it: reconciliation finds out.
		return plugins.OpenedChange{}, err
	case err != nil:
		return plugins.OpenedChange{}, plugins.EffectNotPerformed(err)
	}
	return plugins.OpenedChange{Branch: result.Change.Branch, PullRequest: result.PullRequest, URL: result.URL, Existing: result.Existing}, nil
}

func (o workflowChangeOpener) FindChange(ctx context.Context, ws memory.WorkspaceID, resource, execution string) (*plugins.OpenedChange, error) {
	folder, err := o.store.WorkspaceFolder(ctx, ws)
	if err != nil {
		return nil, err
	}
	pull, err := o.pusher.FindPullRequest(ctx, ws, folder.Path, resource, execution)
	if err != nil || pull == nil {
		return nil, err
	}
	return &plugins.OpenedChange{Branch: pull.Head.Ref, PullRequest: pull.Number, URL: pull.HTMLURL, Existing: true}, nil
}
