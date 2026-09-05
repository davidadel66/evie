# Bounded candidate compilation engineering checkpoint

This is implementation evidence for ticket #136 under the active Stage 4 spec
and its evidence, work, and review contracts. It does not select a model or
change the quality gate. The complete ticket remains pending the selected local
wire integration and an actual selected-generation acceptance observation.
There is no installed compiler configuration, default model, activation,
automatic retry, supervisor, acceptance command, backfill interface, or UI in
this ticket. Explicit Stage 3 memory remains independent of extraction.

## Kernel boundary

`Store.CompileCandidateUnit` takes an exact local owner Scope Context, one
finite source-session/root/cutoff selection, the complete pinned generation,
and a `CompilerExtractor`. Durable selection precedes closure and resource
admission. A repeated covering selection reuses its original job and attempt
budget, including after reopen. A later root extension owns a disjoint suffix.

Source capture and closure use one `BEGIN IMMEDIATE` decision. Only committed
owner content can support this slice; assistant content has usage `context`
and authority `none`. Other tools, payloads, compaction, and diagnostics never
enter extraction as factual or hidden context. An observable final assistant,
valid terminal, later root, or serialized no-live-lease observation closes a
prefix; no terminal or outcome is invented. The limits are 32 KiB/64 new support,
8 KiB/16 overlap, 4 KiB/8 assistant context, and 128 inspected source events.
Whole overlap fields are chosen newest first and stop at the first exceeded
bound. Earlier current-root material consumes one of the two overlap units.

A versioned detector excludes an entire secret-bearing content field before
input. Validation and inspection recheck the full original field, even when the
candidate cites a smaller range. Range projection uses canonical half-open UTF-8
byte coordinates and exact SHA256; it never falls back to full event content.
The Kernel binds source type, role/authority, session/scope, sequence, observation
time, format version, usage, and exact quote. A candidate's latest required
support must be new. Structure and exact source identity do not prove entailment,
correct attribution, usefulness, or interpretation.

Accepted Entity and reviewed Predicate context is bounded and deterministic.
Owner/context anchors are prioritized, at most 32 Entity and 32 Predicate
records are offered within independent 8 KiB budgets, and omitted accepted
context is explicit. Oversized or secret-bearing identity/vocabulary records
are omitted as whole records, never truncated into another identity. Unaccepted
candidates are absent from this snapshot. IDs must be offered; the extractor
cannot create identities, Predicates, scope grants, authority, or accepted effects.

## Durable work and visibility

An event-insert trigger allocates its database-wide append position in the same
transaction. The side-record view labels retained pre-install events `legacy`
with no invented position. Installation works while compilation is unconfigured;
later scheduling obligations can join this same event transaction.

Generation, window, request, staging, and original candidate records have seals.
A claim consumes an attempt and acquires a monotonically increasing fence, a
30-second database-time lease renewed every 10 seconds, and the shared SQLite
request reservation. No transaction spans inference. Caller cancellation fences
before signalling the local request, with bounded cleanup. Caller cancellation
remains active through staging, publication lock waits, and COMMIT. A cancelled
unpublished stage remains intact and unconsumed behind the incremented fence;
detached contexts are used only for bounded cleanup and its inspection. Stale
output cannot publish. A complete validated envelope stages atomically; publication creates
the entire at-most-16-candidate group, exact coverage, and stage-consumption
receipt together. Empty is successful only for an explicit valid empty array.
Malformed, missing, truncated, oversized, and unauthorized output cannot become
empty completion. Jobs retain failure/gap states and their attempt count.

Admission checks unfinished-job, unpublished-stage, byte, and unresolved inbox
bounds. Each unpublished stage continues reserving 16 presentation slots after
inference capacity is released. Equivalent complete source/effect envelopes use `compiler-equivalence-v1`: a
versioned encoding sorts copies of supporting/context reference sets and source
manifests. Reference ordering alone cannot create another actionable item. The
original extraction order remains immutable, and equivalents link to a primary
candidate without copying or changing its review authority. Original envelopes are
immutable; initial review state is unresolved and revision zero. Accepted queries,
operations, scope revisions, traversal, and replay remain separate. Generic SQL
allowlisting denies every compiler table/view; storage remains in the existing
protected Evie DB and its protected WAL/SHM files.

## Pinned local transport and remaining integration

The current adapter implements canonical `CompilerResponse` transport through
Ollama generate. It accepts only an explicit literal loopback HTTP endpoint,
disables proxies and redirects, verifies runtime version, tag **manifest** digest,
manifest-to-model-artifact binding, quantization, template, empty server system
prompt, and tokenizer metadata hash. The generation retains exact model manifest
bytes (base64 in configuration JSON), their digest, the distinct model artifact
digest, tokenizer/template/prompt/schema/decoding identities, conservative token
proof identity and limits, and all policy versions. Tokenizer metadata hashes
use compact Go JSON encoding with sorted object keys and unchanged array order.

Verbose runtime metadata has a separate 32 MiB ceiling. Inference input, model
response, and staged envelope each retain the 128 KiB ceiling. Rendering executes
the pinned template under a bounded writer; actual rendered bytes must satisfy
the pinned conservative token upper bound plus output and template reserves.
Unsupported templates or unverifiable context bounds fail before generation.
There is one generate call, no repair loop, remote fallback, download, or runtime
installation.

A contracted complete `done` response proves release; a trusted adapter's
`not_dispatched` result also permits releasing a claim that made no inference.
Timeout/disconnect/client return cannot prove server release. Their durable slot
remains `release_pending`, and all later inference is capacity-blocked. The
endpoint/version/model identity string is **not** a boot identity or proof of
restart. This external Ollama adapter has no request-specific idle/cancel API or
controlled-process restart mechanism. A later recovery implementation must
verify an owned old process ended and a new process identity started, or leave
capacity blocked; timers and replacement configuration are insufficient.

The #135 measured compact formats use sealed short references rather than asking
the model for canonical IDs and hashes. Mapping the eventual selected compact
wire into the canonical Kernel seam is still pending. The current canonical
adapter and synthetic local HTTP fixtures are engineering verification, not an
adequate-model claim or measured acceptance result.

## Inspection and verification

The short commands execute before conversational provider construction:

- `evie memory-compile --session SESSION --root EVENT --cutoff SEQUENCE --config PINNED_CONFIG`
- Add `--session-scope` only for the narrower source-session destination.
- `evie memory-candidates inspect --session SESSION --id SELECTION_OR_JOB`

No PINNED_CONFIG is installed by this change. Inspection is read-only, works for
closed source sessions, and displays exact evidence, authority, destination,
generation, unresolved review metadata, completion/failure, and capacity state.
Authorization or current source-eligibility failure returns no protected payload.

Focused public-boundary checks use actual temporary SQLite, scripted extraction,
local HTTP fixtures, and the CLI parser. They cover group/empty/reopen identity,
source and Unicode projection, scope errors, secret exclusion, event/byte/context
bounds, legacy append positions and rollback, growing accepted vocabulary,
atomic publication failure, outstanding stage reservations, unknown release,
cancellation ordering and publication-lock/commit races, an equivalence encoding
golden hash and reordered-reference review preservation, strict JSON, pinned model/manifest separation, verbose
metadata, redirect/nonlocal rejection, timeout/truncation, and generic SQL fences.
Repository-wide verification and independent reviews are coordinated by the root
owner against the isolated #136 snapshot so concurrent #140 work is not included
in this ticket's commit. Actual-model acceptance remains pending as stated above.
