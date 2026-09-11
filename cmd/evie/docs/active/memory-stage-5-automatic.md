# Automatic recall on ordinary requests (#160)

An ordinary new user message now uses the existing scoped retrieval boundary
before the first conversational provider request. This implements
[#160](https://github.com/davidadel66/evie/issues/160) and the Automatic Recall,
context composition, egress and original-request contracts in the adjacent
Stage 5 specification. A reader can receive an accepted dinner preference or
an attributed uncompiled greenhouse statement without first issuing a search.

## Interpretation and source selection

`bounded-conversation-lexical-v3` constructs a bounded query from the current
user root, relevant earlier user roots and validated persisted compaction
continuity. The interpretation limits are:

| Input | Limit |
| --- | ---: |
| Current root | 512 UTF-8 bytes, 16 content terms |
| Earlier roots inspected | Last 16 before the current root |
| Earlier root content | 384 bytes each |
| Earlier roots included | 2, at most 5 terms each |
| Persisted summary | 512 bytes, 6 terms |
| Combined query | 32 distinct terms, 1,024 UTF-8 bytes |
| Automatic searches | 1 accepted-memory and 1 conversation search |
| Automatic results per search | 2 records, 6 KiB |

Function-word removal is independent of personal preferences or entity names.
Earlier roots rank by lexical overlap with the current request and bounded
continuity; older roots break ties inside the sixteen-root window. When a
validated summary provides continuity, unrelated zero-overlap earlier roots
are omitted. Without a summary, the bounded earlier-root fallback can recover
a prior subject. These lexical choices are deterministic interpretation hints;
reference-resolution evaluation belongs to #161.

Input clipping preserves UTF-8. Secret detection examines the entire field
before clipping. The plan records only version, outcome and measured byte/root
counts in the request receipt. It does not persist its query, summary text or
hidden reasoning as a new retrieval record. Summaries guide the query; every
returned source still resolves to an eligible original Claim source or
conversation event. Retrieval does not extract or accept a Claim.

Automatic conversation selection excludes exact byte-for-byte copies of the
active user request before candidate limits. The Kernel derives that request
from the bound durable session. The restriction also applies to newer owner
statements accompanying accepted Claims. It prevents repeated requests from
crowding out their original answer evidence. This is a trusted caller option,
not a model argument; explicit searches can still retrieve prior questions.
It does not classify every question as useless or claim to recognize paraphrases
of the current request.

## Dispatch, accounting and availability

The two automatic reads use the existing turn ledger: eight total searches,
eight held results, 12 KiB per targeted result, 36 KiB of cumulative memory
delivery, a 750 ms per-call deadline and three seconds of turn retrieval work.
Automatic reads leave capacity for deeper model-directed searches. The
complete escaped memory message is charged, and the context composer measures
the complete provider request including stable instructions and tool schemas.
The synthetic `EVIE_MEMORY_DATA` message appears immediately before the current
user request; it is never appended as an independent conversation episode.

A sizing preview may precede automatic compaction. Compaction receives original
durable conversation, not the synthetic memory block. After compaction, the
turn revalidates current source/lifecycle eligibility and remote-memory opt-in,
charges the final projection, composes the actual request and commits its exact
source receipt before dispatch. Every continuation repeats that validation.
Retraction, retirement or opt-out during the turn cannot be bypassed by the
earlier automatic selection. Removing all held evidence reports unavailable;
removing some reports partial. Lookup failure is distinct from successful
absence, and mixed incomplete searches report partial.

Automatic recall uses only reads available in the resolved memory toolset and
the existing `EVIE_REMOTE_MEMORY=on` egress choice. A fresh opted-out memory
composition reports unavailable without a lookup or evidence identifiers.
Omitting the Memory plugin entirely creates no automatic memory activity.
`WithAutomaticMemoryRecall(false)` is a construction-time host/testing policy
for the tool-only and no-recall comparison conditions; it does not change scope,
capabilities, tool definitions or the remote-memory opt-in. Earlier manual-search
fixtures explicitly select that policy to preserve their intended test condition.

## Deterministic checks and development measurements

Complete turns with real SQLite cover the first ordinary request, fresh-chat
accepted preference and uncompiled fact, distractors, empty results, opt-out,
Global/General/another Workspace/two-project scope boundaries, retirement before
a continuation, and opt-out inside actual automatic compaction. A separate
closed SQLite read connection causes a real lookup failure while the durable
turn connection remains healthy. Earlier-discussion and public `Session.Compact`
cases recover an original after twenty topic changes without citing the summary.
The focused HTTP case inspects the first ordinary answer and compares its exact
request bytes/hash and original source with the actual provider request.

The first growing-corpus diagnostic failed: only 13/30 cases found the exact
original target or correctly returned empty. Repeated prior requests displaced
originals. Its unchanged raw report and fixture remain in
`../fixtures/memory-stage5-automatic/v1/`. After the exact-copy filter, the frozen
v2 run passed 30/30 with no non-target evidence. A separate reader preflight then
found debugging roots displacing compaction continuity; that failed preflight is
retained in the automatic-reader artifacts. Planner v3 adds the summary-aware
relevance check, and the same thirty-case measurement fixture remains unchanged.

The final [v3 freeze](../fixtures/memory-stage5-automatic/v3/freeze.json),
[samples](../fixtures/memory-stage5-automatic/v3/report.json) and
[verification](../fixtures/memory-stage5-automatic/v3/verification.json) record
the isolated exported source tree and separately compiled executable before
execution. On the declared Apple M3 Pro/macOS host, v3 passed 30/30 cases with
zero non-target evidence. Whole-turn p50 was 3.890 ms and p95 was 5.676 ms;
maximum complete request size was 22,893 bytes and the largest serialized
memory message was 2,335 bytes. Initial index refresh took 20.091 ms. These are
scripted-provider development measurements with one accepted preference, one
uncompiled fact, forty distractors and a growing thirty-request corpus; they
exclude model/network latency and do not establish held-out release readiness.

To reproduce a measurement, materialize only the files named in its freeze from
the owning #160 commit, use `git apply --unidiff-zero` with that version's
`reproduce-source.patch` to restore
its exact earlier planner/test sources and artifact attributes, verify their hashes, compile the
recorded Go test command, and run the recorded executable command. Later
test-harness additions are not part of that earlier executable. Preserve
previous results and use a new output directory. The JSON report's version
identifies its unchanged report schema;
the freeze and interpretation versions identify the measured configuration.

Focused exported-tree checks passed:

- `go test ./internal/agent -run '^TestAutomaticMemoryRecall' -count=1`: PASS, 1.254s after planner v3.
- `go test ./internal/web -run 'TestAutomaticMemoryEvidenceHTTP|TestMemoryReceiptHTTP|TestMemoryEvidenceHTTP|TestHistorical' -count=1`: PASS, 1.105s.
- `go vet ./internal/agent ./internal/eviedb ./internal/memory`: PASS.
- `npm --prefix internal/web/ui run build`: PASS, existing large-chunk warning.

The broader memory/context regression run exposed an old tool-only reader
fixture inheriting the new default automatic recall. Its constructors now
explicitly disable automatic recall, preserving the original comparison.
Model-backed case results and final regression results are recorded below.
The final PR still requires the repository-wide verification script.

The separate [production-reader results](../fixtures/memory-stage5-automatic-reader/v1/RESULTS.md)
passed all four predeclared manual cases: a natural vegetarian dinner answer,
an attributed uncompiled greenhouse answer preserving its uncertainty, honest
unavailability without withheld details, and recovery of the exact original
after persisted compaction. They used the configured production reader through
the actual OpenRouter Responses transport, with no scripted initial search.
The observed model was `openai/gpt-6-astra-20260903`, provider OpenAI, low reasoning
and a 768-token output cap. Six actual calls completed in the 16.69s test;
whole turns were 2.988s / 2.831s / 6.297s / 3.815s respectively. The unavailable
case used three model calls, including two model-chosen reads that remained
unavailable. Recorded native usage totaled 11,310 input and 353 output tokens;
the reported cost was $0.0885425. Exact public answers, original IDs, wire bytes,
settings and manual judgments remain in the artifacts. The compactor itself
was scripted; these cases do not assess model-generated summary quality or
claim a held-out release pass. Focused model-free reader race checks passed
(7.190s). Raw response streams retain exact bytes under a binary Git attribute;
normalized JSON remains reviewable text.

After the explicit legacy tool-only fixture correction,
`go test ./internal/agent -run 'Memory|Compaction|Context' -count=1` passed
(6.735s), and `go vet ./internal/agent ./internal/eviedb ./internal/memory` passed.

For a manual demonstration, save a dinner preference, open a fresh chat and ask
for dinner. Inspect the answer's memory activity to see its first request and
original source. Repeat with an uncompiled conversation fact, then disable
remote memory and ask again. The request receipt distinguishes supplied evidence
from unavailable memory; it does not claim that supplied evidence was cited.
