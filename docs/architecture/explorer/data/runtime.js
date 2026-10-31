evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "runtime",
  "nodes": [
    {
      "id": "pkg:internal/agent",
      "level": "component",
      "kind": "package",
      "title": "agent",
      "group": "runtime",
      "summary": {
        "text": "Runs one conversation Session's turn loop: it streams a model response, stores it durably, runs the requested tools through the session's pinned Toolset, and repeats until the model answers without tools.",
        "refs": [
          "code:internal/agent/agent.go#Session.Send",
          "code:internal/agent/turn.go#Session.runOwnedTurn"
        ]
      },
      "does": [
        {
          "text": "Send runs one turn: it refuses a second concurrent turn on the same Session with ErrBusy, acquires the session's durable turn lease, and stores the owner's text as the turn's root user_message event before any model work.",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/agent/agent.go#ErrBusy",
            "code:internal/agent/ownership.go#TurnOwnership"
          ]
        },
        {
          "text": "Each iteration reloads the durable history, composes the request (system prompt, working-folder note, repository instructions, standing and recalled memory, the conversation, and the tool schemas), and stores the request's shape as a context_snapshot event before calling the provider.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/context.go#ContextComposer.Compose",
            "code:internal/agent/context.go#ContextComposeInput"
          ]
        },
        {
          "text": "Streams the provider response through the Events callbacks and retries a transient failure (no response, or HTTP 429, 502, 503 or 504) at most twice, only while nothing has streamed yet, with backoff capped at 30 seconds.",
          "refs": [
            "code:internal/agent/turn.go#Session.callProvider",
            "code:internal/agent/turn.go#providerRetryDelay",
            "code:internal/agent/turn.go#maxProviderRetries"
          ]
        },
        {
          "text": "Stores each assistant message as an assistant_message event; a message without tool calls is the turn's durable success, otherwise every call is stored as a tool_intent, dispatched through the Toolset, and closed with a tool_succeeded, tool_failed or tool_cancelled event before the next model request.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#toolIntentInput",
            "code:internal/agent/agent.go#toolOutcomeInput"
          ]
        },
        {
          "text": "Bounds a turn at EVIE_TURN_STEP_LIMIT model responses (100 by default): the last permitted request goes out with tool_choice none and a harness notice asking for an answer, and a response that still asks for tools fails the turn with ErrStepLimitExceeded.",
          "refs": [
            "code:internal/agent/step_limit.go#DefaultTurnStepLimit",
            "code:internal/agent/step_limit.go#stepLimitNote",
            "code:internal/agent/step_limit.go#ErrStepLimitExceeded",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "Compacts older context automatically when the composed request would exceed its working budget, and gives a provider context-length rejection exactly one compact-and-retry per turn.",
          "refs": [
            "code:internal/agent/automatic_compaction.go#selectAutomaticCompaction",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "Runs automatic memory recall once, on the turn's first iteration, unless the session turned it off, and reads standing memory for every request so an approval earlier in the turn applies to the next request.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#WithAutomaticMemoryRecall",
            "code:internal/agent/standing_memory.go#Session.standingMemory"
          ]
        },
        {
          "text": "Puts the harness-owned InvocationContext and the Task mutation attribution on the context of every tool call, so a tool gets the scope, lease, source event and working Directory without the model choosing them.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/tools/registry.go#InvocationContext",
            "code:internal/task/task.go#WithMutationAttribution"
          ]
        },
        {
          "text": "When a turn fails for a reason with durable meaning (provider error or invalid response, caller cancellation or deadline, context overflow, repository-instruction failure, step limit) it stores a turn_failed or turn_interrupted event naming the stage, and tells the frontend when streamed text was discarded.",
          "refs": [
            "code:internal/agent/turn.go#causeHasDurableTerminal",
            "code:internal/agent/turn.go#Session.appendTerminal",
            "code:internal/agent/agent.go#Session.Send"
          ]
        },
        {
          "text": "Builds delegated worker sessions with NewDelegatedWithToolset: the worker's pinned instructions replace the primary system prompt, and automatic recall and model-directed memory reads are off.",
          "refs": [
            "code:internal/agent/agent.go#NewDelegatedWithToolset",
            "code:internal/agent/context.go#ContextComposeInput.WorkerInstructions"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the order and shape of a turn's durable events: user_message, context_snapshot, assistant_message, tool_intent, approval, the tool outcome and a terminal turn event, linked by parent event IDs.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#assistantEventInput",
            "code:internal/agent/agent.go#approvalEventInput",
            "code:internal/agent/agent.go#toolOutcomeInput"
          ]
        },
        {
          "text": "Holds no tables of its own: every event goes through the History interface, which eviedb's SessionHistory implements against the events table, fenced by the turn lease.",
          "refs": [
            "code:internal/agent/history.go#History",
            "code:internal/eviedb/history.go#SessionHistory.Append",
            "code:internal/eviedb/events.go#Store.AppendEventWithLease"
          ]
        },
        {
          "text": "Keeps per-session state in memory: the pinned Toolset, the context profile and reasoning setting, the session's working Directory, and opaque Responses continuation items that live only for the current turn.",
          "refs": [
            "code:internal/agent/agent.go#Session",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "Bounds every tool result at 100 KiB when it is stored, keeping its head and tail plus the original size and SHA-256 when it must cut.",
          "refs": [
            "code:internal/agent/tool_results.go#admitToolResult",
            "code:internal/agent/tool_results.go#toolResultAdmissionBytes"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Session.Send(ctx, input, Events, tools.Approver) is the one entry point frontends call; the web server and the REPL both drive it.",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/web/serve.go#Server.runTurn",
            "code:cmd/evie/repl.go"
          ]
        },
        {
          "text": "Events is the frontend's rendering contract (Delta, Reasoning, ReasoningDone, AssistantDone, ToolCall, ToolResult, ResponseDiscarded); a frontend that also implements ActivityEvents gets the committed event in place of AssistantDone.",
          "refs": [
            "code:internal/agent/agent.go#Events",
            "code:internal/agent/agent.go#ActivityEvents",
            "code:internal/agent/turn.go#Session.emitCommittedAssistant"
          ]
        },
        {
          "text": "Client is the consumer-owned provider seam, one ChatStream call with streaming handlers, satisfied by the OpenRouter client.",
          "refs": [
            "code:internal/agent/agent.go#Client",
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "text": "History (Append, Events) and TurnOwnership (Acquire, Heartbeat, Authorize, Release) are consumer-owned persistence seams; the adapters bind the session and lease holder, so no method takes them as arguments.",
          "refs": [
            "code:internal/agent/history.go#History",
            "code:internal/agent/ownership.go#TurnOwnership",
            "code:internal/eviedb/turn_owner.go#Store.BindTurnOwner"
          ]
        },
        {
          "text": "Constructors take one resolved, immutable tools.Toolset (NewWithToolset, NewWithCompactorAndToolset, NewDelegatedWithToolset); SessionOption values may narrow reads but never widen the grant.",
          "refs": [
            "code:internal/agent/agent.go#NewWithToolset",
            "code:internal/agent/agent.go#NewWithCompactorAndToolset",
            "code:internal/agent/agent.go#SessionOption"
          ]
        },
        {
          "text": "WrapUpSignal and CompactionMemoryFlush are optional hooks: the subagent supervisor ends a worker's turn early through the first, and the background memory pass is offered every committed compaction through the second.",
          "refs": [
            "code:internal/agent/wrap_up.go#WrapUpSignal",
            "code:internal/agent/compaction_flush.go#CompactionMemoryFlush"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The turn lease lasts 30 seconds and is renewed every 10; a lost lease cancels the turn with ErrLeaseLost, and every append is fenced by the lease token, so a turn that lost its lease cannot write.",
          "refs": [
            "code:internal/agent/ownership.go#defaultTurnTiming",
            "code:internal/agent/turn.go#Session.startHeartbeat",
            "code:internal/eviedb/history.go#SessionHistory.Append"
          ]
        },
        {
          "text": "A tool call cut off by cancellation or a lost lease leaves a tool_intent with no outcome; history reconstruction drops such incomplete tool-call groups instead of replaying them.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/history.go#incompleteToolGroupEvents"
          ]
        },
        {
          "text": "A session given a nil Approver declines every gated tool call, which is how delegated workers run.",
          "refs": [
            "code:internal/agent/turn.go#admitApproval",
            "code:internal/subagents/supervisor.go#Supervisor.execute"
          ]
        },
        {
          "text": "The agent-extract spec gave Send per-call extra tools; today's code pins one Toolset per session and rejects different extras with \"session Toolset is already pinned\", keeping New plus Send(extra...) only as a legacy path.",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/agent/agent.go#New",
            "doc:cmd/evie/docs/done/agent-extract.spec.md#decisions-with-why"
          ]
        },
        {
          "text": "The primary system prompt is derived from the session's pinned tool schemas, so a session pinned before a tool existed keeps its exact prompt; worker instructions replace it entirely.",
          "refs": [
            "code:internal/agent/prompt.go#primaryInstructions",
            "code:internal/agent/context.go#ContextComposeInput.WorkerInstructions"
          ]
        },
        {
          "text": "A committed final assistant message wins over a cancellation that arrived while its append was in flight: the turn still ends as a success.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "Provider retries happen only before any output streamed; once text reached the frontend a provider failure ends the turn, and the frontend is told the streamed text was not saved.",
          "refs": [
            "code:internal/agent/turn.go#Session.callProvider",
            "code:internal/agent/ownership.go#DiscardedResponseMessage"
          ]
        }
      ],
      "terms": [
        "Working Memory",
        "Episodic Memory",
        "Automatic Recall",
        "Action Approval"
      ],
      "links": [
        "doc:cmd/evie/docs/done/agent-extract.spec.md",
        "doc:cmd/evie/docs/active/serve.spec.md"
      ],
      "diagrams": [
        {
          "id": "agent-turn-stages",
          "title": "Turn stages",
          "kind": "state",
          "summary": {
            "text": "The stages one Send passes through, as the turn coordinator records them; a terminal turn event names the stage a failed or interrupted turn ended in.",
            "refs": [
              "code:internal/memory/event.go#TurnStage",
              "code:internal/agent/ownership.go#turnCoordinator",
              "code:internal/agent/turn.go#Session.appendTerminal"
            ]
          },
          "states": [
            {
              "id": "turn_start",
              "label": "turn_start: lease held, owner message being stored",
              "refs": [
                "code:internal/agent/agent.go#Session.Send",
                "code:internal/memory/event.go#StageTurnStart"
              ]
            },
            {
              "id": "context_compose",
              "label": "context_compose: load history, compose the request",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#StageContextCompose"
              ]
            },
            {
              "id": "context_compaction",
              "label": "context_compaction: summarize older turns",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#StageContextCompaction"
              ]
            },
            {
              "id": "provider",
              "label": "provider: stream the model response",
              "refs": [
                "code:internal/agent/turn.go#Session.callProvider",
                "code:internal/memory/event.go#StageProvider"
              ]
            },
            {
              "id": "assistant_commit",
              "label": "assistant_commit: store the assistant message",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#StageAssistantCommit"
              ]
            },
            {
              "id": "tool_prepare",
              "label": "tool_prepare: store the intent, prepare the call",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#StageToolPrepare"
              ]
            },
            {
              "id": "tool_approval",
              "label": "tool_approval: waiting for the owner",
              "refs": [
                "code:internal/agent/turn.go#admitApproval",
                "code:internal/memory/event.go#StageToolApproval"
              ]
            },
            {
              "id": "tool_execute",
              "label": "tool_execute: the tool runs",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#StageToolExecute"
              ]
            },
            {
              "id": "tool_commit",
              "label": "tool_commit: store the tool outcome",
              "refs": [
                "code:internal/agent/ownership.go#turnCoordinator.completeToolPhase",
                "code:internal/memory/event.go#StageToolCommit"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded: a final assistant message is committed",
              "refs": [
                "code:internal/agent/ownership.go#turnCoordinator.finishSuccessBoundary"
              ]
            },
            {
              "id": "ended",
              "label": "failed or interrupted: turn_failed or turn_interrupted event",
              "refs": [
                "code:internal/agent/turn.go#Session.appendTerminal",
                "code:internal/agent/turn.go#causeHasDurableTerminal"
              ]
            }
          ],
          "initial": "turn_start",
          "transitions": [
            {
              "from": "turn_start",
              "to": "context_compose",
              "label": "user_message stored",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "context_compose",
              "to": "context_compaction",
              "label": "request over its working budget",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/automatic_compaction.go#selectAutomaticCompaction"
              ]
            },
            {
              "from": "context_compaction",
              "to": "provider",
              "label": "summary committed, context_snapshot stored",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "context_compaction",
              "to": "context_compose",
              "label": "compaction failed but the projection still fits",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "context_compose",
              "to": "provider",
              "label": "context_snapshot stored",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "provider",
              "to": "context_compose",
              "label": "context-length rejection: one compact-and-retry",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "provider",
              "to": "assistant_commit",
              "label": "response validated",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/turn.go#validateAssistantResponse"
              ]
            },
            {
              "from": "assistant_commit",
              "to": "succeeded",
              "label": "no tool calls",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "assistant_commit",
              "to": "tool_prepare",
              "label": "tool calls to run",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "tool_prepare",
              "to": "tool_approval",
              "label": "gated call asks the Approver",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
              ]
            },
            {
              "from": "tool_prepare",
              "to": "tool_execute",
              "label": "ungated call authorized",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/tools/registry.go#AuthorizeExecution"
              ]
            },
            {
              "from": "tool_prepare",
              "to": "tool_commit",
              "label": "preparation failed or unknown tool: an error result",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
                "code:internal/agent/ownership.go#turnCoordinator.completeToolPhase"
              ]
            },
            {
              "from": "tool_approval",
              "to": "tool_execute",
              "label": "approved; approval event stored",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "tool_approval",
              "to": "tool_commit",
              "label": "declined or expired; approval event stored",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "tool_execute",
              "to": "tool_commit",
              "label": "tool returned",
              "refs": [
                "code:internal/agent/ownership.go#turnCoordinator.completeToolPhase"
              ]
            },
            {
              "from": "tool_commit",
              "to": "tool_prepare",
              "label": "next call of the same response",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/ownership.go#turnCoordinator.finishAdmittedCallbackStage"
              ]
            },
            {
              "from": "tool_commit",
              "to": "context_compose",
              "label": "all calls answered: next model request",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "provider",
              "to": "ended",
              "label": "provider error, invalid response or step limit",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/turn.go#Session.appendTerminal"
              ]
            },
            {
              "from": "context_compose",
              "to": "ended",
              "label": "context overflow or repository-instruction failure",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/turn.go#Session.classifyRepositoryInstructionError"
              ]
            },
            {
              "from": "tool_execute",
              "to": "ended",
              "label": "caller cancelled or deadline passed",
              "refs": [
                "code:internal/agent/turn.go#Session.classifyLocalError",
                "code:internal/agent/turn.go#Session.appendTerminal"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/tools",
      "level": "component",
      "kind": "package",
      "title": "tools",
      "group": "runtime",
      "summary": {
        "text": "Defines every model-facing tool, one file per tool, and the immutable Toolset that dispatches a tool call through approval, lifecycle authorization and execution.",
        "refs": [
          "code:internal/tools/registry.go#Tool",
          "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
        ]
      },
      "does": [
        {
          "text": "A Tool binds a wire schema to an Execute function that takes the model's raw JSON arguments and returns plain text; tools never build wire messages, the dispatcher does.",
          "refs": [
            "code:internal/tools/registry.go#Tool",
            "code:internal/tools/registry.go"
          ]
        },
        {
          "text": "NewToolset copies every definition and schema, so a live session's capability surface cannot change underneath it; WithTools and WithoutTools return new Toolsets.",
          "refs": [
            "code:internal/tools/registry.go#NewToolset",
            "code:internal/tools/registry.go#Toolset.WithTools",
            "code:internal/tools/registry.go#Toolset.WithoutTools"
          ]
        },
        {
          "text": "The dispatcher finds the tool by name, decides whether it needs approval, prepares the effect and its preview, asks the Approver, hands the decision to the observer, and only then executes.",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "text": "It asks the caller to authorize the turn's lease at two boundaries: before preparation work that may reach a server, and before execution.",
          "refs": [
            "code:internal/tools/registry.go#AuthorizationBoundary",
            "code:internal/tools/registry.go#LifecycleAuthorizer"
          ]
        },
        {
          "text": "Declines, expiries, tool errors and unknown tools become ordinary model-visible tool results, with error text cut at 16 KiB.",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
            "code:internal/tools/registry.go#toolError",
            "code:internal/tools/registry.go#maxToolErrorBytes"
          ]
        },
        {
          "text": "KernelToolset is the non-plugin surface (get_time, query_db, edit_db, read_file, edit_file, bash and the cron tools) that production composition extends with plugin capabilities; of these only edit_db and edit_file need approval.",
          "refs": [
            "code:internal/tools/registry.go#KernelToolset",
            "code:internal/tools/registry.go#kernelToolsAfterYouTube",
            "code:internal/tools/registry.go#kernelToolsBeforeFinance"
          ]
        },
        {
          "text": "Supplies the confined worker toolsets: ExploreToolset (root-confined read_file, the read-only query_db, and a bash that runs only vetted read-only commands) and BuildToolset (worktree-confined read_file, edit_file and write_file, plus a bash only where an OS sandbox exists).",
          "refs": [
            "code:internal/tools/explore.go#ExploreTools",
            "code:internal/tools/build.go#BuildTools",
            "code:internal/tools/readonly_bash.go#runReadOnlyBash"
          ]
        },
        {
          "text": "Runs the owner's setup and verify commands for the merge gate under the same OS sandbox as worker bash, and refuses to run them at all without one.",
          "refs": [
            "code:internal/tools/gate.go#RunGateCommand",
            "code:internal/tools/sandbox.go#DetectWorkerSandbox"
          ]
        },
        {
          "text": "Frames third-party text (web pages, YouTube transcripts, transcript queries, stored tool results) as untrusted data before returning it.",
          "refs": [
            "code:internal/tools/webfetch.go#frameUntrustedWeb",
            "code:internal/tools/youtube.go#escapeFrameDelimiters",
            "code:internal/tools/tool_result_read.go"
          ]
        }
      ],
      "owns": [
        {
          "text": "Keeps the bash tool's working directory in the session's Directory, falling back to a process-wide remembered directory only when no Directory is supplied.",
          "refs": [
            "code:internal/tools/directory.go#Directory",
            "code:internal/tools/bash.go#runBash",
            "code:internal/tools/bash.go#sessionCwd"
          ]
        },
        {
          "text": "Spills output past the inline limit (30,000 characters for bash) to unique temporary files, which are swept after 24 hours.",
          "refs": [
            "code:internal/tools/bash.go#maxBashOutput",
            "code:internal/tools/spill.go#createSpillFile",
            "code:internal/tools/spill.go#spillRetention"
          ]
        },
        {
          "text": "Holds the secrets denylist the file tools enforce for reads and writes (~/.ssh, ~/.aws, ~/.gnupg, ~/.evie/procedural, shell rc files, .netrc and Evie's database files).",
          "refs": [
            "code:internal/tools/file.go#deniedDirs",
            "code:internal/tools/file.go#deniedNames",
            "code:internal/tools/file.go#deniedFiles"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Tool, Toolset, Approver, Decision (Declined, Approved, Expired, ApprovedAlternative), ApprovalObserver and LifecycleAuthorizer form the dispatch contract the agent loop drives.",
          "refs": [
            "code:internal/tools/registry.go#Approver",
            "code:internal/tools/registry.go#Decision",
            "code:internal/tools/registry.go#ApprovalObserver",
            "code:internal/tools/registry.go#LifecycleAuthorizer"
          ]
        },
        {
          "text": "PreparedTool carries a FileChangePreview for the approval card, ApprovalMetadata binding a semantic proposal to its durable approval, an optional Alternative, and an optional Review that queues the effect for the owner instead of asking.",
          "refs": [
            "code:internal/tools/registry.go#PreparedTool",
            "code:internal/tools/file.go#FileChangePreview",
            "code:internal/tools/registry.go#ApprovalMetadata"
          ]
        },
        {
          "text": "InvocationContext, read with InvocationFromContext, is the harness-owned authority a tool sees: profile, Directory, scope, lease, source and intent events, memory search and stored-result reads.",
          "refs": [
            "code:internal/tools/registry.go#InvocationContext",
            "code:internal/tools/registry.go#InvocationFromContext"
          ]
        },
        {
          "text": "Toolset.WithWorkerRoot and Toolset.WithBuildFence bind a worker's pinned folder or worktree without changing schemas, so the Toolset still matches its receipt.",
          "refs": [
            "code:internal/tools/explore.go#Toolset.WithWorkerRoot",
            "code:internal/tools/build.go#Toolset.WithBuildFence"
          ]
        },
        {
          "text": "ReadDB gives Workspace tabs the same fenced, read-only SELECT path as query_db.",
          "refs": [
            "code:internal/tools/db_read.go#ReadDB"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "bash is deliberately ungated, with no approval and no path denylist, because a shell walks around any fence; the file tools' secrets denylist does not apply to it.",
          "refs": [
            "code:internal/tools/bash.go#bashTool",
            "code:internal/tools/file.go#deniedDirs"
          ]
        },
        {
          "text": "The web server's cross-origin guard exists because bash is ungated: a forged browser request must be rejected before it can start a turn.",
          "refs": [
            "code:internal/web/serve.go#Server.Handler",
            "code:internal/web/serve.go#Server.guard"
          ]
        },
        {
          "text": "A RequiresApproval check that errors counts as needing approval.",
          "refs": [
            "code:internal/tools/registry.go#Tool.RequiresApproval",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "text": "A prepared tool with semantic ApprovalMetadata fails unless an approval observer is wired, so a semantic change never runs without a durable approval record.",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
            "code:internal/tools/registry.go#ApprovalMetadata"
          ]
        },
        {
          "text": "ApprovedAlternative runs the prepared alternative as an ordinary approval; for a tool without one it counts as declined.",
          "refs": [
            "code:internal/tools/registry.go#Decision",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "text": "LegacyKernelToolset and PreTodoExtractionKernelToolset exist only to rebuild old Composition Receipts; new sessions must use KernelToolset.",
          "refs": [
            "code:internal/tools/registry.go#LegacyKernelToolset",
            "code:internal/tools/registry.go#PreTodoExtractionKernelToolset"
          ]
        },
        {
          "text": "A build worker has no bash on a host without a usable OS sandbox, and EVIE_WORKER_SANDBOX=off turns worker bash off everywhere.",
          "refs": [
            "code:internal/tools/sandbox.go#DetectWorkerSandbox",
            "code:internal/tools/build.go#BuildTools"
          ]
        },
        {
          "text": "bash's process-wide fallback directory reverses the earlier decision that the tools package is stateless.",
          "refs": [
            "code:internal/tools/bash.go#sessionCwd",
            "doc:cmd/evie/docs/done/bash.decisions.md"
          ]
        }
      ],
      "terms": [
        "Action Approval",
        "Capability",
        "Kernel"
      ],
      "links": [
        "doc:cmd/evie/docs/done/bash.decisions.md",
        "doc:cmd/evie/docs/done/file-tools.spec.md",
        "doc:cmd/evie/docs/done/web-fetch.spec.md"
      ],
      "diagrams": [
        {
          "id": "tools-tool-call-lifecycle",
          "title": "Tool call lifecycle",
          "kind": "state",
          "summary": {
            "text": "How one model tool call moves through the dispatcher, and which durable event closes it.",
            "refs": [
              "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
              "code:internal/agent/turn.go#Session.runOwnedTurn"
            ]
          },
          "states": [
            {
              "id": "intent",
              "label": "tool_intent stored",
              "refs": [
                "code:internal/agent/agent.go#toolIntentInput",
                "code:internal/memory/event.go#EventToolIntent"
              ]
            },
            {
              "id": "preparing",
              "label": "preparing: approval needed, Prepare builds the effect",
              "refs": [
                "code:internal/tools/registry.go#Tool.Prepare",
                "code:internal/tools/registry.go#PreparedTool"
              ]
            },
            {
              "id": "awaiting",
              "label": "awaiting the owner's decision",
              "refs": [
                "code:internal/tools/registry.go#Approver",
                "code:internal/web/approvals.go#Server.approver"
              ]
            },
            {
              "id": "decided",
              "label": "approval event stored",
              "refs": [
                "code:internal/agent/agent.go#approvalEventInput",
                "code:internal/memory/event.go#EventApproval"
              ]
            },
            {
              "id": "executing",
              "label": "executing",
              "refs": [
                "code:internal/tools/registry.go#Tool.Execute",
                "code:internal/tools/registry.go#PreparedTool.Execute"
              ]
            },
            {
              "id": "succeeded",
              "label": "tool_succeeded",
              "refs": [
                "code:internal/agent/agent.go#toolOutcomeInput",
                "code:internal/memory/event.go#EventToolSucceeded"
              ]
            },
            {
              "id": "failed",
              "label": "tool_failed",
              "refs": [
                "code:internal/agent/agent.go#toolOutcomeInput",
                "code:internal/memory/event.go#EventToolFailed"
              ]
            },
            {
              "id": "cancelled",
              "label": "tool_cancelled",
              "refs": [
                "code:internal/agent/agent.go#toolOutcomeInput",
                "code:internal/memory/event.go#EventToolCancelled"
              ]
            }
          ],
          "initial": "intent",
          "transitions": [
            {
              "from": "intent",
              "to": "executing",
              "label": "no approval needed; execution authorized",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
                "code:internal/tools/registry.go#AuthorizeExecution"
              ]
            },
            {
              "from": "intent",
              "to": "preparing",
              "label": "NeedsApproval, or RequiresApproval says yes or errors",
              "refs": [
                "code:internal/tools/registry.go#Tool.NeedsApproval",
                "code:internal/tools/registry.go#Tool.RequiresApproval"
              ]
            },
            {
              "from": "intent",
              "to": "failed",
              "label": "unknown tool name",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
              ]
            },
            {
              "from": "preparing",
              "to": "failed",
              "label": "Prepare returned an error",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
                "code:internal/tools/registry.go#toolError"
              ]
            },
            {
              "from": "preparing",
              "to": "succeeded",
              "label": "Review set: queued for the owner, no approval recorded",
              "refs": [
                "code:internal/tools/registry.go#PreparedTool.Review",
                "code:internal/tools/registry.go#queueForReview"
              ]
            },
            {
              "from": "preparing",
              "to": "awaiting",
              "label": "Approver called with name, arguments and preview",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
              ]
            },
            {
              "from": "awaiting",
              "to": "decided",
              "label": "approved, declined or expired; observer stores it",
              "refs": [
                "code:internal/tools/registry.go#ApprovalObserver",
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "decided",
              "to": "executing",
              "label": "approved; execution authorized",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
              ]
            },
            {
              "from": "decided",
              "to": "cancelled",
              "label": "declined or expired",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "executing",
              "to": "succeeded",
              "label": "Execute returned text",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
              ]
            },
            {
              "from": "executing",
              "to": "failed",
              "label": "Execute returned an error",
              "refs": [
                "code:internal/tools/registry.go#toolError"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/plugins",
      "level": "component",
      "kind": "package",
      "title": "plugins",
      "group": "runtime",
      "summary": {
        "text": "The Kernel's Plugin Manager for compiled First-party Plugins: it validates their manifests, runs the reversible Plugin Lifecycle, and resolves an Agent Preset into a session's Toolset and Composition Receipt.",
        "refs": [
          "code:internal/plugins/manager.go#Manager",
          "code:internal/plugins/preset.go#Manager.ResolvePresetContext"
        ]
      },
      "does": [
        {
          "text": "NewManager validates the compiled catalog once: unique plugin IDs and Capability IDs, valid manifests, a provider role for every declared capability, compatibility evidence, dependency ranges, and a dependency order without cycles.",
          "refs": [
            "code:internal/plugins/manager.go#NewManager",
            "code:internal/plugins/manager.go#validateManifest",
            "code:internal/plugins/manager.go#Manager.validateDependencies",
            "code:internal/plugins/manager.go#Manager.dependencyOrder"
          ]
        },
        {
          "text": "ConfigureEnabledState seeds each plugin's default enabled state once in Evie's database and replays the owner's durable choices; every later management call refreshes from that store first.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.ConfigureEnabledState",
            "code:internal/plugins/manager.go#Manager.RefreshEnabledState",
            "code:internal/eviedb/plugin_configuration.go#Store.ResolvePluginEnabled"
          ]
        },
        {
          "text": "SetEnabledWithTransition stores the owner's intent, reconciles every affected plugin, and reports the target's transition plus each dependent whose lifecycle changed.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.SetEnabledWithTransition",
            "code:internal/plugins/manager.go#LifecycleTransition"
          ]
        },
        {
          "text": "Reconciliation stops plugins in reverse dependency order and starts them in dependency order, hiding a plugin's capabilities before any Stop hook runs.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.reconcile",
            "code:internal/plugins/manager.go#Manager.beginShutdownLocked",
            "code:internal/plugins/manager.go#Manager.markShutdownLocked"
          ]
        },
        {
          "text": "A failed start is rolled back by calling Stop under a bounded cleanup context detached from cancellation; a failed Stop leaves the plugin failed with cleanup pending until a newer command retries it.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.startPlugin",
            "code:internal/plugins/manager.go#Manager.rollbackPlugin",
            "code:internal/plugins/manager.go#Manager.stopPlugin",
            "code:internal/plugins/manager.go#Manager.boundedCleanupContext"
          ]
        },
        {
          "text": "ResolvePresetContext composes one of the four built-in presets (standard, research, explore, build): each required Capability must come from a ready provider within its contract range, an unavailable optional one only warns, and duplicate tool names fail.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.ResolvePresetContext",
            "code:internal/plugins/preset.go#Manager.resolvePreset",
            "code:internal/plugins/preset.go#Manager.resolveCapabilityLocked"
          ]
        },
        {
          "text": "Builds the session's Composition Receipt from the selected providers, capabilities, schema hashes, tool schemas, instruction hashes and configuration references, and validates it before returning.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.resolvePreset",
            "code:internal/composition/receipt.go#Validate"
          ]
        },
        {
          "text": "ResumeCompositionContext rebuilds a pinned session exactly from its receipt, choosing the historical preset content by its version hash, and accepts a newer provider only with exact contract and schema evidence, returning a Compatibility Resolution for it.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.ResumeCompositionContext",
            "code:internal/plugins/preset.go#Manager.resumePresetWithBase"
          ]
        },
        {
          "text": "Ships ten first-party plugins (web, finance, youtube, todo, memory, subagents, workflows, github, mcp and workspace-tabs), each enabled by default until the owner chooses otherwise.",
          "refs": [
            "code:cmd/evie/main.go#defaultPluginEnabled",
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the in-memory lifecycle record of each compiled plugin: enabled and started flags, state, diagnostic, warnings, active capabilities, loaded optional dependencies and Connection Readiness.",
          "refs": [
            "code:internal/plugins/manager.go#compiledPlugin"
          ]
        },
        {
          "text": "Stores only desired enabled state, through the EnabledStateStore boundary, which eviedb implements as plugin_enabled_configuration rows whose revision grows with every command.",
          "refs": [
            "code:internal/plugins/manager.go#EnabledStateStore",
            "code:internal/eviedb/plugin_configuration.go#Store.SetPluginEnabled"
          ]
        },
        {
          "text": "Owns the built-in Agent Presets and every historical standard preset version, each identified by the SHA-256 of its canonical content.",
          "refs": [
            "code:internal/plugins/preset.go#BuiltinStandardPreset",
            "code:internal/plugins/preset.go#canonicalPresetVersion",
            "code:internal/plugins/preset.go#StandardPresetVersion"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Plugin (Manifest, Start, Stop) is the one small lifecycle interface; contributions go through focused provider roles: ToolProvider, ResumableToolProvider, WorkflowCapabilityProvider, WorkflowEffectProvider and ConnectionReadinessProvider.",
          "refs": [
            "code:internal/plugins/manager.go#Plugin",
            "code:internal/plugins/manager.go#ToolProvider",
            "code:internal/plugins/manager.go#ResumableToolProvider",
            "code:internal/plugins/workflow.go#WorkflowCapabilityProvider",
            "code:internal/plugins/workflow_effect.go#WorkflowEffectProvider",
            "code:internal/plugins/manager.go#ConnectionReadinessProvider",
            "adr:0015"
          ]
        },
        {
          "text": "Manifest declares the plugin ID, implementation version, Kernel compatibility range, required and optional dependencies, Capability Contracts, and the prior implementations it can resume.",
          "refs": [
            "code:internal/plugins/manager.go#Manifest",
            "adr:0037"
          ]
        },
        {
          "text": "Management reads: InspectContext lists every plugin's lifecycle, health, Connection Readiness and diagnostics, and InspectPresetsContext and ValidatePresetContext report Preset Validity.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.InspectContext",
            "code:internal/plugins/preset.go#Manager.InspectPresetsContext",
            "code:internal/plugins/preset.go#Manager.ValidatePresetContext",
            "code:internal/plugins/preset.go#PresetInspection"
          ]
        },
        {
          "text": "Plugins reach Kernel services through narrow interfaces of their own, such as the Subagents plugin's ResearchSupervisor.",
          "refs": [
            "code:internal/plugins/subagents.go#ResearchSupervisor"
          ]
        },
        {
          "text": "CanonicalTodoMutationAttempt exposes the Todo wire contract's idempotency and request identities for evaluation.",
          "refs": [
            "code:internal/plugins/todo_arguments.go#CanonicalTodoMutationAttempt"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Production sessions are composed through Agent Presets; NewSessionToolset, which would add every ready capability, is not used by the commands.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.NewSessionToolset",
            "code:cmd/evie/main.go#main",
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession"
          ]
        },
        {
          "text": "ADR 0012 and the plugins spec describe user-authored presets versioned in procedural Git; the code has only the four immutable built-ins and refuses any other preset ID.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.ResolvePresetContext",
            "code:internal/plugins/preset.go#Manager.InspectPresets",
            "adr:0012",
            "doc:cmd/evie/docs/active/plugins.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "ADR 0036 and the plugins spec give presets a lazily loaded skill catalog; Preset has no skill field and nothing loads skills.",
          "refs": [
            "code:internal/plugins/preset.go#Preset",
            "adr:0036",
            "doc:cmd/evie/docs/active/plugins.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "The plugins spec says Phase 1 implements only the tool-provider role; the Manager already accepts workflow read and effect providers.",
          "refs": [
            "code:internal/plugins/manager.go#NewManager",
            "code:internal/plugins/workflow.go#WorkflowCapabilityProvider",
            "doc:cmd/evie/docs/active/plugins.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "A Capability's provider is the part of its ID before the first dot, so every Capability ID must be namespaced by its plugin.",
          "refs": [
            "code:internal/plugins/preset.go#providerIDForCapability",
            "code:internal/composition/receipt.go#ValidCapabilityID",
            "adr:0035"
          ]
        },
        {
          "text": "Connection Readiness never affects lifecycle or Preset Validity: a plugin that needs account setup still loads and its preset still resolves.",
          "refs": [
            "code:internal/plugins/manager.go#ConnectionReadinessProvider",
            "code:internal/plugins/manager.go#Manager.readConnectionReadiness",
            "adr:0038"
          ]
        },
        {
          "text": "Ready is not visible: a ready plugin's capabilities reach a session only when its Agent Preset names them.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.resolvePreset",
            "term:Loaded Plugin"
          ]
        },
        {
          "text": "The command revision makes a same-value enable meaningful: it retries a failed start or pending cleanup instead of being ignored.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.consumeEnabledIntentLocked",
            "code:internal/eviedb/plugin_configuration.go#Store.SetPluginEnabled"
          ]
        },
        {
          "text": "A session whose receipt pins a capability of a plugin that is no longer ready cannot be reopened: resume requires every pinned provider to be ready and fails visibly otherwise.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.resumePresetWithBase",
            "adr:0013"
          ]
        },
        {
          "text": "Disabling a plugin leaves already-open sessions' Toolsets intact, because a Toolset copies its tool definitions; whether those tools still work is up to the provider's Stop, as when the Subagents supervisor refuses new work once stopped.",
          "refs": [
            "code:internal/tools/registry.go#NewToolset",
            "code:internal/plugins/web.go#Web.Stop",
            "code:internal/subagents/supervisor.go#Supervisor.enter"
          ]
        },
        {
          "text": "Reconciliation gives up after four passes per plugin plus one and reports that the lifecycle did not stabilize.",
          "refs": [
            "code:internal/plugins/manager.go#Manager.reconcile"
          ]
        }
      ],
      "terms": [
        "Plugin",
        "First-party Plugin",
        "Kernel",
        "Plugin Manager",
        "Plugin Manifest",
        "Plugin Lifecycle",
        "Plugin Health",
        "Connection Readiness",
        "Enabled Plugin",
        "Loaded Plugin",
        "Agent Preset",
        "Preset Validity",
        "Capability",
        "Capability ID",
        "Capability Contract",
        "Capability Provider",
        "Compatibility Resolution",
        "Composition Receipt"
      ],
      "links": [
        "doc:cmd/evie/docs/active/plugins.spec.md",
        "adr:0004",
        "adr:0006",
        "adr:0013",
        "adr:0014",
        "adr:0039",
        "adr:0041"
      ],
      "diagrams": [
        {
          "id": "plugins-plugin-lifecycle",
          "title": "Plugin Lifecycle",
          "kind": "state",
          "summary": {
            "text": "The seven externally observable states the Plugin Manager moves a compiled plugin through.",
            "refs": [
              "code:internal/plugins/manager.go#LifecycleState",
              "adr:0014",
              "term:Plugin Lifecycle"
            ]
          },
          "states": [
            {
              "id": "disabled",
              "label": "disabled",
              "refs": [
                "code:internal/plugins/manager.go#StateDisabled"
              ]
            },
            {
              "id": "waiting",
              "label": "waiting: enabled, required dependency not ready",
              "refs": [
                "code:internal/plugins/manager.go#StateWaiting",
                "code:internal/plugins/manager.go#Manager.requiredDependencyDiagnosticLocked"
              ]
            },
            {
              "id": "loading",
              "label": "loading: Plugin.Start running",
              "refs": [
                "code:internal/plugins/manager.go#StateLoading",
                "code:internal/plugins/manager.go#Manager.startPlugin"
              ]
            },
            {
              "id": "ready",
              "label": "ready: capabilities active",
              "refs": [
                "code:internal/plugins/manager.go#StateReady"
              ]
            },
            {
              "id": "failed",
              "label": "failed: start failed, or cleanup pending",
              "refs": [
                "code:internal/plugins/manager.go#StateFailed"
              ]
            },
            {
              "id": "stopping",
              "label": "stopping: capabilities hidden, Plugin.Stop running",
              "refs": [
                "code:internal/plugins/manager.go#StateStopping",
                "code:internal/plugins/manager.go#Manager.markShutdownLocked"
              ]
            },
            {
              "id": "stopped",
              "label": "stopped",
              "refs": [
                "code:internal/plugins/manager.go#StateStopped"
              ]
            }
          ],
          "initial": "disabled",
          "transitions": [
            {
              "from": "disabled",
              "to": "waiting",
              "label": "owner enables it",
              "refs": [
                "code:internal/plugins/manager.go#Manager.applyEnabledIntentLocked"
              ]
            },
            {
              "from": "waiting",
              "to": "loading",
              "label": "required dependencies ready",
              "refs": [
                "code:internal/plugins/manager.go#Manager.reconcile",
                "code:internal/plugins/manager.go#Manager.startPlugin"
              ]
            },
            {
              "from": "loading",
              "to": "ready",
              "label": "Start succeeded",
              "refs": [
                "code:internal/plugins/manager.go#Manager.startPlugin"
              ]
            },
            {
              "from": "loading",
              "to": "failed",
              "label": "Start failed; registration rolled back",
              "refs": [
                "code:internal/plugins/manager.go#Manager.startPlugin",
                "code:internal/plugins/manager.go#Manager.rollbackPlugin"
              ]
            },
            {
              "from": "ready",
              "to": "stopping",
              "label": "disabled, stopped, a required dependency leaving, or optional availability changed",
              "refs": [
                "code:internal/plugins/manager.go#Manager.planShutdownLocked",
                "code:internal/plugins/manager.go#Manager.shouldRemainLoadedLocked"
              ]
            },
            {
              "from": "stopping",
              "to": "stopped",
              "label": "Stop returned",
              "refs": [
                "code:internal/plugins/manager.go#Manager.stopPlugin"
              ]
            },
            {
              "from": "stopping",
              "to": "failed",
              "label": "Stop failed: cleanup pending",
              "refs": [
                "code:internal/plugins/manager.go#Manager.stopPlugin"
              ]
            },
            {
              "from": "failed",
              "to": "stopping",
              "label": "a newer command retries pending cleanup",
              "refs": [
                "code:internal/plugins/manager.go#Manager.authorizeCleanupRetriesLocked",
                "code:internal/plugins/manager.go#Manager.markShutdownLocked"
              ]
            },
            {
              "from": "failed",
              "to": "waiting",
              "label": "re-enabled after a clean failed start",
              "refs": [
                "code:internal/plugins/manager.go#Manager.applyEnabledIntentLocked"
              ]
            },
            {
              "from": "stopped",
              "to": "waiting",
              "label": "enabled or started again",
              "refs": [
                "code:internal/plugins/manager.go#Manager.applyEnabledIntentLocked",
                "code:internal/plugins/manager.go#Manager.Start"
              ]
            },
            {
              "from": "stopped",
              "to": "loading",
              "label": "still enabled and not manually stopped: restarted",
              "refs": [
                "code:internal/plugins/manager.go#Manager.reconcile"
              ]
            },
            {
              "from": "stopped",
              "to": "disabled",
              "label": "no longer enabled",
              "refs": [
                "code:internal/plugins/manager.go#Manager.normalizeInactiveStates"
              ]
            },
            {
              "from": "failed",
              "to": "disabled",
              "label": "disabled after a clean failed start",
              "refs": [
                "code:internal/plugins/manager.go#Manager.normalizeInactiveStates"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/composition",
      "level": "component",
      "kind": "package",
      "title": "composition",
      "group": "runtime",
      "summary": {
        "text": "Defines the Composition Receipt, the content-free durable identity of what one session was composed from, and the Compatibility Resolution that records a provider substitution.",
        "refs": [
          "code:internal/composition/receipt.go#Receipt",
          "code:internal/composition/receipt.go#CompatibilityResolution"
        ]
      },
      "does": [
        {
          "text": "Validate checks a receipt completely: format version 1, a canonical preset identity with a sha256: version, MAJOR.MINOR.PATCH versions, no repeated providers, capabilities or tool schemas, and every Capability naming a listed provider it is namespaced by.",
          "refs": [
            "code:internal/composition/receipt.go#Validate",
            "code:internal/composition/receipt.go#FormatVersion"
          ]
        },
        {
          "text": "Marshal validates before encoding, and Unmarshal rejects unknown fields and trailing JSON before validating.",
          "refs": [
            "code:internal/composition/receipt.go#Marshal",
            "code:internal/composition/receipt.go#Unmarshal"
          ]
        },
        {
          "text": "ValidateCompatibilityResolution accepts a substitution only by a strictly newer implementation of the same provider, with a timestamp and per-Capability contract and schema evidence.",
          "refs": [
            "code:internal/composition/receipt.go#ValidateCompatibilityResolution"
          ]
        },
        {
          "text": "Provides the identity rules shared by providers and persistence: ValidIdentity, ValidProviderID, ValidCapabilityID, and strict semantic-version parsing and comparison.",
          "refs": [
            "code:internal/composition/receipt.go#ValidIdentity",
            "code:internal/composition/receipt.go#ValidProviderID",
            "code:internal/composition/receipt.go#ValidCapabilityID",
            "code:internal/composition/receipt.go#ParseVersion",
            "code:internal/composition/receipt.go#Version.Compare"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the receipt schema: preset ID and version, Evie version, providers with implementation versions, capabilities with contract versions and schema hashes, tool schema hashes, instruction hashes, configuration references and warnings.",
          "refs": [
            "code:internal/composition/receipt.go#Receipt"
          ]
        },
        {
          "text": "Owns the closed vocabulary of warnings for an optional Capability left out: provider not compiled, disabled, stopped, waiting or failed, capability not exposed, contract incompatible, or provider contract invalid.",
          "refs": [
            "code:internal/composition/receipt.go#WarningCode"
          ]
        },
        {
          "text": "Stores nothing itself: eviedb keeps one receipt per session in session_composition_receipts and appends Compatibility Resolutions separately.",
          "refs": [
            "code:internal/eviedb/composition_receipts.go#Store.SaveCompositionReceipt",
            "code:internal/eviedb/composition_receipts.go#insertCompositionReceipt",
            "code:cmd/evie/session_composition.go#resumeAndRecordSessionComposition"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Receipt, Provider, Capability, ToolSchema, InstructionReference, ConfigurationReference and Warning are plain JSON structs, which plugins re-exports as its own receipt type names.",
          "refs": [
            "code:internal/composition/receipt.go#Receipt",
            "code:internal/plugins/preset.go#CompositionReceipt",
            "code:internal/plugins/preset.go#ProviderReceipt"
          ]
        },
        {
          "text": "Clone copies a receipt's slices so callers cannot alias a pinned receipt.",
          "refs": [
            "code:internal/composition/receipt.go#Clone"
          ]
        },
        {
          "text": "CompatibilityResolutionKey, MarshalCompatibilityResolution and UnmarshalCompatibilityResolution give resolutions a stable identity and strict encoding.",
          "refs": [
            "code:internal/composition/receipt.go#CompatibilityResolutionKey",
            "code:internal/composition/receipt.go#MarshalCompatibilityResolution",
            "code:internal/composition/receipt.go#UnmarshalCompatibilityResolution"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The only configuration reference kind is connection, and its ID must be a canonical UUID, so a receipt cannot carry a credential or a free-form setting.",
          "refs": [
            "code:internal/composition/receipt.go#ValidateConfigurationReference",
            "term:Connection ID"
          ]
        },
        {
          "text": "It imports neither providers nor persistence, so both can consume one closed schema without a cycle.",
          "refs": [
            "code:internal/composition/receipt.go"
          ]
        },
        {
          "text": "Instructions are pinned by ID and SHA-256 only; a receipt never holds instruction text.",
          "refs": [
            "code:internal/composition/receipt.go#InstructionReference"
          ]
        },
        {
          "text": "A saved receipt never changes: a substitution is recorded as a separate Compatibility Resolution rather than by rewriting the receipt.",
          "refs": [
            "code:internal/composition/receipt.go#CompatibilityResolution",
            "adr:0041"
          ]
        }
      ],
      "terms": [
        "Composition Receipt",
        "Compatibility Resolution",
        "Connection ID",
        "Agent Preset",
        "Capability ID",
        "Capability Contract",
        "Kernel"
      ],
      "links": [
        "adr:0013",
        "adr:0041"
      ]
    },
    {
      "id": "pkg:internal/subagents",
      "level": "component",
      "kind": "package",
      "title": "subagents",
      "group": "runtime",
      "summary": {
        "text": "Supervises bounded child agents inside the Kernel: delegated research, explore and build workers for a parent turn, and run-owned workers for a Workflow Run.",
        "refs": [
          "code:internal/subagents/supervisor.go#Supervisor",
          "code:internal/subagents/run.go#Supervisor.RunWorker"
        ]
      },
      "does": [
        {
          "text": "Delegate, Explore and Build validate a batch against the Policy, derive the worker's receipt from the parent's own pinned receipt, admit every child durably, then run them in parallel and wait for all of them.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate",
            "code:internal/subagents/supervisor.go#Supervisor.Delegate"
          ]
        },
        {
          "text": "A child starts only when per-parent and runtime-wide capacity allow, checked in a write transaction, and fails with queue_deadline if it waits longer than the policy deadline.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.own",
            "code:internal/eviedb/subagent_execution.go#Store.StartSubagent"
          ]
        },
        {
          "text": "Each child is an ordinary agent Session over its own child session, built with NewDelegatedWithToolset and a metered model client that authorizes every call, enforces the token and request-size budgets, and asks for a tool-free wrap-up at 90 percent of a budget.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.execute",
            "code:internal/subagents/budget.go#budget.ChatStream",
            "code:internal/subagents/budget.go#budget.wrapUp",
            "code:internal/delegation/types.go#WrapUpPercent"
          ]
        },
        {
          "text": "A watchdog re-checks the child's authority every 100 ms and cancels it once a durable check proves that authority gone.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.watch",
            "code:internal/subagents/supervisor.go#watchdogInterval"
          ]
        },
        {
          "text": "Settles every attempt with a terminal state and reason; an outcome the store cannot record is retried, then handed to recovery and to same-key retries.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.finish",
            "code:internal/subagents/supervisor.go#Supervisor.handOff",
            "code:internal/subagents/supervisor.go#executionFailure"
          ]
        },
        {
          "text": "Continue resumes a finished child's own session as a new attempt with a fresh budget, under the current parent turn's authority.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.Continue"
          ]
        },
        {
          "text": "For a build worker it prepares the git worktree, runs the owner's setup command with network on the base commit, fences the tools to the worktree, and commits the worker's changes on its branch whatever the outcome.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.execute",
            "code:internal/subagents/gate.go#Supervisor.runSetup",
            "code:internal/subagents/supervisor.go#Supervisor.commitBuild"
          ]
        },
        {
          "text": "Runs the merge gate: ProposeMerge verifies the branch offline, rebases it onto the target and verifies again, and Merge fast-forwards the target only for the exact proposal the owner approved.",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.ProposeMerge",
            "code:internal/subagents/gate.go#Supervisor.proposeMerge",
            "code:internal/subagents/gate.go#Supervisor.merge"
          ]
        },
        {
          "text": "RunRecovery reconciles abandoned attempts and interrupted gate steps for the whole process lifetime, and never starts a child or a model call.",
          "refs": [
            "code:internal/subagents/recovery.go#Supervisor.RunRecovery",
            "code:internal/subagents/gate.go#Supervisor.RecoverGates"
          ]
        },
        {
          "text": "RunWorker runs a worker for a Workflow Run's agent step under the run's fenced lease and the step's pinned Model Policy, refusing when the worker preset no longer composes to what the run pinned.",
          "refs": [
            "code:internal/subagents/run.go#Supervisor.RunWorker",
            "code:internal/delegation/types.go#ErrWorkerChanged"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns only in-process supervision state: active calls, outcomes awaiting a write, build commit reports, gates running in this process, and target-branch moves to tell running workers about.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor"
          ]
        },
        {
          "text": "Durable attempt records live in Evie's database through eviedb's subagent_executions, admitted, started and settled in immediate transactions.",
          "refs": [
            "code:internal/eviedb/subagents.go#Store.AdmitSubagents",
            "code:internal/eviedb/subagent_execution.go#Store.StartSubagent",
            "code:internal/eviedb/subagent_execution.go#Store.FinishSubagent"
          ]
        },
        {
          "text": "Each child's conversation is an ordinary session with its own events, created at admission with the delegating session as its parent.",
          "refs": [
            "code:internal/eviedb/subagents.go#Store.AdmitSubagents",
            "code:internal/delegation/types.go#Attempt.Child"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "New(store, policy) and Configure(client, profile, resolver) set the supervisor up; the Subagents plugin's Start and Stop enable it and drain it.",
          "refs": [
            "code:internal/subagents/supervisor.go#New",
            "code:internal/subagents/supervisor.go#Supervisor.Configure",
            "code:internal/subagents/supervisor.go#Supervisor.Start",
            "code:internal/subagents/supervisor.go#Supervisor.Stop"
          ]
        },
        {
          "text": "Resolver composes a worker preset fresh, or exactly from a pinned receipt, into its Toolset, receipt and pinned instructions.",
          "refs": [
            "code:internal/subagents/supervisor.go#Resolver",
            "code:internal/subagents/supervisor.go#Composition"
          ]
        },
        {
          "text": "The Supervisor satisfies plugins.ResearchSupervisor (Delegate, Explore, Build, ReadReport, Continue, ProposeMerge, Merge) without the plugins package importing it.",
          "refs": [
            "code:internal/plugins/subagents.go#ResearchSupervisor",
            "code:cmd/evie/main.go#main"
          ]
        },
        {
          "text": "MergeAsOwner and CloseAsOwner let the owner merge or close a build's Change from the Changes tab or the command line.",
          "refs": [
            "code:internal/subagents/gate.go#Supervisor.MergeAsOwner",
            "code:internal/subagents/gate.go#Supervisor.CloseAsOwner"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Children run with no Approver, so any approval-gated tool a child calls is declined.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.execute",
            "code:internal/agent/turn.go#admitApproval"
          ]
        },
        {
          "text": "A child never gets more than its parent: research and build receipts copy the parent's pinned capability versions, and explore and build require the parent to hold the tools theirs are narrower forms of.",
          "refs": [
            "code:internal/subagents/receipt.go#researchReceipt",
            "code:internal/subagents/receipt.go#buildReceipt",
            "code:internal/subagents/receipt.go#exploreReceipt"
          ]
        },
        {
          "text": "The package doc speaks of foreground children, but run-owned workers run for a Workflow Run, under its lease and node attempt, with no parent turn at all.",
          "refs": [
            "code:internal/subagents/supervisor.go",
            "code:internal/subagents/run.go#Supervisor.RunWorker",
            "doc:cmd/evie/docs/active/subagents.decisions.md#2026-10-04--run-owned-workers-stage-14-sub-stage-9"
          ]
        },
        {
          "text": "Delivery rechecks data access after the last child settles; a child that cannot be delivered becomes an error entry without its content, while its siblings are still returned.",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate",
            "code:internal/subagents/supervisor.go#undeliverable"
          ]
        },
        {
          "text": "A pinned build worker that had bash will not resume on a host without an OS sandbox, rather than get an unsandboxed shell.",
          "refs": [
            "code:internal/plugins/preset.go#Manager.ResumeCompositionContext"
          ]
        },
        {
          "text": "SQLite lock contention (SQLITE_BUSY or SQLITE_LOCKED) proves nothing about authority, so the supervisor retries it instead of ending a child.",
          "refs": [
            "code:internal/subagents/supervisor.go#transientStoreError"
          ]
        }
      ],
      "terms": [
        "Kernel",
        "Agent Preset",
        "Composition Receipt",
        "Workflow Run",
        "Model Policy",
        "Change",
        "Task"
      ],
      "links": [
        "doc:cmd/evie/docs/active/subagents.spec.md",
        "doc:cmd/evie/docs/active/subagents.decisions.md",
        "adr:0002"
      ],
      "diagrams": [
        {
          "id": "subagents-attempt-lifecycle",
          "title": "Subagent attempt lifecycle",
          "kind": "state",
          "summary": {
            "text": "The durable states of one delegated or run-owned worker attempt, as subagent_executions records them.",
            "refs": [
              "code:internal/eviedb/subagents.go",
              "code:internal/delegation/types.go#Attempt.Terminal"
            ]
          },
          "states": [
            {
              "id": "admitted",
              "label": "admitted",
              "refs": [
                "code:internal/eviedb/subagents.go#Store.AdmitSubagents"
              ]
            },
            {
              "id": "running",
              "label": "running",
              "refs": [
                "code:internal/eviedb/subagent_execution.go#Store.StartSubagent"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded",
              "refs": [
                "code:internal/subagents/supervisor.go#Supervisor.execute"
              ]
            },
            {
              "id": "partial",
              "label": "partial: wrap-up report after a budget ran low",
              "refs": [
                "code:internal/delegation/types.go#StatePartial",
                "code:internal/subagents/supervisor.go#Supervisor.execute"
              ]
            },
            {
              "id": "failed",
              "label": "failed",
              "refs": [
                "code:internal/subagents/supervisor.go#executionFailure"
              ]
            },
            {
              "id": "cancelled",
              "label": "cancelled",
              "refs": [
                "code:internal/subagents/supervisor.go#stopped"
              ]
            },
            {
              "id": "interrupted",
              "label": "interrupted: authority ended",
              "refs": [
                "code:internal/subagents/supervisor.go#refused"
              ]
            }
          ],
          "initial": "admitted",
          "transitions": [
            {
              "from": "admitted",
              "to": "running",
              "label": "capacity free and the owner's authority holds",
              "refs": [
                "code:internal/eviedb/subagent_execution.go#Store.StartSubagent",
                "code:internal/subagents/supervisor.go#Supervisor.own"
              ]
            },
            {
              "from": "admitted",
              "to": "failed",
              "label": "queue_deadline: waited past the policy deadline",
              "refs": [
                "code:internal/subagents/supervisor.go#Supervisor.own"
              ]
            },
            {
              "from": "admitted",
              "to": "cancelled",
              "label": "parent cancelled or supervisor shut down",
              "refs": [
                "code:internal/subagents/supervisor.go#stopped"
              ]
            },
            {
              "from": "admitted",
              "to": "interrupted",
              "label": "authority ended before start",
              "refs": [
                "code:internal/subagents/supervisor.go#refused"
              ]
            },
            {
              "from": "running",
              "to": "succeeded",
              "label": "child turn ended with an answer",
              "refs": [
                "code:internal/subagents/supervisor.go#Supervisor.execute"
              ]
            },
            {
              "from": "running",
              "to": "partial",
              "label": "time, token, context or step budget triggered the wrap-up",
              "refs": [
                "code:internal/subagents/supervisor.go#Supervisor.execute",
                "code:internal/subagents/budget.go#budget.wrapUp"
              ]
            },
            {
              "from": "running",
              "to": "failed",
              "label": "deadline, token budget, context limit, wrap-up failed or infrastructure",
              "refs": [
                "code:internal/subagents/supervisor.go#executionFailure"
              ]
            },
            {
              "from": "running",
              "to": "cancelled",
              "label": "parent cancelled or shutdown",
              "refs": [
                "code:internal/subagents/supervisor.go#executionFailure"
              ]
            },
            {
              "from": "running",
              "to": "interrupted",
              "label": "lease or authority lost",
              "refs": [
                "code:internal/subagents/supervisor.go#executionFailure",
                "code:internal/subagents/supervisor.go#Supervisor.watch"
              ]
            },
            {
              "from": "running",
              "to": "interrupted",
              "label": "recovery: the owning lease is no longer live",
              "refs": [
                "code:internal/eviedb/subagent_execution.go#Store.RecoverSubagents"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/delegation",
      "level": "component",
      "kind": "package",
      "title": "delegation",
      "group": "runtime",
      "summary": {
        "text": "Defines the durable records and contracts of delegated work, kept separate from Tasks: assignments, policies, attempts, results as the parent reads them, build reports and the merge gate.",
        "refs": [
          "code:internal/delegation/types.go#Attempt",
          "code:internal/delegation/types.go#Result",
          "code:internal/delegation/merge.go#MergeProposal"
        ]
      },
      "does": [
        {
          "text": "Names the delegation tools and their subagents.* Capability IDs: delegate_research, read_subagent_report, continue_research, delegate_explore, delegate_build and propose_build_merge.",
          "refs": [
            "code:internal/delegation/types.go#CapabilityID",
            "code:internal/delegation/types.go#ToolName",
            "code:internal/delegation/types.go#ExploreToolName",
            "code:internal/delegation/types.go#BuildToolName",
            "code:internal/delegation/types.go#MergeToolName"
          ]
        },
        {
          "text": "Policy bounds delegation by per-parent and runtime-wide concurrency, batch size, children per turn, a deadline, a token budget and byte limits; DefaultPolicy is 2 per parent, 4 at once, 8 per batch, 16 per turn, 15 minutes and one million tokens.",
          "refs": [
            "code:internal/delegation/types.go#Policy",
            "code:internal/delegation/types.go#DefaultPolicy"
          ]
        },
        {
          "text": "Policy.Validate and ValidateWorkerBatch refuse a bad policy or batch before any child or provider is touched, naming the setting, key, field and limit.",
          "refs": [
            "code:internal/delegation/types.go#Policy.Validate",
            "code:internal/delegation/types.go#Policy.ValidateWorkerBatch"
          ]
        },
        {
          "text": "Result.ParentView renders a result for the parent model: harness-written fields stay plain JSON, and everything the child wrote becomes one untrusted-data frame labelled with the execution ID.",
          "refs": [
            "code:internal/delegation/parent_view.go#Result.ParentView",
            "code:internal/delegation/parent_view.go#frameChildOutput"
          ]
        },
        {
          "text": "MergeProposal.ParentView frames verification output as untrusted data, and ApprovalCard renders the merge approval card the owner sees.",
          "refs": [
            "code:internal/delegation/merge.go#MergeProposal.ParentView",
            "code:internal/delegation/merge.go#MergeProposal.ApprovalCard"
          ]
        },
        {
          "text": "ChangeText.Check bounds a Change's title to one printable line of 72 characters and its description to 4,096 bytes.",
          "refs": [
            "code:internal/delegation/change_text.go#ChangeText.Check"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the Attempt record: dispatch and execution IDs, parent authority, assignment and digest, child session, pinned receipt and policy, state and times, wrap-up, continuation, explore root, build worktree, gate, run owner and result.",
          "refs": [
            "code:internal/delegation/types.go#Attempt"
          ]
        },
        {
          "text": "Owns the attempt states (admitted, running, succeeded, partial, failed, cancelled, interrupted) and the reasons a child stopped; eviedb enforces the same list in a CHECK constraint.",
          "refs": [
            "code:internal/delegation/types.go#Attempt.Terminal",
            "code:internal/delegation/types.go#StatePartial",
            "code:internal/delegation/types.go#ReasonWrapUpFailed",
            "code:internal/eviedb/subagents.go"
          ]
        },
        {
          "text": "Owns the merge-gate record: the step, the setup and verify runs with their outcomes, target and branch heads, and the merge.",
          "refs": [
            "code:internal/delegation/types.go#BuildGate",
            "code:internal/delegation/types.go#GateRun",
            "code:internal/delegation/types.go#GateMerge"
          ]
        },
        {
          "text": "Owns RunOwner, a run-owned worker's authority: the Workflow Run's lease holder and token, the node attempt, and the step's pinned Model Policy.",
          "refs": [
            "code:internal/delegation/types.go#RunOwner"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Parent is the live invocation the harness supplies (profile, scope, lease, source and intent events); the model cannot set it.",
          "refs": [
            "code:internal/delegation/types.go#Parent",
            "code:internal/plugins/subagents.go#invokingParent"
          ]
        },
        {
          "text": "AdmitsChildren and BindsParentLease say which committed tool intents may authorize children or record the parent's lease.",
          "refs": [
            "code:internal/delegation/types.go#AdmitsChildren",
            "code:internal/delegation/types.go#BindsParentLease"
          ]
        },
        {
          "text": "Error sentinels (ErrConflict, ErrCapacity, ErrAuthority, ErrPolicy, ErrNotFound, ErrNotResumable, ErrUnavailable, ErrWorkerChanged) are the shared failure contract of the store, the supervisor and the plugin.",
          "refs": [
            "code:internal/delegation/types.go#ErrCapacity",
            "code:internal/delegation/types.go#ErrAuthority",
            "code:internal/delegation/types.go#ErrUnavailable",
            "code:internal/delegation/types.go#ErrWorkerChanged"
          ]
        },
        {
          "text": "RunAssignment, RunOutcome and RunKey are the Workflow Runtime's contract for run-owned workers.",
          "refs": [
            "code:internal/delegation/types.go#RunAssignment",
            "code:internal/delegation/types.go#RunOutcome",
            "code:internal/delegation/types.go#RunKey"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "ErrNotFound deliberately does not distinguish a missing execution from another session's, so an execution ID reveals nothing.",
          "refs": [
            "code:internal/delegation/types.go#ErrNotFound"
          ]
        },
        {
          "text": "A batch of max_batch results at result_bytes each must fit a 96 KiB envelope, under the agent loop's 100 KiB tool-result bound, and Validate rejects a policy that would not.",
          "refs": [
            "code:internal/delegation/types.go#Policy.Validate",
            "code:internal/delegation/types.go#BatchEnvelopeBytes",
            "code:internal/agent/tool_results.go#toolResultAdmissionBytes"
          ]
        },
        {
          "text": "Fields added later (Path, Base, Worker and others) are omitted when empty, so earlier records and assignment digests stay byte-identical.",
          "refs": [
            "code:internal/delegation/types.go#Assignment",
            "code:internal/delegation/types.go#Result"
          ]
        },
        {
          "text": "A build child's commands are worker-written, so ParentBuild leaves them out of the plain JSON and they appear only inside the framed summary.",
          "refs": [
            "code:internal/delegation/parent_view.go#ParentBuild"
          ]
        },
        {
          "text": "Attempts stay separate from Tasks: a Task names no execution, and orchestration joins them only when it reads a Task's latest build.",
          "refs": [
            "code:internal/delegation/types.go",
            "code:internal/task/task.go#Task",
            "code:internal/task/plan.go#PlanBuild"
          ]
        }
      ],
      "terms": [
        "Task",
        "Kernel",
        "Task Access Grant",
        "Change",
        "Workflow Run",
        "Model Policy"
      ],
      "links": [
        "doc:cmd/evie/docs/active/subagents.spec.md"
      ],
      "diagrams": [
        {
          "id": "delegation-merge-gate",
          "title": "Merge gate steps",
          "kind": "state",
          "summary": {
            "text": "The steps a build attempt's merge gate records; recovery settles an in-progress step whose owner's turn is gone.",
            "refs": [
              "code:internal/delegation/types.go#GateSetupRunning",
              "code:internal/delegation/types.go#GateInProgress",
              "code:internal/subagents/gate.go#Supervisor.RecoverGates"
            ]
          },
          "states": [
            {
              "id": "none",
              "label": "no gate record yet",
              "refs": [
                "code:internal/delegation/types.go#Attempt.Gate"
              ]
            },
            {
              "id": "setup_running",
              "label": "setup_running",
              "refs": [
                "code:internal/delegation/types.go#GateSetupRunning",
                "code:internal/subagents/gate.go#Supervisor.runSetup"
              ]
            },
            {
              "id": "setup_done",
              "label": "setup_done",
              "refs": [
                "code:internal/delegation/types.go#GateSetupDone",
                "code:internal/subagents/gate.go#Supervisor.runSetup"
              ]
            },
            {
              "id": "verifying",
              "label": "verifying: the branch as the worker left it",
              "refs": [
                "code:internal/delegation/types.go#GateVerifying",
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "id": "rebasing",
              "label": "rebasing onto the target's head",
              "refs": [
                "code:internal/delegation/types.go#GateRebasing",
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "id": "verifying_rebased",
              "label": "verifying_rebased",
              "refs": [
                "code:internal/delegation/types.go#GateVerifyingAfter",
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "id": "awaiting_approval",
              "label": "awaiting_approval",
              "refs": [
                "code:internal/delegation/types.go#GateAwaiting",
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "id": "merging",
              "label": "merging: fast-forward in progress",
              "refs": [
                "code:internal/delegation/types.go#GateMerging",
                "code:internal/subagents/gate.go#Supervisor.merge"
              ]
            },
            {
              "id": "merged",
              "label": "merged",
              "refs": [
                "code:internal/delegation/types.go#GateMerged",
                "code:internal/subagents/gate.go#Supervisor.completeMerge"
              ]
            },
            {
              "id": "verify_failed",
              "label": "verify_failed",
              "refs": [
                "code:internal/delegation/types.go#GateVerifyFailed",
                "code:internal/subagents/gate.go#verifyStep"
              ]
            },
            {
              "id": "rebase_conflict",
              "label": "rebase_conflict",
              "refs": [
                "code:internal/delegation/types.go#GateRebaseConflict",
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "id": "target_moved",
              "label": "target_moved",
              "refs": [
                "code:internal/delegation/types.go#GateTargetMoved",
                "code:internal/subagents/gate.go#Supervisor.targetMoved"
              ]
            },
            {
              "id": "merge_refused",
              "label": "merge_refused: the owner's checkout would be overwritten",
              "refs": [
                "code:internal/delegation/types.go#GateMergeRefused",
                "code:internal/subagents/gate.go#Supervisor.merge"
              ]
            },
            {
              "id": "refused",
              "label": "refused",
              "refs": [
                "code:internal/delegation/types.go#GateRefused",
                "code:internal/subagents/gate.go#Supervisor.gateFailure"
              ]
            },
            {
              "id": "interrupted",
              "label": "interrupted",
              "refs": [
                "code:internal/delegation/types.go#GateInterrupted",
                "code:internal/subagents/gate.go#Supervisor.recoverGate"
              ]
            }
          ],
          "initial": "none",
          "transitions": [
            {
              "from": "none",
              "to": "setup_running",
              "label": "build starts and a setup command is set",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.runSetup"
              ]
            },
            {
              "from": "none",
              "to": "setup_done",
              "label": "setup skipped: base commit not in the owner's history",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.runSetup"
              ]
            },
            {
              "from": "setup_running",
              "to": "setup_done",
              "label": "setup command finished",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.runSetup"
              ]
            },
            {
              "from": "setup_running",
              "to": "interrupted",
              "label": "recovery: setup was never recorded",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.recoverGate"
              ]
            },
            {
              "from": "none",
              "to": "verifying",
              "label": "propose_build_merge",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "setup_done",
              "to": "verifying",
              "label": "propose_build_merge",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "verifying",
              "to": "verify_failed",
              "label": "verify failed, timed out, or changed tracked files",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge",
                "code:internal/subagents/gate.go#verifyStep"
              ]
            },
            {
              "from": "verifying",
              "to": "rebasing",
              "label": "first run passed",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "rebasing",
              "to": "rebase_conflict",
              "label": "conflicts; rebase aborted",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "rebasing",
              "to": "verifying_rebased",
              "label": "branch rebased onto a newer head",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "rebasing",
              "to": "awaiting_approval",
              "label": "branch already contained the head: second run skipped",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "verifying_rebased",
              "to": "verify_failed",
              "label": "second run failed",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "verifying_rebased",
              "to": "awaiting_approval",
              "label": "both runs passed",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge"
              ]
            },
            {
              "from": "awaiting_approval",
              "to": "merging",
              "label": "owner approved this exact proposal",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.merge"
              ]
            },
            {
              "from": "awaiting_approval",
              "to": "target_moved",
              "label": "target moved after verification",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.merge",
                "code:internal/subagents/gate.go#Supervisor.targetMoved"
              ]
            },
            {
              "from": "merging",
              "to": "merged",
              "label": "fast-forward done; worktree removed",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.completeMerge"
              ]
            },
            {
              "from": "merging",
              "to": "merge_refused",
              "label": "git refused to overwrite uncommitted changes",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.merge"
              ]
            },
            {
              "from": "rebasing",
              "to": "interrupted",
              "label": "recovery: rebase aborted",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.recoverGate"
              ]
            },
            {
              "from": "merging",
              "to": "interrupted",
              "label": "recovery: target never moved",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.recoverGate"
              ]
            },
            {
              "from": "verifying",
              "to": "refused",
              "label": "worktree or target could not be read",
              "refs": [
                "code:internal/subagents/gate.go#Supervisor.proposeMerge",
                "code:internal/subagents/gate.go#Supervisor.gateFailure"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/task",
      "level": "component",
      "kind": "package",
      "title": "task",
      "group": "runtime",
      "summary": {
        "text": "Defines the Kernel-owned contract for durable Tasks and Task Trees: their fields, validation, status rules, idempotent mutation identities, and the pure read model orchestration uses to dispatch work.",
        "refs": [
          "code:internal/task/task.go#Task",
          "code:internal/task/task.go#Service",
          "code:internal/task/plan.go#Plan.Readiness"
        ]
      },
      "does": [
        {
          "text": "Validates every mutation input before persistence: non-blank titles, priorities 0 to 5, YYYY-MM-DD due dates, positive revisions, idempotency keys of at most 256 bytes, and at most 100 children per decomposition.",
          "refs": [
            "code:internal/task/task.go#ValidateCreateInput",
            "code:internal/task/task.go#ValidateDecomposeInput",
            "code:internal/task/task.go#ValidateUpdateInput",
            "code:internal/task/task.go#ValidateIdempotencyKey",
            "code:internal/task/task.go#MaxDecomposeChildren"
          ]
        },
        {
          "text": "ValidateStatusTransition allows any change among open, in_progress and blocked and into completed or cancelled, while a completed or cancelled Task can only be reopened.",
          "refs": [
            "code:internal/task/task.go#ValidateStatusTransition"
          ]
        },
        {
          "text": "Every update is a compare-and-swap on the Task's revision, and a stale revision is a ConflictError.",
          "refs": [
            "code:internal/task/task.go#UpdateInput",
            "code:internal/task/task.go#ConflictError"
          ]
        },
        {
          "text": "Computes canonical SHA-256 request identities for create, decompose, update, claim and release, so a retried mutation under the same idempotency key is recognized and a different request under that key is refused.",
          "refs": [
            "code:internal/task/canonical.go#CanonicalCreateRequestSHA256",
            "code:internal/task/canonical.go#CanonicalUpdateRequestSHA256",
            "code:internal/task/canonical.go#IdempotencySHA256",
            "code:internal/task/task.go#ErrIdempotencyConflict"
          ]
        },
        {
          "text": "Plans Task Trees with dependencies between Tasks of one tree and expected paths, rejecting dependency cycles, including waiting on an ancestor or a descendant.",
          "refs": [
            "code:internal/task/plan.go#DependencyCycle",
            "code:internal/task/plan.go#ValidateDependencyIDs",
            "code:internal/task/plan.go#NormalizePaths"
          ]
        },
        {
          "text": "Plan.Readiness computes todo_ready's read model: dispatchable Tasks in batches whose paths do not conflict, sized by DispatchLimits.Capacity, plus in-flight builds, blocked Tasks with reasons, and active Tasks whose work looks finished.",
          "refs": [
            "code:internal/task/plan.go#Plan.Readiness",
            "code:internal/task/plan.go#DispatchLimits.Capacity",
            "code:internal/task/plan.go#Readiness"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the Task shape: ID, parent and root, sibling order, scope, title, description, priority, due date, result summary, dependencies, paths, status and revision.",
          "refs": [
            "code:internal/task/task.go#Task"
          ]
        },
        {
          "text": "Owns the audit vocabulary of Task events: operation, actor, session, run, revisions, outcome, diagnostic code, claim and grant IDs, and management override.",
          "refs": [
            "code:internal/task/task.go#Event",
            "code:internal/task/task.go#DiagnosticCode"
          ]
        },
        {
          "text": "Owns the delegated-access record: an AccessGrant at read, contribute or manage level for one child-session execution, ended rather than deleted.",
          "refs": [
            "code:internal/task/task.go#AccessGrant",
            "code:internal/task/task.go#AccessLevel"
          ]
        },
        {
          "text": "Persists nothing itself: eviedb implements Service and chooses identities and authority-bearing fields.",
          "refs": [
            "code:internal/task/task.go#Service",
            "code:internal/eviedb/tasks.go#Store.CreateGlobalTask"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Service is the focused persistence boundary first-party plugins consume: create, list, get, tree, update, decompose, claim, release and claim lookup.",
          "refs": [
            "code:internal/task/task.go#Service",
            "code:internal/plugins/todo.go#NewTodo"
          ]
        },
        {
          "text": "MutationAttribution carries actor, session, run, parent session, Workspace or project and the lease fence; the agent loop installs it with WithMutationAttribution just before a tool runs, and it is never a model argument.",
          "refs": [
            "code:internal/task/task.go#MutationAttribution",
            "code:internal/task/task.go#WithMutationAttribution",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "Typed errors such as InputError, NotFoundError, ConflictError, TransitionError and ClaimHeldError unwrap to package sentinels so callers can classify failures.",
          "refs": [
            "code:internal/task/task.go#InputError",
            "code:internal/task/task.go#TransitionError",
            "code:internal/task/task.go#ClaimHeldError",
            "code:internal/task/task.go#ErrClaimHeld"
          ]
        },
        {
          "text": "ReadyQuery's dispatch limits and turn source are trusted inputs from the runtime, never model arguments.",
          "refs": [
            "code:internal/task/plan.go#ReadyQuery"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "Task Focus is a trusted projection anchor (ListFilter.FocusID) that model-facing schemas never expose, and it grants no access.",
          "refs": [
            "code:internal/task/task.go#ListFilter",
            "term:Task Focus"
          ]
        },
        {
          "text": "A Task Claim needs full lease attribution (holder, token and generation) from a still-active execution, and eviedb refuses claim-requiring updates without one.",
          "refs": [
            "code:internal/task/task.go#ValidateClaimAttribution",
            "code:internal/task/task.go#ErrClaimRequired",
            "code:internal/eviedb/tasks_claims.go",
            "term:Task Claim"
          ]
        },
        {
          "text": "Delegated sessions can neither create Task Tree roots nor use management overrides.",
          "refs": [
            "code:internal/task/task.go#ErrRootCreationDenied",
            "code:internal/task/task.go#ErrManagementOverrideDenied"
          ]
        },
        {
          "text": "Paths only keep conflicting work apart; they never grant or fence file access, and omitting them means a Task may touch any file.",
          "refs": [
            "code:internal/task/plan.go#NormalizePaths"
          ]
        },
        {
          "text": "Readiness schedules nothing: the orchestrating model dispatches inside its own turn, and the harness re-checks the same rules when it admits a build worker.",
          "refs": [
            "code:internal/task/plan.go",
            "term:Ready Set"
          ]
        },
        {
          "text": "A dependency counts as done only when its Task is completed and its build is merged or has nothing to merge, so dependent work starts from the merged result.",
          "refs": [
            "code:internal/task/plan.go#DependencyDone",
            "term:Task Dependency"
          ]
        }
      ],
      "terms": [
        "Task",
        "Subtask",
        "Task Tree",
        "Task Focus",
        "Task Access Grant",
        "Task Claim",
        "Task Dependency",
        "Ready Set",
        "Kernel"
      ],
      "links": [
        "doc:cmd/evie/docs/active/subagents.decisions.md#2026-10-04--task-tree-orchestration-stage-13"
      ],
      "diagrams": [
        {
          "id": "task-status",
          "title": "Task status",
          "kind": "state",
          "summary": {
            "text": "The statuses a Task moves between; ValidateStatusTransition allows any change among active statuses and only reopening from a finished one.",
            "refs": [
              "code:internal/task/task.go#ValidateStatusTransition",
              "code:internal/task/task.go#Status"
            ]
          },
          "states": [
            {
              "id": "open",
              "label": "open",
              "refs": [
                "code:internal/task/task.go#StatusOpen"
              ]
            },
            {
              "id": "in_progress",
              "label": "in_progress",
              "refs": [
                "code:internal/task/task.go#StatusInProgress"
              ]
            },
            {
              "id": "blocked",
              "label": "blocked",
              "refs": [
                "code:internal/task/task.go#StatusBlocked"
              ]
            },
            {
              "id": "completed",
              "label": "completed",
              "refs": [
                "code:internal/task/task.go#StatusCompleted"
              ]
            },
            {
              "id": "cancelled",
              "label": "cancelled",
              "refs": [
                "code:internal/task/task.go#StatusCancelled"
              ]
            }
          ],
          "initial": "open",
          "transitions": [
            {
              "from": "open",
              "to": "in_progress",
              "label": "work starts",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "in_progress",
              "to": "open",
              "label": "back to open",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "open",
              "to": "blocked",
              "label": "marked blocked",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "blocked",
              "to": "open",
              "label": "unblocked",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "in_progress",
              "to": "blocked",
              "label": "marked blocked",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "blocked",
              "to": "in_progress",
              "label": "resumed",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "open",
              "to": "completed",
              "label": "completed; no active descendants",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition",
                "code:internal/task/task.go#ErrActiveDescendants"
              ]
            },
            {
              "from": "in_progress",
              "to": "completed",
              "label": "completed; no active descendants",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition",
                "code:internal/task/task.go#ErrActiveDescendants"
              ]
            },
            {
              "from": "blocked",
              "to": "completed",
              "label": "completed; no active descendants",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition",
                "code:internal/task/task.go#ErrActiveDescendants"
              ]
            },
            {
              "from": "open",
              "to": "cancelled",
              "label": "abandoned",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "in_progress",
              "to": "cancelled",
              "label": "abandoned",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "blocked",
              "to": "cancelled",
              "label": "abandoned",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "completed",
              "to": "open",
              "label": "reopened",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            },
            {
              "from": "cancelled",
              "to": "open",
              "label": "reopened",
              "refs": [
                "code:internal/task/task.go#ValidateStatusTransition"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/taskeval",
      "level": "component",
      "kind": "package",
      "title": "taskeval",
      "group": "runtime",
      "summary": {
        "text": "Derives a stable, content-free evidence baseline for evaluating durable Tasks; it does not score model quality or store reports.",
        "refs": [
          "code:internal/taskeval/evaluation.go#Derive",
          "code:internal/taskeval/evaluation.go#Report"
        ]
      },
      "does": [
        {
          "text": "Derive counts Tasks, completions (completed), abandonments (cancelled), accepted decompositions, rejected revision conflicts, and claim releases made by recovery or because the execution ended.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive"
          ]
        },
        {
          "text": "Joins each Task event to the episodic tool evidence of the same session and execution: the tool intent, the approval if any, and the outcome.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive",
            "code:internal/taskeval/evaluation.go#indexEpisodicEvidence"
          ]
        },
        {
          "text": "Groups Todo tool intents by canonical idempotency identity to count retries of the same request, duplicate attempts, and different requests reusing one key.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive",
            "code:internal/taskeval/evaluation.go#canonicalAttempt",
            "code:internal/plugins/todo_arguments.go#CanonicalTodoMutationAttempt"
          ]
        },
        {
          "text": "Sorts evidence links by session, execution and Task event, so the same input always yields the same report.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns the Report schema, version 1: counts plus EvidenceLink entries.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Report",
            "code:internal/taskeval/evaluation.go#ReportSchemaVersion"
          ]
        },
        {
          "text": "EvidenceLink holds correlation identities only and deliberately excludes prompts, reasoning, tool arguments and results, Task content and credentials.",
          "refs": [
            "code:internal/taskeval/evaluation.go#EvidenceLink"
          ]
        },
        {
          "text": "Stores nothing: the caller supplies the Input and receives the Report.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Input",
            "code:internal/taskeval/evaluation.go#Derive"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Derive(Input) (Report, error) is the whole interface; Input carries Tasks, Task events, episodic events and each session's Task scope.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive",
            "code:internal/taskeval/evaluation.go#Input"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "No production code imports it; its only caller is its own test, so the baseline is computed only under go test.",
          "refs": [
            "code:internal/taskeval/evaluation_test.go#TestDeriveReportsDeterministicTaskAndEpisodicEvidenceCounts"
          ]
        },
        {
          "text": "It reuses the Todo plugin's strict argument decoders to canonicalize attempts, so a change to the Todo wire contract changes what counts as a retry.",
          "refs": [
            "code:internal/plugins/todo_arguments.go#CanonicalTodoMutationAttempt",
            "code:internal/taskeval/evaluation.go#canonicalAttempt"
          ]
        },
        {
          "text": "Todo arguments that fail to decode are skipped rather than reported, and only a todo_add without a resolvable context scope is an error.",
          "refs": [
            "code:internal/plugins/todo_arguments.go#CanonicalTodoMutationAttempt"
          ]
        },
        {
          "text": "ConflictCount adds rejected revision conflicts and different requests under one key together, so the report cannot tell the two causes apart.",
          "refs": [
            "code:internal/taskeval/evaluation.go#Derive"
          ]
        }
      ],
      "terms": [
        "Task",
        "Task Claim",
        "Episodic Memory"
      ]
    },
    {
      "id": "pkg:internal/untrusted",
      "level": "component",
      "kind": "package",
      "title": "untrusted",
      "group": "runtime",
      "summary": {
        "text": "Fences text Evie did not author, such as web pages, transcripts, stored tool output and child agents' reports, so it reads as data rather than instructions.",
        "refs": [
          "code:internal/untrusted/frame.go#Frame"
        ]
      },
      "does": [
        {
          "text": "Escape puts a backslash before every occurrence of each marker prefix in the data; escaping prefixes rather than exact markers also neutralizes guessed numbered or relabelled variants.",
          "refs": [
            "code:internal/untrusted/frame.go#Escape"
          ]
        },
        {
          "text": "Delimiters numbers the begin and end markers (#1, #2 and so on) until neither occurs in the data, so the closing marker never appears inside the payload.",
          "refs": [
            "code:internal/untrusted/frame.go#Delimiters"
          ]
        },
        {
          "text": "Frame escapes the text and wraps it between a begin line built from a prefix and a label and a matching end line, numbered as Delimiters requires.",
          "refs": [
            "code:internal/untrusted/frame.go#Frame"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns no state and no data: it is three pure string functions.",
          "refs": [
            "code:internal/untrusted/frame.go"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Callers choose their own marker prefixes and labels: web content, research, explore and build child output, verification output, MCP results, workflow output and assignment data each have their own.",
          "refs": [
            "code:internal/tools/webfetch.go#frameUntrustedWeb",
            "code:internal/delegation/parent_view.go#frameChildOutput",
            "code:internal/delegation/merge.go#MergeProposal.ParentView",
            "code:internal/plugins/mcp.go",
            "code:internal/plugins/workflows.go",
            "code:internal/subagents/run.go#runAssignmentMessage"
          ]
        },
        {
          "text": "The tools package wraps it as escapeFrameDelimiters and collisionSafeFrame for its own frames.",
          "refs": [
            "code:internal/tools/youtube.go#escapeFrameDelimiters",
            "code:internal/tools/youtube.go#collisionSafeFrame"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "A frame labels text, it does not sanitize it: the model still reads the content, and only the frame and the reader's instructions say it is not authority.",
          "refs": [
            "code:internal/untrusted/frame.go#Frame",
            "code:internal/plugins/research.go#ResearchInstructions"
          ]
        },
        {
          "text": "It imports only the standard library, so tools, delegation, plugins and subagents share one implementation without import cycles.",
          "refs": [
            "code:internal/untrusted/frame.go"
          ]
        },
        {
          "text": "A label must not carry untrusted text: delegation labels child output only with the harness-generated execution ID.",
          "refs": [
            "code:internal/delegation/parent_view.go#frameChildOutput"
          ]
        }
      ]
    },
    {
      "id": "pkg:internal/repoinstructions",
      "level": "component",
      "kind": "package",
      "title": "repoinstructions",
      "group": "runtime",
      "summary": {
        "text": "Loads a Workspace folder's root AGENTS.md, or CLAUDE.md when AGENTS.md is absent, as bounded repository guidance, and renders it for the model.",
        "refs": [
          "code:internal/repoinstructions/load.go#Load",
          "code:internal/repoinstructions/load.go#Render"
        ]
      },
      "does": [
        {
          "text": "Load returns a snapshot whose status is disabled, unattached, missing, loaded or error, with the file name, exact text and SHA-256 when loaded.",
          "refs": [
            "code:internal/repoinstructions/load.go#Load"
          ]
        },
        {
          "text": "Reads only AGENTS.md, then CLAUDE.md, at the folder's root; it never looks above the root, in subfolders, or at files the guidance mentions.",
          "refs": [
            "code:internal/repoinstructions/load.go#Load",
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#outcome-and-decisions"
          ]
        },
        {
          "text": "Refuses symlinks, non-regular files, files over 32 KiB, and text that is not UTF-8 or contains NUL bytes, as visible errors rather than truncating or skipping.",
          "refs": [
            "code:internal/repoinstructions/load.go#Load",
            "code:internal/repoinstructions/load.go#MaxBytes"
          ]
        },
        {
          "text": "Opens the folder one path component at a time and compares file identities after each open, so a symlink swapped in during the walk cannot redirect the read.",
          "refs": [
            "code:internal/repoinstructions/load.go#openRootWithoutSymlinks"
          ]
        },
        {
          "text": "Render wraps loaded text as JSON with its folder and file, after a fixed preface saying the owner's explicit request takes precedence and the guidance cannot change runtime policy, tool permissions, approval requirements or memory scope.",
          "refs": [
            "code:internal/repoinstructions/load.go#Render"
          ]
        }
      ],
      "owns": [
        {
          "text": "Owns no storage: eviedb captures the snapshot once per root user turn in repository_instruction_snapshots, and every iteration and retry of that turn reuses it.",
          "refs": [
            "code:internal/eviedb/repository_instructions.go#SessionHistory.RepositoryInstructions",
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#outcome-and-decisions"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Load(WorkspaceID, WorkspaceFolder, RepositoryInstructionSettings) and Render(snapshot) are the whole surface; the snapshot type itself belongs to memory.",
          "refs": [
            "code:internal/repoinstructions/load.go#Load",
            "code:internal/repoinstructions/load.go#Render",
            "code:internal/memory/repository_instructions.go#RepositoryInstructionSnapshot"
          ]
        },
        {
          "text": "The agent loop asks the history for the turn's snapshot and places Render's text in its own user-role message after the system prompt and the working-folder note.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/context.go#ContextComposeInput.RepositoryInstructions"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "An error status stops an enabled turn before any provider call and leaves a durable turn_failed record; a missing file is normal and adds nothing.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/turn.go#Session.classifyRepositoryInstructionError",
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#outcome-and-decisions"
          ]
        },
        {
          "text": "Delegated workers never receive repository instructions: the loop skips them for a session with worker instructions.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#outcome-and-decisions"
          ]
        },
        {
          "text": "If the Workspace folder changes between the snapshot and the turn, the turn fails rather than apply another folder's guidance.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "text": "The setting is per Workspace and on by default; turning it off records a disabled snapshot rather than nothing.",
          "refs": [
            "code:internal/repoinstructions/load.go#Load",
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#outcome-and-decisions"
          ]
        },
        {
          "text": "Since October 12, 2026 the web UI has no per-turn control, so a turn's stored snapshot is no longer reachable from the browser even though it is still captured.",
          "refs": [
            "doc:cmd/evie/docs/active/repository-instructions.spec.md#per-turn-control-removal--october-12-2026"
          ]
        }
      ],
      "terms": [
        "Workspace",
        "Working Memory"
      ],
      "links": [
        "doc:cmd/evie/docs/active/repository-instructions.spec.md"
      ]
    }
  ],
  "edges": [
    {
      "from": "pkg:internal/agent",
      "to": "pkg:internal/eviedb",
      "via": "call",
      "label": "stores turn events and holds the turn lease through History and TurnOwnership",
      "refs": [
        "code:internal/agent/history.go#History",
        "code:internal/agent/ownership.go#TurnOwnership",
        "code:internal/eviedb/history.go#Store.BindHistory",
        "code:internal/eviedb/turn_owner.go#Store.BindTurnOwner"
      ]
    },
    {
      "from": "pkg:internal/plugins",
      "to": "pkg:internal/subagents",
      "via": "call",
      "label": "the Subagents plugin drives the Supervisor through ResearchSupervisor",
      "refs": [
        "code:internal/plugins/subagents.go#ResearchSupervisor",
        "code:cmd/evie/main.go#main"
      ]
    },
    {
      "from": "pkg:internal/plugins",
      "to": "pkg:internal/eviedb",
      "via": "call",
      "label": "stores desired enabled state through EnabledStateStore",
      "refs": [
        "code:internal/plugins/manager.go#EnabledStateStore",
        "code:internal/eviedb/plugin_configuration.go#Store.SetPluginEnabled"
      ]
    }
  ],
  "flows": [
    {
      "id": "chat-turn",
      "title": "Chat turn from the web UI",
      "summary": {
        "text": "One owner message travels from the browser through the agent loop, the model and any tools, and every step lands in durable history before the next one starts.",
        "refs": [
          "code:internal/agent/turn.go#Session.runOwnedTurn",
          "code:internal/web/serve.go#Server.handleChat"
        ]
      },
      "trigger": {
        "text": "The owner sends a message in an open chat; the web UI starts the turn detached and follows the chat's live feed.",
        "refs": [
          "code:internal/web/ui/src/store/useSession.ts",
          "code:internal/web/ui/src/api/stream.ts",
          "doc:cmd/evie/docs/active/multi-chat.spec.md#stage-3--a-live-feed-per-chat-and-rejoining-a-running-turn"
        ]
      },
      "participants": [
        "person:owner",
        "container:web-ui",
        "pkg:internal/web",
        "pkg:internal/agent",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/openrouter",
        "external:openrouter",
        "pkg:internal/tools"
      ],
      "steps": [
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "Follows the open chat's live feed (POST /api/chats/follow)",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/store/useSession.ts",
            "code:internal/web/ui/src/api/stream.ts",
            "code:internal/web/feed.go#Server.handleFollow"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Sends a message in the chat",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/store/useSession.ts"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/chat with detach: true",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/stream.ts",
            "code:internal/web/serve.go#Server.handleChat"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "202 started: the turn runs without this request",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/web/serve.go#Server.handleChat"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/agent",
          "label": "runTurn calls Session.Send with the followed approver",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/web/serve.go#Server.runTurn",
            "code:internal/agent/agent.go#Session.Send"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Acquires the session's turn lease",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/eviedb/turn_owner.go#SessionTurnOwner.Acquire"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends the owner's text as the root user_message",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/eviedb/history.go#SessionHistory.Append"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Inserts the event into events, fenced by the lease token",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/events.go#Store.AppendEventWithLease"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Captures the turn's repository-instruction snapshot and loads history",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/eviedb/repository_instructions.go#SessionHistory.RepositoryInstructions"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/agent",
          "label": "Composes the request: system prompt, notes, memory, history and pinned tool schemas",
          "kind": "note",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/context.go#ContextComposer.Compose"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends a context_snapshot event describing the request",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/openrouter",
          "label": "ChatStream through the consumer-owned Client",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.streamProviderAttempt",
            "code:internal/agent/agent.go#Client"
          ]
        },
        {
          "from": "pkg:internal/openrouter",
          "to": "external:openrouter",
          "label": "Streams the completion",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/web",
          "label": "Delta and Reasoning callbacks as text arrives",
          "kind": "async",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.streamProviderAttempt",
            "code:internal/web/events.go#sseEvents.Delta"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "Publishes delta events to the chat's feed",
          "kind": "async",
          "via": "http",
          "refs": [
            "code:internal/web/events.go#sseEvents.emit",
            "code:internal/web/feed.go#chatFeed.publish"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends the assistant_message event",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#assistantEventInput"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "For each tool call, appends a tool_intent and emits ToolCall",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#toolIntentInput"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/tools",
          "label": "Dispatches the call through the pinned Toolset",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Returns the tool's text, or an error result",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends the outcome, bounded to 100 KiB, and emits ToolResult",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/tool_results.go#admitToolResult"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/agent",
          "label": "Composes again from stored events until a response has no tool calls",
          "kind": "note",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/web",
          "label": "AssistantCommitted with the final message's stored event",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.emitCommittedAssistant",
            "code:internal/web/activity.go#sseEvents.AssistantCommitted"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Stops the heartbeat and releases the turn lease",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/agent/ownership.go#TurnOwnership"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/web",
          "label": "Send returns nil",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/agent/agent.go#Session.Send",
            "code:internal/web/serve.go#Server.runTurn"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "turn_done closes the turn on the feed",
          "kind": "async",
          "via": "http",
          "refs": [
            "code:internal/web/serve.go#Server.runTurn",
            "code:internal/web/events.go#sseEvents.TurnDone"
          ]
        }
      ]
    },
    {
      "id": "approval-handshake",
      "title": "Approval handshake for a gated tool call",
      "summary": {
        "text": "A gated tool call pauses the turn inside Send while the owner's answer arrives as a separate HTTP request; the decision is stored before the tool runs.",
        "refs": [
          "code:internal/web/approvals.go",
          "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
        ]
      },
      "trigger": {
        "text": "The model calls a tool that needs approval, such as edit_file or edit_db, or an MCP tool whose RequiresApproval check says ask.",
        "refs": [
          "code:internal/tools/registry.go#kernelToolsAfterYouTube",
          "code:internal/tools/registry.go#Tool.RequiresApproval"
        ]
      },
      "participants": [
        "pkg:internal/agent",
        "pkg:internal/tools",
        "pkg:internal/web",
        "container:web-ui",
        "person:owner",
        "pkg:internal/eviedb"
      ],
      "steps": [
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/tools",
          "label": "Dispatches the gated call",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Authorize preparation: is the turn's lease still held?",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/tools/registry.go#AuthorizePreparation",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/tools",
          "label": "Prepare builds the effect and, for edit_file, a full before-and-after preview",
          "kind": "note",
          "refs": [
            "code:internal/tools/registry.go#PreparedTool",
            "code:internal/tools/file.go#prepareEditFileTool"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Calls the Approver with name, arguments and preview",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/tools/registry.go#Approver",
            "code:internal/agent/turn.go#admitApproval"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/web",
          "label": "Enters tool_approval and calls the frontend's approver",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#admitApproval",
            "code:internal/web/feed.go#Server.followedApprover"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/web",
          "label": "Registers a pending approval under a random id, tagged with its chat",
          "kind": "note",
          "refs": [
            "code:internal/web/approvals.go#Server.newPending",
            "code:internal/web/approvals.go#Server.tagPending"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "approval_request on the chat's feed",
          "kind": "async",
          "via": "http",
          "refs": [
            "code:internal/web/events.go#sseEvents.ApprovalRequest"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "person:owner",
          "label": "Shows the approval card",
          "kind": "note",
          "refs": [
            "code:internal/web/ui/src/chat/ApprovalCard.tsx"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Approves or declines",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/chat/ApprovalCard.tsx",
            "code:internal/web/ui/src/store/useSession.ts"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/approve with the id and the answer",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/approve.ts",
            "code:internal/web/approvals.go#Server.handleApprove"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/web",
          "label": "claimPending removes the entry under the lock, so exactly one answer wins",
          "kind": "note",
          "refs": [
            "code:internal/web/approvals.go#Server.claimPending",
            "code:internal/web/approvals.go#Server.handleApprove"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "204, or 404 when the id expired or was already answered",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/web/approvals.go#Server.handleApprove",
            "code:internal/web/ui/src/api/approve.ts"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/agent",
          "label": "The approver returns the Decision and publishes approval_decided",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/web/feed.go#Server.followedApprover",
            "code:internal/web/approvals.go#sseEvents.approvalDecided"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/tools",
          "label": "Decision back to the dispatcher",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#admitApproval",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Observes the decision with its approval metadata",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/tools/registry.go#ApprovalObserver",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends the approval event, plus a semantic approval when the tool bound a proposal",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/agent.go#approvalEventInput",
            "code:internal/agent/agent.go#semanticApprovalEventInput"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "If approved, authorize execution against the lease",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/tools/registry.go#AuthorizeExecution",
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/tools",
          "label": "Runs the prepared effect; a decline or expiry becomes a fixed message instead",
          "kind": "note",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Returns the result",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Appends the outcome under the approval: succeeded or failed if approved, tool_cancelled otherwise",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/agent.go#toolOutcomeInput"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/web",
          "label": "With no tab following for two minutes, or when the turn stops, the approval expires and the model is told the owner never saw it",
          "kind": "note",
          "refs": [
            "code:internal/web/feed.go#approvalGrace",
            "code:internal/web/feed.go#Server.followedApprover",
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        }
      ]
    },
    {
      "id": "plugin-activation-and-composition",
      "title": "Plugin activation and session composition",
      "summary": {
        "text": "The owner enables a plugin, the Plugin Manager stores the intent and reconciles its lifecycle, and the next new session resolves its Agent Preset into a pinned Toolset and Composition Receipt.",
        "refs": [
          "code:internal/plugins/manager.go#Manager.SetEnabledWithTransition",
          "code:internal/plugins/preset.go#Manager.ResolvePresetContext"
        ]
      },
      "trigger": {
        "text": "The owner turns a plugin on in Management, then opens a new chat.",
        "refs": [
          "code:internal/web/ui/src/management/Management.tsx",
          "code:internal/web/ui/src/api/management.ts",
          "code:internal/web/ui/src/api/contextSessions.ts"
        ]
      },
      "participants": [
        "person:owner",
        "container:web-ui",
        "pkg:internal/web",
        "pkg:internal/plugins",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:cmd/evie",
        "pkg:internal/composition",
        "pkg:internal/agent"
      ],
      "steps": [
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Turns a plugin on in Management",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/management/Management.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/plugins/lifecycle with id and enabled",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/management.ts",
            "code:internal/web/management.go#Server.handlePluginLifecycle"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/plugins",
          "label": "SetEnabledWithTransition",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/web/management.go#Server.handlePluginLifecycle",
            "code:internal/plugins/manager.go#Manager.SetEnabledWithTransition"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/eviedb",
          "label": "Refreshes every plugin's desired state, then stores the new intent with a new revision",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/plugins/manager.go#Manager.refreshEnabledStateInOperation",
            "code:internal/eviedb/plugin_configuration.go#Store.SetPluginEnabled"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Upserts plugin_enabled_configuration",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/plugin_configuration.go#Store.SetPluginEnabled"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/plugins",
          "label": "Marks the plugin waiting and reconciles: stops in reverse dependency order, starts in dependency order",
          "kind": "note",
          "refs": [
            "code:internal/plugins/manager.go#Manager.applyEnabledIntentLocked",
            "code:internal/plugins/manager.go#Manager.reconcile"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/plugins",
          "label": "loading, Plugin.Start, then ready with the capabilities whose optional dependencies are available, or failed and rolled back",
          "kind": "note",
          "refs": [
            "code:internal/plugins/manager.go#Manager.startPlugin",
            "code:internal/plugins/manager.go#Manager.rollbackPlugin"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/web",
          "label": "LifecycleTransition: from, to, and each affected dependent",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/plugins/manager.go#LifecycleTransition",
            "code:internal/plugins/manager.go#Manager.SetEnabledWithTransition"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "container:web-ui",
          "label": "200 with the transition",
          "kind": "return",
          "via": "http",
          "refs": [
            "code:internal/web/management.go#Server.handlePluginLifecycle"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Opens a new chat in a Workspace",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/api/contextSessions.ts"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/context-sessions/select",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/contextSessions.ts",
            "code:internal/web/context_sessions.go#Server.handleContextSessionSelect"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:cmd/evie",
          "label": "SelectSession through the ContextSessionController interface",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/web/context_sessions.go#ContextSessionController",
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.SelectSession"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/eviedb",
          "label": "Reads the Workspace's default Agent Preset for its revision",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:internal/eviedb/workspace_presets.go#Store.WorkspaceDefaultPreset"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/plugins",
          "label": "ResolvePresetContext for that preset",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:internal/plugins/preset.go#Manager.ResolvePresetContext"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/plugins",
          "label": "Resolves each required and optional Capability against ready providers and contract ranges, then adds them to the base tools",
          "kind": "note",
          "refs": [
            "code:internal/plugins/preset.go#Manager.resolvePreset",
            "code:internal/plugins/preset.go#Manager.resolveCapabilityLocked"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/composition",
          "label": "Builds and validates the Composition Receipt",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/plugins/preset.go#Manager.resolvePreset",
            "code:internal/composition/receipt.go#Validate"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:cmd/evie",
          "label": "ResolvedComposition: Toolset, receipt and warnings",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/plugins/preset.go#ResolvedComposition"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/eviedb",
          "label": "Creates the Workspace session together with its receipt",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:internal/eviedb/sessions.go#Store.CreateWorkspaceSessionForChooserWithComposition"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/composition",
          "label": "Marshal validates the receipt before anything is written",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/eviedb/sessions.go#Store.createWorkspaceSession",
            "code:internal/composition/receipt.go#Marshal"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "One transaction inserts the session, checks the preset is allowed in the Workspace revision, and inserts the receipt",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/sessions.go#Store.createWorkspaceSession",
            "code:internal/eviedb/composition_receipts.go#insertCompositionReceipt"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:cmd/evie",
          "label": "bindSessionComposition keeps the created composition when the stored receipt matches it",
          "kind": "note",
          "refs": [
            "code:cmd/evie/session_composition.go#bindSessionComposition"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/agent",
          "label": "NewWithToolset with the resolved, immutable Toolset",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/agent/agent.go#NewWithToolset"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/web",
          "label": "Returns the opened session and its agent",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:cmd/evie/web_context_sessions.go#webContextSessionController.selectSession",
            "code:internal/web/context_sessions.go#OpenedContextSession"
          ]
        }
      ]
    },
    {
      "id": "subagent-run",
      "title": "Subagent run",
      "summary": {
        "text": "A parent turn's delegate_research call admits durable child attempts, runs each child as its own bounded agent Session, and returns the children's framed results as one tool result.",
        "refs": [
          "code:internal/subagents/supervisor.go#Supervisor.delegate",
          "code:internal/plugins/subagents.go#Subagents.ToolCapabilities"
        ]
      },
      "trigger": {
        "text": "The model in a parent turn calls delegate_research (delegate_explore and delegate_build follow the same path with their own worker preset).",
        "refs": [
          "code:internal/delegation/types.go#ToolName",
          "code:internal/subagents/supervisor.go#Supervisor.Explore",
          "code:internal/subagents/supervisor.go#Supervisor.Build"
        ]
      },
      "participants": [
        "pkg:internal/agent",
        "pkg:internal/tools",
        "pkg:internal/plugins",
        "pkg:internal/subagents",
        "pkg:internal/delegation",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/openrouter"
      ],
      "steps": [
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Parent stores the delegate_research tool_intent",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/delegation/types.go#AdmitsChildren"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/tools",
          "label": "Dispatches the call with the live InvocationContext",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/tools/registry.go#WithInvocationContext"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/plugins",
          "label": "Runs the Subagents capability's Execute",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
            "code:internal/plugins/subagents.go#Subagents.ToolCapabilities"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/plugins",
          "label": "Takes the parent from the InvocationContext, never from model arguments, and decodes the assignments strictly",
          "kind": "note",
          "refs": [
            "code:internal/plugins/subagents.go#invokingParent",
            "code:internal/plugins/subagents.go#decodeStrict"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/subagents",
          "label": "Delegate through ResearchSupervisor",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/plugins/subagents.go#ResearchSupervisor",
            "code:internal/subagents/supervisor.go#Supervisor.Delegate"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/delegation",
          "label": "Validates the batch against the Policy",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate",
            "code:internal/delegation/types.go#Policy.ValidateWorkerBatch"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "Reads the parent's receipt; the child's receipt copies the parent's pinned capability versions",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate",
            "code:internal/subagents/receipt.go#researchReceipt"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "Admits every child keyed by its idempotency key, each with its own child session",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate",
            "code:internal/eviedb/subagents.go#Store.AdmitSubagents"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Writes subagent_executions rows in the admitted state",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/subagents.go#Store.AdmitSubagents"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "StartSubagent takes the admitted-to-running edge when capacity allows",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.own",
            "code:internal/eviedb/subagent_execution.go#Store.StartSubagent"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/agent",
          "label": "Builds a delegated Session with the worker's Toolset, pinned instructions and a metered client, and calls Send with no approver",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.execute",
            "code:internal/agent/agent.go#NewDelegatedWithToolset"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/subagents",
          "label": "The child's ChatStream reaches the metered budget client",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.streamProviderAttempt",
            "code:internal/subagents/budget.go#budget.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/openrouter",
          "label": "Authorizes the attempt, checks the token and request-size budgets, and forwards the request",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/subagents/budget.go#budget.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "The watchdog re-authorizes the attempt every 100 ms",
          "kind": "async",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.watch",
            "code:internal/eviedb/subagent_execution.go#Store.AuthorizeSubagent"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/subagents",
          "label": "Send returns: succeeded, partial after a wrap-up, or a classified failure",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.execute",
            "code:internal/subagents/supervisor.go#executionFailure"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/eviedb",
          "label": "FinishSubagent settles the attempt and keeps its result",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.finish",
            "code:internal/eviedb/subagent_execution.go#Store.FinishSubagent"
          ]
        },
        {
          "from": "pkg:internal/subagents",
          "to": "pkg:internal/plugins",
          "label": "Returns the batch after re-checking delivery access",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/subagents/supervisor.go#Supervisor.delegate"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/delegation",
          "label": "ParentView frames each child's text as untrusted data",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/plugins/subagents.go#Subagents.ToolCapabilities",
            "code:internal/delegation/parent_view.go#Result.ParentView"
          ]
        },
        {
          "from": "pkg:internal/plugins",
          "to": "pkg:internal/tools",
          "label": "Returns the JSON result",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/plugins/subagents.go#Subagents.ToolCapabilities"
          ]
        },
        {
          "from": "pkg:internal/tools",
          "to": "pkg:internal/agent",
          "label": "Tool result back to the parent loop",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Parent appends the tool outcome; the next request shows the framed results to the model",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn"
          ]
        }
      ]
    }
  ]
}
);
