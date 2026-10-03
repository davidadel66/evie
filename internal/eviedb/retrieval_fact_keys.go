package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Fact-augmented keys (memory architecture Stage 6; LongMemEval's key
// expansion). A conversation message that is a Source of an accepted, current
// Claim is also indexed under that Claim's words: its Predicate label and
// token, its literal value, and the names and aliases of its Entities other
// than the owner and Evie. A question phrased like the saved fact ("my blood
// type") then finds the original message even when the message said it
// differently ("I'm O negative"). Keys only match: an excerpt still renders
// the message's own words under the ordinary rules, and every key is
// recomputed from accepted state when a hit is read, so a stale index cannot
// match through a Claim that is no longer current.
const (
	// conversationFactKeyClaims bounds the Claims one message is keyed by.
	conversationFactKeyClaims = 32
	// conversationFactKeyAliases bounds the aliases read per Entity.
	conversationFactKeyAliases = 16
	// conversationFactKeyBytes bounds one message's key text.
	conversationFactKeyBytes = 4096
)

// conversationFactKey is one Claim's key on a message: its words, and the
// span of the message its Source Link cites.
type conversationFactKey struct {
	claim memory.SemanticID
	span  evidenceSpan
	words []string
}

// conversationFactKeys reads the keys of message e as known at known: the
// accepted Claims that are currently active, whose eligible, non-Evie-proposed
// Source Link cites e's content with a verified locator, in a scope every
// reader of e may read. Conversation search reads e only from e's own area, so
// that is Global and e's own Workspace or project; a session-scoped Claim is
// never a key, because other sessions of the area may not read it. Entity
// names and aliases join only from those same scopes, so no key carries words
// its readers could not already read as accepted memory.
func conversationFactKeys(ctx context.Context, q semanticInspectionQueryer, e conversationEvidence, known time.Time) ([]conversationFactKey, error) {
	rows, err := q.QueryContext(ctx, `SELECT sl.claim_id,sl.event_part,sl.locator_kind,sl.locator_value,sl.evidence_sha256,
 c.subject_entity_id,COALESCE(c.object_entity_id,''),COALESCE(c.literal_value,''),c.predicate_token,p.label
 FROM semantic_source_links sl JOIN semantic_claims c ON c.claim_id=sl.claim_id
 JOIN semantic_scopes sc ON sc.scope_id=c.scope_id JOIN semantic_predicates p ON p.predicate_id=c.predicate_id
 WHERE sl.event_id=? AND sl.event_part='content' AND sl.authority<>'evie_proposed'
 AND sc.scope_key IN ('global',?) AND c.transaction_time<=?
 AND COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=c.claim_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),c.lifecycle)='active'
 AND COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=sl.source_link_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),sl.eligibility)='eligible'
 ORDER BY sl.source_link_id LIMIT ?`, e.id, e.scope, formatSemanticTime(known), conversationFactKeyClaims)
	if err != nil {
		return nil, err
	}
	type row struct {
		claim                          memory.SemanticID
		locator                        memory.EvidenceLocator
		subject, object                memory.SemanticID
		literal, token, predicateLabel string
	}
	var found []row
	for rows.Next() {
		var r row
		r.locator.EventID = e.id
		if err = rows.Scan(&r.claim, &r.locator.EventPart, &r.locator.LocatorKind, &r.locator.LocatorValue, &r.locator.EvidenceSHA256,
			&r.subject, &r.object, &r.literal, &r.token, &r.predicateLabel); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	var keys []conversationFactKey
	names := map[memory.SemanticID][]string{}
	for _, r := range found {
		span, err := conversationLocatorSpan(e, r.locator)
		if err != nil {
			// An association that no longer verifies is not a key; retrieval
			// rules elsewhere decide what the message itself may show.
			continue
		}
		parts := []string{r.predicateLabel, r.token, r.literal}
		for _, entity := range []memory.SemanticID{r.subject, r.object} {
			if entity == "" {
				continue
			}
			if _, done := names[entity]; !done {
				if names[entity], err = conversationFactKeyNames(ctx, q, entity, e.scope); err != nil {
					return nil, err
				}
			}
			parts = append(parts, names[entity]...)
		}
		words := conversationFactKeyWords(parts)
		if len(words) == 0 || compilerHasSecret(strings.Join(words, " ")) {
			continue
		}
		keys = append(keys, conversationFactKey{claim: r.claim, span: span, words: words})
	}
	return keys, nil
}

// conversationFactKeyNames returns an Entity's canonical name and active
// aliases readable in scope, or nothing for the owner's and Evie's own
// anchors, whose names would key every Claim about them.
func conversationFactKeyNames(ctx context.Context, q semanticInspectionQueryer, entity memory.SemanticID, scope string) ([]string, error) {
	var name, anchor, entityScope, state string
	err := q.QueryRowContext(ctx, `SELECT e.canonical_name,COALESCE(e.anchor_kind,''),sc.scope_key,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='entity' AND object_id=e.entity_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),e.lifecycle)
 FROM semantic_entities e JOIN semantic_scopes sc ON sc.scope_id=e.scope_id WHERE e.entity_id=?`, entity).Scan(&name, &anchor, &entityScope, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil || anchor == "owner" || anchor == "evie" || state != string(memory.SemanticStateActive) || entityScope != "global" && entityScope != scope {
		return nil, err
	}
	names := []string{name}
	rows, err := q.QueryContext(ctx, `SELECT a.value FROM semantic_aliases a JOIN semantic_scopes sc ON sc.scope_id=a.scope_id
 WHERE a.entity_id=? AND sc.scope_key IN ('global',?)
 AND COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='alias' AND object_id=a.alias_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),a.lifecycle)='active'
 ORDER BY a.alias_id LIMIT ?`, entity, scope, conversationFactKeyAliases)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		names = append(names, alias)
	}
	return names, rows.Err()
}

// conversationFactKeyWords lower-cases and splits the key parts into
// distinct words of two or more characters, or digits, in order.
func conversationFactKeyWords(parts []string) []string {
	var words []string
	seen := map[string]bool{}
	for _, part := range parts {
		if !utf8.ValidString(part) {
			continue
		}
		for _, word := range strings.FieldsFunc(strings.ToLower(part), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if seen[word] || utf8.RuneCountInString(word) < 2 && !unicode.IsDigit([]rune(word)[0]) {
				continue
			}
			seen[word] = true
			words = append(words, word)
		}
	}
	return words
}

// conversationFactKeyText is the indexed keys column of a message: its keys'
// distinct words, bounded.
func conversationFactKeyText(keys []conversationFactKey) string {
	var words []string
	for _, key := range keys {
		words = append(words, key.words...)
	}
	text := strings.Join(conversationFactKeyWords([]string{strings.Join(words, " ")}), " ")
	if len(text) > conversationFactKeyBytes {
		text = text[:strings.LastIndexByte(text[:conversationFactKeyBytes], ' ')+1]
	}
	return strings.TrimSpace(text)
}

// matches reports whether the key shares a word with the query terms.
func (k conversationFactKey) matches(terms map[string]bool) bool {
	for _, word := range k.words {
		if terms[word] {
			return true
		}
	}
	return false
}

// conversationQueryTerms is the set of a query's lower-cased words, as the
// FTS match and excerpt choice see them.
func conversationQueryTerms(query string) map[string]bool {
	terms := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		terms[word] = true
	}
	return terms
}

// chooseFactKeyExcerpt picks the excerpt for a message whose own words did
// not match the query but whose key did: up to conversationExcerptBytes of
// readable text starting shortly before the cited span, as a word match
// would. A key whose span is not readable for this intent is skipped.
func chooseFactKeyExcerpt(content, query string, spans []conversationReadSpan, keys []conversationFactKey) (conversationReadSpan, bool) {
	terms := conversationQueryTerms(query)
	for _, key := range keys {
		if !key.matches(terms) {
			continue
		}
		for _, span := range spans {
			if key.span.start < span.start || key.span.start >= span.end {
				continue
			}
			start := max(span.start, key.span.start-200)
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
	}
	return conversationReadSpan{}, false
}

// factKeyTokens adds, to an excerpt's folded words, the folded words of every
// key whose cited span overlaps it, so the relevance floor sees the saved
// fact the excerpt states.
func factKeyTokens(tokens map[string]bool, span evidenceSpan, keys []conversationFactKey) (map[string]bool, bool) {
	added := false
	for _, key := range keys {
		if key.span.end <= span.start || key.span.start >= span.end {
			continue
		}
		for _, word := range key.words {
			folded := relevanceKey(word)
			if !tokens[folded] {
				tokens[folded] = true
				added = true
			}
		}
	}
	return tokens, added
}
