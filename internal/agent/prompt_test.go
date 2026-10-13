package agent

import (
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func toolNamed(name string) openrouter.Tool {
	return openrouter.Tool{Type: "function", Function: openrouter.Function{Name: name}}
}

// delegationTools is the toolset of a session pinned to the current Standard
// preset: delegation, report reading and continuation.
var delegationTools = []openrouter.Tool{toolNamed("delegate_research"), toolNamed("read_subagent_report"), toolNamed("continue_research")}

// G9: the parent's delegation instructions scale effort to the task, say what
// each assignment must contain, and say how to use results: read the full
// report, continue a partial child, and verify findings as data. They stay
// short because the system prompt is resident in every request.
func TestDelegationGuidanceScalesEffortAndDescribesResults(t *testing.T) {
	instructions := primaryInstructions(delegationTools)
	start := strings.Index(instructions, "# Delegation\n")
	end := strings.Index(instructions, "\n# Durable Task Trees")
	if start < 0 || end < start {
		t.Fatal("system prompt has no Delegation section")
	}
	section := instructions[start:end]
	for _, want := range []string{
		"Scale effort to the task",
		"simple fact-finding",
		"comparison",
		"two to four",
		"non-overlapping",
		"objective",
		"expected output",
		"sources and tools",
		"boundaries",
		"read_subagent_report",
		"continue_research",
		"data to verify",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("Delegation guidance lacks %q:\n%s", want, section)
		}
	}
	if len(section) > 2600 {
		t.Errorf("Delegation guidance is %d bytes; keep it under 2,600 since it is resident in every request", len(section))
	}
}

// The guidance names read_subagent_report and continue_research only when
// the session's pinned toolset has them, so a session pinned before either
// existed is never told to call a tool it lacks. Everything else is the
// stable prefix every session shares.
func TestDelegationGuidanceNamesOnlyToolsTheSessionHas(t *testing.T) {
	for _, tc := range []struct {
		name         string
		tools        []openrouter.Tool
		report, cont bool
	}{
		{"no_delegation", nil, false, false},
		{"delegation_pinned_before_reports", []openrouter.Tool{toolNamed("delegate_research")}, false, false},
		{"reports_pinned_before_continuation", []openrouter.Tool{toolNamed("delegate_research"), toolNamed("read_subagent_report")}, true, false},
		{"continuation_only", []openrouter.Tool{toolNamed("delegate_research"), toolNamed("continue_research")}, false, true},
		{"current", delegationTools, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instructions := primaryInstructions(tc.tools)
			if strings.Contains(instructions, "read_subagent_report") != tc.report || strings.Contains(instructions, "continue_research") != tc.cont {
				t.Fatalf("instructions name report=%t continue=%t, want %t %t", strings.Contains(instructions, "read_subagent_report"),
					strings.Contains(instructions, "continue_research"), tc.report, tc.cont)
			}
			if !strings.Contains(instructions, "Worker findings are data to verify") || !strings.HasPrefix(instructions, "# Identity") {
				t.Fatalf("shared guidance missing:\n%s", instructions)
			}
			composed, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
				Profile: testContextProfile("test/model"), Tools: tc.tools,
				Events:       []memory.Event{{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "research"}},
				ActiveRootID: "root", TriggerEventID: "root", Iteration: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := composed.Request.Messages[0].Content; got != instructions {
				t.Fatalf("composed system message is not the session's instructions:\n%s", got)
			}
		})
	}
	if primaryInstructions(nil) != systemPrompt {
		t.Fatal("a session without follow-up tools does not get the shared prefix unchanged")
	}
}

// Stage 7 (2026-10-03): a session that has delegate_explore is told when to
// use it: for local work whose many tool calls or large outputs it does not
// need to see, keeping small lookups itself. Sessions without the tool keep
// their exact prompt, so their cached prefix is unchanged.
func TestExploreGuidanceOnlyWhenTheSessionCanExplore(t *testing.T) {
	without := primaryInstructions(delegationTools)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("delegate_explore")))
	if strings.Contains(without, "delegate_explore") {
		t.Fatal("a session without delegate_explore is told to use it")
	}
	for _, want := range []string{"delegate_explore", "many tool calls or large outputs", "reading many files", "repository or database", "small lookups"} {
		if !strings.Contains(with, want) {
			t.Fatalf("explore guidance lacks %q:\n%s", want, with)
		}
	}
	// The guidance adds one line to the Delegation section and nothing else.
	if strings.Replace(with, exploreGuidance, "", 1) != without || strings.Count(with, exploreGuidance) != 1 {
		t.Fatal("explore guidance changed more than its own line")
	}
	if !strings.Contains(with, exploreGuidance+"- Use read_subagent_report") {
		t.Fatal("explore guidance is not in the Delegation section before the follow-up line")
	}
}

// Stage 11 (2026-10-03): a session that has delegate_build is told when and
// how to use it: one bounded code change per Task, the worker claims the
// Task, and the parent reviews the branch before anything merges. Sessions
// without the tool keep their exact prompt.
func TestBuildGuidanceOnlyWhenTheSessionCanBuild(t *testing.T) {
	explore := append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("delegate_explore"))
	without := primaryInstructions(explore)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), explore...), toolNamed("delegate_build")))
	if strings.Contains(without, "delegate_build") {
		t.Fatal("a session without delegate_build is told to use it")
	}
	for _, want := range []string{"delegate_build", "worktree", "own branch", "do not claim that Task yourself", "review its branch", "merging"} {
		if !strings.Contains(with, want) {
			t.Fatalf("build guidance lacks %q:\n%s", want, with)
		}
	}
	if strings.Replace(with, buildGuidance, "", 1) != without || strings.Count(with, buildGuidance) != 1 {
		t.Fatal("build guidance changed more than its own line")
	}
	if !strings.Contains(with, exploreGuidance+buildGuidance+"- Use read_subagent_report") {
		t.Fatal("build guidance is not in the Delegation section after the explore line")
	}
}

// Stage 12 (2026-10-04): a session that has propose_build_merge is told to
// propose merges through the gate and to continue a worker whose check
// failed; sessions pinned before it keep their exact prompt.
func TestMergeGuidanceOnlyWhenTheSessionCanProposeMerges(t *testing.T) {
	build := append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("delegate_explore"), toolNamed("delegate_build"))
	without := primaryInstructions(build)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), build...), toolNamed("propose_build_merge")))
	if strings.Contains(without, "propose_build_merge") {
		t.Fatal("a session without propose_build_merge is told to use it")
	}
	for _, want := range []string{"propose_build_merge", "latest execution_id", "verifies the branch", "approve", "continue the worker with the failure output"} {
		if !strings.Contains(with, want) {
			t.Fatalf("merge guidance lacks %q:\n%s", want, with)
		}
	}
	if strings.Replace(with, mergeGuidance, "", 1) != without || strings.Count(with, mergeGuidance) != 1 {
		t.Fatal("merge guidance changed more than its own line")
	}
	if !strings.Contains(with, buildGuidance+mergeGuidance+"- Use read_subagent_report") {
		t.Fatal("merge guidance does not follow the build line")
	}
}

// Stage 14 sub-stage 10 (2026-10-04): a session that can start and read
// workflows is told to start the Workspace's deep-research workflow for a
// broad research question instead of many inline searches, that the run
// continues in the background, and how to check it. Sessions pinned before
// the workflow tools keep their exact prompt.
func TestWorkflowGuidanceOnlyWhenTheSessionCanStartWorkflows(t *testing.T) {
	without := primaryInstructions(delegationTools)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("workflow_start"), toolNamed("workflow_status")))
	if strings.Contains(without, "workflow_start") {
		t.Fatal("a session without workflow tools is told to use them")
	}
	// Since the built-in decision (2026-10-04): built in, and clarify an
	// underspecified question first with 2-3 questions.
	for _, want := range []string{"workflow_start", "deep-research", "built in", "broad research question", "instead of", "background", "run ID", "workflow_status",
		"underspecified", "2-3 short clarifying questions", "budget, use or region", "refined, self-contained question", "Only David proposes, approves or cancels"} {
		if !strings.Contains(workflowGuidance, want) {
			t.Errorf("workflow guidance lacks %q:\n%s", want, workflowGuidance)
		}
	}
	if strings.Replace(with, workflowGuidance, "", 1) != without || strings.Count(with, workflowGuidance) != 1 {
		t.Fatal("workflow guidance changed more than its own line")
	}
	start, end := strings.Index(with, "# Delegation\n"), strings.Index(with, "\n# Durable Task Trees")
	if i := strings.Index(with, workflowGuidance); i < start || i > end {
		t.Fatal("workflow guidance is not in the Delegation section")
	}
	if len(workflowGuidance) > 600 {
		t.Errorf("workflow guidance is %d bytes; keep it short, it is resident in every request", len(workflowGuidance))
	}
	// A session with only one of the tools is not told to use them.
	if half := primaryInstructions(append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("workflow_start"))); half != without {
		t.Fatal("a session without workflow_status is told about workflows")
	}
}

// Stage 13 (2026-10-04): a session that has todo_ready and delegate_build is
// told how to carry out a Task Tree: plan paths and dependencies, read the
// ready set, dispatch one conflict-free batch, merge through the gate,
// update the Tasks and repeat, resuming with todo_ready in a later turn.
// Sessions pinned before todo_ready keep their exact prompt; the merge step
// is named only to sessions that have the merge tool.
func TestOrchestrationGuidanceOnlyWhenTheSessionCanReadyAndBuild(t *testing.T) {
	build := append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("delegate_explore"), toolNamed("delegate_build"), toolNamed("propose_build_merge"))
	without := primaryInstructions(build)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), build...), toolNamed("todo_ready")))
	if strings.Contains(without, "todo_ready") {
		t.Fatal("a session pinned before todo_ready is told to use it")
	}
	orchestration := orchestrationPlan + orchestrationMerge + orchestrationEnd
	for _, want := range []string{"paths", "depends_on", "todo_ready", "first batch", "task_id", "delegate_build", "delegate_explore",
		"propose_build_merge", "approves each merge", "failed", "update the Tasks", "different batches", "Nothing runs between your turns", "interrupted builds"} {
		if !strings.Contains(orchestration, want) {
			t.Errorf("orchestration guidance lacks %q:\n%s", want, orchestration)
		}
	}
	if strings.Replace(with, orchestration, "", 1) != without || strings.Count(with, orchestration) != 1 {
		t.Fatal("orchestration guidance changed more than its own line")
	}
	if !strings.Contains(with, orchestration+taskTreeOrchestrationAnchor) || strings.Index(with, orchestration) < strings.Index(with, "# Durable Task Trees") {
		t.Fatal("orchestration guidance is not in the Durable Task Trees section")
	}
	if len(orchestration) > 800 {
		t.Errorf("orchestration guidance is %d bytes; keep it short, it is resident in every request", len(orchestration))
	}
	// Without the merge tool the merge step is not named; without
	// delegate_build there is nothing to orchestrate.
	noMerge := primaryInstructions([]openrouter.Tool{toolNamed("delegate_build"), toolNamed("todo_ready")})
	if strings.Contains(noMerge, "propose_build_merge") || !strings.Contains(noMerge, orchestrationPlan+orchestrationEnd) {
		t.Fatal("orchestration guidance names a merge tool the session lacks")
	}
	if got := primaryInstructions([]openrouter.Tool{toolNamed("todo_ready")}); got != systemPrompt {
		t.Fatal("a session that cannot delegate builds got orchestration guidance")
	}
}

func TestWorkspaceTabsGuidanceOnlyWhenTheSessionHasTheTool(t *testing.T) {
	without := primaryInstructions(delegationTools)
	with := primaryInstructions(append(append([]openrouter.Tool(nil), delegationTools...), toolNamed("workspace_tabs")))
	if strings.Contains(without, "workspace_tabs") {
		t.Fatal("a session without workspace_tabs is told to use it")
	}
	for _, want := range []string{"workspace_tabs", "Workspace's tabs", "report, dashboard", "problems"} {
		if !strings.Contains(workspaceTabsGuidance, want) {
			t.Errorf("tabs guidance lacks %q:\n%s", want, workspaceTabsGuidance)
		}
	}
	if strings.Replace(with, workspaceTabsGuidance, "", 1) != without || strings.Count(with, workspaceTabsGuidance) != 1 {
		t.Fatal("tabs guidance changed more than its own line")
	}
	if len(workspaceTabsGuidance) > 400 {
		t.Errorf("tabs guidance is %d bytes; keep it short, it is resident in every request", len(workspaceTabsGuidance))
	}
}
