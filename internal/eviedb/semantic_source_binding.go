package eviedb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Owner-span binding (harness review M5, Stage 14). A remembered value cites
// the owner message of its turn. Only the exact span of that message that
// contains the value carries owner authority, so the source shows David's own
// words, nothing else from the message reaches another Context Scope, and
// retirement suppresses that span rather than the whole message. A value the
// message does not contain is recorded as Evie-proposed: the message is kept
// as the request it was proposed in, by hash, and is never quoted.
//
// Matching is deterministic and decided once at Prepare time; the accepted
// operation records its outcome, so replay never re-runs these rules.
//
//   - Words fold case, punctuation and whitespace, and common inflection
//     (relevanceKey, plus -ves and -ies plurals); a value must appear as
//     consecutive words, so a paraphrase is Evie-proposed.
//   - Numbers compare by value: "1,500", "1500" and "1500.0" are equal, as are
//     the number words zero to twenty and their digits.
//   - A date also matches its month-name forms ("June 3", "3rd of June") and
//     its US numeric form ("6/3"); a stated year must agree. A datetime
//     matches by its UTC calendar date.
//   - A number word ("one", "two") counts only in a sentence that also holds
//     one of the Predicate's words, or in a short answer; digits count
//     anywhere.
//   - A boolean has no words of its own: it binds to a declarative sentence
//     containing min(2, n) of a Predicate wording's n content words, in the
//     first person when the owner is the subject.
//   - An Entity counts only through a name or Alias that appears in the
//     message. Owner, Evie and Context anchors are implied by the speaker.
//
// Several sentences may hold the value; only one that states the Claim
// qualifies (harness review final pass, confirmation review). Clauses, their
// subjects, negation scope, reported speech and conditionals come from the
// clause analysis shared with the wording rules
// (retrieval_wording_clauses.go). Each clause holding the value must
//
//   - be a statement: not quoted or reported ("My friend said 'I live in
//     Boston'", "The doctor says I'm allergic"), not conditional, wished or
//     future ("If I lived in Boston", "I wish", "I might", "I'd"), and not a
//     question;
//   - agree in polarity: an affirmed value never binds where its clause is
//     negated ("not", "never", "no", "n't", "hardly"; "don't forget", "never
//     forget", "not only" and "no doubt" are not negations; a "not" right
//     after the value negates only what follows, "Cambridge not Boston"), a
//     denied or false value binds only to a negated clause, and a clause with
//     two negations ("It's not that I don't live in Boston") binds neither;
//   - not say an affirmed value no longer holds ("used to", "no longer",
//     "left", "quit", "dropped", "former", "moved from");
//   - for an owner Claim, not be about someone else: its subject is not
//     "he", "she", "you", "my sister", "my therapist", "our team", "the
//     doctor", "Selma likes" or a possessive chain ("my dad's").
//
// A window also needs one of the Predicate's words, the owner speaking (the
// owner as the clause's subject, or a first-person word that is not someone
// else's possessive, such as "me" or "my bank"), an explicit memory cue
// ("remember", "note", "save", "memorize", "correction"), or, for the whole
// message, a short answer: one sentence of at most bindingTerseWords words
// whose other words are answer words or a negated clause ("Boston.", "Teal,
// please.", "No, it's Chicago.", "Chicago, not Boston."). A boolean or other
// wordless owner value needs the owner speaking.
//
// Among qualifying windows the narrowest wins, then the one with the most
// Predicate words, then the owner as subject over a first-person word or a
// short answer, then a memory cue, then the shortest span (so the least
// extra text is quoted), then the latest in the message. With none, the
// value is Evie-proposed rather than bound to an unrelated sentence. Every
// rule is a heuristic and fails safe: when unsure, the value is
// Evie-proposed, which still needs approval and costs only the label.
//
// The span is the chosen window: the sentence containing the value (every
// sentence it touches, when the value itself runs across sentences), or for
// an Entity Claim the sentence or two adjacent sentences naming both its
// subject and object, trimmed of surrounding whitespace. A sentence longer
// than ownerSpanMaxBytes narrows to the matches and up to
// ownerSpanContextWords words either side. A span covering the entire
// message is cited as whole content, which is what it denotes.

const (
	ownerSpanMaxSentences = 2
	ownerSpanMaxBytes     = 600
	ownerSpanContextWords = 8
)

type ownerSourceBinding struct {
	kind      memory.EvidenceLocatorKind
	value     string
	evidence  string
	hash      string
	authority memory.SourceAuthority
}

// bindingOccurrences lists where one required item occurs, as byte ranges.
type bindingOccurrences [][2]int

// bindingClaim is what one proposition requires of the owner's words.
type bindingClaim struct {
	needs     []bindingOccurrences // every item the words must contain
	predicate bindingPredicate
	// ownerSubject: the owner anchor is the subject, so the first person
	// names it.
	ownerSubject bool
	// negative: a denied Claim, or a false boolean; only a negated clause
	// states it.
	negative bool
	// explicit: the value has no words of its own, so only a declarative
	// sentence (in the first person, for the owner) states it.
	explicit bool
}

// literalBindingClaim is the binding requirement of a Typed Literal Claim
// about the owner.
func literalBindingClaim(content string, literal memory.TypedLiteral, polarity memory.ClaimPolarity, predicateToken, predicateLabel string) bindingClaim {
	claim := bindingClaim{predicate: newBindingPredicate(predicateToken, predicateLabel), ownerSubject: true}
	claim.needs = literalOccurrences(content, literal, claim.predicate)
	claim.setPolarity(&literal, polarity)
	return claim
}

func (c *bindingClaim) setPolarity(literal *memory.TypedLiteral, polarity memory.ClaimPolarity) {
	c.negative = polarity == memory.PolarityDenied
	if literal != nil && literal.Kind == memory.LiteralBoolean {
		c.explicit = true
		if literal.Value == "false" {
			c.negative = !c.negative
		}
	}
}

// bindingPredicate holds each Predicate wording's distinct content words, one
// group per wording, folded like value words.
type bindingPredicate [][][]string

func newBindingPredicate(wordings ...string) bindingPredicate {
	var predicate bindingPredicate
	for _, wording := range wordings {
		wording = strings.ReplaceAll(wording, "_", " ")
		var group [][]string
		seen := map[string]bool{}
		for _, token := range bindingTokens(wording) {
			if wordingFunctionWords[strings.ToLower(wording[token.start:token.end])] || seen[token.keys[0]] {
				continue
			}
			seen[token.keys[0]] = true
			group = append(group, token.keys)
		}
		if len(group) != 0 {
			predicate = append(predicate, group)
		}
	}
	return predicate
}

// groupWords returns the tokens holding one group's words and how many of the
// group's words they cover.
func groupWords(group [][]string, tokens []bindingToken) (held []bindingToken, count int) {
	for _, keys := range group {
		found := false
		for _, token := range tokens {
			if keysIntersect(token.keys, keys) {
				held = append(held, token)
				found = true
			}
		}
		if found {
			count++
		}
	}
	return held, count
}

// words returns the largest number of one group's words the tokens hold.
func (p bindingPredicate) words(tokens []bindingToken) int {
	best := 0
	for _, group := range p {
		_, count := groupWords(group, tokens)
		best = max(best, count)
	}
	return best
}

// covering returns the byte range from the first to the last word of a group
// whose requirement of min(2, n) of its n words the tokens cover.
func (p bindingPredicate) covering(tokens []bindingToken) ([2]int, bool) {
	for _, group := range p {
		held, count := groupWords(group, tokens)
		if count == 0 || count < min(2, len(group)) {
			continue
		}
		span := [2]int{held[0].start, held[0].end}
		for _, token := range held[1:] {
			span[0], span[1] = min(span[0], token.start), max(span[1], token.end)
		}
		return span, true
	}
	return [2]int{}, false
}

// bindingToken is one word with its byte range and the keys it folds to. Two
// words match when their keys intersect.
type bindingToken struct {
	start, end int
	keys       []string
}

var bindingNumberWords = map[string]string{
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7",
	"eight": "8", "nine": "9", "ten": "10", "eleven": "11", "twelve": "12", "thirteen": "13", "fourteen": "14",
	"fifteen": "15", "sixteen": "16", "seventeen": "17", "eighteen": "18", "nineteen": "19", "twenty": "20",
}

var bindingThousands = regexp.MustCompile(`^\d{1,3}(,\d{3})+(\.\d+)?$`)
var bindingPlainNumber = regexp.MustCompile(`^\d+(\.\d+)?$`)

func isBindingWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// bindingTokens splits text into words with byte offsets. A '.' or ',' between
// two digits stays inside a number, so "1,500" and "3.12" are single tokens.
func bindingTokens(text string) []bindingToken {
	var tokens []bindingToken
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !isBindingWordRune(r) {
			i += size
			continue
		}
		start := i
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if isBindingWordRune(r) {
				i += size
				continue
			}
			if (r == '.' || r == ',') && i > start && isASCIIDigit(text[i-1]) && i+1 < len(text) && isASCIIDigit(text[i+1]) {
				i += size
				continue
			}
			break
		}
		tokens = append(tokens, bindingToken{start: start, end: i, keys: bindingKeySet(text[start:i])})
	}
	return tokens
}

// bindingKeySet folds one word. Besides relevanceKey's inflection folding, a
// plural in -ves may stand for -f or -fe ("scarves", "knives") and one in -ies
// for -y ("cities"); each reading is kept, so "gloves" still matches "glove".
func bindingKeySet(raw string) []string {
	if number, ok := canonicalBindingNumber(raw); ok {
		return []string{"num:" + number}
	}
	lower := strings.ToLower(raw)
	if digits, ok := bindingNumberWords[lower]; ok {
		return []string{"num:" + digits}
	}
	keys := []string{relevanceKey(lower)}
	add := func(word string) {
		key := relevanceKey(word)
		for _, existing := range keys {
			if existing == key {
				return
			}
		}
		keys = append(keys, key)
	}
	if stem, ok := strings.CutSuffix(lower, "ves"); ok && len(stem) >= 2 {
		add(stem + "f")
		add(stem + "fe")
	}
	if stem, ok := strings.CutSuffix(lower, "ies"); ok && len(stem) >= 2 {
		add(stem + "y")
	}
	return keys
}

func keysIntersect(left, right []string) bool {
	for _, a := range left {
		for _, b := range right {
			if a == b {
				return true
			}
		}
	}
	return false
}

func canonicalBindingNumber(raw string) (string, bool) {
	switch {
	case bindingThousands.MatchString(raw):
		raw = strings.ReplaceAll(raw, ",", "")
	case !bindingPlainNumber.MatchString(raw):
		return "", false
	}
	whole, fraction, _ := strings.Cut(raw, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return whole, true
	}
	return whole + "." + fraction, true
}

func bindingKeys(text string) [][]string {
	tokens := bindingTokens(text)
	keys := make([][]string, len(tokens))
	for i, token := range tokens {
		keys[i] = token.keys
	}
	return keys
}

// sequenceOccurrences finds keys as consecutive tokens.
func sequenceOccurrences(tokens []bindingToken, keys [][]string) bindingOccurrences {
	var found bindingOccurrences
	if len(keys) == 0 {
		return found
	}
	for i := 0; i+len(keys) <= len(tokens); i++ {
		matched := true
		for j, key := range keys {
			if !keysIntersect(tokens[i+j].keys, key) {
				matched = false
				break
			}
		}
		if matched {
			found = append(found, [2]int{tokens[i].start, tokens[i+len(keys)-1].end})
		}
	}
	return found
}

func phraseOccurrences(tokens []bindingToken, phrases ...string) bindingOccurrences {
	var found bindingOccurrences
	for _, phrase := range phrases {
		found = append(found, sequenceOccurrences(tokens, bindingKeys(phrase))...)
	}
	return found
}

var bindingMonths = [...][]string{
	{"january", "jan"}, {"february", "feb"}, {"march", "mar"}, {"april", "apr"}, {"may"}, {"june", "jun"},
	{"july", "jul"}, {"august", "aug"}, {"september", "sept", "sep"}, {"october", "oct"}, {"november", "nov"}, {"december", "dec"},
}

// dateOccurrences finds a calendar date's spoken and numeric forms. A form
// that names a different year does not match.
func dateOccurrences(text string, tokens []bindingToken, date time.Time) bindingOccurrences {
	found := phraseOccurrences(tokens, date.Format("2006-01-02"))
	month := strings.Join(bindingMonths[date.Month()-1], "|")
	day := strconv.Itoa(date.Day())
	year := strconv.Itoa(date.Year())
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:` + month + `)\.?\s+0?` + day + `(?:st|nd|rd|th)?\b(?:,?\s+(\d{4})\b)?`),
		regexp.MustCompile(`(?i)\b0?` + day + `(?:st|nd|rd|th)?\s+(?:of\s+)?(?:` + month + `)\b\.?(?:,?\s+(\d{4})\b)?`),
		regexp.MustCompile(`\b0?` + strconv.Itoa(int(date.Month())) + `/0?` + day + `(?:/(\d{4}|\d{2}))?\b`),
	}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
			if match[2] >= 0 {
				stated := text[match[2]:match[3]]
				if stated != year && !(len(stated) == 2 && strings.HasSuffix(year, stated)) {
					continue
				}
			}
			found = append(found, [2]int{match[0], match[1]})
		}
	}
	return found
}

// predicateOccurrences returns, in each sentence containing min(2, n) of one
// Predicate wording's n content words, the range from the first to the last
// of those words. It stands in for a value that has no words of its own.
func predicateOccurrences(text string, predicate bindingPredicate) bindingOccurrences {
	var found bindingOccurrences
	for _, sentence := range wordingSentences(text) {
		if span, ok := predicate.covering(bindingTokensIn(text, sentence.start, sentence.end)); ok {
			found = append(found, span)
		}
	}
	return found
}

// bindingTokensIn tokenizes text[start:end] with offsets into text.
func bindingTokensIn(text string, start, end int) []bindingToken {
	tokens := bindingTokens(text[start:end])
	for i := range tokens {
		tokens[i].start += start
		tokens[i].end += start
	}
	return tokens
}

// quantityContext drops a number-word occurrence ("one", "two") whose
// sentence holds none of the Predicate's words: "No one told me" does not
// state a floor of 1. Digits are deliberate and always count, and so does a
// number word in a short answer ("Three, please."), which the window must
// still find to be one.
func quantityContext(text string, found bindingOccurrences, predicate bindingPredicate) bindingOccurrences {
	sentences := wordingSentences(text)
	if len(sentences) == 1 && len(sentences[0].words) <= bindingTerseWords {
		return found
	}
	kept := found[:0]
	for _, occurrence := range found {
		spelled := false
		for _, token := range bindingTokensIn(text, occurrence[0], occurrence[1]) {
			_, spelled = bindingNumberWords[strings.ToLower(text[token.start:token.end])]
			if spelled {
				break
			}
		}
		if spelled {
			index := sentenceIndex(sentences, occurrence[0])
			if index < 0 || predicate.words(bindingTokensIn(text, sentences[index].start, sentences[index].end)) == 0 {
				continue
			}
		}
		kept = append(kept, occurrence)
	}
	return kept
}

func numericKeys(keys [][]string) bool {
	for _, key := range keys {
		if len(key) != 1 || !strings.HasPrefix(key[0], "num:") {
			return false
		}
	}
	return len(keys) > 0
}

// literalOccurrences is the single required item of a Typed Literal Claim.
// It returns nil when the literal has no words to look for.
func literalOccurrences(text string, literal memory.TypedLiteral, predicate bindingPredicate) []bindingOccurrences {
	tokens := bindingTokens(text)
	var found bindingOccurrences
	switch literal.Kind {
	case memory.LiteralBoolean:
		found = predicateOccurrences(text, predicate)
	case memory.LiteralDate:
		date, err := time.Parse("2006-01-02", literal.Value)
		if err != nil {
			return nil
		}
		found = dateOccurrences(text, tokens, date)
	case memory.LiteralDatetime:
		instant, err := time.Parse(time.RFC3339Nano, literal.Value)
		if err != nil {
			return nil
		}
		found = append(phraseOccurrences(tokens, literal.Value), dateOccurrences(text, tokens, instant.UTC())...)
	default:
		keys := bindingKeys(literal.Value)
		if len(keys) == 0 {
			return nil
		}
		found = sequenceOccurrences(tokens, keys)
		if numericKeys(keys) {
			found = quantityContext(text, found, predicate)
		}
	}
	return []bindingOccurrences{found}
}

// Closed English lists for choosing the sentence that states a Claim. No
// topic dictionary or learned score is involved. Clauses, negation scope,
// reported speech, conditionals, change words and the clause's subject come
// from the analysis shared with the wording rules
// (retrieval_wording_clauses.go).
var (
	// "so" starts a clause only before its subject ("so I do not have to"),
	// not as an adverb ("not so sure").
	bindingSoSubjects = map[string]bool{"i": true, "we": true, "you": true, "he": true, "she": true, "they": true, "it": true, "that": true}
	bindingMemoryCues = map[string]bool{"remember": true, "note": true, "save": true, "memorize": true, "memorise": true, "correction": true}
	// The other words of a short answer: "It's Boston.", "Teal, please.",
	// "No, it's Chicago.", "Actually Chicago.", "Fix that: Chicago."
	bindingAnswerWords = setOf("it", "its", "s", "is", "was", "that", "thats", "the", "a", "an", "please", "thanks", "thank", "you", "yes", "yeah",
		"yep", "no", "nope", "actually", "correction", "wrong", "fix", "just", "ok", "okay", "sure", "oh", "so", "well", "now", "still", "definitely",
		"mine", "here", "um", "hmm", "right")
)

// A message of at most this many words, one sentence and one value, whose
// other words are answer words, is the owner answering with the value.
const bindingTerseWords = 6

// vocabulary is the Claim's words for the shared clause analysis: its
// Predicate words, and the words of every required item's occurrences (the
// value, or an Entity's name).
func (c bindingClaim) vocabulary() wordingVocabulary {
	predicate := func(word wordingWord) bool {
		keys := bindingKeySet(word.text)
		for _, group := range c.predicate {
			for _, groupKeys := range group {
				if keysIntersect(keys, groupKeys) {
					return true
				}
			}
		}
		return false
	}
	return wordingVocabulary{owner: c.ownerSubject, predicate: predicate, claim: func(word wordingWord) bool {
		if predicate(word) {
			return true
		}
		for _, need := range c.needs {
			for _, occurrence := range need {
				if word.start >= occurrence[0] && word.end <= occurrence[1] {
					return true
				}
			}
		}
		return false
	}}
}

// occurrenceStates checks the clauses of sentence s holding an occurrence. A
// clause states the Claim only if it is not reported, conditional or a
// question; holds at most one negation that reaches the occurrence (two make
// it unsure, so neither polarity binds); is not negated or a change ("used
// to", "left", "no longer") for an affirmed value; and, for an owner Claim,
// is not about someone else. ownerSubject reports that the owner is the
// subject of every such clause.
func (c bindingClaim) occurrenceStates(s wordingSentence, occurrence [2]int, v wordingVocabulary) (ok, negated, ownerSubject bool) {
	from, to := -1, -1
	for i, word := range s.words {
		if word.end > occurrence[0] && word.start < occurrence[1] {
			if from < 0 {
				from = i
			}
			to = i + 1
		}
	}
	if from < 0 {
		return true, false, true // a separator only; the touched words decide
	}
	ownerSubject = true
	for n := s.words[from].clause; n <= s.words[to-1].clause; n++ {
		clause := s.clauses[n]
		if clause.reported || clause.conditional || clause.question {
			return false, false, false
		}
		count := s.negations(n, from, to)
		if count > 1 || !c.negative && (count == 1 || clause.change) {
			return false, false, false
		}
		negated = negated || count == 1
		switch s.subject(n, v) {
		case subjectOther:
			return false, false, false
		case subjectPronoun:
			if c.ownerSubject {
				return false, false, false
			}
			ownerSubject = false
		case subjectOwner:
		default:
			ownerSubject = false
		}
	}
	return true, negated, ownerSubject
}

// pickOccurrences chooses, for each required item, one occurrence inside
// sentences [first,last] whose clauses state the Claim, preferring one with
// the owner as subject. An affirmed Claim needs every chosen clause
// un-negated; a negative one needs at least one negated.
func (c bindingClaim) pickOccurrences(sentences []wordingSentence, first, last int, v wordingVocabulary) (chosen [][2]int, ownerSubject, ok bool) {
	low, high := sentences[first].start, sentences[last].end
	chosen = make([][2]int, 0, len(c.needs))
	negated := false
	ownerSubject = true
	for _, need := range c.needs {
		picked, found, pickedNegated, pickedOwner := [2]int{}, false, false, false
		for _, occurrence := range need {
			if occurrence[0] < low || occurrence[1] > high {
				continue
			}
			states, isNegated, isOwner := true, false, true
			for _, sentence := range sentences[first : last+1] {
				if occurrence[1] <= sentence.start || occurrence[0] >= sentence.end {
					continue
				}
				sentenceOK, sentenceNegated, sentenceOwner := c.occurrenceStates(sentence, occurrence, v)
				states, isNegated, isOwner = states && sentenceOK, isNegated || sentenceNegated, isOwner && sentenceOwner
			}
			if !states {
				continue
			}
			better := !found || c.negative && isNegated && !pickedNegated || isNegated == pickedNegated && isOwner && !pickedOwner
			if better {
				picked, found, pickedNegated, pickedOwner = occurrence, true, isNegated, isOwner
			}
		}
		if !found {
			return nil, false, false
		}
		chosen = append(chosen, picked)
		negated = negated || pickedNegated
		ownerSubject = ownerSubject && pickedOwner
	}
	return chosen, ownerSubject, !c.negative || negated
}

// ownerReference reports a first-person word in a clause that is neither
// reported nor about someone else: "me" in "Wake me at 6:30", "my" in "Save
// my bank: Chase". A possessive naming a person ("my sister") does not
// count.
func ownerReference(sentences []wordingSentence, v wordingVocabulary) bool {
	for _, s := range sentences {
		for n, clause := range s.clauses {
			if clause.reported {
				continue
			}
			if subject := s.subject(n, v); subject == subjectOther || subject == subjectPronoun {
				continue
			}
			for i := clause.first; i < clause.last; i++ {
				word := s.words[i].text
				if wordingOwnerPronouns[word] {
					return true
				}
				if word == "my" || word == "our" {
					if _, other := s.possessiveOwner(i, clause.last, v); !other {
						return true
					}
				}
			}
		}
	}
	return false
}

// terse reports a short answer whose content is the value: the message is
// one sentence of at most bindingTerseWords words, not a question, and each
// of its other words is an answer word or sits in a negated clause of its own
// ("Chicago, not Boston."). It is the owner answering, so it binds as the
// owner's statement (confirmation review).
func (c bindingClaim) terse(sentences []wordingSentence, chosen [][2]int) bool {
	if len(sentences) != 1 || len(c.needs) != 1 || c.explicit || len(chosen) != 1 {
		return false
	}
	s := sentences[0]
	if len(s.words) > bindingTerseWords || s.question {
		return false
	}
	for _, word := range s.words {
		if word.end > chosen[0][0] && word.start < chosen[0][1] || bindingAnswerWords[word.text] {
			continue
		}
		clause := s.clauses[word.clause]
		valueClause := false
		for _, other := range s.words[clause.first:clause.last] {
			valueClause = valueClause || other.end > chosen[0][0] && other.start < chosen[0][1]
		}
		if valueClause || s.negations(word.clause, len(s.words), len(s.words)+1) == 0 {
			return false
		}
	}
	return true
}

// bindingWindow is one candidate span: the sentences [first,last] and the
// occurrence chosen for each required item, ranked by how well it states the
// Claim (higher is better, compared in order).
type bindingWindow struct {
	first, last int
	chosen      [][2]int
	rank        [6]int
}

func (w bindingWindow) better(other bindingWindow) bool {
	for i := range w.rank {
		if w.rank[i] != other.rank[i] {
			return w.rank[i] > other.rank[i]
		}
	}
	return false
}

// window ranks the sentences [first,last] as the statement of the Claim, or
// reports that they do not state it. Rank, in order: fewer sentences, more
// Predicate words, the owner as the clause's subject (2) or a first-person
// word or short answer (1), a memory cue, a shorter span, then the later
// position. Shorter before later is the privacy tie-break: the least extra
// text is quoted (confirmation review).
func (c bindingClaim) window(content string, sentences []wordingSentence, first, last int, v wordingVocabulary) (bindingWindow, bool) {
	chosen, ownerSubject, ok := c.pickOccurrences(sentences, first, last, v)
	if !ok {
		return bindingWindow{}, false
	}
	low, high := sentences[first].start, sentences[last].end
	tokens := bindingTokensIn(content, low, high)
	cue := false
	for _, token := range tokens {
		cue = cue || bindingMemoryCues[strings.ToLower(content[token.start:token.end])]
	}
	owner := 0
	switch {
	case ownerSubject:
		owner = 2
	case ownerReference(sentences[first:last+1], v), len(sentences) == 1 && c.terse(sentences, chosen):
		owner = 1
	}
	predicateWords := c.predicate.words(tokens)
	if c.ownerSubject && (c.explicit && owner == 0 || predicateWords == 0 && owner == 0 && !cue) {
		return bindingWindow{}, false
	}
	start, end := trimSpan(content, low, high)
	return bindingWindow{first: first, last: last, chosen: chosen, rank: [6]int{
		-(last - first), predicateWords, owner, boolRank(cue), -(end - start), low,
	}}, true
}

func boolRank(value bool) int {
	if value {
		return 1
	}
	return 0
}

// bindOwnerSource decides the Source of a remembered value inside its owner
// message. Every required item must occur in one window that states the
// Claim; with no such window, or no required items, the value is
// Evie-proposed.
func bindOwnerSource(content string, claim bindingClaim) ownerSourceBinding {
	evieProposed := ownerSourceBinding{kind: memory.LocatorWhole, hash: evidenceHash(content), authority: memory.AuthorityEvieProposed}
	if len(claim.needs) == 0 {
		return evieProposed
	}
	for _, need := range claim.needs {
		if len(need) == 0 {
			return evieProposed
		}
	}
	sentences := wordingSentences(content)
	vocabulary := claim.vocabulary()
	var best bindingWindow
	found := false
	consider := func(first, last int) {
		if first < 0 || last < first {
			return
		}
		if window, ok := claim.window(content, sentences, first, last, vocabulary); ok && (!found || window.better(best)) {
			best, found = window, true
		}
	}
	if len(claim.needs) == 1 {
		// One value: each occurrence's window is the sentences it touches,
		// however many.
		for _, occurrence := range claim.needs[0] {
			consider(sentenceIndex(sentences, occurrence[0]), sentenceIndex(sentences, occurrence[1]-1))
		}
	} else {
		for width := 1; width <= ownerSpanMaxSentences; width++ {
			for first := 0; first+width <= len(sentences); first++ {
				consider(first, first+width-1)
			}
		}
	}
	if !found {
		return evieProposed
	}
	if binding, ok := spanBinding(content, sentences[best.first].start, sentences[best.last].end, best.chosen); ok {
		return binding
	}
	return evieProposed
}

func sentenceIndex(sentences []wordingSentence, offset int) int {
	for i, sentence := range sentences {
		if offset >= sentence.start && offset < sentence.end {
			return i
		}
	}
	return -1
}

func spanBinding(content string, low, high int, chosen [][2]int) (ownerSourceBinding, bool) {
	start, end := trimSpan(content, low, high)
	if end-start > ownerSpanMaxBytes {
		start, end = narrowSpan(content, low, high, chosen)
	}
	if start >= end {
		return ownerSourceBinding{}, false
	}
	if start == 0 && end == len(content) {
		// The span is the entire message: cite it as whole content, the same
		// locator an equivalent compiler candidate uses.
		return ownerSourceBinding{kind: memory.LocatorWhole, evidence: content, hash: evidenceHash(content), authority: memory.AuthorityOwnerStatement}, true
	}
	evidence := content[start:end]
	return ownerSourceBinding{kind: memory.LocatorUTF8ByteRange, value: fmt.Sprintf("%d:%d", start, end),
		evidence: evidence, hash: evidenceHash(evidence), authority: memory.AuthorityOwnerStatement}, true
}

func evidenceHash(text string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(text)))
}

func trimSpan(content string, start, end int) (int, int) {
	for start < end {
		r, size := utf8.DecodeRuneInString(content[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		start += size
	}
	for end > start {
		r, size := utf8.DecodeLastRuneInString(content[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	return start, end
}

// narrowSpan keeps an over-long sentence to its matches plus a few words of
// context on each side, within the chosen window.
func narrowSpan(content string, low, high int, chosen [][2]int) (int, int) {
	start, end := chosen[0][0], chosen[0][1]
	for _, occurrence := range chosen[1:] {
		start, end = min(start, occurrence[0]), max(end, occurrence[1])
	}
	tokens := bindingTokens(content[low:high])
	before, after := 0, 0
	for i := len(tokens) - 1; i >= 0 && before < ownerSpanContextWords; i-- {
		if low+tokens[i].start < start {
			start = low + tokens[i].start
			before++
		}
	}
	for _, token := range tokens {
		if after >= ownerSpanContextWords {
			break
		}
		if low+token.end > end {
			end = low + token.end
			after++
		}
	}
	return start, end
}

func (b ownerSourceBinding) apply(source *memory.SemanticSource) {
	source.LocatorKind, source.LocatorValue = b.kind, b.value
	source.Evidence, source.EvidenceSHA256, source.Authority = b.evidence, b.hash, b.authority
}

// validateRememberSourceShape accepts exactly the owner-message Source shapes
// a remember proposal can carry: a bound span with owner authority, a whole
// message with owner authority (a span covering the entire message, and every
// pre-Stage-14 remember operation in accepted history), or an Evie-proposed
// whole-message reference that quotes nothing.
func validateRememberSourceShape(source memory.SemanticSource) error {
	if source.EventPart != memory.EvidenceContent || source.Actor != memory.SemanticActorOwner ||
		source.SourceType != memory.SourceTypeUserMessage || source.Eligibility != memory.EligibilityEligible {
		return errors.New("remember proposal source is not an owner message")
	}
	switch {
	case source.LocatorKind == memory.LocatorUTF8ByteRange && source.Authority == memory.AuthorityOwnerStatement && source.Evidence != "":
	case source.LocatorKind == memory.LocatorWhole && source.LocatorValue == "" && source.Authority == memory.AuthorityEvieProposed && source.Evidence == "":
	case source.LocatorKind == memory.LocatorWhole && source.LocatorValue == "" && source.Authority == memory.AuthorityOwnerStatement:
	default:
		return errors.New("remember proposal source locator and authority do not agree")
	}
	return nil
}

// verifyRememberSourceEvidence checks a remember proposal's quoted evidence
// and hash against the immutable owner message.
func verifyRememberSourceEvidence(source memory.SemanticSource, content string) error {
	if err := validateRememberSourceShape(source); err != nil {
		return err
	}
	changed := errors.New("semantic source evidence changed")
	switch source.LocatorKind {
	case memory.LocatorWhole:
		if source.EvidenceSHA256 != evidenceHash(content) {
			return changed
		}
		if source.Authority == memory.AuthorityOwnerStatement && source.Evidence != content {
			return changed
		}
	case memory.LocatorUTF8ByteRange:
		startText, endText, ok := strings.Cut(source.LocatorValue, ":")
		start, startErr := strconv.Atoi(startText)
		end, endErr := strconv.Atoi(endText)
		if !ok || startErr != nil || endErr != nil || strconv.Itoa(start) != startText || strconv.Itoa(end) != endText ||
			start < 0 || end <= start || end > len(content) || !utf8.ValidString(content[:start]) || !utf8.ValidString(content[:end]) ||
			source.Evidence != content[start:end] || source.EvidenceSHA256 != evidenceHash(source.Evidence) {
			return changed
		}
	}
	return nil
}

// sourceReader is the Context Scope and session a source is rendered for.
type sourceReader struct{ context, session string }

func readerFromScope(scope memory.ScopeContext) sourceReader {
	return sourceReader{context: scopeKeyForContext(scope), session: "session:" + string(scope.SessionID)}
}

// readerFromAllowed recovers the reader from its exact allowed read scopes:
// global, at most one Workspace or project, and its session.
func readerFromAllowed(allowed []string) sourceReader {
	reader := sourceReader{context: "global"}
	for _, key := range allowed {
		switch {
		case strings.HasPrefix(key, "session:"):
			reader.session = key
		case key != "global":
			reader.context = key
		}
	}
	return reader
}

func readerFromAllowedSet(allowed map[string]struct{}) sourceReader {
	keys := make([]string, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	return readerFromAllowed(keys)
}

// wholeSourceNeedsNarrowing reports a whole-message owner statement shown
// outside the Context Scope it was said in. Remember operations accepted
// before Stage 14 cite the whole message this way.
func wholeSourceNeedsNarrowing(source memory.SemanticSource, reader sourceReader) bool {
	return source.Evidence != "" && source.LocatorKind == memory.LocatorWhole && source.Authority == memory.AuthorityOwnerStatement &&
		source.SourceType == memory.SourceTypeUserMessage && source.ScopeKey != reader.context && source.ScopeKey != reader.session
}

// narrowForeignWholeSource renders a whole-message owner Source read from
// another Context Scope as only the sentence holding its Claim's value, found
// with the same rules as owner-span binding; with no such sentence it renders
// no text. The Source's locator, hash and authority are unchanged: this is
// read-time rendering, not a rewrite of accepted history (harness review M5).
// Readers in the Source's own scope keep the whole message.
func narrowForeignWholeSource(ctx context.Context, q semanticInspectionQueryer, source *memory.SemanticSource, claim memory.SemanticClaim, reader sourceReader) error {
	if !wholeSourceNeedsNarrowing(*source, reader) {
		return nil
	}
	required, err := claimValueNeeds(ctx, q, source.Evidence, claim)
	if err != nil {
		return err
	}
	binding := bindOwnerSource(source.Evidence, required)
	source.Evidence = ""
	if binding.authority == memory.AuthorityOwnerStatement {
		source.Evidence = binding.evidence
	}
	return nil
}

// narrowForeignWholeSourceLink is narrowForeignWholeSource for a Source Link
// inspected on its own.
func narrowForeignWholeSourceLink(ctx context.Context, q semanticInspectionQueryer, source *memory.SemanticSource, reader sourceReader) error {
	if !wholeSourceNeedsNarrowing(*source, reader) {
		return nil
	}
	var claimID memory.SemanticID
	if err := q.QueryRowContext(ctx, `SELECT claim_id FROM semantic_source_links WHERE source_link_id = ?`, source.ID).Scan(&claimID); err != nil {
		return err
	}
	claim, err := loadSemanticClaim(ctx, q, claimID)
	if err != nil {
		return err
	}
	return narrowForeignWholeSource(ctx, q, source, claim, reader)
}

// Operation history renders owner text too (harness review final pass, M5):
// prepared and canonical operation JSON quote Sources (remember, correct,
// promote, compiler review) and the owner's request (lifecycle, promotion,
// graph link). Shown to a reader outside the Context Scope the text was said
// in, every quoted field follows the Source rule:
//
//   - text said in the reader's own Context Scope or session stays whole;
//   - text from another Workspace, project or session is never shown;
//   - a Global bound span (utf8_byte_range) stays: it is already only the
//     owner's words for its Claim;
//   - other Global text (a pre-Stage-14 whole-message Source, a request
//     message, compiler support or context) narrows to the sentence holding
//     the value of the Claim it supports (a quoted Source Link's own Claim,
//     else the inspected Claim), found with the owner-span rules, or to no
//     text when there is no such Claim or sentence;
//   - free text that cites no value (a compiler review edit's "reason", any
//     "note" or "comment", and an approval card's review-only identity
//     details, "example_claim" and "aliases") stays only for a reader in the
//     Context Scope it was written in (the nearest enclosing scope key; for
//     identity details, the operation's own Source scope) or its session,
//     and is otherwise blank (confirmation review).
//
// The stored operation is unchanged; only its rendering is narrowed.

// operationNarrower rewrites one inspection's operation JSON for its reader.
type operationNarrower struct {
	ctx    context.Context
	q      semanticInspectionQueryer
	reader sourceReader
	claim  *memory.SemanticClaim
}

type jsonMember struct {
	key   string
	value json.RawMessage
}

// Free-text members of operation JSON that quote the owner or describe other
// memories without citing a value.
var operationFreeText = setOf("reason", "note", "notes", "comment", "comments", "example_claim", "aliases")

func (n operationNarrower) narrow(operation *memory.SemanticOperationInspection) error {
	for _, field := range []*string{&operation.ProposalJSON, &operation.PreparedJSON, &operation.ResultJSON} {
		quotes := strings.Contains(*field, `"evidence"`)
		for key := range operationFreeText {
			quotes = quotes || strings.Contains(*field, `"`+key+`"`)
		}
		if !quotes {
			continue
		}
		rewritten, changed, err := n.rewrite(json.RawMessage(*field), "")
		if err != nil {
			return err
		}
		if changed {
			*field = string(rewritten)
		}
	}
	return nil
}

// memberScope is the Context Scope an object's text was written in: its own
// "source_scope_key" or "scope_key", or its quoted Source's scope. An
// approval card's identity entry ("selected_by") names its Entity's scope,
// not where its details came from, so it inherits.
func memberScope(members []jsonMember) string {
	if _, identity := memberString(members, "selected_by"); identity {
		return ""
	}
	for _, key := range []string{"source_scope_key", "scope_key"} {
		if scope, ok := memberString(members, key); ok && scope != "" {
			return scope
		}
	}
	for _, member := range members {
		if member.key == "source" {
			var source struct {
				ScopeKey string `json:"scope_key"`
			}
			if json.Unmarshal(member.value, &source) == nil && source.ScopeKey != "" {
				return source.ScopeKey
			}
		}
	}
	return ""
}

// rewrite re-emits raw JSON with each object's members in their original
// order, narrowing quoted text on the way. scope is the nearest enclosing
// object's Context Scope, or "" when none is known.
func (n operationNarrower) rewrite(raw json.RawMessage, scope string) (json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, false, nil
	}
	switch trimmed[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, false, err
		}
		changed := false
		parts := make([][]byte, len(items))
		for i := range items {
			rewritten, itemChanged, err := n.rewrite(items[i], scope)
			if err != nil {
				return nil, false, err
			}
			parts[i], changed = rewritten, changed || itemChanged
		}
		if !changed {
			return raw, false, nil
		}
		return append(append([]byte{'['}, bytes.Join(parts, []byte{','})...), ']'), true, nil
	case '{':
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		if _, err := decoder.Token(); err != nil {
			return nil, false, err
		}
		var members []jsonMember
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, false, err
			}
			key, ok := token.(string)
			if !ok {
				return nil, false, errors.New("operation JSON object key is not a string")
			}
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return nil, false, err
			}
			members = append(members, jsonMember{key: key, value: value})
		}
		if own := memberScope(members); own != "" {
			scope = own
		}
		changed := false
		for i := range members {
			rewritten, valueChanged, err := n.rewrite(members[i].value, scope)
			if err != nil {
				return nil, false, err
			}
			members[i].value = rewritten
			changed = changed || valueChanged
		}
		objectChanged, err := n.narrowObject(members)
		if err != nil {
			return nil, false, err
		}
		freeTextChanged, err := n.narrowFreeText(members, scope)
		if err != nil {
			return nil, false, err
		}
		objectChanged = objectChanged || freeTextChanged
		if !changed && !objectChanged {
			return raw, false, nil
		}
		var out bytes.Buffer
		out.WriteByte('{')
		for i, member := range members {
			if i > 0 {
				out.WriteByte(',')
			}
			key, err := json.Marshal(member.key)
			if err != nil {
				return nil, false, err
			}
			out.Write(key)
			out.WriteByte(':')
			out.Write(member.value)
		}
		out.WriteByte('}')
		return out.Bytes(), true, nil
	default:
		return raw, false, nil
	}
}

func memberString(members []jsonMember, key string) (string, bool) {
	for _, member := range members {
		if member.key == key {
			var value string
			if json.Unmarshal(member.value, &value) == nil {
				return value, true
			}
		}
	}
	return "", false
}

// narrowObject applies the Source rule to one object's quoted "evidence".
func (n operationNarrower) narrowObject(members []jsonMember) (bool, error) {
	index := -1
	for i, member := range members {
		if member.key == "evidence" {
			index = i
		}
	}
	if index < 0 {
		return false, nil
	}
	var text string
	if json.Unmarshal(members[index].value, &text) != nil || text == "" {
		return false, nil
	}
	scope, ok := memberString(members, "source_scope_key")
	if !ok {
		scope, _ = memberString(members, "scope_key")
	}
	if scope != "" && (scope == n.reader.context || scope == n.reader.session) {
		return false, nil
	}
	narrowed := ""
	if scope == "global" {
		locator, _ := memberString(members, "locator_kind")
		for _, member := range members {
			if member.key == "locator" {
				var nested struct {
					Kind string `json:"locator_kind"`
				}
				if json.Unmarshal(member.value, &nested) == nil && nested.Kind != "" {
					locator = nested.Kind
				}
			}
		}
		if locator == string(memory.LocatorUTF8ByteRange) {
			return false, nil
		}
		claim := n.claim
		if id, ok := memberString(members, "source_link_id"); ok && id != "" {
			var claimID memory.SemanticID
			err := n.q.QueryRowContext(n.ctx, `SELECT claim_id FROM semantic_source_links WHERE source_link_id = ?`, id).Scan(&claimID)
			switch {
			case err == nil:
				own, err := loadSemanticClaim(n.ctx, n.q, claimID)
				if err != nil {
					return false, err
				}
				claim = &own
			case !errors.Is(err, sql.ErrNoRows):
				return false, err
			}
		}
		if claim != nil {
			required, err := claimValueNeeds(n.ctx, n.q, text, *claim)
			if err != nil {
				return false, err
			}
			if binding := bindOwnerSource(text, required); binding.authority == memory.AuthorityOwnerStatement {
				narrowed = binding.evidence
			}
		}
	}
	if narrowed == text {
		return false, nil
	}
	encoded, err := json.Marshal(narrowed)
	if err != nil {
		return false, err
	}
	members[index].value = encoded
	return true, nil
}

// narrowFreeText blanks one object's free-text members for a reader outside
// the Context Scope (and session) they were written in. They cite no value,
// so there is no sentence to narrow to: they are shown whole or not at all.
func (n operationNarrower) narrowFreeText(members []jsonMember, scope string) (bool, error) {
	if scope != "" && (scope == n.reader.context || scope == n.reader.session) {
		return false, nil
	}
	changed := false
	for i, member := range members {
		if !operationFreeText[member.key] {
			continue
		}
		var text string
		var list []string
		var blank string
		switch {
		case json.Unmarshal(member.value, &text) == nil:
			if text == "" {
				continue
			}
			blank = `""`
		case json.Unmarshal(member.value, &list) == nil:
			if len(list) == 0 {
				continue
			}
			blank = `[]`
		default:
			continue
		}
		members[i].value, changed = json.RawMessage(blank), true
	}
	return changed, nil
}
