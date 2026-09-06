package memory

// OwnerCandidateScopes is a bounded local-owner navigation page. Selecting a
// row still requires fresh authority for its one exact scope on every request.
type OwnerCandidateScope struct {
	ScopeKey string `json:"scope_key"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
}

type OwnerCandidateScopes struct {
	Scopes     []OwnerCandidateScope `json:"scopes"`
	NextCursor string                `json:"next_cursor"`
	Indexing   bool                  `json:"indexing"`
}

type OwnerCandidateScopeQuery struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}
