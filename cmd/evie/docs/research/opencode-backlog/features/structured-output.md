# F19 - Structured agent output

Status: selective candidate, unapproved.

## Purpose

Require machine-consumed agent work to finish with schema-valid data rather than
parsing prose.

## How OpenCode does it

OpenCode can add a required synthetic final tool whose arguments match the
requested output schema. If the model finishes without calling it, the run is an
error. Provider-native structured generation is used elsewhere for generated
agent configuration.

Source: [`session/prompt.ts`, structured output](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/prompt.ts#L1243-L1315).

## Proposed EVIE adaptation

Use only where another program consumes the result:

- spec-review findings and verdict;
- test-writer artifact report;
- code-review findings;
- memory proposals;
- build-stage result.

Prefer provider-native JSON schema when the selected model/route supports it.
Otherwise expose one harness-owned final-result tool and validate its arguments.
The tool performs no external side effect; it commits a typed result to the
session. Ordinary user conversation remains prose.

One bounded repair attempt may return validation errors to the model. After
that, fail the stage rather than inventing missing fields in Go.

## Acceptance evidence

- Missing final structured result fails the workflow stage.
- Unknown fields/version changes follow an explicit compatibility policy.
- Validation errors are actionable and bounded.
- Schema-valid output cannot claim gate success without referenced evidence.

## Open decisions

1. Which first workflow justifies the abstraction?
2. Persist raw candidate output for diagnosis, or only the validated result?
