package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

const conversationExcerptBytes = 800

var ErrConversationAssociation = errors.New("conversation evidence association cannot be resolved safely")

type conversationEvidence struct {
	id             memory.EventID
	session        memory.SessionID
	scope, content string
	sourceType     memory.SemanticSourceType
	actor          memory.SemanticActor
	authority      memory.SourceAuthority
	observed       string
	sequence       int64
}
type evidenceSpan struct{ start, end int }

func loadConversationEvidence(ctx context.Context, q semanticInspectionQueryer, id memory.EventID) (conversationEvidence, bool, error) {
	e := conversationEvidence{id: id}
	var kind, role, workspace, project, state string
	var archived, version int
	err := q.QueryRowContext(ctx, `SELECT e.session_id,e.sequence,e.event_type,COALESCE(e.role,''),e.content,e.recorded_at,e.format_version,
 COALESCE(s.workspace_id,''),COALESCE(s.project_id,''),COALESCE(w.lifecycle_state,'active'),COALESCE(p.archived,0)
 FROM events e JOIN sessions s ON s.id=e.session_id LEFT JOIN workspaces w ON w.id=s.workspace_id LEFT JOIN projects p ON p.id=s.project_id WHERE e.id=? AND s.parent_session_id IS NULL`, id).
		Scan(&e.session, &e.sequence, &kind, &role, &e.content, &e.observed, &version, &workspace, &project, &state, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return e, false, nil
	}
	if err != nil {
		return e, false, err
	}
	e.scope = scopeKeyForContext(memory.ScopeContext{SessionID: e.session, WorkspaceID: memory.WorkspaceID(workspace), ProjectID: memory.ProjectID(project)})
	if version != 1 || state != "active" || archived != 0 || e.content == "" || len(e.content) > 32768 || !utf8.ValidString(e.content) || memory.HasRetrievalSecret([]byte(e.content)) {
		return e, false, nil
	}
	if err = requireSemanticScopeKeysAvailable(ctx, q, []string{e.scope}); err != nil {
		if errors.Is(err, ErrSemanticScopeQuarantined) {
			return e, false, nil
		}
		return e, false, err
	}
	switch {
	case kind == string(memory.EventUserMessage) && role == string(memory.RoleUser):
		e.actor = memory.SemanticActorOwner
		e.authority = memory.AuthorityOwnerStatement
	case kind == string(memory.EventAssistantMessage) && role == string(memory.RoleAssistant):
		e.actor = "assistant"
		e.authority = "none"
	default:
		return e, false, nil
	}
	e.sourceType = memory.SemanticSourceType(kind)
	return e, true, nil
}

// A Source Link is the durable association, including when several Claims
// share it. Suppression is the union of exact locators; retirement never asks
// a model to guess which characters should disappear.
func eligibleConversationSpans(ctx context.Context, q semanticInspectionQueryer, e conversationEvidence) ([]evidenceSpan, error) {
	spans, err := conversationReadSpans(ctx, q, e, memory.RetrievalCurrent, time.Time{})
	if err != nil {
		return nil, err
	}
	result := make([]evidenceSpan, 0, len(spans))
	for _, span := range spans {
		result = append(result, span.evidenceSpan)
	}
	return result, nil
}

func conversationLocatorSpan(e conversationEvidence, locator memory.EvidenceLocator) (evidenceSpan, error) {
	span := evidenceSpan{0, len(e.content)}
	if locator.EventID != e.id || locator.EventPart != memory.EvidenceContent {
		return span, ErrConversationAssociation
	}
	switch locator.LocatorKind {
	case memory.LocatorWhole:
		if locator.LocatorValue != "" {
			return span, ErrConversationAssociation
		}
	case memory.LocatorUTF8ByteRange:
		parts := strings.Split(locator.LocatorValue, ":")
		if len(parts) != 2 {
			return span, ErrConversationAssociation
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil || strconv.Itoa(start) != parts[0] {
			return span, ErrConversationAssociation
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil || strconv.Itoa(end) != parts[1] || start < 0 || end <= start || end > len(e.content) || !utf8.ValidString(e.content[:start]) || !utf8.ValidString(e.content[:end]) {
			return span, ErrConversationAssociation
		}
		span = evidenceSpan{start, end}
	default:
		return span, ErrConversationAssociation
	}
	if memory.CompilerHash([]byte(e.content[span.start:span.end])) != strings.TrimPrefix(locator.EvidenceSHA256, "sha256:") {
		return span, ErrConversationAssociation
	}
	return span, nil
}

func conversationExcerpt(e conversationEvidence, span evidenceSpan, known, valid time.Time) memory.RetrievalEvidence {
	text := e.content[span.start:span.end]
	locator := fmt.Sprintf("%d:%d", span.start, span.end)
	source := memory.SemanticSource{EventID: e.id, SessionID: e.session, ScopeKey: e.scope, EventPart: memory.EvidenceContent,
		LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: locator, EvidenceSHA256: memory.CompilerHash([]byte(text)), Actor: e.actor, SourceType: e.sourceType, Authority: e.authority, ObservedAt: e.observed, Evidence: text, Eligibility: memory.EligibilityEligible}
	return memory.RetrievalEvidence{ID: "excerpt:" + string(e.id) + ":" + locator, Kind: memory.RetrievalConversationExcerpt,
		Intent: memory.RetrievalCurrent, CurrentStatus: memory.SemanticStatusActive, AsKnownAt: known, ValidAt: valid, ScopeKey: e.scope, Status: memory.SemanticStatusActive, Text: text, Sources: []memory.SemanticSource{source}, Paths: []string{"conversation_lexical"}}
}

// Match positions are counted in original UTF-8 bytes, even when Unicode case
// folding changes a character's encoded length. The snippet never crosses an
// excluded interval merely to obtain more neighboring text.
func chooseConversationExcerpt(content, query string, spans []evidenceSpan) (evidenceSpan, bool) {
	terms := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, span := range spans {
		start := -1
		for offset := span.start; offset < span.end; {
			r, size := utf8.DecodeRuneInString(content[offset:span.end])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				offset += size
				continue
			}
			end := offset + size
			for end < span.end {
				r, n := utf8.DecodeRuneInString(content[end:span.end])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
					break
				}
				end += n
			}
			for _, term := range terms {
				if strings.EqualFold(content[offset:end], term) {
					start = offset
					break
				}
			}
			if start >= 0 {
				break
			}
			offset = end
		}
		if start < 0 {
			continue
		}
		start = max(span.start, start-200)
		for start < span.end && !utf8.RuneStart(content[start]) {
			start++
		}
		end := min(span.end, start+conversationExcerptBytes)
		for end > start && !utf8.ValidString(content[start:end]) {
			end--
		}
		return evidenceSpan{start, end}, true
	}
	return evidenceSpan{}, false
}

// conversationPass is one candidate read of conversation search: the
// ordinary read, or the same read restricted to a requested time window.
type conversationPass struct {
	match     string // FTS expression
	text      string // query words, for choosing the excerpt
	relevance *memory.RetrievalRelevance
	// size is the request's content-word count when relevance omits its
	// time words; zero otherwise.
	size   int
	keys   bool // fact keys may match (retrieval_fact_keys.go)
	window *memory.RecallTimeWindow
}

type conversationPassResult struct {
	evidence                []memory.RetrievalEvidence
	candidates              int
	truncated, unclassified bool
}

// conversationMatchExpression restricts an FTS expression to the message's
// own words unless fact keys may match.
func conversationMatchExpression(fts string, keys bool) string {
	if keys {
		return fts
	}
	return "body : (" + fts + ")"
}

// conversationKeysAllowed reports whether fact keys may match. A
// model-directed search may always use them. Automatic Recall uses them only
// for a request to recall something about the owner: a Claim's words describe
// the owner's world, and a general request that shares a Predicate label
// ("Explain how daily medication reminders work") is not asking about it.
func conversationKeysAllowed(query memory.RetrievalQuery) bool {
	return query.Relevance == nil || query.Relevance.PersonalRecall
}

func (s *Store) searchConversations(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery, fts string) (memory.RetrievalResult, error) {
	result := memory.RetrievalResult{Status: memory.RetrievalFailed, Evidence: []memory.RetrievalEvidence{}}
	ctx, cancel := context.WithTimeout(ctx, retrievalDeadline)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	defer tx.Rollback()
	if err = validateSessionScopeIdentity(ctx, tx, scope); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if err = requireSemanticScopeKeysAvailable(ctx, tx, []string{scopeKeyForContext(scope)}); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Coverage, err = conversationIndexCoverage(ctx, tx)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if result.Coverage.State != "active" {
		dense, err := denseIndexCoverage(ctx, tx)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if dense.State != "active" {
			result.Status = memory.RetrievalUnavailable
			return result, nil
		}
	}
	known := s.now().UTC()
	if query.AsKnownAt != nil {
		known = query.AsKnownAt.UTC()
	}
	prepared, err := prepareDenseQuery(ctx, tx, query.Text)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	defer prepared.close()
	lexicalLimit := retrievalCandidateLimit
	if prepared != nil && prepared.coverage.State == "active" {
		// Use the same bounded lexical allocation as accepted-memory plans.
		// Common words cannot spend all authoritative candidate work before an
		// independent dense generator has contributed eligible suggestions.
		lexicalLimit = retrievalLexicalCandidates
	}
	cutoff, err := conversationSessionCutoff(ctx, tx, scope.SessionID, query.Relevance)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	keys := conversationKeysAllowed(query)
	ordinary := conversationPass{match: conversationMatchExpression(fts, keys), text: query.Text, relevance: query.Relevance, keys: keys}
	var pass conversationPassResult
	windowed, unclassified := false, false
	if timed, ok := s.conversationTimePass(query, ordinary); ok {
		// A named past time ("last week") is read first: the request without
		// its time words, among messages recorded inside the window. Only
		// when nothing there qualifies does the ordinary read run, so a time
		// word that named the topic rather than the moment loses nothing.
		result.TimeWindow = &memory.RetrievalTimeWindow{Expression: timed.window.Expression, From: timed.window.From, To: timed.window.To}
		if pass, err = s.readConversationPass(ctx, tx, scope, query, timed, known, cutoff, lexicalLimit); err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		windowed, unclassified = len(pass.evidence) > 0, pass.unclassified
		result.TimeWindow.Matched = windowed
	}
	if !windowed {
		if pass, err = s.readConversationPass(ctx, tx, scope, query, ordinary, known, cutoff, lexicalLimit); err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
	}
	// An excerpt withheld as unclassifiable in either read leaves coverage
	// incomplete.
	unclassified = unclassified || pass.unclassified
	result.Evidence, result.Truncated = pass.evidence, pass.truncated
	denseIncomplete := false
	if !windowed {
		// Dense hits join the ordinary read only; the window applies to
		// lexical excerpts, whose recorded time it compares.
		dense, err := s.denseConversationCandidates(ctx, tx, scope, query, known, retrievalCandidateLimit-pass.candidates, prepared, cutoff)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		result.DenseCoverage = dense.coverage
		denseIncomplete = dense.incomplete
		if dense.cut {
			result.Gaps = append(result.Gaps, memory.RetrievalGapDenseScan)
		}
		var fusedTruncated bool
		result.Evidence, fusedTruncated = fuseConversationEvidence(result.Evidence, dense.evidence, query.Limit)
		result.Truncated = result.Truncated || dense.truncated || fusedTruncated
	}
	if err = tx.Commit(); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Status = memory.RetrievalSuccess
	if len(result.Evidence) == 0 {
		result.Status = memory.RetrievalEmpty
	}
	if result.Coverage.State != "active" || result.Coverage.Pending > 0 || denseIncomplete || unclassified {
		result.Status = memory.RetrievalPartial
	}
	if result.Coverage.State != "active" && len(result.Evidence) == 0 && (result.DenseCoverage == nil || result.DenseCoverage.State != "active") {
		result.Status = memory.RetrievalUnavailable
	}
	for {
		encoded, err := json.Marshal(result)
		if err != nil {
			return result, err
		}
		result.SerializedBytes = len(encoded)
		encoded, _ = json.Marshal(result)
		result.SerializedBytes = len(encoded)
		if len(encoded) <= query.MaxBytes {
			return result, nil
		}
		result.Truncated = true
		if len(result.Evidence) == 0 {
			result.Status = memory.RetrievalExhausted
			return result, nil
		}
		result.Evidence = result.Evidence[:len(result.Evidence)-1]
		if len(result.Evidence) == 0 {
			result.Status = memory.RetrievalExhausted
		}
	}
}

// readConversationPass fetches lexical candidates for one pass and keeps
// those that are eligible now, readable for the intent, matched by the
// message's words or (when allowed) a fact key, and above the relevance floor.
func (s *Store) readConversationPass(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, query memory.RetrievalQuery, pass conversationPass, known time.Time, cutoff int64, limit int) (conversationPassResult, error) {
	var out conversationPassResult
	from, to := "", ""
	if pass.window != nil {
		from, to = formatSemanticTime(pass.window.From), formatSemanticTime(pass.window.To)
	}
	rows, err := tx.QueryContext(ctx, `SELECT f.event_id FROM memory_retrieval_event_fts_v4 f JOIN events e ON e.id=f.event_id
 WHERE memory_retrieval_event_fts_v4 MATCH ? AND f.generation=? AND f.scope_key=?
 AND EXISTS(SELECT 1 FROM memory_retrieval_generations g WHERE g.generation=f.generation AND g.state='active')
 AND `+conversationObservedTimeSQL+`<=?
 AND (?='' OR `+conversationObservedTimeSQL+`>=?) AND (?='' OR `+conversationObservedTimeSQL+`<?)
 AND (e.session_id!=? OR e.sequence<?)
 AND (?=0 OR e.content!=COALESCE((SELECT content FROM events WHERE session_id=? AND event_type='user_message' ORDER BY sequence DESC LIMIT 1),''))
 ORDER BY `+conversationRankSQL+`,e.recorded_at DESC,f.event_id LIMIT ?`, pass.match, conversationIndexGeneration, scopeKeyForContext(scope), formatSemanticTime(known),
		from, from, to, to, scope.SessionID, cutoff, query.ExcludeCurrentRequestCopies, scope.SessionID, limit+1)
	if err != nil {
		return out, err
	}
	var ids []memory.EventID
	for rows.Next() {
		var id memory.EventID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		if len(ids) == limit {
			out.truncated = true
			break
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	rows.Close()
	out.candidates = len(ids)
	var frequency map[string]int
	if pass.relevance != nil && len(ids) > 0 {
		if frequency, err = relevanceFrequencies(ctx, tx, pass.relevance, scopeKeyForContext(scope), known, scope.SessionID, cutoff, pass.keys, pass.window); err != nil {
			return out, err
		}
	}
	for _, id := range ids {
		e, eligible, err := loadConversationEvidence(ctx, tx, id)
		if err != nil {
			return out, err
		}
		if !eligible || e.scope != scopeKeyForContext(scope) {
			continue
		}
		spans, err := conversationReadSpans(ctx, tx, e, query.Intent, known)
		if err != nil {
			return out, err
		}
		// Keys are read from accepted state now, never trusted from the index.
		var keys []conversationFactKey
		keysRead := false
		readKeys := func() error {
			if keysRead || !pass.keys {
				return nil
			}
			keysRead = true
			keys, err = conversationFactKeys(ctx, tx, e, known)
			return err
		}
		span, matched := chooseConversationReadExcerpt(e.content, pass.text, spans)
		byKey := false
		if !matched {
			if err = readKeys(); err != nil {
				return out, err
			}
			span, matched = chooseFactKeyExcerpt(e.content, pass.text, spans, keys)
			byKey = matched
		}
		if !matched {
			continue
		}
		if pass.relevance != nil {
			tokens := relevanceTokens(e.content[span.start:span.end])
			if !relevantTokens(tokens, pass.relevance, frequency, pass.size) {
				if err = readKeys(); err != nil {
					return out, err
				}
				var added bool
				if tokens, added = factKeyTokens(tokens, span.evidenceSpan, keys); !added || !relevantTokens(tokens, pass.relevance, frequency, pass.size) {
					continue
				}
				byKey = true
			}
		}
		if len(out.evidence) >= query.Limit {
			out.truncated = true
			break
		}
		evidence, err := annotatedConversationExcerpt(ctx, tx, scope, e, span, known, known, query.Intent)
		if errors.Is(err, ErrConversationAssociation) {
			// An excerpt that cannot be classified is withheld, never shown as current.
			out.unclassified = true
			continue
		}
		if err != nil {
			return out, err
		}
		if byKey {
			evidence.Paths = append(evidence.Paths, "conversation_fact_key")
		}
		if pass.window != nil {
			evidence.Paths = append(evidence.Paths, "time_window")
		}
		out.truncated = out.truncated || span.start > 0 || span.end < len(e.content)
		out.evidence = append(out.evidence, evidence)
	}
	return out, nil
}

// recallTimeWindow resolves the request's explicit past time: Automatic
// Recall's reading of the request when it supplied one, otherwise the query
// text a model wrote. The window is resolved at the Store's clock in the
// owner's zone.
func (s *Store) recallTimeWindow(query memory.RetrievalQuery) (memory.RecallTimeReference, memory.RecallTimeWindow, bool) {
	var reference memory.RecallTimeReference
	if query.RecallTime != nil {
		reference = *query.RecallTime
	} else {
		reference, _ = memory.FindRecallTimeReference(query.Text)
	}
	window, ok := reference.Window(s.now(), s.location)
	return reference, window, ok
}

// withoutTimeWords returns the words of text that are not the reference's
// time words, in order.
func withoutTimeWords(words []string, reference memory.RecallTimeReference) []string {
	var kept []string
	for _, word := range words {
		if !slices.Contains(reference.Words, strings.ToLower(word)) {
			kept = append(kept, word)
		}
	}
	return kept
}

// conversationTimePass is the ordinary pass restricted to the request's
// past window, without the time words themselves. It reports false when the
// request names no past window, or nothing besides the time.
func (s *Store) conversationTimePass(query memory.RetrievalQuery, ordinary conversationPass) (conversationPass, bool) {
	reference, window, ok := s.recallTimeWindow(query)
	if !ok {
		return conversationPass{}, false
	}
	kept := withoutTimeWords(strings.FieldsFunc(query.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), reference)
	if len(kept) == 0 {
		return conversationPass{}, false
	}
	quoted := make([]string, len(kept))
	for i, word := range kept {
		quoted[i] = `"` + word + `"`
	}
	timed := ordinary
	timed.match = conversationMatchExpression(strings.Join(quoted, " OR "), ordinary.keys)
	timed.text = strings.Join(kept, " ")
	timed.window = &window
	if ordinary.relevance != nil {
		relevance := *ordinary.relevance
		relevance.Current = withoutTimeWords(relevance.Current, reference)
		if len(relevance.Current) == 0 && len(relevance.Context) == 0 {
			return conversationPass{}, false
		}
		folded := map[string]bool{}
		for _, term := range ordinary.relevance.Current {
			folded[relevanceKey(term)] = true
		}
		timed.relevance, timed.size = &relevance, len(folded)
	}
	return timed, true
}

func (s *Store) resolveConversationReference(ctx context.Context, q semanticInspectionQueryer, scope memory.ScopeContext, ref memory.RetrievalReference) (memory.RetrievalEvidence, bool, error) {
	return s.resolveConversationReferenceAt(ctx, q, scope, ref, false)
}

func (s *Store) resolveConversationReferenceAt(ctx context.Context, q semanticInspectionQueryer, scope memory.ScopeContext, ref memory.RetrievalReference, inspection bool) (memory.RetrievalEvidence, bool, error) {
	if ref.Kind != memory.RetrievalConversationExcerpt || ref.ClaimID != "" || ref.ClaimOperationID != "" || len(ref.Sources) != 1 || ref.ScopeKey != scopeKeyForContext(scope) || ref.AsKnownAt.IsZero() || (ref.Intent != "" && ref.Intent != memory.RetrievalCurrent && ref.Intent != memory.RetrievalHistorical) {
		return memory.RetrievalEvidence{}, false, nil
	}
	source := ref.Sources[0]
	if source.SourceLinkID != "" || source.LocatorKind != memory.LocatorUTF8ByteRange {
		return memory.RetrievalEvidence{}, false, nil
	}
	e, eligible, err := loadConversationEvidence(ctx, q, source.EventID)
	if err != nil || !eligible {
		return memory.RetrievalEvidence{}, false, err
	}
	if e.scope != scopeKeyForContext(scope) || e.scope != source.ScopeKey || e.session != source.SessionID || e.authority != source.Authority || e.observed != source.ObservedAt {
		return memory.RetrievalEvidence{}, false, nil
	}
	observed, err := time.Parse(time.RFC3339Nano, e.observed)
	if err != nil || observed.After(ref.AsKnownAt) {
		return memory.RetrievalEvidence{}, false, err
	}
	span, err := conversationLocatorSpan(e, source.EvidenceLocator)
	if err != nil {
		return memory.RetrievalEvidence{}, false, nil
	}
	if span.end-span.start > conversationExcerptBytes {
		return memory.RetrievalEvidence{}, false, nil
	}
	intent := ref.Intent
	if inspection {
		intent = memory.RetrievalHistorical
	}
	spans, err := conversationReadSpans(ctx, q, e, intent, ref.AsKnownAt)
	if err != nil {
		return memory.RetrievalEvidence{}, false, err
	}
	selected := conversationReadSpan{span, memory.SemanticStatusActive, memory.SemanticStatusActive}
	cursor := span.start
	for _, allowed := range spans {
		if allowed.end <= cursor || allowed.start > cursor || allowed.start >= span.end {
			continue
		}
		cursor = min(span.end, allowed.end)
		if allowed.status == memory.SemanticStatusRetired {
			selected.status = memory.SemanticStatusRetired
		}
		if allowed.currentStatus == memory.SemanticStatusRetired {
			selected.currentStatus = memory.SemanticStatusRetired
		}
	}
	if cursor != span.end {
		return memory.RetrievalEvidence{}, false, nil
	}
	// Historical-claim links are recomputed from current accepted state, never
	// copied from the reference. An excerpt that cannot be classified is not
	// eligible to be shown as current.
	resolved, err := annotatedConversationExcerpt(ctx, q, scope, e, selected, ref.AsKnownAt, ref.ValidAt, ref.Intent)
	if errors.Is(err, ErrConversationAssociation) {
		return memory.RetrievalEvidence{}, false, nil
	}
	if err != nil {
		return memory.RetrievalEvidence{}, false, err
	}
	if resolved.ID != ref.ID {
		return memory.RetrievalEvidence{}, false, nil
	}
	resolved.Paths = append([]string(nil), ref.Paths...)
	resolved.RetrievalGeneration = ref.RetrievalGeneration
	resolved.RelatedClaimIDs = append([]memory.SemanticID(nil), ref.RelatedClaimIDs...)
	return resolved, true, nil
}
