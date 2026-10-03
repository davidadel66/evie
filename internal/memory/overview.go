package memory

import "fmt"

// Memory overview (docs/memory-architecture.md, Stage 3): one bounded read
// answers "what do you know about me / this project" with the standing set
// plus the current profile or Workspace memories, so the model needs one
// call, not a search.

// MemoryOverviewScope selects the overview's sections.
type MemoryOverviewScope string

const (
	MemoryOverviewProfile   MemoryOverviewScope = "profile"
	MemoryOverviewWorkspace MemoryOverviewScope = "workspace"
	MemoryOverviewAll       MemoryOverviewScope = "all"
)

func (s MemoryOverviewScope) Valid() bool {
	return s == MemoryOverviewProfile || s == MemoryOverviewWorkspace || s == MemoryOverviewAll
}

const (
	// MemoryOverviewSectionLimit bounds each section's listed memories.
	MemoryOverviewSectionLimit = 30
	// MemoryOverviewTextLimit bounds one memory's text in bytes; longer text
	// is shortened visibly and marked.
	MemoryOverviewTextLimit = 400
)

// MemoryOverviewItem is one current, approved memory: the Claim's own text,
// never source text.
type MemoryOverviewItem struct {
	ClaimID   SemanticID `json:"claim_id"`
	Applies   string     `json:"applies"`
	ScopeKey  string     `json:"scope_key"`
	Kind      ClaimKind  `json:"kind"`
	Standing  bool       `json:"standing"`
	Memory    string     `json:"memory"`
	Shortened bool       `json:"shortened,omitempty"`
}

// MemoryOverviewSection lists current memories that do not always apply.
// Omitted counts current memories beyond the bound; Withheld counts items the
// secret fence removed.
type MemoryOverviewSection struct {
	Items    []MemoryOverviewItem `json:"items"`
	Omitted  int                  `json:"omitted"`
	Withheld int                  `json:"withheld"`
}

// MemoryOverview is the bounded answer. Standing holds what applies to every
// request in the selected sections, in rendering order. Profile is Global and
// this conversation; Workspace is the conversation's Workspace or project,
// absent in a Global conversation. PendingReview counts proposals awaiting
// the owner's review in these scopes; their content is not included.
type MemoryOverview struct {
	Scope            MemoryOverviewScope    `json:"scope"`
	Standing         []MemoryOverviewItem   `json:"standing"`
	StandingWithheld int                    `json:"standing_withheld"`
	Profile          *MemoryOverviewSection `json:"profile,omitempty"`
	Workspace        *MemoryOverviewSection `json:"workspace,omitempty"`
	WorkspaceNote    string                 `json:"workspace_note,omitempty"`
	PendingReview    int                    `json:"pending_review"`
}

// MemoryOverviewQuery selects sections; an empty scope means all.
type MemoryOverviewQuery struct {
	Scope MemoryOverviewScope
}

func (q MemoryOverviewQuery) Normalize() (MemoryOverviewQuery, error) {
	if q.Scope == "" {
		q.Scope = MemoryOverviewAll
	}
	if !q.Scope.Valid() {
		return q, fmt.Errorf("unsupported overview scope %q; use profile, workspace or all", q.Scope)
	}
	return q, nil
}

// ShortenOverviewText bounds one memory's text at a UTF-8 boundary.
func ShortenOverviewText(text string) (string, bool) {
	if len(text) <= MemoryOverviewTextLimit {
		return text, false
	}
	cut := MemoryOverviewTextLimit
	for cut > 0 && (text[cut]&0xC0) == 0x80 {
		cut--
	}
	return text[:cut] + "…", true
}
