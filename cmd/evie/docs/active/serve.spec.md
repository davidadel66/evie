# serve — web frontend for the evie harness

Status: draft (pending David's approval)

## What this is

`evie serve` starts a localhost HTTP server hosting a web UI for the agent:
streaming chat with Evie, visible tool calls, a real approval UI for gated tools
(with a diff view for `edit_file`), and a side panel ("whiteboard") where the
model draws and displays rich content — free-stroke SVG, mermaid diagrams,
HTML, markdown, images — by emitting tagged blocks inline in its streamed
response (no tool call; Claude-Artifacts style). A vision critic ("the eyes")
reviews rendered boards and feeds corrections back. The terminal REPL stays
and keeps working exactly as today; serve is a second frontend over the same
loop.

Decided in `docs/decisions.md` (2026-07-29): web frontend over desktop/TUI,
stdlib `net/http` + `go:embed` UI + streaming, a new `cmd/` door, localhost-first.

## Architecture at a glance

```
cmd/evie/
  main.go        dispatch: no args → REPL (today's behavior), "serve" → server
  repl.go        today's loop body, now a consumer of internal/agent
internal/web/
  serve.go       HTTP server: static UI (go:embed), SSE chat, approvals
  ui/            Vite + React + TS app (source)
  ui/dist/       build output (gitignored), embedded via //go:embed all:ui/dist
internal/agent/
  agent.go       Session: the extracted loop — messages, Send(), events out
internal/tools/  gains: SchemasWith/ExecuteWith (registry + per-turn extras)
```

Serve lives in `internal/web` (not `cmd/`) by David's call: the server will
grow (sessions API, multi-agent dashboard), and go:embed needs the UI assets
in the same package tree. `cmd/evie` stays a thin door: dispatch + REPL.

Backend is stdlib-only (no new Go deps). Frontend is React 19 + Vite + TypeScript
with: assistant-ui (chat components, ExternalStoreRuntime), Streamdown (streaming
markdown + mermaid + syntax highlighting), a diff viewer (see spike in Part 4),
Tailwind. Rationale for each: `serve.decisions.md`.

## Part 1 — extract the loop into `internal/agent`

The inner loop of `cmd/evie/main.go:117-151` moves to `internal/agent`,
parameterized by an event sink and approver so any frontend can drive it.

```go
package agent

// Client is what the session needs from a provider. Satisfied by
// *openrouter.Client; defined here because the consumer owns the interface.
type Client interface {
    ChatStream(req openrouter.ChatRequest, onDelta func(string)) (openrouter.ChatResponse, error)
}

// Events receives everything a frontend needs to render a turn.
// Implementations must be fast or buffer internally; the loop blocks on them.
type Events interface {
    Delta(text string)                            // streaming assistant text
    AssistantDone(content string)                 // fired for EVERY assistant message, even empty (tool-only)
    ToolCall(id, name, args string)               // emitted immediately before executing that call
    ToolResult(id, content string, isErr bool)    // tool finished (includes declines)
}

type Session struct {
    mu       sync.Mutex          // one turn at a time; TryLock → ErrBusy
    client   Client
    model    string
    messages []openrouter.Message // seeded with the Evie system prompt (moves here)
}

func New(client Client, model string) *Session

// Send runs one full turn: append user msg, then model↔tools until the model
// answers without tool calls. extra tools are offered to the model for this
// turn only (unused in v1; reserved for stateful board ops). Returns the error that
// aborted the turn ("no choices" is an error return, not an event). Concurrent
// Send: second caller gets ErrBusy immediately (TryLock).
func (s *Session) Send(input string, ev Events, approve func(name, args string) bool, extra ...tools.Tool) error
```

Model string: default lives in `internal/agent` (`moonshotai/kimi-k3` today),
overridable via `EVIE_MODEL`. main.go stops hardcoding it.

Event order within a turn is deterministic and sequential:
`Delta*, AssistantDone, (ToolCall, [approval_request], ToolResult)*` repeating
until an AssistantDone with no tool calls ends the turn. Tool events are keyed
by the provider's tool-call id; the client reducer needs no other message ids.

- [x] `internal/agent/agent.go` as above; loop body is a faithful lift of
      main.go:117-151 (append semantics, error → abort turn).
- [x] `internal/tools`: add `SchemasWith(extra []Tool) []openrouter.Tool` and
      `ExecuteWith(extra []Tool, call, approve) openrouter.Message`; existing
      `Schemas`/`Execute` become thin wrappers over them with no extras. The
      wrapping semantics (decline message, error message, unknown tool) stay in
      exactly one place.
- [x] `cmd/evie/repl.go`: REPL becomes an `Events` impl wrapping smoothPrinter
      + the `[calling %s]` print + the terminal y/N approver. Behavior parity:
      same prompts, same prints, same decline text.
- [x] `main.go` dispatches: `evie` → REPL, `evie serve` → server (Part 2).
      `tools.Warm()` runs in both paths. Env loading for both paths:
      `godotenv.Load` from `~/.evie/.env` if present, else ambient env only
      (the old cwd-relative `../../.env` load is dropped — a binary on PATH has
      no cwd guarantee). Document copying the repo `.env` there once.
- [x] Unit tests with a fake `Client` (scripted responses): plain answer;
      tool-call round trip; gated tool approved/declined; extra-tool call;
      request error mid-turn. Assert on the messages slice and recorded events,
      not stdout.

## Part 2 — the server

- [x] `serve.go`: `http.Server` on `EVIE_ADDR` (default `127.0.0.1:6687`).
      **Refuse to start on a non-loopback address** with an error naming the
      missing auth story (per docs/decisions.md). Log the URL on start.
- [x] **Cross-origin defense** (bash is ungated — a drive-by form POST or DNS
      rebinding against localhost is code execution): every `/api/*` handler
      requires `Content-Type: application/json` exactly, and rejects requests
      whose `Origin` (when present) or `Host` is not the server's own
      loopback origin. 403 on failure. httptest-covered.
- [ ] Static: `//go:embed all:ui/dist`, `fs.Sub`, serve `index.html` for
      unknown non-`/api` paths (SPA fallback).
- [x] One global `*agent.Session` created at start (single conversation, v1).
      Structured so "sessions map keyed by id" is an add, not a rewrite.
- [x] `POST /api/chat` `{"message": string}` → responds `text/event-stream`,
      streaming the turn's events (vocabulary below). `turn_done` always
      terminates the stream — after an `error` event too. If a turn is already
      running in this process, or its durable session lease is held by another
      process before output begins → `409` `{"error": "..."}`.
- [x] `POST /api/approve` `{"id": string, "approve": bool}` → resolves a
      pending approval. Unknown/expired id → `404` `{"error": "..."}`.
- [x] Approval plumbing: the approver registers a pending approval (server-
      generated random id), emits `approval_request` on the stream, then blocks
      until `/api/approve` resolves it **or** the SSE request's context is done
      (client gone → declined). Declined-by-disconnect gets its own tool-result
      text ("approval request expired — David never saw it") so the model isn't
      told David declined something he never saw. Requires a small change to
      the decline text plumbing: `ExecuteWith` uses the standard decline text;
      the disconnect case is produced by the serve-side approver wrapper
      returning false plus serve substituting the message — simplest coherent
      mechanism wins, but the two texts must differ.
- [x] Client disconnect mid-turn: the turn runs to completion server-side
      (ChatStream has no cancellation — out of scope); pending approvals
      decline as above; events after disconnect are dropped.

### SSE event vocabulary (server → client)

Each event: `event: <type>` + `data: <one-line JSON>`. Mirror the parsing idiom
of `internal/openrouter/client.go:40-129` when writing the encoder. Order
follows the Events contract in Part 1.

| type               | data                                    |
|--------------------|-----------------------------------------|
| `delta`            | `{"text": string}`                      |
| `reasoning`        | `{"text": string}`                      |
| `reasoning_done`   | `{}`                                    |
| `assistant_done`   | `{"content": string}` (may be `""`)     |
| `tool_call`        | `{"id", "name", "args"}`                |
| `tool_result`      | `{"id", "content", "isError": bool}`    |
| `approval_request` | `{"id", "name", "args"}`                |
| `board_start`      | `{"id", "type", "title"}` (model-chosen id)  |
| `board_delta`      | `{"id", "text"}`                        |
| `board_end`        | `{"id"}`                                |
| `critic_note`      | `{"boardId", "note"}`                   |
| `response_discarded` | `{"reason", "message"}`              |
| `error`            | `{"message": string}`                   |
| `turn_done`        | `{}`                                    |

`response_discarded.reason` is one of `provider_error`,
`provider_response_invalid`, `caller_cancelled`, `caller_deadline_exceeded`,
`lease_lost`, `lease_heartbeat_failed`, or `assistant_persistence_failed`; its
message is exactly
`Response interrupted; streamed text was not saved.` It is emitted only when
reasoning or response content was rendered but the corresponding assistant event
did not commit. If reasoning is open, ordering is `reasoning_done`,
`response_discarded`, `error`, `turn_done`. A final committed no-tool assistant
event is durable success and never gets a discarded marker even if its later
frontend callback is cancelled.

The browser flushes buffered deltas before reducing `response_discarded`. It
keeps partial assistant text visible and renders the fixed message inline below
that transcript item as an interrupted/not-saved warning. If only reasoning was
visible, it closes the reasoning item and adds the same message as a standalone
transcript warning. `turn_done` preserves this discarded state rather than
marking it ordinarily complete; the later `error` event may also populate the
existing banner.

- [x] `httptest` tests: full turn streams the right event sequence (fake
      Client, incl. an empty tool-only assistant message); 409 when busy;
      approval approve/decline/disconnect paths; origin/content-type rejection;
      SPA fallback serves index.html; `/api/*` never falls back.

## Part 3 — the whiteboard (inline streamed boards)

The model draws by emitting tagged blocks **inline in its normal streamed
response** — no tool call, no extra round trip. Decision + research trail in
`serve.decisions.md` (2026-08-04): this is how Claude Artifacts works, and
what LibreChat/Open WebUI converged on; the tool-call route pays a model
round-trip per display action.

```
<whiteboard id="loop-1" type="mermaid" title="The agent loop">
...body streams here...
</whiteboard>
```

- `id` is model-chosen; re-emitting a used id **replaces** that board —
  Canvas-style iteration without a tool.
- `type`: `svg` | `mermaid` | `html` | `markdown` | `image`. Free-stroke
  drawing IS `svg` — an SVG path is literally pen coordinates, so the model
  draws by streaming path/shape elements and the panel renders strokes as
  they arrive. Mermaid is the structured-diagram alternative, not the
  drawing surface.
- The system prompt gains the whiteboard convention (when/how to use it, the
  tag syntax, "boards are your whiteboard — teach on them"). The broader
  system-prompt rewrite stays a separate backlog item; serve appends the
  whiteboard section to the existing prompt.

- [ ] Streaming scanner in `internal/web`: a state machine over the delta
      stream that detects open/close tags **even when a tag is split across
      deltas**, suppresses board bodies from `delta` events (chat shows a
      board chip instead), and emits `board_start`/`board_delta`/`board_end`.
      Unit-tested hard: tag split at every byte boundary, `<` inside bodies,
      unclosed tag at turn end (auto-close, mark truncated), malformed
      attributes (treat as plain text, don't eat the content).
- [ ] Tags stay in `messages` untouched — the model re-reads and revises its
      own boards on later turns.
- [ ] REPL: no scanner; tags print raw. Acceptable — the terminal is not the
      rich surface.
- [ ] The per-turn extras seam from Part 1 goes unused for now — it stays for
      future stateful board ops (a tool the model calls when it must *know*
      an action succeeded).

## Part 3b — the critic ("the eyes")

The model can't see while it draws; the critic sees for it, after each board
renders. The browser is the renderer we already have, so it is also the
camera:

- [ ] On `board_end`, the UI rasterizes the rendered board (canvas snapshot)
      and `POST /api/board-snapshot {boardId, png(base64)}`.
- [ ] The server runs the critic in a goroutine: sends the snapshot + the
      board's source + the turn's intent to a vision-capable model
      (`EVIE_CRITIC_MODEL`, must support images; via the same openrouter
      client — needs image-content support in the request schema, a small
      additive change). Prompt: "does this render correctly and serve the
      stated intent? Answer OK or describe what's wrong."
- [ ] A non-OK verdict is (a) emitted as `critic_note` on the next available
      stream so David sees it, and (b) appended to `messages` as a user-role
      note ("[critic] board loop-1: arrow B→C overlaps the label") before the
      next turn, so the model corrects itself — same feedback loop as tool
      errors. OK verdicts are dropped silently.
- [ ] The critic never blocks the turn: it races the conversation, and its
      note lands whenever it lands (mid-turn notes queue until the turn ends;
      the Session lock stays single-writer in this feature).

## Part 4 — the frontend (`internal/web/ui/`)

Scaffold: `npm create vite@latest` (react-ts), Tailwind, assistant-ui,
Streamdown, diff viewer. No router. Dev mode: `vite dev` proxies `/api` to the
Go server (`server.proxy` in vite config).

**Two spikes before building Part 4 proper** (half a page of throwaway code
each; record outcomes in `serve.decisions.md`):

1. assistant-ui `ExternalStoreRuntime` rendering a fake thread containing a
   custom tool card and an approval card. If its message model fights our
   shapes, fall back to hand-rolled chat components (Streamdown + Tailwind) —
   the store/reducer design below is identical either way.
2. Diff rendering: verify `react-diff-view` can render a jsdiff
   `createTwoFilesPatch` of two snippets; if not, use `@git-diff-view/react`
   or a minimal hand-rolled two-pane. Pick whichever renders correctly with
   least fuss.

- [ ] **SSE client**: `EventSource` can't POST, so the client reads the fetch
      `ReadableStream` and parses `event:`/`data:` lines by hand (~40 lines,
      mirroring the Go client's parser; no dependency).
- [ ] **Layout**: chat pane + collapsible whiteboard side panel. Panel opens
      automatically on the first `board_start` of a turn.
- [ ] **Chat**: one message store fed by the SSE reader; deltas are buffered in
      a ref and flushed to state on a ~50ms single shared timer (never setState
      per token). Streamdown renders assistant markdown. Composer disabled
      while a turn is streaming (server enforces with 409 anyway).
- [ ] **Tool calls**: collapsed cards (name + args preview, expandable result).
      Errors visibly marked.
- [ ] **Approvals**: `approval_request` renders an inline card with tool name,
      raw args (pretty-printed JSON), Approve / Decline → `POST /api/approve`.
      Special case `edit_file` args — fields are `path`, `old_string`,
      `new_string` (`internal/tools/file.go`) — render a real diff (spike 2)
      instead of raw JSON. The card stays visible after resolution, marked with
      the outcome.
- [ ] **Whiteboard**: boards render progressively from `board_*` events,
      newest-first, replace-by-id. Per type: `svg` — strokes append live as
      elements arrive (parse the partial SVG, render what's complete; this is
      the free-draw surface), then a final sanitized render on `board_end`
      via data-URI `<img>` (blocks scripts/external loads without a sanitizer
      dep); `mermaid` — re-render debounced ~300ms and on `board_end` (partial
      mermaid rarely parses); `markdown` via Streamdown live; `html` via
      sandboxed iframe `srcdoc` `sandbox=""` on `board_end`; `image` on end.
      Chat pane shows a board chip where the tag was (click → focus board).
      Remote URLs in `image`/`html` are allowed in v1 — accepted exfil risk,
      recorded in decisions. Client-side clear button. Boards are
      per-page-load (no persistence, v1).
- [ ] **Snapshot for the critic**: on `board_end`, rasterize the rendered
      board to PNG (svg → canvas draw; html/mermaid via same path; skip
      `image` boards) and POST `/api/board-snapshot`. `critic_note` events
      render as a subtle inline notice ("the eyes caught something") attached
      to the board and in chat.
- [ ] **Errors**: `error` events and failed fetches surface as a dismissible
      banner, not console-only.

Frontend testing: the SSE→store reducer gets vitest coverage (event sequences →
expected message list, incl. empty assistant messages and out-of-band
approvals). Visual components are covered by the e2e demo instead.

## Build / dev workflow

- [ ] `internal/web/ui/dist/` fully gitignored (no committed placeholder — a
      placeholder gets clobbered by every build and leaves the tree dirty).
      Consequence: `go build ./cmd/evie` requires one prior
      `npm --prefix internal/web/ui run build`; the embed error names the fix
      closely enough, and CLAUDE.md documents it. Other tools (`todo`,
      `finance`) build independently and are unaffected.
- [ ] Deploy documented in CLAUDE.md:
      `npm --prefix internal/web/ui run build && go build -o ~/go/bin/evie ./cmd/evie`.

## Out of scope (v1 — each is its own future feature)

- Multi-writer Session: mid-stream interruption ("student asks while teacher
  writes") and parallel answerers sharing `[]messages` — needs relaxing the
  single-writer lock + transcript merge rules. Its own spec later; the critic
  deliberately stays within single-writer bounds (notes queue until turn end).
- Stateful board *tools* (ops the model must confirm succeeded) — the extras
  seam is ready when needed.
- Persistence / session resume / multi-conversation. In-memory only. **Page
  reload shows an empty UI over the still-live conversation** — known gap; a
  history endpoint arrives with the persistence feature, don't build it now.
- Multi-agent orchestration UI (the 100s-of-agents dashboard). v1 is one
  session; the perf patterns (coalescing, block-memoized markdown) are the
  groundwork.
- Stop/cancel mid-turn (needs context plumbed through ChatStream).
- Auth. Loopback only, enforced; revisit before any non-loopback bind.
- Context trimming, token accounting, model switching UI.
- Mobile layout polish.

## Codebase context (read before writing code)

- The loop being lifted: `cmd/evie/main.go:103-153`. Registry + dispatch:
  `internal/tools/registry.go` (wrapping semantics live in `Execute`).
- SSE parsing to mirror when encoding: `internal/openrouter/client.go:40-129`.
- HTTP style exemplar: `internal/tools/webfetch.go` (timeouts, limits);
  server-side test exemplar: `internal/tools/webfetch_test.go` (httptest).
- Conventions (CLAUDE.md): errors wrapped `%w`; domain layer silent, frontends
  own output; env-var config with defaults; subcommand dispatch on `os.Args[1]`.
- Decisions that bind this feature: `docs/decisions.md` — "reads wide, writes
  gated" (the approval UI is load-bearing, show raw args), "secrets never enter
  messages"; `cmd/evie/docs/done/file-tools.decisions.md` — edit_file's
  safety story assumes David sees before/after (hence the diff view);
  `cmd/evie/docs/done/bash.decisions.md` — bash stays ungated (why the
  cross-origin defense in Part 2 is mandatory).
- Frontend/stack rationale: `serve.decisions.md` (React/Vite/assistant-ui/
  Streamdown choices, and why not Next.js/Turbopack/Solid).

## End-to-end verification (must actually run)

1. `npm --prefix internal/web/ui run build && go build -o ~/go/bin/evie ./cmd/evie`
2. `evie serve` → open the printed URL.
3. Send: "read cmd/evie/main.go and show me a mermaid diagram of the agent
   loop on the whiteboard" → streaming text in chat, a `read_file` tool card,
   a board chip in chat, and a rendered mermaid diagram in the panel.
3b. Send: "draw me a simple house, free-hand, on the whiteboard" → an `svg`
   board whose strokes appear progressively; on completion a snapshot POST
   fires (verify in server log) and any critic note renders.
4. Send: "in my scratch file <path>, change X to Y using edit_file" → approval
   card with a rendered diff → Approve → file actually changed on disk (verify
   with `cat`); repeat with Decline → file untouched, model told.
5. Cross-origin check: `curl -X POST -H 'Origin: http://evil.example' -H
   'Content-Type: application/json' http://127.0.0.1:6687/api/chat -d
   '{"message":"hi"}'` → 403.
6. `evie` (no args) still runs the REPL: one plain turn + one gated-tool
   decline behave exactly as before.
