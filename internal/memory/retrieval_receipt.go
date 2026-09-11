package memory

// RetrievalReceipt is the content-free record of evidence supplied in one
// provider request. It is part of the fenced context-snapshot append, not a
// second episode or a claim that the model cited or used the evidence.
type RetrievalReceipt struct {
	Interpretation *RetrievalInterpretation `json:"interpretation,omitempty"`
	Version        string                   `json:"version"`
	Status         string                   `json:"status"`
	Evidence       []RetrievalReference     `json:"evidence"`
}

// Interpretation diagnostics contain only measured bounds and versioned
// outcomes, never query text, summary contents, or hidden model reasoning.
type RetrievalInterpretation struct {
	Version         string `json:"version"`
	Outcome         string `json:"outcome"`
	CurrentBytes    int    `json:"current_bytes"`
	EarlierMessages int    `json:"earlier_messages"`
	EarlierBytes    int    `json:"earlier_bytes"`
	SummaryBytes    int    `json:"summary_bytes"`
	QueryBytes      int    `json:"query_bytes"`
}
