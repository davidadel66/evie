# Exact original #165 broker source

This preservation record adds the original source named by the immutable
`v1/freeze-v3.json`. It does not rerun the experiment or replace any source,
measurement, threshold, model manifest, or executable digest in that freeze.

The original measured revision is `f9706f8d0a5cb0d18fcdac22ef969e5bef824ccf`.
That object remains available in the development repository, but it is not an
ancestor of the rewritten implementation branch. Reproduction therefore uses
`original-baseline-source.tar.gz` instead of relying on that old object being
available in a fresh clone. The archive contains 173 exact original files:
`go.mod`, `go.sum`, and all non-test Go files in the broker's recursively selected
internal-package import closure, including platform variants. No selected package
uses `go:embed`. Uncompiled tests, other commands, UI assets and unrelated documents
are outside this broker's build inputs.

`source-manifest.json` records the original Git blob and SHA256 of every file,
the archive digest, the separately committed experiment driver's exact hashes,
and the selection method. Every extracted archive file was checked against its
recorded source hash. All four Python script hashes named by the original freeze
still match. The locally retained original broker also still matches the frozen
SHA256 `fc8f75253c77c45590f7912fa86787ac986c32f0742192211e798a5e371b9252`.
The binary is not rebuilt or replaced by this preservation step.

From the repository root, reconstruct in a new disposable directory:

```sh
SPIKE_REPRO="$(mktemp -d /tmp/evie-spike-source.XXXXXX)"
python3 - "$SPIKE_REPRO" <<'PYVERIFY'
import hashlib, json, pathlib, sys, tarfile
base = pathlib.Path('docs/experiments/memory-retrieval-spike/reproduction/v1')
manifest = json.loads((base / 'source-manifest.json').read_text())
archive = base / manifest['archive']
assert hashlib.sha256(archive.read_bytes()).hexdigest() == manifest['archive_sha256']
source = pathlib.Path(sys.argv[1])
with tarfile.open(archive, 'r:gz') as stream:
    stream.extractall(source, filter='data')
for path, record in manifest['files'].items():
    assert hashlib.sha256((source / path).read_bytes()).hexdigest() == record['sha256'], path
for path, record in manifest['spike_driver_files'].items():
    assert hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest() == record['sha256'], path
PYVERIFY
mkdir -p "$SPIKE_REPRO/scripts"
cp -R scripts/memory-retrieval-spike "$SPIKE_REPRO/scripts/"
go -C "$SPIKE_REPRO" build -o "$SPIKE_REPRO/broker" ./scripts/memory-retrieval-spike
```

Continue with the model/driver setup in `scripts/memory-retrieval-spike/README.md`.
A fresh build may have different bytes because of its build path, toolchain or VCS
metadata. Record a new reproduction freeze and preserve the original broker hash;
do not pretend a fresh binary was the one originally measured. External modules
are pinned by `go.mod`/`go.sum`, rather than vendored here. The original freeze
recorded a broker hash but no per-Go-file manifest; the source archive's provenance
is the exact Git revision that the original freeze identified.

As a supplementary check, every archived source/dependency file is byte-identical
to reachable issue #158 revision `1a64fde6348bcafb84b26e74937ea19ca8a8919a`.
The history repair at that stage changed only tests and documents. That comparison
supports production equivalence but does not replace the exact original archive.

Validation for this documentation-only preservation: archive re-extraction and
all 173 source hashes passed; driver hashes and original broker digest passed;
`git diff --check` passed. No build, model call or experiment was run.
