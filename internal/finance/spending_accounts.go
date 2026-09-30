package finance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"time"
)

var (
	ErrSpendingAccountsUnavailable        = errors.New("connected accounts are unavailable")
	ErrSpendingAccountsRefreshUnavailable = errors.New("connected accounts could not be refreshed")
	ErrSpendingAccountsRefreshInProgress  = errors.New("an account refresh is already in progress")
	ErrSpendingLinkUnavailable            = errors.New("bank linking is unavailable; check Plaid configuration and try again")
	ErrSpendingLinkNotFound               = errors.New("bank linking session was not found; start again")
	ErrSpendingLinkInProgress             = errors.New("bank linking is being completed; try again shortly")
)

type SpendingAccount struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mask    string `json:"mask"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
}

type SpendingInstitution struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	AccountsKnown     bool              `json:"accountsKnown"`
	AccountsUpdatedAt *time.Time        `json:"accountsUpdatedAt,omitempty"`
	Accounts          []SpendingAccount `json:"accounts"`
}

type SpendingAccountsReport struct {
	Institutions  []SpendingInstitution `json:"institutions"`
	LinkAvailable bool                  `json:"linkAvailable"`
	PendingLink   *SpendingAccountLink  `json:"pendingLink,omitempty"`
}

type SpendingAccountsRefresh struct {
	InstitutionsSucceeded int `json:"institutionsSucceeded"`
	InstitutionsFailed    int `json:"institutionsFailed"`
}

type SpendingAccountLink struct {
	ID        string    `json:"id"`
	HostedURL string    `json:"hostedUrl"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type SpendingAccountLinkResult struct {
	Status             string `json:"status"`
	InstitutionsLinked int    `json:"institutionsLinked"`
}

const spendingAccountsSchema = `
CREATE TABLE IF NOT EXISTS finance_accounts (
 item_id TEXT NOT NULL REFERENCES items(item_id), account_id TEXT NOT NULL,
 name TEXT NOT NULL, mask TEXT NOT NULL, type TEXT NOT NULL, subtype TEXT NOT NULL,
 PRIMARY KEY(item_id,account_id)
);
CREATE TABLE IF NOT EXISTS finance_account_snapshots (
 item_id TEXT PRIMARY KEY REFERENCES items(item_id), updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS finance_link_sessions (
 id TEXT PRIMARY KEY, link_token TEXT NOT NULL, hosted_url TEXT NOT NULL,
 expires_at TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN('pending','exchanging','linked','failed','cancelled','expired')),
 claim_expires_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS finance_link_one_pending ON finance_link_sessions((1)) WHERE state IN('pending','exchanging');
`

// These identities are stable display keys derived from high-entropy provider
// identifiers. The underlying account/item IDs and credentials never leave
// this package's owner-facing results.
func spendingAccountID(kind, itemID, accountID string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + itemID + "\x00" + accountID))
	return kind + "_" + hex.EncodeToString(sum[:16])
}

func (s *SpendingService) accountLinkAvailable() bool {
	return s.accountProviderFactory != nil || (os.Getenv("PLAID_CLIENT_ID") != "" && os.Getenv("PLAID_SECRET") != "")
}

// InspectAccounts reads only existing local records. Legacy installations are
// shown immediately with unknown inventories, without migration or bank calls.
func (s *SpendingService) InspectAccounts(ctx context.Context) (SpendingAccountsReport, error) {
	report := SpendingAccountsReport{Institutions: []SpendingInstitution{}, LinkAvailable: s.accountLinkAvailable()}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
	}
	if db == nil {
		return report, nil
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT item_id,COALESCE(institution,'') FROM items ORDER BY COALESCE(institution,''),linked_at,item_id`)
	if err != nil {
		return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
	}
	var itemIDs []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
		}
		if name == "" {
			name = "Connected bank"
		}
		itemIDs = append(itemIDs, id)
		report.Institutions = append(report.Institutions, SpendingInstitution{ID: spendingAccountID("bank", id, ""), Name: name, Accounts: []SpendingAccount{}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
	}
	var hasAccounts, hasLinks bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='finance_account_snapshots'),EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='finance_link_sessions')`).Scan(&hasAccounts, &hasLinks); err != nil {
		return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
	}
	if hasAccounts {
		for i, id := range itemIDs {
			if err := readSpendingInstitution(ctx, tx, id, &report.Institutions[i]); err != nil {
				return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
			}
		}
	}
	if hasLinks {
		link, err := loadActiveAccountLink(ctx, tx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return report, spendingError(ctx, ErrSpendingAccountsUnavailable)
		}
		if err == nil && ((link.State == "pending" && link.ExpiresAt.Add(accountLinkResultRecovery).After(s.now())) || (link.State == "exchanging" && link.ClaimExpiresAt.After(s.now()))) {
			if !validHostedLinkURL(link.HostedURL) {
				return report, ErrSpendingAccountsUnavailable
			}
			report.PendingLink = &link.SpendingAccountLink
		}
	}
	return report, nil
}

func readSpendingInstitution(ctx context.Context, tx *sql.Tx, itemID string, institution *SpendingInstitution) error {
	var updated string
	err := tx.QueryRowContext(ctx, `SELECT updated_at FROM finance_account_snapshots WHERE item_id=?`, itemID).Scan(&updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	stamp, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return err
	}
	institution.AccountsKnown = true
	institution.AccountsUpdatedAt = &stamp
	rows, err := tx.QueryContext(ctx, `SELECT account_id,name,mask,type,subtype FROM finance_accounts WHERE item_id=? ORDER BY name,mask,account_id`, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var account SpendingAccount
		if err := rows.Scan(&id, &account.Name, &account.Mask, &account.Type, &account.Subtype); err != nil {
			return err
		}
		account.ID = spendingAccountID("account", itemID, id)
		institution.Accounts = append(institution.Accounts, account)
	}
	return rows.Err()
}

var accountRefreshGate = make(chan struct{}, 1)

func (s *SpendingService) RefreshAccounts(ctx context.Context) (SpendingAccountsRefresh, error) {
	var result SpendingAccountsRefresh
	if err := ctx.Err(); err != nil {
		return result, err
	}
	select {
	case accountRefreshGate <- struct{}{}:
		defer func() { <-accountRefreshGate }()
	default:
		return result, ErrSpendingAccountsRefreshInProgress
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	provider, err := s.newAccountProvider()
	if err != nil {
		return result, ErrSpendingAccountsRefreshUnavailable
	}
	readDB, err := s.openRead(ctx)
	if err != nil {
		return result, spendingError(ctx, ErrSpendingAccountsRefreshUnavailable)
	}
	if readDB == nil {
		return result, nil
	}
	items, err := loadAccountItems(ctx, readDB)
	readDB.Close()
	if err != nil {
		return result, spendingError(ctx, ErrSpendingAccountsRefreshUnavailable)
	}
	if len(items) == 0 {
		return result, nil
	}
	db, err := s.openWrite(ctx)
	if err != nil {
		return result, spendingError(ctx, ErrSpendingAccountsRefreshUnavailable)
	}
	defer db.Close()
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := s.refreshOneAccountInventory(ctx, db, provider, item); err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.InstitutionsFailed++
		} else {
			result.InstitutionsSucceeded++
		}
	}
	return result, nil
}

func loadAccountItems(ctx context.Context, db *sql.DB) ([]syncItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT item_id,access_token,COALESCE(institution,'') FROM items ORDER BY item_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []syncItem
	for rows.Next() {
		var item syncItem
		if err := rows.Scan(&item.itemID, &item.accessToken, &item.institution); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SpendingService) refreshOneAccountInventory(ctx context.Context, db *sql.DB, provider spendingAccountProvider, item syncItem) error {
	inventory, err := provider.Accounts(ctx, item.accessToken)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A refreshed credential supersedes any inventory still arriving for the
	// previous access token. Do not erase a newer connection's account snapshot.
	var current bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM items WHERE item_id=? AND access_token=?)`, item.itemID, item.accessToken).Scan(&current); err != nil {
		return err
	}
	if !current {
		return errors.New("bank connection changed")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM finance_accounts WHERE item_id=?`, item.itemID); err != nil {
		return err
	}
	for _, account := range inventory.Accounts {
		if account.ID == "" {
			return errors.New("missing provider account identity")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO finance_accounts(item_id,account_id,name,mask,type,subtype) VALUES(?,?,?,?,?,?)`, item.itemID, account.ID, account.Name, account.Mask, account.Type, account.Subtype); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finance_account_snapshots(item_id,updated_at) VALUES(?,?) ON CONFLICT(item_id) DO UPDATE SET updated_at=excluded.updated_at`, item.itemID, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if inventory.Institution != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE items SET institution=? WHERE item_id=?`, inventory.Institution, item.itemID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
