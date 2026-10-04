package memory

import (
	"errors"
	"strings"
	"time"
)

// The always-on background pass (docs/memory-architecture.md, Stage 4 and
// decision 4). After a root conversation goes idle, after every N new owner
// messages, or once a committed compaction has summarized owner messages
// (the Stage 10 flush), Evie reads only the owner's new messages since a durable
// per-conversation cursor, extracts candidate memories, compares each with
// the most similar existing memories, and chooses ADD, UPDATE, INVALIDATE or
// NOOP (Mem0's operations, with Zep-style invalidation instead of deletion).
// Every non-NOOP result becomes an exact prepared proposal in the owner's
// review inbox; nothing is applied without approval.

// MemoryProposalBackground marks an inbox proposal the background pass found
// in the owner's conversations, rather than one Evie proposed in a turn.
const MemoryProposalBackground MemoryProposalReason = "background"

// Inbox tools for background changes to existing memory: an UPDATE is a
// correction and an INVALIDATE a retirement, prepared exactly as the
// approval card's tools prepare them.
const (
	CorrectClaimToolName = "memory_correct_claim"
	RetireToolName       = "memory_retire"
)

// BackgroundOperation is one Mem0-style decision for a candidate memory.
type BackgroundOperation string

const (
	BackgroundAdd        BackgroundOperation = "ADD"
	BackgroundUpdate     BackgroundOperation = "UPDATE"
	BackgroundInvalidate BackgroundOperation = "INVALIDATE"
	BackgroundNoop       BackgroundOperation = "NOOP"
)

// BackgroundTrigger records why a pass ran.
type BackgroundTrigger string

const (
	BackgroundTriggerIdle       BackgroundTrigger = "idle"
	BackgroundTriggerTurns      BackgroundTrigger = "turns"
	BackgroundTriggerCompaction BackgroundTrigger = "compaction"
	BackgroundTriggerManual     BackgroundTrigger = "manual"
)

func (t BackgroundTrigger) Valid() bool {
	switch t {
	case BackgroundTriggerIdle, BackgroundTriggerTurns, BackgroundTriggerCompaction, BackgroundTriggerManual:
		return true
	}
	return false
}

// BackgroundRunStatus is the outcome of one pass over one conversation.
type BackgroundRunStatus string

const (
	// BackgroundRunSucceeded committed its proposals and advanced the cursor.
	BackgroundRunSucceeded BackgroundRunStatus = "succeeded"
	// BackgroundRunFailed left the cursor in place for a later retry.
	BackgroundRunFailed BackgroundRunStatus = "failed"
	// BackgroundRunSkipped advanced the cursor past a batch whose model
	// output stayed malformed after repeated attempts; the gap is visible.
	BackgroundRunSkipped BackgroundRunStatus = "skipped"
	// BackgroundRunDeferred could not commit because a live turn held the
	// conversation; the cursor stays and the batch runs again later.
	BackgroundRunDeferred BackgroundRunStatus = "deferred"
	// BackgroundRunSuperseded found its batch already covered by another
	// pass; it is recorded only so its spend counts toward the caps.
	BackgroundRunSuperseded BackgroundRunStatus = "superseded"
)

func (s BackgroundRunStatus) Valid() bool {
	switch s {
	case BackgroundRunSucceeded, BackgroundRunFailed, BackgroundRunSkipped, BackgroundRunDeferred, BackgroundRunSuperseded:
		return true
	}
	return false
}

var (
	// ErrBackgroundCursorMoved means another pass committed this batch first.
	ErrBackgroundCursorMoved = errors.New("memory background cursor moved; another pass covered this batch")
	// ErrBackgroundDailyCap means today's token or USD cap is reached.
	ErrBackgroundDailyCap = errors.New("memory background pass paused: today's spending cap is reached")
)

// BackgroundOwnerMessage is one owner message in a pass's batch: the only
// text the pass ever sends to its model.
type BackgroundOwnerMessage struct {
	EventID    EventID   `json:"event_id"`
	Sequence   int64     `json:"sequence"`
	Content    string    `json:"-"`
	RecordedAt time.Time `json:"recorded_at"`
}

// BackgroundBatchLimits bound one batch. Through, when positive, stops the
// batch at that event sequence (the compaction seam).
type BackgroundBatchLimits struct {
	MaxMessages int
	MaxBytes    int
	Through     int64
}

// BackgroundBatch is the owner messages new since a conversation's cursor,
// bounded. Messages holding a detected secret and local slash commands are
// covered by the batch (the cursor moves past them) but never sent.
type BackgroundBatch struct {
	Session         Session                  `json:"-"`
	FromSequence    int64                    `json:"from_sequence"`
	ThroughSequence int64                    `json:"through_sequence"`
	Messages        []BackgroundOwnerMessage `json:"messages"`
	Withheld        int                      `json:"withheld"`
	Commands        int                      `json:"commands"`
	More            bool                     `json:"more"`
	// Failures and Malformed count the cursor's consecutive failed attempts
	// and how many of them were malformed model output.
	Failures  int `json:"failures"`
	Malformed int `json:"malformed"`
}

// BackgroundExistingMemory is one current memory, or one pending inbox
// proposal, a candidate is compared with. Writable is true only for a Claim
// in the conversation's own Context Scope, which is the only scope a
// correction or retirement from that conversation may change.
type BackgroundExistingMemory struct {
	ClaimID         SemanticID    `json:"claim_id,omitempty"`
	ProposalID      string        `json:"proposal_id,omitempty"`
	ScopeKey        string        `json:"scope_key"`
	Text            string        `json:"text"`
	SubjectEntityID SemanticID    `json:"subject_entity_id,omitempty"`
	PredicateID     SemanticID    `json:"predicate_id,omitempty"`
	PredicateToken  string        `json:"predicate_token"`
	PredicateLabel  string        `json:"predicate_label"`
	Literal         *TypedLiteral `json:"literal,omitempty"`
	Polarity        ClaimPolarity `json:"polarity"`
	Kind            ClaimKind     `json:"kind"`
	Standing        bool          `json:"standing"`
	Writable        bool          `json:"writable"`
	Pending         bool          `json:"pending"`
}

// BackgroundPool is everything a pass compares candidates with: current
// memories the conversation can read and pending inbox proposals in those
// scopes, bounded, plus the Predicate vocabulary already in use.
type BackgroundPool struct {
	ContextScopeKey string                     `json:"context_scope_key"`
	Memories        []BackgroundExistingMemory `json:"memories"`
	Predicates      []BackgroundPredicate      `json:"predicates"`
	Truncated       bool                       `json:"truncated"`
}

// BackgroundPredicate is one Predicate token and label already in use.
type BackgroundPredicate struct {
	Token string `json:"token"`
	Label string `json:"label"`
}

// BackgroundDueQuery selects conversations whose pass is due at Now.
type BackgroundDueQuery struct {
	Now   time.Time
	Idle  time.Duration
	Turns int
	Limit int
}

// BackgroundDue is one conversation whose pass is due, and why.
type BackgroundDue struct {
	SessionID      SessionID         `json:"session_id"`
	Cursor         int64             `json:"cursor"`
	PendingOwner   int               `json:"pending_owner_messages"`
	LastActivityAt time.Time         `json:"last_activity_at"`
	Trigger        BackgroundTrigger `json:"trigger"`
	// CompactedPending counts the owner messages past the cursor up to
	// Through, the last event a committed compaction of the conversation
	// summarized: the memory flush before compaction still owed (Stage 10).
	// Any makes the conversation due as a compaction flush, regardless of
	// the idle and turn triggers.
	CompactedPending int   `json:"compacted_pending_owner_messages,omitempty"`
	Through          int64 `json:"through_sequence,omitempty"`
}

// BackgroundRun is the durable record of one pass over one conversation.
type BackgroundRun struct {
	RunID           string              `json:"run_id"`
	SessionID       SessionID           `json:"session_id"`
	Trigger         BackgroundTrigger   `json:"trigger"`
	Status          BackgroundRunStatus `json:"status"`
	FromSequence    int64               `json:"from_sequence"`
	ThroughSequence int64               `json:"through_sequence"`
	Model           string              `json:"model"`
	InputTokens     int64               `json:"input_tokens"`
	OutputTokens    int64               `json:"output_tokens"`
	CostMicroUSD    int64               `json:"cost_micro_usd"`
	Messages        int                 `json:"messages"`
	Candidates      int                 `json:"candidates"`
	Queued          int                 `json:"queued"`
	Duplicates      int                 `json:"duplicates"`
	Noops           int                 `json:"noops"`
	Skipped         int                 `json:"skipped"`
	Malformed       bool                `json:"malformed,omitempty"`
	Error           string              `json:"error,omitempty"`
	StartedAt       time.Time           `json:"started_at"`
	FinishedAt      time.Time           `json:"finished_at"`
}

// BackgroundCommit is one pass's result: the exact prepared proposals to
// queue and the cursor move, committed together under the conversation's
// turn lease so a pass never writes while a live turn holds it.
type BackgroundCommit struct {
	Proposals []QueuedMemoryProposal
	Run       BackgroundRun
}

// BackgroundCommitResult says what queueing did with each proposal.
type BackgroundCommitResult struct {
	ProposalIDs        []string `json:"proposal_ids"`
	Queued             int      `json:"queued"`
	Duplicates         int      `json:"duplicates"`
	PreviouslyDeclined int      `json:"previously_declined"`
	AlreadyApplied     int      `json:"already_applied"`
	Refused            int      `json:"refused"`
}

// BackgroundFailure records a pass that could not commit. Malformed marks
// model output that failed strict validation; after enough consecutive
// malformed attempts the batch is skipped (visible as a skipped run) so one
// bad batch cannot stall a conversation forever.
type BackgroundFailure struct {
	Run         BackgroundRun
	NextAttempt time.Time
	SkipBatch   bool
}

// BackgroundSpend is the pass's spend since a time.
type BackgroundSpend struct {
	Since        time.Time `json:"since"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	CostMicroUSD int64     `json:"cost_micro_usd"`
}

func (s BackgroundSpend) Tokens() int64 { return s.InputTokens + s.OutputTokens }

// BackgroundSessionProgress is one conversation's cursor and latest outcome.
type BackgroundSessionProgress struct {
	SessionID     SessionID           `json:"session_id"`
	Title         string              `json:"title"`
	ScopeKey      string              `json:"scope_key"`
	Cursor        int64               `json:"cursor"`
	PendingOwner  int                 `json:"pending_owner_messages"`
	Failures      int                 `json:"failures"`
	NextAttemptAt *time.Time          `json:"next_attempt_at,omitempty"`
	LastStatus    BackgroundRunStatus `json:"last_status,omitempty"`
	LastRunAt     *time.Time          `json:"last_run_at,omitempty"`
	// CompactedPending counts the pending owner messages a committed
	// compaction already summarized: the flush still owed (Stage 10).
	CompactedPending int `json:"compacted_pending_owner_messages"`
}

// BackgroundCompactionFlush is the memory flush before compaction still
// owed across root conversations (Stage 10): owner messages up to the last
// event a committed compaction summarized that the pass has not covered.
type BackgroundCompactionFlush struct {
	Conversations        int `json:"conversations"`
	PendingOwnerMessages int `json:"pending_owner_messages"`
}

// BackgroundProgress is the durable half of the pass's status.
type BackgroundProgress struct {
	ActivatedAt        *time.Time                  `json:"activated_at,omitempty"`
	LastRun            *BackgroundRun              `json:"last_run,omitempty"`
	LastError          *BackgroundRun              `json:"last_error,omitempty"`
	Today              BackgroundSpend             `json:"today"`
	PendingProposals   int                         `json:"pending_proposals"`
	Sessions           []BackgroundSessionProgress `json:"sessions"`
	SkippedBatches     int                         `json:"skipped_batches"`
	RunsToday          int                         `json:"runs_today"`
	ProposalsQueuedDay int                         `json:"proposals_queued_today"`
	CompactionFlush    BackgroundCompactionFlush   `json:"compaction_flush"`
}

// BackgroundStatus is what `evie memory-background status` and the web
// Review area show: configuration (enabled, model, triggers, caps) from the
// running process and progress (cursors, last run, spend) from SQLite.
type BackgroundStatus struct {
	Enabled           bool               `json:"enabled"`
	State             string             `json:"state"`
	Reason            string             `json:"reason,omitempty"`
	Provider          string             `json:"provider,omitempty"`
	Model             string             `json:"model,omitempty"`
	PricingSource     string             `json:"pricing_source,omitempty"`
	IdleMinutes       int                `json:"idle_minutes"`
	TurnTrigger       int                `json:"turn_trigger"`
	PassTokenBudget   int64              `json:"pass_token_budget"`
	DailyTokenCap     int64              `json:"daily_token_cap"`
	DailyMicroUSDCap  int64              `json:"daily_micro_usd_cap"`
	RemainingTokens   int64              `json:"remaining_tokens"`
	RemainingMicroUSD int64              `json:"remaining_micro_usd"`
	Progress          BackgroundProgress `json:"progress"`
	// CompactionFlushSkipped says why an owed compaction flush is not
	// running now (the pass is disabled, or paused at today's cap); the
	// compaction itself was unaffected. Empty when nothing is skipped.
	CompactionFlushSkipped string `json:"compaction_flush_skipped,omitempty"`
}

// Background status states.
const (
	BackgroundStateOn       = "on"
	BackgroundStateDisabled = "disabled"
	BackgroundStatePaused   = "paused"
)

// BackgroundClaimText is a candidate's Claim text in the standing tier's
// form (subject — predicate: value), so it compares with existing memories
// exactly as they render.
func BackgroundClaimText(label, value string, polarity ClaimPolarity) string {
	text := "owner — " + strings.TrimSpace(label) + ": " + strings.TrimSpace(value)
	if polarity == PolarityDenied {
		text = "Denied: " + text
	}
	return text
}
