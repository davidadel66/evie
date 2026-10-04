# Deep research

A bundled Workflow Definition (Stage 14 sub-stage 10). It researches one
question the way a careful analyst would, as a durable background run
instead of one long chat turn: plan angles, research them in parallel,
check every claim with independent verifiers, keep what a majority supports,
and write a report whose citations are checked against what the verifiers
actually read.

## What it does

1. **Plan** (AI step, no tools): split the question into at most
   `max_angles` (default 3, never more than 4) distinct, self-contained
   angles.
2. **Research** (map of research workers, at most 4 at once): one research
   worker per angle with web search and fetch, a budget of 10 minutes and
   250,000 tokens, one retry. Each returns up to six short, checkable
   claims, each citing one to four URLs it fetched. The step continues when
   at least one angle succeeds.
3. **Collect claims** (`collect_claims@1`): merge duplicates, keep only
   sources a researcher actually fetched, at most 12 claims.
4. **Ballots** (`expand_ballots@1`): three ballots per claim.
5. **Verify** (map of research workers, at most 36): each ballot is an
   independent verifier with 5 minutes and 100,000 tokens. It reads the
   cited sources itself, looks for an independent one, and votes
   `supported`, `refuted` or `insufficient`, naming the URLs its vote rests
   on.
6. **Tally** (`tally_verdicts@1`): a `supported` or `refuted` vote counts
   only when its evidence includes a URL the harness saw that verifier
   fetch; otherwise it counts as `insufficient`. A verifier that failed
   counts against. A claim needs two of three `supported` votes. An
   accepted claim's citable sources are exactly its supporting verifiers'
   fetched evidence.
7. **Synthesize** (AI step, no tools): write the report from the verified
   claims only, with inline markers `[S1]` and a citation list.
8. **Check citations** (`check_citations@1`): the citation rule below.

The run's outputs are `report` (the checked report), `verified_claims` and
`rejected_claims`.

## Citation rule

Every citation in the final report must be a source a verifier fetched and
relied on for an accepted claim, compared as an exact string after
trimming spaces. Concretely:

- A citation in the list is kept only when its marker is `S<n>`, it appears
  once, its URL is an accepted claim's source, and the text cites it.
  Anything else is removed.
- An inline marker survives only for a kept citation; others are deleted
  from the text, and `[S1, S2]` is rewritten as `[S1][S2]`.
- A link or bare URL in the text that is not an accepted source loses its
  URL: a markdown link keeps its text, and a bare URL becomes
  `[unverified link removed]`.
- The report lists every removal with its reason, says `verified` when
  nothing was removed and `corrected` otherwise, and names the accepted
  claims its citations cover and those none does.

## Authority, models and limits

The workflow requests no Standing Authority: it writes nothing outside
Evie. Research workers read the public web, so the question and the claims
leave Evie through search queries and fetched URLs, and every model call
sends run data to OpenRouter. That is why runs need
`EVIE_WORKFLOW_REMOTE_MODELS=on` (with `OPENROUTER_API_KEY`), the Subagents
plugin enabled and research workers allowed in the Workspace.

Every step uses one pinned model, `deepseek/deepseek-v4.1-flash`, through
four versioned Model Policies. The run is bounded by `limits.max_tokens`
6,000,000, which covers every worker's worst case (2,000,000 for research:
4 angles, 2 attempts, 250,000 each; 3,600,000 for verification: 36
verifiers at 100,000) with room for the AI steps; `max_duration` 3 hours;
`max_attempts` 60.

## Cost

Evie records each worker's and AI step's tokens and cost at the model's
OpenRouter catalog price when the run starts, and `evie workflows inspect`
shows the run's spend. At DeepSeek V4.1 Flash's catalog price of $0.30 per
million input tokens and $1.20 per million output tokens:

- **Hard ceiling:** 6,000,000 tokens, even if all were billed as output:
  **$7.20**. Evie refuses to start a worker or AI call that could pass the
  limit.
- **Worst realistic case:** the limit reached with workers' usual mix of
  about 95% input: about **$2.10**.
- **Expected:** three angles and about nine claims (27 verifiers) use
  roughly 2 to 3 million tokens: about **$0.70 to $1.00**. This is an
  estimate until measured on real questions.

With the default `EVIE_SUBAGENTS_PER_PARENT=2`, a run runs two workers at
once and takes roughly 30 to 70 minutes; raising it to 4 lets the maps use
their full `max_parallel`.

## Using it

```sh
evie workflows templates
evie workflows propose <workspace-id> --template deep-research
# review the card, then approve exactly what it printed:
evie workflows approve <proposal-id> --hash <executable-hash> --authority <authority-hash> --expect-version <n>
```

Approval activates it for future runs and starts none. Then start a run
from the CLI, the Workspace page, or a chat in that Workspace (Evie's
`workflow_start` tool), and check it with `evie workflows inspect <run-id>`
or `workflow_status`.

### A measured live run

Nothing runs automatically or in tests. To measure a real run (it is
billed by OpenRouter, at most $7.20):

```sh
evie plugins enable subagents
# allow research workers in the Workspace's settings on the Workspace page
export EVIE_WORKFLOW_REMOTE_MODELS=on   # OPENROUTER_API_KEY in ~/.evie/.env
evie workflows run <workspace-id> deep-research \
  --input question="What limits the range of battery-electric aircraft?"
evie workflows inspect <run-id>
```

`inspect` prints the map items, every worker call with its tokens and cost,
the AI calls and the run's total spend; compare it with the bounds above.
Step outputs keep each worker's summary, sources and typed result; `evie
workflows report <run-id> <execution-id>` prints a worker's full report.

Simulate the dry run that proposing it records with:

```sh
evie workflows simulate internal/workflow/templates/deep-research \
  --fixtures internal/workflow/templates/deep-research/fixtures.json
```

This README is documentation: editing it does not change the executable
hash, so it does not need a new approval.
