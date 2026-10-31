evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "intelligence",
  "nodes": [
    {
      "id": "pkg:internal/openrouter",
      "level": "component",
      "kind": "package",
      "title": "openrouter",
      "group": "intelligence",
      "summary": {
        "text": "A minimal client for OpenRouter's Chat Completions and Responses APIs that owns request encoding, streaming, response normalization and model metadata, and depends on nothing else in Evie.",
        "refs": [
          "code:internal/openrouter/schema.go#Client",
          "code:internal/openrouter/client.go"
        ]
      },
      "does": [
        {
          "text": "NewClient builds the only kind of Client, rejecting an empty API key so a misconfigured environment fails at startup, and points it at `https://openrouter.ai/api/v1`.",
          "refs": [
            "code:internal/openrouter/client.go#NewClient"
          ]
        },
        {
          "text": "ChatStream sends one streaming request and assembles the reply: content and reasoning fragments go to live callbacks as they arrive, tool-call fragments are joined by their index, and a Chat Completions stream must end with `[DONE]`.",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream",
            "code:internal/openrouter/client.go#StreamHandlers"
          ]
        },
        {
          "text": "Carries the Working Memory projection the agent assembles for each model call, as the request's messages, tools and output limit.",
          "refs": [
            "code:internal/agent/context.go#ContextComposer.projectAtStart",
            "code:internal/openrouter/schema.go#ChatRequest",
            "term:Working Memory"
          ]
        },
        {
          "text": "UsesResponses routes exactly one verified model, `openai/gpt-6-astra`, to the Responses API, and every other model keeps Chat Completions rather than guessing its capabilities.",
          "refs": [
            "code:internal/openrouter/responses_request.go#UsesResponses",
            "code:internal/openrouter/responses_request.go#AstraModel"
          ]
        },
        {
          "text": "For Responses it encodes history as input items with `store:false`, low reasoning effort by default, encrypted reasoning included and `require_parameters` set, and checks that streamed function calls agree with the final completed response before returning.",
          "refs": [
            "code:internal/openrouter/responses_request.go#encodeResponsesRequest",
            "code:internal/openrouter/responses.go#Client.responses",
            "doc:cmd/evie/docs/active/gpt-6-astra.spec.md#acceptance"
          ]
        },
        {
          "text": "PrepareRequest freezes the exact Responses wire bytes and RequestBytes returns them, so context admission, snapshots and dispatch all measure and send the same body.",
          "refs": [
            "code:internal/openrouter/responses_request.go#PrepareRequest",
            "code:internal/openrouter/responses_request.go#RequestBytes",
            "doc:cmd/evie/docs/active/gpt-6-astra.spec.md#acceptance"
          ]
        },
        {
          "text": "ResolveContextProfile turns a model into validated token limits: it reads the model and its endpoints from OpenRouter, takes the smallest window among eligible active routes, and applies the working ceiling, the output reserve and a 4,096-token estimation margin.",
          "refs": [
            "code:internal/openrouter/context_profile.go#Client.ResolveContextProfile",
            "code:internal/openrouter/context_profile.go#routeSafeWindow",
            "code:internal/openrouter/context_profile.go#contextEstimationMargin",
            "code:internal/openrouter/context_profile.go#newContextProfile"
          ]
        },
        {
          "text": "ResolveChatContextProfile does the same for a chat's selected model, but lets the default working ceiling shrink to that model's window unless EVIE_CONTEXT_WORKING_TOKENS is set.",
          "refs": [
            "code:internal/openrouter/models.go#Client.ResolveChatContextProfile",
            "code:internal/openrouter/context_profile.go#Client.resolveContextProfile"
          ]
        },
        {
          "text": "Reuses a discovered profile for ten minutes per model and configuration, so opening a chat on a recently used model makes no request; failed lookups and fallbacks are never cached.",
          "refs": [
            "code:internal/openrouter/context_profile.go#profileReuse",
            "code:internal/openrouter/context_profile.go#profileCache",
            "doc:cmd/evie/docs/active/multi-chat.spec.md#stage-1--opening-a-chat-without-waiting-on-openrouter"
          ]
        },
        {
          "text": "ListChatModels returns the catalog models a chat can actually use (text in and out, tool support, an output limit that fits Evie's reply reserve, no variants or routers), sorted by author and name.",
          "refs": [
            "code:internal/openrouter/models.go#Client.ListChatModels"
          ]
        },
        {
          "text": "ModelPricing reads one model's per-token prompt and completion prices from the catalog, which the background memory pass and workflow model steps use for their spend caps.",
          "refs": [
            "code:internal/openrouter/models.go#Client.ModelPricing",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:internal/workflowruntime/models.go#OpenRouterModels.Pricing"
          ]
        },
        {
          "text": "Serves workflow AI steps through the Workflow Runtime's adapter, which sends exactly two messages, asks for a JSON object and sends no tools, and agent steps through the subagent supervisor configured with the same kind of client.",
          "refs": [
            "code:internal/workflowruntime/models.go#OpenRouterModels.Complete",
            "code:cmd/evie/workflow_activation.go#workflowModels",
            "code:cmd/evie/workflow_activation.go#workflowWorkers",
            "term:AI Node"
          ]
        },
        {
          "text": "Normalizes token usage from both protocols into one TokenUsage, keeping a missing counter distinct from a reported zero.",
          "refs": [
            "code:internal/openrouter/usage.go#parseUsage",
            "code:internal/openrouter/schema.go#TokenUsage"
          ]
        },
        {
          "text": "Encodes prompt-cache breakpoints as `cache_control` content parts for models that OpenRouter caches only at explicit markers, which UsesExplicitCacheBreakpoints names as Anthropic and Qwen models.",
          "refs": [
            "code:internal/openrouter/cache_control.go#UsesExplicitCacheBreakpoints",
            "code:internal/openrouter/cache_control.go#encodeChatRequest",
            "code:internal/agent/context.go#ContextComposer.projectAtStart"
          ]
        },
        {
          "text": "WithWorkerLimits and WithOutputLimit derive narrower profiles for subagent workers and bounded calls, never raising the parent's limits.",
          "refs": [
            "code:internal/openrouter/worker_context.go#ContextProfile.WithWorkerLimits",
            "code:internal/openrouter/context_profile.go#ContextProfile.WithOutputLimit"
          ]
        }
      ],
      "owns": [
        {
          "text": "No durable data: each Client holds only its API key, its HTTP client and an in-memory cache of context profiles.",
          "refs": [
            "code:internal/openrouter/schema.go#Client",
            "code:internal/openrouter/context_profile.go#profileCache"
          ]
        },
        {
          "text": "The shared conversation wire types that the agent and its callers exchange: Message, ToolCall, Tool, ChatRequest, ChatResponse and TokenUsage.",
          "refs": [
            "code:internal/openrouter/schema.go#Message",
            "code:internal/openrouter/schema.go#ChatRequest",
            "code:internal/openrouter/schema.go#ChatResponse"
          ]
        },
        {
          "text": "The model ids it knows by name: the built-in Kimi model and the default DeepSeek model, which have checked-in fallback windows, and Astra, which uses Responses.",
          "refs": [
            "code:internal/openrouter/context_profile.go#BuiltinModel",
            "code:internal/openrouter/context_profile.go#builtinContextFallback",
            "code:internal/openrouter/responses_request.go#AstraModel"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Client methods: ChatStream (used by every production caller), Chat (non-streaming, called only by tests), ResolveContextProfile, ResolveChatContextProfile, ListChatModels and ModelPricing.",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream",
            "code:internal/openrouter/client.go#Client.Chat",
            "code:internal/openrouter/context_profile.go#Client.ResolveContextProfile",
            "code:internal/openrouter/models.go#Client.ResolveChatContextProfile",
            "code:internal/openrouter/models.go#Client.ListChatModels",
            "code:internal/openrouter/models.go#Client.ModelPricing"
          ]
        },
        {
          "text": "Consumers own the narrow interfaces they need over it: agent.Client, memorybackground.ChatClient and workflowruntime.OpenRouterClient.",
          "refs": [
            "code:internal/agent/agent.go#Client",
            "code:internal/memorybackground/models.go#ChatClient",
            "code:internal/workflowruntime/models.go#OpenRouterClient"
          ]
        },
        {
          "text": "StreamError is the failure contract: a stable Kind (provider_error or provider_response_invalid), the HTTP status, a clamped Retry-After, NoResponse for failures before any status, and ContextLengthExceeded.",
          "refs": [
            "code:internal/openrouter/client.go#StreamError"
          ]
        },
        {
          "text": "PrepareRequest, RequestBytes, ResponseRequestPartSizes and UsesResponses let callers account for the exact bytes they will send.",
          "refs": [
            "code:internal/openrouter/responses_request.go#PrepareRequest",
            "code:internal/openrouter/responses_request.go#RequestBytes",
            "code:internal/openrouter/responses_request.go#ResponseRequestPartSizes",
            "code:internal/openrouter/responses_request.go#UsesResponses"
          ]
        },
        {
          "text": "It reads three settings itself: EVIE_CONTEXT_WINDOW_TOKENS (an explicit hard window that skips discovery), EVIE_CONTEXT_WORKING_TOKENS (default 262,144) and EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS (default 16,384).",
          "refs": [
            "code:internal/openrouter/context_profile.go#loadContextProfileConfig",
            "code:internal/openrouter/context_profile.go#defaultContextWorkingTokens",
            "code:internal/openrouter/context_profile.go#defaultContextOutputTokens"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "A stream that goes five minutes without any line, keepalive comments included, is cancelled by an idle watchdog; there is no total timeout, so long healthy streams run as long as they need.",
          "refs": [
            "code:internal/openrouter/client.go#streamWatchdog",
            "code:internal/openrouter/client.go#defaultStreamIdleTimeout",
            "code:internal/openrouter/client.go#newProviderHTTPClient"
          ]
        },
        {
          "text": "Only a failure before any HTTP status that is neither a timeout nor a cancellation is marked NoResponse, because only that proves nothing was generated and the call may be retried.",
          "refs": [
            "code:internal/openrouter/client.go#transportFailure"
          ]
        },
        {
          "text": "Provider error bodies are untrusted and may echo credentials, so at most 64 KiB is drained and only scanned for a context-length marker, and the body is never kept.",
          "refs": [
            "code:internal/openrouter/client.go#httpStatusFailure",
            "code:internal/openrouter/client.go#contextLengthRejection"
          ]
        },
        {
          "text": "A Chat Completions stream that stops at `finish_reason: length` is an error, so truncated text or tool arguments never look complete, and `finish_reason: error` mid-stream fails too.",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "text": "When metadata discovery fails, only `moonshotai/kimi-k3` and `deepseek/deepseek-v4.1-flash` fall back to a checked-in 262,144-token window, with a startup warning; every other model, Astra included, fails.",
          "refs": [
            "code:internal/openrouter/context_profile.go#builtinContextFallback",
            "code:cmd/evie/main.go#contextFallbackWarning",
            "doc:cmd/evie/docs/active/gpt-6-astra.spec.md#acceptance"
          ]
        },
        {
          "text": "Astra rejects a temperature and accepts only low, medium, high, xhigh or max reasoning effort, and the background memory pass and workflow model steps refuse Responses models outright.",
          "refs": [
            "code:internal/openrouter/responses_request.go#encodeResponsesRequest",
            "code:internal/memorybackground/models.go#NewOpenRouterModel",
            "code:internal/workflowruntime/models.go#OpenRouterModels.Complete"
          ]
        },
        {
          "text": "Responses output items, including encrypted reasoning, live on Message.ResponseItems only for the active tool loop and are never serialized as Chat messages or evidence.",
          "refs": [
            "code:internal/openrouter/schema.go#Message",
            "doc:cmd/evie/docs/active/gpt-6-astra.spec.md#acceptance"
          ]
        },
        {
          "text": "Client.Chat takes no context, so it cannot be cancelled; no production code calls it.",
          "refs": [
            "code:internal/openrouter/client.go#Client.Chat"
          ]
        },
        {
          "text": "The HTTP client clones Go's default transport with bounded dial, TLS and header timeouts, unless an instrumented process replaced that transport, in which case it routes through the replacement unchanged.",
          "refs": [
            "code:internal/openrouter/client.go#newProviderHTTPClient"
          ]
        },
        {
          "text": "Responses bodies and streams are capped at 16 MiB and must be valid UTF-8; model metadata is capped at 1 MiB per lookup and 8 MiB for the catalog.",
          "refs": [
            "code:internal/openrouter/responses.go#maxResponsesBody",
            "code:internal/openrouter/context_profile.go#maxContextMetadataBody",
            "code:internal/openrouter/models.go#Client.ListChatModels"
          ]
        },
        {
          "text": "A message with blank text cannot carry a cache breakpoint, because providers reject empty text blocks.",
          "refs": [
            "code:internal/openrouter/cache_control.go#cacheMarked"
          ]
        }
      ],
      "terms": [
        "Working Memory",
        "AI Node"
      ]
    },
    {
      "id": "pkg:internal/localembedding",
      "level": "component",
      "kind": "package",
      "title": "localembedding",
      "group": "intelligence",
      "summary": {
        "text": "Implements the one selected local-only embedding protocol: Ollama's pinned `all-minilm:22m` model at a loopback or Unix-socket endpoint, verified by manifest digest, producing normalized 384-value vectors for memory retrieval.",
        "refs": [
          "code:internal/localembedding/client.go#Client",
          "code:internal/localembedding/client.go#Model",
          "code:internal/localembedding/client.go#Dimensions"
        ]
      },
      "does": [
        {
          "text": "New accepts only plain loopback HTTP with a literal IP address (no host name, credentials, query, fragment or path beyond `/`) or an absolute `unix:` socket path, and builds a transport that dials only that address, uses no proxy and refuses redirects.",
          "refs": [
            "code:internal/localembedding/client.go#New"
          ]
        },
        {
          "text": "Embed sends one batch of 1 to 16 non-empty UTF-8 inputs of at most 1,024 bytes each to `/api/embed`, with truncation off, a 30-second keep-alive and four threads.",
          "refs": [
            "code:internal/localembedding/client.go#Client.Embed",
            "code:internal/localembedding/client.go#BatchSize",
            "code:internal/localembedding/client.go#InputBytes"
          ]
        },
        {
          "text": "It checks the model's manifest digest at `/api/tags` before and after inference, so a re-tagged model cannot silently replace the selected generation mid-batch.",
          "refs": [
            "code:internal/localembedding/client.go#Client.verifyManifest",
            "code:internal/localembedding/client.go#ManifestSHA256",
            "code:internal/localembedding/client.go#Client.Embed"
          ]
        },
        {
          "text": "It validates the reply (same model, one vector per input, 384 finite values, non-zero magnitude) and returns unit-length float32 vectors.",
          "refs": [
            "code:internal/localembedding/client.go#Client.Embed"
          ]
        },
        {
          "text": "Every call is bounded: one ten-second deadline covers both manifest reads and inference, and a response may be at most 1 MiB.",
          "refs": [
            "code:internal/localembedding/client.go#Client.Embed",
            "code:internal/localembedding/client.go#ResponseBytes",
            "code:internal/localembedding/client.go#Client.request"
          ]
        }
      ],
      "owns": [
        {
          "text": "No data of its own, only the pinned model identity (name, manifest and layer digests, dimensions and bounds) as constants; vectors and their generations live in the Evie database.",
          "refs": [
            "code:internal/localembedding/client.go#Model",
            "code:internal/localembedding/client.go#ModelLayerSHA256",
            "code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "New(endpoint), Client.Embed(ctx, inputs), Client.Close() and ErrDisabled, plus constants such as Dimensions and BatchSize that the Evie database's dense index reads.",
          "refs": [
            "code:internal/localembedding/client.go#New",
            "code:internal/localembedding/client.go#Client.Embed",
            "code:internal/localembedding/client.go#Client.Close",
            "code:internal/localembedding/client.go#ErrDisabled",
            "code:internal/eviedb/retrieval_dense.go#decodeDenseVector"
          ]
        },
        {
          "text": "The endpoint comes from EVIE_MEMORY_EMBEDDING_ENDPOINT, which the Evie database reads only when EVIE_REMOTE_MEMORY=on.",
          "refs": [
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in"
          ]
        },
        {
          "text": "It is used at chat time to embed one recall query alongside lexical search, and by retrieval maintenance to embed new Claim and conversation parts.",
          "refs": [
            "code:internal/eviedb/retrieval_dense_query.go#prepareDenseQuery",
            "code:internal/eviedb/retrieval_dense_index.go#Store.refreshDenseIndex",
            "code:internal/eviedb/retrieval_dense_conversation.go#Store.refreshDenseEvents"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Every request re-checks that EVIE_REMOTE_MEMORY=on and returns ErrDisabled otherwise, so a strictly local feature is gated by a switch named for remote memory.",
          "refs": [
            "code:internal/localembedding/client.go#Client.request",
            "code:internal/eviedb/retrieval_dense_index.go#denseEndpoint",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in"
          ]
        },
        {
          "text": "Similarity is untrusted candidate input: a dense hit only proposes, and SQLite still decides acceptance, scope, lifecycle and the text that may leave the Kernel.",
          "refs": [
            "code:internal/localembedding/client.go",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#query-and-provenance-contract"
          ]
        },
        {
          "text": "At chat time the embedding call gets at most half of the 500 ms retrieval deadline (or of the caller's remaining time), far less than the client's own ten-second ceiling.",
          "refs": [
            "code:internal/eviedb/retrieval_dense.go#denseQueryContext",
            "code:internal/eviedb/retrieval.go#retrievalDeadline",
            "code:internal/localembedding/client.go#Client.Embed"
          ]
        },
        {
          "text": "The selected manifest runs with `num_ctx=256`, so the index embeds 240-byte chunks with a 48-byte overlap after longer inputs were measured to fail with HTTP 400.",
          "refs": [
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in"
          ]
        },
        {
          "text": "There is no remote fallback and no model download: a missing model or a changed digest fails embedding, and retrieval reports partial or unavailable coverage.",
          "refs": [
            "code:internal/localembedding/client.go#Client.verifyManifest",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#configuration-and-opt-in",
            "doc:cmd/evie/docs/active/memory-stage-5-dense.md#query-and-provenance-contract"
          ]
        },
        {
          "text": "memory.spec.md calls for `Embedder` and `VectorIndex` interfaces, but in code the Evie database calls this concrete Client directly and keeps vectors in its own SQLite tables, with no interface between them.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#vector-retrieval",
            "code:internal/eviedb/retrieval_dense_index.go#Store.refreshDenseIndex",
            "code:internal/localembedding/client.go#Client"
          ]
        }
      ],
      "terms": [
        "Automatic Recall",
        "Semantic Memory",
        "Claim",
        "Episodic Memory"
      ]
    },
    {
      "id": "pkg:internal/localextractor",
      "level": "component",
      "kind": "package",
      "title": "localextractor",
      "group": "intelligence",
      "summary": {
        "text": "Implements the explicitly configured, pinned local compiler transport: a client for an Ollama server on a literal loopback address that verifies the pinned runtime and model before sending any evidence.",
        "refs": [
          "code:internal/localextractor/ollama.go#Ollama",
          "code:internal/localextractor/activation.go#Ollama.VerifyCompilerConfiguration"
        ]
      },
      "does": [
        {
          "text": "New takes a Config (an endpoint plus a memory.CompilerGeneration), computes the generation's identity, and accepts only `http://` with a literal loopback IP and an explicit port, with no credentials, path, query or fragment.",
          "refs": [
            "code:internal/localextractor/ollama.go#New",
            "code:internal/localextractor/ollama.go#Config"
          ]
        },
        {
          "text": "Its transport uses no proxy and no keep-alives, a five-second dial timeout and a 120-second response-header timeout, and refuses redirects.",
          "refs": [
            "code:internal/localextractor/ollama.go#New"
          ]
        },
        {
          "text": "VerifyCompilerConfiguration checks within five seconds, using metadata calls only, that the runtime version, the single model artifact digest, the prompt template, an empty system prompt, the quantization and the tokenizer hash all match the pinned generation.",
          "refs": [
            "code:internal/localextractor/activation.go#Ollama.VerifyCompilerConfiguration"
          ]
        },
        {
          "text": "Extract, for the memory compiler, checks the generation identity and input budget, verifies the configuration, renders the pinned template with the request and extraction schema, and calls `/api/generate` raw and non-streaming with the schema as the output format and the generation's seed, temperature, context and output limits.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract",
            "code:internal/localextractor/ollama.go#render"
          ]
        },
        {
          "text": "Chat, for the local background memory pass, verifies the configuration and sends a system and a user message to `/api/chat` in JSON format, returning the reply with its prompt and output token counts.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Chat"
          ]
        },
        {
          "text": "It rejects any reply that is not finished with `done_reason: stop` from the pinned model, so a truncated or wrong-model answer never counts as output.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract",
            "code:internal/localextractor/ollama.go#Ollama.Chat",
            "code:internal/localextractor/ollama.go#ErrRuntimeOutputInvalid"
          ]
        }
      ],
      "owns": [
        {
          "text": "No durable data: only the endpoint, a server identity made of the endpoint, runtime version and manifest digest, and the generation id; Memory Candidates and coverage live in the Evie database.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama",
            "code:internal/localextractor/ollama.go#Ollama.ServerIdentity",
            "code:internal/eviedb/compiler_work.go#CompilerExtraction"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Ollama satisfies two consumer-owned interfaces: eviedb.CompilerExtractor (ServerIdentity and Extract) and memorybackground.LocalCompleter (Chat).",
          "refs": [
            "code:internal/eviedb/compiler_work.go#CompilerExtractor",
            "code:internal/memorybackground/models.go#LocalCompleter"
          ]
        },
        {
          "text": "It is built from the JSON file named by EVIE_COMPILER_CONFIG, for the compiler host and the local background pass, or from `evie memory-compile --config` for one owner-selected range.",
          "refs": [
            "code:cmd/evie/compiler_activation.go#readCompilerHostConfiguration",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:cmd/evie/compiler_management.go#runCompilerManagement"
          ]
        },
        {
          "text": "It returns eviedb.ErrCompilerConfiguration for an identity or pin mismatch and eviedb.ErrCompilerEndpointUnavailable for transport failures, 5xx and 429, so the Kernel can tell a misconfiguration from an unavailable endpoint.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.call",
            "code:internal/localextractor/activation.go#Ollama.VerifyCompilerConfiguration"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "memory.spec.md describes the compiler endpoint as a loopback or Unix-socket OpenAI-compatible server such as Ollama, LM Studio or llama.cpp, but the code speaks Ollama's own API (`/api/generate`, `/api/chat`, `/api/show`, `/api/tags`, `/api/version`) and accepts loopback HTTP only, not Unix sockets.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#local-inference-boundary",
            "doc:cmd/evie/docs/active/memory.decisions.md",
            "code:internal/localextractor/ollama.go#New",
            "code:internal/localextractor/ollama.go#Ollama.call"
          ]
        },
        {
          "text": "There is no default model or installed configuration: without EVIE_COMPILER_CONFIG nothing runs locally, and explicit memory operations keep working.",
          "refs": [
            "code:internal/localextractor/ollama.go#Config",
            "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost",
            "doc:cmd/evie/docs/active/memory.spec.md#local-inference-boundary"
          ]
        },
        {
          "text": "Responses are size-bounded, with separate limits for metadata and generation, and strictly framed: duplicate keys are rejected even though the runtime may add fields.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.call"
          ]
        },
        {
          "text": "With a pinned seed and temperature a truncated reply repeats on retry, so the background pass counts ErrRuntimeOutputInvalid as malformed and eventually skips the batch.",
          "refs": [
            "code:internal/localextractor/ollama.go#ErrRuntimeOutputInvalid",
            "code:internal/memorybackground/models.go#LocalModel.Complete"
          ]
        },
        {
          "text": "The rendered prompt must fit the generation's proven context: rendered bytes times tokens per byte, plus template overhead and output tokens, may not exceed the context size.",
          "refs": [
            "code:internal/localextractor/ollama.go#render"
          ]
        },
        {
          "text": "Extract reports `not_dispatched` release evidence when it refuses before calling the runtime, and `completed` once the runtime has answered.",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract"
          ]
        }
      ],
      "terms": [
        "Compiler Generation",
        "Memory Candidate",
        "Episodic Memory"
      ]
    }
  ],
  "edges": [],
  "flows": []
}
);
