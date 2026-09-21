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

type spendingServiceStub struct {
	inspect      func(context.Context, int) (finance.SpendingReport, error)
	transactions func(context.Context, finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error)
	cashFlow     func(context.Context, finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error)
	refresh      func(context.Context) (finance.SpendingRefresh, error)
}

func (stub *spendingServiceStub) InspectSpending(ctx context.Context, year int) (finance.SpendingReport, error) {
	return stub.inspect(ctx, year)
}

func (stub *spendingServiceStub) RefreshSpending(ctx context.Context) (finance.SpendingRefresh, error) {
	return stub.refresh(ctx)
}

func (stub *spendingServiceStub) InspectSpendingTransactions(ctx context.Context, query finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
	return stub.transactions(ctx, query)
}

func (stub *spendingServiceStub) InspectSpendingCashFlow(ctx context.Context, query finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
	return stub.cashFlow(ctx, query)
}

func TestSpendingHTTPValidatesBeforeAccessingFinance(t *testing.T) {
	for _, endpoint := range []string{"summary", "transactions", "cash-flow", "refresh"} {
		t.Run(endpoint, func(t *testing.T) {
			validBody := `{}`
			if endpoint == "summary" {
				validBody = `{"year":2026}`
			} else if endpoint == "transactions" {
				validBody = `{"month":"2026-09"}`
			} else if endpoint == "cash-flow" {
				validBody = `{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20"}`
			}
			for _, test := range []struct {
				name, body, method, origin, host, content string
				status                                    int
			}{
				{name: "valid", body: validBody, status: http.StatusOK},
				{name: "method", body: validBody, method: "GET", status: http.StatusMethodNotAllowed},
				{name: "origin", body: validBody, origin: "https://example.com", status: http.StatusForbidden},
				{name: "host", body: validBody, host: "example.com", status: http.StatusForbidden},
				{name: "content", body: validBody, content: "text/plain", status: http.StatusForbidden},
				{name: "unknown field", body: `{"path":"private.sqlite"}`, status: http.StatusBadRequest},
				{name: "malformed", body: `{`, status: http.StatusBadRequest},
				{name: "missing body", body: "", status: http.StatusBadRequest},
				{name: "null", body: "null", status: http.StatusBadRequest},
				{name: "array", body: "[]", status: http.StatusBadRequest},
				{name: "trailing value", body: validBody + `{}`, status: http.StatusBadRequest},
				{name: "oversize", body: validBody + strings.Repeat(" ", maxManagementBodyBytes), status: http.StatusRequestEntityTooLarge},
			} {
				t.Run(test.name, func(t *testing.T) {
					inspects, refreshes, transactions, cashFlows := 0, 0, 0, 0
					stub := &spendingServiceStub{
						inspect: func(_ context.Context, year int) (finance.SpendingReport, error) {
							inspects++
							if year != 2026 {
								t.Errorf("year = %d; want 2026", year)
							}
							return finance.SpendingReport{}, nil
						},
						refresh: func(context.Context) (finance.SpendingRefresh, error) {
							refreshes++
							return finance.SpendingRefresh{}, nil
						},
						transactions: func(_ context.Context, query finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
							transactions++
							if query.Month != "2026-09" || query.Status != "all" || query.Offset != 0 || query.PageSize != 50 {
								t.Errorf("query not normalized: %+v", query)
							}
							return finance.SpendingTransactionsReport{}, nil
						},
						cashFlow: func(_ context.Context, query finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
							cashFlows++
							if query.Month != "2026-09" || query.HistoryEnd != "2026-09" || query.AsOfDate != "2026-09-20" {
								t.Errorf("cash-flow query changed: %+v", query)
							}
							return finance.SpendingCashFlowReport{}, nil
						},
					}
					handler := WithSpending(NewServer(nil), stub).Handler()
					method := test.method
					if method == "" {
						method = http.MethodPost
					}
					request := httptest.NewRequest(method, "http://localhost/api/data/spending/"+endpoint, strings.NewReader(test.body))
					content := test.content
					if content == "" {
						content = "application/json"
					}
					request.Header.Set("Content-Type", content)
					request.Header.Set("Origin", test.origin)
					if test.host != "" {
						request.Host = test.host
					}
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, request)
					if rec.Code != test.status {
						t.Fatalf("status %d; want %d: %s", rec.Code, test.status, rec.Body.String())
					}
					wantInspects, wantRefreshes, wantTransactions, wantCashFlows := 0, 0, 0, 0
					if test.status == http.StatusOK {
						if endpoint == "summary" {
							wantInspects = 1
						} else if endpoint == "transactions" {
							wantTransactions = 1
						} else if endpoint == "cash-flow" {
							wantCashFlows = 1
						} else {
							wantRefreshes = 1
						}
						if rec.Header().Get("Cache-Control") != "no-store" {
							t.Fatal("spending response is cacheable")
						}
					}
					if inspects != wantInspects || refreshes != wantRefreshes || transactions != wantTransactions || cashFlows != wantCashFlows {
						t.Fatalf("finance calls inspect=%d refresh=%d transactions=%d cash-flow=%d; want %d, %d, %d, %d", inspects, refreshes, transactions, cashFlows, wantInspects, wantRefreshes, wantTransactions, wantCashFlows)
					}
				})
			}
		})
	}
}

func TestSpendingHTTPSummaryRequiresCalendarYear(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		inspect: func(context.Context, int) (finance.SpendingReport, error) {
			t.Fatal("invalid year reached finance")
			return finance.SpendingReport{}, nil
		},
	}).Handler()
	for _, body := range []string{`{}`, `{"year":0}`, `{"year":-1}`, `{"year":10000}`, `{"year":2026.5}`, `{"year":"2026"}`} {
		t.Run(body, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), "summary", body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid year response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPTransactionsValidateMonthFilterAndPagination(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		transactions: func(context.Context, finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
			t.Fatal("invalid query reached finance")
			return finance.SpendingTransactionsReport{}, nil
		},
	}).Handler()
	for _, body := range []string{
		`{}`, `{"month":""}`, `{"month":"2026-9"}`, `{"month":"2026-00"}`, `{"month":"2026-13"}`,
		`{"month":"0000-09"}`, `{"month":"10000-09"}`, `{"month":"2026-09-20"}`, `{"month":" 2026-09"}`,
		`{"month":"2026-09","status":"proposed"}`, `{"month":"2026-09","status":"ALL"}`,
		`{"month":"2026-09","offset":-1}`, `{"month":"2026-09","offset":0.5}`,
		`{"month":"2026-09","pageSize":-1}`, `{"month":"2026-09","pageSize":101}`,
		`{"month":"2026-09","pageSize":"50"}`, `{"month":"2026-09","accountId":"private-account"}`,
	} {
		t.Run(body, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), "transactions", body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid transaction query response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPTransactionsPassFiltersPaginationAndContext(t *testing.T) {
	type markerKey struct{}
	ctx := context.WithValue(context.Background(), markerKey{}, "owner request")
	for _, status := range []string{"all", "unclassified", "classified", "pending"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				transactions: func(gotCtx context.Context, query finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
					calls++
					if gotCtx.Value(markerKey{}) != "owner request" || query.Month != "2026-09" || query.Status != status || query.Offset != 25 || query.PageSize != 100 {
						t.Errorf("query/context changed: %+v", query)
					}
					return finance.SpendingTransactionsReport{}, nil
				},
			}).Handler()
			rec := spendingHTTP(t, handler, ctx, "transactions", fmt.Sprintf(`{"month":"2026-09","status":%q,"offset":25,"pageSize":100}`, status))
			if rec.Code != http.StatusOK || calls != 1 {
				t.Fatalf("valid transaction query response %d: %s (calls=%d)", rec.Code, rec.Body.String(), calls)
			}
		})
	}
}

func TestSpendingHTTPTransactionsPreserveTypedRowsAndExactCents(t *testing.T) {
	const fixture = `{
		"month":"2026-09","status":"classified","offset":0,"pageSize":1,
		"total":2,"counts":{"all":4,"unclassified":1,"classified":2,"pending":1},
		"transactions":[{
			"id":"transaction-1","date":"2026-09-20","name":"Saved transaction","merchantName":"Merchant",
			"netCents":"-9223372036854775808","classification":"classified",
			"entries":[{"category":"Food","amountCents":"9223372036854775808","source":"rule"}],
			"legacyCategory":"Old category","legacySource":null
		}],"hasMore":true
	}`
	var report finance.SpendingTransactionsReport
	if err := json.Unmarshal([]byte(fixture), &report); err != nil {
		t.Fatal(err)
	}
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		transactions: func(context.Context, finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
			return report, nil
		},
	}).Handler()
	rec := spendingHTTP(t, handler, context.Background(), "transactions", `{"month":"2026-09","status":"classified","pageSize":1}`)
	var got, want map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("transaction response changed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSpendingHTTPCashFlowValidatesBeforeReading(t *testing.T) {
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		cashFlow: func(context.Context, finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
			t.Fatal("invalid cash-flow query reached finance")
			return finance.SpendingCashFlowReport{}, nil
		},
	}).Handler()
	for _, body := range []string{
		`{}`, `{"month":"2026-09"}`, `{"month":"2026-09","historyEnd":"2026-09"}`,
		`{"month":"2026-9","historyEnd":"2026-09","asOfDate":"2026-09-20"}`,
		`{"month":"2026-09","historyEnd":"2026-13","asOfDate":"2026-09-20"}`,
		`{"month":"0000-01","historyEnd":"0001-01","asOfDate":"0001-01-01"}`,
		`{"month":"9999-12","historyEnd":"10000-01","asOfDate":"9999-12-31"}`,
		`{"month":"2025-09","historyEnd":"2026-09","asOfDate":"2026-09-20"}`,
		`{"month":"2026-10","historyEnd":"2026-09","asOfDate":"2026-09-20"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-02-29"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-31"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-9-20"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20T00:00:00Z"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"0000-01-01"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"10000-01-01"}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":null}`,
		`{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20","accountId":"private-account"}`,
	} {
		t.Run(body, func(t *testing.T) {
			rec := spendingHTTP(t, handler, context.Background(), "cash-flow", body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("invalid cash-flow query response %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSpendingHTTPCashFlowPreservesQueryIdentityAndContext(t *testing.T) {
	type markerKey struct{}
	ctx := context.WithValue(context.Background(), markerKey{}, "cash-flow request")
	for _, query := range []finance.SpendingCashFlowQuery{
		{Month: "0001-01", HistoryEnd: "0001-06", AsOfDate: "0001-01-01"},
		{Month: "2024-02", HistoryEnd: "2024-12", AsOfDate: "2024-02-29"},
		{Month: "2025-10", HistoryEnd: "2026-09", AsOfDate: "2026-09-20"},
		{Month: "2026-09", HistoryEnd: "2026-09", AsOfDate: "2026-09-21"},
		{Month: "9999-12", HistoryEnd: "9999-12", AsOfDate: "9999-12-31"},
	} {
		t.Run(query.Month+"/"+query.AsOfDate, func(t *testing.T) {
			calls := 0
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				cashFlow: func(gotCtx context.Context, got finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
					calls++
					if got != query || gotCtx.Value(markerKey{}) != "cash-flow request" {
						t.Errorf("query/context changed: %+v", got)
					}
					return finance.SpendingCashFlowReport{Month: got.Month, HistoryEnd: got.HistoryEnd, AsOfDate: got.AsOfDate}, nil
				},
			}).Handler()
			body, err := json.Marshal(query)
			if err != nil {
				t.Fatal(err)
			}
			rec := spendingHTTP(t, handler, ctx, "cash-flow", string(body))
			var report finance.SpendingCashFlowReport
			if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || calls != 1 || report.Month != query.Month || report.HistoryEnd != query.HistoryEnd || report.AsOfDate != query.AsOfDate {
				t.Fatalf("cash-flow query identity response %d: %s (calls=%d)", rec.Code, rec.Body.String(), calls)
			}
		})
	}
}

func TestSpendingHTTPCashFlowPreservesExactCentsAndCategoryBuckets(t *testing.T) {
	const fixture = `{
		"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20",
		"history":[{"month":"2026-09","inflowCents":"9223372036854775808","outflowCents":"300","netCents":"9223372036854775508","transactions":3}],
		"selected":{"inflowCents":"9223372036854775808","outflowCents":"300","netCents":"9223372036854775508","transactions":3},
		"categories":[
			{"kind":"category","category":"Income","inflowCents":"9223372036854775808","outflowCents":"0","netCents":"9223372036854775808","transactions":1},
			{"kind":"unclassified","category":null,"inflowCents":"0","outflowCents":"100","netCents":"-100","transactions":1},
			{"kind":"unreconciled","category":null,"inflowCents":"0","outflowCents":"200","netCents":"-200","transactions":1}
		],"pendingTransactions":2,"undatedTransactions":1
	}`
	var report finance.SpendingCashFlowReport
	if err := json.Unmarshal([]byte(fixture), &report); err != nil {
		t.Fatal(err)
	}
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		cashFlow: func(context.Context, finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
			return report, nil
		},
	}).Handler()
	rec := spendingHTTP(t, handler, context.Background(), "cash-flow", `{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20"}`)
	var got, want map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !reflect.DeepEqual(got, want) {
		t.Fatalf("cash-flow response changed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSpendingHTTPPreservesExactTotalsAndPartialRefresh(t *testing.T) {
	const cents = "92233720368547758081234"
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		inspect: func(context.Context, int) (finance.SpendingReport, error) {
			return finance.SpendingReport{
				Year: 2026, Years: []int{2026}, LinkedBanks: 2,
				Days: []finance.SpendingDay{{Date: "2026-09-19", InflowCents: cents, OutflowCents: "0", NetCents: cents, Transactions: 3}},
			}, nil
		},
		refresh: func(context.Context) (finance.SpendingRefresh, error) {
			return finance.SpendingRefresh{Added: 3, BanksSucceeded: 1, BanksFailed: 1}, nil
		},
	}).Handler()
	rec := spendingHTTP(t, handler, context.Background(), "summary", `{"year":2026}`)
	var report finance.SpendingReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || report.Year != 2026 || len(report.Days) != 1 || report.Days[0].NetCents != cents || report.Days[0].InflowCents != cents {
		t.Fatalf("daily totals changed at HTTP boundary: %s", rec.Body.String())
	}
	rec = spendingHTTP(t, handler, context.Background(), "refresh", `{}`)
	var refresh finance.SpendingRefresh
	if err := json.Unmarshal(rec.Body.Bytes(), &refresh); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || refresh.Added != 3 || refresh.BanksSucceeded != 1 || refresh.BanksFailed != 1 || refresh.RefreshedAt != nil {
		t.Fatalf("partial refresh outcome changed at HTTP boundary: %s", rec.Body.String())
	}
}

func TestSpendingHTTPErrorsAreSafeAndTyped(t *testing.T) {
	for _, test := range []struct {
		name, endpoint, code string
		err                  error
		status               int
	}{
		{"summary unavailable", "summary", "spending_unavailable", errors.New("access-token-secret database /private/finance.db"), http.StatusUnprocessableEntity},
		{"transactions unavailable", "transactions", "spending_unavailable", errors.New("access-token-secret database /private/finance.db"), http.StatusUnprocessableEntity},
		{"cash-flow unavailable", "cash-flow", "spending_unavailable", errors.New("access-token-secret database /private/finance.db"), http.StatusUnprocessableEntity},
		{"refresh unavailable", "refresh", "spending_refresh_unavailable", errors.New("Plaid client-secret-secret"), http.StatusUnprocessableEntity},
		{"overlap", "refresh", "spending_refresh_in_progress", fmt.Errorf("access-token-secret: %w", finance.ErrSpendingRefreshInProgress), http.StatusConflict},
		{"year", "summary", "invalid_spending_year", finance.ErrSpendingYear, http.StatusBadRequest},
		{"transactions query", "transactions", "invalid_spending_transactions_query", finance.ErrSpendingTransactionsQuery, http.StatusBadRequest},
		{"cash-flow query", "cash-flow", "invalid_spending_cash_flow_query", finance.ErrSpendingCashFlowQuery, http.StatusBadRequest},
		{"summary cancellation", "summary", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"refresh cancellation", "refresh", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"transactions cancellation", "transactions", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"cash-flow cancellation", "cash-flow", "spending_cancelled", context.Canceled, http.StatusRequestTimeout},
		{"cash-flow timeout", "cash-flow", "spending_timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"timeout", "refresh", "spending_timeout", context.DeadlineExceeded, http.StatusGatewayTimeout},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := WithSpending(NewServer(nil), &spendingServiceStub{
				inspect: func(context.Context, int) (finance.SpendingReport, error) { return finance.SpendingReport{}, test.err },
				refresh: func(context.Context) (finance.SpendingRefresh, error) { return finance.SpendingRefresh{}, test.err },
				transactions: func(context.Context, finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
					return finance.SpendingTransactionsReport{}, test.err
				},
				cashFlow: func(context.Context, finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
					return finance.SpendingCashFlowReport{}, test.err
				},
			}).Handler()
			body := `{}`
			if test.endpoint == "summary" {
				body = `{"year":2026}`
			} else if test.endpoint == "transactions" {
				body = `{"month":"2026-09"}`
			} else if test.endpoint == "cash-flow" {
				body = `{"month":"2026-09","historyEnd":"2026-09","asOfDate":"2026-09-20"}`
			}
			rec := spendingHTTP(t, handler, context.Background(), test.endpoint, body)
			var result struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rec.Code != test.status || result.Code != test.code || result.Error == "" {
				t.Fatalf("response %d %s; want %d %s", rec.Code, rec.Body.String(), test.status, test.code)
			}
			if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "private") || strings.Contains(rec.Body.String(), "Plaid") {
				t.Fatal("private service error leaked")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("error response is cacheable")
			}
		})
	}
}

func TestSpendingHTTPRefreshUsesBoundedRequestContext(t *testing.T) {
	type markerKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), markerKey{}, "request"))
	defer cancel()
	started := make(chan struct{})
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		refresh: func(ctx context.Context) (finance.SpendingRefresh, error) {
			deadline, bounded := ctx.Deadline()
			if !bounded || time.Until(deadline) > spendingRefreshTimeout {
				t.Error("refresh context has no bounded deadline")
			}
			if ctx.Value(markerKey{}) != "request" {
				t.Error("refresh context did not preserve request")
			}
			close(started)
			<-ctx.Done()
			return finance.SpendingRefresh{}, ctx.Err()
		},
	}).Handler()
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- spendingHTTP(t, handler, ctx, "refresh", `{}`) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case rec := <-finished:
		if rec.Code != http.StatusRequestTimeout {
			t.Fatalf("cancelled refresh response %d: %s", rec.Code, rec.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not stop refresh")
	}
}

func TestSpendingHTTPRefreshHonorsEarlierDeadline(t *testing.T) {
	deadline := time.Now().Add(-time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	handler := WithSpending(NewServer(nil), &spendingServiceStub{
		refresh: func(ctx context.Context) (finance.SpendingRefresh, error) {
			actual, _ := ctx.Deadline()
			if !actual.Equal(deadline) || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Error("refresh replaced an earlier request deadline")
			}
			return finance.SpendingRefresh{}, ctx.Err()
		},
	}).Handler()
	rec := spendingHTTP(t, handler, ctx, "refresh", `{}`)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expired refresh response %d: %s", rec.Code, rec.Body.String())
	}
}

func spendingHTTP(t *testing.T, handler http.Handler, ctx context.Context, endpoint, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/data/spending/"+endpoint, strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	return rec
}
