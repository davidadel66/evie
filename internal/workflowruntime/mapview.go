package workflowruntime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// MapItemView is one map item as inspection shows it (sub-stage 9): its
// index and element, its state (succeeded, failed, interrupted, running or
// not started), its attempts, the worker execution of its latest agent
// attempt and its output or latest error.
type MapItemView struct {
	Index     int             `json:"index"`
	Item      json.RawMessage `json:"item"`
	State     string          `json:"state"`
	Attempts  int             `json:"attempts"`
	Execution string          `json:"execution,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// MapView is one map step's items at one checkpoint.
type MapView struct {
	Step       string        `json:"step"`
	Checkpoint int64         `json:"checkpoint"`
	Items      []MapItemView `json:"items"`
}

// MapViews derives each map step's items from a run's attempts: the list
// its latest attempt at a checkpoint ran over, and every item attempt.
func MapViews(d eviedb.WorkflowRunDetail) []MapView {
	type key struct {
		step       string
		checkpoint int64
	}
	lists := map[key][]json.RawMessage{}
	var order []key
	for _, a := range d.Attempts {
		if a.Kind != workflow.KindMap || a.Item != nil {
			continue
		}
		var input struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(a.Input, &input) != nil {
			continue
		}
		k := key{a.Step, a.InputCheckpoint}
		if _, seen := lists[k]; !seen {
			order = append(order, k)
		}
		lists[k] = input.Items
	}
	executions := map[int64]string{}
	for _, w := range d.Workers {
		executions[w.AttemptSeq] = w.ExecutionID
		if w.AdoptedFrom != "" {
			executions[w.AttemptSeq] = w.AdoptedFrom
		}
	}
	var out []MapView
	for _, k := range order {
		view := MapView{Step: k.step, Checkpoint: k.checkpoint, Items: []MapItemView{}}
		for i, element := range lists[k] {
			item := MapItemView{Index: i, Item: element, State: "not started"}
			for _, a := range d.Attempts {
				if a.Item == nil || *a.Item != i || a.Step != k.step || a.InputCheckpoint != k.checkpoint {
					continue
				}
				item.Attempts++
				if item.State == string(eviedb.WorkflowAttemptSucceeded) {
					continue
				}
				item.State, item.Error, item.Output = string(a.State), a.Error, a.Output
				if e := executions[a.Seq]; e != "" {
					item.Execution = e
				}
			}
			view.Items = append(view.Items, item)
		}
		out = append(out, view)
	}
	return out
}

// mapsText renders each map step's items.
func mapsText(b *strings.Builder, d eviedb.WorkflowRunDetail) {
	for _, view := range MapViews(d) {
		fmt.Fprintf(b, "Map steps.%s from checkpoint %d\n", view.Step, view.Checkpoint)
		for _, it := range view.Items {
			text := fmt.Sprintf("  item %d %s: %s", it.Index, it.Item, it.State)
			if it.Attempts > 0 {
				text += " after " + attempts(it.Attempts)
			}
			if it.Execution != "" {
				text += ", worker execution " + it.Execution
			}
			if it.Error != "" {
				text += " - " + it.Error
			}
			fmt.Fprintln(b, text)
		}
	}
}
