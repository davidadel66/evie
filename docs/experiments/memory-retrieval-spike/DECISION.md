# #165: select local MiniLM with SQLite vector storage and bounded brute force

The measured selection is `all-minilm:22m` through local Ollama 0.6.3, normalized
384-dimensional float32 vectors, SQLite persistence, and bounded cosine scoring.
Use dense suggestions alongside the exact/lexical baseline, with current Kernel
eligibility checks before fusion and before rendering. This supports proceeding
to #166; it does not implement that ticket or establish the Stage 5 release gate.
The HNSW candidate is rejected at this scale: it missed its held-out delivered-set
overlap gate and did not produce the required end-to-end latency advantage.

## Frozen inputs and protocol

[Freeze v3](v1/freeze-v3.json) was written at 2026-09-10T23:44:24Z before the
successful development comparison and the held-out comparison. Its SHA256 is
`0ba07a926e7082ff453d0f21ffc83b2d6c34433e28a564e3e15c714ed3628a06`.
The production baseline is commit `f9706f8d0a5cb0d18fcdac22ef969e5bef824ccf`.
The [development decision](v1/development-decision.md) records the provisional
SQLite selection before opening held-out query results. No model, gate,
numeric parameter, question, or expected evidence ID was tuned after that run.

The corpus contains 389 versioned synthetic source records: 32 target families,
32 forbidden-scope twins, 320 inventory distractors, and five secret probes.
Every family and its paraphrase/control/forbidden twin belong to one partition:
16 development families and 16 held-out families, two questions per family.
Indexing includes held-out source documents as in a normal retrieval corpus;
held-out questions are not used during development. This synthetic #165
partition is entirely separate from #167/#168. It is authored by the implementer,
not an independently annotated or natural owner-memory benchmark.

The disposable Go broker creates approved Claims with real recorded approval
and public semantic operations. It creates retained user/assistant episodes and
uses the shipped `Store.SearchMemory` for the lexical/exact condition.
All exported embedding input and every candidate result pass
`Store.RevalidateMemoryEvidence` on real SQLite. Global, General, a second
Workspace, and two projects enforce the same scope matrix as production.
The five secret records never enter embedding input; forbidden-scope twins
never enter a question's result. Assistant acknowledgments remain eligible
conversation distractors with their original authority. No raw SQL modifies
accepted state; only the separate disposable vector database uses experiment SQL.

The resulting corpus has 529 unique eligible embedding texts and 917 rows
across ten actor/evidence-kind indexes (36–145 rows each). Both vector indexes
contain identical normalized vectors. Query candidates are capped at eight,
cosine similarity at least 0.25, and delivered results at four / 8192 bytes.
Hybrid conditions use RRF k=60. HNSW uses M=16, construction ef=100, search ef=64,
seed=165, one thread. Ollama uses four requested CPU threads, batch size 16,
truncate=false and a 30-second keepalive. All configuration and model-layer
digests are retained in the freeze and [runtime metadata](v1/runtime-metadata.json).

The context cap covers `EVIE_MEMORY_DATA\n` plus the serialized evidence array,
including fixture IDs and exact Kernel source metadata. It does not represent
the complete agent provider request; #166/#167 must validate that boundary.
Latency starts before each condition's query embedding/search and ends after
current Kernel evidence validation, fusion and context packing. There are
three repetitions per case, 480 measurements per partition, with condition
order rotated deterministically. No query-embedding cache is used.

## Measured results

| Condition | Development recall | Held-out recall | Held-out paraphrase recall | Held-out p50 / p95 ms |
|---|---:|---:|---:|---:|
| Shipped lexical/exact | 22/32 | 23/32 | 7/16 | 0.785 / 7.477 |
| SQLite dense | 32/32 | 31/32 | 15/16 | 8.392 / 26.392 |
| HNSW dense | 32/32 | 31/32 | 15/16 | 7.894 / 25.278 |
| SQLite hybrid | 32/32 | 31/32 | 15/16 | 11.117 / 28.630 |
| HNSW hybrid | 32/32 | 31/32 | 15/16 | 12.298 / 28.430 |

All conditions recovered every lexical control; one held-out paraphrase
(`heldout-13-paraphrase`) was missed by both vector configurations, unchanged
across repetitions. Both dense configurations improved held-out paraphrase
recall from 43.75% to 93.75%, clearing the frozen 85% minimum and ten percentage
point improvement gates. SQLite full-query p95 clears the frozen 250 ms ceiling.
Zero forbidden-scope results, zero secret embedding inputs, and zero context
or result-cap violations were observed. The largest delivered array was 4844
bytes, so the four-result cap bound this short-text fixture before the byte cap.

HNSW delivered-set overlap with SQLite was 100% on development and 96.09375%
on held-out, below its frozen 98% gate. Its held-out dense p95 advantage was
about 4.22%, and hybrid advantage about 0.70%, below the frozen 20% requirement
for selecting the additional index dependency. Equal gold-evidence recall does
not erase these failures. HNSW is not selected and its gate was not weakened.

| Cost | Development | Held-out |
|---|---:|---:|
| Kernel fixture creation + eligible export | 1.488 s | 1.239 s |
| Embed 529 unique texts | 1.848 s | 1.838 s |
| Build SQLite vector table | 4.871 ms | 7.300 ms |
| Build and save ten HNSW indexes | 53.821 ms | 79.050 ms |
| Reopen SQLite + verify 917 rows | 0.152 ms | 0.162 ms |
| Load HNSW files + restore search ef | 5.085 ms | 5.416 ms |
| Rebuild HNSW from persisted SQLite vectors | 62.960 ms | 63.934 ms |
| Peak combined sampled process RSS | 360,644,608 bytes | 380,583,936 bytes |

SQLite vectors occupy 1,945,600 bytes; HNSW files occupy 1,546,688 bytes. The
HNSW reload and rebuild returned identical query rankings in both partitions.
The original comparison timed SQLite reopen as a connection/row-count check. Each partition independently
re-embedded and constructed its SQLite vector table; only HNSW has an additional
timed rebuild from persisted vectors. Resource samples cover Python, the broker,
the owned Ollama server, and descendants every 20 ms. Combined RSS includes
shared pages and is not unique physical footprint or a GPU-allocation measure.
Other heavy task checks were paused during the successful comparisons.

A subsequent [operational rebuild check](v1/sqlite-rebuild-check.json) reads no
questions and changes no frozen parameters or quality gates. All 917 vectors
were byte-identical across the development and held-out runs' independent
corpus re-embedding/builds. Rebuilding a fresh SQLite table from those persisted
vectors took 4.843 ms; reopening and comparing every row took 1.359 ms, with
exact equality. This is one supplemental sample, not crash injection or a
query-latency benchmark. The new file is 1,949,696 bytes; canonical sorted
insertion gives a different page layout from the original table.

Raw evidence IDs and per-condition latency/context observations are preserved in
[development traces](v1/development-results-v3/traces.json) and
[held-out traces](v1/heldout-results-v3/traces.json). Reports retain complete
sorted timing arrays, embedding batches, index costs and coverage; resource
files retain every sampled process RSS value. There are no invented or
interpolated measurements.

## Runtime, probes and limitations

The observed host is Apple M3 Pro (Mac15,6), 11 cores, 18 GiB RAM, macOS 15.7.8.
The 45,949,216-byte F16 model layer SHA256 is
`797b70c4edf85907fe0a49eb85811256f65fa0f7bf52166b147fd16be2be4662`;
the immutable manifest digest is
`1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef`.
The local model metadata reports BERT architecture, 384 embedding dimensions
and a 512-token context. The included model license is Apache 2.0. The
[official model page](https://ollama.com/library/all-minilm) and
[embedding API](https://docs.ollama.com/api/embed) describe acquisition and the
local endpoint; the experiment verifies the installed 0.6.3 behavior directly.

Nineteen [endpoint probes](v1/endpoint-checks.json) passed: literal loopback and
Unix-socket success; remote/DNS/TLS/credential/path rejection; redirects denied
without contacting their target; cancellation and deadline; malformed JSON,
wrong dimensions/count/model, nonfinite and zero-vector rejection; and closed
endpoint without fallback. Direct sockets do not consult HTTP proxy or TLS
environment variables. These probes exercise the disposable transport, not
future #166 production endpoint code. That code must preserve and test these
boundaries independently. Model acquisition is the only intentional registry
network use; synthetic embedding evidence goes only to the owned local server.

hnswlib 0.8.0 built successfully in a disposable Python 3.13.4 environment with
NumPy 2.2.2 and psutil 7.2.2. Its native extension and additional index lifecycle
are operational costs; [upstream documentation](https://github.com/nmslib/hnswlib)
requires setting search ef again after loading, which this experiment does.
No package was added to `go.mod` or another production dependency list. The
selected SQLite approach uses the existing database dependency; the embedding
runtime/model is an explicit local optional runtime requirement.

The corpus is small, short, English, and synthetic; 320 repetitive inventory
distractors do not approximate a large natural memory store. It cannot justify
latency or HNSW crossover claims at larger sizes. The threshold is a measured
spike configuration, not a release-quality precision/abstention decision.
Answer grounding, unseen-user generalization, reference interpretation,
unanswerable questions, concurrent writes, and production provider-request
cost remain work for their assigned tickets and #167/#168. Learned similarities
remain candidate scores; they confer no scope, source, acceptance or truth
authority.

Two initial integration attempts aborted, without a completed comparison:
`freeze.json` exceeded the Kernel's eight-item revalidation bound during corpus
export; `freeze-v2.json` lacked IDs for auxiliary assistant conversation hits.
The broker was corrected to use bounded exports and retain those real baseline
hits, and `freeze-v3.json` pinned the corrected executable. Their resource files
and initial freezes are retained. No held-out result existed during those fixes,
and no measurement or selection gate was changed because of them.

Reproduction and model-free verification commands are in the
[script README](../../../scripts/memory-retrieval-spike/README.md).

## Original baseline source preservation

The [source reproduction record](reproduction/v1/README.md) preserves the exact
173 source/dependency files for rebuilding the broker against the original
`f9706f8d0a5cb0d18fcdac22ef969e5bef824ccf` baseline, without relying on that
pre-history-rewrite commit being reachable in a fresh clone. The archive and
per-file manifest preserve original bytes. The locally retained measured broker
still matches its frozen digest; no build or measurement was rerun for this
preservation. A fresh reproduction must retain its own executable hash and leave
the original freeze unchanged.
