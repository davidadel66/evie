# Graphiti episode persistence inspection

## Inspection boundary

This record is design evidence for Semantic Memory Stage 3.1. It describes what
the inspected Graphiti code persists; it does not make Graphiti's graph schema an
Evie requirement.

- Repository: [`getzep/graphiti`](https://github.com/getzep/graphiti)
- Release: [`v0.30.0`](https://github.com/getzep/graphiti/releases/tag/v0.30.0)
- Commit: [`31653f61d5d4dd916f85d7dba89666ce6a621552`](https://github.com/getzep/graphiti/commit/31653f61d5d4dd916f85d7dba89666ce6a621552)
- Inspected example: [`examples/quickstart/quickstart_neo4j.py`](https://github.com/getzep/graphiti/blob/31653f61d5d4dd916f85d7dba89666ce6a621552/examples/quickstart/quickstart_neo4j.py), especially the first two text episodes about Kamala Harris
- Inspected backend: the Neo4j model and Cypher path, not the FalkorDB, Kuzu, or
  Neptune representations
- Inspection date: 2026-09-01

The example was inspected statically because running it requires Neo4j plus LLM
and embedding credentials. Static inspection is sufficient for this ticket: the
Pydantic models, generated Cypher, and transaction helper expose the persisted
shape directly. No claims are made here about rows emitted by a particular
nondeterministic model run.

The exact inspection commands were:

```sh
graphiti_dir=$(mktemp -d /tmp/evie-graphiti-103.XXXXXX)
git clone --quiet --depth 1 --branch v0.30.0 https://github.com/getzep/graphiti.git "$graphiti_dir"
git -C "$graphiti_dir" rev-parse HEAD
sed -n '1,260p' "$graphiti_dir/examples/quickstart/quickstart_neo4j.py"
sed -n '318,570p' "$graphiti_dir/graphiti_core/nodes.py"
sed -n '263,375p' "$graphiti_dir/graphiti_core/edges.py"
sed -n '1,230p' "$graphiti_dir/graphiti_core/models/nodes/node_db_queries.py"
sed -n '1,225p' "$graphiti_dir/graphiti_core/models/edges/edge_db_queries.py"
sed -n '980,1230p' "$graphiti_dir/graphiti_core/graphiti.py"
sed -n '128,230p' "$graphiti_dir/graphiti_core/utils/bulk_utils.py"
```

## Observed episode-to-entity-to-edge representation

The quickstart calls `Graphiti.add_episode` with a name, body, source type,
source description, and reference time. The pipeline retrieves previous episodes,
extracts and resolves Entity nodes and relationship edges with model calls,
hydrates summaries and embeddings, then saves the episode, `MENTIONS` edges,
Entities, and `RELATES_TO` edges in one backend write transaction.

The inspected Neo4j representation is:

| Concept | Observed persisted representation |
| --- | --- |
| Episode | An `Episodic` node keyed by `uuid`, with `name`, `group_id`, `source`, `source_description`, raw `content`, `entity_edges`, `created_at`, and episode `valid_at` properties. |
| Entity | An `Entity` node keyed by `uuid`, with dynamic labels and attributes plus `name`, `group_id`, `summary`, `name_embedding`, and `created_at`. |
| Episode to Entity | A `MENTIONS` relationship keyed by its own `uuid`, with `group_id` and `created_at`. |
| World relationship | A `RELATES_TO` relationship keyed by `uuid` between two Entity nodes. It stores `name`, free-text `fact`, `fact_embedding`, a list of episode UUIDs, `created_at`, `expired_at`, `valid_at`, `invalid_at`, `reference_time`, and optional attributes. |
| Partition | The same `group_id` string appears on Episodes, Entities, `MENTIONS`, and `RELATES_TO`. The quickstart omits it and therefore uses the provider's default group/database. |
| Provenance | A relationship carries an `episodes` list of episode UUIDs. The Episode also carries an `entity_edges` list, and `MENTIONS` records Episode-to-Entity association. There is no separately identified, lifecycle-managed evidence link for one exact excerpt. |
| Valid time | The Episode's `valid_at` is its supplied reference time. A relationship may carry nullable `valid_at` and `invalid_at` values extracted by the model, plus `reference_time`. Later information may set `invalid_at` and wall-clock `expired_at`. |
| Database shape | Neo4j nodes and relationships are the stored graph. `MERGE` matches generated UUIDs, range indexes cover UUID, group, name, and temporal properties, and vector/full-text indexes support retrieval. |

The save path is atomic for the episode graph batch passed to
`add_nodes_and_edges_bulk`. Model extraction and resolution happen before that
write. Optional saga/community work extends the call with additional persisted
objects and is outside this episode-to-Entity-to-edge comparison.

## Direct comparison with Evie

| Boundary | Graphiti v0.30.0 observation | Evie Stage 3 decision |
| --- | --- | --- |
| Accepted truth | The output of model extraction, deduplication, and edge invalidation is written by `add_episode`. | A model may prepare a proposal, but only an explicitly approved, revision-checked Semantic Operation enters accepted history. Stage 3 has no automatic extraction. |
| Canonical history | The inspected graph records are the primary persisted representation. | The accepted operation stream is canonical; SQLite graph rows are a deterministic replayable projection. |
| Episode | Raw content is copied into an `Episodic` graph node. | Episodic Memory remains the canonical evidence. Semantic state cites existing event identity rather than copying the event into the graph. |
| Entity | Entity nodes include learned summaries, embeddings, dynamic labels, and attributes. | Accepted Entities contain stable identity, scope, canonical name, type, lifecycle, and provenance only. Learned summaries and embeddings are later derived state. |
| Relationship | Free-text `RELATES_TO` edges represent world facts and carry embeddings. | A reified Claim has a versioned Predicate, Entity or Typed Literal object, polarity, half-open Valid Time, lifecycle, and Source Links. Structural Graph Links are separate from world Claims. |
| Provenance | An edge's episode UUID list identifies supporting episodes but does not identify one evidence span or give that association independent lifecycle. | Each random-ID Source Link identifies one exact eligible event locator, authority class, observation time, and append-only eligibility history. |
| Temporal state | Relationships store `valid_at`, `invalid_at`, and `expired_at`; model extraction and invalidation update the graph. | Valid Time is `[valid_from, valid_to)`. Transaction Time and Scope Revision record when Evie accepted every append-only transition; corrections never rewrite accepted history. |
| Scope | `group_id` partitions records and may select a provider database. | A canonical registry gives global, Workspace, project, and session scopes distinct identities. Every semantic row has one non-null `scope_id`; model text cannot select or widen it. |
| Identity and equality | UUIDs identify graph records; model-assisted resolution deduplicates Entities and relationships. | Random UUIDs are generated once and recorded. Predicate, Typed Literal, Claim, Source Link, and idempotency equality are deterministic and model-independent. |
| Storage and recovery | The inspected backend is a graph database with graph/vector/full-text indexes. | SQLite is canonical local storage. Replay, verification, quarantine, and shadow rebuild use accepted operations without model, network, or external calls. |

## Resulting constraint

Graphiti validates the usefulness of an Episode-to-Entity-to-temporal-edge view,
but its persisted edge is too broad to be Evie's Claim contract. Evie retains
the useful temporal graph shape while separating evidence, accepted operation
history, deterministic identity/equality, scope authority, lifecycle, and
derived retrieval data. The exact Evie encodings and fixture contract are frozen
in `semantic-memory-encodings.spec.md` before semantic DDL begins.
