# Stage 4 deterministic conformance, version 1

This is the engineering acceptance artifact for story #149 and the Stage 4
specification. It uses temporary real SQLite databases, scripted conversational
and extraction responses, actual REPL turns, public compiler/review methods,
CLI adapters, loopback HTTP, independent OS processes, and separately recorded
browser interaction. No downloaded model, live inference service, owner
adjudication, human pilot, resource result, or final holdout is involved.

Run from the repository root after installing the existing UI dependencies:

```sh
npm --prefix internal/web/ui ci
EVIE_PLAYWRIGHT_MODULE=/absolute/path/to/external/node_modules/playwright-core \
  node scripts/memory-stage-4-browser.cjs /absolute/path/new-browser-run
python3 scripts/memory-stage-4-conformance.py \
  --browser-receipt /absolute/path/new-browser-run/browser.json \
  --output-dir /absolute/path/new-conformance-run
```

The browser driver uses an externally supplied Playwright module and installed
Google Chrome; `EVIE_CHROME_PATH` can select another Chrome executable. It builds
the existing UI and starts the opt-in temporary test server itself. It installs
no production dependency and touches no user database. The script records its
actual browser interactions and source fingerprint in `browser.json`.

The output directory must be new and outside source directories. The runner
retains command output, a copy of the browser receipt, and `report.json` with
source file hashes, HEAD, environment versions, exact command arrays, exit
codes, log hashes, test outcomes, scenario evidence, warnings, and skipped
checks. SQLite's actual runtime version is in the public startup receipt; the
Go driver module version is also recorded. Generated UUIDs, observation times,
and hashes derived from them vary between runs. The assertions, schema, and
required scenario set are deterministic.

`--focused-only` runs the integrated normal/race suites and the report boundary
checks during development. It explicitly skips repository verification and UI
tests, leaves the result incomplete, and exits 2. A missing browser receipt also
leaves the result incomplete. The full command runs `./scripts/verify-change.sh`
and the complete UI Vitest suite, in addition to the normal/race integrated
checks. It retains the focused checks from every implementation story through
the repository's full Go suite. A failed required command, missing scenario,
skipped required focused test, stale browser receipt, or source change during
verification fails the result and exits 1. Only a complete passing run exits 0.
A failure in a scope, authority, source, persistence, or replay boundary blocks
conformance and the pilot/release path; learned-quality metrics cannot offset it.

The source fingerprint includes tracked and untracked code, configuration,
tests, relevant documentation, executable bits, and recorded source deletions.
It excludes build output, dependency directories, and run output in `.scratch/`
or outside the repository. Generate the browser receipt after freezing the
source, then run the conformance command against that same source. Any later
source change requires rerunning the affected checks and issuing a new bound
receipt. The fingerprint is an integrity binding, not a signature or an
attestation that an operator actually performed a browser action.

| Integrated scenario | Observable contract |
| --- | --- |
| Foreground, restart, closed review, replay | A real REPL turn commits and finalizes while extraction is stalled. Retry recovers the sealed selected window after reopening SQLite. The original source stays closed through exact inspection, approval, accepted reads, quarantine, and canonical rebuild. Replay makes zero model/extractor/tool calls. |
| Scope adapter matrix | Global, two Workspaces, two projects, and two exact session destinations yield the same candidate, source, preview, and result through Kernel, CLI, and HTTP. Every wrong destination is denied; private memory never enters global reads by implicit Promotion. |
| Generic storage containment | The registered `query_db` tool reads allowed control rows from the temporary read-only database, then rejects ordinary direct/nested, quoted, and qualified protected-table queries with exact policy errors and zero database opens. Unaccepted candidates remain absent from accepted reads. The privileged local shell is outside this model-tool boundary. |
| History and generations | A failed earlier unit keeps the contiguous frontier behind a later empty success; explicit history counts selected and outside events separately. Edited/rejected interpretation lineage survives a new generation, while new support still needs owner review. Activation does not sweep old history. |
| Atomic batch and source policy | Two dependent claims share explicitly chosen definitions in one atomic group. Failure writing the outer result rolls back all new effects. Exact retry preserves independent permanent rejection. A later source-policy revision redacts current disclosure without rewriting historical authority or accepted truth. |
| Independent startup | Forty-eight ordinary public starts cover concurrent fresh and retained databases without test-only WAL priming. File mode and each physical connection's WAL, foreign key, and busy timeout settings are checked. |
| Worker processes | Real process death, lease expiry, cancellation, uncertain capacity, mismatched release, trusted exact release, replacement, and delayed old transport completion preserve fencing, attempts, and single publication. |
| Review processes | Process exit just before/after durable acceptance plus two competing identical delivery retries produce one canonical operation; a different delivery exposes the prior resolution. |

`contract.json` binds eleven evidence records to ten named public integration
or boundary tests. Each record is accepted only if its emitting test, parent
scenario, and package pass. The report gate has its own deterministic tests for
missing/duplicate/wrong-package evidence, split Go output, failed child tests,
skips, stale browser data, and tracked/untracked/deleted source changes.

Browser evidence uses this envelope:

```json
{
  "version": "memory-stage-4-browser-v1",
  "source_sha256": "value from --fingerprint",
  "status": "passed",
  "provenance": {
    "web_interactions": "Describe actual candidate inspection, exact preview/approval, recorded operation provenance, and reload.",
    "store_checks": "Describe accepted Store reads and projection/replay checks after browser completion."
  },
  "cases": [
    {
      "name": "global",
      "checks": {
        "scope": true,
        "evidence": true,
        "preview": true,
        "resolution": true,
        "reload": true,
        "no_implicit_promotion": true,
        "accepted_graph": true
      }
    }
  ]
}
```

This abbreviated example is incomplete and fails validation. The real receipt
must contain all seven names from `contract.json`, with results backed by the
actual interaction and exact Store checks. Inspect and approve each prepared
candidate in its destination, compare the displayed source/preview hashes and
recorded result to the public fixture values, reload, and verify the same
accepted outcome. Include forbidden cross-scope access and private-to-global
Promotion checks. Global accepted-record browsing uses a separate active global observer. Public
accepted Store reads for global, Workspace, and project destinations use separate
active observers in the exact context. Closed-source session destinations use
owner operation inspection, exact temporary-database projection checks, and
canonical replay; do not reopen the
original source merely to make a UI route work, or label Store checks as graphical
browser interactions. No human preference or quality judgment is inferred from
scripted acceptance clicks.

Controlled fixture changes are explicit: source sessions are closed in SQLite
because the production session API exposes no close method; durable retry and
lease clocks are advanced to make crash recovery deterministic; narrowly scoped
triggers induce transaction failures or projection divergence; and a source
policy revision tests current redaction. Work selection, scheduling, extraction,
review, idempotency, accepted reads, verification, and owner rebuild still run
through the implemented public boundaries.

The SQLite startup change owned by #149 initializes the first physical SQLite
connection before schema work and retries typed primary/extended `SQLITE_BUSY`
errors. It does not retry DDL, migration, accepted operations, `SQLITE_LOCKED`,
I/O errors, or message text that merely contains “busy.” Existing incompatible
schema data must remain intact when initialization fails. Retry admission has
a five-second context budget; modernc SQLite v1.53.0 initializes DSN PRAGMAs in
`Driver.Open` with a background context, so an already entered physical open
still observes the existing five-second SQLite busy timeout. This is not a
claim that cancellation can interrupt that driver call or that all database
contention is eliminated. The tests preserve the distinction between physical
connection startup and subsequent schema/migration transactions.

The other narrow production change provides a trusted Evie-reader constructor
for the query tool. The default tool still supplies its existing default reader;
the conformance test supplies a reader for its temporary database. Both paths use
the same query implementation and unchanged SQL policy. This permits real allowed
reads and exact containment checks without changing `HOME` or reading a user
store. A targeted test-only Go overlay that permits unquoted memory identifiers
passes the old quoted-only test and fails the enhanced test; the mutation is never
applied to production source or a user database.
