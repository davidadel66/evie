# Workspace MCP connections

Status: approved by David on 2026-10-11. Story 0 (the spike) finished on
2026-10-12 and its answers are folded in below. Story 1 is next. The
decisions are in
[workspace-mcp-connections.decisions.md](workspace-mcp-connections.decisions.md).
Requested by David on 2026-10-11 for Cairo's Kitchen.

## Outcome

In a Workspace chat, Evie can use tools from hosted MCP servers the owner has
connected. Square in Cairo's Kitchen is the first proof. The owner signs in
once through the browser (OAuth), Evie keeps the token in the macOS Keychain,
and each Workspace chooses which servers and which tools its chats may reach.
Every call shows an approval card unless the owner set that tool to Allow in
that Workspace.

Evie is the MCP client. It runs no MCP server, and it never starts a local
server process.

## Terms

- **MCP Connection**: an Account Connection (CONTEXT.md) to one hosted MCP
  server. The owner gives it a name such as `square`. Its Connection ID is
  `mcp-<name>`, for example `mcp-square`. The prefix keeps it apart from
  built-in Connection IDs such as `github` and `finance-local`. The Kernel
  owns the connection, and several Workspaces may share it (ADR 0042).
- **Reviewed MCP Tool**: one server tool a Workspace has enabled, recorded as
  the resource selector `<tool-name>@<schema-sha256>`. The hash covers the
  tool's `name`, `description`, `inputSchema` and `annotations` as JSON with
  sorted keys, exactly as the server listed them when the owner enabled it.
- **Tool Policy**: `ask` (the default) or `allow`, set per Workspace for each
  Reviewed MCP Tool.

## Boundaries

- **Hosted servers over Streamable HTTP only.** The spike showed Square
  serves Streamable HTTP at `https://mcp.squareup.com/mcp`. Its `/sse`
  endpoint rejects tokens issued for `/mcp`. The SDK's SSE client also has
  no auth hook and follows a message endpoint the server chooses, so the
  older HTTP+SSE transport is out. The URL must be `https`. Local stdio
  servers are out of scope, so ADR 0004 holds: the client is compiled
  first-party code, and the servers are external systems.
- **Web chats only.** MCP tools work in chats run by `evie serve`. In the
  terminal REPL they return "MCP tools run only in evie serve chats".
  Workflow Definitions cannot reference MCP tools. A follow-up spec covers
  workflows, because ADR 0009 requires reconcilable effects and a generic MCP
  tool cannot promise that.
- **Tools only.** Evie advertises no client capabilities: no sampling,
  elicitation or roots. It does not use server prompts, resources or the
  `instructions` field.
- **Server text is untrusted.** Tool descriptions, schemas and results reach
  the model only as tool-result data framed with `internal/untrusted`. They
  never become tool definitions or instructions.
- **Credentials live only in the Keychain.** Access tokens, refresh tokens
  and client secrets never appear in SQLite, procedural Git, Composition
  Receipts, session events, logs, tool results, process arguments or model
  context (ADR 0002, ADR 0028).
- **The model cannot manage connections.** No tool adds, connects, enables or
  allows anything. Only the owner can, through the web UI or the CLI.
- **`evie serve` is the only process that holds tokens.** It runs OAuth
  attempts, refresh, live MCP sessions and tool listing. Each CLI connection
  command is a thin client of serve's loopback management routes (JSON POST,
  which `guard` already accepts from a non-browser client). If serve is not
  running, the command fails with "evie serve is not running at <addr>".
- **One new production dependency:** `github.com/modelcontextprotocol/go-sdk`
  v1.8.0, pinned exactly. It brings in its own module requirements, which
  the decisions file lists. Evie uses only its `mcp` client package. Evie
  runs OAuth itself with the standard library, so `golang.org/x/oauth2`
  stays indirect.

## Connections

- **Record.** A new `mcp_connections` table stores, per connection:
  - `id` (`mcp-<name>`) and `name`;
  - `url` and an optional pre-registered `client_id`;
  - `status`, a secret-free `status_detail`, and timestamps.

  The name must match `^[a-z][a-z0-9-]{0,23}$` and be unique. A new connection
  starts as `not_connected`.
- **Connect.** OAuth 2.1 authorization code with PKCE (S256), following the
  MCP authorization spec:
  1. Discover the protected resource metadata, then the authorization server
     metadata.
  2. Register the client by Dynamic Client Registration when the server
     offers it. Otherwise use the pre-registered client ID, plus a secret the
     owner pipes in on stdin (for example, from Canva's Developer Portal).
     Client ID Metadata Documents are not used, because they need a public
     https URL that Evie does not have. A reconnect reuses an earlier dynamic
     registration while its issuer and redirect URI are unchanged (the
     redirect is recorded with the OAuth metadata). A confidential client
     authenticates with HTTP Basic, unless the server lists only
     `client_secret_post`.
  3. Send the resource indicator, and request the scopes the metadata
     advertises. Square advertises none and grants read scopes.

  Evie runs discovery, registration, PKCE and the code exchange itself. The
  SDK's `AuthorizationCodeHandler` is not used, because it repeats DCR on
  every authorization and never exposes the registration. It also runs the
  full interactive flow from inside a call when the call gets a 401, instead
  of refreshing. Serve keeps each pending attempt (`state`, PKCE verifier, registration) in
  memory. It is single-use and expires after 10 minutes, and a restart drops
  it. The redirect URI is `http://<EVIE_ADDR>/api/connections/oauth/callback`.
  That GET route is exempt from `guard` because the authorization server's
  redirect is a cross-site navigation, so only a matching unexpired `state`
  is accepted there. It answers with a plain page saying "Connected. You can
  close this tab."
- **Keychain storage.** Each connection has one generic-password item, with
  service `dev.evie.mcp` and account set to the Connection ID. Its value is
  JSON holding the access token, refresh token, expiry, client ID and client
  secret. The store runs `/usr/bin/security` through an injectable command
  runner, and secret material reaches it over stdin, never in argv. The
  value goes through `security -i` as a quoted base64 `-w "..."` argument.
  `security -i` reads one line of at most about 4 KB, so the JSON may be at
  most 3,000 bytes. A larger value is refused with status
  `keychain_unavailable` and the detail "credentials too large". Square's
  tokens are 63 characters each.
- **Refresh.** Serve refreshes under a per-connection lock, before expiry or
  once after a 401. It sends the `resource` parameter, and it stores the
  rotated tokens under the same Connection ID before using them (ADR 0024).
  Square rotates the refresh token on every refresh, so a lost write means
  signing in again.
  - The code exchange and refresh, together with saving their results, run
    detached from the caller with a 30-second limit. A cancelled turn
    therefore cannot drop a rotation.
  - If the Keychain refuses the rotated tokens, serve keeps them in memory,
    unused, and retries the save on the next call.
  - Only an OAuth refusal (`invalid_grant`, `invalid_client` or
    `unauthorized_client`) sets `needs_sign_in`. That status drops the
    tokens, keeps the client registration, and makes calls return a readable
    result without retrying.
  - Any other failure sets `unreachable` and keeps the refresh token.
  - A later success sets `connected` again, as does a successful Keychain
    read after `keychain_unavailable`.
- **Status** is one of `not_connected`, `connected`, `needs_sign_in`,
  `unreachable` or `keychain_unavailable`, and serve writes it to the record.
  The `mcp` plugin reports `ConnectionReadiness` as `not_required`, because
  connections are optional per Workspace. Per-connection status appears in
  `evie connections list`, in `mcp_list_tools`, and in Workspace Settings.
- **Disconnect** attempts revocation if the server advertises a revocation
  endpoint, deletes the tokens from the Keychain, closes the live MCP
  session, and sets the status to `not_connected`. The client registration
  stays, including a pre-registered client's secret, so reconnecting works
  without adding the connection again. The record and any Workspace
  references remain.
- **Live MCP sessions.** Serve keeps at most one live MCP session per
  connection, shared by chats. It opens the session on first use and reopens
  it after a failure or a closed connection.
  - Sessions run with `DisableStandaloneSSE: true` and
    `MultiRoundTrip.Disabled: true`. The second setting stops the SDK from
    re-sending a call on protocol 2026-07-28.
  - Tools are listed fresh by `mcp_list_tools`, by owner review, and
    immediately before every `mcp_call`. Nothing caches them and nothing
    waits for list-changed notifications.
  - Opening the session or listing tools times out after 10 seconds through
    the context. `http.Client.Timeout` is not used.
  - No lock is held across network work. Callers that arrive while a
    session opens wait for it, or for their own context.
  - A session is dropped, and closed in the background, only when its
    connection breaks. A caller's own cancellation or timeout does not drop
    it, and neither does the server's JSON-RPC error answer.
  - A disconnect forgets the session and closes it in the background. It
    runs after the connection's token lock is released, so it never waits on
    a request.
- **HTTP transport (owned by Evie).** The Streamable HTTP transport gets one
  `http.Client` per connection whose RoundTripper:
  - adds `Authorization: Bearer` only over https, and only to the
    connection's own host;
  - takes the token from the per-connection token source;
  - on a 401, refreshes under the lock and re-sends that request exactly
    once from its own buffered copy of the body;
  - never re-sends after a 403 or any other status;
  - never follows a redirect: the client returns the 3xx as it is, and the
    transport refuses a redirected request;
  - clears `GetBody`, so `net/http` cannot replay a POST on its own;
  - records whether a connection was obtained, through `httptrace`, and
    which status answered. The resulting outcome is one of:
    - no connection obtained: the request was never sent;
    - a complete 4xx answer: rejected;
    - anything else (no answer, a broken stream, or a 5xx from a gateway
      that may have forwarded the request): unknown.

  The SDK is given no `OAuthHandler`.

## Workspace access and tool review

- A Workspace reaches an MCP Connection only through its existing Workspace
  Access. The Connection ID is the connection, and each Reviewed MCP Tool
  (`<name>@<sha256>`) is a resource. `NormalizeAccess` requires at least one
  resource, so attaching a connection means enabling its first tool, and
  disabling the last tool removes the connection from the Workspace.
- **Review.** Evie lists the server's current tools with their name,
  description and input schema. Enabling a tool adds `<name>@<sha256>` to the
  access. Enable and disable read the current access revision and write back
  with that revision as `expect`. If another change landed first, the owner
  sees "access changed, try again" and nothing is written. Archived
  Workspaces cannot be changed, which is existing `SetWorkspaceAccess`
  behaviour. Disable needs no server: revoking a tool works while the
  connection is signed out or unreachable.
- **A tool is visible but cannot be enabled when** its name contains
  whitespace or control characters, or is longer than 64 characters.
- **Session pin** (workspaces.spec.md). When a chat session is created in a
  Workspace, Evie writes the Workspace's current access revision to a new
  immutable table, `session_workspace_access_pins(session_id, workspace_id,
  access_revision)`. The chat's effective MCP access is the access at that
  revision intersected with the current access. Tools enabled later therefore
  reach new chats only, and removals apply at once (ADR 0033).
- **Tool Policies** are stored in a new append-only table,
  `workspace_mcp_tool_policies(workspace_id, revision, policies)`, keyed by
  Connection ID and `<name>@<sha256>`. The default is `ask`. Every call reads
  the latest revision, because a policy changes only whether Evie asks, not
  what Evie can reach. A policy is bound to its hash, so a re-reviewed tool
  starts at `ask`.

## Chat tools

A new compiled plugin, `mcp`, contributes two static tools. The built-in
`standard` preset adds them as optional Capabilities. Its current content is
frozen as `preMCPStandardPresetContent` for resume, following the
`preWorkflowsStandardPresetContent` pattern in `internal/plugins/preset.go`.
The Composition Receipt, preset resolution and resume code are unchanged:
which MCP tools a chat may reach is decided at call time, not by
composition.

- **`mcp_list_tools`** (`mcp.list_tools`), with no arguments. For each MCP
  Connection in the chat's effective access, it returns:
  - the connection's name and status;
  - every reviewed tool whose current hash is in that access, with its
    description and input schema as JSON;
  - reviewed tools whose hash has changed, as "changed since review: not
    available";
  - reviewed tools the server no longer lists, as "no longer offered".

  It needs no approval. The output is framed as untrusted and capped at
  100,000 bytes. Outside a Workspace it returns "no MCP connections in this
  chat".
- **`mcp_call`** (`mcp.call`), with arguments `{connection: string, tool:
  string, arguments: object}`. `connection` is the connection's name. Evie
  does not validate `arguments` against the input schema; the server does,
  and its error comes back as a tool error.
  - **Approval** uses a new per-call hook on `tools.Tool`, such as
    `RequiresApproval(ctx, args) (bool, error)`, which the dispatcher in
    `internal/tools/registry.go` consults in place of the static
    `NeedsApproval`. For `mcp_call` it returns false only when the tool's
    current `<name>@<sha256>` has policy `allow` in this Workspace. On any
    error it returns true. The dispatcher authorizes the preparation step
    before calling the hook, because the decision lists the server's tools.
    Outside serve the hook shows no card, because the call can only report
    that it needs serve. The existing approval card shows the three
    arguments.
  - **Check before sending.** Inside `Execute`, after approval and
    immediately before sending, Evie lists the server's tools again. It then
    checks that the tool's current hash is in the chat's effective access.
    If not, nothing is sent, and the result is "removed from this Workspace"
    or "changed since review". A tool removed while its card is open
    therefore never runs.
  - **Timeout and cancellation.** Each call has a 60-second timeout. If the
    turn is cancelled, Evie cancels the in-flight request.
  - **Results.**
    - Text content is joined.
    - `structuredContent` is appended as JSON.
    - Image, audio, and resource or link content becomes a one-line
      placeholder naming its type.
    - The result is framed as untrusted and capped at 100,000 bytes, with a
      notice telling the model to narrow the request. Nothing spills to a
      temp file.
    - A result with `isError: true` reaches the model as a tool error.
  - **No blind retries.** Evie re-sends a call only once, and only after a
    401 answered by a successful refresh, because the server rejected the
    first request. The transport's outcome sets the result:
    - unknown (a timeout, a dropped connection, a broken stream or a 5xx):
      "outcome unknown: check in <connection> before retrying", and Evie
      does not retry;
    - never sent: "not sent; nothing reached <connection>";
    - a complete 4xx answer, or the server's JSON-RPC error: "rejected by
      <connection>".

    Any server words in these errors stay inside an untrusted frame. A
    tool-error result is capped at 15,000 bytes before framing, so the frame
    survives the 16 KiB limit on tool errors.

## Surfaces

- **CLI** (thin clients of serve):
  - `evie connections list`
  - `evie connections add <name> <url> [--client-id <id>]`. With
    `--client-id`, the client secret is read from stdin.
  - `evie connections connect <name>` prints and opens the authorization
    URL, then waits for the result.
  - `evie connections disconnect <name>`
  - `evie workspaces mcp-tools <workspace-id> <name>` lists the current tools,
    their hashes and their review state.
  - `--enable <tool>` and `--disable <tool>` change the Workspace Access.
  - `--policy <tool>=ask|allow` changes the Tool Policy.
- **Web:** a Connections section in Workspace Settings, following
  `WorkspaceExploreSettings`. It can add a server or pick an existing
  connection, Connect or Reconnect, show status, list tools with their
  description and an expandable schema, enable or disable each tool, and set
  Ask or Allow. A changed tool appears as "Changed since review", with the
  reviewed and current versions side by side.

## Stories

0. **Spike** (done 2026-10-12). It answered seven questions about the SDK,
   Square's live server and the Keychain. The answers are in the decisions
   file. Outcome: the fallback applies. An Evie-owned RoundTripper holds the
   token, refresh and send-once rules, the SDK is used only for the
   protocol, and only Streamable HTTP is supported.
1. **Connections and OAuth.** The `mcp_connections` table, the Keychain
   store, OAuth and the callback, refresh, status, the serve routes and
   `evie connections`. Demo: connect Square; `evie connections list` shows
   `connected`; the Keychain holds the item; SQLite holds no token.
2. **Workspace tools in chat.** Review via CLI, the access pin table, the
   `mcp` plugin, `mcp_list_tools`, and `mcp_call` with every call on `ask`.
   Also the `standard` preset bump with its frozen branch, the per-call
   approval hook, result shaping and revocation. Demo: a Cairo's Kitchen chat
   pulls last week's Farmer's Market sales from Square after an approval
   card.
3. **Allow policy and web settings.** The policy table, `allow` behaviour,
   and the Workspace Settings Connections section.

## Non-goals

- Local stdio servers, including Google Ads MCP.
- MCP tools in workflows or in the terminal REPL.
- MCP prompts, resources, sampling, elicitation, roots, and server
  instructions.
- Exposing each server tool as its own model-facing tool definition.
- Removing a connection record (Disconnect is enough for now).
- An "allow from now on" choice on the approval card.
- Per-argument policies, such as allowing Square GET requests but asking for
  POST.
- Client-side validation of `arguments` against the input schema.
- Moving existing secrets (Plaid, GitHub, OpenRouter) into the Keychain.
- Secret storage on any OS other than macOS.
- Cairo's Kitchen business rules.

## Risks

- Square's `make_api_request` covers reads and writes, so `allow` on it would
  allow writes. On 2026-10-12, Square granted only `*_READ` scopes, and it
  also offers `make_api_request_read`. Enable and allow the read tool, and
  keep `make_api_request` disabled or on `ask`.
- Each server tool takes the model two steps: list, then call. That costs
  one extra round trip in exchange for an unchanged composition model.
- A tool description is a prompt-injection surface. Untrusted framing, owner
  review and the hash check contain it, but they cannot stop a server from
  changing how a tool behaves.
- The MCP protocol changes often (2026-07-28 removed the `initialize`
  handshake). The SDK negotiates the version, and the pinned SDK version
  sets what Evie speaks.
- Changing `EVIE_ADDR` changes the redirect URI, so dynamically registered
  connections must reconnect.
- If the Keychain is locked, MCP is unavailable (`keychain_unavailable`), but
  the rest of Evie keeps working.

## Verification

- An in-process fake MCP server (the SDK's Streamable HTTP server on
  `httptest`) and a fake authorization server cover:
  - DCR and a pre-registered client;
  - PKCE;
  - `state` that is mismatched, replayed or expired is rejected;
  - refresh rotation under concurrent calls, with `resource` sent;
  - 401, then refresh, then exactly one re-send;
  - a 403 is never re-sent;
  - refresh failure leading to `needs_sign_in`;
  - no bearer token is sent to another host;
  - a dial failure reports "not sent";
  - a server that runs the tool and then drops the connection reports
    "outcome unknown" and is never retried.
- The Keychain store is tested with a fake command runner that captures
  argv and stdin. One opt-in test (`EVIE_KEYCHAIN_TEST=1`) round-trips
  against the real Keychain, because CI has none.
- A secret-leak test runs one fake token through connect, list and call,
  then asserts the token appears in none of: the SQLite file, receipts,
  session events, logs, tool results, or the captured argv.
- CLI tests: every connection command fails with the "not running" message
  when serve is absent.
- Access tests:
  - the first enable attaches the connection, and the last disable removes
    it;
  - a stale `expect` writes nothing;
  - an enable after session creation does not reach that session;
  - a removal blocks that session's next call;
  - an archived Workspace refuses changes.
- Tool tests:
  - `mcp_list_tools` marks changed and missing tools;
  - `ask` shows a card, and declining sends nothing;
  - `allow` shows no card;
  - an error in the approval hook falls back to a card;
  - a re-hashed tool falls back to `ask`;
  - removal while a card is open sends nothing;
  - result framing, placeholders, the 100,000-byte cap and the timeout work
    as specified;
  - the REPL refusal message appears in the terminal REPL.
- Preset tests: existing sessions resume on the frozen pre-MCP branch, and
  new `standard` sessions carry the two optional Capabilities.
- Manual demonstration with real data:
  1. Connect Square.
  2. Enable `make_api_request_read` in Cairo's Kitchen.
  3. Ask for last week's sales at location `L85HMPN9N4MZ9`.
  4. Approve the card (in story 3, set the tool to Allow and see no card).
  5. Compare the answer to the Square Dashboard.
- Run `./scripts/verify-change.sh` before each handoff.
