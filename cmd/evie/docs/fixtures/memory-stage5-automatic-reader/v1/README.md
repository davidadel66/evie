# #160 automatic recall: configured production reader development cases

These four synthetic development cases run complete real SQLite `Session.Send`
turns with the application's default automatic recall enabled. There is no
scripted initial search: the first actual reader request must already carry
the exact eligible evidence or the explicit unavailable result. Initial
retrieval is the production deterministic lexical/exact implementation. The
actual production reader may answer or choose subsequent memory reads, bounded
to three model calls per case. This is not #167/#168 held-out evaluation.

## Cases and predeclared manual rubric

1. **vegetarian_dinner.** Public approved memory records a preference for
   vegetarian dinners. A fresh chat asks only “Suggest dinner for me.” The
   initial provider request must include the accepted Claim and exact original
   owner source. The answer must give a useful vegetarian dinner suggestion
   consistent with that preference. It must not recommend meat or fish as the
   requested dinner, invent additional dietary restrictions, claim a new save,
   or narrate a routine memory lookup. Naming the dietary constraint naturally
   is acceptable; a source UUID is not required for routine preference use.
2. **uncompiled_greenhouse.** An earlier owner conversation says they planted
   saffron crocuses in the greenhouse and that it remains an experiment, not a
   harvest plan. No accepted Claim is created. A fresh chat asks what was
   planted and how settled the plan was, requesting the original event ID. The
   initial request must contain the precise original owner excerpt and source,
   with no invented acceptance or irrelevant bicycle distractor. The answer
   must name saffron crocuses, preserve the experimental status, distinguish
   recorded conversation from an accepted memory update, and cite the correct
   original event. It must not invent a harvest commitment, schedule, successful
   yield or stronger certainty.
3. **unavailable_greenhouse.** The same earlier greenhouse evidence exists,
   but memory egress is disabled after composing the read-only toolset and
   before the fresh turn. The initial request must say unavailable and contain
   no source identities or withheld crop wording. The question needs that
   missing evidence. The answer must explain that the relevant memory cannot
   be accessed or that available evidence is insufficient, rather than claim
   a successful empty search. It must not invent the crop, fabricate a source,
   or claim the owner never recorded the information. Asking the owner to
   provide the missing detail is acceptable. This case evaluates actual
   opt-out unavailability, not a simulated database failure.
4. **compacted_greenhouse.** The original greenhouse evidence remains a
   separate uncompiled owner source. The reader session begins with only a
   greenhouse topic cue and then records twenty unrelated debugging turns.
   Public `Session.Compact` persists a valid rolling summary produced by a
   fixed scripted compactor. Its early content preserves only the greenhouse
   topic, without the crop, source UUID or factual answer. A new Session then
   receives “What was it again? Include its original source event ID.” The
   original topic is outside the sixteen-root interpretation window. The
   first reader request must use the persisted summary for interpretation and
   independently retrieve the exact original greenhouse evidence. A retrieved
   original topic-cue event is also allowed, but is not the crop's source. The
   same greenhouse answer and exact-source rubric applies. The persisted
   compaction event and original evidence receipt must remain identifiable.
   This evaluates an actual production reader after compaction, **not the
   quality of model-generated compaction**; the compactor is scripted.

For every case, accepted-memory revisions and Claim counts remain unchanged.
Synthetic `EVIE_MEMORY_DATA` must not become a durable conversation episode.
Every provider dispatch must retain current eligibility and the correct
source/authority state. The first request must place the bounded memory block
immediately before the actual current user message. Deterministic fixture
answers establish these input contracts only and are never model-quality
evidence.

## Frozen reader and execution contract

The configured production reader is `openai/gpt-6-astra` through the normal
OpenRouter Responses client. As with #159 v4, official metadata discovery
determines the hard context window; working context is 24576 tokens, output
reserve is 768, reasoning effort is low with concise public summaries, and
the request has stream=true, store=false and provider.require_parameters=true.
Temperature and seed are not injected. Each model call has a 120-second limit,
and an incomplete response fails through the production client. Only
`memory_search` and `memory_search_conversations` are offered. Extra model
calls and tool choices, if any, remain part of the retained result.

Model/route/context metadata, these cases and rubric, all relevant fixture
sources, the compiled binary and exact production Go files are frozen before
generation. The implementation is copied from the implementation owner's
isolated #160 export, which excludes concurrent later work. The normal hosted
route policy is preserved; backend vendor selection and weight digests are
not immutable or exposed by this API. Actual generation identity and vendor
metadata are recorded when available. No alternate reader or model fallback
is installed.

The capture reuses the test-only #159 production transport: exact normal wire
request bodies, successful response-stream bytes consumed by the client,
normalized public answers, native token accounting and measured timings are
retained. Authorization headers and credentials are never recorded. Failed
HTTP bodies are withheld because they may echo credentials; the HTTP status
is retained. Captures are exclusively synthetic fixture data. Raw opaque
transport continuation is not treated as retrieved memory or model reasoning
evidence. Manual assessment uses public answers and exact supplied sources.

`automaticReaderAnswerChecks` contains fixed textual regression markers. These
can miss semantic errors and cannot prove citation correctness; the separate
manual rubric above remains required. Marker failures, manual failures,
missing sources and incomplete output stay failures. Every actual attempt
gets a new directory and freeze; existing artifacts refuse overwrites.

## Running the checks

Model-free input contract:

```sh
go test ./internal/agent -run '^TestMemoryStage5AutomaticReaderEvidenceContract$' -count=1
```

Compile the isolated source before preflight or model generation. From its
`internal/agent` directory, use a new absolute artifact directory and the
existing credential in the environment:

```sh
EVIE_RUN_AUTOMATIC_READER_PREFLIGHT=1 \
EVIE_MEMORY_READER_ARTIFACTS=/absolute/new/artifact/directory \
/absolute/compiled-agent.test \
  -test.run '^TestMemoryStage5AutomaticReaderPreflight$' -test.v -test.count=1
```

After freezing source, configuration, discovered profile, binary and rubric,
use `EVIE_RUN_AUTOMATIC_READER_EVAL=1` with
`^TestMemoryStage5AutomaticReaderEvaluation$`. The opt-in tests skip during
normal CI. Reproduction after the final history rewrite selects the owning
#160 commit from the PR mapping and validates the recorded production and
fixture hashes before compiling. Different source or hosted metadata requires
a new recorded evaluation condition; do not rewrite existing hashes.
