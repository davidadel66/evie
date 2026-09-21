package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/plaid/plaid-go/v43/plaid"
)

func spendingTestService(t *testing.T) (*SpendingService, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "finance.db")
	db, err := openDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewSpendingService()
	s.openRead = func(ctx context.Context) (*sql.DB, error) {
		readDB, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			return nil, err
		}
		if err := readDB.PingContext(ctx); err != nil {
			readDB.Close()
			return nil, err
		}
		return readDB, nil
	}
	s.openWrite = func(ctx context.Context) (*sql.DB, error) { return openDBAtContext(ctx, path) }
	return s, db
}

func insertSpendingTxn(t *testing.T, db *sql.DB, id string, date any, cents int64, pending int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO transactions(transaction_id,item_id,date,amount_cents,pending) VALUES(?, 'item-1', ?, ?, ?)`, id, date, cents, pending); err != nil {
		t.Fatal(err)
	}
}

func TestSpendingDailyFlowUsesPostedDatesAndAllRawTransactions(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "income", "2026-01-01", -20000, 0)
	insertSpendingTxn(t, db, "purchase", "2026-01-01", 1234, 0)
	insertSpendingTxn(t, db, "transfer", "2026-01-01", 5000, 0)
	insertSpendingTxn(t, db, "pending", "2026-01-01", 99999, 1)
	insertSpendingTxn(t, db, "offset-out", "2026-01-02", 100, 0)
	insertSpendingTxn(t, db, "offset-in", "2026-01-02", -100, 0)
	insertSpendingTxn(t, db, "leap", "2024-02-29", 42, 0)
	insertSpendingTxn(t, db, "future-pending", "2027-01-01", 1, 1)
	insertSpendingTxn(t, db, "undated", nil, 9, 0)
	insertSpendingTxn(t, db, "blank", "", 9, 0)
	insertSpendingTxn(t, db, "invalid", "2026-02-29", 9, 0)
	insertSpendingTxn(t, db, "year-zero", "0000-01-01", 9, 0)
	if _, err := db.Exec(`INSERT INTO categories(name) VALUES('excluded')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO budget_entries(transaction_id,category,amount_cents) VALUES('transfer','excluded',5000)`); err != nil {
		t.Fatal(err)
	}
	report, err := s.InspectSpending(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := []SpendingDay{
		{Date: "2026-01-01", InflowCents: "20000", OutflowCents: "6234", NetCents: "13766", Transactions: 3},
		{Date: "2026-01-02", InflowCents: "100", OutflowCents: "100", NetCents: "0", Transactions: 2},
	}
	if !reflect.DeepEqual(report.Days, want) {
		t.Fatalf("days = %+v, want %+v", report.Days, want)
	}
	if !reflect.DeepEqual(report.Years, []int{2027, 2026, 2024}) || report.LinkedBanks != 1 || report.PendingTransactions != 1 || report.UndatedTransactions != 4 || report.Currency != nil || report.RefreshedAt != nil {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	// Exact cancellation remains a recorded day; dates with no rows remain
	// absent so the UI can distinguish no data from zero net flow.
	empty, err := s.InspectSpending(context.Background(), 2025)
	if err != nil || len(empty.Days) != 0 || empty.PendingTransactions != 0 || !reflect.DeepEqual(empty.Years, []int{2027, 2026, 2025, 2024}) {
		t.Fatalf("empty selected year = %+v, %v", empty, err)
	}
}

func TestSpendingCentsPreserveIntegerExtremesAndLargeSums(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "largest-inflow", "2026-01-01", math.MinInt64, 0)
	insertSpendingTxn(t, db, "largest-outflow-1", "2026-01-01", math.MaxInt64, 0)
	insertSpendingTxn(t, db, "largest-outflow-2", "2026-01-01", math.MaxInt64, 0)
	report, err := s.InspectSpending(context.Background(), 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := SpendingDay{Date: "2026-01-01", InflowCents: "9223372036854775808", OutflowCents: "18446744073709551614", NetCents: "-9223372036854775806", Transactions: 3}
	if len(report.Days) != 1 || report.Days[0] != want {
		t.Fatalf("exact daily cents = %+v, want %+v", report.Days, want)
	}
}

func TestSpendingMissingDatabaseAndInvalidQueriesNeverWrite(t *testing.T) {
	s := NewSpendingService()
	reads := 0
	s.openRead = func(context.Context) (*sql.DB, error) { reads++; return nil, nil }
	s.openWrite = func(context.Context) (*sql.DB, error) { t.Fatal("attempted to create a database"); return nil, nil }
	report, err := s.InspectSpending(context.Background(), 2026)
	if err != nil || report.LinkedBanks != 0 || len(report.Days) != 0 {
		t.Fatalf("missing database = %+v, %v", report, err)
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"days":[]`) || !strings.Contains(string(encoded), `"currency":null`) || !strings.Contains(string(encoded), `"refreshedAt":null`) {
		t.Fatalf("empty JSON = %s, %v", encoded, err)
	}
	if _, err := s.RefreshSpending(context.Background()); !errors.Is(err, ErrSpendingRefreshUnavailable) {
		t.Fatalf("refresh missing database error = %v", err)
	}
	before := reads
	for _, year := range []int{0, -1, 10000} {
		if _, err := s.InspectSpending(context.Background(), year); !errors.Is(err, ErrSpendingYear) {
			t.Fatalf("year %d error = %v", year, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.InspectSpending(ctx, 2026); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inspect = %v", err)
	}
	if _, err := s.RefreshSpending(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh = %v", err)
	}
	if reads != before {
		t.Fatal("invalid or cancelled request reached database")
	}
}

func TestSpendingRefreshUpdatesLocalViewAndSanitizesPartialFailures(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	at := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return at }
	s.syncBanks = func(ctx context.Context, db *sql.DB) (*SyncResult, error) {
		_, err := db.ExecContext(ctx, `INSERT INTO transactions(transaction_id,item_id,date,amount_cents) VALUES('latest','item-1','2026-09-19',500)`)
		return &SyncResult{Banks: []BankSync{{Label: "private bank id", Counts: SyncCounts{Added: 1}}}, Totals: SyncCounts{Added: 1}}, err
	}
	refresh, err := s.RefreshSpending(context.Background())
	if err != nil || refresh.Added != 1 || refresh.BanksSucceeded != 1 || refresh.BanksFailed != 0 || refresh.RefreshedAt == nil || !refresh.RefreshedAt.Equal(at) {
		t.Fatalf("successful refresh = %+v, %v", refresh, err)
	}
	report, err := s.InspectSpending(context.Background(), 2026)
	if err != nil || len(report.Days) != 1 || report.Days[0].NetCents != "-500" || report.RefreshedAt == nil || !report.RefreshedAt.Equal(at) {
		t.Fatalf("post-refresh report = %+v, %v", report, err)
	}
	s.now = func() time.Time { return at.Add(time.Hour) }
	s.syncBanks = func(context.Context, *sql.DB) (*SyncResult, error) {
		return &SyncResult{
			Banks: []BankSync{
				{Label: "private bank id", Err: errors.New("private provider payload"), Warnings: []string{"private warning"}},
				{Counts: SyncCounts{Modified: 2}},
			}, Totals: SyncCounts{Modified: 2},
		}, nil
	}
	partial, err := s.RefreshSpending(context.Background())
	if err != nil || partial.BanksFailed != 1 || partial.BanksSucceeded != 1 || partial.Modified != 2 || partial.RefreshedAt == nil || !partial.RefreshedAt.Equal(at) {
		t.Fatalf("partial refresh = %+v, %v", partial, err)
	}
	encoded, err := json.Marshal(partial)
	if err != nil || strings.Contains(string(encoded), "private") {
		t.Fatalf("unsafe partial response = %s, %v", encoded, err)
	}
	s.syncBanks = func(context.Context, *sql.DB) (*SyncResult, error) { return nil, errors.New("private fatal payload") }
	if _, err := s.RefreshSpending(context.Background()); err != ErrSpendingRefreshUnavailable {
		t.Fatalf("refresh exposed raw error: %v", err)
	}
}

func TestSpendingRefreshRejectsOverlapAndCancelsProvider(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	started := make(chan struct{})
	s.syncBanks = func(ctx context.Context, _ *sql.DB) (*SyncResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.RefreshSpending(ctx); done <- err }()
	<-started
	if _, err := s.RefreshSpending(context.Background()); !errors.Is(err, ErrSpendingRefreshInProgress) {
		t.Fatalf("overlapping refresh = %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled provider = %v", err)
	}
	s.syncBanks = func(context.Context, *sql.DB) (*SyncResult, error) { return &SyncResult{Banks: []BankSync{{}}}, nil }
	if _, err := s.RefreshSpending(context.Background()); err != nil {
		t.Fatalf("cancelled refresh did not release gate: %v", err)
	}
	s.timeout = time.Millisecond
	s.syncBanks = func(ctx context.Context, _ *sql.DB) (*SyncResult, error) { <-ctx.Done(); return nil, ctx.Err() }
	if _, err := s.RefreshSpending(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unresponsive provider was not bounded: %v", err)
	}
}

func TestSyncSerializesCallersBeforeLoadingCursors(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	if _, err := db.Exec(`UPDATE items SET institution='Bank'`); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLAID_CLIENT_ID", "test")
	t.Setenv("PLAID_SECRET", "test")
	original := runSyncItem
	t.Cleanup(func() { runSyncItem = original })
	started := make(chan struct{})
	release := make(chan struct{})
	calls := 0
	var secondCursor string
	runSyncItem = func(ctx context.Context, _ *plaid.APIClient, db *sql.DB, item syncItem) (int, int, int, error) {
		calls++
		if calls == 1 {
			close(started)
			<-release
			_, err := db.ExecContext(ctx, `UPDATE items SET cursor='new-cursor'`)
			return 0, 0, 0, err
		}
		secondCursor = item.cursor
		return 0, 0, 0, nil
	}
	first := make(chan error, 1)
	go func() { _, err := Sync(context.Background(), db); first <- err }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() { _, err := Sync(ctx, db); second <- err }()
	cancel()
	if err := <-second; !errors.Is(err, context.Canceled) {
		close(release)
		<-first
		t.Fatalf("waiting sync error = %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cancelled waiter entered provider: calls=%d", calls)
	}
	if _, err := Sync(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if secondCursor != "new-cursor" {
		t.Fatalf("next caller loaded cursor %q before preceding sync committed", secondCursor)
	}
}
