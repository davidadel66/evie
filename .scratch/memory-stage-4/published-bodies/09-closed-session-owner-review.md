## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Implement the first closed-session owner-review path for the narrow candidate types already produced. Provide a scope-level CLI inbox, exact preparation and explicit acceptance or rejection through the new typed Kernel authority contract. Reuse accepted-operation semantics while preserving the current session-bound explicit-memory API.

## Acceptance criteria

- [ ] List and inspect bounded candidates in an explicit authorized scope after their source sessions close, without reopening those sessions. Scope and provenance expansion follow the existing visibility matrix.
- [ ] Preview exact evidence projections and hashes, original authority, target identities, Predicate, typed value, scope, relevant temporal meaning, and resulting effects. Accepted-source inspection resolves the same projection as review.
- [ ] Acceptance binds the frozen preview/effect identity and revisions, revalidates current source eligibility, scope, identities and conflicts, and rejects stale or changed effects until the contractually required refresh/reapproval.
- [ ] Atomically record review resolution/audit and accepted Semantic Operation effects. Duplicate delivery and concurrent review yield the frozen idempotent outcome with no partial accepted effects.
- [ ] Rejection is durable and inspectable without altering accepted memory. Approval audit is distinct from evidence authority and cannot silently promote source scope.
- [ ] Candidates stay outside accepted reads until acceptance; accepted results preserve Stage 3 equality, replay, quarantine/recovery and model-independent operation guarantees.
- [ ] Keep memory-storage fences and existing explicit-command validation intact. No candidate adapter can invoke a session-bound apply method with fabricated authority.
- [ ] Test inactive sessions, multiple scope families, invalid source projection, changed eligibility, stale/concurrent approval, rejection, reopen and exact replay using real SQLite plus CLI acceptance. Run repository-required full change verification.

## Blocked by

- [Freeze owner review after source sessions close](https://github.com/davidadel66/evie/issues/134)
- [Compile one selected source unit into durable candidates](https://github.com/davidadel66/evie/issues/136)

