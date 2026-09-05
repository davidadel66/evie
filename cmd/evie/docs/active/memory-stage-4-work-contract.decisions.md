# Memory Stage 4 durable work and coverage contract

Status: binding technical contract, 2026-09-04. This records the durable-work
prerequisite for [ticket #133](https://github.com/davidadel66/evie/issues/133),
under the [Stage 4 specification, issue #131](https://github.com/davidadel66/evie/issues/131),
[memory decisions](memory.decisions.md), and the approved
[evidence and closure contract](memory-stage-4-evidence-contract.decisions.md).
The [worked scenarios](memory-stage-4-work-contract.fixtures.md) are acceptance
oracles for the dependent persistence and worker tickets. No production code,
general workflow engine, cleanup, or automatic acceptance is introduced here.

## Recorded choices

The parent decisions already require explicit activation, separately selected
history, preserved review decisions, one local inference request, and retained
episodes. The rules below resolve their technical implementation; they do not
reopen D1–D3 of the evidence contract. There are no new material owner choices.

- Coverage belongs to one generation and exact selected source intervals.
  Independently completed later work can be reviewed while an older gap remains.
- Initial scheduling has no explicit skip action. Cancellation, exhaustion,
  invalid sources, and oversized input remain visible gaps, never completion.
- Exact repeated suggestions link to the original review without copying its
  authority. Different evidence or effects require fresh review.
- Nothing here deletes episodes, generations, candidates, edits, review history,
  accepted operations, or their source/context manifests. Resource backpressure
  pauses admission; it does not erase retained work to make space.

## Source coordinates, selection, and generation identity

An event retains its immutable event ID and `(session ID, sequence)` coordinate.
A source unit is one root-turn prefix or incremental suffix with a captured
inclusive last sequence, its exact root-member event IDs, closure reason, source
lineage, and selected destination scope. Root membership follows ancestry and
`turn_id`, as #132 defines. An interval's control/excluded events are accounted
for explicitly; they never become support. Overlap and interpretation context
do not acquire coverage. Units never combine sessions.

Scheduling needs a durable append position across sessions, not a fabricated
cross-session conversation. On installation of the Stage 4 persistence schema,
new event transactions allocate a monotonically increasing database-wide
`commit_position` in a side record bound uniquely to event ID. Rollback commits
neither record. Retained pre-installation events have the explicit `legacy`
cohort, retain their session sequence, and are historical. Do not infer commit
order from timestamps, UUIDs, SQLite rowids, or a migration's scan order.
Within a session, sequence remains the source order. Across sessions, live
scheduling uses commit position; historical enumeration uses bytewise session
ID then sequence. These orders choose work, never establish conversational or
semantic dependence.

A Compiler Generation pins a canonical manifest containing model artifact
identity/digest and quantization, runtime/protocol compatibility version,
tokenizer/chat template, prompt bytes, extraction schema, decoding parameters,
evidence/secret/closure/window policies, Predicate vocabulary and Entity-context
selection policy, output validation, equivalence policy, and semantic effect
contract versions. Hash a versioned canonical encoding and retain the full
manifest; never trust a mutable model alias alone. Changing any of these makes
a distinct generation. Changing endpoint address, retry timing, worker holder,
or diagnostic labels does not, provided the endpoint demonstrably serves the
same pinned contract. Concrete model/runtime fields await the local-model
spike; an incomplete or unverifiable manifest cannot activate. An accepted-graph
snapshot and its scope revisions are pinned per request, not folded into a new
generation for every accepted operation. Unaccepted candidates are absent.

Activation is an owner-authorized, revision-checked database transaction for an
exact destination scope and source-lineage selector. It records generation,
activation revision, and current commit position `F`. Only source events with
positions greater than `F` enter its live selection. Eligible older events and
all legacy history remain outside selection unless separately selected. An
already-open root contributes only its post-`F` suffix as new evidence; earlier
root content may be bounded overlap, without being counted as newly compiled.
The activation frontier means “new selection starts after F,” not “history
through F was processed.” The installed append-position facility operates even
while extraction is disabled, so activation does not need a whole-history scan.

One generation is active for live selection per destination/source selector.
Replacement captures `F2`, atomically ends the previous interval at `F2`, and
starts the new one after it. Old selected work remains pinned and may finish;
new-generation activation does not automatically recompile any old interval.
Compare-and-swap on activation revision makes two racing replacements conflict;
repeating the same activation request ID returns its original frontier. The
selectors for one destination may not overlap; a conflicting activation is
rejected instead of silently selecting an event twice under different live
generations. Explicit session-scope selection remains a different destination.

Historical selection records the owner, generation, destination, source lineage,
and a finite list of inclusive `(session ID, first sequence, last sequence)`
ranges with exact boundary event IDs. Capture and validate the upper bounds in
one transaction; later appends are excluded. Initially accept at most 100 ranges
and 10,000 selected events per request, using bounded count queries and rejecting
larger requests without partially selecting them. Selection does not force a
live unfinished root closed; its captured portion waits for a #132 closure
condition. Selected portions remain prefixes/suffixes, never arbitrary snippets.
Overlapping requests for the same generation/destination unite selection and
reuse existing owned intervals and jobs; do not create overlapping ownership.
Historical work under a newer generation requires this same explicit selection.

## Atomic scheduling and rediscovery

Every selected event append commits its append-position record and a compact
idempotent scheduling indication in the same transaction. A final no-tool
assistant event, failure/interruption event, or later root additionally records
the eligible closure indication there; no extractor runs in that transaction.
This may be a coalesced dirty/high-water record for the activation, rather than
one eagerly allocated job per event. Thus a full job queue never causes loss of
the scheduling obligation or a wait for inference. A rolled-back event has no
scheduling record. A committed event with lost channel notification is recovered
from SQLite. Source append and activation decisions serialize: an event before
activation is outside its live range; an event after it is selected, regardless
of when the process notices the activation.

Reconciliation walks selected intervals in bounded pages, checks source policy,
and captures a sealed prefix/suffix plus job identity in one serialized decision.
The no-live-turn-lease check and captured event cutoff must share that decision.
A later root extension owns a disjoint new suffix even when an earlier prefix
failed or has not finished. Durable ownership prevents two reconcilers from
choosing different overlapping cuts. A scan cursor is an optimization, never
proof of completed coverage: unresolved/deferred intervals remain revisitable,
including work skipped by a page or queue limit. Persisted source errors,
oversize classification, and absent support are visible without calling a model.

No configured extractor means no activation can start and no new pending jobs
are materialized. Disabling an activation closes live selection at a captured
frontier and pauses its already selected incomplete work; it is not cancellation
or successful empty extraction. Events continue committing outside that closed
selection. Reactivation captures a fresh frontier; the disabled interval is
historical and requires explicit selection. Previously selected work remains
paused until the owner resumes it with its pinned configuration available.
An activated but unreachable configured endpoint instead produces bounded
retryable failures. Lost credentials/configuration is a visible configuration
pause, not an invitation to use another model or remote endpoint.

## Jobs, staging, and coverage

The unique job key binds generation, destination/source lineage, session/root,
newly owned interval boundaries, and window-manifest hash. The manifest pins
exact newly covered, excluded, overlap, and context references, event/policy
versions, closure reason and cutoff. A request's bounded accepted-state context
is sealed on its first attempt and reused on retries; later graph changes are
review-time concerns. Job IDs, attempt IDs, and candidate IDs are separate.
Candidate group identity is the job ID; stable item IDs derive from its committed
result manifest. Retried model output cannot create a second completed group.

| State | Meaning and permitted next step |
| --- | --- |
| `selected_unmaterialized` | Durable selection needs closure/eligibility evaluation or a queue slot; no attempt or completion is implied. |
| `deferred_live` | Selected cutoff still lacks an observable #132 closure; reconsider after lease/closure change. |
| `queued` | Exact window is sealed; eligible for a fenced claim when resources and configuration permit. |
| `running` | One current lease/attempt owns a reserved request; no candidate is reviewable. |
| `retry_wait` | Retryable failed attempt with a durable due time and remaining budget. |
| `staged` | A complete, validated, immutable candidate-group envelope is durable but unpublished; no new inference is needed. |
| `completed_candidates` | Whole group and exact completed interval are committed atomically and available for review, subject to duplicate presentation rules. |
| `completed_empty` | A valid extractor response explicitly contained zero candidates; retains its successful attempt and interval. |
| `excluded` | Deterministic policy inspection found no admitted supporting input; records content-free exclusion reasons and makes no model call. It is not successful empty extraction. |
| `failed` | Terminal invalid source/configuration/output or exhausted attempts; reason, history, and unresolved interval remain. |
| `cancelled` | Explicit cancellation invalidated worker authority; the interval remains unresolved and can only be resumed explicitly without resetting its attempt budget. |

Configuration/resource pause is an orthogonal reason, preserving the durable
state and due time. Oversized source input has `failed:oversized_input` with zero
inference attempts. A transport/request-size failure is distinct. Invalid or
missing output cannot take the `completed_empty` transition. Permanent raw
source corruption cannot take `excluded`. Detected secret content is excluded
under the pinned policy; a valid unaffected source can still be compiled.

Before publishing a result, persist the complete bounded validated envelope and
its hash in one fenced staging transaction. Staging either stores the entire
group or none of it; partial item rows never become reviewable. Repeated staging
with identical job/fence/hash returns the existing receipt; a different hash
conflicts. Publication atomically verifies current authority and manifest,
creates the entire group, classifies equivalence against current review
revisions, records exact completion, and marks the staged envelope consumed.
Publication is idempotent by job identity. No accepted operation or semantic
revision is changed. A restarted supervisor can adopt staged work with a new
fence, revalidate it, and publish without calling the extractor again. Cancellation
before publication prevents publication, including automatic adoption of its
staged data. Explicit resume may revalidate and adopt an intact saved stage
without another inference attempt, even when all five attempts were used.

Coverage exposes selected intervals, exact successful intervals, excluded
intervals with reasons, unresolved gaps/states, and history outside selection
separately for each generation, destination, session, and selection segment.
The contiguous frontier of a segment is the largest source sequence through
which every selected event is covered by a successfully completed unit or a
deterministically excluded unit. It cannot cross a failed, cancelled, deferred,
queued, running, retrying, or staged unit. Excluded units are always separately
counted; the frontier must not label them as model-processed. A disjoint later
selection has its own frontier; unselected holes are never filled by inference.
Scope progress is this vector plus exact interval counts, never one fabricated
cross-session sequence. Acceptance/rejection does not change coverage.

## Attempts, cancellation, and resource ownership

Claiming an extraction attempt atomically acquires a monotonically increasing
job fence, lease holder, resource reservation, and increments its durable attempt
count before dispatch. A crash after reservation but before the HTTP write still
consumes that attempt; uncertainty cannot create uncounted model calls. There
are at most five inference attempts for the job across all processes/restarts.
After failed attempt `n`, retry at `failure_time + min(5 * 2^(n-1), 600)` seconds
when `n < 5`: the four automatic delays are 5, 10, 20, and 40 seconds. A retry
never resets the counter or the pinned request. Schema repair is another model
request and consumes the next attempt under the same delay; no hidden repair
loop or additional five-attempt budget exists. Pure deterministic validation,
capacity waits, and publication of a durable stage are not inference attempts.

Unavailable endpoints, transport disconnects, request timeouts, server overload,
and malformed/truncated/schema-invalid output are retryable within that budget.
Unsafe endpoint/configuration, unverifiable model identity, invalid source
binding, forbidden scope/effect, or oversized admitted source input fails closed
without retrying unchanged invalid input. A valid result whose meaning is
unsupported is not repaired into authority; quality evaluation remains separate.
On the fifth failed attempt record `failed:attempts_exhausted`. Explicit resume
can clear cancellation or a corrected operational pause, but cannot reset
attempts, alter sealed evidence, or replay a completed job. Reselection of the
same generation/range reuses that job and counter. A new material generation
with explicitly selected history gets new jobs; old failure gaps remain recorded
under the old generation. An exhausted same-generation job has no retry override
in v1. This conservative ceiling is deliberate, not a silently renewable budget.

Initial job leases last 30 seconds and renew every 10 seconds using database
time. Every renewal, stage, publish, retry, and release requires the current
unexpired holder/fence. Cancellation increments the fence transactionally and
records the reason before signalling local cancellation. Expiry/replacement
also fences previous holders. A stale request can return bytes but cannot stage,
publish, mark empty, change coverage, or release a replacement's reservation.
Review remains separately authorized and never uses this worker fence.

The Kernel starts a bounded supervisor for each long-lived CLI/REPL or web
process with configured compilation. Short commands can append scheduling
records and exit without draining inference. There is no new daemon requirement;
when no host is running, selected work stays durable and resumes on a later host.
Shutdown stops claims, cancels in-flight clients, and attempts fenced recovery
state/resource updates within a five-second cleanup deadline. Durable lease and
capacity reconciliation covers process death or an unsuccessful cleanup write.

A SQLite capacity record shared by cooperating processes allows one active
inference request across all generations/scopes in this database. It binds the
request ID, job fence, owner process, endpoint/server identity, and release state.
All cooperating hosts for the same endpoint must use this database; independently
configured databases are outside this single-machine cooperation contract and
must not be presented as sharing its capacity guarantee. Reserving a job lease
alone never grants a second model slot. Staging/publication can proceed while
another job uses the slot once the prior server request is known finished.

Client cancellation must interrupt the local request promptly (the deterministic
transport check requires return within one second), independently of server
completion. A successful response completed by the transport's contracted end
marker, an explicit request-specific cancellation/idle acknowledgement, or a
verified controlled server restart can establish release. Connection close,
client timeout, process death, and lease expiry alone cannot establish server
release. If release is uncertain, retain the slot as `release_pending` and block
all new inference while polling only a contracted bounded status/cancel API.
When none exists, expose `capacity_blocked` until release can be established
through the selected runtime's documented mechanism or a verified server
restart; do not time out the reservation into a second concurrent request.
Runtime support and empirical release behavior are gates for the spike/transport
ticket, not an invented assumption in this record. Old stale output remains
fenced even after actual server capacity becomes available.

## Bounds and scheduling fairness

These are initial implementation bounds, not measured performance claims:

| Resource | Initial bound and full-capacity behavior |
| --- | --- |
| Evidence | #132's 32 KiB/64 new support events, 8 KiB/16 overlap events, and 4 KiB/8 assistant context events; never split/truncate oversized new input. |
| Serialized extractor input | 128 KiB including schema, prompt and accepted-state context; reject visibly if it cannot fit. Also enforce the pinned model's smaller token/context limit. |
| Extractor response and staged envelope | 128 KiB each and at most 16 candidates per group; reject over-bound output, never accept a truncated prefix. |
| Materialized unfinished jobs | 1,024 globally; later selection stays durable and unmaterialized. Terminal/completed audit records are retained separately. |
| Unpublished stages | 128 groups and 16 MiB globally; reserve space before requesting output, then release only on recorded consumption or cancellation disposition. |
| Unresolved candidate presentation | 2,048 unsuppressed candidates globally; reserve up to 16 positions before extraction so publication cannot overflow. Existing review/history remains accessible when full. |
| Database work | At most 128 inspected events or 64 candidate/ledger row mutations per background transaction; a complete at-most-16-candidate group is the bounded atomic publication unit. Manifests are bounded envelopes, not unbounded per-reference mutation loops. |
| Inference lifetime | 120-second request deadline; expiration cancels the client and retains the capacity reservation until server release is established. |

Reservations are owned by job/fence and recovered transactionally, preventing
multiple processes from each assuming the whole remaining capacity is free.
Resource backpressure consumes no inference attempts. Retained raw events and
terminal audit history can grow; expose their sizes and do not claim a total
database-size bound or delete them. A change to source/model/validation limits
that changes what a request can mean requires a new pinned policy/generation;
queue caps and polling intervals are operational settings.

The shared scheduler dispatches new-evidence work first, in earliest eligible
commit-position order with stable job-ID tie-breaks. Retries reenter their
original lane only when due; an unavailable earlier job does not block later
independent work. After at most eight new-evidence attempts while any historical
job is ready, reserve the next available request slot for the oldest ready
historical selection (then session ID and sequence). Persist the counter with
the shared capacity claim; resetting a process cannot reset fairness. Historical
work uses all slots when no new work is ready. This permits one historical
request per nine saturated dispatches without concurrent inference or an
unbounded starvation claim. Paused, oversized, not-due, or capacity-blocked jobs
are not ready. Fairness cannot bypass resource, source, or authority checks.

## Equivalent suggestions and retained review decisions

Equivalence is a deterministic comparison of the complete proposed effect,
destination/source scope and lineage, bound Entity identities or unresolved
identity alternatives, Predicate definitions, typed values, polarity, temporal
and correction effects, dependencies, and exact supporting/context manifests
including authority and hashes. Use canonical semantic values and ordered
manifests; display prose, confidence, generation ID, and item ordering alone do
not change meaning. Unknown identity cannot equal a known Entity because the
names match. Unresolved proposed Entities use stable source-bound placeholders;
do not use generated IDs to manufacture differences or name matching to collapse
possibilities. If deterministic identity cannot be established, do not suppress.
The exact canonical encoding/version must have fixtures in the storage ticket.

Review state is `unresolved`, `accepted`, or `rejected`; an owner edit appends an
immutable interpretation revision and leaves the candidate unresolved until
review. Generation-specific extraction groups remain distinct. Duplicate
presentation is separate metadata with a link and checked review revision;
it never copies another candidate's accepted/rejected state or creates an
accepted operation. The shared
[owner-review contract, ticket #134](https://github.com/davidadel66/evie/issues/134)
owns exact previews and resolution transactions.

| Existing equivalent candidate | New generation's retained suggestion |
| --- | --- |
| Accepted, including an accepted owner edit | Link the exact accepted interpretation/review/operation and suppress a duplicate inbox item. Add no support or operation. If current accepted state would require a different effect, it is not the same actionable suggestion. |
| Rejected | Link that rejection and suppress an exact repeat from the same evidence. Never reopen or overwrite the rejection automatically. |
| Edited but unresolved | Preserve the owner's current interpretation as primary. An exact repeat of the original extraction links to that edit lineage and is suppressed; it cannot replace the edit. |
| Unedited and unresolved | Keep the first durable candidate as the primary review item; link/suppress the equivalent later candidate. A generation upgrade cannot switch the item under an open preview. |

If the full proposal differs, present a new candidate with prior-related review
links where deterministically known. New supporting evidence changes the
manifest: an equal accepted proposition may yield a new attachment suggestion,
requiring explicit review, while new evidence for a previously rejected proposal
may reappear with the rejection visible. Existing accepted operations remain
unchanged. Cosmetic wording or a new confidence score is not new evidence.
An edit that was later accepted or rejected preserves both the reviewed
interpretation and original extraction in the suppression lineage; an exact
original-output repeat links to that resolution and remains suppressed, even
when the reviewed edit had a different effect. It must not undo that owner
decision or pretend the original interpretation was accepted.

Classification and publication read/check the current review revision within
the same transaction as inserting duplicate links. A racing accept/edit/reject
therefore either precedes classification or forces it to use the resulting
revision; no stale decision can hide an owner edit. A primary's later resolution
retains the links and inherited presentation reason. Suppressed candidates are
inspectable with their own original generation/output and the actual review
origin, and have no independent actionable preview while that link is valid.
Revoked visibility or changed source policy redacts or blocks inspection under
current rules; retention is not permission to disclose formerly visible bytes.

## Verification and remaining implementation gates

Use [W01–W14](memory-stage-4-work-contract.fixtures.md) as deterministic scenarios
through the agreed Kernel seam with real temporary SQLite and scripted
extraction. CLI/web must report these same states; local HTTP fixtures belong
to the transport ticket. None requires a live model for coverage or fencing.
The spike still must choose and measure a local runtime/model, demonstrate
server-capacity release, and supply complete generation fields. Numerical pilot
latency/resource release gates remain the separate evaluation prerequisite;
these are explicit downstream gates, not unresolved behavior in this contract.

For this documentation-only ticket, run `git diff --check`, check both new
files for whitespace and local Markdown links, and mechanically check the
worked retry/frontier/fairness arithmetic. Go tests/vet, UI lint/build, and live
model evaluation are skipped because no code changes here; dependent code
tickets must run their required verification and the agreed seam tests.
