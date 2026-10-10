package eviedb

import (
	"context"
	"errors"
	"strings"
)

// WorkflowRunStepStates is, for each listed run, the state of each step's
// latest attempt (a map step's item attempts aside), so a run list can draw
// every run's progress without reading its attempts' inputs and outputs
// (workspace-activity.spec.md). Runs with no attempts are absent.
func (s *Store) WorkflowRunStepStates(ctx context.Context, runIDs []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if len(runIDs) == 0 {
		return out, nil
	}
	if len(runIDs) > 200 {
		return nil, errors.New("step states read at most 200 runs at a time")
	}
	args := make([]any, len(runIDs))
	for i, id := range runIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.run_id, a.step_id, a.state FROM workflow_node_attempts a
		WHERE a.run_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(runIDs)), ",")+`) AND a.item_index IS NULL
		AND a.seq = (SELECT MAX(b.seq) FROM workflow_node_attempts b WHERE b.run_id = a.run_id AND b.step_id = a.step_id AND b.item_index IS NULL)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var run, step, state string
		if err := rows.Scan(&run, &step, &state); err != nil {
			return nil, err
		}
		if out[run] == nil {
			out[run] = map[string]string{}
		}
		out[run][step] = state
	}
	return out, rows.Err()
}
