# SPEC: `finance sync` — pull transactions from Plaid into SQLite

## Purpose

Add a `finance sync` subcommand that, for every linked bank in the `items` table
(`~/.finance/finance.db`), calls Plaid's `/transactions/sync` API and mirrors the
results into the existing `transactions` table. Re-running `sync` is incremental:
each item's `cursor` column tracks where the last sync left off.

## Files involved

| File | Change |
|---|---|
| `plaid.go` | Add `runSync(db *sql.DB) error` — Plaid API calls, pagination loop, institution backfill. |
| `sync.go` (new) | Pure DB logic: `applySyncPage(db, itemID, added, modified, removed, nextCursor) error`. No printing, returns errors (repo convention: domain layer is silent). |
| `db.go` | Refactor `openDB()` to delegate to `openDBAt(path string)` so tests can use a temp DB. No schema changes. |
| `main.go` | Add `sync` case to the arg dispatch. |
| `sync_test.go` (new) | Tests for `applySyncPage` against a temp SQLite DB. |

## Key decisions

- **`/transactions/sync`, not `/transactions/get`** — cursor-based incremental sync is Plaid's recommended API and matches the existing `cursor` column.
- **Atomic per page**: each page of results (added/modified/removed + `next_cursor`) is applied in **one SQL transaction**. If the process dies mid-sync, the cursor always matches what's in the DB — no lost or double-applied updates. (Plaid's own guidance.)
- **Added + modified → UPSERT** into `transactions` by `transaction_id`; **removed → DELETE**. This handles pending→posted transitions correctly (Plaid removes the pending txn and adds the posted one).
- **Amounts stored as cents** (`int64`), converted from Plaid's float dollars with `math.Round(v*100)`. Plaid convention kept as-is: positive = money out.
- **Category**: use Plaid's `personal_finance_category.primary` (fallback empty string).
- **Pagination**: loop while `has_more` is true. On Plaid error `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION`, restart that item's sync from the item's last **committed** cursor (which is exactly what's in the DB — so simply retry the item once).
- **Institution backfill**: when `items.institution` is empty, call `/item/get` → `institution_id` → `/institutions/get_by_id` → store the name. Done once per item, before syncing it.
- **Output** (CLI layer only): one line per bank — `Chase: 42 added, 3 modified, 1 removed` — and a final total. Errors on one item don't abort the others; they're reported and `sync` exits non-zero if any item failed.

## Out of scope

- No HTTP server / webhooks.
- No `list`/query command (query with `sqlite3` for now).
- No new tables or schema changes — the existing `transactions` table is the target.
- No account-level detail (`accounts` table) beyond the `account_id` column already present.
- No handling of Plaid `ITEM_LOGIN_REQUIRED` re-auth flows (just surface the error; re-link with `finance link`).

## Contract (exact signatures — tests and implementation must both conform)

```go
// sync.go
type SyncTxn struct {
	TransactionID string
	AccountID     string
	Date          string  // YYYY-MM-DD
	Name          string
	MerchantName  string
	Amount        float64 // Plaid dollars; positive = money out
	Category      string
	Pending       bool
}

// Applies one page of Plaid sync results and the item's new cursor in a
// single SQL transaction. Silent; returns error.
func applySyncPage(db *sql.DB, itemID string, added, modified []SyncTxn, removed []string, nextCursor string) error

// db.go
func openDBAt(path string) (*sql.DB, error) // openDB() calls this with ~/.finance/finance.db
```

## Testing plan

`applySyncPage` is the testable core — pure Go + SQLite, no network. Tests (written
first, from this spec, red→green):

1. Added transactions are inserted with correct cents conversion (e.g. `12.34` → `1234`, `-0.01` → `-1`).
2. Modified transaction with same `transaction_id` updates fields instead of duplicating.
3. Removed transaction IDs are deleted; removing a nonexistent ID is not an error.
4. Cursor is persisted on the item in the same call.
5. All-of-the-above in one page applies atomically (spot check: a failing statement rolls back the cursor update — simulate with a duplicate insert or closed DB if practical, else skip and note).
6. Pending flag round-trips (`true` → `1`).

`runSync` (network layer) is exercised by the end-to-end step, not unit tests.

## End-to-end verification (must run before "done")

```sh
go build -o ~/go/bin/finance . && go vet ./... && go test ./...
finance sync         # real Plaid call against the 2 linked items
sqlite3 ~/.finance/finance.db "SELECT COUNT(*) FROM transactions;"          # > 0
sqlite3 ~/.finance/finance.db "SELECT institution FROM items;"              # names, not empty
finance sync         # second run: mostly "0 added" (incremental works)
```

## Checklist

- [x] `openDBAt(path)` refactor in `db.go`
- [x] Failing tests in `sync_test.go` (from spec, before implementation)
- [x] `applySyncPage` in `sync.go` — tests green
- [x] `runSync` in `plaid.go` (pagination, retry-once on mutation error, institution backfill)
- [x] `sync` dispatch in `main.go`
- [x] End-to-end verification run, output captured
