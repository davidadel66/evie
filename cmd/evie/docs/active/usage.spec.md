# Data Usage

Status: approved by David on 2026-09-06, following the Usage proposal and preview.

## Outcome

The owner can open Data > Usage to inspect real Codex account activity and
Evie's recorded conversation token usage, with explicit coverage and no
invented account attribution. The sidebar wordmark is lowercase `evie.` with a
teal period, matching the approved preview.

## First delivery

- Usage joins Database and Memory behind a typed, guarded owner read API.
- A date range (inclusive from date, exclusive to date, at most 90 days) and
  IANA timezone apply to dated request observations. Default is the last 30
  calendar days including today. Account daily dates retain provider boundaries.
- Show each available Codex account's daily account tokens and current quota
  windows. Use the installed local app-server read methods with no login,
  logout, account switch, reset consumption, or external tool execution.
- Support explicitly configured Codex homes for separate existing logins;
  default to CODEX_HOME or ~/.codex. No credential is returned to the browser.
- Detailed local Codex usage is a separate unassigned source where history has
  no account identity. Deduplicate per-response records across archive moves
  and inherited histories. Legacy cumulative usage is not naively summed.
- Evie reads existing accepted assistant events, including tool-call iterations.
  Its coverage explicitly excludes compaction, extraction and unrecorded failures.
- Source rows show reported input/output/total, cached input and weighted cached
  percentage. Details show daily tokens, reasoning/cache writes when reported,
  requested model where attributable, coverage and collection freshness.
- Missing remains unavailable and reported zero remains zero. Each counter
  carries reporting coverage. Cached input and reasoning are subsets, never
  additional consumption. Percentages use the same compatible measured cohort.
- Account daily totals, local request measurements and allowance windows are
  independent views and are never added together. Task lifetime estimates may
  not be represented as daily/date-filtered cache data.
- Errors are per source; Codex unavailable must not hide Evie. Bound subprocess
  duration, file reading, concurrency and response size; partial coverage must
  be visible. The UI must not let an older request replace a newer date query.
- No operational metric changes conversation continuation, semantic evidence,
  authorization, approvals or runtime budgets.

## Non-goals and dependencies

No OpenAI API organization integration, billing reconciliation, pricing lookup,
alerts, multi-device synchronization, credential management, new production
dependency, or complete runtime-attempt ledger. Compaction/extraction/failure
capture remains a separately reviewable story. Account-provider availability
and local file retention constrain historical coverage.

## Verification

Verify public usage aggregation and storage reads for missing versus zero,
weighted coverage, subset and duplicate accounting, time boundaries, model
provenance, numeric overflow and content exclusion. Test the protected HTTP
contract, source failures, Codex subprocess/parser boundaries, and UI rendering
of real/empty/partial states. Run focused Vitest explicitly (the verification
script does not run it), then ./scripts/verify-change.sh. Demonstrate Usage
source selection/date filtering and the wordmark in the browser.
