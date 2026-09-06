# Memory Stage 4: compiler design research

Research date: 2026-09-04. Status: evidence and proposed design questions, not
accepted requirements. No model has been installed, started, or benchmarked.
Online source branches are moving references; the implementation spike must pin
the tested runtime version and model artifact digest.

## Existing contract

The [memory roadmap](../active/memory.spec.md#memory-compiler) already requires a
local-only extractor, bounded source projection, exact evidence validation,
durable jobs and leases, generation-specific backfill, candidate review, and no
automatic admission. Its Stage 4 outcome is asynchronous candidate production
that survives restart. The [decision record](../active/memory.decisions.md)
additionally binds failed/cancelled head ranges to blocking later commits until
an explicit retry or reasoned skip. That ordering rule cannot simply be omitted
in implementation; changing it requires an explicit amendment.

Stage 3 separates accepted semantic state from extraction and establishes exact
revision checks and deterministic accepted-operation replay. Stage 4 should use
that authority boundary when an owner approves a Candidate.

## Local environment observations

Read-only checks on this task's machine returned:

| Check | Observation |
| --- | --- |
| `sysctl machdep.cpu.brand_string hw.memsize hw.physicalcpu hw.logicalcpu` | Apple M3 Pro; 19,327,352,832 bytes = 18 GiB RAM; 11 physical and 11 logical CPUs |
| `sw_vers` | macOS 15.7.8, build 24G809 |
| `command -v ollama llama-server lmstudio lms` | None found on the shell's PATH; this does not establish that no GUI runtime is installed |
| Read-only GET of `http://127.0.0.1:11434/api/version` | Connection unavailable from this task; no running Ollama endpoint was verified |

The initial sandboxed CPU query was denied; the same read-only query succeeded
through automatic approval. No live Evie database, secrets, or personal model
conversation history was inspected. RAM capacity is known; available RAM,
inference speed, GPU allocation, memory pressure, and battery effects were not
measured.

**Recommendation:** begin with one loaded model and one in-flight inference call
across Evie processes. Compare compact quantized configurations that fit a
measured memory budget under normal desktop workload. CPU count is not a
reasonable default for model worker count. Keep runtime installation and model
selection as spike outcomes.

## What the primary sources establish

### SQLite concurrency

WAL allows concurrent reading and writing but still permits only one writer.
Readers retain a snapshot; a long read transaction can prevent checkpoint
completion and grow the WAL. Automatic checkpointing can make some commits
slower. WAL also assumes processes on one host, which fits Evie's present local
architecture. [SQLite WAL documentation](https://sqlite.org/wal.html)

**Design inference:** load immutable extraction input in a short read transaction,
close it before inference, and persist candidates in a short write transaction.
Paginate reconciliation instead of scanning and enqueuing all history inside one
transaction. More worker goroutines do not remove writer contention. Measure
foreground event-commit p95/p99 while compiler persistence, review, and backfill
run concurrently. The requirement should say that the turn never waits for model
extraction and define an acceptable enqueue overhead; a literal promise that
background work never slows a response is not mechanically attainable on shared
CPU, storage, and SQLite resources.

### Structured output is a shape constraint

Ollama documents JSON-schema output and OpenAI-compatible `response_format`,
recommends providing schema information in the prompt, and demonstrates parsing
the returned value with application validation. It recommends low temperature
for more consistent completions. [Ollama structured output documentation](https://docs.ollama.com/capabilities/structured-outputs)

llama.cpp supports a subset of JSON Schema. Its documentation warns that
unsupported features may be silently skipped, including some useful constraints,
and that the schema is not automatically inserted into an ordinary completion
prompt. [llama.cpp grammar documentation](https://github.com/ggml-org/llama.cpp/blob/master/grammars/README.md)

**Design inference:** the Go decoder and semantic validator remain authoritative.
A syntactically valid claim can cite invented evidence, reverse a relation,
misidentify a speaker, mistake a hypothetical for fact, or invent valid time.
Even a byte-exact quote only proves that the text exists; it does not prove that
the proposition follows from it. Separate schema success, evidence-match success,
and supported-claim precision in the evaluation report.

Use small explicit schemas with bounded arrays/strings and an abstain/empty
result. Reject trailing content, unknown fields, invalid IDs, oversized values,
unsupported enum values, and invalid references in Evie. Let the model select
supplied evidence IDs and exact quotes; Evie derives/verifies hashes and canonical
anchors. Never ask the model to calculate authoritative hashes or accept its
own evidence verdict.

### Local throughput and cancellation

Ollama documents that simultaneous requests increase context-cache memory; RAM
requirements scale with parallel requests times configured context length. When
memory is insufficient, requests queue. It exposes limits for loaded models,
parallel requests, and queue size. [Ollama FAQ](https://docs.ollama.com/faq)

Go request contexts control the outgoing HTTP request and response lifetime.
[Go `net/http` documentation](https://pkg.go.dev/net/http#NewRequestWithContext)
llama.cpp's HTTP implementation passes connection-closed detection into handlers,
and its task processor has a cancellation case that releases the matching slot.
[llama.cpp HTTP source](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/server-http.cpp),
[llama.cpp task source](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/server-context.cpp)
Its documented prediction-time limit begins after the first token and depends on
a newline having been generated; it is not an end-to-end request timeout.
[llama.cpp server documentation](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md)

**Design inference:** separately test (1) prompt return to the caller, (2) durable
cancellation/lease fencing preventing a late commit, and (3) server slot/compute
release. An HTTP client timeout proves neither immediate GPU release nor that a
replacement call has capacity. Test cancellation during queuing, prompt
processing, and generation against the actual pinned runtime. Independently cap
input, output, request duration, attempts, and total job work. Keep authoritative
pending work in Evie rather than submitting a large opaque backlog to the model
server.

### Useful Graphiti ideas and mismatches

Graphiti separates node extraction/resolution from edge extraction/resolution
and supplies previous episodes to extraction. Its API exposes bounded concurrency
and a last-N episode retrieval interface. [Graphiti orchestration source](https://github.com/getzep/graphiti/blob/main/graphiti_core/graphiti.py)
Its semaphore helper bounds each gather invocation; the implementation creates
that semaphore inside the helper, which is not evidence of a service-wide
inference limit across separate callers or processes. [Graphiti concurrency helper](https://github.com/getzep/graphiti/blob/main/graphiti_core/helpers.py)

The inspected node-resolution implementation retrieves bounded similarity
candidates, applies deterministic matching, and sends unresolved cases to a
model. It defensively checks returned resolution IDs. It also falls back to
attributing a node to all supplied episodes when model episode attribution is
empty or invalid. That fallback conflicts with Evie's exact evidence contract.
[Graphiti extraction and resolution source](https://github.com/getzep/graphiti/blob/main/graphiti_core/utils/maintenance/node_operations.py)

**Recommendation:** borrow separate observable pipeline steps and bounded
candidate lists. Preserve Evie's exact/lexical Stage 4 baseline and quarantine
ambiguous identity. Do not copy fallback evidence attribution, dense retrieval
dependencies, or automatic graph mutations. Treat each added model call as a
measured cost; start by testing one bounded extraction call plus deterministic
validation/resolution before introducing model reflection passes.

## Proposed Stage 4 amendments and decisions

1. **Separate compiler progress from accepted knowledge.** Consider allowing
   independent jobs to persist candidates out of order, with durable completed
   ranges, gaps, and a contiguous coverage frontier. Failed work remains visible
   and does not become silently covered. Owner approval preserves Stage 3's exact
   proposal/revision safeguards through a new durable Candidate acceptance
   contract; it cannot require the original conversation to remain active.
   Strict ordering for future automatic accepted
   effects can be designed when that policy exists. This changes the binding
   blocked-head rule and needs approval; the alternative is to preserve it while
   clearly explaining its head-of-line freshness cost.
2. **Define the source unit before choosing context length.** Prefer completed
   root turns within one immutable session/scope, with bounded preceding original
   evidence for pronouns and corrections. Never blend unrelated sessions solely
   because they share scope. Distinguish the new coverage range from contextual
   evidence so overlap does not produce duplicate jobs or false coverage. Specify
   oversized-turn splitting, maximum coalescing delay, and zero-candidate success.
3. **Make authority an extraction-policy choice.** Begin by evaluating direct user
   assertions separately from assistant/tool content. Assistant text may repeat
   hallucinations or previously retrieved claims; syntactic eligibility is not
   sufficient independent support. Recommend direct-user evidence for the first
   production slice, then admit individually evaluated tool fields. Broader
   evidence remains a later expansion if the owner agrees to narrow Stage 4.
4. **Bound every growth axis.** Define maximum event bytes, prompt budget, output
   size, entities/claims per job, lexical alternatives per mention, queued/staged
   work per process, and pending review volume. Stage results concurrently only
   under a shared endpoint capacity limit. Schedule live turns and historical
   backfill fairly so either cannot starve the other.
5. **Treat activation and recomputation as explicit operations.** Pin extractor
   artifact/runtime, prompt, schema, projection, chunking, and resolution versions
   in the generation manifest. Separate this semantic configuration from
   operational worker-count settings that should not require re-extraction.
   A new generation may produce a distinct candidate group; do not automatically
   replay years of history merely because the process starts with a new model
   tag. Show estimated work and allow bounded backfill selection. Never rewrite
   previously accepted operations when recompiling.
6. **Make retry classes explicit.** Temporary endpoint unavailability can back
   off; invalid schema configuration, permanent evidence-validation failures,
   and unsupported endpoint behavior need a diagnostic fix. Distinguish transport
   retries from any permitted model repair attempt so the five-attempt limit is
   not multiplied invisibly across pipeline steps. Exact valid output that the
   graph has outgrown should normally trigger deterministic revalidation, not a
   second extraction call.
7. **Measure review capacity alongside extraction capacity.** Track duplicate,
   rejected, edited, accepted, and deferred candidates; review seconds per useful
   accepted claim; and oldest unreviewed candidate age. Preserve reviewer edits
   as new proposals linked to original candidates. Specify how unchanged rejected
   suggestions are grouped across generations without erasing the audit history.

## A bounded first spike

Use only synthetic or deliberately redacted fixtures. Freeze a manifest before
comparison: normal assertions, empty/no-memory turns, pronouns spanning turns,
same-name entities, corrections versus real-world changes, negation, uncertain
time, quotes/hypotheticals, hostile embedded instructions, exact Unicode anchors,
long inputs, and intentionally malformed/truncated output.

Compare a small number of pinned local configurations on identical inputs. Report
per-case failures and separate precision/recall for claims and entity links,
exact evidence success, schema rejection, abstention, repeated-run variation,
warm/cold latency, prompt/output tokens, peak memory, server cancellation latency,
and normal foreground responsiveness. Set the model-backed quality thresholds
before the held-out comparison; infrastructure invariants remain deterministic
pass/fail requirements. A parser-only pass does not select a model.

The first production gate should demonstrate that the compiler keeps up with a
declared daily workload while foreground event persistence meets a chosen p95
overhead target and the review queue is usable. Also replay a larger synthetic
backfill to reveal storage and queue growth. These are measurements to collect,
not claims that the present machine already meets particular throughput targets.

This brings extraction/resource evaluation into Stage 4. Stage 5 still owns
retrieval-quality evaluation, and later end-to-end evaluation tests whether memory
improves actual answers.

## Verification of this research change

Documentation-only change. `git diff --check` passed (exit 0).
`git diff --no-index --check /dev/null cmd/evie/docs/research/memory-stage-4-compiler-design.md`
reported no whitespace errors (exit 1 because the new file differs from
`/dev/null`). Full Go/UI checks and model-backed experiments are not appropriate
for this research-only change and were not run. Existing working-tree changes
are outside this note's ownership.
