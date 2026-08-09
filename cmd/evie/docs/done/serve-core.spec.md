# serve-core — the Go server (feature 2 of the serve umbrella)

The `internal/web` package: HTTP in, SSE out, approvals round-tripped. After
this lands, `evie serve` runs and a `curl` can hold a full streamed
conversation with Evie, approvals included — before any React exists. The UI
(ui-chat), whiteboard scanner, and critic are the next features; they plug
into what this builds.

## Design (from the design conversations, 2026-08-04)

- **One turn = one request.** `POST /api/chat` runs `session.Send`
  synchronously in the handler; the response is the SSE stream; the handler
  returning closes it. No background turn state to manage.
- **`sseEvents` is the web twin of `replEvents`**: an `agent.Events`
  implementation whose methods write `event:`/`data:` blocks to the
  `ResponseWriter` and `Flush()` after each — flushing is what makes it
  streaming rather than arrive-at-the-end.
- **Approvals are a channel handshake.** Per turn, the approver: random id →
  register `chan bool` in a mutex-guarded `pending` map → emit
  `approval_request` → block on `select { answer channel / request context }`.
  `/api/approve` looks up the id and sends into the channel. Context-done →
  decline with the "expired — David never saw it" text (distinct from the
  standard decline; the model must not be told David refused something he
  never saw).
- **`Server` struct, `Handler()` method.** Fields: session, pending map + mu,
  config. `Handler()` returns the mux so tests drive it via `httptest` with a
  fake `agent.Client` — no port, no real model.
- **Static serving ships with ui-chat, not here.** `ui/dist` doesn't exist
  yet and go:embed would break every `go build`. Root path serves a one-line
  placeholder ("evie serve is up — UI not built yet"). The embed lands with
  the ui-chat feature.
- **Cross-origin guard is not optional.** bash is ungated; a drive-by form
  POST or DNS-rebind against localhost is code execution. Every `/api/*`
  route: `Content-Type: application/json` exactly, and `Origin` (when
  present) / `Host` must match the server's loopback origin → else 403.

## Wire protocol

SSE events, each `event: <type>` + `data: <one-line JSON>` + blank line
(vocabulary and per-turn order defined in serve.spec.md; this feature emits
`delta`, `assistant_done`, `tool_call`, `tool_result`, `approval_request`,
`error`, `turn_done`). Client→server: `POST /api/chat {"message"}`,
`POST /api/approve {"id","approve"}`. Errors: 409 busy, 404 unknown approval
id, 403 guard, all `{"error": "..."}`.

## Stages

1. **sseEvents** — `internal/web/events.go`: the four `Events` methods +
   `TurnDone`/`Error`/`ApprovalRequest` writers, SSE encoding in one place,
   flush-per-event. Unit tests against `httptest.NewRecorder`: exact wire
   bytes, JSON escaping (newlines in deltas must stay one `data:` line).
2. **Server + /api/chat** — `internal/web/serve.go`: `Server` struct,
   `Handler()`, the guard middleware, chat handler (parse → `Send` with
   `sseEvents` → `turn_done`; `ErrBusy` → 409; `Send` error → `error` event
   then `turn_done`). httptest + fake Client: full-turn event sequence, busy,
   guard rejections, placeholder root.
3. **Approvals** — pending map, per-turn approver closure, `/api/approve`
   handler, context-done decline with the expired text. Tests: approve path,
   decline path, expired/unknown id 404, disconnect-declines (cancel the
   request context mid-approval).
4. **The door** — `web.Serve(session)`: `EVIE_ADDR` (default
   `127.0.0.1:6687`), refuse non-loopback with an error naming the missing
   auth story, log the URL; `main.go`'s `serve` case calls it. E2E demo via
   curl (below).

## Definition of done

- `go build ./...`, `go vet ./...`, `go test ./...` clean — output shown.
- Real e2e (real model, curl):
  - `curl -N -X POST -H 'Content-Type: application/json' -d '{"message":"what time is it"}' http://127.0.0.1:6687/api/chat`
    → streamed `delta`s, a `tool_call`/`tool_result` pair, `turn_done`.
  - A gated-tool turn: `approval_request` arrives, stream visibly pauses; from
    a second terminal `POST /api/approve {"approve":true}` → stream resumes,
    tool runs. Repeat with `false` → decline text reaches the model.
  - Guard: foreign-Origin POST → 403. Second concurrent chat → 409.
- Umbrella spec Part 2 checkboxes ticked (minus static embed, moved to
  ui-chat); decisions recorded in serve.decisions.md.

## Open questions

- None.
