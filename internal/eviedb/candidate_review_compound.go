package eviedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/davidadel66/evie/internal/memory"
	"sort"
	"strings"
	"time"
)

var ErrReviewTooLarge = errors.New("review_too_large")
var ErrReviewDependencies = errors.New("invalid_dependencies")

func reviewNativeVersion(c memory.OwnerCandidate) string {
	if candidateHasClock(c.Candidate) {
		return "owner-review-preview-v4"
	}
	if c.Candidate.Proposal.Temporal != nil {
		return "owner-review-preview-v3"
	}
	if c.Candidate.Proposal.Identity != nil {
		return "owner-review-preview-v2"
	}
	return "owner-review-preview-v1"
}
func reviewMemberPreview(p memory.ReviewPreview, index int) memory.ReviewPreview {
	if index < 0 || index >= len(p.Candidates) || p.Effect != nil && (index >= len(p.Effect.Members) || len(p.Effect.Members[index].Claims) != 1) {
		return memory.ReviewPreview{}
	}
	c := p.Candidates[index]
	// Native member validation consumes only the supported typed interpretation;
	// the v5 envelope separately binds the immutable edit's before/after lineage.
	c.Edit = nil
	c.Original = nil
	if c.Identity == nil && c.Temporal == nil {
		c.Ref.InterpretationRevision = 0
	}
	out := p
	out.BatchID = ""
	out.Candidates = []memory.OwnerCandidate{c}
	out.JobID = c.JobID
	out.GenerationID = c.GenerationID
	out.Version = reviewNativeVersion(c)
	out.Dependencies = nil
	if p.Effect != nil {
		var member memory.ReviewEffect
		_ = json.Unmarshal(compilerJSON(p.Effect.Members[index]), &member)
		member.Claims[0].Candidate = c.Ref
		out.Effect = &member
	}
	out.EffectSHA256, _, _ = ownerReviewEffectHash(out.Effect)
	out.SHA256, _, _ = ownerReviewPreviewHash(out)
	return out
}

func prepareReviewCompound(ctx context.Context, q reviewQuery, a OwnerReviewContext, candidates []memory.OwnerCandidate, dependencies []memory.ReviewDependency) (*memory.ReviewEffect, error) {
	var effect *memory.ReviewEffect
	for _, candidate := range candidates {
		member, err := prepareReviewEffects(ctx, q, a, []memory.OwnerCandidate{candidate})
		if err != nil {
			return nil, err
		}
		if effect == nil {
			effect = &memory.ReviewEffect{Version: "owner-review-effect-v5", OperationID: member.OperationID, Scope: member.Scope, Scopes: member.Scopes, PriorRevisions: member.PriorRevisions, Claims: []memory.ReviewClaimEffect{}, Members: []memory.ReviewEffect{}, Dependencies: dependencies}
		}
		old := member.OperationID
		member.OperationID = effect.OperationID
		for i := range member.Claims {
			item := &member.Claims[i]
			if item.Claim.CreatedOperationID == old {
				item.Claim.CreatedOperationID = effect.OperationID
			}
			for n := range item.Sources {
				if item.Sources[n].OperationID == old {
					item.Sources[n].OperationID = effect.OperationID
				}
			}
		}
		if member.Identity != nil {
			for i := range member.Identity.Aliases {
				member.Identity.Aliases[i].OperationID = effect.OperationID
			}
		}
		effect.Members = append(effect.Members, *member)
	}
	if effect == nil {
		return nil, ErrReviewDependencies
	}
	if err := bindReviewDependencies(candidates, effect, dependencies); err != nil {
		return nil, err
	}
	for _, member := range effect.Members {
		effect.Claims = append(effect.Claims, member.Claims...)
	}
	for i := range effect.Claims {
		warnings := combinedReviewWarnings(effect.Claims[i].Conflicts, compoundInternalWarnings(effect, effect.Claims[i]))
		effect.Claims[i].Conflicts = warnings
		effect.Members[i].Claims[0].Conflicts = warnings
	}
	if err := validateCompoundWrites(effect); err != nil {
		return nil, err
	}
	effect.Records = enumerateReviewRecords(effect)
	if countReviewRecords(effect) > 256 {
		return nil, ErrReviewTooLarge
	}
	return effect, nil
}
func dependencyField(item *memory.ReviewClaimEffect, field string) (*memory.SemanticEntity, *memory.SemanticPredicate) {
	switch field {
	case "subject":
		return &item.Subject, nil
	case "object":
		return item.ObjectEntity, nil
	case "predicate":
		return nil, &item.Predicate
	}
	return nil, nil
}
func bindReviewDependencies(candidates []memory.OwnerCandidate, effect *memory.ReviewEffect, deps []memory.ReviewDependency) error {
	if effect == nil || len(effect.Members) != len(candidates) {
		return ErrReviewDependencies
	}
	for _, member := range effect.Members {
		if len(member.Claims) != 1 {
			return ErrReviewDependencies
		}
	}
	positions := map[string]int{}
	for i, c := range candidates {
		if _, ok := positions[c.Ref.ID]; ok {
			return ErrReviewDependencies
		}
		positions[c.Ref.ID] = i
	}
	seen := map[string]bool{}
	for _, dep := range deps {
		to, toOK := positions[dep.CandidateID]
		from, fromOK := positions[dep.FromCandidateID]
		key := dep.CandidateID + "/" + dep.Field
		if !toOK || !fromOK || from >= to || seen[key] {
			return ErrReviewDependencies
		}
		seen[key] = true
		target := &effect.Members[to].Claims[0]
		provider := &effect.Members[from].Claims[0]
		entity, predicate := dependencyField(target, dep.Field)
		sourceEntity, sourcePredicate := dependencyField(provider, dep.FromField)
		if entity != nil && sourceEntity != nil {
			if !entity.Create || !sourceEntity.Create || entity.CanonicalName != sourceEntity.CanonicalName || entity.EntityType != sourceEntity.EntityType || entity.ScopeKey != sourceEntity.ScopeKey {
				return ErrReviewDependencies
			}
			old := entity.ID
			entity.ID = sourceEntity.ID
			if target.Claim.SubjectEntityID == old {
				target.Claim.SubjectEntityID = entity.ID
			}
			if target.Claim.Object.EntityID == old {
				target.Claim.Object.EntityID = entity.ID
			}
			if effect.Members[to].Identity == nil {
				return ErrReviewDependencies
			}
			for i := range effect.Members[to].Identity.Aliases {
				alias := &effect.Members[to].Identity.Aliases[i]
				if alias.EntityID == old {
					alias.EntityID = entity.ID
				}
			}
		} else if predicate != nil && sourcePredicate != nil {
			copy := *predicate
			copy.ID = sourcePredicate.ID
			if !predicate.Create || !sourcePredicate.Create || copy != *sourcePredicate {
				return ErrReviewDependencies
			}
			*predicate = *sourcePredicate
			target.Claim.Predicate = *predicate
		} else {
			return ErrReviewDependencies
		}
	}
	return nil
}
func validateCompoundWrites(effect *memory.ReviewEffect) error {
	predicates := map[string]memory.SemanticID{}
	claims := map[string]bool{}
	corrections := map[memory.SemanticID]bool{}
	entities := map[memory.SemanticID]memory.SemanticEntity{}
	for _, member := range effect.Members {
		item := member.Claims[0]
		if item.Predicate.Create {
			if id, ok := predicates[item.Predicate.Token]; ok && id != item.Predicate.ID {
				return ErrReviewDependencies
			}
			predicates[item.Predicate.Token] = item.Predicate.ID
		}
		for _, entity := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
			if entity != nil && entity.Create {
				if prior, ok := entities[entity.ID]; ok && prior != *entity {
					return ErrReviewDependencies
				}
				entities[entity.ID] = *entity
			}
		}
		key := reviewClaimMeaning(item.Claim)
		if claims[key] {
			return ErrReviewDependencies
		}
		claims[key] = true
		if member.Correction != nil {
			old := member.Correction.OldClaim.ID
			if corrections[old] {
				return ErrReviewDependencies
			}
			corrections[old] = true
		}
	}
	for _, item := range effect.Claims {
		if corrections[item.Claim.ID] {
			return ErrReviewDependencies
		}
	}
	return nil
}
func reviewClaimMeaning(c memory.SemanticClaim) string {
	return string(compilerJSON(struct {
		Scope              string
		Subject, Predicate memory.SemanticID
		Object             memory.ClaimObject
		Polarity           memory.ClaimPolarity
		Time               memory.ValidTime
	}{c.ScopeKey, c.SubjectEntityID, c.Predicate.ID, c.Object, c.Polarity, c.ValidTime}))
}
func countReviewRecords(effect *memory.ReviewEffect) int { return len(enumerateReviewRecords(effect)) }
func enumerateReviewRecords(effect *memory.ReviewEffect) []memory.ReviewEffectRecord {
	out := []memory.ReviewEffectRecord{}
	if effect == nil {
		return out
	}
	seen := map[string]bool{}
	add := func(kind string, id memory.SemanticID, create bool, state string) {
		key := kind + "/" + string(id)
		if seen[key] {
			return
		}
		seen[key] = true
		action := "reuse"
		if create {
			action = "create"
		}
		out = append(out, memory.ReviewEffectRecord{Kind: kind, ID: id, Action: action})
		if create && state != "" {
			out = append(out, memory.ReviewEffectRecord{Kind: "state_event", ID: id, Action: "initialize", AfterState: state})
		}
	}
	members := effect.Members
	if len(members) == 0 {
		members = []memory.ReviewEffect{*effect}
	}
	for _, member := range members {
		for _, item := range member.Claims {
			add("entity", item.Subject.ID, item.Subject.Create, "active")
			if item.ObjectEntity != nil {
				add("entity", item.ObjectEntity.ID, item.ObjectEntity.Create, "active")
			}
			add("predicate", item.Predicate.ID, item.Predicate.Create, "")
			add("claim", item.Claim.ID, item.Create, "active")
			for _, source := range item.Sources {
				add("source_link", source.ID, source.Create, "eligible")
			}
		}
		if member.Identity != nil {
			for _, alias := range member.Identity.Aliases {
				add("alias", alias.ID, alias.Create, "active")
			}
		}
		if member.Correction != nil {
			old := member.Correction.OldClaim.ID
			add("correction", old, true, "")
			out = append(out, memory.ReviewEffectRecord{Kind: "state_event", ID: old, Action: "transition", BeforeState: string(member.Correction.OldState.State), AfterState: "superseded"})
		}
	}
	return out
}

func validateReviewCompoundEncoding(p memory.ReviewPreview) error {
	if p.Version != "owner-review-preview-v5" || len(p.Candidates) < 1 || len(p.Candidates) > 64 || p.Action != "accept" && p.Action != "reject" {
		return ErrReviewDependencies
	}
	if p.BatchID != "" && validateSemanticUUID(p.BatchID) != nil {
		return ErrReviewDependencies
	}
	seen := map[string]bool{}
	for _, c := range p.Candidates {
		if seen[c.Ref.ID] || len(c.Ref.ID) != 64 || c.Candidate.ID != c.Ref.ID || p.Action == "accept" && len(c.Candidate.Support) == 0 {
			return ErrReviewDependencies
		}
		seen[c.Ref.ID] = true
		if err := validateReviewEditEnvelope(c); err != nil {
			return err
		}
	}
	if p.Action == "reject" {
		if p.Effect != nil || len(p.Dependencies) != 0 {
			return ErrReviewDependencies
		}
		for i := range p.Candidates {
			if err := validateOwnerReviewEncoding(reviewMemberPreview(p, i)); err != nil {
				return err
			}
		}
		return nil
	}
	e := p.Effect
	if e == nil || e.Version != "owner-review-effect-v5" || e.Identity != nil || e.Correction != nil || len(e.Members) != len(p.Candidates) || len(e.Claims) != len(p.Candidates) || string(compilerJSON(e.Dependencies)) != string(compilerJSON(p.Dependencies)) {
		return ErrReviewDependencies
	}
	// Prove every indexed native-member shape before dependency traversal or
	// native envelope validation. Retained JSON is untrusted during replay.
	for _, member := range e.Members {
		if len(member.Claims) != 1 || len(member.Members) != 0 || len(member.Claims[0].Sources) == 0 {
			return ErrReviewDependencies
		}
	}
	// Apply the declared bindings again to a copy: already bound IDs must be an
	// exact fixed point, so an omitted/changed dependency cannot change meaning.
	var copy memory.ReviewEffect
	if json.Unmarshal(compilerJSON(e), &copy) != nil {
		return ErrReviewDependencies
	}
	if err := bindReviewDependencies(p.Candidates, &copy, p.Dependencies); err != nil {
		return err
	}
	if string(compilerJSON(copy)) != string(compilerJSON(e)) {
		return ErrReviewDependencies
	}
	byID := map[string]int{}
	for i, c := range p.Candidates {
		byID[c.Ref.ID] = i
	}
	shared := map[string]int{}
	for i, member := range e.Members {
		if err := validateReviewEditEnvelope(p.Candidates[i]); err != nil {
			return err
		}
		if member.OperationID != e.OperationID || member.Scope != e.Scope || string(compilerJSON(member.Scopes)) != string(compilerJSON(e.Scopes)) || string(compilerJSON(member.PriorRevisions)) != string(compilerJSON(e.PriorRevisions)) || len(member.Members) != 0 || len(member.Claims) != 1 || len(member.Dependencies) != 0 || len(member.Records) != 0 || string(compilerJSON(member.Claims[0])) != string(compilerJSON(e.Claims[i])) {
			return ErrReviewDependencies
		}
		for _, field := range []string{"subject", "object", "predicate"} {
			entity, predicate := dependencyField(&member.Claims[0], field)
			key := ""
			if entity != nil && entity.Create {
				key = "entity/" + string(entity.ID)
			}
			if predicate != nil && predicate.Create {
				key = "predicate/" + string(predicate.ID)
			}
			if key == "" {
				continue
			}
			if earlier, ok := shared[key]; ok && earlier != i {
				found := false
				for _, dep := range p.Dependencies {
					if dep.CandidateID == p.Candidates[i].Ref.ID && dep.Field == field && byID[dep.FromCandidateID] < i {
						found = true
					}
				}
				if !found {
					return ErrReviewDependencies
				}
			} else {
				shared[key] = i
			}
		}
		if err := validateOwnerReviewOperation(memory.OwnerReviewOperation{SchemaVersion: 6, Kind: "owner_candidate_review", OperationID: e.OperationID, IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000001", Actor: memory.SemanticActorOwner, SessionID: p.Candidates[i].Candidate.Support[0].SessionID, SourceEventID: p.Candidates[i].Candidate.Support[0].Locator.EventID, Preview: reviewMemberPreview(p, i), AuditID: p.ID}); err != nil {
			return err
		}
	}
	if err := validateCompoundWrites(e); err != nil {
		return err
	}
	if string(compilerJSON(e.Records)) != string(compilerJSON(enumerateReviewRecords(e))) {
		return ErrReviewDependencies
	}
	if countReviewRecords(e) > 256 {
		return ErrReviewTooLarge
	}
	return nil
}
func validateOwnerCompoundOperation(op memory.OwnerReviewOperation) error {
	if (op.Batch == nil) != (op.Preview.BatchID == "") || op.Batch != nil && op.Batch.PreviewID != op.Preview.BatchID {
		return ErrReviewDependencies
	}
	if op.SchemaVersion != 6 || op.Kind != "owner_candidate_review" || op.Actor != memory.SemanticActorOwner || !reviewDeliveryValid(op.IdempotencyKey) || op.Preview.Action != "accept" || op.Preview.Effect == nil || op.OperationID != op.Preview.Effect.OperationID {
		return errors.New("invalid compound review operation")
	}
	if b := op.Batch; b != nil {
		digest, err := hex.DecodeString(strings.TrimPrefix(b.PreviewSHA256, "sha256:"))
		if err != nil || len(digest) != 32 || !strings.HasPrefix(b.PreviewSHA256, "sha256:") || b.PreviewSHA256 != strings.ToLower(b.PreviewSHA256) || len(b.GroupID) < 1 || len(b.GroupID) > 64 || !reviewBatchLabel(b.GroupID) || b.GroupIndex < 0 || b.GroupIndex >= 20 || len(b.PriorGroups) > b.GroupIndex {
			return ErrReviewDependencies
		}
	}
	if err := validateOwnerReviewEncoding(op.Preview); err != nil {
		return err
	}
	for _, id := range []string{string(op.OperationID), op.AuditID, op.Preview.ID, string(op.SessionID), string(op.SourceEventID)} {
		if validateSemanticUUID(id) != nil {
			return ErrReviewDependencies
		}
	}
	first := op.Preview.Candidates[0].Candidate.Support[0]
	if first.SessionID != op.SessionID || first.Locator.EventID != op.SourceEventID {
		return ErrReviewInvalidSource
	}
	return nil
}

// workingReviewEffect derives only this delivery's already committed group
// advances. Original preview bytes and approval hashes are never rewritten.
func workingReviewEffect(ctx context.Context, q reviewQuery, op memory.OwnerReviewOperation) (*memory.ReviewEffect, error) {
	var effect memory.ReviewEffect
	if err := json.Unmarshal(compilerJSON(op.Preview.Effect), &effect); err != nil {
		return nil, err
	}
	if op.Batch == nil {
		return &effect, nil
	}
	b := op.Batch
	if validateSemanticUUID(b.PreviewID) != nil || len(strings.TrimPrefix(b.PreviewSHA256, "sha256:")) != 64 || b.GroupIndex < 0 || b.GroupIndex >= 20 || len(b.PriorGroups) > b.GroupIndex {
		return nil, ErrReviewDependencies
	}
	revisions := map[string]int64{}
	for _, r := range effect.PriorRevisions {
		revisions[r.ScopeKey] = r.Revision
	}
	seen := map[memory.SemanticID]bool{}
	lastIndex := -1
	for _, prior := range b.PriorGroups {
		if seen[prior.OperationID] {
			return nil, ErrReviewDependencies
		}
		seen[prior.OperationID] = true
		var raw, result []byte
		var previous memory.OwnerReviewOperation
		if err := q.QueryRowContext(ctx, `SELECT prepared_proposal_json,result_json FROM semantic_operations WHERE operation_id=? AND schema_version=6`, prior.OperationID).Scan(&raw, &result); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &previous) != nil || previous.Batch == nil || previous.Batch.PreviewID != b.PreviewID || previous.Batch.PreviewSHA256 != b.PreviewSHA256 || previous.Batch.GroupIndex <= lastIndex || previous.Batch.GroupIndex >= b.GroupIndex || string(result) != string(compilerJSON(prior)) {
			return nil, ErrReviewDependencies
		}
		lastIndex = previous.Batch.GroupIndex
		for _, r := range prior.ResultingRevisions {
			before, ok := revisions[r.ScopeKey]
			if !ok {
				return nil, ErrReviewDependencies
			}
			delta := int64(0)
			if r.ScopeKey == previous.Preview.Effect.Scope.Key || r.ScopeKey == "global" && reviewWritesGlobal(previous.Preview.Effect) {
				delta = 1
			}
			if r.Revision != before+delta {
				return nil, ErrReviewDependencies
			}
			revisions[r.ScopeKey] = r.Revision
		}
	}
	for i := range effect.Scopes {
		effect.Scopes[i].Revision = revisions[effect.Scopes[i].Key]
	}
	for i := range effect.PriorRevisions {
		effect.PriorRevisions[i].Revision = revisions[effect.PriorRevisions[i].ScopeKey]
	}
	effect.Scope.Revision = revisions[effect.Scope.Key]
	for i := range effect.Members {
		effect.Members[i].Scope = effect.Scope
		effect.Members[i].Scopes = effect.Scopes
		effect.Members[i].PriorRevisions = effect.PriorRevisions
	}
	return &effect, nil
}
func (s *Store) applyOwnerCompoundOperation(ctx context.Context, conn *sql.Conn, op memory.OwnerReviewOperation, clock time.Time) (memory.OwnerReviewOperationResult, error) {
	var result memory.OwnerReviewOperationResult
	if err := validateOwnerCompoundOperation(op); err != nil {
		return result, err
	}
	effect, err := workingReviewEffect(ctx, conn, op)
	if err != nil {
		return result, err
	}
	writer := reviewWriter{conn}
	byKey, err := validateSemanticScopeVector(ctx, writer, effect.Scopes, effect.PriorRevisions, clock)
	if err != nil {
		if !compilerDataFailure(err) {
			return result, err
		}
		return result, ErrReviewStale
	}
	for _, member := range effect.Members {
		item := member.Claims[0]
		dbWarnings, err := reviewClaimConflicts(ctx, conn, effect.Scope.ID, item.Claim)
		if err != nil {
			return result, err
		}
		expected := combinedReviewWarnings(dbWarnings, compoundInternalWarnings(effect, item))
		if string(compilerJSON(expected)) != string(compilerJSON(item.Conflicts)) {
			return result, ErrReviewStale
		}
		item.Conflicts = dbWarnings
		if err = validateReviewClaimEffects(ctx, conn, &member, item); err != nil {
			return result, err
		}
		if err = validateReviewCorrectionCurrent(ctx, conn, &member); err != nil {
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
	effectHash, _, err := ownerReviewEffectHash(op.Preview.Effect)
	if err != nil {
		return result, err
	}
	if err = recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{SchemaVersion: 6, OperationID: op.OperationID, Kind: op.Kind, IdempotencyKey: op.IdempotencyKey, Actor: op.Actor, SessionID: op.SessionID, TargetScopeID: effect.Scope.ID, SourceEventID: op.SourceEventID, ProposalHash: proposalHash, EffectHash: effectHash, ProposalJSON: proposalJSON, PreparedJSON: compilerJSON(op), ResultJSON: compilerJSON(result), TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey}); err != nil {
		return result, err
	}
	written := map[memory.SemanticID]bool{}
	for _, member := range effect.Members {
		// All duplicates here were explicitly bound and validated above. Each
		// sourced Alias remains distinct; shared Entity/Predicate rows are written once.
		item := &member.Claims[0]
		if item.Predicate.Create {
			if written[item.Predicate.ID] {
				item.Predicate.Create = false
			}
			written[item.Predicate.ID] = true
		}
		for _, entity := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
			if entity != nil && entity.Create {
				if written[entity.ID] {
					entity.Create = false
				}
				written[entity.ID] = true
			}
		}
		if err = applyReviewIdentityEffects(ctx, conn, &member, byKey, now); err != nil {
			return result, err
		}
	}
	for _, member := range effect.Members {
		item := member.Claims[0]
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
		if err = applyReviewCorrectionEffect(ctx, conn, &member, now); err != nil {
			return result, err
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
		n, err := updated.RowsAffected()
		if err != nil || n != 1 {
			return result, ErrReviewStale
		}
	}
	return result, nil
}

// Group independence concerns object identities and equality/conflict reads,
// not shared registry revision rows. Scope revisions are handled by the batch.
func reviewGroupAccess(p memory.ReviewPreview) (map[string]bool, map[string]bool) {
	reads, writes := map[string]bool{}, map[string]bool{}
	if p.Effect == nil {
		return reads, writes
	}
	for _, member := range p.Effect.Members {
		item := member.Claims[0]
		for _, entity := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
			if entity != nil {
				key := "entity/" + string(entity.ID)
				reads[key] = true
				if entity.Create {
					writes[key] = true
				}
			}
		}
		predicate := "predicate/" + string(item.Predicate.ID)
		reads[predicate] = true
		if item.Predicate.Create {
			writes[predicate] = true
			writes["token/"+item.Predicate.Token] = true
		}
		writes["claim/"+string(item.Claim.ID)] = true
		writes["meaning/"+reviewClaimMeaning(item.Claim)] = true
		for _, source := range item.Sources {
			writes["source/"+string(source.ID)] = true
		}
		for _, conflict := range item.Conflicts {
			for _, id := range conflict.ClaimIDs {
				reads["claim/"+string(id)] = true
			}
		}
		if member.Correction != nil {
			writes["claim/"+string(member.Correction.OldClaim.ID)] = true
		}
	}
	return reads, writes
}
func validateReviewGroupIndependence(groups []memory.ReviewBatchGroup) error {
	for i, g := range groups {
		r, w := reviewGroupAccess(g.Preview)
		for n := 0; n < i; n++ {
			pr, pw := reviewGroupAccess(groups[n].Preview)
			if g.Preview.Effect != nil && groups[n].Preview.Effect != nil {
				for _, left := range g.Preview.Effect.Claims {
					for _, right := range groups[n].Preview.Effect.Claims {
						if len(classifyClaimConflicts([]claimConflictCandidate{reviewConflictCandidate(left.Claim), reviewConflictCandidate(right.Claim)})) != 0 {
							return ErrReviewDependencies
						}
					}
				}
			}
			for key := range w {
				if pw[key] || pr[key] {
					return ErrReviewDependencies
				}
			}
			for key := range r {
				if pw[key] {
					return ErrReviewDependencies
				}
			}
		}
	}
	return nil
}
func canonicalReviewDependencies(deps []memory.ReviewDependency) []memory.ReviewDependency {
	out := append([]memory.ReviewDependency{}, deps...)
	sort.Slice(out, func(i, j int) bool { return string(compilerJSON(out[i])) < string(compilerJSON(out[j])) })
	return out
}

func reviewConflictCandidate(claim memory.SemanticClaim) claimConflictCandidate {
	return claimConflictCandidate{ID: claim.ID, SubjectID: claim.SubjectEntityID, PredicateID: claim.Predicate.ID, PredicateToken: claim.Predicate.Token, ObjectKey: string(compilerJSON(claim.Object)), Polarity: claim.Polarity, ValidTime: claim.ValidTime, Cardinality: claim.Predicate.Cardinality}
}
func compoundInternalWarnings(effect *memory.ReviewEffect, item memory.ReviewClaimEffect) []memory.ClaimConflictWarning {
	values := []claimConflictCandidate{reviewConflictCandidate(item.Claim)}
	for _, other := range effect.Claims {
		if other.Claim.ID != item.Claim.ID {
			values = append(values, reviewConflictCandidate(other.Claim))
		}
	}
	result := []memory.ClaimConflictWarning{}
	for _, warning := range classifyClaimConflicts(values) {
		if containsSemanticID(warning.ClaimIDs, item.Claim.ID) {
			result = append(result, warning)
		}
	}
	return result
}
func combinedReviewWarnings(a, b []memory.ClaimConflictWarning) []memory.ClaimConflictWarning {
	seen := map[string]bool{}
	out := []memory.ClaimConflictWarning{}
	for _, warnings := range [][]memory.ClaimConflictWarning{a, b} {
		for _, warning := range warnings {
			key := string(compilerJSON(warning))
			if !seen[key] {
				out = append(out, warning)
				seen[key] = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(compilerJSON(out[i])) < string(compilerJSON(out[j])) })
	return out
}

// Replay preflight has no database handle. Derive its scope envelope from the
// frozen prior-group receipts; workingReviewEffect later verifies each receipt
// against that preceding operation in the rebuilt canonical stream.
func reviewReplayEffect(op memory.OwnerReviewOperation) (*memory.ReviewEffect, error) {
	var effect memory.ReviewEffect
	if err := json.Unmarshal(compilerJSON(op.Preview.Effect), &effect); err != nil {
		return nil, err
	}
	if op.Batch == nil {
		return &effect, nil
	}
	b := op.Batch
	if b.GroupIndex < 0 || b.GroupIndex >= 20 || len(b.PriorGroups) > b.GroupIndex {
		return nil, ErrReviewDependencies
	}
	revisions := map[string]int64{}
	for _, r := range effect.PriorRevisions {
		revisions[r.ScopeKey] = r.Revision
	}
	seen := map[memory.SemanticID]bool{}
	for _, prior := range b.PriorGroups {
		if seen[prior.OperationID] || len(prior.ResultingRevisions) != len(effect.PriorRevisions) || validateSemanticUUID(string(prior.OperationID)) != nil {
			return nil, ErrReviewDependencies
		}
		seen[prior.OperationID] = true
		for i, r := range prior.ResultingRevisions {
			before, ok := revisions[r.ScopeKey]
			if !ok || r.ScopeKey != effect.PriorRevisions[i].ScopeKey || r.Revision < before || r.Revision > before+1 || r.ScopeKey == effect.Scope.Key && r.Revision != before+1 {
				return nil, ErrReviewDependencies
			}
			revisions[r.ScopeKey] = r.Revision
		}
	}
	for i := range effect.Scopes {
		effect.Scopes[i].Revision = revisions[effect.Scopes[i].Key]
	}
	for i := range effect.PriorRevisions {
		effect.PriorRevisions[i].Revision = revisions[effect.PriorRevisions[i].ScopeKey]
	}
	effect.Scope.Revision = revisions[effect.Scope.Key]
	return &effect, nil
}
