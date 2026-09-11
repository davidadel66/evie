package agent

import (
	"context"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/google/uuid"
)

const automaticRecallVersion = "bounded-reference-lexical-v2"

const automaticReferenceReadingGuide = "Resolve references using relevant earlier discussion and eligible original sources; compaction continuity and proposed identities are hypotheses, not new facts. Use a targeted memory read if useful. When remaining recipient ambiguity materially changes the answer, ask one focused question before giving person-specific recommendations. Do not ask merely because multiple people are known when context already identifies the recipient."

// This plan selects interpretation input, not additional source evidence. Only
// the Kernel's subsequent scoped search can supply a source-bearing result.
type automaticRecallPlan struct {
	query       string
	exact       []string
	diagnostics memory.RetrievalInterpretation
}

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
		distinctiveness int
	}
	var candidates []contextCandidate
	frequency := make(map[string]int)
	for i, event := range earlier {
		text := boundedRecallText(event.Content, 384)
		plan.diagnostics.ExaminedEarlierMessages++
		plan.diagnostics.ExaminedEarlierBytes += len(text)
		candidate := contextCandidate{text: text, terms: recallTerms(text, 32), index: i}
		for _, word := range candidate.terms {
			frequency[word]++
			for _, term := range relevanceTerms {
				if word == term {
					candidate.score++
				}
			}
		}
		// Validated continuity provides a topic anchor after compaction. Do not
		// spend its small query/result budget on unrelated retained discussion.
		if len(continuityTerms) > 0 && candidate.score == 0 {
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
		// Keep an earlier subject in the bounded window when lexical overlap
		// ties; recency alone cannot resolve an ambiguous reference.
		return candidates[i].index < candidates[j].index
	})
	if len(candidates) > 2 {
		candidates = candidates[:2]
	}
	for _, candidate := range candidates {
		terms = appendRecallTerms(terms, recallTerms(candidate.text, 5), 26)
		plan.diagnostics.EarlierMessages++
		plan.diagnostics.EarlierBytes += len(candidate.text)
	}
	terms = appendRecallTerms(terms, continuityTerms, 32)
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

func recallTerms(text string, limit int) []string {
	// Function words are query noise, not a domain-specific preference or entity
	// dictionary. Every content term still comes from the bounded conversation.
	const noise = " a an the and or but if is are was were be been being am i me my mine you your yours we our us it its this that these those to of for from in on at with about as by do does did have has had can could would should will shall may might please tell find search look up recall remember saved memory original conversation statement evidence what which who how when where why now then suggest "
	var terms []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) < 2 || strings.Contains(noise, " "+word+" ") {
			continue
		}
		terms = appendRecallTerms(terms, []string{word}, limit)
	}
	return terms
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
		result, _ := r.search(ctx, memory.RetrievalQuery{Kind: kind, Text: plan.query, Limit: 2, MaxBytes: 6 * 1024, ExcludeCurrentRequestCopies: true})
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
