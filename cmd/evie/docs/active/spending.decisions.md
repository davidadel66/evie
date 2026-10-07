# Data Spending — decisions

Spec: [spending.spec.md](spending.spec.md).

## 2026-10-06 — Transfers leave the overview

Source: the owner, after the name rules of categorize-patterns put every
transfer, card payment and paycheck under `excluded` (199 rows at the time).
"The transfers cancel out, so we can disregard them; I don't even want to
show them in the Data/Finance tab."

- **Chosen: hide `excluded` from the Overview only** (option 1 of three).
  Calendar, monthly bars and net line, the four totals, the category bars
  and the day-dialog summary omit `excluded` allocations; the Transactions
  view gains an Excluded filter and All leaves those rows out, so every bank
  row stays one click away. Rejected: hiding them everywhere (a transfer you
  need to find would mean chat or `query_db`), and a "show transfers" toggle
  that keeps the reconciling view (an extra control for a view the owner
  does not want).
- **What this supersedes.** The 2026-09-20 rule that the raw view reconciles
  to the bank and that an `excluded` entry is never a reason to omit a bank
  row. The reason it existed, proof of complete bank coverage, is weaker than
  the owner's reading of his own numbers: with transfers in, Inflow and
  Outflow are each inflated by the same amount and Net rate is meaningless.
- **Granularity is the allocation, not the transaction.** An `excluded`
  allocation contributes nothing; a split's other allocations still count;
  a transaction with no entries or an unreconciled split counts in full. So
  nothing is hidden that the owner has not classified, and a wrong
  exclusion is fixed by recategorizing the entry.
- **The workflow read follows the Overview.** `finance.month_cash_flow`
  shares the Spending queries; its contract shape is unchanged and its
  figures now omit `excluded`, which is what a workflow asking "how much did
  I spend" means.
