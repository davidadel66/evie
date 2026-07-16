package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/plaid/plaid-go/v43/plaid"
)

// plaidClient builds a Plaid API client from the PLAID_CLIENT_ID and
// PLAID_SECRET environment variables, failing fast with a clear message if
// either is missing. Production environment is hardcoded — there is no
// sandbox mode in this tool.
func plaidClient() (*plaid.APIClient, error) {
	clientID := os.Getenv("PLAID_CLIENT_ID")
	secret := os.Getenv("PLAID_SECRET")
	if clientID == "" || secret == "" {
		return nil, fmt.Errorf("set PLAID_CLIENT_ID and PLAID_SECRET to your environment")
	}

	env := plaid.Production

	cfg := plaid.NewConfiguration()
	cfg.AddDefaultHeader("PLAID-CLIENT-ID", clientID)
	cfg.AddDefaultHeader("PLAID-SECRET", secret)
	cfg.UseEnvironment(env)
	return plaid.NewAPIClient(cfg), nil
}

// waitForPublicToken polls the hosted Link session every 3 seconds (up to
// ~5 minutes) until the user finishes linking in their browser, then
// returns the public token from the session results. Interactive by
// nature: it prints progress and debug output while it waits.
func waitForPublicToken(ctx context.Context, client *plaid.APIClient, linkToken string) (string, error) {
	fmt.Println("Waiting for you to finish linking in the browser...")
	for attempt := 0; attempt < 100; attempt++ {
		req := plaid.NewLinkTokenGetRequest(linkToken)
		resp, _, err := client.PlaidApi.LinkTokenGet(ctx).LinkTokenGetRequest(*req).Execute()
		b, err := json.MarshalIndent(resp.GetLinkSessions(), "", " ")
		if err != nil {
			return "", fmt.Errorf("link token get: %w", err)
		}
		fmt.Printf("[debug] sessions: %s\n", string(b))
		for _, s := range resp.GetLinkSessions() {
			if exit, ok := s.GetExitOk(); ok {
				perr := exit.GetError()
				fmt.Printf("[debug] EXIT code=%s message=%s\n",
					perr.GetErrorCode(), perr.GetErrorMessage())
			}
			results, ok := s.GetResultsOk()
			if !ok {
				continue
			}
			for _, item := range results.GetItemAddResults() {
				if pt := item.GetPublicToken(); pt != "" {
					return pt, nil
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	return "", fmt.Errorf("timed out waiting for link to complete")
}

// Link runs the full hosted Plaid Link flow: create a link token, hand the
// user a browser URL, wait for them to finish, exchange the public token
// for an access token, and save the item. Saving is an upsert on item_id —
// re-linking the same bank refreshes the access token but keeps the
// existing sync cursor, so we don't re-pull every transaction. This is the
// one human-interactive flow in the package; it prints to stdout.
func Link() error {
	client, err := plaidClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	req := plaid.NewLinkTokenCreateRequest(
		"finance",
		"en",
		[]plaid.CountryCode{plaid.COUNTRYCODE_US},
	)
	req.SetUser(*plaid.NewLinkTokenCreateRequestUser("david"))
	req.SetProducts([]plaid.Products{plaid.PRODUCTS_TRANSACTIONS})
	req.SetHostedLink(*plaid.NewLinkTokenCreateHostedLink())

	resp, _, err := client.PlaidApi.LinkTokenCreate(ctx).LinkTokenCreateRequest(*req).Execute()
	if err != nil {
		if plaidErr, ok := err.(plaid.GenericOpenAPIError); ok {
			fmt.Println("plaid error body:", string(plaidErr.Body()))
		}
		return fmt.Errorf("link token create: %w", err)
	}
	fmt.Println("Open this in your browser to link your bank:")
	fmt.Println(resp.GetHostedLinkUrl())

	publicToken, err := waitForPublicToken(ctx, client, resp.GetLinkToken())
	if err != nil {
		return err
	}

	exchReq := plaid.NewItemPublicTokenExchangeRequest(publicToken)
	exchResp, _, err := client.PlaidApi.ItemPublicTokenExchange(ctx).ItemPublicTokenExchangeRequest(*exchReq).Execute()
	if err != nil {
		return fmt.Errorf("exchange: %w", err)
	}
	itemID := exchResp.GetItemId()
	accessToken := exchResp.GetAccessToken()

	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
		INSERT INTO items (item_id, access_token, cursor, linked_at)
		VALUES (?, ?, '', ?)
		ON CONFLICT(item_id) DO UPDATE SET
			access_token = excluded.access_token,
			linked_at    = excluded.linked_at`,
		itemID, accessToken, time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save item: %w", err)
	}

	fmt.Printf("Linked! Saved item %s to ~/.finance/finance.db\n", itemID)
	return nil
}

// syncItem is one linked bank as loaded from the items table: the
// credentials and cursor needed to sync it, plus its display name.
type syncItem struct {
	itemID      string
	accessToken string
	cursor      string
	institution string
}

// plaidErrorCode digs Plaid's machine-readable error_code out of an SDK
// error body, returning "" if the error isn't a Plaid API error. It exists
// so Sync can react to specific Plaid failures (like the
// mutation-during-pagination restart) instead of string-matching messages.
func plaidErrorCode(err error) string {
	var apiErr plaid.GenericOpenAPIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	var body struct {
		ErrorCode string `json:"error_code"`
	}
	if json.Unmarshal(apiErr.Body(), &body) != nil {
		return ""
	}
	return body.ErrorCode
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
