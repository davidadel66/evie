# A06 - Title agent

Status: deferred, unapproved. Kind: hidden internal agent.

## Purpose

Generate a short, searchable session title after the first substantive user
message.

## How OpenCode does it

OpenCode registers a hidden, tool-free `title` profile with temperature `0.5`.
Title generation runs asynchronously for a root session after its first real
user message. It prefers a small model from the same provider when available.
The prompt requires one line, at most 50 characters, in the user's language.

Sources:

- [`Agent` registry, `title`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L234-L249)
- [`SessionPrompt` title generation](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/prompt.ts#L193-L253)
- [`title.txt`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/prompt/title.txt)

## EVIE assessment

Defer until durable sessions have a picker or history page. Titles add no value
while EVIE exposes one global in-memory conversation.

## Proposed EVIE adaptation

- Run after the first committed user turn, never before persistence.
- No tools and no reasoning persistence.
- Use a configured cheap model or deterministic fallback from the first message.
- Treat failure as non-fatal; retain a timestamp/session-ID fallback.
- Permit user rename; automatic generation must not overwrite it.
- Do not send tool results or unrelated historical secrets merely to title a
  session.

## Acceptance evidence

- Title generation cannot block or fail the user turn.
- A manual title wins permanently.
- Child sessions derive clear role/task titles without another paid call when
  possible.
- Titles remain bounded and safe for display and filesystem export names.
