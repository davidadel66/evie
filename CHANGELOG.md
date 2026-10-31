# Changelog

Every merge to master, newest first: what changed and why. The system
explorer's History tab shows this file, and its drift check validates it.
The entry format, the check's rules, and how to add an entry are in the
[explorer README](docs/architecture/explorer/README.md#history). After
editing this file, run `go run ./scripts/explorer extract` and
`go run ./scripts/explorer check`.

## 2026-10-29 — System explorer
Branch: feat/system-explorer
Touches: scripts/explorer, docs/architecture/explorer
Sources: doc:cmd/evie/docs/active/system-explorer.spec.md, doc:cmd/evie/docs/active/system-explorer.decisions.md, doc:docs/architecture/explorer/README.md, #186, #187, #188, #189, #190, #191, #192, #193, #194

Adds the system explorer, a page opened straight from disk that shows what
Evie does: every package under `cmd/` and `internal/` as a component, grouped
under the architecture map in `AGENTS.md`, with levels from the system context
down to one package's focus view, key runtime flows as sequence diagrams to
step through, and design proposals toggled on top of the built system.

Written content cites its sources, and the facts beneath it are extracted from
the code: packages, imports, commands, routes, database tables, glossary terms,
and ADRs. The drift check, run by `./scripts/verify-change.sh`, fails when the
data and the code disagree, so the map cannot drift quietly.

## 2026-10-26 — Tool results projected in batches, and lighter turns
Branch: cache/batched-projection
Merge: cc69b4d
Touches: internal/agent, internal/eviedb
Sources: doc:cmd/evie/docs/active/memory.decisions.md, code:internal/agent/request_history.go, code:internal/agent/turn_decodes.go

While a request fits usable input, older tool-result groups past the three
retained ones now become eligible for pressure projection in whole batches of
four, so the projected history changes once a batch instead of on almost every
tool call. Cached-token counts from the owner's history showed that rolling
projection caused most avoidable provider cache misses; in a replay of a long
chat, the previous request's bytes left unreused fell from 1.70 MB to 1.37 MB
for 1.6 percent more bytes sent. The composer version becomes
`context-composer-v6`.

The branch also cuts the work each model request repeats within a turn: the
turn reads only the events committed since its previous request
(`EventsAfter`), derives history checks once per request, decodes each stored
event once, and sizes pressure projection without re-encoding the whole
request. On a 70-turn chat, overhead per model request fell from 68 ms to
23 ms; requests are unchanged.
