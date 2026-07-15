# sync — decisions

Spec: [sync.spec.md](sync.spec.md). Shipped 2026-07-02.

- **`/transactions/sync` over `/transactions/get`** — cursor-based incremental sync is Plaid's recommended API; the `items.cursor` column is the per-bank bookmark. First sync sends no cursor (field omitted, not `""`).
- **Atomic per page** — each Plaid page (added/modified/removed + `next_cursor`) is applied in one SQL transaction in `applySyncPage`. A crash mid-sync can't desync cursor and data; re-running `sync` resumes from the last committed page.
- **Added + modified → upsert, removed → delete** — handles pending→posted transitions (Plaid removes the pending txn and adds the posted one).
- **Amounts as cents (`int64`)** — converted with `math.Round(v*100)`; naive truncation corrupts values like `1.15` (float64 gives `114`). Plaid's sign convention kept: positive = money out.
- **Cursor UPDATE checks `RowsAffected`** — SQLite silently "succeeds" on 0-row updates; without the check, transactions could commit while the cursor never persists (review finding).
- **FK enforcement via DSN** — modernc.org/sqlite defaults `foreign_keys` OFF; `openDBAt` opens with `?_pragma=foreign_keys(1)` so it applies to every pooled connection.
- **Empty `next_cursor` + `has_more` is an error** — Plaid returns this while the historical pull is still preparing; looping on it would restart pagination from scratch. Surface it and retry later.
- **One bank's failure doesn't abort the rest** — errors are collected (`errors.Join`), per-bank summaries still print, exit is non-zero if any item failed.
- **Institution names backfilled lazily** — on sync, when `items.institution` is empty: `/item/get` → `/institutions/get_by_id`. Non-fatal on failure (item still syncs, labeled by item ID).

Known gaps (accepted for now): `TRANSACTIONS_SYNC_MUTATION_DURING_PAGINATION` retry can double-count in the printed summary (DB stays correct — upserts are idempotent); no timeout on the Plaid context; `plaid.Production` is hardcoded.
