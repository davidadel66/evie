package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/davidadel66/evie/internal/memory"
)

const reviewBatchSchema = `
CREATE TABLE IF NOT EXISTS memory_review_batch_previews(preview_id TEXT PRIMARY KEY,scope_key TEXT NOT NULL,preview_sha256 TEXT NOT NULL,envelope BLOB NOT NULL CHECK(length(envelope)<=262144));
CREATE TABLE IF NOT EXISTS memory_review_batch_deliveries(owner_id TEXT NOT NULL,delivery_key TEXT NOT NULL,scope_key TEXT NOT NULL,request_hash TEXT NOT NULL,result BLOB NOT NULL,PRIMARY KEY(owner_id,delivery_key));
CREATE TRIGGER IF NOT EXISTS memory_review_batch_previews_no_update BEFORE UPDATE ON memory_review_batch_previews BEGIN SELECT RAISE(ABORT,'batch previews are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_batch_previews_no_delete BEFORE DELETE ON memory_review_batch_previews BEGIN SELECT RAISE(ABORT,'batch previews are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_batch_deliveries_no_update BEFORE UPDATE ON memory_review_batch_deliveries BEGIN SELECT RAISE(ABORT,'batch deliveries are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_batch_deliveries_no_delete BEFORE DELETE ON memory_review_batch_deliveries BEGIN SELECT RAISE(ABORT,'batch deliveries are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_single_delivery_excludes_batch BEFORE INSERT ON memory_review_deliveries WHEN EXISTS(SELECT 1 FROM memory_review_batch_deliveries WHERE owner_id=NEW.owner_id AND delivery_key=NEW.delivery_key) BEGIN SELECT RAISE(ABORT,'idempotency_conflict'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_batch_delivery_excludes_single BEFORE INSERT ON memory_review_batch_deliveries WHEN EXISTS(SELECT 1 FROM memory_review_deliveries WHERE owner_id=NEW.owner_id AND delivery_key=NEW.delivery_key) BEGIN SELECT RAISE(ABORT,'idempotency_conflict'); END;
`
const reviewBatchFailureBehavior = "atomic_groups_independent_failures; committed_failures_are_not_retried"

func ownerReviewBatchHash(p memory.ReviewBatchPreview) (string, []byte, error) {
	p.SHA256 = ""
	return semanticHash(struct {
		Domain  string                    `json:"domain"`
		Preview memory.ReviewBatchPreview `json:"preview"`
	}{"evie-owner-review-batch-v1", p})
}
func completeReviewBatchBytes(p memory.ReviewBatchPreview) []byte {
	return compilerJSON(struct {
		Domain  string                    `json:"domain"`
		Preview memory.ReviewBatchPreview `json:"preview"`
	}{"evie-owner-review-batch-v1", p})
}
func validateReviewBatchBounds(groups, refs, records, bytes int) error {
	if groups > 20 || refs > 64 || records > 256 || bytes > 256*1024 {
		return ErrReviewTooLarge
	}
	return nil
}
func validateReviewBatchRequest(r memory.ReviewBatchRequest) error {
	refs := 0
	groups := map[string]bool{}
	seen := map[string]bool{}
	if len(r.Groups) == 0 {
		return ErrReviewDependencies
	}
	for _, g := range r.Groups {
		if len(g.ID) < 1 || len(g.ID) > 64 || !reviewBatchLabel(g.ID) || groups[g.ID] || g.Action != "accept" && g.Action != "reject" || len(g.Candidates) == 0 {
			return ErrReviewDependencies
		}
		groups[g.ID] = true
		refs += len(g.Candidates)
		for _, c := range g.Candidates {
			if seen[c.ID] {
				return ErrReviewDependencies
			}
			seen[c.ID] = true
		}
		if len(g.Dependencies) > 3*len(g.Candidates) {
			return ErrReviewDependencies
		}
		if g.Action == "reject" && len(g.Dependencies) != 0 {
			return ErrReviewDependencies
		}
	}
	return validateReviewBatchBounds(len(r.Groups), refs, 0, 0)
}
func reviewBatchLabel(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func (s *Store) PrepareOwnerCandidateBatch(ctx context.Context, a OwnerReviewContext, r memory.ReviewBatchRequest) (memory.ReviewBatchPreview, error) {
	var out memory.ReviewBatchPreview
	if err := validateReviewBatchRequest(r); err != nil {
		return out, err
	}
	// Copy the caller's slices before holding the write transaction.
	if err := json.Unmarshal(compilerJSON(r), &r); err != nil {
		return out, err
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		id, err := newSemanticID()
		if err != nil {
			return err
		}
		out = memory.ReviewBatchPreview{Version: "owner-review-batch-v1", ID: string(id), OwnerID: memory.LocalOwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, ScopeKey: a.scope, FailureBehavior: reviewBatchFailureBehavior, Groups: []memory.ReviewBatchGroup{}, PriorRevisions: []memory.ScopeRevision{}}
		if err = conn.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&out.SourcePolicy); err != nil {
			return err
		}
		keys, err := reviewScopeKeys(ctx, conn, a.scope)
		if err != nil {
			return err
		}
		for _, key := range keys {
			scope, err := loadSemanticScope(ctx, conn, key)
			if err != nil {
				return err
			}
			out.PriorRevisions = append(out.PriorRevisions, memory.ScopeRevision{ScopeKey: key, Revision: scope.Revision})
		}
		for _, g := range r.Groups {
			candidates := []memory.OwnerCandidate{}
			for _, ref := range g.Candidates {
				item, err := loadReviewCandidate(ctx, conn, a, ref.ID, g.Action == "accept")
				if err != nil {
					return err
				}
				if item.Candidate.ReviewState != "unresolved" {
					return ErrReviewResolved
				}
				if item.Ref != ref || item.Candidate.EquivalentTo != "" {
					return ErrReviewStale
				}
				candidates = append(candidates, item)
			}
			pid, err := newSemanticID()
			if err != nil {
				return err
			}
			p := memory.ReviewPreview{Version: "owner-review-preview-v5", ID: string(pid), BatchID: out.ID, OwnerID: out.OwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, ScopeKey: a.scope, JobID: candidates[0].JobID, GenerationID: candidates[0].GenerationID, Action: g.Action, Candidates: candidates, SourcePolicy: out.SourcePolicy, Dependencies: canonicalReviewDependencies(g.Dependencies)}
			if g.Action == "accept" {
				p.Effect, err = prepareReviewCompound(ctx, conn, a, candidates, p.Dependencies)
				if err != nil {
					return err
				}
			}
			p.EffectSHA256, _, err = ownerReviewEffectHash(p.Effect)
			if err != nil {
				return err
			}
			p.SHA256, _, err = ownerReviewPreviewHash(p)
			if err != nil {
				return err
			}
			if err = validateOwnerReviewEncoding(p); err != nil {
				return err
			}
			out.Groups = append(out.Groups, memory.ReviewBatchGroup{ID: g.ID, Preview: p})
		}
		if err = validateReviewGroupIndependence(out.Groups); err != nil {
			return err
		}
		out.SHA256, _, err = ownerReviewBatchHash(out)
		if err != nil {
			return err
		}
		if err = validateOwnerReviewBatch(out); err != nil {
			return err
		}
		for _, group := range out.Groups {
			p := group.Preview
			if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_previews VALUES(?,?,?,?)`, p.ID, p.ScopeKey, p.SHA256, compilerJSON(p)); err != nil {
				return err
			}
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO memory_review_batch_previews VALUES(?,?,?,?)`, out.ID, a.scope, out.SHA256, compilerJSON(out))
		return err
	})
	if err != nil {
		return memory.ReviewBatchPreview{}, err
	}
	return out, nil
}
func validateOwnerReviewBatch(p memory.ReviewBatchPreview) error {
	if p.Version != "owner-review-batch-v1" || p.FailureBehavior != reviewBatchFailureBehavior || validateSemanticUUID(p.ID) != nil || p.OwnerID != memory.LocalOwnerID || p.AuthorizationRevision < 1 {
		return ErrReviewDependencies
	}
	r := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{}}
	records, refs := 0, 0
	for _, g := range p.Groups {
		member := g.Preview
		if member.BatchID != p.ID || member.Version != "owner-review-preview-v5" || member.OwnerID != p.OwnerID || member.AuthenticationBinding != p.AuthenticationBinding || member.AuthorizationRevision != p.AuthorizationRevision || member.ScopeKey != p.ScopeKey || member.SourcePolicy != p.SourcePolicy {
			return ErrReviewDependencies
		}
		if err := validateOwnerReviewEncoding(member); err != nil {
			return err
		}
		selected := []memory.CandidateRef{}
		for _, c := range member.Candidates {
			selected = append(selected, c.Ref)
		}
		r.Groups = append(r.Groups, memory.ReviewBatchGroupRequest{ID: g.ID, Action: member.Action, Candidates: selected, Dependencies: member.Dependencies})
		if member.Effect != nil && string(compilerJSON(member.Effect.PriorRevisions)) != string(compilerJSON(p.PriorRevisions)) {
			return ErrReviewDependencies
		}
		refs += len(selected)
		records += countReviewRecords(member.Effect)
	}
	if err := validateReviewBatchRequest(r); err != nil {
		return err
	}
	if err := validateReviewGroupIndependence(p.Groups); err != nil {
		return err
	}
	if err := validateReviewBatchBounds(len(p.Groups), refs, records, len(completeReviewBatchBytes(p))); err != nil {
		return err
	}
	hash, _, err := ownerReviewBatchHash(p)
	if err != nil {
		return err
	}
	if hash != p.SHA256 {
		return ErrReviewStale
	}
	return nil
}
func (s *Store) InspectOwnerCandidateBatch(ctx context.Context, a OwnerReviewContext, id string) (memory.ReviewBatchPreview, error) {
	var p memory.ReviewBatchPreview
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return p, err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT envelope FROM memory_review_batch_previews WHERE preview_id=? AND scope_key=?`, id, a.scope).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrOwnerReviewUnauthorized
		}
		return p, err
	}
	if json.Unmarshal(raw, &p) != nil {
		return memory.ReviewBatchPreview{}, ErrReviewStale
	}
	if err = validateOwnerReviewBatch(p); err != nil {
		return memory.ReviewBatchPreview{}, err
	}
	for _, g := range p.Groups {
		if err = reviewOperationSourcesVisible(ctx, tx, memory.OwnerReviewOperation{Preview: g.Preview}); err != nil {
			return memory.ReviewBatchPreview{}, err
		}
	}
	return p, tx.Commit()
}
func (s *Store) ResolveOwnerCandidateBatch(ctx context.Context, a OwnerReviewContext, d memory.ReviewBatchDecision) (memory.ReviewBatchResult, error) {
	var out memory.ReviewBatchResult
	if !reviewDeliveryValid(d.DeliveryKey) || !reviewReasonValid(d.Reason) {
		return out, ErrReviewInvalidRequest
	}
	if err := json.Unmarshal(compilerJSON(d), &d); err != nil {
		return out, err
	}
	requestHash := memory.CompilerHash(compilerJSON(d))
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		var hash, scope string
		var raw []byte
		err := conn.QueryRowContext(ctx, `SELECT scope_key,request_hash,result FROM memory_review_batch_deliveries WHERE owner_id=? AND delivery_key=?`, memory.LocalOwnerID, d.DeliveryKey).Scan(&scope, &hash, &raw)
		if err == nil {
			if scope != a.scope {
				return ErrOwnerReviewUnauthorized
			}
			if hash != requestHash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal(raw, &out)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var exists int
		if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM memory_review_deliveries WHERE owner_id=? AND delivery_key=?`, memory.LocalOwnerID, d.DeliveryKey).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			return ErrIdempotencyConflict
		}
		var p memory.ReviewBatchPreview
		if err = conn.QueryRowContext(ctx, `SELECT envelope FROM memory_review_batch_previews WHERE preview_id=? AND scope_key=?`, d.PreviewID, a.scope).Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOwnerReviewUnauthorized
			}
			return err
		}
		if json.Unmarshal(raw, &p) != nil || p.SHA256 != d.PreviewSHA256 {
			return ErrReviewStale
		}
		if err = validateOwnerReviewBatch(p); err != nil {
			return err
		}
		if p.AuthenticationBinding != a.binding || p.AuthorizationRevision != a.revision {
			return ErrReviewStale
		}
		if len(d.Actions) != len(p.Groups) {
			return ErrReviewDependencies
		}
		for i, action := range d.Actions {
			if action.GroupID != p.Groups[i].ID || action.Action != p.Groups[i].Preview.Action {
				return ErrReviewDependencies
			}
		}
		var policy string
		if err = conn.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&policy); err != nil {
			return err
		}
		if policy != p.SourcePolicy {
			return ErrReviewStale
		}
		for _, r := range p.PriorRevisions {
			scope, err := loadSemanticScope(ctx, conn, r.ScopeKey)
			if err != nil {
				return err
			}
			if scope.Revision != r.Revision {
				return ErrReviewStale
			}
		}
		// Immutable event-byte drift invalidates the starting preview globally.
		// Group-local eligibility failures are recorded below without exposing text.
		for _, g := range p.Groups {
			if g.Preview.Action != "accept" {
				continue
			}
			for _, candidate := range g.Preview.Candidates {
				for _, sources := range reviewCandidateDisclosureSources(candidate) {
					for _, source := range sources {
						var text string
						err := conn.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(content AS BLOB))<=32768 THEN content ELSE '' END FROM events WHERE id=?`, source.Locator.EventID).Scan(&text)
						if err == nil {
							offered := source
							offered.Evidence = text
							if _, err = projectHistoricalCompilerSource(offered, source.Locator); err != nil {
								return ErrReviewStale
							}
						} else if !errors.Is(err, sql.ErrNoRows) {
							return err
						}
					}
				}
			}
		}
		out = memory.ReviewBatchResult{DeliveryKey: d.DeliveryKey, PreviewID: p.ID, Groups: []memory.ReviewBatchGroupResult{}}
		prior := []memory.OwnerReviewOperationResult{}
		for i, g := range p.Groups {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if _, err = conn.ExecContext(ctx, `SAVEPOINT owner_review_group`); err != nil {
				return err
			}
			result, groupErr := s.resolveReviewBatchGroup(ctx, conn, a, p, g, d, i, prior)
			if groupErr != nil {

				if ctx.Err() != nil {
					return ctx.Err()
				}
				if _, err = conn.ExecContext(ctx, `ROLLBACK TO owner_review_group`); err != nil {
					return err
				}
				if _, err = conn.ExecContext(ctx, `RELEASE owner_review_group`); err != nil {
					return err
				}
				if reviewBatchFatal(groupErr) {
					return groupErr
				}
				failed := memory.ReviewBatchGroupResult{GroupID: g.ID, Outcome: "failed", FailureCode: reviewBatchFailureCode(groupErr)}
				var terminal reviewBatchTerminalError
				if errors.As(groupErr, &terminal) {
					failed.PriorResolutions = terminal.resolutions
				}
				out.Groups = append(out.Groups, failed)
			} else {
				if _, err = conn.ExecContext(ctx, `RELEASE owner_review_group`); err != nil {
					return err
				}
				out.Groups = append(out.Groups, memory.ReviewBatchGroupResult{GroupID: g.ID, Outcome: g.Preview.Action + "ed", Result: &result})
				if result.Operation != nil {
					prior = append(prior, *result.Operation)
				}
			}
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_batch_deliveries VALUES(?,?,?,?,?)`, memory.LocalOwnerID, d.DeliveryKey, a.scope, requestHash, compilerJSON(out)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return memory.ReviewBatchResult{}, err
	}
	return out, nil
}
func (s *Store) resolveReviewBatchGroup(ctx context.Context, conn *sql.Conn, a OwnerReviewContext, b memory.ReviewBatchPreview, g memory.ReviewBatchGroup, d memory.ReviewBatchDecision, index int, prior []memory.OwnerReviewOperationResult) (memory.ReviewResult, error) {
	var result memory.ReviewResult
	p := g.Preview
	// A winning resolution is durable metadata. Check every selected identity
	// and destination before source/effect eligibility can hide that result.
	winners := []memory.ReviewResult{}
	seenWinners := map[string]bool{}
	for _, selected := range p.Candidates {
		state, err := reviewCandidateState(ctx, conn, a, selected.Ref.ID, selected.JobID)
		if err != nil {
			return result, err
		}
		if state == "unresolved" {
			continue
		}
		var raw []byte
		if err := conn.QueryRowContext(ctx, `SELECT d.result FROM memory_review_resolutions r JOIN memory_review_deliveries d ON d.owner_id=r.owner_id AND d.delivery_key=r.delivery_key WHERE r.candidate_id=? AND r.owner_id=? AND d.scope_key=?`, selected.Ref.ID, memory.LocalOwnerID, a.scope).Scan(&raw); err != nil {
			return result, err
		}
		var winner memory.ReviewResult
		if err := json.Unmarshal(raw, &winner); err != nil {
			return result, err
		}
		matches := false
		for _, ref := range winner.Candidates {
			matches = matches || ref.ID == selected.Ref.ID
		}
		if !matches || !reviewDeliveryValid(winner.DeliveryKey) || validateSemanticUUID(winner.PreviewID) != nil || validateSemanticUUID(winner.AuditID) != nil || winner.Action+"ed" != state || (winner.Operation == nil) != (winner.Action == "reject") {
			return result, errors.New("invalid recorded terminal resolution")
		}
		if !seenWinners[winner.DeliveryKey] {
			winners = append(winners, winner)
			seenWinners[winner.DeliveryKey] = true
		}
	}
	if len(winners) != 0 {
		return result, reviewBatchTerminalError{winners}
	}
	current := []memory.OwnerCandidate{}
	for _, selected := range p.Candidates {
		item, err := loadReviewCandidate(ctx, conn, a, selected.Ref.ID, p.Action == "accept")
		if err != nil {
			return result, err
		}
		if item.Candidate.ReviewState != "unresolved" {
			return result, ErrReviewResolved
		}
		if item.Ref != selected.Ref || item.Candidate.EquivalentTo != "" || string(compilerJSON(item)) != string(compilerJSON(selected)) {
			return result, ErrReviewStale
		}
		current = append(current, item)
	}
	auditID, err := newSemanticID()
	if err != nil {
		return result, err
	}
	// One stable internal delivery per group gives existing resolution inspection
	// its native receipt. The explicit outer delivery remains the retry authority.
	groupKey := "idem:v1:" + p.ID
	result = memory.ReviewResult{DeliveryKey: groupKey, PreviewID: p.ID, Action: p.Action, AuditID: string(auditID), Candidates: []memory.CandidateRef{}}
	if p.Action == "accept" {
		source := current[0].Candidate.Support[0]
		op := memory.OwnerReviewOperation{SchemaVersion: 6, Kind: "owner_candidate_review", OperationID: p.Effect.OperationID, IdempotencyKey: groupKey, Actor: memory.SemanticActorOwner, SessionID: source.SessionID, SourceEventID: source.Locator.EventID, Preview: p, AuditID: string(auditID), Batch: &memory.ReviewBatchCommit{PreviewID: b.ID, PreviewSHA256: b.SHA256, GroupID: g.ID, GroupIndex: index, PriorGroups: append([]memory.OwnerReviewOperationResult{}, prior...)}}
		accepted, err := s.applyOwnerReviewOperation(ctx, conn, op, s.now())
		if err != nil {
			return result, err
		}
		result.Operation = &accepted
	}
	state := "accepted"
	if p.Action == "reject" {
		state = "rejected"
	}
	for _, item := range current {
		updated, err := conn.ExecContext(ctx, `UPDATE memory_compiler_candidates SET review_state=?,review_revision=review_revision+1 WHERE candidate_id=? AND review_state='unresolved' AND review_revision=?`, state, item.Ref.ID, item.Ref.ReviewRevision)
		if err != nil {
			return result, reviewBatchPersistenceError{err}
		}
		n, err := updated.RowsAffected()
		if err != nil || n != 1 {
			return result, reviewBatchPersistenceError{ErrReviewStale}
		}
		ref := item.Ref
		ref.ReviewRevision++
		result.Candidates = append(result.Candidates, ref)
	}
	decision := memory.ReviewDecision{DeliveryKey: groupKey, PreviewID: p.ID, PreviewSHA256: p.SHA256, Action: p.Action, Reason: d.Reason}
	audit := struct {
		Preview          memory.ReviewPreview  `json:"preview"`
		Decision         memory.ReviewDecision `json:"decision"`
		Result           memory.ReviewResult   `json:"result"`
		BatchPreviewID   string                `json:"batch_preview_id"`
		BatchDeliveryKey string                `json:"batch_delivery_key"`
	}{p, decision, result, b.ID, d.DeliveryKey}
	if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_audits VALUES(?,?,?,?,?)`, auditID, a.scope, p.ID, p.Action, compilerJSON(audit)); err != nil {
		return result, reviewBatchPersistenceError{err}
	}
	if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_deliveries VALUES(?,?,?,?,?)`, memory.LocalOwnerID, groupKey, a.scope, memory.CompilerHash(compilerJSON(decision)), compilerJSON(result)); err != nil {
		return result, reviewBatchPersistenceError{err}
	}
	for _, item := range current {
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_resolutions VALUES(?,?,?,?)`, item.Ref.ID, memory.LocalOwnerID, groupKey, auditID); err != nil {
			return result, reviewBatchPersistenceError{err}
		}
	}
	return result, nil
}
func reviewBatchFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrReviewResolved):
		return "already_resolved"
	case errors.Is(err, ErrReviewStale):
		return "stale_preview"
	case errors.Is(err, ErrReviewInvalidSource):
		return "source_ineligible"
	case errors.Is(err, ErrOwnerReviewUnauthorized):
		return "source_ineligible"
	case errors.Is(err, ErrReviewDependencies):
		return "invalid_dependencies"
	default:
		return "semantic_validation_failed"
	}
}

// Only recorded resolution metadata is returned; it carries no source quotes
// and never claims that other members of the failed group committed.
type reviewBatchTerminalError struct{ resolutions []memory.ReviewResult }

func (reviewBatchTerminalError) Error() string { return ErrReviewResolved.Error() }
func (reviewBatchTerminalError) Unwrap() error { return ErrReviewResolved }

type reviewBatchPersistenceError struct{ error }

func (e reviewBatchPersistenceError) Unwrap() error { return e.error }
func reviewBatchFatal(err error) bool {
	var persistence reviewBatchPersistenceError
	if errors.As(err, &persistence) {
		return true
	}
	// A failed semantic constraint is local to its savepoint; transport, lock,
	// cancellation, corruption and all other coded SQLite errors abort delivery.
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		return coded.Code()&255 != 19
	}
	if !compilerDataFailure(err) {
		return true
	}
	switch {
	case errors.Is(err, ErrReviewStale), errors.Is(err, ErrReviewResolved), errors.Is(err, ErrReviewInvalidSource), errors.Is(err, ErrOwnerReviewUnauthorized), errors.Is(err, ErrReviewDependencies):
		return false
	default:
		return true
	}
}
