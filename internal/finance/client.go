package finance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

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
