package eviedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"modernc.org/sqlite"
)

type clockReadFailure struct{}

func (*clockReadFailure) Error() string { return "injected SQLite read lock" }
func (*clockReadFailure) Code() int     { return 5 } // SQLITE_BUSY, using the driver's coded-error boundary.

type clockReadFault struct {
	target  memory.EventID
	cause   error
	enabled bool
	onNext  bool
	hits    int
}
type clockFaultConnector struct {
	path  string
	fault *clockReadFault
}

func (c clockFaultConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c clockFaultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	return &clockFaultConnection{Conn: conn, fault: c.fault}, nil
}

type clockFaultConnection struct {
	driver.Conn
	fault *clockReadFault
}

func (c *clockFaultConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.fault.enabled && strings.Contains(query, "payload_json") && strings.Contains(query, "FROM events WHERE id=?") && len(args) > 0 && args[0].Value == string(c.fault.target) {
		c.fault.hits++
		if c.fault.onNext {
			return clockFaultRows{cause: c.fault.cause}, nil
		}
		return nil, c.fault.cause
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

// Only the control row under test fails. All work, source, scheduling and
// recovery records still use real SQLite and the production worker entry point.
func clockFaultFixture(t *testing.T, path string) (*workerFixture, memory.Compilation, memory.EventID) {
	t.Helper()
	f := newWorkerFixture(t)
	f.generation.EvidencePolicy = memory.CompilerClockEvidencePolicy
	f.generationID, _, _ = memory.CompilerGenerationIdentity(f.generation)
	appendEvent := func(input memory.EventInput) memory.Event { t.Helper(); return activationAppend(t, f, input) }
	// Empty root/assistant content keeps these control rows out of the offered
	// source list, so each injected failure occurs inside clock ancestry lookup.
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser})
	appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "On the checked date I adopted tea as my standing drink."})
	assistant := appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: []byte(`{"tool_calls":[{"id":"clock-call","name":"get_time","arguments":"{}"}]}`)})
	intent := appendEvent(memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: "clock-fault", Payload: []byte(`{"call":{"id":"clock-call","name":"get_time","arguments":"{}"}}`)})
	parent := intent.ID
	if path == "approval parent" {
		approval := appendEvent(memory.EventInput{ParentID: intent.ID, Type: memory.EventApproval, ExecutionID: "clock-fault", Payload: []byte(`{"decision":"approved"}`)})
		parent = approval.ID
	}
	outcome := appendEvent(memory.EventInput{ParentID: parent, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: "clock-fault", Content: "2026-09-04 11:42:00", Payload: []byte(`{"tool_call_id":"clock-call","is_error":false}`)})
	last := appendEvent(memory.EventInput{ParentID: outcome.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant})
	queued, err := f.store.QueueCandidateUnit(context.Background(), f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, f.generation, &workerScript{})
	if err != nil || queued.State != "queued" {
		t.Fatalf("queue %s %v", queued.State, err)
	}
	target := intent.ID
	switch path {
	case "assistant":
		target = assistant.ID
	case "ancestor":
		target = root.ID
	}
	return f, queued, target
}

func TestCompilerClockAncestryReadFailuresKeepQueuedWorkRetryable(t *testing.T) {
	for _, path := range []string{"parent", "approval parent", "assistant", "ancestor"} {
		for _, cause := range []error{&clockReadFailure{}, context.Canceled, context.DeadlineExceeded} {
			t.Run(path+"/"+cause.Error(), func(t *testing.T) {
				f, queued, target := clockFaultFixture(t, path)
				fault := &clockReadFault{target: target, cause: cause, enabled: true}
				db := sql.OpenDB(clockFaultConnector{path: f.path, fault: fault})
				defer db.Close()
				store := NewStore(db)
				extractor := &workerScript{}
				worked, err := store.RunCompilerStep(context.Background(), f.config(extractor))
				if worked || !errors.Is(err, cause) || compilerDataFailure(err) || fault.hits == 0 {
					t.Fatalf("transient read lost: worked=%v err=%v hits=%d", worked, err, fault.hits)
				}
				inspected, err := f.store.InspectCompilation(context.Background(), f.owner, queued.JobID)
				if err != nil || inspected.State != "queued" || inspected.Attempts != 0 || inspected.Reason != "" || extractor.calls.Load() != 0 {
					t.Fatalf("transient read terminally changed queued work: %+v %v", inspected, err)
				}
				fault.enabled = false
				worked, err = store.RunCompilerStep(context.Background(), f.config(extractor))
				if err != nil || !worked || extractor.calls.Load() != 1 {
					t.Fatalf("retry failed worked=%v calls=%d err=%v", worked, extractor.calls.Load(), err)
				}
				inspected, err = f.store.InspectCompilation(context.Background(), f.owner, queued.JobID)
				if err != nil || inspected.State != "completed_empty" {
					t.Fatalf("retry result %s %v", inspected.State, err)
				}
			})
		}
	}
}

func TestCompilerClockAncestryTransportIdentityAndMissingRowBoundary(t *testing.T) {
	for _, path := range []string{"parent", "approval parent", "assistant", "ancestor"} {
		t.Run(path, func(t *testing.T) {
			f, queued, target := clockFaultFixture(t, path)
			var source memory.CompilerSource
			for _, s := range queued.Window.Sources {
				if s.Observation != nil {
					source = s
				}
			}
			outcome, err := readCompilerEvent(f.db.QueryRow(`SELECT `+compilerEventColumns+` FROM events WHERE id=?`, source.Locator.EventID))
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				cause   error
				missing bool
			}{{sql.ErrConnDone, false}, {driver.ErrBadConn, false}, {sql.ErrNoRows, true}, {errors.Join(sql.ErrNoRows, context.Canceled), false}, {errors.Join(sql.ErrNoRows, &clockReadFailure{}), false}} {
				cause := tc.cause
				t.Run(cause.Error(), func(t *testing.T) {
					lookup := func(id memory.EventID) (compilerEvent, error) {
						if id == target {
							return compilerEvent{}, cause
						}
						return readCompilerEvent(f.db.QueryRow(`SELECT `+compilerEventColumns+` FROM events WHERE id=?`, id))
					}
					_, err := validateClockAncestry(context.Background(), f.db, f.owner.SessionID, outcome, lookup)
					if tc.missing {
						if !errors.Is(err, errClockInvalidSource) || !compilerDataFailure(err) {
							t.Fatalf("missing ancestor not classified as source data failure: %v", err)
						}
					} else if !errors.Is(err, cause) || compilerDataFailure(err) {
						t.Fatalf("transport identity lost: %v", err)
					}
				})
			}
		})
	}
}

// Historical source replay uses QueryContext-only adapters. Errors delivered
// while advancing the first row must not be mistaken for a confirmed absence.
type clockFaultRows struct{ cause error }

func (clockFaultRows) Columns() []string           { return []string{"unread"} }
func (clockFaultRows) Close() error                { return nil }
func (r clockFaultRows) Next([]driver.Value) error { return r.cause }

type clockQueryOnly struct{ db *sql.DB }

func (q clockQueryOnly) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(ctx, query, args...)
}
func TestCompilerClockAncestryPreservesRowIterationFailures(t *testing.T) {
	for _, cause := range []error{&clockReadFailure{}, context.Canceled, driver.ErrBadConn} {
		t.Run(cause.Error(), func(t *testing.T) {
			f, queued, target := clockFaultFixture(t, "parent")
			var source memory.CompilerSource
			for _, s := range queued.Window.Sources {
				if s.Observation != nil {
					source = s
				}
			}
			fault := &clockReadFault{target: target, cause: cause, enabled: true, onNext: true}
			db := sql.OpenDB(clockFaultConnector{path: f.path, fault: fault})
			defer db.Close()
			err := resolveClockObservation(context.Background(), clockQueryOnly{db: db}, source)
			if !errors.Is(err, cause) || compilerDataFailure(err) || fault.hits != 1 {
				t.Fatalf("iteration error lost: err=%v hits=%d", err, fault.hits)
			}
		})
	}
}
