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
	ErrInvalidInput = errors.New("task: invalid input")
	ErrNotFound     = errors.New("task: not found")
)

type ID string

type Scope string

const ScopeGlobal Scope = "global"

type Status string

const StatusOpen Status = "open"

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

// Service is the focused Task persistence boundary consumed by first-party
// plugins. Implementations own identity generation and recovery policy.
type Service interface {
	CreateGlobalTask(context.Context, CreateInput) (Task, error)
	ListOpenGlobalTasks(context.Context) ([]Task, error)
	GetGlobalTask(context.Context, ID) (Task, error)
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
