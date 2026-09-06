# #149 frozen conformance contribution

Eleven new files are frozen in `149-owner-frozen-files/` with per-file before/after
hashes in `149-owner-frozen-manifest.json`. All were absent at HEAD
9f97fdc10adb75445c118d00cbc6442133927af4. No staging, commits, user-file edits,
production dependencies, live models, pilot work, or owner observations were
created. The process contributor's independently frozen file is included exactly
(SHA256 bf80c6528ebb21ad16d30d8022098265c26997cd6c0df23f0c424878ab2bea51).
Root's db.go connection hook and browser helper/driver remain separate root-owned
contributions. Do not fold the old startup prototype's entire db.go.

The implementation addresses published ticket18 (#149), its handoff preflight,
the Stage4 complete acceptance path and binding testing/review/work contracts.
The only production helper retries typed BUSY during first physical SQLite
connection initialization, before schema or application work. It preserves
cancel/deadline/error codes and never retries later DDL/migrations/writes, LOCKED,
I/O errors, or error text alone. Driver.Open's context limitation is documented
both in code and the fixture README; this does not promise universal contention
recovery or a strict five-second physical-open wall time.

Four cmd tests integrate actual REPL, Store, CLI and real loopback HTTP over real
SQLite: stalled extraction with foreground commit/finalization, retry after
reopen, permanently closed original source review, exact candidate/source/preview/
result, accepted reads, quarantine/rebuild/replay; seven exact scopes and 42 wrong
scope pairs per adapter; actual query_db containment; explicit history, failed
earlier gap, later empty success, outside events, edited/rejected recurrence and
fresh evidence; shared-definition atomic batch, outer persistence rollback,
competing rejection, immutable partial retry, current-policy redaction and
preserved historical authority. Two real-process scenarios add five receipts
covering worker crash, expiry/cancel/late completion, conservative capacity and
exact release fencing, review process exit around COMMIT, and competing duplicate
review. Public startup performs 48 fresh/retained starts without WAL priming.

The versioned Python stdlib runner binds 11 receipts to 10 required named tests
and their passing test/package events in normal and race runs, handles split Go
output, records source files/HEAD/environment/SQLite runtime and driver versions,
commands/log hashes/warnings/skips, and rejects absent, duplicate, stale or failed
evidence. `--fingerprint` prints only the current source SHA256. Default mode also
runs repository verification and the full UI Vitest suite. A matching actual
browser receipt is mandatory for `passed`; focused-only or omitted browser stays
`incomplete` and exits2. The source fingerprint includes code/tests/docs under
cmd/internal/scripts/docs/.github and root Go/package/AGENTS files; generated
scratch/dependency/build output is excluded. Browser driver source should live in
one included source root. Source changes during execution fail conformance.

Verification completed:

- `python3 scripts/memory_stage4_conformance_test.py`: PASS6 tests. Report gate
  regression cases cover failed/skipped/missing tests, duplicate/wrong-package
  receipts, split output, stale/incomplete browser evidence, and source mutations.
- Focused normal Go command covering the exact ten named Stage4 tests: PASS,
  cmd/evie3.907s and internal/eviedb3.285s, `149-all-focused-normal.log`.
- `python3 scripts/memory-stage-4-conformance.py --focused-only --output-dir
  .scratch/memory-stage-4/149-conformance-focused-v1`: expected exit2 `incomplete`;
  all environment and boundary commands PASS; integrated normal PASS2.9s;
  integrated race PASS53.7s; 11 receipts/36 test outcomes in each; zero failures,
  zero warnings; source unchanged. Exact argv, log hashes, outcomes and reasoned
  skips are in `149-conformance-focused-v1/report.json`.
- All seven Go files `gofmt -l`: empty output. All11 new files individual
  `git diff --no-index --check /dev/null <file>`: empty output, no whitespace errors
  (exit1 merely reflects a new-file diff).
- Process owner separately ran normal0.989s/race33.133s PASS; its original
  contribution records are `149-process-manifest.json` and `149-process-handoff.md`.

Required aggregate checks remain root-owned: isolated combined-tree
`./scripts/verify-change.sh`, complete UI Vitest, actual browser interactions
against this version's seven scopes, independent Standards/Spec reviews, and one
#149 commit. These are explicitly pending; the focused report does not claim
conformance complete. No live model/extraction-quality/resource/human-pilot result
can be inferred from this contribution.

Review entry points: `scripts/memory-stage-4-conformance.py`, fixture README and
contract.json; `memory_stage4_conformance_test.go` main integrated acceptance;
`sqlite_startup.go` narrow production change plus physical process tests;
`stage4_process_conformance_test.go` OS-process fault scenarios; the batch and
history integration tests for durable/authority boundaries.

Preserved intermediate failures were fixture corrections, not hidden product
violations: expected owner actor was incorrectly `user`; accepted reads initially
used the closed original conversation instead of a new active observer; moving
query timestamps prevented a meaningful before/after replay comparison; actual
REPL emits three events per turn, so history counts are6/7; CLI pretty-printing
changes RawMessage whitespace only, so compare canonical JSON; a valid atomic
batch must share explicit newly chosen definitions, not group unrelated refs.
All corrections are reflected in final normal/race evidence. Root's earlier
public startup failures and minimized driver diagnosis remain separately recorded
as the concrete production defect that the narrow connection helper addresses.
