# budget — spec

## Vision

A monthly budget with per-category limits, kept current automatically:
scheduled jobs sync transactions and auto-categorize the easy ones;
anything uncertain accumulates awaiting review. David reviews in evie
chat — the agent pulls the pending pile, proposes categories (including
splits), writes back his decisions via the gated edit_db, and mints new
rules from them. Monthly / on-demand analysis of spend vs budget.

## Data model (decided 2026-07-28)

Two new tables, two jobs. Transactions stay Plaid's raw truth; budget
rows **reference** transactions, never copy them, so re-syncs
(modify/remove) can't strand stale copies.

```sql
-- Where money went: every categorization is an entry. A normal
-- transaction has one row for its full amount; a split bill has
-- several rows summing to the transaction total. Refunds/returns are
-- entries with negative amounts — they net the category's SUM down
-- while staying visible as rows.
CREATE TABLE IF NOT EXISTS budget_entries (
    id             INTEGER PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES transactions(transaction_id),
    category       TEXT NOT NULL REFERENCES categories(name),
    amount_cents   INTEGER NOT NULL,
    source         TEXT NOT NULL DEFAULT 'rule'   -- 'rule' | 'human'
);

-- Where money is allowed to go: month NULL = standing template;
-- month 'YYYY-MM' = override for that month. Override wins.
CREATE TABLE IF NOT EXISTS budget_limits (
    category    TEXT NOT NULL REFERENCES categories(name),
    month       TEXT,
    limit_cents INTEGER NOT NULL,
    UNIQUE(category, month)
);
```

Spend for a category-month = SUM(entries) joined to transactions for
the date. `transactions.category` becomes a legacy/Plaid-hint field
once Categorize writes entries instead.

## Decisions

- **Categories are David's own list** (~10–15); Plaid's category is a
  hint. `categories` table is the source of truth.
- **Splits are just multiple entries** for one transaction (amounts
  sum to the transaction total — advisory, not DB-enforced for now).
- **Refunds net, visibly**: negative entries in the same category
  cancel outflows in the SUM but remain as rows. Income (paychecks) is
  not budgeted.
- **No rollover** — each month stands alone against its limit.
- **Pending counts** — budget reflects committed spending.
- **Limits are template + overrides** (month NULL vs 'YYYY-MM').
- **No new budget tools needed to start**: query_db and edit_db already
  cover reading and gated writing of both tables — the schema blurbs in
  their descriptions are the integration point. Dedicated tools only if
  the flows prove clumsy.
- **Sync/categorize on a schedule** (cron capability, future); review
  is async in evie chat whenever David asks.
- **Reports are HTML**, monthly or on demand — built last.

## Build steps

1. **Schema**: add both tables to db.go's schema const.
2. **Migrate Categorize**: write budget_entries (full amount, source
   'rule') instead of transactions.category; one-time backfill entries
   for already-categorized transactions.
3. **Tool descriptions**: add budget_entries/budget_limits to query_db
   and edit_db schema blurbs, with the netting and split conventions
   spelled out.
4. **Seed limits**: David sets his standing template via chat (edit_db)
   or a small CLI command.
5. **Analysis**: spend vs limits via query_db + model math; dedicated
   report tool only if needed.
6. **HTML report**: separate design pass.
