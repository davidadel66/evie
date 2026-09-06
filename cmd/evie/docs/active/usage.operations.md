# Usage operation and handoff

Implements [usage.spec.md](usage.spec.md) and
[usage.decisions.md](usage.decisions.md), approved on 2026-09-06.

## Open and inspect

Open the local web app, choose Data, then Usage. The initial view selects Evie
and the last 30 calendar days. Select an account or local source for its daily
chart, exact Daily values table, reporting coverage, models or allowances.
From/Through are inclusive in the UI; Apply submits an exclusive end date.
Refresh collects current metadata. Collection refreshes automatically while
the pane is open. The sidebar brand is `evie.` with a teal period.

The default Codex home is CODEX_HOME, falling back to ~/.codex. Existing
independent logins can be configured with EVIE_CODEX_HOMES, an OS path list of
up to four absolute home directories (colon-separated on macOS/Linux). This
does not create or switch logins. EVIE_CODEX_BIN overrides executable discovery;
otherwise the macOS desktop bundle is preferred, then Codex on PATH.

Account snapshots refresh at most once per minute per home. Local collection
uses bounded background scans, continues while the pane polls, and refreshes
completed indexes at most every 30 seconds. The first scan can take several
minutes for large histories. Indexing/partial status and collection timestamps
show the available coverage. The in-memory metadata index rebuilds after a
server restart or detected source removal/replacement/truncation.

## Coverage limits

- Account totals retain provider calendar dates. Missing dates are unknown.
  Account usage has no input/cache breakdown; allowance windows are independent.
- Local detail uses available modern per-response records, deduplicated across
  copied histories. Historical account/model attribution, legacy cumulative
  snapshots and deleted files are excluded. Local and account totals overlap.
- Evie includes accepted conversation responses across sessions, including
  tool-call iterations. Compaction, extraction and unrecorded failures are
  excluded. Models are requested model identities, not billing attribution.
- Missing counters remain unavailable. Each counter reports its call coverage.
  Cache share is weighted over calls with compatible input and cached counts.
  Prices, bills, costs and cache savings in dollars are not inferred.
- Requests are limited to 90 calendar days. A historical timezone boundary
  that normalizes into another date is rejected instead of silently shifted.
  At most 200,000 local response identities and 10,000 local files are indexed;
  reaching a limit keeps an explicit partial-coverage warning.

The account collector uses only read methods and disables app/MCP integrations.
The HTTP response includes token metadata, account labels and status, never
conversation content, credentials or raw subprocess errors. No new production
dependencies, schema changes or conversation-capture changes were introduced.

## Verification and review

Commands used from the repository root unless otherwise specified:

```sh
go test -race ./internal/usage ./internal/web -run 'Test(Service|CodexAccount|LocalIndex|Aggregate|Period|UsageHTTP)' -count=1
go test ./internal/eviedb -run TestConversationUsage -count=1
cd internal/web/ui
npx --no-install vitest run src/data/Usage.test.tsx src/api/usage.test.ts src/data/DataHub.test.tsx
cd ../../..
./scripts/verify-change.sh
```

The focused race suite, storage tests and all nine focused UI tests passed.
The full repository script passed, including Go tests/vet, UI lint/build and
whitespace checks. Existing warnings remain: five fast-refresh export warnings
in memory/presentation.tsx and ui/Icon.tsx, and Vite's large bundle warning.
No required check was skipped. Standards and specification reviews found
collection-retention, limit-checkpoint, historical timezone, duplicate-account,
subprocess cancellation and chart-coverage/accessibility issues; all were fixed
and their focused rechecks were clean.

The updated local binary was installed and the idle server restarted on port
6687. Browser verification confirmed Data > Usage navigation, the wordmark,
source selection, account allowances, date filtering to an empty Evie day,
restoring the default range, and the expanded exact daily table. The live API
returned HTTP 200 with no-store and 83,239 reported Evie tokens from ten recorded
responses in the checked September range. The temporary read-only demo server
was stopped and its task-owned source removed.

Review entry points: internal/usage/usage.go (accounting), service.go (collection
lifecycle), local.go and codex.go (sources), internal/eviedb/usage.go (content-free
storage query), internal/web/usage.go (guarded API), and
internal/web/ui/src/data/Usage.tsx (display).
