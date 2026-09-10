package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

const (
	retrievalCandidateLimit = 64
	retrievalResultLimit    = 8
	retrievalContextLimit   = 24576
	retrievalQueryLimit     = 1024
	retrievalDeadline       = 500 * time.Millisecond
)

var ErrInvalidRetrievalQuery = errors.New("invalid bounded memory query")

func normalizeRetrievalQuery(q memory.RetrievalQuery) (memory.RetrievalQuery, string, error) {
	if q.Kind == "" {
		q.Kind = memory.RetrievalAcceptedMemory
	}
	if q.Kind != memory.RetrievalAcceptedMemory && q.Kind != memory.RetrievalConversationExcerpt {
		return q, "", ErrInvalidRetrievalQuery
	}
	q.Text = strings.TrimSpace(q.Text)
	if q.Text == "" || len(q.Text) > retrievalQueryLimit || !utf8.ValidString(q.Text) {
		return q, "", ErrInvalidRetrievalQuery
	}
	for _, r := range q.Text {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return q, "", ErrInvalidRetrievalQuery
		}
	}
	terms := strings.FieldsFunc(q.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(terms) == 0 || len(terms) > 32 {
		return q, "", ErrInvalidRetrievalQuery
	}
	for i, term := range terms {
		terms[i] = `"` + term + `"`
	}
	if q.Limit == 0 {
		q.Limit = retrievalResultLimit
	}
	if q.MaxBytes == 0 {
		q.MaxBytes = retrievalContextLimit
	}
	if q.Limit < 1 || q.Limit > retrievalResultLimit || q.MaxBytes < 512 || q.MaxBytes > retrievalContextLimit {
		return q, "", ErrInvalidRetrievalQuery
	}
	return q, strings.Join(terms, " OR "), nil
}

// SearchMemory treats FTS and exact/alias matches as suggestions. Every returned
// Claim is re-read from accepted SQLite state in the same read transaction.
func (s *Store) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	result := memory.RetrievalResult{Status: memory.RetrievalFailed, Evidence: []memory.RetrievalEvidence{}}
	query, fts, err := normalizeRetrievalQuery(query)
	if err != nil {
		return result, err
	}
	if query.Kind == memory.RetrievalConversationExcerpt {
		return s.searchConversations(ctx, scope, query, fts)
	}
	ctx, cancel := context.WithTimeout(ctx, retrievalDeadline)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	defer tx.Rollback()
	metadata, err := s.exactReadMetadata(ctx, tx, scope, memory.ClaimQuery{ValidAt: query.ValidAt, AsKnownAt: query.AsKnownAt}, nil, true)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Coverage, err = memoryIndexCoverage(ctx, tx)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if result.Coverage.State != "active" {
		result.Status = memory.RetrievalUnavailable
		return result, nil
	}
	type candidate struct {
		id    memory.SemanticID
		score float64
		paths []string
	}
	candidates := map[memory.SemanticID]*candidate{}
	add := func(id memory.SemanticID, path string, rank int) {
		c := candidates[id]
		if c == nil {
			if len(candidates) >= retrievalCandidateLimit {
				return
			}
			c = &candidate{id: id}
			candidates[id] = c
		}
		c.score += 1 / float64(60+rank)
		if !containsString(c.paths, path) {
			c.paths = append(c.paths, path)
		}
	}
	// Exact identities and accepted aliases contribute independently of FTS.
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT c.claim_id FROM semantic_claims c JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sc.scope_key IN (?,?,?) AND (c.claim_id=? OR c.subject_entity_id=? OR c.object_entity_id=?
 OR c.subject_entity_id IN (SELECT a.entity_id FROM semantic_aliases a WHERE a.normalized_value=?
 AND (SELECT state FROM semantic_state_events se WHERE se.object_kind='alias' AND se.object_id=a.alias_id AND se.transaction_time<=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='active'))
 ORDER BY c.claim_id LIMIT ?`, "global", scopeKeyForContext(scope), "session:"+string(scope.SessionID), query.Text, query.Text, query.Text, normalizeAlias(query.Text), formatSemanticTime(metadata.AsKnownAt), retrievalCandidateLimit)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	rank := 0
	for rows.Next() {
		var id memory.SemanticID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return retrievalReadFailure(ctx, result, err)
		}
		rank++
		add(id, "exact_or_alias", rank)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return retrievalReadFailure(ctx, result, err)
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT claim_id FROM memory_retrieval_fts WHERE memory_retrieval_fts MATCH ?
 AND generation=? AND scope_key IN (?,?,?) AND claim_id NOT IN (SELECT claim_id FROM memory_retrieval_dirty)
 ORDER BY bm25(memory_retrieval_fts),claim_id LIMIT ?`, fts, memoryIndexGeneration, "global", scopeKeyForContext(scope), "session:"+string(scope.SessionID), retrievalCandidateLimit)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	rank = 0
	for rows.Next() {
		var id memory.SemanticID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return retrievalReadFailure(ctx, result, err)
		}
		rank++
		add(id, "lexical", rank)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return retrievalReadFailure(ctx, result, err)
	}
	rows.Close()
	ordered := make([]*candidate, 0, len(candidates))
	for _, c := range candidates {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return ordered[i].id < ordered[j].id
	})
	for _, c := range ordered {
		evidence, eligible, err := s.retrievalClaim(ctx, tx, metadata, c.id)
		if err != nil {
			return retrievalReadFailure(ctx, result, err)
		}
		if !eligible {
			continue
		}
		evidence.Paths = c.paths
		if len(result.Evidence) == query.Limit {
			result.Truncated = true
			break
		}
		result.Evidence = append(result.Evidence, evidence)
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

func retrievalReadFailure(ctx context.Context, result memory.RetrievalResult, err error) (memory.RetrievalResult, error) {
	result.Status = memory.RetrievalFailed
	result.Evidence = nil
	if ctx.Err() != nil {
		result.Status = memory.RetrievalCancelled
		err = ctx.Err()
	}
	return result, err
}

func (s *Store) retrievalClaim(ctx context.Context, q semanticInspectionQueryer, metadata memory.ExactReadMetadata, id memory.SemanticID) (memory.RetrievalEvidence, bool, error) {
	e := memory.RetrievalEvidence{}
	claim, err := loadSemanticClaim(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return e, false, nil
	}
	if err != nil {
		return e, false, err
	}
	if !containsString(metadata.AllowedScopes, claim.ScopeKey) || claim.TransactionTime.After(metadata.AsKnownAt) {
		return e, false, nil
	}
	known := formatSemanticTime(metadata.AsKnownAt)
	state, err := latestStateAt(ctx, q, memory.SemanticObjectClaim, id, known)
	if err != nil {
		return e, false, err
	}
	effective := claim.ValidTime
	correction, found, err := loadVisibleCorrection(ctx, q, id, metadata.AsKnownAt)
	if err != nil {
		return e, false, err
	}
	if found {
		if correction.Mode == memory.CorrectionError {
			return e, false, nil
		}
		effective = correction.OldAfter
	} else if state != memory.SemanticStateActive {
		return e, false, nil
	}
	if !validTimeContains(effective, metadata.ValidAt) {
		return e, false, nil
	}
	sources, err := loadEligibleSourcesAt(ctx, q, id, metadata.AsKnownAt)
	if err != nil {
		return e, false, err
	}
	var eligibleSources []memory.SemanticSource
	for _, source := range sources {
		projected, ok, err := retrievalSource(ctx, q, source, metadata.AllowedScopes)
		if err != nil {
			return e, false, err
		}
		if ok {
			eligibleSources = append(eligibleSources, projected)
		}
		if len(eligibleSources) == 3 {
			break
		}
	}
	if len(eligibleSources) == 0 {
		return e, false, nil
	}
	subject, err := loadSemanticEntityForInspection(ctx, q, claim.SubjectEntityID)
	if err != nil {
		return e, false, err
	}
	object := ""
	if claim.Object.Literal != nil {
		object = claim.Object.Literal.Value
	} else {
		entity, err := loadSemanticEntityForInspection(ctx, q, claim.Object.EntityID)
		if err != nil {
			return e, false, err
		}
		object = entity.CanonicalName
	}
	text := fmt.Sprintf("%s — %s: %s", subject.CanonicalName, claim.Predicate.Label, object)
	if claim.Polarity == memory.PolarityDenied {
		text = "Denied: " + text
	}
	if len(text) > 4096 || compilerHasSecret(text) {
		return e, false, nil
	}
	e = memory.RetrievalEvidence{ID: "claim:" + string(id), Kind: memory.RetrievalAcceptedMemory, ClaimID: id, ClaimOperationID: claim.CreatedOperationID,
		AsKnownAt: metadata.AsKnownAt, ValidAt: metadata.ValidAt, ScopeKey: claim.ScopeKey, Status: memory.SemanticStatusActive, Text: text, Sources: eligibleSources, Paths: []string{}}
	return e, true, nil
}

// retrievalSource resolves the immutable evidence field and validates its
// locator/hash anew. It never substitutes arbitrary payload JSON for evidence.
func retrievalSource(ctx context.Context, q semanticInspectionQueryer, source memory.SemanticSource, allowed []string) (memory.SemanticSource, bool, error) {
	if source.EventPart != memory.EvidenceContent || source.Evidence == "" {
		return source, false, nil
	}
	var session memory.SessionID
	var kind, role, content, workspace, project string
	var workspaceState string
	var projectArchived int
	err := q.QueryRowContext(ctx, `SELECT e.session_id,e.event_type,COALESCE(e.role,''),e.content,COALESCE(s.workspace_id,''),COALESCE(s.project_id,''),COALESCE(w.lifecycle_state,'active'),COALESCE(p.archived,0)
 FROM events e JOIN sessions s ON s.id=e.session_id LEFT JOIN workspaces w ON w.id=s.workspace_id LEFT JOIN projects p ON p.id=s.project_id WHERE e.id=?`, source.EventID).Scan(&session, &kind, &role, &content, &workspace, &project, &workspaceState, &projectArchived)
	if errors.Is(err, sql.ErrNoRows) {
		return source, false, nil
	}
	if err != nil {
		return source, false, err
	}
	contextKey := scopeKeyForContext(memory.ScopeContext{SessionID: session, WorkspaceID: memory.WorkspaceID(workspace), ProjectID: memory.ProjectID(project)})
	if session != source.SessionID || (source.ScopeKey != contextKey && source.ScopeKey != "session:"+string(session)) || workspaceState != "active" || projectArchived != 0 || len(content) > 32768 || compilerHasSecret(content) {
		return source, false, nil
	}
	if err := requireSemanticScopeKeysAvailable(ctx, q, []string{contextKey, source.ScopeKey}); err != nil {
		if errors.Is(err, ErrSemanticScopeQuarantined) {
			return source, false, nil
		}
		return source, false, err
	}
	if kind == "user_message" {
		if role != "user" || source.Actor != memory.SemanticActorOwner || source.Authority != memory.AuthorityOwnerStatement {
			return source, false, nil
		}
	} else if kind != "tool_succeeded" || role != "tool" || source.Authority != memory.AuthorityToolObservation {
		return source, false, nil
	}
	var current string
	if err = q.QueryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, source.ID).Scan(&current); err != nil {
		return source, false, err
	}
	if current != "eligible" {
		return source, false, nil
	}
	if kind == "tool_succeeded" { // Accepted review rendering already validated the allowlisted observation contract.
		if source.Evidence == "" {
			return source, false, nil
		}
	} else {
		offered := acceptedCompilerSource(source)
		offered.Evidence = content
		locator := memory.EvidenceLocator{EventID: source.EventID, EventPart: source.EventPart, LocatorKind: source.LocatorKind, LocatorValue: source.LocatorValue, EvidenceSHA256: strings.TrimPrefix(source.EvidenceSHA256, "sha256:")}
		projected, err := projectCompilerSource(offered, locator)
		if err != nil {
			return source, false, nil
		}
		source.Evidence = projected.Evidence
	}
	if allowed != nil && !containsString(allowed, source.ScopeKey) {
		source.Evidence = ""
	}
	return source, true, nil
}

func (s *Store) RevalidateMemoryEvidence(ctx context.Context, scope memory.ScopeContext, evidence []memory.RetrievalEvidence) ([]memory.RetrievalEvidence, error) {
	if len(evidence) > retrievalResultLimit {
		return nil, ErrInvalidRetrievalQuery
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	metadata, err := s.exactReadMetadata(ctx, tx, scope, memory.ClaimQuery{}, nil, true)
	if err != nil {
		return nil, err
	}
	var valid []memory.RetrievalEvidence
	for _, prior := range evidence {
		if prior.Kind == memory.RetrievalConversationExcerpt {
			current, eligible, err := s.resolveConversationReference(ctx, tx, scope, prior.Reference())
			if err != nil {
				return nil, err
			}
			if eligible {
				valid = append(valid, current)
			}
			continue
		}
		if prior.Kind != memory.RetrievalAcceptedMemory {
			continue
		}
		current, eligible, err := s.retrievalClaim(ctx, tx, metadata, prior.ClaimID)
		if err != nil {
			return nil, err
		}
		if !eligible {
			continue
		}
		if current.ClaimOperationID != prior.ClaimOperationID || current.Text != prior.Text {
			continue
		}
		// Preserve the original read version for identical supplied evidence.
		current.AsKnownAt = prior.AsKnownAt
		current.ValidAt = prior.ValidAt
		current.Paths = append([]string(nil), prior.Paths...)
		if !sameRetrievalSources(current.Reference().Sources, prior.Reference().Sources) {
			continue
		}
		valid = append(valid, current)
	}
	return valid, tx.Commit()
}

func sameRetrievalSources(a, b []memory.RetrievalSourceReference) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
