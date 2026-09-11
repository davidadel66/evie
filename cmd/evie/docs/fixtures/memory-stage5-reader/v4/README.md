# #159 configured production reader development evaluation, v4

This attempt evaluates the application's configured default reader through its
normal `openrouter.NewClient` and Responses dispatch. The same three synthetic
development cases, initial scripted searches, exact evidence assertions,
text-marker checks and manual rubric from `../v1/README.md` remain unchanged.
The local 7B Qwen failures in v1–v3 remain model-specific limitations; no check
is removed or weakened. This attempt is not #167/#168 held-out evaluation.

## Fixed configuration before generation

The implementation comes from `git archive 4ec5429`, with only the new test-only
`retrieval_production_reader_evaluation_test.go` adapter copied into the archive.
This isolates the run from concurrent later-issue work. The original reader
fixture and scenario definitions are already present in that commit. The
binary, adapter, original fixture, baseline commit, metadata and rubric are
pinned before generation in `freeze.json` and `run-metadata.json`.

The model is the application's default `openai/gpt-6-astra`, advertised as
canonical `openai/gpt-6-astra-20260903`. Availability and endpoint metadata were
retrieved from the official API. `preflight-context-profile.json` records the
actual production client's discovered context contract with working context
24576 tokens and output reserve 768. The hard window follows the smallest
eligible route limit, including its maximum prompt and output allowance; the
same discovered profile must hold at evaluation time. No context-profile
fallback is permitted.

Requests use the ordinary production Responses encoding: reasoning effort low,
concise public reasoning summaries, stream=true, store=false, and
max_output_tokens=768. Temperature is unsupported in this production encoding
and is not supplied; no seed is injected. Normal routing uses
provider.require_parameters=true. The official model/endpoint metadata and the
routing policy are pinned as a snapshot, but the production API does not expose
model weight digests or pin one backend vendor. This evaluation does not claim
immutable hosted weights or deterministic generations. Returned model/response
identity and routing details, when provided, are retained in the raw response.

Each case runs a complete real SQLite `Session.Send`. The initial one/two
memory reads are scripted, then the actual reader answers or selects subsequent
reads. At most three model calls per case and 120 seconds per model call are
allowed. Only `memory_search` and `memory_search_conversations` are available;
this is not the full Standard preset. Accepted memory revisions and Claim
counts must remain unchanged. An incomplete provider response fails through
the normal production client. For this reasoning model the 768-token output
limit includes the provider's output accounting, which can include reasoning;
truncation will not be relabeled as success.

## Capture and assessment

The test temporarily wraps the process's HTTP transport to capture the exact
production request body and response bytes consumed by the production client.
It changes neither the request encoding nor the returned response stream.
The transport permits only direct HTTPS to the official OpenRouter host, uses
no proxy, and rejects redirect follow-up requests. The normal production
client still resolves context metadata and dispatches Responses requests.
No alternate model or endpoint fallback is installed.

Authorization headers and credentials are never recorded. Non-success HTTP
response bodies are withheld because they can echo credentials; their status
is retained. An exact credential match withholds any artifact. Successful raw
Responses streams may contain opaque encrypted continuation and public summary
items; these are retained only as synthetic test transport artifacts, not
retrieved memory or owner telemetry. Assessment uses the complete public answer
and supplied evidence, not hidden reasoning. Response-stream trailers after the
production completion boundary are not required or reconstructed.

The original text markers remain regression checks, not an entailment judge.
The original manual rubric is applied independently to the complete answer and
exact source events. Results must report marker failures, missing citations,
unsupported current claims, misattribution and incomplete generation without
softening the criteria. Three development cases cannot establish wider reader
quality or automatic search-planning quality.

## Reproduction

Create a new versioned artifact directory for a fresh run, preserving this
attempt. Archive baseline `4ec5429`, copy the test-only adapter from this change,
then compile that archive:

```sh
go test -c -o /tmp/evie-memory-stage5-reader-v4.test ./internal/agent
```

From the archive's `internal/agent` directory, with the existing API credential
in the environment, run preflight without generation:

```sh
EVIE_RUN_PRODUCTION_READER_PREFLIGHT=1 \
EVIE_MEMORY_READER_ARTIFACTS=/absolute/new/artifact/directory \
/tmp/evie-memory-stage5-reader-v4.test \
  -test.run '^TestMemoryStage5(ProductionReaderPreflight|ReaderEvidenceContract)$' \
  -test.v -test.count=1
```

Freeze the new binary, source, metadata, context profile, settings and unchanged
rubric before using `EVIE_RUN_PRODUCTION_READER_EVAL=1` to run
`^TestMemoryStage5ProductionReaderEvaluation$`. Artifact writes use exclusive
creation and refuse overwriting an earlier attempt. Both production-reader
tests skip during normal CI unless explicitly enabled. No local Ollama server
is loaded, stopped or altered by this attempt.
