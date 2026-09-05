# Memory Stage 4 evidence and closure contract

Status: binding, 2026-09-04. David approved D1 and D2. D3 records technical
defaults chosen by implementation judgment within the approved scope, rather
than a separate owner evaluation of byte limits. This completes the evidence
and closure prerequisite; the other Stage 4 prerequisites remain separate.

This is the contract deliverable for [ticket #132](https://github.com/davidadel66/evie/issues/132),
under the [Stage 4 specification, issue #131](https://github.com/davidadel66/evie/issues/131) and
[binding memory decisions](memory.decisions.md). The accompanying
[worked fixtures](memory-stage-4-evidence-contract.fixtures.md) distinguish exact
mechanical checks from human judgments of meaning and usefulness.

## Recorded decisions

| Decision | Contract | Consequence |
| --- | --- | --- |
| D1: indirect speech; owner approved 2026-09-04 | Initially use unendorsed quotations and reported speech as interpretation context only. An explicit owner assertion or endorsement can support the asserted proposition. | Attributed-report Claims are excluded from this slice; admitting them later requires an explicit Predicate/representation contract. |
| D2: initial named observation; owner approved 2026-09-04 | Admit only `get_time` under `local-clock-display-v1` below, and only to support an owner assertion that explicitly refers to that checked clock/date. | Every other tool remains ineligible; arbitrary prose or Task state cannot fill the gap. |
| D3: bounded interpretation and incomplete histories; technical defaults recorded 2026-09-04 | Use one newly covered root-turn prefix per window; include at most two preceding root prefixes as overlap. Pin the byte limits below. Reconcile an incomplete prefix when no unexpired turn lease exists, retaining its recorded status as incomplete. | Some useful facts needing more context or oversized input will remain visibly unprocessed. A split/continuation policy requires a separate contract; no truncation or inferred successful turn is permitted. |

The rest of this record defines these decisions and their verification.
Existing parent invariants are restated where needed; technical projection
details bind the Stage 4 implementation without changing Stage 3 behavior.

## Scope and invariants

This outcome defines evidence admission, selection, source projection, bounded
interpretation, and source-history eligibility. It does not implement a
compiler, job store, worker, model, automatic admission, cleanup, or review
transaction. The durable-work ticket owns job identity, retries, cancellation,
selected-range accounting, and atomic scheduling. The review ticket owns
approval authority, exact effects, edits, batches, and acceptance transactions.

The Kernel binds every window to its source session, immutable Workspace or
project identity, and one selected destination scope. The default destination
is the active Context Scope, or global when the source session has no Context
Scope, matching current explicit memory. An explicitly selected session scope
is also permitted. The extractor cannot choose or widen that destination.
All support must be visible under the selected source authority and compatible
with the destination's existing reference rules. Scope identity comes from
registered IDs, never names, paths supplied by the model, or quoted source text.

One session cannot combine a Workspace and a project. A resumed session with
the same durable session identity retains its lineage; another session, including
one sharing a Workspace, is a different conversation. Initial windows never
combine sessions. Existing accepted Entities may be offered as bounded
resolution alternatives under existing scope rules; they are not new episodic
support and cannot supply a missing assertion or missing conversational context.

Broader reuse requires explicit Promotion. Neither an owner accepting a
candidate, a globally defined Predicate, a global owner Entity, nor a shared
Alias imports narrower evidence. Provenance expansion retains existing source
visibility and redaction rules. Review after source-session closure does not
revive that session or relabel tool evidence as an owner statement. These
invariants are retained by the published parent specification; its supporting
ADRs 0034, 0061, and 0064 do not independently widen this ticket's scope.

## Selection and meaning

Selection has four labels. **Required useful** means a human-reviewed fixture
expects a supported candidate for the stated initial use case; it is a recall
label, not permission to bypass acceptance or invent certainty. **Optional
useful** permits a supported suggestion but does not count abstention as a
miss. **Unsupported** means the proposition does not follow from eligible
evidence. **Unwanted but true** means the information follows from evidence but
does not serve the initial memory use case. Both latter classes expect no
reviewable candidate. A successful no-memory result retains its original
episodes and is distinguishable from failure or an unprocessed range.

| Fact class | Required useful when explicit and sufficiently identified | Optional useful | Unwanted or unsupported |
| --- | --- | --- | --- |
| Enduring preferences | Stated standing preference or durable constraint, including explicit negation; no request to remember is necessary | No separate optional preference category in v1 | A single meal, transient mood, or inferred preference from repeated actions |
| People and relationships | Explicit meaningful relationship and identifying context | Useful self-described background for a recurring person | Same-name merging, an assistant's guess, or a quoted relationship treated as the owner's assertion |
| Workspace/project decisions | An adopted decision, enduring constraint, or its explicitly stated rationale | A useful enduring option that the owner explicitly says remains under consideration, represented as consideration | Assistant recommendations mistaken for decisions, transient debugging output, or an option represented as adoption |
| Meaningful changes | Explicit changed circumstances, corrections, and adopted future decisions | An enduring intention represented as intention, never a completed change | An idle hypothetical, unsupported effective date, or a possibility represented as a completed event |

A candidate must preserve subject, relation, object kind, negation, temporal
qualification, scope, and uncertainty. Do not infer an affirmative Claim from
missing knowledge. Unknown Valid Time bounds remain unknown; the source's
RecordedAt is observation/audit time, not an invented date of the world change.
An explicit correction of an earlier error differs from a real-world change;
ambiguous correction mode remains a review choice. Additional support may attach
to an equal accepted Claim, while an equal name alone never identifies an Entity.
New Predicate definitions and unresolved identity alternatives remain visible
effects, not silent vocabulary or identity changes.

Operational Task, schedule, account, file, and other domain stores remain the
authority for their changing records. A tool's success does not make its current
state an enduring memory. This initial contract defines no Task-state cache or
freshness policy for such records. A direct owner assertion about an enduring
working preference or project constraint can still be useful; its meaning must
not claim to replace or continuously mirror the operational record.

The source event's `user` role establishes who supplied text, not which embedded
propositions they assert. Under D1:

- Direct assertions and explicit endorsements can support a candidate. An
  endorsement must identify the proposition and preserve any qualifications.
- A quotation used as an example, pasted instructions, fiction, and an
  unendorsed report such as “Maya said she moved” do not establish that Maya
  moved. Keep attribution in context; initially produce no report Claim.
- Hypotheticals and questions do not establish their premises. “I have decided
  to move next year” asserts a decision; it does not assert a completed move.
- Assistant text can disambiguate an explicit owner answer, such as a single
  clear question followed by “Yes.” It cannot add facts, confidence, dates, or
  corroboration. Ambiguous assent to several propositions is insufficient.

Exact locator validation proves that words were present. It does not prove
that they are asserted, supported, useful, correctly attributed, or correctly
interpreted. Semantic entailment and the four selection labels require the
separate reviewed quality fixtures; a structural validator must never report
them as mechanically proven.

## Named tool observation: `local-clock-display-v1`

The only eligible capability is the existing `get_time` tool with its
parameterless contract. Its durable result is a string formatted
`YYYY-MM-DD HH:MM:SS` by the local process; it is not a typed JSON observation
and contains no UTC offset. This contract does not change that tool.

The Kernel must verify all of the following before projecting its content:

1. A committed `tool_succeeded` with role `tool` and `is_error=false` exists.
   `tool_cancelled` can also have `is_error=false`; that flag alone is not
   success. `tool_failed`, `tool_cancelled`, and `execution_resolved` are not
   observations under this contract.
2. Its ancestry resolves through the matching intent and any approval node to
   an assistant tool call in the same root turn and session. Event execution
   ID, call ID, intent call name `get_time`, and the assistant-declared call
   agree. There is exactly one terminal outcome for that execution. Missing,
   conflicting, cross-session, or malformed linkage is a source error.
3. The call has the contract's empty JSON object arguments. A same-named future
   tool with a changed argument/result contract needs a new evidence policy.
4. Durable content is exactly 19 ASCII bytes, contains a valid calendar date
   and clock time in the stated format, and contains no truncation envelope,
   extra text, invalid encoding, or detected secret.

Permitted projections are the whole content or `content` UTF-8 byte range
`0:10` for its calendar date. Other subranges are ineligible. Authority is
`tool_observation`, actor is `tool`, and source type is `tool_succeeded`, with
the named observation contract pinned in candidate provenance. These are new
Stage 4 authority values, not values currently accepted by Stage 3. Approval
never changes them to `owner_statement`.

The observation means “the runtime's local clock displayed this value during
this execution.” It is historical and fresh only for that observation. It
does not prove the owner's timezone, location, a trusted UTC instant, or a
lasting world fact. `ObservedAt` is the tool result's RecordedAt, not a claim
that its unzoned display is UTC. It may support the calendar date of an owner
assertion only when the owner explicitly refers to this checked date. The
owner assertion must also be cited. A bare clock observation is unwanted but
true and produces no candidate. Conflicting clock and owner context must
remain uncertain rather than being resolved silently.

Every other tool is ineligible in this initial contract, including arbitrary
shell output, file reads, web/search results, transcript ingestion, database
queries, finance results, and Task tools. Their output is also omitted from
interpretation context, so it cannot become a hidden source of a candidate.
Tool intent/arguments and approval records remain control metadata, not proof
of an observed outcome. A later named contract must identify a concrete useful
memory case, eligible capability and version, completion semantics, allowed
fields, original authority, freshness, and source rendering before admission.

## Exact projection and source references

Every supporting reference records event ID, session ID, source scope,
original authority, event part, locator kind/value, exact evidence bytes,
`sha256:<64 lowercase hexadecimal digits>`, and observed time. The Kernel
resolves these values from immutable events; the extractor can only nominate
locations. Authority, scope, and hashes supplied by the model are not trusted.
The event's FormatVersion and named evidence-policy version are pinned with
the window. Invalid or unknown versions fail closed.

| Event part and locator | Initial contract |
| --- | --- |
| `content`, `whole`, empty locator value | Exact UTF-8 bytes of an eligible owner's content or eligible named observation; no trimming, normalization, or appended UI labels |
| `content`, `utf8_byte_range`, `start:end` | Zero-based, half-open byte interval `[start,end)` in the original durable content; canonical unsigned decimal integers with no leading zero except `0`; `0 <= start < end <= byte length`; both ends must be UTF-8 scalar boundaries |
| `content` or `payload`, `json_pointer` | Ineligible initially: no current admitted tool supplies contracted JSON fields. The reserved Stage 3 locator kind does not grant permission to parse arbitrary JSON. |
| `payload`, any locator | Ineligible as factual support initially. Tool result payload holds correlation/error metadata; assistant payload contains tool calls and usage, not observation fields. |

Byte coordinates refer to original committed content, never HTML, Markdown
rendering, JSON escape sequences, a normalized copy, or rune/UTF-16 indexes.
Preserve newlines, carriage returns, combining marks, variation selectors,
emoji, and Unicode normalization exactly. Reject invalid UTF-8 and byte cuts
inside a scalar. Scalar boundaries do not guarantee whole graphemes or semantic
completeness; the quoted range must still support its interpretation. Hash
exactly the projected bytes with SHA-256, without a BOM, encoding envelope,
JSON quotation marks, or a newline not present in the source. The source ID and
locator are separately bound; equal text hashes never identify the same event.

No structured-field projection is permitted by v1. In particular, a tool's
JSON-looking `Content` is not `Payload`, a JSON pointer cannot index across both,
and the Kernel must not hash a reserialized object while showing a different
string. Admitting structured fields requires a named future contract that
freezes scalar/object encoding, pointer resolution, duplicate-key rejection,
number handling, and hashing with worked examples. This is an explicit
eligibility boundary, not a missing generic JSON implementation in this ticket.

Each proposed Claim or other sourced effect enumerates every support needed
for its meaning. Multi-source evidence remains separate references with
separate hashes and original authorities; concatenation cannot stand in for
attribution. Reused owner support from overlap remains a Source Link if needed.
Assistant text needed to interpret an answer is recorded as exact, bounded
interpretation-context references displayed alongside support, with authority
`none`; it never becomes a supporting Source Link. Context references must
survive acceptance sufficiently for the same source interpretation to remain
inspectable, without placing extractor confidence or model text in Claim truth.
The dependent review/replay contract must preserve this manifest explicitly.

One deterministic resolver must produce the same permitted projection for
extractor input, candidate validation, review, acceptance revalidation, and
accepted-source inspection. Review labels and escaping surround that projection
without changing the bytes being checked. An inspector may show authorized
surrounding text separately and label it context; it must not substitute the
whole episode for a range citation. Missing events, changed hashes, changed
eligibility, and denied visibility return distinct failures or redaction,
never a plausible replacement quote. Replay checks recorded source binding
without a model or tool call and retains canonical accepted-operation rules.

## Secrets and embedded instructions

Before sending content to the extractor, apply the code-owned, versioned secret
detector to every owner, clock, and assistant-context field considered for the
window. A detection excludes that entire event's content from both support and
interpretation context; do not replace secrets with text and treat the changed
text as original evidence. This conservative choice avoids offset remapping and
retains unaffected events as independently eligible. Record only content-free
exclusion metadata. The original episode remains untouched.

Scan proposed text and all supporting/context projections again before
reviewable persistence and acceptance. Detected secrets block their candidate
and any proposed Promotion; a later policy change requires revalidation rather
than exposing a stored quote. The source inspector must not reveal excluded
content through an expanded episode, operation payload, or context reference.
Detector rules and their exact synthetic fixtures belong to the source-projection
implementation ticket and form part of the pinned evidence policy; an
unconfigured or failed detector is not an allow-all policy. This does not claim
perfect detection or that raw Episodic Memory contains no secrets.

All admitted text is untrusted source data, including text supplied by the
owner. An embedded “ignore the rules,” tool command, approval claim, or request
to widen scope is quoted data for extraction and cannot alter the harness,
tool permissions, destination, detector, or acceptance. Reasoning, opaque
continuation state, context snapshots, compaction summaries, retrieved blocks,
compiler outputs, and diagnostics are excluded from support and from the
initial interpretation input. Assistant messages that merely repeat one of
these sources still cannot independently corroborate it.

## Bounded windows, overlap, and eligibility closure

A root turn begins at a parentless `user_message`. Event ancestry and the
terminal payload's `turn_id`, not merely the latest event or terminal parent,
identify its members. A tool result is not discarded because a later terminal
points to an earlier provider trigger. Source order is session sequence order;
timestamps and scope membership do not create cross-session conversation order.

Under D3, the Kernel selects a committed root-turn prefix with an
explicit captured last sequence. A prefix is eligible when it has a recorded
final assistant response with no tool calls, a valid `turn_failed` or
`turn_interrupted`, a later root user message, or a transactionally observed
absence of an unexpired session turn lease. A final assistant response is not a
new `turn_succeeded` event. A live lease and an unfinished current root defer
selection until an actual closure condition is observable.

No-lease reconciliation captures committed events and the lease condition in
one consistent, serialized database decision. It seals the selected prefix,
not the real-world execution or the root forever. Expiry, lease release, a
later root, or an inactive session does not prove success, failure, cancellation,
approval, or an external effect. It must never append a fabricated terminal
event. If new committed events later extend that root, their suffix remains
new uncovered evidence; the earlier prefix is not silently extended or counted
as having processed those new bytes. The durable-work contract must retain this
cutoff and support incremental selection without duplicating covered evidence.

Each window owns one newly covered prefix or suffix from one root. It contains
eligible owner support, contracted observations, and bounded assistant
interpretation context from that root. When processing a suffix, earlier
content from the same root may be overlap under the same limits. Initial limits
are 32 KiB of newly covered admitted support and 64 newly covered support
events, 8 KiB/16 events of overlapping support, and 4 KiB/8 events of assistant
context across the entire window. All limits count original projected UTF-8
bytes after secret exclusion, before serialization. Model transport separately
limits the complete serialized request and output; fitting these evidence
limits is not a claim that every model request will fit.

Overlap uses only the immediately preceding at most two eligible root prefixes
in the same session and scope lineage; it is never a search through other
conversations or skipped ranges. Earlier content of the current root counts as
one of those overlap units. Add whole eligible fields in reverse chronological
order within these units until a byte/event bound would be exceeded, then stop;
render the selected fields in chronological order with omissions explicit.
Do not truncate a field or bridge a missing context field to invent continuity.
Assistant context is the nearest whole assistant content under its independent
bound and is always labeled non-supporting. If required interpretation is
missing, ambiguous, excluded, or outside the bound, abstain from that candidate.

The candidate's latest required supporting event must belong to newly covered
evidence. Its owning window is the window containing that event. An old fact
whose only support is overlap cannot be proposed again just because it was
visible. New support for an existing proposition can produce a sourced
attachment suggestion subject to the later equivalence/review contract. Context
does not advance coverage, and assistant assent cannot supply newly owned
support. Later jobs cannot depend on earlier unaccepted candidates.

If the newly covered eligible support exceeds its byte/event limit, keep the
whole selected prefix/suffix visibly unprocessed with an oversized-input
reason; do not split, truncate, summarize, mark successful empty, or implicitly
skip it. Independent later ranges can still progress. Missing context alone
does not make the whole window fail: process independently supported assertions
and record candidate-specific abstention in the quality output. A future larger
bound or split policy changes the evidence policy and Compiler Generation.

## Closure and eligibility matrix

| Durable history at captured cutoff | Eligible support | Closure treatment |
| --- | --- | --- |
| Root user plus final assistant with no tool calls | Eligible owner assertions; any fully linked named successes | Recorded conversational completion; assistant is context only |
| Root user plus `turn_failed` | Eligible committed assertions and earlier named successes | Recorded failure; its safe diagnostic is not support |
| Root user plus `turn_interrupted` | Same, including a tool result committed before interruption | Recorded interruption; preserve actual classification/stage |
| Root/intent and no terminal; live unexpired lease | None selected yet for that unfinished root | Deferred live work, not empty success |
| Root/intent and no terminal; no live lease | Eligible committed assertions and already completed named successes | Reconciled incomplete prefix; unfinished intent proves no outcome |
| Root followed by a newer root | Eligible committed evidence in the earlier prefix | Structural cutoff; do not infer the earlier outcome |
| Explicit semantic command root, optional approval, no assistant | Any direct assertion actually expressed by the owner; approval is not support | Finalize from later root or no-live-lease cutoff; do not infer a semantic effect from the command or approval alone |
| Read-only local command with no root event | Nothing | No episodic work exists; do not create a source |
| `/compact` event or standalone diagnostics | Nothing | Excluded evidence; do not compile generated summaries |
| Partial tool group, with one named success and another missing outcome | Named success can support its own contracted observation; root assertions remain usable | At an eligible cutoff retain the incomplete group's status; no sibling completion is inferred |
| Corrupt version, impossible lineage, or contradictory outcomes | No source whose validity depends on that corrupt unit | Visible source error/gap; no successful empty extraction |

Eligibility is not selection: excluded historical ranges remain outside the
owner's activation/backfill choices. Excluded source categories, zero useful
candidates from successfully processed eligible input, oversized gaps, invalid
source failures, and still-live work are distinct outcomes. The durable-work
contract owns their stored representation and contiguous frontier rules.

## Current-code alignment and dependent verification

The current event types and payloads are defined in
[event.go](../../../../internal/memory/event.go); tool completion and terminal
parenting in [turn.go](../../../../internal/agent/turn.go). Command-only owner
events come from [semantic_memory.go](../../../../internal/agent/semantic_memory.go).
The existing [get_time implementation](../../../../internal/tools/time.go)
returns the unzoned local display specified above.

[Semantic source types](../../../../internal/memory/semantic.go) already name
whole/range/JSON-pointer locator kinds, but current
[owner-source validation](../../../../internal/eviedb/semantic_entity.go)
admits only whole owner messages, and
[source inspection](../../../../internal/eviedb/semantic_lifecycle.go)
currently loads full event content. Those paths do not yet implement this
Stage 4 contract. Dependent production tickets must extend the shared
projection/inspection and versioned acceptance/replay contracts together; they
cannot merely accept range locators in extractor output or reuse Stage 3
session-bound approval under a new name.

The worked fixture IDs below are the deterministic implementation checklist,
with semantic gold judgments labeled separately. Exercise compilation and
review through the agreed Kernel seam using real temporary SQLite and scripted
extraction. Verify all CLI/web adapters render the same source bytes and
authority. Use local HTTP fixtures only for the transport ticket; no live model
is needed to establish source coordinates, eligibility, scope, or replay.

Documentation-only verification is `git diff --check`, a local Markdown link
check, and executable calculations of the worked byte ranges/hashes. Full Go,
vet, UI lint/build, and live-model quality runs are deferred because this ticket
changes no code; each dependent implementation must run its required checks.
