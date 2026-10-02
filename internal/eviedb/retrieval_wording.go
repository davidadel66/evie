package eviedb

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Deterministic wording rules for harness review Stage 13 (M2, M3). A rule
// compares one sentence of owner or assistant text with an accepted Claim's
// own words: its saved value, its Predicate token and labels, and a non-owner
// subject's canonical name. The only fixed language data are the closed
// English lists below (change cues, first-person pronouns, abbreviations that
// do not end a sentence, function words, relation words), those of the clause
// analysis shared with the owner-span binder (retrieval_wording_clauses.go),
// and relevanceKey's inflection folding. No topic dictionary or learned score
// is involved.

// Change cues say a stated fact differs from before. They qualify a later
// statement that names a saved value or the Predicate's words.
var wordingChangeCues = map[string]bool{
	"now": true, "nowadays": true, "anymore": true, "instead": true,
	"moved": true, "moving": true, "relocated": true, "relocating": true,
	"switched": true, "switching": true, "changed": true, "changing": true,
	"left": true, "quit": true, "quitting": true, "dropped": true,
	"stopped": true, "stopping": true, "cancelled": true, "canceled": true,
	"former": true, "formerly": true, "previously": true,
}

// Two-word change cues, matched as consecutive words.
var wordingChangePhrases = [][2]string{{"no", "longer"}, {"used", "to"}, {"behind", "me"}, {"these", "days"}}

// "new" announces a replacement ("my new barber"), but beside a saved value it
// is usually just news about that value ("Verizon sent a new bill"). It
// qualifies only the Predicate-words rule.
const wordingNoveltyCue = "new"

var wordingFirstPerson = map[string]bool{"i": true, "me": true, "my": true, "mine": true, "myself": true, "we": true, "us": true, "our": true, "ours": true}

// First-person possessives speak for the owner only when they own the
// Claim's words ("my favorite coffee shop", "my new barber"); "my sister" and
// "my friend Sam" name someone else. A possessive owns the Claim's words when
// one of the next wordingPossessiveReach words is a Predicate or value word.
var wordingFirstPersonPossessives = map[string]bool{"my": true, "our": true}

const wordingPossessiveReach = 3

// Third-person pronouns make a clause about someone other than the owner.
var wordingThirdPerson = map[string]bool{"he": true, "she": true, "him": true, "his": true, "her": true, "hers": true, "they": true, "them": true, "their": true, "theirs": true}

// A cue governs the Claim's words only when nothing but these light words
// (and numbers, cue words, or the Claim's own words) separates them in one
// clause: "moved to Boston", "Boston is behind me", "a size 10 now". "Left my
// umbrella in Boston" has a content word between, so "left" is not about
// Boston.
var wordingLightWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "on": true, "at": true, "to": true, "for": true, "with": true,
	"by": true, "from": true, "as": true, "and": true, "or": true, "into": true, "onto": true, "out": true, "off": true,
	"away": true, "back": true, "over": true, "up": true, "down": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "am": true, "there": true, "here": true,
	"not": true, "no": true, "still": true, "just": true, "finally": true, "officially": true, "really": true, "also": true,
	"already": true, "then": true, "s": true, "m": true, "t": true, "re": true, "ve": true, "d": true, "ll": true,
	"isn": true, "aren": true, "wasn": true, "don": true, "doesn": true, "didn": true, "won": true,
	"i": true, "me": true, "my": true, "we": true, "us": true, "our": true,
}

// A cue also governs the Claim's words across an ordinary verb of living,
// working or using, and a noun for the owner's tie to it (confirmation
// review): "I no longer live in Boston", "I stopped using Verizon", "I quit
// my job at Initech". The clause's subject is checked separately, so "My boss
// quit his job at Initech" still does not link. "Left my umbrella in Boston"
// and "switched hotels in Boston" do not qualify.
var wordingRelationWords = setOf(
	"live", "lives", "living", "lived", "work", "works", "working", "worked", "use", "uses", "using", "used",
	"stay", "stays", "staying", "stayed", "shop", "shops", "shopping", "shopped", "bank", "banks", "banking", "banked",
	"go", "goes", "going", "went", "attend", "attends", "attending", "attended", "rent", "rents", "renting", "rented",
	"see", "sees", "seeing", "saw", "study", "studies", "studying", "studied", "belong", "belongs", "belonging", "belonged",
	"call", "calls", "calling", "called", "have", "has", "having", "had", "based", "reside", "resides", "residing", "resided",
	"job", "role", "position", "post", "contract", "plan", "subscription", "membership", "account", "service", "lease",
	"apartment", "flat", "house", "home", "place", "office", "company", "firm",
)

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
	// start and end are the word's byte range in the analysed text.
	start, end int
	// clause numbers the clause of the sentence the word is in
	// (retrieval_wording_clauses.go).
	clause int
	// quoted: inside quotation marks; apostrophe: directly after an
	// apostrophe that follows a letter ("s" of "Sam's", "t" of "don't");
	// capital: written with a leading capital.
	quoted, apostrophe, capital bool
}

type wordingSentence struct {
	start, end int
	words      []wordingWord
	clauses    []wordingClause
	question   bool
	// breaks[i]: punctuation or a quotation mark comes before word i.
	breaks []bool
}

func wordingWords(text string) []wordingWord {
	var words []wordingWord
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		words = append(words, wordingWord{text: field, key: relevanceKey(field)})
	}
	return words
}

// wordingSentences splits text at '.', '!', '?', ';' or a line break that is
// followed by whitespace or the end, and each sentence into clauses
// (retrieval_wording_clauses.go). Byte ranges index the original text.
func wordingSentences(text string) []wordingSentence {
	var sentences []wordingSentence
	start := 0
	quoted := wordingQuoted(text)
	flush := func(end int, question bool) {
		if sentence := analyseSentence(text, start, end, question, quoted); len(sentence.words) > 0 {
			sentences = append(sentences, sentence)
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

// wordingSpan is a run of words [start, end] (inclusive) in one sentence.
type wordingSpan struct {
	start, end int
	novelty    bool // a cue span: the novelty cue "new" rather than a change cue
}

// cues returns every change cue (single word or two-word phrase) and every
// novelty cue in the sentence.
func (s wordingSentence) cues() []wordingSpan {
	var cues []wordingSpan
	for i, word := range s.words {
		if wordingChangeCues[word.text] {
			cues = append(cues, wordingSpan{start: i, end: i})
		}
		if word.text == wordingNoveltyCue {
			cues = append(cues, wordingSpan{start: i, end: i, novelty: true})
		}
		if i > 0 {
			for _, phrase := range wordingChangePhrases {
				if s.words[i-1].text == phrase[0] && word.text == phrase[1] {
					cues = append(cues, wordingSpan{start: i - 1, end: i})
				}
			}
		}
	}
	return cues
}

// clauseWords returns the words of one clause of the sentence.
func (s wordingSentence) clauseWords(clause int) []wordingWord {
	var words []wordingWord
	for _, word := range s.words {
		if word.clause == clause {
			words = append(words, word)
		}
	}
	return words
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
	return len(w.valueSpans(s)) > 0
}

// valueSpans returns every occurrence of a saved value in the sentence.
func (w claimWording) valueSpans(s wordingSentence) []wordingSpan {
	var spans []wordingSpan
	for _, value := range w.values {
		for i := 0; i+len(value) <= len(s.words); i++ {
			matched := true
			for j, key := range value {
				if s.words[i+j].key != key {
					matched = false
					break
				}
			}
			if matched {
				spans = append(spans, wordingSpan{start: i, end: i + len(value) - 1})
			}
		}
	}
	return spans
}

// claimWord reports whether a folded key is one of the Claim's own words: a
// Predicate word or a word of a saved value.
func (w claimWording) claimWord(key string) bool {
	for _, group := range append(append([][]string(nil), w.predicates...), w.values...) {
		for _, word := range group {
			if word == key {
				return true
			}
		}
	}
	return false
}

func (w claimWording) predicateWord(key string) bool {
	for _, group := range w.predicates {
		for _, word := range group {
			if word == key {
				return true
			}
		}
	}
	return false
}

// possessiveOwns reports whether the possessive at words[i] owns the Claim's
// words: its noun phrase (up to the first verb or function word, and before
// any possessive "'s") holds a Predicate or value word. "My dad's favorite
// coffee shop" is dad's; in "my flight left Boston" the phrase is "flight".
func (w claimWording) possessiveOwns(words []wordingWord, i int) bool {
	end, possessive := phraseEnd(words, i+1, min(len(words), i+1+wordingPossessiveReach))
	if possessive >= 0 {
		return false
	}
	for _, word := range words[i+1 : end] {
		if w.claimWord(word.key) {
			return true
		}
	}
	return false
}

// ownerSpeaks reports a first-person word that speaks for the owner: a
// pronoun, or a possessive that owns the Claim's words ("my home city", not
// "my sister").
func (w claimWording) ownerSpeaks(words []wordingWord) bool {
	for i, word := range words {
		if !wordingFirstPerson[word.text] {
			continue
		}
		if !wordingFirstPersonPossessives[word.text] || w.possessiveOwns(words, i) {
			return true
		}
	}
	return false
}

// thirdParty reports a word that makes the words about someone other than
// the owner: a third-person pronoun, or a first-person possessive that does
// not own the Claim's words ("my sister", "my friend Sam").
func (w claimWording) thirdParty(words []wordingWord) bool {
	for i, word := range words {
		if wordingThirdPerson[word.text] {
			return true
		}
		if wordingFirstPersonPossessives[word.text] && !w.possessiveOwns(words, i) {
			return true
		}
	}
	return false
}

// vocabulary is the Claim's words for the shared clause analysis.
func (w claimWording) vocabulary() wordingVocabulary {
	return wordingVocabulary{
		owner:     w.owner,
		predicate: func(word wordingWord) bool { return w.predicateWord(word.key) },
		claim: func(word wordingWord) bool {
			if w.claimWord(word.key) {
				return true
			}
			for _, subject := range w.subjects {
				if slices.Contains(subject, word.key) {
					return true
				}
			}
			return false
		},
	}
}

// subjectSpeaks is the subject requirement for one clause of a sentence,
// using the clause analysis the owner-span binder also uses. A reported or
// conditional clause never speaks for the subject. For the owner: the
// clause's subject is the owner ("I", "my home city"); or its subject is the
// Claim's own words or nobody in particular, it names no third party, and
// the owner speaks in it ("Boston is behind me") or in another clause of the
// sentence ("I moved to Chicago, so Boston is no longer home"). Someone
// else's clause ("my sister", "our team", "the train") never does. For
// another subject: the sentence names it.
func (w claimWording) subjectSpeaks(s wordingSentence, clause int) bool {
	c := s.clauses[clause]
	if c.reported || c.conditional {
		return false
	}
	if !w.owner {
		return w.subjectNamedIn(s)
	}
	v := w.vocabulary()
	switch s.subject(clause, v) {
	case subjectOwner:
		return true
	case subjectOther, subjectPronoun:
		return false
	}
	words := s.clauseWords(clause)
	return !w.thirdParty(words) && (w.ownerSpeaks(words) || s.ownerSubjectClause(v))
}

// governs reports whether a cue and the Claim words at target sit in one
// clause with only light words, numbers, cue words, Claim words or relation
// words between.
func (w claimWording) governs(s wordingSentence, cue, target wordingSpan) bool {
	if s.words[cue.start].clause != s.words[target.start].clause || s.words[cue.end].clause != s.words[target.end].clause {
		return false
	}
	from, to := cue.end+1, target.start
	if target.end < cue.start {
		from, to = target.end+1, cue.start
	}
	for i := from; i < to; i++ {
		word := s.words[i]
		if word.clause != s.words[cue.start].clause {
			return false
		}
		number := strings.IndexFunc(word.text, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
		if !wordingLightWords[word.text] && !number && !wordingChangeCues[word.text] && word.text != wordingNoveltyCue && !w.claimWord(word.key) && !wordingRelationWords[word.text] {
			return false
		}
	}
	return true
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

// wordingStrength orders candidate newer statements: an update (a governed
// cue with the subject speaking) first, then how many of the Claim's words
// the sentence names (a saved value counts one, plus the Predicate words),
// then whether the subject speaks at all. Recency breaks remaining ties.
type wordingStrength struct {
	update  bool
	words   int
	subject bool
}

func (a wordingStrength) stronger(b wordingStrength) bool {
	if a.update != b.update {
		return a.update
	}
	if a.words != b.words {
		return a.words > b.words
	}
	return a.subject && !b.subject
}

// assess measures one sentence against the Claim. update is the newer-
// statement rule (M3): a declarative sentence in which a change cue governs a
// saved value ("Boston is behind me", "I dropped Verizon"), or a change or
// novelty cue governs one of the Predicate's words when the sentence has
// min(2, n) of a Predicate group's n words ("my shoes are a size 10 now",
// "my new barber"), and the subject speaks in the clause carrying that cue,
// which is not a question, conditional or reported speech. A relation word
// may sit between the cue and the Claim's words ("I no longer live in
// Boston", "I quit my job at Initech"). "I left my umbrella in Boston" (the
// cue governs the umbrella), "My sister moved to Boston now" and "Our team
// left Boston" (someone else's subject) and "My sister said, Boston is no
// longer an option" (reported) are not updates. It proposes a candidate
// discrepancy only; it never accepts, corrects or supersedes memory.
func (w claimWording) assess(s wordingSentence) wordingStrength {
	var strength wordingStrength
	best, covered := w.predicateWordsIn(s)
	values := w.valueSpans(s)
	strength.words = best
	if len(values) > 0 {
		strength.words++
	}
	for i, word := range s.words {
		if i == 0 || word.clause != s.words[i-1].clause {
			strength.subject = strength.subject || w.subjectSpeaks(s, word.clause)
		}
	}
	if s.question {
		return strength
	}
	var predicates []wordingSpan
	if covered {
		for i, word := range s.words {
			if w.predicateWord(word.key) {
				predicates = append(predicates, wordingSpan{start: i, end: i})
			}
		}
	}
	for _, cue := range s.cues() {
		if c := s.clauses[s.words[cue.start].clause]; c.question {
			continue
		}
		governed := false
		if !cue.novelty {
			for _, value := range values {
				governed = governed || w.governs(s, cue, value)
			}
		}
		for _, predicate := range predicates {
			governed = governed || w.governs(s, cue, predicate)
		}
		if governed && w.subjectSpeaks(s, s.words[cue.start].clause) {
			strength.update = true
			break
		}
	}
	return strength
}

// newerStatement (M3) reports a sentence that assess finds to be an update.
func (w claimWording) newerStatement(s wordingSentence) bool {
	return w.assess(s).update
}

// restatement (M2) repeats a saved value together with one of the
// Predicate's words, or a non-owner subject's name, in one sentence, with the
// subject speaking in the value's clause: a friend's preference or news about
// the value is not the owner repeating a retired fact.
func (w claimWording) restatement(s wordingSentence) bool {
	values := w.valueSpans(s)
	if len(values) == 0 || !w.subjectSpeaks(s, s.words[values[0].start].clause) {
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
