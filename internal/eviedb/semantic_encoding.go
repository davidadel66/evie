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
	PredicateID      memory.SemanticID           `json:"predicate_id"`
	Token            string                      `json:"token"`
	Version          int64                       `json:"version"`
	Label            string                      `json:"label"`
	ObjectConstraint memory.LiteralKind          `json:"object_constraint"`
	Cardinality      memory.PredicateCardinality `json:"cardinality"`
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
	Eligibility  string                    `json:"eligibility"`
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
	effect.Claims = []canonicalClaim{{
		ClaimID: proposal.ClaimID, ScopeKey: proposal.Scope.Key, SubjectEntityID: proposal.Subject.ID,
		PredicateID: proposal.Predicate.ID, PredicateToken: proposal.Predicate.Token,
		PredicateVersion: proposal.Predicate.Version, Object: canonicalClaimObject{Literal: proposal.Literal},
		Polarity: proposal.Polarity, ValidTime: canonicalValidTime{}, Lifecycle: "active",
	}}
	effect.SourceLinks = []canonicalSourceLink{{
		SourceLinkID: proposal.SourceLinkID, ClaimID: proposal.ClaimID,
		Locator: canonicalLocator{EventID: proposal.Source.EventID, EventPart: proposal.Source.EventPart,
			LocatorKind: proposal.Source.LocatorKind, LocatorValue: proposal.Source.LocatorValue,
			EvidenceSHA256: proposal.Source.EvidenceSHA256},
		Actor: proposal.Source.Actor, SourceType: proposal.Source.SourceType, Authority: proposal.Source.Authority,
		ObservedAt: proposal.Source.ObservedAt, Eligibility: "eligible",
	}}
	return canonicalProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Source.EventID}, Effect: effect,
	}
}
