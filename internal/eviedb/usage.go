package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/usage"
)

// ReadConversationUsage selects only operational scalars, never episode text
// or raw payloads. The owner view includes closed and unselected sessions.
func (s *Store) ReadConversationUsage(ctx context.Context, p usage.Period) (usage.Summary, error) {
	var columns []string
	for _, field := range []string{"input_tokens", "cached_input_tokens", "output_tokens", "total_tokens", "reasoning_output_tokens", "cache_write_input_tokens"} {
		path := "'$.usage." + field + "'"
		columns = append(columns, `CASE WHEN json_type(e.payload_json, `+path+`) = 'integer' AND typeof(json_extract(e.payload_json, `+path+`)) = 'integer' AND json_extract(e.payload_json, `+path+`) >= 0 THEN json_extract(e.payload_json, `+path+`) END`)
	}
	query := `SELECT e.recorded_at, ` + strings.Join(columns, ", ") + `,
 CASE WHEN snap.format_version=1 AND snap.role IS NULL AND snap.content=''
  AND json_extract(snap.payload_json,'$.schema_version')=1
  AND json_type(snap.payload_json,'$.canonical_model')='text'
  AND length(json_extract(snap.payload_json,'$.canonical_model')) BETWEEN 1 AND 160
 THEN json_extract(snap.payload_json,'$.canonical_model') ELSE '' END
 FROM events e LEFT JOIN events snap
 ON snap.session_id=e.session_id AND snap.sequence=e.sequence-1
 AND snap.parent_id=e.parent_id AND snap.event_type='context_snapshot'
 WHERE e.event_type='assistant_message' AND e.role='assistant'
 AND substr(e.recorded_at,1,19)>=? AND substr(e.recorded_at,1,19)<?
 ORDER BY e.recorded_at,e.id LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, p.Start.UTC().Format("2006-01-02T15:04:05"), p.End.UTC().Format("2006-01-02T15:04:05"), usage.MaxObservations+1)
	if err != nil {
		return usage.Summary{}, err
	}
	defer rows.Close()
	observations := []usage.Observation{}
	for rows.Next() {
		var timestamp, model string
		var counters [6]sql.NullInt64
		if err := rows.Scan(&timestamp, &counters[0], &counters[1], &counters[2], &counters[3], &counters[4], &counters[5], &model); err != nil {
			return usage.Summary{}, err
		}
		at, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return usage.Summary{}, fmt.Errorf("invalid usage timestamp")
		}
		values := make([]*int64, 6)
		for i, c := range counters {
			if c.Valid {
				v := c.Int64
				values[i] = &v
			}
		}
		observations = append(observations, usage.Observation{At: at, Model: model, Tokens: usage.Tokens{Input: values[0], Cached: values[1], Output: values[2], Total: values[3], Reasoning: values[4], CacheWrite: values[5]}})
		if len(observations) > usage.MaxObservations {
			return usage.Summary{}, usage.ErrLimit
		}
	}
	if err := rows.Err(); err != nil {
		return usage.Summary{}, err
	}
	return usage.Summarize(p, observations)
}
