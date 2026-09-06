# Proposed next extractor experiment: compact, ordered wire representation

Status: reviewable proposal only, 2026-09-05 UTC. No human output judgments,
inference, downloads, tests, commits, or changes to frozen artifacts were made.
This note does not approve the proposed Qwen adjudications or authorize a
production implementation.

## Recommendation and evidence

Run one bounded experiment with the already acquired Qwen model, replacing the
standalone model-facing wire representation with a lossless ordered source
projection, short request-local source references, and explicitly typed
subjects. Keep memory selection, evidence eligibility, supported meaning,
identity uncertainty, and human review requirements intact. Measure this before
another download or any relaxation of the memory requirements.

The [Qwen report](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development.json)
contains 20 completed responses, 30 raw proposals and 16 structurally retained
proposals. Four occurrences exactly match previously approved meanings: the
tea and denied café preferences, each twice. The incidental-meal window returns
empty twice. These observations demonstrate some success on simple cases; they
do not establish overall adequacy. The
[Qwen adjudication packet](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/output-adjudication-packet.md)
still has 13 distinct novel objects awaiting human judgments.

| Observed failure | Evidence | What a wire experiment can test |
| --- | --- | --- |
| Subject and destination scope become conflated. | Qwen N03-b and N06-a use the project scope UUID as subject; corrected Mistral does the same. | Removing opaque destination metadata from generated meaning and separating subject type from its value might reduce this confusion. It cannot prove that an owner or project is the correct semantic subject. |
| A notation placeholder becomes literal output. | Qwen N02-b emits exactly `new:Name` twice per traversal. | A subject tag plus a separate actual name field avoids requiring the model to interpret prefix notation. This hypothesis must be measured; the existing output must never be silently repaired to Maya. |
| Overlap is repeated without newly owned support. | Qwen N03-b repeats the old drinks discussion; N05-b repeats the old SQLite decision and offline constraint. The validator rejects these. Corrected Mistral also repeats overlap-only claims. | Chronological order with explicit `new`/`overlap` labels may help selection while retaining exactly the same source set. Short IDs alone will not make old support new. |
| Supporting sources are placed into assistant context. | Qwen N04-b places owner text in `context`; N08-b places the clock and old owner instruction there. The validator rejects these references. | Typed source aliases and one clearly ordered conversation view may reduce category confusion. Mechanical role validation still cannot establish entailment. |
| Polarity, relation, and temporal meaning remain wrong or incomplete. | Qwen N06-a emits affirmed employment after an explicit departure; N08-b emits a preference for “stopped drinking coffee,” without the checked date. | These remain semantic concerns even if every wire reference becomes valid. A representation change may have no effect on them. |

These are observations of the exact records plus hypotheses about a useful
experiment. They do not identify the input format or either model as the
conclusive cause. The
[corrected Mistral report](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/corrected-schema.json)
provides the shared-failure examples; no model-family conclusion is implied.
The Qwen report SHA-256 at inspection was
`15c64a287f0f46f33752137010897fc0c170ee6a133bda00e8e12a2c280be2ce`.

## Concrete proposed wire contract

**Input:** derive a new transport view from the unchanged frozen source windows.
Render precisely the selected support and assistant-context fields in their
original within-session sequence, retaining root boundaries, omissions, original
authority, and `new`/`overlap`/`context` labels. Source text stays byte-for-byte
identical after JSON decoding. Do not summarize, normalize, filter additional
sentences, combine fields, add future events, or select sources using gold.

Assign each supplied field a short alias such as `s1`, in that order. Each
visible entry needs its alias, role/authority, ownership, and exact text. The
standalone harness retains a sealed table mapping that alias to the complete
original source identity, scope, part, permitted projection, exact bytes/hash,
observed time, policy version and window identity. The model does not need to
copy UUIDs, hashes or destination scope. Bounded accepted Entity alternatives,
when present, use a separate alias table such as `a1`; they remain labeled as
identity context rather than evidence.

**Output references:** a source alias nominates its exact supplied whole-field
projection. Preserve an explicit optional UTF-8 byte-range selector for cases
requiring a smaller projection; for the contracted clock, offer a named `date`
selector that maps only to the existing permitted `0:10` projection. The model
must select support and interpretation-context references separately. Do not
silently move a reference between those categories. The range selector retains
the existing scalar-boundary, eligibility and bounds checks; an invalid selector
does not fall back to whole content.

**Output subjects:** replace the overloaded subject string with closed fields:

| Field | Meaning |
| --- | --- |
| `subject_type` | One of `owner`, `project`, `new_entity`, `accepted_entity`. |
| `subject_name` | Actual source-supported identifying name/description for `new_entity`; empty otherwise. It is a value copied from the source, not the literal word `Name` or a notation template. |
| `subject_entity_ref` | A request-local accepted-Entity alias for `accepted_entity`; empty otherwise. Unknown aliases and incompatible field combinations fail validation. |

Keep relation, object kind/value, polarity, fact/change/correction mode and
temporal qualification explicit. Do not let conversion infer those values.
The adapter can deterministically encode `owner` and `project`, bind an accepted
alias to its offered Entity ID, or preserve the supplied new name as an
unresolved source-bound proposal. A new name never resolves an existing Entity
or merges two same-name possibilities. Do not invent a missing name, repair a
placeholder, or resolve an ambiguous pronoun. Ambiguity remains visible or
requires abstention under the existing contract. A new production placeholder
encoding is outside this experiment.

**Validation and scoring:** bind the response to the exact sealed request and
alias table before expanding any reference. Unknown aliases, wrong reference
categories, mismatched window identity, invalid ranges and forbidden effects
remain explicit failures. Expand valid aliases to the existing canonical
candidate/evidence representation before using the same approved gold. Persist
raw wire output, the alias table/hash, expanded output and rejection reasons as
separate auditable artifacts. Count every raw proposal, including rejected ones;
do not present better retention as better meaning. Never transform earlier
Mistral/Qwen reports retrospectively or apply the proposed adjudication labels
without actual human review.

## Bounded execution proposal

Use the same ten development windows in the Qwen manifest, one request each,
schema mode, the same pinned Qwen artifact/runtime, temperature 0, seed 17,
context 8192, output cap 768, 60-second timeout, and one inference request at a
time. One traversal is sufficient for this initial diagnosis: all ten prior
temperature-zero pairs were identical. This is a comparison of two complete
wire configurations, not an isolation of the effect of shorter IDs alone.

Freeze the revised transport projection, schema, prompt, alias-table encoding,
deterministic expansion and validation versions before dispatch. Re-establish
the model-specific predispatch context bound for the actual rendered requests;
the old request hashes and empirical counts do not apply. Preserve the current
capacity-release stop rule. No extra repair requests or enlarged output budget
are part of this proposal.

Report reference/category errors, literal-placeholder/invalid-subject encoding,
whole-response failures and canonical exact approved-meaning matches separately.
Keep unsupported/no-memory proposals and unresolved human judgments visible.
No new release threshold is proposed. Improvement would justify broader
development/pilot evaluation; no improvement would leave this representation
hypothesis unsupported. Neither outcome by itself closes #135 or clears #136.

## Binding-contract assessment

This is a standalone wire/prompt/schema configuration change, not a required
change to the memory requirements, **provided the expansion is lossless and the
boundaries above remain enforced**:

- Kernel-owned destination is already required: the extractor cannot choose or
  widen scope. Removing model-authored scope agrees with the
  [evidence contract, scope invariant](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:35).
- The Kernel already resolves full source identity, scope, authority and hashes;
  the extractor only nominates locations. Request-local aliases are nomination
  syntax, not replacements for retained provenance. See
  [exact projection](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:165).
- Ordered presentation is already required for selected overlap. The new view
  must retain exactly the existing windows and new-evidence ownership. See
  [ordering and ownership](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:287).
- Separate support/context and the same exact deterministic projection through
  validation, review and inspection remain required. See
  [source manifests and resolver](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:200).
- Prompt, extraction schema, validation and effect encoding changes require a
  new pinned Compiler Generation if later used in production. They cannot
  mutate existing sealed work or reinterpret retained outputs. See
  [generation identity](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md:51)
  and [sealed work manifest](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md:135).

Changes that **would** require a contract revision include dropping exact
references/hashes, treating an alias as durable provenance, normalizing or
summarizing source bytes, making assistant or uncontracted tool text support,
changing windows or overlap limits, merging same-name identities, or losing
subject/negation/time to simplify output. These conflict respectively with
the cited projection/manifest rules,
[meaning preservation and identity](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:78),
[window limits](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:276),
and [source-bound unresolved identity](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md:294).
None is recommended here. A JSON transport wrapper also does not authorize
JSON-pointer factual evidence; the
[structured-field prohibition](/Users/davidboktor/code/evie/cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md:192)
continues to apply to original event content/payload.
