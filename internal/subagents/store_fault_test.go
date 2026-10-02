package subagents_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// busyError uses the SQLite driver's coded-error boundary for SQLITE_BUSY.
type busyError struct{}

func (busyError) Error() string { return "injected database is locked (SQLITE_BUSY)" }
func (busyError) Code() int     { return 5 }

// storeFault fails selected statements on a real SQLite connection. Every
// other statement, including the one under test once disarmed, uses SQLite.
// Several rules can be armed at once; each counts its own hits.
type storeFault struct {
	mu    sync.Mutex
	rules []*faultRule
}

type faultRule struct {
	match     string
	err       error
	remaining int // negative: until disarmed
	hits      int
}

// arm replaces every rule with one.
func (f *storeFault) arm(match string, err error, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = nil
	if err != nil {
		f.rules = []*faultRule{{match: match, err: err, remaining: count}}
	}
}

// also adds a rule beside the armed ones.
func (f *storeFault) also(match string, err error, count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = append(f.rules, &faultRule{match: match, err: err, remaining: count})
}
func (f *storeFault) disarm() { f.arm("", nil, 0) }

// hitCount is the hits of every rule armed since the last arm.
func (f *storeFault) hitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.rules {
		n += r.hits
	}
	return n
}

// hitsOf is the hits of the rules matching match.
func (f *storeFault) hitsOf(match string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.rules {
		if r.match == match {
			n += r.hits
		}
	}
	return n
}
func (f *storeFault) inject(query string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rules {
		if r.remaining == 0 || !strings.Contains(query, r.match) {
			continue
		}
		if r.remaining > 0 {
			r.remaining--
		}
		r.hits++
		return r.err
	}
	return nil
}

// waitForHits waits through the public fault seam rather than for elapsed time.
func (f *storeFault) waitForHits(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(gateTimeout)
	for f.hitCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("fault reached %d of %d hits", f.hitCount(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type faultConnector struct {
	dsn   string
	fault *storeFault
}

func (c faultConnector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c faultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &faultConn{Conn: conn, fault: c.fault}, nil
}

type faultConn struct {
	driver.Conn
	fault *storeFault
}

func (c *faultConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := c.fault.inject(query); err != nil {
		return nil, err
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *faultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := c.fault.inject(query); err != nil {
		return nil, err
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *faultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

// openFaultDB opens an already migrated database with Evie's connection pragmas.
func openFaultDB(path string, fault *storeFault) *sql.DB {
	return sql.OpenDB(faultConnector{dsn: path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", fault: fault})
}
