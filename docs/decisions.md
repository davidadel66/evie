# Decisions

Cross-feature decisions, newest first: date, what, why — a few lines each. Feature-scoped decisions belong in that feature's `docs/*/<feature>.decisions.md`.

**2026-07-29 — Rich output goes to a web frontend, not desktop app or TUI graphics.**
`evie serve`: Go net/http + go:embed UI + WebSocket/SSE, a new cmd/ door. The frontend is web tech under any shell, so web-first keeps Wails wrapping open and adds phone-on-LAN for free. When it leaves localhost, the approval gate + an auth token become security-critical.

**2026-07-29 — Tool roadmap ordered; typed tools coexist with bash.**
When bash lands, CLI knowledge moves to prompt layers (training data, system-prompt paragraph, `help` self-discovery) — schemas don't transfer. High-traffic flows keep typed tools; bash serves the long tail. Todo tools are retirement candidates only after bash proves reliable.

**2026-07-28 — Reads wide, writes gated (not narrow).**
General query_db (engine-level read-only) and edit_db (free-form writes behind a per-call y/N approval gate) instead of per-action tools. David's standing preference: general capability + explicit guard mechanism over safety-by-narrowness. Registered-db enum, never free paths.

**2026-07-28 — Secrets never enter `messages`.**
Everything in the conversation goes to remote providers (OpenRouter is not local, even for open-weight models). Plaid tokens (items table), .env, key-exporting dotfiles are mechanically fenced out of every read surface — denylists plus read-only connections, independent of the approval gate.

**2026-07-16 — Flat tool registry until ~20 tools.**
Matches Claude Code (~20 always-on, one tool = one schema, no action-enum gateways). Deferred-schema loading (ToolSearch pattern, threshold = schemas >10% of context) is designed and parked; Hermes-style config toolsets rejected for now.
