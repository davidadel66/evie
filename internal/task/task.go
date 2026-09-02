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
	ErrInvalidInput       = errors.New("task: invalid input")
	ErrNotFound           = errors.New("task: not found")
	ErrConflict           = errors.New("task: revision conflict")
	ErrInvalidTransition  = errors.New("task: invalid status transition")
	ErrMissingAttribution = errors.New("task: trusted mutation attribution is missing")
)

type ID string

type Scope string

const ScopeGlobal Scope = "global"

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
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
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
)

// Task is the durable state of one owner-controlled unit of work.
type Task struct {
	ID          ID        `json:"id"`
	Scope       Scope     `json:"scope"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Priority    int       `json:"priority,omitempty"`
	DueDate     string    `json:"due_date,omitempty"`
	Status      Status    `json:"status"`
	Revision    uint64    `json:"revision"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateInput deliberately excludes persistence, scope, owner, actor, and
// session identities. Those values belong to the trusted Kernel boundary.
type CreateInput struct {
	Title       string
	Description string
	Priority    int
	DueDate     string
}

// UpdateInput is a compare-and-swap patch. Pointer fields distinguish omission
// from explicitly clearing optional metadata.
type UpdateInput struct {
	ExpectedRevision uint64
	Title            *string
	Description      *string
	Priority         *int
	DueDate          *string
	Status           *Status
}

// ListFilter defaults to open Tasks when neither Statuses nor IncludeHistory
// is supplied. IncludeHistory returns every retained lifecycle state.
type ListFilter struct {
	Statuses       []Status
	IncludeHistory bool
}

// MutationAttribution is installed by the trusted runtime immediately before
// tool execution. It is intentionally absent from model-facing arguments.
type MutationAttribution struct {
	ActorID   string
	SessionID string
	RunID     string
}

type Event struct {
	ID                string          `json:"id"`
	TaskID            ID              `json:"task_id"`
	Sequence          uint64          `json:"sequence"`
	Operation         Operation       `json:"operation"`
	ActorID           string          `json:"actor_id"`
	SessionID         string          `json:"session_id"`
	RunID             string          `json:"run_id"`
	RecordedAt        time.Time       `json:"recorded_at"`
	PreviousRevision  uint64          `json:"previous_revision"`
	ResultingRevision uint64          `json:"resulting_revision"`
	Outcome           MutationOutcome `json:"outcome"`
	DiagnosticCode    DiagnosticCode  `json:"diagnostic_code,omitempty"`
}

// Service is the focused Task persistence boundary consumed by first-party
// plugins. Implementations own identity generation and recovery policy.
type Service interface {
	CreateGlobalTask(context.Context, CreateInput) (Task, error)
	ListOpenGlobalTasks(context.Context) ([]Task, error)
	ListGlobalTasks(context.Context, ListFilter) ([]Task, error)
	GetGlobalTask(context.Context, ID) (Task, error)
	UpdateGlobalTask(context.Context, ID, UpdateInput) (Task, error)
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

func ValidateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return &InputError{Field: "title", Message: "must not be blank"}
	}
	if input.Priority < 0 || input.Priority > 5 {
		return &InputError{Field: "priority", Message: "must be zero or between one and five"}
	}
	if input.DueDate != "" {
		parsed, err := time.Parse("2006-01-02", input.DueDate)
		if err != nil || parsed.Format("2006-01-02") != input.DueDate {
			return &InputError{Field: "due", Message: "must be a valid YYYY-MM-DD owner-local date"}
		}
	}
	return nil
}

func ValidateUpdateInput(input UpdateInput) error {
	if input.ExpectedRevision == 0 {
		return &InputError{Field: "expected_revision", Message: "must be greater than zero"}
	}
	if input.Title == nil && input.Description == nil && input.Priority == nil && input.DueDate == nil && input.Status == nil {
		return &InputError{Field: "patch", Message: "must change metadata or lifecycle status"}
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
	if input.Status != nil && !ValidStatus(*input.Status) {
		return &InputError{Field: "status", Message: "must be open, in_progress, blocked, completed, or cancelled"}
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
