# serve — decisions

- **2026-08-23 — approval visibility and turn cancellation stay separate.**
  The web chat request continues to launch the server-side turn from an
  independent root, so an SSE disconnect does not cancel provider or tool work.
  The request context owns only its pending approval visibility. Approval POST
  and disconnect remove the pending entry under the same lock: the first
  claimant wins. Approval-first honors David's decision even if disconnect is
  observed immediately afterward; disconnect-first expires the approval and a
  later POST receives 404. This preserves the existing whole-turn disconnect
  behavior while making the approval race deterministic and fail-closed.

- **2026-08-05 — approvers return `tools.Decision`, not bool** (serve-core,
  shipped — spec in docs/done/). A bool can't distinguish "David said no"
  from "David never saw it" (browser disconnect), and the model must be told
  different things. Triad: Approved / Declined / Expired; zero value Declined
  so gates fail closed. Rippled through `Send`'s approver param and the REPL
  y/N closure. Verified end-to-end: approvals pause a real curl stream and
  resume on `/api/approve`; disconnect mid-approval writes the expired text.

- **2026-08-04 — `ExecuteWith` returns `(openrouter.Message, bool)`.** The
  Events contract promises `ToolResult(..., isErr)`, but errors were folded
  into message text with no flag. The bool marks failures (tool error, unknown
  tool); a decline is the gate working, not a failure. `Execute` keeps its
  one-value signature. Surfaced while writing `Send` in feature 1
  (agent-extract, shipped — spec in docs/done/).

- **2026-08-03 — React 19 + Vite + TS, static SPA embedded via go:embed.**
  Surveyed AI UIs: everything is web tech; the ones pairing a non-Node backend
  (Open WebUI, LibreChat, Hollama, n8n, Flowise) all ship Vite SPA builds.
  Next.js rejected: it's a Node *server* framework — wrong shape next to a Go
  binary, and Electron-wrapping later favors static files (Vite) anyway.
  Turbopack rejected: not usable outside Next.js. Bundler choice doesn't affect
  runtime performance — only bundle size, where Vite/Rolldown's output measured
  smallest of the mainstream tools.

- **2026-08-03 — React over Solid/Svelte for the 100s-of-agents ambition.**
  Fine-grained reactivity's ~2x benchmark edge is the *unbatched* pathological
  case. Once token deltas are coalesced (shared flush, 40ms), render cost scales
  with flushes, not tokens, and the real bottlenecks (markdown parse, mermaid
  layout, DOM size) are framework-independent. Leaving React forfeits
  assistant-ui + Streamdown — the libraries that encode the hard streaming-
  markdown edge cases. Required patterns recorded in the spec: delta coalescing,
  block-memoized markdown (Streamdown), virtualization when the agent grid
  arrives.

- **2026-08-08 — paced text release inside the flush (smoothPrinter ported to
  the UI).** Plain coalescing renders whatever burst the network delivered:
  text jumps. `splitBatch` (useSession.ts) now gates delta/reasoning events to
  a per-flush character budget that grows with the backlog, so bursts render
  as steady typing and a deep queue catches up — one render per flush still,
  so the perf property above is untouched. `turn_done` (and the error path)
  dump the tail immediately, matching the REPL's `done()`.

- **2026-08-03 — custom minimal SSE protocol, not the AI SDK data-stream
  protocol.** Our two distinctive flows (approval round-trip, whiteboard `show`)
  aren't native to AI SDK's protocol anyway; the harness is the product, so the
  wire format is part of what David owns. assistant-ui consumes it through
  ExternalStoreRuntime (bring-your-own-backend path). Client→server stays plain
  POST; SSE downstream only — no WebSocket dependency, matching the repo's
  stdlib-first bias.

- **2026-08-04 — whiteboard is inline streamed tags, not a tool call**
  (supersedes 2026-08-03 "`show` tool" and "markup-only v1"). David flagged
  the latency: a tool call buffers the full args and costs a model round-trip
  per display action. Research confirmed: Claude Artifacts emits inline
  `<antArtifact>` tags parsed from the token stream (zero extra trips, content
  in history); LibreChat and Open WebUI converged on inline too; ChatGPT
  Canvas uses a tool but only feels live because OpenAI streams tool args —
  and JSON-escaping large markup measurably degrades model output vs raw text
  in fences. Ours: `<whiteboard id type title>` blocks, server-side streaming
  scanner, `board_start/delta/end` SSE events, replace-by-id for iteration.
  The extras seam from feature 1 stays for future stateful board ops.

- **2026-08-04 — free-stroke drawing is `svg`, in v1** (supersedes "deferred
  drawing API"). An SVG path is literally pen coordinates, so streaming path
  elements IS freehand drawing — no new medium needed. Strokes render
  progressively as elements complete. Coordinate math is where models are
  weakest, which is exactly why the critic ships alongside it.

- **2026-08-04 — the critic: browser as camera, vision model as eyes.** The
  model can't see mid-generation; correction is a feedback loop after render.
  Rather than headless rendering in Go (would need a browser engine), the UI —
  which already rendered the board — rasterizes it to PNG and posts it back;
  a goroutine sends snapshot + source + intent to `EVIE_CRITIC_MODEL`
  (vision-capable, via openrouter — needs additive image-content support in
  the request schema). Non-OK verdicts append to `messages` as a "[critic]"
  user-role note (same loop as tool errors) and surface as `critic_note`.
  Single-writer Session preserved: notes queue until the turn ends.
  Multi-writer (interruption, parallel answerers) is fenced to its own
  future spec.

- **2026-08-03 — spec-review round 1 rulings.** Cross-origin defense (Origin/
  Host check + strict JSON content type) is mandatory because bash is ungated —
  a drive-by form POST against localhost would be code execution. Extra tools
  are per-`Send`, not per-`Session`, so `show` can close over the current
  turn's SSE encoder. `web/dist` is fully gitignored (a committed placeholder
  gets clobbered by every build); `go build ./cmd/evie` therefore needs one
  prior npm build. Env loading moves to `~/.evie/.env` + ambient (cwd-relative
  `.env` can't work for a binary on PATH). Remote URLs in `show` image/html are
  allowed in v1 — a conscious, recorded exfil-risk acceptance, same philosophy
  as ungated bash. Server refuses non-loopback bind until an auth story exists.

- **2026-08-03 — in-memory single session for v1.** Persistence (transcripts,
  memory, graph maps) is wanted and will come, but one step at a time; the
  server holds one `*agent.Session` structured so a keyed session map is an
  add, not a rewrite.
