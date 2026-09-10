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

## #157: scoped conversation evidence and retirement association

Conversation search reads allowlisted public user/assistant content from the
same durable Context Scope: one Workspace, one project, or Global-to-Global.
Accepted Global-memory access is not raw Global-conversation access. General
remains a Workspace. The source session may be old; retrieval does not resume it,
read its session-scoped Claims, or widen current mutation authority.

The durable association is the existing immutable Source Link tuple
`(claim_id,event_id,event_part,locator_kind,locator_value,evidence_sha256)`.
For ordinary excerpts, the Kernel subtracts the union of corresponding ranges
for every retired Claim or ineligible Source Link before selecting any passage.
A whole-content locator denotes the entire cited message; a canonical UTF-8
range denotes exactly that half-open byte interval. Multiple supporting Claims
share a passage: one retired association suppresses its interval even if another
Claim remains active. Restoration removes only that Claim's exclusion, so an
independent retired association still applies. Repeating a suppressed interval
inside a larger excerpt cannot evade filtering. Unrelated eligible messages and
non-overlapping precise passages remain available.

The implementation does not infer smaller fact-level ranges from whole-content
citations. Such provenance cannot prove that a sub-passage is independently
unrelated. Whole-content suppression is therefore visible as the cited-message
boundary, not silently widened to the whole session. If a later acceptance case
requires recovering unrelated facts inside the same whole-content source, that
requires a new approved provenance association; it is not manufactured here.

The event index has its own immutable generation and durable retained-row
checkpoint. Initial backfill runs in bounded maintenance batches; eligible
appends update the allowlisted projection inside the event transaction. Only
that generation's complete coverage allows conversation queries. The active
accepted-Claim generation remains independently queryable while event coverage
is building. Every hit is rechecked for scope, current source access, secret
fences, retirement intervals and exact UTF-8 locator/hash before rendering.
Source text remains in the synthetic request projection; durable outcomes carry
counts/status, and receipts retain exact original references.

First red: `go test ./internal/agent -run
'^TestConversationSearchFindsUncompiledOriginalStatement$' -count=1` failed
because the complete turn had no conversation-search evidence message.

A second red exposed a stale-index bug: retiring a precise range removed its
entire event from candidates, hiding a disjoint passage. Search now treats stale
event hits only as suggestions and applies the current exclusion union to the
original bytes. Pending refresh is reported as partial coverage. Seven complete
reader turns cover shared ranges, overlapping ranges, independent restoration,
and a disjoint surviving passage without writing semantic state during reads.

The scope matrix verifies exact source IDs in both provider requests and durable
receipts for Global, General, another Workspace, and two projects, including
earlier current-session roots and same-area sibling sessions. Secret-bearing
content is excluded as a whole field. Long UTF-8 excerpts are centered on the
match, capped at 800 bytes, and preserve original byte positions through restart.
Assistant content remains attributed to the assistant with authority `none`.
Only public message content is indexed; tool payloads are not conversation data.
Retirement follows recorded Source Links; an unlinked assistant paraphrase is
still separately attributed conversation evidence, not an inferred association.

### #157 verification

- `go test ./internal/agent ./internal/plugins ./internal/web ./cmd/evie` — pass
  (agent 2.586 s; plugins/web cached; command 10.221 s).
- `go test -race ./internal/agent -run '^TestConversationSearch' -count=1` — pass,
  6.732 s (before the separately verified scope-matrix addition).
- `go test ./internal/agent -run '^TestConversationSearchTurnScopeMatrixAndEarlierRoots$' -count=1` — pass, 0.354 s.
- `go vet ./internal/agent ./internal/eviedb ./internal/plugins ./internal/web ./cmd/evie` — pass.
- From `internal/web/ui`, `npx vitest run src/chat/MemoryActivity.test.tsx src/artifacts/MemoryEvidence.test.tsx src/api/memoryEvidence.test.ts` — 3 files, 8 tests passed;
  `npx tsc -b` — pass.
- `git diff --check` — pass. Final repository verification remains pending all
  thirteen tickets.

The original Stage 3 schema check now allows only the additional event FTS
projection and its SQLite-managed shadow tables. The accepted-slice measurement
reads accepted-generator coverage separately from combined host maintenance;
its retained #156 measurements are unchanged. Existing published preset hashes
and tool schemas remain fixed; the unreleased current preset adds scoped search.

Demonstration: record an unaccepted, tentative statement in one conversation,
then ask for it from a second conversation in the same area. Open Conversation
excerpt to inspect its speaker, original session, and byte range. A conversation
in another Workspace or project must not receive that raw history.

## #158: bounded conversation expansion

The model requests `memory_expand_conversation` using an exact evidence ID
already supplied in the current turn and counts before/after. The turn resolves
that ID to its trusted reference; arbitrary event IDs, other-area references,
accepted Claim IDs, and same-area sources never retrieved this turn grant no
expansion authority. Trusted references and covered ranges are not model
arguments. The Store revalidates the anchor in the same bounded transaction as
neighbor reads, including the original scope, speaker, byte range and hash.

A request permits zero through two public messages on each side within 64
original event positions, in the anchor's source session only. Current-session
expansion excludes the live user root and later events. The Store selects the
bounded candidate window before filtering; an excluded neighbor does not cause
an unbounded search for replacement text. Every passage passes source, secret,
retirement, and UTF-8 checks. The Kernel subtracts previously supplied ranges
before emitting new exact slices of at most 800 bytes, retaining source order.
Repeated requests add no duplicate evidence. Omitted inaccessible intervals,
clipped passages and a bounded sequence gap report truncation explicitly.

Expansion shares the existing eight-call, eight-evidence, 12 KiB result, 36 KiB
cumulative context, and three-second work limits with search. The turn passes its
remaining evidence capacity to the Store; results cannot claim undisclosed
matches after the turn is full. Invalid bounds fail, untrusted anchors are
unavailable, a fully covered window has no new matches, and exhaustion preserves
supported findings without claiming that missing evidence does not exist.

A complete-turn regression exposed source identifiers replayed in the original
expansion tool arguments after remote-memory opt-out. Provider history now
projects a source-reference placeholder for this tool while preserving the
original durable event. Native Responses continuation items for that event are
replaced by the canonical projected tool call, so opaque transport data cannot
bypass the same boundary. Exact source references remain in revalidated memory
and immutable request receipts. Chat and native Responses tests check actual
serialized wire requests, not only the visible memory block.

The development window experiment runs ten real SQLite turns for each window
size with a scripted provider. Zero/one/two messages per side supplied 1/3/5
excerpts; the two-message setting used at most 10,770 serialized memory bytes and
31,723 complete request bytes, with whole-turn p95 11.837 ms in the retained run.
This supports the provisional bounded window for development; it does not
measure model interpretation quality or establish release gates. The runnable
`TestConversationExpansionWindowMeasurements`, source hash, hardware,
configuration and thirty raw samples are retained in
`../fixtures/memory-stage5-retrieval/v1/expansion-window-report.json`.

The pronoun example receives “She hasn't booked it yet” first, then reads the
original preceding statement that Maya is considering Kyoto. Both remain
attributed conversation excerpts, without fabricated Claims or semantic writes.
Separate request receipts preserve the exact additions and resolve the same
source positions after database restart. The model's scripted response is not
claimed as an answer-quality evaluation.

Additional full-turn checks cover forged and out-of-scope anchors, disjoint UTF-8
ranges, duplicate/overlapping expansions, retired neighboring ranges with an
unrelated surviving passage, secret exclusion, cancellation without a late
receipt, mixed-call exhaustion and oversized windows. The existing Conversation
excerpt activity and source inspector render expansion references directly.

### #158 verification

- `go test ./internal/agent ./internal/plugins ./internal/web ./cmd/evie` — pass
  (agent 4.749 s; plugins 1.728 s; web 4.088 s; command 10.573 s).
- `go test -race ./internal/agent -run '^TestConversationExpansion' -count=1` —
  pass, 32.207 s.
- `go test ./internal/web -run '^TestConversationExpansionEvidenceHTTPInspectsAdditionalOriginalPositionsAfterRestart$' -count=1` — pass,
  0.407 s. The later receipt supplies three sources and the earlier receipt
  retains one after restart; SSE and HTTP preserve exact source positions.
- `go vet ./internal/agent ./internal/eviedb ./internal/plugins ./internal/web ./cmd/evie` — pass.
- `go test ./internal/agent -run '^TestConversationExpansionWindowMeasurements$' -count=1 -v` — pass; retained thirty-sample report above.
- `git diff --check` — pass. No UI production files changed in this ticket;
  its existing eight focused component/API tests passed in #157. The final
  full UI checks and repository verification remain pending integration.

Demonstration: record an antecedent in one message and an ambiguous tentative
statement in a later message. In a fresh same-area chat, request its context.
Open Conversation excerpt after deeper recall to inspect the additional original
messages. Reopening the earlier request receipt must not add the later sources.
