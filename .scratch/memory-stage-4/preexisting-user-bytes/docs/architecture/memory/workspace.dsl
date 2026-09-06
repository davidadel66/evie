workspace "Evie Memory Architecture" "Implemented Memory Stages 1-3" {
    !identifiers hierarchical

    model {
        david = person "David" "Evie's local owner; explicitly selects scope, approves mutations, and inspects memory."
        openrouter = softwareSystem "OpenRouter" "Remote conversational model transport." {
            tags "External"
        }

        evie = softwareSystem "Evie" "A local-first personal agent runtime with restart-safe episodic, working, and semantic memory." {
            web = container "Web UI" "Session, approval, context, and read-only Memory views." "React / TypeScript" {
                tags "Web"
            }
            runtime = container "Evie Runtime" "Owns sessions, turns, context assembly, tools, scope, approvals, and memory APIs." "Go" {
                adapters = component "CLI and HTTP Adapters" "Create or resume scoped sessions and translate owner actions into Kernel calls." "cmd/evie, internal/web"
                agent = component "Agent Session Loop" "Owns one fenced conversational turn and coordinates model and tool execution." "internal/agent"
                context = component "Working Context Engine" "Builds a bounded request projection and manages validated rolling compaction." "internal/agent/context.go, compaction.go"
                tools = component "Tool and Memory Plugin Adapters" "Expose focused capabilities while preserving approval, egress, and scope fences." "internal/tools, internal/plugins"
                contracts = component "Memory Domain Contracts" "Defines events, scopes, semantic objects, proposals, queries, and lifecycle values." "internal/memory"
                history = component "Episodic History Store" "Appends lease-fenced provider-neutral events and reconstructs accepted history." "internal/eviedb/events.go, history.go"
                semantic = component "Semantic Memory Kernel" "Prepares and atomically applies accepted operations; serves exact temporal graph reads." "internal/eviedb/semantic*.go"
                recovery = component "Replay and Recovery" "Verifies live projections against replay, quarantines divergent scopes, and performs fenced rebuilds." "internal/eviedb/semantic_replay.go"
            }
            database = container "Local Memory Store" "Canonical sessions, immutable events, accepted semantic operations, and deterministic query projections." "SQLite / WAL" {
                tags "Database"
            }
            evaluation = container "Conformance Evaluation" "Runs fixed, model-free scenarios and records semantic, latency, and storage baselines." "Go test harness" {
                tags "Supporting"
            }
        }

        david -> evie "Uses and governs"
        david -> evie.web "Browses sessions and memory" "HTTPS / local"
        david -> evie.runtime "Uses the REPL" "Terminal"
        evie.web -> evie.runtime "Calls scoped HTTP and streaming endpoints" "HTTP / SSE"
        evie.runtime -> openrouter "Sends approved, bounded conversational requests" "HTTPS"
        evie.runtime -> evie.database "Commits events, operations, and projections" "SQL"
        evie.evaluation -> evie.runtime "Exercises public memory seams"
        evie.evaluation -> evie.database "Uses fresh local databases and verifies replay"

        evie.runtime.adapters -> evie.runtime.agent "Creates and drives scoped sessions"
        evie.runtime.adapters -> evie.runtime.semantic "Invokes local inspection and approved mutations"
        evie.runtime.agent -> evie.runtime.context "Requests canonical working context"
        evie.runtime.agent -> evie.runtime.history "Appends and reloads accepted events"
        evie.runtime.agent -> evie.runtime.tools "Executes pinned capabilities"
        evie.runtime.tools -> evie.runtime.semantic "Adapts focused Memory capabilities"
        evie.runtime.context -> evie.runtime.history "Reads events and appends snapshots or compactions"
        evie.runtime.semantic -> evie.runtime.contracts "Implements typed memory contracts"
        evie.runtime.history -> evie.runtime.contracts "Persists typed event contracts"
        evie.runtime.recovery -> evie.runtime.semantic "Replays accepted operations"
        evie.runtime.history -> evie.database "Appends immutable events" "SQL transaction"
        evie.runtime.semantic -> evie.database "Atomically writes operations and graph projections" "SQL transaction"
        evie.runtime.recovery -> evie.database "Builds and compares shadow projections" "SQL transaction"
    }

    views {
        properties {
            "structurizr.title" "false"
            "structurizr.description" "false"
            "structurizr.metadata" "false"
        }

        systemContext evie "EvieMemoryContext" "Evie Memory system context" {
            include *
            autoLayout lr
            title "Evie Memory - System Context"
        }

        container evie "EvieMemoryContainers" "Runtime and persistence boundaries" {
            include *
            autoLayout lr
            title "Evie Memory - Runtime Containers"
        }

        component evie.runtime "EvieMemoryComponents" "Go package ownership and collaboration" {
            include *
            autoLayout lr
            title "Evie Memory - Runtime Components"
        }

        styles {
            element "Person" {
                shape Person
                background #17324d
                color #ffffff
                stroke #17324d
            }
            element "Software System" {
                shape RoundedBox
                background #245b87
                color #ffffff
                stroke #17324d
            }
            element "Container" {
                shape RoundedBox
                background #dcebf7
                color #17324d
                stroke #47789e
            }
            element "Component" {
                shape RoundedBox
                background #edf4f9
                color #17324d
                stroke #6f93ad
            }
            element "Database" {
                shape Cylinder
                background #e7f2ec
                color #204c38
                stroke #5c8f75
            }
            element "Web" {
                shape WebBrowser
            }
            element "External" {
                background #f1f2f4
                color #303740
                stroke #8b929a
            }
            element "Supporting" {
                background #f5efe0
                color #624d1d
                stroke #aa8b43
            }
            relationship "Relationship" {
                color #5c6873
                thickness 2
            }
        }
    }

    configuration {
        scope softwaresystem
    }
}
