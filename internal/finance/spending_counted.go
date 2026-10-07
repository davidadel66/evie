package finance

import (
	"context"
	"database/sql"
	"math/big"
)

// Overview counting (spending.spec.md, amended 2026-10-06, transfers). The
// Overview shows the owner's money, not the bank's ledger: an allocation in
// the excluded category contributes nothing to any Overview figure. The
// rule is per allocation, so a split's other allocations still count, and
// it applies only to transactions whose saved entries reconcile exactly to
// the bank amount with coherent signs and labels; a transaction with no
// entries or an unreconciled split still counts in full, as Needs
// classification or Needs review. Nothing is inferred: only a saved
// excluded entry removes money from the Overview.

// countedTransaction is one posted or pending bank transaction with the
// amount the Overview counts for it.
type countedTransaction struct {
	ID      string
	Date    string
	Pending bool
	// Amount is the raw bank amount; Counted is what the Overview counts.
	Amount, Counted *big.Int
	// Omitted is true when every reconciled allocation is excluded, so the
	// transaction appears in no Overview count.
	Omitted bool
}

type spendingQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// countedFlowSQL aggregates each transaction's saved entries: how many,
// their exact sum, the excluded part of that sum, and how many are
// incoherent (blank label, or a nonzero amount whose sign disagrees with
// the bank amount's). Sums are over one transaction's few entries, so they
// stay far inside SQLite's integer range; the Overview totals are summed in
// Go with big.Int.
const countedFlowSQL = `
	SELECT t.transaction_id, COALESCE(t.date, ''), t.amount_cents, t.pending,
	       COUNT(e.id),
	       COALESCE(SUM(e.amount_cents), 0),
	       COALESCE(SUM(CASE WHEN e.category = 'excluded' THEN e.amount_cents ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN e.category = 'excluded' THEN 1 ELSE 0 END), 0),
	       COALESCE(SUM(CASE WHEN TRIM(e.category) = ''
	                           OR (e.amount_cents != 0 AND SIGN(e.amount_cents) != SIGN(t.amount_cents))
	                      THEN 1 ELSE 0 END), 0)
	FROM transactions t LEFT JOIN budget_entries e ON e.transaction_id = t.transaction_id
	`

// readCountedFlow returns every transaction matching where (a predicate
// over the transactions alias t, before GROUP BY) with its counted amount.
func readCountedFlow(ctx context.Context, q spendingQuerier, where string, args ...any) ([]countedTransaction, error) {
	query := countedFlowSQL
	if where != "" {
		query += " WHERE " + where
	}
	rows, err := q.QueryContext(ctx, query+" GROUP BY t.transaction_id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []countedTransaction
	for rows.Next() {
		var c countedTransaction
		var amount, entrySum, excludedSum int64
		var pending, entries, excludedEntries, incoherent int
		if err := rows.Scan(&c.ID, &c.Date, &amount, &pending, &entries, &entrySum, &excludedSum, &excludedEntries, &incoherent); err != nil {
			return nil, err
		}
		c.Pending = pending != 0
		c.Amount = big.NewInt(amount)
		c.Counted = big.NewInt(amount)
		reconciled := entries > 0 && incoherent == 0 && entrySum == amount
		if reconciled && excludedEntries > 0 {
			c.Counted = big.NewInt(amount - excludedSum)
			c.Omitted = excludedEntries == entries
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
