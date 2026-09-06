# LangGraph semantics for Evie procedural workflows

**Research date:** 2026-08-30

**Authority:** current official LangGraph documentation and source, compared with
Evie's approved memory specification and decisions. This note is research, not an
approved implementation specification.

## Conclusion

LangGraph's useful lesson is not “store a workflow as a graph.” It is that a
workflow definition, a durable run, and the side effects produced by that run
are three different things.

Git-backed Markdown can remain the reviewed, versioned authority for Evie's
procedures. It cannot alone provide checkpointed run state, atomic node-status
transitions, durable interrupts, exclusive run ownership, retry accounting, or
idempotency receipts. Those semantics require transactional runtime state.

Evie should adopt a small Go-native declarative workflow model backed by its
existing SQLite and event/lease patterns, not add LangGraph as a runtime
dependency. LangGraph's official core package is a Python package that requires
Python 3.10 or newer, and there is a separate JavaScript implementation
([Python package](https://github.com/langchain-ai/langgraph/blob/main/libs/langgraph/pyproject.toml),
[JavaScript documentation](https://docs.langchain.com/oss/javascript/langgraph/overview)).
A dependency would therefore add a second-language execution and persistence
boundary to a design that explicitly keeps Evie as the Go runtime and SQLite as
the local transactional store
([memory.spec.md](../active/memory.spec.md), lines 12-24;
[memory.decisions.md](../active/memory.decisions.md), lines 564-583).

## What LangGraph means

### Graph orchestration

A graph is executable control flow over shared state: **state** is the current
application snapshot, **nodes** are functions that compute or perform side
effects and return state updates, and **edges** select the next node, including
fixed transitions and conditional branches
([Graph API](https://docs.langchain.com/oss/python/langgraph/graph-api)). A graph
therefore means more than a diagram or a Markdown checklist.

LangGraph distinguishes a **workflow**, whose path is predetermined by code,
from an **agent**, which dynamically chooses its own process and tool usage
([Workflows and agents](https://docs.langchain.com/oss/python/langgraph/workflows-agents)).
One graph may still mix deterministic nodes, model calls, and agent nodes. The
important boundary is explicit: deterministic business rules remain ordinary
code; model judgment is isolated to named agentic nodes with typed inputs and
outputs.

### Durable execution and checkpointing

With a checkpointer, LangGraph saves thread-scoped graph state as checkpoints at
execution-step boundaries. A checkpoint records current values, the nodes to run
next, metadata, parent checkpoint, and pending tasks. Completed writes from
other nodes in a failed parallel step are retained so those nodes need not run
again
([Persistence](https://docs.langchain.com/oss/python/langgraph/persistence)).
This is the basis for restart recovery, fault tolerance, human review, state
inspection, and replay.

LangGraph exposes `exit`, `async`, and `sync` durability modes. Only `sync`
durably saves a checkpoint before the next execution step starts; `async`
admits a crash-loss window, and `exit` cannot recover a mid-run crash
([Checkpoint durability modes](https://docs.langchain.com/oss/python/langgraph/checkpointers#durability-modes)).
For accounting or payment work, Evie should specify the `sync`-equivalent
boundary: commit the accepted checkpoint and effect receipt before scheduling
its successor. That still cannot make SQLite and a remote API one transaction.

Durability is not “the procedure file still exists.” It is “the runtime can
identify one run, load its last accepted state and next node, distinguish
completed from uncertain work, and continue under a current owner.”

### Interrupts and human-in-the-loop

An interrupt persists graph state, returns a JSON-serializable review payload to
the caller, and waits until a later invocation supplies a resume value. A durable
checkpointer and thread ID are required
([Interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts)). The
pause is therefore a durable run state, not a blocked process or a prose reminder.

On resume, the affected node starts again from the beginning; execution does not
continue from the source line after the interrupt. Code and effects before the
interrupt may run again. LangGraph consequently requires stable interrupt order
and idempotent effects before an interrupt
([Graph API: re-execution and idempotency](https://docs.langchain.com/oss/python/langgraph/graph-api#re-execution-and-idempotency),
[Interrupt rules](https://docs.langchain.com/oss/python/langgraph/interrupts#rules-of-interrupts)).

### Deterministic and agentic nodes

“Deterministic workflow” does not mean every node must return the same bytes on
every new run. It means resuming a particular run follows control flow that can
be reconstructed from persisted inputs and task results. LangGraph requires
non-deterministic work such as model/API calls, time, and randomness to be
isolated in checkpointed tasks; completed results are restored during resume
rather than recomputed
([Functional API: determinism](https://docs.langchain.com/oss/python/langgraph/functional-api#determinism)).

For Evie, a tip formula, validation, reconciliation check, and sheet-row mapping
can be deterministic nodes. A node that interprets an ambiguous exception may
be agentic. The graph must declare which is which so model judgment cannot
silently replace a business rule.

### Replay and idempotency

Checkpoint replay is at-least-once at the node/task failure boundary, not an
exactly-once guarantee for external systems. A node interrupted or retried may
run from its beginning, and a task that started but did not durably finish may
run again. LangGraph directs side-effecting tasks to use idempotency keys,
upserts, or read-before-write checks
([Functional API: idempotency](https://docs.langchain.com/oss/python/langgraph/functional-api#idempotency),
[Graph API](https://docs.langchain.com/oss/python/langgraph/graph-api#re-execution-and-idempotency)).

Deliberate time-travel replay is different from crash recovery: nodes after the
chosen checkpoint execute again, including model calls, API calls, and
interrupts
([Time travel](https://docs.langchain.com/oss/python/langgraph/use-time-travel)).
Evie must name these operations separately. Retrying an incomplete run should
reuse stable side-effect identities; forking or replaying history must not
silently resend payments or repeat other external mutations.

Definition versioning is also part of recovery. LangGraph applies the latest
deployed graph to existing checkpoints and documents the resulting technical
and business compatibility burden
([Backward compatibility](https://docs.langchain.com/oss/python/langgraph/backward-compatibility)).
Evie's reviewed Git procedures make a safer choice available: pin each run to
the approved commit and content hash it started with, and require an explicit
migration or restart to use a newer definition.

## Why Git-backed Markdown is necessary but insufficient

The approved Evie design correctly assigns Git the canonical reviewed procedure
and its human-auditable history
([memory.spec.md](../active/memory.spec.md), lines 156-188 and 844-895;
[memory.decisions.md](../active/memory.decisions.md), lines 333-338). Git can
answer what definition was approved, who changed it, and how to roll it back.

Git alone cannot atomically answer:

- which node of run `R` is accepted and which node is next;
- whether an external call never started, completed, or has an uncertain result;
- which approval payload is pending and which response resumed it;
- which worker currently owns the run and whether its lease is stale;
- which retry attempt used which idempotency key; or
- which checkpoint is safe for recovery rather than deliberate re-execution.

Encoding mutable run files and committing after every step would recreate the
dual-write recovery problem the memory spec already rejects for factual state
([memory.spec.md](../active/memory.spec.md), lines 70-86). Git and an external
side effect cannot share a transaction, and Git is not the current fenced SQLite
owner used by Evie's concurrent processes.

## Small Go-native model to specify

The smallest model that captures the LangGraph semantics is:

1. An approved Git document defines a typed graph: stable workflow ID and
   version, typed state schema, named nodes, fixed/conditional edges, node kind
   (`deterministic`, `agentic`, `side_effect`, or `interrupt`), retry policy,
   required capability, approval policy, and idempotency-key derivation.
2. Activation validates and compiles that document into a canonical immutable
   representation identified by commit and content hash. New activation affects
   new runs; an in-flight run remains pinned.
3. SQLite owns workflow runs, node attempts, accepted checkpoints, pending
   interrupts, resume decisions, leases, and external-effect receipts. A run
   has its own identity, optionally linked to an originating session or
   schedule. State transitions and corresponding episodic events commit
   together where they share the database.
4. A run advances only from an accepted checkpoint under its current fencing
   token. Durable human review uses a workflow-run/step lease, not a
   conversational turn lease held across an indefinite pause. An incomplete
   side-effect attempt becomes `unknown` until a typed reconciliation step
   proves the outcome; it is never blindly replayed.
5. `resume` continues crash recovery with the same run and effect identities;
   `retry` is an explicit new attempt under the node policy; `fork/replay` is a
   separate diagnostic operation that defaults to suppressing external effects.

This is intentionally smaller than LangGraph: no general Pregel runtime,
parallel super-steps, arbitrary Python/JavaScript node code, time-travel UI, or
dynamic graph mutation is needed for the first procedural workflow.

## Exact Evie specification amendments required

The current memory documents do not authorize executable procedural workflows.
Before implementation, amend them as follows:

1. **Purpose and sources of truth** — clarify that Git owns approved workflow
   definitions, while SQLite owns workflow-run/checkpoint state and effect
   receipts. The current four-layer description and source table mention
   procedural instructions but no executable run state (`memory.spec.md`, lines
   12-24 and 156-188).
2. **Invariants** — extend durable background execution (lines 207-209) with a
   pinned definition hash, durable checkpoint/next-node state, fenced run
   ownership, stable idempotency keys, durable interrupts, and explicit
   `unknown` side-effect recovery. Scope invariant 13 (lines 215-218) to
   conversational turns and add a distinct workflow-run/step lease that
   authorizes workflow tool/provider starts. A short turn lease cannot remain
   held across a durable human interrupt.
3. **SQLite schema** — add workflow-definition activation records, runs, node
   attempts, checkpoints, interrupts/resume decisions, run leases, and effect
   receipts. Give every run its own ID, optionally linked to a session or
   schedule; the current event model requires a session ID and explicitly
   defers ambiguous-effect recovery (`memory.spec.md`, lines 509-529). Do not
   reuse `memory_jobs`, which is specified as the compiler's source-range
   outbox (`memory.spec.md`, lines 537-570). Keep
   `procedural_operations` for Git mutation recovery, not run execution (lines
   585-588).
4. **Procedural memory** — define a closed declarative workflow schema,
   validation/canonicalization rules, capability references, version migration,
   and the rule that activation changes future runs only. The existing
   `pending -> applying -> committed` protocol (lines 879-892) describes a Git
   operation, not workflow progress.
5. **Operations and approvals** — add typed local operations to inspect, start,
   cancel, retry, reconcile, and resume a run. A durable interrupt response must
   identify the run, node attempt, definition hash, and reviewed action. Preserve
   the existing rule that memory/procedure content cannot authorize tools or
   change approval requirements (`memory.spec.md`, lines 897-936). Specify three
   separate decisions: approving/activating a definition, authorizing a run or
   schedule to start, and approving a particular consequential external effect.
6. **Uncertain side effects** — amend the decision that unfinished tool intent
   does not block later work (`memory.decisions.md`, lines 602-607). That policy
   may remain for ordinary chat turns, but a procedural run with a consequential
   external effect must block that run at `unknown` until reconciliation.
7. **Build order and seams** — add an independently approved workflow-runtime
   stage after procedural Git definitions rather than silently expanding Stage
   7, whose current done condition is only a versioned, scoped, loadable lesson
   (`memory.spec.md`, lines 1130-1150). Add consumer-owned Go interfaces and
   packages for definition compilation, run coordination, checkpoint storage,
   reconciliation, and scheduling to the anticipated seams (lines 1231-1263).
8. **Replay terminology** — reserve semantic-operation replay for the exact,
   no-model projection rebuild already required at lines 1271-1272. Name
   workflow `resume` as continuation from the latest durable boundary, and name
   workflow time-travel `replay` as deliberate re-execution of later nodes that
   is unsafe for external effects without idempotency or a sandbox.
9. **Definition of done** — add deterministic restart tests at every node
   boundary; interrupt/approve/reject/restart tests; stale-runner fencing; pinned
   definition compatibility; duplicate-delivery/idempotency tests; and
   crash-before, crash-during, and crash-after external-effect reconciliation.

These amendments preserve the current decisions that Evie owns its lifecycle in
Go/SQLite, background work is durable and fenced, procedural changes are
reviewed, and procedure text cannot widen runtime authority.
