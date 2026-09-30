package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/plaid/plaid-go/v43/plaid"
)

func testPlaidAccountProvider(t *testing.T, handler http.HandlerFunc) plaidSpendingAccountProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	config := plaid.NewConfiguration()
	config.Servers = plaid.ServerConfigurations{{URL: server.URL}}
	config.HTTPClient = server.Client()
	return plaidSpendingAccountProvider{client: plaid.NewAPIClient(config)}
}

func TestSpendingAccountProviderSDKRequestsAndNullableMetadata(t *testing.T) {
	provider := testPlaidAccountProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		switch r.URL.Path {
		case "/link/token/create":
			hosted, ok := body["hosted_link"].(map[string]any)
			if !ok || hosted["url_lifetime_seconds"] != float64(1800) || body["enable_multi_item_link"] != false || !reflect.DeepEqual(body["products"], []any{"transactions"}) {
				t.Errorf("unexpected hosted request: %+v", body)
			}
			fmt.Fprint(w, `{"link_token":"secret-link","hosted_link_url":"https://secure.plaid.com/link/fixture","expiration":"2026-09-21T16:30:00Z","request_id":"request"}`)
		case "/link/token/get":
			if body["link_token"] != "secret-link" {
				t.Errorf("link token=%v", body["link_token"])
			}
			fmt.Fprint(w, `{"link_token":"secret-link","created_at":"2026-09-21T16:00:00Z","expiration":"2026-09-21T16:30:00Z","metadata":{},"request_id":"request","link_sessions":[{"link_session_id":"session","results":{"item_add_results":[{"public_token":"secret-public"}]}}]}`)
		case "/item/public_token/exchange":
			if body["public_token"] != "secret-public" {
				t.Errorf("public token=%v", body["public_token"])
			}
			fmt.Fprint(w, `{"access_token":"secret-access","item_id":"private-item","request_id":"request"}`)
		case "/accounts/get":
			if body["access_token"] != "secret-access" {
				t.Errorf("access token=%v", body["access_token"])
			}
			fmt.Fprint(w, `{"accounts":[{"account_id":"account-one","balances":{"current":42,"available":null,"limit":null},"mask":null,"name":"Savings","official_name":null,"type":"depository","subtype":null},{"account_id":"account-two","balances":{"current":0,"available":null,"limit":null},"mask":"1234","name":"Checking","official_name":null,"type":"depository","subtype":"checking"}],"item":{"item_id":"private-item","institution_id":"ins_test"},"request_id":"request"}`)
		case "/institutions/get_by_id":
			if body["institution_id"] != "ins_test" {
				t.Errorf("institution=%v", body["institution_id"])
			}
			fmt.Fprint(w, `{"institution":{"institution_id":"ins_test","name":"Fixture bank","products":[],"country_codes":["US"],"routing_numbers":[],"oauth":false},"request_id":"request"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	link, err := provider.StartLink(ctx)
	if err != nil || link.Token != "secret-link" || link.URL != "https://secure.plaid.com/link/fixture" || !link.ExpiresAt.Equal(time.Date(2026, 9, 21, 16, 30, 0, 0, time.UTC)) {
		t.Fatalf("start=%+v %v", link, err)
	}
	token, err := provider.PublicToken(ctx, link.Token)
	if err != nil || token != "secret-public" {
		t.Fatalf("public=%q %v", token, err)
	}
	item, err := provider.Exchange(ctx, token)
	if err != nil || item.ID != "private-item" || item.AccessToken != "secret-access" {
		t.Fatalf("exchange=%+v %v", item, err)
	}
	inventory, err := provider.Accounts(ctx, item.AccessToken)
	if err != nil || inventory.Institution != "Fixture bank" || len(inventory.Accounts) != 2 || inventory.Accounts[0].Mask != "" || inventory.Accounts[0].Subtype != "" || inventory.Accounts[1].Subtype != "checking" {
		t.Fatalf("inventory=%+v %v", inventory, err)
	}
}

func TestSpendingAccountProviderSDKGetErrorIsPreservedWithoutLogging(t *testing.T) {
	provider := testPlaidAccountProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error_type":"INVALID_INPUT","error_code":"INVALID_LINK_TOKEN","error_message":"secret-link-token","display_message":null,"request_id":"request"}`)
	})
	// The old CLI overwrote this SDK error while serializing and printing the
	// response. The shared adapter must return it without writing any output.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; reader.Close(); writer.Close() }()
	token, providerErr := provider.PublicToken(context.Background(), "secret-link-token")
	writer.Close()
	os.Stdout = original
	output, readErr := io.ReadAll(reader)
	if readErr != nil || len(output) != 0 {
		t.Fatalf("provider wrote output=%q err=%v", output, readErr)
	}
	if token != "" || providerErr == nil || plaidErrorCode(providerErr) != "INVALID_LINK_TOKEN" {
		t.Fatalf("lost SDK error: token=%q err=%v", token, providerErr)
	}
	s, db, p := accountTestService(t)
	link := startTestAccountLink(t, s)
	p.public = func(context.Context, string) (string, error) { return "", providerErr }
	_, err = s.CompleteAccountLink(context.Background(), link.ID)
	if err != ErrSpendingLinkUnavailable || strings.Contains(err.Error(), "secret-link-token") {
		t.Fatalf("unsanitized error: %v", err)
	}
	requireLinkState(t, db, link.ID, "pending")
}
