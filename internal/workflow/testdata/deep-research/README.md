# Deep research (test example)

This is the frozen sub-stage 1–9 example that validation, hashing and
simulation tests are written against. The deep-research workflow Evie ships
is the bundled template in `internal/workflow/templates/deep-research`
(Stage 14 sub-stage 10), with reviewed prompts, verifiers that must fetch
their evidence and a citation check; propose that one with
`evie workflows propose <workspace-id> --template deep-research`.

Researches one question the way a careful analyst would, as a durable
workflow rather than one chat turn.

1. **Plan** (AI step, no tools): split the question into up to
   `max_angles` distinct angles.
2. **Research** (map of research workers, at most 4 at once): each angle
   gets its own research worker with web search and fetch, a 10-minute and
   400,000-token budget, and returns claims with the sources it fetched.
   The step continues if at least one angle succeeds.
3. **Collect claims** (deterministic): merge duplicate claims, keep only
   sources a worker actually fetched, cap at 12 claims and report any
   dropped.
4. **Ballots** (deterministic): three ballots per claim.
5. **Verify** (map of research workers): each ballot is an independent
   worker that checks one claim and votes `supported`, `refuted` or
   `unclear`.
6. **Majority filter** (deterministic): keep claims with at least two
   `supported` votes; a missing vote counts against.
7. **Synthesize** (AI step, no tools): write the report from the verified
   claims, citing only their sources.

Nothing here writes outside Evie: research workers only read the web, and
the workflow requests no Standing Authority. Simulate it with:

```sh
evie workflows simulate internal/workflow/testdata/deep-research \
  --fixtures internal/workflow/testdata/deep-research.fixtures.json
```

This README is documentation: editing it does not change the executable
hash, so it does not need a new approval.
