# Subagents implementation and local demonstration

This implements the foundation of #155 and #169–#175 plus the owner's additional
parallel foreground outcome. The local amendment is in
[subagents.decisions.md](subagents.decisions.md); published issues are unchanged.

## Review slices

| Slice | Main review entry points | Observable verification |
| --- | --- | --- |
| #169 | `internal/plugins/research.go`, `preset.go` | Research resolution and receipt reopening expose exactly Web search/fetch; old Standard versions retain their capabilities. |
| #170 | `internal/agent/agent.go`, `context.go` | Dedicated trusted worker instructions, explicit assignment origin, own history, no automatic Task context. |
| #171 | `internal/eviedb/compiler_source.go`, owner-source queries and compiler reconciliation | Live and historical compiler admission reject delegated lineage; owner-evidence preparation rejects it too. |
| #172 | `internal/delegation/types.go`, `internal/eviedb/subagents.go`, `internal/subagents/supervisor.go` | One durable attempt, child and receipt per parent/key; committed invocation and exact arguments authorize admission. |
| #173/#174 | `internal/eviedb/subagent_execution.go`, `turn_leases.go` | Parent plus child fencing, bounded cleanup, accepted-answer arbitration, recovery without execution. |
| #175 | `internal/plugins/subagents.go`, `cmd/evie/subagents.go`, `main.go` | Optional compiled Plugin, shared CLI/web supervisor, current disable and shutdown, explicit Workspace permission checks. |
| Parallel foreground and Task Tree amendment | `internal/subagents/supervisor_test.go` | Actual overlap and shared capacity, ordered partial results, duplicate waiting, explicit orchestrator claims and Task completion. |

The Plugin provides `delegate_research` with an `assignments` array. Each member
has `idempotency_key`, `objective`, optional `context`, and optional `task_id`.
There are no model arguments for providers, presets, credentials, scope,
permissions, parent identity or policy. Results carry execution and child IDs,
terminal state, a safe reason, the report's summary, structured sources,
unverified URLs, limitations and usage. Since 2026-10-01 the Plugin also provides
`read_subagent_report(execution_id, offset?, limit?)`, which pages a child's full
stored report for the parent session that delegated it, and
`continue_research(execution_id, message)`, which extends the parent's own
finished child in its same session (see "Continuation" below).

An identical key resolves to its retained attempt. Changed content conflicts.
Failed, cancelled and interrupted keys require a new key to run again. A failed
child does not cancel independent siblings. Cancelling a duplicate waiter ends
only that wait. Parent cancellation and shutdown stop unfinished work belonging
to the original dispatch. Retained successful siblings are never rerun.

Retained reasons distinguish provider transport, invalid provider responses,
infrastructure, policy limits, cancellation and authority interruption without
including raw provider errors. Recovery revisits initially live ownership during
CLI/web operation even when the Plugin is disabled. It only reconciles metadata;
it never resumes execution or appends a conversation outcome.

The deadline applies to each child from its own start, so a child that waited
for capacity still receives the full deadline. A child waits for capacity at
most one deadline after admission; if it never starts it ends
`failed`/`queue_deadline`. A child exceeding its own deadline ends
`failed`/`deadline_limit`. Parent turn cancellation ends unfinished children
`cancelled`/`parent_cancelled`; stopping the supervisor (process shutdown, or
stopping or disabling the Subagents Plugin) ends them `cancelled`/`shutdown`;
lost parent authority ends them `interrupted`/`authority_ended`. One tool call
can therefore wait up to two deadlines when children queue. Since 2026-10-01 a
child that wraps up at 90% of its time, token or context budget, or at the
step limit, ends `partial`/`time_budget`, `token_budget`, `context_budget` or
`step_limit` with its report; a wrap-up that produces no report (the response
still requests tools, or even its smallest request cannot fit) ends
`failed`/`wrap_up_failed`.

Parent authority is proved by the intent's position in the current parent turn
rather than by walking event ancestry, so it holds at any turn depth. Checks that
only read (the child watchdog, pre-model-call authorization, inspection and
recovery candidate selection) use read transactions; every child mutation still
re-authorizes under the write lock. SQLite lock contention (`SQLITE_BUSY`/
`SQLITE_LOCKED`) is retried and never ends a child's authority. Recovery
reconciles each attempt in its own transaction; an unreadable record is reported
without blocking other attempts or new delegation, and failed passes retry with
backoff up to 10 seconds.

Task associations are lineage only. The orchestrator uses existing Todo access,
claims and revision checks to maintain the Task Tree explicitly. Children cannot
read Tasks, receive focus or claim work through association. Incidental research
creates no Task. No dependency scheduling is performed.

## Configuration and demonstration

From this worktree, with the usual OpenRouter and Web credentials already set:

```sh
npm --prefix internal/web/ui ci
npm --prefix internal/web/ui run build
go run ./cmd/evie plugins enable subagents
go run ./cmd/evie
# Alternatively:
go run ./cmd/evie serve
```

Subagents is installed but disabled by default. Enabling it affects newly
composed eligible conversations; existing receipts are unchanged.
Start a new conversation and request: “Research two independent aspects of this
question in parallel. Use a separate research assignment and stable key for each,
then compare their evidence.” Observe the normal delegation tool call and result,
followed by the orchestrator's answer. Child streaming is never presented as the
orchestrator speaking.

For tracked work, ask the orchestrator to create/decompose a Task Tree, claim
independent siblings, associate each assignment, then review findings and update
the Tasks with current revisions. Association itself leaves Tasks unchanged.
Try completing the parent before all descendants finish; the existing Task
service rejects it. Repeat a batch with the same keys to observe retained results.
Cancel an in-progress parent turn or disable the Plugin to stop unfinished work:

```sh
go run ./cmd/evie plugins disable subagents
```

For a Standard Workspace, also enable **Allow research delegation** in its
settings (or during creation), then start a new chat. Disabling this permission
stops active research; enabling it again applies only to new chats. See
[Workspace research](workspace-research.spec.md) for the reviewed allowance,
revocation, model-budget, and excerpt contracts.

Operator environment settings (all must be finite and positive):

| Variable suffix after `EVIE_SUBAGENTS_` | Default |
| --- | --- |
| `PER_PARENT` | 2 running children |
| `RUNTIME` | 4 running children |
| `MAX_BATCH` | 8 assignments |
| `PER_TURN` | 16 children per parent turn, across its delegation calls |
| `DEADLINE` | `15m` per child from its start; also bounds its capacity wait |
| `TOKEN_BUDGET` | 1000000 input plus output tokens per child, shared with compaction |
| `ASSIGNMENT_BYTES` | 8192 objective/context bytes |
| `REQUEST_BYTES` | 1048576 serialized model-request/response bytes (1 MiB), subject to the invoking model's route-safe context limit |
| `RESULT_BYTES` | 12000 serialized inline result bytes per child, minimum 512 |

`MODEL_CALLS` and `OUTPUT_TOKENS` were retired on 2026-10-01; setting either
fails startup with the replacement named. Children use the model's normal
output reserve and wrap up at 90% of their time or token budget.

The complete batch result envelope must fit 96 KiB so it survives the existing
100 KiB tool-result admission boundary. This limits one response, not the product's
number of agents. Runtime capacity is coordinated through the local database,
so simultaneous Evie processes sharing it also compete for running slots.
The deterministic tests measure overlap under selected small configurations;
they do not establish a hardware capacity recommendation.

## Deterministic checks

The main acceptance suite uses the real Plugin, existing agent loop and temporary
SQLite. Only external model and Web execution are replaced. It covers concurrent
providers, partial failures, accepted-result replay, cancellation, shutdown,
parent lease loss, Task isolation/claims/completion, Workspace refusal and limits.
The database tests reopen admission and final-answer gaps, preserve foreign live
ownership, interrupt abandoned work, and prove recovery appends no conversations.

```sh
go test ./internal/subagents
go test ./internal/plugins -run TestResearchPresetResolvesAndReopensOnlyWeb
go test ./internal/agent -run TestDelegatedConversationUsesPinnedRoleAndOwnAssignment
go test ./internal/eviedb -run 'TestSubagentAdmission|TestRecoveryPreservesAcceptedChild|TestDelegatedAssignment|TestOngoingRecovery'
go test -race ./internal/subagents -timeout 120s
go test -race ./internal/agent ./internal/eviedb -run 'Subagent|Delegated|RecoveryPreservesAcceptedChild|OngoingRecovery' -timeout 120s
./scripts/verify-change.sh
```

### Verification record — 2026-09-10

- `gofmt` formatted all 39 changed/new Go files.
- `go test ./internal/subagents -timeout 45s` passed.
- `go test -race ./internal/subagents -timeout 120s` passed (30.957s).
- `go test -race ./internal/agent ./internal/eviedb -run 'Subagent|Delegated|RecoveryPreservesAcceptedChild|OngoingRecovery' -timeout 120s`
  passed (agent 4.850s, eviedb 13.551s).
- `go test -race ./internal/subagents -run 'TestForegroundBatchActuallyOverlapsWithinConfiguredCapacity|TestRuntimeCapacity' -count=3 -timeout 120s`
  passed (21.594s). An earlier concurrent race run exceeded a one-second gate
  arrival timeout. The liveness allowance is now five seconds; measured overlap,
  peak capacity and exact call-count assertions are unchanged.
- `./scripts/verify-change.sh` passed after the final source/test edits: UI lint
  and build, full Go tests, full Go vet, staged and unstaged whitespace checks.
- Standards and Spec reviews have no remaining findings after regression fixes
  for old receipts, recovery after initially live ownership, serialized output
  limits, incomplete usage and failure/authority classifications.

`npm --prefix internal/web/ui ci` completed and reported three dependency
vulnerabilities (two moderate, one high). No dependencies were changed. Vite
reported the existing chunk-size warning above 500 kB. Live model/Web execution
was not performed; the external boundaries use deterministic fakes in tests,
and credentialed manual demonstration steps are provided above. No required
automated check was skipped.

Limits retained from the approved scope: foreground only; one immutable web-only
research preset; no nesting, background continuation, automatic restart execution,
Task grants to children, memory-enabled workers, distributed execution, new UI
panels or Workspace authoring. Model usage remains unknown when complete provider
measurements are unavailable. A single-turn child has no prior closed turns to
compact; the existing runtime rejects pressure without a legal compaction boundary.

## Integration with Memory Stage 5 — 2026-09-11

Local master advanced to `fe55a666abd21cc78d076c4da6cbc13f83c7fbd7` before the
owner requested the merge. Integration preserves both runtime hosts and parent
retrieval accounting, while delegated construction explicitly disables automatic
and model-directed memory retrieval. Conversation indexing and its shared exact
source loader exclude delegated session lineage, preventing child assignments
from becoming owner statements or entering embedding requests.

The combined Standard preset has a new content hash. Exact historical definitions
remain available for the original preset (`35d56…`), retrieval-only (`3c812…`) and
Subagents-only (`ea528…`). Regression checks reopen all three and the combined
preset without changing receipts or capabilities.

New real-SQLite tests cover lexical and dense child exclusion after append,
reopen and index rebuild, automatic recall, persisted memory receipts, and legacy
child references rejected by inspection, revalidation and expansion. Positive
owner controls remain available. The partial-failure fixture now waits for both
completed siblings to become durable before cancellation. The message-ordering
fixture leaves room for both features' trusted instructions; context-overflow
boundaries remain separately tested.

Integration verification passed:

- `./scripts/verify-change.sh` — full Go tests/vet, UI lint/build and both
  whitespace checks; only the existing Vite chunk-size warning.
- `go test -race ./internal/subagents ./internal/agent ./internal/eviedb ./internal/plugins -run 'Subagent|Delegated|ForegroundBatch|ForegroundAssignment|BatchRetains|ChildModel|DuplicateWaiter|ComposedParent|TaskAssociation|RuntimeCapacity|AuthorityLoss|RecoveryPreservesAcceptedChild|ConfiguredDeadline|WorkspaceAdmission|ChildOutputLimit|ProviderFailures|ChildPersistenceFailure|ChildComposedContext|OngoingRecovery|ParallelAndRetrievalPreset' -timeout 180s`
  — all four packages passed, with no races.
- Read-only integration review — no remaining findings.

## Supervision durability — 2026-10-01

Stage 7 of the 2026-09-30 harness review (D1–D6). No schema change or
migration. Behavior changes are described above: per-child deadlines with a
bounded capacity wait, distinct `queue_deadline` and `shutdown` reasons, parent
authority without an ancestry walk, read-only authority checks, retried lock
contention, and recovery isolated per attempt with bounded backoff.

Regression tests: `internal/eviedb/subagent_durability_test.go` (110-round
parent turn, preserved lineage refusals, never-started attempt after reopen,
unreadable record isolation) and `internal/subagents/durability_test.go`
(queued child at the deadline, late starter's full deadline, shutdown versus
parent cancellation, transient errors in the watchdog and pre-call check,
recovery surviving transient errors, capacity-wait failure, delegation past an
unreadable record). `store_fault_test.go` injects coded SQLite errors into a
real SQLite connection.

Verification passed:

- `./scripts/verify-change.sh` — UI lint/build, full Go tests and vet, both
  whitespace checks; only the existing UI lint and Vite chunk-size warnings.
- `go test -race ./internal/subagents -count=1 -timeout 300s`
- `go test -race ./internal/agent ./internal/eviedb ./internal/plugins -run 'Subagent|Delegated|ForegroundBatch|ForegroundAssignment|BatchRetains|ChildModel|DuplicateWaiter|ComposedParent|TaskAssociation|RuntimeCapacity|AuthorityLoss|RecoveryPreservesAcceptedChild|RecoveryInterrupts|RecoveryIsolates|ConfiguredDeadline|WorkspaceAdmission|ChildOutputLimit|ProviderFailures|ChildPersistenceFailure|ChildComposedContext|OngoingRecovery|ParallelAndRetrievalPreset' -count=1 -timeout 300s`
- `go test -race ./internal/subagents -count=5 -run` over the new and existing
  timing-sensitive supervisor tests.

Startup in `cmd/evie/main.go` still treats any recovery error as fatal, so an
unreadable record now reported by recovery would stop startup as before; that
file was outside this change.

## Budgets and results — 2026-10-01

Stage 8 of the 2026-09-30 harness review (G1, G2, G3, G6); the binding record is
the 2026-10-01 entry in [subagents.decisions.md](subagents.decisions.md).

Review entry points: `internal/subagents/budget.go` (token meter, wrap-up
signal, budget brief), `internal/agent/wrap_up.go` (`WithWrapUp`, context
measurement and fitting of the wrap-up request; `turn.go` only calls it at the
step-limit decision and records its placeholders),
`internal/eviedb/subagent_result.go` (sources, summary,
bounding, report paging, durable wrap-up mark), `finishSubagent` in
`subagent_execution.go`, the per-turn count and `partial` migration in
`subagents.go`, and `read_subagent_report` in `internal/plugins/subagents.go`.

Schema change: `subagent_executions.state` accepts `partial`. Startup rebuilds
an earlier table under the write lock, copying rows and rowids unchanged; an
unknown shape fails closed. The Standard preset gains the optional
`subagents.report` capability (new version `sha256:adeb2e7b…`); the previous
version `sha256:50ff6768…` is kept as a historical definition, so existing
sessions reopen unchanged and need a new chat to read reports.

Regression tests: `internal/subagents/budget_results_test.go` (step-limit,
injected-clock and reported-token wrap-ups returning `partial` with findings;
unreported usage counted conservatively; a wrap-up that still requests tools;
fetched, cited and unverified sources; long reports paged only for their own
parent; the 17th child of a parent turn; context pressure wrapping up with
excerpts or markers, and an unfittable wrap-up failing with its fetched
sources), `TestFailedChildReportsMeasuredUsageAsIncomplete`,
`internal/eviedb/subagent_budget_test.go` (pre-amendment table and records
load, recover and render unchanged), `internal/agent/wrap_up_test.go` and
`cmd/evie/subagents_test.go`.

Verification passed:

- `./scripts/verify-change.sh` — UI lint/build, full Go tests and vet, both
  whitespace checks; only the existing UI lint and Vite chunk-size warnings.
- `go test -race -count=1 ./internal/subagents/ ./internal/delegation/ ./internal/plugins/`
- `go test -race -count=1 -run 'Subagent|Delegated|RecoveryPreservesAcceptedChild|OngoingRecovery|PreBudget|WrapUpSignal|StepLimit' ./internal/eviedb/ ./internal/agent/`
- `go test -race -count=3` over the new budget and result tests.

Known limits: the time budget is checked before each model call, so a step in
flight at 90% can consume the remaining 10% and end at the hard deadline
without a report (its fetched sources are still listed). A one-turn child has
no closed turns to compact, so context is a wrap-up budget: at 90% of its usable
request budget the wrap-up request is fitted by shortening tool results, and it
fails (`wrap_up_failed`, sources listed) only when assistant messages, the
assignment and instructions alone overflow. Live model and Web execution was
not exercised.

## Continuation — 2026-10-01

Stage 9 of the 2026-09-30 harness review (G10); the binding record is the
"Continuing a finished child" entry in
[subagents.decisions.md](subagents.decisions.md).

`continue_research(execution_id, message)` extends one of the calling
session's own attempts that finished with a report (`succeeded` or
`partial`), provided it is the child's latest report and none of the child's
attempts is unfinished. It returns one result in the `delegate_research`
format with `continues_execution_id`; the new `execution_id` pages the new
report and can itself be continued. Refusals name the running or latest
execution to use; another session's attempt reads as not found.

Review entry points: `AdmitSubagentContinuation` and `resumableSubagent` in
`internal/eviedb/subagents.go` (admission, idempotency, resumability, the
table rebuild), `StartSubagent`/`reopenSubagentChild`, the latest-attempt
child fence and the `after_sequence` boundary in `finishSubagent`
(`internal/eviedb/subagent_execution.go`), turn-scoped evidence in
`subagent_result.go`, `Continue` and `enter` in
`internal/subagents/supervisor.go`, the follow-up framing in `budget.go`, the
capability in `internal/plugins/subagents.go`, and the lease stamp for
`continue_research` intents in `internal/eviedb/events.go`.

Schema change: `subagent_executions.child_session_id` is no longer unique; a
child index and a unique partial index (one unfinished attempt per child) are
added. Startup rebuilds the Stage 8 table, or an earlier one without
`partial`, once under the write lock with rows and rowids unchanged; an
unknown shape fails closed. The Standard preset gains the optional
`subagents.continue` capability (new version `sha256:3b3ef3ab…`); the previous
version `sha256:a42624b0…` joins `adeb2e7b…` and `50ff6768…` as historical
definitions, so existing sessions reopen unchanged and need a new chat to
continue children.

Demonstration, after enabling Subagents and starting a new chat: ask for
research on a broad question with a small budget (for example
`EVIE_SUBAGENTS_TOKEN_BUDGET=20000`) so the child returns `partial`, then ask
Evie to continue that child on the gaps it listed. Observe one
`continue_research` call whose result has the same `child_session_id`, a new
`execution_id` and `continues_execution_id`, and a report that builds on the
first. Asking to continue the earlier execution again is refused with the
latest one named.

Regression tests: `internal/subagents/continuation_test.go` (continuing a
partial child in the same session with its history and a fresh budget, and a
new result with turn-scoped sources and usage; refusal for another parent, an
unknown ID, a running child and a failed attempt; same-intent retry,
changed-arguments refusal, superseded-attempt refusal and chaining; per-turn
counting; compaction of the earlier turn; wrap-up and parent authority loss
during a continuation; the composed parent loop delegating then continuing),
`internal/eviedb/subagent_continuation_test.go` (crash mid-continuation with
and without an accepted report, the stale child lease fenced out, the parent
capability and intent requirement, the Stage 8 table and a pre-budget record
migrating and continuing) and the preset history case
`combined_before_continuation` in `internal/plugins/preset_test.go`.

Known limits: the context wrap-up is decided before compaction, so a single
step that jumps past 90% of the usable budget ends the continuation with a
wrap-up even when its earlier turns could have been compacted; the next
continuation compacts them. A continuation's result lists earlier-turn pages
only when its report cites them. Live model and Web execution was not
exercised.
