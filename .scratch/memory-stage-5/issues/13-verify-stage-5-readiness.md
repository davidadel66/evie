## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

A reproducible assessment establishes whether Stage 5 meets its agreed behavior and measured release gates.

## Acceptance criteria

- [ ] Run the frozen held-out retrieval and reader evaluation without changing its corpus, configuration, metric definitions, or pass thresholds in response to the results.
- [ ] Report retrieval evidence quality separately from model answer quality, including oracle comparison, attribution, clarification, conflicting updates, retired evidence, and unanswerable questions.
- [ ] Run the full deterministic boundary, recovery, and provider-payload checks plus required repository verification, and compare measured resource costs with the frozen gates.
- [ ] Demonstrate a fresh-chat preference, uncompiled original statement, cross-topic reference, bounded investigation, historical conflict, retirement suppression, original-source inspection after correction/restart, and Memory unavailable behavior.
- [ ] Declare readiness only when every required gate passes; otherwise publish the precise failures and follow-up work without changing thresholds to manufacture a pass.
- [ ] Record exact configuration, commands, results, warnings, and skipped checks with reasons. This ticket does not authorize deployment, changes to unrelated features, or closing/modifying the parent issue.

## Blocked by

- https://github.com/davidadel66/evie/issues/167
