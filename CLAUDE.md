# CLAUDE.md

> Status board: `docs/BACKLOG.md` (updated by `/wrap`) — read its "Next session" block when starting work.

## What this repo is

A personal collection of small Go command-line tools, built by David. It serves **two purposes at once**:

1. **Learning Go.** David is using these tools as hands-on practice to get fluent in Go — language idioms, the standard library, project structure, error handling, etc.
2. **Building a useful CLI toolkit.** Each tool is meant to be genuinely useful day-to-day, installed on `PATH`, and — longer term — usable by an **agent/harness as a tool** (an agent shells out to `todo add ...`, `todo list`, etc.). The tools are being designed to be scriptable and composable with that future in mind.

Today there is **one tool (`todo`)**. The repo will grow to hold more over time.

## How to work with David here (IMPORTANT)

**This is a learning repo. David writes the code himself.** Default to a tutor stance:

- **Do NOT edit his Go source files** to implement features or refactors. When he asks for "the code," that means **show it in a chat message** for him to type — it does not authorize editing his files.
- Guide, point, and explain the *why* (idioms, "the Go way," design tradeoffs, what good looks like). Make him earn the implementation; teach the judgment he can't self-derive.
- Reading, building (`go build`), running, and `go vet` to check his work is fine and encouraged.
- Only make a direct file edit when he **explicitly asks for that specific change** ("add this one piece"), and then scope it to exactly what he asked — never refactor adjacent code unprompted.
- Authoring non-source docs like this file is fine when requested.

## Conventions

- **Build / install:** `go build -o ~/go/bin/todo .` — compiles and drops the binary in `~/go/bin` (on PATH). The installed binary is a snapshot; rerun this after any change you want live. This is the "deploy" step. Don't leave stray binaries in the source dir.
- **Data location:** tools store state in `~/.todo/<name>.json` (JSON, human-readable). The source directory stays clean — only code lives here.
- **Config via env vars with defaults:** e.g. `TODO_NAME` / `TODO_DIR` select which list, falling back to sensible defaults so the common case needs zero config. Prefer this over flags for ambient config, since flags fight subcommand layout.
- **CLI shape:** `tool <command> [args] [--flags]`. Dispatch on `os.Args[1]` with a `switch`; per-subcommand `flag.NewFlagSet` for optional flags; keep required values positional.
- **Separation of concerns:** data/domain methods (`Add`, `Delete`, `Save`, …) change state and stay silent — return errors instead of printing. The CLI/`main` layer owns all user-facing output. This keeps the core reusable (including by an agent).
- **Errors:** wrap with `fmt.Errorf("...: %w", err)` to preserve the chain; check every returned error.
- **Feature docs:** each spec'd feature gets a short kebab-case name shared across all its artifacts — `docs/active/<feature>.spec.md` + `docs/active/<feature>.decisions.md` while in flight, moved to `docs/done/` when shipped, with source/tests named to match (`<feature>.go`, `<feature>_test.go`). `docs/` lives inside the tool's directory (e.g. `cmd/finance/docs/`).

## Current tools

### `todo`
A task manager. Full CRUD with stable, persisted IDs (a monotonic `NextID` counter, never reused), multiple named lists (via `TODO_NAME`), priorities and due dates (via flags), and JSON persistence in `~/.todo/`.

Commands: `list`, `add <title> [--priority N] [--due YYYY-MM-DD] [--desc text]`, `done <id>`, `delete <id>`, `help`.

### `finance`
Personal finance tool backed by Plaid + SQLite (`~/.finance/finance.db`, tables `items` + `transactions`; access tokens live there, file is 0600). Env: `PLAID_CLIENT_ID` / `PLAID_SECRET` (loaded from repo-root `.env`).

Commands: `link` (hosted Plaid Link flow, saves item + access token), `sync` (incremental `/transactions/sync` per linked bank, cursor-based, atomic per page), `db` (sanity check). Decisions and gotchas: `cmd/finance/docs/done/sync.decisions.md`.

## Future direction

- More tools, each following the conventions above so they're consistent and agent-friendly.
- As the number of tools grows, expect to move from a single root `main.go` to a multi-binary layout (e.g. `cmd/<tool>/main.go`, shared code in packages) so each tool builds independently.
- Keep tools scriptable: predictable exit codes, parseable output, no interactive prompts — so an agent can drive them as harness tools.
