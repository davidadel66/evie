# #159 reader development v1: one manual pass, two failures

Three actual local Qwen answers were generated with the frozen configuration,
one model call per complete SQLite-backed turn. The deterministic evidence
contracts and all fixed text-marker checks passed. **The predeclared manual
rubric passed only Boston/Chicago; historical retirement and Kyoto source
attribution failed.** This run does not satisfy those reader-quality criteria.
No failure is relabeled as a pass and no case or check was changed after seeing
the answers.

| Case | Model input / output tokens | Model-call time | Marker checks | Manual rubric |
|---|---:|---:|---|---|
| historical_retired | 3115 / 119 | 28.339 s | Pass | **Fail** |
| saved_boston_newer_chicago | 3052 / 179 | 14.979 s | Pass | Pass |
| tentative_quote_and_inference | 2613 / 185 | 13.102 s | Pass | **Fail** |

The compiled test ran in 56.66 seconds and returned PASS for its deterministic
and marker checks. That process status is not the manual quality result.
`run-metadata.json` pins the compiled test binary and the relevant uncommitted
#159 production Go tree. The original `freeze.json` pins the reader source,
model, configuration and rubric. The earlier `freeze-before-output-guard.json`
predates an overwrite-prevention guard; no model call occurred before that
guard and the final freeze. The root task's server was not stopped or modified.

## Manual assessment against the frozen rubric

**Historical retirement — fail.** The answer names the chickpea-sandwich
preference and correctly cites event `05ced59a-6d7d-44bb-9cd1-79fffc869563`.
However, it says that there have been “no subsequent changes or overrides” and
that the preference “can still be treated as your current preference.” The
actual composed request includes `current_status: retired` for the historical
evidence. The answer therefore fails the required distinction between past
acceptance and current retirement, and falsely asserts present applicability.
It invents no replacement preference or precise original date.

**Boston/Chicago — pass.** The answer preserves Boston as the saved record,
attributes Chicago to the later update, explicitly identifies a discrepancy,
and cites the correct original event for each statement. It does not claim
that memory was corrected. Its qualified conclusion that the newer statement
suggests Chicago remains attributed, and the Kernel's accepted revisions are
unchanged after the turn.

**Tentative quote and inference — fail.** The answer correctly concludes that
the owner's Kyoto trip is not confirmed and that the colleague's definite plan
does not apply to the owner. However, it cites assistant event
`d5a0a91b-1dde-49a1-80dd-871285af9943` for the colleague's quotation, then describes
that quotation as an assistant inference. The quotation actually appears in
owner event `4b339ba8-ecc8-47b9-b277-1427955671ca`; the assistant event contains
the separate speculation that the trip could be confirmed. Merely including
both UUIDs did not prove faithful citation. This fails original-versus-inferred
source attribution even though destination, uncertainty and colleague markers
all passed. No booking or itinerary detail was invented.

The fixed marker checks were intentionally documented as regression signals,
not entailment judgments. Their false positives in these two cases demonstrate
that limitation. All raw composed requests, actual Ollama wire requests,
complete responses, timings and case source IDs are retained. A production
instruction or rendering fix must be evaluated in a **new frozen attempt**;
this v1 failure remains available and must not be overwritten. These three
development cases do not establish automatic search selection, broader reader
quality, or #167/#168 release acceptance.
