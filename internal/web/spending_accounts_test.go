package web

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/finance"
)

type spendingAccountsStub struct {
	spendingServiceStub
	called func(context.Context, string)
	err    error
}

func (s *spendingAccountsStub) InspectAccounts(ctx context.Context) (finance.SpendingAccountsReport, error) {
	s.called(ctx, "accounts")
	return finance.SpendingAccountsReport{Institutions: []finance.SpendingInstitution{}, LinkAvailable: true}, s.err
}
func (s *spendingAccountsStub) RefreshAccounts(ctx context.Context) (finance.SpendingAccountsRefresh, error) {
	s.called(ctx, "accounts/refresh")
	return finance.SpendingAccountsRefresh{InstitutionsSucceeded: 1, InstitutionsFailed: 1}, s.err
}
func (s *spendingAccountsStub) StartAccountLink(ctx context.Context) (finance.SpendingAccountLink, error) {
	s.called(ctx, "link/start")
	return finance.SpendingAccountLink{ID: "local-link", HostedURL: "https://secure.plaid.com/link/test", ExpiresAt: time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC)}, s.err
}
func (s *spendingAccountsStub) CompleteAccountLink(ctx context.Context, id string) (finance.SpendingAccountLinkResult, error) {
	s.called(ctx, "link/status:"+id)
	return finance.SpendingAccountLinkResult{Status: "linked", InstitutionsLinked: 1}, s.err
}
func (s *spendingAccountsStub) CancelAccountLink(ctx context.Context, id string) error {
	s.called(ctx, "link/cancel:"+id)
	return s.err
}

func TestSpendingAccountsHTTPGuardsAndStrictBodies(t *testing.T) {
	for _, action := range []string{"accounts", "accounts/refresh", "link/start", "link/status", "link/cancel"} {
		t.Run(action, func(t *testing.T) {
			valid := `{}`
			if action == "link/status" || action == "link/cancel" {
				valid = `{"id":"local-link"}`
			}
			for _, test := range []struct {
				name, method, body, origin, host, content string
				status                                    int
			}{
				{name: "valid", body: valid, status: 200},
				{name: "method", method: "GET", body: valid, status: 405},
				{name: "origin", origin: "https://untrusted.example", body: valid, status: 403},
				{name: "host", host: "untrusted.example", body: valid, status: 403},
				{name: "content", content: "text/plain", body: valid, status: 403},
				{name: "provider token", body: `{"public_token":"secret"}`, status: 400},
				{name: "null", body: `null`, status: 400},
				{name: "array", body: `[]`, status: 400},
				{name: "missing", body: ``, status: 400},
				{name: "trailing", body: valid + `{}`, status: 400},
				{name: "oversize", body: valid + strings.Repeat(" ", maxManagementBodyBytes), status: 413},
			} {
				t.Run(test.name, func(t *testing.T) {
					calls := 0
					stub := &spendingAccountsStub{called: func(ctx context.Context, called string) {
						calls++
						if called != action && called != action+":local-link" {
							t.Errorf("unexpected action %q", called)
						}
						deadline, ok := ctx.Deadline()
						if !ok || time.Until(deadline) > 2*time.Minute {
							t.Error("provider request must be bounded")
						}
					}}
					handler := WithSpending(NewServer(nil), stub).Handler()
					method := test.method
					if method == "" {
						method = "POST"
					}
					req := httptest.NewRequest(method, "http://localhost/api/data/spending/"+action, strings.NewReader(test.body))
					req.Header.Set("Content-Type", "application/json")
					if test.origin != "" {
						req.Header.Set("Origin", test.origin)
					}
					if test.content != "" {
						req.Header.Set("Content-Type", test.content)
					}
					if test.host != "" {
						req.Host = test.host
					}
					rec := httptest.NewRecorder()
					handler.ServeHTTP(rec, req)
					if rec.Code != test.status {
						t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
					}
					if test.status == 200 {
						if calls != 1 {
							t.Fatalf("calls=%d", calls)
						}
						if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
							t.Fatal("connection responses must not be cached or used as referrers")
						}
					} else if calls != 0 {
						t.Fatalf("invalid request invoked provider %d times", calls)
					}
				})
			}
		})
	}
}

func TestSpendingAccountsHTTPRejectsInvalidLinkHandles(t *testing.T) {
	for _, action := range []string{"status", "cancel"} {
		for _, body := range []string{`{}`, `{"id":""}`, `{"id":" padded "}`, `{"id":"` + strings.Repeat("a", 129) + `"}`, `{"id":"local-link","url":"https://untrusted.example"}`} {
			stub := &spendingAccountsStub{called: func(context.Context, string) { t.Fatal("invalid handle reached service") }}
			req := httptest.NewRequest("POST", "http://localhost/api/data/spending/link/"+action, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			WithSpending(NewServer(nil), stub).Handler().ServeHTTP(rec, req)
			if rec.Code != 400 {
				t.Fatalf("status %d for %s", rec.Code, body)
			}
		}
	}
}

func TestSpendingAccountsHTTPSafeErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{errors.New("private access-token provider-response bank-id"), 422},
		{finance.ErrSpendingAccountsRefreshInProgress, 409},
		{finance.ErrSpendingLinkInProgress, 409},
		{finance.ErrSpendingLinkNotFound, 404},
		{context.Canceled, 408},
		{context.DeadlineExceeded, 504},
	} {
		for _, action := range []string{"accounts", "accounts/refresh", "link/start", "link/status", "link/cancel"} {
			body := `{}`
			if action == "link/status" || action == "link/cancel" {
				body = `{"id":"local-link"}`
			}
			stub := &spendingAccountsStub{called: func(context.Context, string) {}, err: test.err}
			req := httptest.NewRequest("POST", "http://localhost/api/data/spending/"+action, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			WithSpending(NewServer(nil), stub).Handler().ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Fatalf("%s: status %d for %v", action, rec.Code, test.err)
			}
			for _, secret := range []string{"private", "access-token", "provider-response", "bank-id"} {
				if strings.Contains(rec.Body.String(), secret) {
					t.Fatalf("raw error exposed: %s", rec.Body.String())
				}
			}
		}
	}
}
