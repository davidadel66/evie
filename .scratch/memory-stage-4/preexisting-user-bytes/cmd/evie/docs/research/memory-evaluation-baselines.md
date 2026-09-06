# Semantic-memory evaluation baselines

**Research date:** 2026-09-01

**Authority:** primary benchmark papers, official benchmark repositories, and
Evie's approved [memory specification](../active/memory.spec.md) and
[semantic-memory ADRs](../../../../docs/adr/0049-separate-semantic-state-extraction-and-retrieval.md).
This note is research guidance, not an approved implementation specification.

## Recommendation

Evaluate Evie's semantic memory as three distinct systems:

1. **Accepted-state semantics:** deterministic operations, graph projection,
   scope, provenance, valid/transaction time, lifecycle, and replay.
2. **Learned components:** extraction, entity resolution, temporal parsing, and
   retrieval, each scored against gold intermediate artifacts.
3. **End-to-end behavior:** whether the final answer is correct, supported by
   the right evidence, and appropriately abstains.

Stage 3 should establish the first layer without any model calls. Benchmark QA
scores would be premature there: they would mix extraction, retrieval, and
reader behavior that Stage 3 deliberately does not implement. LongMemEval's own
analysis separates indexing, retrieval, and reading, and reports retrieval
Recall@k/NDCG@k separately from answer accuracy
([paper](https://proceedings.iclr.cc/paper_files/paper/2025/file/d813d324dbf0598bbdc9c8e79740ed01-Paper-Conference.pdf),
[official repository](https://github.com/xiaowu0162/LongMemEval)). Evie should
preserve the same diagnostic separation throughout its roadmap.

## What the external benchmarks contribute

### LongMemEval

LongMemEval has 500 questions spanning information extraction, multi-session
reasoning, temporal reasoning, knowledge updates, and abstention. Its released
records identify evidence at session and turn level, enabling retrieval
evaluation independently of QA. The paper evaluates flexible answers with a
pinned GPT-4o judge and reports Recall@k and NDCG@k for retrieval; the official
scripts report per-question-type, task-macro, overall, and abstention accuracy,
and exclude evidence-free abstention cases from retrieval metrics
([paper](https://proceedings.iclr.cc/paper_files/paper/2025/file/d813d324dbf0598bbdc9c8e79740ed01-Paper-Conference.pdf),
[QA aggregation](https://github.com/xiaowu0162/LongMemEval/blob/main/src/evaluation/print_qa_metrics.py),
[retrieval aggregation](https://github.com/xiaowu0162/LongMemEval/blob/main/src/evaluation/print_retrieval_metrics.py)).

Evie should reuse the capability slices and index/retrieve/read decomposition,
not treat one overall accuracy as the memory metric. Knowledge-update cases map
to correction and valid-time behavior; temporal and multi-session cases map to
historical and multi-evidence queries; abstention maps to unsupported-query
behavior. Any future LongMemEval run must pin the dataset revision, evaluator
prompt/model, and reader configuration because the official repository has
released cleaned dataset revisions after the paper.

### LoCoMo

LoCoMo's current official release contains ten long conversations with QA,
turn-level evidence IDs, and event-summary annotations. Its QA categories are
single-hop, multi-hop, temporal, open-domain, and adversarial/unanswerable. The
paper reports normalized token-overlap F1 for answers and correct-context recall
for RAG, while its event-summary task decomposes factual quality into atomic-fact
precision, recall, and F1
([paper](https://aclanthology.org/2024.acl-long.747/),
[official repository and data schema](https://github.com/snap-research/locomo)).

Evie should use LoCoMo's long, causally connected histories and evidence IDs to
shape multi-hop, temporal, and abstention fixtures. Official F1 should remain a
compatibility metric; it is not a substitute for exact provenance or semantic
state checks. Pin `locomo10.json` and its repository revision because the
official repository explains that this ten-conversation release is a selected
subset of an earlier fifty-conversation release.

### Entity, temporal, and provenance metrics

- For learned entity resolution, report mention detection separately from
  identity clustering. The official CoNLL-2012 evaluation uses the unweighted
  average of MUC, B-CUBED, and CEAF rather than trusting one clustering view
  ([task definition](https://conll.cemantix.org/2012/introduction.html),
  [reference scorer](https://conll.github.io/reference-coreference-scorers/)).
  Evie also needs domain-specific unsafe-merge and duplicate/split counts,
  stratified by scope, because a good aggregate cluster score cannot excuse one
  cross-person or cross-scope merge.
- For learned temporal extraction, TempEval-3 separates temporal-entity
  recognition, entity attributes, and relations; it reports strict and relaxed
  entity matches and closure-aware precision/recall/F1 for temporal relations
  ([task paper](https://aclanthology.org/S13-2001/)). Stage 3 needs stronger
  exact boundary conformance for Evie's already-typed intervals; TempEval-style
  metrics become useful only when Stage 4 extracts times and relations from
  text.
- For grounded answers, KILT reports retrieval provenance separately and also
  defines downstream scores that receive credit only when a complete provenance
  set is ranked at the top. This is a useful pattern for an Evie
  `supported_answer_rate`: answer credit requires both correctness and complete,
  allowed source support
  ([KILT paper](https://aclanthology.org/2021.naacl-main.200/),
  [official repository](https://github.com/facebookresearch/KILT)).

## Stage 3: deterministic baseline without model calls

Build a small, versioned fixture corpus of explicit accepted operations. Each
fixture should contain the initial scope registry, source events, operation
stream, expected canonical snapshot, and expected query/path results. Cover
global, Workspace, project, and session scopes; multiple independent sources;
aliases and same-name entities; corrections by `error` and `changed`; source
retraction; retirement/restore; contradictions; promotion; half-open validity
boundaries; and two-hop paths.

The first Stage 3 report should establish these hard gates:

| Dimension | Required observation | Initial gate |
| --- | --- | --- |
| Operation conformance | Every accepted/rejected operation and resulting revision matches the fixture | 100% exact |
| Replay and idempotence | Live projection, replayed projection, and repeated replay have the same canonical snapshot | 100% exact; zero model/capability calls |
| Scope and authority | Allowed read/write/reference matrix, promotion, and source-text expansion | Zero forbidden observations or mutations |
| Temporal semantics | Current, valid-at, and as-known-at results at before/start/inside/end/after boundaries | 100% exact answer-set match |
| Lifecycle | Legal transitions succeed; stale or illegal transitions fail without partial writes | Zero incorrect transitions |
| Provenance | Active claims have the exact eligible source links, evidence locations, authority, and visibility | 100% source completeness; zero dangling or out-of-scope expansion |
| Query equivalence | Direct SQL, recursive-CTE paths, inspection, and FTS rejoin return the expected accepted objects | 100% exact IDs/paths after canonical ordering |
| Recovery | Reopen, interrupted transaction, projection drop/rebuild, and stale revision cases preserve accepted truth | Zero divergence or partial projection |

The canonical snapshot comparison should include stable semantic IDs, typed
values, source links, state events, valid and transaction times, operation IDs,
and scope revisions, while normalizing row order and excluding SQLite-internal
or wall-clock noise. Failures in this table are release blockers and must never
be averaged into a quality score.

Stage 3 should also record, but not initially optimize, deterministic performance
baselines at fixed fixture sizes:

- operation commit latency and allocations;
- current, historical, provenance, and one-/two-hop query p50/p95/max latency;
- replay operations/second and total rebuild time;
- FTS rebuild time;
- database, WAL, and derived-index bytes per entity/claim/source/operation; and
- cold-open versus warm-query behavior.

Record hardware, OS, Go and SQLite versions, fixture cardinalities, journal
settings, repetitions, and whether caches are cold or warm. Approve absolute
budgets only after this baseline exists; later reports should show both the
absolute result and paired delta from the accepted baseline.

## What later stages add

| Roadmap stage | Added evaluation |
| --- | --- |
| **Stage 4: extraction/compiler** | Gold event-to-candidate fixtures; entity mention precision/recall/F1; MUC/B-CUBED/CEAF clustering scores; explicit unsafe-merge, duplicate, and ambiguous-quarantine counts; claim/predicate/typed-value/source-link precision/recall/F1; temporal span/attribute/relation scores; candidate staleness, idempotent backfill, queue delay, retry, lease, and ordered-commit diagnostics. Keep deterministic compiler mechanics separate from model quality, and pin model, prompt, schema, decoding settings, and repeated-run count. |
| **Stage 5: retrieval** | Evidence Recall@k and NDCG@k at claim, source-turn, and source-session levels; exact-ID/alias, temporal, multi-hop, update, and abstention slices; useful-context precision under the token budget; zero scope leakage after every retrieval signal; and provenance-complete retrieval. Run an oracle-evidence reader beside the real retriever so retrieval loss and reading loss remain distinguishable. |
| **Stage 6: graph acceleration** | Cached-adjacency versus recursive-CTE path-set equivalence as a 100% gate, then p50/p95 latency, allocations, memory, rebuild time, and stale-revision fallback at matched graph sizes. |
| **Stage 8: UI parity** | The same typed fixtures through REPL, conversational, and web inspection/proposal paths; no surface-specific scope, provenance, temporal, or approval semantics. |
| **Stage 9: end to end/policy** | Versioned redacted Evie replays plus pinned LongMemEval and LoCoMo adapters. Report write precision/recall, entity errors, lifecycle accuracy, retrieval metrics, answer accuracy/F1, abstention, KILT-style supported-answer rate, latency, token use, storage growth, and rebuild time. Tune admission, extraction, graph depth, and ranking only against this fixed suite. |

LongMemEval and LoCoMo should be supplemental external checks, not replacements
for Evie fixtures: neither benchmark encodes Evie's scope lattice, explicit
promotion, source authority, bitemporal transaction history, accepted-operation
replay, or projection-rebuild guarantees.

## Regression report contract

Each run should emit machine-readable per-case results plus a concise Markdown
summary containing:

- run ID; Evie commit; schema and fixture-manifest versions; exact dataset
  revision/hash; and baseline run ID;
- extractor, prompt, evaluator, reader, embedder, index, and configuration
  identities where applicable;
- case counts, skips, errors, and results by capability and scope, with macro and
  case-weighted aggregates shown separately;
- current value, baseline value, paired delta, and pass threshold for every
  metric; deterministic hard-gate failures listed before quality deltas;
- p50/p95/max performance with environment and cold/warm labels; and
- an error table keyed by fixture ID and taxonomy: extraction omission,
  unsupported write, entity over-merge/split, temporal-boundary error, stale
  lifecycle state, missing/wrong provenance, retrieval miss, reader error,
  false answer, or false abstention.

Do not publish one composite “memory score.” The minimum useful dashboard keeps
four top-level panels separate: **semantic conformance**, **write/extraction
quality**, **retrieval/provenance**, and **answer/abstention quality**. That shape
makes a regression attributable and prevents gains in model-based QA from
masking a deterministic safety failure.
