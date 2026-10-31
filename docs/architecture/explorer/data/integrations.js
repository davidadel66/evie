evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "integrations",
  "nodes": [
    {
      "id": "pkg:cmd/finance",
      "level": "component",
      "kind": "package",
      "title": "cmd/finance",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The finance command: a thin CLI over internal/finance that links banks through Plaid Hosted Link, syncs transactions, seeds categorization rules, categorizes, and opens the database.",
        "refs": [
          "code:cmd/finance/main.go#main",
          "code:cmd/finance/main.go#usage"
        ]
      },
      "does": [
        {
          "text": "link runs the shared, durable Hosted Link flow: it prints the hosted URL, then polls until the bank is linked, expired, cancelled or failed, giving up after 35 minutes.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/link.go#Link",
            "code:internal/finance/link.go#linkWithService"
          ]
        },
        {
          "text": "sync syncs every linked bank and prints each bank's counts, warnings and failures, then the totals, and exits non-zero when any bank failed.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/sync.go#Sync"
          ]
        },
        {
          "text": "rules seeds merchant rules from ~/.finance/merchantLookup.json and then the built-in name rules embedded in internal/finance.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/categorize.go#RulesSeed",
            "code:internal/finance/name_rules.go#NameRulesSeed"
          ]
        },
        {
          "text": "categorize mints Budget Entries for posted transactions that have none, and prints how many were categorized and how many remain uncategorized.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/categorize.go#Categorize"
          ]
        },
        {
          "text": "db opens the database, which applies the schema, and prints that it is ok.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/db.go#OpenDB"
          ]
        }
      ],
      "owns": [
        {
          "text": "Nothing of its own: every command reads and writes internal/finance's database at ~/.finance/finance.db.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/db.go#OpenDBContext"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Subcommands link, sync, rules, categorize, db and help; a missing or unknown command prints the usage to stderr and exits 2.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:cmd/finance/main.go#usage"
          ]
        },
        {
          "text": "Plaid credentials come from PLAID_CLIENT_ID and PLAID_SECRET in the environment, read by internal/finance against Plaid's production environment.",
          "refs": [
            "code:internal/finance/client.go#plaidClient"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "main calls godotenv.Load(\"../../.env\", \".env\"), which stops at the first missing file, so .env is never read when ../../.env is absent, although the comment calls it a fallback; evie loads each file in a separate call for this reason, and also reads ~/.evie/.env, which finance never does.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "categorize.spec.md also specifies finance review, finance set and rules list or add; none of those subcommands exists, and rules only seeds from the fixed merchantLookup.json path.",
          "refs": [
            "doc:cmd/finance/docs/active/categorize.spec.md#finance-review",
            "doc:cmd/finance/docs/active/categorize.spec.md#finance-rules",
            "code:cmd/finance/main.go#main"
          ]
        },
        {
          "text": "db checks nothing beyond opening the file, and its message names only the items and transactions tables whatever else the schema holds.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/db.go#schema"
          ]
        },
        {
          "text": "The CLI shares ~/.finance/finance.db with evie serve; a unique index on finance_link_sessions allows one pending or exchanging link across both processes, so a CLI link picks up a pending flow the web started instead of starting another.",
          "refs": [
            "code:internal/finance/spending_accounts.go#spendingAccountsSchema",
            "code:internal/finance/spending_account_link.go#SpendingService.StartAccountLink",
            "code:internal/finance/link.go#linkWithService"
          ]
        }
      ],
      "terms": [
        "Budget Entry"
      ]
    },
    {
      "id": "pkg:cmd/ytscribe",
      "level": "component",
      "kind": "package",
      "title": "cmd/ytscribe",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The ytscribe command: the CLI for Evie's YouTube transcript library, with fetch, scrape, import and help, owning all printing.",
        "refs": [
          "code:cmd/ytscribe/main.go#runYTScribe",
          "code:cmd/ytscribe/main.go#printUsage",
          "doc:cmd/evie/docs/done/youtube-transcripts.spec.md#ytscribe-cli"
        ]
      },
      "does": [
        {
          "text": "fetch prints one video's metadata and full transcript, saying whether it came from the cache or the network, with --language (default en) and --refresh.",
          "refs": [
            "code:cmd/ytscribe/main.go#runFetch",
            "code:cmd/ytscribe/main.go#renderFetch"
          ]
        },
        {
          "text": "scrape walks a channel's videos serially, the whole channel when --limit is 0 (the default), printing one progress line per video and a summary, with --delay between network attempts (1.5 seconds by default).",
          "refs": [
            "code:cmd/ytscribe/main.go#runScrape",
            "code:cmd/ytscribe/main.go#renderScrapeSummary",
            "code:internal/youtube/service.go#Service.Scrape"
          ]
        },
        {
          "text": "import copies a legacy transcript corpus from a root folder and prints counts of seen, inserted, updated, skipped, matched, unmatched, failed and warnings.",
          "refs": [
            "code:cmd/ytscribe/main.go#runImport",
            "code:internal/youtube/import.go#ImportLegacy"
          ]
        }
      ],
      "owns": [
        {
          "text": "Nothing of its own: it opens the transcript library's writable database through internal/youtube for each command.",
          "refs": [
            "code:cmd/ytscribe/main.go#openYTScribeDB",
            "code:internal/youtube/db.go#OpenDB"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Exit codes: 2 for bad usage, 1 for a failed fetch or for any per-item failure in scrape or import, 0 otherwise.",
          "refs": [
            "code:cmd/ytscribe/main.go#runYTScribe",
            "code:cmd/ytscribe/main.go#runFetch",
            "code:cmd/ytscribe/main.go#runScrape",
            "code:cmd/ytscribe/main.go#runImport"
          ]
        },
        {
          "text": "The package variables openYTScribeDB, newYTScribeService and importYTScribeLegacy are its test seams.",
          "refs": [
            "code:cmd/ytscribe/main.go#openYTScribeDB",
            "code:cmd/ytscribe/main.go#newYTScribeService",
            "code:cmd/ytscribe/main.go#importYTScribeLegacy",
            "code:cmd/ytscribe/main_test.go"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "scrape with no limit walks a channel's full history, whereas the youtube_scrape_channel chat tool examines 1 to 50 newest videos, 10 by default.",
          "refs": [
            "code:cmd/ytscribe/main.go#runScrape",
            "code:internal/tools/youtube.go#youtubeScrapeChannel",
            "doc:cmd/evie/docs/done/youtube-transcripts.spec.md#evie-tools"
          ]
        },
        {
          "text": "Flag errors print only the usage text, because each flag set's own output is discarded.",
          "refs": [
            "code:cmd/ytscribe/main.go#runFetch",
            "code:cmd/ytscribe/main.go#runScrape"
          ]
        },
        {
          "text": "It writes the same transcripts database the Evie tools write, opened writable with the schema applied.",
          "refs": [
            "code:cmd/ytscribe/main.go#openYTScribeDB",
            "code:internal/tools/youtube.go#openYouTubeDB"
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/mcpconn",
      "level": "component",
      "kind": "package",
      "title": "mcpconn",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns MCP Connections: the record of each hosted MCP server, the OAuth sign-in that connects it, the Keychain that holds its credentials, the HTTP transport that carries its tokens, and the pool of live MCP sessions.",
        "refs": [
          "code:internal/mcpconn/connection.go#Connection",
          "code:internal/mcpconn/manager.go#Manager",
          "code:internal/mcpconn/pool.go#Pool"
        ]
      },
      "does": [
        {
          "text": "Adds a connection after validating its name (1 to 24 lowercase letters, digits or dashes, starting with a letter) and its https URL, recording it as not_connected under the Connection ID mcp-<name>.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Add",
            "code:internal/mcpconn/connection.go#ValidateName",
            "code:internal/mcpconn/connection.go#ValidateURL",
            "code:internal/mcpconn/connection.go#ConnectionID"
          ]
        },
        {
          "text": "Saves a pre-registered client's secret straight to the Keychain when the connection is added, before the record is created.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Add"
          ]
        },
        {
          "text": "Starts a sign-in by discovering the server's protected-resource metadata (from a 401's resource_metadata, else the well-known paths) and its authorization server's metadata, refusing a server without PKCE S256 or with non-https endpoints.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:internal/mcpconn/oauth.go#discover",
            "code:internal/mcpconn/oauth.go#probeResourceMetadata",
            "code:internal/mcpconn/oauth.go#authorizationServerMetadata"
          ]
        },
        {
          "text": "Settles the OAuth client: the pre-registered client ID, an earlier dynamic registration while its issuer and redirect URI are unchanged, or a new Dynamic Client Registration as a public client.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:internal/mcpconn/oauth.go#register"
          ]
        },
        {
          "text": "Keeps each pending attempt (state, PKCE verifier, client and metadata) in memory for ten minutes and returns an authorization URL carrying the S256 challenge, the resource indicator and any advertised scopes.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:internal/mcpconn/manager.go#attemptLifetime",
            "code:internal/mcpconn/oauth.go#authorizationURL"
          ]
        },
        {
          "text": "Completes the callback once per unexpired state: it exchanges the code with the verifier, saves the tokens to the Keychain, then records status connected with the OAuth metadata.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Callback",
            "code:internal/mcpconn/manager.go#Manager.completeAttempt",
            "code:internal/mcpconn/oauth.go#exchangeCode"
          ]
        },
        {
          "text": "Hands out each connection's bearer token, refreshing under that connection's lock a minute before expiry or once after a 401, and saving rotated tokens before using them.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.token",
            "code:internal/mcpconn/manager.go#refreshMargin",
            "code:internal/mcpconn/manager.go#Manager.TokenSource"
          ]
        },
        {
          "text": "Disconnects by revoking the refresh token (or the access token) when the server lists a revocation endpoint, keeping only the client registration in the Keychain, setting not_connected, and then running disconnect hooks such as closing the live session.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Disconnect",
            "code:internal/mcpconn/manager.go#Manager.forgetTokens",
            "code:internal/mcpconn/oauth.go#revoke",
            "code:internal/mcpconn/manager.go#Manager.OnDisconnect"
          ]
        },
        {
          "text": "Keeps at most one live MCP session per connection, opened on first use with the official SDK over Streamable HTTP, advertising no client capabilities and with multi-round-trip and the standalone SSE stream off.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.session",
            "code:internal/mcpconn/pool.go#Pool.connect"
          ]
        },
        {
          "text": "Lists a server's tools fresh on every request, hashing each tool's name, description, input schema and annotations as sorted-key JSON into its reviewed selector <name>@<sha256>.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.ListTools",
            "code:internal/mcpconn/pool.go#toolFromSDK",
            "code:internal/mcpconn/pool.go#Tool.Selector"
          ]
        },
        {
          "text": "Sends one tools/call per request and shapes the answer: text joined, structuredContent appended as JSON, and images, audio and resources as one-line placeholders.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.CallTool",
            "code:internal/mcpconn/pool.go#shapeResult"
          ]
        }
      ],
      "owns": [
        {
          "text": "The Connection record type, which never carries credentials; eviedb persists it in mcp_connections through the Records interface declared here.",
          "refs": [
            "code:internal/mcpconn/connection.go#Connection",
            "code:internal/mcpconn/manager.go#Records",
            "code:internal/eviedb/mcp_connections.go#Store.CreateMCPConnection"
          ]
        },
        {
          "text": "One Keychain generic-password item per connection, with service dev.evie.mcp and the Connection ID as account, holding the access and refresh tokens, expiry, client ID and client secret as JSON of at most 3,000 bytes.",
          "refs": [
            "code:internal/mcpconn/keychain.go#Keychain",
            "code:internal/mcpconn/keychain.go#keychainService",
            "code:internal/mcpconn/keychain.go#maxCredentialsJSON",
            "code:internal/mcpconn/connection.go#Credentials"
          ]
        },
        {
          "text": "The serve process's in-memory sign-in state: pending attempts keyed by state, and per connection a credential cache, its lock, and rotated credentials the Keychain has not accepted yet.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager",
            "code:internal/mcpconn/manager.go#attempt",
            "code:internal/mcpconn/manager.go#connectionState"
          ]
        },
        {
          "text": "The live MCP sessions, one entry per connection in the pool's map.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool",
            "code:internal/mcpconn/pool.go#poolEntry"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Manager offers List, Connection, Add, BeginConnect, Callback, Wait and Disconnect to the owner's surfaces; web's MCPConnections interface names the six its routes use.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager",
            "code:internal/web/connections.go#MCPConnections"
          ]
        },
        {
          "text": "It consumes Records (implemented by eviedb's Store) and Secrets (implemented by Keychain), wired by serve's composition.",
          "refs": [
            "code:internal/mcpconn/manager.go#Records",
            "code:internal/mcpconn/manager.go#Secrets",
            "code:cmd/evie/connections.go#newMCPConnectionManager"
          ]
        },
        {
          "text": "TokenSource (Token, and Refresh with the rejected token) is what the transport needs; NewHTTPClient builds a connection's client on Evie's transport, with no redirect following.",
          "refs": [
            "code:internal/mcpconn/transport.go#TokenSource",
            "code:internal/mcpconn/transport.go#NewHTTPClient",
            "code:internal/mcpconn/manager.go#Manager.TokenSource"
          ]
        },
        {
          "text": "Pool offers ListTools, CallTool, Close and CloseAll, and satisfies mcpchat's Tools interface.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool",
            "code:internal/mcpchat/service.go#Tools"
          ]
        },
        {
          "text": "A failed call comes back as a CallError whose outcome, from SendTrace, is not_sent, rejected or unknown.",
          "refs": [
            "code:internal/mcpconn/pool.go#CallError",
            "code:internal/mcpconn/transport.go#SendTrace.Outcome"
          ]
        },
        {
          "text": "Keychain runs /usr/bin/security through an injectable SecurityRunner, so tests capture argv and stdin.",
          "refs": [
            "code:internal/mcpconn/keychain.go#SecurityRunner",
            "code:internal/mcpconn/keychain.go#RunSecurity"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Only evie serve builds a Manager, so exactly one process holds and refreshes tokens; the evie connections commands are thin clients of serve's routes and fail when serve is not running.",
          "refs": [
            "code:internal/mcpconn/connection.go",
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/connections.go#runConnectionManagement"
          ]
        },
        {
          "text": "The code exchange and every refresh run detached from the caller's context with a 30-second limit, so a cancelled turn cannot lose a refresh token the server already rotated.",
          "refs": [
            "code:internal/mcpconn/manager.go#detached",
            "code:internal/mcpconn/manager.go#tokenWorkTimeout",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#connections"
          ]
        },
        {
          "text": "When the Keychain refuses rotated tokens they stay in memory marked unsaved and unused, and the next token request retries the save first.",
          "refs": [
            "code:internal/mcpconn/manager.go#connectionState",
            "code:internal/mcpconn/manager.go#Manager.token"
          ]
        },
        {
          "text": "Only an OAuth refusal (invalid_grant, invalid_client or unauthorized_client on a 400 or 401) sets needs_sign_in and drops the tokens; any other refresh failure sets unreachable and keeps the refresh token.",
          "refs": [
            "code:internal/mcpconn/oauth.go#requestToken",
            "code:internal/mcpconn/manager.go#Manager.token",
            "code:internal/mcpconn/manager.go#Manager.needSignIn"
          ]
        },
        {
          "text": "Secret material reaches security(1) only on stdin, as quoted base64 in a `security -i` line, never in argv.",
          "refs": [
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        },
        {
          "text": "The transport adds the bearer only over https to the connection's own host, re-sends exactly once after a 401 answered by a successful refresh, refuses redirects, and clears GetBody so net/http cannot replay a POST.",
          "refs": [
            "code:internal/mcpconn/transport.go#transport.RoundTrip",
            "code:internal/mcpconn/transport.go#transport.send",
            "code:internal/mcpconn/transport.go#NewHTTPClient"
          ]
        },
        {
          "text": "A server's JSON-RPC error is a complete answer (rejected) and keeps the shared session; only a broken connection drops it, and a caller's own cancellation or timeout does not.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.CallTool",
            "code:internal/mcpconn/pool.go#serverAnswered"
          ]
        },
        {
          "text": "No lock is held across network work: Close forgets a session and closes it in the background, and a session that finishes opening after a disconnect is closed and reported as disconnected.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.Close",
            "code:internal/mcpconn/pool.go#Pool.open"
          ]
        },
        {
          "text": "A tool listing stops silently at 500 tools per server.",
          "refs": [
            "code:internal/mcpconn/pool.go#maxTools",
            "code:internal/mcpconn/pool.go#Pool.ListTools"
          ]
        },
        {
          "text": "The redirect URI is built from EVIE_ADDR, and a dynamic registration is reused only while that URI and the issuer still match, so changing EVIE_ADDR means a new registration at the next connect.",
          "refs": [
            "code:cmd/evie/connections.go#newMCPConnectionManager",
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#risks"
          ]
        },
        {
          "text": "A used attempt stays in memory until it expires, and Wait keeps waiting for a callback that has already started, even past the deadline.",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Wait",
            "code:internal/mcpconn/manager.go#Manager.pruneLocked"
          ]
        }
      ],
      "terms": [
        "Account Connection",
        "Connection ID",
        "Kernel"
      ]
    },
    {
      "id": "pkg:internal/mcpconn/mcptest",
      "level": "component",
      "kind": "package",
      "title": "mcpconn/mcptest",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "A fake hosted MCP endpoint and OAuth authorization server for tests, shaped like Square's: a 401 with resource_metadata, Dynamic Client Registration for public clients, PKCE S256, and a rotated refresh token on every refresh.",
        "refs": [
          "code:internal/mcpconn/mcptest/authserver.go",
          "code:internal/mcpconn/mcptest/authserver.go#AuthServer"
        ]
      },
      "does": [
        {
          "text": "Runs one TLS httptest server whose /mcp answers 401 with a WWW-Authenticate resource_metadata URL unless the bearer is a token it issued, and serves protected-resource and authorization-server metadata.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#NewAuthServer",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve"
          ]
        },
        {
          "text": "Registers clients that ask for exactly one redirect URI and token_endpoint_auth_method none, issuing client IDs dcr-1, dcr-2 and so on.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve"
          ]
        },
        {
          "text": "Approve plays the owner signing in: it checks the authorization URL's parameters, including S256 and the resource, and returns the code and state the browser would carry to Evie's callback.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.Approve"
          ]
        },
        {
          "text": "Checks a code exchange against the stored client, redirect URI, PKCE challenge and resource, and on each refresh issues new tokens and retires the old refresh token.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.issueLocked"
          ]
        },
        {
          "text": "Can be switched to refuse registration, fail every refresh with invalid_grant, answer refreshes with any status and body, or run a hook between rotation and the answer.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.SetNoRegistration",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.SetRefreshFails",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.SetRefreshAnswer",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.SetAfterRotate"
          ]
        }
      ],
      "owns": [
        {
          "text": "Only in-memory test state: issued codes, live tokens, and a Snapshot of registrations, refreshes, token forms, revoked tokens and the Authorization headers its MCP endpoint saw.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer",
            "code:internal/mcpconn/mcptest/authserver.go#Snapshot",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.Snapshot"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "NewAuthServer(t), Resource, Approve, Snapshot and the Set switches; mcpconn's manager tests and eviedb's secret-leak test use it.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#NewAuthServer",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.Resource",
            "code:internal/mcpconn/manager_test.go",
            "code:internal/eviedb/mcp_connections_leak_test.go"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Its /mcp endpoint is not an MCP server: with a valid token it answers an empty JSON-RPC result to anything, so tests that list or call tools stand up the SDK's real Streamable HTTP server instead.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve",
            "code:internal/mcpconn/pool_test.go",
            "code:internal/mcpchat/integration_test.go"
          ]
        },
        {
          "text": "Its metadata lists both plain and S256 challenge methods and client_secret_basic, client_secret_post and none, so Evie's own choices decide what is sent.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve",
            "code:internal/mcpconn/oauth.go#requestToken",
            "code:internal/mcpconn/oauth.go#authorizationURL"
          ]
        },
        {
          "text": "It is a non-test package so that eviedb's leak test can run against the real SQLite file; the facts mark it as test support by its name.",
          "refs": [
            "doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md#story-1-review-2026-10-12",
            "pkg:internal/mcpconn/mcptest"
          ]
        },
        {
          "text": "Revocation records any token it is sent and only forgets it when it is a live refresh token.",
          "refs": [
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.serve"
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/mcpchat",
      "level": "component",
      "kind": "package",
      "title": "mcpchat",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "The Workspace side of MCP connections: decides which reviewed server tools a chat may reach, and applies the owner's tool review and Tool Policies to Workspace Access.",
        "refs": [
          "code:internal/mcpchat/service.go",
          "code:internal/mcpchat/service.go#Service"
        ]
      },
      "does": [
        {
          "text": "Computes a chat's reach: per mcp- Connection ID, the reviewed selectors present both in the Workspace Access revision the chat pinned at creation and in the current access; a chat with no pin reaches nothing.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Reach",
            "code:internal/eviedb/sessions.go#Store.createWorkspaceSession"
          ]
        },
        {
          "text": "Reviews a connection's live tools for one Workspace as enabled, changed (an older hash is reviewed), available or unselectable, plus the reviewed names the server no longer offers.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Review",
            "code:internal/mcpchat/service.go#review",
            "code:internal/mcpchat/service.go#ToolState"
          ]
        },
        {
          "text": "Enables a tool by replacing any older reviewed version with <name>@<hash>; the first enabled tool attaches the connection to the Workspace and disabling the last one removes it.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolEnabled",
            "code:internal/mcpchat/service.go#withTool"
          ]
        },
        {
          "text": "Writes each access change expecting the revision it read, so a concurrent change is refused and nothing is written.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolEnabled",
            "code:internal/eviedb/workflows.go#Store.SetWorkspaceAccess",
            "code:internal/web/connection_tools.go#toolReviewError"
          ]
        },
        {
          "text": "Sets Allow or Ask on the current version of an enabled tool, and answers whether a selector is allowed in the latest policy revision.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolPolicy",
            "code:internal/mcpchat/service.go#Service.Allowed"
          ]
        },
        {
          "text": "Passes Connection, ListTools and CallTool through to the record store and the pool, so one Service meets the mcp plugin's MCPChat interface.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Connection",
            "code:internal/mcpchat/service.go#Service.ListTools",
            "code:internal/mcpchat/service.go#Service.CallTool",
            "code:internal/plugins/mcp.go#MCPChat",
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "owns": [
        {
          "text": "No storage of its own: reviewed tools are Workspace Access resources under the Connection ID, and Tool Policies are eviedb's workspace_mcp_tool_policies revisions, both reached through its Store interface.",
          "refs": [
            "code:internal/mcpchat/service.go#Store",
            "code:internal/eviedb/workflows.go#Store.SetWorkspaceAccess",
            "code:internal/eviedb/mcp_tool_policies.go"
          ]
        },
        {
          "text": "The one definition of a reviewed tool's standing (ToolState) and of a Tool Policy (ask or allow), shared by the chat tools and the review routes.",
          "refs": [
            "code:internal/mcpchat/service.go#ToolState",
            "code:internal/mcpchat/service.go#ToolPolicy",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md#story-2-2026-10-12"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Service offers Reach, Connection, ListTools, CallTool, Allowed, Review, SetToolEnabled and SetToolPolicy; web's MCPToolReview names the last three.",
          "refs": [
            "code:internal/mcpchat/service.go#Service",
            "code:internal/web/connection_tools.go#MCPToolReview"
          ]
        },
        {
          "text": "It consumes Store, the access and policy state eviedb implements, and Tools, the server reach mcpconn's Pool implements.",
          "refs": [
            "code:internal/mcpchat/service.go#Store",
            "code:internal/mcpchat/service.go#Tools",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "SelectableName says whether a server tool's name may be enabled.",
          "refs": [
            "code:internal/mcpchat/service.go#SelectableName"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Disabling works without the server: a failed listing leaves the tools unknown and the review marked Unlisted with the reviewed names, so revoking never waits for a sign-in.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.reviewInputs",
            "code:internal/mcpchat/service.go#Service.SetToolEnabled",
            "code:internal/mcpchat/service.go#review"
          ]
        },
        {
          "text": "Disabling is two writes, the access change and then clearing Allow on every version of that tool, each with its own expected revision; if the second fails the access is already revoked.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolEnabled",
            "code:internal/mcpchat/service.go#Service.clearAllow"
          ]
        },
        {
          "text": "SelectableName measures len(name), so its 64 limit counts bytes, while the spec says characters.",
          "refs": [
            "code:internal/mcpchat/service.go#SelectableName",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#workspace-access-and-tool-review"
          ]
        },
        {
          "text": "Review and SetToolPolicy list the server's tools, so both need the connection signed in and reachable.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Review",
            "code:internal/mcpchat/service.go#Service.SetToolPolicy",
            "code:internal/mcpchat/service.go#Service.reviewInputs"
          ]
        },
        {
          "text": "Reach is an error, not an empty answer, when the chat's pin names a different Workspace from the one asked about.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Reach"
          ]
        },
        {
          "text": "Chats created before access pins existed have no pin and so reach no MCP tools.",
          "refs": [
            "code:internal/mcpchat/service.go#Service.Reach",
            "code:internal/eviedb/mcp_connections.go#ErrNoWorkspaceAccessPin",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md#story-2-2026-10-12"
          ]
        }
      ],
      "terms": [
        "Workspace",
        "Workspace Access",
        "Connection ID"
      ]
    },
    {
      "id": "pkg:internal/github",
      "level": "component",
      "kind": "package",
      "title": "github",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "The small GitHub REST client the Changes feature reads and writes through, with one fine-grained token sent only in the Authorization header.",
        "refs": [
          "code:internal/github/client.go",
          "code:internal/github/client.go#Client",
          "code:internal/github/pulls.go#Client.CreatePullRequest"
        ]
      },
      "does": [
        {
          "text": "Reads the token's user, which proves the token works, one repository, and the token owner's own repositories, newest first.",
          "refs": [
            "code:internal/github/client.go#Client.User",
            "code:internal/github/client.go#Client.Repository",
            "code:internal/github/client.go#Client.Repositories"
          ]
        },
        {
          "text": "Creates a repository on the token owner's personal account, optionally with GitHub's first commit so it can be cloned.",
          "refs": [
            "code:internal/github/client.go#Client.CreateRepository"
          ]
        },
        {
          "text": "Lists open pull requests and the pull requests on one head branch in any state, and reads one pull request with its merged and mergeable fields.",
          "refs": [
            "code:internal/github/pulls.go#Client.PullRequests",
            "code:internal/github/pulls.go#Client.PullRequestsOnBranch",
            "code:internal/github/pulls.go#Client.PullRequest",
            "code:internal/github/pulls.go#PullRequest"
          ]
        },
        {
          "text": "Reads a commit's checks as GitHub Actions workflow runs and commit statuses, because GitHub's Checks API does not accept fine-grained tokens.",
          "refs": [
            "code:internal/github/pulls.go#Client.WorkflowRuns",
            "code:internal/github/pulls.go#Client.CombinedStatus",
            "code:internal/github/pulls.go#WorkflowRun",
            "doc:cmd/evie/docs/active/changes.spec.md#github-connection"
          ]
        },
        {
          "text": "Opens a ready (not draft) pull request, merges one with Rebase and merge only while its head is the expected SHA, and closes one without merging.",
          "refs": [
            "code:internal/github/pulls.go#Client.CreatePullRequest",
            "code:internal/github/pulls.go#Client.MergePullRequest",
            "code:internal/github/pulls.go#Client.ClosePullRequest"
          ]
        },
        {
          "text": "Maps GitHub's answers to typed errors: 401 unauthorized, 404 not found or invisible, 403 forbidden, 422 refused, 405 not mergeable and 409 head changed, carrying GitHub's own explanation cut to 200 bytes.",
          "refs": [
            "code:internal/github/client.go#Client.do",
            "code:internal/github/client.go#message"
          ]
        }
      ],
      "owns": [
        {
          "text": "No state beyond its API base, token and HTTP client; the repositories and pull requests live on GitHub, and the changes sync records what it reads through eviedb.",
          "refs": [
            "code:internal/github/client.go#Client",
            "code:internal/changes/sync.go#PullRequestStore"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Client and its methods; changes declares the slices it consumes as GitHubClient, PullRequestClient, PullRequestOpener and PullRequestMerger.",
          "refs": [
            "code:internal/github/client.go#Client",
            "code:internal/changes/repository.go#GitHubClient",
            "code:internal/changes/sync.go#PullRequestClient",
            "code:internal/changes/push.go#PullRequestOpener",
            "code:internal/changes/merge.go#PullRequestMerger"
          ]
        },
        {
          "text": "New takes the API base so tests point it at a fake server; API is the production base.",
          "refs": [
            "code:internal/github/client.go#New",
            "code:internal/github/client.go#API"
          ]
        },
        {
          "text": "ValidName and the invalid-name, number, SHA and ref errors are checked before any request is made.",
          "refs": [
            "code:internal/github/client.go#ValidName",
            "code:internal/github/client.go#ErrInvalidName",
            "code:internal/github/pulls.go#ErrInvalidNumber",
            "code:internal/github/pulls.go#ErrInvalidSHA",
            "code:internal/github/pulls.go#ErrInvalidRef"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Every list is one page of at most 100 with no pagination; sync makes up for pull requests past the first page by reading each one it last saw open.",
          "refs": [
            "code:internal/github/pulls.go#Client.PullRequests",
            "code:internal/github/client.go#Client.Repositories",
            "code:internal/github/pulls.go#Client.WorkflowRuns",
            "code:internal/changes/sync.go#Syncer.sync"
          ]
        },
        {
          "text": "GitHub answers 404 both for a missing repository and for one the token cannot see, so callers word it as an access problem.",
          "refs": [
            "code:internal/github/client.go#ErrNotFound",
            "code:internal/changes/repository.go#refusal"
          ]
        },
        {
          "text": "The token is GITHUB_TOKEN, read by startup composition; without it no client is built and GitHub actions are refused with ErrNoToken.",
          "refs": [
            "code:cmd/evie/changes_push.go#changePusher",
            "code:cmd/evie/main.go#startChangeSync",
            "code:internal/changes/publish.go#ErrNoToken"
          ]
        },
        {
          "text": "A transport failure names only the host and unwraps the URL error, so no request URL reaches an error message.",
          "refs": [
            "code:internal/github/client.go#Client.do"
          ]
        },
        {
          "text": "The default HTTP client times out after ten seconds, and a response is read up to 8 MB.",
          "refs": [
            "code:internal/github/client.go#New",
            "code:internal/github/client.go#Client.do"
          ]
        }
      ],
      "terms": [
        "Change",
        "Repository"
      ]
    },
    {
      "id": "pkg:internal/finance",
      "level": "component",
      "kind": "package",
      "title": "finance",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns the personal-finance domain: linking banks through Plaid Hosted Link, syncing transactions into ~/.finance/finance.db, rule-based categorization into Budget Entries, the review workflow's read and write, and the Spending views' queries.",
        "refs": [
          "code:internal/finance/db.go",
          "code:internal/finance/sync.go#Sync",
          "code:internal/finance/categorize.go#Categorize",
          "code:internal/finance/spending.go#SpendingService"
        ]
      },
      "does": [
        {
          "text": "Opens the one database at ~/.finance/finance.db, applying the whole schema on every writable open and locking the file to 0600 because Plaid access tokens live in it.",
          "refs": [
            "code:internal/finance/db.go#OpenDBContext",
            "code:internal/finance/db.go#openDBAtContext",
            "code:internal/finance/db.go#schema"
          ]
        },
        {
          "text": "Links a bank through Plaid Hosted Link as a durable flow, at most one pending at a time, and saves the exchanged access token with the item.",
          "refs": [
            "code:internal/finance/spending_account_link.go#SpendingService.StartAccountLink",
            "code:internal/finance/spending_account_link.go#SpendingService.CompleteAccountLink",
            "code:internal/finance/spending_account_link.go#saveLinkedAccountItem",
            "code:internal/finance/spending_accounts.go#spendingAccountsSchema"
          ]
        },
        {
          "text": "Syncs every linked bank through /transactions/sync, committing each page with its cursor in one SQL transaction; one bank's failure does not stop the others, and a mutation-during-pagination error is retried once from the committed cursor.",
          "refs": [
            "code:internal/finance/sync.go#Sync",
            "code:internal/finance/sync.go#syncOneItem",
            "code:internal/finance/sync.go#applySyncPage"
          ]
        },
        {
          "text": "Deletes a removed transaction's budget entries before the transaction itself, so a retracted pending row cannot wedge a bank's sync.",
          "refs": [
            "code:internal/finance/sync.go#applySyncPage"
          ]
        },
        {
          "text": "Categorizes every posted transaction without a Budget Entry: the legacy category column first, then an exact case-insensitive merchant rule, then the lowest-priority matching name rule, with disagreement at equal priority an error.",
          "refs": [
            "code:internal/finance/categorize.go#categorizeTx",
            "code:internal/finance/categorize.go#matchTxn",
            "code:internal/finance/categorize.go#matchName"
          ]
        },
        {
          "text": "Seeds merchant rules from a JSON file and name rules from an embedded, owner-reviewed seed, both as upserts.",
          "refs": [
            "code:internal/finance/categorize.go#RulesSeed",
            "code:internal/finance/name_rules.go#NameRulesSeed",
            "code:internal/finance/name_rules.go#SeedNameRules"
          ]
        },
        {
          "text": "Performs the workflow effects sync_ledger, categorize_ledger and apply_review once per idempotency key, recording an effect receipt so a replay returns the recorded output and reconciliation answers from it.",
          "refs": [
            "code:internal/finance/effects.go#SyncLedger",
            "code:internal/finance/effects.go#CategorizeLedger",
            "code:internal/finance/review_apply.go#ApplyReview",
            "code:internal/finance/effects.go#LookupEffect"
          ]
        },
        {
          "text": "Builds review batches of transactions awaiting review with deterministic evidence, and applies the owner's answered batch through the same rechecks Categorize uses.",
          "refs": [
            "code:internal/finance/review.go#ReviewCandidates",
            "code:internal/finance/review_apply.go#ApplyReview"
          ]
        },
        {
          "text": "Serves the Spending views: the yearly daily flow, monthly cash flow, transactions by classification, one day's detail, a single category edit, a refresh, and the connected-account inventory.",
          "refs": [
            "code:internal/finance/spending.go#SpendingService.InspectSpending",
            "code:internal/finance/spending_cash_flow.go#SpendingService.InspectSpendingCashFlow",
            "code:internal/finance/spending_transactions.go#SpendingService.InspectSpendingTransactions",
            "code:internal/finance/spending_day.go#SpendingService.InspectSpendingDay",
            "code:internal/finance/spending_day.go#SpendingService.UpdateSpendingCategory",
            "code:internal/finance/spending.go#SpendingService.RefreshSpending",
            "code:internal/finance/spending_accounts.go#SpendingService.RefreshAccounts"
          ]
        }
      ],
      "owns": [
        {
          "text": "The finance database's tables: items, transactions, categories, rules, name_rules, budget_entries, budget_limits, review_decisions and effect_receipts, plus finance_accounts, finance_account_snapshots and finance_link_sessions.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "code:internal/finance/spending_accounts.go#spendingAccountsSchema"
          ]
        },
        {
          "text": "Each linked bank's Plaid access token and sync cursor, in items.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "code:internal/finance/sync.go#Sync"
          ]
        },
        {
          "text": "The categories excluded and Credit Card Payments, whose entries count toward no Overview figure.",
          "refs": [
            "code:internal/finance/categorize.go#ExcludedCategory",
            "code:internal/finance/categorize.go#CreditCardPaymentsCategory",
            "code:internal/finance/categorize.go#OverviewOmitted"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Package functions shared by the CLI and the agent (OpenDB, Link, Sync, Categorize, the seeders, the effects and the review functions) return data rather than printing, with Link the exception.",
          "refs": [
            "code:internal/finance/db.go",
            "code:internal/finance/link.go#Link"
          ]
        },
        {
          "text": "SpendingService backs the web Spending and account routes, whose SpendingService and SpendingAccountsService interfaces name what they use.",
          "refs": [
            "code:internal/finance/spending.go#SpendingService",
            "code:internal/web/spending.go#SpendingService",
            "code:internal/web/spending_accounts.go#SpendingAccountsService"
          ]
        },
        {
          "text": "Plaid access uses PLAID_CLIENT_ID and PLAID_SECRET against Plaid's production environment; there is no sandbox mode.",
          "refs": [
            "code:internal/finance/client.go#plaidClient"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Plaid access tokens sit in plain text in finance.db (mode 0600); MCP's tokens went to the Keychain partly to avoid this, and moving Plaid there is a stated non-goal.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "code:internal/finance/db.go#openDBAtContext",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md#the-owners-decisions",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#non-goals"
          ]
        },
        {
          "text": "Pending transactions never get a Budget Entry (the Pending Fence): Plaid replaces them with posted rows, and an entry at the pending amount would be wrong.",
          "refs": [
            "code:internal/finance/categorize.go#categorizeTx",
            "code:internal/finance/categorize.go#insertCategorizedCandidate"
          ]
        },
        {
          "text": "The schema is CREATE IF NOT EXISTS, applied on every writable open; there is no migration step.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "code:internal/finance/db.go#openDBAtContext"
          ]
        },
        {
          "text": "The sync gate is a channel inside one process, so the finance CLI and evie serve are not serialized against each other; each page still commits atomically with its cursor.",
          "refs": [
            "code:internal/finance/sync.go#syncGate",
            "code:internal/finance/sync.go#applySyncPage"
          ]
        },
        {
          "text": "categorize.spec.md describes categorizing through the reviewed and category_source columns, but the code mints budget_entries for posted transactions that have none and no longer writes the legacy category column.",
          "refs": [
            "doc:cmd/finance/docs/active/categorize.spec.md#finance-categorize",
            "code:internal/finance/categorize.go#Categorize"
          ]
        },
        {
          "text": "sync_ledger records its receipt only after the last bank, so an interrupted sync leaves none although some pages committed; a re-sent intent continues from the committed cursor.",
          "refs": [
            "code:internal/finance/effects.go",
            "code:internal/finance/effects.go#SyncLedger",
            "doc:cmd/evie/docs/active/finance-daily.spec.md#receipts-internalfinanceeffectsgo"
          ]
        },
        {
          "text": "Spend queries that sum across all categories must leave out excluded and Credit Card Payments.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "code:internal/finance/categorize.go#overviewOmittedSQL"
          ]
        },
        {
          "text": "A category edit opens an existing database only, so it can never create the file or apply the schema.",
          "refs": [
            "code:internal/finance/spending_day.go#openSpendingExisting"
          ]
        }
      ],
      "terms": [
        "Budget Entry",
        "Awaiting Review",
        "Pending Fence",
        "Effect Intent",
        "Effect Receipt"
      ]
    },
    {
      "id": "pkg:internal/youtube",
      "level": "component",
      "kind": "package",
      "title": "youtube",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns Evie's durable YouTube transcript library: its database, the HTTP protocol that reads YouTube, cache-first fetch and channel scrape, and the legacy corpus import.",
        "refs": [
          "code:internal/youtube/db.go",
          "code:internal/youtube/service.go#Service",
          "code:internal/youtube/client.go#Client",
          "code:internal/youtube/import.go#ImportLegacy"
        ]
      },
      "does": [
        {
          "text": "Fetches one video's transcript cache-first: a cached transcript, or a cached terminal outcome, is returned without touching YouTube unless refresh is set.",
          "refs": [
            "code:internal/youtube/service.go#Service.Fetch"
          ]
        },
        {
          "text": "Reads YouTube by fetching the watch page for its API key, asking the youtubei player endpoint as the Android client, and downloading the chosen caption track as json3.",
          "refs": [
            "code:internal/youtube/client.go#Client.fetchVideo",
            "code:internal/youtube/client.go#json3CaptionURL"
          ]
        },
        {
          "text": "Saves the channel, video, transcript and ready state together, and records terminal outcomes (no captions, language unavailable, unavailable) so later fetches skip them.",
          "refs": [
            "code:internal/youtube/service.go#Service.fetchRemote",
            "code:internal/youtube/store.go#saveRemote",
            "code:internal/youtube/service.go#Service.persistTerminalState",
            "code:internal/youtube/db.go#schema"
          ]
        },
        {
          "text": "Scrapes a channel serially: lists its newest videos (all of them when the limit is zero), saves the listing, skips cached transcripts and terminal outcomes, and waits between network attempts.",
          "refs": [
            "code:internal/youtube/service.go#Service.Scrape"
          ]
        },
        {
          "text": "Imports a legacy file corpus by copying transcript files, never moving or editing them.",
          "refs": [
            "code:internal/youtube/import.go#ImportLegacy",
            "doc:cmd/evie/docs/done/youtube-transcripts.spec.md#legacy-import"
          ]
        },
        {
          "text": "Indexes transcripts in an FTS5 table kept current by triggers, and offers a transcript_library view that prefers a manual transcript over a generated one over a legacy one.",
          "refs": [
            "code:internal/youtube/db.go#schema"
          ]
        }
      ],
      "owns": [
        {
          "text": "The transcript database at ~/.evie/transcripts/transcripts.db (folder 0700, file 0600): channels, videos, transcripts, transcript_states, the transcript_library view and the transcript_fts index.",
          "refs": [
            "code:internal/youtube/db.go#databasePath",
            "code:internal/youtube/db.go#OpenDBContext",
            "code:internal/youtube/db.go#schema"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "OpenDB, OpenDBReadOnly and OpenDBAt; NewClient and NewService with Fetch and Scrape; and ImportLegacy.",
          "refs": [
            "code:internal/youtube/db.go#OpenDB",
            "code:internal/youtube/db.go#OpenDBReadOnly",
            "code:internal/youtube/db.go#OpenDBAt",
            "code:internal/youtube/client.go#NewClient",
            "code:internal/youtube/service.go#NewService",
            "code:internal/youtube/import.go#ImportLegacy"
          ]
        },
        {
          "text": "The youtube_transcript and youtube_scrape_channel tools and ytscribe are thin adapters, and query_db reads the library through the read-only opener.",
          "refs": [
            "code:internal/tools/youtube.go#YouTubeTools",
            "code:cmd/ytscribe/main.go#runYTScribe",
            "code:internal/tools/db.go#openTranscriptDB"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The protocol uses YouTube's internal youtubei player endpoint posing as Android client 20.10.38, not a public API.",
          "refs": [
            "code:internal/youtube/client.go#Client.fetchVideo",
            "code:internal/youtube/client.go#androidClientVersion"
          ]
        },
        {
          "text": "A request is tried at most three times, retrying only on 429 or 5xx with Retry-After honored up to 30 seconds; consent, reCAPTCHA and bot-confirmation pages fail at once.",
          "refs": [
            "code:internal/youtube/client.go#Client.do",
            "code:internal/youtube/client.go#retryDelay",
            "code:internal/youtube/client.go#detectBlockingResponse"
          ]
        },
        {
          "text": "Responses are capped at 10 MiB, and the default client times out after 30 seconds.",
          "refs": [
            "code:internal/youtube/client.go#readBoundedBody",
            "code:internal/youtube/client.go#NewClient"
          ]
        },
        {
          "text": "Caption URLs and their redirects are validated before they are followed.",
          "refs": [
            "code:internal/youtube/client.go#validateCaptionRequestURL",
            "code:internal/youtube/client.go#NewClient"
          ]
        },
        {
          "text": "A refresh that fails keeps the existing transcript and its ready state.",
          "refs": [
            "code:internal/youtube/service.go#Service.Fetch",
            "code:internal/youtube/service.go#Service.fetchRemote",
            "doc:cmd/evie/docs/done/youtube-transcripts.spec.md#evie-tools"
          ]
        },
        {
          "text": "The schema is one idempotent blob applied on every writable open, with no migration framework.",
          "refs": [
            "code:internal/youtube/db.go#OpenDBAtContext",
            "doc:cmd/evie/docs/done/youtube-transcripts.spec.md#location-and-opening"
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/changes",
      "level": "component",
      "kind": "package",
      "title": "changes",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "A Workspace's Repository and the Changes made in it: detection, creation and publishing, the merged list of Evie's build branches and GitHub pull requests, sync, push, merge, close and cleanup.",
        "refs": [
          "code:internal/changes/repository.go",
          "code:internal/changes/change.go#Change",
          "code:internal/changes/merge.go#Merger"
        ]
      },
      "does": [
        {
          "text": "Reads the Repository from the Workspace folder every time, never storing it: none, local (a non-bare work tree's top level), or GitHub when origin names github.com/<owner>/<name> in HTTPS, SSH or scp-like form.",
          "refs": [
            "code:internal/changes/repository.go#Service.Repository",
            "code:internal/changes/repository.go#gitHubRepository"
          ]
        },
        {
          "text": "For a GitHub Repository it checks the token against GitHub, takes GitHub's default branch as the base, and counts how many commits the local base branch is behind origin's.",
          "refs": [
            "code:internal/changes/repository.go#Service.check",
            "code:internal/changes/repository.go#behind"
          ]
        },
        {
          "text": "Sets up a new Workspace's Repository before the Workspace is registered: a new local repository on main with one empty commit, a new GitHub repository cloned into the folder, or an existing one cloned or adopted when its origin matches.",
          "refs": [
            "code:internal/changes/prepare.go#Service.Prepare",
            "code:internal/changes/prepare.go#Service.newLocal",
            "code:internal/changes/prepare.go#Service.newGitHub",
            "code:internal/changes/prepare.go#Service.existingGitHub"
          ]
        },
        {
          "text": "Publishes a local Repository: creates the GitHub repository, sets origin, and pushes only the base branch; publishing again with the same name retries the push.",
          "refs": [
            "code:internal/changes/publish.go#Service.Publish"
          ]
        },
        {
          "text": "Lists Changes by merging Evie's build branches, read from build attempts, with the synced pull requests, linking a pull request to the build on its branch and keeping the 20 most recent finished ones.",
          "refs": [
            "code:internal/changes/change.go#Lister.List",
            "code:internal/changes/change.go#link",
            "code:internal/changes/change.go#ordered",
            "code:internal/changes/builds.go#BuildSource.Changes"
          ]
        },
        {
          "text": "Syncs each GitHub Repository every 60 seconds while serve runs, one sync per Workspace at a time: open pull requests, the checks on each head, and pull requests that left the open list.",
          "refs": [
            "code:internal/changes/sync.go#Syncer.Run",
            "code:internal/changes/sync.go#Syncer.Sync",
            "code:internal/changes/sync.go#Syncer.sync",
            "code:cmd/evie/main.go#startChangeSync"
          ]
        },
        {
          "text": "Pushes one of Evie's draft branches over HTTPS, after running the Workspace's Verify command when one is set, then opens or finds its pull request into the base the Change was built from.",
          "refs": [
            "code:internal/changes/push.go#Pusher.Push",
            "code:internal/changes/push.go#Pusher.push",
            "code:internal/changes/push.go#Pusher.openOrFind"
          ]
        },
        {
          "text": "Merges for the owner (a local Change through the Kernel's merge gate, a GitHub Change by Rebase and merge of the head Evie last showed), and arms or disarms Merge when checks pass, which each sync then fires.",
          "refs": [
            "code:internal/changes/merge.go#Merger.Merge",
            "code:internal/changes/merge.go#Merger.mergeOnGitHub",
            "code:internal/changes/merge.go#Merger.Arm",
            "code:internal/changes/merge.go#Merger.Disarm",
            "code:internal/changes/merge.go#Merger.fire"
          ]
        },
        {
          "text": "After a merge or close it deletes Evie's own branch on GitHub and locally, removes its worktree, and fast-forwards the owner's base branch only when it is checked out with no uncommitted edits.",
          "refs": [
            "code:internal/changes/merge.go#Merger.cleanUp",
            "code:internal/changes/merge.go#Merger.catchUp",
            "code:internal/changes/merge.go#Merger.AfterSync"
          ]
        },
        {
          "text": "Shows one Change with its diff through localfiles.Compare, fetching a pull request into refs/evie/pull/<number> when no branch of Evie's holds it.",
          "refs": [
            "code:internal/changes/show.go#Lister.Show",
            "code:internal/changes/show.go#Lister.span",
            "code:internal/localfiles/compare.go#Compare"
          ]
        }
      ],
      "owns": [
        {
          "text": "The Change model (draft, open, merged and closed states; checks; the armed and needs-rebase markers) shared by the web routes and the CLI.",
          "refs": [
            "code:internal/changes/change.go#Change",
            "code:internal/changes/change.go#State",
            "code:internal/changes/change.go#CheckState"
          ]
        },
        {
          "text": "No tables of its own: Evie's Changes are derived from build attempts, while synced pull requests, merge arms and Change notices are stored by eviedb through interfaces declared here.",
          "refs": [
            "code:internal/changes/builds.go#BuildRecords",
            "code:internal/changes/sync.go#PullRequestStore",
            "code:internal/changes/merge.go#MergeRecords",
            "code:internal/eviedb/change_pull_requests.go#Store.SaveChangePullRequests"
          ]
        },
        {
          "text": "The in-process folder claim that lets one action at a time change a Repository, and the per-Workspace sync claim.",
          "refs": [
            "code:internal/changes/publish.go#Service.claim",
            "code:internal/changes/sync.go#Syncer.claim"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Service (Repository, Prepare, Publish, GitHubRepositories, Account), Lister (List, Show), Syncer (Run, Sync, Refresh), Pusher (Push, PushForWorkflow, FindPullRequest) and Merger (Merge, Close, Arm, Disarm, AfterSync).",
          "refs": [
            "code:internal/changes/repository.go#Service",
            "code:internal/changes/change.go#Lister",
            "code:internal/changes/sync.go#Syncer",
            "code:internal/changes/push.go#Pusher",
            "code:internal/changes/merge.go#Merger"
          ]
        },
        {
          "text": "It consumes GitHubClient, PullRequestClient, PullRequestOpener and PullRequestMerger for GitHub, BuildGate for the Kernel's merge gate, Verifier for Verify, and Source, BuildRecords, PullRequestRecords, PullRequestStore, MergeRecords and MergeArms for storage.",
          "refs": [
            "code:internal/changes/repository.go#GitHubClient",
            "code:internal/changes/sync.go#PullRequestClient",
            "code:internal/changes/push.go#PullRequestOpener",
            "code:internal/changes/merge.go#PullRequestMerger",
            "code:internal/changes/merge.go#BuildGate",
            "code:internal/changes/push.go#Verifier",
            "code:internal/changes/change.go#Source",
            "code:internal/changes/builds.go#BuildRecords",
            "code:internal/changes/pullrequests.go#PullRequestRecords",
            "code:internal/changes/merge.go#MergeRecords",
            "code:internal/changes/pullrequests.go#MergeArms"
          ]
        },
        {
          "text": "Repository.Resource names a GitHub Repository to workflows as repo:<owner>/<name>, the resource a workflow's push must name.",
          "refs": [
            "code:internal/changes/push.go#Repository.Resource",
            "code:internal/changes/push.go#Pusher.PushForWorkflow"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The spec's Change model says Changes are stored per Workspace with their state and markers, but nothing is stored for Evie's builds: their state is derived from each build attempt's gate and worktree, and only pull requests are stored.",
          "refs": [
            "doc:cmd/evie/docs/active/changes.spec.md#change-model",
            "code:internal/changes/builds.go#BuildSource.Changes",
            "code:internal/changes/builds.go#buildChange"
          ]
        },
        {
          "text": "The token reaches git only as an http.extraheader in a per-command environment, with credential helpers and every protocol but HTTPS off, and it is scrubbed from error text.",
          "refs": [
            "code:internal/changes/publish.go#Service.remoteGit",
            "code:internal/changes/publish.go#Service.remoteEnv",
            "code:internal/changes/publish.go#Service.scrub"
          ]
        },
        {
          "text": "A merge's own sync cannot run AfterSync on the folder the merge still holds, so a merge cleans up and catches up by itself; AfterSync skips a busy folder until the next sync.",
          "refs": [
            "code:internal/changes/merge.go#Merger.Merge",
            "code:internal/changes/merge.go#Merger.mergeOnGitHub",
            "code:internal/changes/merge.go#Merger.AfterSync",
            "code:internal/changes/publish.go#Service.claim"
          ]
        },
        {
          "text": "The folder claim is a map inside one Service, so it serializes actions within one process only; evie changes merge builds its own Merger in the CLI process.",
          "refs": [
            "code:internal/changes/publish.go#Service.claim",
            "code:cmd/evie/changes.go#runChangeManagement"
          ]
        },
        {
          "text": "A GitHub merge is refused when the pull request has new commits since Evie last read it, is a GitHub draft, or is reported unmergeable; a conflict is recorded against that head as needs rebase.",
          "refs": [
            "code:internal/changes/merge.go#Merger.mergeOnGitHub",
            "code:internal/changes/merge.go#Merger.needsRebase"
          ]
        },
        {
          "text": "Merge when checks pass needs at least one check, merges once every check passed or was neutral or skipped, clears on a failed check or a conflict, and stays set across new commits and errors that may pass.",
          "refs": [
            "code:internal/changes/merge.go#Merger.Arm",
            "code:internal/changes/merge.go#Merger.fire",
            "code:internal/changes/merge.go#checksPassed",
            "code:internal/changes/sync.go#workflowRunState"
          ]
        },
        {
          "text": "A sync that GitHub refuses records nothing, so a missing check is never read as no checks.",
          "refs": [
            "code:internal/changes/sync.go#Syncer.sync",
            "doc:cmd/evie/docs/active/changes.spec.md#change-model"
          ]
        },
        {
          "text": "A workflow's push needs Verify set and passing, while the owner's push goes ahead after a failed Verify and the pull request body says so.",
          "refs": [
            "code:internal/changes/push.go#Pusher.push",
            "code:internal/changes/push.go#pullRequestBody"
          ]
        },
        {
          "text": "Opening a pull request that fails without GitHub refusing it is ErrOpenUncertain: the pull request may exist, and pushing again finds it.",
          "refs": [
            "code:internal/changes/push.go#Pusher.openOrFind",
            "code:internal/changes/push.go#ErrOpenUncertain"
          ]
        },
        {
          "text": "Cleanup deletes a branch only when the Change is Evie's and the branch starts with evie/; the owner's branches are never deleted.",
          "refs": [
            "code:internal/changes/merge.go#Merger.cleanUp"
          ]
        }
      ],
      "terms": [
        "Repository",
        "Change",
        "Workspace",
        "Workflow Run",
        "Kernel"
      ]
    },
    {
      "id": "pkg:internal/worktree",
      "level": "component",
      "kind": "package",
      "title": "worktree",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Owns the lifecycle of a build worker's git worktree as the harness: plan it, create it on a fresh evie/ branch, commit the worker's changes, report the branch, run the merge gate's rebase and fast-forward, and clean it up.",
        "refs": [
          "code:internal/worktree/worktree.go",
          "code:internal/worktree/worktree.go#Plan",
          "code:internal/worktree/gate.go#FastForward"
        ]
      },
      "does": [
        {
          "text": "Plans without changing anything: the folder must be a non-bare repository's top level with no merge, rebase or similar in progress, the base (by default the checked-out branch) is pinned to a commit, and the branch is evie/<id prefix>-<slug of the objective>.",
          "refs": [
            "code:internal/worktree/worktree.go#Plan",
            "code:internal/worktree/worktree.go#slug"
          ]
        },
        {
          "text": "Places a worktree at <repo>/.evie/worktrees/<attempt> with a private temp folder at <repo>/.evie/tmp/<attempt>, and adds /.evie/ to the repository's local info/exclude, never to a tracked file.",
          "refs": [
            "code:internal/worktree/worktree.go",
            "code:internal/worktree/worktree.go#Ensure",
            "code:internal/worktree/worktree.go#ensureExcluded"
          ]
        },
        {
          "text": "Creates the worktree on its branch from the pinned base commit, or proves an existing one is a plain folder linked to its pinned git directory and on its branch.",
          "refs": [
            "code:internal/worktree/worktree.go#Ensure",
            "code:internal/worktree/worktree.go#verify"
          ]
        },
        {
          "text": "Commits every change as the harness, with the repository's identity or Evie's when none is set and no hooks, and refuses a worktree that holds a nested .git.",
          "refs": [
            "code:internal/worktree/worktree.go#Commit",
            "code:internal/worktree/worktree.go#nestedRepository",
            "code:internal/worktree/worktree.go#ErrNestedRepository"
          ]
        },
        {
          "text": "Reports a branch against its base: its head, up to 50 commits, the files changed with line counts, and whether uncommitted changes remain.",
          "refs": [
            "code:internal/worktree/worktree.go#Report",
            "code:internal/worktree/worktree.go#Status"
          ]
        },
        {
          "text": "Rebases a branch inside its worktree, and on a conflict aborts so the branch stays as it was and reports the conflicting files.",
          "refs": [
            "code:internal/worktree/gate.go#Rebase",
            "code:internal/worktree/gate.go#RebaseResult"
          ]
        },
        {
          "text": "Fast-forwards a merge target with git merge --ff-only in the checkout that has it, which refuses to overwrite uncommitted changes, and otherwise by a compare-and-swap of the ref; it never merges into a build worktree and never pushes.",
          "refs": [
            "code:internal/worktree/gate.go#FastForward",
            "code:internal/worktree/gate.go#MergeRefusedError",
            "code:internal/worktree/gate.go#ErrTargetMoved"
          ]
        },
        {
          "text": "Removes a worktree and its temp folder, and its branch when asked, pruning a worktree that is already gone.",
          "refs": [
            "code:internal/worktree/worktree.go#Remove"
          ]
        },
        {
          "text": "Proposes setup and verify commands found in AGENTS.md and CLAUDE.md, as a suggestion only, for Workspace creation and the Verify settings; the owner sets the commands.",
          "refs": [
            "code:internal/worktree/gate.go#ProposeCommands",
            "code:internal/worktree/gate.go#Proposal",
            "code:cmd/evie/workspace_creation.go",
            "code:cmd/evie/build_verify.go"
          ]
        }
      ],
      "owns": [
        {
          "text": "The .evie/worktrees and .evie/tmp folders inside a repository and the evie/ branches it creates; the Worktree record is delegation's type, persisted by its callers.",
          "refs": [
            "code:internal/worktree/worktree.go#Folder",
            "code:internal/worktree/worktree.go#Plan",
            "code:internal/delegation/types.go#Worktree"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Plan, Ensure, Commit and Report run a build worker's worktree for subagents; Status and Remove also back the evie worktrees command; Exists checks that a worktree folder is present.",
          "refs": [
            "code:internal/worktree/worktree.go#Plan",
            "code:internal/worktree/worktree.go#Ensure",
            "code:internal/worktree/worktree.go#Commit",
            "code:internal/worktree/worktree.go#Report",
            "code:internal/worktree/worktree.go#Status",
            "code:internal/worktree/worktree.go#Remove",
            "code:internal/worktree/gate.go#Exists",
            "code:internal/subagents/supervisor.go",
            "code:cmd/evie/worktrees.go#runWorktreeManagement"
          ]
        },
        {
          "text": "BranchHead, TargetHead, IsAncestor, Rebase, AbortRebase, FastForward, Compare, ChangedPaths, TrackedChanges and OwnerHistoryContains for the merge gate.",
          "refs": [
            "code:internal/worktree/gate.go#BranchHead",
            "code:internal/worktree/gate.go#TargetHead",
            "code:internal/worktree/gate.go#IsAncestor",
            "code:internal/worktree/gate.go#AbortRebase",
            "code:internal/worktree/gate.go#Compare",
            "code:internal/worktree/gate.go#ChangedPaths",
            "code:internal/worktree/gate.go#TrackedChanges",
            "code:internal/worktree/gate.go#OwnerHistoryContains",
            "code:internal/subagents/gate.go#Supervisor.MergeAsOwner"
          ]
        },
        {
          "text": "Git, the hardened git runner that the changes package uses to inspect Workspace repositories.",
          "refs": [
            "code:internal/worktree/worktree.go#Git",
            "code:internal/changes/repository.go#Service.Repository"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Every git command runs with hooks, fsmonitor, pager, signing and submodule recursion off, transport disabled, and an environment built from scratch, so nothing the worker wrote or Evie's own environment can steer it.",
          "refs": [
            "code:internal/worktree/worktree.go#hardening",
            "code:internal/worktree/worktree.go#gitEnv",
            "code:internal/worktree/worktree.go#git"
          ]
        },
        {
          "text": "Commands against a worktree name its git directory and work tree explicitly, so the worktree's own .git file is never consulted.",
          "refs": [
            "code:internal/worktree/worktree.go#inWorktree"
          ]
        },
        {
          "text": "Worktree changes are serialized per repository by an in-process lock only.",
          "refs": [
            "code:internal/worktree/worktree.go#lockRepo"
          ]
        },
        {
          "text": "Plan refuses a repository under ~/.evie, a repository that tracks .evie, and .evie folders that are links.",
          "refs": [
            "code:internal/worktree/worktree.go#checkFolder"
          ]
        },
        {
          "text": "Because Git has protocol.allow=never, changes runs fetch, push and clone through its own remoteGit instead.",
          "refs": [
            "code:internal/worktree/worktree.go#hardening",
            "code:internal/changes/publish.go#Service.remoteGit"
          ]
        },
        {
          "text": "Report and Compare list at most 200 files and only short, plain paths; other files are counted but not named.",
          "refs": [
            "code:internal/worktree/worktree.go#Report",
            "code:internal/worktree/gate.go#Compare",
            "code:internal/worktree/worktree.go#plainPath",
            "code:internal/worktree/worktree.go#maxReportFiles"
          ]
        },
        {
          "text": "The rebase and merge run with autostash, autosquash, rerere, update-refs and signature verification off, and nothing in the gate stashes, resets, forces or pushes.",
          "refs": [
            "code:internal/worktree/gate.go#gateHardening",
            "code:internal/worktree/gate.go"
          ]
        }
      ],
      "terms": [
        "Workspace"
      ]
    },
    {
      "id": "pkg:internal/localfiles",
      "level": "component",
      "kind": "package",
      "title": "localfiles",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Reads current disk contents beneath one attached Workspace folder, for the owner's Files and Working tree views and for a Change's diff, and never stands in for recorded conversation evidence.",
        "refs": [
          "code:internal/localfiles/files.go",
          "code:internal/localfiles/files.go#Read",
          "code:internal/localfiles/compare.go#Compare"
        ]
      },
      "does": [
        {
          "text": "Lists one directory, marking it truncated past 2,000 entries, or filters the whole tree by path (up to 300 matches within 20,000 entries, skipping node_modules and .venv), and always skips .git, symbolic links and anything the file tools exclude.",
          "refs": [
            "code:internal/localfiles/files.go#List"
          ]
        },
        {
          "text": "Reads one regular UTF-8 text file of at most 512 KiB, refusing binaries, and checks the opened file again so a swapped-in replacement cannot slip past the size and type checks.",
          "refs": [
            "code:internal/localfiles/files.go#Read",
            "code:internal/localfiles/files.go#MaxFileBytes"
          ]
        },
        {
          "text": "Reviews Git changes read-only: the working tree with untracked files, the staged changes, or the branch against a base (base...HEAD), and one file's patch.",
          "refs": [
            "code:internal/localfiles/git.go#Review",
            "code:internal/localfiles/git.go#Diff",
            "code:internal/localfiles/git.go#comparison"
          ]
        },
        {
          "text": "Compares two revisions for a Change (base...head, renames found): each file's status, line counts, ending blob and hunks, reading patches for at most 100 files and withholding excluded files such as .env.",
          "refs": [
            "code:internal/localfiles/compare.go#Compare",
            "code:internal/localfiles/compare.go#maxComparedPatches",
            "code:internal/localfiles/compare.go#ComparedFile"
          ]
        }
      ],
      "owns": [
        {
          "text": "Nothing persistent: it reads the attached folder through an os.Root and git.",
          "refs": [
            "code:internal/localfiles/files.go#openRoot",
            "code:internal/localfiles/git.go#git"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "List, Read, Review, Diff and Compare with their result types; web's folder routes call the first four and changes' Show calls Compare.",
          "refs": [
            "code:internal/localfiles/files.go#List",
            "code:internal/localfiles/files.go#Read",
            "code:internal/localfiles/git.go#Review",
            "code:internal/localfiles/git.go#Diff",
            "code:internal/localfiles/compare.go#Compare",
            "code:internal/web/local_folder.go#Server.registerFolderRoutes",
            "code:internal/changes/show.go#Lister.Show"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Exclusions come from the file tools' own rules through tools.ValidateFileAccess, and containment comes from os.Root.",
          "refs": [
            "code:internal/localfiles/files.go#allowed",
            "code:internal/tools/file.go#ValidateFileAccess",
            "code:internal/localfiles/files.go#openRoot"
          ]
        },
        {
          "text": "Every git call disables hooks and fsmonitor, uses literal pathspecs without optional locks, and neutralizes each configured clean, smudge or process filter for that one invocation, because diff can run them.",
          "refs": [
            "code:internal/localfiles/git.go#git"
          ]
        },
        {
          "text": "Git output is capped at 2 MiB and each git command at 5 seconds; an oversized patch in a comparison is listed without its hunks.",
          "refs": [
            "code:internal/localfiles/git.go#cappedBuffer",
            "code:internal/localfiles/git.go#git",
            "code:internal/localfiles/compare.go#Compare"
          ]
        },
        {
          "text": "The folder must still be at its canonical path: one that has moved or become unavailable is refused, not followed.",
          "refs": [
            "code:internal/localfiles/files.go#openRoot"
          ]
        },
        {
          "text": "Paths are relative and slash-separated; a backslash or an invalid fs path is refused.",
          "refs": [
            "code:internal/localfiles/files.go#relative"
          ]
        }
      ],
      "terms": [
        "Workspace",
        "Change",
        "Episodic Memory"
      ]
    },
    {
      "id": "pkg:internal/localterminal",
      "level": "component",
      "kind": "package",
      "title": "localterminal",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Owns the owner's interactive shell in an attached Workspace folder and its PTY lifetime, independent of agent tools, approvals and conversation history.",
        "refs": [
          "code:internal/localterminal/session.go",
          "code:internal/localterminal/session.go#Session",
          "code:internal/localterminal/session.go#Start"
        ]
      },
      "does": [
        {
          "text": "Starts the owner's login shell ($SHELL, else /bin/sh) with -l -i in the canonical folder, on a PTY of 2 to 500 columns and 1 to 200 rows, with TERM=xterm-256color.",
          "refs": [
            "code:internal/localterminal/session.go#Start",
            "code:internal/localterminal/session.go#ValidSize"
          ]
        },
        {
          "text": "Writes input of at most 3,072 bytes per call under a one-second write deadline, and resizes through the PTY's descriptor.",
          "refs": [
            "code:internal/localterminal/session.go#Session.Write",
            "code:internal/localterminal/session.go#Session.Resize"
          ]
        },
        {
          "text": "Ends when its context ends or on Close: it sends SIGHUP and then SIGKILL to every process group in the shell's session, rechecking membership before each signal.",
          "refs": [
            "code:internal/localterminal/session.go#Start",
            "code:internal/localterminal/session.go#Session.Close",
            "code:internal/localterminal/session.go#Session.members",
            "code:internal/localterminal/session.go#Session.signalGroups"
          ]
        },
        {
          "text": "Reports the shell's exit code once it has exited.",
          "refs": [
            "code:internal/localterminal/session.go#Session.ExitCode",
            "code:internal/localterminal/session.go#Session.Exited"
          ]
        }
      ],
      "owns": [
        {
          "text": "One shell process and its PTY per Session; nothing survives a restart of evie serve.",
          "refs": [
            "code:internal/localterminal/session.go#Session",
            "doc:cmd/evie/docs/active/workspace-local-folder.spec.md#accepted-decisions"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Start and ValidSize, and Session's Read, Write, Resize, Close, Exited, Closed and ExitCode; web's terminal routes keep at most eight Sessions and stream their output as NDJSON.",
          "refs": [
            "code:internal/localterminal/session.go#Start",
            "code:internal/localterminal/session.go#Session",
            "code:internal/web/terminal.go#Server.openTerminal",
            "code:internal/web/terminal.go#Server.registerTerminalRoutes"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Terminal input is owner input, not a model tool: it needs no approval and is not recorded as conversation evidence.",
          "refs": [
            "code:internal/localterminal/session.go",
            "doc:cmd/evie/docs/active/workspace-local-folder.spec.md#accepted-decisions"
          ]
        },
        {
          "text": "The shell inherits evie serve's whole environment, which includes whatever ~/.evie/.env set at startup, such as GITHUB_TOKEN when it is kept there.",
          "refs": [
            "code:internal/localterminal/session.go#Start",
            "code:cmd/evie/main.go#main",
            "doc:cmd/evie/docs/active/changes.spec.md#github-connection"
          ]
        },
        {
          "text": "Processes deliberately detached into another session are outside the terminal's lifetime and survive Close.",
          "refs": [
            "code:internal/localterminal/session.go#Session.Close",
            "doc:cmd/evie/docs/active/workspace-local-folder.spec.md#accepted-decisions"
          ]
        },
        {
          "text": "The PTY descriptor is duplicated and made nonblocking so reads can be interrupted, and resizes go through SyscallConn because Fd() would switch it back to blocking.",
          "refs": [
            "code:internal/localterminal/session.go#Start",
            "code:internal/localterminal/session.go#Session.Resize"
          ]
        },
        {
          "text": "Changing the Workspace's folder stops that Workspace's terminals.",
          "refs": [
            "code:internal/web/local_folder.go#Server.registerFolderRoutes"
          ]
        }
      ],
      "terms": [
        "Workspace"
      ]
    },
    {
      "id": "pkg:internal/workspacetabs",
      "level": "component",
      "kind": "package",
      "title": "workspacetabs",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Reads a Workspace's own tabs from its folder: each .evie/tabs/<tab-id>/ holding tab.json and index.html, validated and read only through an os.Root.",
        "refs": [
          "code:internal/workspacetabs/tabs.go",
          "code:internal/workspacetabs/tabs.go#Tab",
          "code:internal/workspacetabs/tabs.go#List"
        ]
      },
      "does": [
        {
          "text": "Lists the tabs by order then id, marking a problem on any tab that is invalid or over the limits: 12 shown tabs, 5 MB and 200 files per tab, and 256 entries in .evie/tabs of which the first 32 folders by name are examined.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#List",
            "code:internal/workspacetabs/tabs.go#MaxTabs",
            "code:internal/workspacetabs/tabs.go#MaxTabBytes",
            "code:internal/workspacetabs/tabs.go#MaxTabFiles",
            "code:internal/workspacetabs/tabs.go#maxFolders",
            "code:internal/workspacetabs/tabs.go#readDir"
          ]
        },
        {
          "text": "Validates tab.json strictly: unknown fields are a problem, the title is 1 to 24 characters, a built-in view (spending) has no data and needs no index.html, and each data source names exactly one of db, files or runs.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#readManifest",
            "code:internal/workspacetabs/tabs.go#DataSource.problem",
            "code:internal/workspacetabs/tabs.go#Builtins",
            "code:internal/workspacetabs/tabs.go#inspect"
          ]
        },
        {
          "text": "Opens one file of a valid tab for serving, refusing absolute paths, .., dotfiles, links, non-regular files and anything under the reserved _evie path.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#Open",
            "code:internal/workspacetabs/tabs.go#openRegular",
            "code:internal/workspacetabs/tabs.go#allowedPath",
            "code:internal/workspacetabs/tabs.go#Reserved"
          ]
        },
        {
          "text": "Reads a Workspace-folder file for a tab's declared globs: a regular file with one link, reached without a symlink, at most 1 MB, and never a secret-looking name or a SQLite database by name or header.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#ReadFile",
            "code:internal/workspacetabs/tabs.go#secretName",
            "code:internal/workspacetabs/tabs.go#hardLinked",
            "code:internal/workspacetabs/tabs.go#sqliteHeader"
          ]
        },
        {
          "text": "Pins a data approval to the data list's hash within this folder, so a changed list or folder asks again.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#ApprovalKey",
            "code:internal/workspacetabs/tabs.go#dataKey"
          ]
        }
      ],
      "owns": [
        {
          "text": "No storage: it reads the folder, while eviedb keeps data approvals and each tab's latest problems.",
          "refs": [
            "code:internal/workspacetabs/tabs.go",
            "code:internal/eviedb/workspace_tabs.go#Store.ApproveWorkspaceTabData",
            "code:internal/eviedb/workspace_tabs.go#Store.RecordWorkspaceTabProblem"
          ]
        },
        {
          "text": "The tab contract's types: Tab, DataSource and Problem.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#Tab",
            "code:internal/workspacetabs/tabs.go#DataSource",
            "code:internal/workspacetabs/tabs.go#Problem"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "List, Open, ReadFile, ValidID and ApprovalKey with the errors ErrNotFound, ErrInvalid, ErrPath, ErrRefused and ErrTooMany; web's tab routes and the workspace_tabs tool call them.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#List",
            "code:internal/workspacetabs/tabs.go#Open",
            "code:internal/workspacetabs/tabs.go#ReadFile",
            "code:internal/workspacetabs/tabs.go#ValidID",
            "code:internal/workspacetabs/tabs.go#ErrRefused",
            "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabRoutes",
            "code:internal/plugins/workspace_tabs.go"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Open lists every tab first, so a thirteenth or broken tab is refused exactly as the header leaves it out.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#Open"
          ]
        },
        {
          "text": "Glob matching fills a pattern-by-path table, so its time is their product, with at most 16 pattern and 32 path segments.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#matchGlob",
            "code:internal/workspacetabs/tabs.go#validGlob"
          ]
        },
        {
          "text": "Files are opened non-blocking with O_NOFOLLOW and checked again after opening, so a pipe or link swapped in after the checks cannot hang the read or be followed.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#openRegular"
          ]
        },
        {
          "text": "The page sandbox and Content-Security-Policy are set by web when it serves the files, not by this package.",
          "refs": [
            "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabRoutes",
            "doc:cmd/evie/docs/active/workspace-tabs.spec.md#discovery-and-serving"
          ]
        },
        {
          "text": "A Workspace without a folder has no tabs.",
          "refs": [
            "code:internal/workspacetabs/tabs.go#List"
          ]
        }
      ],
      "terms": [
        "Workspace"
      ]
    }
  ],
  "edges": [],
  "flows": [
    {
      "id": "mcp-sign-in",
      "title": "MCP sign-in",
      "summary": {
        "text": "From adding a hosted MCP server to a chat calling one of its tools: Evie runs the OAuth sign-in itself, keeps the tokens in the Keychain, and a tool becomes callable once a Workspace enables its exact hash.",
        "refs": [
          "code:internal/mcpconn/manager.go#Manager.BeginConnect",
          "code:internal/mcpconn/manager.go#Manager.completeAttempt",
          "code:internal/mcpchat/service.go#Service.SetToolEnabled",
          "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#connections"
        ]
      },
      "trigger": {
        "text": "The owner adds a server in Workspace Settings, Connections (or runs evie connections add and then connect).",
        "refs": [
          "code:internal/web/ui/src/workspaces/WorkspaceConnections.tsx",
          "code:cmd/evie/connections.go#runConnectionManagement"
        ]
      },
      "participants": [
        "person:owner",
        "container:web-ui",
        "pkg:internal/web",
        "pkg:internal/mcpconn",
        "pkg:internal/eviedb",
        "container:keychain",
        "external:mcp-servers",
        "pkg:internal/mcpchat",
        "pkg:internal/plugins"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Add a server: name, https URL, optional pre-registered client ID and secret",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/workspaces/WorkspaceConnections.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/connections/add (owner-only management route)",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/connections.ts",
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "code:internal/web/serve.go#Server.managementRoute"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/mcpconn",
          "label": "Manager.Add validates the name and the https URL",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Add",
            "code:internal/mcpconn/connection.go#ValidateName",
            "code:internal/mcpconn/connection.go#ValidateURL"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "container:keychain",
          "label": "Save a pre-registered client's secret, only when one was given",
          "kind": "call",
          "via": "process",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Add",
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "pkg:internal/eviedb",
          "label": "CreateMCPConnection: mcp-<name>, status not_connected",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Add",
            "code:internal/eviedb/mcp_connections.go#Store.CreateMCPConnection"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Connect",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/workspaces/WorkspaceConnections.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/connections/connect",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/connections.ts",
            "code:internal/web/connections.go#Server.registerConnectionRoutes"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/mcpconn",
          "label": "BeginConnect",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "external:mcp-servers",
          "label": "Probe the MCP URL unauthenticated, read resource_metadata from the 401, then fetch protected-resource and authorization-server metadata",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/oauth.go#discover",
            "code:internal/mcpconn/oauth.go#probeResourceMetadata",
            "code:internal/mcpconn/oauth.go#authorizationServerMetadata"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "external:mcp-servers",
          "label": "Dynamic Client Registration as a public client, unless a pre-registered or still-valid earlier client applies",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:internal/mcpconn/oauth.go#register"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "pkg:internal/web",
          "label": "Attempt: state ID and authorization URL with the S256 challenge and resource; the attempt lives in memory for ten minutes",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:internal/mcpconn/oauth.go#authorizationURL",
            "code:internal/mcpconn/manager.go#attemptLifetime"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "Open the authorization URL in a new tab, then POST /api/connections/wait with the attempt ID",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/workspaces/WorkspaceConnections.tsx",
            "code:internal/web/ui/src/api/connections.ts",
            "code:internal/mcpconn/manager.go#Manager.Wait"
          ]
        },
        {
          "from": "person:owner",
          "to": "external:mcp-servers",
          "label": "Sign in and consent at the authorization server",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/oauth.go#authorizationURL",
            "code:internal/mcpconn/mcptest/authserver.go#AuthServer.Approve"
          ]
        },
        {
          "from": "external:mcp-servers",
          "to": "pkg:internal/web",
          "label": "Browser redirect: GET /api/connections/oauth/callback with code and state (unguarded; the single-use state is the check)",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "code:internal/mcpconn/oauth.go#authorizationURL"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/mcpconn",
          "label": "Callback accepts the state once, before it expires",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Callback"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "external:mcp-servers",
          "label": "Exchange the code with the PKCE verifier and resource, detached from the request with a 30-second limit",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.completeAttempt",
            "code:internal/mcpconn/oauth.go#exchangeCode",
            "code:internal/mcpconn/manager.go#detached"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "container:keychain",
          "label": "Save the access and refresh tokens, expiry and client over security(1)'s stdin",
          "kind": "call",
          "via": "process",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.completeAttempt",
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "pkg:internal/eviedb",
          "label": "SetMCPConnectionStatus: connected, with the OAuth metadata",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.completeAttempt",
            "code:internal/eviedb/mcp_connections.go#Store.SetMCPConnectionStatus"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "person:owner",
          "label": "Plain page in the sign-in tab: Connected to <name>. You can close this tab.",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/web/connections.go#Server.registerConnectionRoutes",
            "code:internal/web/connections.go#writeCallbackPage"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "The wait answers with the connected record",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/manager.go#Manager.Wait",
            "code:internal/web/connections.go#Server.registerConnectionRoutes"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Open the connection's tools in this Workspace and enable one",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/workspaces/WorkspaceConnections.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/connections/tools/set",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/connections.ts",
            "code:internal/web/connection_tools.go#Server.registerConnectionToolRoutes"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/mcpchat",
          "label": "SetToolEnabled",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolEnabled"
          ]
        },
        {
          "from": "pkg:internal/mcpchat",
          "to": "pkg:internal/mcpconn",
          "label": "ListTools now, nothing cached: each tool with its hash",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpchat/service.go#Service.reviewInputs",
            "code:internal/mcpconn/pool.go#Pool.ListTools",
            "code:internal/mcpconn/pool.go#toolFromSDK"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "external:mcp-servers",
          "label": "Open the shared session if needed, then tools/list with the bearer token on Evie's transport",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.session",
            "code:internal/mcpconn/pool.go#Pool.connect",
            "code:internal/mcpconn/transport.go#transport.RoundTrip"
          ]
        },
        {
          "from": "pkg:internal/mcpchat",
          "to": "pkg:internal/eviedb",
          "label": "SetWorkspaceAccess with <name>@<sha256> under mcp-<name>, expecting the revision it read",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpchat/service.go#Service.SetToolEnabled",
            "code:internal/eviedb/workflows.go#Store.SetWorkspaceAccess"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/eviedb",
          "label": "Later the owner starts a new chat in this Workspace; creating it pins the current Workspace Access revision, so tools enabled afterwards reach only newer chats",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:internal/eviedb/sessions.go#Store.createWorkspaceSession"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/mcpchat",
          "label": "The chat's mcp_call: Reach (pinned and current access), Connection and a fresh ListTools resolve the exact reviewed version",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/plugins/mcp.go#MCP.resolve",
            "code:internal/plugins/mcp.go#MCPChat",
            "code:internal/mcpchat/service.go#Service.Reach"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/mcpchat",
          "label": "Allowed: no approval card only when this exact hash is set to Allow; any error asks",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/plugins/mcp.go#MCP.requiresApproval",
            "code:internal/mcpchat/service.go#Service.Allowed",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/mcpchat",
          "label": "After approval, resolve again, then CallTool with a 60-second timeout",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/plugins/mcp.go#MCP.call",
            "code:internal/plugins/mcp.go#mcpCallTimeout",
            "code:internal/mcpchat/service.go#Service.CallTool"
          ]
        },
        {
          "from": "pkg:internal/mcpchat",
          "to": "pkg:internal/mcpconn",
          "label": "Pool.CallTool",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/mcpchat/service.go#Service.CallTool",
            "code:internal/mcpconn/pool.go#Pool.CallTool"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "external:mcp-servers",
          "label": "tools/call; a 401 refreshes the token and re-sends once, nothing else is re-sent",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.CallTool",
            "code:internal/mcpconn/transport.go#transport.RoundTrip",
            "code:internal/mcpconn/manager.go#Manager.token"
          ]
        },
        {
          "from": "pkg:internal/mcpconn",
          "to": "pkg:internal/mcpchat",
          "label": "CallResult, or a CallError whose outcome is not_sent, rejected or unknown",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.CallTool",
            "code:internal/mcpconn/pool.go#CallError",
            "code:internal/mcpconn/transport.go#SendTrace.Outcome"
          ]
        },
        {
          "from": "pkg:internal/mcpchat",
          "to": "pkg:internal/plugins",
          "label": "mcp_call frames the result as untrusted server data, capped at 100,000 bytes",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/plugins/mcp.go#MCP.call",
            "code:internal/plugins/mcp.go#mcpResultLimit"
          ]
        }
      ]
    },
    {
      "id": "change-merge",
      "title": "Change merge",
      "summary": {
        "text": "A Change merges only by the owner's action: in a local Repository through the Kernel's merge gate as a fast-forward, on GitHub by Rebase and merge of the head Evie last showed, and either way Evie then removes its own branch and worktree and brings the owner's checkout up to date when it safely can.",
        "refs": [
          "code:internal/changes/merge.go#Merger.Merge",
          "code:internal/changes/merge.go#Merger.mergeOnGitHub",
          "code:internal/changes/merge.go#Merger.cleanUp",
          "code:internal/changes/merge.go#Merger.catchUp",
          "doc:cmd/evie/docs/active/changes.spec.md#merge-close-and-cleanup"
        ]
      },
      "trigger": {
        "text": "The owner presses Approve and merge on a Change's review page in the Changes tab (or runs evie changes merge).",
        "refs": [
          "code:internal/web/ui/src/workspaces/WorkspaceChanges.tsx",
          "code:internal/web/ui/src/api/changes.ts",
          "code:cmd/evie/changes.go#runChangeManagement"
        ]
      },
      "participants": [
        "person:owner",
        "container:web-ui",
        "pkg:internal/web",
        "pkg:internal/changes",
        "pkg:internal/eviedb",
        "pkg:internal/subagents",
        "pkg:internal/worktree",
        "pkg:internal/github",
        "external:github"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Approve and merge",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/workspaces/WorkspaceChanges.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/changes/merge with the Workspace and Change IDs",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/changes.ts",
            "code:internal/web/changes_merge.go#Server.registerChangeMergeRoutes"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/eviedb",
          "label": "WorkspaceFolder: the Workspace's current folder",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/web/changes_merge.go#Server.registerChangeMergeRoutes",
            "code:internal/web/local_folder.go#WorkspaceFolderController",
            "code:internal/eviedb/workspace_folder.go#Store.WorkspaceFolder"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/changes",
          "label": "Merger.Merge, which first claims the folder so one action at a time changes it",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.Merge",
            "code:internal/web/changes_merge.go#ChangeMerger",
            "code:internal/changes/publish.go#Service.claim"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/worktree",
          "label": "Read the Repository from the folder with hardened git: top level, origin, base branch",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/repository.go#Service.Repository",
            "code:internal/worktree/worktree.go#Git"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/github",
          "label": "On GitHub, check the token: read the user and the repository, whose default branch is the base",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/repository.go#Service.check",
            "code:internal/github/client.go#Client.User",
            "code:internal/github/client.go#Client.Repository",
            "code:cmd/evie/main.go#repositoryReader"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/eviedb",
          "label": "List: Evie's build attempts and the synced pull requests, linked by branch, to find the unfinished Change",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/change.go#Lister.List",
            "code:internal/changes/merge.go#Merger.find",
            "code:internal/eviedb/build_changes.go#Store.WorkspaceBuilds",
            "code:internal/eviedb/change_pull_requests.go#Store.ChangePullRequests"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/subagents",
          "label": "Local Repository: the merge gate's MergeAsOwner for the Change's build",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/changes/merge.go#Merger.Merge",
            "code:internal/changes/merge.go#BuildGate",
            "code:internal/subagents/gate.go#Supervisor.MergeAsOwner"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/worktree",
          "label": "Unless the gate's pass still matches the branch and target heads: verify, rebase and verify again first",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.MergeAsOwner",
            "code:internal/subagents/gate.go#passedProposal",
            "code:internal/worktree/gate.go#Rebase"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/worktree",
          "label": "FastForward the target: git merge --ff-only where it is checked out, else a compare-and-swap of the ref",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.merge",
            "code:internal/worktree/gate.go#FastForward"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "Record the merge on the build gate and mark other branches on that target as needing a rebase",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.completeMerge"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/worktree",
          "label": "Remove the build's worktree and branch",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.completeMerge",
            "code:internal/worktree/worktree.go#Remove"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/changes",
          "label": "The merge proposal: merged, or the gate's refusal with its runs and notes",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.MergeAsOwner",
            "code:internal/changes/merge.go#Merger.Merge"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/github",
          "label": "GitHub Repository: read the pull request's state, draft marking, head and mergeability",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.mergeOnGitHub",
            "code:internal/github/pulls.go#Client.PullRequest"
          ]
        },
        {
          "from": "pkg:internal/github",
          "to": "external:github",
          "label": "GET /repos/{owner}/{repo}/pulls/{number}",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/github/pulls.go#Client.PullRequest",
            "code:internal/github/client.go#Client.do"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/github",
          "label": "MergePullRequest with merge_method rebase and the head SHA Evie last showed",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.mergeOnGitHub",
            "code:internal/github/pulls.go#Client.MergePullRequest"
          ]
        },
        {
          "from": "pkg:internal/github",
          "to": "external:github",
          "label": "PUT /repos/{owner}/{repo}/pulls/{number}/merge; 405 means not mergeable, 409 means the head changed",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/github/pulls.go#Client.MergePullRequest",
            "code:internal/github/client.go#Client.do"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/eviedb",
          "label": "On a conflict, record that this head needs a rebase, and stop",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.needsRebase",
            "code:internal/eviedb/change_pull_requests.go#Store.RecordChangeMergeable"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "external:github",
          "label": "Clean up: delete Evie's evie/ branch on GitHub with git push --delete, the token in a per-command header",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/changes/merge.go#Merger.cleanUp",
            "code:internal/changes/publish.go#Service.remoteGit",
            "code:internal/changes/publish.go#Service.remoteEnv"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/subagents",
          "label": "Clean up: CloseAsOwner removes Evie's worktree and local branch",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/changes/merge.go#Merger.cleanUp",
            "code:internal/subagents/gate.go#Supervisor.CloseAsOwner"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "external:github",
          "label": "Catch up: fetch the base branch from origin",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/changes/merge.go#Merger.catchUp",
            "code:internal/changes/publish.go#Service.remoteGit"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/worktree",
          "label": "Catch up: fast-forward the owner's checkout only when it is on the base branch with no uncommitted edits",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.catchUp",
            "code:internal/worktree/worktree.go#Git"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/github",
          "label": "Sync at once: list open pull requests and read the merged one, so the Change shows merged",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.sync",
            "code:internal/changes/sync.go#Syncer.Sync",
            "code:internal/changes/sync.go#Syncer.sync"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/eviedb",
          "label": "SaveChangePullRequests",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/changes/sync.go#Syncer.sync",
            "code:internal/eviedb/change_pull_requests.go#Store.SaveChangePullRequests"
          ]
        },
        {
          "from": "pkg:internal/changes",
          "to": "pkg:internal/web",
          "label": "MergeResult: the Change as the list now shows it, the status and the notes",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/changes/merge.go#Merger.after",
            "code:internal/changes/merge.go#MergeResult"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "200 with the result, or 409 not_merged with the gate's runs when nothing merged",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/web/changes_merge.go#Server.registerChangeMergeRoutes"
          ]
        }
      ]
    }
  ]
}
);
