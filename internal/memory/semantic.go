package memory

import "time"

type SemanticID string

type LiteralKind string
type PredicateObjectConstraint string
type PredicateCardinality string
type ClaimPolarity string
type ClaimConflictCode string
type SourceEligibility string
type SemanticStateValue string
type SemanticActor string
type SemanticSourceType string
type SourceAuthority string
type EvidencePart string
type EvidenceLocatorKind string
type CorrectionMode string
type SemanticObjectKind string
type MemoryLifecycleAction string
type SemanticObjectStatus string
type GraphRelation string

const (
	LiteralText      LiteralKind               = "text"
	LiteralInteger   LiteralKind               = "integer"
	LiteralDecimal   LiteralKind               = "decimal"
	LiteralBoolean   LiteralKind               = "boolean"
	LiteralDate      LiteralKind               = "date"
	LiteralDatetime  LiteralKind               = "datetime"
	ConstraintEntity PredicateObjectConstraint = "entity"

	CardinalityOne  PredicateCardinality = "one"
	CardinalityMany PredicateCardinality = "many"

	PolarityAffirmed ClaimPolarity = "affirmed"
	PolarityDenied   ClaimPolarity = "denied"

	ConflictOppositePolarity ClaimConflictCode  = "opposite_polarity"
	ConflictOneCardinality   ClaimConflictCode  = "one_cardinality_overlap"
	EligibilityEligible      SourceEligibility  = "eligible"
	EligibilityRetracted     SourceEligibility  = "retracted"
	SemanticStateActive      SemanticStateValue = "active"
	SemanticStateRetired     SemanticStateValue = "retired"
	SemanticStateSuperseded  SemanticStateValue = "superseded"
	SemanticStateEligible    SemanticStateValue = "eligible"
	SemanticStateRetracted   SemanticStateValue = "retracted"

	SemanticActorOwner      SemanticActor      = "owner"
	SourceTypeUserMessage   SemanticSourceType = "user_message"
	AuthorityOwnerStatement SourceAuthority    = "owner_statement"

	EvidenceContent EvidencePart = "content"
	EvidencePayload EvidencePart = "payload"

	LocatorWhole         EvidenceLocatorKind = "whole"
	LocatorUTF8ByteRange EvidenceLocatorKind = "utf8_byte_range"
	LocatorJSONPointer   EvidenceLocatorKind = "json_pointer"

	CorrectionError   CorrectionMode = "error"
	CorrectionChanged CorrectionMode = "changed"

	SemanticObjectEntity     SemanticObjectKind = "entity"
	SemanticObjectAlias      SemanticObjectKind = "alias"
	SemanticObjectClaim      SemanticObjectKind = "claim"
	SemanticObjectSourceLink SemanticObjectKind = "source_link"
	SemanticObjectGraphLink  SemanticObjectKind = "graph_link"

	GraphRelationDerivation     GraphRelation = "derivation"
	GraphRelationGeneralization GraphRelation = "generalization"
	GraphRelationContradiction  GraphRelation = "contradiction"

	LifecycleRetire        MemoryLifecycleAction = "retire"
	LifecycleRestore       MemoryLifecycleAction = "restore"
	LifecycleRetractSource MemoryLifecycleAction = "retract_source"
	LifecycleRestoreSource MemoryLifecycleAction = "restore_source"

	SemanticStatusActive          SemanticObjectStatus = "active"
	SemanticStatusRetired         SemanticObjectStatus = "retired"
	SemanticStatusSuperseded      SemanticObjectStatus = "superseded"
	SemanticStatusUnsupported     SemanticObjectStatus = "unsupported"
	SemanticStatusEligible        SemanticObjectStatus = "eligible"
	SemanticStatusSourceRetracted SemanticObjectStatus = "source_retracted"
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
	ID               SemanticID                `json:"predicate_id"`
	Token            string                    `json:"token"`
	Version          int64                     `json:"version"`
	Label            string                    `json:"label"`
	ObjectConstraint PredicateObjectConstraint `json:"object_constraint"`
	Cardinality      PredicateCardinality      `json:"cardinality"`
	Create           bool                      `json:"create"`
}

type SemanticEntity struct {
	ID            SemanticID `json:"entity_id"`
	ScopeKey      string     `json:"scope_key"`
	CanonicalName string     `json:"canonical_name"`
	EntityType    string     `json:"entity_type"`
	AnchorKind    string     `json:"anchor_kind"`
	Create        bool       `json:"create"`
}

type SemanticAlias struct {
	ID              SemanticID `json:"alias_id"`
	EntityID        SemanticID `json:"entity_id"`
	ScopeKey        string     `json:"scope_key"`
	Value           string     `json:"value"`
	NormalizedValue string     `json:"normalized_value"`
	OperationID     SemanticID `json:"operation_id,omitempty"`
	SourceEventID   EventID    `json:"source_event_id"`
	Create          bool       `json:"create"`
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
	OperationID    SemanticID          `json:"operation_id,omitempty"`
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
	Eligibility    SourceEligibility   `json:"eligibility"`
	Create         bool                `json:"create"`
}

type ValidTime struct {
	From *time.Time `json:"from"`
	To   *time.Time `json:"to"`
}

// ClaimObject is the closed union used by correction and exact-query APIs.
// Exactly one of EntityID and Literal is present.
type ClaimObject struct {
	EntityID SemanticID    `json:"entity_id,omitempty"`
	Literal  *TypedLiteral `json:"literal,omitempty"`
}

type ClaimProposition struct {
	SubjectEntityID SemanticID    `json:"subject_entity_id"`
	PredicateID     SemanticID    `json:"predicate_id"`
	Object          ClaimObject   `json:"object"`
	Polarity        ClaimPolarity `json:"polarity"`
}

type SemanticClaim struct {
	ID                 SemanticID        `json:"claim_id"`
	ScopeKey           string            `json:"scope_key"`
	SubjectEntityID    SemanticID        `json:"subject_entity_id"`
	Predicate          SemanticPredicate `json:"predicate"`
	Object             ClaimObject       `json:"object"`
	Polarity           ClaimPolarity     `json:"polarity"`
	ValidTime          ValidTime         `json:"valid_time"`
	CreatedOperationID SemanticID        `json:"created_operation_id"`
	TransactionTime    time.Time         `json:"transaction_time"`
}

type SemanticTransition struct {
	ObjectKind string             `json:"object_kind"`
	ObjectID   SemanticID         `json:"object_id"`
	State      SemanticStateValue `json:"state"`
}

type CorrectionValidTimeEffect struct {
	OldBefore   ValidTime `json:"old_before"`
	OldAfter    ValidTime `json:"old_after"`
	Replacement ValidTime `json:"replacement"`
}

type CorrectClaimRequest struct {
	IdempotencyKey       string
	SourceEventID        EventID
	OldClaimID           SemanticID
	Replacement          ClaimProposition
	Mode                 CorrectionMode
	EffectiveTime        *time.Time
	ReplacementValidTime *ValidTime
}

type CorrectClaimProposal struct {
	SchemaVersion    int                       `json:"schema_version"`
	Kind             string                    `json:"kind"`
	OperationID      SemanticID                `json:"operation_id"`
	IdempotencyKey   string                    `json:"idempotency_key"`
	Actor            SemanticActor             `json:"actor"`
	SessionID        SessionID                 `json:"session_id"`
	Scope            SemanticScope             `json:"scope"`
	Scopes           []SemanticScope           `json:"scopes"`
	PriorRevisions   []ScopeRevision           `json:"prior_revisions"`
	ExpectedRevision int64                     `json:"expected_revision"`
	OldClaim         SemanticClaim             `json:"old_claim"`
	ReplacementClaim SemanticClaim             `json:"replacement_claim"`
	Source           SemanticSource            `json:"source"`
	Mode             CorrectionMode            `json:"mode"`
	EffectiveTime    *time.Time                `json:"effective_time"`
	ValidTimeEffect  CorrectionValidTimeEffect `json:"valid_time_effect"`
	Transitions      []SemanticTransition      `json:"transitions"`
	Request          CorrectClaimRequest       `json:"request"`
	ProposalSHA256   string                    `json:"-"`
	PreparedSHA256   string                    `json:"-"`
}

type CorrectClaimResult struct {
	OperationID        SemanticID      `json:"operation_id"`
	OldClaimID         SemanticID      `json:"old_claim_id"`
	ReplacementClaimID SemanticID      `json:"replacement_claim_id"`
	SourceLinkID       SemanticID      `json:"source_link_id"`
	TransactionTime    time.Time       `json:"transaction_time"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
	ScopeRevision      int64           `json:"scope_revision"`
}

type ClaimQuery struct {
	ValidAt         *time.Time    `json:"valid_at,omitempty"`
	AsKnownAt       *time.Time    `json:"as_known_at,omitempty"`
	PredicateToken  string        `json:"predicate_token,omitempty"`
	Polarity        ClaimPolarity `json:"polarity,omitempty"`
	SubjectEntityID SemanticID    `json:"subject_entity_id,omitempty"`
	ObjectEntityID  SemanticID    `json:"object_entity_id,omitempty"`
}

// ExactReadMetadata pins one exact read to its temporal and authorization context.
type ExactReadMetadata struct {
	ValidAt        time.Time       `json:"valid_at"`
	AsKnownAt      time.Time       `json:"as_known_at"`
	AllowedScopes  []string        `json:"allowed_scopes"`
	ScopeRevisions []ScopeRevision `json:"scope_revisions"`
}

type ClaimInspection struct {
	SemanticClaim
	Scope              SemanticScope    `json:"scope"`
	Subject            SemanticEntity   `json:"subject"`
	ObjectEntity       *SemanticEntity  `json:"object_entity,omitempty"`
	Sources            []SemanticSource `json:"sources"`
	Lifecycle          []SemanticState  `json:"lifecycle"`
	EffectiveValidTime ValidTime        `json:"effective_valid_time"`
}

type ClaimsInspection struct {
	Scope          SemanticScope     `json:"scope"`
	Scopes         []SemanticScope   `json:"scopes"`
	ScopeRevisions []ScopeRevision   `json:"scope_revisions"`
	ScopeRevision  int64             `json:"scope_revision"`
	ValidAt        time.Time         `json:"valid_at"`
	AsKnownAt      time.Time         `json:"as_known_at"`
	Claims         []ClaimInspection `json:"claims"`
	AllowedScopes  []string          `json:"allowed_scopes"`
}

type GraphEndpoint struct {
	Kind SemanticObjectKind `json:"kind"`
	ID   SemanticID         `json:"id"`
}

type SemanticGraphLink struct {
	ID                 SemanticID    `json:"graph_link_id"`
	ScopeKey           string        `json:"scope_key"`
	Relation           GraphRelation `json:"relation"`
	Source             GraphEndpoint `json:"source"`
	Target             GraphEndpoint `json:"target"`
	CreatedOperationID SemanticID    `json:"created_operation_id"`
	TransactionTime    time.Time     `json:"transaction_time"`
}

type CreateGraphLinkRequest struct {
	IdempotencyKey  string
	SourceEventID   EventID
	Relation        GraphRelation
	Source          GraphEndpoint
	Target          GraphEndpoint
	UseSessionScope bool
}

type CreateGraphLinkProposal struct {
	SchemaVersion  int                       `json:"schema_version"`
	Kind           string                    `json:"kind"`
	OperationID    SemanticID                `json:"operation_id"`
	IdempotencyKey string                    `json:"idempotency_key"`
	Actor          SemanticActor             `json:"actor"`
	SessionID      SessionID                 `json:"session_id"`
	Scope          SemanticScope             `json:"scope"`
	Scopes         []SemanticScope           `json:"scopes"`
	PriorRevisions []ScopeRevision           `json:"prior_revisions"`
	Link           SemanticGraphLink         `json:"graph_link"`
	Evidence       SemanticOperationEvidence `json:"evidence"`
	Request        CreateGraphLinkRequest    `json:"request"`
	ProposalSHA256 string                    `json:"-"`
	PreparedSHA256 string                    `json:"-"`
}

type CreateGraphLinkResult struct {
	OperationID        SemanticID      `json:"operation_id"`
	GraphLinkID        SemanticID      `json:"graph_link_id"`
	TransactionTime    time.Time       `json:"transaction_time"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
	ScopeRevision      int64           `json:"scope_revision"`
}

type SemanticObjectSummary struct {
	ObjectKind SemanticObjectKind   `json:"object_kind"`
	ObjectID   SemanticID           `json:"object_id"`
	ScopeKey   string               `json:"scope_key"`
	Status     SemanticObjectStatus `json:"status"`
	Claim      *SemanticClaim       `json:"claim,omitempty"`
	GraphLink  *SemanticGraphLink   `json:"graph_link,omitempty"`
}

type SemanticObjectListQuery struct {
	ClaimQuery
	Kinds     []SemanticObjectKind `json:"kinds,omitempty"`
	Relations []GraphRelation      `json:"relations,omitempty"`
	PageSize  int                  `json:"page_size"`
	Cursor    string               `json:"cursor,omitempty"`
}

type SemanticObjectPage struct {
	Metadata   ExactReadMetadata       `json:"metadata"`
	Objects    []SemanticObjectSummary `json:"objects"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type SemanticScopeListQuery struct {
	ClaimQuery
	PageSize int    `json:"page_size"`
	Cursor   string `json:"cursor,omitempty"`
}

type SemanticScopePage struct {
	Metadata   ExactReadMetadata `json:"metadata"`
	Scopes     []SemanticScope   `json:"scopes"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type SemanticPath struct {
	Nodes []GraphEndpoint     `json:"nodes"`
	Links []SemanticGraphLink `json:"links"`
}

type SemanticTraversalQuery struct {
	ClaimQuery
	Start     GraphEndpoint   `json:"start"`
	Depth     int             `json:"depth"`
	Relations []GraphRelation `json:"relations,omitempty"`
}

type SemanticNeighborhood struct {
	Metadata ExactReadMetadata       `json:"metadata"`
	Objects  []SemanticObjectSummary `json:"objects"`
	Paths    []SemanticPath          `json:"paths"`
}

type RememberLiteralRequest struct {
	IdempotencyKey       string
	SourceEventID        EventID
	Predicate            string
	PredicateLabel       string
	PredicateCardinality PredicateCardinality
	Literal              TypedLiteral
	Polarity             ClaimPolarity
	ValidTime            ValidTime
}

type RememberLiteralProposal struct {
	SchemaVersion    int                    `json:"schema_version"`
	Kind             string                 `json:"kind"`
	OperationID      SemanticID             `json:"operation_id"`
	IdempotencyKey   string                 `json:"idempotency_key"`
	Actor            SemanticActor          `json:"actor"`
	SessionID        SessionID              `json:"session_id"`
	Scope            SemanticScope          `json:"scope"`
	Scopes           []SemanticScope        `json:"scopes"`
	PriorRevisions   []ScopeRevision        `json:"prior_revisions"`
	ExpectedRevision int64                  `json:"expected_revision"`
	Predicate        SemanticPredicate      `json:"predicate"`
	Subject          SemanticEntity         `json:"subject"`
	Evie             SemanticEntity         `json:"evie"`
	ClaimID          SemanticID             `json:"claim_id"`
	ClaimCreate      bool                   `json:"claim_create"`
	SourceLinkID     SemanticID             `json:"source_link_id"`
	Literal          TypedLiteral           `json:"literal"`
	Polarity         ClaimPolarity          `json:"polarity"`
	ValidTime        ValidTime              `json:"valid_time"`
	Source           SemanticSource         `json:"source"`
	Request          RememberLiteralRequest `json:"request"`
	ProposalSHA256   string                 `json:"-"`
	PreparedSHA256   string                 `json:"-"`
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
	Sources         []SemanticSource  `json:"sources,omitempty"`
	Lifecycle       []SemanticState   `json:"lifecycle"`
}

type SemanticState struct {
	State           SemanticStateValue `json:"state"`
	OperationID     SemanticID         `json:"operation_id"`
	ScopeRevision   int64              `json:"scope_revision"`
	TransactionTime time.Time          `json:"transaction_time"`
}

// MemoryLifecycleRequest identifies one explicit reversible lifecycle change.
// Prepare expands an Entity target into every active dependent Alias and Claim.
type MemoryLifecycleRequest struct {
	IdempotencyKey  string
	SourceEventID   EventID
	Action          MemoryLifecycleAction
	ObjectKind      SemanticObjectKind
	ObjectID        SemanticID
	UseSessionScope bool
}

type SemanticOperationEvidence struct {
	EventID        EventID            `json:"event_id"`
	SessionID      SessionID          `json:"session_id"`
	ScopeKey       string             `json:"scope_key"`
	Actor          SemanticActor      `json:"actor"`
	SourceType     SemanticSourceType `json:"source_type"`
	ObservedAt     string             `json:"observed_at"`
	EvidenceSHA256 string             `json:"evidence_sha256"`
	Evidence       string             `json:"evidence"`
}

type MemoryLifecycleProposal struct {
	SchemaVersion  int                       `json:"schema_version"`
	Kind           string                    `json:"kind"`
	OperationID    SemanticID                `json:"operation_id"`
	IdempotencyKey string                    `json:"idempotency_key"`
	Actor          SemanticActor             `json:"actor"`
	SessionID      SessionID                 `json:"session_id"`
	Scope          SemanticScope             `json:"scope"`
	Scopes         []SemanticScope           `json:"scopes"`
	PriorRevisions []ScopeRevision           `json:"prior_revisions"`
	ObjectKind     SemanticObjectKind        `json:"object_kind"`
	ObjectID       SemanticID                `json:"object_id"`
	ExpectedState  SemanticStateValue        `json:"expected_state"`
	Transitions    []SemanticTransition      `json:"transitions"`
	EffectScopes   []string                  `json:"effect_scopes"`
	Evidence       SemanticOperationEvidence `json:"evidence"`
	Request        MemoryLifecycleRequest    `json:"request"`
	ProposalSHA256 string                    `json:"-"`
	PreparedSHA256 string                    `json:"-"`
}

type MemoryLifecycleResult struct {
	OperationID        SemanticID         `json:"operation_id"`
	ObjectKind         SemanticObjectKind `json:"object_kind"`
	ObjectID           SemanticID         `json:"object_id"`
	TransactionTime    time.Time          `json:"transaction_time"`
	ResultingRevisions []ScopeRevision    `json:"resulting_revisions"`
	ScopeRevision      int64              `json:"scope_revision"`
}

type SemanticSourceInspection struct {
	Source    SemanticSource  `json:"source"`
	Lifecycle []SemanticState `json:"lifecycle"`
}

type SemanticOperationInspection struct {
	OperationID        SemanticID      `json:"operation_id"`
	SchemaVersion      int             `json:"schema_version"`
	Kind               string          `json:"kind"`
	SourceEventID      EventID         `json:"source_event_id"`
	ProposalSHA256     string          `json:"proposal_sha256"`
	EffectSHA256       string          `json:"effect_sha256"`
	ProposalJSON       string          `json:"proposal_json"`
	PreparedJSON       string          `json:"prepared_proposal_json"`
	ResultJSON         string          `json:"result_json"`
	TransactionTime    time.Time       `json:"transaction_time"`
	PriorRevisions     []ScopeRevision `json:"prior_revisions"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
}

type SemanticObjectInspection struct {
	ObjectKind SemanticObjectKind            `json:"object_kind"`
	ObjectID   SemanticID                    `json:"object_id"`
	Scope      SemanticScope                 `json:"scope"`
	Status     SemanticObjectStatus          `json:"status"`
	Entity     *SemanticEntity               `json:"entity,omitempty"`
	Alias      *SemanticAlias                `json:"alias,omitempty"`
	Claim      *SemanticClaim                `json:"claim,omitempty"`
	Source     *SemanticSource               `json:"source,omitempty"`
	GraphLink  *SemanticGraphLink            `json:"graph_link,omitempty"`
	Lifecycle  []SemanticState               `json:"lifecycle"`
	Sources    []SemanticSourceInspection    `json:"sources"`
	Operations []SemanticOperationInspection `json:"operations"`
	Metadata   ExactReadMetadata             `json:"metadata"`
}

type ClaimConflictWarning struct {
	Code           ClaimConflictCode `json:"code"`
	PredicateToken string            `json:"predicate_token"`
	ClaimIDs       []SemanticID      `json:"claim_ids"`
}

type LiteralClaimsInspection struct {
	Scope          SemanticScope            `json:"scope"`
	Scopes         []SemanticScope          `json:"scopes"`
	ScopeRevisions []ScopeRevision          `json:"scope_revisions"`
	ScopeRevision  int64                    `json:"scope_revision"`
	EffectiveAt    time.Time                `json:"effective_at"`
	ValidAt        time.Time                `json:"valid_at"`
	AsKnownAt      time.Time                `json:"as_known_at"`
	Claims         []LiteralClaimInspection `json:"claims"`
	Warnings       []ClaimConflictWarning   `json:"warnings"`
	ConflictClaims []LiteralClaimInspection `json:"conflict_claims"`
}

type EntitySelector struct {
	EntityID      SemanticID
	Create        bool
	CanonicalName string
	EntityType    string
	Alias         string
}

type RememberEntityRequest struct {
	IdempotencyKey       string
	SourceEventID        EventID
	Predicate            string
	PredicateLabel       string
	PredicateCardinality PredicateCardinality
	Polarity             ClaimPolarity
	ValidTime            ValidTime
	Subject              EntitySelector
	Object               EntitySelector
	UseSessionScope      bool
}

type SemanticEntityClaim struct {
	ID               SemanticID    `json:"claim_id"`
	ScopeKey         string        `json:"scope_key"`
	SubjectEntityID  SemanticID    `json:"subject_entity_id"`
	PredicateID      SemanticID    `json:"predicate_id"`
	PredicateToken   string        `json:"predicate_token"`
	PredicateVersion int64         `json:"predicate_version"`
	ObjectEntityID   SemanticID    `json:"object_entity_id"`
	Polarity         ClaimPolarity `json:"polarity"`
	ValidTime        ValidTime     `json:"valid_time"`
	Create           bool          `json:"create"`
}

type RememberEntityProposal struct {
	SchemaVersion      int                   `json:"schema_version"`
	Kind               string                `json:"kind"`
	OperationID        SemanticID            `json:"operation_id"`
	IdempotencyKey     string                `json:"idempotency_key"`
	Actor              SemanticActor         `json:"actor"`
	SessionID          SessionID             `json:"session_id"`
	Scope              SemanticScope         `json:"scope"`
	Scopes             []SemanticScope       `json:"scopes"`
	PriorRevisions     []ScopeRevision       `json:"prior_revisions"`
	Predicate          SemanticPredicate     `json:"predicate"`
	Entities           []SemanticEntity      `json:"entities"`
	Aliases            []SemanticAlias       `json:"aliases"`
	Claim              SemanticEntityClaim   `json:"claim"`
	Source             SemanticSource        `json:"source"`
	ResultingRevision  int64                 `json:"resulting_revision"`
	ResultingRevisions []ScopeRevision       `json:"resulting_revisions"`
	Request            RememberEntityRequest `json:"request"`
	ProposalSHA256     string                `json:"-"`
	PreparedSHA256     string                `json:"-"`
}

type RememberEntityResult struct {
	OperationID        SemanticID      `json:"operation_id"`
	ClaimID            SemanticID      `json:"claim_id"`
	SourceLinkID       SemanticID      `json:"source_link_id"`
	TransactionTime    time.Time       `json:"transaction_time"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
	ScopeRevision      int64           `json:"scope_revision"`
}

type PromotedEntity struct {
	SourceEntityID    SemanticID     `json:"source_entity_id"`
	DestinationEntity SemanticEntity `json:"destination_entity"`
}

type PromotionRequest struct {
	IdempotencyKey      string
	SourceEventID       EventID
	SourceClaimID       SemanticID
	DestinationScopeKey string
}

type PromotionProposal struct {
	SchemaVersion          int                       `json:"schema_version"`
	Kind                   string                    `json:"kind"`
	OperationID            SemanticID                `json:"operation_id"`
	IdempotencyKey         string                    `json:"idempotency_key"`
	Actor                  SemanticActor             `json:"actor"`
	SessionID              SessionID                 `json:"session_id"`
	SourceScope            SemanticScope             `json:"source_scope"`
	DestinationScope       SemanticScope             `json:"destination_scope"`
	Scopes                 []SemanticScope           `json:"scopes"`
	PriorRevisions         []ScopeRevision           `json:"prior_revisions"`
	SourceClaim            SemanticClaim             `json:"source_claim"`
	DestinationClaim       SemanticClaim             `json:"destination_claim"`
	DestinationClaimCreate bool                      `json:"destination_claim_create"`
	PromotedEntities       []PromotedEntity          `json:"promoted_entities"`
	Sources                []SemanticSource          `json:"sources"`
	Evidence               SemanticOperationEvidence `json:"evidence"`
	Request                PromotionRequest          `json:"request"`
	ProposalSHA256         string                    `json:"-"`
	PreparedSHA256         string                    `json:"-"`
}

type PromotionResult struct {
	OperationID         SemanticID      `json:"operation_id"`
	SourceClaimID       SemanticID      `json:"source_claim_id"`
	DestinationClaimID  SemanticID      `json:"destination_claim_id"`
	TransactionTime     time.Time       `json:"transaction_time"`
	ResultingRevisions  []ScopeRevision `json:"resulting_revisions"`
	DestinationRevision int64           `json:"destination_revision"`
}

type AliasEntityMatch struct {
	Entity SemanticEntity `json:"entity"`
	Alias  SemanticAlias  `json:"alias"`
}

type EntityClaimInspection struct {
	Claim           SemanticEntityClaim `json:"claim"`
	Scope           SemanticScope       `json:"scope"`
	Subject         SemanticEntity      `json:"subject"`
	Object          SemanticEntity      `json:"object"`
	Predicate       SemanticPredicate   `json:"predicate"`
	OperationID     SemanticID          `json:"operation_id"`
	TransactionTime time.Time           `json:"transaction_time"`
	Sources         []SemanticSource    `json:"sources"`
	Lifecycle       []SemanticState     `json:"lifecycle"`
}

type EntityClaimsInspection struct {
	Scope          SemanticScope           `json:"scope"`
	Scopes         []SemanticScope         `json:"scopes"`
	ScopeRevisions []ScopeRevision         `json:"scope_revisions"`
	ScopeRevision  int64                   `json:"scope_revision"`
	EffectiveAt    time.Time               `json:"effective_at"`
	ValidAt        time.Time               `json:"valid_at"`
	AsKnownAt      time.Time               `json:"as_known_at"`
	Claims         []EntityClaimInspection `json:"claims"`
	Warnings       []ClaimConflictWarning  `json:"warnings"`
	ConflictClaims []EntityClaimInspection `json:"conflict_claims"`
}
