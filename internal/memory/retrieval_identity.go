package memory

// An identity reference records the accepted mapping used by an exact search.
// Alias text is resolved again through its immutable ID and original source.
type RetrievalIdentityReference struct {
	Kind     string                    `json:"kind"`
	EntityID SemanticID                `json:"entity_id"`
	AliasID  SemanticID                `json:"alias_id,omitempty"`
	Source   *RetrievalSourceReference `json:"source,omitempty"`
}

type RetrievalIdentityMatch struct {
	RetrievalIdentityReference
	AliasValue string `json:"alias_value,omitempty"`
}
