package eviedb

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// newerStatementFamily is one subject and Predicate family among the Claims a
// read selected, with every label its token versions use (harness review M3,
// M4). It finds later owner statements that may update those Claims.
type newerStatementFamily struct {
	subject memory.SemanticID
	token   string
	labels  []string
	query   string
	owner   bool
	name    string
	wording claimWording
}

func newNewerStatementFamily(ctx context.Context, q semanticInspectionQueryer, claims []memory.SemanticClaim, query string) (newerStatementFamily, error) {
	f := newerStatementFamily{subject: claims[0].SubjectEntityID, token: claims[0].Predicate.Token, query: query}
	subject, err := loadSemanticEntityForInspection(ctx, q, f.subject)
	if err != nil {
		return f, err
	}
	f.owner, f.name = subject.AnchorKind == "owner", subject.CanonicalName
	f.wording.addSubject(subject.CanonicalName, f.owner)
	f.wording.addPredicate(f.token)
	for _, claim := range claims {
		if !containsString(f.labels, claim.Predicate.Label) {
			f.labels = append(f.labels, claim.Predicate.Label)
			f.wording.addPredicate(claim.Predicate.Label)
		}
		if claim.Object.Literal != nil {
			f.wording.addValue(string(claim.Object.Literal.Kind), claim.Object.Literal.Value)
		} else if claim.Object.EntityID != "" {
			object, err := loadSemanticEntityForInspection(ctx, q, claim.Object.EntityID)
			if err != nil {
				return f, err
			}
			f.wording.addValue("entity", object.CanonicalName)
		}
	}
	return f, nil
}

func (f newerStatementFamily) phrases() []string {
	phrases := append([]string{f.token}, f.labels...)
	if f.query != "" {
		phrases = append(phrases, f.query)
	}
	return phrases
}

func ftsAny(parts []string) string {
	return "(" + strings.Join(parts, " OR ") + ")"
}

// match is the bounded FTS suggestion query: the Predicate or query phrase
// (the pre-Stage 13 rule), a saved value with a change cue, or the Predicate's
// words with a change or novelty cue. window re-checks every hit.
func (f newerStatementFamily) match() string {
	var phrases []string
	for _, phrase := range f.phrases() {
		if len(wordingWords(phrase)) > 0 {
			phrases = append(phrases, retrievalPhrase(phrase))
		}
	}
	var cues []string
	for cue := range wordingChangeCues {
		cues = append(cues, `"`+cue+`"`)
	}
	for _, phrase := range wordingChangePhrases {
		cues = append(cues, `"`+phrase[0]+" "+phrase[1]+`"`)
	}
	sort.Strings(cues)
	alternatives := []string{ftsAny(phrases)}
	var values []string
	// FTS matches exact unicode61 tokens, so the phrase uses the value's own
	// words; window re-checks the folded form.
	for _, value := range f.wording.valueWords {
		values = append(values, `"`+strings.Join(value, " ")+`"`)
	}
	if len(values) > 0 {
		alternatives = append(alternatives, "("+ftsAny(values)+" AND "+ftsAny(cues)+")")
	}
	var predicates []string
	for _, group := range f.wording.predicates {
		if len(group) > 6 {
			group = group[:6]
		}
		if len(group) == 1 {
			predicates = append(predicates, `"`+group[0]+`"*`)
			continue
		}
		for i := range group {
			for j := i + 1; j < len(group); j++ {
				predicates = append(predicates, `("`+group[i]+`"* AND "`+group[j]+`"*)`)
			}
		}
	}
	if len(predicates) > 0 {
		alternatives = append(alternatives, "("+ftsAny(predicates)+" AND "+ftsAny(append(cues, `"`+wordingNoveltyCue+`"`))+")")
	}
	match := ftsAny(alternatives)
	if !f.owner {
		match = "(" + match + " AND " + retrievalPhrase(f.name) + ")"
	}
	return match
}

func containsExactWords(words []wordingWord, phrase string) bool {
	sequence := wordingWords(phrase)
	for i := 0; len(sequence) > 0 && i+len(sequence) <= len(words); i++ {
		matched := true
		for j, word := range sequence {
			if words[i+j].text != word.text {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// window selects the excerpt of an unrepresented owner span that states a
// possible update. The Predicate or query phrase keeps the earlier rule and
// excerpt choice unchanged; otherwise one declarative sentence must satisfy
// claimWording.newerStatement, and the excerpt starts near that sentence.
func (f newerStatementFamily) window(content string, spans []conversationReadSpan) (conversationReadSpan, bool) {
	words := wordingWords(content)
	phrased := false
	for _, phrase := range f.phrases() {
		phrased = phrased || containsExactWords(words, phrase)
	}
	if phrased && (f.owner || containsExactWords(words, f.name)) {
		return chooseConversationReadExcerpt(content, strings.Join(f.phrases(), " "), spans)
	}
	for _, span := range spans {
		sentence, ok := firstSentence(content, span.start, span.end, f.wording.newerStatement)
		if !ok {
			continue
		}
		start := max(span.start, sentence.start-200)
		for start < span.end && !utf8.RuneStart(content[start]) {
			start++
		}
		end := min(span.end, start+conversationExcerptBytes)
		for end > start && !utf8.ValidString(content[start:end]) {
			end--
		}
		span.evidenceSpan = evidenceSpan{start, end}
		return span, true
	}
	return conversationReadSpan{}, false
}
