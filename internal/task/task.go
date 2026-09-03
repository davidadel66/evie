// Package task defines the Kernel-owned durable Task contract. Persistence
// implementations choose identities and authority-bearing fields; callers can
// supply only task content.
package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidInput             = errors.New("task: invalid input")
	ErrNotFound                 = errors.New("task: not found")
	ErrConflict                 = errors.New("task: revision conflict")
	ErrInvalidTransition        = errors.New("task: invalid status transition")
	ErrMissingAttribution       = errors.New("task: trusted mutation attribution is missing")
	ErrIdempotencyConflict      = errors.New("task: idempotency identity was reused for a different request")
	ErrRootCreationDenied       = errors.New("task: delegated sessions cannot create Task Tree roots")
	ErrActiveDescendants        = errors.New("task: active descendants prevent completion")
	ErrClaimHeld                = errors.New("task: claimed by another active execution")
	ErrClaimRequired            = errors.New("task: an active claim is required")
	ErrClaimNotOwned            = errors.New("task: active claim belongs to another execution")
	ErrClaimExecutionInactive   = errors.New("task: claiming execution is not active")
	ErrManagementOverrideDenied = errors.New("task: delegated sessions cannot use Task management overrides")
	ErrScopeDenied              = errors.New("task: Context Scope does not authorize this Task")
)

type ID string
type IdempotencyKey string

type Scope string

const ScopeGlobal Scope = "global"

type ScopeSelection string

const (
	ScopeSelectionDefault ScopeSelection = ""
	ScopeSelectionContext ScopeSelection = "context"
	ScopeSelectionGlobal  ScopeSelection = "global"
)

func WorkspaceScope(id string) Scope { return Scope("workspace:" + id) }
func ProjectScope(id string) Scope   { return Scope("project:" + id) }

func ValidateScopeSelection(selection ScopeSelection) error {
	switch selection {
	case ScopeSelectionDefault, ScopeSelectionContext, ScopeSelectionGlobal:
		return nil
	default:
		return &InputError{Field: "scope", Message: "must be context or global"}
	}
}

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

type Operation string

const (
	OperationCreate    Operation = "create"
	OperationUpdate    Operation = "update"
	OperationDecompose Operation = "decompose"
	OperationClaim     Operation = "claim"
	OperationRelease   Operation = "release"
)

type MutationOutcome string

const (
	MutationAccepted MutationOutcome = "accepted"
	MutationRejected MutationOutcome = "rejected"
)

type DiagnosticCode string

const (
	DiagnosticInvalidInput      DiagnosticCode = "invalid_input"
	DiagnosticInvalidTransition DiagnosticCode = "invalid_transition"
	DiagnosticRevisionConflict  DiagnosticCode = "revision_conflict"
	DiagnosticActiveDescendants DiagnosticCode = "active_descendants"
	DiagnosticClaimHeld         DiagnosticCode = "claim_held"
	DiagnosticClaimRequired     DiagnosticCode = "claim_required"
	DiagnosticClaimNotOwned     DiagnosticCode = "claim_not_owned"
	DiagnosticExecutionInactive DiagnosticCode = "execution_inactive"
)

const (
	DefaultTreeDepth      = 8
	MaxTreeDepth          = 64
	MaxTreeNodes          = 1000
	MaxDecomposeChildren  = 100
	MaxResultSummaryBytes = 4000
)

// Task is the durable state of one owner-controlled unit of work.
type Task struct {
	ID            ID        `json:"id"`
	ParentID      ID        `json:"parent_id,omitempty"`
	RootID        ID        `json:"root_id"`
	SiblingOrder  uint64    `json:"sibling_order,omitempty"`
	Scope         Scope     `json:"scope"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	DueDate       string    `json:"due_date,omitempty"`
	ResultSummary string    `json:"result_summary,omitempty"`
	Status        Status    `json:"status"`
	Revision      uint64    `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CreateInput deliberately excludes persistence, scope identities, owner,
// actor, and session identities. Scope is only a bounded context/global
// selection; the trusted Kernel derives the stable identity.
type CreateInput struct {
	Title                  string
	Description            string
	Priority               int
	DueDate                string
	ParentID               ID
	ExpectedParentRevision uint64
	IdempotencyKey         IdempotencyKey
	Scope                  ScopeSelection
}

type ChildInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
}

type DecomposeInput struct {
	ExpectedRevision uint64
	Children         []ChildInput
	IdempotencyKey   IdempotencyKey
}

type Decomposition struct {
	Parent   Task   `json:"parent"`
	Children []Task `json:"children"`
}

type Claim struct {
	ID        string    `json:"id"`
	TaskID    ID        `json:"task_id"`
	ClaimedAt time.Time `json:"claimed_at"`
}

type ClaimInput struct {
	IdempotencyKey IdempotencyKey
}

type ReleaseInput struct {
	IdempotencyKey IdempotencyKey
}

type ClaimRelease struct {
	Claim      Claim     `json:"claim"`
	ReleasedAt time.Time `json:"released_at"`
	Reason     string    `json:"reason"`
}

type TreeQuery struct {
	MaxDepth       int
	IncludeHistory bool
}

type Tree struct {
	Task      Task   `json:"task"`
	Children  []Tree `json:"children"`
	Truncated bool   `json:"truncated,omitempty"`
}

// UpdateInput is a compare-and-swap patch. Pointer fields distinguish omission
// from explicitly clearing optional metadata.
type UpdateInput struct {
	ExpectedRevision uint64
	Title            *string
	Description      *string
	Priority         *int
	DueDate          *string
	ResultSummary    *string
	Status           *Status
	IdempotencyKey   IdempotencyKey
}

// ListFilter defaults to open Tasks when neither Statuses nor IncludeHistory
// is supplied. IncludeHistory returns every retained lifecycle state.
type ListFilter struct {
	Statuses       []Status
	IncludeHistory bool
	RootID         ID
	ParentID       ID
	Scope          ScopeSelection
	// FocusID is a trusted session projection anchor. Model-facing schemas do
	// not expose it; ordinary callers select focus through the session API.
	FocusID ID
	// Limit is a trusted projection bound and is not model-facing.
	Limit int
}

// MutationAttribution is installed by the trusted runtime immediately before
// tool execution. It is intentionally absent from model-facing arguments.
type MutationAttribution struct {
	ActorID         string
	SessionID       string
	RunID           string
	ParentSessionID string
	WorkspaceID     string
	ProjectID       string
	LeaseHolderID   string
	LeaseToken      uint64
	LeaseGeneration uint64
}

type Event struct {
	ID                 string          `json:"id"`
	TaskID             ID              `json:"task_id"`
	Sequence           uint64          `json:"sequence"`
	Operation          Operation       `json:"operation"`
	ActorID            string          `json:"actor_id"`
	SessionID          string          `json:"session_id"`
	RunID              string          `json:"run_id"`
	RecordedAt         time.Time       `json:"recorded_at"`
	PreviousRevision   uint64          `json:"previous_revision"`
	ResultingRevision  uint64          `json:"resulting_revision"`
	Outcome            MutationOutcome `json:"outcome"`
	DiagnosticCode     DiagnosticCode  `json:"diagnostic_code,omitempty"`
	IdempotencySHA256  string          `json:"idempotency_sha256,omitempty"`
	ClaimID            string          `json:"claim_id,omitempty"`
	ClaimReason        string          `json:"claim_reason,omitempty"`
	ManagementOverride bool            `json:"management_override,omitempty"`
	ManagementReason   string          `json:"management_reason,omitempty"`
}

// Service is the focused Task persistence boundary consumed by first-party
// plugins. Implementations own identity generation and recovery policy.
type Service interface {
	CreateGlobalTask(context.Context, CreateInput) (Task, error)
	ListOpenGlobalTasks(context.Context) ([]Task, error)
	ListGlobalTasks(context.Context, ListFilter) ([]Task, error)
	GetGlobalTask(context.Context, ID) (Task, error)
	GetGlobalTaskTree(context.Context, ID, TreeQuery) (Tree, error)
	UpdateGlobalTask(context.Context, ID, UpdateInput) (Task, error)
	DecomposeGlobalTask(context.Context, ID, DecomposeInput) (Decomposition, error)
	ClaimGlobalTask(context.Context, ID, ClaimInput) (Claim, error)
	ReleaseGlobalTaskClaim(context.Context, ID, ReleaseInput) (ClaimRelease, error)
	GetGlobalTaskClaim(context.Context, ID) (Claim, bool, error)
}

type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string {
	return fmt.Sprintf("invalid task %s: %s", e.Field, e.Message)
}

func (e *InputError) Unwrap() error { return ErrInvalidInput }

type NotFoundError struct {
	ID ID
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("task %q not found", e.ID) }

func (e *NotFoundError) Unwrap() error { return ErrNotFound }

type ConflictError struct {
	ID       ID
	Expected uint64
	Current  uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("task %q revision conflict: expected %d, current %d", e.ID, e.Expected, e.Current)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

type TransitionError struct {
	From Status
	To   Status
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("task status cannot transition from %q to %q", e.From, e.To)
}

func (e *TransitionError) Unwrap() error { return ErrInvalidTransition }

type IdempotencyConflictError struct {
	Operation Operation
}

func (e *IdempotencyConflictError) Error() string {
	return fmt.Sprintf("%s: %s", ErrIdempotencyConflict, e.Operation)
}

func (e *IdempotencyConflictError) Unwrap() error { return ErrIdempotencyConflict }

type ActiveDescendantsError struct {
	ID ID
}

func (e *ActiveDescendantsError) Error() string {
	return fmt.Sprintf("%s: task %q", ErrActiveDescendants, e.ID)
}

func (e *ActiveDescendantsError) Unwrap() error { return ErrActiveDescendants }

type ClaimHeldError struct{ TaskID ID }

func (e *ClaimHeldError) Error() string { return fmt.Sprintf("%s: task %q", ErrClaimHeld, e.TaskID) }
func (e *ClaimHeldError) Unwrap() error { return ErrClaimHeld }

type ClaimRequiredError struct{ TaskID ID }

func (e *ClaimRequiredError) Error() string {
	return fmt.Sprintf("%s: task %q", ErrClaimRequired, e.TaskID)
}
func (e *ClaimRequiredError) Unwrap() error { return ErrClaimRequired }

type ClaimNotOwnedError struct{ TaskID ID }

func (e *ClaimNotOwnedError) Error() string {
	return fmt.Sprintf("%s: task %q", ErrClaimNotOwned, e.TaskID)
}
func (e *ClaimNotOwnedError) Unwrap() error { return ErrClaimNotOwned }

type ClaimExecutionInactiveError struct{ TaskID ID }

func (e *ClaimExecutionInactiveError) Error() string {
	return fmt.Sprintf("%s: task %q", ErrClaimExecutionInactive, e.TaskID)
}
func (e *ClaimExecutionInactiveError) Unwrap() error { return ErrClaimExecutionInactive }

func ValidateCreateInput(input CreateInput) error {
	if err := ValidateScopeSelection(input.Scope); err != nil {
		return err
	}
	if err := validateTaskContent("", ChildInput{
		Title: input.Title, Description: input.Description, Priority: input.Priority, DueDate: input.DueDate,
	}); err != nil {
		return err
	}
	if input.ParentID == "" && input.ExpectedParentRevision != 0 {
		return &InputError{Field: "expected_parent_revision", Message: "must be zero when parent_id is omitted"}
	}
	if input.ParentID != "" {
		if input.Scope != ScopeSelectionDefault {
			return &InputError{Field: "scope", Message: "must be omitted when parent_id is supplied"}
		}
		if strings.TrimSpace(string(input.ParentID)) == "" {
			return &InputError{Field: "parent_id", Message: "must not be blank"}
		}
		if input.ExpectedParentRevision == 0 {
			return &InputError{Field: "expected_parent_revision", Message: "must be greater than zero for a child Task"}
		}
	}
	return ValidateIdempotencyKey(input.IdempotencyKey)
}

func ValidateDecomposeInput(input DecomposeInput) error {
	if input.ExpectedRevision == 0 {
		return &InputError{Field: "expected_revision", Message: "must be greater than zero"}
	}
	if len(input.Children) == 0 {
		return &InputError{Field: "children", Message: "must contain at least one child"}
	}
	if len(input.Children) > MaxDecomposeChildren {
		return &InputError{Field: "children", Message: fmt.Sprintf("must not exceed %d children", MaxDecomposeChildren)}
	}
	for i, child := range input.Children {
		if err := validateTaskContent(fmt.Sprintf("children[%d].", i), child); err != nil {
			return err
		}
	}
	return ValidateIdempotencyKey(input.IdempotencyKey)
}

func validateTaskContent(prefix string, input ChildInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return &InputError{Field: prefix + "title", Message: "must not be blank"}
	}
	if input.Priority < 0 || input.Priority > 5 {
		return &InputError{Field: prefix + "priority", Message: "must be zero or between one and five"}
	}
	if input.DueDate != "" {
		parsed, err := time.Parse("2006-01-02", input.DueDate)
		if err != nil || parsed.Format("2006-01-02") != input.DueDate {
			return &InputError{Field: prefix + "due", Message: "must be a valid YYYY-MM-DD owner-local date"}
		}
	}
	return nil
}

func ValidateTreeQuery(query TreeQuery) error {
	if query.MaxDepth < 0 || query.MaxDepth > MaxTreeDepth {
		return &InputError{Field: "max_depth", Message: fmt.Sprintf("must be between zero and %d", MaxTreeDepth)}
	}
	return nil
}

func ValidateClaimInput(input ClaimInput) error { return ValidateIdempotencyKey(input.IdempotencyKey) }

func ValidateReleaseInput(input ReleaseInput) error {
	return ValidateIdempotencyKey(input.IdempotencyKey)
}

func ValidateClaimAttribution(value MutationAttribution) error {
	if strings.TrimSpace(value.ActorID) == "" || strings.TrimSpace(value.SessionID) == "" ||
		strings.TrimSpace(value.RunID) == "" || strings.TrimSpace(value.LeaseHolderID) == "" ||
		value.LeaseToken == 0 || value.LeaseGeneration == 0 {
		return ErrMissingAttribution
	}
	return nil
}

func ValidateUpdateInput(input UpdateInput) error {
	if input.ExpectedRevision == 0 {
		return &InputError{Field: "expected_revision", Message: "must be greater than zero"}
	}
	if input.Title == nil && input.Description == nil && input.Priority == nil && input.DueDate == nil &&
		input.ResultSummary == nil && input.Status == nil {
		return &InputError{Field: "patch", Message: "must change metadata, result, or lifecycle status"}
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return &InputError{Field: "title", Message: "must not be blank"}
	}
	if input.Priority != nil && (*input.Priority < 0 || *input.Priority > 5) {
		return &InputError{Field: "priority", Message: "must be zero or between one and five"}
	}
	if input.DueDate != nil && *input.DueDate != "" {
		parsed, err := time.Parse("2006-01-02", *input.DueDate)
		if err != nil || parsed.Format("2006-01-02") != *input.DueDate {
			return &InputError{Field: "due", Message: "must be a valid YYYY-MM-DD owner-local date"}
		}
	}
	if input.ResultSummary != nil && len(*input.ResultSummary) > MaxResultSummaryBytes {
		return &InputError{Field: "result_summary", Message: fmt.Sprintf("must not exceed %d bytes", MaxResultSummaryBytes)}
	}
	if input.Status != nil && !ValidStatus(*input.Status) {
		return &InputError{Field: "status", Message: "must be open, in_progress, blocked, completed, or cancelled"}
	}
	return ValidateIdempotencyKey(input.IdempotencyKey)
}

func ValidateIdempotencyKey(key IdempotencyKey) error {
	if strings.TrimSpace(string(key)) == "" {
		return &InputError{Field: "idempotency_key", Message: "must not be blank"}
	}
	if len(key) > 256 {
		return &InputError{Field: "idempotency_key", Message: "must not exceed 256 bytes"}
	}
	return nil
}

func ValidStatus(status Status) bool {
	switch status {
	case StatusOpen, StatusInProgress, StatusBlocked, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

func ValidateStatusTransition(from, to Status) error {
	if !ValidStatus(from) || !ValidStatus(to) || from == to {
		return &TransitionError{From: from, To: to}
	}
	if from == StatusCompleted || from == StatusCancelled {
		if to != StatusOpen {
			return &TransitionError{From: from, To: to}
		}
	}
	return nil
}

type mutationAttributionKey struct{}
type globalScopeCompatibilityKey struct{}

func WithMutationAttribution(ctx context.Context, attribution MutationAttribution) context.Context {
	return context.WithValue(ctx, mutationAttributionKey{}, attribution)
}

func MutationAttributionFromContext(ctx context.Context) (MutationAttribution, error) {
	attribution, ok := ctx.Value(mutationAttributionKey{}).(MutationAttribution)
	if !ok || strings.TrimSpace(attribution.ActorID) == "" || strings.TrimSpace(attribution.SessionID) == "" || strings.TrimSpace(attribution.RunID) == "" {
		return MutationAttribution{}, ErrMissingAttribution
	}
	return attribution, nil
}

func MutationAttributionContext(ctx context.Context) (MutationAttribution, bool) {
	attribution, ok := ctx.Value(mutationAttributionKey{}).(MutationAttribution)
	return attribution, ok
}

func WithGlobalScopeCompatibility(ctx context.Context) context.Context {
	return context.WithValue(ctx, globalScopeCompatibilityKey{}, true)
}

func GlobalScopeCompatibility(ctx context.Context) bool {
	compatible, _ := ctx.Value(globalScopeCompatibilityKey{}).(bool)
	return compatible
}
