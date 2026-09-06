## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Deliver the parent specification's complete deterministic acceptance scenario and conformance report over the implemented compiler and review surfaces. Use real persistence and scripted extraction. This is a verifiable end-to-end acceptance outcome, not a deferred catch-all for missing behavior or safety hardening.

## Acceptance criteria

- [ ] Run the required path: stall scripted extraction while a foreground turn finishes, restart/recover selected work, close the original source conversation, inspect and approve the exact candidate, and verify accepted reads and canonical replay.
- [ ] Verify equivalent scope, evidence, preview and resolution outcomes through the Kernel, CLI, HTTP and web interactions. Include global, multiple Workspace/project and session cases and forbidden implicit Promotion.
- [ ] Exercise out-of-order completion with an earlier unresolved gap, zero-candidate success, explicit historical selection, outside-selection history and changed generations with preserved review decisions.
- [ ] Cover competing processes, cancellation, lease expiry/replacement, stale completion, duplicate scheduling/review, source eligibility changes and crash points around durable acceptance. Retain focused tests already required by each implementation slice.
- [ ] Prove unaccepted-candidate isolation, generic storage containment, original evidence authority, stale-preview rejection, atomic effects, quarantine/recovery compatibility and zero model/external-effect calls during replay.
- [ ] Record exact deterministic failures separately from learned-quality or resource metrics. Any scope, authority, source-binding, persistence or replay violation blocks the pilot/release path.
- [ ] Run repository-required full change verification and record exact commands/results, warnings and any skipped checks with reasons. A required failing or unjustifiably skipped check cannot count as conformance.
- [ ] Deliver a repeatable demonstration and versioned conformance result; do not require a live model or claim learned quality from scripted fixtures.

## Blocked by

- [Review identities, edits, and compound effects on the web](https://github.com/davidadel66/evie/issues/146)
- [Change generations without losing review decisions](https://github.com/davidadel66/evie/issues/147)

