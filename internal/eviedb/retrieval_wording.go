package eviedb

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Deterministic wording rules for harness review Stage 13 (M2, M3). A rule
// compares one sentence of owner or assistant text with an accepted Claim's
// own words: its saved value, its Predicate token and labels, and a non-owner
// subject's canonical name. The only fixed language data are the closed
// English lists below (change cues, first-person pronouns, abbreviations that
// do not end a sentence, function words) and relevanceKey's inflection
// folding. No topic dictionary or learned score is involved.

// Change cues say a stated fact differs from before. They qualify a later
// statement that names a saved value or the Predicate's words.
var wordingChangeCues = map[string]bool{
	"now": true, "nowadays": true, "anymore": true, "instead": true,
	"moved": true, "moving": true, "relocated": true, "relocating": true,
	"switched": true, "switching": true, "changed": true, "changing": true,
	"left": true, "quit": true, "quitting": true,
	"former": true, "formerly": true, "previously": true,
}

// Two-word change cues, matched as consecutive words.
var wordingChangePhrases = [][2]string{{"no", "longer"}, {"used", "to"}, {"behind", "me"}, {"these", "days"}}

// "new" announces a replacement ("my new barber"), but beside a saved value it
// is usually just news about that value ("Verizon sent a new bill"). It
// qualifies only the Predicate-words rule.
const wordingNoveltyCue = "new"

var wordingFirstPerson = map[string]bool{"i": true, "me": true, "my": true, "mine": true, "myself": true, "we": true, "us": true, "our": true, "ours": true}

// A period after these words (or after a single letter) does not end a
// sentence, so "Dr. Okafor" stays one value.
var wordingAbbreviations = map[string]bool{"dr": true, "mr": true, "mrs": true, "ms": true, "st": true, "jr": true, "sr": true, "prof": true, "vs": true, "etc": true, "mt": true}

// Function words never count as Predicate words.
var wordingFunctionWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "on": true, "at": true, "to": true, "for": true, "with": true,
	"by": true, "from": true, "as": true, "and": true, "or": true, "is": true, "are": true, "my": true, "your": true, "s": true,
}

type wordingWord struct {
	text, key string
}

type wordingSentence struct {
	start, end int
	words      []wordingWord
	question   bool
}

func wordingWords(text string) []wordingWord {
	var words []wordingWord
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		words = append(words, wordingWord{text: field, key: relevanceKey(field)})
	}
	return words
}

// wordingSentences splits text at '.', '!', '?', ';' or a line break that is
// followed by whitespace or the end. Byte ranges index the original text.
func wordingSentences(text string) []wordingSentence {
	var sentences []wordingSentence
	start := 0
	flush := func(end int, question bool) {
		if words := wordingWords(text[start:end]); len(words) > 0 {
			sentences = append(sentences, wordingSentence{start: start, end: end, words: words, question: question})
		}
		start = end
	}
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		next := i + size
		switch r {
		case '\n':
			flush(next, false)
		case '.', '!', '?', ';':
			if next < len(text) {
				following, _ := utf8.DecodeRuneInString(text[next:])
				if !unicode.IsSpace(following) {
					break
				}
			}
			if r == '.' {
				prior := wordingWords(text[start:i])
				if n := len(prior); n > 0 && (wordingAbbreviations[prior[n-1].text] || utf8.RuneCountInString(prior[n-1].text) == 1 && unicode.IsLetter([]rune(prior[n-1].text)[0])) {
					break
				}
			}
			flush(next, r == '?')
		}
		i = next
	}
	flush(len(text), strings.HasSuffix(strings.TrimSpace(text[start:]), "?"))
	return sentences
}

func (s wordingSentence) keys() map[string]bool {
	keys := make(map[string]bool, len(s.words))
	for _, word := range s.words {
		keys[word.key] = true
	}
	return keys
}

func (s wordingSentence) hasChangeCue() bool {
	for i, word := range s.words {
		if wordingChangeCues[word.text] {
			return true
		}
		if i > 0 {
			for _, phrase := range wordingChangePhrases {
				if s.words[i-1].text == phrase[0] && word.text == phrase[1] {
					return true
				}
			}
		}
	}
	return false
}

func (s wordingSentence) hasWord(text string) bool {
	for _, word := range s.words {
		if word.text == text {
			return true
		}
	}
	return false
}

func (s wordingSentence) hasFirstPerson() bool {
	for _, word := range s.words {
		if wordingFirstPerson[word.text] {
			return true
		}
	}
	return false
}

// contains reports whether the sentence has the key sequence as consecutive
// words.
func (s wordingSentence) contains(sequence []string) bool {
	if len(sequence) == 0 {
		return false
	}
	for i := 0; i+len(sequence) <= len(s.words); i++ {
		matched := true
		for j, key := range sequence {
			if s.words[i+j].key != key {
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

// claimWording is the vocabulary of one subject and Predicate family: every
// token version shares one family.
type claimWording struct {
	owner      bool
	subjects   [][]string // non-owner subject canonical names, as key sequences
	values     [][]string // usable saved values, as key sequences
	valueWords [][]string // the same values as exact lowercase words, for FTS
	predicates [][]string // distinct content keys of the token and each label
}

func wordingKeys(text string) []string {
	var keys []string
	for _, word := range wordingWords(text) {
		keys = append(keys, word.key)
	}
	return keys
}

// addValue keeps a saved value specific enough to identify the Claim: at
// least two words, or one word with a letter. Booleans and lone numbers are
// too common to anchor a match.
func (w *claimWording) addValue(kind, value string) {
	if kind == "boolean" {
		return
	}
	words := wordingWords(value)
	letter := false
	for _, word := range words {
		letter = letter || strings.IndexFunc(word.text, unicode.IsLetter) >= 0
	}
	if len(words) == 0 || len(words) == 1 && !letter {
		return
	}
	keys := wordingKeys(value)
	for _, existing := range w.values {
		if strings.Join(existing, " ") == strings.Join(keys, " ") {
			return
		}
	}
	exact := make([]string, 0, len(words))
	for _, word := range words {
		exact = append(exact, word.text)
	}
	w.values = append(w.values, keys)
	w.valueWords = append(w.valueWords, exact)
}

func (w *claimWording) addPredicate(text string) {
	var group []string
	seen := map[string]bool{}
	for _, word := range wordingWords(strings.ReplaceAll(text, "_", " ")) {
		if wordingFunctionWords[word.text] || seen[word.key] {
			continue
		}
		seen[word.key] = true
		group = append(group, word.key)
	}
	if len(group) == 0 {
		return
	}
	for _, existing := range w.predicates {
		if strings.Join(existing, " ") == strings.Join(group, " ") {
			return
		}
	}
	w.predicates = append(w.predicates, group)
}

func (w *claimWording) addSubject(name string, owner bool) {
	if owner {
		w.owner = true
		return
	}
	if keys := wordingKeys(name); len(keys) > 0 {
		w.subjects = append(w.subjects, keys)
	}
}

func (w claimWording) valueIn(s wordingSentence) bool {
	for _, value := range w.values {
		if s.contains(value) {
			return true
		}
	}
	return false
}

// predicateWordsIn reports the largest number of one Predicate group's words
// the sentence contains, and whether that covers the group's requirement of
// min(2, group size) words.
func (w claimWording) predicateWordsIn(s wordingSentence) (best int, covered bool) {
	keys := s.keys()
	for _, group := range w.predicates {
		matched := 0
		for _, key := range group {
			if keys[key] {
				matched++
			}
		}
		best = max(best, matched)
		covered = covered || matched >= min(2, len(group))
	}
	return best, covered
}

func (w claimWording) subjectNamedIn(s wordingSentence) bool {
	for _, subject := range w.subjects {
		if s.contains(subject) {
			return true
		}
	}
	return false
}

// subjectIn is the owner speaking in the first person, or a non-owner subject
// named in the sentence.
func (w claimWording) subjectIn(s wordingSentence) bool {
	if w.owner {
		return s.hasFirstPerson()
	}
	return w.subjectNamedIn(s)
}

// newerStatement (M3) is a declarative sentence about the subject that names
// a saved value with a change cue ("I moved to Chicago, Boston is behind me"),
// or the Predicate's words with a change or novelty cue ("my shoes are a size
// 10 now"). It proposes a candidate discrepancy only; it never accepts,
// corrects or supersedes memory.
func (w claimWording) newerStatement(s wordingSentence) bool {
	if s.question || !w.subjectIn(s) {
		return false
	}
	change := s.hasChangeCue()
	if change && w.valueIn(s) {
		return true
	}
	_, covered := w.predicateWordsIn(s)
	return covered && (change || s.hasWord(wordingNoveltyCue))
}

// restatement (M2) repeats a saved value together with one of the
// Predicate's words, or a non-owner subject's name, in one sentence.
func (w claimWording) restatement(s wordingSentence) bool {
	if !w.valueIn(s) {
		return false
	}
	if best, _ := w.predicateWordsIn(s); best > 0 {
		return true
	}
	return !w.owner && w.subjectNamedIn(s)
}

// firstSentence returns the first sentence inside [start,end) the rule
// accepts.
func firstSentence(text string, start, end int, rule func(wordingSentence) bool) (wordingSentence, bool) {
	for _, sentence := range wordingSentences(text[start:end]) {
		if rule(sentence) {
			sentence.start += start
			sentence.end += start
			return sentence, true
		}
	}
	return wordingSentence{}, false
}
