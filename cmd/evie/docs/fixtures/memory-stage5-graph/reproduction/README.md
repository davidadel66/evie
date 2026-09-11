# Preserved #164 graph experiment sources and remaining gaps

The original graph v1 and v2 freezes and measured results remain unchanged.
These new archives preserve only files whose recovered bytes match the original
frozen SHA256. They are explicitly incomplete source subsets, not reconstructed
original build trees. No missing file was synthesized from remembered code,
replaced with a later implementation, or assigned a new expected hash.

| Original run | Frozen Go files | Exact files recovered | Missing files |
|---|---:|---:|---:|
| v1 | 300 | 296 | 4 |
| v2 | 301 | 299 | 2 |

Both original measurements included a disabled draft of Automatic Recall while
isolating manual graph search. The separately verified owning #164 commit omits
that later-stage draft. It therefore supplies the committed graph behavior but
is not byte-identical to the originally measured source tree. Current production
or later experimental archives must not be presented as those original bytes.

The exact gaps in both runs are:

- `internal/agent/retrieval_automatic.go`: `36bf0443692aa3d1d6302ee6f45589a7509dc279d85f3e7407fde7a090ff3f78`
- `internal/agent/retrieval_automatic_acceptance_test.go`: `b99a628b11dd7001cc1a49f41d13d371d7f7400e0963aedb6c8af5804997d7dc`

The additional v1 gaps are:

- `internal/agent/retrieval_graph_acceptance_test.go`: `152fa9a501e39a72ab0804f06d3fd8773d4d5fea2c67b60627ed9daa7ef694c0`
- `internal/eviedb/retrieval_graph.go`: `7568d136fa17d7b35e54071f8e0f5cf83b21d8b3d61ed90d3a66689456110a55`

Recovery checked retained temporary source exports, current source, reachable and
reflog blobs, and those four paths across all 273 locally available Git commit
trees. An independent bounded check of 694 unreachable Git blobs also recovered
neither missing automatic-draft hash. None supplied the missing exact hashes. The manifests list every recovered
file's origin and hash and retain the missing expected hashes separately. The
original locally retained `graph-tests` and `graph-tests-v2` executables still
match their frozen digests; source recovery did not execute them.

`go.mod` and `go.sum` are supplied from the original baseline revision
`a594ba22ad2d8e05fbfc02f8aef7e4b09312e868`, with explicit supplementary provenance.
The original freeze did not hash these files or all transitive dependencies.
Consequently, recovering its listed Go files alone would not retrospectively
prove an entirely frozen dependency closure. No original-source rebuild claim is
made here, and an exact rebuild remains blocked by the missing files.

From the repository root, inspect either preserved subset in a new directory:

```sh
GRAPH_REPRO="$(mktemp -d /tmp/evie-graph-preserved.XXXXXX)"
python3 - v2 "$GRAPH_REPRO" <<'PYVERIFY'
import hashlib, json, pathlib, sys, tarfile
base = pathlib.Path('cmd/evie/docs/fixtures/memory-stage5-graph/reproduction') / sys.argv[1]
manifest = json.loads((base / 'source-manifest.json').read_text())
archive = base / manifest['archive']
assert hashlib.sha256(archive.read_bytes()).hexdigest() == manifest['archive_sha256']
source = pathlib.Path(sys.argv[2])
with tarfile.open(archive, 'r:gz') as stream:
    stream.extractall(source, filter='data')
for path, record in {**manifest['files'], **manifest['supplementary_files']}.items():
    assert hashlib.sha256((source / path).read_bytes()).hexdigest() == record['sha256'], path
print('Verified recovered subset; missing original files:', manifest['missing_frozen_files'])
assert manifest['frozen_source_complete'] is False
PYVERIFY
```

A future reproduction must label its own source/configuration and retain these
original records and gaps. It cannot replace a missing source with a newer file
and reuse the original executable digest or claim to repeat the original run.

Validation of this documentation-only preservation: all 296 v1 and 299 v2 frozen
source hashes passed, along with archive re-extraction and the two original
locally retained executable digests. `git diff --check` passed. No build, model
call or experiment was run.
