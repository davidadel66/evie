package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/plaid/plaid-go/v43/plaid"
)

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
