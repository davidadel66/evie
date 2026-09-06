package eviedb

import (
	"context"
	"database/sql"
)

type compilerAncestryEvent struct {
	ID, Kind, Role, Payload string
	Parent                  sql.NullString
	Sequence                int64
}

type compilerAncestryRoot struct {
	ID, Kind, Role  string
	Parent          sql.NullString
	Sequence, Depth int64
}

// Live and historical discovery share the same terminal turn_id/parent rules.
// The caller already read the seed: at most 127 additional metadata projections
// are inspected, and no cross-session or non-decreasing ancestry is admitted.
func resolveCompilerAncestry(ctx context.Context, conn compilerQueryer, session string, event compilerAncestryEvent) (root compilerAncestryRoot, err error) {
	err = conn.QueryRowContext(ctx, `WITH RECURSIVE ancestry(id,parent_id,sequence,event_type,role,payload_json,depth) AS (
 VALUES(?,?,?,?,?,?,1)
 UNION ALL SELECT e.id,e.parent_id,e.sequence,e.event_type,COALESCE(e.role,''),CASE WHEN length(CAST(e.payload_json AS BLOB))<=8192 THEN COALESCE(e.payload_json,'') ELSE '' END,a.depth+1 FROM ancestry a JOIN events e ON e.id=CASE WHEN a.event_type IN ('turn_failed','turn_interrupted') AND json_valid(a.payload_json) THEN json_extract(a.payload_json,'$.turn_id') ELSE a.parent_id END
 WHERE e.session_id=? AND e.sequence<a.sequence AND a.depth<128
) SELECT id,sequence,event_type,role,parent_id,depth FROM ancestry ORDER BY depth DESC LIMIT 1`, event.ID, event.Parent, event.Sequence, event.Kind, event.Role, event.Payload, session).Scan(&root.ID, &root.Sequence, &root.Kind, &root.Role, &root.Parent, &root.Depth)
	return
}
