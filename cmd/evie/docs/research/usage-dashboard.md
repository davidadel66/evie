# Data > Usage: collection and display proposal

Research date: 2026-09-05. This note answers David's request to compare his
OpenAI/Codex usage and Evie's usage. It is a researched proposal, not a binding
specification or a production implementation. The working assumption is Codex
used through ChatGPT subscriptions; OpenAI API usage is an optional separate
source if that is also intended.

## Recommended experience

Add Usage beside Database and Memory inside Data. Make the source/account table
the first view, with a selected row opening a daily breakdown and collection
details below it. Preserve Evie's compact typography, teal accent, and tables.

- One row for each connected Codex account, plus Evie. Historical activity with
  unknown account attribution gets an explicit Unassigned row.
- Primary columns: input, cached input, output, cached input percentage, and
  coverage. A date range applies consistently to all historical measurements.
- Selected source: daily stacked cached input / other input / output; model and
  task breakdown when supported; reasoning and cache-write counts; measured
  calls; collection scope, last successful collection, and missing data.
- Codex detail also shows account daily token activity and current quota windows
  with reset times. Account activity, task observations, and allowance snapshots
  retain separate labels because their definitions and coverage differ.
- Show actual provider charges only when recorded; label server cost estimates
  as estimates. Subscription token usage does not imply a per-token invoice.
- Use an owner-level operational view. The currently selected conversation
  context must not silently filter account totals. Project/workspace filters
  are explicit and available only for attributable observations.

The conversation preview uses illustrative figures and two example accounts;
it demonstrates row selection, changing token charts, and source-specific
coverage. It is not a report of the owner's actual accounts.

Existing integration points:
[DataHub.tsx](../../../../internal/web/ui/src/data/DataHub.tsx),
[App.tsx](../../../../internal/web/ui/src/App.tsx), and
[theme.css](../../../../internal/web/ui/src/theme.css).
The [Data decision](../active/serve.decisions.md) requires each new source to
have a typed read contract before production chrome is exposed.

## Collection paths

| Source | Preferred measurement | Important boundary |
| --- | --- | --- |
| Codex account | `account/usage/read` summary and daily buckets | Null is unavailable; the documented account response lacks cache detail. |
| Codex task detail | Feature-detect installed `account/usage/read` with `threadId` | Installed experimental schema includes cached/input/output model groups and estimates; runtime availability is untested. |
| Codex local fallback | Structured per-response usage records; legacy snapshots only when needed | Local coverage, duplicate/inherited history, and unknown historical account attribution need explicit handling. |
| Codex allowance | `account/rateLimits/read` | Percent used and reset time are current quota measurements, not historical token counts. |
| Evie conversation | Aggregate existing accepted assistant usage from SQLite | Does not include all failed calls, compaction, or extraction. |
| OpenAI API, if wanted | Organization Usage and Costs endpoints, using an authorized admin credential | API organizations are separate from ChatGPT subscription usage. |

Codex supports account daily token activity and quota reads in its
[official App Server documentation](https://learn.chatgpt.com/docs/app-server).
The installed protocol and local-history evidence, including exact limitations,
are recorded in [Codex usage data sources](codex-usage-data-sources.md). Prefer a
supported query over importing local history when the runtime spike establishes
that it provides the required detail. The installed per-task request accepts
only a task ID, and its returned groups have no timestamps. Keep those
estimates task/lifetime-scoped; they cannot populate date-filtered cache totals
or daily cache charts without additional dated evidence. Daily token splits
require timestamped attributable request observations. Otherwise show the
account total-only daily series and mark the date-range cache split unavailable.
A separate app-server does not
necessarily receive another running desktop process's live task notifications.

If API usage is included, the
[organization completions usage endpoint](https://developers.openai.com/api/reference/ruby/resources/admin/subresources/organization/subresources/usage/methods/completions)
supports time buckets and grouping by model/project/user/API-key identifiers.
The returned token fields include cached input. Follow pagination and replace
re-fetched bucket observations instead of adding each poll again. Use the
[Costs endpoint](https://platform.openai.com/docs/api-reference/usage/costs)
for recorded organization spend rather than treating token-based estimates as
invoice reconciliation. Keep admin credentials server-side and out of Data
responses.

OpenRouter supplies native token counts, cache read/write counts, reasoning,
and cost in its [usage response](https://openrouter.ai/docs/cookbook/administration/usage-accounting).
Its [cache documentation](https://openrouter.ai/docs/guides/best-practices/prompt-caching)
explains why paid cache writes and discounted cache reads need separate fields.
Evie already captures token counters, but deliberately excludes cost and
provider/model identity from that record.

## What Evie has today

[OpenRouter normalization](../../../../internal/openrouter/usage.go) preserves
six optional counters: input, output, total, cached input, cache-write input,
and reasoning output. Missing values remain absent; reported zero remains
present. Each successful assistant iteration, including an iteration that
calls tools, persists usage with the accepted assistant event in
[the conversation loop](../../../../internal/agent/turn.go).

A read-only snapshot of `/Users/davidboktor/.evie/evie.db` found 10 accepted
assistant iterations spanning 2026-09-05 18:05:13–19:48:45 UTC. All ten had all
six counters: 78,705 input, 45,385 cached input, 4,534 output, 2,633 reasoning
output, 83,239 provider total, and zero cache-write input. Weighted cached input
percentage was 57.66%. These are recorded conversation measurements only.

Usage is lost for some failures before assistant persistence. Compaction
responses are not recorded as usage. The Ollama extractor currently ignores
its token counters. Consequently, an initial Evie row must say Conversation
usage and expose these omissions; it cannot claim complete runtime consumption.

Newer context snapshots retain the requested model. A validated join to the
same-session, same-parent preceding snapshot could identify requested model;
it cannot identify the actual routed/billed model. Older records without that
provenance remain unknown.

## Accounting contract

1. Preserve each observation's origin, source/account binding, device scope,
   time or time interval, supported dimensions, and collection timestamp.
   Never attach the current account or model to unattributable historical data.
2. Account totals, server task estimates, local request counters, and Evie
   counters can overlap. Never sum alternative observations of the same work.
   Count each underlying response once across parent and child tasks: establish
   whether parent totals include descendants before combining groups, and
   exclude inherited/copied records. A grand total is deferred until the
   aggregation can prove disjoint scope.
3. Cache reads are an input subset; reasoning is an output subset. Calculate
   input plus output without adding those details again. Retain provider total
   separately if supplied and flag inconsistencies rather than forcing equality.
4. Calculate cached percentage from sum(cached input) / sum(input) over the same
   compatible rows with both counters present and valid. Report the eligible
   input coverage. Zero denominator gives unavailable, not 0%. Do not average
   per-call percentages. Validate subset bounds; preserve anomalous raw counts
   as diagnostics instead of clamping them into misleading valid metrics.
5. Other input is input minus cached reads where provider semantics permit.
   It may include cache writes. A three-part fresh/read/write cost breakdown
   requires provider-specific semantics and prices, not another generic sum.
6. Keep absent, reported zero, pending, partial, and unsupported distinct. Use
   checked integer sums in Go and a lossless JSON representation if aggregated
   values exceed JavaScript's safe integer range.
7. Use explicit half-open time ranges and a display timezone. Preserve the
   provider's daily bucket boundaries; do not relabel them as Detroit midnights
   unless their timezone contract supports that conversion.
8. API reads return only typed counts and allowed metadata. They never return
   prompts, tool arguments, credential material, or whole raw transport logs.

## Proposed implementation sequence

**1. Confirm Codex data and attribution.** A bounded runtime spike reads one
account and one known task using the installed API, then checks cached detail,
nulls, subagent inclusion, archived tasks, and repeat reads. Resolve how multiple
accounts are connected without switching the user's active development login.
Establish device/cloud coverage, retention, bucket timezone, and stable source
identity. If task detail is unavailable, validate a local importer against
repeated snapshots, copied/forked history, archive moves, truncation and partial
writes. Unprovable historical attribution stays unassigned.

**2. Ship Evie conversation Usage.** Author a focused specification that
explicitly extends the existing usage boundary, then add bounded aggregate
queries over existing assistant events, a consumer-owned query interface, and
a protected `/api/data/usage/...` read API. Wire the real Data tab. Reuse the
immutable existing events instead of creating a duplicate historical ledger.
Demonstrate repeated tool iterations, missing counters, date/model filters,
coverage, and restart-stable totals.

**3. Add connected Codex sources.** Persist source bindings and idempotent
observations/checkpoints appropriate to the validated collector. Keep quota
snapshots and token observations distinct. Demonstrate two sources, unknown
ownership, stale/disconnected status, and polling without double counting.

**4. Expand Evie coverage.** A separate story records compaction, extraction,
and provider attempts independently of accepted conversation evidence. It needs
its own persistence/recovery contract for charged failures and interruptions.
Add actual cost only when the transport preserves authoritative charge data;
include response identity to support reconciliation where available.

The existing [memory usage decision](../active/memory.decisions.md) explicitly
excludes aggregation/frontend state and excludes compaction usage. The proposed
feature extends those decisions; implementation must amend the relevant
specification/decision before treating analytics or broader capture as already
authorized feature behavior. This note leaves those binding files untouched.

## Verification and open choices

The initial implementation should test null versus zero, compatible cache
coverage, duplicate observations, nested subset accounting, timestamp boundaries,
integer overflow, guarded API access, requested-model provenance, restart, and
unassigned accounts. Full runtime attempt capture is a separate verified story.

For this proposal, `git diff --check` is the documentation-only repository
check. The preview requires a browser check of source selection and responsive
layout. Go/UI test suites and `./scripts/verify-change.sh` are not applicable to
this research-only change; run them when product code is implemented.

The outstanding product choice is whether OpenAI means Codex subscription
accounts only or also OpenAI API organizations. The proposed first scope is
Codex subscriptions plus Evie on this Mac. Multi-device collection, billing
reconciliation, budgets/alerts, and cache optimization are separate follow-ups.
