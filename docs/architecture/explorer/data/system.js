evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "system",
  "nodes": [
    {
      "id": "person:owner",
      "level": "context",
      "kind": "person",
      "title": "Owner",
      "summary": {
        "text": "The one person Evie works for: Evie has no user accounts, and its web server binds only to loopback because it has no authentication.",
        "refs": [
          "code:internal/web/serve.go#listenAddr",
          "doc:GLOSSARY.md#evie"
        ]
      },
      "does": [
        {
          "text": "Chats with Evie in the web UI, where the browser posts each message and reads the turn's events back as a stream.",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "code:internal/web/ui/src/api/stream.ts"
          ]
        },
        {
          "text": "Chats in the terminal REPL that `evie` with no arguments starts, answering each gated tool call at an `approve? [y/N]` prompt.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/repl.go#runREPLContextIOWithMemory"
          ]
        },
        {
          "text": "Runs one-shot commands: `evie` management subcommands, `todo` for Task Trees, `finance` for banks and transactions, and `ytscribe` for YouTube transcripts.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/todo/main.go#run",
            "code:cmd/finance/main.go#main",
            "code:cmd/ytscribe/main.go#runYTScribe"
          ]
        },
        {
          "text": "Grants Action Approval for consequential tool calls; the web route for it accepts only JSON requests whose Origin and Host are loopback, so a foreign web page cannot forge an approval.",
          "refs": [
            "code:internal/web/serve.go#Server.guard",
            "code:internal/web/serve.go#Server.Handler",
            "term:Action Approval"
          ]
        },
        {
          "text": "Signs in to outside accounts in the browser: MCP servers through an OAuth flow that Evie starts, and banks through Plaid's hosted Link page.",
          "refs": [
            "code:cmd/evie/connections.go#defaultConnectionCommands",
            "code:internal/finance/link.go#Link"
          ]
        },
        {
          "text": "Reviews what background work proposes, such as the background memory pass's memory proposals, before anything is accepted.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
          ]
        }
      ],
      "owns": [
        {
          "text": "Supplies the credentials and switches Evie reads at startup, from the shell environment or `~/.evie/.env`, such as OPENROUTER_API_KEY and GITHUB_TOKEN.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#repositoryReader"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "There is no login: anything that can reach the loopback port acts as the owner, which is why a non-loopback `EVIE_ADDR` is refused outright rather than warned about.",
          "refs": [
            "code:internal/web/serve.go#listenAddr"
          ]
        }
      ],
      "terms": [
        "Action Approval",
        "Task Tree",
        "Account Connection"
      ]
    },
    {
      "id": "system:evie",
      "level": "context",
      "kind": "system",
      "title": "Evie",
      "summary": {
        "text": "A Go-native personal agent runtime with local SQLite persistence, purpose-built tools, an OpenRouter conversational client, and a web interface.",
        "refs": [
          "doc:AGENTS.md#project"
        ]
      },
      "does": [
        {
          "text": "Runs a model-and-tools loop for each chat, calling OpenRouter for every model turn and running the tools its session's Agent Preset composes.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/agent/agent.go#NewWithToolset",
            "term:Agent Preset"
          ]
        },
        {
          "text": "Composes ten compiled First-party Plugins (Web, Finance, YouTube, Todo, Memory, Subagents, Workflows, GitHub, MCP and Workspace tabs) through the Plugin Manager; each defaults to enabled and the owner's choice is durable.",
          "refs": [
            "code:cmd/evie/main.go#defaultPluginEnabled",
            "code:internal/plugins/manager.go#NewManager",
            "code:internal/plugins/manager.go#Manager.ConfigureEnabledState"
          ]
        },
        {
          "text": "Keeps durable state on the owner's machine in three SQLite databases (Evie, finance and the YouTube library) and a bare procedural Git repository.",
          "refs": [
            "code:internal/eviedb/db.go#OpenDBContext",
            "code:internal/finance/db.go#OpenDBContext",
            "code:internal/youtube/db.go#databasePath",
            "code:internal/procedural/repo.go#DefaultRoot"
          ]
        },
        {
          "text": "Runs background hosts inside its long-lived processes: the workflow scheduler, the background memory pass, retrieval index maintenance, the optional memory compiler, subagent recovery and, in `evie serve` only, GitHub change sync.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#startChangeSync"
          ]
        },
        {
          "text": "Reaches outside systems only through purpose-built clients: OpenRouter, a local Ollama, hosted MCP servers, GitHub, Plaid, YouTube, Brave Search, web pages, the local Codex app-server, launchd and the owner's login shell.",
          "refs": [
            "code:internal/openrouter/client.go#NewClient",
            "code:internal/localembedding/client.go#New",
            "code:internal/mcpconn/pool.go#Pool.connect",
            "code:internal/github/client.go#New",
            "code:internal/finance/client.go#plaidClient",
            "code:internal/youtube/client.go#NewClient",
            "code:internal/tools/websearch.go#webSearch",
            "code:internal/tools/webfetch.go#webFetch",
            "code:internal/usage/codex.go#ReadCodexAccount",
            "code:internal/tools/cron.go#installJob",
            "code:internal/tools/bash.go#runBash"
          ]
        }
      ],
      "owns": [
        {
          "text": "Conversations, memory, Tasks, Workspaces, Workflow Runs, plugin state and MCP connection records in the Evie database at `~/.evie/evie.db`.",
          "refs": [
            "code:internal/eviedb/db.go#OpenDBContext",
            "code:internal/eviedb/db.go#schema"
          ]
        },
        {
          "text": "Linked banks and transactions in `~/.finance/finance.db`, and the transcript library in `~/.evie/transcripts/transcripts.db`.",
          "refs": [
            "code:internal/finance/db.go#OpenDBContext",
            "code:internal/youtube/db.go#databasePath"
          ]
        },
        {
          "text": "Reviewed Workflow Definitions in the procedural Git repository at `~/.evie/procedural`, and MCP credentials in the macOS Keychain.",
          "refs": [
            "code:internal/procedural/repo.go#DefaultRoot",
            "code:internal/mcpconn/keychain.go#Keychain"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "A loopback web UI and API served by `evie serve` at `EVIE_ADDR` (default `127.0.0.1:6687`).",
          "refs": [
            "code:internal/web/serve.go#listenAddr",
            "code:internal/web/lifecycle.go#ServeWithContext"
          ]
        },
        {
          "text": "Four command-line binaries: `evie` (REPL, `serve`, `cron-exec` and management subcommands), `todo`, `finance` and `ytscribe`.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/todo/main.go#main",
            "code:cmd/finance/main.go#main",
            "code:cmd/ytscribe/main.go#main"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Third-party text (web pages, search snippets, YouTube transcripts and MCP results) reaches the model only framed as untrusted data, never as instructions.",
          "refs": [
            "code:internal/tools/webfetch.go#frameUntrustedWeb",
            "code:internal/tools/youtube.go#youtubeFrameBegin",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries",
            "pkg:internal/untrusted"
          ]
        },
        {
          "text": "Several pieces assume macOS: launchd fires cron jobs, `/usr/bin/security` reaches the Keychain, the folder picker runs `osascript`, and the build-worker sandbox is Seatbelt (bubblewrap on Linux).",
          "refs": [
            "code:internal/tools/cron.go#installJob",
            "code:internal/mcpconn/keychain.go#RunSecurity",
            "code:internal/web/native_folder_picker.go#chooseNativeWorkspaceFolder",
            "code:internal/tools/sandbox.go#WorkerSandbox"
          ]
        }
      ],
      "terms": [
        "Kernel",
        "First-party Plugin",
        "Plugin Manager",
        "Agent Preset",
        "Workspace"
      ],
      "links": [
        "doc:AGENTS.md#architecture-map",
        "doc:GLOSSARY.md#evie"
      ],
      "contains": [
        "container:serve",
        "container:repl",
        "container:management-cli",
        "container:cron-exec",
        "container:web-ui",
        "container:todo-cli",
        "container:finance-cli",
        "container:ytscribe-cli",
        "container:workflow-scheduler",
        "container:memory-background-pass",
        "container:compiler-host",
        "container:retrieval-maintenance",
        "container:subagent-recovery",
        "container:change-sync",
        "container:evie-db",
        "container:finance-db",
        "container:youtube-db",
        "container:procedural-repo",
        "container:keychain",
        "container:env-file",
        "container:workspace-folders"
      ]
    },
    {
      "id": "external:openrouter",
      "level": "context",
      "kind": "external-system",
      "title": "OpenRouter",
      "summary": {
        "text": "The hosted model router Evie calls for every remote model: chat turns, compaction, subagent workers, the background memory pass, and workflow AI and agent steps.",
        "refs": [
          "pkg:internal/openrouter",
          "code:internal/openrouter/client.go#NewClient"
        ]
      },
      "does": [
        {
          "text": "Receives chat turns as streamed Chat Completions at `/api/v1/chat/completions`, or at `/api/v1/responses` for the one model Evie sends through the Responses API, GPT-6 Astra.",
          "refs": [
            "code:internal/openrouter/client.go#NewClient",
            "code:internal/openrouter/client.go#Client.ChatStream",
            "code:internal/openrouter/responses_request.go#UsesResponses"
          ]
        },
        {
          "text": "Answers model metadata: the `/models` catalog for chat selection and prices, and per-model and per-endpoint limits that size each chat's context window.",
          "refs": [
            "code:internal/openrouter/models.go#Client.ListChatModels",
            "code:internal/openrouter/models.go#Client.ModelPricing",
            "code:internal/openrouter/context_profile.go#Client.discoverContextProfile"
          ]
        },
        {
          "text": "Is called from serve, the REPL, the background memory pass, the workflow scheduler and a foreground `evie workflows run`, always with OPENROUTER_API_KEY as a bearer token.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:cmd/evie/workflow_activation.go#workflowModels",
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Its error bodies are treated as untrusted: Evie drains a bounded prefix and keeps only the status, a Retry-After hint and whether the body reported a context-length rejection.",
          "refs": [
            "code:internal/openrouter/client.go#httpStatusFailure"
          ]
        },
        {
          "text": "The default chat model is `deepseek/deepseek-v4.1-flash` (EVIE_MODEL overrides it); when metadata discovery fails, only it and `moonshotai/kimi-k3` fall back to a checked-in window, and every other model fails to start.",
          "refs": [
            "code:internal/agent/agent.go#DefaultModel",
            "code:internal/openrouter/context_profile.go#builtinContextFallback",
            "code:cmd/evie/main.go#contextFallbackWarning"
          ]
        },
        {
          "text": "Background work sends data here only on opt-in: the background memory pass needs EVIE_REMOTE_MEMORY=on, and workflow AI and agent steps need EVIE_WORKFLOW_REMOTE_MODELS=on.",
          "refs": [
            "code:internal/memorybackground/config.go#FromEnv",
            "code:cmd/evie/workflow_activation.go#workflowRemoteModels"
          ]
        }
      ],
      "terms": [
        "AI Node",
        "Working Memory"
      ]
    },
    {
      "id": "external:ollama",
      "level": "context",
      "kind": "external-system",
      "title": "Ollama",
      "summary": {
        "text": "A local Ollama server on a loopback address, which Evie uses for memory embeddings and for the pinned local extractor, so memory text sent to it stays on the machine.",
        "refs": [
          "code:internal/localembedding/client.go#Client",
          "code:internal/localextractor/ollama.go#Ollama"
        ]
      },
      "does": [
        {
          "text": "Embeds memory text with the pinned `all-minilm:22m` model at `/api/embed`, after checking the model's manifest digest at `/api/tags`, when EVIE_MEMORY_EMBEDDING_ENDPOINT is set and EVIE_REMOTE_MEMORY=on.",
          "refs": [
            "code:internal/localembedding/client.go#Client.Embed",
            "code:internal/localembedding/client.go#Client.verifyManifest",
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint"
          ]
        },
        {
          "text": "Runs the pinned extraction model for the memory compiler at `/api/generate` and for the local background memory pass at `/api/chat`, after verifying the runtime version, model digest, template and tokenizer through `/api/version`, `/api/tags` and `/api/show`.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract",
            "code:internal/localextractor/ollama.go#Ollama.Chat",
            "code:internal/localextractor/activation.go#Ollama.VerifyCompilerConfiguration"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The two clients accept different endpoints: embeddings take loopback HTTP or a `unix:/absolute/socket` path, while the extractor takes only loopback HTTP with an explicit port.",
          "refs": [
            "code:internal/localembedding/client.go#New",
            "code:internal/localextractor/ollama.go#New"
          ]
        },
        {
          "text": "Neither client uses a proxy or follows redirects, so a misconfigured endpoint cannot carry memory text elsewhere.",
          "refs": [
            "code:internal/localembedding/client.go#New",
            "code:internal/localextractor/ollama.go#New"
          ]
        },
        {
          "text": "Evie never downloads models: the owner installs the pinned model, and a missing model or a changed digest fails the request instead.",
          "refs": [
            "code:internal/localembedding/client.go#Client.verifyManifest",
            "code:internal/localextractor/activation.go#Ollama.VerifyCompilerConfiguration",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in"
          ]
        }
      ],
      "terms": [
        "Compiler Generation",
        "Automatic Recall"
      ]
    },
    {
      "id": "external:mcp-servers",
      "level": "context",
      "kind": "external-system",
      "title": "Hosted MCP servers",
      "summary": {
        "text": "Hosted MCP servers at https URLs, reached over Streamable HTTP, which the owner connects through an OAuth sign-in that serve runs; their reviewed tools reach web chats.",
        "refs": [
          "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries",
          "pkg:internal/mcpconn",
          "pkg:internal/mcpchat"
        ]
      },
      "does": [
        {
          "text": "Sign-in: serve discovers the server's protected-resource and authorization-server metadata, registers a client dynamically when needed, and runs PKCE S256 with the browser returning to serve's own callback route.",
          "refs": [
            "code:internal/mcpconn/oauth.go#discover",
            "code:internal/mcpconn/oauth.go#register",
            "code:internal/mcpconn/oauth.go#authorizationURL",
            "code:internal/mcpconn/manager.go#Manager.BeginConnect",
            "code:cmd/evie/connections.go#newMCPConnectionManager"
          ]
        },
        {
          "text": "Tool use: serve keeps at most one live MCP session per connection, opened on first use with no client capabilities, and lists and calls tools on it.",
          "refs": [
            "code:internal/mcpconn/pool.go#Pool.connect",
            "code:internal/mcpconn/pool.go#Pool.ListTools",
            "code:internal/mcpconn/pool.go#Pool.CallTool"
          ]
        },
        {
          "text": "Tokens: serve's transport adds the bearer token only over https to the connection's own host, and re-sends a request once only after a 401 that a token refresh answered.",
          "refs": [
            "code:internal/mcpconn/transport.go"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Server text is untrusted: tool descriptions, schemas and results reach the model only as framed tool-result data, never as tool definitions or instructions.",
          "refs": [
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries"
          ]
        },
        {
          "text": "Evie does not follow an MCP redirect, because following one would re-send a POST.",
          "refs": [
            "code:internal/mcpconn/transport.go#ErrRedirect"
          ]
        },
        {
          "text": "MCP tools run only in serve's chats, and Workflow Definitions cannot reference them.",
          "refs": [
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries"
          ]
        }
      ],
      "terms": [
        "Account Connection",
        "Connection ID"
      ]
    },
    {
      "id": "external:github",
      "level": "context",
      "kind": "external-system",
      "title": "GitHub",
      "summary": {
        "text": "GitHub, reached through its REST API with a fine-grained token from GITHUB_TOKEN, and through git over HTTPS for pushes, fetches and clones.",
        "refs": [
          "code:internal/github/client.go#Client",
          "code:internal/changes/publish.go#Service.remoteGit"
        ]
      },
      "does": [
        {
          "text": "Answers REST calls for the token's user, repositories (read and create), pull requests (list, read, open, merge, close), Actions workflow runs and combined commit statuses.",
          "refs": [
            "code:internal/github/client.go#Client.User",
            "code:internal/github/client.go#Client.CreateRepository",
            "code:internal/github/pulls.go#Client.CreatePullRequest",
            "code:internal/github/pulls.go#Client.MergePullRequest",
            "code:internal/github/pulls.go#Client.WorkflowRuns",
            "code:internal/github/pulls.go#Client.CombinedStatus"
          ]
        },
        {
          "text": "Receives git push, fetch and clone from the Workspace folder, with the token sent per command as an extra HTTP header for `https://github.com/`.",
          "refs": [
            "code:internal/changes/publish.go#Service.remoteGit",
            "code:internal/changes/publish.go#Service.remoteEnv"
          ]
        },
        {
          "text": "Is synced by serve every 60 seconds, and written to by the owner's pushes and merges (web UI or `evie changes`) and by workflows' GitHub steps.",
          "refs": [
            "code:cmd/evie/main.go#startChangeSync",
            "code:cmd/evie/changes_push.go#changePusher",
            "code:cmd/evie/changes_push.go#changeMerger",
            "code:cmd/evie/changes_push.go#workflowGitHub"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Without GITHUB_TOKEN, GitHub is simply not connected: Repositories stay local and the change sync has nothing to do.",
          "refs": [
            "code:cmd/evie/main.go#repositoryReader",
            "code:cmd/evie/main.go#startChangeSync"
          ]
        },
        {
          "text": "The token never goes into a URL, git config, remote, log, returned error or worker sandbox, and git's output is scrubbed of it.",
          "refs": [
            "code:internal/github/client.go#Client",
            "code:internal/changes/publish.go#Service.scrub",
            "doc:cmd/evie/docs/active/changes.spec.md#github-connection"
          ]
        },
        {
          "text": "GitHub's Checks API does not accept fine-grained tokens, so a Repository's checks are its Actions workflow runs and commit statuses, and services that report only check runs are not seen.",
          "refs": [
            "doc:cmd/evie/docs/active/changes.spec.md#github-connection",
            "code:internal/github/pulls.go#Client.WorkflowRuns"
          ]
        },
        {
          "text": "Each REST request times out after ten seconds; a remote git command after five minutes.",
          "refs": [
            "code:internal/github/client.go#New",
            "code:internal/changes/publish.go#Service.remoteGit"
          ]
        }
      ],
      "terms": [
        "Repository",
        "Change",
        "Account Connection",
        "Connection ID"
      ]
    },
    {
      "id": "external:plaid",
      "level": "context",
      "kind": "external-system",
      "title": "Plaid",
      "summary": {
        "text": "Plaid's production environment, through which the finance domain links banks and syncs their transactions.",
        "refs": [
          "pkg:internal/finance",
          "code:internal/finance/client.go#plaidClient"
        ]
      },
      "does": [
        {
          "text": "Hosts bank linking: Evie creates a hosted Link session whose URL must be on `https://secure.plaid.com`, the owner finishes consent in the browser, and Evie completes the link and saves the new Item.",
          "refs": [
            "code:internal/finance/spending_account_link.go#validHostedLinkURL",
            "code:internal/finance/spending_account_link.go#SpendingService.StartAccountLink",
            "code:internal/finance/spending_account_link.go#SpendingService.CompleteAccountLink"
          ]
        },
        {
          "text": "Returns added, modified and removed transactions for each linked Item from its stored sync cursor.",
          "refs": [
            "code:internal/finance/sync.go#Sync",
            "code:internal/finance/db.go#schema"
          ]
        },
        {
          "text": "Is called by the `finance` CLI, by serve's Spending page and finance tools, and by the finance workflow's ledger-sync effect.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/spending.go#NewSpendingService",
            "code:internal/tools/finance.go#financeSync",
            "code:internal/plugins/finance_effects.go#Finance.performSyncLedger"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The environment is hard-coded to production; there is no sandbox mode.",
          "refs": [
            "code:internal/finance/client.go#plaidClient"
          ]
        },
        {
          "text": "Plaid access tokens are stored in the finance database's `items` table, not in a secret store.",
          "refs": [
            "code:internal/finance/db.go#schema"
          ]
        },
        {
          "text": "Plaid's machine-readable error codes drive recovery, such as restarting a sync after the data changed during pagination.",
          "refs": [
            "code:internal/finance/client.go#plaidErrorCode"
          ]
        }
      ]
    },
    {
      "id": "external:youtube",
      "level": "context",
      "kind": "external-system",
      "title": "YouTube",
      "summary": {
        "text": "YouTube's website and internal player and browse endpoints, from which the transcript library fetches captions and channel video lists.",
        "refs": [
          "code:internal/youtube/client.go#Client"
        ]
      },
      "does": [
        {
          "text": "Serves watch pages, the `youtubei/v1/player` endpoint that Evie calls as the Android app client to find caption tracks, and the caption files themselves.",
          "refs": [
            "code:internal/youtube/client.go#Client.fetchVideo",
            "code:internal/youtube/client.go#youtubeOrigin"
          ]
        },
        {
          "text": "Serves channel video lists through channel pages and the `youtubei/v1/browse` endpoint for `ytscribe scrape`.",
          "refs": [
            "code:internal/youtube/client.go#Client.listChannel",
            "code:cmd/ytscribe/main.go#printUsage"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Fetching depends on YouTube's internal API and an Android client identity pinned to one app version, so a YouTube change can break it with no change in Evie.",
          "refs": [
            "code:internal/youtube/client.go#androidClientVersion",
            "code:internal/youtube/client.go#Client.fetchVideo"
          ]
        },
        {
          "text": "A redirect to YouTube's consent page stops the fetch, and caption URLs must be https on a youtube.com host.",
          "refs": [
            "code:internal/youtube/client.go#NewClient",
            "code:internal/youtube/client.go#isConsentHost",
            "code:internal/youtube/client.go#validateCaptionRequestURL"
          ]
        },
        {
          "text": "Terminal outcomes such as a video without captions are cached and reused until a fetch asks for a refresh.",
          "refs": [
            "code:internal/youtube/client.go#TerminalError",
            "code:internal/tools/youtube.go#youtubeTranscriptTool"
          ]
        }
      ]
    },
    {
      "id": "external:brave-search",
      "level": "context",
      "kind": "external-system",
      "title": "Brave Search",
      "summary": {
        "text": "Brave's web search API, which the Web plugin's `web_search` tool queries with BRAVE_API_KEY.",
        "refs": [
          "code:internal/tools/websearch.go#webSearch"
        ]
      },
      "does": [
        {
          "text": "Receives one GET per search at `api.search.brave.com/res/v1/web/search` with the query and a result count from 1 to 20, authenticated by the X-Subscription-Token header.",
          "refs": [
            "code:internal/tools/websearch.go#webSearch",
            "code:internal/tools/websearch.go#braveSearchURL"
          ]
        },
        {
          "text": "Returns titles, URLs and snippets, which Evie frames as untrusted third-party text before the model sees them.",
          "refs": [
            "code:internal/tools/websearch.go#formatResults",
            "code:internal/tools/webfetch.go#frameUntrustedWeb"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "A search times out after 15 seconds, and a response over 1 MB is refused rather than parsed.",
          "refs": [
            "code:internal/tools/websearch.go#webSearch",
            "code:internal/tools/websearch.go#maxSearchBody"
          ]
        },
        {
          "text": "The tool's description tells the model the free tier allows one request per second, and a 429 comes back as an instruction to wait and retry.",
          "refs": [
            "code:internal/tools/websearch.go#webSearchTool",
            "code:internal/tools/websearch.go#webSearch"
          ]
        }
      ]
    },
    {
      "id": "external:web",
      "level": "context",
      "kind": "external-system",
      "title": "Web pages",
      "summary": {
        "text": "Any http or https page the Web plugin's `web_fetch` tool reads for the model, including local and private-network addresses for the main chat.",
        "refs": [
          "code:internal/tools/webfetch.go#webFetch",
          "code:internal/tools/webfetch.go#webFetchTool"
        ]
      },
      "does": [
        {
          "text": "Is fetched one URL per call, with http upgraded to https except for local and private hosts, credentials in the URL stripped, and non-text content or pages over 10 MB refused.",
          "refs": [
            "code:internal/tools/webfetch.go#normalizeURL",
            "code:internal/tools/webfetch.go#webFetchTool",
            "code:internal/tools/webfetch.go#maxFetchBytes"
          ]
        },
        {
          "text": "Is converted from HTML to Markdown and framed as untrusted data; text past 100 KB is trimmed and saved to a file.",
          "refs": [
            "code:internal/tools/webfetch.go#htmlToMarkdown",
            "code:internal/tools/webfetch.go#frameUntrustedWeb",
            "code:internal/tools/webfetch.go#maxFetchOutput"
          ]
        },
        {
          "text": "Redirects on the same host are followed; a redirect to another host is returned to the model as a new URL to fetch again.",
          "refs": [
            "code:internal/tools/webfetch.go#reportCrossHostRedirect",
            "code:internal/tools/webfetch.go#webFetchTool"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The main chat may fetch localhost and private-network addresses on purpose, but delegated research workers may dial only public addresses, judged after DNS resolution on every connection.",
          "refs": [
            "code:internal/tools/webfetch.go#isLocalHost",
            "code:internal/tools/webfetch.go#isDelegatedWorker",
            "code:internal/tools/webfetch.go#refuseNonPublicDial"
          ]
        },
        {
          "text": "There is no JavaScript rendering, so a single-page app returns an error suggesting the page is rendered by JavaScript.",
          "refs": [
            "code:internal/tools/webfetch.go#webFetchTool"
          ]
        }
      ]
    },
    {
      "id": "external:codex",
      "level": "context",
      "kind": "external-system",
      "title": "Codex",
      "summary": {
        "text": "The owner's local Codex installation, whose app-server and session logs serve reads to show Codex account and token usage in Data > Usage.",
        "refs": [
          "code:internal/usage/codex.go#ReadCodexAccount",
          "doc:cmd/evie/docs/active/usage.spec.md#first-delivery"
        ]
      },
      "does": [
        {
          "text": "Runs as `codex app-server --stdio` once per configured Codex home, with MCP servers, analytics and apps turned off, answering only allowlisted account reads within 20 seconds.",
          "refs": [
            "code:internal/usage/codex.go#ReadCodexAccount"
          ]
        },
        {
          "text": "Provides token metadata from the `sessions` and `archived_sessions` JSONL files in each Codex home, which Evie indexes in memory by file offset without copying text.",
          "refs": [
            "code:internal/usage/local.go#LocalIndex",
            "code:internal/usage/local.go#NewLocalIndex"
          ]
        },
        {
          "text": "Is found through EVIE_CODEX_BIN, the ChatGPT desktop bundle or PATH, with homes from EVIE_CODEX_HOMES (at most four absolute paths), CODEX_HOME or `~/.codex`.",
          "refs": [
            "code:internal/usage/codex.go#CodexBinary",
            "code:internal/usage/codex.go#CodexHomes"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It is read only when Data > Usage asks: the account at most once a minute, and the local index every 30 seconds, or every 3 seconds until a first full scan completes.",
          "refs": [
            "code:internal/usage/service.go#Service.refresh"
          ]
        },
        {
          "text": "Its errors, stderr, reset credits and credentials are never forwarded, and an unavailable Codex shows as stale or unavailable without hiding Evie's own usage.",
          "refs": [
            "code:internal/usage/codex.go#ReadCodexAccount",
            "code:internal/usage/service.go#Service.readAccount",
            "doc:cmd/evie/docs/active/usage.spec.md#first-delivery"
          ]
        },
        {
          "text": "An invalid EVIE_CODEX_HOMES stops `evie serve` at startup.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/usage/codex.go#CodexHomes"
          ]
        }
      ]
    },
    {
      "id": "external:launchd",
      "level": "context",
      "kind": "external-system",
      "title": "launchd",
      "summary": {
        "text": "macOS's launchd, which fires the owner's cron jobs by starting `evie cron-exec <job-id>` on each job's calendar schedule, whether or not Evie is running.",
        "refs": [
          "code:internal/tools/cron.go#plistFor",
          "doc:cmd/evie/docs/done/cron.spec.md#purpose"
        ]
      },
      "does": [
        {
          "text": "Loads the plist `cron_add` writes to `~/Library/LaunchAgents/com.evie.cron.<name>.plist`, with StartCalendarInterval entries translated from the cron expression, after a launchctl bootout and bootstrap.",
          "refs": [
            "code:internal/tools/cron.go#cronAdd",
            "code:internal/tools/cron.go#cronPlistPath",
            "code:internal/tools/cron.go#installJob"
          ]
        },
        {
          "text": "Unloads a job when `cron_remove` boots it out and deletes its plist; `cron_list` reads jobs from the Evie database and never asks launchd.",
          "refs": [
            "code:internal/tools/cron.go#cronRemove",
            "code:internal/tools/cron.go#reconcileCronJob",
            "code:internal/tools/cron.go#cronList",
            "doc:cmd/evie/docs/done/cron.spec.md#the-mechanism-decided-by-research-verified-claims-marked"
          ]
        },
        {
          "text": "Runs a fire missed during sleep on wake, coalescing several misses into one run; fires missed while the machine was off are lost.",
          "refs": [
            "doc:cmd/evie/docs/done/cron.spec.md#the-mechanism-decided-by-research-verified-claims-marked"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The plist records the `evie` binary's path at the time of `cron_add`, so jobs must be removed and added again if the binary moves.",
          "refs": [
            "code:internal/tools/cron.go#cronAdd",
            "doc:cmd/evie/docs/done/cron.spec.md#the-mechanism-decided-by-research-verified-claims-marked"
          ]
        },
        {
          "text": "The Evie database is the source of truth and plists are generated, so drift such as a hand-deleted plist goes unnoticed.",
          "refs": [
            "doc:cmd/evie/docs/done/cron.spec.md#the-mechanism-decided-by-research-verified-claims-marked"
          ]
        },
        {
          "text": "Cron jobs are a separate, older mechanism from Workflow Schedules: launchd fires a raw shell command, outside the Workflow Runtime and its leases.",
          "refs": [
            "code:internal/tools/cron.go#cronAdd",
            "code:internal/tools/cron.go#RunScheduled",
            "term:Workflow Schedule"
          ]
        }
      ]
    },
    {
      "id": "external:shell",
      "level": "context",
      "kind": "external-system",
      "title": "Login shell",
      "summary": {
        "text": "The owner's own login shell (`$SHELL`), which runs the model's bash commands, cron job commands and the web terminal on the owner's machine.",
        "refs": [
          "code:internal/tools/bash.go#shellPath",
          "code:internal/localterminal/session.go#Start"
        ]
      },
      "does": [
        {
          "text": "Runs each bash tool command as `$SHELL -l -c` with the owner's shell snapshot sourced, keeping the working directory between calls, with a two-minute default timeout that can rise to ten minutes.",
          "refs": [
            "code:internal/tools/bash.go#runBash",
            "code:internal/tools/bash.go#bashTool"
          ]
        },
        {
          "text": "Runs cron job commands the same way from the home directory, bounded at 30 minutes per run.",
          "refs": [
            "code:internal/tools/cron.go#RunScheduled",
            "code:cmd/evie/cronexec.go#cronExecTimeout"
          ]
        },
        {
          "text": "Runs as an interactive login shell on a PTY in the Workspace folder for the web terminal.",
          "refs": [
            "code:internal/localterminal/session.go#Start"
          ]
        },
        {
          "text": "Runs build workers' bash only inside an OS sandbox (Seatbelt on macOS, bubblewrap on Linux) with no network and writes confined to the worktree and its private temp folder; without a sandbox, build workers get no bash.",
          "refs": [
            "code:internal/tools/sandbox.go#WorkerSandbox"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The main chat's bash is ungated, with no approval prompt and no path denylist, because a shell walks around any fence; the defense is that the owner reads what Evie does.",
          "refs": [
            "code:internal/tools/bash.go#bashTool"
          ]
        },
        {
          "text": "The main chat's bash and the web terminal inherit Evie's whole environment, including keys loaded from `~/.evie/.env`; only build workers' sandboxed bash gets a built environment without them.",
          "refs": [
            "code:internal/tools/bash.go#runBash",
            "code:internal/localterminal/session.go#Start",
            "code:internal/tools/sandbox.go#buildEnv"
          ]
        }
      ]
    },
    {
      "id": "container:serve",
      "level": "container",
      "kind": "process",
      "title": "evie serve",
      "summary": {
        "text": "The `evie serve` process: the loopback web server and the runtime behind it, which also runs Evie's background hosts while it is up.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:internal/web/lifecycle.go#ServeWithContext"
        ]
      },
      "does": [
        {
          "text": "Listens on EVIE_ADDR, by default `127.0.0.1:6687`, and refuses any address that is not loopback.",
          "refs": [
            "code:internal/web/serve.go#listenAddr",
            "code:internal/web/lifecycle.go#ServeWithContext"
          ]
        },
        {
          "text": "Serves the React build compiled into the binary and the `/api` routes, streaming each chat turn's events back to the browser.",
          "refs": [
            "code:internal/web/static.go#Server.staticHandler",
            "code:internal/web/serve.go#Server.Handler",
            "doc:cmd/evie/docs/active/serve.spec.md#sse-event-vocabulary-server--client"
          ]
        },
        {
          "text": "Runs each open chat's agent session against OpenRouter, resolving the selected model's context profile per chat and composing tools from the chat's Agent Preset.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/web_context_sessions.go#newWebContextSessionController"
          ]
        },
        {
          "text": "Starts the workflow scheduler, subagent recovery, retrieval index maintenance, the background memory pass, the optional memory compiler host and, with GITHUB_TOKEN, the 60-second GitHub change sync.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#startChangeSync"
          ]
        },
        {
          "text": "Is the only process that holds MCP tokens: it runs OAuth sign-ins, refreshes tokens, keeps one live session per MCP connection and answers the CLI's connection commands.",
          "refs": [
            "code:cmd/evie/connections.go#newMCPConnectionManager",
            "code:internal/mcpconn/pool.go#NewPool",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries"
          ]
        },
        {
          "text": "Hosts the owner's Workspace surfaces: files, diffs and tabs from the attached folder, a terminal, Changes with push and merge, Spending through Plaid, Data > Usage from Codex, and memory review.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/terminal.go#Server.registerTerminalRoutes",
            "code:internal/finance/spending.go#NewSpendingService",
            "code:internal/usage/service.go#NewService"
          ]
        },
        {
          "text": "Shuts down on SIGINT or SIGTERM: the HTTP server gets five seconds to drain, and each background host up to five seconds to stop.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/lifecycle.go#serveListenerContext",
            "code:cmd/evie/memory_background.go#memoryBackgroundHost.Start",
            "code:cmd/evie/compiler_host.go#startCompilerHost",
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        }
      ],
      "owns": [
        {
          "text": "Only per-process state: open chats and their live feeds, pending approvals, MCP sessions and terminals; everything durable is written to the databases.",
          "refs": [
            "code:internal/web/serve.go#Server",
            "code:internal/mcpconn/pool.go#Pool",
            "code:internal/web/terminal.go#Server.registerTerminalRoutes"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Loopback HTTP: `/api` JSON routes, most of them POST-only management routes behind the same guard, and the static app for every other path.",
          "refs": [
            "code:internal/web/serve.go#Server.managementRoute",
            "code:internal/web/serve.go#Server.guard",
            "code:internal/web/static.go#Server.staticHandler"
          ]
        },
        {
          "text": "The OAuth callback for MCP sign-ins at `/api/connections/oauth/callback` on serve's own address.",
          "refs": [
            "code:cmd/evie/connections.go#newMCPConnectionManager"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "If the default Agent Preset is invalid, serve still starts, but as a management-only server with no chat, while the memory hosts and the workflow scheduler run anyway.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/serve.go#NewManagedServer"
          ]
        },
        {
          "text": "A workflow runtime configuration error stops serve at startup, whereas the REPL logs `workflow scheduler off` and keeps chatting.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Once the preset is valid, a missing OPENROUTER_API_KEY stops serve, because the chat client is built at startup.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/openrouter/client.go#NewClient"
          ]
        },
        {
          "text": "Running serve and the REPL together runs two workflow schedulers and two background memory passes on one database; leases stored there keep them from doing the same work twice.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:internal/memorybackground/pass.go#Runner.Tick"
          ]
        }
      ],
      "terms": [
        "Agent Preset",
        "Action Approval",
        "Workspace",
        "Change",
        "Account Connection"
      ],
      "links": [
        "doc:cmd/evie/docs/active/serve.spec.md"
      ],
      "contains": [
        "pkg:cmd/evie",
        "pkg:internal/web",
        "pkg:docs/architecture/explorer",
        "pkg:internal/agent",
        "pkg:internal/openrouter",
        "pkg:internal/plugins",
        "pkg:internal/tools",
        "pkg:internal/composition",
        "pkg:internal/task",
        "pkg:internal/subagents",
        "pkg:internal/delegation",
        "pkg:internal/untrusted",
        "pkg:internal/repoinstructions",
        "pkg:internal/memory",
        "pkg:internal/eviedb",
        "pkg:internal/localembedding",
        "pkg:internal/mcpconn",
        "pkg:internal/mcpchat",
        "pkg:internal/changes",
        "pkg:internal/github",
        "pkg:internal/worktree",
        "pkg:internal/localfiles",
        "pkg:internal/localterminal",
        "pkg:internal/workspacetabs",
        "pkg:internal/usage",
        "pkg:internal/finance",
        "pkg:internal/youtube",
        "container:workflow-scheduler",
        "container:memory-background-pass",
        "container:compiler-host",
        "container:retrieval-maintenance",
        "container:subagent-recovery",
        "container:change-sync"
      ]
    },
    {
      "id": "container:repl",
      "level": "container",
      "kind": "cli",
      "title": "evie REPL",
      "summary": {
        "text": "The interactive terminal REPL that `evie` with no command starts: one chat at a time, plus the background hosts a long-lived Evie process runs.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:cmd/evie/repl.go#runREPLContextIOWithMemory"
        ]
      },
      "does": [
        {
          "text": "Shows a numbered chooser of resumable sessions, registered projects and Workspaces, a new Global session, and registering the launch directory as a project; the choice sets the session's Context Scope.",
          "refs": [
            "code:cmd/evie/repl.go#selectREPLSession",
            "code:cmd/evie/repl.go#renderREPLChooserWithWorkspaces",
            "term:Context Scope"
          ]
        },
        {
          "text": "Streams each reply to the terminal and pauses gated tool calls at an `approve? [y/N]` prompt.",
          "refs": [
            "code:cmd/evie/repl.go#runREPLContextIOWithMemory"
          ]
        },
        {
          "text": "Answers `/memory` commands typed into the chat, which inspect Semantic Memory or replay accepted operations into a shadow projection, without a model call and never as model capabilities.",
          "refs": [
            "code:cmd/evie/repl.go#handleMemoryCommand",
            "code:cmd/evie/repl.go#semanticMaintenanceMemory"
          ]
        },
        {
          "text": "Runs the workflow scheduler, subagent recovery, retrieval index maintenance, the background memory pass and the optional compiler host, like serve, but no GitHub change sync.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Closes standard input when SIGINT or SIGTERM arrives, so a blocked read ends and the hosts stop.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/runtime_lifecycle.go#closeInputOnCancellation"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "MCP tools are unavailable in REPL chats: they answer that MCP tools run only in `evie serve` chats.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries"
          ]
        },
        {
          "text": "Failing to resolve the default Agent Preset, or a missing OPENROUTER_API_KEY, stops the REPL when it builds the session.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "A workflow runtime configuration problem only turns the scheduler off with a reason; chat still starts.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "terms": [
        "Context Scope",
        "Action Approval",
        "Semantic Memory",
        "Workspace"
      ],
      "contains": [
        "pkg:cmd/evie",
        "pkg:internal/agent",
        "pkg:internal/openrouter",
        "pkg:internal/plugins",
        "pkg:internal/tools",
        "pkg:internal/composition",
        "pkg:internal/task",
        "pkg:internal/subagents",
        "pkg:internal/delegation",
        "pkg:internal/untrusted",
        "pkg:internal/repoinstructions",
        "pkg:internal/memory",
        "pkg:internal/eviedb",
        "pkg:internal/localembedding",
        "pkg:internal/changes",
        "pkg:internal/github",
        "pkg:internal/worktree",
        "pkg:internal/workspacetabs",
        "pkg:internal/finance",
        "pkg:internal/youtube",
        "container:workflow-scheduler",
        "container:memory-background-pass",
        "container:compiler-host",
        "container:retrieval-maintenance",
        "container:subagent-recovery"
      ]
    },
    {
      "id": "container:management-cli",
      "level": "container",
      "kind": "cli",
      "title": "evie management commands",
      "summary": {
        "text": "One-shot `evie` subcommands that inspect or manage Evie's state and then exit.",
        "refs": [
          "code:cmd/evie/main.go#main"
        ]
      },
      "does": [
        {
          "text": "Dispatches on the first argument through a chain of handlers: `workflows` review (validate, simulate, templates) needs no database, `connections` and `workspaces mcp-tools` only call serve, and every other command opens the Evie database first.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/workflows.go#runWorkflowManagement",
            "code:cmd/evie/connections.go#runConnectionManagement"
          ]
        },
        {
          "text": "Offers memory commands: `memory-health`, `memory-backfill`, `memory-compiler` (status and disable), `memory-compile` and `memory-candidates`, `memory-review`, and `memory-background status`.",
          "refs": [
            "code:cmd/evie/compiler_diagnostics.go#runCompilerDiagnostics",
            "code:cmd/evie/compiler_history.go#runCompilerHistoryManagement",
            "code:cmd/evie/compiler_activation.go#runCompilerActivationManagement",
            "code:cmd/evie/compiler_management.go#runCompilerManagement",
            "code:cmd/evie/candidate_review.go#runOwnerReviewManagement",
            "code:cmd/evie/memory_background.go#runMemoryBackgroundManagement"
          ]
        },
        {
          "text": "Offers Workspace and code commands: `workspaces`, `projects`, `worktrees` (list and clean) and `changes` (list, push, merge, disarm and close).",
          "refs": [
            "code:cmd/evie/workspace_explore.go#runWorkspaceManagement",
            "code:cmd/evie/build_verify.go#runProjectManagement",
            "code:cmd/evie/worktrees.go#runWorktreeManagement",
            "code:cmd/evie/changes.go#runChangeManagement"
          ]
        },
        {
          "text": "Runs `workflows` activation commands (propose, approve, run, inspect, schedules and more) on a Workflow Runtime and subagent supervisor of its own; `workflows run` without `--detach` executes the run in this process until it finishes or pauses.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/workflows.go#workflowActivationCommands",
            "code:cmd/evie/workflow_activation.go#runWorkflowActivation",
            "code:cmd/evie/workflow_runs.go"
          ]
        },
        {
          "text": "Runs `plugins`, `presets` and `sessions` through the same Plugin Manager the chat uses, to manage plugin enablement and inspect Agent Presets and sessions.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/management.go#runManagementCommand"
          ]
        },
        {
          "text": "Runs `memory-compile` against one owner-selected range with the pinned local extractor named by `--config`, in this process.",
          "refs": [
            "code:cmd/evie/compiler_management.go#runCompilerManagement"
          ]
        },
        {
          "text": "Runs the owner's merge gate for `changes merge` and `close` in this process with its own subagent supervisor; serve's recovery leaves those steps alone while they run.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The usage line printed for an unknown command lists only `serve`, `cron-exec`, `plugins`, `presets`, `sessions` and `workflows`, though many more subcommands exist.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Opening the Evie database applies its schema upgrades, so any database-backed command migrates `~/.evie/evie.db` to the binary's version.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/eviedb/db.go#openDBAtContextWithHooks"
          ]
        },
        {
          "text": "Before `plugins`, `presets` and `sessions` run, startup imports the legacy Todo list, recovers inactive Task Claims and subagent records, and applies plugin defaults, so even these inspection commands write to the database.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "`connections` and `workspaces mcp-tools` are clients of serve's routes, because only serve holds MCP tokens: when the connection to serve is refused they fail with 'evie serve is not running at <addr>', and any other failure to reach it is returned as it is.",
          "refs": [
            "code:cmd/evie/connections.go#connectionCommands.post"
          ]
        }
      ],
      "terms": [
        "Workflow Definition",
        "Workflow Run",
        "Memory Candidate",
        "Change",
        "Agent Preset",
        "Plugin Manager"
      ],
      "contains": [
        "pkg:cmd/evie",
        "pkg:internal/eviedb",
        "pkg:internal/memory",
        "pkg:internal/plugins",
        "pkg:internal/composition",
        "pkg:internal/workflow",
        "pkg:internal/workflowruntime",
        "pkg:internal/procedural",
        "pkg:internal/subagents",
        "pkg:internal/delegation",
        "pkg:internal/task",
        "pkg:internal/changes",
        "pkg:internal/github",
        "pkg:internal/worktree",
        "pkg:internal/localextractor",
        "pkg:internal/openrouter"
      ]
    },
    {
      "id": "container:cron-exec",
      "level": "container",
      "kind": "process",
      "title": "evie cron-exec",
      "summary": {
        "text": "The one-shot `evie cron-exec <job-id>` process that launchd starts on a cron job's schedule: it runs the job's command and records the run.",
        "refs": [
          "code:cmd/evie/cronexec.go#runCronExec",
          "code:internal/tools/cron.go#plistFor"
        ]
      },
      "does": [
        {
          "text": "Reads the job's command from the `jobs` table, runs it with `tools.RunScheduled` in the owner's login shell from the home directory, and appends the output and exit code to `job_runs`.",
          "refs": [
            "code:cmd/evie/cronexec.go#cronExec",
            "code:internal/tools/cron.go#RunScheduled"
          ]
        },
        {
          "text": "Bounds each run at 30 minutes and caps the output it stores.",
          "refs": [
            "code:cmd/evie/cronexec.go#cronExecTimeout",
            "code:internal/tools/cron.go#capRunOutput"
          ]
        },
        {
          "text": "Exits 0 even when the job fails, so launchd does not treat Evie as broken; it exits 2 for bad arguments and 1 for an unusable job or database.",
          "refs": [
            "code:cmd/evie/cronexec.go#cronExec"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It never builds a model client, so it runs headless under launchd without an API key.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "A plist whose job row was deleted fails with a message that its plist is stale and should be removed and added again.",
          "refs": [
            "code:cmd/evie/cronexec.go#cronExec"
          ]
        }
      ],
      "terms": [
        "Workflow Schedule"
      ],
      "contains": [
        "pkg:cmd/evie",
        "pkg:internal/tools",
        "pkg:internal/eviedb"
      ]
    },
    {
      "id": "container:web-ui",
      "level": "container",
      "kind": "user-interface",
      "title": "Web UI",
      "summary": {
        "text": "The React single-page app the owner uses in a browser, built with Vite and compiled into the `evie` binary.",
        "refs": [
          "code:internal/web/ui/src/App.tsx",
          "code:internal/web/ui/package.json",
          "code:internal/web/static.go#distFS"
        ]
      },
      "does": [
        {
          "text": "Is served by `evie serve` from the embedded `ui/dist` build, with fingerprinted assets cached forever and `index.html` never cached.",
          "refs": [
            "code:internal/web/static.go#Server.staticHandler",
            "code:internal/web/static.go#serveIndex"
          ]
        },
        {
          "text": "Sends each chat message as a JSON POST to `/api/chat` and parses the reply's `event:` and `data:` blocks from the fetch body, because EventSource cannot POST.",
          "refs": [
            "code:internal/web/ui/src/api/stream.ts",
            "code:internal/web/serve.go#Server.Handler"
          ]
        },
        {
          "text": "Offers the chat with model selection, Workspaces with files, a terminal and tabs, data views, memory review and notifications.",
          "refs": [
            "code:internal/web/ui/src/App.tsx",
            "code:internal/web/ui/package.json"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Talks only to serve's loopback origin, sending the exact `application/json` content type the server's guard requires.",
          "refs": [
            "code:internal/web/ui/src/api/stream.ts",
            "code:internal/web/serve.go#Server.guard"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The build output is gitignored, so a fresh clone must run `npm --prefix internal/web/ui run build` before `go build`, and the embed fails naming the missing directory.",
          "refs": [
            "code:internal/web/static.go",
            "code:internal/web/static.go#distFS"
          ]
        },
        {
          "text": "Unknown paths return the app shell rather than a 404.",
          "refs": [
            "code:internal/web/static.go#Server.staticHandler"
          ]
        }
      ]
    },
    {
      "id": "container:evie-db",
      "level": "container",
      "kind": "store",
      "title": "Evie database",
      "summary": {
        "text": "Evie's main SQLite database at `~/.evie/evie.db`, shared by every `evie` process and `todo`: conversations, memory, Tasks, Workspaces, Workflow Runs, plugin state and more.",
        "refs": [
          "code:internal/eviedb/db.go#OpenDBContext",
          "code:internal/eviedb/db.go#schema"
        ]
      },
      "does": [
        {
          "text": "Opens through eviedb, which creates `~/.evie` with mode 0700, forces the file to mode 0600, and applies the whole schema and its in-place upgrades on every open.",
          "refs": [
            "code:internal/eviedb/db.go#OpenDBContext",
            "code:internal/eviedb/db.go#hardenWritableDBFile",
            "code:internal/eviedb/db.go#openDBAtContextWithHooks"
          ]
        },
        {
          "text": "Runs in WAL mode with foreign keys on and a five-second busy timeout on every pooled connection.",
          "refs": [
            "code:internal/eviedb/db.go#connectionPragmas"
          ]
        },
        {
          "text": "Can be open in several processes at once: serve, the REPL, management commands, cron-exec and `todo`, with the cron tools opening it again for their own job rows.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/cronexec.go#cronExec",
            "code:cmd/todo/main.go#run",
            "code:internal/tools/cron.go#cronAdd"
          ]
        }
      ],
      "owns": [
        {
          "text": "Conversation history, which is Episodic Memory, plus Semantic Memory, Memory Candidates and the derived full-text and dense retrieval indexes.",
          "refs": [
            "code:internal/eviedb/history.go#Store.BindHistory",
            "code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration",
            "term:Episodic Memory"
          ]
        },
        {
          "text": "Task Trees and their claims, Workspaces with their folders and presets, Workflow Runs, cron jobs and their runs.",
          "refs": [
            "code:cmd/todo/main.go#run",
            "code:internal/eviedb/workspace_presets.go#ensureWorkspacePresets",
            "code:internal/eviedb/workflow_runs.go#WorkflowRun",
            "code:cmd/evie/cronexec.go#cronExec"
          ]
        },
        {
          "text": "MCP connection records, which never carry credentials.",
          "refs": [
            "code:internal/mcpconn/connection.go#Connection"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Every process that opens it runs the schema upgrades, so a newer binary migrates the file the moment any command touches it, even `todo list`.",
          "refs": [
            "code:internal/eviedb/db.go#openDBAtContextWithHooks",
            "code:cmd/todo/main.go#run"
          ]
        },
        {
          "text": "The read-only open skips the schema and fails if the file does not exist yet.",
          "refs": [
            "code:internal/eviedb/db.go#OpenDBReadOnlyContext"
          ]
        }
      ],
      "terms": [
        "Episodic Memory",
        "Semantic Memory",
        "Memory Candidate",
        "Task Tree",
        "Workflow Run",
        "Workspace"
      ],
      "contains": [
        "pkg:internal/eviedb"
      ]
    },
    {
      "id": "container:finance-db",
      "level": "container",
      "kind": "store",
      "title": "Finance database",
      "summary": {
        "text": "The finance domain's SQLite database at `~/.finance/finance.db`, which holds linked banks, transactions and their categorization, including Plaid access tokens.",
        "refs": [
          "code:internal/finance/db.go#OpenDBContext",
          "code:internal/finance/db.go#schema"
        ]
      },
      "does": [
        {
          "text": "Opens through finance, which creates `~/.finance` with mode 0700, forces the file to mode 0600 because access tokens live in it, and applies an idempotent CREATE IF NOT EXISTS schema on every open.",
          "refs": [
            "code:internal/finance/db.go#OpenDBContext",
            "code:internal/finance/db.go#openDBAtContext",
            "code:internal/finance/db.go#schema"
          ]
        },
        {
          "text": "Is written by the `finance` CLI, by serve's Spending page and the finance tools, and by the finance workflow effects that sync and categorize the ledger.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/spending.go#NewSpendingService",
            "code:internal/tools/finance.go#financeSync",
            "code:internal/plugins/finance_effects.go#Finance.performSyncLedger"
          ]
        }
      ],
      "owns": [
        {
          "text": "Plaid Items with their access tokens and sync cursors, transactions, categories, merchant and name rules, Budget Entries and limits, review decisions and effect receipts.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "term:Budget Entry"
          ]
        },
        {
          "text": "Linked accounts, account snapshots and the hosted Link sessions of bank linking.",
          "refs": [
            "code:internal/finance/spending_accounts.go#spendingAccountsSchema"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Unlike the Evie database it does not switch to WAL, so it uses SQLite's default rollback journal and each connection waits up to five seconds for a lock.",
          "refs": [
            "code:internal/finance/db.go#openDBAtContext",
            "code:internal/eviedb/db.go#connectionPragmas"
          ]
        },
        {
          "text": "Holding Plaid access tokens in SQLite departs from ADR 0028, which gives credentials to the Kernel's secret storage.",
          "refs": [
            "code:internal/finance/db.go#schema",
            "adr:0028"
          ]
        }
      ],
      "terms": [
        "Budget Entry"
      ],
      "contains": [
        "pkg:internal/finance"
      ]
    },
    {
      "id": "container:youtube-db",
      "level": "container",
      "kind": "store",
      "title": "YouTube library database",
      "summary": {
        "text": "The YouTube transcript library's SQLite database at `~/.evie/transcripts/transcripts.db`.",
        "refs": [
          "code:internal/youtube/db.go#databasePath"
        ]
      },
      "does": [
        {
          "text": "Opens through youtube, which creates and locks down its folder to mode 0700, forces the file to 0600, and applies an idempotent schema on every open.",
          "refs": [
            "code:internal/youtube/db.go#OpenDBContext",
            "code:internal/youtube/db.go#OpenDBAtContext"
          ]
        },
        {
          "text": "Is written by `ytscribe` and the YouTube plugin's transcript tool, and read through a read-only open by the database query tool.",
          "refs": [
            "code:cmd/ytscribe/main.go#openYTScribeDB",
            "code:internal/tools/youtube.go#openYouTubeDB",
            "code:internal/tools/db.go#openTranscriptDB"
          ]
        }
      ],
      "owns": [
        {
          "text": "Channels, videos, transcripts with their fetch states, and a full-text index over transcripts.",
          "refs": [
            "code:internal/youtube/db.go#schema"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Like the finance database it does not switch to WAL, unlike the Evie database.",
          "refs": [
            "code:internal/youtube/db.go#OpenDBAtContext",
            "code:internal/eviedb/db.go#connectionPragmas"
          ]
        }
      ],
      "contains": [
        "pkg:internal/youtube"
      ]
    },
    {
      "id": "container:procedural-repo",
      "level": "container",
      "kind": "store",
      "title": "Procedural Git repository",
      "summary": {
        "text": "Evie's procedural Git repository at `~/.evie/procedural`: a private bare repository that is the reviewed, versioned home of Workflow Definitions.",
        "refs": [
          "code:internal/procedural/repo.go#Repo",
          "code:internal/procedural/repo.go#DefaultRoot"
        ]
      },
      "does": [
        {
          "text": "Is created on first use as a private bare repository (mode 0700, initial branch master).",
          "refs": [
            "code:internal/procedural/repo.go#Repo.ensure"
          ]
        },
        {
          "text": "Is written only with Git plumbing and a compare-and-swap move of master, so there is no work tree to leave dirty and a crash before the ref moves leaves only unreferenced objects.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.Commit"
          ]
        },
        {
          "text": "Receives Workflow Definition proposals, and each built-in workflow version when it is first resolved, from the Workflow Runtime, which reads definitions back at exact commits.",
          "refs": [
            "code:internal/workflowruntime/runtime.go#Runtime.Propose",
            "code:internal/workflowruntime/builtins.go#Runtime.recordBuiltin",
            "code:internal/workflowruntime/builtins.go#Runtime.resolve",
            "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition",
            "code:internal/procedural/repo.go#Repo.Source"
          ]
        },
        {
          "text": "Runs every git command with hooks, fsmonitor, pagers, signing, automatic maintenance and transports off, in a built environment that ignores global and system Git configuration.",
          "refs": [
            "code:internal/procedural/repo.go#hardening",
            "code:internal/procedural/repo.go#env"
          ]
        }
      ],
      "owns": [
        {
          "text": "What was proposed and approved, as committed files; approval records and Workflow Runs live in the Evie database.",
          "refs": [
            "code:internal/procedural/repo.go#Repo"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It sits inside `~/.evie`, which every file tool, explore root and sandboxed shell refuses, so no worker can read or write it.",
          "refs": [
            "code:internal/procedural/repo.go"
          ]
        },
        {
          "text": "Readers name an exact commit, so later commits never change what an earlier commit reads.",
          "refs": [
            "code:internal/procedural/repo.go#Repo.ReadFile"
          ]
        },
        {
          "text": "ADR 0012 places user-authored Agent Presets here and ADR 0028 places reviewed Workspace configuration here, but today only the Workflow Runtime writes it; presets are compiled into the plugins package and Workspace presets live in the Evie database.",
          "refs": [
            "adr:0012",
            "adr:0028",
            "code:internal/workflowruntime/runtime.go#Runtime.Propose",
            "code:internal/plugins/preset.go#standardPresetContent",
            "code:internal/eviedb/workspace_presets.go#ensureWorkspacePresets"
          ]
        }
      ],
      "terms": [
        "Procedural Memory",
        "Workflow Definition",
        "Workflow Approval"
      ],
      "contains": [
        "pkg:internal/procedural"
      ]
    },
    {
      "id": "container:keychain",
      "level": "container",
      "kind": "store",
      "title": "Keychain",
      "summary": {
        "text": "The macOS login Keychain, where serve keeps each MCP connection's OAuth credentials as one generic-password item.",
        "refs": [
          "code:internal/mcpconn/keychain.go#Keychain"
        ]
      },
      "does": [
        {
          "text": "Is reached through `/usr/bin/security`, with service `dev.evie.mcp` and the Connection ID as the account.",
          "refs": [
            "code:internal/mcpconn/keychain.go#RunSecurity",
            "code:internal/mcpconn/keychain.go#keychainService",
            "code:cmd/evie/connections.go#newMCPConnectionManager"
          ]
        },
        {
          "text": "Receives secrets only on standard input, base64-encoded inside a `security -i` line, never in process arguments.",
          "refs": [
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        }
      ],
      "owns": [
        {
          "text": "MCP access tokens, refresh tokens and client secrets, which never appear in SQLite, procedural Git, logs, tool results or model context.",
          "refs": [
            "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md#boundaries",
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Credentials are capped at 3,000 bytes of JSON so one base64 line fits security(1)'s interactive line limit; larger ones fail.",
          "refs": [
            "code:internal/mcpconn/keychain.go#maxCredentialsJSON",
            "code:internal/mcpconn/keychain.go#Keychain.Save"
          ]
        },
        {
          "text": "Only serve touches it, so exactly one process refreshes a connection's tokens; CLI connection commands call serve instead.",
          "refs": [
            "code:internal/mcpconn/connection.go",
            "code:cmd/evie/connections.go#runConnectionManagement"
          ]
        },
        {
          "text": "ADR 0028 gives all credentials to the Kernel's secret storage, but only MCP credentials live here: API keys and GITHUB_TOKEN come from the environment or `~/.evie/.env`, and Plaid access tokens sit in the finance database.",
          "refs": [
            "adr:0028",
            "code:cmd/evie/main.go#main",
            "code:internal/finance/db.go#schema",
            "doc:cmd/evie/docs/active/changes.spec.md#github-connection"
          ]
        },
        {
          "text": "When security(1) cannot run or fails, the store reports the Keychain as unavailable and a connection can show the `keychain_unavailable` status.",
          "refs": [
            "code:internal/mcpconn/keychain.go#ErrKeychainUnavailable",
            "code:internal/mcpconn/connection.go#StatusKeychainUnavailable"
          ]
        }
      ],
      "terms": [
        "Connection ID",
        "Account Connection"
      ],
      "contains": [
        "pkg:internal/mcpconn"
      ]
    },
    {
      "id": "container:env-file",
      "level": "container",
      "kind": "store",
      "title": "Environment file",
      "summary": {
        "text": "`~/.evie/.env`, the dotenv file every `evie` command loads at startup for credentials and settings.",
        "refs": [
          "code:cmd/evie/main.go#main"
        ]
      },
      "does": [
        {
          "text": "Is loaded first, then `.env` and `../../.env` relative to the working directory as a development convenience, each in its own call so a missing file does not stop the next.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Never overrides a variable the shell already set.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "owns": [
        {
          "text": "The credentials the code reads from the environment: OPENROUTER_API_KEY, GITHUB_TOKEN, BRAVE_API_KEY, and Plaid's PLAID_CLIENT_ID and PLAID_SECRET.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#repositoryReader",
            "code:internal/tools/websearch.go#webSearch",
            "code:internal/finance/client.go#plaidClient"
          ]
        },
        {
          "text": "Feature settings such as EVIE_MODEL, EVIE_ADDR, EVIE_REMOTE_MEMORY, EVIE_COMPILER_CONFIG, EVIE_MEMORY_BACKGROUND and EVIE_WORKFLOW_REMOTE_MODELS.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/web/serve.go#listenAddr",
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint",
            "code:internal/memorybackground/config.go#FromEnv",
            "code:cmd/evie/workflow_activation.go#workflowRemoteModels"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "`web_search`'s missing-key error tells the model to add BRAVE_API_KEY to the repo-root `.env`, but an installed `evie` reads `~/.evie/.env` first and sees a repo-root `.env` only when started from the repository root or `cmd/evie`.",
          "refs": [
            "code:internal/tools/websearch.go#webSearch",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "The `finance` command does not read this file: it calls godotenv once with `../../.env` and `.env`, and godotenv stops at the first file it cannot read, so the local `.env` its comment calls a fallback loads only when `../../.env` also exists.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Evie does not check this file's permissions, unlike its databases, which it forces to mode 0600.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/eviedb/db.go#hardenWritableDBFile"
          ]
        },
        {
          "text": "Keys loaded from it reach the main chat's bash commands and the web terminal, which inherit Evie's environment; only build workers' sandboxed bash gets a built environment.",
          "refs": [
            "code:internal/tools/bash.go#runBash",
            "code:internal/localterminal/session.go#Start",
            "code:internal/tools/sandbox.go#buildEnv"
          ]
        }
      ]
    },
    {
      "id": "container:workspace-folders",
      "level": "container",
      "kind": "store",
      "title": "Workspace folders",
      "summary": {
        "text": "The folder on the owner's disk attached to each Workspace, which may hold its Repository; Evie reads it, runs git and shells in it, and keeps build worktrees under its `.evie` folder.",
        "refs": [
          "code:internal/web/local_folder.go#Server.registerFolderRoutes",
          "code:internal/worktree/worktree.go#Folder"
        ]
      },
      "does": [
        {
          "text": "Is listed, read and diffed by serve for the Files view, and supplies the Workspace's tabs, which serve reads from the folder.",
          "refs": [
            "code:internal/web/local_folder.go#Server.registerFolderRoutes",
            "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabRoutes",
            "pkg:internal/localfiles"
          ]
        },
        {
          "text": "Hosts the web terminal's interactive login shell.",
          "refs": [
            "code:internal/web/terminal.go#Server.registerTerminalRoutes",
            "code:internal/localterminal/session.go#Start"
          ]
        },
        {
          "text": "Holds build workers' git worktrees at `<repo>/.evie/worktrees/<attempt>` with private temp folders at `<repo>/.evie/tmp/<attempt>`, kept out of the owner's status by the repository's local info/exclude.",
          "refs": [
            "code:internal/worktree/worktree.go#Folder",
            "code:internal/worktree/worktree.go#excludeLine"
          ]
        },
        {
          "text": "Is where pushes, fetches and clones to GitHub run, with the token passed to git per command.",
          "refs": [
            "code:internal/changes/publish.go#Service.remoteGit",
            "code:internal/changes/publish.go#Service.remoteEnv"
          ]
        },
        {
          "text": "Is chosen by the owner in the web UI, where serve can open the macOS folder dialog through osascript.",
          "refs": [
            "code:internal/web/native_folder_picker.go#chooseNativeWorkspaceFolder",
            "code:internal/web/local_folder.go#Server.registerFolderRoutes"
          ]
        }
      ],
      "owns": [
        {
          "text": "The owner's own files and Repository; Evie's only additions are the `.evie` build area and its exclude line.",
          "refs": [
            "code:internal/worktree/worktree.go#Folder",
            "code:internal/worktree/worktree.go#excludeLine"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Changing a Workspace's folder is refused while a turn runs or the Workspace has unfinished Changes, and the change stops that Workspace's open terminals.",
          "refs": [
            "code:internal/web/local_folder.go#Server.registerFolderRoutes"
          ]
        },
        {
          "text": "Build worktrees live inside the Workspace folder, never inside `~/.evie`, and a repository that tracks `.evie` cannot be used for build work.",
          "refs": [
            "code:internal/worktree/worktree.go#Folder",
            "code:internal/worktree/worktree.go#checkFolder"
          ]
        }
      ],
      "terms": [
        "Workspace",
        "Repository",
        "Change"
      ],
      "contains": [
        "pkg:internal/localfiles",
        "pkg:internal/localterminal",
        "pkg:internal/workspacetabs",
        "pkg:internal/worktree",
        "pkg:internal/changes"
      ]
    },
    {
      "id": "container:todo-cli",
      "level": "container",
      "kind": "cli",
      "title": "todo",
      "summary": {
        "text": "The `todo` command, the owner's CLI for durable Task Trees in the Evie database.",
        "refs": [
          "code:cmd/todo/main.go#main",
          "code:cmd/todo/main.go#usageText"
        ]
      },
      "does": [
        {
          "text": "Lists, gets, adds, updates and changes the status of Tasks, releases Task Claims, and shows a Task Tree's orchestration progress, including dependencies, claims, build workers and what `todo_ready` would offer.",
          "refs": [
            "code:cmd/todo/main.go#usageText",
            "code:cmd/todo/main.go#run"
          ]
        },
        {
          "text": "Opens `~/.evie/evie.db` directly and, before every command, imports the legacy Todo list and recovers inactive Task Claims.",
          "refs": [
            "code:cmd/todo/main.go#run"
          ]
        },
        {
          "text": "Works in Global scope without `--session`; with a primary session it uses that session's Workspace or project context.",
          "refs": [
            "code:cmd/todo/main.go#usageText",
            "code:cmd/todo/main.go#ownerSession"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Without `--session`, changes are attributed to the owner under a Global session with the fixed ID `todo-cli`, and a delegated session cannot authorize owner CLI access.",
          "refs": [
            "code:cmd/todo/main.go#ownerSession",
            "code:cmd/todo/main.go#todoCLISessionID"
          ]
        },
        {
          "text": "`todo delete` is deprecated: it cancels and keeps the Task.",
          "refs": [
            "code:cmd/todo/main.go#usageText"
          ]
        }
      ],
      "terms": [
        "Task",
        "Subtask",
        "Task Tree",
        "Task Claim",
        "Ready Set"
      ],
      "contains": [
        "pkg:cmd/todo",
        "pkg:internal/eviedb",
        "pkg:internal/task",
        "pkg:internal/memory",
        "pkg:internal/delegation"
      ]
    },
    {
      "id": "container:finance-cli",
      "level": "container",
      "kind": "cli",
      "title": "finance",
      "summary": {
        "text": "The `finance` command, the CLI over the finance domain: link banks through Plaid, sync transactions, seed rules and categorize.",
        "refs": [
          "code:cmd/finance/main.go#main"
        ]
      },
      "does": [
        {
          "text": "`link` runs or resumes the durable hosted Plaid Link flow: it prints the hosted URL, waits up to 35 minutes for completion and saves the new Item.",
          "refs": [
            "code:internal/finance/link.go#Link",
            "code:internal/finance/link.go#linkWithService"
          ]
        },
        {
          "text": "`sync` pulls new transactions for every linked bank and prints added, modified and removed counts per bank.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/sync.go#Sync"
          ]
        },
        {
          "text": "`rules` seeds merchant rules from `~/.finance/merchantLookup.json` plus the built-in name patterns, and `categorize` applies rules to unreviewed transactions.",
          "refs": [
            "code:cmd/finance/main.go#main"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "`link` and `sync` need PLAID_CLIENT_ID and PLAID_SECRET and fail fast without them; the Plaid environment is always production.",
          "refs": [
            "code:internal/finance/client.go#plaidClient"
          ]
        },
        {
          "text": "Its `.env` loading differs from `evie`: one godotenv call with `../../.env` then `.env` stops at the first missing file, and `~/.evie/.env` is never read.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "`finance db` prints a fixed 'tables: items, transactions' line; it only proves the database opens and its schema applies.",
          "refs": [
            "code:cmd/finance/main.go#main",
            "code:internal/finance/db.go#schema"
          ]
        }
      ],
      "contains": [
        "pkg:cmd/finance",
        "pkg:internal/finance"
      ]
    },
    {
      "id": "container:ytscribe-cli",
      "level": "container",
      "kind": "cli",
      "title": "ytscribe",
      "summary": {
        "text": "The `ytscribe` command, the CLI for the YouTube transcript library.",
        "refs": [
          "code:cmd/ytscribe/main.go#main",
          "code:cmd/ytscribe/main.go#runYTScribe"
        ]
      },
      "does": [
        {
          "text": "`fetch` fetches and caches one video's transcript, `scrape` walks a channel's videos with a limit and a delay between them, and `import` loads a legacy transcript corpus from a folder.",
          "refs": [
            "code:cmd/ytscribe/main.go#printUsage",
            "code:cmd/ytscribe/main.go#runYTScribe",
            "code:internal/youtube/import.go#ImportLegacy"
          ]
        },
        {
          "text": "Uses the same transcript service and HTTP client as the YouTube plugin's tool, over the same database.",
          "refs": [
            "code:cmd/ytscribe/main.go#newYTScribeService",
            "code:internal/tools/youtube.go#newYouTubeService"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It needs no credentials and reads no `.env` file.",
          "refs": [
            "code:cmd/ytscribe/main.go#main"
          ]
        }
      ],
      "contains": [
        "pkg:cmd/ytscribe",
        "pkg:internal/youtube"
      ]
    },
    {
      "id": "container:workflow-scheduler",
      "level": "container",
      "kind": "process",
      "title": "Workflow scheduler",
      "summary": {
        "text": "The Workflow Runtime's background scheduler inside serve and the REPL: it executes queued Workflow Runs, fires due Workflow Schedules and recovers runs whose process died.",
        "refs": [
          "code:cmd/evie/workflow_runs.go#startWorkflowScheduler",
          "code:internal/workflowruntime/scheduler.go#Runtime.Start"
        ]
      },
      "does": [
        {
          "text": "Polls for runnable work and runs passes in a bounded pool of worker slots (EVIE_WORKFLOW_WORKERS), waking early when the runtime signals new work.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:cmd/evie/workflow_runs.go#workflowSchedulerConfig"
          ]
        },
        {
          "text": "Decides due schedule slots under a single runner lease, with the first tick at start, so every startup applies the Catch-up Policies.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "term:Catch-up Policy"
          ]
        },
        {
          "text": "Recovers runs whose process died once their leases expire, and retries after durable backoffs.",
          "refs": [
            "code:cmd/evie/workflow_runs.go#startWorkflowScheduler"
          ]
        },
        {
          "text": "Reads approved Workflow Definitions from the procedural repository at their pinned commits and keeps run state in the Evie database.",
          "refs": [
            "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition",
            "code:internal/procedural/repo.go#Repo.Source"
          ]
        },
        {
          "text": "Executes capability steps through plugins (such as the finance ledger sync and GitHub Changes), AI steps through OpenRouter, and agent steps through the Kernel subagent supervisor.",
          "refs": [
            "code:cmd/evie/workflow_activation.go#newWorkflowRuntime",
            "code:cmd/evie/workflow_activation.go#workflowModels",
            "code:cmd/evie/workflow_activation.go#workflowWorkers",
            "code:internal/plugins/finance_effects.go#Finance.performSyncLedger"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "AI and agent steps are off by default because they send run data to the model provider: they need EVIE_WORKFLOW_REMOTE_MODELS=on and OPENROUTER_API_KEY, a setting separate from EVIE_REMOTE_MEMORY.",
          "refs": [
            "code:cmd/evie/workflow_activation.go#workflowRemoteModels",
            "code:cmd/evie/workflow_activation.go#workflowModels"
          ]
        },
        {
          "text": "Stopping lets nodes in flight finish and releases leases within five seconds; a pass still inside a node keeps its lease until it expires and the next process recovers the run.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start"
          ]
        },
        {
          "text": "serve and the REPL each start one, and another process, such as a foreground `evie workflows run`, may claim a run first; leases in the Evie database decide who executes it.",
          "refs": [
            "code:internal/workflowruntime/scheduler.go#Runtime.Start",
            "code:cmd/evie/workflow_runs.go"
          ]
        }
      ],
      "terms": [
        "Workflow Run",
        "Workflow Schedule",
        "Catch-up Policy",
        "Workflow Definition",
        "AI Node",
        "Background Run"
      ],
      "contains": [
        "pkg:internal/workflowruntime",
        "pkg:internal/workflow",
        "pkg:internal/procedural",
        "pkg:internal/plugins",
        "pkg:internal/subagents"
      ]
    },
    {
      "id": "container:memory-background-pass",
      "level": "container",
      "kind": "process",
      "title": "Background memory pass",
      "summary": {
        "text": "The always-on background memory pass inside serve and the REPL: it reads owner messages and proposes memory changes for the owner to review.",
        "refs": [
          "code:cmd/evie/memory_background.go#memoryBackgroundHost",
          "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
        ]
      },
      "does": [
        {
          "text": "Resolves its configuration from the environment at startup, and any problem disables it with a reason instead of stopping Evie.",
          "refs": [
            "code:internal/memorybackground/config.go#FromEnv",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost"
          ]
        },
        {
          "text": "Uses a pinned OpenRouter Chat Completions model when remote memory is allowed, otherwise the pinned loopback extractor of EVIE_COMPILER_CONFIG, and reads the remote model's catalog price at start for its spend caps.",
          "refs": [
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:cmd/evie/memory_background.go#memoryBackgroundHost.Start",
            "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
          ]
        },
        {
          "text": "Reads only owner user messages of root conversations, never assistant, tool, web or delegated text.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
          ]
        },
        {
          "text": "Is also every root conversation's compaction flush, so a compaction offers its range to the pass.",
          "refs": [
            "code:cmd/evie/memory_background.go#memoryBackgroundHost",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "Ticks until shutdown under a runner lease in the Evie database; shutdown cancels an in-flight model call and the durable cursor lets the next process redo the batch.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.Run",
            "code:internal/memorybackground/pass.go#Runner.Tick"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It only proposes: it never writes Memory Candidates or accepted memory itself.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#local-inference-boundary"
          ]
        },
        {
          "text": "If the catalog price cannot be read within ten seconds, the pass uses a conservative fallback price.",
          "refs": [
            "code:cmd/evie/memory_background.go#memoryBackgroundHost.Start"
          ]
        },
        {
          "text": "`evie memory-background status` shows the same configuration and durable progress but never starts a pass or contacts a provider.",
          "refs": [
            "code:cmd/evie/memory_background.go#runMemoryBackgroundManagement"
          ]
        }
      ],
      "terms": [
        "Episodic Memory",
        "Semantic Memory",
        "Memory Candidate"
      ],
      "contains": [
        "pkg:internal/memorybackground",
        "pkg:internal/openrouter",
        "pkg:internal/localextractor",
        "pkg:internal/eviedb"
      ]
    },
    {
      "id": "container:compiler-host",
      "level": "container",
      "kind": "process",
      "title": "Memory compiler host",
      "summary": {
        "text": "The optional memory compiler host inside serve and the REPL, started only when EVIE_COMPILER_CONFIG names a pinned local extractor configuration.",
        "refs": [
          "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost",
          "code:cmd/evie/main.go#main"
        ]
      },
      "does": [
        {
          "text": "Reads the configuration file as strict, size-bounded JSON and builds the loopback Ollama extractor for its Compiler Generation.",
          "refs": [
            "code:cmd/evie/compiler_activation.go#readCompilerHostConfiguration",
            "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost"
          ]
        },
        {
          "text": "Runs the Evie database's compiler host, whose separate selection and extraction loops turn retained Episodic Memory into Memory Candidates, so a blocked local request cannot stop new work from being found.",
          "refs": [
            "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost",
            "term:Compiler Generation"
          ]
        },
        {
          "text": "Logs when reconciliation starts failing and when it recovers.",
          "refs": [
            "code:cmd/evie/compiler_host.go#logCompilerReconcileHealth"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "A bad configuration only logs 'memory compiler unavailable'; chat and explicit memory keep working.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost"
          ]
        },
        {
          "text": "Per-conversation compiler activation is retired: the background pass reads every conversation, and `memory-compiler` keeps only status and disable.",
          "refs": [
            "code:cmd/evie/compiler_activation.go#errCompilerActivationRetired"
          ]
        },
        {
          "text": "Shutdown waits at most five seconds for cleanup.",
          "refs": [
            "code:cmd/evie/compiler_host.go#startCompilerHost"
          ]
        }
      ],
      "terms": [
        "Compiler Generation",
        "Memory Candidate",
        "Compilation Coverage",
        "Episodic Memory"
      ],
      "contains": [
        "pkg:internal/eviedb",
        "pkg:internal/localextractor",
        "pkg:internal/memory"
      ]
    },
    {
      "id": "container:retrieval-maintenance",
      "level": "container",
      "kind": "process",
      "title": "Retrieval index maintenance",
      "summary": {
        "text": "A loop inside serve and the REPL that keeps memory's derived retrieval indexes caught up, independently of chat turns.",
        "refs": [
          "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost"
        ]
      },
      "does": [
        {
          "text": "Calls RefreshMemoryIndex in batches of 256 under a five-second deadline: every second when idle, every 100 ms while work is pending, and every five seconds after a failure.",
          "refs": [
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost",
            "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex"
          ]
        },
        {
          "text": "Embeds at most 16 new inputs per call through the local Ollama embedding model when dense retrieval is enabled.",
          "refs": [
            "code:internal/eviedb/retrieval_dense_index.go#Store.refreshDenseIndex",
            "code:internal/localembedding/client.go#BatchSize",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in"
          ]
        },
        {
          "text": "Logs once when the index becomes unavailable and once when it recovers.",
          "refs": [
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "A new or rebuilt index generation cannot serve queries until maintenance has reconciled its retained coverage, so reads report partial coverage meanwhile.",
          "refs": [
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#durable-projection-and-recovery"
          ]
        }
      ],
      "terms": [
        "Automatic Recall",
        "Semantic Memory",
        "Episodic Memory"
      ],
      "contains": [
        "pkg:internal/eviedb",
        "pkg:internal/localembedding"
      ]
    },
    {
      "id": "container:subagent-recovery",
      "level": "container",
      "kind": "process",
      "title": "Subagent recovery",
      "summary": {
        "text": "A loop inside serve and the REPL that reconciles subagent execution records for the process's lifetime; it never starts a child or a model call.",
        "refs": [
          "code:cmd/evie/subagents.go#startSubagentRecovery",
          "code:internal/subagents/recovery.go#Supervisor.RunRecovery"
        ]
      },
      "does": [
        {
          "text": "Settles outcomes this process decided but could not record, then recovers subagent records, every 250 ms while healthy and backing off to ten seconds after failures.",
          "refs": [
            "code:internal/subagents/recovery.go#Supervisor.RunRecovery",
            "code:internal/subagents/recovery.go#recoveryInterval",
            "code:internal/subagents/recovery.go#recoveryMaxBackoff"
          ]
        },
        {
          "text": "Checks merge-gate steps that a crash or lost turn left in progress every ten seconds.",
          "refs": [
            "code:internal/subagents/recovery.go#gateRecoveryInterval",
            "code:internal/subagents/recovery.go#Supervisor.RunRecovery"
          ]
        },
        {
          "text": "Runs whether or not the Subagents plugin is enabled.",
          "refs": [
            "code:internal/subagents/recovery.go#Supervisor.RunRecovery"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Startup also runs one recovery pass before the loop, and an unrecoverable record is logged and retried rather than blocking startup.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "contains": [
        "pkg:internal/subagents",
        "pkg:internal/delegation",
        "pkg:internal/eviedb"
      ]
    },
    {
      "id": "container:change-sync",
      "level": "container",
      "kind": "process",
      "title": "Change sync",
      "summary": {
        "text": "A loop inside `evie serve` that syncs each Workspace's GitHub pull requests every 60 seconds when GITHUB_TOKEN is set.",
        "refs": [
          "code:cmd/evie/main.go#startChangeSync",
          "code:internal/changes/sync.go#Syncer.Run"
        ]
      },
      "does": [
        {
          "text": "Syncs every Workspace with a GitHub Repository at start and then every 60 seconds, refreshing each open Change's pull request state, checks and mergeability.",
          "refs": [
            "code:internal/changes/sync.go#Syncer.Run",
            "code:internal/changes/sync.go#SyncEvery",
            "doc:cmd/evie/docs/active/changes.spec.md#sync"
          ]
        },
        {
          "text": "Hands each sync result to the owner's Merger, which fires armed merges and cleans up after merges done on GitHub.",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/main.go#startChangeSync",
            "code:cmd/evie/changes_push.go#changeMerger"
          ]
        },
        {
          "text": "Still answers the refresh route without a token, but has nothing to sync.",
          "refs": [
            "code:cmd/evie/main.go#startChangeSync"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Only serve starts this loop; the REPL and the CLI start none.",
          "refs": [
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "terms": [
        "Change",
        "Repository",
        "Workspace"
      ],
      "contains": [
        "pkg:internal/changes",
        "pkg:internal/github",
        "pkg:internal/eviedb"
      ]
    }
  ],
  "edges": [
    {
      "from": "person:owner",
      "to": "system:evie",
      "label": "uses the web UI on loopback",
      "via": "http",
      "refs": [
        "code:internal/web/serve.go#listenAddr",
        "code:internal/web/static.go#Server.staticHandler"
      ]
    },
    {
      "from": "person:owner",
      "to": "system:evie",
      "label": "runs evie, todo, finance and ytscribe",
      "via": "process",
      "refs": [
        "code:cmd/evie/main.go#main",
        "code:cmd/todo/main.go#main",
        "code:cmd/finance/main.go#main",
        "code:cmd/ytscribe/main.go#main"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:openrouter",
      "label": "model turns and model metadata",
      "via": "http",
      "refs": [
        "code:internal/openrouter/client.go#Client.ChatStream",
        "code:internal/openrouter/context_profile.go#Client.getMetadata"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:ollama",
      "label": "embeddings and local extraction",
      "via": "http",
      "refs": [
        "code:internal/localembedding/client.go#Client.Embed",
        "code:internal/localextractor/ollama.go#Ollama.call"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:mcp-servers",
      "label": "OAuth sign-in and MCP tool calls",
      "via": "http",
      "refs": [
        "code:internal/mcpconn/oauth.go#discover",
        "code:internal/mcpconn/pool.go#Pool.CallTool"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:github",
      "label": "REST API and git over HTTPS",
      "via": "http",
      "refs": [
        "code:internal/github/client.go#Client.do",
        "code:internal/changes/publish.go#Service.remoteGit"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:plaid",
      "label": "bank linking and transaction sync",
      "via": "http",
      "refs": [
        "code:internal/finance/client.go#plaidClient",
        "code:internal/finance/sync.go#Sync"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:youtube",
      "label": "transcripts and channel listings",
      "via": "http",
      "refs": [
        "code:internal/youtube/client.go#Client.fetchVideo",
        "code:internal/youtube/client.go#Client.listChannel"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:brave-search",
      "label": "web_search queries",
      "via": "http",
      "refs": [
        "code:internal/tools/websearch.go#webSearch"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:web",
      "label": "web_fetch page reads",
      "via": "http",
      "refs": [
        "code:internal/tools/webfetch.go#fetchWebContent"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:codex",
      "label": "app-server account reads and session logs",
      "via": "process",
      "refs": [
        "code:internal/usage/codex.go#ReadCodexAccount",
        "code:internal/usage/local.go#NewLocalIndex"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:launchd",
      "label": "installs and removes cron jobs",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#installJob",
        "code:internal/tools/cron.go#reconcileCronJob"
      ]
    },
    {
      "from": "external:launchd",
      "to": "system:evie",
      "label": "starts evie cron-exec on schedule",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#plistFor",
        "code:cmd/evie/cronexec.go#runCronExec"
      ]
    },
    {
      "from": "system:evie",
      "to": "external:shell",
      "label": "bash, cron commands and terminals",
      "via": "process",
      "refs": [
        "code:internal/tools/bash.go#runBash",
        "code:internal/tools/cron.go#RunScheduled",
        "code:internal/localterminal/session.go#Start"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:web-ui",
      "label": "opens 127.0.0.1:6687 in a browser",
      "via": "http",
      "refs": [
        "code:internal/web/serve.go#listenAddr"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:serve",
      "label": "starts evie serve",
      "via": "process",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:repl",
      "label": "starts evie and chats",
      "via": "process",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:management-cli",
      "label": "runs evie subcommands",
      "via": "process",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:todo-cli",
      "label": "runs todo",
      "via": "process",
      "refs": [
        "code:cmd/todo/main.go#main"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:finance-cli",
      "label": "runs finance",
      "via": "process",
      "refs": [
        "code:cmd/finance/main.go#main"
      ]
    },
    {
      "from": "person:owner",
      "to": "container:ytscribe-cli",
      "label": "runs ytscribe",
      "via": "process",
      "refs": [
        "code:cmd/ytscribe/main.go#main"
      ]
    },
    {
      "from": "container:web-ui",
      "to": "container:serve",
      "label": "loads the app, calls /api, streams chat events",
      "via": "http",
      "refs": [
        "code:internal/web/static.go#Server.staticHandler",
        "code:internal/web/serve.go#Server.Handler",
        "code:internal/web/ui/src/api/stream.ts"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "container:serve",
      "label": "connections and mcp-tools call serve's routes",
      "via": "http",
      "refs": [
        "code:cmd/evie/connections.go#connectionCommands.post"
      ]
    },
    {
      "from": "external:launchd",
      "to": "container:cron-exec",
      "label": "starts on each scheduled fire",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#plistFor",
        "code:cmd/evie/cronexec.go#runCronExec"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:evie-db",
      "label": "reads and writes durable state",
      "via": "sql",
      "refs": [
        "code:cmd/evie/main.go#main",
        "code:internal/eviedb/store.go#NewStore"
      ]
    },
    {
      "from": "container:repl",
      "to": "container:evie-db",
      "label": "reads and writes durable state",
      "via": "sql",
      "refs": [
        "code:cmd/evie/main.go#main",
        "code:internal/eviedb/history.go#Store.BindHistory"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "container:evie-db",
      "label": "inspects and manages state",
      "via": "sql",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:cron-exec",
      "to": "container:evie-db",
      "label": "reads the job, appends job_runs",
      "via": "sql",
      "refs": [
        "code:cmd/evie/cronexec.go#cronExec"
      ]
    },
    {
      "from": "container:todo-cli",
      "to": "container:evie-db",
      "label": "reads and writes Task Trees",
      "via": "sql",
      "refs": [
        "code:cmd/todo/main.go#run"
      ]
    },
    {
      "from": "container:finance-cli",
      "to": "container:finance-db",
      "label": "links, syncs and categorizes",
      "via": "sql",
      "refs": [
        "code:cmd/finance/main.go#main",
        "code:internal/finance/db.go#OpenDBContext"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:finance-db",
      "label": "Spending page and finance tools",
      "via": "sql",
      "refs": [
        "code:internal/finance/spending.go#NewSpendingService",
        "code:internal/tools/finance.go#financeSync"
      ]
    },
    {
      "from": "container:repl",
      "to": "container:finance-db",
      "label": "finance tools",
      "via": "sql",
      "refs": [
        "code:internal/tools/finance.go#financeSync"
      ]
    },
    {
      "from": "container:ytscribe-cli",
      "to": "container:youtube-db",
      "label": "caches transcripts",
      "via": "sql",
      "refs": [
        "code:cmd/ytscribe/main.go#openYTScribeDB"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:youtube-db",
      "label": "transcript and database tools",
      "via": "sql",
      "refs": [
        "code:internal/tools/youtube.go#openYouTubeDB",
        "code:internal/tools/db.go#openTranscriptDB"
      ]
    },
    {
      "from": "container:repl",
      "to": "container:youtube-db",
      "label": "transcript and database tools",
      "via": "sql",
      "refs": [
        "code:internal/tools/youtube.go#openYouTubeDB",
        "code:internal/tools/db.go#openTranscriptDB"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "container:procedural-repo",
      "label": "commits workflow proposals",
      "via": "process",
      "refs": [
        "code:internal/workflowruntime/runtime.go#Runtime.Propose",
        "code:internal/procedural/repo.go#Repo.Commit"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:keychain",
      "label": "MCP credentials via /usr/bin/security",
      "via": "process",
      "refs": [
        "code:internal/mcpconn/keychain.go#RunSecurity",
        "code:cmd/evie/connections.go#newMCPConnectionManager"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:env-file",
      "label": "loads at startup",
      "via": "file",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:repl",
      "to": "container:env-file",
      "label": "loads at startup",
      "via": "file",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "container:env-file",
      "label": "loads at startup",
      "via": "file",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:cron-exec",
      "to": "container:env-file",
      "label": "loads at startup",
      "via": "file",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:workspace-folders",
      "label": "files, diffs and tabs",
      "via": "file",
      "refs": [
        "code:internal/web/local_folder.go#Server.registerFolderRoutes",
        "code:internal/web/workspace_tabs.go#Server.registerWorkspaceTabRoutes"
      ]
    },
    {
      "from": "container:serve",
      "to": "container:workspace-folders",
      "label": "git for Repositories, Changes and worktrees",
      "via": "process",
      "refs": [
        "code:internal/changes/publish.go#Service.remoteGit",
        "code:internal/worktree/worktree.go#git"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "container:workspace-folders",
      "label": "worktrees clean and changes",
      "via": "process",
      "refs": [
        "code:cmd/evie/worktrees.go#runWorktreeManagement",
        "code:cmd/evie/changes.go#runChangeManagement"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "container:evie-db",
      "label": "run state, leases and checkpoints",
      "via": "sql",
      "refs": [
        "code:internal/workflowruntime/scheduler.go#Runtime.Start"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "container:procedural-repo",
      "label": "reads definitions at pinned commits",
      "via": "process",
      "refs": [
        "code:internal/workflowruntime/runs.go#Runtime.approvedDefinition",
        "code:internal/procedural/repo.go#Repo.Source"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "external:openrouter",
      "label": "AI and agent steps when allowed",
      "via": "http",
      "refs": [
        "code:cmd/evie/workflow_activation.go#workflowModels",
        "code:internal/workflowruntime/models.go#OpenRouterModels.Complete",
        "code:cmd/evie/workflow_activation.go#workflowWorkers"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "container:finance-db",
      "label": "finance ledger effects",
      "via": "sql",
      "refs": [
        "code:internal/plugins/finance_effects.go#Finance.performSyncLedger"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "external:plaid",
      "label": "ledger sync effect",
      "via": "http",
      "refs": [
        "code:internal/plugins/finance_effects.go#Finance.performSyncLedger",
        "code:internal/finance/effects.go#SyncLedger"
      ]
    },
    {
      "from": "container:workflow-scheduler",
      "to": "external:github",
      "label": "workflow Changes",
      "via": "http",
      "refs": [
        "code:cmd/evie/changes_push.go#workflowGitHub"
      ]
    },
    {
      "from": "container:memory-background-pass",
      "to": "container:evie-db",
      "label": "cursors, runs and proposals",
      "via": "sql",
      "refs": [
        "code:internal/memorybackground/pass.go#Runner.Tick"
      ]
    },
    {
      "from": "container:memory-background-pass",
      "to": "external:openrouter",
      "label": "remote extraction",
      "via": "http",
      "refs": [
        "code:internal/memorybackground/models.go#OpenRouterModel.Complete"
      ]
    },
    {
      "from": "container:memory-background-pass",
      "to": "external:ollama",
      "label": "local extraction via /api/chat",
      "via": "http",
      "refs": [
        "code:internal/memorybackground/models.go#LocalModel.Complete",
        "code:internal/localextractor/ollama.go#Ollama.Chat"
      ]
    },
    {
      "from": "container:compiler-host",
      "to": "container:evie-db",
      "label": "selects work, records candidates",
      "via": "sql",
      "refs": [
        "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost"
      ]
    },
    {
      "from": "container:compiler-host",
      "to": "external:ollama",
      "label": "extraction via /api/generate",
      "via": "http",
      "refs": [
        "code:internal/localextractor/ollama.go#Ollama.Extract"
      ]
    },
    {
      "from": "container:retrieval-maintenance",
      "to": "container:evie-db",
      "label": "refreshes retrieval indexes",
      "via": "sql",
      "refs": [
        "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex"
      ]
    },
    {
      "from": "container:retrieval-maintenance",
      "to": "external:ollama",
      "label": "embeds new index inputs",
      "via": "http",
      "refs": [
        "code:internal/eviedb/retrieval_dense_index.go#Store.refreshDenseIndex",
        "code:internal/localembedding/client.go#Client.Embed"
      ]
    },
    {
      "from": "container:subagent-recovery",
      "to": "container:evie-db",
      "label": "reconciles subagent records",
      "via": "sql",
      "refs": [
        "code:internal/subagents/recovery.go#Supervisor.RunRecovery"
      ]
    },
    {
      "from": "container:change-sync",
      "to": "external:github",
      "label": "pull request sync",
      "via": "http",
      "refs": [
        "code:internal/changes/sync.go#Syncer.Run"
      ]
    },
    {
      "from": "container:change-sync",
      "to": "container:evie-db",
      "label": "records pull request state",
      "via": "sql",
      "refs": [
        "code:internal/changes/sync.go#Syncer.syncAll",
        "code:cmd/evie/main.go#startChangeSync"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:openrouter",
      "label": "chat turns and model catalog",
      "via": "http",
      "refs": [
        "code:cmd/evie/main.go#main",
        "code:cmd/evie/web_context_sessions.go#webContextSessionController.ListChatModels"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:openrouter",
      "label": "chat turns",
      "via": "http",
      "refs": [
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "external:openrouter",
      "label": "foreground workflow model steps",
      "via": "http",
      "refs": [
        "code:cmd/evie/workflow_activation.go#workflowModels",
        "code:cmd/evie/workflow_runs.go"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:ollama",
      "label": "embeds recall queries",
      "via": "http",
      "refs": [
        "code:internal/eviedb/retrieval_dense_query.go#prepareDenseQuery",
        "code:internal/eviedb/retrieval_dense.go#denseQueryContext"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:ollama",
      "label": "embeds recall queries",
      "via": "http",
      "refs": [
        "code:internal/eviedb/retrieval_dense_query.go#prepareDenseQuery"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:mcp-servers",
      "label": "sign-in and tool calls",
      "via": "http",
      "refs": [
        "code:internal/mcpconn/manager.go#Manager.BeginConnect",
        "code:internal/mcpconn/pool.go#Pool.connect"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:github",
      "label": "Repository status, pushes and merges",
      "via": "http",
      "refs": [
        "code:cmd/evie/main.go#repositoryReader",
        "code:cmd/evie/changes_push.go#changePusher",
        "code:cmd/evie/changes_push.go#changeMerger"
      ]
    },
    {
      "from": "container:management-cli",
      "to": "external:github",
      "label": "evie changes push and merge",
      "via": "http",
      "refs": [
        "code:cmd/evie/changes.go#runChangeManagement",
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:plaid",
      "label": "Spending refresh, bank linking, finance tools",
      "via": "http",
      "refs": [
        "code:internal/finance/spending.go#NewSpendingService",
        "code:internal/finance/spending_account_link.go#SpendingService.StartAccountLink",
        "code:internal/tools/finance.go#financeSync"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:plaid",
      "label": "finance tools",
      "via": "http",
      "refs": [
        "code:internal/tools/finance.go#financeSync"
      ]
    },
    {
      "from": "container:finance-cli",
      "to": "external:plaid",
      "label": "link and sync",
      "via": "http",
      "refs": [
        "code:internal/finance/link.go#Link",
        "code:internal/finance/sync.go#Sync"
      ]
    },
    {
      "from": "container:ytscribe-cli",
      "to": "external:youtube",
      "label": "fetch and scrape",
      "via": "http",
      "refs": [
        "code:cmd/ytscribe/main.go#newYTScribeService",
        "code:internal/youtube/client.go#NewClient"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:youtube",
      "label": "youtube_transcript tool",
      "via": "http",
      "refs": [
        "code:internal/tools/youtube.go#newYouTubeService"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:youtube",
      "label": "youtube_transcript tool",
      "via": "http",
      "refs": [
        "code:internal/tools/youtube.go#newYouTubeService"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:brave-search",
      "label": "web_search tool",
      "via": "http",
      "refs": [
        "code:internal/tools/websearch.go#webSearch"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:brave-search",
      "label": "web_search tool",
      "via": "http",
      "refs": [
        "code:internal/tools/websearch.go#webSearch"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:web",
      "label": "web_fetch tool",
      "via": "http",
      "refs": [
        "code:internal/tools/webfetch.go#webFetch"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:web",
      "label": "web_fetch tool",
      "via": "http",
      "refs": [
        "code:internal/tools/webfetch.go#webFetch"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:codex",
      "label": "Data > Usage reads",
      "via": "process",
      "refs": [
        "code:internal/usage/service.go#NewService",
        "code:internal/usage/codex.go#ReadCodexAccount"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:launchd",
      "label": "cron tools",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#installJob"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:launchd",
      "label": "cron tools",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#installJob"
      ]
    },
    {
      "from": "container:serve",
      "to": "external:shell",
      "label": "bash tool and web terminal",
      "via": "process",
      "refs": [
        "code:internal/tools/bash.go#runBash",
        "code:internal/localterminal/session.go#Start"
      ]
    },
    {
      "from": "container:repl",
      "to": "external:shell",
      "label": "bash tool",
      "via": "process",
      "refs": [
        "code:internal/tools/bash.go#runBash"
      ]
    },
    {
      "from": "container:cron-exec",
      "to": "external:shell",
      "label": "runs the job's command",
      "via": "process",
      "refs": [
        "code:internal/tools/cron.go#RunScheduled"
      ]
    }
  ],
  "flows": []
}
);
