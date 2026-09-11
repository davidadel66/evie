# Integrated Memory development workload, version 1

This is development material for issue #167. It contains 24 newly worded cases
in 12 behavior families, with 61 explicitly labeled source records: 18 accepted
literal Claims, 15 accepted entity Claims, 25 original owner messages, and three
original assistant messages. There are 19 answerable cases, two clarification
cases, two abstention cases, and one task that needs no memory. The smallest
sufficient support sets contain 27 records in total.

No model answers or retrieval results were used to write these labels or the
initial gate proposal below. All earlier Stage 5 fixtures and evaluations,
including partitions previously called held out for component experiments, are
development material for the integrated release. This directory contains no
release-held-out cases. New wording here does not make this a release holdout.

The implementation owner must adopt and freeze a complete configuration before
the pilot. The values in `proposed-gates.json` are initial engineering proposals,
not measurements, past owner approvals, or a claim that the pilot passes. A
development change after a failed pilot requires a new configuration version and
retained original results. No threshold may be changed against release-held-out
outputs.

## Files and coverage

- `workload.json` is the source, question, and gold-label manifest.
- `reader-rubric.md` defines semantic answer assessment and evidence accounting.
- `proposed-gates.json` records the numerical proposal before any pilot run.
- `operating-matrix.json` maps 39 operating and boundary scenarios to exact
  existing complete-turn, HTTP, UI and local-embedding adapter tests. Its
  `not_run_in_this_mapping_task` entries are expectations, not passing results.

| Cases | Family | Behavior to demonstrate |
| --- | --- | --- |
| 01–02 | Fresh accepted preference | A new reader recovers an approved personal preference. |
| 03–04 | Uncompiled original statement | Original wording supplies a condition or observation without becoming accepted knowledge. |
| 05–06 | Cross-topic reference | Earlier recipient context wins over unrelated debugging; unresolved recipients require clarification. |
| 07–08 | Compacted continuity | A real accepted compaction preserves topic or ambiguity but does not become the source of a personal fact. |
| 09–10 | Exact identity | An accepted opaque alias or exact Entity ID resolves the intended person. |
| 11–12 | Graph and dense paraphrase | A complete two-Claim relationship path and a low-overlap original statement supply answers. |
| 13–14 | Bounded neighbor investigation | Adjacent assistant advice remains distinct from an owner selection or tentative choice. |
| 15–16 | Historical and retired | Explicit historical validity and current retirement suppression remain different views. |
| 17–18 | Conflict and newer statement | Accepted conflicts remain visible alongside an unaccepted or tentative owner update. |
| 19–20 | Speaker, quote, and tentative attribution | A reported quotation and an assistant suggestion retain their actual authority. |
| 21–22 | Unanswerable and unavailable | Missing support and disabled access do not authorize guessed facts or an exhaustive negative claim. |
| 23–24 | Scope and unnecessary recall | A language rewrite needs no memory; a Global accepted Claim grants no raw Global or other-project access. |

## Construction contract

Construct records through public approved `PrepareRememberLiteral` /
`ResolveRememberLiteral`, `PrepareRememberEntity` / `ResolveRememberEntity`,
ordinary complete turns, and approved lifecycle operations against real SQLite.
Do not insert accepted state, fabricated source locators, or success-shaped
retrieval results directly. Gold labels never come from a search result.

Each `id`, `session_id`, and entity `key` is a stable manifest label. After a
public operation succeeds, bind that record label to its actual source event,
event part, byte locator, evidence hash, scope, actor/authority, Claim ID and
operation ID where applicable. Bind actual acceptance, observation and lifecycle
timestamps too. Persist this independent source map before evaluating retrieval.
Do not fabricate old observation timestamps to make a historical case convenient.

Records are constructed in array order. Reuse the same source session for a
shared `session_id`; repeated entity keys reuse the original accepted Entity.
The `owner` subject on accepted literals is the actual local owner created by the
public literal API. The `cardinality` values are the real predicate declaration,
not scorer hints. Omitted polarity is affirmed. Omitted valid-time endpoints are
unknown, represented as null in the source map. `valid_time` is passed to the
public remember request; it is independent of the actual save time.

The public entity-creation API requires a nonempty Alias as well as canonical
name and type. If an entity selector omits `alias`, use its exact canonical
`name` as the Alias when creating it. Preserve any explicitly declared alias,
including KIN-84 and KIN-48. This is an API-mandated construction default, not an
inferred second identity or a change to the original source wording. Bind and
verify the resulting accepted Alias ID through the public inspection path.

An `owner_conversation` immediately followed by `assistant_conversation` in the
same session is one actual scripted-provider turn. Use the owner's text as the
input and the assistant's text as its response. Bind both distinct event IDs and
locators. Other owner records can receive a minimal acknowledgment; such generated
acknowledgments must be listed separately in the source map, never counted as
gold or independent corroboration. They count as unwanted evidence if retrieved
without a case-specific reason. No record is hidden model reasoning.

Preserve increasing observation order, especially the newer owner messages in
17–18. If clock granularity gives equal times, complete the next public write
after the previous recorded time rather than rewriting stored timestamps. Apply
`retired: true` through approved Claim retirement after all source records are
created and before making the reader turn. It does not mean delete the episode,
retract every source, or retire an unrelated Claim.

`scope: global` accepted records are approved Everywhere and may be visible from
a project. A `project:other` record is accepted only in that project. Its literal
"Remember in this project" wording does not establish scope by itself: the
constructor must use the corresponding project session and destination. For
case 24 the reader belongs to a separate `project:reader`; raw Global messages
and other-project Claims are forbidden. For `memory_mode: unavailable`, construct
the source while access is enabled, then disable the existing remote-memory
gate for the measured turn. Record that condition as unavailable, not empty.

Recent discussion is reader-session history, separate from the original source
corpus. Seed those exact public turns for all conditions except `no_recall`.
When `compaction_continuity` is present, use the real accepted compaction path
with the supplied continuity text in a valid structured compactor response,
covering the listed recent discussion. Record its actual checkpoint event and
boundary. This evaluates retrieval through compaction, not the quality of a
scripted compactor. The continuity text deliberately omits the personal
preference. Do not additionally inject covered history into the provider request.

Construct a canonical seed and clone it, or otherwise preserve identical bound
source/Entity IDs across conditions. Render
`{{subject_entity_id:dev10_soren}}` from the source map once. The fully rendered
question, original source text, source identities, reader model/profile and
budget ceilings remain identical across all six conditions. Changing tool
schemas and seeded recent context is an explicit ablation, not permission to
rewrite the question for a condition.

## Gold and temporal semantics

Every inner array in `gold.support_sets` is a complete sufficient alternative.
There is one alternative per v1 case; the array shape permits future independently
declared equivalents. `[[]]` means no retrieved personal evidence is required.
It produces no evidence-recall denominator and must not award a synthetic recall
success. Records in `acceptable_context_record_ids` may help interpret the
question but do not substitute for required support or increase recall.

`forbidden_current_record_ids` excludes that record as **current** evidence.
In case 15 the retired historical Claim is both required historical support and
forbidden current support. An explicitly historical reference with the requested
validity date may carry it; a current-intent reference may not. The question
requires a historical valid-at read for 2022-06-15T00:00:00Z, with actual current
knowledge access and source eligibility still checked. Bind oracle references
with that historical intent and date through the same authorized exact-read
policy. Do not flatten the historical permission into a whole-turn text ban.

The machine-readable fields `gold.oracle_intent` and `gold.oracle_valid_at`
declare this view independently of the case name. They are strings: the former
is `current` or `historical`, and the latter is an RFC 3339 instant. Omitted
fields mean current intent with no valid-at constraint. An oracle constructor
must use these fields, never branch on a development case ID to choose a date.

In cases 16, 22 and 24 there is no historical exception: the listed forbidden
records, their IDs/locators, and `forbidden_current_text` must be absent from the
entire actual provider payload. Check canonical messages, arguments, tool
outcomes and native opaque response items, not only the memory projection. The
case 22 source remains present in SQLite specifically to prove disabled access.

Current requests may supply an independently active record while suppressing a
retired or restricted sibling. Accepted Claims and conversation excerpts of the
same underlying owner statement are not two independent sources. A conversation
copy may support factual wording but does not prove acceptance, an Entity alias,
a predicate conflict, or a graph relationship; those obligations require the
corresponding accepted Claim metadata too.

## Proposed pilot procedure and numerical gates

Before a first pilot call, freeze the exact source/rubric hashes, built harness,
actual production Default reader model/canonical ID and context profile, local
embedding model/layer manifest, hardware, retrieval/index configuration, condition
order, raw capture method, and a machine-readable adopted gate file. Preserve
all raw requests, responses, source maps, receipts, timings, failures and manual
assessments. Never replace a failed sample with its retry.

The initial proposal is one measured real reader turn for each of the 24 cases
under each of six conditions: 144 turns. Rotate the condition order by case
index. Every provider call in `tool_only`, `automatic`, and `automatic_deeper`
must be chosen by the actual frozen reader from its first call. An oracle
selector may supply independently labeled exact eligible support through the
real Kernel bounds, with its intervention explicitly recorded. None of these
minimal support sets requires more than two items of either evidence kind.

The source attribution and safety critical cases must pass individually under
the applicable production/oracle condition. Aggregate coverage cannot waive a
single invented source, wrong speaker, forbidden payload, retired-as-current
assertion, silently resolved conflict, or failed material clarification. Missing
an otherwise answerable source is a coverage failure even if the model abstains
honestly. Honest baseline abstention is expected when an ablation removes the
source; baseline evidence/coverage scores still expose that loss.

Proposed production-condition thresholds are initial source recall >=0.55,
final source recall >=0.75, union source recall >=0.85, complete final-request
support on >=0.80 of the 19
answerable cases, supported answer-component recall >=0.85, proposition grounding
>=0.95, citation accuracy >=0.95, unwanted evidence fraction <=0.35, and no
unnecessary clarification on the clear reference cases. The oracle proposal is
supported component recall >=0.95 and grounding/citation accuracy >=0.95. Report
first/final/union separately. Require at least one semantically passing case in
each two-case family and every listed required demonstration. There are 37
expected personal answer components; missing components or required original
source citations receive zero credit. Universal abstention cannot pass the
coverage gates. The machine-readable proposal defines all denominators and units.

Each case's `gate_roles` array determines its mandatory gate obligations.
`material_clarification` requires clarification before a personal recommendation;
`clear_reference` requires resolving the recipient without an unnecessary
question; `critical_semantic` requires the full case-specific semantic criteria.
The critical role includes the union of previously designated critical cases and
required demonstrations, preserving every mandatory pass. Frozen selectors use
these roles, not case IDs, and require at least two clarification cases, two
clear cases and 15 critical cases. Cases may have multiple roles. Existing
development IDs in `development_selector_examples` are documentation only.

A future sealed curator must assign the same role obligations from source and
question semantics before evaluation, with fresh identities, wording and source
families. Do not omit or relabel a role after seeing results. Metric denominators
are computed from the selected frozen workload; the development counts 27, 19
and 37 are reference counts, not hard-coded held-out source or component totals.
No release-held-out cases are authored or inspected by this metadata change.

For local timing, run 20 fresh-reader repetitions per case and condition with a
scripted provider and frozen search script, without concurrent CPU-heavy work.
This is a separate Kernel/resource measurement, not evidence of model-directed
query quality. Report nearest-rank p50/p95, cold and warm conditions, and all
failures. The initial local first-request p95 ceiling is 1500 ms and local
retrieval-work p95 ceiling is 3000 ms. Remote whole-turn p95 is proposed at
120000 ms, with a 120000 ms total evaluation deadline and at most six actual
reader calls; timed-out turns remain failures in the report. These are pilot
proposals for this workload, not reuse of the different 250 ms #165 query gate.

Hard proposed resource bounds retain the implemented 8 actual Kernel searches, 8 held
evidence items, 12 KiB result cap, 36 KiB cumulative memory-message delivery,
3 seconds cumulative Kernel work, expansion before/after <=2, at most five
messages per expansion and 800 bytes per message. Use a 24576-token working
profile and 768-token output reserve for the comparison, subject to the actual
provider context profile supporting those values. The proposed additional whole
request cap is 98304 serialized bytes; independently verify actual snapshot hash
and size agreement. Do not treat token estimates as measured serialized bytes.
Report refused post-budget attempts separately. The diagnostic `SearchAttempts`
includes refusals and can exceed eight; it must not be confused with authorized
Kernel searches or used to weaken the eight-search work limit.

At this 61-record workload, propose active index coverage, exact restart/rebuild
agreement, build and rebuild wall time <=30000 ms each, database/WAL derived-index
growth <=32 MiB, and incremental worker RSS growth <=512 MiB after model warmup.
Report absolute RSS and model startup/storage separately; the RSS proposal does
not hide model memory by omitting it from the report. Record the exact derived
table byte accounting method before timing, rather than attributing the whole
event database to the index. A workload this small does not establish scalability;
the already-declared component-scale workload remains a separate required check.
Each case uses its own source database. Report its indexed record count and
costs, plus workload totals; 61 records across isolated cases must not be called
a measured 61-record query index without running that additional benchmark.

The operating matrix must separately retain successful empty, disabled/unavailable,
partial generator failure, timeout, cancellation/lease loss, exhaustion, source
restriction, index rebuild and restart outcomes. Zero deterministic boundary
violations, exact receipt/source agreement and all required recovery checks are
hard gates in every applicable condition. Every expected failure must keep its
real state and obey the existing deadline; no failed generator can count as an
exhaustive empty search. These operational checks supplement the 24 reader cases.

The operating diagnostic additionally declares 20 repetitions per cancellation,
lease-loss and deadline scenario, with a 1000 ms maximum tail from the signal or
ownership loss to terminated work. Report every measured tail, nearest-rank
p50/p95 and maximum; the shared bound is not a reason to drop a slower failure.
These values are declared before measurement and do not replace the existing
search, turn or local embedding deadlines.

One turn per case/condition is a limited pilot sample, not a reliability estimate
for all future conversations. Report both case-level numbers and denominators,
including per-family failures; never multiply the number of independent cases
by the number of local timing repetitions. The implementation owner must resolve
and document any proposed threshold or construction conflict before freezing
and running the pilot, then freeze release gates and procedure before #168.
