package eviedb

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Shared clause and subject analysis (harness review confirmation pass). The
// owner-span binder (semantic_source_binding.go) and the M2/M3 wording rules
// (retrieval_wording.go) ask the same questions of one sentence: where its
// clauses are, whose statement each clause is, and whether a clause asserts
// anything at all. Both answer them here, from closed English word lists, so
// they agree. This is a heuristic, not a parser, and it is wrong sometimes;
// every consumer is written so that an unsure answer fails safe: authority
// becomes Evie-proposed, a newer statement is not linked, and no extra text
// is quoted.
//
// A sentence splits into clauses at , (not inside a number) : ( ) an en or
// em dash, a hyphen with spaces on both sides, and a quotation mark; before a
// subordinating or contrasting word ("but", "because", "if", "unless",
// "until", ...); before "so" followed by a subject; before "and" or "or"
// followed by a new subject, a possessive, an auxiliary or a negation ("and
// I don't", "and doesn't"); and after a reporting verb ("said", "says",
// "told me", "thinks"), where the reported complement starts. A base form
// ("say", "think", "claim") reports only after a subject pronoun, a modal or
// "to", and a complement does not start with an auxiliary, a number or a
// preposition, so "scope claim 81" and "the reports are due" do not report.
// A clause is
//
//   - reported when it is quoted, follows a reporting verb whose subject is
//     not "I" or "we" ("My sister said, ..."; "The doctor says I'm ..."),
//     follows "heard", or is in a sentence with "according to";
//   - conditional when it starts with "if", "whether" or "unless", or holds
//     a wish, modal or future marker ("wish", "would", "could", "might",
//     "should", "may", "'d", "will", "going to", "want to"); "would like",
//     "'d prefer" and the other preference idioms are not conditional;
//   - a question when its sentence ends with '?' and it is the last clause or
//     starts with a question word or an inverted auxiliary;
//   - a change when it holds a word that says a value no longer holds
//     ("used to", "no longer", "left", "quit", "dropped", "stopped",
//     "former", "moved from", "switched away"). A bounded interval ("from
//     March until November") is not a change: it is how an owner states a
//     Claim with an ended valid time.
//
// A clause's subject is its first word after leading conjunctions, discourse
// words ("yes", "actually", "honestly") and memory cues ("remember that",
// "don't forget"): the owner ("I", "we", or "my"/"our" before the Claim's
// Predicate words, "my sister's birthday" for sister_birthday); someone else
// (he, she, they, you, "my sister", "our team", "the doctor", "Selma likes",
// a possessive chain such as "my dad's"); the Claim's own words ("Boston is
// behind me", "the dose is"); or nobody in particular ("it", an imperative, a
// fragment). A clause that starts with a conjunction and a verb ("and doesn't
// like it") keeps the previous clause's subject.

type wordingSubject int

const (
	subjectNone    wordingSubject = iota // impersonal, imperative, fragment or unknown
	subjectOwner                         // I, we, or my/our owning the Claim's Predicate words
	subjectClaim                         // the Claim's own words: its value, Predicate or subject name
	subjectPronoun                       // he, she, they: may refer to an Entity subject
	subjectOther                         // someone or something else
)

// wordingVocabulary tells the analysis which words belong to the Claim being
// checked. predicate holds its Predicate words; claim also its value and a
// non-owner subject's name.
type wordingVocabulary struct {
	predicate func(wordingWord) bool
	claim     func(wordingWord) bool
	// owner: the Claim's subject is the owner. Otherwise a possessive phrase
	// naming the Claim's subject ("my mother Maya") is about that subject.
	owner bool
}

// wordingClause is one clause of a sentence: words [first, last) and the
// claim-independent facts the rules need.
type wordingClause struct {
	first, last int
	// lead is the first word after leading conjunctions, discourse words and
	// memory cues: where the subject is read. lead == last when none is left.
	lead        int
	reported    bool
	conditional bool
	question    bool
	change      bool
	// inherits: the clause starts with a verb, so its subject is the previous
	// clause's ("and doesn't like it", ", switched to Mint").
	inherits bool
}

var (
	wordingClauseStarters = setOf("but", "although", "though", "however", "whereas", "yet", "because", "since", "unless", "until", "while", "if", "whether")
	wordingContrasts      = setOf("but", "although", "though", "however", "whereas", "yet")
	wordingSubjectWords   = setOf("i", "we", "you", "he", "she", "they", "it", "that", "there")
	wordingPossessives    = setOf("my", "our", "your", "his", "her", "their")
	// Auxiliaries, modals and their negated fragments ("don" of "don't").
	wordingAuxiliaries = setOf("do", "does", "did", "don", "doesn", "didn", "dont", "doesnt", "didnt", "is", "isn", "isnt", "am", "are", "aren", "arent",
		"was", "wasn", "wasnt", "were", "weren", "werent", "have", "haven", "havent", "has", "hasn", "hasnt", "had", "hadn", "hadnt", "will", "won", "wont",
		"would", "wouldn", "wouldnt", "can", "cannot", "cant", "could", "couldn", "couldnt", "should", "shouldn", "shouldnt", "might", "may", "must", "shall", "be", "been")
	wordingNegationWords = setOf("not", "never", "no", "cannot", "nor", "neither", "hardly", "barely", "scarcely",
		"dont", "doesnt", "didnt", "isnt", "arent", "wasnt", "werent", "wont", "cant", "havent", "hasnt", "hadnt", "wouldnt", "shouldnt", "couldnt", "aint")
	// Reporting verbs start a reported complement; "heard" always reports
	// someone else.
	wordingReportingVerbs = setOf("say", "says", "said", "saying", "tell", "tells", "told", "telling", "think", "thinks", "thought", "believe", "believes",
		"believed", "claim", "claims", "claimed", "mention", "mentions", "mentioned", "write", "writes", "wrote", "ask", "asks", "asked", "insist", "insists",
		"insisted", "reckon", "reckons", "suggest", "suggests", "suggested", "warn", "warns", "warned", "report", "reports", "reported", "explain",
		"explains", "explained", "hear", "hears", "heard")
	wordingHearsay = setOf("hear", "hears", "heard")
	// A base form is a reporting verb only after a subject pronoun, a modal
	// or "to" ("I think", "they say", "to tell you"); otherwise it is an
	// imperative or a noun ("scope claim 81", "write a cover letter").
	wordingReportingBase  = setOf("say", "tell", "think", "believe", "claim", "mention", "write", "ask", "insist", "reckon", "suggest", "warn", "report", "explain", "hear")
	wordingReportingAfter = setOf("i", "you", "we", "they", "he", "she", "to", "ll", "d", "will", "would", "can", "could", "should", "must", "might", "do", "did", "don", "didn", "doesn")
	// A reported complement does not start with an auxiliary, a number or a
	// preposition ("the reports are due", "insurance claims for 2025").
	wordingNotComplement  = setOf("of", "for", "in", "on", "at", "to", "by", "with", "from", "about", "into", "and", "or")
	wordingReportedObject = setOf("me", "you", "us", "him", "her", "them")
	// Leading words that are not the clause's subject.
	wordingLeadSkips = setOf("and", "or", "but", "so", "yet", "then", "plus", "although", "though", "however", "whereas", "because", "since", "unless",
		"until", "while", "if", "whether", "yes", "yeah", "yep", "ok", "okay", "well", "oh", "honestly", "actually", "anyway", "anyways", "basically",
		"btw", "fyi", "hey", "hi", "hello", "um", "uh", "hmm", "please", "just", "also", "really", "finally", "now", "again",
		"remember", "note", "save", "memorize", "memorise", "correction", "reminder", "update", "forget")
	wordingConditionalStarts = setOf("if", "whether", "unless")
	wordingConditionalWords  = setOf("wish", "wishes", "wished", "wishing", "would", "wouldn", "wouldnt", "could", "couldn", "couldnt", "might", "mightn",
		"should", "shouldn", "shouldnt", "maybe", "perhaps", "possibly", "hypothetically", "suppose", "supposing", "imagine", "imagining", "pretend",
		"will", "ll", "shall", "gonna")
	// "would like", "'d prefer": a stated preference, not a condition.
	wordingPreferenceVerbs = setOf("like", "prefer", "rather", "love", "enjoy")
	// Aspiration or plan: "going to", "want to", "planning to".
	wordingIntentVerbs   = setOf("going", "want", "wants", "wanted", "plan", "plans", "planning", "hoping", "hope", "trying", "try", "about")
	wordingQuestionLeads = setOf("what", "which", "who", "whom", "whose", "where", "when", "why", "how", "is", "are", "am", "was", "were", "do", "does",
		"did", "can", "could", "should", "would", "will", "shall", "have", "has", "may")
	// Words that say a value no longer holds. "moved", "switched" and the
	// like do so only with "from", "out" or "away" after them ("moved to
	// Boston" states Boston).
	wordingEndedWords  = setOf("left", "quit", "quitting", "dropped", "stopped", "stopping", "cancelled", "canceled", "cancelling", "canceling", "former", "formerly", "previously", "ex")
	wordingMovingWords = setOf("moved", "moving", "relocated", "relocating", "switched", "switching", "transferred")
	wordingAwayWords   = setOf("from", "out", "away")
	// Words a possessive or determiner phrase stops at.
	wordingPhraseStops = setOf("in", "at", "on", "of", "for", "from", "with", "to", "by", "about", "into", "near", "over", "under", "after", "before",
		"during", "like", "and", "or", "but", "so", "who", "which", "that", "the", "a", "an", "this", "these", "those", "i", "me", "we", "us", "you",
		"he", "him", "she", "they", "them", "it", "my", "our", "your", "his", "her", "their", "its", "not", "never", "no", "now", "too")
	// People and groups: a possessive naming one is about someone else ("my
	// sister", "my therapist", "our team"), unless it holds the Claim's own
	// Predicate words ("my sister's birthday" for sister_birthday).
	wordingPeople = setOf("sister", "sisters", "brother", "brothers", "sibling", "siblings", "mom", "mum", "mother", "dad", "father", "parent", "parents",
		"wife", "husband", "spouse", "partner", "boyfriend", "girlfriend", "fiance", "fiancee", "ex", "son", "sons", "daughter", "daughters", "kid", "kids",
		"child", "children", "baby", "friend", "friends", "roommate", "roommates", "flatmate", "housemate", "neighbor", "neighbors", "neighbour",
		"neighbours", "boss", "manager", "colleague", "colleagues", "coworker", "coworkers", "teammate", "teammates", "team", "family", "aunt", "uncle",
		"cousin", "cousins", "grandma", "grandpa", "grandmother", "grandfather", "nephew", "niece", "landlord", "landlady", "tenant", "teacher", "coach",
		"doctor", "therapist", "psychiatrist", "psychologist", "counselor", "counsellor", "oncologist", "lawyer", "attorney", "accountant", "nurse",
		"dentist", "vet", "surgeon", "physician", "gp", "client", "clients", "customer", "student", "assistant", "employee", "staff", "people")
	wordingOwnerPronouns = setOf("i", "me", "myself", "we", "us", "mine", "ours")
	wordingOtherPronouns = setOf("he", "she", "they")
	wordingOtherSubjects = setOf("you", "someone", "somebody", "everyone", "everybody", "nobody", "anyone", "anybody")
	wordingImpersonal    = setOf("it", "there", "this", "that", "these", "those", "here")
	wordingDeterminers   = setOf("the", "a", "an")
)

func setOf(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

func isApostrophe(r rune) bool { return r == '\'' || r == '’' }

// wordingQuoted marks the bytes of text inside quotation marks. A double quote
// toggles; a single quote opens only before a word and after a space or the
// start, so apostrophes in "I'm" or "the kids' toys" are not quotes.
func wordingQuoted(text string) []bool {
	quoted := make([]bool, len(text)+1)
	double, single := false, false
	prev := ' '
	for i, r := range text {
		next, _ := utf8.DecodeRuneInString(text[i+utf8.RuneLen(r):])
		switch {
		case r == '"':
			double = !double
		case r == '“':
			double = true
		case r == '”':
			double = false
		case (r == '\'' || r == '‘') && !single && (unicode.IsSpace(prev) || strings.ContainsRune("([{\"“", prev)) && (unicode.IsLetter(next) || unicode.IsDigit(next)):
			single = true
		case (r == '\'' || r == '’') && single && !unicode.IsSpace(prev) && !unicode.IsLetter(next) && !unicode.IsDigit(next):
			single = false
		}
		for j := i; j < i+utf8.RuneLen(r); j++ {
			quoted[j] = double || single
		}
		prev = r
	}
	return quoted
}

// analyseSentence tokenizes text[start:end] into words with byte offsets
// into text and splits them into clauses.
func analyseSentence(text string, start, end int, question bool, quoted []bool) wordingSentence {
	s := wordingSentence{start: start, end: end, question: question}
	var breaks []bool // breaks[i]: punctuation or a quotation mark before word i
	pending := false
	for i := start; i < end; {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			switch {
			case r == ',' && i > 0 && i+1 < len(text) && isASCIIDigit(text[i-1]) && isASCIIDigit(text[i+1]):
				// A comma inside a number ("1,500") is not a boundary.
			case strings.ContainsRune(",:()–—\"“”‘", r):
				pending = true
			case r == '-' && i > 0 && text[i-1] == ' ' && i+1 < len(text) && text[i+1] == ' ':
				pending = true
			case (r == '\'' || r == '’') && quoted[i] != quoted[min(i+size, len(quoted)-1)]:
				pending = true
			}
			i += size
			continue
		}
		wordStart := i
		for i < end {
			r, size = utf8.DecodeRuneInString(text[i:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				break
			}
			i += size
		}
		raw := text[wordStart:i]
		lower := strings.ToLower(raw)
		word := wordingWord{text: lower, key: relevanceKey(lower), start: wordStart, end: i, quoted: quoted[wordStart]}
		if wordStart > 0 {
			before, _ := utf8.DecodeLastRuneInString(text[:wordStart])
			if isApostrophe(before) && wordStart-utf8.RuneLen(before) > 0 {
				prior, _ := utf8.DecodeLastRuneInString(text[:wordStart-utf8.RuneLen(before)])
				word.apostrophe = unicode.IsLetter(prior)
			}
		}
		// "May" the month is not the modal.
		word.capital = unicode.IsUpper([]rune(raw)[0])
		if n := len(s.words); n > 0 && s.words[n-1].quoted != word.quoted {
			pending = true
		}
		s.words = append(s.words, word)
		breaks = append(breaks, pending)
		pending = false
	}
	s.breaks = breaks
	s.segment()
	return s
}

func (s *wordingSentence) text(i int) string {
	if i < 0 || i >= len(s.words) {
		return ""
	}
	return s.words[i].text
}

// segment splits the words into clauses and marks each clause.
func (s *wordingSentence) segment() {
	if len(s.words) == 0 {
		return
	}
	breaks := s.breaks
	starts := []int{0}
	open := func(i int) {
		if i > starts[len(starts)-1] && i < len(s.words) {
			starts = append(starts, i)
		}
	}
	for i := range s.words {
		w := s.text(i)
		next := s.text(i + 1)
		switch {
		case breaks[i]:
			open(i)
		case wordingClauseStarters[w]:
			open(i)
		case w == "so" && bindingSoSubjects[next]:
			open(i)
		case (w == "and" || w == "or") && (wordingSubjectWords[next] || wordingPossessives[next] || wordingAuxiliaries[next] || wordingNegationWords[next]):
			open(i)
		}
		if s.reportingAt(i) {
			open(s.complementAt(i))
		}
	}
	for n, first := range starts {
		last := len(s.words)
		if n+1 < len(starts) {
			last = starts[n+1]
		}
		c := wordingClause{first: first, last: last}
		c.lead = s.leadOf(first, last)
		s.clauses = append(s.clauses, c)
		for i := first; i < last; i++ {
			s.words[i].clause = n
		}
	}
	s.markClauses()
}

// reportingAt reports a reporting verb at word i whose complement follows:
// "said, ...", "says the", "told me Boston", "think the Celtics".
func (s *wordingSentence) reportingAt(i int) bool {
	w := s.text(i)
	if !wordingReportingVerbs[w] || wordingReportingBase[w] && !wordingReportingAfter[s.text(i-1)] {
		return false
	}
	j := s.complementAt(i)
	if j >= len(s.words) || s.breaks[j] {
		return true // "My sister said, Boston is ..."
	}
	next := s.text(j)
	return !wordingAuxiliaries[next] && !wordingNotComplement[next] && !unicode.IsDigit([]rune(next)[0])
}

// complementAt is the first word of the complement of a reporting verb at i,
// after an object pronoun and "that".
func (s *wordingSentence) complementAt(i int) int {
	j := i + 1
	if wordingReportedObject[s.text(j)] {
		j++
	}
	if s.text(j) == "that" {
		j++
	}
	return j
}

// leadOf skips leading conjunctions, discourse words and memory cues.
func (s *wordingSentence) leadOf(first, last int) int {
	i, skipped := first, false
	for i < last {
		w, next := s.text(i), s.text(i+1)
		switch {
		case wordingLeadSkips[w]:
			i++
		case w == "that" && skipped:
			i++
		case (w == "don" || w == "dont" || w == "never") && (next == "forget" || next == "t" && s.text(i+2) == "forget"):
			i++
		case w == "t" && s.words[i].apostrophe && next == "forget":
			i++
		case w == "not" && next == "only":
			i += 2
			if setOf("do", "does", "did")[s.text(i)] {
				i++
			}
		case w == "you" && next == "know":
			i += 2
		default:
			return i
		}
		skipped = true
	}
	return last
}

// markClauses sets each clause's reported, conditional, question, change and
// inherits flags.
func (s *wordingSentence) markClauses() {
	according := false
	for i := range s.words {
		according = according || s.text(i) == "according" && s.text(i+1) == "to"
	}
	reporting := false // a reporting verb by someone other than the owner
	for n := range s.clauses {
		c := &s.clauses[n]
		lead := s.text(c.lead)
		if n > 0 && (wordingAuxiliaries[lead] || wordingNegationWords[lead] || wordingChangeCues[lead] || wordingReportingVerbs[lead]) && c.lead > c.first {
			c.inherits = true
		}
		if n > 0 && wordingContrasts[s.text(c.first)] && (lead == "i" || lead == "we") {
			reporting = false
		}
		c.reported = according || reporting || s.words[c.first].quoted
		reporter := s.reporter(n)
		for i := c.first; i < c.last; i++ {
			if s.reportingAt(i) && (wordingHearsay[s.text(i)] || reporter != "i" && reporter != "we") {
				reporting = true
			}
		}
		c.conditional = wordingConditionalStarts[s.text(c.first)] || wordingConditionalStarts[lead]
		for i := c.first; i < c.last && !c.conditional; i++ {
			w, next := s.text(i), s.text(i+1)
			switch {
			case w == "d" && s.words[i].apostrophe, w == "would" || w == "wouldn":
				c.conditional = !wordingPreferenceVerbs[next]
			case w == "may":
				c.conditional = !s.words[i].capital && (next == "" || !unicode.IsDigit([]rune(next)[0]))
			case w == "won" && next == "t":
				c.conditional = true
			case wordingIntentVerbs[w] && next == "to", w == "thinking" && (next == "about" || next == "of"), w == "considering":
				c.conditional = true
			case wordingConditionalWords[w]:
				c.conditional = true
			}
		}
		c.question = s.question && (n == len(s.clauses)-1 || wordingQuestionLeads[lead])
		for i := c.first; i < c.last; i++ {
			w, next := s.text(i), s.text(i+1)
			switch {
			case wordingEndedWords[w], w == "used" && next == "to", w == "no" && next == "longer", w == "behind" && (next == "me" || next == "us"):
				c.change = true
			case wordingMovingWords[w]:
				for j := i + 1; j < c.last; j++ {
					c.change = c.change || wordingAwayWords[s.text(j)]
				}
			}
		}
	}
}

// reporter is the first word of clause n's subject, carried back through
// clauses that inherit it.
func (s *wordingSentence) reporter(n int) string {
	for n > 0 && s.clauses[n].inherits {
		n--
	}
	return s.text(s.clauses[n].lead)
}

// phrase returns the end of the noun phrase starting at i, up to four words
// and stopping at a function word or a verb-like word, and the index of a
// possessive "'s" inside it, or -1.
func (s wordingSentence) phrase(i, last int) (end, possessive int) {
	return phraseEnd(s.words, i, last)
}

func phraseEnd(words []wordingWord, i, last int) (end, possessive int) {
	possessive = -1
	text := func(j int) string {
		if j < len(words) {
			return words[j].text
		}
		return ""
	}
	for end = i; end < last && end < i+4; end++ {
		w := words[end]
		if w.text == "s" && w.apostrophe {
			if possessive >= 0 || end+1 >= last || wordingPhraseStops[text(end+1)] || wordingAuxiliaries[text(end+1)] {
				break // "'s" as "is"
			}
			possessive = end
			continue
		}
		if wordingPhraseStops[w.text] || wordingAuxiliaries[w.text] || wordingReportingVerbs[w.text] || wordingChangeCues[w.text] ||
			wordingEndedWords[w.text] || end > i && likelyVerb(w.text) {
			break
		}
	}
	return end, possessive
}

// likelyVerb is a third-person or past form ("lives", "moved"): it ends a
// phrase that is not its first word.
func likelyVerb(word string) bool {
	if wordingPhraseStops[word] || wordingPossessives[word] || wordingNotVerbs[word] {
		return false
	}
	return len(word) >= 4 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss") || len(word) >= 5 && strings.HasSuffix(word, "ed")
}

// Common words ending in -s or -ed that are not verbs.
var wordingNotVerbs = setOf("this", "his", "hers", "ours", "yours", "theirs", "yes", "plus", "thus", "always", "perhaps", "towards", "afterwards",
	"besides", "whereas", "news", "series", "lens", "bus", "gas", "less", "unless", "indeed", "need", "speed", "seed", "bed", "red", "shed", "weed")

// possessiveOwner reports whether "my" or "our" at i owns the Claim's
// Predicate words ("my home city", "my sister's birthday"), and whether the
// phrase instead names a person or a possessor ("my therapist", "my dad's").
// For a Claim about someone else, a phrase naming that subject ("my mother
// Maya") is neither: it is the Claim's subject.
func (s wordingSentence) possessiveOwner(i, last int, v wordingVocabulary) (owner, other bool) {
	end, possessive := s.phrase(i+1, last)
	words := s.words[i+1 : end]
	if possessive >= 0 {
		words = s.words[i+1 : possessive]
	}
	named := false
	for _, word := range words {
		owner = owner || v.predicate(word)
		other = other || wordingPeople[word.text]
		named = named || !v.owner && v.claim(word)
	}
	return owner, !owner && !named && (other || possessive >= 0)
}

// subject classifies clause n's subject for one Claim's vocabulary.
func (s wordingSentence) subject(n int, v wordingVocabulary) wordingSubject {
	for n > 0 && s.clauses[n].inherits {
		n--
	}
	c := s.clauses[n]
	i := c.lead
	if i >= c.last {
		return subjectNone
	}
	w := s.text(i)
	switch {
	case wordingOwnerPronouns[w]:
		return subjectOwner
	case v.claim(s.words[i]):
		return subjectClaim // "Boston is behind me", "The Overstory is ..."
	case w == "my" || w == "our":
		owner, other := s.possessiveOwner(i, c.last, v)
		switch {
		case owner:
			return subjectOwner
		case other:
			return subjectOther
		}
		end, _ := s.phrase(i+1, c.last)
		for _, word := range s.words[i+1 : end] {
			if !v.owner && v.claim(word) {
				return subjectClaim
			}
		}
		return subjectNone
	case wordingPossessives[w], wordingOtherSubjects[w]:
		return subjectOther
	case wordingOtherPronouns[w]:
		return subjectPronoun
	case wordingImpersonal[w]:
		return subjectNone
	case wordingDeterminers[w]:
		end, _ := s.phrase(i+1, c.last)
		for _, word := range s.words[i+1 : end] {
			if v.claim(word) {
				return subjectClaim
			}
		}
		return subjectOther
	}
	end, possessive := s.phrase(i, c.last)
	if possessive >= 0 {
		for _, word := range s.words[i:possessive] {
			if v.claim(word) {
				return subjectClaim // "Maya's sister" for Maya's sister
			}
		}
		return subjectOther // "Selma's favorite color", "Alex's employer"
	}
	for _, word := range s.words[i:end] {
		if v.claim(word) {
			return subjectClaim
		}
	}
	if wordingPeople[w] {
		return subjectOther
	}
	// A name followed by a verb: "Selma likes", "Alex works". A lower-case
	// word may as well be an imperative or a plural ("pack my gloves").
	if next := s.text(end); s.words[i].capital && end < c.last && (wordingAuxiliaries[next] || wordingReportingVerbs[next] || likelyVerb(next)) {
		return subjectOther
	}
	return subjectNone
}

// ownerSubjectClause reports a clause of the sentence, neither reported nor
// conditional, whose subject is the owner.
func (s wordingSentence) ownerSubjectClause(v wordingVocabulary) bool {
	for n, c := range s.clauses {
		if !c.reported && !c.conditional && s.subject(n, v) == subjectOwner {
			return true
		}
	}
	return false
}

// negations counts the negations in clause n that reach the word range
// [from, to): a "not" right after that range ("Cambridge not Boston", "4 not
// 5") negates only what follows it. "don't forget", "never forget", "not
// only" and "no doubt" are not negations.
func (s wordingSentence) negations(n, from, to int) int {
	c := s.clauses[n]
	count := 0
	for i := c.first; i < c.last; i++ {
		w, next := s.text(i), s.text(i+1)
		negation := wordingNegationWords[w] || w == "t" && s.words[i].apostrophe && i > 0 && strings.HasSuffix(s.text(i-1), "n")
		if !negation || next == "forget" || w == "not" && next == "only" || w == "no" && next == "doubt" {
			continue
		}
		if i == to && w == "not" && next != "" {
			continue
		}
		count++
	}
	return count
}
