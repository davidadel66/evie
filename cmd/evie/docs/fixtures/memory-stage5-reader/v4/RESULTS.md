# #159 configured production reader v4: all three original cases pass

The actual configured production reader passed all three unchanged development
cases, both the fixed text checks and the original manual rubric. Each complete
SQLite-backed turn made one actual OpenRouter Responses call. Historical
retirement, accepted-memory discrepancy and exact source attribution were
correct in these answers. Earlier Qwen failures remain preserved in v1–v3.

| Case | Native input / output tokens | Model-call time | Whole turn | Actual wire request | Marker / manual |
|---|---:|---:|---:|---:|---|
| historical_retired | 2820 / 103 | 3.058 s | 3.082 s | 13433 bytes | Pass / Pass |
| saved_boston_newer_chicago | 2726 / 162 | 4.538 s | 4.551 s | 13143 bytes | Pass / Pass |
| tentative_quote_and_inference | 2360 / 148 | 3.463 s | 3.475 s | 11946 bytes | Pass / Pass |

The complete test returned PASS in 11.86 seconds. All three requests returned
HTTP 200 and completed successfully without truncation or additional tool
calls. The provider reported zero reasoning-output tokens in each response.
Provider-native token counts above are those consumed by the production usage
contract; OpenRouter's separate standardized estimates differ and are retained
separately in the allowlisted generation metadata. The reported total cost was
$0.0851825 for all three calls. These three observations are not a latency
benchmark or a percentile estimate.

## Manual assessment against the original rubric

**Historical retirement — pass.** The answer identifies the past chickpea
sandwich preference and cites owner event
`1ff00684-d380-4f82-b911-af2d9e3d27f3`. It explicitly says the record is retired
and must not establish the current preference. It invents no replacement
preference or precise original date.

**Boston/Chicago — pass.** The answer names Chicago according to the later
direct statement while explicitly identifying the saved Boston record as
inconsistent with it. It cites owner event
`11f364e6-3e54-49ab-8d0f-98c3dd777d3f` for Boston and owner event
`dac32e81-8703-4598-aaf0-812870630b62` for Chicago. It states that the accepted
Boston record remains active, that Chicago is conversation evidence rather than
an updated accepted memory, and that it has not changed the saved record.

**Tentative quote and inference — pass.** The answer preserves the owner's
uncertainty and lack of bookings, attributes the definite plan to the colleague,
and cites original owner event `338a985a-e5c8-49be-8f13-bbf82423d98c`. It quotes
the separate assistant inference with the correct assistant event
`d37d642d-5121-4aff-8c8b-71c78dbc36ec`, and explicitly rejects that speculation
as confirmation from the owner. No stronger authority, booking, itinerary or
precise date is invented.

The same exact evidence checks ran before provider dispatch, and accepted-memory
revision/Claim-count checks passed after each turn. This establishes successful
reader behavior on these three development scenarios without changing the
original quality criteria or treating scripted answers as model evidence.

## Identity, verification and limits

The binary used an archive of the then-current #159 commit
`4ec5429`, isolated from concurrent #160/#164 changes. Freeze and production
file hashes were recorded before generation. The canonical model advertised by
official metadata was `openai/gpt-6-astra-20260903`. Subsequent allowlisted
generation metadata confirms that exact model and provider `OpenAI` for all
three completed requests. The application used its normal automatic provider
routing with `require_parameters`; no backend route was forced and no model
weight digest is exposed. Raw successful wire streams and normalized answers
are retained; no credentials or authorization headers are recorded.

Reproduction after the planned history rewrite uses the final PR's owning
#159 commit and validates the exact recorded production file hashes, as detailed
in [REPRODUCE.md](REPRODUCE.md). The historical `4ec5429` identity records what
actually ran and is not a requirement to fetch an otherwise unreachable commit.
The frozen original README and freeze are intentionally unchanged.

Exact local checks on the archived baseline plus adapter:

- `go test -c -o /tmp/evie-memory-stage5-reader-v4.test ./internal/agent` — PASS.
- Compiled `TestMemoryStage5ProductionReaderPreflight` — PASS, 0.53 s; official
  metadata discovery only, no generation.
- Compiled `TestMemoryStage5ReaderEvidenceContract` — PASS, 0.26 s; all three
  original deterministic cases.
- Compiled `TestMemoryStage5ProductionReaderEvaluation` — PASS, 11.86 s;
  original marker checks and the three actual model calls reported above.
- `go vet ./internal/agent` — PASS.
- `git diff --check` in the task worktree — PASS.

Repository-wide verification remains the implementation owner's handoff check.
These results do not establish automatic initial search selection, the full
Standard toolset, robustness across repeated hosted generations, or #167/#168
held-out acceptance. Compared with v3, this run changes both the reader stack
and uses the committed same-evidence relation-preservation fix; it is not a
controlled model-only ablation. The Qwen failures remain a limitation of that
tested local configuration, not a silently removed acceptance gate. Further
production or model changes require a new frozen evaluation attempt.
