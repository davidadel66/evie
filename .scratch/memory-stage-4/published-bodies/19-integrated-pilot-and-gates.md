## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Run the integrated local compiler and real owner-review pilot on development/pilot data after deterministic acceptance passes. Measure the actual runtime and review surfaces, declare the local deployment workload, and record numerical release gates before exposing the final holdout. Standalone model results cannot substitute for these measurements.

## Acceptance criteria

- [ ] Use the real Kernel compilation/review path and the spike's pinned corpus/scoring/report contract. Keep final-holdout narratives and variants unexposed.
- [ ] Compare compilation disabled, normal new-evidence processing and historical catch-up under the same fixed foreground workload. Vary source length, retained event count, accepted graph size, scope distribution and runtime capacity independently.
- [ ] Use the proposed 10k/100k/1m event stress levels as starting experiments; record actual workloads and limits rather than performance promises. Include multiple processes, a busy scope versus many scopes, failed-gap/later-progress and competing backfill.
- [ ] Measure foreground terminal-event commit and response-finalization overhead separately from extraction; report queue/inference/validation/database latency, candidate freshness, throughput, retries/cancellation, zero-candidate coverage, stale wasted work and catch-up behavior.
- [ ] Measure Evie and model-server resources, database/WAL/candidate growth and resolution cost. State both source-arrival versus compilation capacity and candidate-arrival versus owner-review capacity.
- [ ] Observe David's actual accept/edit/reject/defer sessions and reasons. Measure active review seconds and candidates per useful accepted change, oldest unresolved age and backlog growth; neither approval rate nor an empty inbox is a quality score. Completion depends on these owner sessions and accessible local runtime resources; missing observations remain incomplete rather than being simulated or inferred from agent self-review.
- [ ] Report supported useful precision and required-memory recall separately with identity, temporal, source-attribution and unwanted-suggestion errors, counts, denominators, repetitions and meaningful limitations.
- [ ] Record the chosen configuration, workload assumptions, explicit numerical quality/recall/resource/foreground/freshness/review gates and deterministic zero-tolerance gates before final holdout evaluation or ongoing enablement.
- [ ] If the pilot exposes an unmet contract or no viable workload/configuration, report it and retain dependent release blocking; do not lower gates using final-holdout observations or silently broaden scope.
- [ ] Publish a reproducible versioned pilot report. Run checks applicable to experimental code/configuration/documentation changes and preserve exact conformance.

## Blocked by

- [Inspect compiler health, coverage, and review backlog](https://github.com/davidadel66/evie/issues/148)
- [Prove the complete Stage 4 path with deterministic acceptance](https://github.com/davidadel66/evie/issues/149)
