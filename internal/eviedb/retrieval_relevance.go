package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Automatic Recall's relevance floor (harness review M1, retrieval decision
// 7: do not pad a result with weak evidence). Every term comes from the
// bounded conversation and every frequency from the searchable history. No
// topic or preference dictionary is involved; the only fixed language data
// are the planner's English function words and the inflection suffixes below.
const (
	relevanceTermLimit  = 48
	relevanceGroupLimit = 4
	// A request with this many content terms is not explained by one shared
	// word; shorter requests may be, if the word is distinctive.
	relevanceMultiTermRequest = 3
)

func validRetrievalRelevance(relevance *memory.RetrievalRelevance) bool {
	if relevance == nil {
		return true
	}
	if len(relevance.Context) > relevanceGroupLimit {
		return false
	}
	seen := map[string]bool{}
	for _, group := range append([][]string{relevance.Current}, relevance.Context...) {
		for _, term := range group {
			if term == "" || len(term) > 128 || !utf8.ValidString(term) || strings.ToLower(term) != term {
				return false
			}
			for _, r := range term {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					return false
				}
			}
			seen[term] = true
		}
	}
	return len(seen) <= relevanceTermLimit
}

// relevanceKey folds common inflection so "runs", "running" and "run" (or
// "booked" and "book") count as one word when an excerpt is checked. Candidate
// fetching stays exact; this only stops the floor from rejecting a fetched
// excerpt over a word form.
func relevanceKey(word string) string {
	switch {
	case len(word) >= 6 && strings.HasSuffix(word, "ing"):
		word = word[:len(word)-3]
	case len(word) >= 5 && strings.HasSuffix(word, "ed"):
		word = word[:len(word)-2]
	case len(word) >= 4 && strings.HasSuffix(word, "s") && !strings.HasSuffix(word, "ss"):
		word = word[:len(word)-1]
	}
	if n := len(word); n >= 4 && word[n-1] == word[n-2] && word[n-1] >= 'a' && word[n-1] <= 'z' && !strings.ContainsRune("aeiouls", rune(word[n-1])) {
		word = word[:n-1]
	}
	if n := len(word); n >= 4 && word[n-1] == 'e' {
		word = word[:n-1]
	}
	return word
}

func relevanceTokens(text string) map[string]bool {
	tokens := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		tokens[relevanceKey(word)] = true
	}
	return tokens
}

// distinctiveTerms is the rarer half of a group's known vocabulary: terms
// that occur in the searchable history no more often than the group's median
// present term. A term absent from history cannot be matched, so it neither
// qualifies evidence nor moves the median.
func distinctiveTerms(group []string, frequency map[string]int) map[string]bool {
	var counts []int
	for _, term := range group {
		if frequency[term] > 0 {
			counts = append(counts, frequency[term])
		}
	}
	distinctive := map[string]bool{}
	if len(counts) == 0 {
		return distinctive
	}
	slices.Sort(counts)
	median := counts[(len(counts)-1)/2]
	for _, term := range group {
		if count := frequency[term]; count > 0 && count <= median {
			distinctive[term] = true
		}
	}
	return distinctive
}

// relevanceGroupMatched reports whether tokens cover at least need distinct
// group terms, one of them distinctive.
func relevanceGroupMatched(tokens map[string]bool, group []string, frequency map[string]int, need int) bool {
	distinctive := distinctiveTerms(group, frequency)
	matched, anchored := 0, false
	seen := map[string]bool{}
	for _, term := range group {
		key := relevanceKey(term)
		if seen[key] || !tokens[key] {
			continue
		}
		seen[key] = true
		matched++
		anchored = anchored || distinctive[term]
	}
	return anchored && matched >= need
}

// relevantToRequest is the floor for one excerpt. It must match a distinctive
// term of the current request, and two of its terms when the request has
// three or more; or, for a follow-up, a distinctive term of an earlier topic
// the request depends on.
func relevantToRequest(text string, relevance *memory.RetrievalRelevance, frequency map[string]int) bool {
	tokens := relevanceTokens(text)
	need := 1
	if len(relevance.Current) >= relevanceMultiTermRequest {
		need = 2
	}
	if relevanceGroupMatched(tokens, relevance.Current, frequency, need) {
		return true
	}
	for _, group := range relevance.Context {
		if relevanceGroupMatched(tokens, group, frequency, 1) {
			return true
		}
	}
	return false
}

// relevanceFrequencies counts, for each relevance term, the documents this
// search could return: the same scope, generation, observation bound and
// live-context exclusion as the candidate read.
func relevanceFrequencies(ctx context.Context, tx *sql.Tx, relevance *memory.RetrievalRelevance, scopeKey string, known time.Time, session memory.SessionID, cutoff int64) (map[string]int, error) {
	frequency := map[string]int{}
	for _, group := range append([][]string{relevance.Current}, relevance.Context...) {
		for _, term := range group {
			if _, done := frequency[term]; done {
				continue
			}
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM memory_retrieval_event_fts_v3 f JOIN events e ON e.id=f.event_id
 WHERE memory_retrieval_event_fts_v3 MATCH ? AND f.generation=? AND f.scope_key=?
 AND `+conversationObservedTimeSQL+`<=? AND (e.session_id!=? OR e.sequence<?)`,
				`"`+term+`"`, conversationIndexGeneration, scopeKey, formatSemanticTime(known), session, cutoff).Scan(&count); err != nil {
				return nil, err
			}
			frequency[term] = count
		}
	}
	return frequency, nil
}

// conversationSessionCutoff is the first sequence of the bound session that
// conversation recall must not return: the active request onward, and for
// Automatic Recall every message still in the provider request.
func conversationSessionCutoff(ctx context.Context, q semanticInspectionQueryer, session memory.SessionID, relevance *memory.RetrievalRelevance) (int64, error) {
	var cutoff int64
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=? AND event_type='user_message'`, session).Scan(&cutoff); err != nil {
		return 0, err
	}
	if relevance == nil {
		return cutoff, nil
	}
	if relevance.LiveFrom == "" {
		return 0, nil
	}
	var live int64
	err := q.QueryRowContext(ctx, `SELECT sequence FROM events WHERE id=? AND session_id=?`, relevance.LiveFrom, session).Scan(&live)
	if errors.Is(err, sql.ErrNoRows) {
		// An unknown frontier cannot widen recall; treat the whole session as live.
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return min(cutoff, live), nil
}
