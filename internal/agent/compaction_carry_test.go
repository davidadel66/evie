package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

const (
	sectionGoal       = "Goal / criteria / constraints"
	sectionState      = "Current state / completed actions"
	sectionDecisions  = "Decisions / discoveries"
	sectionDurable    = "Durable paths / IDs / artifacts / tool outcomes"
	sectionUnresolved = "Unresolved questions / blockers / risks"
	sectionNext       = "Next steps"
	sectionPrefs      = "User preferences / commitments"
)

// summaryWithSections renders a valid summary; unnamed sections say "kept".
func summaryWithSections(bodies map[string]string) string {
	var b strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		body, ok := bodies[heading]
		if !ok {
			body = "kept"
		}
		b.WriteString("## " + heading + "\n" + body + "\n\n")
	}
	return b.String()
}

func summarySection(t *testing.T, summary, heading string) string {
	t.Helper()
	marker := "## " + heading + "\n"
	start := strings.Index(summary, marker)
	if start < 0 {
		t.Fatalf("summary lacks %q: %q", heading, summary)
	}
	body := summary[start+len(marker):]
	if end := strings.Index(body, "\n## "); end >= 0 {
		body = body[:end]
	}
	return strings.TrimSpace(body)
}

func priorContinuitySummary() string {
	return summaryWithSections(map[string]string{
		sectionGoal:       "- Ship the importer by Friday",
		sectionState:      "- Parser written",
		sectionDecisions:  "- Use SQLite, not Postgres\n- Keep CSV as the only input",
		sectionDurable:    "- internal/importer/csv.go\n- task id=task-42",
		sectionUnresolved: "- Date format for EU banks",
		sectionNext:       "- Add EU date parsing",
		sectionPrefs:      "- David wants plain commit messages",
	})
}

func TestCompactionCarriesForwardSectionsCollapsedToPlaceholders(t *testing.T) {
	prior := priorContinuitySummary()
	next := summaryWithSections(map[string]string{
		sectionGoal:       "- Ship the importer by Friday",
		sectionState:      "- Parser written\n- EU dates parsed",
		sectionDecisions:  "None.",
		sectionDurable:    "_Unchanged from the prior summary._",
		sectionUnresolved: "- None",
		sectionNext:       "- Same as before",
		sectionPrefs:      "No new preferences.",
	})
	carried, err := carryForwardCompactionSections(prior, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompactionSummary(carried); err != nil {
		t.Fatalf("carried summary is invalid: %v\n%s", err, carried)
	}
	// An empty placeholder is replaced; a back-reference keeps its own text
	// after the restored prior section, so a misread line is never lost.
	for heading, want := range map[string]string{
		sectionGoal:       "- Ship the importer by Friday",
		sectionState:      "- Parser written\n- EU dates parsed",
		sectionDecisions:  "- Use SQLite, not Postgres\n- Keep CSV as the only input",
		sectionDurable:    "- internal/importer/csv.go\n- task id=task-42\n_Unchanged from the prior summary._",
		sectionUnresolved: "- None",
		sectionNext:       "- Add EU date parsing\n- Same as before",
		sectionPrefs:      "- David wants plain commit messages\nNo new preferences.",
	} {
		if got := summarySection(t, carried, heading); got != want {
			t.Errorf("section %q = %q, want %q", heading, got, want)
		}
	}
}

func TestCompactionCarryForwardNeverDropsGeneratedText(t *testing.T) {
	prior := priorContinuitySummary()
	next := summaryWithSections(map[string]string{sectionDecisions: "- No new dependencies"})
	carried, err := carryForwardCompactionSections(prior, next)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summarySection(t, carried, sectionDecisions),
		"- Use SQLite, not Postgres\n- Keep CSV as the only input\n- No new dependencies"; got != want {
		t.Fatalf("decisions = %q, want %q", got, want)
	}
}

func TestCompactionKeepsSubstantiveAndFirstGenerationSummaries(t *testing.T) {
	next := summaryWithSections(map[string]string{sectionDecisions: "None.", sectionNext: "Unchanged"})
	if carried, err := carryForwardCompactionSections("", next); err != nil || carried != next {
		t.Fatalf("first generation changed: %q, %v", carried, err)
	}
	prior := summaryWithSections(map[string]string{sectionDecisions: "None.", sectionPrefs: "- No emojis in commit messages"})
	updated := summaryWithSections(map[string]string{sectionDecisions: "N/A", sectionPrefs: "- No emojis anywhere"})
	if carried, err := carryForwardCompactionSections(prior, updated); err != nil || carried != updated {
		t.Fatalf("substantive or already-empty sections changed: %q, %v", carried, err)
	}
}

func TestCompactionCarryForwardRejectsAnOversizedResult(t *testing.T) {
	prior := summaryWithSections(map[string]string{sectionDecisions: strings.Repeat("d", 9000)})
	next := summaryWithSections(map[string]string{sectionDecisions: "None", sectionState: strings.Repeat("s", 9000)})
	if _, err := carryForwardCompactionSections(prior, next); err == nil {
		t.Fatal("carry-forward accepted a summary above the size limit")
	}
}

func TestManualCompactionStoresCarriedForwardSections(t *testing.T) {
	turns := make([][]memory.Event, 7)
	for i := range turns[:5] {
		turns[i] = completedCompactionTurn("turn-"+string(rune('1'+i)), int64(i*2+1), "user", "assistant")
	}
	prior := priorContinuitySummary()
	first := contextCompactionEvent(t, "compact-1", 11, 1, "", turns[0][0], turns[2][1], turns[3][0], prior, "old/model", "compaction-v1")
	for i := 5; i < len(turns); i++ {
		turns[i] = completedCompactionTurn("turn-"+string(rune('1'+i)), int64(i*2+2), "user", "assistant")
	}
	events := append(flattenContextTurns(turns[:5]), first)
	events = append(events, flattenContextTurns(turns[5:])...)
	history := &fakeHistory{events: events}
	generated := summaryWithSections(map[string]string{sectionDecisions: "None.", sectionPrefs: "See prior summary."})
	compactor := &fakeClient{steps: []step{{res: openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: generated,
	}}}}}}}
	session := NewWithCompactor(compactor, compactor, testContextProfile("current/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	result, err := session.Compact(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	summary, _, err := reconstructCompactionChain(history.allEvents())
	if err != nil {
		t.Fatal(err)
	}
	if summary.CompactionEventID != result.CompactionEventID ||
		summarySection(t, summary.Content, sectionDecisions) != "- Use SQLite, not Postgres\n- Keep CSV as the only input" ||
		summarySection(t, summary.Content, sectionPrefs) != "- David wants plain commit messages\nSee prior summary." {
		t.Fatalf("accepted summary did not carry vanished sections:\n%s", summary.Content)
	}
	if !strings.Contains(compactor.reqs[0].Messages[0].Content, "Carry forward") {
		t.Fatalf("compactor prompt does not ask for carried continuity: %q", compactor.reqs[0].Messages[0].Content)
	}
}

func TestAutomaticCompactionStoresCarriedForwardSections(t *testing.T) {
	turn1 := completedCompactionTurn("turn-1", 1, "old", "old answer")
	turn2 := completedCompactionTurn("turn-2", 3, strings.Repeat("b", 90_000), "answer two")
	prior := priorContinuitySummary()
	accepted := contextCompactionEvent(t, "compaction-1", 5, 1, "", turn1[0], turn1[1], turn2[0], prior, "old/model", "compaction-v1")
	turn3 := completedCompactionTurn("turn-3", 6, strings.Repeat("c", 90_000), "answer three")
	events := append(append(append([]memory.Event{}, turn1...), turn2...), accepted)
	events = append(events, turn3...)
	history := &fakeHistory{events: events}
	generated := summaryWithSections(map[string]string{sectionDurable: "Unchanged."})
	compactor := &fakeClient{steps: []step{assistantStep(generated, nil)}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 230_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	if err := session.Send(context.Background(), strings.Repeat("d", 8_000), &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	var stored memory.Event
	for _, event := range history.allEvents() {
		if event.Type == memory.EventContextCompacted && event.ID != accepted.ID {
			stored = event
		}
	}
	if stored.ID == "" || summarySection(t, stored.Content, sectionDurable) != "- internal/importer/csv.go\n- task id=task-42\nUnchanged." {
		t.Fatalf("automatic summary did not carry the durable section: %+v", stored)
	}
	var payload memory.ContextCompactedPayload
	if err := json.Unmarshal(stored.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SummaryBytes != int64(len(stored.Content)) {
		t.Fatalf("payload bytes %d do not describe the carried summary %d", payload.SummaryBytes, len(stored.Content))
	}
	if len(conversation.reqs) != 1 || conversation.reqs[0].Messages[1].Content != contextSummaryMessage(stored.Content) {
		t.Fatalf("conversation did not receive the carried summary: %+v", conversation.reqs)
	}
}
