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
terminal state, findings, sources, limitations, a safe reason and nullable usage.

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
| `DEADLINE` | `2m` foreground deadline |
| `MODEL_CALLS` | 8 per child, shared with compaction |
| `ASSIGNMENT_BYTES` | 8192 objective/context bytes |
| `REQUEST_BYTES` | 1048576 serialized model-request/response bytes (1 MiB), subject to the invoking model's route-safe context limit |
| `RESULT_BYTES` | 2048 serialized result bytes per child, minimum 512 |
| `OUTPUT_TOKENS` | 1024 per model call |

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
