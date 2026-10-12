# workspace-mcp-connections — decisions

Spec: [workspace-mcp-connections.spec.md](workspace-mcp-connections.spec.md).
Decided 2026-10-11 with the owner, one question at a time.

## The owner's decisions

- **Hosted servers, with the client and OAuth in one spec** (Q1). Square,
  Canva, Metricool and Meta all offer hosted MCP with an OAuth sign-in. Evie
  runs no third-party code, which keeps ADR 0004's compiled-first-party rule
  intact. Rejected: local stdio servers, which run third-party processes on
  the Mac, and doing both in the first spec.
- **Ask unless allowed** (Q2). Every MCP call asks by default. The owner can
  set an individual tool to Allow per Workspace. The owner accepted the
  known catch: Square's one `make_api_request` tool covers reads and writes,
  so Allow on it allows both. Rejected: always asking, which is noisy for
  sales pulls, and trusting the server's read-only hint, which Evie cannot
  verify.
- **macOS Keychain for tokens** (Q3). This is the first concrete Kernel
  secret storage that ADR 0002 and ADR 0028 describe. Rejected: SQLite
  beside Plaid's `access_token`, which leaves tokens in plain text in
  `~/.evie`, and a Go keyring library, which adds a dependency for no gain on
  a Mac-only runtime.
- **Chats only** (Q4). Workflows get MCP in a follow-up spec. A workflow
  effect must be reconcilable after a crash (ADR 0009), and a generic MCP
  tool offers no idempotency or lookup to reconcile against.
- **The official MCP Go SDK** (Q5), `github.com/modelcontextprotocol/go-sdk`.
  It handles both transports, version negotiation and client OAuth (the
  `auth` package from v1.5.0). Rejected: a hand-written client that would
  have to track a protocol that changes several times a year.

## Calls Claude made inside those decisions (open to redirect)

- **Two general chat tools, `mcp_list_tools` and `mcp_call`, instead of one
  tool definition per server tool.** The first draft turned each server tool
  into its own Evie tool. The spec review showed that this conflicts with the
  existing code in five places:
  - resume rejects unknown Capabilities and any schema or tool-list drift
    (`internal/plugins/preset.go`);
  - receipts only accept UUID connection references
    (`internal/composition/receipt.go`);
  - Evie's tool schema type cannot express arbitrary JSON Schema
    (`internal/openrouter/schema.go`);
  - Capability IDs must be lowercase;
  - a connection named `memory` or `todo` would pick up protected
    tool-result handling.

  With two static tools, the composition, receipt and resume code stay
  unchanged, and the Workspace limits are enforced on every call. This is
  the same shape as `finance_query` and `edit_db`: one general tool plus a
  gate. The cost is one extra model step to list tools before calling one.
- **The Connection ID is `mcp-<name>`.** The prefix prevents a collision with
  built-in Connection IDs (`github`, `finance-local`). The model and the CLI
  use the plain name.
- **Reviewed tools are pinned by hash in Workspace Access** as
  `<name>@<sha256>`. This reuses the existing access revisions and immediate
  revocation, and it catches a server quietly changing a tool after review.
- **`evie serve` is the only process that holds tokens.** If the CLI also
  refreshed tokens, a rotated refresh token could be lost. Every CLI
  connection command calls serve's loopback management routes, and the
  terminal REPL refuses MCP calls instead of opening a second token holder.
- **A chat pins the access revision it started with**, in a new
  `session_workspace_access_pins` table. workspaces.spec.md says additions
  reach new sessions only and removals apply at once. The existing session
  row pins the Workspace Revision, but not the separate access revision.
- **Tool Policy is separate from Workspace Access and is read at call time.**
  Access limits what Evie can reach. Policy only decides whether Evie asks,
  so a change applies to the next call in both directions. The policy is
  bound to the tool's hash, so a changed tool starts at Ask again.
- **Approval is a per-call hook on `tools.Tool`**, because `NeedsApproval` is
  a fixed flag. Any error falls back to asking. The access check runs inside
  `Execute`, after approval and immediately before sending.
- **No Client ID Metadata Documents.** They need a public https URL, and
  Evie listens only on loopback. Servers without Dynamic Client Registration
  (Canva) take a pre-registered client ID instead.
- **No Remove in v1.** Disconnect deletes the tokens. Removing the record
  would bring edge cases (shared references, archived Workspaces) with no
  current need.
- **Results are capped at 100,000 bytes with no spill file.** The web tools
  spill large output to `/tmp`, but MCP results carry business and customer
  data.
- **An unknown outcome is never retried.** The only re-send is after a 401
  plus a successful refresh, where the server provably rejected the first
  request. This keeps ADR 0009's rule ("never blindly retry an uncertain
  write") in chats too.
- **A spike came first (story 0)**, because the no-retry and token-rotation
  rules depended on unverified SDK behaviour. It chose the fallback: an
  Evie-owned HTTP transport that handles auth and sending, with the SDK used
  only for the protocol. See "Spike answers" below.
- **Three build stories after the spike** (connections, chat tools, policy
  and web UI). Each crosses one high-risk boundary, as AGENTS.md asks.

## Spike answers (story 0, 2026-10-12)

The spike code lives in `experiments/mcp-spike/`, which is not tracked. It
was checked against go-sdk v1.8.0 and against Square's live server, with
David signing in. The token was revoked afterwards.

1. **Does the SDK refresh and re-send on its own? It re-sends, but it never
   refreshes.**
   - On a 401 or 403, the Streamable transport calls `OAuthHandler.Authorize`.
     If that succeeds, the transport POSTs the same body once more
     (`mcp/streamable.go:2324-2352`).
   - `AuthorizationCodeHandler` answers a 401 by running the whole
     interactive flow from inside the call, not by refreshing.
   - A plain 403 is re-sent with nothing changed.
   - Two 401s in a row break the connection permanently.

   Outcome: Evie gives the SDK no `OAuthHandler`.
2. **Can Evie persist rotated tokens? Only if Evie wires it.**
   - Without `NewTokenSource`, the SDK refreshes internally and keeps the
     rotated refresh token only in memory.
   - The default refresh omits `resource`.
   - `Token()` runs while holding the connection mutex.

   Outcome: Evie owns refresh.
3. **Can the code come from Evie's callback? Yes, but the handler hides the
   DCR registration and repeats DCR on every authorization.**

   Outcome: Evie runs discovery, DCR, PKCE and the exchange itself. The spike
   did this end to end with the standard library alone.
4. **Do the transports re-send POSTs? Streamable does not.**
   - Its `MaxRetries` covers only GET stream reconnects.
   - The SSE client has no auth hook and follows a server-chosen endpoint.
   - The 2026-07-28 multi-round-trip feature can re-send `tools/call`; turn it
     off with `MultiRoundTrip.Disabled`.
   - `net/http` can replay a POST on some HTTP/2 errors when `GetBody` is set.

   Outcome: Evie clears `GetBody` and disables multi-round-trip.
5. **Can the SDK tell "never sent" from "sent, no response"? No.** A dial
   failure and "the server ran the tool and then dropped the connection" give
   identical errors (code -32005). An Evie-owned RoundTripper using
   `httptrace` (`GotConn`, `WroteRequest`) separated them cleanly in a probe.
   Outcome: the fallback transport.
6. **Square's live server:**
   - It serves Streamable HTTP at `/mcp` (protocol 2025-11-25, "Square MCP
     Server 1.0.0").
   - `/sse` rejects a token issued for `/mcp`.
   - DCR works as a public client (`token_endpoint_auth_method: none`, no
     secret issued).
   - PKCE uses S256.
   - It advertises no scopes and granted only `*_READ` scopes.
   - Access tokens last 3,600 seconds, and every refresh rotates the refresh
     token. Both tokens are 63 characters.
   - Revoking through the advertised endpoint (`/token`) returns 200.
   - Its 4 tools are `get_service_info`, `get_type_info`, `make_api_request`
     (annotated destructive) and `make_api_request_read`.
   - The server sends 2,298 characters of `instructions`, which Evie ignores.
7. **Does the Keychain accept the value over stdin? Yes, within a limit.**
   `security -i` reads one line of about 4 KB. Through stdin it round-tripped
   at most 2,018 secret bytes as hex `-X`, or 3,027 bytes as quoted base64
   `-w "..."`. Outcome: base64 quoted, with a 3,000-byte cap on the JSON.

**Spec changes from the spike:**
- Streamable HTTP only.
- An Evie-owned transport, plus OAuth run by Evie.
- Tools listed fresh before every call instead of cached.
- Base64 Keychain values.
- The demo uses `make_api_request_read`.

**Modules go-sdk v1.8.0 brings in** (from `go list -m all`):
- `github.com/google/jsonschema-go` v0.4.3
- `github.com/segmentio/encoding` v0.5.4 and `segmentio/asm` v1.1.3
- `github.com/yosida95/uritemplate/v3` v3.0.2
- `github.com/golang-jwt/jwt/v5` v5.3.1
- `golang.org/x/time` v0.15.0
- `golang.org/x/oauth2`, `x/sync`, `x/sys` and `x/tools`, at versions Evie
  already meets or exceeds

The exact list Evie's `go.mod` gains is recorded with story 2, the first
story that imports the SDK.

## Story 1 review (2026-10-12)

A code review of story 1 found these changes, now in the spec:

- **Disconnect keeps the client registration.** The spec first said
  Disconnect deletes the Keychain item. That also deleted a pre-registered
  client's secret (the Canva path), and the owner could then neither
  reconnect nor add the connection again. Now only the tokens go.
- **Token work is detached from the turn.** Square rotates the refresh
  token on every refresh, so a cancelled turn that dropped the response
  would have forced a new sign-in.
- **A rotation the Keychain refuses is kept in memory, unused,** and saved
  on the next call.
- **Only an OAuth refusal requires signing in again.** A firewall's HTML 400
  or a 503 sets `unreachable` and keeps the refresh token.
- **Redirects are never followed, and the bearer goes only over https.**
  `net/http` would otherwise re-POST a body on a 307 or 308.
- **The outcome adds "rejected"** for a complete 4xx answer. Before, a 403
  would have read as "outcome unknown".
- **A dynamic registration is reused** while its issuer and redirect are
  unchanged, so Square doesn't collect an orphan client per sign-in.
- **A confidential client defaults to HTTP Basic** (RFC 8414).
- **`Wait` waits for a callback that has already started,** even past the
  deadline.
- **The fake authorization server moved to `internal/mcpconn/mcptest`,** so
  the leak test can run against the real SQLite file, WAL included.

## Source facts (2026-10-11, superseded where the spike answered)

- Canva: hosted at `https://mcp.canva.com/mcp` (Streamable HTTP) with OAuth.
  It needs a Developer Portal client (canva.dev/docs/mcp).
- Metricool: an HTTP/OAuth endpoint at `https://ai.metricool.com/mcp`
  (help.metricool.com).
