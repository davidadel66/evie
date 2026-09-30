package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
)

// All finance responses are invented. The real shell uses only this fixture's
// new context database; neither a provider nor the owner's finance DB is used.
type spendingAccountsBrowserService struct {
	t                 *testing.T
	output            string
	mu                sync.Mutex
	counts            map[string]int
	updated           time.Time
	refreshed, linked bool
	mode, status      string
	polls             int
	pending           *finance.SpendingAccountLink
}

var _ web.SpendingService = (*spendingAccountsBrowserService)(nil)
var _ web.SpendingAccountsService = (*spendingAccountsBrowserService)(nil)

func (s *spendingAccountsBrowserService) countLocked(name string) {
	s.counts[name]++
	raw, err := json.Marshal(s.counts)
	if err == nil {
		err = os.WriteFile(filepath.Join(s.output, "counts.tmp"), raw, 0600)
	}
	if err == nil {
		err = os.Rename(filepath.Join(s.output, "counts.tmp"), filepath.Join(s.output, "counts.json"))
	}
	if err != nil {
		s.t.Errorf("write fixture counters: %v", err)
	}
}

func (s *spendingAccountsBrowserService) count(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked(name)
}

func (s *spendingAccountsBrowserService) InspectAccounts(context.Context) (finance.SpendingAccountsReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("inspectAccounts")
	updated := s.updated
	report := finance.SpendingAccountsReport{LinkAvailable: true, Institutions: []finance.SpendingInstitution{
		{ID: "fixture-bank", Name: "Example Bank", AccountsKnown: true, AccountsUpdatedAt: &updated, Accounts: []finance.SpendingAccount{
			{ID: "fixture-checking", Name: "Everyday Checking", Mask: "1234", Type: "depository", Subtype: "checking"},
			{ID: "fixture-savings", Name: "Rainy Day Savings", Mask: "", Type: "depository", Subtype: "savings"},
		}},
		{ID: "fixture-legacy", Name: "Legacy Credit Union", AccountsKnown: s.refreshed, Accounts: []finance.SpendingAccount{}},
	}}
	if s.refreshed {
		report.Institutions[1].AccountsUpdatedAt = &updated
		report.Institutions[1].Accounts = []finance.SpendingAccount{{ID: "fixture-legacy-card", Name: "Everyday Card", Mask: "5678", Type: "credit", Subtype: "credit card"}}
	}
	if s.linked {
		report.Institutions = append(report.Institutions, finance.SpendingInstitution{ID: "fixture-new", Name: "New Example Bank", AccountsKnown: true, AccountsUpdatedAt: &updated, Accounts: []finance.SpendingAccount{{ID: "fixture-new-account", Name: "New Checking", Mask: "9012", Type: "depository", Subtype: "checking"}}})
	}
	if s.pending != nil {
		link := *s.pending
		report.PendingLink = &link
	}
	return report, nil
}

func (s *spendingAccountsBrowserService) RefreshAccounts(context.Context) (finance.SpendingAccountsRefresh, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("refreshAccounts")
	s.refreshed = true
	s.updated = time.Now().UTC()
	return finance.SpendingAccountsRefresh{InstitutionsSucceeded: 2}, nil
}

func (s *spendingAccountsBrowserService) startLocked() finance.SpendingAccountLink {
	link := finance.SpendingAccountLink{ID: "fixture-link", HostedURL: "https://secure.plaid.com/link/fixture", ExpiresAt: time.Now().Add(30 * time.Minute)}
	s.pending, s.polls, s.status = &link, 0, "pending"
	return link
}

func (s *spendingAccountsBrowserService) StartAccountLink(context.Context) (finance.SpendingAccountLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("startLink")
	if s.pending != nil {
		return *s.pending, nil
	}
	return s.startLocked(), nil
}

func (s *spendingAccountsBrowserService) CompleteAccountLink(_ context.Context, id string) (finance.SpendingAccountLinkResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("linkStatus")
	if id != "fixture-link" || s.status == "" {
		return finance.SpendingAccountLinkResult{}, finance.ErrSpendingLinkNotFound
	}
	s.polls++
	if s.status == "pending" && s.polls >= 3 && s.mode != "pending" {
		if s.mode == "error" {
			return finance.SpendingAccountLinkResult{}, finance.ErrSpendingLinkUnavailable
		}
		s.status, s.pending = s.mode, nil
		s.linked = s.status == "linked"
	}
	result := finance.SpendingAccountLinkResult{Status: s.status}
	if s.status == "linked" {
		result.InstitutionsLinked = 1
	}
	return result, nil
}

func (s *spendingAccountsBrowserService) CancelAccountLink(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("cancelLink")
	if id != "fixture-link" {
		return finance.ErrSpendingLinkNotFound
	}
	s.pending, s.status = nil, "cancelled"
	return nil
}

// POST {"mode":"linked|pending|expired|cancelled|error"}, then reload the UI.
// This supplies a saved link for checking completion without opening Plaid.
func (s *spendingAccountsBrowserService) configure(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode string `json:"mode"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil {
		http.Error(w, "POST a fixture mode", http.StatusBadRequest)
		return
	}
	switch request.Mode {
	case "linked", "pending", "expired", "cancelled", "error":
	default:
		http.Error(w, "unknown fixture mode", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode, s.linked = request.Mode, false
	s.startLocked()
	s.countLocked("configure")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"mode": s.mode})
}

func (s *spendingAccountsBrowserService) InspectSpending(_ context.Context, year int) (finance.SpendingReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.countLocked("inspectSpending")
	banks := 2
	if s.linked {
		banks++
	}
	currency := "USD"
	return finance.SpendingReport{Year: year, Years: []int{year}, LinkedBanks: banks, Days: []finance.SpendingDay{}, Currency: &currency}, nil
}

func (s *spendingAccountsBrowserService) InspectSpendingTransactions(_ context.Context, q finance.SpendingTransactionsQuery) (finance.SpendingTransactionsReport, error) {
	s.count("inspectTransactions")
	return finance.SpendingTransactionsReport{Month: q.Month, Status: q.Status, Offset: q.Offset, PageSize: q.PageSize, Transactions: []finance.SpendingTransaction{}}, nil
}

func (s *spendingAccountsBrowserService) InspectSpendingCashFlow(_ context.Context, q finance.SpendingCashFlowQuery) (finance.SpendingCashFlowReport, error) {
	s.count("inspectCashFlow")
	return finance.SpendingCashFlowReport{Month: q.Month, HistoryEnd: q.HistoryEnd, AsOfDate: q.AsOfDate, History: []finance.SpendingCashFlowMonth{}, Categories: []finance.SpendingCashFlowCategory{}, Selected: finance.SpendingCashFlowTotals{InflowCents: "0", OutflowCents: "0", NetCents: "0"}}, nil
}

func (s *spendingAccountsBrowserService) InspectSpendingDay(_ context.Context, q finance.SpendingDayQuery) (finance.SpendingDayReport, error) {
	s.count("inspectDay")
	return finance.SpendingDayReport{Date: q.Date, Offset: q.Offset, PageSize: q.PageSize, Categories: []string{}, Transactions: []finance.SpendingDayTransaction{}, Summary: finance.SpendingDay{Date: q.Date, InflowCents: "0", OutflowCents: "0", NetCents: "0"}}, nil
}

func (s *spendingAccountsBrowserService) UpdateSpendingCategory(context.Context, finance.SpendingCategoryUpdate) (finance.SpendingDayTransaction, error) {
	s.count("updateCategory")
	return finance.SpendingDayTransaction{}, finance.ErrSpendingCategoryUnavailable
}

func (s *spendingAccountsBrowserService) RefreshSpending(context.Context) (finance.SpendingRefresh, error) {
	s.count("refreshSpending")
	return finance.SpendingRefresh{BanksSucceeded: 2}, nil
}

func TestSpendingAccountsBrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_SPENDING_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in Spending accounts browser demonstration")
	}
	output := os.Getenv("EVIE_SPENDING_BROWSER_OUTPUT")
	if !filepath.IsAbs(output) {
		t.Fatal("EVIE_SPENDING_BROWSER_OUTPUT must be an absolute NEW directory")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(output, "browser.db")
	db, err := eviedb.OpenDBAt(database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, manager := eviedb.NewStore(db), sessionCompositionManager(t)
	standard, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(context.Background(), "General")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateWorkspaceSessionWithComposition(context.Background(), workspace.ID, workspace.CurrentRevisionID, standard.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "fixture-seed", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEventWithLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Review this month's spending"}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	profile, err := openrouter.NewExplicitContextProfile("test/model", 300000, 200000, 12000)
	if err != nil {
		t.Fatal(err)
	}
	controller := newWebContextSessionController(store, manager, func(session memory.Session, composition plugins.ResolvedComposition) (*agent.Session, error) {
		holder := memory.LeaseHolderID("browser-" + session.ID)
		return agent.NewWithToolset(workspaceSetupNoModelClient{}, profile, store.BindHistory(session.ID, holder), session.ScopeContext(), store.BindTurnOwner(session.ID, holder), composition.Toolset), nil
	})
	service := &spendingAccountsBrowserService{t: t, output: output, mode: "linked", updated: time.Now().UTC(), counts: map[string]int{}}
	service.count("fixtureStarted")
	server := web.WithSpending(web.NewContextServer(nil, manager, store, controller), service)
	mux := http.NewServeMux()
	mux.HandleFunc("/__fixture/spending", service.configure)
	mux.Handle("/", server.Handler())
	host := httptest.NewServer(mux)
	defer func() { server.Close(); host.Close() }()
	metadata := map[string]any{"url": host.URL, "pid": os.Getpid(), "database": database, "counts": filepath.Join(output, "counts.json"), "configureUrl": host.URL + "/__fixture/spending"}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "ready.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	fmt.Println("SPENDING_BROWSER_READY=" + string(raw))
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case <-time.After(time.Hour):
	}
}
