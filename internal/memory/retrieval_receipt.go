package memory

// RetrievalReceipt is the content-free record of evidence supplied in one
// provider request. It is part of the fenced context-snapshot append, not a
// second episode or a claim that the model cited or used the evidence.
type RetrievalReceipt struct {
	Version  string               `json:"version"`
	Status   string               `json:"status"`
	Evidence []RetrievalReference `json:"evidence"`
}
