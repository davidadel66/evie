# Evie

Evie is one person's always-on personal agent: a coding harness, a
researcher and a general assistant that works as a right hand across the
owner's projects. This repository is a rewrite from scratch of an earlier Go
version.

Motto: simplicity is the most powerful foundation. Start from first
principles, and add a part only when it can be justified.

## Where things stand

Milestone 1 is built. Milestone 2 is next and is not designed yet.

- `decisions.md` is the record of what has been decided and why. Read it
  before proposing anything. Add to it when the owner decides something; do
  not record your own suggestions as decisions.
- `design.md` is the proposed design for the current milestone.
- `.vision/map.json` draws that design as an interactive map (built with the
  vision skill; run its commands with `--root` set to this folder). The owner
  reads the map to understand the system, so keep it in step with the design.
- `research/` holds one report per harness studied, each read from source:
  OpenCode, Codex, DeepSeek Harness, Letta, and pi with T3 Code. They are
  evidence, not instructions.
- `milestone-1.spec.md` is the spec for milestone 1. Its tickets are issues
  on GitHub, each naming the tickets that block it.

These documents, the map and `research/` are planning documents. For now
they are kept out of the public repository and exist only in the owner's
working folder.

The owner walks through the map one ticket at a time. No ticket is built
until he has understood its part of the map and said go.

## What Evie will become

- **Always on.** One server that runs all the time. The terminal, a web UI,
  and later a phone app and a voice device are all clients of it.
- **One core.** One way in (the inbox), one loop that runs turns, one way
  out (the event stream), and an add-only event log that is the truth. A
  workspace is the core pointed at a different scope. Coding, research and
  assistant work are modes on the same loop. Orchestration is a session
  starting child sessions.
- **Isolated workspaces.** Each project has its own memory, files,
  retrieval and tabs. Memory about the owner is shared across them.
- **Excellent at coding first.** The coding harness is the first face,
  because it exercises the whole core.
- **Free to act, careful with risk.** Evie works freely inside a workspace
  and asks before anything risky or outward-facing.
- **Later:** voice and visuals in place of streamed text, a phone app, a
  home device, and eventually a robot. None of this is designed yet.

## Milestones

1. The core loop, a plugin system and nine tools, usable in a terminal.
   Built.
2. A real coding harness: prompt and instruction files, compaction,
   permissions, undo, subagents.
3. The always-on server, web UI, workspaces, plugins.
4. Scheduled work, memory, voice, phone.

Work only on the current milestone. Later ones are designed when they start.

## How to build

- TypeScript on Bun, state in SQLite, models through OpenRouter behind a
  small provider interface.
- Write plain TypeScript the way Go would: pass a cancel signal down
  explicitly, return expected failures as values where it matters (such as
  tool results), keep thrown errors for real bugs, and pass services in
  directly. No effects framework.
- Evie has a plugin system from milestone 1. Built-in features are plugins
  that use the same interface as one added by someone else. Approach
  everything this way: as a reusable part with one clear shape.
- Adapt proven pieces from the harnesses in `research/` rather than
  inventing. Keep their licence notices when code is adapted.
- Tool results are written for the model: a failure says what to do next,
  long output is saved in full with the start shown, and nothing blocks.
- Libraries are fine when they are harmless. Do not make Evie depend heavily
  on any one of them.
- Keep it small. A whole headless coding harness is about 13,000 lines in
  the smallest one studied. Anything that grows well past its counterpart in
  the research has to justify itself.

## Open source

This repository is meant to be published. From the first commit:

- No secrets in the repository. Keys come from settings in the environment.
- No personal data in the repository. Evie's state lives in a data folder
  outside it.
- Personal integrations are separate plugins kept elsewhere, never part of
  the core.
- Write documents and comments for a reader who is not the owner.

## Working with the owner

The owner is learning how agent systems work and does not want to hand-write
the code. Agents write the code; he reviews the systems and the design.

- Take one step at a time. Answer the question asked, then stop.
- Explain in plain language and define a term the first time it appears.
  Show a snippet only where it makes an idea clear.
- When he is confused, make the step smaller.
- Say what you are about to build and wait for his go before building it.
- Build each ticket on its own branch and open a pull request for it into
  `dev`. He reviews every pull request before it is merged.
- `main` is the stable version: it is what the public sees and what the
  owner runs day to day. Work reaches it only through a release, which is a
  pull request from `dev` into `main`. GitHub runs the check on every pull
  request.
- Give a recommendation with its real trade-off. When he has decided,
  record it and move on.
