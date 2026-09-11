# #160 automatic reader development: four cases pass

All four actual configured-production-reader cases passed the frozen text
checks and the predeclared manual rubric. Initial searches were automatic,
with exact evidence checked on the first actual provider request. No scripted
search selected that evidence. The final reader test returned PASS in 16.69
seconds, using six real Responses calls across four complete SQLite turns.

| Case | Actual calls | Native input / output tokens | Whole turn | Marker / manual |
|---|---:|---:|---:|---|
| vegetarian_dinner | 1 | 2217 / 72 | 2.988 s | Pass / Pass |
| uncompiled_greenhouse | 1 | 1942 / 90 | 2.831 s | Pass / Pass |
| unavailable_greenhouse | 3 | 4813 / 117, summed across calls | 6.297 s | Pass / Pass |
| compacted_greenhouse | 1 | 2338 / 74 | 3.815 s | Pass / Pass |

Model-call times were 2.975 s for dinner, 2.821 s for the uncompiled fact,
1.885/1.821/2.552 s for the unavailable sequence, and 3.805 s for the compacted
case. Exact samples and actual protocol bytes are retained in each call's
timing and wire artifacts. The six requests totaled 61053 bytes; per-call
sizes were 10966, 10238, 8840, 9238, 9650 and 12121 bytes in execution order.
All calls returned HTTP 200, completed without truncation, and reported zero
reasoning-output tokens. Total provider-reported cost was $0.0885425. These
four observations are not a latency benchmark or a percentile estimate.

Official generation metadata confirms `openai/gpt-6-astra-20260903` on the
OpenAI backend for every call. The normal production route policy was used;
no vendor or model fallback was added. Native token counts are those consumed
by the production usage contract. The API's separate standardized estimates
are retained in the allowlisted generation metadata and are not substituted
for native usage.

## Manual assessment

**Vegetarian dinner — pass.** The reader suggested chickpea and spinach curry
with rice, explicitly described it as vegetarian, and gave practical cooking
steps. It did not recommend meat/fish, invent an additional dietary restriction,
claim a new memory save, or narrate a routine lookup. The correct accepted
preference and owner source were already present on the initial request.

**Uncompiled greenhouse — pass.** The reader named saffron crocuses, preserved
that the planting was experimental rather than a settled harvest plan, and
quoted the original owner statement. It correctly cited event
`c084dc7e-cd3f-40dd-b7f4-76d367b5ae55`, without claiming an accepted-memory
update or inventing a harvest commitment, schedule or outcome.

**Unavailable greenhouse — pass.** The reader first tried accepted-memory
search and then conversation search, both with historical intent. Egress stayed
disabled, and the evidence contract passed before all three provider dispatches:
no source identity or withheld crop wording was sent. The final answer clearly
said memory retrieval was unavailable and it could not verify the crop, plan
status or original source ID. It asked for the original excerpt and metadata
without guessing the crop, inventing a citation, or claiming the owner had
never recorded the information. The two unsuccessful read attempts are retained
as actual model behavior; this case used the complete three-call allowance.

**Compacted greenhouse — pass.** The reader recovered the greenhouse experiment,
quoted the saffron-crocus statement with its uncertainty intact, and correctly
cited original event `355bf199-eb08-44ef-8dcb-66c33a325279`. The persisted
interpretation receipt records `bounded-conversation-lexical-v3`, zero included
earlier roots, 362 summary bytes and 83 query bytes. Thus the twenty debugging
roots did not supply the topic: accepted compaction continuity supplied a topic
cue for retrieval, and the independently retrieved original supplied the fact.
The summary itself contained neither the crop nor the factual source ID.
The compactor was scripted through public `Session.Compact`; this result does
not measure model-generated compaction quality.

Accepted-memory revision and Claim-count checks passed after every turn, as
did original-receipt and synthetic-episode exclusion checks. The final compacted
request retains the actual accepted compaction event identity and the exact
original evidence references.

## Preserved preflight failure and final checks

The initial model-free continuity case failed before any configuration freeze
or model generation: an unrelated numbered checksum excerpt displaced the
required greenhouse source. `preflight-evidence-failure-01.log`, its metadata,
and the old planner/harness source copies preserve that failure. The owner
fixed the planner to score earlier roots against current and bounded summary
terms, omitting zero-overlap earlier roots when summary terms are available.
The identical case then passed. Its expected source gate was not broadened.

The final freeze records the isolated #160 export, full production file hashes,
fixture hashes, compiled binary, rubric, model/context metadata and settings
before generation. It excludes concurrently developed #166 changes. Later
test-only constructor fixes in older #159 reader tests were not in this
already-completed binary; the frozen hashes remain unchanged. Reproduction
uses the owning final #160 commit and compares these hashes, recording any
difference as a new evaluation condition rather than rewriting this record.

Verification on the isolated export plus the new harness:

- `go test -c -o /tmp/evie-memory-stage5-automatic-reader-v1.test ./internal/agent`
  — PASS.
- Compiled `TestMemoryStage5AutomaticReaderEvidenceContract` — PASS, 0.33 s,
  all four complete SQLite-backed turns.
- Compiled `TestMemoryStage5AutomaticReaderPreflight` — PASS; official metadata
  discovery without model generation.
- Compiled `TestMemoryStage5AutomaticReaderEvaluation` — PASS, 16.69 s; six
  actual model calls and all frozen text checks.
- `go test -race ./internal/agent -run '^TestMemoryStage5AutomaticReaderEvidenceContract$' -count=1`
  — PASS.
- `go vet ./internal/agent` — PASS.
- `git diff --check` — PASS.

The implementation owner performs repository-wide verification. These cases
do not establish #167/#168 held-out acceptance, the full Standard toolset,
robustness across repeated hosted generations, or model quality for database
lookup failures. The unavailable case specifically covers disabled memory
egress. Any changed implementation, cases, settings or remote model metadata
requires a new frozen attempt; these raw results must remain intact.
