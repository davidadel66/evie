# F22 - Plugin system

Status: rejected for now, unapproved.

## Purpose

Document why EVIE should not currently copy OpenCode's broad plugin surface.

## How OpenCode does it

OpenCode loads executable local/npm plugins and exposes hooks across config,
authentication, chat parameters, system prompts, messages, tools, permissions,
text completion, compaction, and shell environment. Plugins are trusted code in
the OpenCode process.

Source: [`packages/plugin/src/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/plugin/src/index.ts).

## EVIE decision candidate

Do not port this. EVIE is a small Go personal harness, and compile-time packages
plus an explicit tool registry are easier to understand, test, and secure. A
plugin API freezes internal boundaries early and makes every later refactor a
compatibility problem.

Skills, commands, MCP, and provider adapters cover different extension needs
without one all-powerful hook system. They should remain separate.

## Trigger to reconsider

- A real third-party extension author exists.
- At least three independent extensions need the same stable hook.
- EVIE is willing to define trust, versioning, installation, crash isolation,
  and compatibility policy.

## Non-goals even if reconsidered

- Loading executable code merely because a repository contains it.
- Allowing plugins to bypass permission/safety fences.
- npm-style remote installation as a side effect of opening a project.

## Acceptance evidence if ever promoted

- Versioned minimal API derived from measured extensions.
- Explicit trust/install flow and provenance.
- Plugin crash/failure isolation.
- Core permission and event invariants remain non-bypassable.
