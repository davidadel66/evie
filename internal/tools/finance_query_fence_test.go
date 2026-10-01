package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/finance"
)

const financeFenceToken = "plaid-access-token-SECRET-4242"

// seedFinanceFenceDB creates an isolated finance database whose items table
// holds a recognizable bank token, so a leak shows up as that exact string.
func seedFinanceFenceDB(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatalf("open finance db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO items (item_id, access_token, linked_at) VALUES ('item-1', ?, 'now')`, financeFenceToken); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO transactions (transaction_id, item_id, merchant_name, amount_cents, tags)
		VALUES ('txn-1', 'item-1', 'Coffee Shop', 350, '["work"]')`); err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
}

// T1: the finance branch of query_db is ungated, so the read-only promise has
// to hold for every statement shape — not just a SELECT prefix — and no
// query may read the pages the off-limits items table lives on.
func TestQueryDBFinanceRejectsWritesAndRawPageReads(t *testing.T) {
	seedFinanceFenceDB(t)
	outside := filepath.Join(t.TempDir(), "smuggled.db")

	for _, tc := range []struct{ name, query string }{
		{"multi-statement attach and write", fmt.Sprintf(`SELECT 1; ATTACH DATABASE '%s' AS smuggled; CREATE TABLE smuggled.t(x); INSERT INTO smuggled.t VALUES (1)`, outside)},
		{"trailing second statement", `SELECT merchant_name FROM transactions; SELECT 2`},
		{"raw pages", `SELECT data FROM sqlite_dbpage`},
		{"raw pages with schema argument", `SELECT data FROM sqlite_dbpage('main')`},
		{"qualified raw pages", `SELECT data FROM main.sqlite_dbpage`},
		{"double-quoted raw pages", `SELECT data FROM "sqlite_dbpage"`},
		{"single-quoted raw pages", `SELECT data FROM 'sqlite_dbpage'`},
		{"bracketed raw pages", `SELECT data FROM [SQLITE_DBPAGE]`},
		{"backquoted raw pages", "SELECT data FROM `sqlite_dbpage`"},
		{"page statistics", `SELECT name, pageno FROM dbstat`},
		{"vacuum into a new file", fmt.Sprintf(`VACUUM INTO '%s'`, outside)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := queryDB(context.Background(), queryDBArgs(t, "finance", tc.query))
			if strings.Contains(out, financeFenceToken) {
				t.Fatalf("query_db leaked the bank token for %q", tc.query)
			}
			if err == nil {
				t.Fatalf("query_db accepted %q (%d bytes of output)", tc.query, len(out))
			}
			if strings.Contains(err.Error(), financeFenceToken) {
				t.Fatalf("query_db leaked the bank token for %q", tc.query)
			}
		})
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("an ungated query created %s (stat err %v)", outside, err)
	}

	// Ordinary analysis, including json_each over tags, still works.
	out, err := queryDB(context.Background(), queryDBArgs(t, "finance",
		`SELECT t.merchant_name, j.value FROM transactions t, json_each(t.tags) j`))
	if err != nil {
		t.Fatalf("ordinary finance query failed: %v", err)
	}
	if !strings.Contains(out, "Coffee Shop | work") || !strings.Contains(out, "(1 rows)") {
		t.Fatalf("ordinary finance query output = %q", out)
	}
}

// The engine-level half of T1: even SQL that slips past the lexical check
// cannot ATTACH a writable file on the connection query_db uses.
func TestQueryDBFinanceConnectionCannotAttach(t *testing.T) {
	seedFinanceFenceDB(t)
	db, err := finance.OpenDBReadOnlyContext(context.Background())
	if err != nil {
		t.Fatalf("open read-only finance db: %v", err)
	}
	defer db.Close()

	outside := filepath.Join(t.TempDir(), "smuggled.db")
	for _, query := range []string{
		fmt.Sprintf(`ATTACH DATABASE '%s' AS smuggled`, outside),
		fmt.Sprintf(`SELECT 1; ATTACH DATABASE '%s' AS smuggled; CREATE TABLE smuggled.t(x)`, outside),
		fmt.Sprintf(`VACUUM INTO '%s'`, outside),
	} {
		if _, _, err := queryFinanceDB(context.Background(), db, query); err == nil {
			t.Fatalf("fenced finance connection ran %q", query)
		}
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("fenced finance connection created %s (stat err %v)", outside, err)
	}
}

// Raw-page reads are refused for every registered database, not only finance:
// the shared single-SELECT validator owns the rule.
func TestQueryDBTranscriptsRejectRawPageReads(t *testing.T) {
	_, path := newTranscriptQueryDB(t)
	pointTranscriptQueriesAt(t, path)
	if out, err := queryDB(context.Background(), queryDBArgs(t, "transcripts", `SELECT data FROM sqlite_dbpage`)); err == nil {
		t.Fatalf("transcripts query read raw pages: %q", out)
	}
}
