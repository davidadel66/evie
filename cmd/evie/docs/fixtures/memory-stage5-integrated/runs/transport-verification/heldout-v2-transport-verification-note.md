# Held-out v2 preservation transport and source reconstruction

The existing preservation verifier is correct for the original local preservation
copy, but its direct-file timestamp/mode checks are not a fresh-Git-checkout
contract. Git preserves file contents and a limited executable-bit distinction;
it does not preserve each original file's nanosecond modification time or complete
POSIX mode. A copied file's current checkout metadata therefore cannot establish
that its original metadata survived Git.

The immutable plans still retain the original direct-file metadata as recorded
provenance. Lossless preservation archives additionally retain original member
mtime/mode inside PAX/tar headers, which survive as archive bytes. Do not restore
recorded timestamps onto a fresh checkout just to make a strict verification pass:
that would be reconstructed metadata, not independent evidence of preservation.

## Separate read-only transport verifier

Prepared outside the repository:

- `/tmp/evie-memory-stage5/verify-preserved-transport.py`
- SHA-256 `6766b659ab963096a6042b885b83bbd19b4c140cfafb3ec0203a43dfb0b8a2e9`

The script verifies the exact retained file set, file byte counts and hashes,
plan/manifest consistency, all archive members and their recorded original
mtime/mode, source/input archives against the frozen hashes, and referenced
repository records. It reports direct checkout metadata differences explicitly;
it never treats those differences as changed source bytes or claims that Git
preserved original direct-file times/modes. It performs no model calls, extraction,
build, metadata restoration, Git operation or result reclassification.

After copying this supplementary script outside any immutable run directory, a
fresh checkout can use it as follows (replace `/checkout/evie` with its actual
absolute location):

```sh
python3 -B /path/to/verify-preserved-transport.py \
  --repository /checkout/evie \
  --directory /checkout/evie/cmd/evie/docs/fixtures/memory-stage5-integrated/runs/heldout-v2 \
  --output /tmp/heldout-v2-transport-verification.json
```

The output path must be new and outside the immutable run. Hash verification is
relative to the retained manifests and chosen repository revision; it is not a
cryptographic signature or a claim of independent artifact authenticity. The
original strict verifier remains unchanged and useful against the original
metadata-preserving local copy. Documentation should label those two commands by
purpose instead of promising that the original strict command passes after Git
transport.

Controls executed without Git or model calls:

1. The original retained development-v4 package passed: 73 retained files,
   2,041 preservation-archive members plus 64 direct original files, 15 referenced
   repository records, 394 source inputs and 48 seed files.
2. A disposable copy made with `shutil.copyfile` had all 64 direct-file timestamps
   changed. Transport verification passed with those differences reported; the
   unchanged strict verifier correctly rejected its time/mode mismatch.
3. Adding one newline to that disposable copy's `freeze.json` was rejected by
   the transport verifier. Restoring the original bytes passed again despite
   the new timestamp.

Exact records: `/tmp/evie-memory-stage5/transport-development-v4-verification.json`
and `/tmp/evie-memory-stage5/transport-verifier-controls-heldout-v2.json`.
The heldout-v2 repository package was not yet present when this bounded review
ran, so no claim is made that its final copy was already checked by this script.
Root should run the same command after its final authorized preservation copy.

## Current integrated freeze inputs and actual commands

The original heldout-v2 freeze SHA-256 is
`c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2`.
Read-only archive verification checked every member against its frozen manifest:

| Input | Exact retained identity | Verified members |
| --- | --- | --- |
| Compiled source archive | `3b4e0cc90c9c82f2167b171672ffb05207f0a7ed3914c114400bef731445bec3` | 394 |
| Source manifest | `1c65712b69a6c54e531eb753be7bb0d054c4ef28b89bbc21651ad990f1614447` | 394 |
| Canonical input archive | `95826cfcd43404ad8e14c5f9971a0b965a4ea6179ebfe84dcbfb727bfc9349d4` | 48 |
| Canonical input manifest | `72dd8ab4c1e6d6f497af0938bd1448672537e44ab309a989582d1f18f93241c5` | 48 |

The source archive contains 392 Go compilation inputs plus `go.mod` and `go.sum`.
There are no `//go:embed` declarations in those archived Go inputs, so no declared
application embedded assets are missing from this compilation archive. The 48
input members are the 24 canonical SQLite seeds and their exact public source/ID
maps. No source bytes, seed values, historical metadata or frozen hashes were
changed during this review.

Heldout sealing reused the exact development-v4 executable, whose recorded hash
is `06ba5e56aecf5684f2209bd801074ea65ed032dc88181d07a4d016260532f072`.
It is intentionally excluded from repository preservation. Its original source,
module pins, Go version and complete compilation command are retained. The
preserver copies the development build record to
`provenance/development-build.json`; the historical command is:

```text
go test -c ./internal/agent -o /private/tmp/evie-memory-stage5/integrated-development-v4/integrated.test
```

The measured working directory/source root and worker settings are recorded by
that build/freeze. No bit-identical executable rebuild has been attempted in this
review. Rebuilding needs the matching toolchain/platform and module dependencies;
source reconstruction alone does not claim that a new binary or model answer is
identical to the old observation.

The original closed `index`, `local`, `operating`, and `reader` execution records
all contain their actual commands. Each invokes the exact retained frozen
`run.py`, the frozen JSON and a new mode-specific output directory. Their original
exit codes are respectively 1, 1, 0 and 1. The initial wrapper, the withheld-reader
record, and the explicit first-reader continuation with its protocol-deviation
record are in the preserver's required selection; nothing replaces the failed
prerequisites with a successful sequence. The frozen programs, source/input
archives, original command records and all failure reports remain available for
inspection without rerunning any model.

For relocated source reconstruction, extract into a new isolated directory and
retain the originals unchanged. Use a new reproduction record when changing
absolute paths, rebuilding an executable, reacquiring the pinned embedding model,
or making new provider calls. Original freezes intentionally validate their
observed absolute paths and binary hash; editing them to make a new environment
look like the original run would destroy provenance. Live provider/model
availability remains an external prerequisite, not an artifact guarantee.

Machine-readable source and command verification is retained at
`/tmp/evie-memory-stage5/heldout-v2-source-reproducibility-check.json`.

## Suggested documentation correction

Outside all immutable preservation directories, update `runs/README.md` to state
that original strict commands verify the original local copies' metadata.
Document the supplementary transport command for fresh Git checkouts, with its
explicit content-versus-direct-metadata limits. Preserve this script and its
control records as supplemental evidence outside the strict directory, or select
them before the final heldout preservation plan. Do not modify any previously
frozen preserver, manifest, program, corpus, score, gate or raw output.
