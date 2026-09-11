# Exact historical source supplement

This source-only supplement was added after the development measurements. It
preserves the original local source needed by each measured agent test binary,
with SHA256 and original observed file mode/mtime, while leaving every original
freeze, trace and result unchanged. Historical commit hashes in those original
records describe the commit **at freeze**; these archives make reconstruction
independent of retaining rewritten branch commits.

| Run | Archived local files | Compressed bytes | Original inventory entries checked |
| --- | ---: | ---: | ---: |
| memory-stage5-reference-reader-v1 | 218 | 595571 | 676 |
| memory-stage5-reference-reader-v2 | 222 | 600803 | 680 |

The entire declared inventory was hash-checked before preservation. The adjacent
`*-declared-inventory.json` explicitly identifies excluded entries: unrelated
commands, scratch tools, PDFs/images and executables are outside the measured
agent binary's local import graph and are not copied. This matters particularly
for automatic-v3's original broad repository inventory. No owner data or secret
environment is included.

The archived graph includes all agent production/test Go files and non-test Go
files in every transitively imported local package, plus the original `go.mod`
and `go.sum`. Static traversal deliberately includes build-tag variants rather
than omitting a potential compiler input. No reachable local source uses
`//go:embed`; the repository's web UI embed is outside this graph. Third-party
module source and embedded assets are pinned by those module files and must be
available from the Go module cache or fetched using their recorded checksums.
The archived manifest distinguishes hashes present in the original freeze from
additional unchanged files retained from the original isolated source export.

Run the lossless source/archive and original-freeze inventory verifier from the
repository root:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-reference-reader/source-preservation/v1/verify-source.py
```

To reconstruct one measured source input, extract its `*-source.tar.gz` into a
new empty directory. With the recorded Go 1.26.3 darwin/arm64 toolchain, run the
original compile command there, changing only the output path:

```sh
go test -c -o /absolute/new-output/agent.test ./internal/agent
```

These are exact source inputs, not a claim that relinking at a different path
will reproduce the original binary digest. The original binary hash/build
command remains in that run's freeze or run metadata. Use its original README
for the opt-in fixture/model invocation and a **new** output directory. No model
or benchmark was rerun while creating this supplement; only hashes, archive
members, metadata references, modes and timestamps were verified.
