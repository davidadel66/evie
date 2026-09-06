## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Provide bounded operational and inbox projections through the Kernel, CLI and web so the owner can inspect selected coverage, failures, generation status and review backlog. Add the observable timings/counters needed to run the specified pilot without exposing raw history or pretending operational metrics measure semantic quality.

## Acceptance criteria

- [ ] Display selected/completed coverage, visible gaps, outside-selection ranges, jobs by distinct state, actionable retry/cancellation failures, active generations and candidate inbox age consistently across surfaces.
- [ ] Keep status queries, pagination, result sizes and aggregate work bounded under the contract. Inspect one busy scope and many scopes without crossing source visibility boundaries.
- [ ] Expose safe timing/counter data for queueing, extraction, validation/resolution, database completion, candidate freshness, foreground finalization, retries, cancellation and stale work as needed by the evaluation protocol.
- [ ] Record review outcomes and relevant timestamps/lineage for pilot measurement. Distinguish active measured review time from elapsed inbox age; approval rate is not accuracy.
- [ ] Document how to observe Evie and model-server resources, database/WAL/candidate growth and relevant history/graph sizes. Avoid unbounded graph scans hidden inside diagnostic or resolution paths.
- [ ] Do not include raw protected text, secrets, model reasoning or opaque continuation state in diagnostics. Maintain generic SQL/file containment and existing documented privileged-shell limitations.
- [ ] Demonstrate normal progress, failed-gap/later-success, backlog, an unavailable endpoint, and generation changes through CLI and web with consistent safe projections.
- [ ] Test bounded/status/scope behavior and metric semantics through observable seams, then run repository-required full change verification.

## Blocked by

- Draft 15: Review identities, edits, and compound effects on the web
- Draft 16: Change generations without losing review decisions

