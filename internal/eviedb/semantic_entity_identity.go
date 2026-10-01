package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

// Entity identity disclosure (harness review M6, Stage 14). Entity resolution
// still prefers duplicates over unsafe merges and still reuses the Entity the
// model selected; these helpers only make the choice visible: the approval
// card names the reused Entity with details that tell same-named Entities
// apart, and recall marks a Claim whose Entity shares a name with another.
// Merge and split remain out of scope.

const (
	identityAliasLimit    = 5
	identityNameLimit     = 16
	identitySameNameLimit = 64
)

// activeEntityAliases returns an Entity's currently active Alias values in
// the readable scopes, ordered by Alias ID.
func activeEntityAliases(ctx context.Context, q semanticInspectionQueryer, entity memory.SemanticID, scopes []string) ([]string, error) {
	keys := readableScopeArgs(scopes)
	rows, err := q.QueryContext(ctx, `
		SELECT aliases.value FROM semantic_aliases AS aliases
		JOIN semantic_scopes AS scopes ON scopes.scope_id = aliases.scope_id
		WHERE aliases.entity_id = ? AND scopes.scope_key IN (?,?,?) AND aliases.lifecycle = 'active'
		  AND COALESCE((SELECT state FROM semantic_state_events
		       WHERE object_kind = 'alias' AND object_id = aliases.alias_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), aliases.lifecycle) = 'active'
		ORDER BY aliases.alias_id LIMIT ?
	`, entity, keys[0], keys[1], keys[2], identityNameLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func readableScopeArgs(scopes []string) [3]string {
	var keys [3]string
	for i := range keys {
		keys[i] = "global"
		if i < len(scopes) {
			keys[i] = scopes[i]
		}
	}
	return keys
}

// entityNames is an Entity's canonical name followed by its Aliases,
// normalized and without duplicates.
func entityNames(canonical string, aliases []string) []string {
	seen := map[string]bool{}
	var names []string
	for _, value := range append([]string{canonical}, aliases...) {
		if normalized := normalizeAlias(value); normalized != "" && !seen[normalized] {
			seen[normalized] = true
			names = append(names, normalized)
		}
	}
	return names
}

// otherEntitiesNamed returns the other active ordinary Entities in the
// readable scopes whose canonical name or active Alias is name.
func otherEntitiesNamed(ctx context.Context, q semanticInspectionQueryer, self memory.SemanticID, name string, scopes []string) ([]memory.SemanticID, error) {
	keys := readableScopeArgs(scopes)
	rows, err := q.QueryContext(ctx, `
		WITH named(entity_id) AS (
		  SELECT entity_id FROM semantic_entities WHERE canonical_name = ? COLLATE NOCASE
		  UNION
		  SELECT aliases.entity_id FROM semantic_aliases AS aliases
		  JOIN semantic_scopes AS alias_scopes ON alias_scopes.scope_id = aliases.scope_id
		  WHERE alias_scopes.scope_key IN (?,?,?) AND aliases.normalized_value = ? AND aliases.lifecycle = 'active'
		    AND COALESCE((SELECT state FROM semantic_state_events
		         WHERE object_kind = 'alias' AND object_id = aliases.alias_id
		         ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), aliases.lifecycle) = 'active')
		SELECT DISTINCT entities.entity_id FROM named
		JOIN semantic_entities AS entities ON entities.entity_id = named.entity_id
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.entity_id <> ? AND scopes.scope_key IN (?,?,?) AND entities.lifecycle = 'active'
		  AND COALESCE(entities.anchor_kind, '') = ''
		  AND COALESCE((SELECT state FROM semantic_state_events
		       WHERE object_kind = 'entity' AND object_id = entities.entity_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), entities.lifecycle) = 'active'
		ORDER BY entities.entity_id LIMIT ?
	`, name, keys[0], keys[1], keys[2], name, self, keys[0], keys[1], keys[2], identitySameNameLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sameNamedEntities counts the distinct other Entities sharing any of names,
// and reports the first shared name.
func sameNamedEntities(ctx context.Context, q semanticInspectionQueryer, self memory.SemanticID, names []string, scopes []string) (int, string, error) {
	others := map[memory.SemanticID]bool{}
	first := ""
	for _, name := range names {
		ids, err := otherEntitiesNamed(ctx, q, self, name, scopes)
		if err != nil {
			return 0, "", err
		}
		if len(ids) > 0 && first == "" {
			first = name
		}
		for _, id := range ids {
			others[id] = true
		}
	}
	return len(others), first, nil
}

// exampleEntityClaim renders one current Claim about an existing Entity, the
// detail most likely to tell two same-named Entities apart on approval.
func exampleEntityClaim(ctx context.Context, q semanticInspectionQueryer, entity memory.SemanticID, scopes []string) (string, error) {
	keys := readableScopeArgs(scopes)
	var id memory.SemanticID
	err := q.QueryRowContext(ctx, `
		SELECT claims.claim_id FROM semantic_claims AS claims
		JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
		WHERE (claims.subject_entity_id = ? OR claims.object_entity_id = ?) AND scopes.scope_key IN (?,?,?)
		  AND COALESCE((SELECT state FROM semantic_state_events
		       WHERE object_kind = 'claim' AND object_id = claims.claim_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), claims.lifecycle) = 'active'
		ORDER BY claims.transaction_time DESC, claims.claim_id LIMIT 1
	`, entity, entity, keys[0], keys[1], keys[2]).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	claim, err := loadSemanticClaim(ctx, q, id)
	if err != nil {
		return "", err
	}
	return renderClaimText(ctx, q, claim)
}

func renderClaimText(ctx context.Context, q semanticInspectionQueryer, claim memory.SemanticClaim) (string, error) {
	subject, err := loadSemanticEntityForInspection(ctx, q, claim.SubjectEntityID)
	if err != nil {
		return "", err
	}
	object := ""
	if claim.Object.Literal != nil {
		object = claim.Object.Literal.Value
	} else {
		entity, err := loadSemanticEntityForInspection(ctx, q, claim.Object.EntityID)
		if err != nil {
			return "", err
		}
		object = entity.CanonicalName
	}
	text := fmt.Sprintf("%s — %s: %s", subject.CanonicalName, claim.Predicate.Label, object)
	if claim.Polarity == memory.PolarityDenied {
		text = "Denied: " + text
	}
	return text, nil
}

// proposalEntityIdentity describes one ordinary subject or object Entity of a
// remember proposal for the approval card.
func proposalEntityIdentity(ctx context.Context, q semanticInspectionQueryer, role string, entity memory.SemanticEntity, alias *memory.SemanticAlias, selector memory.EntitySelector, scopes []string) (memory.ProposalEntityIdentity, error) {
	identity := memory.ProposalEntityIdentity{Role: role, EntityID: entity.ID, CanonicalName: entity.CanonicalName,
		EntityType: entity.EntityType, ScopeKey: entity.ScopeKey, Reused: !entity.Create, SelectedBy: memory.EntitySelectedByAlias}
	switch {
	case selector.Create:
		identity.SelectedBy = memory.EntitySelectedByCreate
	case selector.EntityID != "":
		identity.SelectedBy = memory.EntitySelectedByID
	}
	var aliases []string
	if entity.Create {
		if alias != nil {
			aliases = []string{alias.Value}
		}
	} else {
		var err error
		if aliases, err = activeEntityAliases(ctx, q, entity.ID, scopes); err != nil {
			return identity, err
		}
		if identity.ExampleClaim, err = exampleEntityClaim(ctx, q, entity.ID, scopes); err != nil {
			return identity, err
		}
	}
	identity.Aliases = aliases[:min(len(aliases), identityAliasLimit)]
	count, _, err := sameNamedEntities(ctx, q, entity.ID, entityNames(entity.CanonicalName, aliases), scopes)
	identity.SameName = count
	return identity, err
}

// claimBindingNeeds lists, for owner-span binding, what the owner's message
// must contain for one proposition: the names under which each ordinary
// Entity may appear, and a literal object's value. Anchors are implied by the
// speaker; a proposition with neither falls back to its Predicate's words. A
// literal with no words of its own yields an unmatched item, so it is
// Evie-proposed.
func claimBindingNeeds(ctx context.Context, q semanticInspectionQueryer, content, predicateToken, predicateLabel string, literal *memory.TypedLiteral, scopes []string, entities ...entityWithAlias) ([]bindingOccurrences, error) {
	needs, err := entityNameNeeds(ctx, q, content, scopes, entities...)
	if err != nil {
		return nil, err
	}
	if literal != nil {
		occurrences := literalOccurrences(content, *literal, predicateToken, predicateLabel)
		if occurrences == nil {
			occurrences = []bindingOccurrences{nil}
		}
		needs = append(needs, occurrences...)
	}
	if len(needs) == 0 {
		needs = append(needs, predicateOccurrences(content, predicateToken, predicateLabel))
	}
	return needs, nil
}

// claimValueNeeds is claimBindingNeeds for an accepted Claim, its Entities'
// names read from the Claim's own scope.
func claimValueNeeds(ctx context.Context, q semanticInspectionQueryer, content string, claim memory.SemanticClaim) ([]bindingOccurrences, error) {
	var entities []entityWithAlias
	for _, id := range []memory.SemanticID{claim.SubjectEntityID, claim.Object.EntityID} {
		if id == "" {
			continue
		}
		entity, err := loadSemanticEntityForInspection(ctx, q, id)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entityWithAlias{entity: entity})
	}
	return claimBindingNeeds(ctx, q, content, claim.Predicate.Token, claim.Predicate.Label, claim.Object.Literal,
		[]string{"global", claim.ScopeKey}, entities...)
}

func entityNameNeeds(ctx context.Context, q semanticInspectionQueryer, content string, scopes []string, entities ...entityWithAlias) ([]bindingOccurrences, error) {
	tokens := bindingTokens(content)
	seen := map[memory.SemanticID]bool{}
	var needs []bindingOccurrences
	for _, item := range entities {
		if item.entity.AnchorKind != "" || seen[item.entity.ID] {
			continue
		}
		seen[item.entity.ID] = true
		names := []string{item.entity.CanonicalName}
		if item.alias != nil {
			names = append(names, item.alias.Value)
		}
		if !item.entity.Create {
			aliases, err := activeEntityAliases(ctx, q, item.entity.ID, scopes)
			if err != nil {
				return nil, err
			}
			names = append(names, aliases...)
		}
		sort.Strings(names)
		needs = append(needs, phraseOccurrences(tokens, names...))
	}
	return needs, nil
}

type entityWithAlias struct {
	entity memory.SemanticEntity
	alias  *memory.SemanticAlias
}

// ambiguousEntityNames marks the ordinary Entities of a retrieved Claim whose
// name also names another active Entity the reader can see, and returns the
// Claim text with each such Entity identified.
func ambiguousEntityNames(ctx context.Context, q semanticInspectionQueryer, claim memory.SemanticClaim, scopes []string) ([]memory.RetrievalAmbiguousName, string, error) {
	var marks []memory.RetrievalAmbiguousName
	display := map[memory.SemanticID]string{}
	for _, id := range []memory.SemanticID{claim.SubjectEntityID, claim.Object.EntityID} {
		if id == "" || display[id] != "" {
			continue
		}
		entity, err := loadSemanticEntityForInspection(ctx, q, id)
		if err != nil {
			return nil, "", err
		}
		display[id] = entity.CanonicalName
		if entity.AnchorKind != "" {
			continue
		}
		aliases, err := activeEntityAliases(ctx, q, id, scopes)
		if err != nil {
			return nil, "", err
		}
		count, name, err := sameNamedEntities(ctx, q, id, entityNames(entity.CanonicalName, aliases), scopes)
		if err != nil {
			return nil, "", err
		}
		if count == 0 {
			continue
		}
		marks = append(marks, memory.RetrievalAmbiguousName{Name: name, EntityID: id, EntityType: entity.EntityType, Entities: count + 1})
		display[id] = fmt.Sprintf("%s [%s %s; %d entities named %q]", entity.CanonicalName, entity.EntityType, shortSemanticID(id), count+1, name)
	}
	object := ""
	if claim.Object.Literal != nil {
		object = claim.Object.Literal.Value
	} else {
		object = display[claim.Object.EntityID]
	}
	text := fmt.Sprintf("%s — %s: %s", display[claim.SubjectEntityID], claim.Predicate.Label, object)
	if claim.Polarity == memory.PolarityDenied {
		text = "Denied: " + text
	}
	return marks, text, nil
}

func shortSemanticID(id memory.SemanticID) string {
	return strings.SplitN(string(id), "-", 2)[0]
}
