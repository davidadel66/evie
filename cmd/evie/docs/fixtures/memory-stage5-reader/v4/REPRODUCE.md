# Reproduce v4 after the implementation history is rewritten

The historical run used commit `4ec5429` before the implementation commits were
folded/rebased for the final PR. That recorded identity and the original frozen
README must not be rewritten after seeing the result. Reproduction does not
require fetching that temporary commit.

Use the final PR's commit-to-issue mapping to select the commit implementing
#159. Archive that commit into a fresh disposable directory. The owning commit
contains the unchanged original reader fixture, this production-reader adapter,
and the run metadata. Before compiling or generating answers, compare every
production Go file in that archive against `production_go_files` in
`run-metadata.json`, and compare both test-source hashes against `freeze.json`.
If they differ, this is a new implementation condition and requires a new
versioned attempt. Never edit this attempt's recorded hashes to make them match.

For example, from the new archive root, with the v4 artifact files present:

```python
import hashlib
import json
from pathlib import Path

root = Path.cwd()
artifacts = root / "cmd/evie/docs/fixtures/memory-stage5-reader/v4"
metadata = json.loads((artifacts / "run-metadata.json").read_text())
freeze = json.loads((artifacts / "freeze.json").read_text())
digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
for item in metadata["production_go_files"]:
    assert digest(root / item["path"]) == item["sha256"], item["path"]
assert digest(root / "internal/agent/retrieval_reader_evaluation_test.go") == freeze["test_sha256"]
assert digest(root / "internal/agent/retrieval_production_reader_evaluation_test.go") == freeze["adapter_sha256"]
```

Compile that verified archive, run the opt-in metadata preflight into a new
artifact directory, and create a new freeze that records the new binary path
and digest, current official metadata, exact discovered context profile, source
hashes, settings and original rubric. Absolute Go build paths can change the
binary digest even when source is identical; record the new digest honestly.
Use the compiled opt-in production reader test as shown in the original README.
Never reuse the old result directory or overwrite its raw answers.

The hosted model alias, canonical metadata and normal provider-routing policy
must be checked again. Backend vendor choice and weights are not immutable
under the current production API contract. A successful reproduction is a new
measurement under its newly recorded remote configuration, not proof that the
provider executed byte-identical model weights.

The captured `*-wire-response.raw` files are byte-preserving transport artifacts.
SSE keep-alives contain a colon followed by a space, and the stream terminates
with protocol blank lines. Git treats these paths as binary via `.gitattributes`
to preserve them verbatim; they must not be whitespace-normalized. Normalized
response JSON, composed/wire request JSON and the manual assessment remain
ordinary reviewable text. An initial staged whitespace check identified those
protocol bytes; the attribute corrects their classification without changing
the captured bytes, source checks, or any behavioral acceptance gate.
