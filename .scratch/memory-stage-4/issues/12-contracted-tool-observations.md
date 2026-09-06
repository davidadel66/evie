## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Implement the minimal named tool-observation path frozen by the evidence contract, from committed outcome and exact permitted fields to CLI owner review and accepted provenance. Keep this slice restricted to that reviewed capability/field set. Additional unrelated tool contracts require their own reviewed outcomes.

## Acceptance criteria

- [ ] Admit only the named contracted completed outcomes and fields, with explicit authority and freshness meaning. Arbitrary tool output and unfinished intent remain ineligible.
- [ ] Apply the exact structured-field/range projection and hash contract to compiler input, validation, preview and accepted-source rendering, including malformed and missing fields.
- [ ] Preserve scope, source visibility, secret exclusion, failed/cancelled-turn eligibility and original event identity. Generic reasoning/context or tool diagnostics cannot become independent evidence.
- [ ] Keep operational Task/domain stores authoritative. Memory does not create a competing operational record from mutable tool state outside the contracted use case.
- [ ] Show original tool authority during review, and preserve it through explicit acceptance and replay. Owner approval is audit authority and cannot rewrite the source as an owner assertion.
- [ ] Handle disabled/unavailable extraction and replay without invoking the original tool, performing external effects, or calling a model.
- [ ] Demonstrate a real committed contracted observation through candidate production, closed-session CLI review, accepted source inspection and replay.
- [ ] Test rejected tool fields, incomplete intent, source mutation/eligibility, scope and exact projection with real SQLite and scripted extraction. Run repository-required full change verification.

## Blocked by

- Draft 09: Accept or reject a candidate after its conversation closes

