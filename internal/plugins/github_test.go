package plugins

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
)

// fakeOpener records what github.open_change asks and answers as told.
type fakeOpener struct {
	opened []string
	text   []delegation.ChangeText
	found  *OpenedChange
	err    error
}

func (f *fakeOpener) OpenChange(_ context.Context, ws memory.WorkspaceID, resource, execution string, text delegation.ChangeText) (OpenedChange, error) {
	f.opened = append(f.opened, string(ws)+" "+resource+" "+execution)
	f.text = append(f.text, text)
	if f.err != nil {
		return OpenedChange{}, f.err
	}
	return OpenedChange{Branch: "evie/1a2b3c4d-fix-footer", PullRequest: 7, URL: "https://github.com/davidadel66/notes/pull/7"}, nil
}

func (f *fakeOpener) FindChange(_ context.Context, ws memory.WorkspaceID, resource, execution string) (*OpenedChange, error) {
	f.opened = append(f.opened, "find "+string(ws)+" "+resource+" "+execution)
	return f.found, f.err
}

func openChangeRequest(input map[string]any) WorkflowEffectRequest {
	return WorkflowEffectRequest{Connection: GitHubConnectionID, Resource: "repo:davidadel66/notes", Input: input, IdempotencyKey: "sha256:k", WorkspaceID: "w", RunID: "run-7"}
}

// changes.spec.md story 4b: github.open_change pushes one of Evie's draft
// Changes in the run's Workspace Repository and opens its pull request
// with the text the workflow gives; anything it refuses is proven not to
// have happened.
func TestGitHubOpenChangeOpensTheRunsWorkspaceChange(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManager(tools.NewToolset(nil), NewGitHub(nil))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := manager.WorkflowContract(string(GitHubOpenChangeCapabilityID))
	if !ok || c.ID != string(GitHubOpenChangeCapabilityID) || !c.Effect || !c.Idempotent || !c.Reconciles || !reflect.DeepEqual(c.Connections, []string{GitHubConnectionID}) || len(c.Resources) != 0 || c.ResourcePrefix != "repo:" {
		t.Fatalf("contract = %+v", c)
	}

	opener := &fakeOpener{}
	effect := NewGitHub(opener).WorkflowEffects()[0]
	input := map[string]any{"execution_id": "e1", "title": "Move the footer below the page", "description": "The footer overlapped the page."}
	out, err := effect.Perform(ctx, openChangeRequest(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"branch": "evie/1a2b3c4d-fix-footer", "pull_request": int64(7), "url": "https://github.com/davidadel66/notes/pull/7", "existing": false}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("output = %#v", out)
	}
	if len(opener.opened) != 1 || opener.opened[0] != "w repo:davidadel66/notes e1" ||
		opener.text[0] != (delegation.ChangeText{Title: "Move the footer below the page", Description: "The footer overlapped the page."}) {
		t.Fatalf("opened %v %+v", opener.opened, opener.text)
	}

	// The opener's own errors pass through: it alone knows whether GitHub
	// may have opened the pull request.
	opener.err = errors.New("GitHub did not answer")
	if _, err := effect.Perform(ctx, openChangeRequest(input)); err == nil || errors.Is(err, ErrWorkflowEffectNotPerformed) {
		t.Fatalf("an uncertain open = %v", err)
	}
	opener.err = nil

	for name, req := range map[string]WorkflowEffectRequest{
		"another connection": func() WorkflowEffectRequest { r := openChangeRequest(input); r.Connection = "gitlab"; return r }(),
		"no Workspace":       func() WorkflowEffectRequest { r := openChangeRequest(input); r.WorkspaceID = ""; return r }(),
		"an unknown field":   openChangeRequest(map[string]any{"execution_id": "e1", "title": "t", "description": "d", "base": "release"}),
		"no execution":       openChangeRequest(map[string]any{"execution_id": " ", "title": "t", "description": "d"}),
	} {
		if _, err := effect.Perform(ctx, req); !errors.Is(err, ErrWorkflowEffectNotPerformed) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := NewGitHub(nil).WorkflowEffects()[0].Perform(ctx, openChangeRequest(input)); !errors.Is(err, ErrWorkflowEffectNotPerformed) || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("without GitHub: %v", err)
	}
	if len(opener.opened) != 2 {
		t.Fatalf("refused requests reached the opener: %v", opener.opened)
	}

	// Reconciling finds the branch's pull request, or proves there is none.
	opener.found = &OpenedChange{Branch: "evie/1a2b3c4d-fix-footer", PullRequest: 7, URL: "https://github.com/davidadel66/notes/pull/7", Existing: true}
	lookup, err := effect.Reconcile(ctx, openChangeRequest(input))
	if err != nil || lookup.Outcome != WorkflowEffectHappened || !strings.Contains(lookup.Evidence, "pull request #7") ||
		!reflect.DeepEqual(lookup.Output, map[string]any{"branch": "evie/1a2b3c4d-fix-footer", "pull_request": int64(7), "url": "https://github.com/davidadel66/notes/pull/7", "existing": true}) {
		t.Fatalf("found = %+v, %v", lookup, err)
	}
	opener.found = nil
	if lookup, err := effect.Reconcile(ctx, openChangeRequest(input)); err != nil || lookup.Outcome != WorkflowEffectDidNotHappen {
		t.Fatalf("none = %+v, %v", lookup, err)
	}
	opener.err = errors.New("GitHub is down")
	if _, err := effect.Reconcile(ctx, openChangeRequest(input)); err == nil {
		t.Fatal("an unanswered lookup settled the intent")
	}
}
