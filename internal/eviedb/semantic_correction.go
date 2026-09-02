package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type canonicalCorrection struct {
	OldClaimID       memory.SemanticID              `json:"old_claim_id"`
	ReplacementClaim memory.SemanticID              `json:"replacement_claim_id"`
	Mode             memory.CorrectionMode          `json:"mode"`
	EffectiveTime    *string                        `json:"effective_time"`
	ValidTimeEffect  canonicalCorrectionValidEffect `json:"valid_time_effect"`
}

type canonicalCorrectionValidEffect struct {
	OldBefore   canonicalValidTime `json:"old_before"`
	OldAfter    canonicalValidTime `json:"old_after"`
	Replacement canonicalValidTime `json:"replacement"`
}

type canonicalCorrectionClaimObject struct {
	EntityID memory.SemanticID    `json:"entity_id,omitempty"`
	Literal  *memory.TypedLiteral `json:"literal,omitempty"`
}

type canonicalCorrectionClaim struct {
	ClaimID          memory.SemanticID              `json:"claim_id"`
	ScopeKey         string                         `json:"scope_key"`
	SubjectEntityID  memory.SemanticID              `json:"subject_entity_id"`
	PredicateID      memory.SemanticID              `json:"predicate_id"`
	PredicateToken   string                         `json:"predicate_token"`
	PredicateVersion int64                          `json:"predicate_version"`
	Object           canonicalCorrectionClaimObject `json:"object"`
	Polarity         memory.ClaimPolarity           `json:"polarity"`
	ValidTime        canonicalValidTime             `json:"valid_time"`
	Lifecycle        string                         `json:"lifecycle"`
}

type canonicalTransition struct {
	ObjectKind string                    `json:"object_kind"`
	ObjectID   memory.SemanticID         `json:"object_id"`
	State      memory.SemanticStateValue `json:"state"`
}

type canonicalCorrectionEffect struct {
	Scopes      []string                   `json:"scopes"`
	Predicates  []struct{}                 `json:"predicates"`
	Entities    []struct{}                 `json:"entities"`
	Aliases     []struct{}                 `json:"aliases"`
	Claims      []canonicalCorrectionClaim `json:"claims"`
	SourceLinks []canonicalSourceLink      `json:"source_links"`
	GraphLinks  []struct{}                 `json:"graph_links"`
	Transitions []canonicalTransition      `json:"transitions"`
	Corrections []canonicalCorrection      `json:"corrections"`
}

type canonicalCorrectionProposal struct {
	Kind           string                    `json:"kind"`
	IdempotencyKey string                    `json:"idempotency_key"`
	Actor          memory.SemanticActor      `json:"actor"`
	SessionID      memory.SessionID          `json:"session_id"`
	PriorRevisions []memory.ScopeRevision    `json:"prior_revisions"`
	SourceEventIDs []memory.EventID          `json:"source_event_ids"`
	Effect         canonicalCorrectionEffect `json:"effect"`
}

func canonicalCorrectClaimProposal(proposal memory.CorrectClaimProposal) canonicalCorrectionProposal {
	effective := (*string)(nil)
	if proposal.EffectiveTime != nil {
		value := formatSemanticTime(*proposal.EffectiveTime)
		effective = &value
	}
	claim := proposal.ReplacementClaim
	transitions := make([]canonicalTransition, len(proposal.Transitions))
	for i, transition := range proposal.Transitions {
		transitions[i] = canonicalTransition(transition)
	}
	return canonicalCorrectionProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Source.EventID},
		Effect: canonicalCorrectionEffect{
			Scopes: []string{proposal.Scope.Key}, Predicates: []struct{}{}, Entities: []struct{}{}, Aliases: []struct{}{},
			Claims: []canonicalCorrectionClaim{{
				ClaimID: claim.ID, ScopeKey: claim.ScopeKey, SubjectEntityID: claim.SubjectEntityID,
				PredicateID: claim.Predicate.ID, PredicateToken: claim.Predicate.Token,
				PredicateVersion: claim.Predicate.Version,
				Object:           canonicalCorrectionClaimObject{EntityID: claim.Object.EntityID, Literal: claim.Object.Literal},
				Polarity:         claim.Polarity, ValidTime: encodeCanonicalValidTime(claim.ValidTime), Lifecycle: "active",
			}},
			SourceLinks: []canonicalSourceLink{{
				SourceLinkID: proposal.Source.ID, ClaimID: claim.ID,
				Locator: canonicalLocator{EventID: proposal.Source.EventID, EventPart: proposal.Source.EventPart,
					LocatorKind: proposal.Source.LocatorKind, LocatorValue: proposal.Source.LocatorValue,
					EvidenceSHA256: proposal.Source.EvidenceSHA256},
				Actor: proposal.Source.Actor, SourceType: proposal.Source.SourceType,
				Authority: proposal.Source.Authority, ObservedAt: proposal.Source.ObservedAt,
				Eligibility: memory.EligibilityEligible,
			}},
			GraphLinks: []struct{}{}, Transitions: transitions,
			Corrections: []canonicalCorrection{{
				OldClaimID: proposal.OldClaim.ID, ReplacementClaim: claim.ID, Mode: proposal.Mode,
				EffectiveTime: effective,
				ValidTimeEffect: canonicalCorrectionValidEffect{
					OldBefore:   encodeCanonicalValidTime(proposal.ValidTimeEffect.OldBefore),
					OldAfter:    encodeCanonicalValidTime(proposal.ValidTimeEffect.OldAfter),
					Replacement: encodeCanonicalValidTime(proposal.ValidTimeEffect.Replacement),
				},
			}},
		},
	}
}

func normalizeCorrectClaimRequest(request memory.CorrectClaimRequest) (memory.CorrectClaimRequest, error) {
	if request.Mode != memory.CorrectionError && request.Mode != memory.CorrectionChanged {
		return request, fmt.Errorf("unsupported correction mode %q", request.Mode)
	}
	if request.Mode == memory.CorrectionError && request.EffectiveTime != nil {
		return request, errors.New("error correction cannot have an effective time")
	}
	if request.Mode == memory.CorrectionChanged && request.EffectiveTime == nil {
		return request, errors.New("changed correction requires an effective time")
	}
	if request.EffectiveTime != nil {
		normalized, err := normalizeValidTime(memory.ValidTime{From: request.EffectiveTime})
		if err != nil {
			return request, err
		}
		request.EffectiveTime = normalized.From
	}
	if request.ReplacementValidTime != nil {
		normalized, err := normalizeValidTime(*request.ReplacementValidTime)
		if err != nil {
			return request, err
		}
		request.ReplacementValidTime = &normalized
	}
	if request.Mode == memory.CorrectionChanged && request.ReplacementValidTime != nil {
		return request, errors.New("changed correction derives both intervals from its effective time")
	}
	if request.Replacement.Polarity != memory.PolarityAffirmed && request.Replacement.Polarity != memory.PolarityDenied {
		return request, fmt.Errorf("unsupported Claim polarity %q", request.Replacement.Polarity)
	}
	if err := validateClaimObject(request.Replacement.Object); err != nil {
		return request, err
	}
	return request, nil
}

func validateClaimObject(object memory.ClaimObject) error {
	hasEntity := object.EntityID != ""
	hasLiteral := object.Literal != nil
	if hasEntity == hasLiteral {
		return errors.New("Claim object must contain exactly one Entity or Typed Literal")
	}
	if hasEntity {
		return validateSemanticUUID(string(object.EntityID))
	}
	return validateLiteral(*object.Literal)
}

func correctionRequestsEqual(left, right memory.CorrectClaimRequest) bool {
	leftHash, _, leftErr := semanticHash(left)
	rightHash, _, rightErr := semanticHash(right)
	return leftErr == nil && rightErr == nil && leftHash == rightHash
}

const semanticClaimByIDQuery = `
	SELECT claims.claim_id, scopes.scope_key, claims.subject_entity_id,
	       predicates.predicate_id, predicates.token, predicates.version, predicates.label,
	       predicates.object_constraint, predicates.cardinality,
	       claims.object_kind, claims.object_entity_id, claims.literal_kind, claims.literal_value,
	       claims.polarity, claims.valid_from, claims.valid_to,
	       claims.created_operation_id, claims.transaction_time
	FROM semantic_claims AS claims
	JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
	JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
	WHERE claims.claim_id = ?
`

func loadSemanticClaim(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id memory.SemanticID) (memory.SemanticClaim, error) {
	return scanSemanticClaim(query.QueryRowContext(ctx, semanticClaimByIDQuery, id))
}

func correctionValidTimeEffect(old memory.ValidTime, request memory.CorrectClaimRequest) (memory.CorrectionValidTimeEffect, error) {
	effect := memory.CorrectionValidTimeEffect{OldBefore: old, OldAfter: old, Replacement: old}
	if request.Mode == memory.CorrectionError {
		if request.ReplacementValidTime != nil {
			effect.Replacement = *request.ReplacementValidTime
		}
		return effect, nil
	}
	effective := *request.EffectiveTime
	if old.From != nil && !effective.After(*old.From) {
		return effect, errors.New("changed correction effective time must be after the old interval start")
	}
	if old.To != nil && !effective.Before(*old.To) {
		return effect, errors.New("changed correction effective time must be before the old interval end")
	}
	effect.OldAfter = memory.ValidTime{From: old.From, To: &effective}
	effect.Replacement = memory.ValidTime{From: &effective, To: old.To}
	return effect, nil
}

func (s *Store) PrepareCorrectClaim(ctx context.Context, scope memory.ScopeContext, request memory.CorrectClaimRequest) (memory.CorrectClaimProposal, error) {
	var err error
	request, err = normalizeCorrectClaimRequest(request)
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.CorrectClaimProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if err := validateSemanticUUID(string(request.OldClaimID)); err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	var priorJSON, priorHash string
	err = s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&priorJSON, &priorHash)
	if err == nil {
		var proposal memory.CorrectClaimProposal
		if err := json.Unmarshal([]byte(priorJSON), &proposal); err != nil {
			return proposal, err
		}
		if !correctionRequestsEqual(proposal.Request, request) || proposal.SessionID != scope.SessionID {
			return memory.CorrectClaimProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		return proposal, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.CorrectClaimProposal{}, err
	}

	oldClaim, err := loadSemanticClaim(ctx, s.db, request.OldClaimID)
	if err != nil {
		return memory.CorrectClaimProposal{}, fmt.Errorf("load corrected Claim: %w", err)
	}
	targetKey := scopeKeyForContext(scope)
	if oldClaim.ScopeKey != targetKey {
		return memory.CorrectClaimProposal{}, errors.New("corrected Claim is outside the session-bound scope")
	}
	var latestState memory.SemanticStateValue
	if err := s.db.QueryRowContext(ctx, `
		SELECT state FROM semantic_state_events WHERE object_kind = 'claim' AND object_id = ?
		ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC LIMIT 1
	`, oldClaim.ID).Scan(&latestState); err != nil || latestState != memory.SemanticStateActive {
		return memory.CorrectClaimProposal{}, errors.New("corrected Claim is not active")
	}
	var priorCorrection int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_claim_corrections WHERE old_claim_id = ?`, oldClaim.ID).Scan(&priorCorrection); err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	if priorCorrection != 0 {
		return memory.CorrectClaimProposal{}, errors.New("corrected Claim was already superseded")
	}

	target, err := loadSemanticScope(ctx, s.db, targetKey)
	if err != nil || target.Create {
		return memory.CorrectClaimProposal{}, errors.New("corrected Claim scope is not registered")
	}
	global := target
	if targetKey != "global" {
		global, err = loadSemanticScope(ctx, s.db, "global")
		if err != nil || global.Create {
			return memory.CorrectClaimProposal{}, errors.New("global semantic scope is not registered")
		}
	}
	if _, err := loadEntityByID(ctx, s.db, request.Replacement.SubjectEntityID, targetKey, targetKey); err != nil {
		return memory.CorrectClaimProposal{}, fmt.Errorf("resolve replacement subject: %w", err)
	}
	predicate := memory.SemanticPredicate{}
	if err := s.db.QueryRowContext(ctx, `
		SELECT predicate_id, token, version, label, object_constraint, cardinality
		FROM semantic_predicates WHERE predicate_id = ?
	`, request.Replacement.PredicateID).Scan(&predicate.ID, &predicate.Token, &predicate.Version, &predicate.Label,
		&predicate.ObjectConstraint, &predicate.Cardinality); err != nil {
		return memory.CorrectClaimProposal{}, fmt.Errorf("resolve replacement Predicate: %w", err)
	}
	if request.Replacement.Object.EntityID != "" {
		if predicate.ObjectConstraint != memory.ConstraintEntity {
			return memory.CorrectClaimProposal{}, errors.New("replacement object violates its Predicate constraint")
		}
		if _, err := loadEntityByID(ctx, s.db, request.Replacement.Object.EntityID, targetKey, targetKey); err != nil {
			return memory.CorrectClaimProposal{}, fmt.Errorf("resolve replacement object: %w", err)
		}
	} else if predicate.ObjectConstraint != memory.PredicateObjectConstraint(request.Replacement.Object.Literal.Kind) {
		return memory.CorrectClaimProposal{}, errors.New("replacement literal violates its Predicate constraint")
	}
	effect, err := correctionValidTimeEffect(oldClaim.ValidTime, request)
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	source, err := loadOwnerSource(s.db.QueryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
	`, request.SourceEventID), scope.SessionID, request.SourceEventID, targetKey)
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	operationID, err := newSemanticID()
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	replacementID, err := newSemanticID()
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	source.ID, err = newSemanticID()
	if err != nil {
		return memory.CorrectClaimProposal{}, err
	}
	source.OperationID, source.Eligibility, source.Create = operationID, memory.EligibilityEligible, true
	replacement := memory.SemanticClaim{
		ID: replacementID, ScopeKey: targetKey, SubjectEntityID: request.Replacement.SubjectEntityID,
		Predicate: predicate, Object: request.Replacement.Object, Polarity: request.Replacement.Polarity,
		ValidTime: effect.Replacement, CreatedOperationID: operationID,
	}
	scopes := []memory.SemanticScope{global.SemanticScope}
	if targetKey != "global" {
		scopes = append(scopes, target.SemanticScope)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Key < scopes[j].Key })
	priors := make([]memory.ScopeRevision, len(scopes))
	for i, semanticScope := range scopes {
		priors[i] = memory.ScopeRevision{ScopeKey: semanticScope.Key, Revision: semanticScope.Revision}
	}
	proposal := memory.CorrectClaimProposal{
		SchemaVersion: 2, Kind: "correct_claim", OperationID: operationID,
		IdempotencyKey: request.IdempotencyKey, Actor: memory.SemanticActorOwner, SessionID: scope.SessionID,
		Scope: target.SemanticScope, Scopes: scopes, PriorRevisions: priors, ExpectedRevision: target.Revision,
		OldClaim: oldClaim, ReplacementClaim: replacement, Source: source, Mode: request.Mode,
		EffectiveTime: request.EffectiveTime, ValidTimeEffect: effect,
		Transitions: []memory.SemanticTransition{
			{ObjectKind: "claim", ObjectID: oldClaim.ID, State: memory.SemanticStateSuperseded},
			{ObjectKind: "claim", ObjectID: replacement.ID, State: memory.SemanticStateActive},
			{ObjectKind: "source_link", ObjectID: source.ID, State: memory.SemanticStateEligible},
		},
		Request: request,
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalCorrectClaimProposal(proposal))
	if err != nil {
		return proposal, err
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	return proposal, err
}

func validateCorrectClaimProposal(proposal memory.CorrectClaimProposal) error {
	if proposal.SchemaVersion != 2 || proposal.Kind != "correct_claim" || proposal.Actor != memory.SemanticActorOwner {
		return errors.New("unsupported correction proposal")
	}
	request, err := normalizeCorrectClaimRequest(proposal.Request)
	if err != nil || !correctionRequestsEqual(request, proposal.Request) {
		return errors.New("correction proposal request is invalid")
	}
	if proposal.IdempotencyKey != request.IdempotencyKey || proposal.OldClaim.ID != request.OldClaimID ||
		proposal.Mode != request.Mode || !nullableTimesEqual(proposal.EffectiveTime, request.EffectiveTime) ||
		proposal.ExpectedRevision != proposal.Scope.Revision || proposal.ReplacementClaim.ScopeKey != proposal.Scope.Key ||
		proposal.ReplacementClaim.SubjectEntityID != request.Replacement.SubjectEntityID ||
		proposal.ReplacementClaim.Predicate.ID != request.Replacement.PredicateID ||
		proposal.ReplacementClaim.Polarity != request.Replacement.Polarity ||
		proposal.ReplacementClaim.Object.EntityID != request.Replacement.Object.EntityID ||
		!typedLiteralPointersEqual(proposal.ReplacementClaim.Object.Literal, request.Replacement.Object.Literal) {
		return ErrIdempotencyConflict
	}
	if proposal.OldClaim.ID == proposal.ReplacementClaim.ID || proposal.OldClaim.ScopeKey != proposal.Scope.Key ||
		proposal.ReplacementClaim.CreatedOperationID != proposal.OperationID ||
		!proposal.ReplacementClaim.TransactionTime.IsZero() || proposal.OldClaim.TransactionTime.IsZero() ||
		proposal.Source.ID == "" || proposal.Source.ID == proposal.OldClaim.ID ||
		proposal.Source.ID == proposal.ReplacementClaim.ID || proposal.Source.ID == proposal.OperationID {
		return errors.New("correction proposal identities are invalid")
	}
	effect, err := correctionValidTimeEffect(proposal.OldClaim.ValidTime, request)
	if err != nil || !validTimesEqual(effect.OldBefore, proposal.ValidTimeEffect.OldBefore) ||
		!validTimesEqual(effect.OldAfter, proposal.ValidTimeEffect.OldAfter) ||
		!validTimesEqual(effect.Replacement, proposal.ValidTimeEffect.Replacement) ||
		!validTimesEqual(effect.Replacement, proposal.ReplacementClaim.ValidTime) {
		return errors.New("correction proposal Valid Time effect is invalid")
	}
	wantTransitions := []memory.SemanticTransition{
		{ObjectKind: "claim", ObjectID: proposal.OldClaim.ID, State: memory.SemanticStateSuperseded},
		{ObjectKind: "claim", ObjectID: proposal.ReplacementClaim.ID, State: memory.SemanticStateActive},
		{ObjectKind: "source_link", ObjectID: proposal.Source.ID, State: memory.SemanticStateEligible},
	}
	if len(proposal.Transitions) != len(wantTransitions) {
		return errors.New("correction proposal transitions are incomplete")
	}
	for i := range wantTransitions {
		if proposal.Transitions[i] != wantTransitions[i] {
			return errors.New("correction proposal transition changed")
		}
	}
	if proposal.Source.EventID != request.SourceEventID || proposal.Source.Actor != memory.SemanticActorOwner ||
		proposal.Source.SourceType != memory.SourceTypeUserMessage || proposal.Source.Authority != memory.AuthorityOwnerStatement ||
		proposal.Source.EventPart != memory.EvidenceContent || proposal.Source.LocatorKind != memory.LocatorWhole ||
		proposal.Source.LocatorValue != "" || proposal.Source.Eligibility != memory.EligibilityEligible || !proposal.Source.Create ||
		proposal.Source.OperationID != proposal.OperationID {
		return errors.New("correction proposal source is invalid")
	}
	return validateClaimObject(proposal.ReplacementClaim.Object)
}

func typedLiteralPointersEqual(left, right *memory.TypedLiteral) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func validateCorrectionSource(ctx context.Context, writer turnLeaseWriteExecutor, proposal memory.CorrectClaimProposal) error {
	var sessionID, eventType, role, content, recordedAt string
	if err := writer.queryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
	`, proposal.Source.EventID).Scan(&sessionID, &eventType, &role, &content, &recordedAt); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(content))
	observed, err := time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil || sessionID != string(proposal.SessionID) || eventType != string(memory.EventUserMessage) ||
		role != string(memory.RoleUser) || proposal.Source.Evidence != content ||
		proposal.Source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", digest) ||
		proposal.Source.ObservedAt != formatSemanticTime(observed) || proposal.Source.ScopeKey != proposal.Scope.Key {
		return errors.New("correction source evidence changed")
	}
	return nil
}

func semanticClaimsEqual(left, right memory.SemanticClaim) bool {
	return left.ID == right.ID && left.ScopeKey == right.ScopeKey && left.SubjectEntityID == right.SubjectEntityID &&
		left.Predicate == right.Predicate && left.Object.EntityID == right.Object.EntityID &&
		typedLiteralPointersEqual(left.Object.Literal, right.Object.Literal) && left.Polarity == right.Polarity &&
		validTimesEqual(left.ValidTime, right.ValidTime) && left.CreatedOperationID == right.CreatedOperationID &&
		left.TransactionTime.Equal(right.TransactionTime)
}

func (s *Store) ApplyCorrectClaim(ctx context.Context, lease memory.TurnLease, proposal memory.CorrectClaimProposal) (result memory.CorrectClaimResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("semantic correction does not match its turn lease")
	}
	canonical := canonicalCorrectClaimProposal(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if err := validateCorrectClaimProposal(proposal); err != nil {
		return result, err
	}
	if proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != proposalHash ||
		proposal.PreparedSHA256 == "" || proposal.PreparedSHA256 != preparedHash {
		return result, errors.New("semantic correction proposal hash changed")
	}
	for _, id := range []memory.SemanticID{proposal.OperationID, proposal.Scope.ID, proposal.OldClaim.ID,
		proposal.ReplacementClaim.ID, proposal.Source.ID, proposal.ReplacementClaim.SubjectEntityID,
		proposal.ReplacementClaim.Predicate.ID} {
		if err := validateSemanticUUID(string(id)); err != nil {
			return result, err
		}
	}
	if proposal.ReplacementClaim.Object.EntityID != "" {
		if err := validateSemanticUUID(string(proposal.ReplacementClaim.Object.EntityID)); err != nil {
			return result, err
		}
	}
	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var acceptedHash, acceptedResult string
		existingErr := writer.queryRowContext(ctx, `SELECT proposal_sha256, result_json FROM semantic_operations WHERE idempotency_key = ?`, proposal.IdempotencyKey).Scan(&acceptedHash, &acceptedResult)
		if existingErr == nil {
			if acceptedHash != proposalHash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal([]byte(acceptedResult), &result)
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		if err := validateCorrectionSource(ctx, writer, proposal); err != nil {
			return err
		}
		expectedTarget, expectedScopes, err := authorizedSemanticScopes(ctx, writer, proposal.SessionID, false)
		if err != nil {
			return err
		}
		if err := validateAuthorizedSemanticScopes(expectedTarget, expectedScopes, proposal.SessionID,
			proposal.Source.SessionID, proposal.Source.ScopeKey, proposal.Scopes); err != nil {
			return err
		}
		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions, s.now())
		if err != nil {
			return err
		}
		target, ok := byKey[proposal.Scope.Key]
		if !ok || target != proposal.Scope || target.Revision != proposal.ExpectedRevision {
			return errors.New("correction target scope changed")
		}
		var workspaceID, projectID sql.NullString
		if err := writer.queryRowContext(ctx, `SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = ?`, proposal.SessionID, memory.SessionActive).Scan(&workspaceID, &projectID); err != nil {
			return err
		}
		if scopeKeyForSessionValues(workspaceID, projectID) != proposal.Scope.Key {
			return errors.New("correction session scope changed")
		}
		old, err := loadSemanticClaimFromWriter(ctx, writer, proposal.OldClaim.ID)
		if err != nil || !semanticClaimsEqual(old, proposal.OldClaim) {
			return errors.New("corrected Claim changed after preparation")
		}
		var state memory.SemanticStateValue
		if err := writer.queryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind = 'claim' AND object_id = ? ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC LIMIT 1`, old.ID).Scan(&state); err != nil || state != memory.SemanticStateActive {
			return errors.New("corrected Claim lifecycle changed after preparation")
		}
		var correctionCount int
		if err := writer.queryRowContext(ctx, `SELECT COUNT(*) FROM semantic_claim_corrections WHERE old_claim_id = ?`, old.ID).Scan(&correctionCount); err != nil || correctionCount != 0 {
			return errors.New("corrected Claim already has a supersession")
		}
		if err := validateReplacementReferences(ctx, writer, proposal); err != nil {
			return err
		}
		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		result = memory.CorrectClaimResult{
			OperationID: proposal.OperationID, OldClaimID: old.ID, ReplacementClaimID: proposal.ReplacementClaim.ID,
			SourceLinkID: proposal.Source.ID, TransactionTime: now, ScopeRevision: target.Revision + 1,
		}
		result.ResultingRevisions = make([]memory.ScopeRevision, len(proposal.Scopes))
		for i, semanticScope := range proposal.Scopes {
			revision := semanticScope.Revision
			if semanticScope.Key == proposal.Scope.Key {
				revision++
			}
			result.ResultingRevisions[i] = memory.ScopeRevision{ScopeKey: semanticScope.Key, Revision: revision}
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		effectHash, _, err := semanticHash(canonical.Effect)
		if err != nil {
			return err
		}
		if err := recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{
			SchemaVersion: proposal.SchemaVersion,
			OperationID:   proposal.OperationID, Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey,
			Actor: proposal.Actor, SessionID: proposal.SessionID, TargetScopeID: proposal.Scope.ID,
			SourceEventID: proposal.Source.EventID, ProposalHash: proposalHash, EffectHash: effectHash,
			ProposalJSON: proposalJSON, PreparedJSON: preparedJSON, ResultJSON: resultJSON,
			TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey,
		}); err != nil {
			return err
		}
		if err := insertReplacementClaim(ctx, writer, proposal, now); err != nil {
			return err
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_source_links (
				source_link_id, scope_id, claim_id, event_id, source_session_id, source_scope_key,
				event_part, locator_kind, locator_value, evidence_sha256, source_actor,
				source_type, authority, observed_at, eligibility, created_operation_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'eligible', ?)
		`, proposal.Source.ID, proposal.Scope.ID, proposal.ReplacementClaim.ID, proposal.Source.EventID, proposal.SessionID, proposal.Source.ScopeKey,
			proposal.Source.EventPart, proposal.Source.LocatorKind, proposal.Source.LocatorValue,
			proposal.Source.EvidenceSHA256, proposal.Source.Actor, proposal.Source.SourceType,
			proposal.Source.Authority, proposal.Source.ObservedAt, proposal.OperationID); err != nil {
			return err
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_claim_corrections (
				operation_id, scope_id, old_claim_id, replacement_claim_id, mode, effective_time,
				old_valid_from, old_valid_to, old_effective_from, old_effective_to,
				replacement_from, replacement_to, scope_revision, transaction_time
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, proposal.OperationID, proposal.Scope.ID, proposal.OldClaim.ID, proposal.ReplacementClaim.ID, proposal.Mode,
			semanticTimeArgument(proposal.EffectiveTime), semanticTimeArgument(proposal.ValidTimeEffect.OldBefore.From),
			semanticTimeArgument(proposal.ValidTimeEffect.OldBefore.To), semanticTimeArgument(proposal.ValidTimeEffect.OldAfter.From),
			semanticTimeArgument(proposal.ValidTimeEffect.OldAfter.To), semanticTimeArgument(proposal.ValidTimeEffect.Replacement.From),
			semanticTimeArgument(proposal.ValidTimeEffect.Replacement.To), result.ScopeRevision, formatSemanticTime(now)); err != nil {
			return err
		}
		for _, transition := range proposal.Transitions {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, proposal.Scope.ID, transition.ObjectKind, transition.ObjectID, transition.State,
				proposal.OperationID, result.ScopeRevision, formatSemanticTime(now)); err != nil {
				return err
			}
		}
		update, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`, result.ScopeRevision, proposal.Scope.ID, proposal.Scope.Revision)
		if err != nil {
			return err
		}
		if changed, _ := update.RowsAffected(); changed != 1 {
			return ErrStaleScopeRevision
		}
		return nil
	})
	return result, err
}

func scopeKeyForSessionValues(workspaceID, projectID sql.NullString) string {
	if workspaceID.Valid {
		return "workspace:" + workspaceID.String
	}
	if projectID.Valid {
		return "project:" + projectID.String
	}
	return "global"
}

func loadSemanticClaimFromWriter(ctx context.Context, writer turnLeaseWriteExecutor, id memory.SemanticID) (memory.SemanticClaim, error) {
	return scanSemanticClaim(writer.queryRowContext(ctx, semanticClaimByIDQuery, id))
}

func scanSemanticClaim(row rowScanner) (memory.SemanticClaim, error) {
	var claim memory.SemanticClaim
	var objectKind string
	var objectEntity, literalKind, literalValue, validFrom, validTo sql.NullString
	var transactionTime string
	err := row.Scan(&claim.ID, &claim.ScopeKey, &claim.SubjectEntityID,
		&claim.Predicate.ID, &claim.Predicate.Token, &claim.Predicate.Version, &claim.Predicate.Label,
		&claim.Predicate.ObjectConstraint, &claim.Predicate.Cardinality,
		&objectKind, &objectEntity, &literalKind, &literalValue, &claim.Polarity, &validFrom, &validTo,
		&claim.CreatedOperationID, &transactionTime)
	if err != nil {
		return claim, err
	}
	if objectKind == "entity" {
		claim.Object.EntityID = memory.SemanticID(objectEntity.String)
	} else {
		claim.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralKind(literalKind.String), Value: literalValue.String}
	}
	if validFrom.Valid {
		value, err := parseSemanticTime(validFrom.String)
		if err != nil {
			return claim, err
		}
		claim.ValidTime.From = &value
	}
	if validTo.Valid {
		value, err := parseSemanticTime(validTo.String)
		if err != nil {
			return claim, err
		}
		claim.ValidTime.To = &value
	}
	claim.TransactionTime, err = parseSemanticTime(transactionTime)
	return claim, err
}

func validateReplacementReferences(ctx context.Context, writer turnLeaseWriteExecutor, proposal memory.CorrectClaimProposal) error {
	var predicate memory.SemanticPredicate
	if err := writer.queryRowContext(ctx, `SELECT predicate_id, token, version, label, object_constraint, cardinality FROM semantic_predicates WHERE predicate_id = ?`, proposal.ReplacementClaim.Predicate.ID).Scan(
		&predicate.ID, &predicate.Token, &predicate.Version, &predicate.Label, &predicate.ObjectConstraint, &predicate.Cardinality,
	); err != nil || predicate != proposal.ReplacementClaim.Predicate {
		return errors.New("replacement Predicate changed after preparation")
	}
	checkEntity := func(id memory.SemanticID) error {
		var count int
		if err := writer.queryRowContext(ctx, `
			SELECT COUNT(*) FROM semantic_entities AS entities
			JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
			WHERE entities.entity_id = ? AND entities.lifecycle = 'active' AND (scopes.scope_key = ? OR scopes.scope_key = 'global')
		`, id, proposal.Scope.Key).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("replacement Entity is outside the correction scope")
		}
		return nil
	}
	if err := checkEntity(proposal.ReplacementClaim.SubjectEntityID); err != nil {
		return fmt.Errorf("replacement subject changed after preparation: %w", err)
	}
	if proposal.ReplacementClaim.Object.EntityID != "" {
		if err := checkEntity(proposal.ReplacementClaim.Object.EntityID); err != nil {
			return err
		}
	}
	return nil
}

func insertReplacementClaim(ctx context.Context, writer turnLeaseWriteExecutor, proposal memory.CorrectClaimProposal, transactionTime time.Time) error {
	claim := proposal.ReplacementClaim
	if claim.Object.EntityID != "" {
		_, err := writer.execContext(ctx, `
			INSERT INTO semantic_claims (
				claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
				object_kind, object_entity_id, polarity, valid_from, valid_to, lifecycle, created_operation_id, transaction_time
			) VALUES (?, ?, ?, ?, ?, ?, 'entity', ?, ?, ?, ?, 'active', ?, ?)
		`, claim.ID, proposal.Scope.ID, claim.SubjectEntityID, claim.Predicate.ID, claim.Predicate.Token,
			claim.Predicate.Version, claim.Object.EntityID, claim.Polarity, semanticTimeArgument(claim.ValidTime.From),
			semanticTimeArgument(claim.ValidTime.To), proposal.OperationID, formatSemanticTime(transactionTime))
		return err
	}
	_, err := writer.execContext(ctx, `
		INSERT INTO semantic_claims (
			claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
			object_kind, literal_kind, literal_value, polarity, valid_from, valid_to, lifecycle, created_operation_id, transaction_time
		) VALUES (?, ?, ?, ?, ?, ?, 'literal', ?, ?, ?, ?, ?, 'active', ?, ?)
	`, claim.ID, proposal.Scope.ID, claim.SubjectEntityID, claim.Predicate.ID, claim.Predicate.Token,
		claim.Predicate.Version, claim.Object.Literal.Kind, claim.Object.Literal.Value, claim.Polarity,
		semanticTimeArgument(claim.ValidTime.From), semanticTimeArgument(claim.ValidTime.To),
		proposal.OperationID, formatSemanticTime(transactionTime))
	return err
}

type visibleCorrection struct {
	Mode     memory.CorrectionMode
	OldAfter memory.ValidTime
}

type semanticInspectionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) InspectClaims(ctx context.Context, scope memory.ScopeContext, query memory.ClaimQuery) (memory.ClaimsInspection, error) {
	return s.inspectClaimsAtScope(ctx, scope, false, query)
}

func (s *Store) inspectClaimsAtScope(ctx context.Context, scope memory.ScopeContext, useSessionScope bool, query memory.ClaimQuery) (memory.ClaimsInspection, error) {
	return s.inspectClaimsAtScopeCore(ctx, scope, useSessionScope, query, false)
}

func (s *Store) inspectClaimsAtScopeCore(
	ctx context.Context,
	scope memory.ScopeContext,
	useSessionScope bool,
	query memory.ClaimQuery,
	includeOutsideValidTime bool,
) (memory.ClaimsInspection, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.ClaimsInspection{}, err
	}
	defer tx.Rollback()
	result, err := s.inspectClaimsSnapshot(ctx, tx, scope, useSessionScope, query, includeOutsideValidTime)
	if err != nil {
		return memory.ClaimsInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return memory.ClaimsInspection{}, err
	}
	return result, nil
}

func (s *Store) inspectClaimsSnapshot(
	ctx context.Context,
	queryer semanticInspectionQueryer,
	scope memory.ScopeContext,
	useSessionScope bool,
	query memory.ClaimQuery,
	includeOutsideValidTime bool,
) (memory.ClaimsInspection, error) {
	if err := validateSessionScope(ctx, queryer, scope); err != nil {
		return memory.ClaimsInspection{}, err
	}
	captured := s.now().UTC()
	validAt, asKnownAt := captured, captured
	if query.ValidAt != nil {
		validAt = query.ValidAt.UTC()
	}
	if query.AsKnownAt != nil {
		asKnownAt = query.AsKnownAt.UTC()
	} else {
		var latest sql.NullString
		if err := queryer.QueryRowContext(ctx, `SELECT MAX(transaction_time) FROM semantic_operations`).Scan(&latest); err != nil {
			return memory.ClaimsInspection{}, err
		}
		if latest.Valid {
			latestAccepted, err := parseSemanticTime(latest.String)
			if err != nil {
				return memory.ClaimsInspection{}, err
			}
			if latestAccepted.After(asKnownAt) {
				asKnownAt = latestAccepted
			}
		}
	}
	result := memory.ClaimsInspection{ValidAt: validAt, AsKnownAt: asKnownAt}
	keys := allowedSemanticReadScopeKeys(scope)
	if useSessionScope {
		keys = []string{targetScopeKey(scope, true)}
	}
	allowed := make(map[string]struct{}, len(allowedSemanticReadScopeKeys(scope)))
	for _, key := range allowedSemanticReadScopeKeys(scope) {
		allowed[key] = struct{}{}
	}
	targetKey := targetScopeKey(scope, useSessionScope)
	for _, key := range keys {
		part, found, err := s.inspectClaimsInScope(ctx, queryer, key, validAt, asKnownAt, includeOutsideValidTime, allowed)
		if err != nil {
			return result, err
		}
		if !found {
			if key == targetKey {
				result.Scope.Key = key
			}
			continue
		}
		result.Scopes = append(result.Scopes, part.Scope)
		result.ScopeRevisions = append(result.ScopeRevisions, memory.ScopeRevision{ScopeKey: part.Scope.Key, Revision: part.ScopeRevision})
		result.Claims = append(result.Claims, part.Claims...)
		if key == targetKey {
			result.Scope = part.Scope
			result.ScopeRevision = part.ScopeRevision
		}
	}
	if result.Scope.Key == "" {
		result.Scope.Key = targetKey
	}
	if query.Polarity != "" && query.Polarity != memory.PolarityAffirmed && query.Polarity != memory.PolarityDenied {
		return result, errors.New("semantic Claim polarity filter is invalid")
	}
	if query.PredicateToken != "" || query.Polarity != "" || query.SubjectEntityID != "" || query.ObjectEntityID != "" {
		filtered := result.Claims[:0]
		for _, claim := range result.Claims {
			if query.PredicateToken != "" && claim.Predicate.Token != query.PredicateToken {
				continue
			}
			if query.Polarity != "" && claim.Polarity != query.Polarity {
				continue
			}
			if query.SubjectEntityID != "" && claim.SubjectEntityID != query.SubjectEntityID {
				continue
			}
			if query.ObjectEntityID != "" && claim.Object.EntityID != query.ObjectEntityID {
				continue
			}
			filtered = append(filtered, claim)
		}
		result.Claims = filtered
	}
	result.AllowedScopes = append([]string(nil), keys...)
	return result, nil
}

func allowedSemanticReadScopeKeys(scope memory.ScopeContext) []string {
	keys := []string{"global"}
	contextKey := scopeKeyForContext(scope)
	if contextKey != "global" {
		keys = append(keys, contextKey)
	}
	keys = append(keys, "session:"+string(scope.SessionID))
	return keys
}

func (s *Store) inspectClaimsInScope(
	ctx context.Context,
	queryer semanticInspectionQueryer,
	key string,
	validAt time.Time,
	asKnownAt time.Time,
	includeOutsideValidTime bool,
	allowedSourceScopes map[string]struct{},
) (memory.ClaimsInspection, bool, error) {
	result := memory.ClaimsInspection{ValidAt: validAt, AsKnownAt: asKnownAt}
	var registry sql.NullString
	var currentRevision int64
	if err := queryer.QueryRowContext(ctx, `SELECT scope_id, scope_key, registry_id, revision FROM semantic_scopes WHERE scope_key = ?`, key).Scan(
		&result.Scope.ID, &result.Scope.Key, &registry, &currentRevision,
	); errors.Is(err, sql.ErrNoRows) {
		return result, false, nil
	} else if err != nil {
		return result, false, err
	}
	if registry.Valid {
		result.Scope.RegistryID = registry.String
	}
	if err := queryer.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(operation_scopes.resulting_revision), 0)
		FROM semantic_operation_scopes AS operation_scopes
		JOIN semantic_operations AS operations ON operations.operation_id = operation_scopes.operation_id
		WHERE operation_scopes.scope_id = ? AND operations.transaction_time <= ?
	`, result.Scope.ID, formatSemanticTime(asKnownAt)).Scan(&result.ScopeRevision); err != nil {
		return result, false, err
	}
	result.Scope.Revision = result.ScopeRevision
	rows, err := queryer.QueryContext(ctx, `
		SELECT claims.claim_id FROM semantic_claims AS claims
		WHERE claims.scope_id = ? AND claims.transaction_time <= ?
		ORDER BY claims.claim_id
	`, result.Scope.ID, formatSemanticTime(asKnownAt))
	if err != nil {
		return result, false, err
	}
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, false, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return result, false, err
	}
	for _, id := range ids {
		claim, err := loadSemanticClaim(ctx, queryer, id)
		if err != nil {
			return result, false, err
		}
		lifecycle, err := loadSemanticLifecycleAt(ctx, queryer, "claim", id, asKnownAt)
		if err != nil || len(lifecycle) == 0 {
			return result, false, errors.New("semantic Claim has no accepted lifecycle")
		}
		effectiveValidTime := claim.ValidTime
		latest := lifecycle[len(lifecycle)-1].State
		correction, found, err := loadVisibleCorrection(ctx, queryer, id, asKnownAt)
		if err != nil {
			return result, false, err
		}
		if found {
			if correction.Mode == memory.CorrectionError {
				continue
			}
			effectiveValidTime = correction.OldAfter
		} else if latest != memory.SemanticStateActive {
			continue
		}
		if !includeOutsideValidTime && !validTimeContains(effectiveValidTime, validAt) {
			continue
		}
		sources, err := loadEligibleSourcesAt(ctx, queryer, id, asKnownAt)
		if err != nil {
			return result, false, err
		}
		if len(sources) == 0 {
			continue
		}
		for index := range sources {
			if _, allowed := allowedSourceScopes[sources[index].ScopeKey]; !allowed {
				sources[index].Evidence = ""
			}
		}
		inspection := memory.ClaimInspection{
			SemanticClaim: claim, Scope: result.Scope, Sources: sources, Lifecycle: lifecycle,
			EffectiveValidTime: effectiveValidTime,
		}
		inspection.Subject, err = loadSemanticEntityForInspection(ctx, queryer, claim.SubjectEntityID)
		if err != nil {
			return result, false, err
		}
		if claim.Object.EntityID != "" {
			entity, err := loadSemanticEntityForInspection(ctx, queryer, claim.Object.EntityID)
			if err != nil {
				return result, false, err
			}
			inspection.ObjectEntity = &entity
		}
		result.Claims = append(result.Claims, inspection)
	}
	return result, true, nil
}

func loadSemanticLifecycleAt(ctx context.Context, queryer semanticInspectionQueryer, objectKind string, objectID memory.SemanticID, asKnownAt time.Time) ([]memory.SemanticState, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT state, operation_id, scope_revision, transaction_time
		FROM semantic_state_events
		WHERE object_kind = ? AND object_id = ? AND transaction_time <= ?
		ORDER BY transaction_time, scope_revision, operation_id, state
	`, objectKind, objectID, formatSemanticTime(asKnownAt))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []memory.SemanticState
	for rows.Next() {
		var state memory.SemanticState
		var transactionTime string
		if err := rows.Scan(&state.State, &state.OperationID, &state.ScopeRevision, &transactionTime); err != nil {
			return nil, err
		}
		state.TransactionTime, err = parseSemanticTime(transactionTime)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func loadVisibleCorrection(ctx context.Context, queryer semanticInspectionQueryer, oldClaimID memory.SemanticID, asKnownAt time.Time) (visibleCorrection, bool, error) {
	var correction visibleCorrection
	var from, to sql.NullString
	err := queryer.QueryRowContext(ctx, `
		SELECT mode, old_effective_from, old_effective_to
		FROM semantic_claim_corrections
		WHERE old_claim_id = ? AND transaction_time <= ?
		ORDER BY transaction_time DESC, scope_revision DESC, operation_id DESC LIMIT 1
	`, oldClaimID, formatSemanticTime(asKnownAt)).Scan(&correction.Mode, &from, &to)
	if errors.Is(err, sql.ErrNoRows) {
		return correction, false, nil
	}
	if err != nil {
		return correction, false, err
	}
	if from.Valid {
		value, err := parseSemanticTime(from.String)
		if err != nil {
			return correction, false, err
		}
		correction.OldAfter.From = &value
	}
	if to.Valid {
		value, err := parseSemanticTime(to.String)
		if err != nil {
			return correction, false, err
		}
		correction.OldAfter.To = &value
	}
	return correction, true, nil
}

func loadEligibleSourcesAt(ctx context.Context, queryer semanticInspectionQueryer, claimID memory.SemanticID, asKnownAt time.Time) ([]memory.SemanticSource, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT sources.source_link_id, sources.created_operation_id, sources.event_id,
		       sources.source_session_id, sources.source_scope_key, sources.event_part,
		       sources.locator_kind, sources.locator_value, sources.evidence_sha256,
		       sources.source_actor, sources.source_type, sources.authority, sources.observed_at,
		       sources.eligibility, events.content
		FROM semantic_source_links AS sources
		JOIN semantic_operations AS operations ON operations.operation_id = sources.created_operation_id
		JOIN events ON events.id = sources.event_id
		WHERE sources.claim_id = ? AND operations.transaction_time <= ?
		  AND (SELECT state FROM semantic_state_events AS source_states
		       WHERE source_states.object_kind = 'source_link'
		         AND source_states.object_id = sources.source_link_id
		         AND source_states.transaction_time <= ?
		       ORDER BY source_states.transaction_time DESC, source_states.scope_revision DESC,
		                source_states.operation_id DESC, source_states.state DESC LIMIT 1) = 'eligible'
		ORDER BY sources.source_link_id
	`, claimID, formatSemanticTime(asKnownAt), formatSemanticTime(asKnownAt))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []memory.SemanticSource
	for rows.Next() {
		var source memory.SemanticSource
		if err := rows.Scan(&source.ID, &source.OperationID, &source.EventID, &source.SessionID, &source.ScopeKey,
			&source.EventPart, &source.LocatorKind, &source.LocatorValue, &source.EvidenceSHA256,
			&source.Actor, &source.SourceType, &source.Authority, &source.ObservedAt, &source.Eligibility,
			&source.Evidence); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func loadSemanticEntityForInspection(ctx context.Context, queryer semanticInspectionQueryer, id memory.SemanticID) (memory.SemanticEntity, error) {
	var entity memory.SemanticEntity
	err := queryer.QueryRowContext(ctx, `
		SELECT entities.entity_id, scopes.scope_key, entities.canonical_name, entities.entity_type,
		       COALESCE(entities.anchor_kind, '')
		FROM semantic_entities AS entities
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.entity_id = ?
	`, id).Scan(&entity.ID, &entity.ScopeKey, &entity.CanonicalName, &entity.EntityType, &entity.AnchorKind)
	return entity, err
}

// InspectLiteralClaims preserves the focused current-read surface while using
// the shared bitemporal query implementation.
func (s *Store) InspectLiteralClaims(ctx context.Context, scope memory.ScopeContext) (memory.LiteralClaimsInspection, error) {
	claims, diagnostics, err := s.inspectFocusedClaimsAtScope(ctx, scope, false)
	if err != nil {
		return memory.LiteralClaimsInspection{}, err
	}
	result := memory.LiteralClaimsInspection{
		Scope: claims.Scope, Scopes: claims.Scopes, ScopeRevisions: claims.ScopeRevisions,
		ScopeRevision: claims.ScopeRevision, EffectiveAt: claims.ValidAt,
		ValidAt: claims.ValidAt, AsKnownAt: claims.AsKnownAt,
	}
	result.Claims = literalClaimsFromInspection(claims)
	diagnosticClaims := literalClaimsFromInspection(diagnostics)
	result.Warnings = literalConflictWarnings(diagnosticClaims)
	result.ConflictClaims = selectLiteralConflictClaims(diagnosticClaims, result.Warnings)
	return result, nil
}

func literalClaimsFromInspection(claims memory.ClaimsInspection) []memory.LiteralClaimInspection {
	var result []memory.LiteralClaimInspection
	for _, inspected := range claims.Claims {
		if inspected.Object.Literal == nil {
			continue
		}
		claim := memory.LiteralClaimInspection{
			ID: inspected.ID, Scope: inspected.Scope, Subject: inspected.Subject, Predicate: inspected.Predicate,
			Literal: *inspected.Object.Literal, Polarity: inspected.Polarity, ValidTime: inspected.EffectiveValidTime,
			OperationID: inspected.CreatedOperationID, TransactionTime: inspected.TransactionTime,
			EffectiveAt: claims.ValidAt, Sources: inspected.Sources, Lifecycle: inspected.Lifecycle,
		}
		claim.Source = claim.Sources[0]
		result = append(result, claim)
	}
	return result
}

func (s *Store) InspectEntityClaims(ctx context.Context, scope memory.ScopeContext) (memory.EntityClaimsInspection, error) {
	return s.InspectEntityClaimsAtScope(ctx, scope, false)
}

func (s *Store) InspectEntityClaimsAtScope(ctx context.Context, scope memory.ScopeContext, useSessionScope bool) (memory.EntityClaimsInspection, error) {
	claims, diagnostics, err := s.inspectFocusedClaimsAtScope(ctx, scope, useSessionScope)
	if err != nil {
		return memory.EntityClaimsInspection{}, err
	}
	result := memory.EntityClaimsInspection{
		Scope: claims.Scope, Scopes: claims.Scopes, ScopeRevisions: claims.ScopeRevisions,
		ScopeRevision: claims.ScopeRevision, EffectiveAt: claims.ValidAt,
		ValidAt: claims.ValidAt, AsKnownAt: claims.AsKnownAt,
	}
	result.Claims = entityClaimsFromInspection(claims)
	diagnosticClaims := entityClaimsFromInspection(diagnostics)
	result.Warnings = entityConflictWarnings(diagnosticClaims)
	result.ConflictClaims = selectEntityConflictClaims(diagnosticClaims, result.Warnings)
	return result, nil
}

func (s *Store) inspectFocusedClaimsAtScope(
	ctx context.Context,
	scope memory.ScopeContext,
	useSessionScope bool,
) (memory.ClaimsInspection, memory.ClaimsInspection, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.ClaimsInspection{}, memory.ClaimsInspection{}, err
	}
	defer tx.Rollback()
	claims, diagnostics, err := s.inspectFocusedClaimsSnapshot(ctx, tx, scope, useSessionScope)
	if err != nil {
		return memory.ClaimsInspection{}, memory.ClaimsInspection{}, err
	}
	if err := tx.Commit(); err != nil {
		return memory.ClaimsInspection{}, memory.ClaimsInspection{}, err
	}
	return claims, diagnostics, nil
}

func (s *Store) inspectFocusedClaimsSnapshot(
	ctx context.Context,
	queryer semanticInspectionQueryer,
	scope memory.ScopeContext,
	useSessionScope bool,
) (memory.ClaimsInspection, memory.ClaimsInspection, error) {
	claims, err := s.inspectClaimsSnapshot(ctx, queryer, scope, useSessionScope, memory.ClaimQuery{}, false)
	if err != nil {
		return memory.ClaimsInspection{}, memory.ClaimsInspection{}, err
	}
	diagnostics, err := s.inspectClaimsSnapshot(ctx, queryer, scope, useSessionScope, memory.ClaimQuery{
		ValidAt: &claims.ValidAt, AsKnownAt: &claims.AsKnownAt,
	}, true)
	if err != nil {
		return memory.ClaimsInspection{}, memory.ClaimsInspection{}, err
	}
	return claims, diagnostics, nil
}

func entityClaimsFromInspection(claims memory.ClaimsInspection) []memory.EntityClaimInspection {
	var result []memory.EntityClaimInspection
	for _, inspected := range claims.Claims {
		if inspected.Object.EntityID == "" || inspected.ObjectEntity == nil {
			continue
		}
		claim := memory.EntityClaimInspection{
			Claim: memory.SemanticEntityClaim{
				ID: inspected.ID, ScopeKey: inspected.ScopeKey, SubjectEntityID: inspected.SubjectEntityID,
				PredicateID: inspected.Predicate.ID, PredicateToken: inspected.Predicate.Token,
				PredicateVersion: inspected.Predicate.Version, ObjectEntityID: inspected.Object.EntityID,
				Polarity: inspected.Polarity, ValidTime: inspected.EffectiveValidTime,
			},
			Scope: inspected.Scope, Subject: inspected.Subject, Object: *inspected.ObjectEntity,
			Predicate: inspected.Predicate, OperationID: inspected.CreatedOperationID,
			TransactionTime: inspected.TransactionTime, Sources: inspected.Sources, Lifecycle: inspected.Lifecycle,
		}
		result = append(result, claim)
	}
	return result
}
