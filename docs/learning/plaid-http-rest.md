# plaid.go at the HTTP level — request/response, REST or not

Working through what `cmd/finance/plaid.go` actually does on the wire.

## Checklist

- [x] 1. The request/response model: who is the client, who is the server, what one round-trip is
- [x] 2. Anatomy of a request: method, URL, headers, body — mapped to lines in plaid.go
- [x] 3. Anatomy of a response: status codes, JSON body → Go structs, how errors surface
- [x] 4. What the plaid-go SDK layer adds (and hides) over raw net/http (curl exercise)
- [ ] 5. Is this REST? REST vs RPC-over-HTTP, and which one Plaid actually is
- [ ] 6. Why `waitForPublicToken` polls — HTTP's one-way nature, and the webhook alternative

## Notes

- HTTP = strict turn-taking: client speaks first, server answers once, done. Server can never call
  you back → that's why `waitForPublicToken` polls every 3s.
- The CLI is a pure HTTP *client*; there is no server code in this repo. `production.plaid.com` is
  the server.
- `NewLinkTokenGetRequest` doesn't create a token — it builds a *status check* for an existing one.
- `New*` constructors return pointers (setters have pointer receivers); the SDK call takes a value,
  so `*req` dereferences. Same `*` symbol as a pointer type, opposite operation.
- Optional JSON fields → pointer struct fields → `GetXxxOk()` returning `(value, ok)` — same
  comma-ok idiom as map lookups. `ok == false` means "server didn't send this field."
- A *link session* = Plaid's record of one browser visit to the hosted link URL. One token → N
  sessions. `sessions: null` means no session exists yet — the session is created when the URL is
  opened, then events accumulate inside it. Session contents: `events`, `exit` (only if bailed),
  `results.item_add_results[].public_token` (only on success).
- `LinkTokenCreate`'s *response* carries both the link token and the ready-made hosted link URL;
  `waitForPublicToken` only polls status with the token, it builds nothing.
- Two-token design: the browser is hostile territory (logs, extensions, analytics), so the token
  that travels through it (public token) is short-lived, single-use, and useless without the
  `PLAID-SECRET`. The exchange (`ItemPublicTokenExchange`) happens server-side and proves
  possession of the secret; the resulting *access token* never touches a browser and lives in the
  DB. Principle: durable credentials stay on the backend; anything the frontend touches is
  presumed leaked.
