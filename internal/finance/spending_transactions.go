package finance

import (
	"context"
	"database/sql"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
)

var ErrSpendingTransactionsQuery = errors.New("choose a valid month, classification status, and transaction page")

type SpendingTransactionsQuery struct {
	Month    string `json:"month"`
	Status   string `json:"status"`
	Offset   int    `json:"offset"`
	PageSize int    `json:"pageSize"`
}

type SpendingClassificationCounts struct {
	All          int `json:"all"`
	Unclassified int `json:"unclassified"`
	Classified   int `json:"classified"`
	Pending      int `json:"pending"`
}

type SpendingEntry struct {
	Category    string `json:"category"`
	AmountCents string `json:"amountCents"`
	Source      string `json:"source"`
}

type SpendingTransaction struct {
	ID             string          `json:"id"`
	Date           string          `json:"date"`
	Name           string          `json:"name"`
	MerchantName   string          `json:"merchantName"`
	NetCents       string          `json:"netCents"`
	Classification string          `json:"classification"`
	Entries        []SpendingEntry `json:"entries"`
	LegacyCategory *string         `json:"legacyCategory"`
	LegacySource   *string         `json:"legacySource"`
}

type SpendingTransactionsReport struct {
	Month        string                       `json:"month"`
	Status       string                       `json:"status"`
	Offset       int                          `json:"offset"`
	PageSize     int                          `json:"pageSize"`
	Total        int                          `json:"total"`
	Counts       SpendingClassificationCounts `json:"counts"`
	Transactions []SpendingTransaction        `json:"transactions"`
	HasMore      bool                         `json:"hasMore"`
}

func ValidateSpendingTransactionsQuery(query SpendingTransactionsQuery) (SpendingTransactionsQuery, error) {
	month, err := time.Parse("2006-01", query.Month)
	if err != nil || month.Year() < 1 || month.Format("2006-01") != query.Month {
		return SpendingTransactionsQuery{}, ErrSpendingTransactionsQuery
	}
	if query.Status == "" {
		query.Status = "all"
	}
	switch query.Status {
	case "all", "unclassified", "classified", "pending":
	default:
		return SpendingTransactionsQuery{}, ErrSpendingTransactionsQuery
	}
	if query.Offset < 0 || query.PageSize < 0 || query.PageSize > 100 {
		return SpendingTransactionsQuery{}, ErrSpendingTransactionsQuery
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	return query, nil
}

// Pending is distinct from classification. For posted transactions, the
// existence of any budget entry is the exact fence used by Categorize;
// legacy labels and the legacy reviewed flag do not replace that fence.
const spendingMonthTransactions = `WITH month_transactions AS (
	SELECT t.transaction_id, t.date, COALESCE(t.name, '') AS name,
	       COALESCE(t.merchant_name, '') AS merchant_name, t.amount_cents,
	       t.category, t.category_source,
	       CASE WHEN t.pending != 0 THEN 'pending'
	            WHEN EXISTS (SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id) THEN 'classified'
	            ELSE 'unclassified' END AS classification
	FROM transactions t
	WHERE t.date >= ? AND t.date <= ?
	  AND t.date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
) `

// InspectSpendingTransactions returns one bounded page and its classification
// counts from the same read snapshot. It does not run rules, accept proposals,
// or change saved classifications. Split entries remain separate saved rows.
func (s *SpendingService) InspectSpendingTransactions(ctx context.Context, query SpendingTransactionsQuery) (SpendingTransactionsReport, error) {
	query, err := ValidateSpendingTransactionsQuery(query)
	if err != nil {
		return SpendingTransactionsReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return SpendingTransactionsReport{}, err
	}
	report := SpendingTransactionsReport{
		Month: query.Month, Status: query.Status, Offset: query.Offset, PageSize: query.PageSize,
		Transactions: []SpendingTransaction{},
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	if db == nil {
		return report, nil
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer tx.Rollback()
	month, _ := time.Parse("2006-01", query.Month)
	first := month.Format("2006-01-02")
	last := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	// Numeric date spelling plus the exact month's first/last day also
	// excludes impossible dates such as February 30 before counting/paging.
	err = tx.QueryRowContext(ctx, spendingMonthTransactions+`
		SELECT COUNT(*),
		       COALESCE(SUM(classification = 'unclassified'), 0),
		       COALESCE(SUM(classification = 'classified'), 0),
		       COALESCE(SUM(classification = 'pending'), 0)
		FROM month_transactions`, first, last).Scan(
		&report.Counts.All, &report.Counts.Unclassified, &report.Counts.Classified, &report.Counts.Pending,
	)
	if err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	switch query.Status {
	case "unclassified":
		report.Total = report.Counts.Unclassified
	case "classified":
		report.Total = report.Counts.Classified
	case "pending":
		report.Total = report.Counts.Pending
	default:
		report.Total = report.Counts.All
	}
	if query.Offset >= report.Total {
		return report, nil
	}
	rows, err := tx.QueryContext(ctx, spendingMonthTransactions+`
		SELECT transaction_id, date, name, merchant_name, amount_cents,
		       classification, category, category_source
		FROM month_transactions
		WHERE (? = 'all' OR classification = ?)
		ORDER BY date DESC, transaction_id ASC
		LIMIT ? OFFSET ?`, first, last, query.Status, query.Status, query.PageSize, query.Offset)
	if err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer rows.Close()
	for rows.Next() {
		item := SpendingTransaction{Entries: []SpendingEntry{}}
		var cents int64
		var legacyCategory, legacySource sql.NullString
		if err := rows.Scan(&item.ID, &item.Date, &item.Name, &item.MerchantName, &cents, &item.Classification, &legacyCategory, &legacySource); err != nil {
			return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
		amount := big.NewInt(cents)
		item.NetCents = amount.Neg(amount).String()
		if legacyCategory.Valid {
			item.LegacyCategory = &legacyCategory.String
		}
		if legacySource.Valid {
			item.LegacySource = &legacySource.String
		}
		report.Transactions = append(report.Transactions, item)
	}
	if err := rows.Err(); err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	rows.Close()
	if err := readSpendingEntries(ctx, tx, report.Transactions); err != nil {
		return SpendingTransactionsReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	report.HasMore = len(report.Transactions) < report.Total-query.Offset
	if err := ctx.Err(); err != nil {
		return SpendingTransactionsReport{}, err
	}
	return report, nil
}

func readSpendingEntries(ctx context.Context, tx *sql.Tx, transactions []SpendingTransaction) error {
	if len(transactions) == 0 {
		return nil
	}
	positions := make(map[string]int, len(transactions))
	ids := make([]any, 0, len(transactions))
	for i, transaction := range transactions {
		positions[transaction.ID] = i
		ids = append(ids, transaction.ID)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := tx.QueryContext(ctx, `
		SELECT transaction_id, category, amount_cents, source
		FROM budget_entries WHERE transaction_id IN (`+placeholders+`)
		ORDER BY transaction_id ASC, id ASC`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var cents int64
		var entry SpendingEntry
		if err := rows.Scan(&id, &entry.Category, &cents, &entry.Source); err != nil {
			return err
		}
		entry.AmountCents = strconv.FormatInt(cents, 10)
		i := positions[id]
		transactions[i].Entries = append(transactions[i].Entries, entry)
	}
	return rows.Err()
}
