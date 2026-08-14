# F10 - Agent profiles

Status: candidate, unapproved. Priority: after durable policy boundaries.

## Purpose

Represent an agent as a capability profile rather than as prompt text alone.
Profiles are what make `build`, `plan`, `explore`, internal summarizers, and
future review roles mechanically different.

## How OpenCode does it

OpenCode's `Agent.Info` combines:

- stable name and description;
- `primary`, `subagent`, or `all` visibility;
- native/custom and hidden flags;
- optional model and variant;
- optional prompt, temperature, top-p, and provider options;
- a step limit; and
- ordered permission rules.

The registry creates seven native profiles, merges user configuration over
them, and supports custom agents. A configured default must be visible and must
not be subagent-only. Permission-filtered tools are hidden from the model before
a request is sent.

Source: [`packages/opencode/src/agent/agent.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L35-L264).

## EVIE today

`agent.New` creates one hardcoded identity, prompt, model, reasoning setting,
and full tool registry. Per-turn tools are additive, so a caller can add a tool
but cannot make a narrower agent. That is incompatible with trustworthy
read-only planning, exploration, or review.

## Proposed EVIE adaptation

Start with a small code-owned value, not a plugin/config ecosystem:

```text
Profile
  name
  kind: primary | subagent | internal
  prompt addition or replacement
  model override, optional
  tool capability policy
  maximum model steps
  visibility
```

The effective capability set must be computed once for each model request and
enforced again at execution. Unknown tool names in a profile must fail startup;
silently ignoring a typo can accidentally broaden authority.

Profile permissions are ceilings. A child may be narrower than its parent but
must never regain a capability the parent was denied. Adding a new tool to the
global registry must not silently add it to a restricted profile.

## First slice

- Code-owned profiles only.
- `build`, `plan`, and `explore` only.
- Explicit allowlists for restricted profiles.
- One optional model override.
- One maximum-step bound.
- Profile name persisted on every assistant/tool event.

## Not in the first slice

- Model-generated profiles.
- Arbitrary project executables or hooks.
- Remote profile catalogs.
- User-supplied provider option maps.
- Hot reload.

## Acceptance evidence

- A denied tool schema is absent from the request and execution still rejects a
  forged call to it.
- Adding a new global mutator does not widen `plan`, `explore`, or review roles.
- An unknown allowlist entry fails loudly.
- A child cannot override a parent denial.
- The transcript records which profile and model produced every action.

## Open decisions

1. Does selecting `build` imply edit authority, or must implementation authority
   be a separate session setting? Recommendation: separate setting.
2. Are project-defined profiles trusted instructions or untrusted project data?
   Recommendation: load definitions only from an explicitly trusted project.
3. Should prompts replace EVIE's stable identity or append role instructions?
   Recommendation: immutable identity plus a later role block.
