# Memory Stage 5: bounded investigation (#162)

A turn can mix automatic recall, model-chosen accepted-memory search, conversation search and original-context expansion. Reads require no Action Approval and execute serially through the existing scoped Kernel. New useful evidence can replace older held context within the same eight-item limit. A successful read does not reset any budget, and there is no fixed number of unsuccessful searches that forces an answer.

## Validity and refresh

The turn retains source-bearing evidence and bounded query plans. Every actual provider dispatch rechecks the durable session scope, current egress opt-in, source eligibility, Claim/Entity/Alias lifecycle, temporal applicability and original source locators/hashes. Request sizing and compaction previews are local; serialization alone does not rerun searches. Reused references retain their original read pins and accepted operation IDs. Exact per-request receipts are appended under the existing SQLite lease fence before dispatch; later refreshes never edit them.

A current read invalidated by correction, retirement, source restriction or temporal change refreshes its originating query within the remaining shared budget. An accepted correction chain can supply the eligible replacement while asynchronous lexical indexing catches up: at most eight links, with original/current scope, subject, predicate, operation and source checks. A retired/revoked alias cannot authorize following that entity's correction chain. An explicit later view of another result keeps its exact pins; refreshing one lost member of an earlier multi-result query does not undo that choice. Evidence deliberately replaced by a new model query is not automatically restored by its older plan.

A newly accepted or restored conflicting Claim can invalidate a current interpretation even while the old Claim remains active. Revalidation checks at most eight changed canonical peers per held Claim, including older Claims whose Claim or Source Link state changed after the held knowledge date. Current scope/source/temporal/conflict eligibility still decides whether a changed peer matters. Restoring a nonconflicting peer does not redo retrieval.

New uncompiled owner wording can also require a current query refresh. The check considers at most eight same-area owner-event suggestions after the held read and the accepted source observation time, using the existing canonical subject/predicate relevance rule. It excludes the live root, applies current context and source eligibility, subtracts retired and already represented Source Link intervals, and retains the original speaker, byte range and hash. Unrelated wording and other areas do not refresh the query. This is a bounded relevance check, not semantic classification of arbitrary new text; model-directed searches can still investigate other wording.

Either candidate-list overflow conservatively requests a fresh bounded read. These checks use the existing revalidation deadline and cumulative work allowance; they do not backfill indexes or create a separate authority path. A refresh gets a new knowledge date and carries actual conflict or attributed discrepancy support; post-pin evidence is never attached to an old receipt. An explicit `as_known_at` is retained as `as_known_at_constrained` and is not silently advanced. Historical reads preserve their requested interpretation while applying current source access. An explicit `valid_at` stays fixed even when a current read advances its knowledge date.

A query refresh considers only invalidated results it still owns. It runs at most once per plan during one dispatch, and refreshed evidence passes final revalidation again. Current results that remain valid keep their pins. New information that changes the question can be followed through the same model-directed tools; the runtime does not infer a new product intent from arbitrary tool output.

## Resource and outcome contract

The existing initial bounds remain eight Kernel search calls, eight held items, 12 KiB per result, 36 KiB cumulative memory delivery, 750 ms per agent read, 500 ms per Store snapshot and three seconds of cumulative Kernel work. Revalidation and refresh share that work allowance. Expansion still requires a held original anchor, at most two neighbors on each side, and at most 64 raw event positions. Calls are serial, so one turn never launches concurrent read generators through the agent tool loop.

Admission now charges the actual JSON-escaped synthetic memory message and every replay of this turn's retrieval tool outcome in each complete provider request. Merely executing a read or sizing a preview is not delivery. The composer separately measures the entire provider payload and records its exact SHA-256/byte count. The content-free `turn-evidence-v1` investigation receipt records attempted searches, refreshes, unchanged references reused, cumulative delivered memory bytes, measured Kernel work and distinct search outcomes; it contains no queries, source text or hidden reasoning.

The last complete supported projection closes investigation when the remaining allowance cannot fit two copies of its current memory-message/replay cost. It explicitly reports exhaustion while useful sources still fit for an answer. Later attempted searches are refused. If a provider continues, reduced projections discard unsupported graph paths, conflicts and newer-statement relations; if even the minimum control message plus prior outcomes cannot fit, the turn stops with context overflow before another dispatch or receipt. It never emits an over-budget request to obtain a final answer.

Each tool retains its individual success, empty, partial, failed, unavailable, cancelled or exhausted outcome. A later empty result cannot erase an earlier failure or supported evidence. Aggregate activity distinguishes partial availability, failed search, unavailable memory, cancellation and budget exhaustion. Partial/exhausted supported results do not establish exhaustive absence. The configured reader is responsible for a grounded answer and remaining gaps; scripted checks verify the evidence/outcome contract, not model answer quality.

## Development measurements and verification

The versioned workload contains one accepted keepsake, one uncompiled provisional experiment, twenty accepted and twenty conversation distractors, and thirty sequential fresh chats in a growing real SQLite database. Each chat performs two model-directed reads followed by two unchanged continuations: 120 actual provider requests and 90 immutable memory receipts. Every receipt must contain exactly the two original targets; searches stay at two, refreshes at zero, later receipts reuse two exact references, and recorded cumulative bytes equal independently measured provider messages. Configuration, all compiled Go input hashes and the test binary are frozen before each run. This is a scripted development diagnostic, separate from reader quality and #167/#168 release evaluation.

The retained v1 diagnostic passes in 1.03 seconds: whole-turn p50/p95 25.343/36.635 ms; cumulative Kernel-work p50/p95 11.423/14.196 ms; maximum complete request 25,396 bytes, memory delivery per request 3,985 bytes and cumulative memory delivery 11,955 bytes. Initial index refresh is 17.998 ms; across thirty refresh samples p50/p95 are 0.300/0.679 ms. This version predates the additional new-conflict revalidation check; its exact input archive and results remain available. The v2 diagnostic passes in 0.96 seconds under unchanged workload and bounds: whole-turn p50/p95 24.529/28.110 ms; Kernel-work p50/p95 11.260/12.546 ms; identical maximum request/memory/cumulative sizes. Initial index refresh is 18.416 ms; across thirty samples p50/p95 are 0.268/0.490 ms. All 90 receipts contain exactly the original two targets and match actual delivery accounting. These small local samples are diagnostics, not a statistical speedup claim. Both frozen records predate the subsequent review correction for restored support and newer owner statements; they remain unchanged. The separately frozen v3 diagnostic measures that review correction on an isolated #162 snapshot before #166. It passes in 1.07s with 30 turns, 120 requests and 90 exact receipts. Whole-turn p50/p95 are 28.078/32.094ms; Kernel-work p50/p95 are 13.114/14.185ms. Initial index refresh is 19.059ms and refresh p50/p95 are 0.290/0.508ms. Maximum request/memory/cumulative sizes remain 25,396/3,985/11,955 bytes. All original-target, no-unnecessary-refresh and accounting checks pass. These results include the extra bounded validity work without claiming a statistical comparison or model-answer quality.

Focused red/green regressions cover: same-turn approved correction before index catch-up; new conflicting accepted memory versus explicit knowledge pins; a full held set followed by useful new evidence; unchanged continuations and exact accounting; failed generator followed by empty; final supported exhaustion and a provider that keeps asking; real SQLite lease replacement and cancellation without late reads/approvals/writes/receipts; retired identity mappings; preservation of another result's explicit historical view; and graph/conflict/discrepancy support after trimming. HTTP tests exercise the real chat endpoint, live SSE, restored history and answer-associated source inspection, including exact provider request SHA/size, individual replayed outcomes and cumulative byte accounting. Focused UI tests distinguish all activity outcomes and display explicit knowledge filters.

The review correction uses complete public `Session.Send` turns, real SQLite and a scripted provider that performs authorized changes from another session after receiving the first evidence request. The initial RED (1.656s) missed an older restored Claim, an older restored Source Link, and a newly appended same-area owner statement. After the fix, `go test ./internal/agent -run '^TestMemoryInvestigation(Refreshes(RestoredConflicts|NewOwnerStatement)|ReusesEvidenceAfterNonconflicting)' -count=1` passes in 1.744s. The same cases verify exact original receipts, one refresh followed by unchanged reuse, preserved historical/knowledge/world-validity pins, nonconflicting restoration, unrelated wording and other-area exclusion. `go test -race ./internal/agent ./internal/web -run '^TestMemoryInvestigation|^TestConversationExpansionSharesSearchBudget' -count=1 -timeout=90s` passes (agent 51.485s; web 3.267s), as do `go vet ./internal/agent ./internal/eviedb` and `git diff --check`. No frozen workload, report, threshold or source archive was changed.

`go test ./internal/agent -run 'Memory|Conversation|ReferenceRecall|Compaction|Context' -count=1` passes in 14.779s on the isolated #161 + #162 export. `npm --prefix internal/web/ui run build` passes with the existing chunks-over-500-KiB warning. Nine focused UI tests across three files pass, and TypeScript passes. The separate HTTP test passes in 0.427s and `go vet ./internal/web` passes. On the final isolated export, `go test -race ./internal/agent ./internal/web -run '^TestMemoryInvestigation|^TestConversationExpansionSharesSearchBudget' -count=1` passes (agent 24.571s; web 3.171s), and `go vet ./internal/agent ./internal/eviedb ./internal/memory ./internal/web` passes. The combined thirteen-issue handoff runs `./scripts/verify-change.sh` after owning migration-fixture repairs are folded in.

For a manual demonstration, read an accepted fact, continue with an approved correction and inspect both request source records. The first keeps the old Claim operation/read pins; the next supplies the eligible replacement. Select an explicit historical view of a second result before the correction to verify it stays pinned. Add a conflicting single-value fact without correcting the first to see both current sources on the next continuation. After repeated investigation, observe supported partial/exhausted activity and inspect the original sources; unavailable or exhausted retrieval does not prove no fact exists.

The same restored-information tests also passed on the isolated pre-#166 source snapshot (1.780s); its physical conversation FTS name follows that owning stage. #166 carries the later v3 physical table rename. This preserves an independently verifiable #162 commit.

## Complete-request headroom

The integrated pilot exposed a second context limit: fitting the 36 KiB memory
allowance did not guarantee that the complete provider request fit its configured
profile. At a 24,576 working ceiling, 768 output reserve and 4,096 estimation
margin, the usable request allowance is 19,712 bytes. A pair of valid reads could
cross that bound before memory admission ran. The unchanged automatic-compaction
planner correctly rejected a request with no legal summary cut; raising the
profile or weakening its worst-case summary guarantee would hide the retrieval
integration defect.

Memory sizing now measures the complete canonical request without supplemental
memory first. When that original request fits without automatic compaction,
supplemental evidence must fit the remaining space below the same compaction
trigger and usable-input limit. The check runs for the local compaction preview
and again after final source revalidation. It preserves the original question,
earlier discussion, summary and atomic tool outcomes. Pressure already present
without retrieved memory keeps the existing compaction behavior.

Context-limited selection removes whole findings and marks the supplied view
`exhausted`. A finding that cannot fit alone is skipped before discarding smaller
eligible originals. Source text, locators and hashes are never truncated to make
room; unsupported graph paths and dangling relations are pruned with the existing
selection rule. Current source/egress checks still precede dispatch, and exact
request receipts and cumulative escaped-message accounting still occur only at
actual admission. If even the minimum limit notice cannot fit while retaining
the original request, the turn stops before another provider request or receipt.

A minimized public `Session.Send` regression uses real SQLite, one accepted
1,800-byte preference, one short conversation original and two scripted read
calls. It reproduced the exact no-legal-compaction error in 0.339s; removing
either read made the continuation fit. The two fresh-reader/original-discussion
cases also reproduced on the owning pre-#166 commit in 0.443s. A further RED
(0.340s) established that a large first finding must not discard a smaller usable
original. All three complete-turn checks then passed in 0.622s, checking explicit
exhaustion, original-context retention, complete original sources, exact
request/receipt hashes and bytes, cumulative accounting and unchanged accepted
revisions. The frozen failing pilot artifacts and configuration remain unchanged;
replaying the original pilot case against a corrected executable is a separately
identified diagnostic, not a replacement timing sample.

Final focused verification for this correction:

- `go test -race ./internal/agent ./internal/web -run '^TestMemoryInvestigation|^TestConversationExpansionSharesSearchBudget|^TestConversationExpansionCancellation|^TestMemorySearchOptOut|^TestMemorySearchReceiptSurvives|^TestMemorySearchTurnGraphSupportRevalidates|^TestMemorySearchTurnRechecksArchived|^TestAutomaticMemoryRecallRevalidatesEgressAfterCompaction|^TestAutomaticCompaction|^TestSendAutomaticallyCompacts' -count=1 -timeout=90s`
  passed (agent 63.613s; web 3.326s).
- On an isolated export of the owning #162 commit plus this correction,
  `GOPROXY=off GOSUMDB=off go test ./internal/agent -run '^TestMemoryInvestigation(BoundsEvidenceToActualRequestHeadroom|Context)' -count=1`
  passed in 0.512s, without #166/#167 code or fixtures.
- `go vet ./internal/agent ./internal/eviedb ./internal/memory ./internal/web`
  and `git diff --check` passed. No debug instrumentation remains.

The combined handoff still requires the separately versioned original-case
replay and final repository verification; these focused checks do not replace
those records or the integrated quality gates.

The exact original `dev05_clear_reference/tool_only/01` replay then passed on a
separately built corrected executable: three provider requests, no observer
boundary errors, unchanged frozen canonical inputs and accepted revisions. The
request sizes were [10323, 19184, 18710] bytes, within the unchanged profile. The Go test
passed in 0.75s (single turn 0.39s; process wall time 1.061031s). Artifacts are
retained with the #167 failed-pilot diagnostics; the diagnostic freeze hash is
`a3959adab058f5bf685415ae9ab97af69d88246af97de18ab390798467810390`.
This one-case reproduction does not establish the full pilot's quality or
resource gates.

## Restored conflicts across authorized scopes

The pre-release review found that changed-peer detection used the held Claim's
scope alone, although ordinary accepted conflict discovery spans Global, the
reader's one Context Scope and its current session. A Workspace reader holding
a Global Boston residence could therefore reuse that interpretation after an
older, conflicting Workspace Chicago Claim or its Source Link was restored.
The lifecycle request need not contain new residence wording, so the separate
new-owner-statement check did not cover the omission.

The changed-peer query now uses the same three authorized scope keys as ordinary
accepted conflict discovery. Its single `LIMIT 9` still means at most eight
changed peers across the whole union and one overflow sentinel, not eight per
scope. Exact current Claim, source, validity and conflict checks are unchanged.
Explicit knowledge and historical views remain pinned; current views may
advance their knowledge date while preserving an explicit world-validity date.
No new source access or write authority is introduced.

Two complete public `Session.Send` regressions with real SQLite and a scripted
provider first reproduced the missing Workspace Claim and Source Link. The RED
command `go test ./internal/agent -run '^TestMemoryInvestigationRefreshesRestoredConflictsAcrossAuthorizedScopes$' -count=1 -v`
failed both current-view cases (package 0.539s). The same command passed after the
four-line production correction (0.512s). The final matrix adds explicit
knowledge, historical and world-validity views, exact original receipts, one
refresh followed by unchanged reuse, and nine restored conflicting other-project
peers. Those excluded peers neither leak through any complete provider request
nor trigger a needless overflow refresh.

Final focused verification:

- `go test ./internal/agent -run '^TestMemoryInvestigation(RefreshesRestoredConflictsAcrossAuthorizedScopes|IgnoresRestoredConflictsOutsideAuthorizedScopes)$' -count=1 -v`
  passed all ten subcases (1.782s).
- `go test ./internal/agent -run '^TestMemoryInvestigation|^TestMemorySearchOptOut|^TestMemorySearchReceiptSurvives|^TestMemorySearchTurnGraphSupportRevalidates|^TestMemorySearchTurnRechecksArchived' -count=1 -timeout=90s`
  passed (5.282s).
- On an isolated archive of owning #162 commit `5808c54`, with only this
  production correction and the new test file,
  `GOPROXY=off GOSUMDB=off go test ./internal/agent -run '^TestMemoryInvestigation(RefreshesRestoredConflictsAcrossAuthorizedScopes|RefreshesRestoredConflictsWithoutAdvancingPinnedViews|IgnoresRestoredConflictsOutsideAuthorizedScopes|RefreshesNewOwnerStatementWithoutAdvancingPinnedViews|ReusesEvidenceAfterNonconflictingPeerRestoration|RefreshesNewConflictButPreservesExplicitKnowledgePin)$' -count=1 -v`
  passed (3.215s). No #166 or #167 fixture or helper is required.
- `go vet ./internal/agent ./internal/eviedb`, `gofmt -l` for both changed Go
  files, and `git diff --check` passed.

The original failed and successful local traces remain under
`/tmp/evie-memory-stage5/162-cross-scope-*.log`. No frozen evaluation input,
source, answer, scorer or gate was changed by this fix. Existing development
model results describe their original executable; a newly frozen full
version-4 development comparison and final repository verification remain
pending. These deterministic results do not claim model-answer quality or
held-out completion.
