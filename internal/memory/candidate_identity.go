package memory

// CompilerIdentityPolicyV2 admits sourced unresolved references and explicit
// Predicate proposals. It never changes the owner-assertion evidence policy.
const CompilerIdentityPolicyV2 = "identity-review-v2"

// EntityMention preserves an unresolved name. Support must name one of the
// candidate's exact supporting projections and contain the name verbatim.
type EntityMention struct {
	Name       string          `json:"name"`
	EntityType string          `json:"entity_type"`
	Support    EvidenceLocator `json:"support"`
}

type PredicateDefinition struct {
	Token            string                    `json:"token"`
	Label            string                    `json:"label"`
	ObjectConstraint PredicateObjectConstraint `json:"object_constraint"`
	Cardinality      PredicateCardinality      `json:"cardinality"`
}

type CandidateIdentityProposal struct {
	Subject     *EntityMention       `json:"subject"`
	Object      *EntityMention       `json:"object"`
	Predicate   *PredicateDefinition `json:"predicate"`
	Uncertainty string               `json:"uncertainty"`
	Confidence  *float64             `json:"confidence"`
}

type ReviewEntityAlternative struct {
	Entity  SemanticEntity  `json:"entity"`
	Aliases []SemanticAlias `json:"aliases"`
	Context []SemanticClaim `json:"context"`
}

type ReviewIdentityOptions struct {
	Candidate      CandidateRef              `json:"candidate"`
	ScopeKey       string                    `json:"scope_key"`
	ScopeRevisions []ScopeRevision           `json:"scope_revisions"`
	Subject        []ReviewEntityAlternative `json:"subject"`
	Object         []ReviewEntityAlternative `json:"object"`
	Predicates     []SemanticPredicate       `json:"predicates"`
	SHA256         string                    `json:"options_sha256"`
}

type ReviewEntityChoice struct {
	EntityID SemanticID `json:"entity_id"`
	Create   bool       `json:"create"`
}

type ReviewPredicateChoice struct {
	PredicateID SemanticID `json:"predicate_id"`
	Create      bool       `json:"create"`
}

type ReviewIdentityChoices struct {
	Subject   *ReviewEntityChoice    `json:"subject"`
	Object    *ReviewEntityChoice    `json:"object"`
	Predicate *ReviewPredicateChoice `json:"predicate"`
}

type ReviewIdentityDecision struct {
	Candidate     CandidateRef          `json:"candidate"`
	OptionsSHA256 string                `json:"options_sha256"`
	Choices       ReviewIdentityChoices `json:"choices"`
}

// Identity revisions are immutable owner interpretations, not factual sources.
type ReviewIdentityRevision struct {
	Revision              int64                 `json:"revision"`
	ParentRevision        int64                 `json:"parent_revision"`
	ReviewRevision        int64                 `json:"review_revision"`
	AuditID               string                `json:"audit_id"`
	OwnerID               OwnerID               `json:"owner_id"`
	AuthenticationBinding string                `json:"authentication_binding"`
	AuthorizationRevision int64                 `json:"authorization_revision"`
	Options               ReviewIdentityOptions `json:"options"`
	Choices               ReviewIdentityChoices `json:"choices"`
}

type ReviewIdentityEffect struct {
	Revision ReviewIdentityRevision `json:"revision"`
	Aliases  []SemanticAlias        `json:"aliases"`
}
