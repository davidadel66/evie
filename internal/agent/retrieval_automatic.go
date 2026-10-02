package agent

import (
	"context"
	"os"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/google/uuid"
)

const automaticRecallVersion = "bounded-reference-lexical-v3"

const automaticReferenceReadingGuide = "Resolve references using relevant earlier discussion and eligible original sources; compaction continuity and proposed identities are hypotheses, not new facts. Use a targeted memory read if useful. When remaining recipient ambiguity materially changes the answer, ask one focused question before giving person-specific recommendations. Do not ask merely because multiple people are known when context already identifies the recipient."

// This plan selects interpretation input, not additional source evidence. Only
// the Kernel's subsequent scoped search can supply a source-bearing result.
type automaticRecallPlan struct {
	query       string
	exact       []string
	relevance   memory.RetrievalRelevance
	diagnostics memory.RetrievalInterpretation
}

// A follow-up of at most this many content terms may depend on an earlier
// topic it shares no word with ("and the basil?", "what dates did I book?").
const automaticShortFollowUpTerms = 2

func planAutomaticRecall(events []memory.Event, summary *ContextSummary, root memory.EventID) automaticRecallPlan {
	plan := automaticRecallPlan{diagnostics: memory.RetrievalInterpretation{Version: automaticRecallVersion, Outcome: "insufficient"}}
	var current string
	var earlier []memory.Event
	for _, event := range events {
		if event.Type != memory.EventUserMessage || event.Role != memory.RoleUser {
			continue
		}
		if event.ID == root {
			current = boundedRecallText(event.Content, 512)
			break
		}
		earlier = append(earlier, event)
	}
	// Inspect at most the last sixteen roots; distribute the remaining query
	// quota across relevant earlier roots and continuity instead of concatenating
	// an unbounded transcript or letting a long latest message consume it all.
	if len(earlier) > 16 {
		earlier = earlier[len(earlier)-16:]
	}
	plan.exact = explicitRecallSelectors(current)
	plan.diagnostics.ExactSelectors = len(plan.exact)
	for _, selector := range plan.exact {
		plan.diagnostics.ExactQueryBytes += len(selector)
	}
	terms := recallTerms(current, 16)
	plan.diagnostics.CurrentBytes = len(current)
	plan.relevance.PersonalRecall = personalRecallRequest(current)
	plan.relevance.Current = append([]string(nil), terms...)
	if summary != nil {
		// Messages before the retained frontier left the provider request
		// and may be recalled again; later ones are still visible.
		plan.relevance.LiveFrom = summary.FirstRetainedEventID
	}
	refers := refersToEarlierSubject(current)
	if len(plan.exact) == 0 && (len(terms) == 0 && !refers || acknowledgesLiveContext(current, terms)) {
		// Acknowledgements and other low-content messages ask for nothing, so
		// they neither search history nor revive an earlier topic. "Got it",
		// "love it" and "ok do it" refer to the live context, not to history.
		return plan
	}
	if !plan.relevance.PersonalRecall {
		// Filler ("need", "use", "know") is dropped only from a request to
		// recall something about the owner. Elsewhere it may be what the
		// request is about ("Do I need to use a VPN for the bank?"), and
		// dropping it would let one shared word ("bank") explain the request
		// (confirmation review).
		terms = recallTermsWithFiller(current, 16)
		plan.relevance.Current = append([]string(nil), terms...)
	}
	// Only a follow-up may take its subject from an earlier topic it shares
	// no word with. A standalone request keeps its own subject; earlier roots
	// that share one of its words only help rank its matches.
	followUp := refers || len(terms) <= automaticShortFollowUpTerms
	var continuityTerms []string
	if summary != nil {
		continuity := boundedRecallText(summary.Content, 512)
		continuityTerms = recallTerms(continuity, 6)
		plan.diagnostics.SummaryBytes = len(continuity)
	}
	relevanceTerms := appendRecallTerms(append([]string(nil), terms...), continuityTerms, 32)
	type contextCandidate struct {
		text            string
		terms           []string
		index           int
		score           int
		own             int
		distinctiveness int
	}
	var candidates []contextCandidate
	frequency := make(map[string]int)
	for i, event := range earlier {
		text := boundedRecallText(event.Content, 384)
		plan.diagnostics.ExaminedEarlierMessages++
		plan.diagnostics.ExaminedEarlierBytes += len(text)
		candidate := contextCandidate{text: text, terms: recallTerms(text, 32), index: i}
		if len(candidate.terms) == 0 || acknowledgesLiveContext(text, candidate.terms) {
			// An acknowledgement is not a topic; it must not take a slot
			// from the subject a follow-up refers to.
			continue
		}
		for _, word := range candidate.terms {
			frequency[word]++
			for _, term := range relevanceTerms {
				if word == term {
					candidate.score++
				}
			}
			if slices.Contains(terms, word) {
				candidate.own++
			}
		}
		// Validated continuity provides a topic anchor after compaction. Do not
		// spend its small query/result budget on unrelated retained discussion.
		if len(continuityTerms) > 0 && candidate.score == 0 || !followUp && candidate.own == 0 {
			continue
		}
		candidates = append(candidates, candidate)
	}
	for i := range candidates {
		for _, word := range candidates[i].terms {
			candidates[i].distinctiveness += 1024 / frequency[word]
		}
		if len(candidates[i].terms) > 0 {
			candidates[i].distinctiveness /= len(candidates[i].terms)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		// Repeated topic noise must not crowd out a distinct earlier subject.
		// Mean inverse frequency stays independent of domain vocabulary and
		// prevents a long message from winning merely by containing more words.
		if candidates[i].distinctiveness != candidates[j].distinctiveness {
			return candidates[i].distinctiveness > candidates[j].distinctiveness
		}
		// When nothing else separates them, a reference ("and when was it?")
		// most likely means the most recent topic.
		return candidates[i].index > candidates[j].index
	})
	if len(candidates) > 2 {
		candidates = candidates[:2]
	}
	for _, candidate := range candidates {
		topic := recallTerms(candidate.text, 5)
		terms = appendRecallTerms(terms, topic, 26)
		if followUp && len(topic) > 0 {
			plan.relevance.Context = append(plan.relevance.Context, topic)
		}
		plan.diagnostics.EarlierMessages++
		plan.diagnostics.EarlierBytes += len(candidate.text)
	}
	if followUp && len(continuityTerms) > 0 {
		terms = appendRecallTerms(terms, continuityTerms, 32)
		plan.relevance.Context = append(plan.relevance.Context, continuityTerms)
	} else if slices.ContainsFunc(continuityTerms, func(term string) bool { return slices.Contains(plan.relevance.Current, term) }) {
		terms = appendRecallTerms(terms, continuityTerms, 32)
	}
	plan.query = boundedRecallText(strings.Join(terms, " "), 1024)
	plan.diagnostics.QueryBytes = len(plan.query)
	if plan.query != "" {
		plan.diagnostics.Outcome = "searched"
	}
	return plan
}

// Explicit selectors are query hypotheses, never identities accepted by this
// planner. The existing Kernel search must resolve every token in current scope.
func explicitRecallSelectors(text string) []string {
	var selectors []string
	for _, field := range strings.Fields(text) {
		field = strings.TrimFunc(field, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if len(field) == 0 || len(field) > 128 {
			continue
		}
		if id, err := uuid.Parse(field); err == nil {
			selectors = appendRecallTerms(selectors, []string{id.String()}, 2)
			continue
		}
		letter, digit, separator, valid := false, false, false, true
		for _, r := range field {
			switch {
			case unicode.IsLetter(r):
				letter = true
			case unicode.IsDigit(r):
				digit = true
			case r == '-' || r == '_' || r == ':':
				separator = true
			default:
				valid = false
			}
		}
		if valid && letter && digit && separator {
			selectors = appendRecallTerms(selectors, []string{field}, 2)
		}
	}
	return selectors
}

func boundedRecallText(text string, limit int) string {
	if !utf8.ValidString(text) || memory.HasRetrievalSecret([]byte(text)) {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

// Pronouns refer back to someone or something already under discussion
// ("what present would she like?", "what was it again?"); they are noise as
// query terms but mark the message as a follow-up.
const recallReferringPronouns = " he she him her his hers they them their theirs it its "

func refersToEarlierSubject(text string) bool {
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if strings.Contains(recallReferringPronouns, " "+word+" ") {
			return true
		}
	}
	return false
}

// Words that ask for information. A message without one of them and without
// a question mark is not asking about history.
const recallInformationWords = " what which who whom whose how when where why tell remind recall remember show explain find search look describe "

// acknowledgesLiveContext reports an acknowledgement or command about what is
// already in the conversation: "got it, thanks", "that's it", "love it", "ok
// do it". It says "it", asks no question, and carries at most one content
// word, so it neither searches nor revives an earlier topic. "And the
// basil?" (a question) and "her birthday" (no "it") remain follow-ups.
func acknowledgesLiveContext(text string, terms []string) bool {
	if len(terms) > 1 || strings.Contains(text, "?") {
		return false
	}
	it := false
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if strings.Contains(recallInformationWords, " "+word+" ") {
			return false
		}
		it = it || word == "it"
	}
	return it
}

// Function words, pronouns and acknowledgements are query noise, not a
// domain-specific preference or entity dictionary. Every content term still
// comes from the bounded conversation.
const recallNoise = " a an the and or but if is are was were be been being am i me my mine you your yours we our us this that these those to of for from in on at with about as by do does did have has had can could would should will shall may might please tell find search look up recall remember saved memory original conversation statement evidence what which who how when where why now then suggest" +
	recallReferringPronouns + "thanks thank thx ok okay cool great perfect nice awesome good sounds yes yeah yep sure alright got lol haha hi hello hey bye cheers "

// Conversational filler ("ok so", "again", "do you know", "remind me", "do I
// need to", "which one we use") pads a request to recall something without
// saying what it is about.
const recallFiller = " so again also just really actually maybe anyway oh um uh hmm btw remind know need use "

// recallTerms returns the content terms of text, without noise or filler.
func recallTerms(text string, limit int) []string {
	return recallTermsExcept(text, limit, recallNoise+recallFiller)
}

// recallTermsWithFiller keeps the filler words: in a request that is not a
// personal recall they may be its subject ("use a VPN").
func recallTermsWithFiller(text string, limit int) []string {
	return recallTermsExcept(text, limit, recallNoise)
}

func recallTermsExcept(text string, limit int, noise string) []string {
	var terms []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) < 2 || strings.Contains(noise, " "+word+" ") {
			continue
		}
		terms = appendRecallTerms(terms, []string{word}, limit)
	}
	return terms
}

// personalRecallRequest reports a request to recall something about the
// owner: it refers to the owner ("I", "my", "we", "our") and asks a recall
// question or request ("what", "when", "where", "which", "who", "how", "do
// you know", "do you remember", "remind me", "again"). "When do I need to
// renew my passport?" and "Remind me which vet we use for the cat" are;
// "Explain technical debt to the new engineers." and "Write a short poem
// about anxiety for my newsletter." are not. Only such a request has its
// filler dropped and may be explained by one nearly unique word
// (confirmation review).
func personalRecallRequest(text string) bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	owner, recall := false, false
	for i, word := range words {
		next, after := "", ""
		if i+1 < len(words) {
			next = words[i+1]
		}
		if i+2 < len(words) {
			after = words[i+2]
		}
		switch word {
		case "i", "me", "my", "mine", "myself", "we", "us", "our", "ours":
			owner = true
		case "what", "when", "where", "which", "who", "whom", "whose", "how", "again":
			recall = true
		case "remind":
			recall = recall || next == "me" || next == "us"
		case "do":
			recall = recall || next == "you" && (after == "know" || after == "remember")
		}
	}
	return owner && recall
}

func appendRecallTerms(terms, additions []string, limit int) []string {
	for _, word := range additions {
		if len(terms) >= limit {
			break
		}
		found := false
		for _, term := range terms {
			found = found || word == term
		}
		if !found {
			terms = append(terms, word)
		}
	}
	return terms
}

func (r *retrievalTurn) automatic(ctx context.Context, events []memory.Event, summary *ContextSummary, root memory.EventID, schemas []openrouter.Tool) {
	kinds := make(map[string]bool)
	memoryTools := false
	for _, schema := range schemas {
		memoryTools = memoryTools || strings.HasPrefix(schema.Function.Name, "memory_")
		switch schema.Function.Name {
		case "memory_search":
			kinds[memory.RetrievalAcceptedMemory] = true
		case "memory_search_conversations":
			kinds[memory.RetrievalConversationExcerpt] = true
		}
	}
	if len(kinds) == 0 {
		// Fresh opt-out compositions omit model-facing reads. They can still
		// report unavailable memory without performing a lookup or disclosing IDs.
		if memoryTools && os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
			r.status = memory.RetrievalUnavailable
		}
		return
	}
	plan := planAutomaticRecall(events, summary, root)
	r.interpretation = &plan.diagnostics
	if r.kernel == nil || os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		r.status = memory.RetrievalUnavailable
		return
	}
	if plan.query == "" {
		r.status = memory.RetrievalEmpty
		return
	}
	// At most two explicit selector hypotheses follow the two independent
	// evidence kinds, with at most two selected results from each search.
	// The same turn ledger leaves the remaining capacity for model-directed
	// follow-up; automatic success never resets the shared work/context budget.
	var outcomes []string
	for _, kind := range []string{memory.RetrievalAcceptedMemory, memory.RetrievalConversationExcerpt} {
		if !kinds[kind] {
			continue
		}
		query := memory.RetrievalQuery{Kind: kind, Text: plan.query, Limit: 2, MaxBytes: 6 * 1024, ExcludeCurrentRequestCopies: true}
		if kind == memory.RetrievalConversationExcerpt {
			// Raw conversation is the noisy kind: its excerpts must clear the
			// relevance floor and not repeat what the request already shows.
			relevance := plan.relevance
			query.Relevance = &relevance
		}
		result, _ := r.search(ctx, query)
		outcomes = append(outcomes, result.Status)
	}
	if kinds[memory.RetrievalAcceptedMemory] {
		for _, selector := range plan.exact {
			result, _ := r.search(ctx, memory.RetrievalQuery{Kind: memory.RetrievalAcceptedMemory, Text: selector, Limit: 2, MaxBytes: 6 * 1024, ExcludeCurrentRequestCopies: true})
			outcomes = append(outcomes, result.Status)
		}
	}
	r.status = combinedRecallStatus(outcomes)
	r.interpretation.Outcome = r.status
}

func combinedRecallStatus(outcomes []string) string {
	if len(outcomes) > 0 {
		same := true
		for _, outcome := range outcomes[1:] {
			same = same && outcome == outcomes[0]
		}
		if same {
			return outcomes[0]
		}
	}
	status := memory.RetrievalEmpty
	incomplete := ""
	for _, outcome := range outcomes {
		switch outcome {
		case memory.RetrievalCancelled:
			return outcome
		case memory.RetrievalSuccess:
			status = outcome
		case memory.RetrievalEmpty:
		case memory.RetrievalPartial, memory.RetrievalFailed, memory.RetrievalUnavailable, memory.RetrievalExhausted:
			incomplete = outcome
		}
	}
	if incomplete != "" {
		if len(outcomes) > 1 {
			return memory.RetrievalPartial
		}
		return incomplete
	}
	return status
}
