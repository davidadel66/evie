package eviedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// This is the source-policy identity accepted by the immutable v6 envelope,
// independent of whichever policy a future compiler uses for new extraction.
const ownerReviewSourcePolicyV1 = "owner-assertions-v1"

// reviewWriter exposes only package-private accepted-effect helpers, within an
// already authorized serialized owner review transaction. It is not a lease.
type reviewWriter struct{ conn *sql.Conn }

func (w reviewWriter) execContext(ctx context.Context, s string, args ...any) (sql.Result, error) {
	return w.conn.ExecContext(ctx, s, args...)
}
func (w reviewWriter) queryContext(ctx context.Context, s string, args ...any) (*sql.Rows, error) {
	return w.conn.QueryContext(ctx, s, args...)
}
func (w reviewWriter) queryRowContext(ctx context.Context, s string, args ...any) rowScanner {
	return w.conn.QueryRowContext(ctx, s, args...)
}

func (s *Store) ResolveOwnerCandidateReview(ctx context.Context, a OwnerReviewContext, decision memory.ReviewDecision) (memory.ReviewResult, error) {
	var result memory.ReviewResult
	var priorResolution memory.ReviewResult
	if !reviewDeliveryValid(decision.DeliveryKey) || len(decision.Reason) > 4096 || !utf8.ValidString(decision.Reason) || compilerHasSecret(decision.Reason) {
		return result, errors.New("invalid review delivery or reason")
	}
	requestHash, _, err := semanticHash(decision)
	if err != nil {
		return result, err
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		var storedHash, scope string
		var storedResult []byte
		err := conn.QueryRowContext(ctx, `SELECT request_hash,scope_key,result FROM memory_review_deliveries WHERE owner_id=? AND delivery_key=?`, memory.LocalOwnerID, decision.DeliveryKey).Scan(&storedHash, &scope, &storedResult)
		if err == nil {
			if scope != a.scope {
				return ErrOwnerReviewUnauthorized
			}
			if storedHash != requestHash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal(storedResult, &result)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var raw []byte
		if err = conn.QueryRowContext(ctx, `SELECT envelope FROM memory_review_previews WHERE preview_id=? AND scope_key=?`, decision.PreviewID, a.scope).Scan(&raw); err != nil {
			return ErrOwnerReviewUnauthorized
		}
		var p memory.ReviewPreview
		if json.Unmarshal(raw, &p) != nil {
			return errors.New("invalid stored preview")
		}
		if err = validateOwnerReviewEncoding(p); err != nil {
			return err
		}
		if p.SHA256 != decision.PreviewSHA256 || p.Action != decision.Action {
			return ErrReviewStale
		}
		// Current owner authority and preview integrity still apply to a
		// terminal lookup. Source and preview freshness govern new acceptance;
		// they cannot hide an already recorded winning resolution.
		for _, selected := range p.Candidates {
			var state string
			if err := conn.QueryRowContext(ctx, `SELECT c.review_state FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id WHERE c.candidate_id=? AND j.job_id=? AND j.destination=?`, selected.Ref.ID, p.JobID, a.scope).Scan(&state); err != nil {
				return err
			}
			if state != "unresolved" {
				var prior []byte
				if err := conn.QueryRowContext(ctx, `SELECT d.result FROM memory_review_resolutions r JOIN memory_review_deliveries d ON d.owner_id=r.owner_id AND d.delivery_key=r.delivery_key WHERE r.candidate_id=? AND d.scope_key=?`, selected.Ref.ID, a.scope).Scan(&prior); err != nil {
					return err
				}
				if err := json.Unmarshal(prior, &priorResolution); err != nil {
					return err
				}
				return ErrReviewResolved
			}
		}
		if p.AuthenticationBinding != a.binding || p.AuthorizationRevision != a.revision {
			return ErrReviewStale
		}
		var policy string
		if err = conn.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&policy); err != nil {
			return err
		}
		if policy != p.SourcePolicy {
			return ErrReviewStale
		}
		current := make([]memory.OwnerCandidate, 0, len(p.Candidates))
		for _, selected := range p.Candidates {
			item, err := loadReviewCandidate(ctx, conn, a, selected.Ref.ID, p.Action == "accept")
			if err != nil {
				return err
			}
			if item.Candidate.EquivalentTo != "" {
				return ErrReviewStale
			}
			current = append(current, item)
		}
		if string(compilerJSON(current)) != string(compilerJSON(p.Candidates)) {
			return ErrReviewStale
		}
		auditID, err := newSemanticID()
		if err != nil {
			return err
		}
		result = memory.ReviewResult{DeliveryKey: decision.DeliveryKey, PreviewID: p.ID, Action: p.Action, AuditID: string(auditID), Candidates: []memory.CandidateRef{}}
		if p.Action == "accept" {
			op := memory.OwnerReviewOperation{SchemaVersion: 6, Kind: "owner_candidate_review", OperationID: p.Effect.OperationID, IdempotencyKey: decision.DeliveryKey, Actor: memory.SemanticActorOwner, SessionID: current[0].Candidate.Support[0].SessionID, SourceEventID: current[0].Candidate.Support[0].Locator.EventID, Preview: p, AuditID: string(auditID)}
			accepted, err := s.applyOwnerReviewOperation(ctx, conn, op, s.now())
			if err != nil {
				return err
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
				return err
			}
			count, err := updated.RowsAffected()
			if err != nil || count != 1 {
				return ErrReviewStale
			}
			ref := item.Ref
			ref.ReviewRevision++
			result.Candidates = append(result.Candidates, ref)
		}
		audit := struct {
			Preview  memory.ReviewPreview  `json:"preview"`
			Decision memory.ReviewDecision `json:"decision"`
			Result   memory.ReviewResult   `json:"result"`
		}{p, decision, result}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_audits VALUES(?,?,?,?,?)`, auditID, a.scope, p.ID, p.Action, compilerJSON(audit)); err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_deliveries VALUES(?,?,?,?,?)`, memory.LocalOwnerID, decision.DeliveryKey, a.scope, requestHash, compilerJSON(result)); err != nil {
			return err
		}
		for _, item := range current {
			if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_resolutions VALUES(?,?,?,?)`, item.Ref.ID, memory.LocalOwnerID, decision.DeliveryKey, auditID); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrReviewResolved) {
		return priorResolution, err
	}
	if err != nil {
		return memory.ReviewResult{}, err
	}
	return result, nil
}

// applyOwnerReviewOperation writes a complete recorded effect. Live callers
// first validate the durable preview and current evidence under owner authority;
// replay supplies the previously accepted envelope and recorded commit time.
// Neither route uses or changes source-session turn authority.
func (s *Store) applyOwnerReviewOperation(ctx context.Context, conn *sql.Conn, op memory.OwnerReviewOperation, clock time.Time) (memory.OwnerReviewOperationResult, error) {
	var result memory.OwnerReviewOperationResult
	if err := validateOwnerReviewOperation(op); err != nil {
		return result, err
	}
	writer := reviewWriter{conn}
	effect := op.Preview.Effect
	byKey, err := validateSemanticScopeVector(ctx, writer, effect.Scopes, effect.PriorRevisions, clock)
	if err != nil {
		return result, ErrReviewStale
	}
	if byKey[effect.Scope.Key] != effect.Scope {
		return result, errors.New("review target scope mismatch")
	}
	for _, item := range effect.Claims {
		if err = validateReviewClaimEffects(ctx, conn, effect, item); err != nil {
			return result, err
		}
	}
	now, err := nextSemanticTransactionTime(ctx, writer, clock)
	if err != nil {
		return result, err
	}
	result = memory.OwnerReviewOperationResult{OperationID: op.OperationID, ClaimIDs: []memory.SemanticID{}, SourceLinkIDs: []memory.SemanticID{}, TransactionTime: now, ResultingRevisions: []memory.ScopeRevision{}}
	for _, scope := range effect.Scopes {
		revision := scope.Revision
		if scope.Key == effect.Scope.Key || scope.Key == "global" && reviewWritesGlobal(effect) {
			revision++
		}
		result.ResultingRevisions = append(result.ResultingRevisions, memory.ScopeRevision{ScopeKey: scope.Key, Revision: revision})
	}
	for _, item := range effect.Claims {
		result.ClaimIDs = append(result.ClaimIDs, item.Claim.ID)
		for _, source := range item.Sources {
			result.SourceLinkIDs = append(result.SourceLinkIDs, source.ID)
		}
	}
	proposalHash, proposalJSON, err := semanticHash(canonicalOwnerReviewOperation(op))
	if err != nil {
		return result, err
	}
	effectHash, _, err := ownerReviewEffectHash(effect)
	if err != nil {
		return result, err
	}
	if err = recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{SchemaVersion: 6, OperationID: op.OperationID, Kind: op.Kind, IdempotencyKey: op.IdempotencyKey, Actor: op.Actor, SessionID: op.SessionID, TargetScopeID: effect.Scope.ID, SourceEventID: op.SourceEventID, ProposalHash: proposalHash, EffectHash: effectHash, ProposalJSON: proposalJSON, PreparedJSON: compilerJSON(op), ResultJSON: compilerJSON(result), TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey}); err != nil {
		return result, err
	}
	if err = applyReviewIdentityEffects(ctx, conn, effect, byKey, now); err != nil {
		return result, err
	}
	for _, item := range effect.Claims {
		if item.Create {
			if err = insertReplacementClaim(ctx, writer, memory.CorrectClaimProposal{OperationID: op.OperationID, Scope: effect.Scope, ReplacementClaim: item.Claim}, now); err != nil {
				return result, err
			}
			if err = reviewInitialState(ctx, conn, effect.Scope, item.Claim.ID, "claim", "active", op.OperationID, now); err != nil {
				return result, err
			}
		}
		for _, source := range item.Sources {
			if !source.Create {
				continue
			}
			_, err = conn.ExecContext(ctx, `INSERT INTO semantic_source_links(source_link_id,scope_id,claim_id,event_id,source_session_id,source_scope_key,event_part,locator_kind,locator_value,evidence_sha256,source_actor,source_type,authority,observed_at,eligibility,created_operation_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'eligible',?)`, source.ID, effect.Scope.ID, item.Claim.ID, source.EventID, source.SessionID, source.ScopeKey, source.EventPart, source.LocatorKind, source.LocatorValue, source.EvidenceSHA256, source.Actor, source.SourceType, source.Authority, source.ObservedAt, op.OperationID)
			if err != nil {
				return result, err
			}
			if err = reviewInitialState(ctx, conn, effect.Scope, source.ID, "source_link", "eligible", op.OperationID, now); err != nil {
				return result, err
			}
		}
	}
	for _, scope := range effect.Scopes {
		if scope.Key != effect.Scope.Key && !(scope.Key == "global" && reviewWritesGlobal(effect)) {
			continue
		}
		updated, err := conn.ExecContext(ctx, `UPDATE semantic_scopes SET revision=revision+1 WHERE scope_id=? AND revision=?`, scope.ID, scope.Revision)
		if err != nil {
			return result, err
		}
		count, err := updated.RowsAffected()
		if err != nil || count != 1 {
			return result, ErrReviewStale
		}
	}
	return result, nil
}

func reviewInitialState(ctx context.Context, conn *sql.Conn, scope memory.SemanticScope, id memory.SemanticID, kind, state string, operation memory.SemanticID, at time.Time) error {
	_, err := conn.ExecContext(ctx, `INSERT INTO semantic_state_events(scope_id,object_kind,object_id,state,operation_id,scope_revision,transaction_time) VALUES(?,?,?,?,?,?,?)`, scope.ID, kind, id, state, operation, scope.Revision+1, formatSemanticTime(at))
	return err
}

func validateReviewClaimEffects(ctx context.Context, q reviewQuery, effect *memory.ReviewEffect, item memory.ReviewClaimEffect) error {
	keys := []string{}
	for _, scope := range effect.Scopes {
		keys = append(keys, scope.Key)
	}
	var err error
	for _, expected := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
		if expected == nil {
			continue
		}
		if expected.Create {
			if err = validateNewReviewEntity(ctx, q, effect, *expected); err != nil {
				return err
			}
			continue
		}
		entity, err := reviewEntity(ctx, q, keys, expected.ID)
		if err != nil || entity != *expected {
			return ErrReviewStale
		}
	}
	if item.Predicate.Create {
		if effect.Identity == nil {
			return errors.New("unreviewed Predicate creation")
		}
		if err = validateReviewNewPredicate(ctx, q, item.Predicate); err != nil {
			return err
		}
	} else {
		var predicate memory.SemanticPredicate
		err = q.QueryRowContext(ctx, `SELECT predicate_id,token,version,label,object_constraint,cardinality FROM semantic_predicates WHERE predicate_id=?`, item.Predicate.ID).Scan(&predicate.ID, &predicate.Token, &predicate.Version, &predicate.Label, &predicate.ObjectConstraint, &predicate.Cardinality)
		if err != nil || predicate != item.Predicate {
			return ErrReviewStale
		}
	}
	if !item.Create {
		claim, err := loadSemanticClaim(ctx, q, item.Claim.ID)
		if err != nil || string(compilerJSON(claim)) != string(compilerJSON(item.Claim)) {
			return ErrReviewStale
		}
		state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, item.Claim.ID)
		if err != nil || state.State != memory.SemanticStateActive {
			return ErrReviewStale
		}
	}
	for _, source := range item.Sources {
		if !source.Create {
			current, err := loadReviewRecordedSource(ctx, q, source.ID, source.Evidence)
			if err != nil {
				return err
			}
			current.Create = false
			source.Create = false
			if string(compilerJSON(current)) != string(compilerJSON(source)) {
				return ErrReviewStale
			}
			state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectSourceLink, source.ID)
			if err != nil || state.State != memory.SemanticStateEligible {
				return ErrReviewStale
			}
		}
	}
	warnings, err := reviewClaimConflicts(ctx, q, effect.Scope.ID, item.Claim)
	if err != nil {
		return err
	}
	if string(compilerJSON(warnings)) != string(compilerJSON(item.Conflicts)) {
		return ErrReviewStale
	}
	return nil
}

func validateOwnerReviewOperation(op memory.OwnerReviewOperation) error {
	if op.SchemaVersion != 6 || op.Kind != "owner_candidate_review" || op.Actor != memory.SemanticActorOwner || !reviewDeliveryValid(op.IdempotencyKey) || op.Preview.Action != "accept" {
		return errors.New("invalid owner review operation")
	}
	if err := validateOwnerReviewEncoding(op.Preview); err != nil {
		return err
	}
	effect := op.Preview.Effect
	if effect.OperationID != op.OperationID || effect.Scope.Key != op.Preview.ScopeKey {
		return errors.New("owner review effect envelope mismatch")
	}
	for _, id := range []string{string(op.OperationID), string(op.SessionID), string(op.SourceEventID), op.AuditID, op.Preview.ID, string(effect.Scope.ID)} {
		if err := validateSemanticUUID(id); err != nil {
			return err
		}
	}
	for i, item := range effect.Claims {
		if item.Candidate != op.Preview.Candidates[i].Ref || item.Claim.ScopeKey != effect.Scope.Key || item.Claim.SubjectEntityID != item.Subject.ID || item.Claim.Predicate != item.Predicate || len(item.Sources) == 0 {
			return errors.New("invalid review Claim effect")
		}
		if err := validateClaimObject(item.Claim.Object); err != nil {
			return err
		}
		if _, err := normalizeValidTime(item.Claim.ValidTime); err != nil {
			return err
		}
		if item.Claim.Polarity != memory.PolarityAffirmed && item.Claim.Polarity != memory.PolarityDenied {
			return errors.New("invalid review Claim polarity")
		}
		if item.Create && item.Claim.CreatedOperationID != op.OperationID {
			return errors.New("new Claim operation mismatch")
		}
		if item.Claim.Object.EntityID != "" {
			if item.ObjectEntity == nil || item.ObjectEntity.ID != item.Claim.Object.EntityID || item.Predicate.ObjectConstraint != memory.ConstraintEntity {
				return errors.New("object identity mismatch")
			}
		} else if item.ObjectEntity != nil || string(item.Predicate.ObjectConstraint) != string(item.Claim.Object.Literal.Kind) {
			return errors.New("literal constraint mismatch")
		}
		for _, source := range item.Sources {
			if source.Actor != memory.SemanticActorOwner || source.Authority != memory.AuthorityOwnerStatement || source.SourceType != memory.SourceTypeUserMessage || source.EventPart != memory.EvidenceContent || source.Eligibility != memory.EligibilityEligible || source.Create && source.OperationID != op.OperationID {
				return ErrReviewInvalidSource
			}
			if source.EvidenceSHA256 != "sha256:"+memory.CompilerHash([]byte(source.Evidence)) {
				return fmt.Errorf("recorded source digest mismatch")
			}
		}
	}
	return validateReviewTypedEnvelope(op)
}

// The v6 envelope has typed IDs: compiler identities are SHA256 values while
// accepted graph IDs are UUIDv4. Replay does not apply Stage3's generic *_id
// rule to opaque candidate/generation hashes or the local owner principal.
func validateReviewTypedEnvelope(op memory.OwnerReviewOperation) error {
	p := op.Preview
	effect := p.Effect
	if p.OwnerID != memory.LocalOwnerID || p.AuthorizationRevision < 1 || p.SourcePolicy != ownerReviewSourcePolicyV1 {
		return errors.New("invalid recorded owner authorization")
	}
	hashID := func(value string) bool {
		b, err := hex.DecodeString(value)
		return err == nil && len(b) == 32 && value == strings.ToLower(value)
	}
	if !hashID(p.AuthenticationBinding) || !hashID(p.JobID) || !hashID(p.GenerationID) {
		return errors.New("invalid recorded compiler identity")
	}
	for i, scope := range effect.Scopes {
		if validateSemanticUUID(string(scope.ID)) != nil || i > 0 && effect.Scopes[i-1].Key >= scope.Key {
			return errors.New("invalid recorded scope identity")
		}
		_, registry, err := splitScopeKey(scope.Key)
		if err != nil || registry != scope.RegistryID {
			return errors.New("invalid recorded scope lineage")
		}
	}
	for i, item := range effect.Claims {
		original := p.Candidates[i]
		if !hashID(original.Ref.ID) || original.Ref.InterpretationRevision < 0 || original.Identity == nil && original.Ref.InterpretationRevision != 0 || original.Ref.ReviewRevision < 0 || original.JobID != p.JobID || original.GenerationID != p.GenerationID || original.Destination != p.ScopeKey || original.Redacted || original.Candidate.ID != original.Ref.ID || original.Candidate.ReviewRevision != original.Ref.ReviewRevision || original.Candidate.ReviewState != "unresolved" || original.Candidate.EquivalentTo != "" {
			return errors.New("invalid recorded candidate identity")
		}
		proposal := original.Candidate.Proposal
		resolved, err := reviewResolvedProposition(p, i)
		if err != nil {
			return err
		}
		if resolved.SubjectEntityID != item.Claim.SubjectEntityID || resolved.PredicateID != item.Predicate.ID || resolved.Polarity != item.Claim.Polarity || string(compilerJSON(resolved.Object)) != string(compilerJSON(item.Claim.Object)) || !validTimesEqual(proposal.ValidTime, item.Claim.ValidTime) || proposal.TemporalQualification != item.TemporalQualification || string(compilerJSON(original.Candidate.Context)) != string(compilerJSON(item.Context)) {
			return errors.New("review effect differs from reviewed meaning")
		}
		for _, id := range []memory.SemanticID{item.Claim.ID, item.Subject.ID, item.Predicate.ID, item.Claim.CreatedOperationID} {
			if validateSemanticUUID(string(id)) != nil {
				return errors.New("invalid recorded semantic identity")
			}
		}
		if len(item.Sources) != len(original.Candidate.Support) || len(item.Sources) > 64 || len(item.Context) > 8 {
			return errors.New("incomplete source interpretation")
		}
		for n, source := range item.Sources {
			originalSource := original.Candidate.Support[n]
			if validateSemanticUUID(string(source.ID)) != nil || validateSemanticUUID(string(source.OperationID)) != nil || validateSemanticUUID(string(source.EventID)) != nil || validateSemanticUUID(string(source.SessionID)) != nil {
				return errors.New("invalid recorded source identity")
			}
			if source.EventID != originalSource.Locator.EventID || source.SessionID != originalSource.SessionID || source.ScopeKey != originalSource.ScopeKey || source.EventPart != originalSource.Locator.EventPart || source.LocatorKind != originalSource.Locator.LocatorKind || source.LocatorValue != originalSource.Locator.LocatorValue || source.EvidenceSHA256 != "sha256:"+originalSource.Locator.EvidenceSHA256 || source.Evidence != originalSource.Evidence || source.ObservedAt != originalSource.ObservedAt || source.Actor != originalSource.Actor || source.Authority != originalSource.Authority || source.SourceType != originalSource.SourceType {
				return errors.New("review source differs from original authority")
			}
		}
		for _, source := range item.Context {
			if source.Authority != "none" || source.Actor != "assistant" || source.SourceType != "assistant_message" || source.Usage != "context" || source.Locator.EvidenceSHA256 != memory.CompilerHash([]byte(source.Evidence)) {
				return errors.New("invalid recorded interpretation context")
			}
		}
	}
	first := p.Candidates[0].Candidate.Support[0]
	if op.SessionID != first.SessionID || op.SourceEventID != first.Locator.EventID {
		return errors.New("recorded source envelope mismatch")
	}
	return nil
}
