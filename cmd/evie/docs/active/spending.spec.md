# Data Spending: monthly overview and transactions

## Outcome

The owner can open Data > Spending, fetch the latest available transactions
from existing bank connections, and use two separate views: Overview for plots
and Transactions for bank records and their saved classification states. This
extends the initial daily-flow view requested on 2026-09-19 with the owner's
2026-09-20 requests for month navigation, a separate transaction view, and
  Monarch-inspired monthly cash-flow and category charts, then the 2026-09-21
  request for daily transaction popups with editable categories.

## Overview and month navigation

- Display copy is minimal: labels, amounts, controls, selected-day details,
  and concise loading/empty/error states. The owner requested removal of
  subtitles, instructions, explanatory footnotes, and routine sync narration.
- Overview initially opens the current local calendar month. Previous and next
  arrows move one month, including across year boundaries; This month returns
  to the current month. The selected month is shared by both Spending views.
- Daily net flow appears as a seven-column calendar with weekday alignment.
  Only the selected month's recorded dates contribute to its cells and color
  scale. Future amounts do not distort visible colors.
- Provider transaction dates are calendar dates without timezone shifting.
  Stored positive cents mean outflow, so displayed net flow is inflow minus
  outflow. Integer arithmetic and JSON decimal strings preserve exact cents.
- The raw-flow plot includes all posted transactions, including transfers and
  credit-card payments, regardless of classification. Pending transactions
  are excluded. No internal-transfer detection or exclusion is inferred.
- Color separates net inflows from net outflows, with stronger color for larger
  magnitude. Exact values remain readable without color. Date buttons retain
  exact flow details in their accessible label and tooltip; clicking a past or
  current date opens its transaction popup. Focus alone does not open it.
- Missing recorded days use a dash; an actual balanced day shows 0.00. Future
  dates are blank, and impossible dates do not appear. Missing dates are not
  proof of complete bank coverage. Invalid transaction dates are excluded.

## Monthly cash flow and categories

- Overview also shows monthly inflow and outflow bars with a net-flow line,
  followed by selected-month Inflow, Outflow, Net flow, and Net rate totals and
  horizontal category bars. These are raw bank flows, not inferred income,
  expenses, or savings. Net rate is net flow divided by inflow; zero inflow
  has no rate. Exact amounts and shares remain readable without color.
- The timeline starts with twelve months ending at the selected month. Selecting
  a bar or navigating within that range preserves the timeline, including when
  the selection crosses a year. Leaving the range shifts it enough to include
  the new selection. Bar selection also updates the calendar and Transactions.
- An explicit local-calendar `asOfDate` excludes future posted and pending
  transactions from the cash-flow report. The current month's last line segment
  is dotted and its accessible label says to date; no projection is calculated.
  Missing months show a dash and break the line rather than implying measured
  zero flow. A recorded balanced month shows zero. Empty monthly cards show a dash.
- Categories use saved `budget_entries` only. No entries sends the raw bank
  amount to Unclassified. Splits contribute to named categories only when they
  sum exactly to the bank amount and every nonzero allocation agrees with its
  sign. Mismatched, mixed-sign, or blank-label allocations send the whole bank
  amount once to Needs review. Refunds remain inflows in their saved category.
  Each side reconciles to raw totals, including `excluded` allocations.
- Synthetic buckets have distinct identities from named categories; colliding
  named labels receive a `(category)` suffix. These display buckets do not
  create or change classification states. The Transactions view still exposes
  the original saved allocations for inspection.
- Category bars sort by the relevant flow amount and initially show six rows,
  with Show all for the rest. Monthly charts can scroll inside their container
  on narrow screens. Quarterly/yearly modes and other grouping views are deferred.

## Transactions and the classification contract

- Transactions is a separate view, not a list below the plot. It reads the
  selected month's saved bank rows and offers All, Needs classification,
  Classified, and Pending filters, with month-wide counts and bounded pages.
- Each row shows date, merchant and bank description, net amount, saved
  categories, and recorded classification sources. Split entries stay distinct
  allocations under one transaction; they never multiply rows or totals.
- Current `internal/finance/categorize.go`, the finance plugin capabilities,
  and `budget_entries` were checked for this delivery. There is **no persisted
  model-proposed state**. The plugin has sync, rules, and categorize capabilities;
  categorization performs exact case-insensitive merchant-rule matching and
  does not call a model or accept Plaid's reference category as a verdict.
- For posted transactions, any `budget_entries` row means Classified; no entry
  means Needs classification. Pending takes precedence as a separate bank
  settlement state, producing disjoint filter counts. An `excluded` entry is
  still a saved classification; this raw-flow view does not treat it as a
  reason to omit the underlying bank transaction.
- Entries preserve their source. Known `rule` and `human` sources are identified;
  legacy `manual`/`agent` and other sources are shown without implying a model
  proposal or new approval. Existing split amounts are displayed as recorded,
  even if they do not reconcile to the transaction amount.
- The older categorize specification describes legacy `transactions.category`,
  `category_source`, and `reviewed` fields and some unimplemented CLI commands.
  Current Categorize uses budget entries as its fence; legacy labels are only
  backfill inputs and `reviewed` is unused. This UI exposes a legacy label when
  useful, but does not convert it into an entry or a proposal. The newer
  categorize-patterns specification explicitly rejects an autonomous `llm`
  source; its planned name-rule matching is not implemented by this display.
- Opening views, changing filters, or paging never writes classifications,
  runs rules, accepts proposals, or calls a model. Reload saved records reads
  changes made elsewhere. Fetch latest syncs bank records only.

## Daily popup and category edits

- Clicking any non-future calendar day opens a native modal dialog with the
  selected date, its posted transaction count and net flow, and a bounded page
  of transactions. Pending rows are excluded to match the calendar. Each row
  shows merchant/statement description, exact signed amount, and saved categories.
  Empty days are explicit; pagination covers busy days without truncation.
- Category labels open an inline selector with the configured `categories`
  vocabulary and explicit Save/Cancel controls. `excluded` is an ordinary option
  if configured. There is no category creation, clearing, rule creation, or
  propagation to other transactions in this flow.
- Editing an existing allocation changes only its category and source (`human`).
  IDs, signed amounts, tags, other splits, and source fields on unchanged entries
  are preserved. Inconsistent split amounts remain visible and are not silently
  repaired or merged. For a posted transaction without entries, choosing a
  category creates one full-amount human entry. Raw bank fields stay unchanged.
- Each row carries an opaque revision over its raw transaction and complete
  ordered allocation state. The mutation rereads and compares under a SQLite
  writer transaction before modifying data; stale, removed, or pending rows
  conflict rather than overwriting later changes. Category IDs/names must exist.
  The rule categorizer also rechecks eligibility at insertion, so a previously
  collected candidate cannot create a duplicate after an owner classification.
- Successful edits replace the saved row and refresh Overview's category charts
  and the next Transactions read. Other unsaved row edits remain intact. An
  ambiguous save failure offers Reload to check the canonical saved value.
- The dialog traps focus, closes through its close button, Escape, or backdrop,
  and restores focus to the day button. Closing, reloading, paging, and further
  edits are disabled while a save is pending; a dismissed request is not treated
  as proof a write was cancelled. Desktop columns become stacked rows on mobile.

## Refresh, reads, and boundaries

- Reads use the existing canonical finance database without creating it or
  contacting a bank. Missing storage produces an honest disconnected/empty view.
- Fetch latest explicitly runs existing incremental Plaid sync. Added, modified,
  and removed transactions retain the existing atomic page/cursor boundary.
  Saved totals and any visible transaction list reload after refresh, including
  partial failure; partial results never claim all banks have updated.
- Concurrent refreshes in this server are rejected. Agent/web sync calls share
  the process-wide context-aware gate before reading cursors. Refresh is bounded
  to two minutes and observes request cancellation.
- Summary and transaction APIs use existing owner host/origin/JSON guards,
  no-store responses, and generic errors. The monthly query validates YYYY-MM,
  classification filter, non-negative offset, and page size (default 50, max 100).
  Counts, rows, and entries use one read snapshot; ordering is date descending,
  then transaction ID ascending. No arbitrary SQL reaches the endpoint.
- The cash-flow API applies the same guards and safe errors. It validates the
  selected month, history-end month, and exact as-of date, bounds history to
  twelve months (clamped at year 0001), and requires selection within that window.
  History, selected totals, and categories come from one read snapshot. Amounts
  aggregate with arbitrary-precision integer cents and serialize as strings.
- Day inspection and category mutations use these same management guards,
  strict bounded JSON, no-store responses, cancellation, and generic errors.
  Dates and pagination are validated before access. Mutation accepts only a
  transaction ID, optional entry ID, configured category, and revision; callers
  cannot supply amounts, source, tags, SQL, or a database path. Existing-only
  writable access does not create missing finance storage or run schema setup.
- Typed rows may expose their transaction IDs, bank descriptions, and saved
  allocations to the owner; account IDs, item IDs, credentials, and raw
  database/provider errors are excluded.
- Loading, empty, disconnected, stale, and failed states stay explicit. Older
  year/month/filter/page requests cannot overwrite the current selection.

## Existing data limitations and non-goals

The finance schema does not record currency. This delivery leaves currency
unknown and shows no guessed symbol or conversion. Combining different currencies
into meaningful totals needs a separate storage/backfill contract. Existing raw
amounts and current transfer inclusion remain unchanged by this follow-up.

Last successful fetch time is specific to this server lifetime and is not a
claim about a bank's own freshness. Reads or partial failures do not advance it.
Standalone finance CLI processes are outside the in-process sync gate.

Bank linking and cached account inventory are now covered by the owner's
2026-09-21 follow-up in
[Connected accounts and simplified navigation](spending-accounts-and-navigation.spec.md).
That follow-up adds local account/link-session tables on explicit actions. Other
credential changes, model-proposal lifecycle, categorization engine changes,
inferred transfer exclusion, budgets, new production dependencies, and scheduled
sync remain outside this spending-view contract.

## Verification and demonstration

Test calendar alignment, current-month default, month/year navigation, leap days,
month-specific color scaling, exact amounts, and stale request suppression. Test
classification from entries, legacy fields, pending precedence, split preservation,
counts, stable pagination, invalid dates/queries, missing storage, cancellation,
redacted errors, and the protected HTTP boundary. Run focused UI/finance/web tests
and `./scripts/verify-change.sh`. Exercise Overview/Transactions, filters, month
navigation, pagination, reload/refresh, and narrow screens in the production UI
with synthetic API fixtures. Live bank calls are not required for this UI change.

Manual demonstration: open Data > Spending. Use the month arrows in Overview,
or select a monthly bar to update totals, category bars, and the daily calendar.
Use Show all in a category panel, click a daily amount to inspect its transactions,
then select a category label, choose another configured category, and Save.
Close the popup, then switch to Transactions and Needs classification.
Both views retain the selected month. Use This month to return to today’s month,
and Fetch latest to pull the latest available bank updates.

Review entry points: `internal/finance/spending_transactions.go` for saved-state
classification and pagination, `internal/web/spending.go` for the owner HTTP
boundary, and `internal/web/ui/src/data/Spending.tsx` plus
`SpendingTransactions.tsx` for the views and request lifecycle.
Monthly-chart entry points are `internal/finance/spending_cash_flow.go`,
`internal/web/ui/src/data/CashFlow.tsx`, and `cashFlowPresentation.ts`.
Daily inspection and editing enter through `internal/finance/spending_day.go`
and `internal/web/ui/src/data/SpendingDayDialog.tsx`.

## Verification record (2026-09-20, monthly follow-up)

The same checks below passed again after the requested copy cleanup: full
verification, all 16 UI tests, and the production-build browser checks. No
finance behavior changed; only visible copy and toolbar spacing changed.

- `./scripts/verify-change.sh` passed on the final implementation: all Go tests,
  Go vet, UI lint/build, and staged/unstaged whitespace checks. Existing Fast
  Refresh export warnings in `Icon.tsx` and `memory/presentation.tsx` remain,
  along with Vite's existing bundle-size warning.
- From `internal/web/ui`, `npx vitest run src/data/Spending.test.tsx
  src/data/SpendingTransactions.test.tsx src/data/DataHub.test.tsx
  src/api/spending.test.ts` passed: 16 tests across four files.
- `go test ./internal/finance`, `go test -race ./internal/finance`, and
  `go test ./internal/web -run '^TestSpendingHTTP' -count=1` passed.
- `node /tmp/evie-spending-month-browser-20260920/check.cjs` passed against
  the production build with synthetic API responses: current-month default,
  arrows, This month, focused day details, separate/lazy Transactions view,
  classification filters, page navigation, returning to a month resets to
  page one, stale month/filter suppression, refresh/partial errors, and mobile
  page containment at 390 by 844 pixels.
- Browser verification did not contact live banks or change saved financial
  records. Actual provider connectivity/freshness remains unverified; no
  live-bank test was needed for the monthly navigation/read-only list change.
  Unrelated opt-in browser fixtures are skipped by the ordinary Go suite.

## Verification record (2026-09-21, monthly charts)

- `./scripts/verify-change.sh` passed after the final mobile-selection adjustment:
  all Go tests and vet, UI lint/build, and staged/unstaged whitespace checks.
  Existing Fast Refresh export warnings in `src/ui/Icon.tsx:16` and
  `src/memory/presentation.tsx:18,20,24,48`, and the existing Vite bundle-size
  warning remain. Full output: `/tmp/evie-spending-charts-browser-20260921/verify.log`.
- From `internal/web/ui`, `npx vitest run src/data/CashFlow.test.tsx
  src/data/Spending.test.tsx src/data/SpendingTransactions.test.tsx
  src/data/DataHub.test.tsx src/api/spending.test.ts` passed: 25 tests in five files.
- `go test ./internal/finance`, `go test -race ./internal/finance`, and
  `go test ./internal/web -run '^TestSpendingHTTP' -count=1` passed. New cases
  cover exact cents above int64 totals, split/refund reconciliation, synthetic
  bucket collisions, future cutoffs, history bounds, cancellation, and safe errors.
- `node /tmp/evie-spending-charts-browser-20260921/check.cjs` passed against
  the production build using synthetic API fixtures: bar selection and totals,
  category expansion, stable history across year changes, shifting the history
  at its boundary, stale-response suppression and safe retry, explicit refresh,
  separate transaction reads/filters/pagination, and 390-by-844 mobile containment
  with the selected month visible. Desktop and mobile screenshots were inspected.
- Independent focused review found and verified fixes for cross-year remounting
  and category-label collisions; the final review had no actionable findings.
- No live provider calls or saved financial record changes were made during
  verification. Live bank connectivity/freshness and unrelated opt-in browser
  fixtures remain untested; they are not required by this read-only chart change.

## Verification record (2026-09-21, daily popup and editing)

- `./scripts/verify-change.sh` passed on the final code: Go tests and vet,
  UI lint/build, and whitespace checks. The existing Fast Refresh export warnings
  in `Icon.tsx` and `memory/presentation.tsx`, and Vite bundle-size warning remain.
  Full output: `/tmp/evie-day-popup-browser-20260921/verify.log`.
- From `internal/web/ui`, `npx vitest run` passed all 330 tests in 56 files.
- `go test ./internal/finance` and `go test -race ./internal/finance` passed,
  covering exact day totals, pagination, existing-only storage, durable edits,
  split/refund/tag preservation, stale snapshots, concurrent writers, rollback,
  and the categorizer's recheck after human decisions or bank changes.
- `go test ./internal/web -run 'TestSpendingHTTP' -count=1` and
  `go test -race ./internal/web -run 'TestSpendingHTTP' -count=1` passed. Guard,
  strict-body, date/page, revision, entry ownership, cancellation, and safe-error
  cases include proof invalid requests never invoke a finance mutation.
- `node /tmp/evie-day-popup-browser-20260921/check.cjs` passed using the production
  UI and synthetic reads/writes: date popup, exact descriptions/amounts, splits,
  pagination, explicit Save/Cancel, other row draft preservation, chart reload,
  stale edit rejection, pending-save dismissal prevention, lost-response recovery,
  empty dates, stale reads, modal focus/restore, and mobile containment.
  Desktop and 390-by-844 screenshots were inspected.
- Focused review found and verified fixes for category accessible names and
  focus restoration after saving, cancellation, reload, and pagination.
- Browser saves used synthetic fixtures; finance writes used temporary test
  databases. No real transaction categories or bank data were changed for tests.
  Live provider calls and unrelated opt-in Go browser fixtures were not run,
  because this change reads local records and saves owner category decisions.
