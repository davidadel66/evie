# ui-chat — the React frontend (feature 3 of the serve umbrella)

Status: draft (pending David's approval)

The browser half of `evie serve`: the shell, the chat column, tool cards, the
approval card with a diff, the composer. Plus the two Go pieces deferred from
serve-core — `go:embed all:ui/dist` and static serving — so `evie serve` finally
opens a real page. After this lands, everything serve-core emits is visible and
operable in a browser; the whiteboard and critic (feature 4) plug into the same
store.

Implements the imported design `Moussa App.dc.html` (claude-design project
"Moussa Eve desktop UI"), extracted to `/tmp/evie-design.html`. The design
predates the rename: every "Moussa"/"Eve" string becomes **Evie**, the logo
mark stays a single letter (`E`), and the `· eve` subtitle drops.

## What the design specifies

Dark desktop app, 13px base, IBM Plex Sans / IBM Plex Mono / Caveat (whiteboard
handwriting). Tokens, verbatim from the design:

| Role | Value |
|---|---|
| app bg / top bar | `#0e1113` / `#0b0e0f` |
| borders | `#1d2326`, `#20272a`, `#242c30`, `#2a3336` |
| text primary / body / muted / faint | `#e2e6e3` / `#c9cfcb` / `#8b9491` / `#4a5350`, `#5b6663`, `#68716e` |
| surfaces | `#121618` (cards), `#1b2225` (user bubble), `#0b0e10` (code), `#0f1312` (panel), `#131a18` (artifact) |
| teal accent | `#4fb8a5`, hover `#6fd0be`, deep `#2b4a42` / `#2f6b5d`, border `#26433d` |
| amber (pending / attention) | `#d9a04a`, text `#e8c98a`, bg `#1b1810`, border `#2a2415`/`#6b5527` |
| red (error / removed) | `#d96b6b`, text `#d9a0a0`/`#e0a3a3`, bg `#161113`/`#1c1113`, border `#4a2a2e`/`#3a2226` |
| green (ok / added) | `#5fae7d`, text `#a8d4b5` |
| radii | 3–12px (bubbles `10px 10px 3px 10px`, cards 7–9px) |

Layout: 46px top bar (logo, Chat/Whiteboard/Reports tabs, status dot right) →
optional connection banner → tab body. Chat body = message column (`flex:1`,
`max-width:min(720px,100%)` per assistant message, user bubbles `62%`
right-aligned) + composer, beside a 620px artifacts panel that collapses to a
34px vertical rail.

Message kinds in the design: user bubble, thinking block, tool card
(collapsible, mono, status chip right), assistant markdown (with fenced code
blocks carrying a filename header + `copy`), approval card (pending → approved
/ declined), errored tool card, 3D-model attachment chip, streaming caret
(`animation:blink`).

## Scope: what ships, what doesn't

**Ships:** shell + tabs + status dot, connection banner, chat column (user,
assistant markdown, tool cards incl. error state, streaming caret), approval
card with diff and Y/N hotkeys, composer, artifacts panel as rail + empty
state, static serving + embed + build workflow.

**Deferred, with reasons** — the design shows these but the backend has nothing
to feed them; building empty chrome now means building it twice:

- **Thinking block.** ~~Nothing in the stream carries reasoning~~ — shipped
  2026-08-08 by the reasoning spec (`docs/done/reasoning.spec.md`): `reasoning`
  / `reasoning_done` events, a `reasoning` item kind, and `Reasoning.tsx`.
- **Artifact cards** (mermaid / chart / markdown, focus view, prev/next).
  Nothing emits artifacts until the whiteboard feature. The panel ships as the
  collapsed rail plus the "Nothing pinned yet" empty state, so the layout is
  real and feature 4 fills it in.
- **Whiteboard tab, Reports tab, 3D viewer overlay, model attachment chip.**
  Tabs render and are clickable; their bodies are a centered one-line notice
  naming the feature that will fill them. Nothing else.
- **Absolute line numbers in the diff** — see the diff decision below.

## Stack decisions (two amendments to serve.decisions.md, record them)

1. **Hand-rolled chat components, not assistant-ui.** The Part 4 spike is
   cancelled, not run: the design is a fully specified bespoke layout, and our
   SSE vocabulary is custom, so `ExternalStoreRuntime` would be an adapter over
   a store we have to write anyway, plus a component library whose every visual
   is overridden. Streamdown stays for assistant markdown (streaming-safe
   unterminated fences, block memoization, code highlighting) — that's the part
   with real edge cases. If Streamdown's mermaid/katex weight or API fights us,
   fall back to `react-markdown` + `remark-gfm` behind the same
   `<Markdown text streaming>` wrapper, which is the only file that knows.
2. **Tailwind v4 via `@tailwindcss/vite`, tokens in an `@theme` block.** No
   config file. Design tokens become named utilities (`bg-surface`,
   `text-muted`, `border-hair`) so no arbitrary hex litters JSX; layout stays
   utility classes, matching the design's flex/gap structure line for line.
   Font faces, the `evepulse`/`blink` keyframes and the scrollbar rules go in
   one `theme.css` alongside the `@theme` block.

Deps (frontend only; the Go side stays stdlib): `react`, `react-dom`,
`typescript`, `vite`, `@vitejs/plugin-react`, `tailwindcss`,
`@tailwindcss/vite`, `streamdown`, `diff` (jsdiff, for the approval diff),
`vitest`. Fonts load from Google Fonts as the design does — a known offline
gap; self-hosting is a later polish item.

## Files

```
internal/web/
  static.go          //go:embed all:ui/dist + serving (replaces handleRoot)
  static_test.go
  ui/
    index.html  package.json  tsconfig.json  vite.config.ts
    src/
      main.tsx
      theme.css          @theme tokens, fonts, keyframes, scrollbar
      App.tsx            shell: top bar, tabs, banner, tab bodies
      api/stream.ts      POST /api/chat + SSE line parser → callbacks
      api/approve.ts     POST /api/approve
      store/events.ts    SSE event types (mirrors the Go vocabulary)
      store/reducer.ts   pure: (items, event) → items
      store/reducer.test.ts
      store/useSession.ts  hook: reducer + delta buffer + 50ms flush + status
      chat/Chat.tsx  Message.tsx  ToolCard.tsx  ApprovalCard.tsx
      chat/Diff.tsx  Composer.tsx  Markdown.tsx
      artifacts/Panel.tsx  (rail + empty state)
```

## The store model

One flat list, appended in stream order. Approvals are **not** separate items —
serve-core emits `tool_call` → `approval_request` → (answer) → `tool_result`,
and the design shows the pending card resolving into a compact tool row, so the
approval lives on the tool item it gates.

```ts
type Item =
  | { kind: 'user'; key: string; text: string }
  | { kind: 'assistant'; key: string; text: string; streaming: boolean }
  | { kind: 'tool'; key: string; id: string; name: string; args: string;
      approval?: { reqId: string; state: 'pending' | 'approved' | 'declined' | 'expired' };
      result?: string; isErr?: boolean; startedAt: number; ms?: number }
```

Reducer rules (each one gets a test):

- `delta` → append to the trailing assistant item; create one (`streaming:true`)
  if the trailing item isn't a streaming assistant.
- `assistant_done` → `streaming:false`. **If the item has no text, remove it**:
  `Events.AssistantDone` fires for every assistant message including tool-only
  ones, and the design has no empty bubbles.
- `tool_call` → push a tool item, `startedAt` = now.
- `approval_request` → attach `{reqId, state:'pending'}` to the newest tool item
  with a matching `name` and no `result`. No match (out-of-band, e.g. after a
  reload) → attach to a synthetic tool item so the card is still actionable.
- `tool_result` → set `result`/`isErr`, `ms = now - startedAt`. Leaves the
  approval state alone: the client set it when the user clicked, and a still
  `pending` state here means the server resolved it without us (expiry).
- `turn_done` → any `streaming` assistant closes; any `pending` approval becomes
  `expired`.
- `error` → status becomes `error` with the message; the item list is untouched
  (banner surface, per the design).

`useSession` owns everything stateful the reducer can't: **deltas accumulate in
a ref and flush on a single shared ~50ms timer** (never `setState` per token —
this is the load-bearing perf pattern from serve.decisions.md), a
`status: 'idle' | 'streaming' | 'error'` for the dot and banner, and
`send(text)` / `answer(reqId, ok)`. Approve/decline updates the item optimistically
and reverts to `expired` on a 404 (the id already timed out server-side).

## Wire client

`EventSource` can't POST, so `api/stream.ts` uses `fetch` and reads
`res.body` as a `ReadableStream`, decoding with `TextDecoderStream` and
splitting on `\n\n`, then `event:` / `data:` per block — ~40 lines mirroring
`internal/openrouter/client.go:40-129`, no dependency. Contract:

- Non-2xx → parse `{"error"}` and raise it (409 busy and 403 guard both land
  here as a banner).
- Stream ends without `turn_done` → treat as a dropped connection: banner
  "Lost connection to the Evie server", status `error`. The design's "retrying
  in 3s" copy is honest only with a retry, so v1 shows **Retry now** (re-sends
  nothing; it just clears the banner and re-enables the composer) and drops the
  countdown text. Auto-reconnect belongs with persistence, since without a
  history endpoint there's nothing to reconnect *to*.
- Unknown event names are ignored, not errors — feature 4 adds `board_*` and
  `critic_note` to the same stream.

## The approval diff

`edit_file` args are `path`, `old_string`, `new_string`
(`internal/tools/file.go`). The approval payload does **not** carry the file, so
absolute line numbers are unknowable — the design's `14/15/16` gutter would be
invented. `Diff.tsx` renders a jsdiff `diffLines(old_string, new_string)` with a
`−`/`+` gutter instead of line numbers, keeping the design's exact row colors
(`rgba(217,107,107,.08)` / `rgba(95,174,125,.09)`) and mono type. Real line
numbers need the file contents in the approval event — a serve-core change,
noted for later, not smuggled in here.

`edit_db` (the other gated tool) has no diff shape: render `statement` in a mono
block with the `db` name in the header. Any other gated tool: pretty-printed
JSON args. The card keeps the design's amber pending chrome, `Approve Y` /
`Decline N` buttons, and the "Evie is paused until you decide" line; on resolve
it collapses to the compact approved/declined row with the tool name and
outcome, and stays in the transcript.

Hotkeys, from the design's key handler: `y` / `n` resolve the pending approval
when the Chat tab is focused and the event target isn't a `TEXTAREA`/`INPUT`.

## Tool card details

Header: wrench icon, mono tool name, one-line args preview (ellipsised), status
chip, chevron. Body (collapsed by default) is the raw result in a `<pre>`.
Status chip: `✓ {ms}ms` from the client's own `startedAt`/`ms` timing —
truthful and free. The design's `412 rows` needs structured tool metadata the
registry doesn't return; omitted. `isErr` switches the card to the red variant
(`#161113` bg, `#4a2a2e` border, `failed` chip) and auto-expands.

## Static serving (Go)

`internal/web/static.go` replaces `handleRoot`:

- `//go:embed all:ui/dist` (the `all:` prefix keeps Vite's dotfile-ish assets),
  `fs.Sub` to the `ui/dist` root, `http.FileServerFS`.
- Path exists in the embedded FS → serve it. Hashed `/assets/*` get
  `Cache-Control: public, max-age=31536000, immutable`; `index.html` gets
  `no-store` so a rebuilt UI is picked up on reload.
- Path doesn't exist → serve `index.html` (SPA fallback; harmless with no
  router and correct if one ever lands). `/api/*` never reaches here — the mux
  routes those first.
- Non-GET/HEAD on a static path → 405.

`static_test.go`: index served at `/`, an asset served with immutable caching,
an unknown path falls back to index, `/api/chat` still routed (guard first).
Tests read the real embedded dist, so **the test needs a prior npm build** —
same constraint as `go build`, documented in CLAUDE.md.

Dev mode: `vite dev` on 5173 with `server.proxy['/api'] → http://127.0.0.1:6687`.
The guard already accepts a `localhost:5173` Origin (covered by a serve-core
test), so `evie serve` + `vite dev` side by side works with hot reload.

## Stages

1. **Scaffold + embed.** Vite react-ts app, Tailwind v4 + `theme.css` tokens,
   dev proxy, `.gitignore` for `internal/web/ui/dist/` and `node_modules/`;
   `static.go` + `static_test.go`. Demo: `npm run build && go build ./cmd/evie
   && evie serve` serves a token-styled placeholder page at the printed URL,
   and `vite dev` shows the same page with HMR.
2. **Wire client + store.** `api/stream.ts`, `api/approve.ts`,
   `store/events.ts`, `reducer.ts`, `useSession.ts`, `reducer.test.ts`
   (vitest). Demo: a temporary debug view dumping items as JSON while a real
   turn streams from the live server — proves the protocol before any pixels.
3. **Shell + chat.** `App.tsx` (top bar, tabs, status dot, deferred-tab
   notices), `Chat.tsx`, `Message.tsx`, `Markdown.tsx`, `ToolCard.tsx`,
   `Composer.tsx`, artifacts rail + empty state. Demo: a real conversation with
   a tool call, rendered to the design.
4. **Approvals + banner.** `ApprovalCard.tsx`, `Diff.tsx`, Y/N hotkeys,
   connection banner + Retry. Demo: a live gated `edit_file` approved (file
   changes on disk) and declined (untouched).
5. **Close-out.** CLAUDE.md build/deploy lines, umbrella checkboxes, decisions
   recorded, spec moved to `docs/done/`.

## Out of scope (beyond the umbrella's existing fences)

- Session history / reload recovery. A reload still shows an empty UI over the
  live conversation — the umbrella's known gap, unchanged here.
- Auto-reconnect, message editing/retry, copy-conversation, scroll-to-bottom
  button, virtualization (the perf groundwork is delta coalescing; virtualization
  arrives with the agent grid).
- Self-hosted fonts, light theme, mobile layout, accessibility audit.

## Codebase context (read before writing code)

- The wire source of truth: `internal/web/events.go` (exact event names and
  payload fields), `internal/web/serve.go` (routes, guard, 409/403/400 bodies),
  `internal/web/approvals.go` (approval id lifecycle, expiry semantics).
- Event ordering within a turn: `internal/agent/agent.go` `Send`.
- Gated tool arg shapes: `internal/tools/file.go:293` (`edit_file`),
  `internal/tools/db.go:112` (`edit_db`).
- SSE parser to mirror: `internal/openrouter/client.go:40-129`.
- Design source: `/tmp/evie-design.html` — chat tab at line 45, tool card 67,
  approval states 96/118/128, errored tool 140, composer 177, artifacts panel
  185, key handler 546.
- Conventions: CLAUDE.md (build/deploy, feature doc naming); frontends own all
  user-facing output, the domain layer stays silent.

## End-to-end verification (must actually run)

1. `npm --prefix internal/web/ui run build && go build -o ~/go/bin/evie ./cmd/evie`
2. `evie serve` → open the printed URL: shell renders, status dot idle.
3. Ask a plain question → text streams token by token with the blink caret,
   caret clears on completion.
4. "What time is it?" → a `get_time` tool card with a `✓ {ms}ms` chip that
   expands to the raw result.
5. "In <scratch file>, change X to Y with edit_file" → approval card with a
   rendered diff → `Y` → file changed on disk (`cat`), card collapses to the
   approved row. Repeat with `N` → file untouched, Evie says so.
6. Send a second message while the first turn streams → composer is disabled;
   force it via curl → 409 surfaces as a banner, not a console error.
7. Kill the server mid-stream → "Lost connection" banner, Retry clears it.
8. `evie` (no args) still runs the REPL unchanged.
