# Actual Stage 5 dense integration regression

The first frozen run **fails** the required held-out paraphrase recall gate:
81.25% versus the unchanged 85% minimum. This directory preserves that failure,
the passing development comparison, and every measured turn. It does not prove
#166 quality completion or the #167/#168 release gates.

The fixture is the unchanged 389-record corpus and the two 32-question
partitions from `docs/experiments/memory-retrieval-spike/v1`. Those #165 questions
have already been evaluated. Their reuse here is integration regression, not a
new untouched held-out evaluation. No #167/#168 questions were opened.

## Frozen execution

`frozen-v1/freeze.json` was written before either partition was measured. Its
SHA256 is `75b8415a43d9a3b6565eb9f450f0333af861b29285e74db9b0e2297be585f730`.
The actual test executable is retained locally at
`/tmp/evie-stage5-dense-integration-v1.test`, SHA256
`8e312acedaf9a177bf01faa3555cf57d9770d8be14b20789c2f77abc0e3cb13d`.
The stable source snapshot is
`/tmp/evie-stage5-dense-integration-v1-source`; production and test Go inputs were
hashed before/after export and before/after compilation. The observed Git HEAD
at export was `92dc0fc617f193b6be513aedbcdadf81c2b40d4d`, with the uncommitted
#166 implementation and harness included. File hashes identify the measured
code; that historical commit alone does not contain it.

`frozen-v1/compiled-source.tar.gz` additionally preserves the exact compiled
source inputs, original fixture files and runner independently of later commit
rewrites. `source-archive.json` hashes every member and
`source-archive-verification.json` records compilation from a fresh extraction
with network dependency resolution disabled. It contains no executable, model
weights, owner database or credentials. This supplemental archive does not
modify the original freeze.

The freeze verifies every installed model layer, the selected MiniLM manifest,
the actual Ollama executable, and `/api/tags` plus `/api/version`. It records the
same selected model, normalization, threshold and RRF settings as #165. The
production adapter verifies the model tag before and after inference.

Each partition creates a fresh real SQLite database through public operations.
`f.remember` adds its normal owner-source prefix and uses `retrieval_marker`;
the corpus literal is unchanged. `f.converse` records the original corpus text
through a complete turn and adds a scripted `Noted.` acknowledgement. All source
families, including sources belonging to the other partition, are authored and
fully indexed before the selected question file is decoded. The retained
source mapping records exact original Claim and event identities. A reporting
bug left its `source_link_id` values empty by reading the proposal's nested
source field instead of its separate Source Link ID. The exact supplied Source
Link IDs are retained in provider evidence and receipts. Supplemental
`supplied-source-associations.json` files derive only those observed links from
the raw traces; no IDs are invented for corpus Claims that were never supplied.
The current harness corrects this mapping assignment for future freezes, while
both original frozen executables/source archives remain unchanged.

Each measured reader is a fresh session in the question's original context.
Automatic Recall is explicitly disabled. A scripted provider selects the
appropriate production memory-search tool, receives the real composed request,
and completes the turn. The lexical condition leaves the optional embedding
endpoint unset; the hybrid condition uses the actual selected local model.
Three repetitions rotate condition order by case index and repetition.

An actual loopback HTTP observer forwards unchanged requests to the owned
server at `127.0.0.1:11565`, with proxies and redirects disabled. Its input hashes,
byte lengths, HTTP statuses, timing and secret-scanner counts are retained.
It supplies no artificial vectors. Its forwarding overhead is included in the
measurements; the original server is left running for the task owner.

## Measurements and bounds

Whole-turn time brackets `Session.Send`, including persistence, tool dispatch,
actual query embedding, retrieval/fusion, source revalidation and provider
request composition. It excludes corpus construction, maintenance, reader
creation, inspection for the read-only assertion and artifact serialization.
The 250 ms gate is applied to this larger complete-turn boundary. These are not
isolated search timings or model-answer-quality measurements.

The production bounds are eight delivered results, 12 KiB serialized search
result and 36 KiB cumulative retrieval data per turn. #165 used four delivered
results and an 8192-byte evidence-array budget. A synthetic memory message
includes the production rendering guide and JSON wrapper, so its measured
13 KiB maximum is distinct from the 12 KiB search-result bound. The harness
directly checks delivered count and the memory projection against the turn cap;
it records the configured search-result cap rather than claiming to separately
observe the internal search-result serialization. Complete provider request
bytes are obtained from the actual production request encoder. The scripted
test profile is fixed at 300000 hard / 262144 working / 16384 output tokens;
this is not a measurement of an external reader model's token usage.

Each `traces.ndjson.gz` contains the exact encoded provider requests, supplied
evidence, durable final receipt and per-turn observations. `samples.json` keeps
the compact observations; `report.json` contains unchanged gate outcomes and
nearest-rank p50/p95 samples. `build.json` retains every bounded maintenance
batch, and `execution.log` preserves the actual test pass/failure. No result was
overwritten or omitted because it failed.

## Reproduction

Use an isolated copy of the implementation being evaluated. For the original
run, the retained local snapshot and executable must match the frozen hashes.
An equivalent checkout requires matching all recorded compiled inputs; a later
fix must get a fresh executable, freeze and output directory. Do not bypass a
hash mismatch to claim an identical reproduction.

The helper uses Python's standard library only. It neither acquires a model nor
starts/stops a server. The disposable runtime/model acquisition instructions
remain in `scripts/memory-retrieval-spike/README.md`.

```sh
python3 cmd/evie/docs/fixtures/memory-stage5-dense/v1/run.py freeze \
  --source /absolute/path/to/isolated-source \
  --output /absolute/path/to/new-freeze \
  --binary /tmp/new-dense-experiment.test \
  --models /tmp/evie-memory-retrieval-spike-v1/models \
  --runtime /Applications/Ollama.app/Contents/Resources/ollama

python3 cmd/evie/docs/fixtures/memory-stage5-dense/v1/run.py run \
  --freeze /absolute/path/to/new-freeze/freeze.json \
  --partition development --output /absolute/path/to/new-development-attempt

python3 cmd/evie/docs/fixtures/memory-stage5-dense/v1/run.py run \
  --freeze /absolute/path/to/new-freeze/freeze.json \
  --partition heldout --output /absolute/path/to/new-heldout-attempt
```

Routine `go test` skips the opt-in experiment without a freeze. That skip is not
reported as actual-model verification; the retained executable runs here are
the evidence.
