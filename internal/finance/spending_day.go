package finance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSpendingDayQuery            = errors.New("choose a valid day and transaction page")
	ErrSpendingCategoryQuery       = errors.New("choose a saved transaction and configured category")
	ErrSpendingCategoryConflict    = errors.New("this transaction changed; reload it before saving")
	ErrSpendingCategoryUnavailable = errors.New("the category could not be saved")
)

type SpendingDayQuery struct {
	Date     string `json:"date"`
	Offset   int    `json:"offset"`
	PageSize int    `json:"pageSize"`
}

type SpendingDayEntry struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	AmountCents string `json:"amountCents"`
	Source      string `json:"source"`
}

type SpendingDayTransaction struct {
	ID             string             `json:"id"`
	Date           string             `json:"date"`
	Name           string             `json:"name"`
	MerchantName   string             `json:"merchantName"`
	NetCents       string             `json:"netCents"`
	Classification string             `json:"classification"`
	Entries        []SpendingDayEntry `json:"entries"`
	LegacyCategory *string            `json:"legacyCategory"`
	LegacySource   *string            `json:"legacySource"`
	Revision       string             `json:"revision"`
}

type SpendingDayReport struct {
	Date         string                   `json:"date"`
	Offset       int                      `json:"offset"`
	PageSize     int                      `json:"pageSize"`
	Total        int                      `json:"total"`
	HasMore      bool                     `json:"hasMore"`
	Categories   []string                 `json:"categories"`
	Summary      SpendingDay              `json:"summary"`
	Transactions []SpendingDayTransaction `json:"transactions"`
}

type SpendingCategoryUpdate struct {
	TransactionID string  `json:"transactionId"`
	EntryID       *string `json:"entryId"`
	Category      string  `json:"category"`
	Revision      string  `json:"revision"`
}

func ValidateSpendingDayQuery(query SpendingDayQuery) (SpendingDayQuery, error) {
	day, err := time.Parse("2006-01-02", query.Date)
	if err != nil || day.Year() < 1 || day.Format("2006-01-02") != query.Date || query.Offset < 0 || query.PageSize < 0 || query.PageSize > 100 {
		return SpendingDayQuery{}, ErrSpendingDayQuery
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	return query, nil
}

func ValidateSpendingCategoryUpdate(update SpendingCategoryUpdate) (SpendingCategoryUpdate, error) {
	if strings.TrimSpace(update.TransactionID) == "" || len(update.TransactionID) > 4096 || strings.ContainsRune(update.TransactionID, 0) || strings.TrimSpace(update.Category) == "" || len(update.Category) > 512 || strings.ContainsRune(update.Category, 0) {
		return SpendingCategoryUpdate{}, ErrSpendingCategoryQuery
	}
	if len(update.Revision) != 64 {
		return SpendingCategoryUpdate{}, ErrSpendingCategoryQuery
	}
	if decoded, err := hex.DecodeString(update.Revision); err != nil || hex.EncodeToString(decoded) != update.Revision {
		return SpendingCategoryUpdate{}, ErrSpendingCategoryQuery
	}
	if update.EntryID != nil {
		id, err := strconv.ParseInt(*update.EntryID, 10, 64)
		if err != nil || strconv.FormatInt(id, 10) != *update.EntryID {
			return SpendingCategoryUpdate{}, ErrSpendingCategoryQuery
		}
	}
	return update, nil
}

// An edit must never create a finance database or apply schema changes. SQLite's
// mode=rw enforces this at open time, including if the file disappears first.
func openSpendingExisting(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.Join(home, ".finance", "finance.db")}
	uri.RawQuery = "mode=rw&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type spendingDayReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// The revision includes every saved field, even private fields that are never
// serialized to the owner view. Sync or another editor changing any part of
// the transaction/allocation snapshot invalidates an older edit.
type spendingDaySnapshot struct {
	ID             string
	ItemID         string
	AccountID      sql.NullString
	Date           sql.NullString
	Name           sql.NullString
	MerchantName   sql.NullString
	AmountCents    int64
	PlaidCategory  sql.NullString
	Category       sql.NullString
	CategorySource sql.NullString
	Reviewed       int64
	Pending        int64
	Tags           string
	Entries        []spendingDayEntrySnapshot
}

type spendingDayEntrySnapshot struct {
	ID          int64
	Category    string
	AmountCents int64
	Source      string
	Tags        string
}

func (snapshot spendingDaySnapshot) view() SpendingDayTransaction {
	encoded, _ := json.Marshal(snapshot)
	digest := sha256.Sum256(encoded)
	row := SpendingDayTransaction{
		ID: snapshot.ID, Date: snapshot.Date.String, Name: snapshot.Name.String,
		MerchantName:   snapshot.MerchantName.String,
		NetCents:       new(big.Int).Neg(big.NewInt(snapshot.AmountCents)).String(),
		Classification: "unclassified", Entries: []SpendingDayEntry{}, Revision: hex.EncodeToString(digest[:]),
	}
	if snapshot.Category.Valid {
		category := snapshot.Category.String
		row.LegacyCategory = &category
	}
	if snapshot.CategorySource.Valid {
		source := snapshot.CategorySource.String
		row.LegacySource = &source
	}
	for _, entry := range snapshot.Entries {
		row.Entries = append(row.Entries, SpendingDayEntry{
			ID: strconv.FormatInt(entry.ID, 10), Category: entry.Category,
			AmountCents: strconv.FormatInt(entry.AmountCents, 10), Source: entry.Source,
		})
	}
	if len(row.Entries) > 0 {
		row.Classification = "classified"
	}
	if snapshot.Pending != 0 {
		row.Classification = "pending"
	}
	return row
}

func readSpendingDaySnapshots(ctx context.Context, reader spendingDayReader, where string, args ...any) ([]spendingDaySnapshot, error) {
	rows, err := reader.QueryContext(ctx, `SELECT transaction_id,item_id,account_id,date,name,merchant_name,amount_cents,plaid_category,category,category_source,reviewed,pending,tags FROM transactions `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []spendingDaySnapshot
	for rows.Next() {
		var snapshot spendingDaySnapshot
		if err := rows.Scan(&snapshot.ID, &snapshot.ItemID, &snapshot.AccountID, &snapshot.Date, &snapshot.Name, &snapshot.MerchantName, &snapshot.AmountCents, &snapshot.PlaidCategory, &snapshot.Category, &snapshot.CategorySource, &snapshot.Reviewed, &snapshot.Pending, &snapshot.Tags); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if len(snapshots) == 0 {
		return snapshots, nil
	}
	positions := make(map[string]int, len(snapshots))
	ids := make([]any, len(snapshots))
	for i, snapshot := range snapshots {
		positions[snapshot.ID], ids[i] = i, snapshot.ID
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	entries, err := reader.QueryContext(ctx, `SELECT transaction_id,id,category,amount_cents,source,tags FROM budget_entries WHERE transaction_id IN (`+placeholders+`) ORDER BY transaction_id,id`, ids...)
	if err != nil {
		return nil, err
	}
	defer entries.Close()
	for entries.Next() {
		var id string
		var entry spendingDayEntrySnapshot
		if err := entries.Scan(&id, &entry.ID, &entry.Category, &entry.AmountCents, &entry.Source, &entry.Tags); err != nil {
			return nil, err
		}
		i := positions[id]
		snapshots[i].Entries = append(snapshots[i].Entries, entry)
	}
	return snapshots, entries.Err()
}

// InspectSpendingDay includes only the posted rows that contribute to this
// calendar cell. Totals, categories, page rows, and edit revisions share a read
// snapshot. Neither this read nor a category edit contacts a bank.
func (s *SpendingService) InspectSpendingDay(ctx context.Context, query SpendingDayQuery) (SpendingDayReport, error) {
	query, err := ValidateSpendingDayQuery(query)
	if err != nil {
		return SpendingDayReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return SpendingDayReport{}, err
	}
	report := SpendingDayReport{
		Date: query.Date, Offset: query.Offset, PageSize: query.PageSize,
		Categories: []string{}, Transactions: []SpendingDayTransaction{},
		Summary: SpendingDay{Date: query.Date, InflowCents: "0", OutflowCents: "0", NetCents: "0"},
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	if db == nil {
		return report, nil
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT amount_cents FROM transactions WHERE date=? AND pending=0`, query.Date)
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	var inflow, outflow big.Int
	for rows.Next() {
		var cents int64
		if err := rows.Scan(&cents); err != nil {
			rows.Close()
			return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
		if cents < 0 {
			inflow.Sub(&inflow, big.NewInt(cents))
		} else {
			outflow.Add(&outflow, big.NewInt(cents))
		}
		report.Total++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	report.Summary.InflowCents, report.Summary.OutflowCents = inflow.String(), outflow.String()
	report.Summary.NetCents, report.Summary.Transactions = new(big.Int).Sub(&inflow, &outflow).String(), report.Total
	categories, err := tx.QueryContext(ctx, `SELECT name FROM categories WHERE TRIM(name) != '' ORDER BY name COLLATE NOCASE,name`)
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	for categories.Next() {
		var category string
		if err := categories.Scan(&category); err != nil {
			categories.Close()
			return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
		}
		report.Categories = append(report.Categories, category)
	}
	err = categories.Err()
	categories.Close()
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	snapshots, err := readSpendingDaySnapshots(ctx, tx, `WHERE date=? AND pending=0 ORDER BY transaction_id LIMIT ? OFFSET ?`, query.Date, query.PageSize, query.Offset)
	if err != nil {
		return SpendingDayReport{}, spendingError(ctx, ErrSpendingUnavailable)
	}
	for _, snapshot := range snapshots {
		report.Transactions = append(report.Transactions, snapshot.view())
	}
	report.HasMore = query.Offset < report.Total && len(report.Transactions) < report.Total-query.Offset
	if err := ctx.Err(); err != nil {
		return SpendingDayReport{}, err
	}
	return report, nil
}

// UpdateSpendingCategory changes only one category/source, preserving amounts,
// allocation identities and tags. Entry-less rows gain one human entry for the
// raw transaction amount. Existing splits are never collapsed or repaired.
func (s *SpendingService) UpdateSpendingCategory(ctx context.Context, update SpendingCategoryUpdate) (SpendingDayTransaction, error) {
	update, err := ValidateSpendingCategoryUpdate(update)
	if err != nil {
		return SpendingDayTransaction{}, err
	}
	if err := ctx.Err(); err != nil {
		return SpendingDayTransaction{}, err
	}
	db, err := s.openEdit(ctx)
	if err != nil || db == nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	committed := false
	defer func() {
		if !committed {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := conn.ExecContext(rollbackCtx, `ROLLBACK`); err != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
	}()
	snapshots, err := readSpendingDaySnapshots(ctx, conn, `WHERE transaction_id=?`, update.TransactionID)
	if err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	if len(snapshots) != 1 || snapshots[0].Pending != 0 || snapshots[0].view().Revision != update.Revision {
		return SpendingDayTransaction{}, ErrSpendingCategoryConflict
	}
	snapshot := snapshots[0]
	if _, err := ValidateSpendingDayQuery(SpendingDayQuery{Date: snapshot.Date.String}); err != nil {
		return SpendingDayTransaction{}, ErrSpendingCategoryConflict
	}
	var categoryExists bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM categories WHERE name=?)`, update.Category).Scan(&categoryExists); err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	if !categoryExists {
		return SpendingDayTransaction{}, ErrSpendingCategoryQuery
	}
	if update.EntryID == nil {
		if len(snapshot.Entries) != 0 {
			return SpendingDayTransaction{}, ErrSpendingCategoryConflict
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO budget_entries(transaction_id,category,amount_cents,source) VALUES(?,?,?,'human')`, update.TransactionID, update.Category, snapshot.AmountCents)
	} else {
		found := false
		for _, entry := range snapshot.Entries {
			if strconv.FormatInt(entry.ID, 10) == *update.EntryID {
				found = true
				break
			}
		}
		if !found {
			return SpendingDayTransaction{}, ErrSpendingCategoryConflict
		}
		_, err = conn.ExecContext(ctx, `UPDATE budget_entries SET category=?,source='human' WHERE id=? AND transaction_id=?`, update.Category, *update.EntryID, update.TransactionID)
	}
	if err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	updated, err := readSpendingDaySnapshots(ctx, conn, `WHERE transaction_id=?`, update.TransactionID)
	if err != nil || len(updated) != 1 {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return SpendingDayTransaction{}, err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return SpendingDayTransaction{}, spendingError(ctx, ErrSpendingCategoryUnavailable)
	}
	committed = true
	return updated[0].view(), nil
}
