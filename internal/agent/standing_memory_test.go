package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Standing tier (docs/memory-architecture.md, Stage 1): an approved standing
// preference reaches every request by rule, never by relevance.

const standingBlockOpen = "<standing-memory>"

func standingMessageIndex(messages []openrouter.Message) int {
	for i, message := range messages {
		if message.Role == "user" && strings.HasPrefix(message.Content, standingBlockOpen) {
			return i
		}
	}
	return -1
}

func standingBlock(t *testing.T, request openrouter.ChatRequest) string {
	t.Helper()
	index := standingMessageIndex(request.Messages)
	if index < 0 {
		t.Fatalf("request carries no standing memory: %+v", request.Messages)
	}
	return request.Messages[index].Content
}

// rememberThroughTheModel saves one owner preference the way the model does:
// it proposes the kind, the approval card shows it, and the owner approves.
func rememberThroughTheModel(t *testing.T, f *retrievalFixture, record memory.Session, kind, predicate, label, value string, decision tools.Decision) string {
	t.Helper()
	args, _ := json.Marshal(map[string]any{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "predicate": predicate, "predicate_label": label,
		"cardinality": "one", "literal_kind": "text", "literal_value": value, "polarity": "affirmed", "kind": kind,
		"requested_by_owner": true,
	})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("remember", "memory_remember_literal", string(args))), assistantStep("Saved.", nil)}}
	var card string
	approve := func(_ context.Context, name, arguments string, _ *tools.FileChangePreview) tools.Decision {
		if name == "memory_remember_literal" {
			card = arguments
		}
		return decision
	}
	if err := f.session(record, client).Send(context.Background(), "Remember that my "+label+" is "+value+".", &recorder{}, approve); err != nil {
		t.Fatal(err)
	}
	return card
}

func askOnce(t *testing.T, f *retrievalFixture, record memory.Session, question string) openrouter.ChatRequest {
	t.Helper()
	client := &fakeClient{steps: []step{assistantStep("Paris.", nil)}}
	if err := f.session(record, client).Send(context.Background(), question, &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	return client.reqs[0]
}

func TestStandingPreferenceReachesRequestsThatShareNoWordsWithIt(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	saving := f.global()
	card := rememberThroughTheModel(t, f, saving, "preference", "answer_style", "answer style", "concise answers", tools.Approved)
	var proposal memory.RememberLiteralProposal
	if err := json.Unmarshal([]byte(card), &proposal); err != nil || proposal.Classification == nil ||
		proposal.Classification.Kind != memory.ClaimKindPreference || !proposal.Classification.Standing ||
		proposal.Classification.BytesPerTokenMilli != 3000 {
		t.Fatalf("approval card classification = %s (%v)", card, err)
	}

	// A new conversation whose question shares no word with the preference.
	request := askOnce(t, f, f.global(), "What is the capital of France?")
	block := standingBlock(t, request)
	if !strings.Contains(block, `"applies":"Global","kind":"preference","memory":"owner — answer style: concise answers"`) {
		t.Fatalf("standing block = %s", block)
	}
	index := standingMessageIndex(request.Messages)
	question := slices.IndexFunc(request.Messages, func(m openrouter.Message) bool { return m.Content == "What is the capital of France?" })
	if index < 1 || index > 2 || question <= index || request.Messages[0].Role != "system" {
		t.Fatalf("standing memory is not in the stable prefix (index %d, question %d): %+v", index, question, request.Messages)
	}

	// Another Workspace sees the Global preference too.
	workspace, err := f.store.RegisterWorkspace(ctx, "Elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if other := standingBlock(t, askOnce(t, f, reader, "Draft a packing list.")); other != block {
		t.Fatalf("Workspace session standing block = %s, want %s", other, block)
	}

	// Remote memory egress off: nothing is sent.
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	if offline := askOnce(t, f, f.global(), "What is the capital of Peru?"); standingMessageIndex(offline.Messages) >= 0 {
		t.Fatalf("standing memory left the machine with EVIE_REMOTE_MEMORY off: %+v", offline.Messages)
	}
	t.Setenv("EVIE_REMOTE_MEMORY", "on")

	// The receipt names the standing set without its content.
	events, err := f.store.LoadEvents(ctx, saving.ID)
	if err != nil {
		t.Fatal(err)
	}
	var latest memory.ContextSnapshotPayload
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot {
			latest = memory.ContextSnapshotPayload{}
			if err := json.Unmarshal(event.Payload, &latest); err != nil {
				t.Fatal(err)
			}
		}
	}
	if latest.StandingMemory == nil || len(latest.StandingMemory.ClaimIDs) != 1 || latest.StandingMemory.Bytes <= 0 {
		t.Fatalf("post-approval request snapshot lacks the standing receipt: %+v", latest.StandingMemory)
	}
	if encoded, _ := json.Marshal(latest); strings.Contains(string(encoded), "concise answers") {
		t.Fatal("snapshot manifest contains standing memory content")
	}
}

func TestStandingSwitchedOffOnTheCardIsRecallOnly(t *testing.T) {
	f := newRetrievalFixture(t)
	record := f.global()
	rememberThroughTheModel(t, f, record, "preference", "answer_style", "answer style", "bullet points", tools.ApprovedAlternative)
	rememberThroughTheModel(t, f, record, "fact", "favorite_color", "favorite color", "teal", tools.Approved)
	if request := askOnce(t, f, f.global(), "What is the capital of France?"); standingMessageIndex(request.Messages) >= 0 {
		t.Fatalf("a switched-off preference or a fact became standing: %+v", request.Messages)
	}
}

func TestStandingMemoryIsByteStableAcrossIterationsAndTurns(t *testing.T) {
	f := newRetrievalFixture(t)
	record := f.global()
	rememberThroughTheModel(t, f, record, "instruction", "reply_language", "reply language", "French", tools.Approved)
	ran := false
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("echo-1", "echo", `{}`)),
		assistantStep("first", nil),
		assistantStep("second", nil),
	}}
	session := f.session(record, client, echoTool("echo", false, &ran))
	if err := session.Send(context.Background(), "Look something up.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := session.Send(context.Background(), "And another thing.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 {
		t.Fatalf("requests = %d", len(client.reqs))
	}
	first := standingBlock(t, client.reqs[0])
	for _, request := range client.reqs[1:] {
		if block := standingBlock(t, request); block != first {
			t.Fatalf("standing bytes changed while the standing set did not:\n%s\n%s", first, block)
		}
		if !slices.Equal(encodedMessages(t, request.Messages[:2]), encodedMessages(t, client.reqs[0].Messages[:2])) {
			t.Fatal("the prefix through the standing block changed between requests")
		}
	}
}

func TestStandingMemoryLeadsTheSummaryAndCarriesACacheBreakpoint(t *testing.T) {
	b := &contextEvents{t: t}
	b.user("old-user", "old question")
	b.answer("old-user", "old-answer", "old answer")
	b.user("older-tools", "look it up")
	b.toolGroup("older-tools", "older result")
	b.answer("older-tools", "older-answer", "found it")
	b.user("active-user", "current")
	trigger := b.toolGroup("active-user", "active result")
	standing := memory.RenderStandingMemory([]memory.StandingMemoryItem{
		{ClaimID: "claim-1", ScopeKey: "global", Kind: memory.ClaimKindPreference, Text: "owner — answer style: concise answers"},
	})
	input := ContextComposeInput{
		Profile:                testContextProfile("anthropic/claude-sonnet-4.6"),
		EnvironmentNote:        "Local working folder: \"/work\".",
		RepositoryInstructions: "repository rules",
		StandingMemory:         standing,
		StandingMemoryReceipt:  memory.NewStandingMemoryReceipt(standing, []memory.SemanticID{"claim-1"}),
		Summary:                &ContextSummary{CompactionEventID: "compaction-1", FirstRetainedEventID: "old-user", Content: "prior summary"},
		WorkingContext:         "<task-focus-data>\n- id=task-1 revision=1\n</task-focus-data>\n",
		Events:                 b.events, ActiveRootID: "active-user", TriggerEventID: trigger, Iteration: 2,
	}
	composer := NewContextComposer(CanonicalRequestEstimator{})
	result, err := composer.Compose(input)
	if err != nil {
		t.Fatal(err)
	}
	messages := result.Request.Messages
	// system, environment, repository, standing, summary, then history.
	if messages[3].Content != standing || !strings.HasPrefix(messages[4].Content, "<conversation-summary>") {
		t.Fatalf("after compaction the standing block is not between guidance and the summary: %+v", messages)
	}
	// Four breakpoints at most: the standing block replaces the system prompt
	// mark, which its own prefix covers, so the summary keeps its own.
	if got := cacheMarkedIndexes(messages); !slices.Equal(got, []int{3, 4, 9, 13}) {
		t.Fatalf("cache breakpoints = %v", got)
	}
	if result.Snapshot.StandingMemory == nil || result.Snapshot.StandingMemory.Bytes != int64(len(standing)) ||
		!slices.Equal(result.Snapshot.StandingMemory.ClaimIDs, []memory.SemanticID{"claim-1"}) {
		t.Fatalf("standing receipt = %+v", result.Snapshot.StandingMemory)
	}

	input.Summary = nil
	input.Events = b.events
	uncompacted, err := composer.Compose(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := cacheMarkedIndexes(uncompacted.Request.Messages); len(got) < 2 || got[0] != 0 || got[1] != 3 {
		t.Fatalf("without a summary the standing block is the leading breakpoint: %v", got)
	}

	input.StandingMemory, input.StandingMemoryReceipt = "", nil
	input.Summary = &ContextSummary{CompactionEventID: "compaction-1", FirstRetainedEventID: "old-user", Content: "prior summary"}
	plain, err := composer.Compose(input)
	if err != nil {
		t.Fatal(err)
	}
	if standingMessageIndex(plain.Request.Messages) >= 0 || plain.Snapshot.StandingMemory != nil ||
		!slices.Equal(cacheMarkedIndexes(plain.Request.Messages), []int{0, 3, 8, 12}) {
		t.Fatalf("an empty standing set changed the request: %v", cacheMarkedIndexes(plain.Request.Messages))
	}
}

func TestStandingPreferenceReachesTheRequestAfterCompaction(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	rememberThroughTheModel(t, f, f.global(), "preference", "answer_style", "answer style", "concise answers", tools.Approved)
	record := f.global()
	holder := memory.LeaseHolderID("standing-compaction-" + string(record.ID))
	client := &fakeClient{steps: []step{assistantStep("one", nil), assistantStep("two", nil), assistantStep("three", nil), assistantStep("four", nil)}}
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: validCompactionSummary(),
	}}}}}}}
	session := NewWithCompactorAndToolset(client, compactor, testContextProfile("test-model"), f.store.BindHistory(record.ID, holder),
		record.ScopeContext(), f.store.BindTurnOwner(record.ID, holder), tools.NewToolset(nil), WithAutomaticMemoryRecall(false))
	for _, question := range []string{"Name a river.", "Name a mountain.", "Name a desert."} {
		if err := session.Send(ctx, question, &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if err := session.Send(ctx, "Name a lake.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	after := client.reqs[len(client.reqs)-1]
	index := standingMessageIndex(after.Messages)
	summary := slices.IndexFunc(after.Messages, func(m openrouter.Message) bool { return strings.HasPrefix(m.Content, "<conversation-summary>") })
	if index < 0 || summary != index+1 || !strings.Contains(after.Messages[index].Content, "concise answers") {
		t.Fatalf("after compaction: standing at %d, summary at %d in %+v", index, summary, after.Messages)
	}
	if after.Messages[index].Content != standingBlock(t, client.reqs[0]) {
		t.Fatal("compaction changed the standing block bytes")
	}
}
