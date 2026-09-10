package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
 FROM events e JOIN sessions s ON s.id=e.session_id LEFT JOIN workspaces w ON w.id=s.workspace_id LEFT JOIN projects p ON p.id=s.project_id WHERE e.id=?`, id).
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
	rows, err := q.QueryContext(ctx, `SELECT sl.event_part,sl.locator_kind,sl.locator_value,sl.evidence_sha256
 FROM semantic_source_links sl WHERE sl.event_id=? AND (
 (SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='retired'
 OR (SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=sl.source_link_id ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='retracted')
 ORDER BY sl.source_link_id LIMIT 257`, e.id)
	if err != nil {
		return nil, err
	}
	var suppressed []evidenceSpan
	for rows.Next() {
		var locator memory.EvidenceLocator
		locator.EventID = e.id
		if err = rows.Scan(&locator.EventPart, &locator.LocatorKind, &locator.LocatorValue, &locator.EvidenceSHA256); err != nil {
			rows.Close()
			return nil, err
		}
		if len(suppressed) >= 256 {
			rows.Close()
			return nil, ErrConversationAssociation
		}
		span, err := conversationLocatorSpan(e, locator)
		if err != nil {
			rows.Close()
			return nil, ErrConversationAssociation
		}
		suppressed = append(suppressed, span)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	sort.Slice(suppressed, func(i, j int) bool {
		if suppressed[i].start == suppressed[j].start {
			return suppressed[i].end < suppressed[j].end
		}
		return suppressed[i].start < suppressed[j].start
	})
	var allowed []evidenceSpan
	cursor := 0
	for _, span := range suppressed {
		if span.start > cursor {
			allowed = append(allowed, evidenceSpan{cursor, span.start})
		}
		cursor = max(cursor, span.end)
	}
	if cursor < len(e.content) {
		allowed = append(allowed, evidenceSpan{cursor, len(e.content)})
	}
	return allowed, nil
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
		AsKnownAt: known, ValidAt: valid, ScopeKey: e.scope, Status: memory.SemanticStatusActive, Text: text, Sources: []memory.SemanticSource{source}, Paths: []string{"conversation_lexical"}}
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
		result.Status = memory.RetrievalUnavailable
		return result, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT f.event_id FROM memory_retrieval_event_fts f JOIN events e ON e.id=f.event_id
 WHERE memory_retrieval_event_fts MATCH ? AND f.generation=? AND f.scope_key=?
 AND (e.session_id!=? OR e.sequence<COALESCE((SELECT MAX(sequence) FROM events WHERE session_id=? AND event_type='user_message'),0))
 ORDER BY bm25(memory_retrieval_event_fts),e.recorded_at DESC,f.event_id LIMIT ?`, fts, conversationIndexGeneration, scopeKeyForContext(scope), scope.SessionID, scope.SessionID, retrievalCandidateLimit)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	var ids []memory.EventID
	for rows.Next() {
		var id memory.EventID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return retrievalReadFailure(ctx, result, err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return retrievalReadFailure(ctx, result, err)
	}
	rows.Close()
	known := s.now().UTC()
	for _, id := range ids {
		e, eligible, err := loadConversationEvidence(ctx, tx, id)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if !eligible || e.scope != scopeKeyForContext(scope) {
			continue
		}
		spans, err := eligibleConversationSpans(ctx, tx, e)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		span, matched := chooseConversationExcerpt(e.content, query.Text, spans)
		if !matched {
			continue
		}
		if len(result.Evidence) >= query.Limit {
			result.Truncated = true
			break
		}
		result.Truncated = result.Truncated || span.start > 0 || span.end < len(e.content)
		result.Evidence = append(result.Evidence, conversationExcerpt(e, span, known, known))
	}
	if err = tx.Commit(); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Status = memory.RetrievalSuccess
	if len(result.Evidence) == 0 {
		result.Status = memory.RetrievalEmpty
	}
	if result.Coverage.Pending > 0 {
		result.Status = memory.RetrievalPartial
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

func (s *Store) resolveConversationReference(ctx context.Context, q semanticInspectionQueryer, scope memory.ScopeContext, ref memory.RetrievalReference) (memory.RetrievalEvidence, bool, error) {
	if ref.Kind != memory.RetrievalConversationExcerpt || ref.ClaimID != "" || ref.ClaimOperationID != "" || len(ref.Sources) != 1 || ref.ScopeKey != scopeKeyForContext(scope) {
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
	span, err := conversationLocatorSpan(e, source.EvidenceLocator)
	if err != nil {
		return memory.RetrievalEvidence{}, false, nil
	}
	if span.end-span.start > conversationExcerptBytes {
		return memory.RetrievalEvidence{}, false, nil
	}
	spans, err := eligibleConversationSpans(ctx, q, e)
	if err != nil {
		return memory.RetrievalEvidence{}, false, err
	}
	contained := false
	for _, allowed := range spans {
		contained = contained || span.start >= allowed.start && span.end <= allowed.end
	}
	if !contained {
		return memory.RetrievalEvidence{}, false, nil
	}
	resolved := conversationExcerpt(e, span, ref.AsKnownAt, ref.ValidAt)
	if resolved.ID != ref.ID {
		return memory.RetrievalEvidence{}, false, nil
	}
	resolved.Paths = append([]string(nil), ref.Paths...)
	return resolved, true, nil
}
