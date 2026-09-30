package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeAccountProvider struct {
	start                                 func(context.Context) (hostedAccountLink, error)
	public                                func(context.Context, string) (string, error)
	exchange                              func(context.Context, string) (linkedAccountItem, error)
	accounts                              func(context.Context, string) (bankAccountInventory, error)
	starts, polls, exchanges, inventories int
}

func (p *fakeAccountProvider) StartLink(ctx context.Context) (hostedAccountLink, error) {
	p.starts++
	return p.start(ctx)
}
func (p *fakeAccountProvider) PublicToken(ctx context.Context, token string) (string, error) {
	p.polls++
	return p.public(ctx, token)
}
func (p *fakeAccountProvider) Exchange(ctx context.Context, token string) (linkedAccountItem, error) {
	p.exchanges++
	return p.exchange(ctx, token)
}
func (p *fakeAccountProvider) Accounts(ctx context.Context, token string) (bankAccountInventory, error) {
	p.inventories++
	return p.accounts(ctx, token)
}

func accountTestService(t *testing.T) (*SpendingService, *sql.DB, *fakeAccountProvider) {
	t.Helper()
	s, db := spendingTestService(t)
	now := time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	p := &fakeAccountProvider{
		start: func(context.Context) (hostedAccountLink, error) {
			return hostedAccountLink{Token: "secret-link-token", URL: "https://secure.plaid.com/link/fixture", ExpiresAt: now.Add(30 * time.Minute)}, nil
		},
		public: func(context.Context, string) (string, error) { return "secret-public-token", nil },
		exchange: func(context.Context, string) (linkedAccountItem, error) {
			return linkedAccountItem{ID: "private-item", AccessToken: "secret-access-token"}, nil
		},
		accounts: func(context.Context, string) (bankAccountInventory, error) {
			return bankAccountInventory{Institution: "Test bank", Accounts: []bankAccountRecord{{ID: "private-account", Name: "Checking", Mask: "1234", Type: "depository", Subtype: "checking"}}}, nil
		},
	}
	s.accountProviderFactory = func() (spendingAccountProvider, error) { return p, nil }
	return s, db, p
}

func startTestAccountLink(t *testing.T, s *SpendingService) SpendingAccountLink {
	t.Helper()
	link, err := s.StartAccountLink(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return link
}

func requireLinkState(t *testing.T, db *sql.DB, id, state string) {
	t.Helper()
	link, err := loadAccountLink(context.Background(), db, id)
	if err != nil || link.State != state {
		t.Fatalf("link state=%q, err=%v; want %s", link.State, err, state)
	}
	if state != "pending" && state != "exchanging" && (link.Token != "" || link.HostedURL != "") {
		t.Fatal("terminal link retained provider credential/URL")
	}
}

func TestSpendingAccountsInspectIsLocalAndShowsLegacyItems(t *testing.T) {
	s, db, _ := accountTestService(t)
	insertItem(t, db, "private-item-one")
	insertItem(t, db, "private-item-two")
	if _, err := db.Exec(`UPDATE items SET institution='Legacy bank' WHERE item_id='private-item-two'; DROP TABLE finance_accounts; DROP TABLE finance_account_snapshots; DROP TABLE finance_link_sessions`); err != nil {
		t.Fatal(err)
	}
	s.openWrite = func(context.Context) (*sql.DB, error) { t.Fatal("inspection attempted migration"); return nil, nil }
	s.accountProviderFactory = func() (spendingAccountProvider, error) { t.Fatal("inspection constructed provider"); return nil, nil }
	report, err := s.InspectAccounts(context.Background())
	if err != nil || len(report.Institutions) != 2 {
		t.Fatalf("report=%+v, err=%v", report, err)
	}
	for _, bank := range report.Institutions {
		if bank.AccountsKnown || bank.AccountsUpdatedAt != nil || bank.Accounts == nil || len(bank.Accounts) != 0 {
			t.Fatalf("legacy bank=%+v", bank)
		}
	}
	b, _ := json.Marshal(report)
	if strings.Contains(string(b), "private-item") || strings.Contains(string(b), "test-token") {
		t.Fatalf("private data in report: %s", b)
	}
	s.openRead = func(context.Context) (*sql.DB, error) { return nil, nil }
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "")
	s.accountProviderFactory = nil
	report, err = s.InspectAccounts(context.Background())
	if err != nil || report.Institutions == nil || len(report.Institutions) != 0 || report.LinkAvailable {
		t.Fatalf("missing DB=%+v, err=%v", report, err)
	}
}

func TestSpendingAccountsRefreshPreservesSnapshotsOnPartialFailure(t *testing.T) {
	s, db, p := accountTestService(t)
	insertItem(t, db, "private-item-one")
	insertItem(t, db, "private-item-two")
	if _, err := db.Exec(`UPDATE items SET access_token=item_id,cursor='keep-cursor'`); err != nil {
		t.Fatal(err)
	}
	p.accounts = func(context.Context, string) (bankAccountInventory, error) {
		return bankAccountInventory{Institution: "Same bank", Accounts: []bankAccountRecord{{ID: "a", Name: "Checking", Mask: "1234", Type: "depository"}, {ID: "b", Name: "Checking", Mask: "1234", Type: "depository"}}}, nil
	}
	got, err := s.RefreshAccounts(context.Background())
	if err != nil || got.InstitutionsSucceeded != 2 || got.InstitutionsFailed != 0 {
		t.Fatalf("refresh=%+v %v", got, err)
	}
	before, err := s.InspectAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Institutions) != 2 || before.Institutions[0].ID == before.Institutions[1].ID {
		t.Fatalf("same-bank items merged: %+v", before)
	}
	for _, bank := range before.Institutions {
		if !bank.AccountsKnown || bank.AccountsUpdatedAt == nil || len(bank.Accounts) != 2 || bank.Accounts[0].ID == bank.Accounts[1].ID {
			t.Fatalf("accounts collapsed: %+v", bank)
		}
	}
	p.accounts = func(_ context.Context, token string) (bankAccountInventory, error) {
		if token == "private-item-one" {
			return bankAccountInventory{}, errors.New("secret provider failure")
		}
		return bankAccountInventory{Accounts: []bankAccountRecord{}}, nil
	}
	got, err = s.RefreshAccounts(context.Background())
	if err != nil || got.InstitutionsSucceeded != 1 || got.InstitutionsFailed != 1 {
		t.Fatalf("partial=%+v, %v", got, err)
	}
	after, err := s.InspectAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Institutions[0], after.Institutions[0]) || !after.Institutions[1].AccountsKnown || len(after.Institutions[1].Accounts) != 0 {
		t.Fatalf("partial changed failed snapshot: %+v", after)
	}
	p.accounts = func(context.Context, string) (bankAccountInventory, error) {
		return bankAccountInventory{Accounts: []bankAccountRecord{{ID: "duplicate", Name: "new"}, {ID: "duplicate", Name: "new"}}}, nil
	}
	got, err = s.RefreshAccounts(context.Background())
	if err != nil || got.InstitutionsFailed != 2 {
		t.Fatalf("duplicate refresh=%+v %v", got, err)
	}
	rolledBack, _ := s.InspectAccounts(context.Background())
	if !reflect.DeepEqual(after, rolledBack) {
		t.Fatalf("invalid snapshot was partially committed: %+v", rolledBack)
	}
	if cursor := getCursor(t, db, "private-item-one"); cursor != "keep-cursor" {
		t.Fatalf("refresh changed cursor %q", cursor)
	}
}

func TestSpendingAccountLinkPersistsAndCompletesOnce(t *testing.T) {
	s, db, p := accountTestService(t)
	insertItem(t, db, "private-item")
	if _, err := db.Exec(`UPDATE items SET cursor='keep-cursor'`); err != nil {
		t.Fatal(err)
	}
	link := startTestAccountLink(t, s)
	// A second service instance represents reloading/restarting the owner UI.
	other := NewSpendingService()
	other.openRead, other.openWrite, other.now, other.accountProviderFactory = s.openRead, s.openWrite, s.now, s.accountProviderFactory
	reused := startTestAccountLink(t, other)
	if reused != link || p.starts != 1 {
		t.Fatalf("pending was recreated: %+v, starts=%d", reused, p.starts)
	}
	report, err := other.InspectAccounts(context.Background())
	if err != nil || report.PendingLink == nil || *report.PendingLink != link {
		t.Fatalf("pending report=%+v %v", report, err)
	}
	b, _ := json.Marshal(report)
	if strings.Contains(string(b), "secret-") || strings.Contains(string(b), "private-item") {
		t.Fatalf("secret exposed: %s", b)
	}
	result, err := other.CompleteAccountLink(context.Background(), link.ID)
	if err != nil || result.Status != "linked" || result.InstitutionsLinked != 1 {
		t.Fatalf("complete=%+v %v", result, err)
	}
	requireLinkState(t, db, link.ID, "linked")
	if cursor := getCursor(t, db, "private-item"); cursor != "keep-cursor" {
		t.Fatalf("cursor=%q", cursor)
	}
	result, err = other.CompleteAccountLink(context.Background(), link.ID)
	if err != nil || result.Status != "linked" || p.exchanges != 1 || p.polls != 1 {
		t.Fatalf("replayed link=%+v %v exchanges=%d polls=%d", result, err, p.exchanges, p.polls)
	}
	report, err = s.InspectAccounts(context.Background())
	if err != nil || report.PendingLink != nil || len(report.Institutions) != 1 || !report.Institutions[0].AccountsKnown {
		t.Fatalf("saved inventory=%+v %v", report, err)
	}
}

func TestSpendingAccountLinkRecoverExpiredResult(t *testing.T) {
	for _, test := range []struct {
		name, token, want string
		elapsed           time.Duration
	}{{"completed-near-expiry", "public", "linked", 31 * time.Minute}, {"no-result", "", "expired", 31 * time.Minute}, {"recovery-window-over", "public", "expired", 7 * time.Hour}} {
		t.Run(test.name, func(t *testing.T) {
			s, db, p := accountTestService(t)
			link := startTestAccountLink(t, s)
			now := s.now().Add(test.elapsed)
			s.now = func() time.Time { return now }
			p.public = func(context.Context, string) (string, error) { return test.token, nil }
			report, err := s.InspectAccounts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if test.elapsed < 7*time.Hour {
				if report.PendingLink == nil {
					t.Fatal("discarded recoverable pending link")
				}
				if _, err := s.StartAccountLink(context.Background()); !errors.Is(err, ErrSpendingLinkInProgress) {
					t.Fatalf("expired URL reused/replaced: %v", err)
				}
			} else if report.PendingLink != nil {
				t.Fatal("retained link beyond recovery bound")
			}
			result, err := s.CompleteAccountLink(context.Background(), link.ID)
			if err != nil || result.Status != test.want {
				t.Fatalf("result=%+v %v", result, err)
			}
			requireLinkState(t, db, link.ID, test.want)
			if test.elapsed >= 7*time.Hour && p.polls != 0 {
				t.Fatal("polled beyond recovery bound")
			}
		})
	}
}

func TestSpendingAccountLinkSavesAfterBrowserCancellationAndMetadataFailure(t *testing.T) {
	for _, cancelBrowser := range []bool{false, true} {
		t.Run(map[bool]string{false: "metadata-failure", true: "browser-cancelled"}[cancelBrowser], func(t *testing.T) {
			s, db, p := accountTestService(t)
			link := startTestAccountLink(t, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.exchange = func(context.Context, string) (linkedAccountItem, error) {
				if cancelBrowser {
					cancel()
				}
				return linkedAccountItem{ID: "saved-item", AccessToken: "saved-token"}, nil
			}
			p.accounts = func(context.Context, string) (bankAccountInventory, error) {
				return bankAccountInventory{}, errors.New("private provider body")
			}
			result, err := s.CompleteAccountLink(ctx, link.ID)
			if err != nil || result.Status != "linked" {
				t.Fatalf("complete=%+v %v", result, err)
			}
			requireLinkState(t, db, link.ID, "linked")
			var token string
			if err := db.QueryRow(`SELECT access_token FROM items WHERE item_id='saved-item'`).Scan(&token); err != nil || token != "saved-token" {
				t.Fatalf("credentials not durable: %q %v", token, err)
			}
			if cancelBrowser && p.inventories != 0 {
				t.Fatal("metadata requested after browser cancellation")
			}
		})
	}
}

func TestSpendingAccountLinkRetriesLocalBusyWithoutReexchanging(t *testing.T) {
	s, db, p := accountTestService(t)
	link := startTestAccountLink(t, s)
	done := make(chan error, 1)
	p.exchange = func(context.Context, string) (linkedAccountItem, error) {
		conn, err := db.Conn(context.Background())
		if err != nil {
			return linkedAccountItem{}, err
		}
		if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
			conn.Close()
			return linkedAccountItem{}, err
		}
		go func() {
			time.Sleep(80 * time.Millisecond)
			_, err := conn.ExecContext(context.Background(), `ROLLBACK`)
			conn.Close()
			done <- err
		}()
		return linkedAccountItem{ID: "saved-after-busy", AccessToken: "token"}, nil
	}
	result, err := s.CompleteAccountLink(context.Background(), link.ID)
	if unlockErr := <-done; unlockErr != nil {
		t.Fatal(unlockErr)
	}
	if err != nil || result.Status != "linked" || p.exchanges != 1 {
		t.Fatalf("busy recovery=%+v %v exchanges=%d", result, err, p.exchanges)
	}
	requireLinkState(t, db, link.ID, "linked")
}

func TestSpendingAccountLinkAmbiguousExchangeIsNeverReplayed(t *testing.T) {
	s, db, p := accountTestService(t)
	link := startTestAccountLink(t, s)
	p.exchange = func(context.Context, string) (linkedAccountItem, error) {
		return linkedAccountItem{}, errors.New("secret-token ambiguous network error")
	}
	for i := 0; i < 2; i++ {
		result, err := s.CompleteAccountLink(context.Background(), link.ID)
		if err != nil || result.Status != "failed" {
			t.Fatalf("result=%+v %v", result, err)
		}
	}
	if p.exchanges != 1 {
		t.Fatalf("exchange replayed %d times", p.exchanges)
	}
	requireLinkState(t, db, link.ID, "failed")
	if _, err := s.StartAccountLink(context.Background()); err != nil {
		t.Fatalf("failed flow blocks new link: %v", err)
	}
}

func TestSpendingAccountLinkCancelCASAndNoUnlink(t *testing.T) {
	s, db, _ := accountTestService(t)
	insertItem(t, db, "saved-item")
	link := startTestAccountLink(t, s)
	// Model an exchange claim landing after Cancel reads but before its CAS.
	if _, err := db.Exec(`CREATE TRIGGER concurrent_claim BEFORE UPDATE ON finance_link_sessions WHEN NEW.state='cancelled' BEGIN UPDATE finance_link_sessions SET state='exchanging',claim_expires_at='2026-09-21T16:02:00Z' WHERE id=OLD.id; SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelAccountLink(context.Background(), link.ID); !errors.Is(err, ErrSpendingLinkInProgress) {
		t.Fatalf("false cancellation success: %v", err)
	}
	requireLinkState(t, db, link.ID, "exchanging")
	if _, err := db.Exec(`DROP TRIGGER concurrent_claim; UPDATE finance_link_sessions SET state='pending'`); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelAccountLink(context.Background(), link.ID); err != nil {
		t.Fatal(err)
	}
	requireLinkState(t, db, link.ID, "cancelled")
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("cancel removed bank: %d %v", count, err)
	}
}

func TestSpendingAccountLinkRejectsUnsafeURLsAndBoundsExpiry(t *testing.T) {
	for _, raw := range []string{"http://secure.plaid.com/link/a", "https://evil.test/link/a", "https://user:pass@secure.plaid.com/link/a", "https://secure.plaid.com:443/link/a", "https://secure.plaid.com/link/a#secret"} {
		t.Run(raw, func(t *testing.T) {
			s, db, p := accountTestService(t)
			p.start = func(context.Context) (hostedAccountLink, error) {
				return hostedAccountLink{Token: "secret", URL: raw, ExpiresAt: s.now().Add(time.Hour)}, nil
			}
			if _, err := s.StartAccountLink(context.Background()); !errors.Is(err, ErrSpendingLinkUnavailable) {
				t.Fatalf("unsafe URL accepted: %v", err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM finance_link_sessions`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("unsafe URL persisted: %d %v", count, err)
			}
		})
	}
	s, _, p := accountTestService(t)
	p.start = func(context.Context) (hostedAccountLink, error) {
		return hostedAccountLink{Token: "secret", URL: "https://secure.plaid.com/link/a", ExpiresAt: s.now().Add(4 * time.Hour)}, nil
	}
	if link := startTestAccountLink(t, s); !link.ExpiresAt.Equal(s.now().Add(30 * time.Minute)) {
		t.Fatalf("expiry unbounded: %+v", link)
	}
}

func TestSpendingAccountOperationsReleaseGatesOnCancellation(t *testing.T) {
	s, _, p := accountTestService(t)
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	p.start = func(ctx context.Context) (hostedAccountLink, error) {
		close(entered)
		<-ctx.Done()
		return hostedAccountLink{}, ctx.Err()
	}
	finished := make(chan error, 1)
	go func() { _, err := s.StartAccountLink(ctx); finished <- err }()
	<-entered
	if _, err := s.StartAccountLink(context.Background()); !errors.Is(err, ErrSpendingLinkInProgress) {
		t.Fatalf("concurrent start=%v", err)
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	p.start = func(context.Context) (hostedAccountLink, error) {
		return hostedAccountLink{Token: "secret", URL: "https://secure.plaid.com/link/a", ExpiresAt: s.now().Add(time.Minute)}, nil
	}
	startTestAccountLink(t, s)
}

func TestSpendingAccountRefreshCancelsBeforeNextBankAndRejectsStaleCredential(t *testing.T) {
	s, db, p := accountTestService(t)
	insertItem(t, db, "first")
	insertItem(t, db, "second")
	ctx, cancel := context.WithCancel(context.Background())
	p.accounts = func(context.Context, string) (bankAccountInventory, error) {
		if _, err := s.RefreshAccounts(context.Background()); !errors.Is(err, ErrSpendingAccountsRefreshInProgress) {
			t.Fatalf("overlapping inventory refresh: %v", err)
		}
		cancel()
		return bankAccountInventory{Accounts: []bankAccountRecord{{ID: "uncommitted"}}}, nil
	}
	if _, err := s.RefreshAccounts(ctx); !errors.Is(err, context.Canceled) || p.inventories != 1 {
		t.Fatalf("refresh cancellation=%v, provider calls=%d", err, p.inventories)
	}
	p.accounts = func(context.Context, string) (bankAccountInventory, error) {
		// A relink or other writer supersedes the credential while its inventory
		// response is in flight. The stale response cannot replace the cache.
		if _, err := db.Exec(`UPDATE items SET access_token='new-token' WHERE item_id='first'`); err != nil {
			t.Fatal(err)
		}
		return bankAccountInventory{Accounts: []bankAccountRecord{{ID: "current"}}}, nil
	}
	result, err := s.RefreshAccounts(context.Background())
	if err != nil || result.InstitutionsSucceeded != 1 || result.InstitutionsFailed != 1 {
		t.Fatalf("stale credential=%+v, %v", result, err)
	}
	var first, second int
	if err := db.QueryRow(`SELECT COUNT(*) FROM finance_accounts WHERE item_id='first'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM finance_accounts WHERE item_id='second'`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first != 0 || second != 1 {
		t.Fatalf("inventory counts first=%d second=%d", first, second)
	}
}

func TestSpendingAccountLinkFailedSaveAndCrashedClaimNeverReplay(t *testing.T) {
	for _, crashed := range []bool{false, true} {
		t.Run(map[bool]string{false: "local-save-failed", true: "crashed-claim"}[crashed], func(t *testing.T) {
			s, db, p := accountTestService(t)
			link := startTestAccountLink(t, s)
			if crashed {
				if _, err := db.Exec(`UPDATE finance_link_sessions SET state='exchanging',claim_expires_at='2026-09-21T15:59:59Z' WHERE id=?`, link.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`CREATE TRIGGER reject_link BEFORE INSERT ON items BEGIN SELECT RAISE(ABORT,'disk write failed'); END`); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				result, err := s.CompleteAccountLink(context.Background(), link.ID)
				if err != nil || result.Status != "failed" {
					t.Fatalf("failed save/claim=%+v, %v", result, err)
				}
			}
			wantExchanges := 1
			if crashed {
				wantExchanges = 0
			}
			if p.exchanges != wantExchanges {
				t.Fatalf("exchanges=%d, want %d", p.exchanges, wantExchanges)
			}
			requireLinkState(t, db, link.ID, "failed")
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial credential save: %d %v", count, err)
			}
		})
	}
}
