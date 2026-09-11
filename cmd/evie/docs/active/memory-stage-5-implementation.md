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

## #165: measured local semantic retrieval selection

This independent ticket follows its verified #157 prerequisite and is committed
before later retrieval integration. The frozen production baseline is #158 at
`f9706f8`; the reproduction procedure builds the broker against that revision.

The actual local comparison selected Ollama 0.6.3 with pinned `all-minilm:22m`,
384-dimensional normalized float32 vectors and existing SQLite persistence with
bounded brute-force cosine scoring. On the separate #165 held-out partition,
SQLite dense retrieval found 15/16 paraphrase targets versus 7/16 for lexical
search, with full query-to-evidence p95 26.392 ms. HNSW failed both its frozen
98% delivered-set overlap gate and the required 20% end-to-end speed advantage.
One held-out paraphrase remained missed by both dense configurations. No gate
or model parameter was changed after held-out results.

The decision, immutable freezes, 960 raw condition observations, process-resource
samples, model digests and limitations live in
`docs/experiments/memory-retrieval-spike/`. Runnable scripts and pinned disposable
Python dependencies live in `scripts/memory-retrieval-spike/`. No production
dependency was added. The selected configuration gates #166; these measurements
do not claim a complete provider-request budget or Stage 5 release quality.

A supplemental operational check measured SQLite rebuilding from persisted
vectors and confirmed exact vector bytes across both independent corpus builds
and the rebuilt/reopened database. It did not read questions or change the
frozen comparison. Nineteen endpoint probes exercise direct loopback/Unix
connections, denied redirects/remote destinations, cancellation, malformed
outputs and absence of fallback. The disposable server was stopped after use.

### #165 verification

- Actual development and unchanged held-out experiment — completed, 32 cases,
  three repetitions and five conditions per partition; selected SQLite passes
  its frozen gates. HNSW's failed gates are retained explicitly.
- `go test ./scripts/memory-retrieval-spike` — compilation passed; the command
  has no Go test files. `go vet ./scripts/memory-retrieval-spike` — passed.
- `python3 -m py_compile scripts/memory-retrieval-spike/*.py` — passed.
- `python3 scripts/memory-retrieval-spike/check_endpoint.py /tmp/evie-memory-stage5/165-endpoint-root-checks.json` — all 19 probes passed independently again.
- `check_sqlite_rebuild.py` — 917 independently regenerated vectors and all
  rebuilt/reopened rows byte-identical; retained sample and exact command in
  the verification artifact.
- Independent SHA256 recheck of every frozen script and fixture and both
  result-to-freeze references — passed; `git diff --check` — passed.
- Final repository verification remains pending all feature integration.

## #159: historical reads and attributed discrepancies

Search tools carry explicit `current` or `historical` intent and RFC3339
Transaction Time constraints. Accepted-memory reads additionally support Valid
Time constraints. Historical reads without a world-time constraint preserve
unknown bounds; read timestamps never become invented world dates. Conversation
search rejects unsupported Valid Time filters. Expansion inherits its trusted
anchor's historical intent and knowledge cutoff.

The evidence separates state at the original `as_known_at` read from current
lifecycle state, actual Claim transaction time, effective validity, and later
correction mode. Explicit historical reads can recover corresponding retired
intervals, marked retired now, without restoring retracted source access. The
versioned FTS projections retain eligible history; ordinary reads still apply
current retirement exclusions. Exact historical aliases use their original
lifecycle view. Free-text combinations containing retired aliases have limited
lexical coverage because the FTS document retains active aliases only.

Accepted search supplies eligible same-subject/predicate conflicts and up to two
relevant newer owner excerpts under its existing candidate/result/byte limits.
A newer excerpt is an attributed possible discrepancy, never an accepted
correction. Exact Source Link intervals are subtracted from these companion
candidates to avoid presenting the accepted source twice as new corroboration.
Conflict references survive only when both sides are actually supplied and
currently accessible. Historical source inspection preserves the original
receipt while showing later retirement or correction separately.

The first actual pinned Qwen development run passed only one of three manual
rubrics despite passing all weak text markers: retirement was mistaken for a
current preference and a quotation was attributed to the wrong source. Its
frozen inputs, wire requests, raw answers, timings and failures remain immutable
in `../fixtures/memory-stage5-reader/v1/`. Production guidance now distinguishes
original status from current status and requires exact source attribution for
quotes and assistant inference. A separately frozen v2 run evaluates that fix;
these development cases do not establish final release quality.

A full-turn regression exposed duplicated internal-object accounting after the
new temporal metadata: the Kernel result was charged as though it were a tool
message, then charged again in the actual evidence projection. The shared tool
renderer now charges the actual serialized count/status outcome including its
call ID; each provider-bound synthetic evidence message is charged separately.
The original eight-call/eight-result/12 KiB result/36 KiB cumulative/three-second
work caps remain unchanged. Complete requests still pass the context composer.
This replaces the conservative internal-result accounting described in #156.

Real-turn regressions cover half-open Valid Time endpoints, exact transaction
cutoffs, unknown validity, retired historical excerpts and expansion, original
receipts after retirement, retracted sources, same-turn future correction,
conflicting active Claims and newer owner statements, and unchanged accepted
state. Focused HTTP/UI tests preserve the saved activity annotation after a
restore and expose original-versus-current status in the existing inspector.

### #159 verification and outstanding reader gate

- `go test ./internal/agent ./internal/plugins ./internal/web ./cmd/evie` —
  pass (agent 7.932 s; plugins cached; web 5.982 s; command 12.851 s).
- `go test ./internal/agent -run '^(TestHistorical|TestOriginalMemoryReceipt|TestMemoryCorrectionBeforeDispatch|TestMemorySearch|TestConversationSearch|TestConversationExpansion|TestMemoryStage5ReaderEvidenceContract|TestContext)' -count=1`
  — pass, 4.776 s, after adding the bounded reader guide.
- `go test -race ./internal/agent -run '^(TestHistorical|TestOriginalMemoryReceipt|TestMemoryCorrectionBeforeDispatch|TestMemorySearchSuppliesConflicting)' -count=1`
  — pass, 11.971 s. The conflict pattern in this command did not match the
  actual conflict test name; its ordinary complete-turn test passed separately.
- `go vet ./internal/agent ./internal/eviedb ./internal/plugins ./internal/web ./cmd/evie`
  — pass. A prior test/vet invocation raced a test-file import edit and failed
  to import `slices`; the stable sequential reruns above passed.
- `go test ./internal/web -run '^TestMemoryEvidenceHTTPReconstructsOriginalConflictsAndNewerRelationsAfterRestart$' -count=1`
  — pass, 0.432 s, including source revocation and immutable original references.
- From `internal/web/ui`, `npx vitest run src/chat/MemoryActivity.test.tsx src/artifacts/MemoryEvidence.test.tsx src/api/memoryEvidence.test.ts`
  — 3 files, 14 tests passed; `npx tsc -b` — pass.
- Actual pinned Qwen reader v1: process PASS 56.66 s, manual 1/3 pass.
  v2: process PASS 51.92 s, manual 1/3 pass. v3: process FAIL 54.65 s,
  manual 1/3 pass. These are development attempts with unchanged cases,
  model settings and manual rubric, not held-out assessments. Exact executable
  hashes, environment commands and results are retained beside each freeze.

**Reader quality remains unresolved.** V3's explicit historical-only labels
fixed retirement wording, but Boston/Chicago incorrectly asserted supersession
and Kyoto produced an unsupported citation identifier. No gate is waived.
The deterministic retrieval implementation is committed so independent tickets
can proceed; #159 answer quality and the dependent integrated pilot/readiness
remain pending. The final PR must fold the reader fix into this owning commit.
The latest rendering version is `memory-retrieval-v2`; its reading guide and
negative-only historical labels are derived after access revalidation and share
the existing byte budget. They do not grant evidence new authority.

The failed reader run also exposed an implementation bug: re-reading the same
current excerpt replaced its previously discovered newer-statement relation.
A new complete-turn regression first failed, then verified that the relation
survives duplicate reads while dispatch still prunes inaccessible support.
Explicit historical/constrained reads retain their separate temporal view.

Demonstration: record Boston, then state Chicago without accepting a correction;
search and inspect both originals. Retire a saved preference and request its
history; it remains retired now. Restore then retract its Source Link and reopen
the old receipt: its original reference persists while source text is unavailable.
Model answers still require the documented reader-quality correction before
Stage 5 can be declared ready. Final repository verification remains pending.

### #159 reader gate resolved with the configured production reader

The separately frozen v4 attempt uses the application's actual configured
OpenRouter Responses reader, canonical `openai/gpt-6-astra-20260903`, through
its normal route and context-profile discovery. All three original development
cases pass the unchanged manual rubric and exact source checks. The complete
actual evaluation passed in 11.86 s with one request per case; native input/output
tokens were 2820/103, 2726/162 and 2360/148, and model-call times were 3.058,
4.538 and 3.463 s. These three timings are observations, not latency percentiles.
The provider reported a total cost of $0.0851825.

The prior outstanding #159 reader gate is therefore resolved for the configured
production reader. The 7B Qwen configuration's failures remain documented; no
rubric or threshold changed and no local-reader success is claimed. V4 also
contains the committed repeated-read relation fix, so it is not represented as
a controlled model-only comparison. It does not establish automatic tool
selection, the complete Standard toolset, repeated-generation robustness, or
release readiness. Raw successful wire payloads, actual backend/model identities,
frozen configuration, manual assessments, exact reproduction commands and
source hashes are in `../fixtures/memory-stage5-reader/v4/`.

The archived production snapshot plus test adapter compiled, passed metadata
preflight (0.53 s), the original deterministic evidence contract (0.26 s), the
actual production reader evaluation (11.86 s), package vet and whitespace
checks. The repeated-read regression added after v3 passed with the complete
historical/expansion set in 2.707 s. All future integration and final release
checks remain required; these results are development evidence only.

### #159 exact quotation failures in the integrated development reader

The frozen integrated development v2 run exposed five further reader failures
under the unchanged exact-byte quotation rubric. In `dev13_neighbor_clamp`,
the oracle and tool-only answers capitalized the source's lowercase `tighten`
inside a claimed original quote. In `dev14_neighbor_tentative`, the automatic
and oracle answers replaced the original period after `provisional choice`
with a comma inside quotation marks. In `dev17_conflict_newer_owner`, the
automatic answer inserted literal Markdown `**` around `Alderwick` inside the
source quotation. Their source identities and underlying meanings were correct;
their quoted bytes were not. The original requests, answers and manual failures
remain preserved with the integrated v2 artifacts. These reader failures are
separate from that run's source-auditor matching defects.

The generic reading guide now prefers paraphrases with original event citations.
Quotation marks are reserved for verbatim source text with unchanged case and
punctuation, and formatting must remain outside the quotation. Existing source
actor, original event, lifecycle and uncertainty rules remain in the same bounded
projection. No case-specific wording, answer rewriting, source changes or gate
changes are introduced.

The actual v2 answers establish the reader-test failure. The freshly frozen
integrated v3 run completed all 144 reader turns in 178 actual model calls.
Manual assessment found all 48 literal source quotations exact across the six
conditions, and each of the five v2 quotation-failure case/condition pairs above
now semantically passes. The complete production automatic-plus-deeper condition
and oracle each pass 24/24 cases; the combined development report passes all
1,077/1,077 required gates.

These are integrated known-development observations with one reader repetition,
not isolated #159 causal attribution or fresh release evidence. The integrated
run also contains later retrieval and evaluator corrections, so its result does
not measure the guide change alone. The v2 failures and isolated owning-#159
checks remain preserved. The later [#167 pilot record](memory-stage-5-integrated-pilot.md)
and [retained integrated artifacts](../fixtures/memory-stage5-integrated/runs/)
contain the frozen requests, answers, source audit, manual assessments and exact
combined verification results.

- `go test ./internal/agent -run '^(TestHistorical|TestOriginalMemoryReceipt|TestMemoryReceipt|TestMemorySearchReceipt|TestMemorySearchTurnSuppliesConflicting|TestConversationSearchKeepsUTF8|TestMemoryStage5ReaderEvidenceContract|TestAutomaticMemoryRecallRevalidatesEgressAfterCompaction|TestAutomaticMemoryRecallUsesEarlierDiscussionAndCompaction|TestMemoryInvestigationBoundsEvidenceToActualRequestHeadroom|TestMemoryInvestigationContext)' -count=1`
  — pass, 2.437 s. This covers complete-turn original sources, durable receipts,
  historical views, compaction revalidation and actual request headroom.
- `gofmt -w internal/agent/retrieval.go` and
  `git diff --check -- internal/agent/retrieval.go cmd/evie/docs/active/memory-stage-5-implementation.md`
  — pass. The fresh integrated v3 reader and combined development verification
  are recorded above; final handoff and release verification remain separate.

The same guide-only change also passes at the owning #159 boundary: an isolated
`git archive 6f9cff0` export contains exactly one changed source line, the
`ReadingGuide` string. The evidence projection fields and `go.mod`/`go.sum` match
that commit. From that export,
`go test ./internal/agent -run '^(TestHistoricalMemorySearchIncludesMarkedRetiredEvidenceWithoutRestoringAccess|TestHistoricalMemorySearchHonorsHalfOpenValidityAndExactKnowledgePins|TestHistoricalMemorySearchPreservesUnknownValidity|TestHistoricalConversationSearchPartitionsRetiredEvidenceAndKnowledgeCutoff|TestHistoricalConversationExpansionInheritsAnchorIntentAndKnowledgePin|TestOriginalMemoryReceiptRemainsInspectableAfterRetirementButNotSourceRevocation|TestMemorySearchReceiptSurvivesRestartAndRechecksSourceAccess|TestConversationSearchKeepsUTF8SpeakerAndRestartSources|TestMemorySearchTurnSuppliesConflictingClaimsAndNewerOwnerStatementWithoutOverwriting|TestMemoryStage5ReaderEvidenceContract)$' -count=1 -v`
passed all ten named complete-turn tests in 1.512 s. `gofmt -l` reported no
formatting changes. No model calls, later-stage production changes, dependency
changes, branch changes or index mutations were part of this isolated check.
