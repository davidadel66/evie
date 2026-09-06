# #149 storage conformance refinement

The independent Spec finding is fixed and twelve owner files are frozen in
`149-owner-refined-frozen-files/`, with before/after hashes in
`149-owner-refined-frozen-manifest.json`. This supersedes the prior eleven-file
manifest; the old files and evidence remain preserved. No commits or unrelated
edits were made.

One production path is added to the contribution: `internal/tools/db.go`.
`QueryDBToolWithEvieReader` accepts a trusted reader opener and returns the existing
query tool definition. The built-in default handler still supplies the same
`eviedb.OpenDBReadOnlyContext` to the shared query implementation. SQL validation,
its table allowlist, query execution/rendering, and finance/transcript defaults
are unchanged. No global hook or environment mutation was introduced.

The registered tool in the conformance test opens only its temporary SQLite
fixture in read-only mode. Two ordinary and nested jobs queries must return the
exact seeded public row. All 77 initialized compiler/review/semantic tables and
views are tested through four SQL forms: ordinary unquoted, nested unquoted,
quoted, and qualified. Each of the 308 rejections must carry its exact policy
error; dispatch errors, unavailable databases, and nonexistent tables do not
qualify. The test proves no blocked query calls the database opener and verifies
that the unaccepted candidate remains absent from accepted claims.

The targeted mutation permits unquoted protected identifiers while preserving
quoted/qualified syntax guards. With this mutation the old test passes, showing
the missing coverage. The enhanced test fails on an actual temporary database
read of memory_compiler_activation_claims instead of the exact containment error.
Only Go overlays contain this mutation. `149-storage-mutation-evidence.json`
records exact commands, observed results and retained logs.

Verification:

- Normal refined storage plus existing Evie/transcript controls: PASS,
  cmd0.557s and tools0.268s (`149-storage-refined-green.log`).
- `go test -race ./internal/tools -run '^TestQueryDB' -count=1`: PASS13.569s
  (`149-storage-tools-race.log`).
- Full focused conformance runner: normal5.1s/race53.7s PASS, eleven receipts each,
  zero failures/warnings and unchanged source. Its report remains correctly
  incomplete/exit2 for explicitly pending full/UI/browser aggregate checks:
  `149-conformance-storage-refined-v1/report.json`.
- Gofmt and individual whitespace checks on all twelve frozen files: PASS.

The fixture README now includes the runnable browser driver command, external
Playwright/Chrome configuration, and the distinction between active-observer
public reads for global/Workspace/project contexts and owner-operation,
temporary-projection and canonical-replay checks for closed session contexts.
It documents both narrow production changes.

Root still owns the separate db.go startup hook, browser helper/driver, exact
isolated final verification/browser evidence, both reviews, and one #149 commit.
Review entry points are the constructor and unchanged validator in tools/db.go,
TestStage4GenericStorageConformance, and the mutation evidence.
