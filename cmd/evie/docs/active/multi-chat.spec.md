# Several chats at once

Origin: David's October 12, 2026 request. He can view only one chat at a time,
opening a chat lags, and while a turn runs he cannot look at other chats. He
wants chats to keep working while he reads another, a loading indicator on
chats with a running turn, and a bold title on a chat whose finished reply he
has not read. Decisions: [multi-chat.decisions.md](multi-chat.decisions.md).

Today the web server holds one open chat (`s.session`, `s.activeSession`).
Opening another chat replaces it, and is refused while any turn runs. A turn's
events reach the browser only through the response of the request that
started it, so leaving or reloading a running chat loses it, and its history
is refused until the turn ends. Every chat open also asks OpenRouter for the
model's metadata, two sequential requests (about 170 ms, up to 3 s).

## Stage 1 — Opening a chat without waiting on OpenRouter

- A resolved model context profile is reused for 10 minutes per model and
  context configuration, so opening a chat on a recently resolved model makes
  no OpenRouter request. Failed lookups and built-in fallbacks are not reused,
  so a later successful lookup still replaces them.
- Verification measures the open-chat request before and after, with the
  timings reported in the handoff.
- Verified October 12: `go test -race ./internal/openrouter` passed. Against a
  fake OpenRouter answering in 85 ms, a first open spent 174 ms on the two
  lookups and a repeat open 6 µs with no request; another model, the non-chat
  profile and an expired entry each looked up again, and a failed lookup's
  fallback was not kept. Real OpenRouter answers these in about 75–100 ms each.

## Stage 2 — Several open chats on the server

- The server keeps a set of open chats keyed by session, each with its own
  agent session, model revision and turn state. Opening a chat adds it, or
  reuses it when already open, and makes it the focused chat. Opening never
  stops or replaces another chat's turn.
- A turn in one chat does not block opening another chat, reading any open
  chat's history, or sending, stopping, compacting, changing the model of or
  inspecting another chat. Within one chat the current rules hold: one turn at
  a time, and compaction and model changes wait for its turn.
- Every per-chat request names its chat and is answered when that chat is
  open: history, chat, stop, compact, model list and selection, memory
  evidence, subagents and the repository-instruction snapshot. The focused
  chat only supplies defaults, such as the memory view's scope.
- An open chat's history is readable while its turn runs. It returns
  committed events read from the store, without waiting for the turn; text
  still streaming is not included.
- Archiving a chat is refused only while that chat runs a turn; archiving a
  Workspace, or changing its folder or instruction settings, only while a
  chat in that Workspace runs one. Archived chats are closed.
- At most 8 chats stay open. Opening another closes the least recently used
  idle chat (no running turn, pending approval or compaction). If none is
  idle, opening is refused with a message saying so.
- Turn leases, the REPL, and one-turn-per-chat persistence rules are unchanged.

## Stage 3 — A live feed per chat, and rejoining a running turn

- Each open chat publishes its turn events to its own live feed. The browser
  follows the feed of the chat it shows; a new follower first receives the
  running turn's events so far, then live ones. Starting a turn no longer
  depends on keeping its request's response open.
- Leaving a running chat and coming back, or reloading the page, shows the
  turn's progress so far and continues live, including streamed text, tool
  rows, the live status line and pending approvals.
- A pending approval stays pending while any Evie tab is connected, and
  expires as a disconnect does today only after no tab has been connected for
  2 minutes, or when its turn is stopped. It can be answered whenever its chat
  is shown.
- The browser keeps each open chat's view (transcript, queued messages,
  status) so switching back is immediate; queued messages stay with their chat.

## Stage 4 — Chat status in the sidebar

- A status feed reports, for every chat, whether a turn is running, whether an
  approval is waiting, and when its latest turn finished.
- The sidebar shows a spinner on a chat with a running turn, an amber marker on
  a chat waiting for approval, and a bold title on a chat whose latest turn
  finished after David last viewed it. A collapsed Workspace shows the same
  markers when any of its hidden chats has one.
- Read state is kept in this browser only, per chat: showing a chat while the
  page is visible marks it read. Turns that finished before this browser first
  recorded read state do not make a chat bold.
- No notification or sound.

## Non-goals

Side-by-side chat panes; notifications; read state shared across browsers or
devices; turns that start without a browser; REPL changes; changing turn
leases, persistence formats or migrations.

## Risks

Concurrency in the server's chat set (open, close, eviction and turns racing);
SQLite write contention with several turns at once; memory held by up to 8
open agent sessions; the approval expiry change; replay ordering when a
follower joins mid-turn; per-chat browser state; provider rate limits, and the
subagent runtime cap of 4 children now shared by concurrent chats.

## Verification

Each stage: focused Go tests with real SQLite and a scripted provider, the UI
suite, `-race` on the server packages, and `./scripts/verify-change.sh`.
Stage 2 runs two chats' turns concurrently, reads history mid-turn and
exercises eviction and archive refusal. Stage 3 joins a turn midway, reloads,
and answers an approval after switching away and back. Stage 4 covers each
marker and read state. A disposable browser fixture with a scripted provider
checks the live states without model requests, at desktop and 390 px.
