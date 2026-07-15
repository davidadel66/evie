# categorize — spec

Transaction classification for `finance`. Rules map merchant patterns to
categories; everything else stays uncategorized until reviewed. Plaid's
stored `personal_finance_category` is kept for reference only — it is
inaccurate and is **never** used as a categorization source.

## Current state

- `sync` already persists Plaid's `personal_finance_category.primary`
  (e.g. `FOOD_AND_DRINK`) into `transactions.category`. So "add a category
  column" is really a migration decision about that existing column.
- `data/merchantLookup.json`: 155 clean merchant names → 24 categories.
  Seed data for rules.
- Schema is a `CREATE TABLE IF NOT EXISTS` baseline run on every open;
  there is no migration mechanism yet.

## Schema

No migration machinery (considered, built, then dropped — see
decisions). Everything lives in db.go's `CREATE TABLE IF NOT EXISTS`
baseline; the shape change shipped by dropping the live `transactions`
table, resetting `items.cursor`, and resyncing from Plaid (done
2026-07-03; access tokens survived, no relink needed).

1. **`categories`** — controlled vocabulary. `name TEXT PRIMARY KEY`.
2. **`rules`** — `id INTEGER PRIMARY KEY, merchant TEXT NOT NULL
   UNIQUE, category TEXT NOT NULL REFERENCES categories(name)`.
3. **`transactions`** — `plaid_category TEXT` (raw PFC, reference
   only), `category TEXT` (ours; NULL = uncategorized),
   `category_source TEXT` (`rule` | `agent` | `manual`),
   `reviewed INTEGER NOT NULL DEFAULT 0`, and
   `tags TEXT NOT NULL DEFAULT '[]'` (JSON string array).

## Rule matching (decided)

- **Exact match on `merchant_name`, and only that field** (David) — no
  substring, no glob, no regex. Plaid's merchant_name is stable per
  merchant (measured: Chipotle is always `Chipotle Mexican Grill`,
  Starbucks 10/10 identical), and sub-brands (`Kroger Fuel`, `Amazon
  Kindle`) *should* categorize separately. `rules.merchant` is UNIQUE,
  so at most one rule applies — no precedence problem exists.
- Measured day-one coverage: exact = 61% of 299 real transactions
  (substring would have been 76%). The gap converges through review:
  `set --rule` creates rules from Plaid's own spelling, which then
  match forever. Seed entries whose spelling Plaid never emits sit
  inert.
- The raw `name` (statement descriptor, `AMAZON MKTPL*...`) is
  display-only in `review`. Rows with empty merchant_name (38/299)
  can never match a rule — permanently manual via `set`.
- Case sensitivity of the exact compare: David's matcher decision.

## `finance categorize`

Batch pass over transactions:

1. Skip any row with `reviewed = 1` or `category_source = 'manual'`
   (or `'agent'`) — human/agent verdicts are never overwritten.
2. Apply rules to the rest (source NULL or `rule`) — so re-running
   after adding a rule re-classifies.
3. Rows with no matching rule stay uncategorized. No Plaid fallback —
   the gap is closed by `review`/`set` and future helpers (below).
4. Print a summary: N by rule, N uncategorized. Exit 0.

## `finance review`

Read-only list of rows needing attention (`reviewed = 0`), one per
line via tabwriter: short id, date, merchant (or name), amount,
category, source. `--uncategorized` to filter to source NULL,
`--limit N` (default 50). No prompts — scriptable.

## `finance set <id> <category> [--rule [pattern]]`

- `<id>` may be a unique prefix of a transaction_id (full Plaid ids are
  unwieldy); ambiguous prefix is an error listing matches.
- Sets `category` (creating it in `categories` if new), source
  `manual`, `reviewed = 1`.
- `--rule` also creates a rule so the fix generalizes: pattern defaults
  to the row's `merchant_name`, overridable (`--rule "starbucks"`).
  Without the flag, print a hint suggesting it when the row has a
  merchant no rule matches. No interactive prompt (repo convention).

## `finance rules`

- `rules seed <path>` — import merchantLookup-shaped JSON
  (`{"Merchant": "Category", ...}`): upsert categories + rules,
  idempotent.
- `rules list` — dump rules with ids.
- `rules add <pattern> <category>` — one-off manual rule.

## main.go cleanup (folded in)

Switch dispatch on `os.Args[1]`, usage message on `help`/no args,
non-zero exit (2) on unknown command, dead CSV code deleted
(`LoadTxnsFromCSV`, `Recurring`, the `Transaction` CSV types).

## Division of labor

- **David**: rules DDL + transactions migration decisions; `Rule` type,
  matcher, and precedence (the engine). `TODO(human)` markers in
  `categorize.go`.
- **Claude**: spec/docs, migration runner, main.go cleanup, SQL plumbing
  for categorize/review/set/rules, flag parsing, tests.

## Out of scope (future helpers)

Rules won't cover everything and don't have to. Planned follow-on
helpers close the gap, each its own feature:

- **recurring** — detect consistent repeating expenses (subscriptions,
  car payment, mortgage) by merchant/amount/cadence and bulk-categorize
  them. Successor to the deleted CSV `Recurring()` sketch.
- `agent` source is reserved for LLM-assisted categorization,
  unimplemented.

Also out: regex/glob matching, category renames/merges, amount- or
account-based rules.
