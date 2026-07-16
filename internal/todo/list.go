package todo

import (
	"errors"
	"time"
)

// TaskList is a named list of tasks plus the counter that mints their IDs.
// Name and Dir locate the list's JSON file and are excluded from
// persistence — they describe where the data lives, not the data itself.
type TaskList struct {
	Tasks  []Task
	NextID int
	Name   string `json:"-"`
	Dir    string `json:"-"`
}

// Add appends a task, stamping it with the next stable ID and creation
// time. NextID only ever increments — deleted IDs are never reused, so an
// ID always means the same task for the whole life of the list.
func (list *TaskList) Add(t Task) {
	t.ID = list.NextID
	t.CreatedAt = time.Now()
	list.Tasks = append(list.Tasks, t)
	list.NextID++
}

// Find returns the index and a pointer to the task with the given ID, or
// (-1, nil) if absent. The pointer is into the slice itself, so callers
// can mutate the task in place (marking done uses this).
func (list *TaskList) Find(id int) (int, *Task) {
	for idx := range list.Tasks {
		if list.Tasks[idx].ID == id {
			return idx, &list.Tasks[idx]
		}
	}
	return -1, nil
}

// Delete removes the task with the given ID, erroring if it doesn't
// exist. The ID is retired with it — NextID never goes backwards.
func (list *TaskList) Delete(id int) error {
	i, t := list.Find(id)
	if t == nil {
		return errors.New("task does not exist")
	}
	list.Tasks = append(list.Tasks[:i], list.Tasks[i+1:]...)
	return nil
}
