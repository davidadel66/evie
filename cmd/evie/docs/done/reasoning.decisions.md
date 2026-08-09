# reasoning — decisions made during the build

The spec (`reasoning.spec.md`, same directory) holds the up-front design;
this records what only got decided once code existed.

- **`ReasoningDone()` takes no arguments.** The spec text sketched
  `ReasoningDone(content string)` but its own wire section already said
  `reasoning_done` carries `{}` — the client has every fragment, echoing the
  blob doubles bytes for nothing. The Events method matches the wire.
- **`Client.baseURL` test seam.** `ChatStream` hardcoded the OpenRouter URL,
  which made provider-layer tests impossible. The constructor defaults it to
  production; same-package tests override it to point at `httptest`. One
  unexported field, no DI machinery.
- **REPL reasoning reuses the one smoothPrinter.** A second printer would
  mean a second goroutine racing stdout; instead the dim-grey escape and the
  `thinking…` prefix go *through* the printer as ordinary fragments, and
  `ReasoningDone` flushes and nils the pair so content starts a fresh one.
- **`sseEvents` got no-op stubs for one stage.** Stage 2 deliberately broke
  both `Events` implementers at compile time; the web side shipped one commit
  as a no-op (thinking silently not shown) until 3a wired the real emitters.
- **Provider flakiness was a zombie server.** During live verification the
  serve path showed no reasoning while the REPL did — same binary, same
  request. Cause: an old `evie serve` held port 6687 and every curl hit it.
  Lesson recorded: check `bind: address already in use` in the log before
  theorizing about providers.
- **UI text pacing landed in the same session** (David asked for smoother
  streaming mid-build): see the 2026-08-08 entry in
  `docs/active/serve.decisions.md`.
