## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Implement generation changes and equivalent-suggestion behavior using the frozen durable-work and review contracts. Allow the owner to activate a new pinned configuration or deliberately reprocess bounded history, while preserving accepted operations and the history of accepted, edited and rejected candidates.

## Acceptance criteria

- [ ] Material model, prompt or evidence-policy changes create distinct pinned generations and candidate groups. Inspection identifies the producing configuration and selected ranges.
- [ ] New-generation activation captures its explicit frontier; old history is processed only through separate bounded selection. Unselected history never becomes implicitly complete or queued.
- [ ] Apply the exact frozen equivalence/re-presentation policy to prior accepted, edited, rejected and unresolved suggestions. Preserve audit and explain recurring suggestions without silently discarding new evidence.
- [ ] Keep source provenance and completed coverage associated with their actual generations. Reprocessing is idempotent and cannot borrow completion incorrectly from a different configuration.
- [ ] Do not rewrite accepted Claims, Predicate meanings or canonical operations. Accepted reads and replay remain unchanged by model availability or generation replacement.
- [ ] Respect retention decisions without implementing unspecified purging. In-flight old/new work follows cancellation, capacity and fencing rules.
- [ ] Demonstrate an upgrade, explicit selected reprocessing, equivalent rejection/edit cases and genuinely new support through CLI inspection/review and reopen.
- [ ] Run deterministic cross-generation, selection-race, review-history and replay tests plus repository-required full change verification.

## Blocked by

- [Select historical backfill and inspect honest coverage](https://github.com/davidadel66/evie/issues/139)
- [Edit candidates and approve bounded dependent batches](https://github.com/davidadel66/evie/issues/144)

