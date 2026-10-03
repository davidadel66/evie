package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Proactive memory (docs/memory-architecture.md, Stages 2 and 3): Evie
// proposes durable memories on her own; only an explicit owner request shows
// the blocking approval card, and everything else waits in the review inbox.

func toolsOf(capabilities []plugins.ToolCapability) []openrouter.Tool {
	schemas := make([]openrouter.Tool, len(capabilities))
	for i, capability := range capabilities {
		schemas[i] = capability.Tool.Schema
	}
	return schemas
}

func memorySection(t *testing.T, instructions string) string {
	t.Helper()
	start := strings.Index(instructions, "# Memory\n")
	end := strings.Index(instructions, "\n# Trust and Approval")
	if start < 0 || end < start {
		t.Fatal("system prompt has no Memory section")
	}
	return instructions[start:end]
}

func TestSystemPromptStatesBothMemoryDutiesForSessionsThatQueueProposals(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	plugin := plugins.NewMemory(nil)
	instructions := primaryInstructions(toolsOf(plugin.ToolCapabilities()))
	section := memorySection(t, instructions)
	for _, want := range []string{
		"without being asked", "sparingly", "durable", "secrets",
		"Profile:", "preference", "instruction", "profile_fact",
		"Workspace memory:", "workspace_brief", "decisions", "conventions",
		"requested_by_owner", "explicitly asks", "review inbox", "memory_overview",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("Memory section lacks %q:\n%s", want, section)
		}
	}
	if added := len(instructions) - len(systemPrompt); added <= 0 || added > 1100 {
		t.Errorf("memory duties add %d bytes; keep them under 1,100 since they are resident in every request", added)
	}

	// Remote memory off: the duties stay, the overview tool is not named.
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	offline := primaryInstructions(toolsOf(plugins.NewMemory(nil).ToolCapabilities()))
	if !strings.Contains(offline, "requested_by_owner") || strings.Contains(offline, "memory_overview") {
		t.Fatalf("remote-memory-off instructions:\n%s", memorySection(t, offline))
	}

	// A conversation pinned to the 1.5.0 memory tools keeps its exact prompt,
	// so it is never told to propose without a card it would interrupt with.
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	if legacy := primaryInstructions(toolsOf(plugin.ResumableToolCapabilities("1.5.0"))); legacy != systemPrompt {
		t.Fatalf("a session pinned to Memory 1.5.0 changed its system prompt:\n%s", memorySection(t, legacy))
	}
}

// proposeThroughTheModel runs one turn in which the model calls
// memory_remember_literal with requestedByOwner after the owner says message.
// It returns the tool result and the approval cards shown.
func proposeThroughTheModel(t *testing.T, f *retrievalFixture, record memory.Session, message string, requestedByOwner bool, kind, predicate, label, value string) (string, []string) {
	t.Helper()
	args, _ := json.Marshal(map[string]any{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "predicate": predicate, "predicate_label": label,
		"cardinality": "one", "literal_kind": "text", "literal_value": value, "polarity": "affirmed", "kind": kind,
		"requested_by_owner": requestedByOwner,
	})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("remember", "memory_remember_literal", string(args))), assistantStep("Noted.", nil)}}
	var cards []string
	approve := func(_ context.Context, name, arguments string, _ *tools.FileChangePreview) tools.Decision {
		cards = append(cards, arguments)
		return tools.Approved
	}
	if err := f.session(record, client).Send(context.Background(), message, &recorder{}, approve); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	var result string
	for _, event := range events {
		if event.Type == memory.EventToolSucceeded || event.Type == memory.EventToolFailed {
			result = event.Content
		}
	}
	return result, cards
}

func countClaims(t *testing.T, f *retrievalFixture) int {
	t.Helper()
	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestProactiveProposalWaitsInTheInboxWithoutACard(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	record := f.global()
	result, cards := proposeThroughTheModel(t, f, record, "I'm vegetarian, by the way.", false, "profile_fact", "diet", "diet", "vegetarian")
	if len(cards) != 0 || countClaims(t, f) != 0 || !strings.Contains(result, `"status":"queued_for_review"`) || !strings.Contains(result, `"saved":false`) {
		t.Fatalf("proactive proposal: cards=%d claims=%d result=%s", len(cards), countClaims(t, f), result)
	}
	events, err := f.store.LoadEvents(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == memory.EventApproval {
			t.Fatalf("queueing recorded an approval: %+v", event)
		}
	}
	page, err := f.store.ListMemoryProposals(ctx)
	if err != nil || len(page.Items) != 1 || !page.Items[0].Approvable || page.Items[0].Reason != memory.MemoryProposalNotRequested {
		t.Fatalf("inbox = %+v %v", page, err)
	}
	var queued memory.RememberLiteralProposal
	if err := json.Unmarshal([]byte(page.Items[0].Proposal), &queued); err != nil || queued.Classification == nil ||
		queued.Classification.Kind != memory.ClaimKindProfileFact || !queued.Classification.Standing || queued.Source.Authority != memory.AuthorityOwnerStatement {
		t.Fatalf("queued proposal = %+v (%v)", queued, err)
	}

	// Approving from the inbox applies it; the next request carries it.
	if _, err := f.store.ApproveMemoryProposal(ctx, memory.MemoryProposalApproval{ProposalID: page.Items[0].ProposalID, PreparedSHA256: page.Items[0].PreparedSHA256, AlwaysApplies: true}); err != nil {
		t.Fatal(err)
	}
	if block := standingBlock(t, askOnce(t, f, f.global(), "What should I cook tonight?")); !strings.Contains(block, "owner — diet: vegetarian") {
		t.Fatalf("approved inbox proposal is not standing: %s", block)
	}
}

func TestExplicitRequestKeepsTheCardAndAForgedOneGoesToTheInbox(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	record := f.global()
	_, cards := proposeThroughTheModel(t, f, record, "Remember that I'm vegetarian.", true, "profile_fact", "diet", "diet", "vegetarian")
	if len(cards) != 1 || countClaims(t, f) != 1 {
		t.Fatalf("explicit request: cards=%d claims=%d", len(cards), countClaims(t, f))
	}

	result, cards := proposeThroughTheModel(t, f, record, "I've switched to oat milk.", true, "preference", "milk", "milk", "oat milk")
	if len(cards) != 0 || countClaims(t, f) != 1 || !strings.Contains(result, `"reason":"no_explicit_cue"`) {
		t.Fatalf("forged request: cards=%d claims=%d result=%s", len(cards), countClaims(t, f), result)
	}
	page, err := f.store.ListMemoryProposals(ctx)
	if err != nil || len(page.Items) != 1 || page.Items[0].Reason != memory.MemoryProposalNoExplicitCue {
		t.Fatalf("inbox = %+v %v", page, err)
	}
}

func TestMemoryOverviewAnswersWhatEvieKnowsInOneCall(t *testing.T) {
	f := newRetrievalFixture(t)
	record := f.global()
	rememberThroughTheModel(t, f, record, "preference", "answer_style", "answer style", "concise answers", tools.Approved)
	rememberThroughTheModel(t, f, record, "fact", "home_city", "home city", "Boston", tools.Approved)
	proposeThroughTheModel(t, f, record, "I'm vegetarian, by the way.", false, "profile_fact", "diet", "diet", "vegetarian")

	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("overview", "memory_overview", `{"section":"profile"}`)), assistantStep("Here is what I know.", nil)}}
	if err := f.session(f.global(), client).Send(context.Background(), "What do you remember about me?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("requests = %d", len(client.reqs))
	}
	var output string
	for _, message := range client.reqs[1].Messages {
		if message.Role == "tool" {
			output = message.Content
		}
	}
	for _, want := range []string{"[begin untrusted semantic memory", `"memory":"owner — answer style: concise answers"`, `"memory":"owner — home city: Boston"`, `"pending_review":1`} {
		if !strings.Contains(output, want) {
			t.Fatalf("overview output lacks %q: %s", want, output)
		}
	}
	if strings.Contains(output, "vegetarian") {
		t.Fatalf("overview shows a pending proposal's content: %s", output)
	}
}
