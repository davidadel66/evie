// Package delegation defines durable Kernel execution records, separate from Tasks.
package delegation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

// ContinueCapabilityID lets a parent extend a child it delegated, resuming
// the child's own session with a follow-up message (added 2026-10-01).
const ContinueCapabilityID = "subagents.continue"
const ContinueToolName = "continue_research"

// ExploreCapabilityID lets a parent delegate local, read-heavy work to
// read-only explore workers (Stage 7 of docs/memory-architecture.md, added
// 2026-10-03). Their results use the delegate_research contract; reports are
// paged with read_subagent_report and children extended with
// continue_research.
const ExploreCapabilityID = "subagents.explore"
const ExploreToolName = "delegate_explore"

// BuildCapabilityID lets a parent delegate a bounded code change on one Task
// to a build worker whose writes are fenced to its own git worktree and
// branch (Stage 11 of docs/memory-architecture.md, added 2026-10-03). Its
// results use the delegate_research contract plus the harness-recorded
// branch, commits and files changed.
const BuildCapabilityID = "subagents.build"
const BuildToolName = "delegate_build"

// Worker presets a child can be composed from. A child's worker is its pinned
// receipt's preset identity.
const (
	WorkerResearch = "research"
	WorkerExplore  = "explore"
	WorkerBuild    = "build"
)

// ExploreToolNames are the explore preset's tools in schema order: read_file
// confined to the worker's root, the Kernel's read-only query_db, and a bash
// that runs only vetted read-only commands. Each is a narrower form of a
// parent tool of the same name.
var ExploreToolNames = []string{"read_file", "query_db", "bash"}

// BuildKernelToolNames are the build preset's Kernel tools in schema order,
// each confined to the attempt's worktree: read_file, edit_file and
// write_file, then a bash that runs only under an OS sandbox. A host without
// a usable sandbox composes build workers without bash.
var BuildKernelToolNames = []string{"read_file", "edit_file", "write_file", "bash"}

// BuildParentToolNames are the parent tools a build worker's tools are
// confined forms of; a parent without them cannot delegate build work.
var BuildParentToolNames = []string{"read_file", "edit_file", "bash"}

// BuildTaskCapabilityIDs are the Todo Capabilities pinned in a build child,
// in order. With the Task Access Grant the harness issues on its Task, they
// let the worker read, claim, update and decompose that Task's subtree only.
var BuildTaskCapabilityIDs = []string{"todo.get", "todo.update", "todo.decompose", "todo.claim", "todo.release"}

// DelegationToolName is the tool whose committed intent admits a fresh child
// of worker.
func DelegationToolName(worker string) string {
	switch worker {
	case WorkerExplore:
		return ExploreToolName
	case WorkerBuild:
		return BuildToolName
	}
	return ToolName
}

// AdmitsChildren reports whether a committed parent tool intent can authorize
// child execution: a delegation batch or a continuation.
func AdmitsChildren(tool string) bool {
	return tool == ToolName || tool == ExploreToolName || tool == BuildToolName || tool == ContinueToolName
}

var ErrConflict = errors.New("subagent idempotency key conflicts with an existing assignment")
var ErrCapacity = errors.New("subagent concurrency capacity is occupied")
var ErrAuthority = errors.New("subagent authority is no longer available")
var ErrPolicy = errors.New("subagent execution policy limit")

// ErrNotFound is an execution that does not exist or belongs to another
// session; the two are deliberately indistinguishable.
var ErrNotFound = errors.New("no delegated execution with this ID belongs to this session")

// ErrNotResumable refuses a continuation of an attempt that is still running
// or has no report to continue from.
var ErrNotResumable = errors.New("subagent execution cannot be continued")

// Terminal states and reasons added on 2026-10-01. A child that reaches 90%
// of its time or token budget, or the turn step limit, makes one tool-free
// wrap-up call and ends partial with that report; the reason names the budget.
const (
	StatePartial = "partial"

	WrapUpTime   = "time_budget"
	WrapUpTokens = "token_budget"
	WrapUpSteps  = "step_limit"
	// WrapUpContext is a child whose next request, after tool-result
	// projection, would use WrapUpPercent of its usable request budget.
	WrapUpContext = "context_budget"

	// ReasonWrapUpFailed is a failed attempt whose wrap-up produced no
	// report: the response still requested tools, or even the smallest
	// wrap-up request could not fit the context budget.
	ReasonWrapUpFailed = "wrap_up_failed"

	// Failure reasons that name the limit reached. Together they replace
	// the catch-all policy_limit for new attempts (2026-10-01); results
	// stored with policy_limit keep it.
	//
	// ReasonTokenBudgetSpent: the token budget was spent and a further call
	// that was not the wrap-up was needed.
	ReasonTokenBudgetSpent = "token_budget_spent"
	// ReasonContextLimit: a request did not fit the child's request_bytes
	// limit or the model's context window.
	ReasonContextLimit = "context_limit"
	// ReasonResponseTooLarge: a model response exceeded request_bytes.
	ReasonResponseTooLarge = "response_too_large"
)

// WrapUpPercent is the share of the time, token or context budget after which
// the child's next model call is its tool-free wrap-up.
const WrapUpPercent = 90

type Assignment struct {
	Key       string `json:"idempotency_key"`
	Objective string `json:"objective"`
	Context   string `json:"context,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	// Path is an explore assignment's declared folder: inside the parent's
	// Workspace folder or project root, or absolute when the parent has
	// neither. Research assignments have none, so their digests are
	// unchanged (added 2026-10-03).
	Path string `json:"path,omitempty"`
	// Base is a build assignment's base branch or commit; empty means the
	// repository's current branch. Other workers have none, so their
	// digests are unchanged (added 2026-10-03, Stage 11).
	Base string `json:"base,omitempty"`
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

// Validate names every setting that is not a finite positive limit (amended
// 2026-10-01: errors name the setting and its bound).
func (p Policy) Validate() error {
	var invalid []string
	for _, setting := range []struct {
		name     string
		positive bool
	}{
		{"per_parent", p.PerParent > 0}, {"runtime", p.Runtime > 0}, {"max_batch", p.MaxBatch > 0},
		{"per_turn", p.PerTurn > 0}, {"deadline", p.Deadline > 0}, {"token_budget", p.TokenBudget > 0},
		{"assignment_bytes", p.AssignmentBytes > 0}, {"request_bytes", p.RequestBytes > 0},
	} {
		if !setting.positive {
			invalid = append(invalid, setting.name)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("subagent policy requires finite positive limits; invalid: %s", strings.Join(invalid, ", "))
	}
	if p.ResultBytes < MinResultBytes {
		return fmt.Errorf("subagent policy result_bytes is %s; minimum %s", Grouped(p.ResultBytes), Grouped(MinResultBytes))
	}
	if p.LegacyModelCalls != 0 || p.LegacyOutputTokens != 0 {
		return errors.New("subagent model-call and output-token limits were retired; budget children by deadline and token_budget")
	}
	if p.ResultBytes > BatchEnvelopeBytes || p.MaxBatch > (BatchEnvelopeBytes-2)/(p.ResultBytes+1) {
		return fmt.Errorf("subagent batch result envelope exceeds 96 KiB: max_batch %d × result_bytes %s; lower max_batch or result_bytes", p.MaxBatch, Grouped(p.ResultBytes))
	}
	return nil
}

// Bounds of one delegation call. A batch of MaxBatch results, each at most
// ResultBytes as the parent reads it, fits BatchEnvelopeBytes, which stays
// under the agent loop's 100 KiB tool-result admission boundary.
const (
	MinResultBytes     = 512
	BatchEnvelopeBytes = 96 * 1024
	// MaxKeyBytes bounds idempotency keys, Task identities and execution IDs.
	MaxKeyBytes = 128
	// MaxPathBytes bounds an explore assignment's declared path.
	MaxPathBytes = 1024
	// MaxBaseBytes bounds a build assignment's base branch or commit.
	MaxBaseBytes = 200
)

// ValidateBatch checks a delegate_research request before any child or model
// provider is touched. Every violation names its assignment (by key, or by
// position when the key itself is unusable), the field and the limit.
func (p Policy) ValidateBatch(a []Assignment) error {
	return p.validateBatch(ToolName, a)
}

// ValidateExploreBatch checks a delegate_explore request like ValidateBatch,
// and bounds each declared path.
func (p Policy) ValidateExploreBatch(a []Assignment) error {
	return p.validateBatch(ExploreToolName, a)
}

// ValidateBuildBatch checks a delegate_build request like ValidateBatch:
// every build assignment names the Task it works on, and its optional base
// is bounded.
func (p Policy) ValidateBuildBatch(a []Assignment) error {
	return p.validateBatch(BuildToolName, a)
}

// ValidateWorkerBatch checks a batch for worker's delegation tool.
func (p Policy) ValidateWorkerBatch(worker string, a []Assignment) error {
	return p.validateBatch(DelegationToolName(worker), a)
}

func (p Policy) validateBatch(tool string, a []Assignment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if len(a) == 0 {
		return fmt.Errorf("%s requires at least one assignment", tool)
	}
	if len(a) > p.MaxBatch {
		return fmt.Errorf("%s has %d assignments; limit %d per call", tool, len(a), p.MaxBatch)
	}
	var problems []string
	first := map[string]int{}
	for i, v := range a {
		n := i + 1
		name := fmt.Sprintf("assignment %d", n)
		switch {
		case strings.TrimSpace(v.Key) == "":
			problems = append(problems, fmt.Sprintf("assignment %d idempotency_key is blank", n))
		case len(v.Key) > MaxKeyBytes:
			problems = append(problems, fmt.Sprintf("assignment %d idempotency_key is %s bytes; limit %d", n, Grouped(len(v.Key)), MaxKeyBytes))
		case first[v.Key] != 0:
			problems = append(problems, fmt.Sprintf("assignments %d and %d share idempotency_key %q; each assignment needs its own key", first[v.Key], n, v.Key))
		default:
			first[v.Key] = n
			name = fmt.Sprintf("assignment %q", v.Key)
		}
		if strings.TrimSpace(v.Objective) == "" {
			problems = append(problems, name+" objective is blank")
		}
		if size := len(v.Objective) + len(v.Context); size > p.AssignmentBytes {
			problems = append(problems, fmt.Sprintf("%s objective plus context is %s bytes; limit %s", name, Grouped(size), Grouped(p.AssignmentBytes)))
		}
		if len(v.TaskID) > MaxKeyBytes {
			problems = append(problems, fmt.Sprintf("%s task_id is %s bytes; limit %d", name, Grouped(len(v.TaskID)), MaxKeyBytes))
		}
		switch {
		case tool != ExploreToolName && v.Path != "":
			problems = append(problems, fmt.Sprintf("%s path is not supported by %s", name, tool))
		case len(v.Path) > MaxPathBytes:
			problems = append(problems, fmt.Sprintf("%s path is %s bytes; limit %s", name, Grouped(len(v.Path)), Grouped(MaxPathBytes)))
		}
		switch {
		case tool != BuildToolName && v.Base != "":
			problems = append(problems, fmt.Sprintf("%s base is not supported by %s", name, tool))
		case len(v.Base) > MaxBaseBytes:
			problems = append(problems, fmt.Sprintf("%s base is %s bytes; limit %d", name, Grouped(len(v.Base)), MaxBaseBytes))
		}
		if tool == BuildToolName && strings.TrimSpace(v.TaskID) == "" {
			problems = append(problems, name+" task_id is blank; a build worker works on one Task you created for it")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid %s request: %s", tool, strings.Join(problems, "; "))
	}
	return nil
}

// ValidateContinuation bounds a continue_research request before any child
// or model provider is touched. The message has the assignment byte limit.
func (p Policy) ValidateContinuation(executionID, message string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	switch {
	case strings.TrimSpace(executionID) == "":
		return errors.New("continue_research execution_id is blank; use the execution_id of a research result from this conversation")
	case len(executionID) > MaxKeyBytes:
		return fmt.Errorf("continue_research execution_id is %s bytes; limit %d", Grouped(len(executionID)), MaxKeyBytes)
	case strings.TrimSpace(message) == "":
		return errors.New("continue_research message is blank")
	case len(message) > p.AssignmentBytes:
		return fmt.Errorf("continue_research message is %s bytes; limit %s", Grouped(len(message)), Grouped(p.AssignmentBytes))
	}
	return nil
}

// Grouped formats n with thousands separators, as limits are stated to the
// parent model ("9,214 bytes; limit 8,192").
func Grouped[N ~int | ~int64](n N) string {
	digits := strconv.FormatInt(int64(n), 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return sign + b.String()
}

// Continuation is a parent's continue_research request.
type Continuation struct {
	ExecutionID string `json:"execution_id"`
	Message     string `json:"message"`
}

// ContinuationKey is the idempotency key of the continuation admitted for one
// committed parent tool intent, so a retry of that intent finds it again.
func ContinuationKey(intent memory.EventID) string { return "continue_research:" + string(intent) }

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

// Result is the retained outcome of one attempt. The parent reads it through
// ParentView, which frames everything the child wrote as untrusted data. The
// full report stays in the child session and is paged with
// read_subagent_report.
type Result struct {
	ExecutionID    string           `json:"execution_id"`
	ChildSessionID memory.SessionID `json:"child_session_id"`
	// Worker names a child that is not a research child ("explore"); results
	// of research children leave it empty, as stored before it existed
	// (added 2026-10-03).
	Worker string `json:"worker,omitempty"`
	// ContinuesExecutionID is the attempt a continuation extended.
	ContinuesExecutionID string `json:"continues_execution_id,omitempty"`
	Status               string `json:"status"`
	// Replayed marks a result this call did not produce: its idempotency key
	// (or continuation intent) resolved to an attempt an earlier call admitted.
	// CompletedAt is when that attempt ended. Both are set only on delivery
	// and never stored (added 2026-10-01).
	Replayed    bool       `json:"replayed,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
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
	// Limitations are child-written: the bullets of the report's Limitations
	// section. Results stored before the Stage 10 amendment of 2026-10-01 mix
	// harness notes into them, or hold only harness notes, so the parent view
	// frames all of them as child output.
	Limitations []string `json:"limitations,omitempty"`
	// Notes are harness-written: why the attempt stopped, what was cut from
	// this inline result, and how to continue or retry (added 2026-10-01).
	Notes  []string `json:"notes,omitempty"`
	Reason string   `json:"reason,omitempty"`
	// Error replaces the outcome of one batch member whose result could not
	// be settled or delivered; its siblings are unaffected. Status is
	// StatusError. It is set only on delivery and never stored.
	Error string `json:"error,omitempty"`
	Usage *Usage `json:"usage"`
	// Build is a build child's harness-recorded branch, commits, files
	// changed and commands run (added 2026-10-03, Stage 11). Results of other
	// workers leave it empty and stay byte-identical.
	Build *BuildReport `json:"build,omitempty"`
}

// Worktree is a build attempt's harness-owned git worktree (Stage 11). The
// harness resolves every field before admission and pins it on the attempt,
// so recovery can find, resume or clean the worktree after a crash. Path and
// Temp are inside Repo's .evie folder, which the harness adds to the
// repository's local info/exclude; nothing there is ever tracked.
type Worktree struct {
	// Repo is the owner's checkout (the repository's top level), canonical.
	Repo string `json:"repo"`
	// CommonDir is the repository's common git directory and GitDir this
	// worktree's private one. The harness runs git with these explicitly, so
	// a worktree's .git file never decides which repository it writes.
	CommonDir string `json:"common_dir"`
	GitDir    string `json:"git_dir"`
	Path      string `json:"path"`
	// Temp is the attempt's private temporary folder, the only place
	// besides Path where its sandboxed bash can write.
	Temp       string `json:"temp"`
	Branch     string `json:"branch"`
	Base       string `json:"base"`
	BaseCommit string `json:"base_commit"`
	// RemovedAt is when the owner cleaned the worktree up.
	RemovedAt *time.Time `json:"removed_at,omitempty"`
}

// BuildReport is what Evie's harness recorded about a build attempt: its
// branch and base, the commits the harness made on the branch, the files
// they changed against the base, and the bash commands the worker ran with
// their exit statuses. Commands are worker-written text, so the parent view
// frames them; everything else is harness-derived (file paths are listed
// only when they are plain).
type BuildReport struct {
	Branch     string        `json:"branch"`
	Base       string        `json:"base"`
	BaseCommit string        `json:"base_commit"`
	Worktree   string        `json:"worktree"`
	Head       string        `json:"head,omitempty"`
	Commits    []BuildCommit `json:"commits,omitempty"`
	Files      []BuildFile   `json:"files,omitempty"`
	// FilesChanged counts every changed file, listed or not.
	FilesChanged int `json:"files_changed"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
	// Uncommitted marks changes left in the worktree that the harness did
	// not commit; CommitError says why, in harness words.
	Uncommitted     bool           `json:"uncommitted,omitempty"`
	CommitError     string         `json:"commit_error,omitempty"`
	Commands        []BuildCommand `json:"commands,omitempty"`
	CommandsOmitted int            `json:"commands_omitted,omitempty"`
}

// BuildCommit is one commit on a build branch after its base.
type BuildCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// BuildFile is one file the branch changed against its base, as git's
// numstat reports it. Binary files have no line counts.
type BuildFile struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Binary  bool   `json:"binary,omitempty"`
}

// BuildCommand is one bash command the worker ran and how it ended:
// "exit" with its status, or "timeout", "refused" or "error".
type BuildCommand struct {
	Command    string `json:"command"`
	Outcome    string `json:"outcome"`
	ExitStatus int    `json:"exit_status,omitempty"`
}

// StatusError is the delivered status of a batch member whose result could
// not be settled or delivered (added 2026-10-01). It is not a stored state.
const StatusError = "error"

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
	// Worker is set for a child that is not a research child, as in Result.
	Worker     string `json:"worker,omitempty"`
	Status     string `json:"status"`
	TotalBytes int    `json:"total_bytes"`
	Offset     int    `json:"offset"`
	End        int    `json:"end"`
	NextOffset *int   `json:"next_offset,omitempty"`
	Text       string `json:"text"`
}

// WrapUp records, before the child's tool-free wrap-up call, which budget
// triggered it, so an accepted report recovers as partial after a crash.
type WrapUp struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// Continues links a continuation attempt to the attempt it extended. The
// continuation runs a new turn in the same child session; AfterSequence is
// the last child event before it, so its report, usage and fetched pages are
// the events after that.
type Continues struct {
	ExecutionID   string `json:"execution_id"`
	AfterSequence int64  `json:"after_sequence"`
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
	Continues    *Continues          `json:"continues,omitempty"`
	// Root is an explore attempt's pinned folder, resolved by the harness at
	// admission and confining its read-only tools (added 2026-10-03).
	Root string `json:"root,omitempty"`
	// Worktree is a build attempt's pinned git worktree; a continuation
	// inherits it (added 2026-10-03, Stage 11).
	Worktree *Worktree `json:"worktree,omitempty"`
	Result   *Result   `json:"result,omitempty"`
}

// Worker is the attempt's worker preset, from its pinned receipt.
func (a Attempt) Worker() string { return a.Receipt.Preset.ID }

// Resumable reports whether a continuation may extend this attempt: it
// finished with an accepted report, as succeeded or partial.
func (a Attempt) Resumable() bool {
	return (a.State == "succeeded" || a.State == StatePartial) && a.FinalEventID != ""
}

func (a Attempt) Terminal() bool { return a.State != "admitted" && a.State != "running" }
