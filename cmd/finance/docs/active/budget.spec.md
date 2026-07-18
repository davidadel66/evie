# budget — spec

## Vision

A monthly budget with per-category limits, kept current automatically:
scheduled jobs sync transactions and auto-categorize the easy ones via
rules; anything uncertain accumulates in the database in an
awaiting-review state. When David is ready, he reviews in moussa chat —
the agent pulls the pending pile, proposes categories, writes back his
decisions, and mints new rules from them so the pile shrinks over time.
Monthly / on-demand analysis of spend vs budget.

## Decisions (2026-07-17)

- **Categories are David's own list** (~10–15). Plaid's category is a
  hint only; every budgeted transaction maps to David's taxonomy. The
  existing `categories` table is the source of truth.
- **Budgets are template + overrides.** A standing set of limits applies
  to every month; a specific month can override specific categories.
- **Sync/categorize run on a schedule, not in conversation.** moussa
  gains a general cron capability (useful beyond finance): a tool that
  schedules shell commands. The scheduled job is just the `finance` CLI
  (`finance sync && finance categorize`) — no agent in the loop.
- **Review is async and on demand, in moussa chat.** Unmatched
  transactions simply sit in the db (`reviewed = 0`, no category — the
  state already exists); a review session pulls them when David asks.
- **Reports are HTML** (charts/visual), generated monthly or on demand.
  Built last; needs a render-and-open mechanism — design TBD.
- Data lives in the existing SQLite db (`~/.finance/finance.db`) — no
  new storage.

## Schema sketch

```sql
-- month NULL = the standing template; month 'YYYY-MM' = override row.
-- Resolution: override wins over template for that month+category.
CREATE TABLE IF NOT EXISTS budgets (
    category    TEXT NOT NULL REFERENCES categories(name),
    month       TEXT,              -- NULL or 'YYYY-MM'
    limit_cents INTEGER NOT NULL,
    UNIQUE(category, month)
);
```

`transactions` already carries what the review loop needs: `category`,
`category_source` ('rule' | future 'human'), `reviewed`.

## Steps (each independently useful)

1. **finance_query** — read path for the agent (filters: month,
   category, awaiting-review, merchant). Prerequisite for everything
   below.
2. **Budgets** — `budgets` table + domain funcs + `finance_budget_set`
   / `finance_budget_get` tools (template + override resolution).
3. **Review write-path** — tool to categorize one transaction by id
   (set category, `category_source='human'`, `reviewed=1`), with an
   optional "make this a rule" flag. Enables the on-demand review
   session.
4. **Cron capability** — general moussa tool to schedule/list/remove
   recurring shell commands (crontab-backed). First customer: daily
   `finance sync && finance categorize`.
5. **Analysis** — spend vs budget per category/month. Start as
   finance_query + model arithmetic; dedicated report tool only if that
   proves sloppy.
6. **HTML report** — visual monthly report, on demand. Separate design
   pass (rendering + how it opens).
