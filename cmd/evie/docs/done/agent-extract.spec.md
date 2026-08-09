# agent-extract — lift the loop into internal/agent

Feature 1 of the serve umbrella (`serve.spec.md`, approved 2026-08-03). No new
behavior: after this lands, `evie` does exactly what it does today — but the
loop lives in `internal/agent`, driven through an interface any frontend can
implement. Serve (feature 2) plugs into it without touching the loop again.

## Purpose

The agent loop in `cmd/evie/main.go:117-151` is tangled with terminal I/O.
Extract it into a silent, injectable core so REPL and web are two thin faces
over one brain — the repo's "domain layer silent, frontends own output"
convention applied to the agent itself.

## API (from the umbrella spec)

```go
package agent

type Client interface {
    ChatStream(req openrouter.ChatRequest, onDelta func(string)) (openrouter.ChatResponse, error)
}

type Events interface {
    Delta(text string)
    AssistantDone(content string)              // fired for EVERY assistant message, even empty
    ToolCall(id, name, args string)            // emitted immediately before executing
    ToolResult(id, content string, isErr bool) // includes declines
}

type Session struct { /* mu, client, model, messages */ }

func New(client Client, model string) *Session
func (s *Session) Send(input string, ev Events, approve func(name, args string) bool, extra ...tools.Tool) error
```

## Decisions (with why)

- **Consumer-owned `Client` interface** in `internal/agent`, satisfied by
  `*openrouter.Client` — Go idiom; lets tests script a fake provider and lets a
  second provider land later without touching the loop.
- **`Events` interface, not channels** — the loop is sequential; callbacks keep
  ordering trivial and let the REPL impl be a thin wrapper over smoothPrinter.
- **Extras per-`Send`, not per-`Session`** — so serve can hand `show` a closure
  over the current turn's SSE encoder (spec-review ruling, see
  serve.decisions.md).
- **`ErrBusy` sentinel + `TryLock`** — second concurrent `Send` fails fast;
  serve maps it to HTTP 409.
- **"No choices" is an error return, not an event** — it aborts the turn;
  frontends decide how to display errors.
- **Model default moves to `internal/agent`, `EVIE_MODEL` overrides** — two
  frontends, one source of truth; env-with-defaults convention.
- **Env loading: `~/.evie/.env` if present, else ambient env** — the old
  cwd-relative `../../.env` only works when run from `cmd/evie/`; a binary on
  PATH has no cwd guarantee. ⚠ Migration: David copies the repo `.env` to
  `~/.evie/.env` once (or exports the key in his shell).
- **`Schemas`/`Execute` stay, becoming wrappers over `SchemasWith`/`ExecuteWith`**
  — wrapping semantics (decline text, error text, unknown tool) keep exactly one
  home; existing tests and callers keep compiling.

## Stages

1. **tools seam** — `internal/tools/registry.go`: add `SchemasWith(extra []Tool)`
   and `ExecuteWith(extra []Tool, call, approve)`; reimplement `Schemas`/`Execute`
   as thin wrappers. Registry tests for: extra tool dispatched, extra tool
   schema included, base behavior unchanged.
2. **the loop** — `internal/agent/agent.go`: `Session`, `New`, `Send` as above;
   faithful lift of main.go:117-151. Tests with a scripted fake `Client`:
   plain answer; tool round trip; gated approve/decline; extra-tool call;
   provider error mid-turn; ErrBusy. Assert on messages slice + recorded
   events, never stdout.
3. **rewire the faces** — `cmd/evie/repl.go` (Events impl over smoothPrinter,
   `[calling %s]`, y/N approver, same prompts and decline text) + `main.go`
   (dispatch: no args → REPL; `serve` → "not built yet" error for now; env
   loading + `EVIE_MODEL` as decided). Delete the now-dead loop from main.go.
4. **parity demo** — rebuild, run the REPL: one plain turn, one tool turn, one
   gated decline; outputs match today's behavior.

## Open questions

- None. (Anything discovered mid-build gets flagged, not silently resolved.)

## Definition of done

- `go build ./...`, `go vet ./...`, `go test ./...` all clean — output shown.
- Stage 4 parity demo run for real, David eyeballs it.
- Checkboxes above ticked; decisions that emerged recorded in
  `serve.decisions.md` (this feature shares the umbrella's decisions file).
