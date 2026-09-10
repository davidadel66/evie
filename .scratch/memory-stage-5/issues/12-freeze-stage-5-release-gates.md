## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

The complete recall experience has measured default budgets and explicit pass/fail release thresholds.

## Acceptance criteria

- [ ] Exercise the integrated Automatic Recall, deeper searches, expansion, reference resolution, historical/conflict behavior, retirement, hybrid search, and source inspection on a versioned development workload.
- [ ] Compare no recall, recent context, tool-only retrieval, automatic recall, and automatic plus deeper search; include an oracle-evidence reader condition to separate missing evidence from answering failure.
- [ ] Measure evidence recall under fixed context budgets, grounding, abstention/clarification, stale and unwanted recall, p50/p95 latency, serialized context cost, index cost, and failure behavior on declared hardware.
- [ ] Freeze numerical budgets, result/expansion bounds, ranking configuration, quality thresholds, model/index versions, and the held-out evaluation procedure before running the release assessment. Do not tune against held-out answers.
- [ ] Every deterministic scope, egress, provenance, lifecycle, retirement, and restart expectation must pass; an average quality metric cannot waive a violated boundary.
- [ ] Produce a reproducible pilot report and frozen configuration with explicit remaining failures/limitations. A failing pilot keeps readiness blocked until the affected behavior is corrected and reverified.
- [ ] Run focused acceptance, relevant model-backed development evaluations, and required repository verification without changing the parent specification's accepted product boundaries.

## Blocked by

- https://github.com/davidadel66/evie/issues/159
- https://github.com/davidadel66/evie/issues/161
- https://github.com/davidadel66/evie/issues/162
- https://github.com/davidadel66/evie/issues/163
- https://github.com/davidadel66/evie/issues/166
