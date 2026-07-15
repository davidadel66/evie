package main

import (
	"database/sql"
	"fmt"
	"math"

	"github.com/joho/godotenv"
)

// SyncTxn is one transaction from a Plaid /transactions/sync page,
// reduced to the fields we persist.
type SyncTxn struct {
	TransactionID string
	AccountID     string
	Date          string // YYYY-MM-DD
	Name          string
	MerchantName  string
	Amount        float64 // Plaid dollars; positive = money out
	Category      string
	Pending       bool
}

// applySyncPage applies one page of Plaid sync results and the item's new
// cursor in a single SQL transaction. Silent; returns error.
func applySyncPage(db *sql.DB, itemID string, added, modified []SyncTxn, removed []string, nextCursor string) error {
	_ = godotenv.Load("../../.env", ".env")

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin sync page: %w", err)
	}
	defer tx.Rollback()

	upsert, err := tx.Prepare(`
		INSERT INTO transactions
			(transaction_id, item_id, account_id, date, name, merchant_name, amount_cents, plaid_category, pending)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(transaction_id) DO UPDATE SET
			item_id        = excluded.item_id,
			account_id     = excluded.account_id,
			date           = excluded.date,
			name           = excluded.name,
			merchant_name  = excluded.merchant_name,
			amount_cents   = excluded.amount_cents,
			plaid_category = excluded.plaid_category,
			pending        = excluded.pending`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer upsert.Close()

	for _, page := range [][]SyncTxn{added, modified} {
		for _, t := range page {
			cents := int64(math.Round(t.Amount * 100))
			pending := 0
			if t.Pending {
				pending = 1
			}
			if _, err := upsert.Exec(
				t.TransactionID, itemID, t.AccountID, t.Date, t.Name,
				t.MerchantName, cents, t.Category, pending,
			); err != nil {
				return fmt.Errorf("upsert transaction %s: %w", t.TransactionID, err)
			}
		}
	}

	for _, id := range removed {
		if _, err := tx.Exec(`DELETE FROM transactions WHERE transaction_id = ?`, id); err != nil {
			return fmt.Errorf("delete transaction %s: %w", id, err)
		}
	}

	res, err := tx.Exec(`UPDATE items SET cursor = ? WHERE item_id = ?`, nextCursor, itemID)
	if err != nil {
		return fmt.Errorf("update cursor for item %s: %w", itemID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update cursor for item %s: %w", itemID, err)
	}
	if n == 0 {
		return fmt.Errorf("update cursor: item %s not found", itemID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync page: %w", err)
	}
	return nil
}
