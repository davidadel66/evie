# Stage 4 memory compiler: evaluation design

**Research date:** 2026-09-04.

**Status:** recommendations for the Stage 4 design interview, not approved
requirements or authority to implement. No extraction model or benchmark was run
for this note.

**Local basis:** [umbrella specification](../active/memory.spec.md),
[Stage 3 specification](../active/semantic-memory-stage-3.spec.md),
[ADR 0053](../../../../docs/adr/0053-evaluate-memory-as-separate-systems.md), and
the [existing evaluation contract](../fixtures/semantic-memory/evaluation/README.md).
Stage 3 already requires separate semantic, extraction, retrieval, and answer
panels. Preserve its exact deterministic gates and extend its reports.

## Recommendation

Make Stage 4 demonstrate a useful, reviewable memory inbox under a bounded local
workload. Merely producing valid JSON and surviving restart is insufficient:
the compiler can satisfy both while proposing unsupported facts, merging the
wrong people, or generating more review work than David can clear.

Bring a small human-reviewed Evie corpus, extraction grading, review burden, and
foreground interference measurements into Stage 4. Keep the full public
benchmark adapters and production answer-quality claims in later stages. Select
the local model on these measurements after the structured-output spike. Do not
set an arbitrary quality percentage or laptop throughput target in advance.

The sections below distinguish observed source facts from proposed Evie design.

## What the primary sources establish

| Source fact | Consequence for Evie (recommendation) |
| --- | --- |
| LongMemEval separates indexing, retrieval, and reading, and supplies evidence locations for retrieval Recall@k/NDCG@k alongside answer grading. Its experiments found that compressing rounds into extracted facts lost information overall, despite helping some multi-session cases. [Paper, v2](https://arxiv.org/html/2410.10813v2) | Grade compiler output before any reader is involved. Preserve episodic source material and source expansion; do not assume the graph can replace the original conversation. Later compare a graph-assisted path with a raw-event retrieval baseline. |
| MemoryAgentBench evaluates incremental input processing across accurate retrieval, test-time learning, long-range understanding, and selective forgetting. Its FactConsolidation task specifically tells agents to prefer newer conflicting information. [Paper, v4](https://arxiv.org/html/2507.05257v4) | Borrow incremental ingestion, interference, and update stress cases. Do not copy its newest-fact-wins rule into Evie: authority, explicit approval, correction mode, and valid time determine accepted state here. |
| LoCoMo uses generated conversations checked and edited by humans, with temporal event graphs. Its current official release has ten conversations, turn IDs, question evidence, and event-summary annotations. [Paper](https://aclanthology.org/2024.acl-long.747/), [repository](https://github.com/snap-research/locomo) | Use long, connected narrative histories and human-reviewed evidence labels, including facts whose qualifiers cross a chunk boundary. Treat public conversational QA as an external check, not representative usage telemetry. |
| KILT evaluates downstream answers and provenance separately. [Paper](https://aclanthology.org/2021.naacl-main.200/) | Later score correct-and-supported answers, not correctness alone; a reader's prior knowledge cannot demonstrate successful memory. |
| The LongMemEval repository records a cleaned dataset release and now points to LongMemEval-V2. The V2 repository evaluates agent trajectories and query latency, and withholds gold answers and question metadata from memory backends. [Original repository](https://github.com/xiaowu0162/LongMemEval), [V2 repository](https://github.com/xiaowu0162/LongMemEval-V2) | Pin the exact dataset and evaluator revision. V2 may fit later workflow-memory evaluation, but is a separate benchmark and is not a reason to enlarge Stage 4. Keep evaluator metadata outside the compiler/retriever input contract. |

These results do not select an extraction model, establish a performance budget
for David's hardware, or demonstrate that graph memory will improve Evie. Public
benchmark outcomes depend on reader, prompts, history length, retrieval budget,
and task construction. MemoryAgentBench's cost analysis also excludes embedding
index construction in one comparison; Evie should measure compilation and
backfill costs explicitly rather than importing a per-query cost figure.
[MemoryAgentBench, appendix I](https://arxiv.org/html/2507.05257v4)

Two supplemental studies suggest useful probes, not additional architecture.
STALE tests implicit conflicts and questions with stale premises using generated
scenarios with human validation; adapt such probes without treating inferred
conflicts as authority for destructive graph changes.
[STALE, v1](https://arxiv.org/html/2605.06527v1)
The consolidation study finds competitive episodic-only controls and failures
from repeated textual abstraction, but explicitly excludes structured non-text
memory; it strengthens the case for an episodic baseline without establishing
that Evie's typed graph shares those failure rates.
[Useful Memories Become Faulty, v1](https://arxiv.org/html/2605.12978v1)

## Separate correctness from quality and usability

The following are proposed measurements, not new release thresholds.

| Layer | Measurements | Interpretation |
| --- | --- | --- |
| Accepted graph truth | Existing operation, scope, provenance, temporal, replay, recovery, and approval gates; zero unapproved graph writes | Exact deterministic conformance. A model-quality gain cannot compensate for a failed gate. |
| Compiler mechanics | Durable enqueue/coverage, no lost or duplicate committed candidate groups, lease fencing, stale approval rejection, cancelled/failed/skipped job outcomes, disabled-extractor reconciliation | Exercise with scripted extractors and fault injection in ordinary tests; no live model is needed to reproduce ordering bugs. |
| Extraction | Schema success; supported candidate precision; recall of gold required memories; predicate/typed value/polarity/time match; source binding; omission and unwanted-proposal counts | Score raw model proposals and persisted reviewable candidates separately, exposing what validation caught and what it missed. |
| Entity resolution | Mention detection; correct existing/new/ambiguous decision; unsafe proposed merges; duplicate identities/splits; ambiguity frequency and correctness | A similar name is not sufficient identity evidence. Inspect error counts by scope and entity type. |
| Human review | Accept unchanged, edit, reject, defer; rejection/edit reasons; active review seconds; candidates reviewed per useful accepted fact; oldest unresolved candidate and backlog change | Approval rate alone is not truth precision. People may approve errors or reject true facts they do not want remembered. |
| Runtime and scale | Source tokens/bytes per second; eligible events covered; model latency; job queue age; end-to-end candidate freshness; foreground latency delta; memory/CPU/GPU use; DB/WAL/candidate storage growth | Report compilation, persistence, and human wait separately. A queue can be fast while the review inbox becomes unusable. |
| Retrieval and answers, later | Evidence Recall@k/NDCG@k; useful evidence under the token budget; grounded answer correctness; false answers and false abstentions | Stage 4 can prepare fixtures and expected sources, but it cannot claim production retrieval gains before the retrieval path exists. |

Define supported candidate precision against human gold: a candidate has the
correct subject identity, predicate, typed object, polarity, applicable time,
scope, authority, and sufficient eligible source support. Matching an exact
quote/hash proves the cited text exists; it does **not** prove the text entails
the claim. Include a quote such as "I never worked at Acme" cited for "works at
Acme" to make that distinction observable.

Separate gold labels into **required**, **permitted but optional**, and
**forbidden/unsupported** proposals. Required-memory recall uses only required
items as its denominator; permitted extras are not hallucinations. Record
unwanted but true memories separately to evaluate the admission policy. Publish
the rubric before model comparisons so changing the policy cannot quietly
change the score.

Use deterministic matching for IDs, scoped evidence, typed fields, and known
time boundaries. Allow explicit gold equivalence sets where multiple graph
representations are valid. Use human adjudication for semantic entailment and
usefulness; an optional pinned model judge can flag cases, but must not become
the authority for source validity or silently approve its own extracted facts.

For identity clustering, CoNLL's MUC/B-CUBED/CEAF combination offers diagnostic
views; it does not replace the domain-specific wrong-person and cross-scope
counts. Run those richer clustering metrics when fixtures contain meaningful
clusters rather than mostly singleton names.
[CoNLL-2012 task](https://conll.cemantix.org/2012/introduction.html)
Temporal extraction can similarly separate span recognition, normalized
attributes, and temporal relations, following TempEval's decomposition; Evie's
already-typed interval checks remain exact.
[TempEval-3](https://aclanthology.org/S13-2001/)

## Concrete starter corpus

Proposed first cut: 32 short synthetic source windows, four variants in each
family below, plus 10–20 short redacted or fully synthetic task histories shaped
like actual Evie usage. These counts are a manageable authoring starting point,
not sufficient sample sizes to certify a rare-error rate. Start with synthetic
histories if suitable redacted examples are not available.

| Family | Required variation |
| --- | --- |
| Durable facts versus no memory | Stable preference, throwaway comment, hypothetical, explicit request not to remember |
| Attribution and polarity | User statement, assistant speculation, quoted third-party statement, negation that contradicts a tempting positive extraction |
| Identity | Exact known identity, unseen identity, same-name people, pronoun or alias that remains ambiguous |
| Change and contradiction | Genuine change, correction of an earlier error, simultaneous conflicting sources, repeated unchanged claim |
| Time | Explicit date, relative date anchored to event time/timezone, bounded validity, missing/ambiguous date that must remain unknown |
| Evidence and eligibility | Exact quote, Unicode/byte boundaries, non-allowlisted tool field, source containing a synthetic secret marker or instruction injection |
| Scope and authority | Same fact in separate Workspaces, attempted global promotion, lower-authority conflict, withdrawn/ineligible source |
| Chunking and continuity | Cross-turn qualification, split pronoun antecedent, long irrelevant tool output, one malformed candidate among valid siblings |

Narrative histories should contain a handful of questions and gold evidence
sets prepared for later retrieval evaluation, but future questions and answers
must never appear in compiler inputs. Include project decisions, changed
preferences, people with recurring aliases, repeated source references, and
contradictions resolved only by explicit user action. The question rubric should
include both current and historical answers and deliberate abstention.

Each case records immutable source events, scope registry and revision,
projected eligible fields, existing accepted graph, required/optional/forbidden
candidate expectations, source spans/hashes, identity alternatives, and expected
uncertainty. Freeze the evidence projection used by the model as an artifact so
redaction or window construction failures can be diagnosed independently of
extraction.

Keep a separate model-free corpus of crash boundaries, expiry and stale fences,
overlapping coverage ranges, duplicate delivery, model/config changes, failures
at the head of a stream, cancellation, and concurrent approvals. The expected
outcome must state whether later work can proceed and which frontier moves;
counting final candidate rows alone can conceal lost coverage.

## Splits, repeatability, and leakage controls

- Assign development and sealed holdout sets by complete narrative, person,
  project, and scenario family lineage, not random turns. Keep paraphrases,
  renamed twins, and generated variants together. Expose both ordinary and
  adversarial slices in the final report.
- Pin model artifact/digest, quantization, inference server build, tokenizer,
  context limit, extraction schema, prompt, decoding settings, eligibility
  policy, normalization rules, fixture hash, and compiler version. A model name
  alone does not define a reproducible local run.
- Run a small repeated sample during the spike even with deterministic decoding;
  report output instability and failure cases. Freeze a repetition count before
  comparing finalists, and retain all runs rather than selecting the best seed.
- Keep source history, gold graph/candidates, question/answer rubric, and judge
  metadata in separate interfaces. Reset databases, caches, and memory between
  independent cases. Backfill and evaluation must respect source chronology and
  the case's as-of time.
- Report per-slice numerators/denominators, macro and pooled metrics, confidence
  intervals where useful, and paired baseline deltas. Resample by independent
  narrative when multiple turns share a history. With small counts, publish the
  actual failures and avoid treating zero observed failures as zero risk.
- Record when a holdout has been inspected. Promote revealed cases to the
  regression suite and create fresh holdouts for later model/prompt selection.
  Synthetic randomized identities and private histories reduce some leakage
  paths but do not prove absence of pretraining contamination in public data.

## Scalability experiment before adopting budgets

Measure a fixed foreground conversation workload with compilation disabled,
enabled at normal arrival rate, and catching up after an offline period. Repeat
with one busy scope and several independent scopes, and with a blocked head job.
Measure terminal-event commit and response-finalization latency separately from
model inference so the asynchronous design's DB contention remains visible.

Increase retained events and accepted entities geometrically from a small
fixture to an agreed one-year workload estimate; also increase source length
and facts per event independently. Log query counts and candidate-set sizes to
expose entity resolution that scans the whole graph per mention. Test fixed
bounded worker counts and window sizes on David's actual deployment hardware.
The default can be conservative; the evidence should determine whether more
concurrency helps throughput or only increases contention and peak memory.

Report p50/p95/max for queue wait, inference, validation/resolution, DB commit,
and eligible-event-to-reviewable-candidate time. Also record cancelled work,
timeout/retry rates, wasted inference after stale results, backfill completion
time, peak process and model-server memory, and persistent bytes per covered
event and candidate. Include zero-candidate events in coverage and throughput
denominators.

There are two capacity questions: can compilation keep up with incoming source
volume, and can David review the generated proposals? Express the expected
arrival workload and review-time allowance explicitly. Inference throughput
greater than event arrival does not solve an inbox where proposals arrive faster
than review decisions. Coalescing or suppressing repetitions should preserve
the original provenance and remain measurable; throttling must leave uncovered
work visible rather than falsely marking it complete.

## Stage gates and proposed requirement changes

1. **Before choosing the local model:** run the strict schema, evidence,
   timeout/cancellation, and malformed-output spike on common fixtures. Compare
   supported-candidate quality and resource use under the same inputs and
   budgets. Preserve a manual-only outcome if no local configuration is usable;
   the existing spec already forbids silent remote fallback.
2. **Before calling the compiler durable:** retain every Stage 3 exact gate and
   pass deterministic worker, coverage, cancellation, approval, and recovery
   tests. No live model call is part of this gate.
3. **Before enabling ongoing proposals:** freeze the memory-selection rubric,
   run held-out extraction cases, establish measured foreground/catch-up
   baselines, and approve an explicit quality and review-burden budget from those
   observations. There is no evidence yet for a particular precision percentage
   or maximum latency. A valid JSON-only pass is not enough.
4. **Before claiming memory improves answers:** compare no memory, raw-source
   retrieval, graph-assisted retrieval, and oracle-evidence reading with pinned
   reader/context budgets. The graph path should earn its complexity. This is
   later evaluation work; preparing gold sources now prevents redesigning the
   corpus then.
5. **Before any automatic admission:** require a separate policy decision and
   evaluation for narrowly named claim types. A good general extraction score
   or a low human rejection rate does not authorize unattended acceptance.

The current report schema reserves the four panels, but its closed failure
taxonomy only contains Stage 3 categories and its metric values are restricted
to p50/p95/max. Plan a versioned report-schema extension for learned-error
categories, counts and denominators, uncertainty, queue measures, and review
metrics. Preserve the existing semantic results rather than recasting them to
fit quality scoring.
[Report schema v1](../fixtures/semantic-memory/evaluation/v1/report.schema.json)

Replace the absolute phrase "never blocks the main response" in the Stage 4
completion criterion with two testable statements: inference is outside the
foreground critical path, and foreground overhead stays within an approved
measured budget under the declared workload. The transactional enqueue still
has a cost. Also add an observable extraction-quality and review-burden result
to the completion criterion. These are proposed specification amendments, not
changes made by this research note.

## Decisions for David

| Question | Starting recommendation |
| --- | --- |
| Which kinds of memories are valuable enough to review first? | Stable preferences, named project decisions, and explicit personal facts; defer speculative inference and broad world-knowledge extraction. Freeze examples and counterexamples before comparing models. |
| How much review time is acceptable per day or week? | Choose a real time allowance, then derive a proposal-volume budget from observed review speed. Do not pick candidates/day independently of usefulness. |
| Which errors are more costly: missing a useful fact, or proposing a wrong one? | Favor supported precision and conservative ambiguity handling initially; still measure missed required memories so an empty inbox cannot look successful. |
| How quickly should new facts become reviewable, and how much catch-up delay is acceptable? | Define separate active-use freshness and offline-backfill expectations after measuring the target local machine. |
| What is the intended one-year deployment envelope? | Describe events/day, long tool results, retained years, scopes, accepted entities, and always-on versus laptop duty cycle; use that to size the experiment. |
| Should assistant-only assertions produce review candidates initially? | Start with an explicit narrow policy; evaluate their yield and error rate separately. Lower source authority is metadata, not proof of factual support. |
| How should ambiguous names, unwanted truths, and stale candidates appear in review? | Show why review is needed and collect distinct outcomes, rather than one reject counter. Preserve unresolved identity instead of forcing a merge. |
| How much benchmark machinery belongs in this stage? | The small local corpus, component grades, runtime baselines, and review measures now; full external adapters and answer comparisons later. |
