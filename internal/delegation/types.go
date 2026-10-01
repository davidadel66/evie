// Package delegation defines durable Kernel execution records, separate from Tasks.
package delegation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

const CapabilityID = "subagents.research"
const ToolName = "delegate_research"

// ReportCapabilityID lets a parent page the full stored reports of the
// children it delegated.
const ReportCapabilityID = "subagents.report"
const ReportToolName = "read_subagent_report"

var ErrConflict = errors.New("subagent idempotency key conflicts with an existing assignment")
var ErrCapacity = errors.New("subagent concurrency capacity is occupied")
var ErrAuthority = errors.New("subagent authority is no longer available")
var ErrPolicy = errors.New("subagent execution policy limit")

// Terminal states and reasons added on 2026-10-01. A child that reaches 90%
// of its time or token budget, or the turn step limit, makes one tool-free
// wrap-up call and ends partial with that report; the reason names the budget.
const (
	StatePartial = "partial"

	WrapUpTime   = "time_budget"
	WrapUpTokens = "token_budget"
	WrapUpSteps  = "step_limit"

	// ReasonWrapUpFailed is a failed attempt whose wrap-up response still
	// requested tools, so no report was committed.
	ReasonWrapUpFailed = "wrap_up_failed"
)

// WrapUpPercent is the share of the time or token budget after which the
// child's next model call is its tool-free wrap-up.
const WrapUpPercent = 90

type Assignment struct {
	Key       string `json:"idempotency_key"`
	Objective string `json:"objective"`
	Context   string `json:"context,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
}

type Policy struct {
	PerParent int `json:"per_parent"`
	Runtime   int `json:"runtime"`
	MaxBatch  int `json:"max_batch"`
	// PerTurn bounds the children one parent turn may start across all of
	// its delegation calls.
	PerTurn  int           `json:"per_turn,omitempty"`
	Deadline time.Duration `json:"deadline"`
	// TokenBudget bounds each child's input plus output tokens, as reported
	// by the provider or, when unreported, estimated from request size.
	TokenBudget     int64 `json:"token_budget,omitempty"`
	AssignmentBytes int   `json:"assignment_bytes"`
	RequestBytes    int   `json:"request_bytes"`
	ResultBytes     int   `json:"result_bytes"`
	// Limits retired on 2026-10-01. They are never set for new attempts and
	// are kept only so policies pinned by earlier attempts round-trip.
	LegacyModelCalls   int `json:"model_calls,omitempty"`
	LegacyOutputTokens int `json:"output_tokens,omitempty"`
}

func DefaultPolicy() Policy {
	return Policy{PerParent: 2, Runtime: 4, MaxBatch: 8, PerTurn: 16, Deadline: 15 * time.Minute, TokenBudget: 1_000_000, AssignmentBytes: 8192, RequestBytes: 1 << 20, ResultBytes: 12000}
}
func (p Policy) Validate() error {
	if p.PerParent <= 0 || p.Runtime <= 0 || p.MaxBatch <= 0 || p.PerTurn <= 0 || p.Deadline <= 0 || p.TokenBudget <= 0 || p.AssignmentBytes <= 0 || p.RequestBytes <= 0 || p.ResultBytes < 512 {
		return errors.New("subagent policy requires finite positive limits (result_bytes >= 512)")
	}
	if p.LegacyModelCalls != 0 || p.LegacyOutputTokens != 0 {
		return errors.New("subagent model-call and output-token limits were retired; budget children by deadline and token_budget")
	}
	if p.ResultBytes > 96*1024 || p.MaxBatch > (96*1024-2)/(p.ResultBytes+1) {
		return errors.New("subagent batch result envelope exceeds 96 KiB; lower max_batch or result_bytes")
	}
	return nil
}
func (p Policy) ValidateBatch(a []Assignment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if len(a) == 0 || len(a) > p.MaxBatch {
		return errors.New("subagent batch size exceeds policy")
	}
	keys := map[string]bool{}
	for _, v := range a {
		if strings.TrimSpace(v.Key) == "" || len(v.Key) > 128 || keys[v.Key] || strings.TrimSpace(v.Objective) == "" || len(v.Objective)+len(v.Context) > p.AssignmentBytes || len(v.TaskID) > 128 {
			return errors.New("invalid or oversized subagent assignment")
		}
		keys[v.Key] = true
	}
	return nil
}

// TurnLimitError refuses a delegation that would start more children in one
// parent turn than the policy allows.
type TurnLimitError struct{ Limit, Started, Requested int }

func (e *TurnLimitError) Error() string {
	return fmt.Sprintf("subagent per-turn limit: at most %d children per parent turn (%d already started this turn, %d more requested)", e.Limit, e.Started, e.Requested)
}
func (e *TurnLimitError) Unwrap() error { return ErrPolicy }

func Digest(v any) string {
	b, _ := json.Marshal(v)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

type Parent struct {
	// Invocation-only resolved profile. Recovery never restarts workers;
	// accepted child context snapshots record the model and effective limits.
	Profile       *openrouter.ContextProfile `json:"-"`
	Scope         memory.ScopeContext
	Lease         memory.TurnLease
	SourceEventID memory.EventID
	IntentEventID memory.EventID
}

// Result is what the parent receives inline. The full report stays in the
// child session and is paged with read_subagent_report.
type Result struct {
	ExecutionID    string           `json:"execution_id"`
	ChildSessionID memory.SessionID `json:"child_session_id"`
	Status         string           `json:"status"`
	// Summary is the report's Summary section, or its beginning when the
	// report has none. SummaryTruncated marks a summary cut to the result
	// limit; ReportBytes is the full report's size.
	Summary          string `json:"summary,omitempty"`
	SummaryTruncated bool   `json:"summary_truncated,omitempty"`
	ReportBytes      int    `json:"report_bytes,omitempty"`
	// Findings is the inline answer of results recorded before 2026-10-01.
	Findings       string   `json:"findings,omitempty"`
	Sources        []Source `json:"sources,omitempty"`
	UnverifiedURLs []string `json:"unverified_urls,omitempty"`
	Limitations    []string `json:"limitations,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Usage          *Usage   `json:"usage"`
}

// Source is a URL the child's own Web tool events returned. Fetched means the
// child read it with web_fetch; otherwise only a web_search result listed it.
// Cited means the report contains it.
type Source struct {
	URL     string
	Fetched bool
	Cited   bool
	// legacy marks a pre-2026-10-01 source: a URL found in the findings
	// text, never checked against tool events. It renders as it was stored.
	legacy bool
}

type sourceWire struct {
	URL     string `json:"url"`
	Fetched bool   `json:"fetched"`
	Cited   bool   `json:"cited"`
}

func (s Source) MarshalJSON() ([]byte, error) {
	if s.legacy {
		return json.Marshal(s.URL)
	}
	return json.Marshal(sourceWire{URL: s.URL, Fetched: s.Fetched, Cited: s.Cited})
}

func (s *Source) UnmarshalJSON(b []byte) error {
	var url string
	if json.Unmarshal(b, &url) == nil {
		*s = Source{URL: url, legacy: true}
		return nil
	}
	var wire sourceWire
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	*s = Source{URL: wire.URL, Fetched: wire.Fetched, Cited: wire.Cited}
	return nil
}

// Usage sums the provider-reported usage of the child's committed responses.
// Incomplete marks a lower bound: some response or provider call that the
// child made has no reported usage.
type Usage struct {
	memory.TokenUsage
	Incomplete bool `json:"incomplete,omitempty"`
}

// ReportPage is one bounded page of a child's stored final report.
type ReportPage struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	TotalBytes  int    `json:"total_bytes"`
	Offset      int    `json:"offset"`
	End         int    `json:"end"`
	NextOffset  *int   `json:"next_offset,omitempty"`
	Text        string `json:"text"`
}

// WrapUp records, before the child's tool-free wrap-up call, which budget
// triggered it, so an accepted report recovers as partial after a crash.
type WrapUp struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

type Attempt struct {
	DispatchID   string              `json:"dispatch_id"`
	ID           string              `json:"id"`
	Parent       Parent              `json:"parent"`
	Assignment   Assignment          `json:"assignment"`
	Digest       string              `json:"digest"`
	Child        memory.Session      `json:"child"`
	Receipt      composition.Receipt `json:"receipt"`
	Policy       Policy              `json:"policy"`
	PolicyID     string              `json:"policy_id"`
	State        string              `json:"state"`
	CreatedAt    time.Time           `json:"created_at"`
	StartedAt    *time.Time          `json:"started_at,omitempty"`
	EndedAt      *time.Time          `json:"ended_at,omitempty"`
	FinalEventID memory.EventID      `json:"final_event_id,omitempty"`
	WrapUp       *WrapUp             `json:"wrap_up,omitempty"`
	Result       *Result             `json:"result,omitempty"`
}

func (a Attempt) Terminal() bool { return a.State != "admitted" && a.State != "running" }
