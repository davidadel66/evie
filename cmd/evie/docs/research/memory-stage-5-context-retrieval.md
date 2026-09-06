# Stage 5: retrieval and request context

Research checked 2026-09-05. This is a focused primary-source review, not an
exhaustive survey or an implementation decision. Recommendations below do not
describe completed Evie features.

## MemGPT foundation

MemGPT assembles each inference from static instructions, a small editable
working-context block, and a rolling message queue with an older-history
summary. External recall and archival storage become visible to the model only
through retrieval functions. The runtime manages queue pressure and feeds tool
results into subsequent inference; the model chooses memory operations and can
chain retrieval calls. Thus per-request assembly and model-directed retrieval
coexist. MemGPT's autonomous memory writes are not Evie's acceptance policy.
[MemGPT, sections 2.1–2.4](https://arxiv.org/html/2310.08560v2).

## Four findings

1. **Hybrid retrieval and automatic retrieval answer different questions.**
   SimpleMem combines dense semantic matching, lexical matching, and structured
   metadata in its January 2026 paper. Its retrieval budget changes with query
   complexity, and the chosen memories form the reader's context. This supports
   combining complementary search signals and controlling how much evidence is
   returned. It does not establish that every conversational turn requires an
   expensive planning model. Its reported token savings are benchmark results,
   not an Evie latency or accuracy guarantee. The current project has evolved
   beyond that paper, so an implementation comparison must pin a version.
   [SimpleMem v1, sections 2.2–2.3 and 3](https://arxiv.org/html/2601.02553v1),
   [author repository](https://github.com/aiming-lab/SimpleMem).

2. **An “agentic memory” system can still retrieve automatically.** A-MEM's
   section 3.4 explicitly performs retrieval for each interaction: embed the
   current query, select relevant notes, and construct the prompt. Its agency
   primarily concerns writing contextual notes, making links, and evolving their
   organization. The architecture also describes accessing linked memories.
   The paper finds diminishing gains, and sometimes decreases, as more memories
   are retrieved. “Fetch more” therefore needs an evidence budget and evaluation,
   rather than an assumption that more context helps. Evie should treat inferred
   links as retrieval suggestions; this paper's autonomous memory evolution does
   not replace Evie's approval or accepted-fact boundaries.
   [A-MEM v11, sections 3 and 4.5](https://arxiv.org/html/2502.12110v11).

3. **Separate retrieval success from answer success.** LongMemEval's author
   implementation includes 500 questions covering extraction, multi-session
   reasoning, updates, temporal reasoning, and abstention. It provides evidence
   session IDs, answer-bearing turn labels, and an oracle condition containing
   only relevant evidence. These allow separate checks for “did retrieval find
   the evidence?” and “could the model answer when given it?” Its small condition
   contains roughly 115K tokens per history, which is a useful full-history
   baseline at compatible context sizes. Pin the cleaned dataset version and
   reader/judge configuration when comparing runs; scores across different
   settings are not interchangeable.
   [LongMemEval author repository and evaluation format](https://github.com/xiaowu0162/LongMemEval).

4. **Deeper agent search buys accuracy at a real latency cost.** The May 2026
   LongMemEval-V2 release tests evidence gathering from web-agent histories,
   returning length-bounded context to a fixed reader. Its reported small-set
   baselines range from slice-plus-note RAG at 51.0% accuracy and 0.2 seconds to
   a file-based agent memory controller at 74.9% and 108.3 seconds. These are
   authors' benchmark measurements, not predictions for Evie. They illustrate
   why fast automatic recall and optional deeper tool-driven investigation are
   complementary. V2 also measures workflows, environmental pitfalls, and
   incorrect premises; those are useful later evaluation directions beyond
   remembering a personal preference.
   [LongMemEval-V2 author project, evaluation and results](https://xiaowu0162.github.io/longmemeval-v2/).

## What Evie already specifies

The active spec defines working memory as the exact request context, rebuilt for
every request and never the durable source of truth. Its planned ingredients
include instructions, a small owner/scope profile, task state and compaction
summary, a recent transcript tail, retrieved evidence, and tool schemas.
[Memory layers](../active/memory.spec.md#memory-layers).

Stage 5 plans exact entity/alias, lexical, dense, temporal, graph, and episode
search. Deterministic scope, authority, lifecycle, and time constraints precede
rank fusion. Results are rendered as bounded, source-bearing `EVIE_MEMORY_DATA`
immediately before the current user message, without writing that projection
back into history. These are specification requirements, not proof that the
retrieval engine is implemented.
[Hybrid retrieval specification](../active/memory.spec.md#hybrid-retrieval),
[Stage 5](../active/memory.spec.md#stage-5---hybrid-retrieval).

The current turn loop already loads working context and invokes the context
composer before model requests. That is the insertion point; it does not by
itself provide Stage 5 retrieval.
[Turn assembly](../../../../internal/agent/turn.go),
[Context composer](../../../../internal/agent/context.go).

## Recommendation for the next design

Use one retrieval service through two entry points: bounded automatic recall
when a new user turn begins, and a read-only search tool for a model that needs
additional evidence. Put both results through the same scope, provenance,
temporal, egress, and token-budget checks. This is a recommendation for Evie,
inferred from the sources, rather than a universal research consensus.

Anthropic likewise describes combining some upfront context with autonomous
tool-driven exploration, explicitly noting the runtime latency tradeoff. That
is a choice about **when to retrieve**, distinct from Stage 5's hybrid search
signals, which specify **how to find and rank evidence**.
[Anthropic, context retrieval and agentic search](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents).

Recompose the actual model request at each model invocation, including tool
continuations. Recomposition need not rerun every search or embedding: reuse a
retrieval result only while its query, scope, relevant revisions, and temporal
interpretation remain valid. Indexing and memory extraction belong outside the
ordinary answer's critical path. Cheap lookups and a small profile can support
routine preferences; difficult questions can spend more retrieval budget.

Before choosing a policy, compare: no recall, recent context only, tool-only
recall, automatic hybrid recall, and automatic recall plus deeper tools. Include
an oracle-evidence reader run to isolate generation failures. Measure evidence
recall under a fixed token budget, grounded answer correctness, abstention,
stale-fact errors, cross-scope leakage, p50/p95 latency, and total build/query
cost. Preserve a held-out test set, audit a sample of model-judged answers, and
include Evie-specific scope and approval cases. Those safety checks cannot be
inferred from a public benchmark score.
