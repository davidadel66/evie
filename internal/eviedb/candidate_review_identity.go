package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

const reviewIdentitySchema = `
CREATE TABLE IF NOT EXISTS memory_review_identity_revisions (
 candidate_id TEXT NOT NULL REFERENCES memory_compiler_candidates(candidate_id),
 revision INTEGER NOT NULL CHECK(revision>0), envelope BLOB NOT NULL CHECK(length(envelope)<=131072),
 PRIMARY KEY(candidate_id,revision)
);
CREATE TRIGGER IF NOT EXISTS memory_review_identity_no_update BEFORE UPDATE ON memory_review_identity_revisions BEGIN SELECT RAISE(ABORT,'identity revisions are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_identity_no_delete BEFORE DELETE ON memory_review_identity_revisions BEGIN SELECT RAISE(ABORT,'identity revisions are immutable'); END;
CREATE INDEX IF NOT EXISTS memory_review_entity_name ON semantic_entities(canonical_name COLLATE NOCASE,scope_id,entity_id);
`

func loadReviewIdentityRevision(ctx context.Context, q reviewQuery, item *memory.OwnerCandidate) error {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT envelope FROM memory_review_identity_revisions WHERE candidate_id=? ORDER BY revision DESC LIMIT 1`, item.Ref.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var revision memory.ReviewIdentityRevision
	if json.Unmarshal(raw, &revision) != nil || revision.Revision < 1 || revision.ReviewRevision > item.Ref.ReviewRevision || revision.Options.Candidate.ID != item.Ref.ID {
		return errors.New("invalid stored identity revision")
	}
	item.Ref.InterpretationRevision = revision.Revision
	if !item.Redacted {
		item.Identity = &revision
	}
	return nil
}

func (s *Store) OwnerCandidateIdentityOptions(ctx context.Context, a OwnerReviewContext, ref memory.CandidateRef) (memory.ReviewIdentityOptions, error) {
	var result memory.ReviewIdentityOptions
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return result, err
	}
	item, err := loadReviewCandidate(ctx, tx, a, ref.ID, true)
	if err != nil {
		return result, err
	}
	if item.Ref != ref {
		return result, ErrReviewStale
	}
	if item.Candidate.ReviewState != "unresolved" {
		return result, ErrReviewResolved
	}
	result, err = reviewIdentityOptions(ctx, tx, a, item)
	if err != nil {
		return memory.ReviewIdentityOptions{}, err
	}
	return result, tx.Commit()
}

func reviewIdentityOptions(ctx context.Context, q reviewQuery, a OwnerReviewContext, item memory.OwnerCandidate) (memory.ReviewIdentityOptions, error) {
	out := memory.ReviewIdentityOptions{Candidate: item.Ref, ScopeKey: a.scope, ScopeRevisions: []memory.ScopeRevision{}, Subject: []memory.ReviewEntityAlternative{}, Object: []memory.ReviewEntityAlternative{}, Predicates: []memory.SemanticPredicate{}}
	proposal := item.Candidate.Proposal.Identity
	if proposal == nil {
		return out, errors.New("candidate has no unresolved identity proposal")
	}
	keys, err := reviewScopeKeys(ctx, q, a.scope)
	if err != nil {
		return out, err
	}
	for _, key := range keys {
		scope, err := loadSemanticScope(ctx, q, key)
		if err != nil {
			return out, err
		}
		out.ScopeRevisions = append(out.ScopeRevisions, memory.ScopeRevision{ScopeKey: key, Revision: scope.Revision})
	}
	for _, entry := range []struct {
		mention *memory.EntityMention
		target  *[]memory.ReviewEntityAlternative
	}{{proposal.Subject, &out.Subject}, {proposal.Object, &out.Object}} {
		if entry.mention != nil {
			*entry.target, err = reviewEntityAlternatives(ctx, q, keys, entry.mention.Name)
			if err != nil {
				return out, err
			}
		}
	}
	if proposal.Predicate != nil {
		rows, err := q.QueryContext(ctx, `SELECT predicate_id,token,version,CASE WHEN length(CAST(label AS BLOB))<=4096 THEN label ELSE '' END,object_constraint,cardinality FROM semantic_predicates WHERE token=? ORDER BY version,predicate_id LIMIT 33`, proposal.Predicate.Token)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var p memory.SemanticPredicate
			if err = rows.Scan(&p.ID, &p.Token, &p.Version, &p.Label, &p.ObjectConstraint, &p.Cardinality); err != nil {
				rows.Close()
				return out, err
			}
			if p.Label == "" || compilerHasSecret(p.Label) {
				rows.Close()
				return out, ErrReviewInvalidSource
			}
			out.Predicates = append(out.Predicates, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(out.Predicates) > 32 {
			return out, errors.New("Predicate alternatives exceed bound")
		}
	}
	out.SHA256 = reviewIdentityOptionsHash(out)
	if len(compilerJSON(out)) > 128*1024 {
		return memory.ReviewIdentityOptions{}, errors.New("identity alternatives exceed bound")
	}
	return out, nil
}

func reviewIdentityOptionsHash(out memory.ReviewIdentityOptions) string {
	out.SHA256 = ""
	return memory.CompilerHash(compilerJSON(struct {
		Domain  string                       `json:"domain"`
		Options memory.ReviewIdentityOptions `json:"options"`
	}{"owner-identity-options-v1", out}))
}

func reviewEntityAlternatives(ctx context.Context, q reviewQuery, keys []string, name string) ([]memory.ReviewEntityAlternative, error) {
	ids := map[memory.SemanticID]bool{}
	for _, key := range keys {
		rows, err := q.QueryContext(ctx, `SELECT e.entity_id FROM semantic_entities e JOIN semantic_scopes s ON s.scope_id=e.scope_id WHERE s.scope_key=? AND e.canonical_name=? COLLATE NOCASE
 UNION SELECT a.entity_id FROM semantic_aliases a JOIN semantic_scopes s ON s.scope_id=a.scope_id WHERE s.scope_key=? AND a.normalized_value=? AND a.lifecycle='active' ORDER BY entity_id LIMIT 33`, key, name, key, normalizeAlias(name))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id memory.SemanticID
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids[id] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(ids) > 32 {
		return nil, errors.New("identity alternatives exceed bound")
	}
	ordered := []memory.SemanticID{}
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	result := []memory.ReviewEntityAlternative{}
	for _, id := range ordered {
		var textBytes int
		if err := q.QueryRowContext(ctx, `SELECT length(CAST(canonical_name AS BLOB))+length(CAST(entity_type AS BLOB)) FROM semantic_entities WHERE entity_id=?`, id).Scan(&textBytes); err != nil {
			return nil, err
		}
		if textBytes > 4096 {
			return nil, errors.New("identity alternative text exceeds bound")
		}
		entity, err := reviewEntity(ctx, q, keys, id)
		if errors.Is(err, errReviewInactiveEntity) || errors.Is(err, ErrOwnerReviewUnauthorized) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if compilerHasSecret(entity.CanonicalName + " " + entity.EntityType) {
			continue
		}
		alt := memory.ReviewEntityAlternative{Entity: entity, Aliases: []memory.SemanticAlias{}, Context: []memory.SemanticClaim{}}
		rows, err := q.QueryContext(ctx, `SELECT alias_id,CASE WHEN length(CAST(value AS BLOB))<=1024 THEN value ELSE '' END,normalized_value,source_event_id,created_operation_id FROM semantic_aliases WHERE entity_id=? AND scope_id=(SELECT scope_id FROM semantic_scopes WHERE scope_key=?) AND normalized_value=? AND lifecycle='active' ORDER BY alias_id LIMIT 33`, id, entity.ScopeKey, normalizeAlias(name))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			alias := memory.SemanticAlias{EntityID: id, ScopeKey: entity.ScopeKey}
			if err = rows.Scan(&alias.ID, &alias.Value, &alias.NormalizedValue, &alias.SourceEventID, &alias.OperationID); err != nil {
				rows.Close()
				return nil, err
			}
			if alias.Value == "" {
				rows.Close()
				return nil, errors.New("identity Alias text exceeds bound")
			}
			alt.Aliases = append(alt.Aliases, alias)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(alt.Aliases) > 32 {
			return nil, errors.New("identity aliases exceed bound")
		}
		active := []memory.SemanticAlias{}
		for _, alias := range alt.Aliases {
			state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectAlias, alias.ID)
			if err != nil {
				return nil, err
			}
			if state.State == memory.SemanticStateActive && !compilerHasSecret(alias.Value) {
				active = append(active, alias)
			}
		}
		alt.Aliases = active
		if !strings.EqualFold(entity.CanonicalName, name) && len(active) == 0 {
			continue
		}
		for _, key := range keys {
			rows, err = q.QueryContext(ctx, `SELECT c.claim_id FROM semantic_claims c JOIN semantic_scopes s ON s.scope_id=c.scope_id JOIN semantic_predicates p ON p.predicate_id=c.predicate_id WHERE s.scope_key=? AND (c.literal_value IS NULL OR length(CAST(c.literal_value AS BLOB))<=8192) AND length(CAST(p.label AS BLOB))<=4096 AND (c.subject_entity_id=? OR c.object_entity_id=?) ORDER BY c.claim_id LIMIT 5`, key, id, id)
			if err != nil {
				return nil, err
			}
			claimIDs := []memory.SemanticID{}
			for rows.Next() {
				var cid memory.SemanticID
				if err = rows.Scan(&cid); err != nil {
					rows.Close()
					return nil, err
				}
				claimIDs = append(claimIDs, cid)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
			for _, cid := range claimIDs {
				if len(alt.Context) == 4 {
					break
				}
				claim, err := loadSemanticClaim(ctx, q, cid)
				if err != nil {
					return nil, err
				}
				state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, cid)
				if err != nil {
					return nil, err
				}
				if state.State == memory.SemanticStateActive && !compilerHasSecret(string(compilerJSON(claim))) {
					alt.Context = append(alt.Context, claim)
				}
			}
		}
		result = append(result, alt)
	}
	return result, nil
}

func validateReviewIdentityChoices(proposal *memory.CandidateIdentityProposal, options memory.ReviewIdentityOptions, choices memory.ReviewIdentityChoices) error {
	if proposal == nil {
		return errors.New("no identity proposal")
	}
	for _, entry := range []struct {
		mention      *memory.EntityMention
		choice       *memory.ReviewEntityChoice
		alternatives []memory.ReviewEntityAlternative
	}{{proposal.Subject, choices.Subject, options.Subject}, {proposal.Object, choices.Object, options.Object}} {
		if entry.mention == nil {
			if entry.choice != nil {
				return errors.New("unexpected identity choice")
			}
			continue
		}
		if entry.choice == nil || (entry.choice.EntityID == "") == !entry.choice.Create {
			return errors.New("needs_choice: choose an exact Entity or create distinct identity")
		}
		if entry.choice.Create {
			continue
		}
		found := false
		for _, alt := range entry.alternatives {
			if alt.Entity.ID == entry.choice.EntityID {
				found = true
			}
		}
		if !found {
			return errors.New("needs_choice: identity is not an authorized lexical alternative")
		}
	}
	if proposal.Predicate == nil {
		if choices.Predicate != nil {
			return errors.New("unexpected Predicate choice")
		}
		return nil
	}
	if choices.Predicate == nil || (choices.Predicate.PredicateID == "") == !choices.Predicate.Create {
		return errors.New("needs_choice: explicit Predicate definition choice required")
	}
	if choices.Predicate.Create {
		if len(options.Predicates) > 0 {
			return errors.New("existing Predicate token cannot be redefined")
		}
		return nil
	}
	for _, p := range options.Predicates {
		if p.ID == choices.Predicate.PredicateID && sameReviewPredicate(*proposal.Predicate, p) {
			return nil
		}
	}
	return errors.New("needs_choice: Predicate definition differs from proposal")
}
func sameReviewPredicate(p memory.PredicateDefinition, existing memory.SemanticPredicate) bool {
	return p.Token == existing.Token && p.Label == existing.Label && p.ObjectConstraint == existing.ObjectConstraint && p.Cardinality == existing.Cardinality
}

func (s *Store) ChooseOwnerCandidateIdentity(ctx context.Context, a OwnerReviewContext, decision memory.ReviewIdentityDecision) (memory.OwnerCandidate, error) {
	var result memory.OwnerCandidate
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		item, err := loadReviewCandidate(ctx, conn, a, decision.Candidate.ID, true)
		if err != nil {
			return err
		}
		if item.Candidate.ReviewState != "unresolved" {
			return ErrReviewResolved
		}
		if item.Ref != decision.Candidate {
			return ErrReviewStale
		}
		if item.Candidate.EquivalentTo != "" {
			return errors.New("equivalent candidate has no independent review")
		}
		options, err := reviewIdentityOptions(ctx, conn, a, item)
		if err != nil {
			return err
		}
		if options.SHA256 != decision.OptionsSHA256 {
			return ErrReviewStale
		}
		if err = validateReviewIdentityChoices(item.Candidate.Proposal.Identity, options, decision.Choices); err != nil {
			return err
		}
		id, err := newSemanticID()
		if err != nil {
			return err
		}
		revision := memory.ReviewIdentityRevision{Revision: item.Ref.InterpretationRevision + 1, ParentRevision: item.Ref.InterpretationRevision, ReviewRevision: item.Ref.ReviewRevision + 1, AuditID: string(id), OwnerID: memory.LocalOwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, Options: options, Choices: decision.Choices}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_identity_revisions VALUES(?,?,?)`, item.Ref.ID, revision.Revision, compilerJSON(revision)); err != nil {
			return err
		}
		update, err := conn.ExecContext(ctx, `UPDATE memory_compiler_candidates SET review_revision=review_revision+1 WHERE candidate_id=? AND review_revision=? AND review_state='unresolved'`, item.Ref.ID, item.Ref.ReviewRevision)
		if err != nil {
			return err
		}
		count, err := update.RowsAffected()
		if err != nil || count != 1 {
			return ErrReviewStale
		}
		result, err = loadReviewCandidate(ctx, conn, a, item.Ref.ID, true)
		return err
	})
	if err != nil {
		return memory.OwnerCandidate{}, err
	}
	return result, nil
}

// InspectOwnerCandidateIdentityRevision exposes one exact abandoned or accepted
// owner interpretation, retaining current candidate/source disclosure checks.
func (s *Store) InspectOwnerCandidateIdentityRevision(ctx context.Context, a OwnerReviewContext, id string, revision int64) (memory.ReviewIdentityRevision, error) {
	var out memory.ReviewIdentityRevision
	if revision < 1 {
		return out, errors.New("positive interpretation revision required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	if _, err = loadReviewCandidate(ctx, tx, a, id, true); err != nil {
		return out, err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT envelope FROM memory_review_identity_revisions WHERE candidate_id=? AND revision=?`, id, revision).Scan(&raw); err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return memory.ReviewIdentityRevision{}, err
	}
	return out, tx.Commit()
}
