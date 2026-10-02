package eviedb

import (
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// The floor's documented thresholds (retrieval decisions, Stage 12): a
// request of three or more content terms needs two matched terms, shorter
// requests one; at least one match must be distinctive (the rarer half of the
// request's terms that occur in history); a follow-up's earlier topic
// qualifies evidence through one of its own distinctive terms.
func TestRelevanceFloorThresholds(t *testing.T) {
	frequency := map[string]int{
		"relationship": 1, "pressure": 2, "tyre": 1, "car": 21, "need": 17,
		"greenhouse": 1, "dates": 1, "book": 43, "work": 2, "days": 48,
		"lisbon": 1, "plan": 50, "trip": 20, "test": 21, "results": 1,
		"today": 11, "run": 42, "summarize": 15, "service": 37, "sunrise": 1,
		"vet": 1, "use": 30, "cat": 12, "remind": 9, "dentist": 5, "passport": 1,
		"energy": 1, "dishwasher": 6, "doctor": 1, "say": 1, "ferritin": 1,
		"routine": relevanceCommonDocuments, "weekly": relevanceCommonDocuments,
	}
	for _, tc := range []struct {
		name    string
		text    string
		current []string
		context [][]string
		want    bool
	}{
		{"one shared word of a four-term request", "Sam and I argued; I'm not sure our relationship will last.", []string{"model", "many", "relationship", "orm"}, nil, false},
		{"one shared word of a tyre question", "The blood pressure reading was 128 over 84.", []string{"tyre", "pressure", "car", "need"}, nil, false},
		{"the tyre statement", "The car's tyre pressure should be 2.4 bar.", []string{"tyre", "pressure", "car", "need"}, nil, true},
		{"absent word does not block a two-term request", "The greenhouse has saffron crocuses.", []string{"plant", "greenhouse"}, nil, true},
		{"rarer word of a two-term request", "The Alfama hotel dates are October 12 to 16.", []string{"dates", "book"}, nil, true},
		{"common word of a two-term request", "Book the car service before the check.", []string{"dates", "book"}, nil, false},
		{"distinctive pair of a long request", "My blood test results came back.", []string{"summarize", "test", "results", "today", "ci", "run", "payments", "service"}, nil, true},
		{"common pair of a long request", "Keep the service on the run schedule.", []string{"summarize", "test", "results", "today", "ci", "run", "payments", "service"}, nil, false},
		{"follow-up earlier topic", "I booked the Lisbon hotel near Alfama.", []string{"dates", "book"}, [][]string{{"plan", "lisbon", "trip"}}, true},
		{"follow-up earlier topic's common word", "Plan the interval run for Tuesday.", []string{"dates", "book"}, [][]string{{"plan", "lisbon", "trip"}}, false},
		{"case and punctuation fold", "LISBON, again!", nil, [][]string{{"plan", "lisbon", "trip"}}, true},
		{"inflection counts as the same word", "ownerorchid runs before sunrise", []string{"sunrise", "running", "club"}, nil, true},
		{"inflection does not invent a second word", "ownerorchid walks before sunrise", []string{"sunrise", "running", "club"}, nil, false},
		// Final verification pass. A word history never used still counts
		// toward the request's size: dropping it would let one shared rare word
		// through ("the energy rating of the dishwasher" against a private
		// "low energy" message).
		{"unknown word still counts toward the size", "I've had low energy for a few days.", []string{"energy", "rating", "dishwasher"}, nil, false},
		{"mostly unknown request keeps two terms", "My passport is valid until March 2029.", []string{"renew", "expiring", "passport"}, nil, false},
		// A request whose every word history knows may be explained by its
		// uniquely rarest word alone when that word is nearly unique in history.
		{"strong single distinctive match", "Our vet is Dr. Rivera at Oakwood.", []string{"vet", "use", "cat", "remind"}, nil, true},
		{"single match above the strong ceiling", "Our dentist is Dr. Rivera at Oakwood.", []string{"dentist", "use", "cat", "remind"}, nil, false},
		{"single match beside an unknown word", "My blood test results came back.", []string{"summarize", "today", "ci", "results"}, nil, false},
		{"single match that is not the rarest word", "Book the car service before the check.", []string{"vet", "use", "cat", "book"}, nil, false},
		{"single match on a word tied for rarest", "Sam forgot our plans and I didn't say anything.", []string{"doctor", "say", "ferritin"}, nil, false},
		// Words counted in at least relevanceCommonDocuments messages are common:
		// never distinctive, however the request's other words compare.
		{"common words are never distinctive", "My weekly routine starts on Tuesday.", []string{"routine", "weekly"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := relevantToRequest(tc.text, &memory.RetrievalRelevance{Current: tc.current, Context: tc.context}, frequency)
			if got != tc.want {
				t.Fatalf("relevantToRequest = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestRelevanceKeyFoldsInflection(t *testing.T) {
	for _, group := range [][]string{
		{"run", "runs", "running"},
		{"book", "books", "booked", "booking"},
		{"date", "dates", "dated"},
		{"settle", "settled", "settles"},
		{"plan", "plans", "planned", "planning"},
		{"class", "classes"},
		{"fall", "falls"},
	} {
		for _, word := range group[1:] {
			if relevanceKey(word) != relevanceKey(group[0]) {
				t.Errorf("relevanceKey(%q)=%q, want %q", word, relevanceKey(word), relevanceKey(group[0]))
			}
		}
	}
}

func TestRelevanceRejectsMalformedTerms(t *testing.T) {
	for _, relevance := range []*memory.RetrievalRelevance{
		{Current: []string{"Upper"}},
		{Current: []string{"two words"}},
		{Current: []string{`quo"te`}},
		{Current: []string{""}},
		{Context: make([][]string, relevanceGroupLimit+1)},
	} {
		if validRetrievalRelevance(relevance) {
			t.Errorf("accepted malformed relevance %+v", relevance)
		}
	}
	if !validRetrievalRelevance(&memory.RetrievalRelevance{Current: []string{"café", "21k"}, Context: [][]string{{"lisbon"}}}) {
		t.Error("rejected well-formed relevance")
	}
}
