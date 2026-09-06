package eviedb

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// Count actual source/ancestry projections, including a root metadata read.
// Aggregate COUNT and sequence-only index endpoints are coordinate lookups;
// they do not inspect role, ancestry, content, payload or policy eligibility.
// The extra COUNT queries here measure result cardinality without fetching the
// projected source fields into the test or changing the transaction's data.
type compilerCountedSelection struct {
	*sql.Conn
	inspections int
	coordinates int
	t           *testing.T
}

func (q *compilerCountedSelection) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	q.t.Helper()
	switch {
	case strings.HasPrefix(query, "WITH RECURSIVE ancestry"):
		end := strings.LastIndex(query, " SELECT id,sequence,event_type,role,parent_id,depth FROM ancestry")
		if end < 0 {
			q.t.Fatal("ancestry instrumentation must follow the source query")
		}
		var depth int
		if err := q.Conn.QueryRowContext(ctx, query[:end]+" SELECT COUNT(*) FROM ancestry", args...).Scan(&depth); err != nil {
			q.t.Fatal(err)
		}
		if strings.Contains(query, "VALUES(") {
			depth--
		} // cached seed incurs no second source read
		q.inspections += depth
	case strings.Contains(query, "payload_json") || strings.Contains(query, "sequence,event_type,role,parent_id FROM events"):
		q.inspections++
	case strings.Contains(query, "FROM events"):
		q.coordinates++
	}
	return q.Conn.QueryRowContext(ctx, query, args...)
}

func (q *compilerCountedSelection) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if strings.Contains(query, "payload_json") {
		var count int
		if err := q.Conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+query+")", args...).Scan(&count); err != nil {
			q.t.Fatal(err)
		}
		q.inspections += count
	} else if strings.Contains(query, "FROM events") {
		q.coordinates++
	}
	return q.Conn.QueryContext(ctx, query, args...)
}

func TestCompilerActivationDiscoveryInspectsMaximumAncestryOnce(t *testing.T) {
	for _, depth := range []int{128, 129} {
		t.Run(strconv.Itoa(depth), func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			var previous memory.Event
			for n := 1; n < depth; n++ {
				previous = activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: previous.ID, Content: "An owner assertion."})
			}
			activationStart(t, f)
			last := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: previous.ID, Content: "The newly selected assertion."})
			err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
				counted := &compilerCountedSelection{Conn: conn, t: t}
				var result memory.CompilerReconciliation
				if err := discoverCompilerEvidenceInTransaction(ctx, counted, &result); err != nil {
					return err
				}
				if !result.Discovered || counted.inspections != 128 {
					t.Fatalf("depth%d inspected%d events; want128", depth, counted.inspections)
				}
				var root, state, reason string
				if err := conn.QueryRowContext(ctx, `SELECT root_id,state,reason FROM memory_compiler_activation_roots`).Scan(&root, &state, &reason); err != nil {
					return err
				}
				if depth == 128 {
					if root == string(last.ID) || state != "selected_unmaterialized" || reason != "" {
						t.Fatalf("valid depth128 was truncated or misclassified: %s %s %s", root, state, reason)
					}
				} else if root != string(last.ID) || state != "failed" || reason != "source_inspection_limit" {
					t.Fatalf("depth129 lost explicit inspection gap: %s %s %s", root, state, reason)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompilerActivationCaptureInspectsMaximumWindowOnce(t *testing.T) {
	for _, count := range []int{128, 129} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "The owner assertion."})
			var end memory.Event
			for n := 1; n < count; n++ {
				end = activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Context only."})
			}
			err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
				counted := &compilerCountedSelection{Conn: conn, t: t}
				window, state, reason, err := captureCompilerWindow(ctx, counted, f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}, root.Sequence)
				if err != nil {
					return err
				}
				if counted.inspections > 128 {
					t.Fatalf("inspected%d sourceevents; coordinate queries%d", counted.inspections, counted.coordinates)
				}
				if count == 128 {
					if counted.inspections != 128 || state != "queued" || len(window.NewEventIDs) != 128 {
						t.Fatalf("maximum window changed: inspections%d state%s reason%s new%d", counted.inspections, state, reason, len(window.NewEventIDs))
					}
				} else if state != "failed" || reason != "source_inspection_limit" {
					t.Fatalf("over-bound window truncated: %s %s", state, reason)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
