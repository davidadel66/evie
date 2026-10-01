package eviedb

import (
	"crypto/sha256"
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
//   - A boolean has no words of its own: it binds to a sentence containing
//     min(2, n) of a Predicate wording's n content words.
//   - An Entity counts only through a name or Alias that appears in the
//     message. Owner, Evie and Context anchors are implied by the speaker.
//
// The span is the sentence containing the value (every sentence it touches,
// when the value itself runs across sentences), or for an Entity Claim the
// sentence or two adjacent sentences naming both its subject and object,
// trimmed of surrounding whitespace. A sentence longer than
// ownerSpanMaxBytes narrows to the matches and up to ownerSpanContextWords
// words either side. A span covering the entire message is cited as whole
// content, which is what it denotes.

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

// predicateOccurrences returns the sentences containing min(2, n) of one
// Predicate wording's n content words. It stands in for a value that has no
// words of its own.
func predicateOccurrences(text string, wordings ...string) bindingOccurrences {
	var wording claimWording
	for _, value := range wordings {
		wording.addPredicate(value)
	}
	var found bindingOccurrences
	for _, sentence := range wordingSentences(text) {
		if _, covered := wording.predicateWordsIn(sentence); covered {
			found = append(found, [2]int{sentence.start, sentence.end})
		}
	}
	return found
}

// literalOccurrences is the single required item of a Typed Literal Claim.
// It returns nil when the literal has no words to look for.
func literalOccurrences(text string, literal memory.TypedLiteral, predicateToken, predicateLabel string) []bindingOccurrences {
	tokens := bindingTokens(text)
	var found bindingOccurrences
	switch literal.Kind {
	case memory.LiteralBoolean:
		found = predicateOccurrences(text, predicateToken, predicateLabel)
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
	}
	return []bindingOccurrences{found}
}

// bindOwnerSource decides the Source of a remembered value inside its owner
// message. needs lists every item the owner's words must contain; an empty
// list, or any item without an occurrence in a short enough window, makes the
// value Evie-proposed.
func bindOwnerSource(content string, needs []bindingOccurrences) ownerSourceBinding {
	evieProposed := ownerSourceBinding{kind: memory.LocatorWhole, hash: evidenceHash(content), authority: memory.AuthorityEvieProposed}
	if len(needs) == 0 {
		return evieProposed
	}
	for _, need := range needs {
		if len(need) == 0 {
			return evieProposed
		}
	}
	sentences := wordingSentences(content)
	if len(needs) == 1 {
		// One value: the sentences its first occurrence touches, however many.
		occurrence := needs[0][0]
		first, last := sentenceIndex(sentences, occurrence[0]), sentenceIndex(sentences, occurrence[1]-1)
		if first >= 0 && last >= first {
			if binding, ok := spanBinding(content, sentences[first].start, sentences[last].end, needs[0][:1]); ok {
				return binding
			}
		}
		return evieProposed
	}
	for width := 1; width <= ownerSpanMaxSentences; width++ {
		for first := 0; first+width <= len(sentences); first++ {
			low, high := sentences[first].start, sentences[first+width-1].end
			chosen := make([][2]int, 0, len(needs))
			for _, need := range needs {
				for _, occurrence := range need {
					if occurrence[0] >= low && occurrence[1] <= high {
						chosen = append(chosen, occurrence)
						break
					}
				}
			}
			if len(chosen) != len(needs) {
				continue
			}
			if binding, ok := spanBinding(content, low, high, chosen); ok {
				return binding
			}
		}
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
