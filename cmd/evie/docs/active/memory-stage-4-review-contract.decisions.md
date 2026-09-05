# Memory Stage 4 owner-review contract

Status: binding technical contract, 2026-09-04, within the owner's accepted
Stage 4 review decisions. Numeric bounds and transaction mechanics below are
implementation defaults, not additional owner evaluations or measured budgets.

This completes [ticket #134](https://github.com/davidadel66/evie/issues/134), under
the [published Stage 4 specification, issue #131](https://github.com/davidadel66/evie/issues/131)
and [memory decisions](memory.decisions.md).
It depends on the [evidence contract](memory-stage-4-evidence-contract.decisions.md).
The [worked scenarios](memory-stage-4-review-contract.fixtures.md) are shared
Kernel, CLI, and web acceptance oracles. Exact commands, routes, storage schema,
and implementation belong to later tickets.

## Decisions and boundary

| Decision | Binding behavior |
| --- | --- |
| R1: scope owner authority | An authenticated owner explicitly selects one exact review scope. The Kernel grants typed review authority without acquiring a source-session turn lease, appending a source-session approval event, or reopening that session. Existing Stage 3 prepare/apply authority is unchanged. |
| R2: exact approval | Acceptance names an immutable preview and its digest, exact interpretation revisions, and a unique delivery key. It cannot approve an intention, an inbox query, future candidates, or refreshed effects. |
| R3: edit lineage | Editing appends an owner interpretation revision under the same unresolved candidate. Original extraction, evidence, generation, alternatives, and earlier review records remain retained. Approval is never new factual evidence. |
| R4: bounded independent groups | A dependency-connected group succeeds or fails together. A bounded batch may commit independent successful groups and record failures for the others, with that behavior disclosed before approval. |
| R5: exact recurrence | Equivalent later suggestions link preserved earlier review decisions without emitting another Semantic Operation or inheriting acceptance authority. Changed evidence or effect requires fresh review. |

The Kernel owns the new review seam. Reuse canonical semantic validation,
transaction primitives, inspection, and replay beneath it; do not relax the
public Stage 3 session/source/turn-lease checks to implement R1. Compilation,
review decisions, and accepted knowledge are separate lifecycles. Candidates
remain absent from ordinary accepted queries and traversal until an explicit
accepted operation changes the graph. No model, tool, prompt, confidence score,
worker lease, browser field, or candidate ID supplies owner authority.

## Typed authority and exact scope

The following are language-neutral contract types, not final Go declarations:

| Type | Required content and invariant |
| --- | --- |
| `OwnerReviewContext` | Kernel-verified local owner principal, authentication binding, one exact selected scope ID, permitted review actions, current authorization version, and server-resolved source lineage. It is obtained from the trusted owner entry point, never deserialized as trusted authority from request JSON. |
| `CandidateRef` | Immutable candidate ID plus interpretation revision and review revision; candidate includes generation/group/window identity and harness-bound destination. |
| `ReviewPreview` | Version, preview ID, owner/scope binding, ordered atomic groups, candidate refs, exact canonical effect manifest and effect digest, complete disclosure manifest, read/write scope revision vector, current source-policy identity, and preview digest. |
| `ReviewDecision` | Unique owner-scoped delivery key, preview ID and digest, explicit accept/reject action for every selected group, and optional bounded owner reason. Unselected items carry no approval. |
| `ReviewResult` | Delivery key, preview ID, committed outcome per group, resulting review revisions, review/audit IDs, any accepted operation IDs, and resulting semantic revisions; failure codes contain no raw protected source text. |

Local CLI and owner web authentication retain their current trust boundaries.
Web review retains existing owner approval, origin, and request protections;
untrusted model-visible tools cannot mint the context. Each read, edit,
preparation, and resolution checks current authentication and authorization.
Revocation takes effect at the committing transaction. Granting one scope does
not confer a cross-scope inbox or source search. Authorization failure does not
reveal whether an inaccessible candidate exists.

| Explicit owner selection | Candidate listing/review | Semantic references and evidence |
| --- | --- | --- |
| `global` | Candidates whose destination is global | Existing global reference rules; no Workspace, project, or other session evidence is imported. |
| `workspace:w` | Candidates whose destination is that registered Workspace | That Workspace and allowed global identities/Predicates; exact source-session lineage must match. Other Workspaces and projects are excluded. |
| `project:p` | Candidates whose destination is that registered project | That project and allowed global identities/Predicates; exact source-session lineage must match. Other projects and Workspaces are excluded. |
| `session:s` | Candidates whose destination is that exact durable session | Its own session scope, its immutable Context Scope if any, and global references under existing Stage 3 rules. Another session sharing the Context Scope does not gain this access. |

Closing or archiving the source conversation alone does not revoke explicit
owner review or explicit authorized inspection. Scope registry/access
revocations still apply. Selecting `session:s` for review does not resume it or
make its memory available to ordinary retrieval elsewhere. Global and Context
Scope inboxes do not implicitly include session-scoped candidates. Source
expansion is limited to the evidence/context authorized for the selected scope;
broader semantic visibility never grants raw narrower-source visibility.

Every candidate keeps its selected destination. An edit cannot change it.
Broader reuse follows the existing explicit Promotion contract after acceptance;
candidate review cannot perform an implicit Promotion. Global owner/Evie
anchors and global Predicate definitions retain their existing allowed
structural write rules; their presence does not make a local Claim global.

## Preparation, disclosure, and freshness

Preparation reads a consistent SQLite snapshot. It validates unresolved
candidate/review revisions, registered scope lineage, references, current
source eligibility, and canonical semantic rules before creating a preview.
An unresolved identity or correction choice returns `needs_choice`, with
bounded authorized alternatives; it is not an executable preview. The owner
records a choice through an interpretation revision and prepares again.

The effect manifest enumerates all create/reuse identities, canonical Typed
Literals, Predicate identity/version/definition, Claims and polarity, Source
Links, and lifecycle/temporal transitions. It records known prior and resulting
values and generated IDs, not instructions to resolve them later. New Predicate
definitions are visible global effects; an existing definition cannot be
silently replaced. Same-name Entities remain alternatives; choosing a concrete
existing ID or creating a distinct Entity is explicit. Exact Claim equality
may yield reuse plus new Source Links, rather than a duplicate Claim. Reusing a
retracted source never restores it implicitly.

The disclosure manifest binds every required supporting and interpretation
context reference: event/session/scope, original authority, allowed part and
locator, exact bytes/hash, observed time, policy/format version, and each
effect's attribution. It preserves the evidence contract's support/context
distinction. Assistant context has authority `none` and never becomes a
supporting Source Link. Owner and contracted tool support retain separate
authorities even when the owner accepts both together. All needed context
survives acceptance in a source-interpretation manifest bound to the operation
and exposed by authorized accepted-source inspection.

The preview also shows uncertainty, conflicts, candidate/generation/edit origin,
identity choices, scope, each group's dependencies, and batch failure behavior.
It discloses `error` versus real-world `changed` correction, exact affected
Claim IDs, prior/next polarity and Valid Time, and any enumerated lifecycle
effects. Unknown bounds remain unknown; Transaction Time comes from the
accepted commit, never from an invented world date. Future intention is not a
completed change. An unzoned `get_time` display is not converted to a UTC
effective instant. Contradictions cannot choose a winner or trigger a cascade.

Both digests use SHA-256 over a versioned deterministic encoding. The effect
digest covers the closed canonical semantic payload including generated IDs,
sources, context manifest, destination, and dependencies. The preview digest
additionally binds candidate/interpretation/review IDs, disclosure, group order,
chosen actions, revision vector, owner authorization binding, and evidence
policy. Canonical semantic values retain the
[existing encodings](semantic-memory-encodings.spec.md); transport order,
HTML/Markdown rendering, or JSON whitespace is not identity. The encoding must
have fixed field/set ordering, explicit null/unknown representation, and a
versioned domain separator, with golden bytes/hashes in the implementation
ticket. Changing any bound value creates a new preview ID/digest. Client
supplied payloads are compared with the durable Kernel preview, not trusted as
replacement effects.

All scopes read for identity, equality, conflict, lifecycle, or source state,
including global Predicate state, participate in the revision vector. At
resolution the Kernel checks the same candidate and authorization revisions,
scope vector, source bytes/hashes, secret detector, source-policy identity,
lineage, and eligibility again inside the write transaction. Immutable bytes
do not imply continuing eligibility. A changed source-policy identity,
including changed detector rules, makes the entire preview `stale_preview`
before any batch group applies, even if a visible quotation happens to match.
The same pinned deterministic detector cannot reclassify identical bytes
without a policy change. Pure source-session closure is not an eligibility
change.

`stale_preview` changes no candidate resolution or graph state. Refresh prepares
a new preview against current state and requires new explicit approval even
when the displayed proposition appears unchanged. No automatic rebase, hidden
new IDs, substituted sources, or carried approval is permitted. Ineligible or
missing support/context produces a specific safe failure and blocks its whole
dependent group; source redaction is not permission to approve unseen effects.
Current source eligibility also governs inspection of old previews and audit
records, so stored quotes cannot bypass a later secret exclusion.

## Edits, decisions, and repeated delivery

Candidate review state is `unresolved`, `accepted`, or `rejected`. Preparation
does not reserve or resolve the candidate. Editing while unresolved atomically
appends an immutable interpretation revision and owner audit, then advances its
interpretation and review revisions by compare-and-swap. Existing previews
become stale. The edit records its parent revision, exact typed before/after
meaning, changed identity/temporal choices, and optional reason; the original
extraction and evidence are never overwritten. An abandoned edit remains
inspectable and unresolved.

An edit can correct an interpretation supported by the recorded eligible
evidence, including sufficient ranges from its frozen window. It cannot insert
an unrelated source, widen scope, upgrade authority, or silently turn owner
review text into new owner-statement evidence. A new factual assertion uses the
existing explicit-memory path or a later compiled owner episode. Deterministic
validation checks structure and evidence binding; it does not claim to prove
entailment. The owner still sees the changed interpretation beside its sources.

| Requested action | Durable outcome |
| --- | --- |
| Accept exact current preview | Resolve all member candidates `accepted` with their selected interpretation revisions, one group audit and one canonical compound Semantic Operation. A fully reused effect may have no new graph rows, but still records the exact reviewed operation under existing idempotence/revision rules. |
| Reject current candidate revision | Resolve it `rejected` with immutable decision, selected interpretation, and optional reason; emit no Semantic Operation and change no semantic scope revision. Rejection may use a safe redacted candidate summary when evidence is now ineligible; no source/effect approval is implied. |
| Same delivery key and identical request after a committed result | Return that stored outcome, subject to current authorization/redaction. Do not rerun source validation as a fresh acceptance or create another decision/operation. |
| Same delivery key with a different request | `idempotency_conflict`; no effects. |
| Different delivery against an already accepted/rejected candidate | `already_resolved` with the authorized recorded resolution; never change accepted into rejected or vice versa. |
| Edit or accept against an old interpretation/review revision | `stale_preview` unless already terminal, in which case `already_resolved`; never overwrite the winning review. |

Rejection requires candidate visibility and review revision, but no valid graph
effect or source support: the owner can dismiss a now-invalid suggestion. Its
own decision preview binds that identity, revision, action, and safe disclosure.
It does not authorize a later acceptance. Correcting accepted memory requires a
new explicit semantic operation; reopening a terminal review is not part of v1.

The durable-work contract owns exact cross-generation equivalence. Exact
effect, identity alternatives, support/context manifest, original authority,
and scope equality may link a recurrence to an existing unresolved, edited,
accepted, or rejected interpretation. Suppression is presentation metadata, not
copied terminal state or approval. Preserve owner-edited representatives and
all earlier decision/operation links. A new supporting source is a new effect
requiring fresh review, even for an equal accepted Claim; an earlier rejection
remains visible without silently rejecting different evidence. Compilation and
review serialize equivalence links against review revisions so late worker
delivery cannot replace an owner edit or revive resolved inbox work.

## Groups, transactions, and recovery

A group contains the complete dependency closure: newly created or selected
Entity/Alias identities, Predicate additions, sourced Claims, and any correction
or lifecycle changes. No partial selection inside that closure is executable.
Independent groups cannot read or reuse another group's proposed new identity,
write the same semantic object, or assume another group's success. Such items
must be combined into one bounded group or prepared separately after commit.
Cycles or unresolved dependencies return `invalid_dependencies`.

Technical initial bounds are one exact destination per batch, at most 20
groups, 64 candidate references, 256 enumerated semantic effect records, and
256 KiB of complete canonical preview bytes including evidence/context. These
are inclusive hard ceilings, not targets. Exceeding any bound returns
`review_too_large` before approval; do not truncate disclosure or silently split
an atomic group. Inbox pages default to 50 and cap at 100, pinned to the selected
scope's inbox revision; changed pagination returns a stale cursor. Owner edit
and rejection reasons cap at 4 KiB each and undergo secret scanning.

The bounded batch uses one outer SQLite write transaction with a savepoint per
independent group. Authenticate and check the preview's complete starting
scope vector and source-policy identity before any effects. An externally
changed vector or policy identity rejects the whole batch as stale. Evaluate
each group's candidate revisions, evidence,
dependencies, and conflicts under that same serialized transaction. The
declared group order determines operation order. Own earlier successful groups
may advance revisions; subsequent checks account only for those enumerated
advances, never silently reload/rebase onto an outside graph change.

For an accepted group, atomically write its Semantic Operation, all accepted
effects and source-interpretation manifest, semantic revision advances,
candidate resolutions/review revisions, audit, and delivery result. A failed
group rolls its savepoint back completely, remains unresolved (or retains a
competing terminal resolution), and contributes only a safe failed result.
Independent groups continue. Rejection similarly commits resolution and audit
together without graph effects. The final commit durably records successful
groups and the ordered per-group delivery result together. No success is
reported before commit. A whole transaction/commit failure or cancellation
before commit rolls everything back, including review resolution and audit.

If response delivery fails after commit, querying or retrying the same delivery
key returns the stored result. If commit was absent, that same request may be
retried subject to current freshness checks. A committed partly failing batch
is immutable: retrying its key does not retry its failed groups; new attempts
prepare and approve new previews. Competing processes serialize in SQLite;
only one decision can resolve a candidate revision. No process-local lock or
source-session lease substitutes for the durable checks.

Accepted operations carry a new versioned authority envelope identifying
`owner_candidate_review`, authenticated owner approval/audit, preview/effect
digests, source-interpretation manifest, and originating candidate/interpretation
IDs. Original source authority stays in each source; model/prompt/confidence
metadata stays in candidate/audit records, not Claim truth. Replay verifies the
recorded accepted contract and canonical effects without authenticating a live
owner, reopening a session, invoking a model/tool, or reinterpreting old source
text under a new extractor. Current visibility and secret policy govern source
rendering, not historical operation rewriting. Unsupported operation versions
fail closed; existing projection quarantine/rebuild and Stage 3 versioned
replay behavior remain intact.

## Verification and remaining implementation

The scenarios exercise the Kernel seam with real temporary SQLite, scripted
extraction, restart, competing stores, and deterministic source-policy changes.
CLI and web must expose the same effect/source hashes, outcomes, and scopes.
The implementation must freeze versioned envelope/preview encodings with golden
fixtures and cover commit failure, cancellation, duplicate response delivery,
source-policy redaction, and both cross-process resolution orders. Existing
Stage 3 tests must still reject foreign-session and expired-lease applies.

Documentation verification is `git diff --check` plus local Markdown link
validation. Go/vet/UI and live-model tests are not applicable to this
documentation-only ticket; dependent implementation tickets run their focused
checks and `./scripts/verify-change.sh`. No new production dependency, cleanup,
automatic acceptance, or adapter route is authorized by this record.
