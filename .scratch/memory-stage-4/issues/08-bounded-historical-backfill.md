## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Add the owner-facing bounded backfill path over retained eligible history, including history from before Stage 4. Selection, progress inspection, cancellation and any permitted skip behavior must follow the frozen contract. New evidence continues receiving priority while independent historical jobs can finish out of order.

## Acceptance criteria

- [ ] Require explicit bounded scopes and source ranges for historical work, with a reviewable description of the selection. Activating new-evidence processing alone never selects all history.
- [ ] Persist selections and reconcile only uncovered eligible evidence within them. Overlapping/repeated requests follow the defined idempotency and generation semantics.
- [ ] Display exact completed ranges, unresolved gaps, contiguous frontier, successful empty results, failure/retry/cancellation/skip states, and history outside selection without conflation.
- [ ] A failed earlier range does not prevent later independent candidates from appearing; a later completion does not move the contiguous frontier across the failure.
- [ ] Exercise queue and database bounds and show new evidence progressing while catch-up work uses the contractually available capacity.
- [ ] Backfill remains restartable and cancellable without discarding accepted memory, original episodes, review audit, or committed coverage.
- [ ] Demonstrate selection and progress through the CLI/shared Kernel seam with real-SQLite history spanning sessions and scopes. Test overlap, failure gaps, cancellation, priority, and reopen; run repository-required full change verification.

## Blocked by

- Draft 07: Activate background compilation for new evidence

