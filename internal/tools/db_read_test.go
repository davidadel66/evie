package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/finance"
)

func TestReadDBReturnsTypedFinanceRows(t *testing.T) {
	seedFinanceFenceDB(t)
	rows, err := ReadDB(context.Background(), "finance", "SELECT merchant_name, amount_cents, category FROM transactions")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows.Columns, ",") != "merchant_name,amount_cents,category" || len(rows.Rows) != 1 || rows.Truncated {
		t.Fatalf("rows = %+v", rows)
	}
	if row := rows.Rows[0]; row[0] != "Coffee Shop" || row[1] != int64(350) || row[2] != nil {
		t.Fatalf("row = %#v", row)
	}
}

// ReadDB is query_db's read path for tabs, so every refusal query_db makes
// holds here too, and the bank token never comes back.
func TestReadDBKeepsQueryDBFences(t *testing.T) {
	seedFinanceFenceDB(t)
	for _, query := range []string{
		"UPDATE transactions SET amount_cents = 0",
		"DELETE FROM transactions",
		"SELECT 1; DELETE FROM transactions",
		"SELECT * FROM items",
		"SELECT access_token FROM items",
		"ATTACH DATABASE 'x.db' AS x",
		"SELECT 1 FROM transactions; ATTACH DATABASE 'x.db' AS x",
		"VACUUM INTO 'copy.db'",
		"PRAGMA table_info(items)",
		"WITH t AS (SELECT * FROM items) SELECT * FROM t",
	} {
		rows, err := ReadDB(context.Background(), "finance", query)
		if err == nil || strings.Contains(fmt.Sprint(rows), financeFenceToken) {
			t.Errorf("%q: rows = %+v, err = %v", query, rows, err)
		}
	}
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var amount int
	if err := db.QueryRow(`SELECT amount_cents FROM transactions WHERE transaction_id = 'txn-1'`).Scan(&amount); err != nil || amount != 350 {
		t.Fatalf("finance changed: amount = %d, %v", amount, err)
	}
	if _, err := ReadDB(context.Background(), "transcripts", "SELECT 1"); err == nil || !strings.Contains(err.Error(), "tabs can read: finance, evie") {
		t.Fatalf("transcripts: %v", err)
	}
}

func TestReadDBReadsOnlyEviesListedTables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := eviedb.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if rows, err := ReadDB(context.Background(), "evie", "SELECT * FROM jobs"); err != nil || rows.Rows == nil {
		t.Fatalf("jobs = %+v, %v", rows, err)
	}
	for _, query := range []string{"SELECT * FROM sessions", "SELECT * FROM events", "SELECT * FROM jobs WHERE id IN sessions"} {
		if _, err := ReadDB(context.Background(), "evie", query); err == nil {
			t.Errorf("%q was allowed", query)
		}
	}
}

func TestReadDBCapsRows(t *testing.T) {
	seedFinanceFenceDB(t)
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 80 {
		if _, err := db.Exec(`INSERT INTO transactions (transaction_id, item_id, merchant_name, amount_cents, tags) VALUES (?, 'item-1', 'Shop', 1, '[]')`, fmt.Sprintf("txn-cap-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	rows, err := ReadDB(context.Background(), "finance", "SELECT a.transaction_id FROM transactions a JOIN transactions b")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows.Rows) != MaxReadRows || !rows.Truncated {
		t.Fatalf("rows = %d, truncated = %v", len(rows.Rows), rows.Truncated)
	}
}
