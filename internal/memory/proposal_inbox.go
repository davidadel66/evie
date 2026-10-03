package memory

import (
	"strings"
	"time"
	"unicode"
)

// Proactive memory proposals (docs/memory-architecture.md, Stage 2). Evie
// proposes what is worth remembering without being asked. A proposal the
// owner explicitly asked for shows the approval card as before; any other
// proposal is queued, exactly as prepared, in the owner's review inbox and
// applies only when the owner approves it there.

// MemoryProposalReason records why a remember proposal went to the inbox.
type MemoryProposalReason string

const (
	// MemoryProposalNotRequested: the model proposed the memory on its own
	// (requested_by_owner false).
	MemoryProposalNotRequested MemoryProposalReason = "not_requested"
	// MemoryProposalNoExplicitCue: the model said the owner asked, but the
	// owner's current message carries no explicit memory cue.
	MemoryProposalNoExplicitCue MemoryProposalReason = "no_explicit_cue"
)

func (r MemoryProposalReason) Valid() bool {
	return r == MemoryProposalNotRequested || r == MemoryProposalNoExplicitCue || r == MemoryProposalBackground
}

// Inbox tools: only the two remember tools create new memories, so only
// they route proactive in-turn proposals to the inbox. The background pass
// also queues corrections and retirements (CorrectClaimToolName,
// RetireToolName).
const (
	RememberLiteralToolName = "memory_remember_literal"
	RememberEntityToolName  = "memory_remember_entity"
)

// explicitMemoryCueWords and explicitMemoryCuePhrases are the closed English
// cues for an explicit request to remember. A phrase matches as consecutive
// words after case folding and dropping apostrophes ("don't" is "dont").
var (
	explicitMemoryCueWords = map[string]bool{
		"remember": true, "memorize": true, "memorise": true, "save": true,
		"note": true, "jot": true, "store": true,
	}
	explicitMemoryCuePhrases = [][]string{
		{"keep", "in", "mind"}, {"dont", "forget"}, {"do", "not", "forget"},
		{"to", "memory"}, {"to", "your", "memory"}, {"in", "your", "memory"},
	}
)

// HasExplicitMemoryCue reports whether an owner message contains an explicit
// memory cue: "remember", "memorize"/"memorise", "save", "note", "jot",
// "store", "keep in mind", "don't forget"/"do not forget", or "to (your)
// memory"/"in your memory". It is the harness's half of the explicit-request
// rule: the model must also say the owner asked (requested_by_owner). A cue
// alone does not make a request, so "Do you remember…?" is a cue the model
// does not treat as a request.
func HasExplicitMemoryCue(message string) bool {
	words := explicitCueWords(message)
	for i, word := range words {
		if explicitMemoryCueWords[word] {
			return true
		}
		for _, phrase := range explicitMemoryCuePhrases {
			if i+len(phrase) > len(words) {
				continue
			}
			matched := true
			for j, part := range phrase {
				if words[i+j] != part {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	return false
}

func explicitCueWords(message string) []string {
	folded := strings.NewReplacer("'", "", "’", "", "‘", "").Replace(strings.ToLower(message))
	return strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// QueuedMemoryProposal is one exact prepared proposal routed to the review
// inbox. Exactly one of Literal, Entity, Correction and Lifecycle is set, the
// one ToolName prepares. In-turn proposals are remembers citing their tool
// intent; background proposals (Reason MemoryProposalBackground) cite no
// intent and may also be a correction or a retirement.
type QueuedMemoryProposal struct {
	ToolName      string
	IntentEventID EventID
	Reason        MemoryProposalReason
	Literal       *RememberLiteralProposal
	Entity        *RememberEntityProposal
	Correction    *CorrectClaimProposal
	Lifecycle     *MemoryLifecycleProposal
}

// MemoryProposalQueued is what queueing reports to the model. Duplicate means
// an identical memory already awaits review, so nothing new was queued.
type MemoryProposalQueued struct {
	ProposalID string `json:"proposal_id"`
	Duplicate  bool   `json:"duplicate"`
}

// MemoryProposalItem is one pending inbox proposal as the owner reviews it.
// Proposal is the exact prepared proposal JSON, the same document an approval
// card shows, revalidated against current memory when the inbox is read.
// Approvable is false when it can no longer apply; Problem says why.
type MemoryProposalItem struct {
	ProposalID     string               `json:"proposal_id"`
	SessionID      SessionID            `json:"session_id"`
	SessionTitle   string               `json:"session_title"`
	ScopeKey       string               `json:"scope_key"`
	ToolName       string               `json:"tool_name"`
	Reason         MemoryProposalReason `json:"reason"`
	QueuedAt       time.Time            `json:"queued_at"`
	Revision       int64                `json:"revision"`
	Proposal       string               `json:"proposal"`
	PreparedSHA256 string               `json:"prepared_sha256"`
	Approvable     bool                 `json:"approvable"`
	Problem        string               `json:"problem,omitempty"`
	// Change describes what approving does in plain terms: add a memory,
	// update (correct) an existing one, or retire one.
	Change *MemoryProposalChange `json:"change,omitempty"`
}

// MemoryProposalChange is the owner-facing summary of an inbox proposal.
// Current is the existing memory's Claim text for an update or retirement;
// Replacement the new Claim text of an update. Evidence is the owner's words
// the change cites (the bound span, or the whole request message for a
// retirement), empty when the value is Evie-proposed.
type MemoryProposalChange struct {
	Action       string `json:"action"`
	Current      string `json:"current,omitempty"`
	Replacement  string `json:"replacement,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
	EvieProposed bool   `json:"evie_proposed,omitempty"`
}

// Inbox change actions.
const (
	MemoryChangeAdd    = "add"
	MemoryChangeUpdate = "update"
	MemoryChangeRetire = "retire"
)

// MemoryProposalPage lists pending proposals, oldest first. Pending counts
// every pending proposal, including any beyond the listed bound.
type MemoryProposalPage struct {
	Items   []MemoryProposalItem `json:"items"`
	Pending int                  `json:"pending"`
}

// MemoryProposalApproval approves the exact proposal the owner saw, with the
// "Always applies" switch as set. PreparedSHA256 must be the listed hash.
type MemoryProposalApproval struct {
	ProposalID     string `json:"proposal_id"`
	PreparedSHA256 string `json:"prepared_sha256"`
	AlwaysApplies  bool   `json:"always_applies"`
}

// MemoryProposalResult is the accepted operation an inbox approval applied.
type MemoryProposalResult struct {
	ProposalID     string     `json:"proposal_id"`
	OperationID    SemanticID `json:"operation_id"`
	ClaimID        SemanticID `json:"claim_id"`
	Standing       bool       `json:"standing"`
	ProposalSHA256 string     `json:"proposal_sha256"`
	PreparedSHA256 string     `json:"prepared_sha256"`
}
