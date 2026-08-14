# F05 - Capability and permission policy

Status: candidate, unapproved. Priority: first, before restricted agents.

## Purpose

Decide mechanically which profile may see and execute each capability, on which
resource, and whether human approval is required.

## How OpenCode does it

OpenCode evaluates ordered wildcard rules with actions `allow`, `ask`, or
`deny`; the last matching rule wins and no match defaults to `ask`. Denied tools
can be hidden from the model. An `ask` suspends execution until the UI answers
`once`, `always`, or `reject`; "always" approvals live in process memory.

OpenCode has separate permissions for external directories, secret-like reads,
task delegation, repeated-call doom loops, and tool families. Its shell command
analysis is policy assistance, not an OS sandbox.

Source: [`permission/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/permission/index.ts).

## EVIE today

EVIE has per-tool `NeedsApproval` plus `Approved`, `Declined`, and `Expired`.
That distinction and fail-closed zero value are good. Policy is otherwise flat:
all schemas go to every session, only file/DB edits are gated, and unrestricted
Bash can bypass every typed path or write fence.

## Proposed EVIE adaptation

Add two layers:

1. **Capability policy:** profile/session rules decide `allow`, `ask`, or `deny`
   for a tool and optional resource pattern.
2. **Non-bypassable safety fences:** secret redaction, canonical path checks,
   stale previews, and data-scope rules apply regardless of approval.

Denied schemas are omitted from model requests, but execution checks again in
case a provider returns a forged/stale call. Parent denials are hard ceilings for
children. Session "always" grants must be bounded to a project, capability,
resource pattern, and session lifetime unless a separately persisted policy is
explicitly approved.

Do not claim that a tool-name allowlist makes Bash read-only. Restricted
profiles either receive a constrained command capability with proven semantics
or receive no shell.

## First policy classes

- project read;
- external-directory read;
- project file mutation;
- shell execution;
- database/domain mutation;
- subagent spawn by profile;
- Git commit/push/release;
- network publication;
- repeated identical call.

## Acceptance evidence

- Denied capabilities are both invisible and unexecutable.
- `Declined` and `Expired` remain distinguishable.
- A new global tool does not widen restricted profiles.
- A child's effective policy is never broader than its parent's.
- "Always" for one project/path does not leak to another session.
- Approval cannot bypass a secret or canonical-path fence.

## Open decisions

1. Does EVIE retain ungated Bash for the personal profile while coding profiles
   use a different policy?
2. Are "always" approvals process-only, session-durable, or project policy?
3. What UI explains matched rules without overwhelming normal work?
