# Codex usage data sources

**Research date:** 2026-09-05

**Authority:** official OpenAI documentation, the installed Codex protocol
schema, and bounded inspection of local structured usage records. This is
research, not an approved implementation specification. No authentication
secrets, prompts, or transcript contents are reproduced here.

## Main finding

Use the supported account activity endpoint for account totals. The installed
protocol also exposes a promising per-task usage request with cache detail.
Local records can supply a separate history of measured requests, but do not
establish which OpenAI account paid for each historical request. Combining
these sources requires explicit coverage and attribution, not adding their
totals together.

## Supported account activity

The official App Server documentation describes `account/usage/read` returning
nullable summary metrics and optional daily token buckets. It requires
Codex-service-backed authentication; API-key-only authentication is excluded.
The documented account response has no cache, model, or input/output split.
([Token usage documentation](https://learn.chatgpt.com/docs/app-server#7-token-usage-chatgpt))

`account/rateLimits/read` reports quota windows: percentage consumed, window
duration, and reset time. These measure capacity, not a token ledger.
`account/read` reports the current account, with email and plan for ChatGPT;
it can be called without requesting a token refresh.
([Account documentation](https://learn.chatgpt.com/docs/app-server#auth-endpoints))

The documentation does not establish daily-bucket timezone, history retention,
refresh latency, or coverage across local tasks, cloud tasks, and ordinary
ChatGPT conversations. Do not promise those properties without a runtime check.
([App Server documentation](https://learn.chatgpt.com/docs/app-server))

## Installed protocol: additional task detail

The working bundled executable is
[`/Applications/ChatGPT.app/Contents/Resources/codex`](/Applications/ChatGPT.app/Contents/Resources/codex),
version `codex-cli 0.153.3`. Its generated experimental TypeScript schema adds:

```ts
// GetAccountTokenUsageParams
{ threadId?: string | null }

// GetAccountTokenUsageResponse
{ summary, dailyUsageBuckets, threadUsage?: ThreadUsage | null }

// ThreadUsage
{
  threadId: string,
  estimatedUsageCreditsMicros: bigint,
  estimatedUsageUsdMicros: bigint | null,
  groups: Array<ThreadUsageBreakdownGroup>
}

// ThreadUsageBreakdownGroup
{
  model: string | null,
  reasoningEffort: string | null,
  speed: string | null,
  estimatedUsageCreditsMicros: bigint,
  netNewInputTokens: bigint | null,
  cachedInputTokens: bigint | null,
  inputTokens: bigint | null,
  outputTokens: bigint | null,
  totalTokens: bigint | null
}
```

Source: generated `v2/GetAccountTokenUsageParams.ts`,
`v2/GetAccountTokenUsageResponse.ts`, `v2/ThreadUsage.ts`, and
`v2/ThreadUsageBreakdownGroup.ts`. Schema comments describe task usage as
estimated and available when the task's billing route is available. This
request shape was generated with `--experimental`; runtime availability and
historical coverage remain untested. Preserve the distinction between estimated
credits/USD and actual billing. The schema exposes no reasoning-token field
inside these groups.

The same generated schema has `thread/tokenUsage/updated` with `threadId`,
`turnId`, and a `tokenUsage` object containing `total`, `last`, and a nullable
context-window size. Its `TokenUsageBreakdown` contains `inputTokens`,
`cachedInputTokens`, `cacheWriteInputTokens`, `outputTokens`,
`reasoningOutputTokens`, and `totalTokens`. This is a live notification, not an
account-wide historical query. Source: generated
`v2/ThreadTokenUsageUpdatedNotification.ts`, `v2/ThreadTokenUsage.ts`, and
`v2/TokenUsageBreakdown.ts`.

The generated `ClientRequest.ts` contains no `account/list` or `account/switch`
method. Its `Account` type contains only authentication type and, for ChatGPT,
nullable email plus plan; no durable account identifier is exposed there.
It does not prove that the desktop app has no separate account-management
mechanism. Source: generated `ClientRequest.ts` and `v2/Account.ts`.

## Local request records

A bounded sample covered older and recent files in
[`sessions`](/Users/davidboktor/.codex/sessions) and
[`archived_sessions`](/Users/davidboktor/.codex/archived_sessions).
Only metadata keys and usage structures were examined for reporting.

Recent files contain a `token_usage_record` with:

```text
thread_id, turn_id, session_id, root_turn_id, response_id
usage                 per-response usage in the sampled records
turn_token_usage      running turn counters
thread_token_usage    running thread counters
```

Each usage structure contains input, cached input, cache-write input, output,
reasoning output, and total token counters. Records are timestamped. In the
sample, `total_tokens = input_tokens + output_tokens`; cache tokens are already
inside input, and reasoning tokens inside output. Do not add these subsets
again. Cache-write counters were zero in the inspected examples; their presence
does not establish that this provider reports cache writes meaningfully.
Source: [recent archived task, first usage record at line 15](/Users/davidboktor/.codex/archived_sessions/rollout-2026-09-04T21-42-10-01a06f3a-f6e3-7e02-ba18-dd1a26b79b94.jsonl:15).

Older files have `event_msg` / `token_count` records with
`info.total_token_usage` and `info.last_token_usage`. Successive cumulative
increases matched the latest request counters in inspected uninterrupted
sequences. Some snapshots repeat: the recent archived task above had 36
`token_count` events but only 30 per-response records, with six adjacent
identical cumulative snapshots. Summing every `last_token_usage` would overcount;
summing cumulative snapshots would overcount much more. An older sample has
cache/reasoning counters but lacks per-response records and cache-write fields.
Sources: the recent archived task above and
[older archived task](/Users/davidboktor/.codex/archived_sessions/rollout-2025-09-16T08-25-52-1aed33d3-fe11-4e74-abcd-45f3f2c4264d.jsonl).

Subagent files may contain their own `session_meta` followed by inherited parent
metadata. The first record in the sampled child identifies the child and its
parent; a later inherited record identifies the parent. Selecting the last
metadata record would misattribute the file. Per-response records carry explicit
thread IDs and response IDs, so these should guide ownership and deduplication.
Source: [sampled child, metadata at lines 1 and 2](/Users/davidboktor/.codex/archived_sessions/rollout-2026-09-04T21-42-41-01a06f3b-7000-7032-92b3-17bae29094dd.jsonl:1).

No account/email/auth/billing identifier occurred among the sampled session
metadata keys; sampled usage records likewise contain no account identity.
Current login identity cannot safely be applied to all historical files after
account switching. Model comes from turn context in these records, not from
the usage record itself. The generated `Thread.model` comment explicitly warns
that a task's current/latest model is not per-turn execution telemetry.
Sources: sampled files above and generated `v2/Thread.ts`.

## Implications requiring a separate implementation decision

- Prefer account totals from `account/usage/read`; first test optional task
  detail before choosing a local-history importer for cache statistics.
- If local import is necessary, keep it visibly scoped to observed local files
  and report unassigned historical account usage. Remote/cloud and missing or
  deleted files cannot be inferred from one host's directory.
- Treat account totals, server task estimates, local request counts, and quota
  snapshots as overlapping observations with different coverage. Do not sum
  them as independent consumption.
- Deduplicate new records by provider/host and request identity after validating
  response-ID behavior; retain file offsets for incremental reading. Validate
  copied/forked histories and archive moves. Never count both new per-response
  records and equivalent legacy snapshots.
- Legacy fallback needs explicit handling of repeated cumulative snapshots,
  decreases/resets, resumption, truncation, partial writes, and inherited
  history. No decreasing cumulative sequence was observed in the bounded
  sample; reset/compaction behavior remains unverified.
- Cache rate is `sum(cached input) / sum(input)` for the same covered requests,
  not an average of request percentages. Missing cache data is unknown, not
  zero. Keep reasoning as an output subset and show quota percentages separately.

## Reproduction and limits

The following read-only executable checks succeeded:

```sh
/Applications/ChatGPT.app/Contents/Resources/codex --version
/Applications/ChatGPT.app/Contents/Resources/codex app-server generate-ts --help
/Applications/ChatGPT.app/Contents/Resources/codex app-server generate-ts --out <temporary-directory> --experimental
```

The generated files were inspected in a temporary directory; no product code
was changed. The `codex` command on PATH failed with `ENOENT` for its missing
Homebrew-installed vendor binary, so any future integration must resolve the
working executable explicitly. No account endpoint was invoked, account changed,
credential opened, package installed, or complete transcript copied. A runtime
spike must establish endpoint availability, account identity, cache coverage,
subagent inclusion, retention, and refresh behavior before finalizing the story.
