# categorize — decisions

Spec: [categorize.spec.md](categorize.spec.md). In flight.

- **Exact match on merchant_name** (David, overriding Claude's
  substring recommendation) — measured on 299 real rows: exact covers
  61% day one vs substring's 76%, but Plaid's merchant_name is stable
  per merchant, sub-brands (`Kroger Fuel`, `Amazon Kindle`) deserve
  separate categories anyway, and review-created rules use Plaid's own
  spelling so coverage converges to the same place. `rules.merchant`
  UNIQUE ⇒ at most one rule per merchant ⇒ the whole precedence /
  longest-wins / tie-break problem is deleted, and the matcher is a
  map lookup. Raw `name` (statement descriptor) is display-only; the
  38 empty-merchant_name rows stay manual forever. Accepted costs.
- **Drop-and-resync over migrations** (David) — a `user_version`
  migration runner was built, then deleted: while the DB holds only
  Plaid-derived data it's cheaper to put the final shape in the
  baseline const, drop the live `transactions` table, reset
  `items.cursor = ''` (else Plaid thinks we're caught up and resync
  returns nothing), and re-pull. Items table untouched, so access
  tokens survive — no relink. Executed 2026-07-03: 299 rows re-synced.
  This was the *last* free nuke: once transactions carry manual
  categories/reviews, that data is unrecoverable from Plaid, and the
  next shape change needs real migrations.
- **`tags` as JSON text column** (`'[]'` default), not a join table —
  one column, queryable via `json_each`; a join table is unearned
  until tag queries get hot.
- **Plaid PFC is reference-only** (David) — never a categorization
  source or fallback; it's not accurate enough to trust. Stored raw for
  inspection only. `category_source` enum is `rule|agent|manual`.
  Coverage gaps are closed by review/set and future helpers
  (recurring detection), not by Plaid.
- **`category` → ours, `plaid_category` → Plaid's raw PFC** — the
  fresh table made the rename decision free; sync's upsert repointed.
  (Immediate vindication of reference-only: Plaid filed a Timescale
  bill under TRAVEL.)
- **Case sensitivity of the exact compare** — David's matcher decision
  (Claude recommends folding both sides with ToLower: still exact,
  forgiving about case drift). TBD.
- **Name rules landed 2026-10-06** (categorize-patterns.spec.md, stages
  1–2): `name_rules(pattern UNIQUE, category, priority)` in the schema,
  `excluded` seeded on every open, matching order legacy column → merchant
  rule → name rule, matched in SQL so the semantics are exactly SQLite's
  `LIKE`. The twenty reviewed patterns are embedded from
  `internal/finance/data/name_rules.json` (not `cmd/finance/data`: the
  seeder runs inside whichever binary is current and `cmd/finance` is
  slated for deletion); `finance rules` and the `finance_rules` tool seed
  them after the merchant file, upsert on pattern. The tool's schema is
  unchanged on purpose so sessions pinned to Finance 1.0.0/1.1.0 still
  resume.
- **A same-priority tie is an error only when the tied rules disagree**
  (Claude, refining the spec's "equal priority + both match → error"). The
  spec's own seed has `%mr cooper%` and `%mtg pyt%` both at priority 10 and
  both Mortgage, and the real string `MR COOPER MTG PYT` matches both; two
  patterns that say the same thing are not a coin flip. Disagreement still
  fails categorize naming the transaction, every tied pattern and the
  priority, and mints nothing. Dry run on a copy of the live database:
  339 → 125 awaiting review, 212 of the 225 empty-merchant rows classified
  (199 `excluded`, 5 Loans, 5 Bank Fees, 3 Mortgage), no ties.
- **Backfill 2026-10-06 (stage 4).** Seeded and categorized against the
  live database: 339 → 125 awaiting review, a second run minted 0.
  `excluded` holds 199 rows: Citi/Amex/Capital One/Chase card payments,
  the matching "PAYMENT, THANK YOU" credits, transfers between *9243/*9278
  and *9251, Zelle, Venmo (48 rows, $5,942.52), payroll from three
  employers, interest payments and one incoming wire. Name rules also
  placed 5 student-loan rows in Loans, 3 mortgage rows in Mortgage and 5
  interest/wire-fee rows in Bank Fees. 13 merchant-less rows remain for
  chat review. The `query_db`/`edit_db` descriptions carry the `excluded`
  filter and the ratchet; the agent projection test was rebuilt without
  Kernel tools (David, 2026-10-06) so tool-description growth no longer
  trips a context-size guard that was measuring projection, not schemas.
- **Payroll is Income, not excluded** (David, 2026-10-07). The August seed
  table put `%payroll%` under `excluded` with the transfers; once the
  Overview stopped showing excluded money, September's inflow fell to
  $472 and net flow read minus $7,651. Transfers cancel out; income does
  not. `%payroll%` now seeds to `Income` at priority 70, and the rows
  already excluded by it were moved to Income in place.
- **Card payments are their own category** (David, 2026-10-07). The six
  card-payment patterns seed to `Credit Card Payments` instead of
  `excluded`, and the rows already excluded by them were moved in place.
  Accepted cost, stated at the time: the Overview counts a card payment as
  outflow on the bank side and the matching "PAYMENT, THANK YOU" credit as
  inflow on the card side, on top of the purchases that were counted when
  the card was swiped. If that double count turns out to matter, the
  Overview can treat this category like `excluded`.
