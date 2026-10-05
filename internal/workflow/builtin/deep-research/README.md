# Deep research

A built-in Workflow Definition (2026-10-04, version 2 of deep research). It
researches one question as a durable background run instead of one long
chat turn, following the structure of the deep-research harness Claude
Code used to plan Evie's memory architecture: scope the question, search
from several angles, read the best sources, have independent verifiers try
to refute every claim, and write a report from what survives. Evie keeps
its own citation check on top: the report may cite only sources a
surviving verifier fetched.

## What it does

1. **Scope** (AI step, no tools): three to five complementary search angles
   suited to the question's domain, each with a label, one search query and
   a rationale.
2. **Search** (map of research workers, one per angle): each searches the
   web, fetches nothing, and returns up to six results ranked by relevance
   to the original question (URL, title, snippet, relevance). 3 minutes and
   40,000 tokens each, one retry. The step continues when at least one
   search succeeds.
3. **Sources** (`dedup_sources@1`): every result of every angle, ordered by
   relevance, then angle, then position; invalid URLs and duplicates
   dropped; the 15 most relevant kept. It counts the duplicates and the
   results over the budget.
4. **Extract** (map of research workers, one per source): each fetches its
   source, rates it `primary`, `secondary`, `blog`, `forum` or
   `unreliable`, notes its publish date, and extracts two to five
   falsifiable claims, each with a direct quote and an importance
   (`central`, `supporting`, `tangential`). A page that fails to load, is
   paywalled or irrelevant gives no claims. 4 minutes and 60,000 tokens
   each, no retry; a failed extraction is counted, not fatal.
5. **Rank** (`rank_claims@1`): a source counts as read only when the
   harness saw its extractor fetch it; claims of unread sources are
   dropped. The rest are ordered by importance, then source quality, and
   the first 25 kept.
6. **Ballots** (`expand_ballots@2`): three ballots per claim, each carrying
   the claim, its quote and its source.
7. **Verify** (map of research workers, one per ballot, up to 75): each
   verifier is adversarial. It fetches the source, checks the quote really
   supports the claim, searches for contradicting evidence, weighs the
   source's quality against the claim's strength, checks dates, and looks
   for marketing, press releases or cherry-picked benchmarks. It refutes
   when uncertain. 5 minutes and 60,000 tokens each, no retry; a failed
   verifier is counted, not fatal.
8. **Tally** (`tally_adversarial@1`): the rule below.
9. **Decide**: when a claim survived, **Synthesize** (AI step, no tools)
   merges duplicate claims into findings ranked by confidence, with
   caveats and open questions, citing inline `[S1]`; then **Check
   citations** (`check_citations@1`). Otherwise **Inconclusive** checks
   the tally's deterministic inconclusive report the same way and no
   writer runs.

The run's outputs are `report` (the checked report, when a claim
survived) or `inconclusive` (the checked inconclusive report),
`outcome`, `confirmed_claims`, `refuted_claims`, `unverified_claims` and
`stats` (sources, duplicates, budget drops, claims read and dropped,
votes counted).

## Rules

**Source URLs.** A URL is a source only when it is `http` or `https`, at
most 500 bytes, and has no whitespace, control character or backslash. Two
results are the same source when their host (lower case, without userinfo,
port or a leading `www.`), path (without trailing slashes) and query match;
the scheme and fragment are ignored.

**Tally.** A vote counts only when one of its evidence URLs is a URL the
harness saw that verifier fetch; a vote without one, and a verifier that
failed, is not counted. A claim is refuted with two refutations among its
counted votes, confirmed with at least two counted votes and fewer than two
refutations, and otherwise unverified: verification failed, which is not a
finding.

**Confidence.** A confirmed claim starts `high` when all three votes
counted and none refuted, `medium` otherwise. A secondary source caps it
at `medium`; a blog, forum or unreliable source at `low`. When no
surviving voter rated its own confidence `high`, it drops one level, never
below `low`. The writer may raise a finding that merges claims from two or
more independent sources by one level, never above `high`.

**Citations.** A confirmed claim's citable sources are the evidence URLs
its surviving voters fetched, as the harness recorded them, plus its own
source when a surviving voter fetched it: never a URL nobody fetched.
`check_citations@1` then applies the citation rule: a citation is kept only
when its marker is `S<n>`, appears once, its URL is a confirmed claim's
source (exact string after trimming spaces) and the text cites it; inline
markers survive only for kept citations, and `[S1, S2]` becomes
`[S1][S2]`; a link or bare URL that is not a confirmed source loses its URL
(a markdown link keeps its text; a bare URL becomes `[unverified link
removed]`); and the report lists every removal, says `verified` when
nothing was removed and `corrected` otherwise, and names the confirmed
claims its citations cover and those none does.

**Honest outcomes.** `outcome` is `confirmed` when a claim survived;
otherwise the inconclusive report says which of these happened:
`all_refuted` (every claim was refuted on its merits), `verifiers_failed`
(no claim could be verified: an infrastructure failure, such as rate
limits, not a research finding), `refuted_and_unverified` (some of each),
or `no_claims` (no source gave a checkable claim, with how many were
selected, read, failed, deduplicated and over budget).

## Authority, models and limits

Built in: available in every Workspace without a proposal or an approval.
It requests no Standing Authority, writes nothing outside Evie and runs only
research workers, which the Workspace must allow. Research workers read the
public web, so the question and the claims leave Evie through search
queries and fetched URLs, and every model call sends run data to
OpenRouter.

Every step uses one pinned model, `deepseek/deepseek-v4.1-flash`, through
five versioned Model Policies (scoper, searcher, extractor, verifier,
writer). `limits.max_tokens` 6,000,000 covers every worker's worst case
(400,000 for search: 5 angles, 2 attempts, 40,000 each; 900,000 for
extraction: 15 at 60,000; 4,500,000 for verification: 75 at 60,000) with
room for the two AI steps; `max_duration` 4 hours; `max_attempts` 140;
`max_parallel` 4.

A full run makes about 97 calls: 1 scope, up to 5 searches, up to 15
extractions, up to 75 verifiers and 1 writer.

## Cost

Evie records each worker's and AI step's tokens and cost at the model's
OpenRouter catalog price when the run starts; `evie workflows inspect`
shows the run's spend. At DeepSeek V4.1 Flash's catalog price of $0.30 per
million input tokens and $1.20 per million output tokens:

- **Hard ceiling:** 6,000,000 tokens, even if all were billed as output:
  **$7.20**. Evie refuses to start a worker or AI call that could pass the
  limit, and the operator's default daily cap of $5 would pause the run
  first; it continues by itself after 00:00 UTC.
- **Worst realistic case:** the limit reached with workers' usual mix of
  about 95% input: about **$2.10**.
- **Expected:** roughly 3 to 3.5 million tokens: about **$1.05 to $1.25**.
  This is an estimate until measured on real questions.

With the default `EVIE_SUBAGENTS_PER_PARENT=2`, a run runs two workers at
once and takes roughly 60 to 120 minutes; raising it to 4 lets the maps
use their full `max_parallel` and roughly halves that.

## Using it

From a chat in a Workspace, ask for deep research. When the question is
underspecified (which car to buy, without a budget, a use or a region),
Evie first asks two or three clarifying questions, then starts this
workflow with `workflow_start` and a refined question, tells you it runs in
the background with its run ID, and checks it with `workflow_status`.

From the command line:

```sh
evie workflows run <workspace-id> deep-research --input question="What limits the range of battery-electric aircraft?"
evie workflows inspect <run-id>
```

It needs `EVIE_WORKFLOW_REMOTE_MODELS=on` with `OPENROUTER_API_KEY`, the
Subagents plugin enabled (on by default for a new install; an install that
recorded it off keeps that: `evie plugins enable subagents`), and research
delegation allowed in the Workspace's settings. When something is missing,
a chat's `workflow_start` says everything David must change at once, and
the Workspace page's built-in list says why it cannot start there.

Owner controls:

```sh
evie workflows builtin list [--workspace <workspace-id>]
evie workflows builtin disable deep-research [--reason <text>]
evie workflows builtin enable deep-research
# copy it as a proposal to change and approve:
evie workflows customize deep-research --workspace <workspace-id>
evie workflows customize deep-research --global
```

The Workspace page's Workflows section lists it with the same on/off
switch. Disabling it stops new runs in every Workspace and pauses its runs
in progress at their next step; enabling it lets them continue. An
approved custom definition of the same name overrides the built-in in its
scope: a Workspace custom, then a Global custom, then the built-in. A
Global custom is reviewed and approved like a Workspace's (`evie workflows
approve` with the card's hashes) and, like the built-in, may hold no
Standing Authority, capability, schedule or build worker.

Runs pin the built-in version they started with, identified by its
executable hash, whose exact files Evie records in its procedural
repository at `builtin/workflows/deep-research`. When a later Evie ships a
new version, runs pinned to the earlier one continue on it; an Evie that
cannot read a pinned version pauses the run visibly instead of running
another.

### A measured live run

Nothing runs automatically or in tests. To measure a real run (it is
billed by OpenRouter, at most $7.20):

```sh
export EVIE_WORKFLOW_REMOTE_MODELS=on   # OPENROUTER_API_KEY in ~/.evie/.env
evie workflows run <workspace-id> deep-research \
  --input question="What limits the range of battery-electric aircraft?"
evie workflows inspect <run-id>
```

`inspect` prints the map items, every worker call with its tokens and cost,
the AI calls and the run's total spend; compare it with the bounds above.
Step outputs keep each worker's summary, sources and typed result; `evie
workflows report <run-id> <execution-id>` prints a worker's full report.

Every Evie also enforces the operator's daily cap on all workflow runs
(`EVIE_WORKFLOW_DAILY_USD`, default $5, and `EVIE_WORKFLOW_DAILY_TOKENS`,
default 10,000,000, per UTC day). A run that reaches it pauses ("paused at
the daily workflow cap" in `evie workflows notifications`) and continues by
itself after 00:00 UTC; the wait does not count toward `max_duration`. For
a measured run, either let that happen and note it, or raise the cap for
the day, for example `export EVIE_WORKFLOW_DAILY_USD=8`, in the Evie that
runs it. `evie workflows status` shows today's spend against the cap.

### Recording a measurement

Once the run has finished (`evie workflows notifications` says so), read
its numbers from the inspection, which counts every AI call and worker at
its recorded usage (an unfinished call at its bound):

```sh
evie workflows inspect --json <run-id> > run.json
jq '{state: .run.state, started: .run.created_at, finished: .run.finished_at,
  searches: ([.attempts[] | select(.step == "search" and .item != null)] | map(.item) | unique | length),
  extractions: ([.attempts[] | select(.step == "extract" and .item != null)] | map(.item) | unique | length),
  verifiers: ([.attempts[] | select(.step == "verify" and .item != null)] | map(.item) | unique | length),
  ai_tokens: ([.model_calls[] | if .finished_at then (.input_tokens // 0) + (.output_tokens // 0) else .estimated_input_tokens + .estimated_output_tokens end] | add // 0),
  worker_tokens: ([.workers[] | if .finished_at then (.counted_tokens // 0) else .budget_tokens end] | add // 0),
  usd: ((([.model_calls[] | if .finished_at then (.micro_usd // 0) else .estimated_micro_usd end] | add // 0)
       + ([.workers[] | if .finished_at then (.micro_usd // 0) else .estimated_micro_usd end] | add // 0)) / 1000000),
  outcome: .run.outputs.outcome, report: (.run.outputs.report.status // "none"),
  confirmed_claims: (.run.outputs.confirmed_claims | length)}' run.json
```

The duration is `finished` minus `started`, less any time paused at the
daily cap (`evie workflows inspect <run-id>` lists the pauses under
Recovery). Then:

1. Add a row to **Measured runs** below: the date, the Evie commit
   (`git rev-parse --short HEAD`), the question, the searches, extractions
   and verifiers, the AI and worker tokens, the cost, the duration, and the
   outcome with the report's status (`verified` or `corrected`) and how
   many citations were removed. This README is documentation, so the edit
   changes no executable hash.
2. If the numbers argue for different budgets, `max_items` or prompts,
   change them only through a new version of the built-in (or a reviewed
   custom copy), and record why in a dated entry of
   `cmd/evie/docs/active/workflow-runtime.decisions.md`.

### Measured runs

| Date | Evie | Question | Searches / extractions / verifiers | AI tokens | Worker tokens | Cost | Duration | Outcome / report |
|---|---|---|---|---|---|---|---|---|
| (none yet) | | | | | | | | |

Simulate its dry run with:

```sh
evie workflows simulate internal/workflow/builtin/deep-research \
  --fixtures internal/workflow/builtin/deep-research/fixtures.json
```

This README is documentation: editing it does not change the executable
hash.
