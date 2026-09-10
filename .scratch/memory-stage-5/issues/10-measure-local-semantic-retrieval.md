## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

A reproducible experiment identifies how local embeddings recover paraphrased evidence and what they cost.

## Acceptance criteria

- [ ] Use versioned eligible accepted-memory and conversation fixtures with evidence IDs, paraphrases, distractors, scope exclusions, and a held-out partition. Compare against the shipped lexical/exact baseline.
- [ ] Run the planned SQLite/brute-force comparison against a local approximate vector index through a disposable full query-to-evidence evaluation path; do not ship an unmeasured production index in this ticket.
- [ ] Measure evidence recall at a fixed context budget, p95 latency, memory, embedding/index build costs, restart/rebuild behavior, and operational/dependency compatibility on declared hardware.
- [ ] Check loopback/Unix-socket endpoint enforcement, redirects denied, cancellation, malformed output, and no remote fallback. Use only approved eligible local evidence.
- [ ] Record selected model/configuration/index and their limitations, or a reproducible no-go result. Obtain repository-required approval before introducing any production dependency.
- [ ] Produce a runnable experiment and a decision record that gates the production semantic-search ticket. A no-go result does not satisfy the selection gate or authorize inventing a winner; adapt the experiment within scope or report the concrete blocker.

## Blocked by

- https://github.com/davidadel66/evie/issues/157
