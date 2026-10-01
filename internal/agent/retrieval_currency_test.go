package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Harness review Stage 13 (M2, M3, M4): corrected and contradicted facts never
// surface as current. These turns use real SQLite, the Memory Plugin and the
// production retrieval path; only the provider is scripted.

func currencyEvidence(t *testing.T, client *fakeClient, request int) []memory.RetrievalEvidence {
	t.Helper()
	var data struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[request]), "EVIE_MEMORY_DATA\n")), &data); err != nil {
		t.Fatal(err)
	}
	return data.Evidence
}

func currencyExcerpt(evidence []memory.RetrievalEvidence, event memory.EventID) *memory.RetrievalEvidence {
	for i := range evidence {
		if evidence[i].Kind == memory.RetrievalConversationExcerpt && len(evidence[i].Sources) == 1 && evidence[i].Sources[0].EventID == event {
			return &evidence[i]
		}
	}
	return nil
}

func currencyRemember(t *testing.T, f *retrievalFixture, source memory.Session, predicate, label string, cardinality memory.PredicateCardinality, text, value string) memory.RememberLiteralProposal {
	t.Helper()
	a := f.session(source, nil)
	p, err := a.PrepareRememberLiteral(context.Background(), f.store, text, memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: predicate, PredicateLabel: label, PredicateCardinality: cardinality,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResolveRememberLiteral(context.Background(), f.store, p, tools.Approved); err != nil {
		t.Fatal(err)
	}
	return p
}

// M2: after memory_correct_claim the old source is still retrievable, but it is
// labelled superseded and linked to the correction, on every read path.
func TestCorrectedSourceIsLabelledHistoricalAndLinkedToItsCorrection(t *testing.T) {
	for _, mode := range []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged} {
		t.Run(string(mode), func(t *testing.T) {
			f := newRetrievalFixture(t)
			ctx := context.Background()
			source := f.global()
			saved := readerRemember(t, f, source, "mom_city", "Remember that my mom lives in Boston.", "Boston")
			owner := f.session(source, nil)
			request := memory.CorrectClaimRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), OldClaimID: saved.ClaimID, Mode: mode,
				Replacement: memory.ClaimProposition{SubjectEntityID: saved.Subject.ID, PredicateID: saved.Predicate.ID, Polarity: memory.PolarityAffirmed,
					Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}}}}
			if mode == memory.CorrectionChanged {
				effective := time.Now().UTC()
				request.EffectiveTime = &effective
			}
			proposal, err := owner.PrepareCorrectClaim(ctx, f.store, "Correction: my mom lives in Chicago, not Boston.", request)
			if err != nil {
				t.Fatal(err)
			}
			corrected, err := owner.ResolveCorrectClaim(ctx, f.store, proposal, tools.Approved)
			if err != nil {
				t.Fatal(err)
			}
			f.refresh()
			want := []memory.RetrievalHistoricalClaim{{Relation: memory.RetrievalHistoricalSource, ClaimID: saved.ClaimID, Status: memory.SemanticStatusSuperseded,
				CorrectionMode: mode, ReplacementClaimID: corrected.ReplacementClaimID}}
			check := func(path string, evidence []memory.RetrievalEvidence) {
				t.Helper()
				original := currencyExcerpt(evidence, saved.Source.EventID)
				if original == nil {
					t.Fatalf("%s: a corrected source must stay retrievable as history: %+v", path, evidence)
				}
				if original.CurrentStatus != memory.SemanticStatusSuperseded || original.Status != memory.SemanticStatusSuperseded || !reflect.DeepEqual(original.HistoricalClaims, want) {
					t.Fatalf("%s: corrected source is not labelled historical and linked to its correction: %+v", path, original)
				}
				if correction := currencyExcerpt(evidence, proposal.Source.EventID); correction != nil && (len(correction.HistoricalClaims) != 0 || correction.CurrentStatus != memory.SemanticStatusActive) {
					t.Fatalf("%s: the correction itself is the current statement, not a restatement: %+v", path, correction)
				}
			}
			reader := f.global()
			searched := f.searchConversations(reader, "mom Boston")
			check("conversation search", currencyEvidence(t, searched, 1))
			var rendered struct {
				HistoricalOnly []string `json:"historical_only"`
				ReadingGuide   string   `json:"reading_guide"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, searched.reqs[1]), "EVIE_MEMORY_DATA\n")), &rendered); err != nil {
				t.Fatal(err)
			}
			if original := currencyExcerpt(currencyEvidence(t, searched, 1), saved.Source.EventID); !slices.Contains(rendered.HistoricalOnly, original.ID) || !strings.Contains(rendered.ReadingGuide, "historical_claims") {
				t.Fatalf("the corrected source is not rendered as historical only: %+v", rendered)
			}
			_, automatic := f.automaticRecall(f.global(), "Does my mom still live in Boston?")
			check("automatic recall", automatic)
			client, _ := f.search(f.global(), "mom city")
			for _, item := range currencyEvidence(t, client, 1) {
				if item.ClaimID == saved.ClaimID {
					t.Fatalf("the corrected Claim came back as current memory: %+v", item)
				}
			}
		})
	}
}

// M2: a retired fact repeated without a Source Link, before or after the
// retirement, is flagged as restating it instead of reading as a fresh fact.
// Retirement still suppresses the Claim's own source, and a mention of the
// value without the Predicate's words is not flagged.
func TestRetiredFactRestatedWithoutSourceLinkIsFlagged(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	earlier := f.converse(source, "My mom has lived in Boston for years.")
	saved := readerRemember(t, f, source, "mom_city", "Remember that my mom lives in Boston.", "Boston")
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, saved.ClaimID)
	restated := f.converse(source, "Reminder that my mom lives in Boston these days.")
	unrelated := f.converse(source, "Boston has great seafood near the harbor.")
	f.refresh()
	evidence := currencyEvidence(t, f.searchConversations(f.global(), "mom Boston"), 1)
	if currencyExcerpt(evidence, saved.Source.EventID) != nil {
		t.Fatalf("retirement no longer suppresses the Claim's own source: %+v", evidence)
	}
	want := []memory.RetrievalHistoricalClaim{{Relation: memory.RetrievalHistoricalRestatement, ClaimID: saved.ClaimID, Status: memory.SemanticStatusRetired}}
	for name, event := range map[string]memory.EventID{"before retirement": earlier.ID, "after retirement": restated.ID} {
		item := currencyExcerpt(evidence, event)
		if item == nil || !reflect.DeepEqual(item.HistoricalClaims, want) || item.Status != memory.SemanticStatusActive {
			t.Fatalf("restatement %s is not flagged against the retired Claim: %+v", name, item)
		}
	}
	if item := currencyExcerpt(evidence, unrelated.ID); item == nil || len(item.HistoricalClaims) != 0 {
		t.Fatalf("a value mention without the Predicate's words was flagged or lost: %+v", item)
	}
	_, automatic := f.automaticRecall(f.global(), "Where does my mom live these days?")
	if item := currencyExcerpt(automatic, restated.ID); item == nil || !reflect.DeepEqual(item.HistoricalClaims, want) {
		t.Fatalf("automatic recall presented the restated retired fact as current: %+v", automatic)
	}
}

// M3: a later owner statement in different words is linked when it names the
// saved value with a change cue, or the Predicate's words with a change cue.
// Statements without first person, without a cue, questions, and single
// Predicate words are not linked.
func TestNewerOwnerStatementInDifferentWordsIsLinked(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	home := readerRemember(t, f, source, "home_city", "Remember that I live in Boston.", "Boston")
	shoe := readerRemember(t, f, source, "shoe_size", "Remember that my shoe size is 9.", "9")
	// A value whose words fold under inflection ("Mobile" -> "mobil") must
	// still be fetched by its exact words.
	carrier := readerRemember(t, f, source, "phone_carrier", "Remember that my phone carrier is T-Mobile.", "T-Mobile")
	moved := f.converse(source, "Big news: I moved to Chicago last month, Boston is behind me.")
	resized := f.converse(source, "My shoes are a size 10 now after the running season.")
	switched := f.converse(source, "I switched away from T-Mobile last month.")
	var distractors []memory.Event
	for _, text := range []string{
		"The Boston marathon moved to a new date this year.",
		"My Boston friends are visiting next week.",
		"Should I move back to Boston now?",
		"I need new running shoes before the 10k.",
	} {
		distractors = append(distractors, f.converse(source, text))
	}
	f.refresh()
	for _, tc := range []struct {
		query string
		claim memory.SemanticID
		newer memory.EventID
	}{{"home city", home.ClaimID, moved.ID}, {"shoe size", shoe.ClaimID, resized.ID}, {"phone carrier", carrier.ClaimID, switched.ID}} {
		client, _ := f.search(f.global(), tc.query)
		evidence := currencyEvidence(t, client, 1)
		item := currencyExcerpt(evidence, tc.newer)
		if item == nil || !slices.Contains(item.Paths, "newer_owner_statement") || !reflect.DeepEqual(item.RelatedClaimIDs, []memory.SemanticID{tc.claim}) {
			t.Fatalf("%q: newer statement in different words is not linked to the saved Claim: %+v", tc.query, evidence)
		}
		for _, distractor := range distractors {
			if currencyExcerpt(evidence, distractor.ID) != nil {
				t.Fatalf("%q: an unrelated statement was linked as a possible update: %q", tc.query, distractor.Content)
			}
		}
	}
}

// M4: label and cardinality drift version the Predicate, but retrieval still
// returns both active Claims with a conflict warning, and a turn holding one
// of them refreshes when a drifted conflicting Claim is accepted.
func TestPredicateDriftStillWarnsInRetrieval(t *testing.T) {
	for _, tc := range []struct {
		name, label string
		cardinality memory.PredicateCardinality
	}{
		{"label", "dental provider", memory.CardinalityOne},
		{"cardinality", "dentist", memory.CardinalityMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source := f.global()
			first := currencyRemember(t, f, source, "dentist", "dentist", memory.CardinalityOne, "Remember that my dentist is Dr. Patel.", "Dr. Patel")
			second := currencyRemember(t, f, source, "dentist", tc.label, tc.cardinality, "Remember that my dentist is Dr. Okafor now.", "Dr. Okafor")
			if second.Predicate.ID == first.Predicate.ID {
				t.Fatalf("fixture must exercise a new Predicate version: %+v", second.Predicate)
			}
			f.refresh()
			client, _ := f.search(f.global(), "dentist")
			claims := 0
			for _, item := range currencyEvidence(t, client, 1) {
				if item.Kind != memory.RetrievalAcceptedMemory {
					continue
				}
				claims++
				if len(item.Conflicts) != 1 || item.Conflicts[0].Code != memory.ConflictOneCardinality ||
					!slices.Contains(item.Conflicts[0].ClaimIDs, first.ClaimID) || !slices.Contains(item.Conflicts[0].ClaimIDs, second.ClaimID) {
					t.Fatalf("drifted Predicate versions came back without a conflict warning: %+v", item)
				}
			}
			if claims != 2 {
				t.Fatalf("both active drifted Claims must be returned: %d", claims)
			}
		})
	}
}

func TestHeldClaimRefreshesWhenDriftedConflictIsAccepted(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	readerRemember(t, f, source, "live_in", "Remember that I live in Boston.", "Boston")
	f.refresh()
	initial, _ := json.Marshal(map[string]any{"query": "Boston"})
	remember, _ := json.Marshal(map[string]any{"idempotency_key": "idem:v1:" + uuid.NewString(), "predicate": "live_in", "predicate_label": "cities I live in", "cardinality": "many", "literal_kind": "text", "literal_value": "Portland", "polarity": "affirmed", "destination": "everywhere"})
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("initial", "memory_search", string(initial))),
		assistantStep("", nil, toolCall("new-fact", "memory_remember_literal", string(remember))),
		assistantStep("Both records are shown.", nil),
	}}
	if err := f.session(reader, client).Send(context.Background(), "Read Boston, then remember that I also live in Portland.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	claims := 0
	for _, item := range currencyEvidence(t, client, 2) {
		if item.ClaimID == "" {
			continue
		}
		claims++
		if len(item.Conflicts) != 1 {
			t.Fatalf("a drifted conflicting Claim did not refresh the held read: %+v", item)
		}
	}
	if claims != 2 {
		t.Fatalf("held current read did not refresh across Predicate versions: %d Claims", claims)
	}
}
