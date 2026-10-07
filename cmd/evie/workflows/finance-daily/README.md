# Finance daily

A Workspace custom Workflow Definition (2026-10-06) that does the owner's
daily finance run unattended inside `evie serve`, replacing the launchd
cron job that shelled out to a separately installed `finance` binary.

## What it does

1. **Sync** (effect `finance.sync_ledger`): pulls every linked bank's new,
   changed and removed transactions from each bank's committed Plaid
   cursor. One bank failing is reported in the output, not as a failure,
   so the next step still runs. Three attempts, ten minutes apart.
2. **Categorize** (effect `finance.categorize_ledger`): mints a budget
   entry for every posted transaction that a merchant rule matches.
   Pending transactions are never categorized.
3. **Check** (`require_zero@1`): fails the run when any bank failed to
   sync. Failing after categorization means a wedged bank costs that bank's
   day, never the whole day's categorization, and the failed run records a
   durable notification.

## Schedule

Every day at 09:00 America/Detroit. If Evie was not running at 09:00, the
latest missed slot runs once when it is back (`each_missed`, one run):
sync is cursor-based, so one run covers any number of missed days.

## Authority

Both effects on the `finance-local` connection's `ledger` resource, at
most three calls each per run (one per attempt). Propose it into a
Workspace that has been granted `finance-local=ledger`.

## Outputs

The day, the sync totals, the number of failed banks, and the categorize
counts.
