# Stage 4 closed-session owner review

Implementation of [#140](https://github.com/davidadel66/evie/issues/140), under
the [owner-review contract](../active/memory-stage-4-review-contract.decisions.md).
This engineering record does not qualify an extractor or replace the Stage 4
human evaluation gates.

The trusted local CLI obtains an opaque `eviedb.OwnerReviewContext` from the
Kernel for one explicit scope. Request JSON cannot create that capability.
Its authentication binding and authorization revision are checked against
SQLite on every read and committing review. Source sessions remain closed;
review never creates a source event, takes a source turn lease, or calls a
session-bound Semantic Memory apply API.

## Local demonstration

Use a candidate produced by explicitly configured compilation. Inspect the
returned `ref` and copy its exact candidate ID and both revisions:

```sh
evie memory-review inbox --scope global --limit 50
evie memory-review inspect --scope global --id CANDIDATE_ID
evie memory-review prepare --scope global --id CANDIDATE_ID \
  --revision 0 --interpretation 0 --action accept
```

The preview displays original evidence and interpretation context, exact UTF-8
byte ranges and hashes, original authority, current subject/Predicate/object
identities, scope revisions, conflicts, unknown Valid Time bounds, and each
create/reuse effect. Review that complete preview before the explicit resolve:

```sh
evie memory-review resolve --scope global --preview PREVIEW_ID \
  --digest sha256:PREVIEW_HASH --delivery idem:v1:NEW_UUID_V4 --action accept
evie memory-review operation --scope global --id ACCEPTED_OPERATION_ID
```

For rejection, prepare and resolve with `--action reject`. A rejection preview
can disclose a safe redacted candidate identity when supporting text is no
longer eligible. Optional `--reason` is bounded to 4 KiB and secret-scanned.
Rejection records no Semantic Operation and advances no semantic revision.

The scope must be exactly `global`, `workspace:REGISTERED_ID`,
`project:REGISTERED_ID`, or `session:DURABLE_SESSION_ID`. Context Scope inboxes
exclude session-scoped candidates. A session review can use its allowed Context
Scope/global identities while preserving its selected session destination.
A different session sharing that Context Scope cannot access this inbox.
Closing a source session is allowed; revoking its registered scope is not.

## Exact interpretation and transaction

The first slice prepares one `CandidateRef` using already accepted subject,
object and Predicate identities. Each current candidate is an independent
review unit because it introduces no shared proposed identity. A suppressed
recurrence has no independent executable preview, but a fresh sibling from the
same extraction job remains reviewable. An extraction group is not implicitly
a dependency-connected review group. New identities/Predicates, corrections,
contracted tool observations, owner edits and compound batches remain the
respective later ticket outcomes.

The versioned canonical encodings use fixed struct field order, ordered
references, explicit nulls, domain-separated SHA-256, and existing typed literal
semantics. Golden bytes/hashes live in
`internal/eviedb/testdata/candidate_review_encoding_v1.json`. Rendering whitespace
is outside identity. New Transaction Time is assigned by the accepting commit;
unknown real-world time remains unknown.

Preparation seals source projections against the immutable request and original
staged output. Acceptance checks the exact durable preview, current authorization,
source policy, current eligibility, candidate revisions, complete semantic scope
vector, identities, conflicts and equality under one immediate SQLite transaction.
It records one schema-v6 `owner_candidate_review` operation, accepted effects,
Source Links, state events, semantic revisions, review resolution/audit, and
idempotent delivery result together. A failure or cancellation commits none.
An equal active Claim is reused with explicitly reviewed new support; a retracted
Source Link or inactive equal Claim requires an explicit later lifecycle choice.

A duplicate delivery key with identical request returns the stored committed
result even if the source policy subsequently changes. A differing request
conflicts. A competing terminal review returns `already_resolved` and the
recorded authorized result. A stale preview changes neither review nor graph
state; preparing a replacement requires new explicit approval.

## Accepted history and inspection

The accepted envelope retains source interpretation, exact selected projection,
original source session/authority, owner approval binding, candidate revisions,
and preview/effect identities. Source inspection and ordinary exact accepted
reads project selected byte ranges instead of expanding the whole event. The
existing Stage 3 active-session read/API checks remain intact; the new owner
operation inspection is the explicit scope-level closed-session provenance path.

Current source policy can redact old source quotes and the stored operation JSON
that would expose them. Promotion follows exact Source Link provenance back to
the original review, including its supporting assertions and assistant context.
Preparing or applying a Promotion rechecks that origin's current eligibility;
redacted or empty reviewed evidence cannot authorize it. Later source inspection
and stored Promotion payloads use the same disclosure rule.

These checks do not rewrite accepted history. Replay verifies the recorded
support and context against immutable event identity, lineage, metadata, UTF-8
projection, exact bytes and hash. Corruption fails verification and quarantines
the affected projection. A later source policy, authorization, registry, or
session-status change does not invalidate the historical interpretation. Replay
uses the recorded canonical operation and effect writer without live owner
authentication, source-turn authority, extraction, or model calls. The schema
migration preserves old accepted bytes, and both startup compatibility and
shadow replay recognize only the exact v6 operation kind.

Review history remains retained and can grow. Inbox pages are bounded to 100
(default 50), revision-bound, and scoped; source/stage envelopes retain compiler
bounds, and a full preview is capped at 256 KiB. No automatic approval, default
model installation, production activation, or evaluation claim follows from
these deterministic tests.
