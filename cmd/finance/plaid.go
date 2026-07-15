package main

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

func runLink() error {
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

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Upsert: re-linking the same bank refreshes the token but keeps
	// the existing sync cursor so we don't re-pull every transaction.
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

type syncItem struct {
	itemID      string
	accessToken string
	cursor      string
	institution string
}

// plaidErrorCode extracts Plaid's error_code from an SDK error, or "".
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

// backfillInstitution fills items.institution via /item/get ->
// /institutions/get_by_id. Silent; returns error (caller treats as non-fatal).
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

// syncOneItem runs the /transactions/sync pagination loop for one item,
// applying each page atomically. Returns added/modified/removed counts.
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

		// An empty next_cursor with has_more means Plaid's historical pull
		// isn't ready; applying it would reset us to first-sync semantics.
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

func runSync(db *sql.DB) error {
	client, err := plaidClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	rows, err := db.Query(`SELECT item_id, access_token, cursor, COALESCE(institution, '') FROM items`)
	if err != nil {
		return fmt.Errorf("load items: %w", err)
	}
	var items []syncItem
	for rows.Next() {
		var it syncItem
		if err := rows.Scan(&it.itemID, &it.accessToken, &it.cursor, &it.institution); err != nil {
			rows.Close()
			return fmt.Errorf("scan item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read items: %w", err)
	}
	rows.Close()

	if len(items) == 0 {
		fmt.Println("No linked banks. Run `finance link` first.")
		return nil
	}

	var (
		errs                                    []error
		totalAdded, totalModified, totalRemoved int
	)
	for _, it := range items {
		if it.institution == "" {
			if err := backfillInstitution(ctx, client, db, &it); err != nil {
				fmt.Printf("warning: could not resolve institution for %s: %v\n", it.itemID, err)
			}
		}
		label := it.institution
		if label == "" {
			label = it.itemID
		}

		added, modified, removed, err := syncOneItem(ctx, client, db, it)
		if err != nil && plaidErrorCode(err) == "TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION" {
			// Restart from the last committed cursor (what's in the DB) once.
			if dbErr := db.QueryRow(`SELECT cursor FROM items WHERE item_id = ?`, it.itemID).Scan(&it.cursor); dbErr != nil {
				err = errors.Join(err, fmt.Errorf("re-read cursor: %w", dbErr))
			} else {
				var a, m, r int
				a, m, r, err = syncOneItem(ctx, client, db, it)
				added, modified, removed = added+a, modified+m, removed+r
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
			fmt.Printf("%s: sync failed: %v\n", label, err)
			continue
		}

		fmt.Printf("%s: %d added, %d modified, %d removed\n", label, added, modified, removed)
		totalAdded += added
		totalModified += modified
		totalRemoved += removed
	}

	fmt.Printf("Total: %d added, %d modified, %d removed\n", totalAdded, totalModified, totalRemoved)
	return errors.Join(errs...)
}
