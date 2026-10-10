package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// proposingSupervisor answers ProposeMerge with a gate that passed; every
// other method is left to the embedded nil interface and never called.
type proposingSupervisor struct {
	ResearchSupervisor
	merged bool
}

func (p *proposingSupervisor) ProposeMerge(context.Context, delegation.Parent, string, string) (delegation.MergeProposal, error) {
	return delegation.MergeProposal{ExecutionID: "e1", Status: "ready", Branch: "evie/1a2b3c4d-fix-x", Target: "main", Repo: "/Users/me/code/notes",
		FilesChanged: 2, Insertions: 11, Deletions: 4}, nil
}

func (p *proposingSupervisor) Merge(context.Context, delegation.Parent, delegation.MergeProposal) (delegation.MergeProposal, error) {
	p.merged = true
	return delegation.MergeProposal{}, nil
}

// onGitHub says which folders hold a GitHub Repository.
type onGitHub map[string]bool

func (o onGitHub) OnGitHub(_ context.Context, folder string) (bool, error) { return o[folder], nil }

// changes.spec.md story 4a (Q19, Q25): in a GitHub Repository a passed
// gate leaves a draft Change for the owner to push, with no chat merge
// card; a local Repository keeps today's card until story 6.
func TestProposeBuildMergeInAGitHubRepositoryLeavesADraftWithoutACard(t *testing.T) {
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: memory.ScopeContext{WorkspaceID: "w"}})
	args := `{"execution_id":"e1"}`

	supervisor := &proposingSupervisor{}
	github := NewSubagents(supervisor).WithGitHubRepositories(onGitHub{"/Users/me/code/notes": true})
	prepared, err := github.prepareMerge(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Review == nil || prepared.Preview != nil || prepared.Execute != nil {
		t.Fatalf("GitHub prepared = %+v", prepared)
	}
	result, err := prepared.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`"status":"draft"`, "evie/1a2b3c4d-fix-x", "the owner", "evie changes push w evie/1a2b3c4d-fix-x"} {
		if !strings.Contains(result, text) {
			t.Fatalf("result %s lacks %q", result, text)
		}
	}
	if strings.Contains(result, "David") {
		t.Fatalf("result %s names the owner", result)
	}
	if supervisor.merged {
		t.Fatal("a GitHub Repository merged locally")
	}

	for name, s := range map[string]Subagents{
		"local":     NewSubagents(supervisor).WithGitHubRepositories(onGitHub{}),
		"not wired": NewSubagents(supervisor),
	} {
		prepared, err := s.prepareMerge(ctx, args)
		if err != nil || prepared.Preview == nil || prepared.Execute == nil || prepared.Review != nil {
			t.Fatalf("%s prepared = %+v, %v", name, prepared, err)
		}
	}
}
