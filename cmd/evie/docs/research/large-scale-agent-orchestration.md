# Large-scale agent orchestration: what 10,000 workers actually requires

**Research date:** 2026-09-10
**Status:** Evidence and backlog material only. This note does not authorize
implementation, change the active Subagents specification, or modify its
issues. Online claims use first-party documentation or published first-party
experiments. Recommendations and numerical examples are identified separately.

## Finding

An orchestrator can direct 10,000 logical assignments through a distributed
execution system. AWS documents that scale for parallel child workflows. That
establishes scheduling capacity, not that 10,000 autonomous LLM agents can
collaborate effectively on any single problem. The practical distinction is
between a large collection of independently checkable work items and a tightly
coupled task whose workers continually depend on one another.
([AWS Distributed Map](https://docs.aws.amazon.com/step-functions/latest/dg/state-map-distributed.html))

**Inference:** The scalable form is a planner directing a durable, bounded
execution system. The planner need not maintain 10,000 active conversations or
issue 10,000 model-generated spawn calls. It can submit a partitioning plan or
work manifest and inspect aggregated progress, exceptions, and evidence.

## Distinguish the quantities

These are separate design quantities, not interchangeable claims:

| Quantity | Meaning | What must be demonstrated |
| --- | --- | --- |
| Total logical assignments | Distinct work items admitted over an entire task | Identity, deduplication, progress, retained results |
| Simultaneously active workers | Assignments executing or waiting on external operations at once | Capacity, ownership, cancellation, storage and connection load |
| Concurrent model requests | Calls currently using a model provider | Provider concurrency, request/token quotas, burst behavior |
| Coordinating agents | LLM agents exchanging findings or steering dependent work | Useful decomposition, information fidelity, reconciliation and answer quality |

A worker can make many model calls over its lifetime. An active worker can also
be waiting for a capability result while making no model request. These
definitions are used throughout this note; they are not benchmark results.

## What the primary sources demonstrate

### AWS: 10,000 parallel child workflows, with separate histories

Distributed Map starts an independent child workflow for each input item or
batch. Children have separate execution histories, and `MaxConcurrency`
controls parallel execution. The documentation cautions that configured
parallelism must fit downstream service capacity. This is appropriate evidence
for bulk orchestration of independent work items; a workflow invocation does
not itself establish an autonomous agent or a successful agent collaboration.
([Distributed Map](https://docs.aws.amazon.com/step-functions/latest/dg/state-map-distributed.html))

The quota page specifies a hard maximum of **10,000 parallel children per Map
Run**. It separately lists dispatch rates of up to **1,000 Express executions
per second** and **100 Standard executions per second**. It also lists **1,000
open Map Runs**. A headline concurrency limit therefore does not imply instant
launch, unlimited workload admission, or sufficient model-provider capacity.
([Step Functions quotas](https://docs.aws.amazon.com/step-functions/latest/dg/service-quotas.html))

Result handling has a separate limit: returning an oversized collection of
child results can exceed the 256 KiB state output limit. AWS provides
`ResultWriter` to export child execution results to S3, including execution
identity and status, instead of forcing the whole collection through the
parent state payload.
([ResultWriter](https://docs.aws.amazon.com/step-functions/latest/dg/input-output-resultwriter.html))

### Ray: bound pending work separately from running work

Ray documents backpressure with `ray.wait`: when submissions outpace
processing, pending work can accumulate until memory is exhausted. The pattern
limits outstanding submissions. Ray explicitly distinguishes this from
controlling running-task concurrency, for which it recommends resource
requirements rather than using the pending-work limit as the scheduler.
([Pending-task backpressure](https://docs.ray.io/en/latest/ray-core/patterns/limit-pending-tasks.html))

Ray's running-task pattern schedules according to declared resource needs to
avoid admitting more work than a node's resource budget. Its memory resource is
a logical scheduling constraint, not a physical memory limit enforced against
a misbehaving task.
([Running-task admission](https://docs.ray.io/en/latest/ray-core/patterns/limit-running-tasks.html))

Ray also documents supervisor trees: a driver talks to a few supervisors, each
of which delegates to workers and handles their failures. More than one
supervision level is supported. This is a concrete distributed programming
pattern, not a measurement of LLM teamwork at 10,000-agent scale.
([Supervisor actors](https://docs.ray.io/en/latest/ray-core/patterns/tree-of-actors.html))

**Inference:** Evie would need separate controls for queued assignments, active
worker execution, and model/capability traffic. A goroutine semaphore controls
only whichever operation it surrounds. It does not automatically bound the
pending queue, aggregate token usage, or another process's traffic.

### Anthropic: useful multi-agent research has bounded delegation and results

Anthropic's June 2025 Research description uses a lead agent coordinating
parallel workers with separate contexts. It describes three to five parallel
subagents as a speed improvement, and more than ten for complex research with
distinct responsibilities. Workers return findings, and the article recommends
persisting large artifacts externally and passing lightweight references.
It reports roughly fifteen times chat token usage for its multi-agent systems
and warns that tightly dependent work is a poor fit. These are observations
from that system, not universal efficiency or cost ratios. The article does
not demonstrate 10,000 collaborating LLM agents.
([Multi-agent Research system](https://www.anthropic.com/engineering/multi-agent-research-system))

### A coding example shows why total sessions and parallel agents differ

Anthropic's February 2026 compiler experiment used **16 agents** across nearly
**2,000 sessions**. Its author explicitly reports using no orchestration agent.
Independent failing tests initially provided parallel work; when agents all
hit the same kernel-compilation bug, they duplicated fixes and overwrote one
another. A test strategy using GCC and partitioned file subsets restored useful
parallelism. This is evidence that decomposition and verification can determine
whether more workers help. It is neither a 2,000-concurrent-agent experiment
nor a 10,000-agent result.
([C compiler experiment](https://www.anthropic.com/engineering/building-c-compiler))

### Cursor: thousands over a run, hundreds concurrently

Cursor's February 2026 experimental harness used recursive planners and
workers with separate repository copies and single return handoffs. It reports
many thousands of agents over runs, with peaks of several hundred concurrently
on one large Linux VM. Memory and then build/disk traffic constrained
throughput. Flat shared-file coordination performed poorly. The experiment
accepted temporary correctness regressions, so its throughput is not evidence
that Evie should weaken verification or that 10,000 agents ran simultaneously.
([Self-driving codebases](https://cursor.com/blog/self-driving-codebases))

## Inferred architecture for a large Evie run

The following is a design recommendation derived from the evidence, not a
description of implemented Evie behavior or an amendment to its approved work.

1. **Plan and partition.** The parent proposes distinct assignments with input
   references, an output contract, a verifier, and explicit dependency edges.
   The Kernel validates the plan, scope, preset eligibility, and total budget
   before admitting a bounded collection of work.
2. **Persist admission.** A durable execution record tracks the overall run;
   each assignment has a stable identity, pinned composition, state, attempts,
   ownership, cancellation state, and result reference. Queue entries refer to
   these records. A lost worker must not make accepted work disappear.
3. **Schedule a bounded working set.** Worker processes claim ready assignments
   under leases. Pending-work limits, fair admission, and provider/capability
   quotas are separate controls. Ten thousand assignments may be processed by
   a much smaller number of concurrent workers.
4. **Execute within Kernel authority.** Workers consume their selected
   capabilities and resource grants. Delegation does not grant broader
   credentials or allow the model to bypass admission policy. A remote worker
   must have its authority rechecked when work or results are accepted.
5. **Store evidence outside the parent's prompt.** Results include bounded
   findings, provenance, and references to retained artifacts. A result index
   supports selective inspection and re-evaluation without copying all child
   transcripts into a single conversation.
6. **Aggregate in bounded stages.** Deterministic aggregation handles counts,
   validation, duplicates, and ordering. Introduce intermediate LLM reviewers
   only when synthesis requires judgment. Preserve source references across
   reductions so compression does not erase contradictory evidence.
7. **Recover and terminate explicitly.** Per-assignment retry policies,
   idempotency, cancellation propagation, durable accepted results, and
   ownership-aware recovery prevent duplicate execution or a stale worker
   overwriting a newer outcome. Inspect progress through aggregate state and
   exceptions rather than an unbounded stream of every child event.

One possible shape is a coordinator over partition supervisors and leased
workers, followed by several bounded result-reduction stages. The partition
supervisors can be ordinary scheduling code; they do not all need their own
LLM context. This follows the distinction between supervision structure and
agent reasoning demonstrated by Ray's
[supervisor pattern](https://docs.ray.io/en/latest/ray-core/patterns/tree-of-actors.html).

## Scaling should be demonstrated, not inferred from a spawn count

**Recommended evaluation, not an existing benchmark:** start with a workload
that genuinely contains many independent items, such as evaluating an evidence
set or extracting a fixed schema from a document collection. Compare a single
worker, a small worker pool, and larger pools on the same task and quality
criteria. Measure accepted useful results per unit of time and cost, duplicate
work, provider throttling, queued age, storage latency, and recovery behavior.

Before asserting 10,000-worker readiness, exercise a 10,000-item deterministic
run with bounded concurrency and injected failures; then separately measure
the intended active-worker and provider-request concurrency. Kill workers and
supervisors, delay persistence, deny capacity, return partial results, and
verify that accepted outcomes remain consistent. Passing a synthetic scheduler
test would establish infrastructure behavior, not the quality of a
10,000-agent reasoning strategy.

## Evie assessment and quantitative sizing

The accompanying codebase assessment and capacity calculations belong here;
the online sources above alone cannot establish Evie's implemented capacity.
No 10,000-worker Evie benchmark or runtime capability is asserted by this note.
