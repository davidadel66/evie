# reasoning — stream the model's thinking (feature 4 of the serve umbrella)

Status: approved, not started — build via `/copilot` (see "Who writes what")

Reasoning models spend most of their time thinking before the first content
token. Today that shows as dead air: David sends a prompt and waits with only a
pulsing "Evie is thinking…" for company. This feature carries the thinking
itself through the harness — provider → `ChatStream` → `agent.Events` → SSE →
a collapsible block in the chat — so the wait becomes something to read.

It also closes a correctness gap that exists today, unnoticed: reasoning is
silently dropped from the transcript, which some providers reject on the
tool-call round trip (see "Round-tripping" below).

Scope is the whole vertical slice, terminal REPL included — `internal/agent`
sits under both frontends and neither can be left behind.

## What the provider actually sends (verified, 2026-08-06)

Probed live against `moonshotai/kimi-k3` via curl. Each streamed chunk's
`choices[0].delta` carries, alongside `content`:

```json
{
  "reasoning": "The",
  "reasoning_details": [
    { "type": "reasoning.text", "text": "The", "format": "unknown", "index": 0 }
  ]
}
```

Three findings that shape the design:

1. **`reasoning` is a plain string delta**, fragment by fragment, exactly like
   `content`. That's all the UI needs.
2. **`reasoning_details` is the structured twin** — an array of typed, indexed
   parts. Some providers put a signature or an encrypted blob here rather than
   plain text (`reasoning.encrypted`), and require it echoed back verbatim.
3. **Kimi streams reasoning without being asked.** Other models don't. So the
   request must opt in explicitly, and the code must not assume presence.

## Design

- **`reasoning` is a request field, not a per-call argument.** `ChatRequest`
  gains `Reasoning *ReasoningConfig`; `internal/agent` sets it once from
  `EVIE_REASONING` (`on` default / `off` / `high` / `medium` / `low`). A pointer
  so the zero value omits the key entirely — a model that doesn't support
  reasoning must see the request it sees today.
- **`ChatStream` grows one callback, it does not change shape.** Today's
  signature is `ChatStream(req, onDelta func(string))`. Adding a second
  positional func would make every call site read
  `ChatStream(req, nil, nil)`. Instead the callbacks become one struct:

  ```go
  type StreamHandlers struct {
      OnContent   func(string)
      OnReasoning func(string)
  }
  func (c *Client) ChatStream(r ChatRequest, h StreamHandlers) (ChatResponse, error)
  ```

  A zero `StreamHandlers` streams nothing and assembles normally, so the
  non-streaming callers are unaffected. Both fields are nil-checked at the call
  site, as `onDelta` is today.
- **`Events` gains two methods, mirroring the content pair.**
  `Reasoning(text string)` for each fragment and `ReasoningDone(content string)`
  when the thinking ends — which is the moment the first content delta or tool
  call arrives, not a separate provider signal. `Session.Send` detects that
  transition and fires `ReasoningDone` exactly once per assistant message,
  including when the message is tool-only.
  Adding methods to `Events` breaks every implementer at compile time, which is
  what we want: `replEvents` and `sseEvents` are the only two, and both must
  make a deliberate choice.
- **Duration is measured, not modelled.** The design's "Thought for 6s" comes
  from a client-side clock in the UI (first `reasoning` event → `reasoning_done`),
  the same approach as the tool card's `ms`. No new server field.

### Round-tripping (the correctness half)

`Message` gains `Reasoning string` and `ReasoningDetails json.RawMessage`, both
`omitempty`, and `ChatStream` fills them on the assembled message. The reason is
not display — it's that `Session.Send` appends the assistant message back into
`s.messages` and re-sends it on the next tool iteration. Providers that require
their thinking blocks returned (Anthropic's signed blocks, any
`reasoning.encrypted` part) currently get a message with the thinking stripped.
`RawMessage` keeps it opaque: we store and return exactly what arrived without
modelling a schema that varies per provider.

**Fence:** reasoning is *not* rendered from history and *not* persisted beyond
the process (nothing is, v1). It rides along in `messages` for the provider's
benefit; the UI shows only what it saw stream.

### Why not fold reasoning into `delta`

Tempting — one event type, no `Events` change. Rejected: the UI must style it
differently (dim mono, collapsed by default) and must never feed it to the
markdown renderer, so the client would need a mode flag on every delta, which is
a second event type wearing a disguise. Worse, the transcript would interleave
thinking and prose as one string with no boundary.

## Wire protocol additions

Two SSE events, added to the vocabulary in `serve.spec.md`:

| event | payload | meaning |
|---|---|---|
| `reasoning` | `{"text": "..."}` | one fragment of thinking |
| `reasoning_done` | `{}` | thinking ended for this assistant message |

`reasoning_done` carries no content: the client already has every fragment, and
echoing the whole blob back would double the bytes for nothing. (`assistant_done`
carries content for a different reason — it also marks empty tool-only messages.)

Unknown events are already ignored by both the browser client and the Go SSE
parser, so an older UI against a newer server degrades to today's behaviour.

## UI

A collapsed block above the assistant message it belongs to, following the
design (`/tmp/evie-design.html:52-66`):

- Header: chevron, `Thought for 6s` (or `Thinking…` while live), and the
  duration in mono. Click toggles.
- Body: the raw reasoning text in `11.5px/1.75` mono, `--color-faint`, no
  markdown rendering — thinking is a scratchpad, and running it through a
  markdown parser makes half-finished lists jump around.
- **Auto-expanded while streaming, auto-collapsed when it ends.** That is the
  whole point of the feature: readable during the wait, out of the way after.
  A manual toggle sticks (an explicit click is never overridden).
- The store gets a `reasoning` item kind, so it lives in the transcript in wire
  order rather than being attached to an assistant item that may not exist yet
  (thinking often arrives before any content).
- `Waiting` (stage 4's stopgap) stays, but only until the first reasoning or
  content event — with reasoning on it will rarely be seen, and with reasoning
  off it's still the right answer.

REPL: `replEvents` prints reasoning dim-grey through the existing smooth
printer, prefixed once with `thinking…`, then a blank line at
`ReasoningDone`. Same channel, so no second printer goroutine.

## Files

```
internal/openrouter/schema.go     ReasoningConfig, Message fields, streamChunk delta
internal/openrouter/client.go     StreamHandlers, reasoning assembly
internal/openrouter/client_test.go
internal/agent/agent.go           Events additions, reasoning config, done-transition
internal/agent/agent_test.go
cmd/evie/repl.go                  dim thinking output
internal/web/events.go            Reasoning / ReasoningDone emitters
internal/web/events_test.go
internal/web/serve_test.go        wire-order assertion incl. reasoning
internal/web/ui/src/store/events.ts    two event types
internal/web/ui/src/store/reducer.ts   reasoning item kind + rules
internal/web/ui/src/store/reducer.test.ts
internal/web/ui/src/chat/Reasoning.tsx
internal/web/ui/src/chat/Chat.tsx      render the new kind
```

## Who writes what

This is the harness — the part David is here to learn — so the Go is **David's
to type**, tutor stance per `CLAUDE.md`: Claude shows code in chat, explains the
idiom and the tradeoff, and does **not** edit `.go` files. Claude reads, builds,
`go vet`s and runs tests to check the work, and captures the live fixture.

The browser client is not the learning target (David: *"since this is web dev we
can speed things up"*), so Claude writes the TS/TSX directly.

| stage | writes it | why |
|---|---|---|
| 1 provider | David | stream parsing + optional-field design is the lesson |
| 2 harness | David | `Events` as an interface seam, once-per-message state |
| 3a wire | David | SSE emitter + exact-bytes test, same shape as the others |
| 3b UI | Claude | React/Tailwind, not the learning target |
| 4 close-out | Claude | docs |

## Stages

Each stage: Claude explains the shape and the *why*, David types it, Claude
runs `go build` / `go vet` / `go test` and offers a real input→output demo
before moving on. One stage per go-ahead, never two.

1. **Provider layer.** `ReasoningConfig`, `Message.Reasoning` /
   `ReasoningDetails`, `StreamHandlers`, chunk parsing and assembly.
   *Teaching notes:* why `*ReasoningConfig` and not a value (a nil pointer with
   `omitempty` omits the key; a struct value can't); why `json.RawMessage`
   rather than a modelled type (opaque pass-through beats a schema that varies
   per provider); why a handler **struct** rather than a second positional
   func (call-site readability, and it extends without breaking anyone).
   Tests against an `httptest` SSE server replaying the captured Kimi chunks
   (Claude drops the real probe output into `testdata/`, David writes the test):
   reasoning fragments arrive in order, content and reasoning interleave
   correctly, the assembled message carries both, and a stream with no
   reasoning behaves exactly as today.
   Demo: a tiny `go run` that prints reasoning then content from the live API.
2. **Harness layer.** `Events.Reasoning` / `ReasoningDone`, the once-per-message
   done transition, `EVIE_REASONING` resolution, REPL rendering.
   *Teaching notes:* adding a method to `Events` is a deliberate compile-time
   break — find both implementers and choose for each; the done-transition is
   one bool of state on the iteration, and the interesting case is the tool-only
   message where no content delta ever arrives.
   Tests: the recorder sees `reasoning:` lines before `done:`, `ReasoningDone`
   fires exactly once even across a tool round trip, and fires not at all when
   the model sent no reasoning. Demo: `evie` in the terminal, thinking in dim
   grey.
3. **Wire + UI**, in two halves so David's Go and Claude's TS don't collide:
   - **3a (David).** `internal/web/events.go` emitters + exact-bytes tests, and
     the wire-order assertion in `serve_test.go`.
   - **3b (Claude).** `events.ts` union, reducer rules and tests,
     `Reasoning.tsx`, wiring in `Chat.tsx`.
   Demo: a real turn in the browser with a live-expanding thought block that
   collapses to "Thought for 6s".
4. **Close-out.** Umbrella checkboxes, decisions recorded, CLAUDE.md env var
   documented, spec moved to `docs/done/`.

## Out of scope

- Persisting reasoning across restarts (nothing persists yet).
- Showing reasoning for *past* turns after a reload (needs the history endpoint).
- Reasoning token accounting / cost display.
- Per-turn effort switching from the UI (env var only, for now).
- The design's "4 steps" step count — no provider field carries it, and
  splitting thinking into steps by heuristic would be inventing structure.

## Codebase context (read before writing code)

- `internal/openrouter/client.go:40-129` — the stream loop this extends;
  note the keepalive skip and `[DONE]` sentinel already handled.
- `internal/openrouter/schema.go:88-113` — `streamChunk` / `toolCallDelta`,
  the pattern the reasoning delta follows.
- `internal/agent/agent.go` — `Events`, and `Send`'s per-iteration loop where
  the done-transition lands.
- `cmd/evie/repl.go` — `smoothPrinter` and `replEvents`; reasoning reuses the
  same printer rather than starting a second.
- `internal/web/events.go` — one emitter per event, exact-bytes tested in
  `events_test.go`.
- `internal/web/ui/src/store/reducer.ts` — item kinds and the ordering rules;
  `reducer.test.ts` is the pattern for the new cases.
- Design reference: `/tmp/evie-design.html:52-66` (the thinking block).

## End-to-end verification (must actually run)

1. `go test ./... && npx vitest run` (in `internal/web/ui`).
2. `evie` in the terminal: ask something requiring thought → dim thinking text
   streams, then the answer.
3. `EVIE_REASONING=off evie` → no thinking, behaviour identical to today.
4. `evie serve`, browser: ask "what is 17*23, think it through" → the block
   expands live, then collapses to "Thought for Ns"; expanding it again shows
   the full text.
5. A gated `edit_file` turn with reasoning on → approval still works, and
   `ReasoningDone` fired once per assistant message (verify in the raw curl
   stream, not just the UI).
6. `curl` the raw stream and confirm `reasoning` events precede `delta` events
   and that `reasoning_done` appears exactly once per assistant message.
