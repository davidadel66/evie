package plugins

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// proposingSupervisor answers ProposeMerge with a gate that passed; every
// other method is left to the embedded nil interface and never called.
type proposingSupervisor struct {
	ResearchSupervisor
	merged   bool
	proposed []delegation.ChangeText
}

func (p *proposingSupervisor) ProposeMerge(_ context.Context, _ delegation.Parent, _, _ string, text delegation.ChangeText) (delegation.MergeProposal, error) {
	p.proposed = append(p.proposed, text)
	return delegation.MergeProposal{ExecutionID: "e1", Status: "ready", Branch: "evie/1a2b3c4d-fix-x", Target: "main", Repo: "/Users/me/code/notes",
		FilesChanged: 2, Insertions: 11, Deletions: 4}, nil
}

func (*proposingSupervisor) Start(context.Context) error { return nil }
func (*proposingSupervisor) Stop(context.Context) error  { return nil }

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
	args := `{"execution_id":"e1","title":"Fix the footer","description":"The footer overlapped the page."}`

	supervisor := &proposingSupervisor{}
	github := NewSubagents(supervisor).WithGitHubRepositories(onGitHub{"/Users/me/code/notes": true})
	prepared, err := github.prepareMerge(ctx, args, false)
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

	// Story 6b: a local Repository's Change waits in the Workspace's Changes
	// tab too; only a project chat, which has no Changes tab, gets the card.
	project := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: memory.ScopeContext{ProjectID: "p"}})
	for name, s := range map[string]Subagents{
		"local":     NewSubagents(supervisor).WithGitHubRepositories(onGitHub{}),
		"not wired": NewSubagents(supervisor),
	} {
		prepared, err := s.prepareMerge(ctx, args, false)
		if err != nil || prepared.Review == nil || prepared.Preview != nil || prepared.Execute != nil {
			t.Fatalf("%s in a Workspace prepared = %+v, %v", name, prepared, err)
		}
		result, err := prepared.Review(ctx)
		if err != nil || !strings.Contains(result, `"status":"ready"`) || !strings.Contains(result, "Changes tab") || strings.Contains(result, "David") {
			t.Fatalf("%s result = %s, %v", name, result, err)
		}
		if prepared, err := s.prepareMerge(project, args, false); err != nil || prepared.Preview == nil || prepared.Execute == nil || prepared.Review != nil {
			t.Fatalf("%s in a project prepared = %+v, %v", name, prepared, err)
		}
	}
	if supervisor.merged {
		t.Fatal("a Workspace's Change merged from the chat")
	}
}

// changes.spec.md story 4a (2026-10-10): propose_build_merge takes the
// Change's title and description, which become its pull request's, and
// requires both before the gate runs.
func TestProposeBuildMergeTakesTheChangesTitleAndDescription(t *testing.T) {
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: memory.ScopeContext{WorkspaceID: "w"}})
	supervisor := &proposingSupervisor{}
	s := NewSubagents(supervisor)
	if _, err := s.prepareMerge(ctx, `{"execution_id":"e1","title":"Fix the footer","description":"The footer overlapped the page."}`, false); err != nil {
		t.Fatal(err)
	}
	if want := (delegation.ChangeText{Title: "Fix the footer", Description: "The footer overlapped the page."}); len(supervisor.proposed) != 1 || supervisor.proposed[0] != want {
		t.Fatalf("proposed %+v", supervisor.proposed)
	}
	for _, args := range []string{`{"execution_id":"e1"}`, `{"execution_id":"e1","title":"Fix the footer"}`, `{"execution_id":"e1","title":" ","description":"The footer overlapped."}`} {
		if _, err := s.prepareMerge(ctx, args, false); err == nil || !strings.Contains(err.Error(), "title and description are required") {
			t.Fatalf("%s = %v", args, err)
		}
	}
	if len(supervisor.proposed) != 1 {
		t.Fatalf("an incomplete proposal reached the gate: %+v", supervisor.proposed)
	}
	merge := s.mergeCapability("1.2.0")
	if merge.ContractVersion != "1.2.0" || !strings.Contains(string(mustJSON(t, merge.Tool.Schema.Function.Parameters)), `"required":["execution_id","title","description"]`) {
		t.Fatalf("propose_build_merge = %s %s", merge.ContractVersion, mustJSON(t, merge.Tool.Schema.Function.Parameters))
	}
}

// Chats pinned to Subagents 1.0.0 (five on the owner's machine on
// 2026-10-10, schema 2737678c…) reopen with exactly their propose_build_merge
// and propose without text, which the gate accepts; the Change then falls
// back to the worker's own words.
func TestChatsPinnedBeforeTheChangeTextReopenWithTheirMergeTool(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	const pinned = "2737678c19e298dccc2648b1bf2a4779dadb851f023d51a51ceaa76c86e6518d"
	legacy := Subagents{}.mergeCapability("1.0.0")
	if got := schemaHash(legacy.Tool.Schema); got != pinned || legacy.ContractVersion != "1.0.0" {
		t.Fatalf("frozen propose_build_merge = %s %s, want %s 1.0.0", got, legacy.ContractVersion, pinned)
	}
	// Two chats on the owner's machine pinned 1.1.0 (fb61acb5…) on
	// 2026-10-10, before story 6b changed what the description says.
	if got := schemaHash(Subagents{}.mergeCapability("1.1.0").Tool.Schema); got != "fb61acb52c7efae6e8cb7dfa2468fd46bcefdfc4d25a0481b9bb0bcec32d4090" {
		t.Fatalf("frozen 1.1.0 propose_build_merge = %s", got)
	}
	supervisor := &proposingSupervisor{}
	manager := func(subagents Plugin) *Manager {
		t.Helper()
		m, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}), NewMemory(&stubSemanticKernel{}), subagents)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID, MemoryPluginID, SubagentsPluginID} {
			if err := m.Enable(context.Background(), id); err != nil {
				t.Fatal(err)
			}
		}
		return m
	}
	before, err := manager(subagentsBeforeChangeText{NewSubagents(supervisor)}).ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := manager(NewSubagents(supervisor)).ResumeComposition(before.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, before.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), before.Toolset.Schemas()) {
		t.Fatal("reopening changed the pinned receipt or its tools")
	}
	if len(resumed.CompatibilityResolutions) != 1 || resumed.CompatibilityResolutions[0].OriginalProvider.ImplementationVersion != "1.0.0" ||
		resumed.CompatibilityResolutions[0].ReplacementImplementationVersion != "1.2.0" {
		t.Fatalf("compatibility = %+v", resumed.CompatibilityResolutions)
	}
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: memory.ScopeContext{WorkspaceID: "w"}})
	call := openrouter.ToolCall{ID: "c1", Type: "function", Function: openrouter.FunctionCall{Name: delegation.MergeToolName, Arguments: `{"execution_id":"e1"}`}}
	declined := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Declined }
	if _, _, err := resumed.Toolset.ExecuteWithApprovalAuthorizedCompletion(ctx, call, declined, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(supervisor.proposed) != 1 || supervisor.proposed[0] != (delegation.ChangeText{}) {
		t.Fatalf("a pinned chat proposed %+v", supervisor.proposed)
	}

	current, err := manager(NewSubagents(supervisor)).ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range current.Receipt.Capabilities {
		if c.ID == string(SubagentsMergeCapabilityID) && (c.ContractVersion != "1.2.0" || c.SchemaSHA256 == pinned) {
			t.Fatalf("a new chat's propose_build_merge = %+v", c)
		}
	}
}

// subagentsBeforeChangeText is the Subagents Plugin as released before
// story 4a's Change text: version 1.0.0.
type subagentsBeforeChangeText struct{ current Subagents }

func (p subagentsBeforeChangeText) Start(ctx context.Context) error { return p.current.Start(ctx) }
func (p subagentsBeforeChangeText) Stop(ctx context.Context) error  { return p.current.Stop(ctx) }
func (subagentsBeforeChangeText) Manifest() Manifest {
	m := Subagents{}.Manifest()
	m.ImplementationVersion, m.ResumableFrom = "1.0.0", nil
	m.Capabilities = append([]CapabilityContract(nil), m.Capabilities...)
	for i := range m.Capabilities {
		m.Capabilities[i].Version = "1.0.0"
	}
	return m
}
func (p subagentsBeforeChangeText) ToolCapabilities() []ToolCapability {
	return p.current.ResumableToolCapabilities("1.0.0")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
