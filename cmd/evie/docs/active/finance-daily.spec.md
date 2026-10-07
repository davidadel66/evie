# finance-daily — spec

Decided 2026-10-06 with the owner (grilling session; binding record in
[finance-daily.decisions.md](finance-daily.decisions.md), ADR 0067).

## Purpose

Run the owner's daily finance work — sync every linked bank, categorize
the posted transactions that have no budget entry — unattended, on code
that is the code in the repository, with a durable record when a bank
needs attention.

## Why

The daily run was launchd job 3 (`finance sync && finance categorize`), a
cron tool job that shells out to a separately installed `finance` binary.
That binary was built 2026-07-28; the sync fix that deletes budget entries
before a removed transaction (2026-08-09) and the pending fence never
reached it, so the job's own categorize step minted entries on pending
rows, Plaid's retraction of those rows then failed with `FOREIGN KEY
constraint failed (787)`, the bank's cursor could not advance, and the
`&&` skipped categorize on every such day. Two deployments of one package
drifted; the fix is one deployment.

## Outcome

- `evie serve` runs a scheduled Workspace workflow, `finance-daily`, at
  09:00 America/Detroit every day. It syncs, categorizes, and fails the run
  when any bank failed to sync — after categorization has run.
- Finance is the first production Workflow Effect provider, with two
  ledger effects alongside its unchanged chat tools and reads.
- `evie serve` runs as a per-user launchd LaunchAgent with KeepAlive, from
  a checked-in template and install script, so the scheduler is up without
  a terminal open.
- Cron job 3 and `cmd/finance` retire afterwards (see Stages).

## Effects (`internal/plugins/finance_effects.go`)

Capability IDs `finance.sync_ledger` and `finance.categorize_ledger`,
contract `1.0.0`, Finance implementation `1.2.0` (tools unchanged;
`ResumableFrom` continues 1.0.0 and 1.1.0). Both serve connection
`finance-local`, resource `ledger`, are idempotent per key, reconcile,
and use the default Resource Conflict Keys so every effect on the ledger
is serialized.

Input, both: `{day: string}` — the scheduled day, so each day's intent is
its own idempotency key. No behavior depends on it.

`sync_ledger` output: `banks[] {label, added, modified, removed, failed,
error, warnings[]}`, `added`, `modified`, `removed`, `failed_banks`,
`replayed`. A bank that failed is reported, not raised: the workflow
decides what a failed bank means. The error returned by `Perform` is
reserved for "the sync could not run at all" (no credentials, no linked
bank, unreadable items), exactly as `finance.Sync`.

`categorize_ledger` output: `categorized`, `uncategorized`, `replayed`.

### Receipts (`internal/finance/effects.go`)

The finance database gains `effect_receipts(idempotency_key PRIMARY KEY,
effect, output JSON, recorded_at)`. `CategorizeLedger` records its receipt
in the same transaction as the entries it mints. `SyncLedger` records its
receipt after the last bank, because sync commits each bank's pages in
their own transactions. A call with a recorded key returns the recorded
output with `replayed: true` and performs nothing. `Reconcile` answers
`happened` with the recorded output, or `did_not_happen` when no receipt
exists — honest for both: categorize committed nothing without its
receipt, and every sync page that did commit is one the cursor will not
send again, so a re-sent intent continues from it.

## Deterministic function `require_zero@1`

`(count: integer 0..1e9, subject: string ≤200) → {count: 0}`; fails with
`"<count> <subject>"` when the count is not zero. Pure. The reviewed way to
fail a run on something an effect reported, after the steps that should
still happen have happened.

## Definition (`cmd/evie/workflows/finance-daily/`)

`sync` (effect, 3 attempts, 10m backoff) → `categorize` (effect, 3
attempts, 1m backoff) → `check` (`require_zero@1` on
`$steps.sync.output.failed_banks`). Inputs `day`, `at` from
`$schedule.date` / `$schedule.time`; run key `[day, at]`. Schedule
`0 9 * * *` America/Detroit, catch-up `each_missed` with `max_runs: 1`.
Authority: both effects on `finance-local: ledger`, at most 3 calls per
run. Retention `sensitive`. `fixtures.json` simulates a clean day.

## Operations

1. A `Finance` Workspace holds the workflow and its notifications.
2. `evie workspaces access <ws> --set finance-local=ledger`.
3. `evie workflows propose <ws> cmd/evie/workflows/finance-daily --fixtures cmd/evie/workflows/finance-daily/fixtures.json`.
4. The owner approves the proposal (`evie workflows approve ...`). This
   grants the Standing Authority (ADR 0008); nothing else does.
5. `scripts/install-serve-agent.sh` once (a manual `evie serve` must be
   stopped first). Every later change ships with `scripts/release.sh`,
   which verifies, installs the binaries and restarts the agent.
6. After the first clean scheduled run: `cron_remove finance-daily` (job 3).
7. After one clean week and a parity check of Plaid link and rules seeding
   in Evie: delete `cmd/finance`.

Failed runs are visible in `evie workflows status` / `notifications`;
external delivery is a backlog item.

## Non-goals

- No LLM, fuzzy or prefix matching in categorize (categorize.decisions.md).
- No change to the chat tools, reads, or `finance.Sync` semantics.
- No external notification delivery.
- `categorize-patterns.spec.md` (name rules, `excluded`) stays a separate
  story.

## Verification

- `go test ./internal/finance/ ./internal/plugins/ ./internal/workflow/`
  covers receipt replay, reconcile answers, a failed bank in the output,
  credential absence from the response, the manifest, and `require_zero@1`.
- `evie workflows validate cmd/evie/workflows/finance-daily` and
  `evie workflows simulate ... --fixtures` (clean day: succeeded; one failed
  bank: `check` fails with `1 banks failed to sync` after both effects
  previewed).
- `./scripts/verify-change.sh`.
