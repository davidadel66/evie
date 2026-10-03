package memorybackground

import (
	"sort"
	"strings"
	"unicode"

	"github.com/davidadel66/evie/internal/memory"
)

// similarityFloor is the least score that makes an existing memory worth
// showing with a candidate: the same Predicate token, or a shared content
// word in the label or text.
const similarityFloor = 0.25

// stopWords are English function words and the owner anchor every Claim
// text starts with; they say nothing about whether two memories match.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "to": true, "in": true, "on": true, "for": true, "and": true, "or": true,
	"my": true, "i": true, "me": true, "is": true, "are": true, "be": true, "with": true, "at": true, "by": true, "it": true,
	"this": true, "that": true, "owner": true, "denied": true, "not": true, "no": true, "as": true, "from": true, "about": true,
	"s": true, "do": true, "does": true, "should": true, "always": true, "prefer": true, "prefers": true, "like": true, "likes": true,
}

func contentWords(text string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if stopWords[word] {
			continue
		}
		if len(word) > 3 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") {
			word = strings.TrimSuffix(word, "s")
		}
		words[word] = true
	}
	return words
}

func jaccard(left, right map[string]bool) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	shared := 0
	for word := range left {
		if right[word] {
			shared++
		}
	}
	return float64(shared) / float64(len(left)+len(right)-shared)
}

func normalizedValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

// similarityScore is a deterministic lexical score between a candidate and
// an existing memory: the same Predicate token counts most, then overlap of
// the Predicate labels, the values and the whole text. dense is true when
// the dense (or full-text) retriever also returned the memory.
func similarityScore(c candidate, m memory.BackgroundExistingMemory, dense bool) float64 {
	score := 0.0
	if m.PredicateToken == c.Predicate {
		score += 3
	}
	score += 2 * jaccard(contentWords(c.Label+" "+strings.ReplaceAll(c.Predicate, "_", " ")), contentWords(m.PredicateLabel+" "+strings.ReplaceAll(m.PredicateToken, "_", " ")))
	if m.Literal != nil {
		if normalizedValue(m.Literal.Value) == normalizedValue(c.Literal.Value) {
			score += 1
		}
		score += jaccard(contentWords(c.Literal.Value), contentWords(m.Literal.Value))
	}
	score += jaccard(contentWords(c.text()), contentWords(m.Text))
	if dense {
		score += 0.5
	}
	return score
}

// rankSimilar returns the existing memories most similar to a candidate,
// best first, at most limit, above the floor.
func rankSimilar(c candidate, pool []memory.BackgroundExistingMemory, dense map[memory.SemanticID]bool, limit int) []memory.BackgroundExistingMemory {
	type scored struct {
		memory memory.BackgroundExistingMemory
		score  float64
		order  int
	}
	var ranked []scored
	for i, m := range pool {
		score := similarityScore(c, m, m.ClaimID != "" && dense[m.ClaimID])
		if score >= similarityFloor {
			ranked = append(ranked, scored{memory: m, score: score, order: i})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	similar := make([]memory.BackgroundExistingMemory, len(ranked))
	for i := range ranked {
		similar[i] = ranked[i].memory
	}
	return similar
}

// exactDuplicate finds an existing memory that already states the candidate
// (same Predicate token or label, same value, same polarity): a NOOP that
// needs no model call. A Global memory already covers a Workspace candidate.
func exactDuplicate(c candidate, similar []memory.BackgroundExistingMemory) *memory.BackgroundExistingMemory {
	for i := range similar {
		m := similar[i]
		if m.Literal == nil || m.Polarity != c.Polarity || normalizedValue(m.Literal.Value) != normalizedValue(c.Literal.Value) {
			continue
		}
		if m.ScopeKey != c.TargetScope && m.ScopeKey != "global" {
			continue
		}
		if m.PredicateToken == c.Predicate || normalizedValue(m.PredicateLabel) == normalizedValue(c.Label) {
			return &similar[i]
		}
	}
	return nil
}

// sameMemory reports two candidates in one batch stating the same thing.
func sameMemory(left, right candidate) bool {
	return left.TargetScope == right.TargetScope && left.Polarity == right.Polarity &&
		(left.Predicate == right.Predicate || normalizedValue(left.Label) == normalizedValue(right.Label)) &&
		normalizedValue(left.Literal.Value) == normalizedValue(right.Literal.Value)
}
