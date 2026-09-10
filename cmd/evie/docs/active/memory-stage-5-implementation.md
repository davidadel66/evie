# Memory Stage 5 implementation record

Parent: #154. Baseline: `f27546d5872c3c175dc45fca0ce814cbb5eb5afb`.
Delivery is one commit for each of #156–#168, in dependency order. A predecessor
is ready for dependent work only after its implementation is committed and its
focused checks pass. The final PR contains all thirteen commits. Issues remain
open during implementation.

## #156: accepted-memory retrieval contract

The confirmed primary seam is `Session.Send` with real temporary SQLite,
accepted operations, and a scripted provider capturing complete requests.
HTTP and React component tests cover inspection authorization and presentation.

The agent consumes a narrow search and evidence-revalidation interface. The
Memory Plugin receives a turn-bound search function through the existing
invocation context; model arguments cannot choose a session, scope, receipt, or
budget. Exact Stage 3 reads remain available with their existing contracts.
The new relevance capability is separately versioned in the pinned composition.

Search combines the FTS Claim/entity/alias projection and exact identifiers or
accepted aliases. SQLite remains authoritative. An immutable index generation
uses bounded restartable maintenance outside provider dispatch. Initial coverage
must finish before serving. Continuous accepted changes enqueue affected
documents transactionally. Incomplete coverage is explicit; stale documents are
rechecked against exact semantic reads before disclosure.

Evidence uses existing Claim identities, creation operations, exact read times,
source link identities, source scopes and authorities, and immutable locators
and hashes. The provider receives text only in a bounded `EVIE_MEMORY_DATA`
synthetic user message immediately before this turn's actual user message.
Tool outcomes retain counts and status, with no source IDs or copied text.
Neither the synthetic projection nor repeated tool outcomes become new source
evidence.

Each existing context-snapshot event atomically contains a content-free receipt
with rendering version, outcome, and the exact supplied evidence references.
The snapshot is persisted under the active turn fence before provider dispatch.
A later assistant event belongs to the existing turn/request sequence; a
snapshot without a subsequent answer records an attempted request, not a
completed answer or a claim of delivery. Inspection follows saved references,
never current relevance search, and reapplies current source access. Supplied
evidence is distinct from an explicit answer citation.

Every provider dispatch revalidates evidence, source access, and remote-memory
opt-in. Secret scanning applies to the complete rendered evidence. Retrieval
tool content contains counts and status only, so durable replay cannot restore
stale text or disclose source references after remote egress is disabled.
Failures expose a bounded category without source text or driver errors.

The selected slice caps are 8 searches per turn, 8 results and 12 KiB per
search, and 36 KiB cumulative serialized retrieval data per turn. The Kernel
also caps queries at 1,024 UTF-8 bytes/32 literal terms, 64 candidates, three
eligible sources per Claim, 32 KiB per original content field, and a 500 ms
SQLite-query deadline inside the agent's 750 ms invocation deadline. Total
retrieval and revalidation work is capped at 3 s per turn. The turn charges the
full search-result serialization plus each synthetic user-message serialization;
this conservatively overcounts the smaller count/status-only durable tool output.
The complete provider request separately passes the existing context estimator.
Successful calls never reset these budgets. Empty, unavailable, failed, partial,
cancelled and exhausted outcomes remain distinct.

On the versioned 45-Claim fixture (40 distractors, five relevant Claims), 30
fresh turns recovered 150/150 expected evidence instances. The recorded run
measured whole-turn p50 9.149 ms, p95 14.396 ms, a largest serialized memory
message of 6,031 bytes and largest complete provider request of 25,206 bytes.
Initial indexing took 13.732 ms; idle refresh p95 was 263.375 microseconds.
These observations support the conservative slice caps; they are not final
release gates, production-scale measurements, isolated search latency, or model
answer quality. Raw samples and the fixture/configuration are retained in
`../fixtures/memory-stage5-retrieval/v1/accepted-slice-report.json`.

Runtime maintenance processes at most 256 rows per batch with a 5 s deadline,
100 ms pacing while pending, 1 s idle polling, and 5 s retry pacing. Shutdown
cancels and joins the worker with a 5 s bound. Idle refresh performs no write.
Fresh installations reconcile retained Claims before activation; accepted
changes and source-area archival enqueue refresh work in their transaction.

`POST /api/memory/evidence` takes the selected `sessionId` and `snapshotId`.
It resolves only that saved request, reapplies current access, and returns
`Cache-Control: no-store`. Live and replayed activity contains only outcome and
counts. The compact card opens the existing Inspector and labels evidence
"Accepted memory" and "Supplied to model".

## Progress and evidence

- #156 first red: `go test ./internal/agent -run
  '^TestMemorySearchTurnSuppliesAcceptedEvidenceWithoutMutation$' -count=1`
  failed because the provider received no accepted source-bearing search result.
- #156 regressions cover literal and Entity Claims, exact/alias matches, current
  and sibling sessions, Context Scope isolation, Promotion source redaction,
  retirement/restoration, secret exclusion, source retraction/archive, malformed
  and empty queries, incomplete coverage, shared call budgets, remote opt-out
  before dispatch, request receipts/reopen, and original source inspection.
- Initial integration failures were corrected: redundant index trigger broke
  restart schema validation; old preset construction changed frozen capability
  hashes; extra prompt text exceeded a small existing context fixture; old tool
  result references survived egress opt-out. The Stage 3 acceptance explicitly
  permits the new Stage 5 FTS objects while verifying frozen 1.1 tool behavior.
- Focused checks: Go agent/plugin/web packages, command/runtime regression,
  changed-package vet, six HTTP/UI adapter regressions, UI lint/build and
  whitespace checks pass. Exact per-commit command results are recorded below
  before commit. The full `./scripts/verify-change.sh` remains scheduled for the
  complete 13-issue integration, rather than represented as already run.
- UI warnings: five pre-existing Fast Refresh lint warnings, the existing
  >500 KiB bundle-size warning, and `npm ci` reported three audit findings
  (two moderate, one high). No dependency upgrades were made. An attempted
  `npm ... run test` failed because this project has no test script; the direct
  `npx vitest run ...` command passed.
- #157–#168 remain pending. No local model or release quality evaluation has run.

### #156 verification

- `go test ./internal/agent ./internal/plugins ./internal/web ./cmd/evie` —
  pass (agent 3.680 s; plugins cached; web 4.967 s; command 11.991 s).
- `go test -race ./internal/agent -run '^TestMemorySearch' -count=1` —
  pass, 17.119 s.
- `go test ./internal/eviedb -run 'TestSemanticScopeContainmentAcceptanceMatrix|TestDurableCompactionChainSurvivesSQLiteRestart|TestSemantic.*Reopen' -count=1` —
  pass, 1.668 s.
- `go vet ./internal/agent ./internal/eviedb ./internal/plugins ./internal/web ./cmd/evie` — pass.
- `go test ./internal/agent -run '^TestMemoryRetrievalSliceMeasurements$' -count=1 -v` — pass; retained raw report above.
- `npm --prefix internal/web/ui run lint` and
  `npm --prefix internal/web/ui run build` — pass with the recorded warnings.
- From `internal/web/ui`:
  `npx vitest run src/chat/MemoryActivity.test.tsx src/artifacts/MemoryEvidence.test.tsx src/api/memoryEvidence.test.ts` —
  three files, six tests passed.
- `git diff --check` — pass. Full repository script remains pending the
  complete feature integration; this record does not claim final readiness.

Demonstration: with remote-memory opt-in enabled, start a new conversation
using the current standard preset; approve a memorable preference, allow the
background index to finish, and request a focused lookup from another eligible
conversation. Open Accepted memory in the activity card, then reopen the
conversation after process restart. Source inspection must retain the original
reference and show an unavailable state after source access is revoked.
