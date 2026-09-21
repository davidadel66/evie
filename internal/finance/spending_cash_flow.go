package finance

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"sort"
	"strings"
	"time"
)

var ErrSpendingCashFlowQuery = errors.New("choose valid cash-flow months and an as-of date")

type SpendingCashFlowQuery struct {
	Month      string `json:"month"`
	HistoryEnd string `json:"historyEnd"`
	AsOfDate   string `json:"asOfDate"`
}

type SpendingCashFlowTotals struct {
	InflowCents  string `json:"inflowCents"`
	OutflowCents string `json:"outflowCents"`
	NetCents     string `json:"netCents"`
	Transactions int    `json:"transactions"`
}

type SpendingCashFlowMonth struct {
	Month string `json:"month"`
	SpendingCashFlowTotals
}

type SpendingCashFlowCategory struct {
	Kind     string  `json:"kind"`
	Category *string `json:"category"`
	SpendingCashFlowTotals
}

type SpendingCashFlowReport struct {
	Month               string                     `json:"month"`
	HistoryEnd          string                     `json:"historyEnd"`
	AsOfDate            string                     `json:"asOfDate"`
	History             []SpendingCashFlowMonth    `json:"history"`
	Selected            SpendingCashFlowTotals     `json:"selected"`
	Categories          []SpendingCashFlowCategory `json:"categories"`
	PendingTransactions int                        `json:"pendingTransactions"`
	UndatedTransactions int                        `json:"undatedTransactions"`
}

func ValidateSpendingCashFlowQuery(query SpendingCashFlowQuery) (SpendingCashFlowQuery, error) {
	month, err := parseSpendingMonth(query.Month)
	if err != nil {
		return SpendingCashFlowQuery{}, ErrSpendingCashFlowQuery
	}
	end, err := parseSpendingMonth(query.HistoryEnd)
	if err != nil {
		return SpendingCashFlowQuery{}, ErrSpendingCashFlowQuery
	}
	asOf, err := time.Parse("2006-01-02", query.AsOfDate)
	if err != nil || asOf.Year() < 1 || asOf.Format("2006-01-02") != query.AsOfDate {
		return SpendingCashFlowQuery{}, ErrSpendingCashFlowQuery
	}
	if month.Before(spendingHistoryStart(end)) || month.After(end) {
		return SpendingCashFlowQuery{}, ErrSpendingCashFlowQuery
	}
	return query, nil
}

func parseSpendingMonth(value string) (time.Time, error) {
	month, err := time.Parse("2006-01", value)
	if err != nil || month.Year() < 1 || month.Format("2006-01") != value {
		return time.Time{}, ErrSpendingCashFlowQuery
	}
	return month, nil
}

func spendingHistoryStart(end time.Time) time.Time {
	start := end.AddDate(0, -11, 0)
	if start.Year() < 1 {
		return time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	return start
}

type cashFlowTotal struct {
	inflow, outflow big.Int
	transactions    int
}

func (total *cashFlowTotal) add(amount *big.Int) {
	if amount.Sign() < 0 {
		total.inflow.Sub(&total.inflow, amount)
	} else {
		total.outflow.Add(&total.outflow, amount)
	}
	total.transactions++
}

func (total *cashFlowTotal) report() SpendingCashFlowTotals {
	var net big.Int
	net.Sub(&total.inflow, &total.outflow)
	return SpendingCashFlowTotals{
		InflowCents: total.inflow.String(), OutflowCents: total.outflow.String(),
		NetCents: net.String(), Transactions: total.transactions,
	}
}

type cashFlowAllocation struct {
	amount     *big.Int
	sum        big.Int
	entries    bool
	coherent   bool
	categories map[string]*big.Int
}

type cashFlowCategoryKey struct {
	kind, category string
}

// InspectSpendingCashFlow keeps raw flow independent from saved allocations.
// Categories reconcile each raw side only when every allocation agrees with
// its transaction's sign and the exact allocated total matches. Otherwise the
// whole raw transaction appears once in the unreconciled bucket; saved entries
// remain untouched and inspectable in the transaction view.
func (s *SpendingService) InspectSpendingCashFlow(ctx context.Context, query SpendingCashFlowQuery) (SpendingCashFlowReport, error) {
	query, err := ValidateSpendingCashFlowQuery(query)
	if err != nil {
		return SpendingCashFlowReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return SpendingCashFlowReport{}, err
	}
	end, _ := parseSpendingMonth(query.HistoryEnd)
	start := spendingHistoryStart(end)
	months := map[string]*cashFlowTotal{}
	report := SpendingCashFlowReport{
		Month: query.Month, HistoryEnd: query.HistoryEnd, AsOfDate: query.AsOfDate,
		History: []SpendingCashFlowMonth{}, Categories: []SpendingCashFlowCategory{},
		Selected: (&cashFlowTotal{}).report(),
	}
	for month := start; !month.After(end); month = month.AddDate(0, 1, 0) {
		key := month.Format("2006-01")
		months[key] = &cashFlowTotal{}
		report.History = append(report.History, SpendingCashFlowMonth{Month: key, SpendingCashFlowTotals: months[key].report()})
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	if db == nil {
		return report, nil
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT transaction_id, COALESCE(date, ''), amount_cents, pending FROM transactions`)
	if err != nil {
		return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer rows.Close()
	allocations := map[string]*cashFlowAllocation{}
	for rows.Next() {
		var id, date string
		var amount int64
		var pending int
		if err := rows.Scan(&id, &date, &amount, &pending); err != nil {
			return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
		day, err := time.Parse("2006-01-02", date)
		if err != nil || day.Year() < 1 || day.Format("2006-01-02") != date {
			report.UndatedTransactions++
			continue
		}
		if date > query.AsOfDate {
			continue
		}
		month := day.Format("2006-01")
		total, inWindow := months[month]
		if !inWindow {
			continue
		}
		if pending != 0 {
			if month == query.Month {
				report.PendingTransactions++
			}
			continue
		}
		cents := big.NewInt(amount)
		total.add(cents)
		if month == query.Month {
			allocations[id] = &cashFlowAllocation{amount: cents, coherent: true, categories: map[string]*big.Int{}}
		}
	}
	if err := rows.Err(); err != nil {
		return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	rows.Close()
	for i := range report.History {
		report.History[i].SpendingCashFlowTotals = months[report.History[i].Month].report()
	}
	report.Selected = months[query.Month].report()
	if len(allocations) > 0 {
		if err := readCashFlowAllocations(ctx, tx, query, allocations); err != nil {
			return SpendingCashFlowReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
	}
	report.Categories = cashFlowCategories(allocations)
	if err := ctx.Err(); err != nil {
		return SpendingCashFlowReport{}, err
	}
	return report, nil
}

func readCashFlowAllocations(ctx context.Context, tx *sql.Tx, query SpendingCashFlowQuery, allocations map[string]*cashFlowAllocation) error {
	month, _ := parseSpendingMonth(query.Month)
	first := month.Format("2006-01-02")
	last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	rows, err := tx.QueryContext(ctx, `
		SELECT e.transaction_id, e.category, e.amount_cents
		FROM budget_entries e JOIN transactions t ON t.transaction_id = e.transaction_id
		WHERE t.pending = 0 AND t.date >= ? AND t.date <= ? AND t.date <= ?`, first, last, query.AsOfDate)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, category string
		var cents int64
		if err := rows.Scan(&id, &category, &cents); err != nil {
			return err
		}
		allocation, exists := allocations[id]
		if !exists {
			continue
		}
		amount := big.NewInt(cents)
		allocation.entries = true
		allocation.sum.Add(&allocation.sum, amount)
		if (amount.Sign() != 0 && amount.Sign() != allocation.amount.Sign()) || strings.TrimSpace(category) == "" {
			allocation.coherent = false
		}
		if allocation.categories[category] == nil {
			allocation.categories[category] = new(big.Int)
		}
		allocation.categories[category].Add(allocation.categories[category], amount)
	}
	return rows.Err()
}

func cashFlowCategories(allocations map[string]*cashFlowAllocation) []SpendingCashFlowCategory {
	totals := map[cashFlowCategoryKey]*cashFlowTotal{}
	add := func(key cashFlowCategoryKey, amount *big.Int) {
		if totals[key] == nil {
			totals[key] = &cashFlowTotal{}
		}
		totals[key].add(amount)
	}
	for _, allocation := range allocations {
		switch {
		case !allocation.entries:
			add(cashFlowCategoryKey{kind: "unclassified"}, allocation.amount)
		case !allocation.coherent || allocation.sum.Cmp(allocation.amount) != 0:
			add(cashFlowCategoryKey{kind: "unreconciled"}, allocation.amount)
		default:
			for category, amount := range allocation.categories {
				add(cashFlowCategoryKey{kind: "category", category: category}, amount)
			}
		}
	}
	keys := make([]cashFlowCategoryKey, 0, len(totals))
	for key := range totals {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := totals[keys[i]], totals[keys[j]]
		var leftActivity, rightActivity big.Int
		leftActivity.Add(&left.inflow, &left.outflow)
		rightActivity.Add(&right.inflow, &right.outflow)
		if compare := leftActivity.Cmp(&rightActivity); compare != 0 {
			return compare > 0
		}
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		return keys[i].category < keys[j].category
	})
	categories := make([]SpendingCashFlowCategory, 0, len(keys))
	for _, key := range keys {
		category := SpendingCashFlowCategory{Kind: key.kind, SpendingCashFlowTotals: totals[key].report()}
		if key.kind == "category" {
			name := key.category
			category.Category = &name
		}
		categories = append(categories, category)
	}
	return categories
}
