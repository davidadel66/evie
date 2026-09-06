## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Evaluate the frozen compiler configuration against the untouched complete-history holdout and agreed workloads, using the pilot's release gates and deterministic acceptance baseline. Deliver a clear readiness decision and evidence. Failure remains a failed evaluation, not permission to tune on the holdout and relabel it untouched.

## Acceptance criteria

- [ ] Freeze generation/configuration, runtime/environment, rubric, corpus/holdout identity, repetitions, baseline and numerical gates before running; verify the holdout custody/exposure record and record all runs. Gold labels and evaluator metadata remain outside extractor input during the final run.
- [ ] Evaluate raw proposals and reviewable candidates with supported useful precision, required-memory recall, identity, temporal, source-attribution, unwanted-proposal and failure slices. Preserve counts, denominators, paired deltas and meaningful uncertainty.
- [ ] Verify the declared workload/resource/foreground/freshness and review gates using the integrated measurement protocol; state sample limits and any extrapolation.
- [ ] Re-run the required deterministic acceptance/checks on the frozen release configuration. Exact scope, authority, source-binding, persistence or replay failure blocks readiness regardless of averages.
- [ ] Keep accepted-state conformance, learned extraction, retrieval and answer panels separate. Mark Stage 5 retrieval/production-answer claims deferred rather than implying they were evaluated.
- [ ] Do not let universal abstention pass on precision alone or approval rate stand in for usefulness/accuracy. Compare against the frozen baseline and disclose all observed failures.
- [ ] Publish a versioned final report with an explicit pass/fail result per gate, configuration/workload limits, and a readiness decision for ongoing compilation. Do not silently enable ongoing processing after a failed evaluation.
- [ ] If results inform further tuning, move exposed cases into development/regression use and require fresh holdout material for subsequent selection. Record required follow-up without rewriting the evaluated result.
- [ ] Record exact verification results and remaining limitations. No automatic admission, retrieval feature work, parent closure or unrelated deployment is part of this outcome.

## Blocked by

- Draft 19: Run the integrated pilot and freeze release gates
