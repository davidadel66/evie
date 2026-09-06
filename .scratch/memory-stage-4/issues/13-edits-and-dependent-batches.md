## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Add CLI editing and bounded batch review over the supported candidate effects using the frozen owner-review contract. Preserve the original extraction and evidence while making edited interpretations, resolution choices, dependencies and batch failure behavior explicit.

## Acceptance criteria

- [ ] Edits create the contractually defined lineage without overwriting original extraction, evidence, generation, or prior review decisions. Edited evidence/meaning passes the same validation as other review effects.
- [ ] Preview exact edited values, identities, Predicate additions, temporal/correction actions, source authority, scopes and dependent changes before approval.
- [ ] Implement the frozen batch bound, atomic grouping and independent-item failure semantics. An invalid or stale member produces the specified observable result rather than partially applying an unspecified compound action.
- [ ] Bind approval to the exact edited/batched effect and revalidate revisions, current eligibility, identity and conflicts. Changed dependencies require the specified refreshed review.
- [ ] Preserve source authority separately from owner edit/approval audit; neither an edit nor a batch creates implicit Promotion or unsupported source access.
- [ ] Concurrent/repeated review, cancellation and crash/reopen preserve one durable resolution and complete accepted effects according to the contract. Rejection remains inspectable.
- [ ] Demonstrate mixed supported relationship, temporal/correction and contracted-observation candidates through CLI edit, batch preview, acceptance and canonical replay.
- [ ] Run deterministic real-SQLite lineage, stale/dependency, partial-failure, scope, crash and concurrency tests plus repository-required full change verification.

## Blocked by

- Draft 10: Review people, relationships, and new Predicate definitions
- Draft 11: Review temporal changes, corrections, and additional support
- Draft 12: Compile and review the initial contracted tool observation

