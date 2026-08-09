# categorize-patterns — spec

## Purpose

Make the awaiting-review pile mean "transactions David hasn't decided
about yet." Right now it means that plus ~148 rows that can never be
decided by the current mechanism: credit-card payments, internal
transfers, and paychecks that have no `merchant_name` for a rule to match
and shouldn't be budgeted at all.

Two changes: `rules` gains name-pattern matching (for machine-generated
bank strings), and an `excluded` category records "this deliberately does
not count as spending" as a real decision rather than a silent filter.

Measured state of `~/.finance/finance.db` at spec time (2026-08-07): 656
transactions, 335 with no budget entry. Of the 225 that are posted and
unmatched, **156 have an empty `merchant_name`** — 75 `LOAN_PAYMENTS`, 41
`TRANSFER_OUT`, 19 `INCOME`, 18 `TRANSFER_IN`, 6 `BANK_FEES`. The
remaining 69 span only 54 distinct merchants and are ordinary
review-in-chat work, out of scope here.

## Why patterns and not an LLM (decided 2026-08-07)

David asked whether a cheap model call would beat pattern matching, given
patterns are brittle. The split is by input type:

- These 156 strings are **machine-generated and recurring**.
  `ONLINE PAYMENT, THANK YOU` appears 16 times; `%CITI CARD%` is a card
  payment 100% of the time. An LLM re-deciding the same fixed string every
  month can answer differently across months, which makes month-over-month
  budget comparison quietly lie. Determinism matters more than
  intelligence when the input is a fixed bank string.
- The 54-merchant tail (`TROPICAL TOUC`, `the backyard of`) genuinely
  needs world knowledge — and that path **already exists**: evie chat with
  `query_db` + `edit_db`, as budget.spec.md planned. Getting one of those
  wrong costs one transaction, not a recurring monthly error.

So: no model call inside `Categorize`. When David reviews in chat, the
agent should mint a rule alongside the entry, so each judgment is made
once and becomes deterministic afterward — that ratchet shrinks the pile
permanently instead of re-reasoning every run.

**`budget_entries.source` stays `'rule'|'human'`.** An `'llm'` value was
considered and rejected: David's decisions come through evie chat, so a
model-written entry *is* his decision. Adding a third value would imply an
autonomy that doesn't exist here.

## Schema changes

### `rules` gains a match type

```sql
CREATE TABLE IF NOT EXISTS rules (
    id       INTEGER PRIMARY KEY,
    merchant TEXT NOT NULL UNIQUE,          -- unchanged: exact merchant_name, lowercased at match time
    category TEXT NOT NULL REFERENCES categories(name)
);

-- NEW: name patterns, checked only when no merchant rule matched.
CREATE TABLE IF NOT EXISTS name_rules (
    id       INTEGER PRIMARY KEY,
    pattern  TEXT NOT NULL UNIQUE,          -- SQL LIKE pattern, matched case-insensitively against transactions.name
    category TEXT NOT NULL REFERENCES categories(name),
    priority INTEGER NOT NULL DEFAULT 100   -- lower wins; ties are an error, see below
);
```

A separate table rather than a `kind` column on `rules`: `rules.merchant`
is `UNIQUE` and semantically a merchant name, and patterns need `priority`
which merchant rules don't. Adding nullable columns to express "this row
is a different kind of thing" is the shape that rots.

**Why `LIKE` and not regex.** modernc.org/sqlite has no `REGEXP` by
default, and every pattern this feature needs is a substring test. `LIKE`
with `%` is already case-insensitive for ASCII in SQLite.

### `excluded` is a real category

Insert `excluded` into `categories` as part of the schema/seed step. A
transaction that shouldn't be budgeted gets a normal `budget_entries` row
with `category = 'excluded'` and its full `amount_cents`.

Why an entry rather than a filter or a flag column:

- It leaves the review pile through the existing mechanism ("awaiting
  review = no budget_entries row"), so nothing else has to learn a new
  state.
- The decision is queryable and reversible — `DELETE` the entry and the
  transaction is back for review. A `WHERE plaid_category NOT IN (...)`
  filter in `Categorize` would hide rows with no record that a choice was
  made, and would silently change meaning whenever Plaid recategorizes.
- It keeps "I decided this doesn't count" distinct from "I haven't looked
  at this yet," which is the whole point of the pile.

**Every consumer of spend must exclude it.** Spend for a category-month is
already `SUM(budget_entries.amount_cents)` filtered by category, so
`excluded` only leaks into queries that sum across *all* categories. The
`query_db` schema blurb must say so explicitly (see Tool descriptions).

## Matching order in `Categorize`

Per candidate transaction, first match wins:

1. **Legacy `transactions.category` set** — unchanged current behavior:
   the entry comes from it, preserving `category_source`. (Currently 0
   rows in production, but the backfill path stays.)
2. **Merchant rule** — exact, case-insensitive `merchant_name` against
   `rules`. Unchanged.
3. **Name rule** — lowest `priority` whose `pattern` LIKE-matches
   `transactions.name`, case-insensitively.
4. **No match** — stays entry-less. Awaiting review, as today.

Merchant rules beat name rules because a resolved `merchant_name` is
Plaid's own normalization and is more specific than a substring of the raw
bank string.

**Ambiguity is an error, not a coin flip.** If two name rules with the
*same* `priority` both match one transaction, `Categorize` must fail with
an error naming the transaction, both patterns, and the priority — not
pick one. Silently choosing between two plausible categories for money is
the failure mode this whole spec exists to avoid. Different priorities are
not ambiguous: lower wins, by design.

The pending fence (`WHERE t.pending = 0`) stays exactly as it is.

## Seed rules

These come from grouping the real 156 rows; the classification was
reviewed with David transaction-by-transaction. `priority` is set so
specific debt patterns are checked before the broad payment/transfer
sweeps.

| priority | pattern | category |
|---|---|---|
| 10 | `%student ln%` | Loans |
| 10 | `%kmfusa%` | Car Lease |
| 10 | `%mr cooper%` | Mortgage |
| 10 | `%mtg pyt%` | Mortgage |
| 20 | `%interest charge%` | Bank Fees |
| 20 | `%wire fee%` | Bank Fees |
| 20 | `%limit fee%` | Bank Fees |
| 50 | `%citi card%` | excluded |
| 50 | `%amex epayment%` | excluded |
| 50 | `%crcardpmt%` | excluded |
| 50 | `%chase credit crd%` | excluded |
| 50 | `%payment, thank you%` | excluded |
| 50 | `%mobile payment%` | excluded |
| 60 | `%online transfer%` | excluded |
| 60 | `%funds transfer%` | excluded |
| 60 | `venmo` | excluded |
| 60 | `zel %` | excluded |
| 60 | `%domestic incoming wire%` | excluded |
| 70 | `%payroll%` | excluded |
| 70 | `%interest payment%` | excluded |

Notes on the judgment calls, all David's:

- **`Loans` is the home-equity/student-loan category**, matching his
  existing hand-entries. `HONOR CU - STADIUM DR` is a home equity loan
  payment — but it HAS a `merchant_name` ("Honor Credit Union - Stadium
  Drive", 3 rows, $525.69), so it is a **merchant rule**, not a pattern:
  `INSERT INTO rules (merchant, category) VALUES ('Honor Credit Union - Stadium Drive', 'Loans')`.
- **The car loan is a lease** → `Car Lease`, which already exists and he
  has already used for KMFUSA.
- **Card payments and transfers are excluded, but mortgage/lease/student
  loans are kept.** Plaid's `LOAN_PAYMENTS` conflates them: of 80 such
  rows, 55 are credit-card payments ($3,487.89) and 14 are internal
  transfers ($31,075.00) — only 11 are real debt service ($7,490). This is
  exactly why the split comes from `name`, not `plaid_category`.
- **Card payments double-count**, which is the deeper reason to exclude
  them: `*6229 PAYMENT CITI CARD` (+$1,000) and `ONLINE PAYMENT, THANK
  YOU` (−$1,000) are the two sides of one payment, and the spending
  already happened when the card was used.
- **`zel %` is prefix-anchored on purpose** — `ZEL TO LISA WATTERS`,
  `ZEL FROM ...`. A bare `%zel%` would match merchant names containing
  "zel" (Zelda, Hazel).
- Interest charges go to `Bank Fees` per David's existing entry, not a new
  category.

Seeding mechanism: extend `RulesSeed` to also load name rules, or add a
sibling `NameRulesSeed`. Implementer's call — but the seed data must live
in a file under `cmd/finance/data/` (checked in, since it encodes
reviewed decisions), not inline in Go, matching how `merchantLookup.json`
already works.

## Backfill

After the rules exist, `finance categorize` classifies the 156 rows on its
next run — no bespoke migration needed, because "no entry yet" is already
the candidate filter and these transactions have no entries.

One cleanup that IS needed: **8 budget entries currently sit on pending
transactions**, minted before the pending fence landed. 7 are
`source='human'`. When Plaid posts those legs and reports the pending IDs
as removed, `applySyncPage` now deletes those entries (correctly — see
`sync.decisions.md`). Three of David's decisions won't regenerate
identically:

- Walgreens $3.69 → he chose `Groceries`, but his rule says
  Walgreens → `Health`. Re-categorization will pick Health.
- `HONOR CU` → `Loans` and `MI608 - BRICK & BRIN` → `Dining`: the Honor CU
  merchant rule above fixes the first; BRICK & BRIN needs a rule or
  re-review.
- Three Lowe's rows are a split (+179.14, −136.74, +138.84) that a rule
  can only replace with one full-amount entry.

Backed up at spec time to `/tmp/pending-entries-backup.tsv` (ephemeral —
re-query before relying on it). The implementer should NOT try to preserve
these automatically; surface them to David and let him re-decide. That is
a data decision, not a code path.

## Tool descriptions (`internal/tools/db.go`)

The schema blurbs are the model's only view of this, so they must change
with it:

- `query_db`: add `name_rules(id, pattern, category, priority)` to the
  finance schema list. Amend the `budget_entries` line to name the
  `excluded` category. Amend the "Notes" paragraph: spend queries that sum
  across all categories MUST filter out `category = 'excluded'`, and
  awaiting-review still means no `budget_entries` row.
- `edit_db`: under "Common uses", add minting a name rule, and excluding a
  transaction (`INSERT INTO budget_entries ... category='excluded',
  source='human'`). Also state the ratchet: when David decides an
  uncategorized merchant in chat, insert the entry AND a matching rule, so
  the same merchant never needs deciding twice.

## Out of scope

- **No LLM in `Categorize`** — decided above.
- **No fuzzy/prefix matching on `merchant_name`.** 54 merchants over 3
  months is about one new rule a day; fuzzy matching on money buys a wrong
  category you don't notice. Exact match on Plaid's normalized name stays.
- **No regex patterns** — `LIKE` covers every case here.
- **No `rules` → `name_rules` unification**, and no reworking splits,
  `budget_limits`, tags, or reports.
- **No new CLI subcommand.** `finance categorize` picks the new rules up
  for free.
- **No auto-exclusion by `plaid_category`.** Every exclusion is a rule
  David can read and change.

## Testing

`internal/finance/categorize_test.go` exists (added 2026-08-07 with the
pending fence) — extend it. `newTestDB` + `insertItem` + `txn` +
`applySyncPage` are the working fixture pattern; `insertRule` and
`entryCount` are already there.

- Name rule matches a transaction with an empty `merchant_name` and mints
  an entry in the rule's category.
- **Merchant rule beats name rule** when both match one transaction.
- **Legacy `transactions.category` beats both** (existing behavior, now
  with a name rule present to prove precedence).
- **Priority ordering**: two matching patterns at priorities 10 and 50 →
  the 10 wins.
- **Equal priority + both match → error**, and the message names the
  transaction and both patterns. (Assert the error, not a category.)
- `excluded` behaves like any category: entry created, transaction leaves
  the awaiting-review set (`NOT EXISTS budget_entries` no longer holds).
- Case-insensitivity: pattern `%citi card%` matches
  `*6229 PAYMENT CITI CARD ONLINE ACH WEB`.
- `zel %` matches `ZEL TO LISA WATTERS` and does NOT match a merchant
  named `Hazel Cafe`.
- Pending transactions are still skipped even when a name rule matches
  (the fence must not regress).
- Idempotency holds with name rules present: two `Categorize` runs, one
  entry.
- A name rule whose category is absent from `categories` — assert
  whichever behavior the implementer chooses (`INSERT OR IGNORE` as the
  merchant path does, or a clear error), but pin it.

## End-to-end verification

1. `go vet ./... && go test ./internal/... ./cmd/...`
2. `go build -o ~/go/bin/finance ./cmd/finance`
3. Against the real db, BEFORE seeding: record
   `SELECT COUNT(*) FROM transactions t WHERE NOT EXISTS (SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id)`
   — expect 335.
4. Seed the name rules and the Honor CU merchant rule.
5. `finance categorize`. Expect the awaiting-review count to drop by
   roughly 156 + 99 (the pre-existing merchant matches) and no errors.
6. Spot-check by category: `Mortgage` gains 3 rows (~$5,854.20),
   `Car Lease` gains 2 (~$1,154.19 total across 3 incl. the existing
   entry), `Loans` gains the student-loan and Honor CU rows, `excluded`
   holds the card payments, transfers, and payroll.
7. Confirm nothing landed in `excluded` that shouldn't have:
   `SELECT t.name, e.amount_cents FROM budget_entries e JOIN transactions t USING (transaction_id) WHERE e.category = 'excluded' ORDER BY t.name` —
   read it and check every distinct name is genuinely a payment,
   transfer, or paycheck.
8. Confirm August is clean: of the 11 August transactions, the 5 posted
   ones are categorized or deliberately in review, and none are
   `excluded` by accident.
9. Re-run `finance categorize` — second run reports 0 newly categorized
   (idempotent).
