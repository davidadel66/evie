package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

// automaticRecall sends one message through a real automatic-recall turn and
// returns the EVIE_MEMORY_DATA status and evidence the provider received.
func (f *retrievalFixture) automaticRecall(reader memory.Session, message string) (string, []memory.RetrievalEvidence) {
	f.t.Helper()
	client := &fakeClient{steps: []step{assistantStep("Answered from the supplied context.", nil)}}
	if err := f.automaticSession(reader, client).Send(context.Background(), message, &recorder{}, nil); err != nil {
		f.t.Fatal(err)
	}
	var data struct {
		Status   string                     `json:"status"`
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(f.t, client.reqs[0]), "EVIE_MEMORY_DATA\n")), &data); err != nil {
		f.t.Fatal(err)
	}
	return data.Status, data.Evidence
}

func evidenceFromEvent(evidence []memory.RetrievalEvidence, id memory.EventID) bool {
	for _, item := range evidence {
		for _, source := range item.Sources {
			if source.EventID == id {
				return true
			}
		}
	}
	return false
}

func evidenceFromSession(evidence []memory.RetrievalEvidence, id memory.SessionID) bool {
	for _, item := range evidence {
		for _, source := range item.Sources {
			if source.SessionID == id {
				return true
			}
		}
	}
	return false
}

func evidenceTexts(evidence []memory.RetrievalEvidence) []string {
	var texts []string
	for _, item := range evidence {
		texts = append(texts, item.Text)
	}
	return texts
}

// M1: an acknowledgement asks for nothing, so it must not search history or
// carry an earlier topic into recall, with or without a preceding exchange.
func TestAutomaticMemoryRecallLowContentMessageInjectsNothing(t *testing.T) {
	f := newRetrievalFixture(t)
	history := f.global()
	for _, text := range []string{"When should I prune the tomato plants?", "thanks!", "ok thanks", "sounds good", "perfect", "great, thanks!"} {
		f.converse(history, text)
	}
	f.refresh()
	for _, message := range []string{"thanks!", "ok", "sounds good, thanks", "perfect", "great, thanks!"} {
		for _, prelude := range []bool{false, true} {
			reader := f.global()
			if prelude {
				f.converse(reader, "How should I prune the tomato plants this weekend?")
				f.refresh()
			}
			status, evidence := f.automaticRecall(reader, message)
			if len(evidence) != 0 || status != memory.RetrievalEmpty {
				t.Errorf("%q (prelude=%t) injected %d items with status %s: %q", message, prelude, len(evidence), status, evidenceTexts(evidence))
			}
		}
	}
}

// M1: a request that shares one word with unrelated (here, private) history
// is not about that history. Nothing relevant exists, so nothing is injected.
func TestAutomaticMemoryRecallIgnoresOneSharedWord(t *testing.T) {
	f := newRetrievalFixture(t)
	history := f.global()
	f.converse(history, "Sam and I argued again last night; honestly I'm not sure our relationship is going to last.")
	f.converse(history, "The blood pressure reading this morning was 128 over 84.")
	f.converse(history, "Plan the interval run for Tuesday morning.")
	f.converse(history, "Is it too late in autumn to feed the lavender?")
	f.refresh()
	for _, message := range []string{
		"How do I model a many-to-many relationship in the ORM?",
		"Translate good morning into Portuguese.",
		"What tyre pressure does the car need?",
		"Write a haiku about autumn.",
	} {
		_, evidence := f.automaticRecall(f.global(), message)
		if len(evidence) != 0 {
			t.Errorf("%q injected unrelated history: %q", message, evidenceTexts(evidence))
		}
	}
}

// M1: the relevant statement survives; a second item that shares only one
// word of a multi-word request does not ride along in the spare slot.
func TestAutomaticMemoryRecallKeepsRelevantItemWithoutOneWordNoise(t *testing.T) {
	f := newRetrievalFixture(t)
	history := f.global()
	tyres := f.converse(history, "The car's tyre pressure should be 2.4 bar front and 2.2 bar rear.")
	blood := f.converse(history, "The blood pressure reading this morning was 128 over 84.")
	ferritin := f.converse(history, "My blood test results came back and my ferritin is low; the doctor wants a follow-up in six weeks.")
	plans := f.converse(history, "Sam forgot our plans again and I didn't say anything.")
	f.refresh()
	for _, tc := range []struct {
		message         string
		want, forbidden memory.EventID
	}{
		{"What tyre pressure does the car need?", tyres.ID, blood.ID},
		{"What did the doctor say about my ferritin?", ferritin.ID, plans.ID},
	} {
		_, evidence := f.automaticRecall(f.global(), tc.message)
		if !evidenceFromEvent(evidence, tc.want) || evidenceFromEvent(evidence, tc.forbidden) {
			t.Errorf("%q delivered %q", tc.message, evidenceTexts(evidence))
		}
	}
}

// M1: an unrelated earlier topic of the same session does not join a
// standalone request, and the session's own messages are not re-injected.
func TestAutomaticMemoryRecallKeepsUnrelatedEarlierTopicOut(t *testing.T) {
	f := newRetrievalFixture(t)
	history := f.global()
	tomato := f.converse(history, "When should I prune the tomato plants?")
	tyres := f.converse(history, "The car's tyre pressure should be 2.4 bar front and 2.2 bar rear.")
	reader := f.global()
	f.converse(reader, "How should I prune the tomato plants this weekend?")
	f.refresh()
	_, evidence := f.automaticRecall(reader, "What tyre pressure does the car need?")
	if !evidenceFromEvent(evidence, tyres.ID) || evidenceFromEvent(evidence, tomato.ID) || evidenceFromSession(evidence, reader.ID) {
		t.Fatalf("standalone request carried the earlier topic: %q", evidenceTexts(evidence))
	}
}

// M1: messages still in the provider request are not re-injected as memory;
// once compaction removes them from the request they are recallable again.
func TestAutomaticMemoryRecallOmitsMessagesStillInLiveContext(t *testing.T) {
	t.Run("live", func(t *testing.T) {
		f := newRetrievalFixture(t)
		history := f.global()
		booked := f.converse(history, "I booked the Lisbon hotel near Alfama for October 12 to 16.")
		reader := f.global()
		f.converse(reader, "The Lisbon hotel near Alfama looked lovely in the photos.")
		f.refresh()
		_, evidence := f.automaticRecall(reader, "When is the Lisbon hotel booked for?")
		if !evidenceFromEvent(evidence, booked.ID) || evidenceFromSession(evidence, reader.ID) {
			t.Fatalf("live-context message was re-injected or the original was lost: %q", evidenceTexts(evidence))
		}
	})
	t.Run("compacted", func(t *testing.T) {
		f := newRetrievalFixture(t)
		reader := f.global()
		original := f.converse(reader, "The greenhouse trial contains saffron crocuses and remains experimental.")
		for i := 0; i < 20; i++ {
			f.converse(reader, "Next, consider compiler diagnostics.")
		}
		compactor := &fakeClient{steps: []step{assistantStep(validCompactionSummary(), nil)}}
		holder := memory.LeaseHolderID("live-context-compaction")
		session := NewWithCompactorAndToolset(compactor, compactor, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(nil))
		if _, err := session.Compact(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.refresh()
		_, evidence := f.automaticRecall(reader, "What does the greenhouse trial contain?")
		if !evidenceFromEvent(evidence, original.ID) {
			t.Fatalf("compacted same-session statement was not recallable: %q", evidenceTexts(evidence))
		}
		for _, item := range evidence {
			if strings.Contains(item.Text, "compiler diagnostics") {
				t.Fatalf("retained live-context message was re-injected: %q", evidenceTexts(evidence))
			}
		}
	})
}

// gappedMemoryHistory reports every real search as cut short by the dense scan
// budget, so the turn's model-visible surfaces can be checked end to end.
type gappedMemoryHistory struct {
	*eviedb.SessionHistory
	store *eviedb.Store
}

func (h *gappedMemoryHistory) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	result, err := h.store.SearchMemory(ctx, scope, query)
	result.Gaps = append(result.Gaps, memory.RetrievalGapDenseScan)
	result.Status = memory.RetrievalPartial
	return result, err
}

func (h *gappedMemoryHistory) RevalidateMemoryEvidence(ctx context.Context, scope memory.ScopeContext, evidence []memory.RetrievalEvidence) ([]memory.RetrievalEvidence, error) {
	return h.store.RevalidateMemoryEvidence(ctx, scope, evidence)
}

// M7: a dense scan cut by its budget is visible to the model as a distinct
// gap in both the automatic EVIE_MEMORY_DATA block and the tool outcome.
func TestMemoryRecallSurfacesDenseScanGap(t *testing.T) {
	f := newRetrievalFixture(t)
	f.converse(f.global(), "The greenhouse trial contains saffron crocuses and remains experimental.")
	f.refresh()
	reader := f.global()
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("gap-" + string(reader.ID))
	history := &gappedMemoryHistory{SessionHistory: f.store.BindHistory(reader.ID, holder), store: f.store}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("conversation-lookup", "memory_search_conversations", `{"query":"greenhouse trial"}`)),
		assistantStep("Answered with the incomplete coverage noted.", nil),
	}}
	session := NewWithToolset(client, testContextProfile("test-model"), history, reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(definitions))
	if err := session.Send(context.Background(), "What does the greenhouse trial contain?", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	gap := `"gaps":["` + memory.RetrievalGapDenseScan + `"]`
	if data := retrievalData(t, client.reqs[0]); !strings.Contains(data, gap) {
		t.Errorf("automatic memory data hides the dense scan gap: %s", data)
	}
	if outcome := expansionBoundaryToolResult(t, client.reqs[1], "conversation-lookup"); !strings.Contains(outcome, gap) {
		t.Errorf("tool outcome hides the dense scan gap: %s", outcome)
	}
}

// A short follow-up still resolves through the earlier topic it depends on,
// and a verbose request keeps the statement it is actually about.
func TestAutomaticMemoryRecallKeepsFollowUpAndVerboseRequests(t *testing.T) {
	f := newRetrievalFixture(t)
	history := f.global()
	booked := f.converse(history, "I booked the Lisbon hotel near Alfama for October 12 to 16.")
	starter := f.converse(history, "The sourdough starter on the windowsill doubled overnight but smelled sharp.")
	// "book" is common in this history and "dates" rare, so a match on "book"
	// alone is generic; matches on the rarer word or the earlier topic are not.
	f.converse(history, "Book the car service before the 60,000 km check.")
	f.converse(history, "What's a good book for a long flight?")
	f.converse(history, "Should I book the physio for my back?")
	f.converse(f.global(), "Great, the Alfama hotel dates are October 12 to 16.")
	f.refresh()
	reader := f.global()
	f.converse(reader, "Let's plan the Lisbon trip.")
	f.refresh()
	_, evidence := f.automaticRecall(reader, "what dates did I book?")
	if !evidenceFromEvent(evidence, booked.ID) {
		t.Errorf("short follow-up lost its earlier topic: %q", evidenceTexts(evidence))
	}
	if slices.ContainsFunc(evidence, func(item memory.RetrievalEvidence) bool {
		return strings.Contains(item.Text, "car service") || strings.Contains(item.Text, "flight") || strings.Contains(item.Text, "physio")
	}) {
		t.Errorf("short follow-up injected a generic one-word match: %q", evidenceTexts(evidence))
	}
	_, evidence = f.automaticRecall(f.global(), "Can you remind me what I noticed about the sourdough starter on the windowsill when we compared spots last week?")
	if !evidenceFromEvent(evidence, starter.ID) {
		t.Errorf("verbose request lost its statement: %q", evidenceTexts(evidence))
	}
}

// The planner's documented gating (retrieval decisions, Stage 12): a
// low-content message does not search; only a short (at most two content
// terms) or referring follow-up turns earlier topics into qualifying groups;
// a standalone request takes earlier roots only when they share its words,
// and then only for ranking.
func TestAutomaticRecallPlanGatesEarlierTopics(t *testing.T) {
	event := func(id, text string) memory.Event {
		return memory.Event{ID: memory.EventID(id), Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text}
	}
	earlier := []memory.Event{event("e1", "How should I prune the tomato plants this weekend?"), event("e2", "Which tyre brand lasts longest?")}
	for _, tc := range []struct {
		message           string
		searches          bool
		contextGroups     int
		queryHasEarlier   string
		queryLacksEarlier string
	}{
		{"thanks!", false, 0, "", ""},
		{"sounds good, thanks", false, 0, "", ""},
		{"and the basil?", true, 2, "tomato", ""},
		{"What would she like?", true, 2, "tomato", ""},
		{"What tyre pressure does the car need?", true, 0, "brand", "tomato"},
		{"Can you recommend a good novel for a long flight?", true, 0, "", "tomato"},
	} {
		events := append(append([]memory.Event(nil), earlier...), event("now", tc.message))
		plan := planAutomaticRecall(events, nil, "now")
		if (plan.query != "") != tc.searches || len(plan.relevance.Context) != tc.contextGroups {
			t.Errorf("%q: query=%q context=%v", tc.message, plan.query, plan.relevance.Context)
		}
		if tc.queryHasEarlier != "" && !strings.Contains(plan.query, tc.queryHasEarlier) {
			t.Errorf("%q: query %q lacks earlier term %q", tc.message, plan.query, tc.queryHasEarlier)
		}
		if tc.queryLacksEarlier != "" && strings.Contains(plan.query, tc.queryLacksEarlier) {
			t.Errorf("%q: query %q carries unrelated earlier term %q", tc.message, plan.query, tc.queryLacksEarlier)
		}
	}
	summary := &ContextSummary{FirstRetainedEventID: "e2", Content: "Greenhouse experiment is the topic."}
	plan := planAutomaticRecall(append(append([]memory.Event(nil), earlier...), event("now", "What was it again?")), summary, "now")
	if plan.relevance.LiveFrom != "e2" || len(plan.relevance.Context) == 0 || !slices.Contains(plan.relevance.Context[len(plan.relevance.Context)-1], "greenhouse") {
		t.Fatalf("compacted follow-up lost continuity or its live frontier: %+v", plan.relevance)
	}
}
