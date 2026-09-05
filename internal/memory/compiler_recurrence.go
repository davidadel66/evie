package memory

// CompilerEquivalencePolicyV2 explicitly pins complete deterministic recurrence
// comparison. It changes generation identity without changing accepted memory.
const CompilerEquivalencePolicyV2 = "full-effect-equivalence-v2"

// CandidateLineage is an inspection response, never part of a review preview
// or accepted operation. Origin preserves the actual owner edit/resolution;
// Original extraction bytes remain on the independently retained candidate.
type CandidateLineage struct {
	Candidate         OwnerCandidate       `json:"candidate"`
	Generation        CompilerGeneration   `json:"generation"`
	Selection         CompilationSelection `json:"selection"`
	ComparisonVersion string               `json:"comparison_version"`
	Relationship      string               `json:"relationship"`
	Suppressed        bool                 `json:"suppressed"`
	Checked           *CandidateRef        `json:"checked,omitempty"`
	CheckedState      string               `json:"checked_state,omitempty"`
	Origin            *OwnerCandidate      `json:"origin,omitempty"`
	Decision          *ReviewDecision      `json:"decision,omitempty"`
	Resolution        *ReviewResult        `json:"resolution,omitempty"`
	OriginRedacted    bool                 `json:"origin_redacted"`
}
