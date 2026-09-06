# Ticket147 engineering handoff

Branch: `codex/memory-stage-4`. No commit, real-index staging, branch/worktree,
push, model call/configuration, production dependency or user-owned file change.
Root owns final composition, both independent review axes, required full check
and one ticket commit.

The 16-path manifest is `147-engineering-checkpoint.json`; immutable task
variants live under `147-frozen-files`, predecessor originals under
`147-originals`. Base144 is `aab5b88357b025098c419dc9a93400ac398f20e3`, tree
`1151a945eab80163516bdbeecb07c8d77f1a1236`. The final compiler_work.go is frozen
before148's timing overlay (SHA256
`6ee8b7e5ac6d51fac3bdf4e1eafba278664213e02e5af3a38bd6d4a74cec7367`); never copy
its current live148 overlay into147. No db.go/main hook is needed.

One explicit later refinement clears only the source session in the *related*
key, permitting a fresh same-destination proposition with new evidence from
another authorized session to show its prior review. Exact suppression still
binds source session/root and all support/context fields. Initial versions from
root's `9d3d9d12b25db85d7288c11fb52e4e742db67e4a` are preserved in
`147-before-cross-session-files`; `147-cross-session-delta.json` lists exactly
three changed paths (canonicalizer, public integration test, implementation doc).

## Behavior and review entry points

- `compiler_recurrence_encoding.go`: separate versioned exact/related canonical
  projection; bound IDs and definitions, source-bound unresolved identities,
  typed polarity/modal/temporal/correction fields, policy and authority/hash
  manifests. Model confidence/uncertainty, reference order and timestamp zone
  spelling do not create semantic differences. Existing encodings stay intact.
- `compiler_recurrence.go`: indexed bounded classification plus current checked
  interpretation/review metadata within the publication transaction. Original
  outputs stay immutable and unresolved when suppressed. The actual edited
  primary/terminal result is linked. Accepted Claim, Entity, Source Link, Alias
  and correction/group effects are checked against current graph state. Changed
  effects get a fresh primary and explicit relation to the old resolution.
- `compiler_recurrence_schema.go`: immutable indexed side table; at most31 old
  candidates plus one cursor per startup transaction. Original candidate insertion
  order preserves the earliest primary even if migration reaches it later.
  Effect-change epochs keep a newly necessary primary separate from an old one.
  Legacy fallback reads one indexed old hash and proves canonical byte equality;
  inability to prove equality leaves a reviewable item instead of unsafe suppression.
- `compiler_recurrence_inspection.go` and CLI `memory-review lineage`: producing
  pinned configuration/selection, checked publication revisions, current owner
  interpretation, actual decision/reason/resolution. One direct link; current
  source/authorization checks cover both candidates and disclosed group members.
  Inspection is separate from preview/accepted-operation encodings.

No accepted memory, Predicate definition, generation, historical selection,
coverage, original extraction or previous review bytes are rewritten. Existing
activation/frontier, bounded history and worker fencing/capacity contracts remain.

## Checks

Exact commands, logs and development retries are in the manifest. Before the
small final related-key refinement, isolated broad Go tests passed:
`go test ./internal/eviedb ./internal/memory ./cmd/evie` (40.233s/0.159s/7.827s).
The full recurrence/CLI race selection passed (34.898s/3.694s). The new cross-session
case then passed (0.328s); final frozen race checks passed (37.808s/3.699s),
recorded in `147-final-cross-session-race.log`.
Root runs `./scripts/verify-change.sh` against its exact combined tree.

The deterministic suite covers every prior owner state (including accepted and
rejected edits), late delivery after edit/reject, decisions after suppression,
new support within/across sessions, accepted new identity and Predicate creation,
changed Claim/Source-Link lifecycle, policy/meaning/identity/time perturbations,
64-row legacy migration in31/31/2 pages and rollback,2,000-row query plans,
publication rollback and stage adoption without a second model call,
activation→explicit history→cancel/resume→distinct generation coverage,
closed-session CLI inspection and reopen, and model-independent accepted replay.
Real SQLite total_changes measured53 publication writes for16 outputs, including
inbox triggers; the test enforces the shared64-write ceiling so148's two diagnostic
writes fit without weakening the bound. No tests invoke a live extractor.

All frozen Go files are gofmt formatted. All16 per-file whitespace comparisons
emit no diagnostics (`--no-index` exit1 means changed contents, not whitespace).
Old v1/v2/v3/v4/v5 preview/operation goldens pass the broad suite. UI assets used
for owner CLI checks were copied generated artifacts; root builds its own assets.

## Demonstration and pending gates

`go test ./cmd/evie -run TestOwnerReviewCLIGenerationLineageClosedSessionAndReopen -count=1`
demonstrates CLI inspect→edit→reject→new generation→lineage after session closure
and database reopen. `memory-review lineage --scope SCOPE --id CANDIDATE_ID`
shows original versus actual reviewed meaning and preserved origin.

Actual model adequacy, David's pilot review observations and untouched holdout
readiness are separate pending evaluation gates. This ticket makes no claim that
any of them passed, enables no compiler by default and does not close the parent.

## Independent Spec finding: changed-correction recurrence

The independent review found a valid changed correction being reopened because
recurrence read the original Claim interval and compared it with projected
`OldAfter`. The original Claim deliberately stores `OldBefore` forever. A new
real-SQLite regression failed for both native review and a two-member compound
review before the fix (`147-correction-regression-before-fix.log`).

The fix keeps the immutable `OldBefore` check and reads the actual correction
ledger by its exact `(operation_id, old_claim_id)` identity, verifying the
replacement, destination, mode, effective time and all before/after/replacement
bounds. Lifecycle checks remain unchanged. The regression verifies suppression
for each original output after acceptance, then performs a genuine later
correction of the replacement and verifies that current-effect changes still
require fresh review, including every member of the affected compound group.
Canonical replay passes before and after that later correction.

Exactly three files changed; prior frozen bytes and manifest are retained under
`147-before-correction-fix`, and `147-correction-fix-delta.json` lists the hashes.
The sixteen-path manifest remains intact. The live148 compiler_work.go overlay
was not edited. Expanded focused normal checks passed (eviedb2.392s / CLI0.407s);
final race checks passed (eviedb53.779s / CLI3.680s), recorded in
`147-correction-final-race.log` and the manifest. Root owns the final isolated full verification
and independent rereview.
