## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Extend supported candidate effects for existing unambiguous identities to typed meaning, polarity, temporal claims, lifecycle corrections, and additional sources. Use Stage 3 semantics and exact review to preserve history and uncertainty. This slice is independent of introducing new identity-resolution choices.

## Acceptance criteria

- [ ] Preserve Typed Literal meaning, polarity and Predicate semantics. Unknown temporal bounds remain unknown, and future possibilities or plans are not asserted as completed changes.
- [ ] Preview and accept the established distinction between correcting an earlier error and recording a real-world change, retaining Valid Time, Observed Time, Transaction Time and lifecycle history.
- [ ] Expose conflicting evidence and interpretations without choosing an automatic winner, cascading invalidation or confidence-based acceptance.
- [ ] Distinguish exact duplicate propositions from genuinely additional support. Reuse accepted knowledge and add Source Links when appropriate rather than necessarily creating a new Claim.
- [ ] Preview all temporal, source and dependent lifecycle effects with original evidence and authority; revalidate exact revisions and eligibility at acceptance.
- [ ] Apply effects atomically and preserve canonical, model-independent replay. Rejected or unresolved changes do not alter accepted state.
- [ ] Demonstrate changing preferences/circumstances, negation, uncertain dates, planned changes, changed-versus-error correction, contradiction, duplicates and additional support through compilation, CLI review and accepted inspection.
- [ ] Use deterministic real-SQLite fixtures and boundary/replay tests, then run repository-required full change verification.

## Blocked by

- [Accept or reject a candidate after its conversation closes](https://github.com/davidadel66/evie/issues/140)
