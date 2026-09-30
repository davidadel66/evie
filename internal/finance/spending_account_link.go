package finance

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"time"

	"github.com/google/uuid"
)

type savedAccountLink struct {
	SpendingAccountLink
	Token, State   string
	ClaimExpiresAt time.Time
}

// A user may finish consent immediately before the Hosted URL expires. Keep
// its result recoverable without promising that the URL remains usable.
const accountLinkResultRecovery = 6 * time.Hour

type accountLinkReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadActiveAccountLink(ctx context.Context, reader accountLinkReader) (savedAccountLink, error) {
	return scanAccountLink(reader.QueryRowContext(ctx, `SELECT id,link_token,hosted_url,expires_at,state,claim_expires_at FROM finance_link_sessions WHERE state IN('pending','exchanging') LIMIT 1`))
}

func loadAccountLink(ctx context.Context, reader accountLinkReader, id string) (savedAccountLink, error) {
	if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
		return savedAccountLink{}, ErrSpendingLinkNotFound
	}
	link, err := scanAccountLink(reader.QueryRowContext(ctx, `SELECT id,link_token,hosted_url,expires_at,state,claim_expires_at FROM finance_link_sessions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return savedAccountLink{}, ErrSpendingLinkNotFound
	}
	return link, err
}

func scanAccountLink(row *sql.Row) (savedAccountLink, error) {
	var link savedAccountLink
	var expires, claim string
	err := row.Scan(&link.ID, &link.Token, &link.HostedURL, &expires, &link.State, &claim)
	if err != nil {
		return link, err
	}
	link.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return link, err
	}
	if claim != "" {
		link.ClaimExpiresAt, err = time.Parse(time.RFC3339Nano, claim)
	}
	return link, err
}

func validHostedLinkURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "secure.plaid.com" && u.User == nil && u.Fragment == "" && u.Path != ""
}

var accountLinkGate = make(chan struct{}, 1)

func enterAccountLink(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case accountLinkGate <- struct{}{}:
		return func() { <-accountLinkGate }, nil
	default:
		return nil, ErrSpendingLinkInProgress
	}
}

func (s *SpendingService) StartAccountLink(ctx context.Context) (SpendingAccountLink, error) {
	release, err := enterAccountLink(ctx)
	if err != nil {
		return SpendingAccountLink{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	provider, err := s.newAccountProvider()
	if err != nil {
		return SpendingAccountLink{}, ErrSpendingLinkUnavailable
	}
	db, err := s.openWrite(ctx)
	if err != nil {
		return SpendingAccountLink{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	defer db.Close()
	current, err := loadActiveAccountLink(ctx, db)
	if err == nil {
		current, err = s.resolveStaleAccountLink(ctx, db, current)
		if err != nil {
			return SpendingAccountLink{}, spendingError(ctx, ErrSpendingLinkUnavailable)
		}
		if current.State == "pending" || current.State == "exchanging" {
			if !current.ExpiresAt.After(s.now()) {
				return SpendingAccountLink{}, ErrSpendingLinkInProgress
			}
			if !validHostedLinkURL(current.HostedURL) {
				return SpendingAccountLink{}, ErrSpendingLinkUnavailable
			}
			return current.SpendingAccountLink, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SpendingAccountLink{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	now := s.now().UTC()
	created, err := provider.StartLink(ctx)
	if err != nil {
		return SpendingAccountLink{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	if !validHostedLinkURL(created.URL) || created.Token == "" || !created.ExpiresAt.After(now) {
		return SpendingAccountLink{}, ErrSpendingLinkUnavailable
	}
	// The Hosted URL lifetime is explicitly configured to 30 minutes. A shorter
	// provider expiry wins; the browser never promises the URL outlives either.
	if limit := now.Add(30 * time.Minute); created.ExpiresAt.After(limit) {
		created.ExpiresAt = limit
	}
	link := SpendingAccountLink{ID: uuid.NewString(), HostedURL: created.URL, ExpiresAt: created.ExpiresAt.UTC()}
	_, err = db.ExecContext(ctx, `INSERT INTO finance_link_sessions(id,link_token,hosted_url,expires_at,state,created_at) VALUES(?,?,?,?,'pending',?)`, link.ID, created.Token, link.HostedURL, link.ExpiresAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		// Another process may have installed the pending flow, or the write may
		// have landed before cancellation. Return its durable winner when known.
		checkCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		winner, checkErr := loadActiveAccountLink(checkCtx, db)
		if checkErr == nil && winner.ExpiresAt.After(s.now()) && validHostedLinkURL(winner.HostedURL) {
			return winner.SpendingAccountLink, nil
		}
		return SpendingAccountLink{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	return link, nil
}

func (s *SpendingService) resolveStaleAccountLink(ctx context.Context, db *sql.DB, link savedAccountLink) (savedAccountLink, error) {
	state := ""
	if link.State == "pending" && !link.ExpiresAt.Add(accountLinkResultRecovery).After(s.now()) {
		state = "expired"
	}
	// A crash or ambiguous exchange must never replay a single-use public
	// token. After its bounded claim expires the owner can start a fresh flow.
	if link.State == "exchanging" && !link.ClaimExpiresAt.After(s.now()) {
		state = "failed"
	}
	if state != "" {
		if _, err := db.ExecContext(ctx, `UPDATE finance_link_sessions SET state=?,link_token='',hosted_url='' WHERE id=? AND state=?`, state, link.ID, link.State); err != nil {
			return link, err
		}
		return loadAccountLink(ctx, db, link.ID)
	}
	return link, nil
}

func (s *SpendingService) CompleteAccountLink(ctx context.Context, id string) (SpendingAccountLinkResult, error) {
	release, err := enterAccountLink(ctx)
	if err != nil {
		return SpendingAccountLinkResult{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := s.openWrite(ctx)
	if err != nil {
		return SpendingAccountLinkResult{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	defer db.Close()
	link, err := loadAccountLink(ctx, db, id)
	if err != nil {
		return SpendingAccountLinkResult{}, accountLinkError(ctx, err)
	}
	link, err = s.resolveStaleAccountLink(ctx, db, link)
	if err != nil {
		return SpendingAccountLinkResult{}, accountLinkError(ctx, err)
	}
	if link.State != "pending" {
		return accountLinkResult(link.State), nil
	}
	provider, err := s.newAccountProvider()
	if err != nil {
		return SpendingAccountLinkResult{}, ErrSpendingLinkUnavailable
	}
	publicToken, err := provider.PublicToken(ctx, link.Token)
	if err != nil {
		return SpendingAccountLinkResult{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	if publicToken == "" {
		if !link.ExpiresAt.After(s.now()) {
			if _, err := db.ExecContext(ctx, `UPDATE finance_link_sessions SET state='expired',link_token='',hosted_url='' WHERE id=? AND state='pending'`, id); err != nil {
				return SpendingAccountLinkResult{}, spendingError(ctx, ErrSpendingLinkUnavailable)
			}
			known, err := loadAccountLink(ctx, db, id)
			if err != nil {
				return SpendingAccountLinkResult{}, accountLinkError(ctx, err)
			}
			return accountLinkResult(known.State), nil
		}
		return accountLinkResult("pending"), nil
	}
	claimed, err := db.ExecContext(ctx, `UPDATE finance_link_sessions SET state='exchanging',claim_expires_at=? WHERE id=? AND state='pending'`, s.now().UTC().Add(2*time.Minute).Format(time.RFC3339Nano), id)
	if err != nil {
		return SpendingAccountLinkResult{}, spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	n, err := claimed.RowsAffected()
	if err != nil {
		return SpendingAccountLinkResult{}, ErrSpendingLinkUnavailable
	}
	if n != 1 {
		return SpendingAccountLinkResult{}, ErrSpendingLinkInProgress
	}
	item, exchangeErr := provider.Exchange(ctx, publicToken)
	// Obtained credentials are persisted even if the browser went away. This
	// bounded cleanup context also records an ambiguous exchange as terminal.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if exchangeErr != nil || item.ID == "" || item.AccessToken == "" {
		if err := markAccountLinkFailed(saveCtx, db, id); err != nil {
			return SpendingAccountLinkResult{}, ErrSpendingLinkUnavailable
		}
		return accountLinkResult("failed"), nil
	}
	if err := saveLinkedAccountItemWithRetry(saveCtx, db, id, item, s.now()); err != nil {
		// A COMMIT error can be uncertain. Check durable state before deciding
		// what the owner sees; a completed exchange is never submitted again.
		checkCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer stop()
		known, readErr := loadAccountLink(checkCtx, db, id)
		if readErr != nil {
			return SpendingAccountLinkResult{}, ErrSpendingLinkUnavailable
		}
		if known.State != "linked" {
			if err := markAccountLinkFailed(checkCtx, db, id); err != nil {
				return SpendingAccountLinkResult{}, ErrSpendingLinkUnavailable
			}
			return accountLinkResult("failed"), nil
		}
	}
	// Account metadata is optional follow-up to this explicit Link action. A
	// metadata failure cannot undo or misreport the already saved connection.
	if ctx.Err() == nil {
		select {
		case accountRefreshGate <- struct{}{}:
			_ = s.refreshOneAccountInventory(ctx, db, provider, syncItem{itemID: item.ID, accessToken: item.AccessToken})
			<-accountRefreshGate
		default:
		}
	}
	return accountLinkResult("linked"), nil
}

func saveLinkedAccountItemWithRetry(ctx context.Context, db *sql.DB, id string, item linkedAccountItem, now time.Time) error {
	for {
		err := saveLinkedAccountItem(ctx, db, id, item, now)
		var coded interface{ Code() int }
		if !errors.As(err, &coded) || (coded.Code()&255 != 5 && coded.Code()&255 != 6) {
			return err
		}
		// Retry only the local transaction, never the single-use token exchange.
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func saveLinkedAccountItem(ctx context.Context, db *sql.DB, id string, item linkedAccountItem, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO items(item_id,access_token,cursor,linked_at) VALUES(?,?,'',?) ON CONFLICT(item_id) DO UPDATE SET access_token=excluded.access_token,linked_at=excluded.linked_at`, item.ID, item.AccessToken, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE finance_link_sessions SET state='linked',link_token='',hosted_url='' WHERE id=? AND state='exchanging'`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrSpendingLinkInProgress
	}
	return tx.Commit()
}

func markAccountLinkFailed(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `UPDATE finance_link_sessions SET state='failed',link_token='',hosted_url='' WHERE id=? AND state='exchanging'`, id)
	return err
}

func accountLinkResult(state string) SpendingAccountLinkResult {
	if state == "exchanging" {
		state = "pending"
	}
	result := SpendingAccountLinkResult{Status: state}
	if state == "linked" {
		result.InstitutionsLinked = 1
	}
	return result
}

func accountLinkError(ctx context.Context, err error) error {
	if errors.Is(err, ErrSpendingLinkNotFound) {
		return ErrSpendingLinkNotFound
	}
	return spendingError(ctx, ErrSpendingLinkUnavailable)
}

// CancelAccountLink forgets an unfinished local flow. It never calls Item
// removal and never changes an already saved bank connection.
func (s *SpendingService) CancelAccountLink(ctx context.Context, id string) error {
	release, err := enterAccountLink(ctx)
	if err != nil {
		return err
	}
	defer release()
	db, err := s.openWrite(ctx)
	if err != nil {
		return spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	defer db.Close()
	link, err := loadAccountLink(ctx, db, id)
	if err != nil {
		return accountLinkError(ctx, err)
	}
	link, err = s.resolveStaleAccountLink(ctx, db, link)
	if err != nil {
		return accountLinkError(ctx, err)
	}
	if link.State == "exchanging" {
		return ErrSpendingLinkInProgress
	}
	if link.State != "pending" {
		return nil
	}
	result, err := db.ExecContext(ctx, `UPDATE finance_link_sessions SET state='cancelled',link_token='',hosted_url='' WHERE id=? AND state='pending'`, id)
	if err != nil {
		return spendingError(ctx, ErrSpendingLinkUnavailable)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return ErrSpendingLinkUnavailable
	}
	if n != 1 {
		known, err := loadAccountLink(ctx, db, id)
		if err != nil {
			return accountLinkError(ctx, err)
		}
		if known.State == "pending" || known.State == "exchanging" {
			return ErrSpendingLinkInProgress
		}
	}
	return nil
}
