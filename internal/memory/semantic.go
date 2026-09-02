package memory

import "time"

type SemanticID string

type LiteralKind string
type PredicateCardinality string
type ClaimPolarity string
type SemanticActor string
type SemanticSourceType string
type SourceAuthority string
type EvidencePart string
type EvidenceLocatorKind string

const (
	LiteralText     LiteralKind = "text"
	LiteralInteger  LiteralKind = "integer"
	LiteralDecimal  LiteralKind = "decimal"
	LiteralBoolean  LiteralKind = "boolean"
	LiteralDate     LiteralKind = "date"
	LiteralDatetime LiteralKind = "datetime"

	CardinalityOne  PredicateCardinality = "one"
	CardinalityMany PredicateCardinality = "many"

	PolarityAffirmed ClaimPolarity = "affirmed"
	PolarityDenied   ClaimPolarity = "denied"

	SemanticActorOwner      SemanticActor      = "owner"
	SourceTypeUserMessage   SemanticSourceType = "user_message"
	AuthorityOwnerStatement SourceAuthority    = "owner_statement"

	EvidenceContent EvidencePart = "content"
	EvidencePayload EvidencePart = "payload"

	LocatorWhole         EvidenceLocatorKind = "whole"
	LocatorUTF8ByteRange EvidenceLocatorKind = "utf8_byte_range"
	LocatorJSONPointer   EvidenceLocatorKind = "json_pointer"
)

type TypedLiteral struct {
	Kind  LiteralKind `json:"kind"`
	Value string      `json:"value"`
}

type SemanticScope struct {
	ID         SemanticID `json:"scope_id"`
	Key        string     `json:"scope_key"`
	RegistryID string     `json:"registry_id,omitempty"`
	Revision   int64      `json:"revision"`
}

type ScopeRevision struct {
	ScopeKey string `json:"scope_key"`
	Revision int64  `json:"revision"`
}

type SemanticPredicate struct {
	ID               SemanticID           `json:"predicate_id"`
	Token            string               `json:"token"`
	Version          int64                `json:"version"`
	Label            string               `json:"label"`
	ObjectConstraint LiteralKind          `json:"object_constraint"`
	Cardinality      PredicateCardinality `json:"cardinality"`
	Create           bool                 `json:"create"`
}

type SemanticEntity struct {
	ID            SemanticID `json:"entity_id"`
	ScopeKey      string     `json:"scope_key"`
	CanonicalName string     `json:"canonical_name"`
	EntityType    string     `json:"entity_type"`
	AnchorKind    string     `json:"anchor_kind"`
	Create        bool       `json:"create"`
}

type EvidenceLocator struct {
	EventID        EventID             `json:"event_id"`
	EventPart      EvidencePart        `json:"event_part"`
	LocatorKind    EvidenceLocatorKind `json:"locator_kind"`
	LocatorValue   string              `json:"locator_value"`
	EvidenceSHA256 string              `json:"evidence_sha256"`
}

type SemanticSource struct {
	ID             SemanticID          `json:"source_link_id,omitempty"`
	EventID        EventID             `json:"event_id"`
	SessionID      SessionID           `json:"session_id"`
	ScopeKey       string              `json:"source_scope_key"`
	EventPart      EvidencePart        `json:"event_part"`
	LocatorKind    EvidenceLocatorKind `json:"locator_kind"`
	LocatorValue   string              `json:"locator_value"`
	EvidenceSHA256 string              `json:"evidence_sha256"`
	Actor          SemanticActor       `json:"actor"`
	SourceType     SemanticSourceType  `json:"source_type"`
	Authority      SourceAuthority     `json:"authority"`
	ObservedAt     string              `json:"observed_at"`
	Evidence       string              `json:"evidence"`
}

type ValidTime struct {
	From *time.Time `json:"from"`
	To   *time.Time `json:"to"`
}

type RememberLiteralRequest struct {
	IdempotencyKey string
	SourceEventID  EventID
	Predicate      string
	PredicateLabel string
	Literal        TypedLiteral
}

type RememberLiteralProposal struct {
	SchemaVersion    int               `json:"schema_version"`
	Kind             string            `json:"kind"`
	OperationID      SemanticID        `json:"operation_id"`
	IdempotencyKey   string            `json:"idempotency_key"`
	Actor            SemanticActor     `json:"actor"`
	SessionID        SessionID         `json:"session_id"`
	Scope            SemanticScope     `json:"scope"`
	Scopes           []SemanticScope   `json:"scopes"`
	PriorRevisions   []ScopeRevision   `json:"prior_revisions"`
	ExpectedRevision int64             `json:"expected_revision"`
	Predicate        SemanticPredicate `json:"predicate"`
	Subject          SemanticEntity    `json:"subject"`
	Evie             SemanticEntity    `json:"evie"`
	ClaimID          SemanticID        `json:"claim_id"`
	SourceLinkID     SemanticID        `json:"source_link_id"`
	Literal          TypedLiteral      `json:"literal"`
	Polarity         ClaimPolarity     `json:"polarity"`
	ValidTime        ValidTime         `json:"valid_time"`
	Source           SemanticSource    `json:"source"`
	ProposalSHA256   string            `json:"-"`
}

type RememberLiteralResult struct {
	OperationID        SemanticID      `json:"operation_id"`
	ClaimID            SemanticID      `json:"claim_id"`
	SourceLinkID       SemanticID      `json:"source_link_id"`
	TransactionTime    time.Time       `json:"transaction_time"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
	ScopeRevision      int64           `json:"scope_revision"`
}

type LiteralClaimInspection struct {
	ID              SemanticID        `json:"claim_id"`
	Scope           SemanticScope     `json:"scope"`
	Subject         SemanticEntity    `json:"subject"`
	Predicate       SemanticPredicate `json:"predicate"`
	Literal         TypedLiteral      `json:"literal"`
	Polarity        ClaimPolarity     `json:"polarity"`
	ValidTime       ValidTime         `json:"valid_time"`
	OperationID     SemanticID        `json:"operation_id"`
	TransactionTime time.Time         `json:"transaction_time"`
	EffectiveAt     time.Time         `json:"effective_at"`
	Source          SemanticSource    `json:"source"`
}

type LiteralClaimsInspection struct {
	Scope         SemanticScope            `json:"scope"`
	ScopeRevision int64                    `json:"scope_revision"`
	EffectiveAt   time.Time                `json:"effective_at"`
	Claims        []LiteralClaimInspection `json:"claims"`
}
