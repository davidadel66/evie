// Package todo is the task-list engine: stable-ID tasks, named lists, and
// JSON persistence under a caller-chosen directory. It changes state and
// returns errors; rendering, flag parsing, and env config belong to the
// callers — the todo CLI and, eventually, the agent's tools.
package todo

import (
	"fmt"
	"time"
)

// Task is one todo item. ID is stable for the task's lifetime and never
// reused after deletion.
type Task struct {
	ID          int
	Title       string
	Description string
	CreatedAt   time.Time
	Priority    int
	Status      bool
	DueDate     time.Time
}

// String renders one task as a checklist line — "[x] #3 title (p2) due
// 2026-07-20" — omitting priority and due date when unset. Implementing
// the standard Stringer interface means fmt.Println(t) just works.
func (t Task) String() string {
	box := "[ ]"
	if t.Status {
		box = "[x]"
	}
	line := fmt.Sprintf("%s #%d %s", box, t.ID, t.Title)

	if t.Priority != 0 {
		line += fmt.Sprintf(" (p%d)", t.Priority)
	}
	if !t.DueDate.IsZero() {
		line += fmt.Sprintf(" due %s", t.DueDate.Format("2006-01-02"))
	}
	return line
}
