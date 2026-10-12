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
- **A spike comes first (story 0)**, because the no-retry and token-rotation
  rules depend on SDK behaviour that is not yet verified. The fallback is an
  Evie-owned HTTP transport that handles auth and sending, with the SDK used
  only for the protocol.
- **Three build stories after the spike** (connections, chat tools, policy
  and web UI). Each crosses one high-risk boundary, as AGENTS.md asks.

## Source facts (2026-10-11, to recheck in story 0)

- Square: hosted at `https://mcp.squareup.com/sse` (legacy SSE) with OAuth.
  Its tools are `get_service_info`, `get_type_info` and `make_api_request`.
  This comes from third-party listings (mcpservers.org, goose docs), not
  developer.squareup.com.
- Canva: hosted at `https://mcp.canva.com/mcp` (Streamable HTTP) with OAuth.
  It needs a Developer Portal client (canva.dev/docs/mcp).
- Metricool: an HTTP/OAuth endpoint at `https://ai.metricool.com/mcp`
  (help.metricool.com).
- MCP Go SDK: client OAuth (`auth.AuthorizationCodeHandler`, DCR config) since
  v1.5.0. v1.7.0 added protocol 2026-07-28, which drops `initialize`.
