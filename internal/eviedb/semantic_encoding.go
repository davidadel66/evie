package eviedb

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/davidadel66/evie/internal/memory"
)

func semanticHash(value any) (string, []byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest), encoded, nil
}

type canonicalPredicate struct {
	PredicateID      memory.SemanticID                `json:"predicate_id"`
	Token            string                           `json:"token"`
	Version          int64                            `json:"version"`
	Label            string                           `json:"label"`
	ObjectConstraint memory.PredicateObjectConstraint `json:"object_constraint"`
	Cardinality      memory.PredicateCardinality      `json:"cardinality"`
}

type canonicalEntity struct {
	EntityID      memory.SemanticID `json:"entity_id"`
	ScopeKey      string            `json:"scope_key"`
	CanonicalName string            `json:"canonical_name"`
	EntityType    string            `json:"entity_type"`
	Lifecycle     string            `json:"lifecycle"`
}

type canonicalClaimObject struct {
	Literal memory.TypedLiteral `json:"literal"`
}

type canonicalValidTime struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

func encodeCanonicalValidTime(value memory.ValidTime) canonicalValidTime {
	encoded := canonicalValidTime{}
	if value.From != nil {
		from := formatSemanticTime(*value.From)
		encoded.From = &from
	}
	if value.To != nil {
		to := formatSemanticTime(*value.To)
		encoded.To = &to
	}
	return encoded
}

type canonicalClaim struct {
	ClaimID          memory.SemanticID    `json:"claim_id"`
	ScopeKey         string               `json:"scope_key"`
	SubjectEntityID  memory.SemanticID    `json:"subject_entity_id"`
	PredicateID      memory.SemanticID    `json:"predicate_id"`
	PredicateToken   string               `json:"predicate_token"`
	PredicateVersion int64                `json:"predicate_version"`
	Object           canonicalClaimObject `json:"object"`
	Polarity         memory.ClaimPolarity `json:"polarity"`
	ValidTime        canonicalValidTime   `json:"valid_time"`
	Lifecycle        string               `json:"lifecycle"`
}

type canonicalLocator struct {
	EventID        memory.EventID             `json:"event_id"`
	EventPart      memory.EvidencePart        `json:"event_part"`
	LocatorKind    memory.EvidenceLocatorKind `json:"locator_kind"`
	LocatorValue   string                     `json:"locator_value"`
	EvidenceSHA256 string                     `json:"evidence_sha256"`
}

type canonicalSourceLink struct {
	SourceLinkID memory.SemanticID         `json:"source_link_id"`
	ClaimID      memory.SemanticID         `json:"claim_id"`
	Locator      canonicalLocator          `json:"locator"`
	Actor        memory.SemanticActor      `json:"actor"`
	SourceType   memory.SemanticSourceType `json:"source_type"`
	Authority    memory.SourceAuthority    `json:"authority"`
	ObservedAt   string                    `json:"observed_at"`
	Eligibility  memory.SourceEligibility  `json:"eligibility"`
}

type canonicalEffect struct {
	Scopes      []string              `json:"scopes"`
	Predicates  []canonicalPredicate  `json:"predicates"`
	Entities    []canonicalEntity     `json:"entities"`
	Aliases     []struct{}            `json:"aliases"`
	Claims      []canonicalClaim      `json:"claims"`
	SourceLinks []canonicalSourceLink `json:"source_links"`
	GraphLinks  []struct{}            `json:"graph_links"`
	Transitions []struct{}            `json:"transitions"`
}

type canonicalProposal struct {
	Kind           string                 `json:"kind"`
	IdempotencyKey string                 `json:"idempotency_key"`
	Actor          memory.SemanticActor   `json:"actor"`
	SessionID      memory.SessionID       `json:"session_id"`
	PriorRevisions []memory.ScopeRevision `json:"prior_revisions"`
	SourceEventIDs []memory.EventID       `json:"source_event_ids"`
	Effect         canonicalEffect        `json:"effect"`
}

type canonicalAlias struct {
	AliasID         memory.SemanticID `json:"alias_id"`
	EntityID        memory.SemanticID `json:"entity_id"`
	ScopeKey        string            `json:"scope_key"`
	Value           string            `json:"value"`
	NormalizedValue string            `json:"normalized_value"`
	Lifecycle       string            `json:"lifecycle"`
}

type canonicalEntityClaimObject struct {
	EntityID memory.SemanticID `json:"entity_id"`
}

type canonicalEntityClaim struct {
	ClaimID          memory.SemanticID          `json:"claim_id"`
	ScopeKey         string                     `json:"scope_key"`
	SubjectEntityID  memory.SemanticID          `json:"subject_entity_id"`
	PredicateID      memory.SemanticID          `json:"predicate_id"`
	PredicateToken   string                     `json:"predicate_token"`
	PredicateVersion int64                      `json:"predicate_version"`
	Object           canonicalEntityClaimObject `json:"object"`
	Polarity         memory.ClaimPolarity       `json:"polarity"`
	ValidTime        canonicalValidTime         `json:"valid_time"`
	Lifecycle        string                     `json:"lifecycle"`
}

type canonicalEntityEffect struct {
	Scopes      []string               `json:"scopes"`
	Predicates  []canonicalPredicate   `json:"predicates"`
	Entities    []canonicalEntity      `json:"entities"`
	Aliases     []canonicalAlias       `json:"aliases"`
	Claims      []canonicalEntityClaim `json:"claims"`
	SourceLinks []canonicalSourceLink  `json:"source_links"`
	GraphLinks  []struct{}             `json:"graph_links"`
	Transitions []struct{}             `json:"transitions"`
}

type canonicalEntityProposal struct {
	Kind           string                 `json:"kind"`
	IdempotencyKey string                 `json:"idempotency_key"`
	Actor          memory.SemanticActor   `json:"actor"`
	SessionID      memory.SessionID       `json:"session_id"`
	PriorRevisions []memory.ScopeRevision `json:"prior_revisions"`
	SourceEventIDs []memory.EventID       `json:"source_event_ids"`
	Effect         canonicalEntityEffect  `json:"effect"`
}

func canonicalRememberLiteralProposal(proposal memory.RememberLiteralProposal) canonicalProposal {
	writtenScopes := []string{proposal.Scope.Key}
	if proposal.Scope.Key != "global" && proposalWritesGlobal(proposal) {
		writtenScopes = append(writtenScopes, "global")
		sort.Strings(writtenScopes)
	}
	effect := canonicalEffect{
		Scopes: writtenScopes, Predicates: []canonicalPredicate{}, Entities: []canonicalEntity{},
		Aliases: []struct{}{}, GraphLinks: []struct{}{}, Transitions: []struct{}{},
	}
	if proposal.Predicate.Create {
		effect.Predicates = append(effect.Predicates, canonicalPredicate{
			PredicateID: proposal.Predicate.ID, Token: proposal.Predicate.Token, Version: proposal.Predicate.Version,
			Label: proposal.Predicate.Label, ObjectConstraint: proposal.Predicate.ObjectConstraint,
			Cardinality: proposal.Predicate.Cardinality,
		})
	}
	for _, entity := range []memory.SemanticEntity{proposal.Subject, proposal.Evie} {
		if entity.Create {
			effect.Entities = append(effect.Entities, canonicalEntity{
				EntityID: entity.ID, ScopeKey: entity.ScopeKey, CanonicalName: entity.CanonicalName,
				EntityType: entity.EntityType, Lifecycle: "active",
			})
		}
	}
	if proposal.ClaimCreate {
		effect.Claims = []canonicalClaim{{
			ClaimID: proposal.ClaimID, ScopeKey: proposal.Scope.Key, SubjectEntityID: proposal.Subject.ID,
			PredicateID: proposal.Predicate.ID, PredicateToken: proposal.Predicate.Token,
			PredicateVersion: proposal.Predicate.Version, Object: canonicalClaimObject{Literal: proposal.Literal},
			Polarity: proposal.Polarity, ValidTime: encodeCanonicalValidTime(proposal.ValidTime), Lifecycle: "active",
		}}
	} else {
		effect.Claims = []canonicalClaim{}
	}
	if proposal.Source.Create {
		effect.SourceLinks = []canonicalSourceLink{{
			SourceLinkID: proposal.SourceLinkID, ClaimID: proposal.ClaimID,
			Locator: canonicalLocator{EventID: proposal.Source.EventID, EventPart: proposal.Source.EventPart,
				LocatorKind: proposal.Source.LocatorKind, LocatorValue: proposal.Source.LocatorValue,
				EvidenceSHA256: proposal.Source.EvidenceSHA256},
			Actor: proposal.Source.Actor, SourceType: proposal.Source.SourceType, Authority: proposal.Source.Authority,
			ObservedAt: proposal.Source.ObservedAt, Eligibility: memory.EligibilityEligible,
		}}
	} else {
		effect.SourceLinks = []canonicalSourceLink{}
	}
	return canonicalProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Source.EventID}, Effect: effect,
	}
}

func canonicalRememberEntityProposal(proposal memory.RememberEntityProposal) canonicalEntityProposal {
	writtenScopes := []string{proposal.Scope.Key}
	if proposal.Scope.Key != "global" && entityProposalWritesGlobal(proposal) {
		writtenScopes = append(writtenScopes, "global")
		sort.Strings(writtenScopes)
	}
	effect := canonicalEntityEffect{
		Scopes: writtenScopes, Predicates: []canonicalPredicate{}, Entities: []canonicalEntity{},
		Aliases: []canonicalAlias{}, Claims: []canonicalEntityClaim{}, SourceLinks: []canonicalSourceLink{},
		GraphLinks: []struct{}{}, Transitions: []struct{}{},
	}
	if proposal.Predicate.Create {
		effect.Predicates = append(effect.Predicates, canonicalPredicate{
			PredicateID: proposal.Predicate.ID, Token: proposal.Predicate.Token, Version: proposal.Predicate.Version,
			Label: proposal.Predicate.Label, ObjectConstraint: proposal.Predicate.ObjectConstraint,
			Cardinality: proposal.Predicate.Cardinality,
		})
	}
	for _, entity := range proposal.Entities {
		if entity.Create {
			effect.Entities = append(effect.Entities, canonicalEntity{
				EntityID: entity.ID, ScopeKey: entity.ScopeKey, CanonicalName: entity.CanonicalName,
				EntityType: entity.EntityType, Lifecycle: "active",
			})
		}
	}
	for _, alias := range proposal.Aliases {
		if alias.Create {
			effect.Aliases = append(effect.Aliases, canonicalAlias{
				AliasID: alias.ID, EntityID: alias.EntityID, ScopeKey: alias.ScopeKey,
				Value: alias.Value, NormalizedValue: alias.NormalizedValue, Lifecycle: "active",
			})
		}
	}
	if proposal.Claim.Create {
		effect.Claims = append(effect.Claims, canonicalEntityClaim{
			ClaimID: proposal.Claim.ID, ScopeKey: proposal.Claim.ScopeKey,
			SubjectEntityID: proposal.Claim.SubjectEntityID, PredicateID: proposal.Claim.PredicateID,
			PredicateToken: proposal.Claim.PredicateToken, PredicateVersion: proposal.Claim.PredicateVersion,
			Object:   canonicalEntityClaimObject{EntityID: proposal.Claim.ObjectEntityID},
			Polarity: proposal.Claim.Polarity, ValidTime: encodeCanonicalValidTime(proposal.Claim.ValidTime), Lifecycle: "active",
		})
	}
	if proposal.Source.Create {
		effect.SourceLinks = append(effect.SourceLinks, canonicalSourceLink{
			SourceLinkID: proposal.Source.ID, ClaimID: proposal.Claim.ID,
			Locator: canonicalLocator{EventID: proposal.Source.EventID, EventPart: proposal.Source.EventPart,
				LocatorKind: proposal.Source.LocatorKind, LocatorValue: proposal.Source.LocatorValue,
				EvidenceSHA256: proposal.Source.EvidenceSHA256},
			Actor: proposal.Source.Actor, SourceType: proposal.Source.SourceType,
			Authority: proposal.Source.Authority, ObservedAt: proposal.Source.ObservedAt, Eligibility: memory.EligibilityEligible,
		})
	}
	return canonicalEntityProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Source.EventID}, Effect: effect,
	}
}

type canonicalPromotionMapping struct {
	SourceEntityID      memory.SemanticID `json:"source_entity_id"`
	DestinationEntityID memory.SemanticID `json:"destination_entity_id"`
}

type canonicalPromotionRecord struct {
	SourceScopeKey      string                      `json:"source_scope_key"`
	DestinationScopeKey string                      `json:"destination_scope_key"`
	SourceClaimID       memory.SemanticID           `json:"source_claim_id"`
	DestinationClaimID  memory.SemanticID           `json:"destination_claim_id"`
	EntityMappings      []canonicalPromotionMapping `json:"entity_mappings"`
}

type canonicalPromotionEffect struct {
	Scopes      []string                   `json:"scopes"`
	Predicates  []struct{}                 `json:"predicates"`
	Entities    []canonicalEntity          `json:"entities"`
	Aliases     []struct{}                 `json:"aliases"`
	Claims      []canonicalCorrectionClaim `json:"claims"`
	SourceLinks []canonicalSourceLink      `json:"source_links"`
	GraphLinks  []struct{}                 `json:"graph_links"`
	Transitions []struct{}                 `json:"transitions"`
	Promotions  []canonicalPromotionRecord `json:"promotions"`
}

type canonicalPromotionProposal struct {
	Kind           string                   `json:"kind"`
	IdempotencyKey string                   `json:"idempotency_key"`
	Actor          memory.SemanticActor     `json:"actor"`
	SessionID      memory.SessionID         `json:"session_id"`
	PriorRevisions []memory.ScopeRevision   `json:"prior_revisions"`
	SourceEventIDs []memory.EventID         `json:"source_event_ids"`
	Effect         canonicalPromotionEffect `json:"effect"`
}

func canonicalPromoteClaimProposal(proposal memory.PromotionProposal) canonicalPromotionProposal {
	effect := canonicalPromotionEffect{
		Scopes: []string{proposal.DestinationScope.Key}, Predicates: []struct{}{}, Entities: []canonicalEntity{},
		Aliases: []struct{}{}, Claims: []canonicalCorrectionClaim{}, SourceLinks: []canonicalSourceLink{},
		GraphLinks: []struct{}{}, Transitions: []struct{}{},
	}
	mappings := make([]canonicalPromotionMapping, 0, len(proposal.PromotedEntities))
	for _, promoted := range proposal.PromotedEntities {
		mappings = append(mappings, canonicalPromotionMapping{
			SourceEntityID: promoted.SourceEntityID, DestinationEntityID: promoted.DestinationEntity.ID,
		})
		if promoted.DestinationEntity.Create {
			effect.Entities = append(effect.Entities, canonicalEntity{
				EntityID: promoted.DestinationEntity.ID, ScopeKey: promoted.DestinationEntity.ScopeKey,
				CanonicalName: promoted.DestinationEntity.CanonicalName, EntityType: promoted.DestinationEntity.EntityType,
				Lifecycle: "active",
			})
		}
	}
	if proposal.DestinationClaimCreate {
		claim := proposal.DestinationClaim
		effect.Claims = append(effect.Claims, canonicalCorrectionClaim{
			ClaimID: claim.ID, ScopeKey: claim.ScopeKey, SubjectEntityID: claim.SubjectEntityID,
			PredicateID: claim.Predicate.ID, PredicateToken: claim.Predicate.Token, PredicateVersion: claim.Predicate.Version,
			Object:   canonicalCorrectionClaimObject{EntityID: claim.Object.EntityID, Literal: claim.Object.Literal},
			Polarity: claim.Polarity, ValidTime: encodeCanonicalValidTime(claim.ValidTime), Lifecycle: "active",
		})
	}
	for _, source := range proposal.Sources {
		if !source.Create {
			continue
		}
		effect.SourceLinks = append(effect.SourceLinks, canonicalSourceLink{
			SourceLinkID: source.ID, ClaimID: proposal.DestinationClaim.ID,
			Locator: canonicalLocator{EventID: source.EventID, EventPart: source.EventPart,
				LocatorKind: source.LocatorKind, LocatorValue: source.LocatorValue, EvidenceSHA256: source.EvidenceSHA256},
			Actor: source.Actor, SourceType: source.SourceType, Authority: source.Authority,
			ObservedAt: source.ObservedAt, Eligibility: memory.EligibilityEligible,
		})
	}
	effect.Promotions = []canonicalPromotionRecord{{
		SourceScopeKey: proposal.SourceScope.Key, DestinationScopeKey: proposal.DestinationScope.Key,
		SourceClaimID: proposal.SourceClaim.ID, DestinationClaimID: proposal.DestinationClaim.ID,
		EntityMappings: mappings,
	}}
	return canonicalPromotionProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Evidence.EventID}, Effect: effect,
	}
}
