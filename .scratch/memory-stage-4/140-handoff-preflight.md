# #140 closed-session review implementation preflight

Owner: `/root/implement_140_owner_review`. Branch `codex/memory-stage-4`.
No staging, commits, branch changes, or model calls by this owner.

Read: implement skill, AGENTS.md, #140 published body, Stage 4 parent and all
three binding contracts. Heavy tests remain paused for model measurement.

## Bounded initial outcome

Review the current #136 proposal shape: existing subject and Predicate identities,
a typed literal or already resolved object identity, polarity, exact Valid Time,
and sealed support/context. New Entity/Predicate choices belong to #141;
corrections belong to #142; contracted tool evidence belongs to #143; owner edits
and multi-group dependent batches belong to #144. A #140 preview contains one exact independently reviewable CandidateRef.
The current types introduce no shared proposed identities; later dependency
groups remain indivisible. An extraction group is not a review dependency group.

A local trusted owner entry point obtains `eviedb.OwnerReviewContext` with
unexported capability fields. It binds the database authentication secret,
local principal, exact selected scope, allowed actions, durable authorization
revision, and server-resolved session lineage. Request JSON cannot mint it.
The Kernel rechecks current authorization and registry availability for every
read, preparation, and committed review. Closing the source session does not
require a new turn, lease, or source event.

Proposed API: `LocalOwnerReviewContext`, `ListOwnerCandidates`,
`InspectOwnerCandidate`, `PrepareOwnerCandidateReview`, `ResolveOwnerCandidateReview`,
and `InspectOwnerReviewOperation`. Domain request/results live in a new
`internal/memory/candidate_review.go`; opaque authority lives in eviedb.

## Transactions and evidence

Preparation reads one SQLite snapshot, checks current candidate revisions,
full source binding and policy, identities/equality/conflicts, and all read
scope revisions. Persist immutable canonical preview bytes. Acceptance names
only its ID/digest, exact accept/reject action, and unique delivery key; no
replacement client effect payload is accepted. Deterministic fixed struct/set
ordering and domain-separated SHA256 freeze effects and disclosure.

Resolve uses `Store.withImmediateTransaction`. Current authorization then a
stored identical delivery result are checked first; a differing request with
that delivery key conflicts. Current preview policy and scope vector are
checked before effects. Recheck candidate revision and every required source
projection inside the transaction. Record the versioned accepted operation,
Claim/Source Links/state events, semantic revisions, review resolution/audit,
and delivery result together. Rejection writes no semantic operation/revision,
and can prepare a source-redacted safe preview after eligibility changes.

Use #136's projection and source revalidation helper to preserve the same bytes,
format/policy, support/context distinction and original authority. Source hash
encodings must bridge #136 raw lowercase SHA256 and existing Stage3 `sha256:`
format explicitly. Temporal qualification remains in the accepted source
interpretation envelope; no unknown date is invented.

## Accepted-operation and replay seam

Do not call Stage3 Prepare/Apply methods using a fabricated scope or turn lease.
Reuse package-private canonical validators, equality SQL, scope-vector checker,
recordAcceptedSemanticOperation, and state-event semantics under the new typed
review authority. Add operation schema version 6 and kind
`owner_candidate_review`; its prepared envelope contains full canonical effect,
source interpretation, original source session, authenticated owner binding,
preview/effect digests and candidate/revision/audit IDs. Source event remains
the original first supporting event for historical envelope compatibility.

Add a v6 replay handler. Replay calls the private effect writer from recorded
canonical effects, checks envelope/digests and resulting revisions, and neither
mints owner authorization nor acquires/resumes a source turn. Skip existing
ensureReplayLease for v6. The shadow already snapshots source events before
resetting only accepted projections; review metadata stays outside projection.
Unsupported versions remain rejected. Review metadata must not reference
replay-reset semantic rows with foreign keys that would make shadow reset fail.

Schema migration requires extending all earlier version guards to recognize v6
without downgrading on reopen, adding v6 migration, and leaving old envelopes
byte-identical. Review tables are additive, initialized after compiler schema.

## Source rendering seam

`loadSourceForInspection` currently joins whole `events.content`; it must project
and verify the stored locator for review-created sources. Existing Stage3 whole
sources retain existing behavior. Also audit exact-read source loading in
semantic_correction.go and promotion source loading in semantic_promotion.go:
review-created byte ranges cannot be expanded back to whole events or compared
incorrectly during later accepted operations. Current policy redacts unsafe
review source text and any stored v6 operation JSON exposing it. Authorized
owner operation inspection returns required context with the same policy gate.

## Ownership requested

New files: internal/memory/candidate_review.go;
internal/eviedb/candidate_review*.go and tests;
cmd/evie/candidate_review.go and tests.

Narrow existing hooks requested after root handshake:
- semantic.go: v6 schema/migration dispatch and earlier-version detection only.
- semantic_replay.go: v6 handler dispatch and skip lease for v6 only.
- semantic_lifecycle.go: exact source rendering and current-policy redaction of
  v6 stored inspection payloads.
- semantic_correction.go / semantic_promotion.go: source projection call sites
  only if required for existing explicit-command continuity.
- compiler_schema.go / db.go: owner of #136 installs review-schema initializer;
  #140 does not edit concurrently.
- main.go: #136 owner/root installs review command dispatcher; #140 supplies it.

Pending #136 handoff: stable candidate loader returning candidate/job/generation,
review_revision, interpretation_revision (initial zero), destination and sealed
window; stable revalidateCompilerSources/project helper; complete group IDs and
candidate ordering; candidate envelope immutable and review_revision CAS fields.

## Meaningful verification

Real SQLite tests at trusted Kernel seam with scripted compilation:
closed source accepted without changed session status/lease/event count;
global, Workspace, project, exact session visibility plus negative matrix;
zero candidates in accepted reads before review; exact UTF8 projection hash
persists through accepted inspection and reopen/replay; original source authority
unchanged; multiple support/context and no assistant supporting Source Link;
stale semantic vector/current policy/identity/candidate revision rejected;
source missing/redacted blocks accept but permits redacted reject; duplicate key
returns original result, differing key payload conflicts; concurrent stores
accept/reject only one terminal outcome; transaction failure/cancellation leaves
no operation/audit/resolution; equality reuses Claims and eligible sources without
restoring retracted source; unknown times remain null; old Stage3 foreign-session
and expired-lease checks still fail. Golden canonical preview/effect fixtures.

CLI tests should call run helper with temp SQLite, explicit `--scope`, bounded
list/inspect, prepare, and explicit approve by preview/digest/delivery key; no
OpenRouter configuration or source-session resumption. Verify migrations from
v5 with existing operation rows, reopen v6 without migration downgrade, exact
projection replay and quarantine behavior. Focused tests after measurement;
root runs ./scripts/verify-change.sh and controls commit/review boundary.

## Final implementation adjustment

Root approved single CandidateRef preparation after identifying mixed extraction
groups containing a suppressed recurrence plus fresh independent work. CLI now
uses --id/--revision/--interpretation, and the mixed-job regression passes.
The schema initializer and CLI dispatcher were installed by root/#136 owner.
