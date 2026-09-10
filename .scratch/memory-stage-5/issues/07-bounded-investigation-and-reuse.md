## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie follows useful searches, refreshes changed evidence, and stops honestly within its resource limits.

## Acceptance criteria

- [ ] Support model-chosen accepted-memory search, conversation search, and expansion during a turn without per-read approval, a rigid semantic-first sequence, or a fixed unsuccessful-search count.
- [ ] Reuse evidence across provider/tool continuations while interpretation, scope, relevant revisions, source eligibility, and temporal applicability remain valid. Document the validity/refresh contract and recheck access/egress before each dispatch.
- [ ] Refresh when new information changes what is needed or invalidates supplied evidence; recompilation of the serialized request alone is not a reason to redo all searches.
- [ ] Enforce cumulative search/deadline/concurrency/expansion/context limits across mixed tool calls and automatic recall, using measured initial settings. Successful matches do not reset the resource budget.
- [ ] Preserve explicit partial-failure, empty, unavailable, cancelled, and exhausted states in tool outcomes and compact activity. At exhaustion, report supported findings and remaining gaps rather than infer an unsupported negative answer.
- [ ] Demonstrate a multi-step question with a useful follow-up, a same-turn correction, unchanged continuations, a failed generator, exhausted budget, and cancellation/lease loss. Verify immutable per-request evidence receipts, bounded work, and no unauthorized late effects. Run required verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/158
- https://github.com/davidadel66/evie/issues/160
