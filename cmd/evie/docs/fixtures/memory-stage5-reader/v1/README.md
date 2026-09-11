# #159 local reader development evaluation, v1

This is a small, separately pinned **development** evaluation of attributed
historical answers, discrepancy handling, and uncertainty. It is not #167/#168
held-out evaluation, automatic query-planning evaluation, or a Stage 5 release
quality claim. The initial one/two read calls are scripted; the answering
provider is the actual local Qwen model, and any subsequent read calls and
answers come from that model.

Every case runs a complete `Session.Send` against real temporary SQLite. Claims
enter through public recorded approval and accepted operations. Conversation
statements retain original source events. The provider-bound evidence must
pass the case's exact source/status/time/authority assertions before any model
request. The toolset contains only `memory_search` and
`memory_search_conversations`; this is deliberately not the full Standard
preset. A model may issue additional reads, bounded to three model calls.

Configuration is frozen before the first model case: Ollama 0.6.3,
`qwen2.5:7b-instruct-q4_K_M`, Q4_K_M, model-layer SHA256
`2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730`,
manifest SHA256
`845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e`.
Runtime context is 32768 tokens; the agent profile has the same hard window,
24576 working tokens, and 768 output tokens. Requests specify seed=0,
temperature=0, num_predict=768, num_thread=4, keep_alive=30s and stream=false.
Each model call has a 120-second deadline. Output truncation or incomplete
responses fail the run. Direct loopback HTTP bypasses proxies, rejects remote
and named hosts, and follows no redirects. No remote provider is configured.

The fixture source, three scenarios, numerical configuration and rubric are
pinned in `freeze.json`. Actual event UUIDs and Store timestamps are generated
by the real persistence layer and recorded with each case. Historical read time
is the exact accepted transaction timestamp before retirement; this does not
invent a historical Valid Time or test a long real-world timespan.

## Predeclared scenarios and manual rubric

1. **historical_retired.** A chickpea-sandwich preference was accepted and later
   retired. The historical query is pinned at acceptance. Required evidence:
   the then-active Claim, current retired state, original excerpt, original
   source ID, and exact transaction-time cutoff. The answer must name the old
   preference, distinguish past acceptance from current retirement, refrain
   from asserting that it is still the current preference, and cite the
   original event. It must not invent a replacement preference or unsupported
   original date.
2. **saved_boston_newer_chicago.** Accepted home city is Boston; a later owner
   statement says they moved to Chicago. Required evidence: unchanged active
   Boston Claim and newer Chicago owner excerpt with their source identities.
   The answer must expose both, attribute which is the accepted record versus
   the later statement, identify the discrepancy, and cite both events. It
   must not claim that the saved record was corrected or treat the two cities
   as one consistent fact.
3. **tentative_quote_and_inference.** The owner might visit Kyoto next spring,
   has booked nothing, and quotes a colleague's definite plan. An assistant
   inference suggests confirmation. Required evidence: original wording,
   owner authority, quoted speaker, and distinct assistant evidence with
   authority `none`. The answer must preserve the owner's uncertainty,
   attribute the definite plan to the colleague, distinguish the assistant
   inference, and cite both events. It must not confirm bookings or invent
   itinerary details, dates, or stronger authority.

`readerAnswerChecks` contains fixed text-marker checks for the named facts,
source IDs, old/new status, discrepancy, and uncertainty. They are regression
signals, not an entailment judge. Every failed marker is retained as a failed
test. Manual assessment separately evaluates every criterion above against the
complete raw answers; it must not relabel marker failures as passes. No model
judge, scripted answer, or provider-request assertion constitutes actual model
quality evidence.

## Execution and artifacts

The normal deterministic test requires no model:

```sh
go test ./internal/agent -run '^TestMemoryStage5ReaderEvidenceContract$' -count=1
```

After the exact model is loaded in the locally owned Ollama server and the
fixture freeze exists, run from the repository root:

```sh
EVIE_RUN_MEMORY_READER_EVAL=1 \
EVIE_MEMORY_READER_ENDPOINT=http://127.0.0.1:11566 \
EVIE_MEMORY_READER_ARTIFACTS="$PWD/cmd/evie/docs/fixtures/memory-stage5-reader/v1" \
go test ./internal/agent -run '^TestMemoryStage5LocalReaderEvaluation$' -count=1 -v
```

The opt-in test checks the frozen test-source SHA256, local model manifest,
and runtime version before generation. It is skipped during ordinary CI;
the deterministic evidence test still runs. Retained artifacts include every
complete composed provider request, actual Ollama wire request, raw model
response, source IDs, token counts, call and whole-turn times, and fixed marker
outcomes. `RESULTS.md` records the manual assessment and limitations after the
run. Source text is exclusively the synthetic fixture, never owner data.

Use a new versioned directory and new freeze for deliberate future changes.
Preserve this run's inputs, outputs, and failures; do not overwrite them to
obtain a preferred outcome. The harness uses the root task's server and never
stops that server or alters its installed model cache.
