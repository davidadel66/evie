package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/finance"
)

const spendingCategoryRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const spendingCategoryBody = `{"transactionId":"transaction-1","entryId":"17","category":"Food","revision":"` + spendingCategoryRevision + `"}`

func TestSpendingHTTPDayAndCategoryGuardBeforeFinanceAccess(t *testing.T) {
	for _, endpoint := range []string{"day", "category"} {
		t.Run(endpoint, func(t *testing.T) {
			body := `{"date":"2026-09-20"}`
			if endpoint == "category" {
				body = spendingCategoryBody
			}
			for _, test := range []struct {
				name, method, origin, host, content string
				status                              int
			}{
				{name: "valid local", method: http.MethodPost, origin: "http://localhost", host: "localhost", content: "application/json", status: http.StatusOK},
				{name: "GET", method: http.MethodGet, host: "localhost", content: "application/json", status: http.StatusMethodNotAllowed},
				{name: "foreign origin", method: http.MethodPost, origin: "https://example.com", host: "localhost", content: "application/json", status: http.StatusForbidden},
				{name: "null origin", method: http.MethodPost, origin: "null", host: "localhost", content: "application/json", status: http.StatusForbidden},
				{name: "foreign host", method: http.MethodPost, host: "example.com", content: "application/json", status: http.StatusForbidden},
				{name: "plain text", method: http.MethodPost, host: "localhost", content: "text/plain", status: http.StatusForbidden},
				{name: "form", method: http.MethodPost, host: "localhost", content: "application/x-www-form-urlencoded", status: http.StatusForbidden},
				{name: "missing content type", method: http.MethodPost, host: "localhost", status: http.StatusForbidden},
			} {
				t.Run(test.name, func(t *testing.T) {
					reads, writes := 0, 0
					handler := WithSpending(NewServer(nil), &spendingServiceStub{
						day: func(context.Context, finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
							reads++
							return finance.SpendingDayReport{}, nil
						},
						category: func(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
							writes++
							return finance.SpendingDayTransaction{}, nil
						},
					}).Handler()
					request := httptest.NewRequest(test.method, "http://localhost/api/data/spending/"+endpoint, strings.NewReader(body))
					request.Host = test.host
					request.Header.Set("Origin", test.origin)
					request.Header.Set("Content-Type", test.content)
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, request)
					if rec.Code != test.status {
						t.Fatalf("response %d: %s; want %d", rec.Code, rec.Body.String(), test.status)
					}
					wantReads, wantWrites := 0, 0
					if test.status == http.StatusOK {
						if endpoint == "day" {
							wantReads = 1
						} else {
							wantWrites = 1
						}
						if rec.Header().Get("Cache-Control") != "no-store" {
							t.Fatal("spending response is cacheable")
						}
					}
					if reads != wantReads || writes != wantWrites {
						t.Fatalf("finance reads=%d writes=%d; want %d/%d", reads, writes, wantReads, wantWrites)
					}
				})
			}
		})
	}
}

func TestSpendingHTTPDayAndCategoryRejectInvalidBodiesBeforeFinance(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		day: func(context.Context, finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
			t.Fatal("invalid body reached finance read")
			return finance.SpendingDayReport{}, nil
		},
		category: func(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
			t.Fatal("invalid body reached finance write")
			return finance.SpendingDayTransaction{}, nil
		},
	}).Handler()
	for _, endpoint := range []string{"day", "category"} {
		t.Run(endpoint, func(t *testing.T) {
			valid := `{"date":"2026-09-20"}`
			if endpoint == "category" {
				valid = spendingCategoryBody
			}
			for _, test := range []struct {
				name, body string
				status     int
			}{
				{"empty", "", http.StatusBadRequest},
				{"malformed", "{", http.StatusBadRequest},
				{"null", "null", http.StatusBadRequest},
				{"array", "[]", http.StatusBadRequest},
				{"trailing", valid + `{}`, http.StatusBadRequest},
				{"unknown field", `{"path":"/private/finance.db"}`, http.StatusBadRequest},
				{"oversize", valid + strings.Repeat(" ", maxManagementBodyBytes), http.StatusRequestEntityTooLarge},
			} {
				t.Run(test.name, func(t *testing.T) {
					rec := spendingHTTP(t, handler, context.Background(), endpoint, test.body)
					if rec.Code != test.status || rec.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("response %d: %s; want %d and no-store", rec.Code, rec.Body.String(), test.status)
					}
				})
			}
		})
	}
}

func TestSpendingHTTPDayValidatesDateAndPageBeforeReading(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		day: func(context.Context, finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
			t.Fatal("invalid day reached finance")
			return finance.SpendingDayReport{}, nil
		},
	}).Handler()
	for _, body := range []string{
		`{}`, `{"date":null}`, `{"date":"2026-09"}`, `{"date":"2026-9-20"}`,
		`{"date":"2026-02-29"}`, `{"date":"2026-09-31"}`, `{"date":"0000-01-01"}`,
		`{"date":"10000-01-01"}`, `{"date":" 2026-09-20"}`, `{"date":"2026-09-20T00:00:00Z"}`,
		`{"date":"2026-09-20","offset":-1}`, `{"date":"2026-09-20","offset":0.5}`,
		`{"date":"2026-09-20","pageSize":-1}`, `{"date":"2026-09-20","pageSize":101}`,
		`{"date":"2026-09-20","pageSize":"50"}`, `{"date":"2026-09-20","status":"pending"}`,
	} {
		t.Run(body, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), "day", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("invalid day response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPDayPreservesDatePaginationAndContext(t *testing.T) {
	type markerKey struct{}
	ctx := context.WithValue(context.Background(), markerKey{}, "day request")
	for _, query := range []finance.SpendingDayQuery{
		{Date: "0001-01-01"}, {Date: "2024-02-29", Offset: 25, PageSize: 100}, {Date: "9999-12-31", PageSize: 1},
	} {
		t.Run(query.Date, func(t *testing.T) {
			want := query
			if want.PageSize == 0 {
				want.PageSize = 50
			}
			calls := 0
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				day: func(gotCtx context.Context, got finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
					calls++
					if got != want || gotCtx.Value(markerKey{}) != "day request" {
						t.Errorf("query/context changed: %+v; want %+v", got, want)
					}
					return finance.SpendingDayReport{}, nil
				},
			}).Handler()
			body, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			rec := spendingHTTP(t, handler, ctx, "day", string(body))
			if rec.Code != http.StatusOK || calls != 1 {
				t.Fatalf("valid day response %d: %s; calls=%d", rec.Code, rec.Body.String(), calls)
			}
		})
	}
}

func TestSpendingHTTPCategoryValidatesBeforeWriting(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		category: func(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
			t.Fatal("invalid category query reached finance write")
			return finance.SpendingDayTransaction{}, nil
		},
	}).Handler()
	for _, body := range []string{
		`{}`, `{"transactionId":"transaction-1"}`,
		strings.Replace(spendingCategoryBody, `"transaction-1"`, `""`, 1),
		strings.Replace(spendingCategoryBody, `"transaction-1"`, `null`, 1),
		strings.Replace(spendingCategoryBody, `"17"`, `17`, 1),
		strings.Replace(spendingCategoryBody, `"Food"`, `""`, 1),
		strings.Replace(spendingCategoryBody, `"Food"`, `null`, 1),
		strings.Replace(spendingCategoryBody, spendingCategoryRevision, "", 1),
		strings.Replace(spendingCategoryBody, `"revision":"`+spendingCategoryRevision+`"`, `"amountCents":"999"`, 1),
		strings.TrimSuffix(spendingCategoryBody, "}") + `,"createRule":true}`,
	} {
		t.Run(body, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), "category", body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid category response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPCategoryPreservesEntryAndRevision(t *testing.T) {
	type markerKey struct{}
	ctx := context.WithValue(context.Background(), markerKey{}, "category request")
	for _, entry := range []string{"17", ""} {
		t.Run(entry, func(t *testing.T) {
			query := finance.SpendingCategoryUpdate{TransactionID: "transaction-1", Category: "Food", Revision: spendingCategoryRevision}
			if entry != "" {
				query.EntryID = &entry
			}
			calls := 0
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				category: func(gotCtx context.Context, got finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
					calls++
					if !reflect.DeepEqual(got, query) || gotCtx.Value(markerKey{}) != "category request" {
						t.Errorf("category request changed: %+v; want %+v", got, query)
					}
					return finance.SpendingDayTransaction{}, nil
				},
			}).Handler()
			body, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			rec := spendingHTTP(t, handler, ctx, "category", string(body))
			if rec.Code != http.StatusOK || calls != 1 {
				t.Fatalf("category update response %d: %s; calls=%d", rec.Code, rec.Body.String(), calls)
			}
		})
	}
}

func TestSpendingHTTPDayAndCategoryPreserveRowsAndExactCents(t *testing.T) {
	const row = `{"id":"transaction-1","date":"2026-09-20","name":"Saved description","merchantName":"Merchant","netCents":"-9223372036854775808","classification":"classified","entries":[{"id":"17","category":"Food","amountCents":"9223372036854775807","source":"human"},{"id":"18","category":"Shopping","amountCents":"1","source":"rule"}],"legacyCategory":"Old category","legacySource":null,"revision":"` + spendingCategoryRevision + `"}`
	const day = `{"date":"2026-09-20","offset":0,"pageSize":1,"total":2,"hasMore":true,"categories":["Food","Shopping"],"summary":{"date":"2026-09-20","inflowCents":"0","outflowCents":"9223372036854775808","netCents":"-9223372036854775808","transactions":2},"transactions":[` + row + `]}`
	var transaction finance.SpendingDayTransaction
	var report finance.SpendingDayReport
	if err := json.Unmarshal([]byte(row), &transaction); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(day), &report); err != nil {
		t.Fatal(err)
	}
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		day: func(context.Context, finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
			return report, nil
		},
		category: func(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
			return transaction, nil
		},
	}).Handler()
	for _, test := range []struct{ endpoint, body, response string }{
		{"day", `{"date":"2026-09-20","pageSize":1}`, day}, {"category", spendingCategoryBody, row},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), test.endpoint, test.body)
			var got, want map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.response), &want); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
				t.Fatalf("response changed: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPDayAndCategoryErrorsAreTypedAndSafe(t *testing.T) {
	for _, test := range []struct {
		name, endpoint, code string
		err                  error
		status               int
	}{
		{"day unavailable", "day", "spending_unavailable", errors.New("access-token-secret /private/finance.db"), http.StatusUnprocessableEntity},
		{"category unavailable", "category", "spending_category_unavailable", errors.New("access-token-secret /private/finance.db"), http.StatusUnprocessableEntity},
		{"day query", "day", "invalid_spending_day_query", finance.ErrSpendingDayQuery, http.StatusBadRequest},
		{"category query", "category", "invalid_spending_category_query", finance.ErrSpendingCategoryQuery, http.StatusBadRequest},
		{"category conflict", "category", "spending_category_conflict", fmt.Errorf("private revision: %w", finance.ErrSpendingCategoryConflict), http.StatusConflict},
		{"category typed unavailable", "category", "spending_category_unavailable", finance.ErrSpendingCategoryUnavailable, http.StatusUnprocessableEntity},
		{"day canceled", "day", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"category canceled", "category", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"day timeout", "day", "spending_timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"category timeout", "category", "spending_timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				day: func(context.Context, finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
					return finance.SpendingDayReport{}, test.err
				},
				category: func(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
					return finance.SpendingDayTransaction{}, test.err
				},
			}).Handler()
			body := `{"date":"2026-09-20"}`
			if test.endpoint == "category" {
				body = spendingCategoryBody
			}
			rec := spendingHTTP(t, handler, context.Background(), test.endpoint, body)
			var response struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != test.status || response.Code != test.code || response.Error == "" {
				t.Fatalf("response %d: %s; want %d %s", rec.Code, rec.Body.String(), test.status, test.code)
			}
			if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "private") {
				t.Fatal("private service error leaked")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("error response is cacheable")
			}
		})
	}
}

func TestSpendingHTTPDayAndCategoryHonorRequestCancellation(t *testing.T) {
	for _, endpoint := range []string{"day", "category"} {
		t.Run(endpoint, func(t *testing.T) {
			deadline := time.Now().Add(-time.Second)
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			checkContext := func(got context.Context) error {
				actual, bounded := got.Deadline()
				if !bounded || !actual.Equal(deadline) || !errors.Is(got.Err(), context.DeadlineExceeded) {
					t.Error("finance did not receive canceled request context")
				}
				return got.Err()
			}
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				day: func(ctx context.Context, _ finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
					return finance.SpendingDayReport{}, checkContext(ctx)
				},
				category: func(ctx context.Context, _ finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
					return finance.SpendingDayTransaction{}, checkContext(ctx)
				},
			}).Handler()
			body := `{"date":"2026-09-20"}`
			if endpoint == "category" {
				body = spendingCategoryBody
			}
			rec := spendingHTTP(t, handler, ctx, endpoint, body)
			if rec.Code != http.StatusGatewayTimeout {
				t.Fatalf("canceled response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
