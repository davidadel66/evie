package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Memory architecture Stage 6 (recall quality): fact-augmented keys and
// time-aware query expansion, through real turns on real SQLite with the
// Memory Plugin and the production retrieval path.

// recallTestClock is a settable store clock. Each reading advances a
// millisecond, so leases and event order behave as on a real clock.
type recallTestClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *recallTestClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(time.Millisecond)
	return c.at
}

func (c *recallTestClock) set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = at
}

// The owner's zone in these tests: four hours behind UTC.
var recallTestZone = time.FixedZone("UTC-4", -4*60*60)

func recallTestTime(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, recallTestZone)
}

func (f *retrievalFixture) recallClock(start time.Time) *recallTestClock {
	clock := &recallTestClock{at: start}
	f.store.UseClock(clock.now, recallTestZone)
	return clock
}

func excerptFromEvent(evidence []memory.RetrievalEvidence, id memory.EventID) *memory.RetrievalEvidence {
	for i := range evidence {
		if evidence[i].Kind == memory.RetrievalConversationExcerpt && len(evidence[i].Sources) == 1 && evidence[i].Sources[0].EventID == id {
			return &evidence[i]
		}
	}
	return nil
}

func claimEvidence(evidence []memory.RetrievalEvidence, id memory.SemanticID) *memory.RetrievalEvidence {
	for i := range evidence {
		if evidence[i].Kind == memory.RetrievalAcceptedMemory && evidence[i].ClaimID == id {
			return &evidence[i]
		}
	}
	return nil
}

// retrievalOutcome decodes the model-visible outcome of a scripted read tool.
func retrievalOutcome(t *testing.T, client *fakeClient, callID string) struct {
	Status     string                      `json:"status"`
	TimeWindow *memory.RetrievalTimeWindow `json:"time_window"`
} {
	t.Helper()
	var outcome struct {
		Status     string                      `json:"status"`
		TimeWindow *memory.RetrievalTimeWindow `json:"time_window"`
	}
	lines := strings.Split(toolOutcome(t, client.reqs[len(client.reqs)-1], callID), "\n")
	if len(lines) != 3 || json.Unmarshal([]byte(lines[1]), &outcome) != nil {
		t.Fatalf("unexpected tool outcome frame %q", lines)
	}
	return outcome
}

// A question phrased like the saved fact finds the message the fact came
// from, although the message used other words. The excerpt is the message's
// own text; the Claim's words only matched it.
func TestConversationSearchFindsTheOriginalMessageByItsClaimsWords(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	command := "Remember that I'm allergic to penicillin; I broke out in hives after the course in 2019."
	saved := f.prepareLiteral(source, command, "drug_allergy", "penicillin")
	f.approveLiteral(source, saved)
	f.converse(source, "The pharmacy on Elm Street closes at six.")
	f.refresh()

	evidence := suppliedEvidence(t, f.searchConversations(reader, "drug allergy"))
	excerpt := excerptFromEvent(evidence, saved.Source.EventID)
	if excerpt == nil || len(evidence) != 1 {
		t.Fatalf("the original message was not found by its Claim's words: %q", evidenceTexts(evidence))
	}
	if excerpt.Text != command || excerpt.ClaimID != "" || !slices.Contains(excerpt.Paths, "conversation_fact_key") {
		t.Fatalf("a key match must render the message's own words and say how it matched: %+v", excerpt)
	}
}

// Automatic Recall uses a Claim's words only for a request to recall
// something about the owner. A general request that shares a Predicate label
// is not about the owner's saved fact.
func TestAutomaticRecallUsesFactKeysOnlyForPersonalRecall(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	saved := f.prepareLiteral(source, "Remember that I'm allergic to penicillin; I broke out in hives after the course in 2019.", "drug_allergy", "penicillin")
	f.approveLiteral(source, saved)
	f.converse(source, "List the forms the clinic needs before Friday.")
	f.refresh()

	_, personal := f.automaticRecall(f.global(), "Which drug allergy should I list on the clinic form?")
	if excerptFromEvent(personal, saved.Source.EventID) == nil {
		t.Fatalf("a personal recall did not receive the fact's original message: %q", evidenceTexts(personal))
	}
	_, general := f.automaticRecall(f.global(), "Explain how a drug allergy differs from an intolerance.")
	if excerptFromEvent(general, saved.Source.EventID) != nil {
		t.Fatalf("a general request received the owner's message through a Claim's words: %q", evidenceTexts(general))
	}
}

// Keys follow accepted state: another session never matches a
// session-scoped Claim's words, and a corrected Claim's words stop matching
// its old message at once, before the index catches up.
func TestFactKeysFollowClaimScopeAndLifecycle(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	source, reader := f.global(), f.global()
	owner := f.session(source, nil)
	temporary, err := owner.PrepareRememberLiteral(ctx, f.store, "Remember for now that the code for my Equinox locker is 3-1-4.", memory.RememberLiteralRequest{
		Destination: memory.MemorySession, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "gym_locker_combination", PredicateLabel: "gym locker combination",
		PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "3-1-4"}, Polarity: memory.PolarityAffirmed})
	if err != nil {
		t.Fatal(err)
	}
	f.approveLiteral(source, temporary)
	saved := f.prepareLiteral(source, "Remember that our tabby is called Miso and she hates the vacuum.", "cat_name", "Miso")
	f.approveLiteral(source, saved)
	f.refresh()

	if evidence := suppliedEvidence(t, f.searchConversations(reader, "combination")); excerptFromEvent(evidence, temporary.Source.EventID) != nil {
		t.Fatalf("another session matched a session-scoped Claim's words: %q", evidenceTexts(evidence))
	}
	if evidence := suppliedEvidence(t, f.searchConversations(reader, "cat name")); excerptFromEvent(evidence, saved.Source.EventID) == nil {
		t.Fatalf("a current Claim's words did not find its message: %q", evidenceTexts(evidence))
	}

	request := memory.CorrectClaimRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), OldClaimID: saved.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: saved.Subject.ID, PredicateID: saved.Predicate.ID, Polarity: memory.PolarityAffirmed,
			Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Mochi"}}}}
	proposal, err := owner.PrepareCorrectClaim(ctx, f.store, "Correction: our tabby is called Mochi, not Miso.", request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ResolveCorrectClaim(ctx, f.store, proposal, tools.Approved); err != nil {
		t.Fatal(err)
	}
	// The index still holds the old key; the read rechecks accepted state.
	if evidence := suppliedEvidence(t, f.searchConversations(f.global(), "cat name")); excerptFromEvent(evidence, saved.Source.EventID) != nil {
		t.Fatalf("a corrected Claim's stale key still matched its old message: %q", evidenceTexts(evidence))
	}
	f.refresh()
	evidence := suppliedEvidence(t, f.searchConversations(f.global(), "cat name"))
	if excerptFromEvent(evidence, saved.Source.EventID) != nil || excerptFromEvent(evidence, proposal.Source.EventID) == nil {
		t.Fatalf("after refresh only the correction should match the Claim's words: %q", evidenceTexts(evidence))
	}
}

// An Entity Claim keys its message by the Entities' names and aliases (but
// never by the owner's own anchor), and keys stay in the message's area: a
// Global Claim remembered in a Workspace keys that Workspace's message for
// its own readers only, because conversation search never crosses areas.
func TestFactKeysNameEntitiesAndStayInTheMessagesArea(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	sitter := f.rememberGraphEntity(global, "Inge looks after Biscuit when we travel.", memory.RememberEntityRequest{
		Predicate: "pet_sitter_for", PredicateLabel: "pet sitter for",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Ingrid Solberg", EntityType: "person", Alias: "Inge"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Biscuit", EntityType: "pet", Alias: "Biscuit"},
	})
	blood := f.prepareLiteral(global, "Remember that I'm O negative, and my donor card is in the red wallet.", "blood_type", "O negative")
	f.approveLiteral(global, blood)
	kitchen := f.workspaceSessions("Kitchen", 2)
	garden := f.workspaceSessions("Garden", 1)
	request := memory.RememberLiteralRequest{Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "knife_sharpener",
		PredicateLabel: "knife sharpener", PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Okabe"}, Polarity: memory.PolarityAffirmed}
	everywhere, err := f.session(kitchen[0], nil).PrepareRememberLiteral(ctx, f.store, "Remember that Okabe on Dock Street does my blades every spring.", request)
	if err != nil {
		t.Fatal(err)
	}
	f.approveLiteral(kitchen[0], everywhere)
	f.refresh()

	for _, query := range []string{"Ingrid Solberg", "pet sitter"} {
		if evidence := suppliedEvidence(t, f.searchConversations(f.global(), query)); excerptFromEvent(evidence, sitter.Source.EventID) == nil {
			t.Fatalf("%q did not find the message by its Entity's name or Predicate: %q", query, evidenceTexts(evidence))
		}
	}
	if evidence := suppliedEvidence(t, f.searchConversations(f.global(), "owner")); excerptFromEvent(evidence, blood.Source.EventID) != nil {
		t.Fatalf("the owner's anchor keyed a message: %q", evidenceTexts(evidence))
	}
	if evidence := suppliedEvidence(t, f.searchConversations(kitchen[1], "knife sharpener")); excerptFromEvent(evidence, everywhere.Source.EventID) == nil {
		t.Fatalf("a Global Claim did not key its Workspace message for that Workspace: %q", evidenceTexts(evidence))
	}
	for _, reader := range []memory.Session{garden[0], f.global()} {
		if evidence := suppliedEvidence(t, f.searchConversations(reader, "knife sharpener")); excerptFromEvent(evidence, everywhere.Source.EventID) != nil {
			t.Fatalf("a key carried a Workspace message outside its area: %q", evidenceTexts(evidence))
		}
	}

	// Retiring an alias changes the Claim's words; the refresh that
	// re-indexes the Claim re-keys the message it cites.
	keys := func() string {
		t.Helper()
		var text string
		if err := f.db.QueryRow(`SELECT keys FROM memory_retrieval_event_fts_v4 WHERE event_id=?`, sitter.Source.EventID).Scan(&text); err != nil {
			t.Fatal(err)
		}
		return text
	}
	var inge memory.SemanticID
	for _, alias := range sitter.Aliases {
		if alias.Value == "Inge" {
			inge = alias.ID
		}
	}
	if inge == "" || !strings.Contains(keys(), "inge") || strings.Contains(keys(), "owner") {
		t.Fatalf("keys before retiring the alias = %q (alias %q)", keys(), inge)
	}
	f.lifecycle(global, "memory_retire", memory.SemanticObjectAlias, inge)
	f.refresh()
	if text := keys(); strings.Contains(text, "inge") || !strings.Contains(text, "ingrid") || !strings.Contains(text, "pet") {
		t.Fatalf("keys after retiring the alias = %q", text)
	}
}

// A request that names a past time prefers what was said then, among
// messages on the same subject from before and after it.
func TestRecallPrefersMessagesFromTheNamedTime(t *testing.T) {
	f := newRetrievalFixture(t)
	clock := f.recallClock(recallTestTime(time.September, 12, 10, 0))
	older := f.converse(f.global(), "Marek is hard to book in winter, so plan early.")
	clock.set(recallTestTime(time.September, 30, 19, 30))
	target := f.converse(f.global(), "Booked Marek to fix the leak under the kitchen sink on Monday morning.")
	clock.set(recallTestTime(time.October, 1, 8, 15))
	newer := f.converse(f.global(), "Marek says the kitchen sink part might arrive late.")
	clock.set(recallTestTime(time.October, 1, 18, 0))
	f.refresh()

	_, evidence := f.automaticRecall(f.global(), "What did I book with Marek yesterday?")
	found := excerptFromEvent(evidence, target.ID)
	if found == nil || excerptFromEvent(evidence, older.ID) != nil || excerptFromEvent(evidence, newer.ID) != nil {
		t.Fatalf("yesterday's message was not preferred: %q", evidenceTexts(evidence))
	}
	if !slices.Contains(found.Paths, "time_window") {
		t.Fatalf("the excerpt does not say it was selected by its time: %+v", found.Paths)
	}

	// A model-directed search reads the same expression from its query, and
	// its outcome names the window.
	client := f.searchConversations(f.global(), "Marek yesterday")
	evidence = suppliedEvidence(t, client)
	if len(evidence) != 1 || excerptFromEvent(evidence, target.ID) == nil {
		t.Fatalf("the model-directed search did not apply yesterday: %q", evidenceTexts(evidence))
	}
	outcome := retrievalOutcome(t, client, "conversation-lookup")
	if outcome.TimeWindow == nil || !outcome.TimeWindow.Matched || outcome.TimeWindow.Expression != "yesterday" ||
		!outcome.TimeWindow.From.Equal(recallTestTime(time.September, 30, 0, 0)) || !outcome.TimeWindow.To.Equal(recallTestTime(time.October, 1, 0, 0)) {
		t.Fatalf("the outcome does not name yesterday in the owner's zone: %+v", outcome.TimeWindow)
	}

	// A window with nothing in it falls back to all eligible history, so a
	// time word that does not fit loses no recall.
	_, fallback := f.automaticRecall(f.global(), "What did I book with Marek two months ago?")
	if excerptFromEvent(fallback, older.ID) == nil || slices.ContainsFunc(fallback, func(item memory.RetrievalEvidence) bool { return slices.Contains(item.Paths, "time_window") }) {
		t.Fatalf("an empty window did not fall back to ordinary recall: %q", evidenceTexts(fallback))
	}
	// Words that point at the future name no window.
	client = f.searchConversations(f.global(), "Marek kitchen sink for Friday")
	if outcome := retrievalOutcome(t, client, "conversation-lookup"); outcome.TimeWindow != nil {
		t.Fatalf("a future day was read as a past window: %+v", outcome.TimeWindow)
	}
}

// A past window reads a Claim that was valid then but is not current now,
// labelled with its status and the instant it was read at. The present
// question still receives only the current Claim.
func TestRecallReadsAClaimValidDuringAPastWindow(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	clock := f.recallClock(recallTestTime(time.January, 10, 9, 0))
	source := f.global()
	saved := f.prepareLiteral(source, "Remember that I work at Initech.", "employer", "Initech")
	f.approveLiteral(source, saved)
	clock.set(recallTestTime(time.June, 15, 9, 0))
	effective := clock.now().UTC()
	owner := f.session(source, nil)
	proposal, err := owner.PrepareCorrectClaim(ctx, f.store, "Update my employer: I now work at Globex.", memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), OldClaimID: saved.ClaimID, Mode: memory.CorrectionChanged, EffectiveTime: &effective,
		Replacement: memory.ClaimProposition{SubjectEntityID: saved.Subject.ID, PredicateID: saved.Predicate.ID, Polarity: memory.PolarityAffirmed,
			Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Globex"}}}})
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := owner.ResolveCorrectClaim(ctx, f.store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	clock.set(recallTestTime(time.October, 1, 18, 0))
	f.refresh()

	_, evidence := f.automaticRecall(f.global(), "Who was my employer in March?")
	old := claimEvidence(evidence, saved.ClaimID)
	if old == nil || claimEvidence(evidence, corrected.ReplacementClaimID) == nil {
		t.Fatalf("the employer valid in March was not read: %+v", evidence)
	}
	march := recallTestTime(time.March, 1, 0, 0)
	if old.Status != memory.SemanticStatusSuperseded || old.CurrentStatus != memory.SemanticStatusSuperseded || !old.ValidAtConstrained ||
		old.ValidAt.Before(march) || !old.ValidAt.Before(march.AddDate(0, 1, 0)) || !slices.Contains(old.Paths, "valid_during") {
		t.Fatalf("the past Claim must be labelled superseded and read at an instant in March: %+v", old)
	}
	_, current := f.automaticRecall(f.global(), "Who is my employer?")
	if claimEvidence(current, saved.ClaimID) != nil || claimEvidence(current, corrected.ReplacementClaimID) == nil {
		t.Fatalf("a present question must receive only the current Claim: %+v", current)
	}
}

// Episode search selects the summaries whose covered time overlaps a named
// past time, and a query that names only a time lists that range's episodes.
func TestEpisodeSearchSelectsANamedPastTime(t *testing.T) {
	f := newRetrievalFixture(t)
	clock := f.recallClock(recallTestTime(time.August, 18, 19, 0))
	august, lastWeek, reader := f.global(), f.global(), f.global()
	f.converse(august, "For the hallway paint I'm torn between Sage Whisper and Pale Oak.")
	f.converse(august, "The samples are taped up by the front door.")
	f.converse(august, "I'll look again in daylight.")
	f.converse(august, "Thanks.")
	f.compact(august, summaryWithSections(map[string]string{sectionState: "Compared hallway paint colours; undecided between Sage Whisper and Pale Oak."}))
	clock.set(recallTestTime(time.September, 24, 20, 0))
	f.converse(lastWeek, "We picked Sage Whisper for the hallway paint.")
	f.converse(lastWeek, "The painter starts on the 5th.")
	f.converse(lastWeek, "Two coats should be enough.")
	f.converse(lastWeek, "Thanks.")
	f.compact(lastWeek, summaryWithSections(map[string]string{sectionState: "Picked Sage Whisper for the hallway paint."}))
	clock.set(recallTestTime(time.October, 1, 18, 0))

	raw, output := f.searchEpisodes(reader, map[string]any{"query": "hallway paint last week"})
	if len(output.Episodes) != 1 || output.Episodes[0].SessionID != lastWeek.ID || !strings.Contains(raw, `"time_window"`) {
		t.Fatalf("last week's episode was not selected: %s", raw)
	}
	raw, output = f.searchEpisodes(reader, map[string]any{"query": "last week"})
	if len(output.Episodes) != 1 || output.Episodes[0].SessionID != lastWeek.ID {
		t.Fatalf("a time-only query did not list last week's episode: %s", raw)
	}
	raw, output = f.searchEpisodes(reader, map[string]any{"query": "hallway paint"})
	if len(output.Episodes) != 2 || strings.Contains(raw, `"time_window"`) {
		t.Fatalf("a query without a time must search every episode: %s", raw)
	}
}
