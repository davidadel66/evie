# memory delivery initiative

Status: approved by David on 2026-08-21; MEM-1 is in progress

## Outcome

Deliver durable, restart-safe, scoped local memory for Evie with canonical
episodic evidence, explicit temporal semantic state, asynchronous candidate
extraction, hybrid retrieval, reviewed procedural memory, web parity, and
versioned evaluation.

This is a multi-epic initiative. The approved specification defines behavior;
this directory only divides that behavior into independently reviewable delivery
stories.

## Non-goals

The core initiative does not add authentication, cloud sync, backups, session
branching, inherited parent-session memory, implicit cross-project imports, a
remote graph service, or a provider-neutral replacement for OpenRouter. It does
not implement hard erasure. Automatic claim admission remains disabled until an
evaluation-backed policy is approved.

Optional research-topic workspaces from Stage 10 are a separate follow-on
initiative and are not part of core memory completion.

## Sources

- [Approved memory specification](../memory.spec.md)
- [Binding memory decisions](../memory.decisions.md)
- [Request-planning workflow](../../../../../docs/request-planning.md)
- Existing Stage 1 implementation in `internal/agent`, `internal/eviedb`,
  `internal/memory`, `internal/openrouter`, and `internal/tools`

## Current state

The Stage 1 project/session/event schema, immutable scope value, durable history
projection, before-action event ordering, cancellation propagation, and generic
file/SQL storage fences are implemented and tested. They are existing foundation,
not future stories.

The REPL still creates a fresh global session at startup. Durable turn leases,
safe session resume, execution recovery, provider replay policy, and provider
usage capture remain in MEM-1.

The Stage 0 reference-system research note required by the specification was not
found in the repository. It remains a bounded evidence gap that must be closed
before Stage 1 is declared complete.

## Epics

| ID | Epic | Depends on | Status |
|---|---|---|---|
| MEM-1 | [Restart-safe scoped sessions](epics/mem-1-restart-safe-scoped-sessions.md) | Existing event spine | In progress |
| MEM-2 | [Bounded working context and compaction](epics/mem-2-working-context-and-compaction.md) | MEM-1 | Approved; not started |
| MEM-3 | [Explicit temporal semantic graph](epics/mem-3-explicit-temporal-semantic-graph.md) | MEM-2 | Approved; not started |
| MEM-4 | [Durable asynchronous candidate compiler](epics/mem-4-durable-candidate-compiler.md) | MEM-3 | Approved; not started |
| MEM-5 | [Hybrid sourced retrieval](epics/mem-5-hybrid-sourced-retrieval.md) | MEM-4 | Approved; not started |
| MEM-6 | [Revision-safe graph acceleration](epics/mem-6-revision-safe-graph-acceleration.md) | MEM-5 | Approved; not started |
| MEM-7 | [Reviewed procedural Git memory](epics/mem-7-reviewed-procedural-git-memory.md) | MEM-6 | Approved; not started |
| MEM-8 | [Persistent web-session integration](epics/mem-8-persistent-web-session-integration.md) | MEM-7 | Approved; not started |
| MEM-9 | [Evaluation and policy tuning](epics/mem-9-evaluation-and-policy-tuning.md) | MEM-8 | Approved; not started |

## Recommended order

Follow MEM-1 through MEM-9 in order. The specification requires each stage to be
tested and approved before the next begins, even where parts of two epics could
technically be developed independently.

The smallest useful next story is MEM-1.1, explicit REPL scope selection and new
scoped sessions. It follows the current implementation seam without exposing
existing-session resume before durable lease ownership exists.

## Story workflow

Story summaries live in their epic files. A story becomes ready for
implementation only after its dependencies and material decisions are complete
and David selects it. The selected story then receives a GitHub issue containing
its full execution contract, active discussion, status, acceptance criteria,
verification, and pull-request link.

Planning approval does not authorize product-code implementation.

## Open decisions and research spikes

- Complete the Stage 0 Letta, MemGPT, and Generative Agents adaptation note before
  closing MEM-1.
- MEM-1.R1 decides provider continuation behavior without persisting unapproved
  opaque state.
- MEM-2.R1 decides context budgets and compaction failure behavior.
- MEM-3.R1 records the Graphiti comparison and graph encodings before graph DDL.
- MEM-4.R1 selects a local extractor and structured-output protocol.
- MEM-5.R1 selects the embedding model and vector implementation from measured
  evidence.
- MEM-6.R1 sets performance targets before cache optimization.
- MEM-8.R1 amends the serve specification and decisions before web integration.
- MEM-9.R1 may approve admission and retrieval-policy changes only from versioned
  evaluation evidence.
- Hard-erasure semantics remain deferred and do not block the core initiative.
- Optional research-topic workspaces require their own initiative approval and
  unresolved artifact-policy decisions.

## Approval record

- 2026-08-21: David approved the core initiative, nine-epic breakdown, story
  boundaries, dependency order, and MEM-1.1 as the recommended next story.
- 2026-08-21: David authorized creation of these planning artifacts and the
  project-backlog link. No implementation story has been selected yet.
