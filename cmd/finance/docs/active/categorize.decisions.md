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
