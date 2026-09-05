# Standalone local extraction spike

This command implements the experiment in [ticket #135](https://github.com/davidadel66/evie/issues/135).
It reads the versioned synthetic corpus in
[`cmd/evie/docs/fixtures/memory-stage-4-spike/v1`](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/),
calls an already available local Ollama endpoint, and writes bounded experiment
reports. It never opens Evie's database, schedules a compiler, or applies memory.

The frozen `review-packet.md` preserves the packet David reviewed, including its
then-provisional status. `annotation-record.json` records his subsequent approval
against the exact file hashes. `*.gold.json` are physically separate from
source-only `development.json` and `pilot.json`. The inference path serializes
only each selected window's `input`; it cannot send evaluator labels, later
events, or future answers. All content is synthetic. The sentinel detector is
only a deterministic exclusion fixture, not a production secret detector.

## Reproduce a bounded run

From the repository root, after verifying the installed runtime and cached model
against `runtime-manifest.json` and `runtime-api-metadata.json`:

```sh
go build -o /tmp/evie-extractor-spike ./scripts/memory-extractor-spike
/tmp/evie-extractor-spike -only N01-a -repetitions 1 -output /tmp/spike-smoke.json
```

The runtime must already be listening on a literal loopback HTTP origin. This
command does not start, download, upgrade, or replace a runtime/model. Its
`mistral:latest` request name is mutable: verify the actual manifest and every
artifact before interpreting results. The experiment record pins actual hashes;
the command itself is not a model-registry authenticity boundary.

The recorded comparison uses `-only
N01-a,N01-b,N02-a,N02-b,N03-b,N04-b,N05-b,N06-a,N08-b,N09-a`, `-repetitions 2`,
and separately `-mode schema` and `-mode json`. Both use the same prompt/schema,
temperature zero, seeds 17/18, context 4096, at most 768 output tokens, and a
60-second request timeout. Different seeds at temperature zero test
reproducibility; they do not promise diverse samples. Each output path must be
new. Exact source/corpus/prompt/schema/request hashes and request counts are
recorded. `experiment-manifest.json` also pins the actual inference binary.

The corrected comparison uses the same ten cases, `-repetitions 1`, and both
formats separately with these additional flags:

```sh
-prompt cmd/evie/docs/fixtures/memory-stage-4-spike/v1/prompt-v2.txt \
-budgets cmd/evie/docs/fixtures/memory-stage-4-spike/v1/token-budgets-v2.json \
-context 8192
```

The common corrected prompt includes explicit field definitions and the complete
unchanged minified schema. The original JSON arm did not receive the schema in
its prompt, which confounded that comparison. Context also changes, so the new
pass measures the corrected configurations rather than an isolated prompt effect.
`experiment-manifest-v2.json` freezes its binary, Go sources, settings and inputs.
Reports record executable, budget and validation identities. Original inference
reports remain immutable; `-revalidate ORIGINAL -output NEW` repeats only local
validation/scoring, preserves raw text/requests/latencies, and records both
original report and new validation identity. Use the original prompt/budget
when revalidating original reports; supply the corrected flags for new reports.

Requests are sequential. Redirects, proxies, DNS hostnames, nonloopback origins,
credentials, and endpoint paths/queries are rejected. Source input is bounded
by the evidence contract; the serialized request is additionally capped at
32 KiB, the HTTP response at 64 KiB, model text at 16 KiB, and the report at
4 MiB. The executable preflights **all** selected windows/repetitions before sending any
inference. The original configuration requires exact recorded-request budgets
in `token-budgets.json`. Unknown request hashes fail before dispatch. The
corrected configuration uses `token-budgets-v2.json`, the exact pinned tokenizer
and template, and the source-derived full-rendering bound described in
`input-budget-proof.md`. The request byte cap alone never establishes context
fit. Unverified runtime/model/template inputs cannot use that bound.

An incomplete response, disconnect, or timeout leaves request capacity unknown
and stops the experiment immediately. No second inference request tests whether
the first one stopped. A completed request-specific `done` response establishes
release. Where this runtime provides no proven request-specific cancellation
acknowledgment, a verified controlled restart of the experiment-owned server is
the recovery mechanism. A process-wide `/api/ps` model listing cannot prove that
a particular request finished. This spike has no shared production capacity
ledger; that remains the separate compiler implementation.

## Source checks and scoring

The standalone checks validate the frozen source field's event identity,
session/scope, UTF-8 byte coordinates/hash, authority category, synthetic secret
exclusion, evidence bounds, and the admitted `get_time` completion/call lineage.
Closed output shape and exact nominated source/context coordinates are checked
separately. At least one cited supporting event must be new. These checks are
not the production Kernel, a complete event-store validator, semantic entailment,
or an acceptance decision. Source closure and the window selection are frozen
fixture inputs; this executable does not implement durable reconciliation.

Score recorded outputs without making a model request:

```sh
go run ./scripts/memory-extractor-spike \
  -score /tmp/schema.json,/tmp/json.json \
  -gold cmd/evie/docs/fixtures/memory-stage-4-spike/v1/development.gold.json \
  -output /tmp/spike-score.json
```

Scoring verifies the actual human approval and corpus/gold hashes. Exact listed
meanings with reviewed source/context manifests can receive credit. Every
unlisted meaning remains unadjudicated, including apparently plausible synonyms.
A source match never proves that a different proposition follows. The optional
`-adjudications` file must record separate human-reviewed output judgments;
`gold_matches` explicitly maps a supported equivalent to zero-based required
gold indexes when recall credit is justified. The loader binds those indexes to
the exact approved gold SHA-256 and verifies packet/source-score evidence hashes;
the score records the applied adjudication file SHA-256. Approval of raw semantic
credit for a repair-needed proposal does not grant typed/Predicate agreement.

Raw JSON-decodable proposals and proposals retained by the declared checks have
separate panels. Unparseable output remains a visible failure and an attempted
required-recall case. Planned but unexecuted requests are listed separately.
Precision bounds express unresolved human judgments, not statistical confidence
intervals. Identity, temporal, source-attribution, typed-meaning, polarity, and
Predicate errors come from recorded output adjudication rather than invented
gold labels. No acceptance-rate or foreground-overhead claim follows from these
standalone results.

`measure.py` samples only the supplied experiment-owned Ollama PID and its
descendants, plus the standalone command and its descendants. Use it around the
same command to record server/runner RSS separately, with system-wide swap:

```sh
python3 scripts/memory-extractor-spike/measure.py \
  --server-pid OWNED_SERVER_PID --output /tmp/spike-resources.json \
  -- /tmp/evie-extractor-spike -only N01-a -repetitions 1 -output /tmp/spike.json
```

Process RSS does not reliably include all Metal/unified GPU allocations, and
system-wide swap cannot be attributed to this experiment alone. The later
integrated pilot must measure actual Evie foreground behavior and owner review
effort. The final-holdout protocol in `holdout-custody.md` creates no final cases
and grants no permission to expose them during tuning.

## Qwen alternate configuration and measured outcome

After the initial and corrected Mistral failures, the bounded alternate used
`qwen2.5:7b-instruct-q4_K_M` on the same installed Ollama 0.6.3. Verify the actual
model/template/system/runtime artifacts against `v1/qwen/runtime-manifest.json`.
The versioned `inspect_qwen_gguf.py` command streams the exact acquired model
hash and checks bounded metadata, the full byte alphabet and BPE merge closure.
Its output must be new; compare invariant evidence fields with the frozen proof
rather than replacing that proof. This is model-specific experimental evidence.

```sh
python3 scripts/memory-extractor-spike/inspect_qwen_gguf.py \
  --model-file /PATH/TO/sha256-2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730 \
  --output /tmp/qwen-tokenizer-check.json
```

The owned server used `OLLAMA_NEW_ENGINE=false`, explicit loopback binding,
one loaded model and one inference request. The measured command adds these
flags to the same selected ten windows:

```sh
-model qwen2.5:7b-instruct-q4_K_M \
-prompt cmd/evie/docs/fixtures/memory-stage-4-spike/v1/prompt-v2.txt \
-budgets cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/token-budgets.json \
-context 8192 -mode schema -repetitions 2 -stop-on-failure
```

`-stop-on-failure` records the first failed response and the unexecuted plan;
it never hides a failure or substitutes unrun cases into quality denominators.
Qwen completed 20/20 requests; four exact required matches out of 12 opportunities
and unresolved novel outputs do not demonstrate a usable configuration. No
broader development/pilot run followed. All experiment-owned servers were
stopped after saving resource/release observations; cache files remain intact.
The [measured report](../../cmd/evie/docs/research/memory-stage-4-local-extractor-spike.md)
contains the separate Mistral/Qwen outcomes, human-review status, warnings and
limitations. No production extractor has been selected.

## Verification

```sh
go test ./scripts/memory-extractor-spike -count=1
python3 -m py_compile scripts/memory-extractor-spike/measure.py scripts/memory-extractor-spike/freeze_budget.py scripts/memory-extractor-spike/inspect_qwen_gguf.py
./scripts/verify-change.sh
```

The tests use the public executable/report boundary and local HTTP servers.
They check source-only input, closed shape, exact source/scope rejection,
redirect/proxy/nonlocal/unavailable endpoints, response bounds, prompt timeout,
late-output exclusion, stopping after uncertain release, and offline scoring's
human-adjudication and denominator boundaries. Live-model results and actual
server release are separate observed experiments.

## Compact ordered wire experiment

`-wire compact-v1` is a separate standalone Qwen schema configuration. It presents
exactly the frozen selected fields in session order, with root boundaries and
content-free omission ranges. Short `s1` source aliases nominate the exact
supplied projection; explicit `range` coordinates remain original UTF-8 byte
coordinates, and `date` expands only to the contracted clock's `0:10` range.
The model sees original start/end values and separate support/context ownership.
It cannot choose destination scope. Every alias is sealed with full canonical
provenance, observed time, policy/window identity and original source bytes/hash.

Subjects use explicit `subject_type`, `subject_name` and `subject_entity_ref`.
Owner/project are literal canonical subjects. A new name is retained exactly as
an unresolved `new:` proposal; conversion never supplies a missing name or merges
same-name alternatives. Accepted identity aliases bind only offered Entity IDs;
`object_kind=entity` can also use an offered `a1` alias. These bindings prove
identity syntax, not that the selected identity or meaning follows from evidence.
The ten measured development cases contain no accepted identity alternatives;
accepted-alias coverage here is deterministic regression coverage only.

The distinct files under `v1/qwen-compact-v1` pin the compact prompt/schema and
independent request budget while keeping all earlier corpus/gold/reports intact.
Use the same ten selected cases and these flags:

```sh
-wire compact-v1 -model qwen2.5:7b-instruct-q4_K_M \
-prompt cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/prompt.txt \
-schema cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/output.schema.json \
-budgets cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/token-budgets.json \
-context 8192 -max-tokens 768 -mode schema -repetitions 1 \
-timeout 60s -stop-on-failure
```

Add `-preflight-only -output NEW_PLAN_PATH` to persist the complete sealed request
plan without inference. Ordinary inference preflights the same entire batch
before sending anything. The maximum proven bound for these ten requests is
7171 of 8192 tokens, including output and reserve; see the compact proof.

Reports preserve raw wire text, each decoded raw object, the exact request string,
alias seal/hash, canonical expansion and individual rejection reasons. Every
JSON-decodable object remains in the raw denominator even if expansion fails.
The offline scorer regenerates seals from the human-approved source corpus,
verifies recorded requests/raw/expansions and compares canonical meaning/evidence
with the same approved gold. Wire-object hashes remain the identities for future
human adjudication. Failures preserve attempted/unexecuted counts and cannot
carry retained proposals. Compact `-revalidate` is deliberately unsupported;
scoring independently verifies the saved expansion instead of rewriting results.

More valid references do not establish better meaning. For example, a copied
`Maya` subject remains a novel output relative to the approved illustrative
`new:Maya (neighbor)` spelling; the adapter does not rewrite it to manufacture
an exact match. Semantic equivalence, ambiguity and usefulness still require
actual human output judgments. This ten-case transport experiment cannot select
a production extractor or clear #136 by itself.

The recorded compact pass completed all ten requests but retained none of its
12 raw proposals. Its frozen schema allowed `selector`, `start` and `end`
independently, so every emitted dangling-start reference was schema-valid while
violating the adapter's intended selector alternatives. This is a schema design
flaw in that configuration, not evidence that every underlying meaning failed.
The output remains unrepaired. A new schema enforcing whole/date without
coordinates versus range with both coordinates is a possible next experiment;
its correction is recorded separately as compact-v2 below. The separate 12-object human review
packet and paired seed-17 comparison remain experimental evidence, and #136
still has no selected extractor.

## Closed-reference compact schema configuration

`compact-v2` is independently pinned in `v1/qwen-compact-v2`. It changes the
reference generator schema and the same embedded schema text in the prompt;
`compact-v1` source projection, seals and canonical expansion remain unchanged.
The old compact artifacts retain their observed dangling-start failures.

Every reference now has three closed JSON Schema alternatives: `ref` alone,
required `ref` and selector whole/date, or required `ref`, selector range, start
and end. Whole/date cannot carry coordinates; range cannot omit either bound.
The actual pinned Ollama0.6.3 converter/parser passes 62 offline full-output
vectors, with generated grammar 6,213 bytes below its 32,768-byte buffer. See the
[reproducible grammar proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/grammar-proof/REPORT.md).
Property order follows schema properties; range generation therefore uses
end/ref/selector/start. The adapter continues accepting valid JSON permutations.
Grammar membership does not establish source membership, meaningful ranges,
Unicode boundaries, clock eligibility, authority, scope, identity or entailment.

The [new exact input proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/input-budget-proof.md)
checks actual full-template rendering. Its maximum conservative bound is 7,763
of 8,192 with output 768/reserve 64; the serialized request cap stays 32 KiB.
Mixed wire/prompt/schema/budget identities and edited-but-rehashed manifests
fail before dispatch. Offline scoring rejects relabeled configuration identities
and regenerates canonical expansion against approved sources.

Reproduce the frozen predispatch plan without inference:

```sh
go build -o /tmp/evie-extractor-compact-v2 ./scripts/memory-extractor-spike
/tmp/evie-extractor-compact-v2 -wire compact-v2 \
  -endpoint http://127.0.0.1:11434 -model qwen2.5:7b-instruct-q4_K_M \
  -prompt cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/prompt.txt \
  -schema cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/output.schema.json \
  -budgets cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/token-budgets.json \
  -context 8192 -max-tokens 768 -timeout 60s -repetitions 1 -stop-on-failure \
  -only N01-a,N01-b,N02-a,N02-b,N03-b,N04-b,N05-b,N06-a,N08-b,N09-a \
  -preflight-only -output /tmp/compact-v2-preflight.json
```

The separate experiment manifest and execution plan pin the same ten cases once,
seed 17, temperature 0 and 60s, cached Qwen artifact and owned loopback legacy
runtime. The measured pass completed 10/10 requests with request-specific done:
16 raw proposals, 3 retained and one automatic exact required match of six
opportunities. No dangling coordinate combinations remain; ten first rejections
are support/context category mismatches and three concern identity. Fifteen raw
objects, including two retained, await actual human meaning/usefulness review in
the separate packet. Its proposed labels and all four earlier packets remain
unapplied. The owned server and observed runner were verified stopped after
resource sampling. See the main report for paired seed-17 metrics, latency,
resource observations and limits; this result does not select a production
extractor.

## Request-specific support/context schema: compact-v3

The separate `compact-v3` configuration derives schema aliases from each trusted
source seal. Supporting refs are restricted to the offered new/overlap aliases;
context refs are restricted to offered assistant aliases. Context is `const:[]`
when none is available. An input with no support permits only empty candidates.
The pinned runtime does not reliably enforce `maxItems:0` without `items`, so
that spelling is deliberately excluded. Existing projection, selectors, subject
fields, identity validation, source/authority checks and raw denominators stay
unchanged; no old output is repaired.

Its static `prompt.txt` is the natural-language prefix. The generator appends the
actual derived schema and passes the same schema in the decoding format. Report
base-file hashes and per-request derived system/schema hashes are separate.
Scoring reconstructs the exact request from reviewed sources and fixed base
files; a permissive edited schema fails even if its prompt and all request hashes
are recomputed. The generator policy, source code and binary are independently
pinned in the new fixture subtree.

To reproduce the offline request plan, use the compact-v2 command above with
`-wire compact-v3`, all fixture paths changed to `qwen-compact-v3`, and a new
output path. Build `/tmp/evie-extractor-compact-v3` from the same package first.
Keep `-preflight-only`: this preparation authorizes no automatic generation.
The [new input proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3/input-budget-proof.md)
uses the complete generated prompt, with maximum bound 7,750 of 8,192 for the
same ten windows and unchanged output 768/reserve 64. The
[actual-runtime proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3/grammar-proof/README.md)
passes 177 dynamic/empty-array cases plus 62 inherited closed-reference cases.

The measured v3 pass completed 10/10 requests with request-specific done after
full coordinated checks, independent review and current runtime/API preflight.
It produced 14 raw proposals, 9 retained and one automatic exact match of six
required opportunities. No reference-category or dangling-coordinate failure
remains; four objects lack new support and one has incompatible identity.
Thirteen raw meanings, including eight retained, await human review. Retained
fiction, attribution, polarity/change and interpretation errors remain visible
in the exact output; structural retention is not semantic adequacy. The new
packet identifies repeated pending questions. All six packets remain unapplied.
Resource sampling is complete and the owned server/runner were verified stopped.
See the main report for paired metrics, observations and limits. No further
configuration or production selection follows from this pass.
