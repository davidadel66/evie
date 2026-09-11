# #159 reader development v2: one manual pass, two failures

The compact production memory instruction changed, while the three development
cases, model, settings, harness, text checks, and predeclared manual rubric from
`../v1/README.md` stayed fixed. **The same two manual failures remain:** the
reader treated retired historical evidence as a current fact and attached the
colleague's quotation to the assistant event. Boston/Chicago passed. This run
does not satisfy the failed reader-quality criteria.

| Case | Model input / output tokens | Model-call time | Marker checks | Manual rubric |
|---|---:|---:|---|---|
| historical_retired | 3133 / 102 | 24.149 s | Pass | **Fail** |
| saved_boston_newer_chicago | 3056 / 179 | 14.880 s | Pass | Pass |
| tentative_quote_and_inference | 2618 / 180 | 12.620 s | Pass | **Fail** |

The compiled test completed in 51.92 seconds with PASS for its deterministic
evidence assertions and fixed text-marker checks. Each case made one actual
model request, and no response was truncated. That process status is not the
manual quality result. The original v1 artifacts are unchanged.

## Change and execution identity

`prompt-delta.json` retains the previous instruction from the actual v1 request
and the new production instruction. The new instruction explicitly says that
`status` applies then, `current_status` applies now, retired evidence never
supports current facts, and exact source actor/event attribution matters. The
longer intermediate instruction was not used for this inference attempt.

`freeze.json` pins the unchanged model, configuration, harness, rubric and
production prompt. `run-metadata.json` pins the compiled binary and relevant
uncommitted production Go files. After the implementation owner confirmed the
final compact prompt, the binary was compiled again; its hash and prompt hash
matched the frozen values. `final-prompt-verification.json` records that check
before the first model call. No production prompt versions were mixed within
this attempt. The root task's server was not stopped or modified.

## Manual assessment against the frozen rubric

**Historical retirement — fail.** The answer names the chickpea sandwich and
correctly cites original event `ae514148-0fbb-4502-ab25-f27cd7635fe0`. It then
says the accepted preference “should still be treated as your current
preference unless there has been a subsequent change.” Both historical evidence
items actually carry `current_status: retired`, and the system instruction
explicitly prohibits treating retired evidence as current. The answer fails to
distinguish past acceptance from present retirement. It invents no replacement
preference or precise original date.

**Boston/Chicago — pass.** The answer identifies Boston as the saved home city
and Chicago as the later owner update, labels the discrepancy, and correctly
cites event `de93fd11-0a40-4992-a24c-278ac2225c8a` for Boston and event
`612e54e4-d923-446a-a7e4-9831c1d2f86a` for Chicago. It asks about updating or
clarifying the information without claiming that the saved record was changed.
The Kernel's accepted revisions remain unchanged after the turn.

**Tentative quote and inference — fail.** The answer preserves the owner's
uncertainty and correctly distinguishes the colleague's definite plan from the
owner's unconfirmed trip. It also describes the assistant's inference as an
interpretation rather than current factual confirmation. However, it cites
assistant event `5174a196-9f73-44ef-8aab-4f670efa0459` for the colleague's
quotation. That quotation actually appears in owner event
`4c5f7353-665c-47d7-b6b7-a89ffc09f75b`; the assistant event contains the separate
inference. Consequently exact quotation/source attribution still fails. No
booking, itinerary or precise date was invented.

The fixed markers again produced false positives for the two failed quality
criteria. The complete raw answers, evidence, wire requests, source IDs and
measurements remain available to inspect. This is a small development result,
not an evaluation of automatic initial search selection, broader reader
quality, or #167/#168 held-out acceptance. Any subsequent rendering or prompt
change requires a new frozen attempt; it must not replace these failures.
