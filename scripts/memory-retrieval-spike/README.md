# Local semantic retrieval experiment (#165)

Run from the repository root. This is a disposable query-to-evidence experiment,
using public Kernel APIs against fresh SQLite. It adds no production dependency
and does not open an owner database. See the
[decision](../../../docs/experiments/memory-retrieval-spike/DECISION.md).

The reported lexical/exact baseline is commit
`f9706f8d0a5cb0d18fcdac22ef969e5bef824ccf` (#158). Rebuild the broker against that
revision when reproducing this comparison; building against later production
retrieval code changes the baseline. The Python driver may run from the current
checkout, but its frozen source hashes must agree throughout a run.

Requirements: Go 1.26.3, Python 3.13.4, `uv`, and Ollama **0.6.3**. The recorded
host is Apple M3 Pro / Mac15,6, 11 CPU cores, 18 GiB RAM, macOS 15.7.8. hnswlib
0.8.0 compiles a native extension using the local C++ toolchain. Python packages,
model weights, binaries, databases, and indexes belong in a disposable directory.

```sh
SPIKE_ROOT="$(mktemp -d /tmp/evie-retrieval-reproduction.XXXXXX)"
SPIKE_OLLAMA=/Applications/Ollama.app/Contents/Resources/ollama
SPIKE_FIXTURES="$PWD/docs/experiments/memory-retrieval-spike/v1"
mkdir -p "$SPIKE_ROOT/models" "$SPIKE_ROOT/source"
uv venv "$SPIKE_ROOT/venv" --python 3.13.4
uv pip install --python "$SPIKE_ROOT/venv/bin/python" -r scripts/memory-retrieval-spike/requirements.txt
git archive f9706f8d0a5cb0d18fcdac22ef969e5bef824ccf | tar -x -C "$SPIKE_ROOT/source"
cp -R scripts/memory-retrieval-spike "$SPIKE_ROOT/source/scripts/"
go -C "$SPIKE_ROOT/source" build -o "$SPIKE_ROOT/broker" ./scripts/memory-retrieval-spike
env OLLAMA_HOST=127.0.0.1:11565 OLLAMA_MODELS="$SPIKE_ROOT/models" \
  OLLAMA_NUM_PARALLEL=1 OLLAMA_MAX_LOADED_MODELS=1 OLLAMA_KEEP_ALIVE=30s \
  "$SPIKE_OLLAMA" serve > "$SPIKE_ROOT/ollama.log" 2>&1 &
SPIKE_SERVER_PID=$!
```

Wait until the owned server responds at `/api/version`, confirming version
`0.6.3`. Do not reuse an unrelated process already occupying port 11565. Acquire
the model through the owned local server; acquisition may use the public Ollama
registry, whereas embedding requests have no remote path or fallback:

```sh
curl --fail --silent --show-error http://127.0.0.1:11565/api/pull \
  --header 'Content-Type: application/json' \
  --data '{"model":"all-minilm:22m","stream":false}'
"$SPIKE_ROOT/venv/bin/python" - "$SPIKE_ROOT/models" <<'PY'
import hashlib, pathlib, sys
p = pathlib.Path(sys.argv[1]) / 'manifests/registry.ollama.ai/library/all-minilm/22m'
assert hashlib.sha256(p.read_bytes()).hexdigest() == '1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef'
PY
"$SPIKE_ROOT/venv/bin/python" scripts/memory-retrieval-spike/check_endpoint.py "$SPIKE_ROOT/endpoint-checks.json"
"$SPIKE_ROOT/venv/bin/python" scripts/memory-retrieval-spike/measure.py freeze \
  --models "$SPIKE_ROOT/models" --broker "$SPIKE_ROOT/broker" \
  --fixtures "$SPIKE_FIXTURES" --output "$SPIKE_ROOT/freeze.json"
"$SPIKE_ROOT/venv/bin/python" scripts/memory-retrieval-spike/measure.py run \
  --partition development --freeze "$SPIKE_ROOT/freeze.json" \
  --broker "$SPIKE_ROOT/broker" --fixtures "$SPIKE_FIXTURES" \
  --state "$SPIKE_ROOT/development-state" --output "$SPIKE_ROOT/development-results" \
  --server-pid "$SPIKE_SERVER_PID"
```

Read the development report against the frozen gates before proceeding. Run
held-out once with that same freeze and binary, without changing configurations
or thresholds based on its outcomes:

```sh
"$SPIKE_ROOT/venv/bin/python" scripts/memory-retrieval-spike/measure.py run \
  --partition heldout --freeze "$SPIKE_ROOT/freeze.json" \
  --broker "$SPIKE_ROOT/broker" --fixtures "$SPIKE_FIXTURES" \
  --state "$SPIKE_ROOT/heldout-state" --output "$SPIKE_ROOT/heldout-results" \
  --server-pid "$SPIKE_SERVER_PID"
"$SPIKE_ROOT/venv/bin/python" scripts/memory-retrieval-spike/check_sqlite_rebuild.py \
  "$SPIKE_ROOT/development-state/vectors.sqlite" \
  "$SPIKE_ROOT/heldout-state/vectors.sqlite" "$SPIKE_ROOT/sqlite-rebuild-check.json"
kill "$SPIKE_SERVER_PID"
wait "$SPIKE_SERVER_PID"
```

The driver refuses to overwrite a freeze or run directory and checks fixture,
Python-source, and compiled broker digests before opening partition questions.
A reproduction records a new freeze because build paths/toolchain/VCS metadata
can change executable bytes; the recorded `freeze-v3.json` remains immutable.
`fixtures.py` documents deterministic fixture construction. The committed
fixtures, including their family partition and hashes, are the recorded inputs.
These are separate synthetic #165 cases, never the #167/#168 release holdout.

`context_bytes` measures `EVIE_MEMORY_DATA\n` plus the serialized evidence array
with experiment IDs, exact Kernel evidence and source metadata. It is **not**
the complete agent provider request. The result limit is four and the byte cap
is 8192 for all conditions. Full query latency includes local query embedding
where applicable, persisted vector reads/index queries, current Kernel
revalidation, rank fusion where applicable, and context packing. Startup,
corpus embedding, index build/reopen/rebuild are measured separately. Every
condition embeds each query afresh, conditions rotate deterministically, and
three repetitions retain all samples. No answer-generation model runs here.

For script checks without loading a model:

```sh
go test ./scripts/memory-retrieval-spike
go vet ./scripts/memory-retrieval-spike
python3 -m py_compile scripts/memory-retrieval-spike/*.py
python3 scripts/memory-retrieval-spike/check_endpoint.py /tmp/evie-retrieval-endpoint-checks.json
```
