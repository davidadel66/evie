package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

func compilerIdentityContext(ctx context.Context, conn *sql.Conn, request *memory.CompilerRequest) error {
	var raw []byte
	if err := conn.QueryRowContext(ctx, `SELECT manifest FROM memory_compiler_generations WHERE generation_id=?`, request.GenerationID).Scan(&raw); err != nil {
		return err
	}
	var generation memory.CompilerGeneration
	if err := json.Unmarshal(raw, &generation); err != nil {
		return err
	}
	request.IdentityPolicy = ""
	request.Aliases = nil
	if generation.EntityPolicy != memory.CompilerIdentityPolicyV2 {
		return nil
	}
	request.IdentityPolicy = memory.CompilerIdentityPolicyV2
	inspected := 0
	for _, entity := range request.Entities {
		rows, err := conn.QueryContext(ctx, `SELECT alias_id,CASE WHEN length(CAST(value AS BLOB))<=1024 THEN value ELSE '' END,CASE WHEN length(CAST(normalized_value AS BLOB))<=1024 THEN normalized_value ELSE '' END,source_event_id,created_operation_id FROM semantic_aliases WHERE entity_id=? AND scope_id=(SELECT scope_id FROM semantic_scopes WHERE scope_key=?) AND lifecycle='active' ORDER BY alias_id LIMIT ?`, entity.ID, entity.ScopeKey, 33-inspected)
		if err != nil {
			return err
		}
		aliases := []memory.SemanticAlias{}
		for rows.Next() {
			alias := memory.SemanticAlias{EntityID: entity.ID, ScopeKey: entity.ScopeKey}
			if err = rows.Scan(&alias.ID, &alias.Value, &alias.NormalizedValue, &alias.SourceEventID, &alias.OperationID); err != nil {
				rows.Close()
				return err
			}
			aliases = append(aliases, alias)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, alias := range aliases {
			if inspected == 32 {
				request.AcceptedContextOmitted = true
				return nil
			}
			inspected++
			state, err := loadLatestState(ctx, inspectionLifecycleQueryer{conn}, memory.SemanticObjectAlias, alias.ID)
			if err != nil {
				return err
			}
			if state.State != memory.SemanticStateActive {
				continue
			}
			if alias.Value == "" || alias.NormalizedValue == "" || len(alias.Value) > 1024 || !utf8.ValidString(alias.Value) || compilerHasSecret(alias.Value) {
				request.AcceptedContextOmitted = true
				continue
			}
			if len(request.Aliases) == 32 || len(compilerJSON(append(request.Aliases, alias))) > 8192 {
				request.AcceptedContextOmitted = true
				return nil
			}
			request.Aliases = append(request.Aliases, alias)
		}
	}
	return nil
}

func validateCandidateIdentity(proposal memory.ExtractorCandidate) error {
	identity := proposal.Identity
	if identity == nil {
		return nil
	}
	if identity.Subject == nil && identity.Object == nil && identity.Predicate == nil {
		return errors.New("empty identity proposal")
	}
	if len(identity.Uncertainty) > 2048 || !utf8.ValidString(identity.Uncertainty) || compilerHasSecret(identity.Uncertainty) || identity.Confidence != nil && (math.IsNaN(*identity.Confidence) || math.IsInf(*identity.Confidence, 0) || *identity.Confidence < 0 || *identity.Confidence > 1) {
		return errors.New("invalid identity uncertainty")
	}
	for _, mention := range []*memory.EntityMention{identity.Subject, identity.Object} {
		if mention == nil {
			continue
		}
		if mention.Name != strings.TrimSpace(mention.Name) || mention.EntityType != strings.TrimSpace(mention.EntityType) || mention.Name == "" || mention.EntityType == "" || len(mention.Name) > 256 || len(mention.EntityType) > 64 || !utf8.ValidString(mention.Name+mention.EntityType) || compilerHasSecret(mention.Name+" "+mention.EntityType) {
			return errors.New("invalid sourced identity mention")
		}
		found := false
		for _, source := range proposal.Support {
			if source == mention.Support {
				found = true
			}
		}
		if !found {
			return errors.New("identity mention requires exact candidate support")
		}
	}
	if identity.Subject != nil && proposal.Proposition.SubjectEntityID != "" || identity.Object != nil && (proposal.Proposition.Object.EntityID != "" || proposal.Proposition.Object.Literal != nil) || identity.Predicate != nil && proposal.Proposition.PredicateID != "" {
		return errors.New("identity proposal must remain unresolved")
	}
	if p := identity.Predicate; p != nil {
		if len(p.Token) > 64 || !predicateTokenPattern.MatchString(p.Token) || strings.TrimSpace(p.Label) == "" || len(p.Label) > 256 || !utf8.ValidString(p.Label) || compilerHasSecret(p.Token+" "+p.Label) || (p.Cardinality != memory.CardinalityOne && p.Cardinality != memory.CardinalityMany) {
			return errors.New("invalid proposed Predicate definition")
		}
		switch p.ObjectConstraint {
		case memory.ConstraintEntity, "text", "integer", "decimal", "boolean", "date", "datetime":
		default:
			return errors.New("invalid proposed Predicate constraint")
		}
	}
	return nil
}

func validateCompilerProposition(request memory.CompilerRequest, proposal memory.ExtractorCandidate) error {
	if proposal.Identity != nil && request.IdentityPolicy != memory.CompilerIdentityPolicyV2 {
		return fmt.Errorf("%w: identity policy does not admit proposals", ErrCompilerTerminalOutput)
	}
	if request.IdentityPolicy != "" && request.IdentityPolicy != memory.CompilerIdentityPolicyV2 {
		return errors.New("unsupported request identity policy")
	}
	if err := validateCandidateIdentity(proposal); err != nil {
		return errors.Join(ErrCompilerTerminalOutput, err)
	}
	entities := map[memory.SemanticID]bool{}
	for _, e := range request.Entities {
		entities[e.ID] = true
	}
	predicates := map[memory.SemanticID]memory.SemanticPredicate{}
	for _, p := range request.Predicates {
		predicates[p.ID] = p
	}
	identity := proposal.Identity
	if (identity == nil || identity.Subject == nil) && !entities[proposal.Proposition.SubjectEntityID] {
		return fmt.Errorf("%w: unknown subject identity", ErrCompilerTerminalOutput)
	}
	predicate, ok := predicates[proposal.Proposition.PredicateID]
	if identity != nil && identity.Predicate != nil {
		predicate.ObjectConstraint = identity.Predicate.ObjectConstraint
		ok = true
	}
	if !ok {
		return fmt.Errorf("%w: unreviewed Predicate", ErrCompilerTerminalOutput)
	}
	object := proposal.Proposition.Object
	if identity != nil && identity.Object != nil {
		if predicate.ObjectConstraint != memory.ConstraintEntity {
			return errors.New("Predicate does not admit Entity mention")
		}
		return nil
	}
	if (object.EntityID == "") == (object.Literal == nil) {
		return errors.New("object must have exactly one typed value")
	}
	if object.EntityID != "" {
		if !entities[object.EntityID] || predicate.ObjectConstraint != memory.ConstraintEntity {
			return fmt.Errorf("%w: invalid object identity or Predicate constraint", ErrCompilerTerminalOutput)
		}
	} else {
		if err := validateLiteral(*object.Literal); err != nil {
			return err
		}
		if string(predicate.ObjectConstraint) != string(object.Literal.Kind) {
			return fmt.Errorf("%w: Predicate literal constraint mismatch", ErrCompilerTerminalOutput)
		}
		if len(object.Literal.Value) > 8192 || compilerHasSecret(object.Literal.Value) {
			return fmt.Errorf("%w: invalid candidate text", ErrCompilerTerminalOutput)
		}
	}
	return nil
}

func validateCompilerIdentitySupport(candidate memory.MemoryCandidate) error {
	if candidate.Proposal.Identity == nil {
		return nil
	}
	for _, mention := range []*memory.EntityMention{candidate.Proposal.Identity.Subject, candidate.Proposal.Identity.Object} {
		if mention == nil {
			continue
		}
		found := false
		for _, source := range candidate.Support {
			if source.Locator == mention.Support && strings.Contains(source.Evidence, mention.Name) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: identity name absent from cited supporting text", ErrCompilerTerminalOutput)
		}
	}
	return nil
}
