## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Extend the bounded compiler into a recoverable worker lifecycle using the frozen work contract. Demonstrate selected jobs surviving interruption and safely competing across cooperating Evie processes, with later independent work progressing past failed earlier work. Keep worker state and capacity authoritative in persistence.

## Acceptance criteria

- [ ] Recover pending and contractually eligible interrupted/staged work after restart with idempotent candidate-group and completion commits. Channels or process memory are never the only work record.
- [ ] Enforce the frozen retry/failure/repair accounting, attempt ceiling and backoff. Expose distinct pending, running, retrying, failed, cancelled, skipped-if-supported, and successful-empty outcomes.
- [ ] Fence stale workers after expiry, replacement, cancellation, and shutdown. Late responses cannot persist candidates or accepted effects; competing stores/processes cannot commit inconsistent completion.
- [ ] Observe at most one active local inference request across cooperating processes, including cancellation or uncertain server release. Exercise the contract's recovery behavior instead of assuming a cancelled client freed server capacity.
- [ ] Respect bounded queue, staged output, input/output, and database batches. Preserve the work contract's scheduling metadata and bounds; the full new-evidence/backfill priority demonstration belongs to the activation/backfill paths once both exist.
- [ ] Let later independent candidates become visible while an earlier selected source unit remains unresolved. Exact coverage and contiguous frontier retain the gap, and missing output cannot be recast as zero-candidate success.
- [ ] Expose bounded safe status and actionable recovery information through the existing inspection seam without raw protected content.
- [ ] Use real SQLite and multiple stores/processes to inject crashes around durable boundaries, stalled/late extraction, duplicate delivery, lease loss, and shutdown. Run focused checks and repository-required full change verification.

## Blocked by

- Draft 05: Compile one selected source unit into durable candidates
