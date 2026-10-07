package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Workflow effects over the ledger (finance-daily, 2026-10-06). The
// Workflow Runtime sends each Effect Intent with an idempotency key it
// derives from the run, the node and the node's input, and may send the
// same intent again: as a retry within the step's Retry Policy, or as a
// reconciliation replay. The finance database keeps no per-request record
// of its own (Categorize is idempotent over state, not per request), so
// each effect records one effect_receipts row under its key, in the same
// transaction as its writes where there is one transaction. A replay finds
// the row and returns the recorded output without acting; reconciliation
// answers "happened" from the row or "did not happen" from its absence.
//
// Sync commits each bank's pages in their own transactions, so its receipt
// lands after the last bank. An intent interrupted before that leaves no
// receipt: "did not happen" is then honest, because every page it did
// commit is one Plaid's cursor will not send again, and a re-sent intent
// continues from the committed cursor.

// Effect names recorded with each receipt.
const (
	EffectSyncLedger       = "sync_ledger"
	EffectCategorizeLedger = "categorize_ledger"
)

// ErrEffectKeyRequired is returned when an effect is asked to act without
// an idempotency key: nothing is performed.
var ErrEffectKeyRequired = errors.New("an idempotency key is required")

// SyncBankOutcome is one bank's outcome in a SyncLedgerOutput.
type SyncBankOutcome struct {
	Label    string   `json:"label"`
	Added    int      `json:"added"`
	Modified int      `json:"modified"`
	Removed  int      `json:"removed"`
	Failed   bool     `json:"failed"`
	Error    string   `json:"error"`
	Warnings []string `json:"warnings"`
}

// SyncLedgerOutput is the sync_ledger effect's response.
type SyncLedgerOutput struct {
	Banks       []SyncBankOutcome `json:"banks"`
	Added       int               `json:"added"`
	Modified    int               `json:"modified"`
	Removed     int               `json:"removed"`
	FailedBanks int               `json:"failed_banks"`
	// Replayed is true when the output was recorded by an earlier intent
	// with the same key and nothing was performed this time.
	Replayed bool `json:"replayed"`
}

// CategorizeLedgerOutput is the categorize_ledger effect's response.
type CategorizeLedgerOutput struct {
	Categorized   int  `json:"categorized"`
	Uncategorized int  `json:"uncategorized"`
	Replayed      bool `json:"replayed"`
}

// SyncLedger syncs every linked bank once under key. A second call with
// the same key returns the recorded output with Replayed set and calls
// Plaid for nothing.
func SyncLedger(ctx context.Context, db *sql.DB, key string) (SyncLedgerOutput, error) {
	var out SyncLedgerOutput
	if key == "" {
		return out, ErrEffectKeyRequired
	}
	if found, err := lookupReceipt(ctx, db, key, &out); err != nil || found {
		out.Replayed = found
		return out, err
	}
	res, err := Sync(ctx, db)
	if err != nil {
		return out, err
	}
	out = syncLedgerOutput(res)
	if err := recordReceipt(ctx, db, key, EffectSyncLedger, out); err != nil {
		return out, err
	}
	return out, nil
}

func syncLedgerOutput(res *SyncResult) SyncLedgerOutput {
	out := SyncLedgerOutput{Banks: make([]SyncBankOutcome, 0, len(res.Banks)), Added: res.Totals.Added, Modified: res.Totals.Modified, Removed: res.Totals.Removed}
	for _, b := range res.Banks {
		o := SyncBankOutcome{Label: b.Label, Added: b.Counts.Added, Modified: b.Counts.Modified, Removed: b.Counts.Removed, Warnings: append([]string{}, b.Warnings...)}
		if b.Err != nil {
			o.Failed, o.Error = true, b.Err.Error()
			out.FailedBanks++
		}
		out.Banks = append(out.Banks, o)
	}
	return out
}

// CategorizeLedger categorizes every posted transaction without a budget
// entry once under key, recording the receipt in the same transaction as
// the entries it mints. A second call with the same key returns the
// recorded output with Replayed set and mints nothing.
func CategorizeLedger(ctx context.Context, db *sql.DB, key string) (CategorizeLedgerOutput, error) {
	var out CategorizeLedgerOutput
	if key == "" {
		return out, ErrEffectKeyRequired
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("begin categorize: %w", err)
	}
	defer tx.Rollback()
	if found, err := lookupReceipt(ctx, tx, key, &out); err != nil || found {
		out.Replayed = found
		return out, err
	}
	matched, unmatched, err := categorizeTx(ctx, tx)
	if err != nil {
		return out, err
	}
	out = CategorizeLedgerOutput{Categorized: matched, Uncategorized: unmatched}
	if err := recordReceipt(ctx, tx, key, EffectCategorizeLedger, out); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("commit categorize: %w", err)
	}
	return out, nil
}

// LookupEffect answers reconciliation: whether an effect under key was
// recorded, and its output decoded into into.
func LookupEffect(ctx context.Context, db *sql.DB, key string, into any) (bool, error) {
	if key == "" {
		return false, ErrEffectKeyRequired
	}
	return lookupReceipt(ctx, db, key, into)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func lookupReceipt(ctx context.Context, q querier, key string, into any) (bool, error) {
	var output string
	err := q.QueryRowContext(ctx, `SELECT output FROM effect_receipts WHERE idempotency_key = ?`, key).Scan(&output)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up effect receipt: %w", err)
	}
	if err := json.Unmarshal([]byte(output), into); err != nil {
		return false, fmt.Errorf("decode effect receipt %s: %w", key, err)
	}
	return true, nil
}

func recordReceipt(ctx context.Context, q querier, key, effect string, output any) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode effect receipt: %w", err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO effect_receipts (idempotency_key, effect, output, recorded_at) VALUES (?, ?, ?, ?)`,
		key, effect, string(encoded), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record effect receipt: %w", err)
	}
	return nil
}
