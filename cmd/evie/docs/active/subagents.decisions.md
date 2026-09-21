# Subagents implementation decisions

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
   admission explicitly refuses until reviewed preset allowances (#71) exist.
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
