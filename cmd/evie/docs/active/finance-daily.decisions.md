# finance-daily — decisions

Spec: [finance-daily.spec.md](finance-daily.spec.md). Decided 2026-10-06
in a grilling session with the owner; every item below is his answer.

- **Scheduled workflow over the cron job** (ADR 0067). The outage was two
  deployments of one package drifting apart; the cron tool's own reason
  for launchd ("must run at 9am with no terminal open",
  cron.spec.md) is met instead by running `evie serve` as a KeepAlive
  LaunchAgent. Considered and rejected: pointing the cron job at the
  `evie` binary (one deployment, but no approval, catch-up or
  notifications), and merely reinstalling `finance` (the drift returns).
- **Bridge first.** Before the build: reinstall the `finance` binary
  (old one kept as `~/go/bin/finance.bak-20260728`), change job 3 to
  `finance sync; rc=$?; finance categorize || rc=1; exit $rc` so
  categorize always runs and a failed bank still exits non-zero, one
  demonstrated run (run 42: all banks synced, 384 → 377 awaiting
  review). The job is edited in place by id so launchd and the run history
  are untouched; it is removed after the first clean scheduled run.
- **Parallel effect IDs, chat tools kept.** One Capability is contributed
  through one role, and `finance.sync` / `finance.categorize` are tools in
  the `standard` preset, so the effects are `finance.sync_ledger` and
  `finance.categorize_ledger` with a new contract and Finance 1.2.0.
- **One sync effect for all banks**, per-bank outcomes in the output. A
  new bank joins the daily run with no definition change; map steps cannot
  call capabilities anyway.
- **A failed bank fails the run after categorize**, through
  `require_zero@1`, so the day's categorization never depends on every
  bank and the failure is a durable `failed` notification.
- **Idempotency by receipt.** The decision record that shipped the effect
  protocol asked that the first real provider keep a per-key record; it
  does (`effect_receipts`). Reconcile replays nothing: it reads the
  receipt. Default conflict keys serialize the ledger.
- **Keep serve alive with launchd, catch up at most one slot.** Sync is
  cursor-based, so one catch-up run covers any number of missed days.
  Installed from a checked-in template and script (`scripts/`), not an
  `evie serve --install` subcommand: a one-time setup earns no Kernel code.
- **A dedicated `Finance` Workspace** owns the workflow, its access grant
  and its notifications, as a Feature Plugin owns its domain.
- **09:00 America/Detroit**, as before.
- **Workflow notifications are the failure signal for now**; external
  delivery is backlog.
- **`cmd/finance` is deleted** after one clean week and a parity check
  (Plaid link and rules seeding already exist in Evie).
- **Near-miss merchants stay exact** (seeded 2026-10-06: 38 rows, 377 → 339 awaiting review; Kroger Fuel → Groceries and WMU West Hills, a gym, → Health). The 38 posted transactions whose
  Plaid merchant name extends an existing rule get new exact rules in
  Plaid's spelling, reviewed by the owner, not prefix matching
  (categorize.decisions.md). Dead rules stay.
- **Glossary.** Budget Entry, Awaiting Review (posted only) and Pending
  Fence were added to CONTEXT.md.
- **Cutover 2026-10-07.** The first scheduled run (09:00 America/Detroit,
  run 9cbf8363) succeeded on the launchd-managed serve: three banks synced,
  six transactions categorized, check passed, two effect receipts
  recorded. Cron job 3 was then unloaded from launchd, its plist deleted
  and its job row removed, keeping its run history. `cmd/finance` stays
  until a clean week and the parity check.
