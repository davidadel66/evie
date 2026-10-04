package agent

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// Privacy in accepted-memory recall (memory architecture follow-up to Stages
// 6–7). Automatic Recall applies the conversation excerpt floor to accepted
// Claims, to the newer-statement companions it adds, and to dense-only hits.
// When unsure, it does not inject; the model can still search explicitly.

// A request that merely shares a word with a saved fact is not about it: an
// unrelated or creative request receives neither a private Claim nor the
// newer statement linked to it. A request to recall the fact still does, in
// Global and in a project whose own history never used the fact's words.
func TestAutomaticRecallGatesAcceptedClaimsOnRelevance(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	source := f.global()
	therapist := readerRemember(t, f, source, "therapist", "Remember that I see Dr. Hale every other Tuesday for the panic attacks.", "Dr. Hale")
	later := f.converse(source, "I've been seeing a therapist about my anxiety since March.")
	medication := readerRemember(t, f, source, "daily_medication", "Remember that I take sertraline every morning with breakfast.", "sertraline")
	bank := readerRemember(t, f, source, "primary_bank", "Remember that my primary bank is Chase.", "Chase")
	dinner := readerRemember(t, f, source, "dinner_preference", "Remember that I prefer vegetarian dinners.", "vegetarian dinners")
	f.converse(source, "The VPN client needs an update.")
	project, err := f.store.RegisterProject(ctx, "Payments", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inProject, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.converse(inProject, "The flaky refund test fails when the clock skews.")
	f.refresh()

	for _, tc := range []struct {
		message   string
		forbidden memory.SemanticID
	}{
		{"Write a short story about a therapist and a lighthouse keeper.", therapist.ClaimID},
		{"Explain how daily medication reminders work in phone apps.", medication.ClaimID},
		{"Do I need to use a VPN for the bank?", bank.ClaimID},
	} {
		_, evidence := f.automaticRecall(f.global(), tc.message)
		if claimEvidence(evidence, tc.forbidden) != nil || evidenceFromEvent(evidence, later.ID) {
			t.Errorf("%q injected a saved fact it only shares a word with: %q", tc.message, evidenceTexts(evidence))
		}
	}
	for _, tc := range []struct {
		area    string
		reader  memory.Session
		message string
		want    memory.SemanticID
	}{
		{"global", f.global(), "Who is my therapist?", therapist.ClaimID},
		{"global", f.global(), "What daily medication do I take?", medication.ClaimID},
		{"global", f.global(), "Which bank is my primary bank?", bank.ClaimID},
		// Not a recall question, but the owner's own words state the fact.
		{"global", f.global(), "Suggest a dinner for me tonight.", dinner.ClaimID},
		// Global facts stay recallable where the project's history is silent.
		{"project", inProject, "Who is my therapist?", therapist.ClaimID},
	} {
		_, evidence := f.automaticRecall(tc.reader, tc.message)
		if claimEvidence(evidence, tc.want) == nil {
			t.Errorf("%q (%s) lost the saved fact it asks about: %q", tc.message, tc.area, evidenceTexts(evidence))
		}
	}
	// The model asked: an explicit search is not gated.
	if claimEvidence(suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "therapist"); return client }()), therapist.ClaimID) == nil {
		t.Fatal("an explicit memory_search lost the Claim it names")
	}
}

// A newer owner statement joins Automatic Recall only when its Claim
// qualified and the statement is itself relevant to the request: by its own
// words, or, for a request to recall the fact, through the words of the
// Claim it updates. Explicit searches keep every companion.
func TestAutomaticRecallAddsANewerStatementOnlyWhenRelevant(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	pharmacy := readerRemember(t, f, source, "pharmacy", "Remember that my pharmacy is Walgreens.", "Walgreens")
	update := f.converse(source, "I no longer use Walgreens; after the overdose scare my psychiatrist sends my prescriptions to CVS.")
	f.refresh()

	_, task := f.automaticRecall(f.global(), "Pharmacy hours, please.")
	if claimEvidence(task, pharmacy.ClaimID) == nil {
		t.Fatalf("a request in the owner's own words lost the saved fact: %q", evidenceTexts(task))
	}
	if evidenceFromEvent(task, update.ID) {
		t.Fatalf("a newer statement unrelated to the request rode along with its Claim: %q", evidenceTexts(task))
	}
	_, recall := f.automaticRecall(f.global(), "Which pharmacy do I use?")
	if claimEvidence(recall, pharmacy.ClaimID) == nil || !companionFrom(recall, update.ID) {
		t.Fatalf("a request to recall the fact lost its newer statement: %q", evidenceTexts(recall))
	}
	searched := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "pharmacy hours"); return client }())
	if claimEvidence(searched, pharmacy.ClaimID) == nil || !companionFrom(searched, update.ID) {
		t.Fatalf("an explicit search lost the newer statement: %q", evidenceTexts(searched))
	}
}

func companionFrom(evidence []memory.RetrievalEvidence, id memory.EventID) bool {
	item := excerptFromEvent(evidence, id)
	return item != nil && slices.Contains(item.Paths, "newer_owner_statement")
}

// A dense-only hit reaches Automatic Recall only with lexical corroboration,
// or, for a request to recall something about the owner, a similarity far
// above the dense floor. Explicit conversation search keeps dense recall.
func TestAutomaticRecallNeedsCorroborationForDenseOnlyHits(t *testing.T) {
	endpoint := newChosenSimilarityEndpoint(t, []chosenSimilarity{
		// Documents.
		{contains: "sertraline", axis: 0},
		{contains: "booster", axis: 1},
		// Queries, as Automatic Recall and the tools phrase them.
		{contains: "portuguese", axis: 0, cosine: 0.45},
		{contains: "swallow", axis: 0, cosine: 0.45},
		{contains: "checkup", axis: 1, cosine: 0.9},
	})
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	private := f.converse(source, "I take sertraline every morning with breakfast.")
	vet := f.converse(source, "Biscuit's annual vet appointment is every March for his booster.")
	// The Claim's dense document ("owner — vet booster: every March") names
	// the booster too.
	booster := readerRemember(t, f, source, "vet_booster", "Remember that the vet booster is every March.", "every March")
	f.refresh()

	for _, tc := range []struct {
		message string
		want    memory.EventID
		reject  memory.EventID
		claim   bool
	}{
		// Not a recall request, one shared word, moderate similarity.
		{message: "Translate good morning into Portuguese.", reject: private.ID},
		// A recall request, moderate similarity only.
		{message: "What do I swallow at dawn?", reject: private.ID},
		// A recall request in other words, near-paraphrase similarity.
		{message: "When is my pup's yearly checkup?", want: vet.ID, claim: true},
		// The same similarity for a request that is not about the owner.
		{message: "Explain a pup's yearly checkup.", reject: vet.ID},
	} {
		_, evidence := f.automaticRecall(f.global(), tc.message)
		if tc.want != "" && !evidenceFromEvent(evidence, tc.want) || tc.reject != "" && evidenceFromEvent(evidence, tc.reject) ||
			tc.claim != (claimEvidence(evidence, booster.ClaimID) != nil) {
			t.Errorf("%q delivered %q", tc.message, evidenceTexts(evidence))
		}
	}
	searched := suppliedEvidence(t, f.searchConversations(f.global(), "swallow at dawn"))
	if !evidenceFromEvent(searched, private.ID) {
		t.Fatalf("explicit conversation search lost its dense hit: %q", evidenceTexts(searched))
	}
}

// chosenSimilarity embeds any text containing a word on one axis, or, for a
// query, at the chosen cosine to that axis.
type chosenSimilarity struct {
	contains string
	axis     int
	cosine   float64
}

func newChosenSimilarityEndpoint(t *testing.T, rules []chosenSimilarity) *httptest.Server {
	t.Helper()
	embed := func(text string) []float32 {
		vector := make([]float32, 384)
		lower := strings.ToLower(text)
		for i, rule := range rules {
			if !strings.Contains(lower, rule.contains) {
				continue
			}
			if rule.cosine == 0 {
				vector[rule.axis] = 1
				return vector
			}
			vector[rule.axis] = float32(rule.cosine)
			vector[100+i] = float32(math.Sqrt(1 - rule.cosine*rule.cosine))
			return vector
		}
		vector[383] = 1
		return vector
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "all-minilm:22m", "model": "all-minilm:22m", "digest": "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}}})
		case "/api/embed":
			var request struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
				http.Error(w, "invalid bounded request", http.StatusBadRequest)
				return
			}
			embeddings := make([][]float32, 0, len(request.Input))
			for _, text := range request.Input {
				embeddings = append(embeddings, embed(text))
			}
			json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "embeddings": embeddings})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
