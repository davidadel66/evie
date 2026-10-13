# Several chats at once — decisions

- **2026-10-12 — Several open chats replace the single session.** David asked
  to use more than one chat at a time. This supersedes the 2026-08-03 "in-memory
  single session for v1" decision in [serve.decisions.md](serve.decisions.md),
  which kept the server "structured so a keyed session map is an add, not a
  rewrite". One turn per chat, turn leases and persistence are unchanged.
- **2026-10-12 — Read state lives in the browser.** David chose browser-only
  read state over a database record: no migration, and it resets if site data
  is cleared. Evie runs on one machine.
- **2026-10-12 — Only a bold title for unread replies.** David declined
  notifications or sounds when a background chat finishes.
- **2026-10-12 — Defaults set in the specification, open to David's review.**
  Model profiles are reused for 10 minutes; at most 8 chats stay open, closing
  the least recently used idle one; a pending approval expires only after no
  Evie tab has been connected for 2 minutes (it expired on disconnect before,
  which would end every approval in a chat David leaves).
- **2026-10-12 — Detached turns are contained.** A turn now runs without the
  request that started it, so a panic inside it is recovered there: the turn
  ends with an error, its chat closes, and the server and other chats keep
  running, as they did when each turn was its own request.
