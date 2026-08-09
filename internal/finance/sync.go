package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/plaid/plaid-go/v43/plaid"
)

// syncItem is one linked bank as loaded from the items table: the
// credentials and cursor needed to sync it, plus its display name.
type syncItem struct {
	itemID      string
	accessToken string
	cursor      string
	institution string
}

// SyncCounts tallies how many transactions a sync added, modified, and
// removed.
type SyncCounts struct {
	Added, Modified, Removed int
}

// BankSync is the outcome of syncing one linked bank. Err non-nil means
// this bank's sync failed; Warnings are non-fatal issues (like a missing
// institution name) that didn't stop the sync.
type BankSync struct {
	Label    string
	Counts   SyncCounts
	Warnings []string
	Err      error
}

// SyncResult is everything a caller needs to report a sync: the per-bank
// outcomes in order, plus precomputed totals across the banks that
// succeeded.
type SyncResult struct {
	Banks  []BankSync
	Totals SyncCounts
}

// Sync pulls new transactions for every linked bank and returns the
// results as data — it prints nothing; rendering belongs to the caller.
// One bank failing is recorded in its BankSync.Err and doesn't stop the
// others; the returned error is reserved for "the job couldn't run at
// all" (no Plaid credentials, no linked banks, unreadable items table).
// A TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION failure is retried once
// from the last committed cursor before being recorded as that bank's
// error.
func Sync(db *sql.DB) (*SyncResult, error) {
	client, err := plaidClient()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	rows, err := db.Query(`SELECT item_id, access_token, cursor, COALESCE(institution, '') FROM items`)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	var items []syncItem
	for rows.Next() {
		var it syncItem
		if err := rows.Scan(&it.itemID, &it.accessToken, &it.cursor, &it.institution); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read items: %w", err)
	}
	rows.Close()

	if len(items) == 0 {
		return nil, errors.New("no linked banks: run `finance link` first")
	}

	result := &SyncResult{}
	for _, it := range items {
		var bank BankSync
		if it.institution == "" {
			if err := backfillInstitution(ctx, client, db, &it); err != nil {
				bank.Warnings = append(bank.Warnings, fmt.Sprintf("could not resolve institution for %s: %v", it.itemID, err))
			}
		}
		bank.Label = it.institution
		if bank.Label == "" {
			bank.Label = it.itemID
		}

		added, modified, removed, err := syncOneItem(ctx, client, db, it)
		if err != nil && plaidErrorCode(err) == "TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION" {
			if dbErr := db.QueryRow(`SELECT cursor FROM items WHERE item_id = ?`, it.itemID).Scan(&it.cursor); dbErr != nil {
				err = errors.Join(err, fmt.Errorf("re-read cursor: %w", dbErr))
			} else {
				var a, m, r int
				a, m, r, err = syncOneItem(ctx, client, db, it)
				added, modified, removed = added+a, modified+m, removed+r
			}
		}
		if err != nil {
			bank.Err = err
			result.Banks = append(result.Banks, bank)
			continue
		}

		bank.Counts = SyncCounts{Added: added, Modified: modified, Removed: removed}
		result.Banks = append(result.Banks, bank)
		result.Totals.Added += added
		result.Totals.Modified += modified
		result.Totals.Removed += removed
	}

	return result, nil
}

// backfillInstitution resolves and stores a human-readable institution
// name for items linked before we recorded one, via /item/get followed by
// /institutions/get_by_id. Failure is non-fatal to the caller — a bank can
// sync fine without a pretty name — so Sync records it as a warning rather
// than an error.
func backfillInstitution(ctx context.Context, client *plaid.APIClient, db *sql.DB, it *syncItem) error {
	itemReq := plaid.NewItemGetRequest(it.accessToken)
	itemResp, _, err := client.PlaidApi.ItemGet(ctx).ItemGetRequest(*itemReq).Execute()
	if err != nil {
		return fmt.Errorf("item get: %w", err)
	}
	item := itemResp.GetItem()
	instID := item.GetInstitutionId()
	if instID == "" {
		return fmt.Errorf("item %s has no institution_id", it.itemID)
	}

	instReq := plaid.NewInstitutionsGetByIdRequest(instID, []plaid.CountryCode{plaid.COUNTRYCODE_US})
	instResp, _, err := client.PlaidApi.InstitutionsGetById(ctx).InstitutionsGetByIdRequest(*instReq).Execute()
	if err != nil {
		return fmt.Errorf("institutions get by id: %w", err)
	}
	inst := instResp.GetInstitution()
	name := inst.GetName()
	if name == "" {
		return fmt.Errorf("institution %s has no name", instID)
	}

	if _, err := db.Exec(`UPDATE items SET institution = ? WHERE item_id = ?`, name, it.itemID); err != nil {
		return fmt.Errorf("save institution: %w", err)
	}
	it.institution = name
	return nil
}

// syncOneItem runs the /transactions/sync pagination loop for one bank:
// fetch a page, persist it atomically with its cursor (applySyncPage),
// repeat until has_more is false. Counts persist across pages, so even on
// a mid-loop error the returned totals reflect what actually landed. One
// guard worth knowing: an empty next_cursor alongside has_more means
// Plaid's historical pull isn't ready yet — applying it would reset us to
// first-sync semantics, so we bail and ask the caller to retry later.
func syncOneItem(ctx context.Context, client *plaid.APIClient, db *sql.DB, it syncItem) (added, modified, removed int, err error) {
	cursor := it.cursor
	for {
		req := plaid.NewTransactionsSyncRequest(it.accessToken)
		if cursor != "" {
			req.SetCursor(cursor)
		}
		resp, _, err := client.PlaidApi.TransactionsSync(ctx).TransactionsSyncRequest(*req).Execute()
		if err != nil {
			return added, modified, removed, fmt.Errorf("transactions sync: %w", err)
		}

		if resp.GetHasMore() && resp.GetNextCursor() == "" {
			return added, modified, removed, fmt.Errorf("item %s: empty next_cursor with has_more; historical pull not ready, retry later", it.itemID)
		}

		pageAdded := toSyncTxns(resp.GetAdded())
		pageModified := toSyncTxns(resp.GetModified())
		var pageRemoved []string
		for _, r := range resp.GetRemoved() {
			pageRemoved = append(pageRemoved, r.GetTransactionId())
		}

		if err := applySyncPage(db, it.itemID, pageAdded, pageModified, pageRemoved, resp.GetNextCursor()); err != nil {
			return added, modified, removed, err
		}

		added += len(pageAdded)
		modified += len(pageModified)
		removed += len(pageRemoved)
		cursor = resp.GetNextCursor()

		if !resp.GetHasMore() {
			return added, modified, removed, nil
		}
	}
}

// toSyncTxns converts Plaid SDK transactions to our persistence type,
// keeping only the fields we store. This is the boundary where Plaid's
// types stop and ours begin.
func toSyncTxns(txns []plaid.Transaction) []SyncTxn {
	out := make([]SyncTxn, 0, len(txns))
	for _, t := range txns {
		pfc := t.GetPersonalFinanceCategory()
		out = append(out, SyncTxn{
			TransactionID: t.GetTransactionId(),
			AccountID:     t.GetAccountId(),
			Date:          t.GetDate(),
			Name:          t.GetName(),
			MerchantName:  t.GetMerchantName(),
			Amount:        t.GetAmount(),
			Category:      pfc.GetPrimary(),
			Pending:       t.GetPending(),
		})
	}
	return out
}

// SyncTxn is one transaction from a Plaid /transactions/sync page, reduced
// to the fields we persist. Date is YYYY-MM-DD. Amount is Plaid's
// convention — dollars as a float, positive meaning money out — and is
// converted to integer cents at persistence time.
type SyncTxn struct {
	TransactionID string
	AccountID     string
	Date          string
	Name          string
	MerchantName  string
	Amount        float64
	Category      string
	Pending       bool
}

// applySyncPage persists one page of Plaid sync results — upserts for
// added/modified, deletes for removed — together with the item's new
// cursor, all inside a single SQL transaction. That atomicity is the
// core sync guarantee: a page and its cursor land together or not at
// all, so a crash mid-sync can never record progress it didn't make.
func applySyncPage(db *sql.DB, itemID string, added, modified []SyncTxn, removed []string, nextCursor string) error {
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

	// budget_entries.transaction_id REFERENCES transactions, so the child
	// rows must go first or the parent delete fails with FOREIGN KEY
	// constraint failed (787) — which is exactly what happened in
	// production: a pending transaction that Plaid later removed had
	// already been categorized, and the whole page (cursor included)
	// rolled back on every subsequent sync, wedging that bank permanently.
	//
	// Deleting the entry is right, not merely expedient: a removed
	// transaction is Plaid retracting a row that was never real, not David
	// un-deciding a categorization. The posted replacement gets its own
	// entry from the same rule on the next categorize run, at the final
	// amount — keeping the old entry would double-count.
	for _, id := range removed {
		if _, err := tx.Exec(`DELETE FROM budget_entries WHERE transaction_id = ?`, id); err != nil {
			return fmt.Errorf("delete budget entries for transaction %s: %w", id, err)
		}
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
