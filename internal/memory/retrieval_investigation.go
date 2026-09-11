package memory

// RetrievalInvestigation records bounded execution/accounting outcomes, never
// a query, interpretation rationale or hidden model reasoning.
type RetrievalInvestigation struct {
	Version               string   `json:"version"`
	SearchAttempts        int      `json:"search_attempts"`
	RefreshAttempts       int      `json:"refresh_attempts"`
	ReusedEvidence        int      `json:"reused_evidence"`
	CumulativeMemoryBytes int      `json:"cumulative_memory_bytes"`
	KernelWorkNanoseconds int64    `json:"kernel_work_ns"`
	Outcomes              []string `json:"outcomes"`
}
