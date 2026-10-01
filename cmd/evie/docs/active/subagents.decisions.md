# Subagents implementation decisions

## 2026-10-01 — Time and token budgets, wrap-up reports, pageable results

Source: harness review Stage 8 (G1, G2, G3, G6) in
[docs/harness-review-2026-09-30.md](../../../../docs/harness-review-2026-09-30.md),
which the owner approved, following the pattern shared by Anthropic's research
system, Claude Code sub-agents and the OpenAI Agents SDK. It deliberately
replaces specified limits.

Superseded wording: the spec's "total model-call allowance, input and output
context limits" and "Count conversational and compaction model calls against
the same child allowance"; the 2026-09-10 entry's "total model-call" limit; the
regex-extracted sources and blind result truncation of #172; and the Workspace
research slice's "no ... larger parent-result cap".

- **Budget.** Each child has a wall-clock deadline (default 15 minutes from its
  own start) and a token budget (default 1,000,000 input plus output tokens).
  Tokens are provider-reported usage of every conversational and compaction
  call; when a response lacks usage, its serialized request and response bytes
  count at one token per byte. Failed calls count nothing, since transport
  retries happen only before a response. The 8-call allowance and the
  1024-token output cap are removed; the child uses the model's normal output
  reserve. The request-byte limit, per-parent and runtime concurrency, the
  capacity wait and the turn step limit stay.
- **Told up front.** The harness appends the budget and the report format to
  the child's first message, after the assignment and selected context. The
  pinned `ResearchInstructions` are unchanged, so the research preset version,
  its instruction digest and every existing receipt stay valid.
- **Wrap-up.** Before each model call the supervisor checks the child's budget
  through the step-limit seam of the agent loop (`agent.WithWrapUp`). At 90% of
  the time or tokens, or at the step limit, it durably records the reason on
  the attempt (`wrap_up`), and that call is sent with `tool_choice: "none"`
  and a trailing harness notice asking for the report now. A text answer
  settles the attempt as the new terminal state `partial` with reason
  `time_budget`, `token_budget`, `step_limit` or `context_budget` (below);
  the durable mark makes a
  report accepted before a crash recover as `partial` too. If the wrap-up
  response still asks for tools, nothing is committed and the attempt is
  `failed`/`wrap_up_failed` (formerly a step-limit child was
  `infrastructure_failure`). Once the token budget is spent no further call
  starts except that one wrap-up. The hard deadline still cancels the child;
  an in-flight step at 90% can use the remaining 10%.
- **Report and result.** The child is asked for `## Summary` (about 1,000 to
  2,000 tokens), then `## Details`, then `## Limitations`. Its final answer is
  the stored report. The inline result carries status and reason, `summary`
  (the Summary section, or the report's beginning when it has none),
  `report_bytes`, structured `sources`, `unverified_urls`, `limitations`
  (harness notes first, then up to eight report bullets), `usage` and the
  execution ID. Older results keep `findings` and string `sources` and render
  exactly as stored. The inline result is bounded by `result_bytes` (default
  12,000 so eight results still fit the 96 KiB batch envelope); the summary is
  cut first, with `summary_truncated` and a limitation pointing to the reader,
  then the least useful list entries, with an omitted count. Nothing is cut
  silently.
- **Reading reports.** A new optional Standard preset capability,
  `subagents.report` (`read_subagent_report(execution_id, offset?, limit?)`),
  pages the stored report in UTF-8-safe byte pages of 256 to 32,768 bytes
  (default 16,384), with `next_offset`. It reads only attempts whose parent
  session is the calling session, under InspectSubagent's current Workspace,
  project and Task access checks; another session's attempt is reported as not
  found. Sessions composed before it keep their receipts (`sha256:50ff…`) and
  need a new chat to gain it. The research preset never includes it.
- **Sources.** Built from the child's successful tool events: every URL
  `web_fetch` read (the excerpt's reported URL, or the requested URL for the
  legacy contract; a cross-host redirect notice reads nothing), and each
  `web_search` result URL line. Fetched URLs are listed cited or not; search
  results only when cited, marked `fetched: false`. URLs the report cites that
  no tool event returned are `unverified_urls`. Citation matching ignores
  scheme and host case, fragments, trailing slashes and trailing punctuation.
- **Salvage.** An attempt that ended without a report (failure, cancellation,
  authority loss, wrap-up failure) still lists the pages it fetched and says
  no report was produced; nothing is synthesized.
- **Usage.** Every outcome reports the sum of usage on the child's committed
  responses. `incomplete` marks a lower bound: a committed response without
  usage (compactions record none), or a turn that ended inside a provider or
  compaction call. With no reported usage at all it stays `null`.
- **Per-turn limit.** One parent turn admits at most `per_turn` children
  (default 16) across all its delegation calls, counted durably by the turn's
  root event. Retrying retained keys admits nothing new. A refusal names the
  limit, the children already started and the number requested.
- **Persistence.** `subagent_executions.state` gains `partial`; startup
  rebuilds an earlier table under the write lock, copying every row
  unchanged, and refuses an unknown table shape. Policies gain `per_turn` and
  `token_budget`; earlier policies keep `model_calls` and `output_tokens` when
  rewritten and cannot execute. Operators setting the retired
  `EVIE_SUBAGENTS_MODEL_CALLS` or `EVIE_SUBAGENTS_OUTPUT_TOKENS` get a startup
  error naming the replacement.

- **Context budget.** A child runs one turn, so automatic compaction, which
  removes only whole earlier turns, never applies to it; its older tool
  results are projected under pressure as in the primary chat. Context is
  therefore a third wrap-up trigger, reason `context_budget`: from the second
  response on, when the next request, measured after that projection and
  before the wrap-up notice, would reach 90% of the child's usable request
  budget, which includes requests that would exceed it. The wrap-up request is
  then fitted to the budget: if it does not fit as projected, every tool
  result over the 4 KiB projection threshold is reduced to its usual
  head-and-tail excerpt, and if that still does not fit, every tool result to
  a one-line marker. Both forms name the event, its original size and hash,
  and the request's context snapshot records each reduction against durable
  content. Assistant messages, the assignment and instructions are never
  shortened. Only when even the marker form cannot fit does the attempt fail,
  as `failed`/`wrap_up_failed`, still listing the pages it fetched. A first
  request that cannot fit at all has no work to wrap up and stays
  `failed`/`policy_limit`.

Continuing a partial child (G10) is harness review Stage 9; until then a new key
starts a fresh attempt.

## 2026-09-11 — Capability-aware parent delegation guidance

The owner approved general delegation guidance in the parent's system prompt:
use bounded delegation when independent progress, focused investigation, or
fresh eyes can improve quality or save time, without requiring an explicit
request for subagents. The parent provides the assignment context and checks
and integrates the result. Coding and review are conditional on available worker
capabilities; the existing web-only research preset and its pinned instructions
remain unchanged. Independent review need not run concurrently with implementation.

## 2026-09-10 — Parallel foreground work and orchestrator-owned Task Trees

Source: the owner's implementation request explicitly amends the original spec
and published tickets. No GitHub issue changes are authorized or required.

Superseded wording: “at most one active child per parent turn”; “Parallel
foreground workers ... are separate outcomes”; the exclusion “Parallel
foreground fan-out”; and the exclusion of Task decomposition/progress mutations
insofar as it applied to the orchestrator. #172 remains the original bounded
single-assignment foundation, not an already-specified parallel extension.

One orchestrator runs independent local research children through a bounded
foreground batch interface. Operator configuration supplies finite per-parent
and runtime concurrency, deadline, total model-call, request-context and result
limits. Arguments cannot raise policy. Children retain separate durable
identities, receipts, histories, fences and terminal results. No background
continuation, automatic restart execution, nesting, distributed execution,
automatic dependency scheduling or cloud infrastructure is introduced.

The Task Tree records intended work, decomposition, progress and reviewed
results. Execution attempts record worker lifecycle separately. The orchestrator
uses its existing Todo access, claims and revision checks; research children
receive only Web search/fetch, trusted worker instructions and explicit assignment
data. They receive no Todo, grants, focus, claims, automatic recall, sibling
history, filesystem/shell or delegation. Optional Task association is validated
against parent access; it grants nothing, claims nothing, launches nothing by
itself and never changes completion. Incidental assignments need no Task.

## Reviewable slices and observable acceptance

1. **#169 — restricted preset:** resolve and reopen exactly Web search/fetch;
   fabricated excluded tools fail and old receipts reconstruct unchanged.
2. **#170 — assignment context:** reuse the agent loop with trusted child role,
   selected context and own history, without parent history, recall or Task projection.
3. **#171 — provenance:** delegated assignments cannot become owner assertions
   through live/historical compilation or source acceptance.
4. **#172 — durable assignment:** atomically admit under committed parent intent
   and live ownership; pin composition/policy; persist bounded result before delivery;
   identical retries reuse one execution and changed arguments conflict.
5. **#173/#174 — authority and recovery:** cancellation, revocation, lost parent
   or child ownership and shutdown stop activity. Accepted final child evidence
   wins terminal races. Database reopen retains accepted findings and marks
   abandoned unfinished work interrupted without model calls or fabricated events.
6. **#175 — rollout:** compiled optional Plugin is available only to newly
   composed eligible CLI/web parents. Disable reaches active workers. Workspace
   admission requires reviewed preset allowances. The September 29
   [Workspace research slice](workspace-research.spec.md) supplies explicit
   Standard Workspace opt-in, pinned revisions, and immediate durable revocation.
7. **Additional parallel foreground outcome (not in original tickets):** a
   bounded batch actually overlaps independent children. Deterministic provider
   gates prove overlap and capacity at multiple configured limits. Results follow
   input order. Per-child provider/policy failures are retained and do not cancel
   independent siblings; parent cancellation/authority loss cancels all unfinished
   children. Duplicate batches reuse each child key and preserve accepted results;
   failed/interrupted keys require a new key to execute again. Validate the complete
   request before admitting new children; reject invalid arguments or changed keys.
   Existing independently selected sibling Tasks may be associated; the orchestrator
   claims and updates them explicitly after reviewing findings. Claim conflicts,
   revision checks and parent completion rules remain the Task service's contract.

The approved test seams are preset resolution/receipt reopening, composed
delegation and the supervisor public interface, existing Task service operations,
and durable execution recovery through SQLite reopen. Use deterministic external
provider fakes and real persistence. Successful spawn counts are not capacity
measurements.
