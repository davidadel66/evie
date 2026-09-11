package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	if q.Intent == "" {
		q.Intent = memory.RetrievalCurrent
	}
	if q.Intent != memory.RetrievalCurrent && q.Intent != memory.RetrievalHistorical {
		return q, "", ErrInvalidRetrievalQuery
	}
	if q.Kind == "" {
		q.Kind = memory.RetrievalAcceptedMemory
	}
	if q.Kind != memory.RetrievalAcceptedMemory && q.Kind != memory.RetrievalConversationExcerpt {
		return q, "", ErrInvalidRetrievalQuery
	}
	if q.Kind == memory.RetrievalConversationExcerpt && q.ValidAt != nil || q.ValidAt != nil && q.ValidAt.IsZero() || q.AsKnownAt != nil && q.AsKnownAt.IsZero() {
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
	q, err := normalizeRetrievalBounds(q)
	return q, strings.Join(terms, " OR "), err
}

func normalizeRetrievalBounds(q memory.RetrievalQuery) (memory.RetrievalQuery, error) {
	if q.Limit == 0 {
		q.Limit = retrievalResultLimit
	}
	if q.MaxBytes == 0 {
		q.MaxBytes = retrievalContextLimit
	}
	if q.Limit < 1 || q.Limit > retrievalResultLimit || q.MaxBytes < 512 || q.MaxBytes > retrievalContextLimit {
		return q, ErrInvalidRetrievalQuery
	}
	return q, nil
}

// SearchMemory treats FTS and exact/alias matches as suggestions. Every returned
// Claim is re-read from accepted SQLite state in the same read transaction.
func (s *Store) SearchMemory(ctx context.Context, scope memory.ScopeContext, query memory.RetrievalQuery) (memory.RetrievalResult, error) {
	if query.Kind == memory.RetrievalConversationExpansion {
		return s.expandConversation(ctx, scope, query)
	}
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
	candidates, err := s.acceptedRetrievalCandidates(ctx, tx, scope, query, metadata, fts)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	for _, candidate := range candidates.ordered() {
		if len(result.Evidence) == query.Limit {
			result.Truncated = true
			break
		}
		result.Evidence = append(result.Evidence, candidate.evidence)
	}
	result.Truncated = result.Truncated || candidates.truncated
	seen := candidates.seen
	incomplete, err := s.supplementAcceptedRetrieval(ctx, tx, scope, query, metadata, &result, seen)
	if err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	if err = tx.Commit(); err != nil {
		return retrievalReadFailure(ctx, result, err)
	}
	result.Status = memory.RetrievalSuccess
	if len(result.Evidence) == 0 {
		result.Status = memory.RetrievalEmpty
	}
	if result.Coverage.Pending > 0 || incomplete {
		result.Status = memory.RetrievalPartial
	}
	for {
		result.Evidence = pruneRetrievalGraphPaths(result.Evidence)
		decorateRetrievalRelations(result.Evidence)
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

func (s *Store) retrievalClaim(ctx context.Context, q semanticInspectionQueryer, metadata memory.ExactReadMetadata, id memory.SemanticID, intent string, validAtConstrained bool) (memory.RetrievalEvidence, bool, error) {
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
	var currentState memory.SemanticStateValue
	if err = q.QueryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, id).Scan(&currentState); err != nil {
		return e, false, err
	}
	if intent == "" {
		intent = memory.RetrievalCurrent
	}
	historical := intent == memory.RetrievalHistorical
	if !historical && currentState == memory.SemanticStateRetired {
		return e, false, nil
	}
	effective := claim.ValidTime
	correction, found, err := loadVisibleCorrection(ctx, q, id, metadata.AsKnownAt)
	if err != nil {
		return e, false, err
	}
	if found {
		if correction.Mode == memory.CorrectionError && !historical {
			return e, false, nil
		}
		if correction.Mode == memory.CorrectionChanged {
			effective = correction.OldAfter
		}
	} else if state != memory.SemanticStateActive && !historical {
		return e, false, nil
	}
	if (!historical || validAtConstrained) && !validTimeContains(effective, metadata.ValidAt) {
		return e, false, nil
	}
	var currentCorrection memory.CorrectionMode
	err = q.QueryRowContext(ctx, `SELECT mode FROM semantic_claim_corrections WHERE old_claim_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, id).Scan(&currentCorrection)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return e, false, err
	}
	if !historical && currentCorrection == memory.CorrectionError {
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
		AsKnownAt: metadata.AsKnownAt, ValidAt: metadata.ValidAt, ScopeKey: claim.ScopeKey, Status: semanticStatus(state), Text: text, Sources: eligibleSources, Paths: []string{},
		Intent: intent, ValidAtConstrained: validAtConstrained, CurrentStatus: semanticStatus(currentState), Claim: &claim, EffectiveValidTime: &effective,
		CorrectionMode: correction.Mode, CurrentCorrectionMode: currentCorrection}
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
		if prior.ValidAt.IsZero() || prior.AsKnownAt.IsZero() {
			continue
		}
		if prior.Intent != memory.RetrievalHistorical {
			currentMetadata := metadata
			if prior.ValidAtConstrained {
				currentMetadata.ValidAt = prior.ValidAt
			}
			_, eligible, err := s.retrievalClaim(ctx, tx, currentMetadata, prior.ClaimID, memory.RetrievalCurrent, prior.ValidAtConstrained)
			if err != nil {
				return nil, err
			}
			if !eligible {
				continue
			}
		}
		// Eligibility above is current; the delivered proposition and temporal
		// metadata below describe the original read. Never relabel a newer
		// correction with an older as_known_at timestamp.
		readMetadata, err := s.exactReadMetadata(ctx, tx, scope, memory.ClaimQuery{ValidAt: &prior.ValidAt, AsKnownAt: &prior.AsKnownAt}, nil, true)
		if err != nil {
			return nil, err
		}
		current, eligible, err := s.retrievalClaim(ctx, tx, readMetadata, prior.ClaimID, prior.Intent, prior.ValidAtConstrained)
		if err != nil {
			return nil, err
		}
		if !eligible {
			continue
		}
		if current.ClaimOperationID != prior.ClaimOperationID || current.Text != prior.Text {
			continue
		}
		current.IdentityMatches, err = resolveRetrievalIdentityMatches(ctx, tx, readMetadata, current, prior.Reference().IdentityMatches)
		if err != nil {
			return nil, err
		}
		current.Paths = append([]string(nil), prior.Paths...)
		current.GraphPaths = prior.Reference().GraphPaths
		if !sameRetrievalSources(current.Reference().Sources, prior.Reference().Sources) {
			continue
		}
		valid = append(valid, current)
	}
	valid = pruneRetrievalGraphPaths(valid)
	decorateRetrievalRelations(valid)
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
