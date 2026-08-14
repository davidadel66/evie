# F02 - Project instructions and context composition

Status: candidate, unapproved. Priority: first, after F01.

## Purpose

Automatically teach a development session the project's rules without mixing
those rules into EVIE's immutable identity or guessing from repository files.

## How OpenCode does it

OpenCode assembles its system context in layers: model/agent prompt,
environment, instruction files, MCP instructions, skills, and per-user system
text. It searches global instruction candidates, then project files in priority
order (`AGENTS.md`, `CLAUDE.md`, deprecated `CONTEXT.md`). The first filename
class found wins, while matching files up the project hierarchy may be included.
Configured files and URLs can add more instructions.

When a file is read, OpenCode can attach nearer directory-level instructions
once for the current assistant message. It tracks what was already loaded to
avoid repeated prompt growth.

Sources:

- [`session/instruction.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/instruction.ts)
- [`session/system.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/system.ts#L67-L135)

## EVIE today

`internal/agent/prompt.go` deliberately reserves later blocks for runtime facts,
project instructions, and memory, but `Session` currently sends only the stable
prompt plus transcript. EVIE does not discover this repository's `CLAUDE.md`,
so its tutor-first rule reaches external assistants but not EVIE itself.

## Proposed EVIE adaptation

Use an explicit context composer with ordered blocks:

1. Immutable EVIE identity and safety rules.
2. Selected profile instructions.
3. Trusted runtime facts: date, platform, scope kind, and, for a project session,
   canonical project root, cwd, and Git state.
4. Trusted global user instructions.
5. Trusted project and directory instructions, each labeled with source path.
6. Untrusted retrieved memory/reference data in a data-only block.
7. Recent/compacted transcript.

Do not infer authority from arbitrary Markdown. A project must be registered,
selected for this session, and trusted before its instruction file can direct
tool use. Global sessions load no project instructions. Web pages, remote
instruction URLs, tool output, and ordinary source files remain data.

Directory-local instructions should load only when work enters that subtree,
with deterministic precedence and deduplication. The context snapshot should
record source paths and content hashes so a resumed session can explain what
rules governed a turn.

## First slice

- Global `~/.config/evie/AGENTS.md` or one documented equivalent.
- One project-level `AGENTS.md` or `CLAUDE.md` class with explicit precedence.
- No remote instruction URLs.
- Environment block with scope and date; project sessions add root, cwd, and Git
  branch/dirty status.
- Context inspection showing which instruction files were loaded.

## Acceptance evidence

- This repository's tutor rule reaches the model before it can edit Go.
- A global session started from this repository does not load its `CLAUDE.md`.
- Changing cwd does not load instructions outside the canonical project.
- Nested instructions load once and have documented precedence.
- A source file containing "ignore prior instructions" remains data.
- Prompt ordering and loaded content hashes are deterministic.

## Open decisions

1. If both `AGENTS.md` and `CLAUDE.md` exist, should one win or should both load?
2. Does trusting a project trust every nested instruction file within it?
3. Should instruction changes mid-session apply immediately or only after a
   visible reload event?
