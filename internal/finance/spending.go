package finance

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var (
	ErrSpendingYear               = errors.New("spending year must be between 1 and 9999")
	ErrSpendingUnavailable        = errors.New("spending data is unavailable")
	ErrSpendingRefreshUnavailable = errors.New("transactions could not be refreshed")
	ErrSpendingRefreshInProgress  = errors.New("a transaction refresh is already in progress")
)

// SpendingDay reports raw posted transaction flow. Decimal strings preserve
// integer cents across JSON and JavaScript, including sums above int64 limits.
type SpendingDay struct {
	Date         string `json:"date"`
	InflowCents  string `json:"inflowCents"`
	OutflowCents string `json:"outflowCents"`
	NetCents     string `json:"netCents"`
	Transactions int    `json:"transactions"`
}

type SpendingReport struct {
	Year                int           `json:"year"`
	Years               []int         `json:"years"`
	LinkedBanks         int           `json:"linkedBanks"`
	Days                []SpendingDay `json:"days"`
	PendingTransactions int           `json:"pendingTransactions"`
	UndatedTransactions int           `json:"undatedTransactions"`
	Currency            *string       `json:"currency"`
	RefreshedAt         *time.Time    `json:"refreshedAt"`
}

// SpendingRefresh deliberately omits bank identifiers, provider responses,
// credentials, and raw errors. Counts describe banks that completed syncing;
// a failed bank may still have committed earlier pages.
type SpendingRefresh struct {
	Added          int        `json:"added"`
	Modified       int        `json:"modified"`
	Removed        int        `json:"removed"`
	BanksSucceeded int        `json:"banksSucceeded"`
	BanksFailed    int        `json:"banksFailed"`
	RefreshedAt    *time.Time `json:"refreshedAt"`
}

// SpendingService reads the existing finance database and exposes an explicit
// refresh action. Its success timestamp lasts only for this server lifetime;
// inspecting a report never claims to have contacted a bank.
type SpendingService struct {
	mu          sync.Mutex
	refreshing  bool
	refreshedAt *time.Time
	openRead    func(context.Context) (*sql.DB, error)
	openWrite   func(context.Context) (*sql.DB, error)
	syncBanks   func(context.Context, *sql.DB) (*SyncResult, error)
	now         func() time.Time
	timeout     time.Duration
}

func NewSpendingService() *SpendingService {
	return &SpendingService{
		openRead: openSpendingReadOnly, openWrite: OpenDBContext, syncBanks: Sync,
		now: time.Now, timeout: 2 * time.Minute,
	}
}

func openSpendingReadOnly(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(home, ".finance", "finance.db")); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return OpenDBReadOnlyContext(ctx)
}

func (s *SpendingService) InspectSpending(ctx context.Context, year int) (SpendingReport, error) {
	if year < 1 || year > 9999 {
		return SpendingReport{}, ErrSpendingYear
	}
	report := SpendingReport{Year: year, Years: []int{year}, Days: []SpendingDay{}, RefreshedAt: s.lastRefresh()}
	if err := ctx.Err(); err != nil {
		return SpendingReport{}, err
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	if db == nil {
		return report, nil
	}
	defer db.Close()
	// One read transaction keeps account counts, available years, and daily
	// totals on the same snapshot while a refresh is committing pages.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&report.LinkedBanks); err != nil {
		return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(date, ''), amount_cents, pending FROM transactions`)
	if err != nil {
		return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer rows.Close()
	type dayTotals struct {
		inflow, outflow big.Int
		transactions    int
	}
	days := map[string]*dayTotals{}
	years := map[int]bool{year: true}
	for rows.Next() {
		var date string
		var cents int64
		var pending int
		if err := rows.Scan(&date, &cents, &pending); err != nil {
			return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
		day, err := time.Parse("2006-01-02", date)
		if err != nil || day.Year() < 1 || day.Format("2006-01-02") != date {
			report.UndatedTransactions++
			continue
		}
		years[day.Year()] = true
		if day.Year() != year {
			continue
		}
		if pending != 0 {
			report.PendingTransactions++
			continue
		}
		total := days[date]
		if total == nil {
			total = &dayTotals{}
			days[date] = total
		}
		amount := big.NewInt(cents)
		if cents < 0 {
			total.inflow.Sub(&total.inflow, amount)
		} else {
			total.outflow.Add(&total.outflow, amount)
		}
		total.transactions++
	}
	if err := rows.Err(); err != nil {
		return SpendingReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	for date, total := range days {
		var net big.Int
		net.Sub(&total.inflow, &total.outflow)
		report.Days = append(report.Days, SpendingDay{
			Date: date, InflowCents: total.inflow.String(), OutflowCents: total.outflow.String(),
			NetCents: net.String(), Transactions: total.transactions,
		})
	}
	sort.Slice(report.Days, func(i, j int) bool { return report.Days[i].Date < report.Days[j].Date })
	report.Years = make([]int, 0, len(years))
	for available := range years {
		report.Years = append(report.Years, available)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(report.Years)))
	if err := ctx.Err(); err != nil {
		return SpendingReport{}, err
	}
	return report, nil
}

func (s *SpendingService) RefreshSpending(ctx context.Context) (SpendingRefresh, error) {
	if err := ctx.Err(); err != nil {
		return SpendingRefresh{}, err
	}
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return SpendingRefresh{}, ErrSpendingRefreshInProgress
	}
	s.refreshing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.refreshing = false
		s.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	// A disconnected installation stays empty, even if Refresh is requested.
	readDB, err := s.openRead(ctx)
	if err != nil || readDB == nil {
		return SpendingRefresh{}, spendingError(ctx, ErrSpendingRefreshUnavailable)
	}
	var linked int
	err = readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&linked)
	readDB.Close()
	if err != nil || linked == 0 {
		return SpendingRefresh{}, spendingError(ctx, ErrSpendingRefreshUnavailable)
	}
	db, err := s.openWrite(ctx)
	if err != nil {
		return SpendingRefresh{}, spendingError(ctx, ErrSpendingRefreshUnavailable)
	}
	defer db.Close()
	result, err := s.syncBanks(ctx, db)
	if err != nil || result == nil {
		return SpendingRefresh{}, spendingError(ctx, ErrSpendingRefreshUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return SpendingRefresh{}, err
	}
	refresh := SpendingRefresh{
		Added: result.Totals.Added, Modified: result.Totals.Modified, Removed: result.Totals.Removed,
	}
	for _, bank := range result.Banks {
		if bank.Err != nil {
			refresh.BanksFailed++
		} else {
			refresh.BanksSucceeded++
		}
	}
	if refresh.BanksFailed == 0 && refresh.BanksSucceeded > 0 {
		now := s.now().UTC()
		s.mu.Lock()
		s.refreshedAt = &now
		s.mu.Unlock()
	}
	refresh.RefreshedAt = s.lastRefresh()
	return refresh, nil
}

func (s *SpendingService) lastRefresh() *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshedAt == nil {
		return nil
	}
	at := *s.refreshedAt
	return &at
}

func spendingError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fallback
}
