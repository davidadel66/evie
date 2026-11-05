evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "memory",
  "nodes": [
    {
      "id": "pkg:internal/memory",
      "level": "component",
      "kind": "package",
      "title": "memory",
      "group": "memory",
      "summary": {
        "text": "The shared contract package for Evie's memory layers: the event, session, scope and turn-lease types of Episodic Memory, the content-free request records of Working Memory, and the typed proposals, results and queries of Semantic Memory, plus the deterministic rules every package that touches memory must apply the same way.",
        "refs": [
          "code:internal/memory/event.go#Event",
          "code:internal/memory/scope.go#ScopeContext",
          "code:internal/memory/semantic.go#RememberLiteralProposal",
          "code:internal/memory/standing.go#RenderStandingMemoryBlock"
        ]
      },
      "does": [
        {
          "text": "Defines the provider-neutral event envelope (EventInput in, Event out with session, sequence, parent and execution IDs) and its event types, from user_message and assistant_message through tool, approval and terminal events to context_snapshot, context_compacted and the in-turn context_turn_compacted; every conversation's Episodic Memory is a chain of these.",
          "refs": [
            "code:internal/memory/event.go#EventInput",
            "code:internal/memory/event.go#Event",
            "code:internal/memory/event.go#EventUserMessage",
            "code:internal/memory/turn_compaction.go#EventContextTurnCompacted",
            "term:Episodic Memory"
          ]
        },
        {
          "text": "Validates what those events carry: a terminal turn payload names its stage and classification, a rolling compaction summary must fill seven fixed section headings within 16 KiB, and a context snapshot is a content-free manifest of one exact provider request.",
          "refs": [
            "code:internal/memory/event.go#TurnTerminalPayload.Validate",
            "code:internal/memory/event.go#ValidateContextCompactionSummary",
            "code:internal/memory/event.go#ContextCompactedSummaryMaxBytes",
            "code:internal/memory/event.go#ContextSnapshotPayload.Validate"
          ]
        },
        {
          "text": "Defines who a conversation is and what it may see: Session, Workspace and Project records and the immutable ScopeContext a session derives for itself, always owned by the single local owner.",
          "refs": [
            "code:internal/memory/scope.go#Session",
            "code:internal/memory/scope.go#Session.ScopeContext",
            "code:internal/memory/scope.go#LocalOwnerID",
            "term:Context Scope"
          ]
        },
        {
          "text": "ResolveMemoryDestination turns a recommended applicability (everywhere, workspace, session, or none) into a concrete scope key derived from the session's own Workspace or project, so a scope ID never comes from model text.",
          "refs": [
            "code:internal/memory/destination.go#ResolveMemoryDestination",
            "code:internal/memory/destination.go#MemoryDestination"
          ]
        },
        {
          "text": "Defines the closed Semantic Memory vocabulary: six Typed Literal kinds, Predicate cardinality one or many, affirmed or denied Claim Polarity, append-only lifecycle states, source authority and evidence locators, error or changed corrections, three Graph Link relations and four lifecycle actions.",
          "refs": [
            "code:internal/memory/semantic.go#LiteralKind",
            "code:internal/memory/semantic.go#PredicateCardinality",
            "code:internal/memory/semantic.go#ClaimPolarity",
            "code:internal/memory/semantic.go#SemanticStateValue",
            "code:internal/memory/semantic.go#EvidenceLocator",
            "code:internal/memory/semantic.go#CorrectionMode",
            "code:internal/memory/semantic.go#GraphRelation",
            "code:internal/memory/semantic.go#MemoryLifecycleAction"
          ]
        },
        {
          "text": "Defines each Prepare and Apply pair the Kernel offers as request, proposal and result types (remember a literal, remember an Entity relation, correct, promote, link, change a lifecycle, set standing); every proposal carries its target scope, the full scope vector with prior Scope Revisions, generated IDs and the exact proposal and prepared hashes.",
          "refs": [
            "code:internal/memory/semantic.go#RememberLiteralRequest",
            "code:internal/memory/semantic.go#RememberLiteralProposal",
            "code:internal/memory/semantic.go#RememberEntityProposal",
            "code:internal/memory/semantic.go#CorrectClaimProposal",
            "code:internal/memory/semantic.go#PromotionProposal",
            "code:internal/memory/semantic.go#CreateGraphLinkProposal",
            "code:internal/memory/semantic.go#MemoryLifecycleProposal",
            "code:internal/memory/standing.go#ClaimStandingProposal"
          ]
        },
        {
          "text": "Owns the standing tier's rules: a closed ClaimKind whose default is standing for every kind but fact, the <standing-memory> block rendered from Global to Workspace or project to this conversation with one JSON line per memory, repeats and secret-flagged items left out, and the 500-token budgets that refuse an approval with \"memory full\".",
          "refs": [
            "code:internal/memory/standing.go#ClaimKind",
            "code:internal/memory/standing.go#DefaultStanding",
            "code:internal/memory/standing.go#RenderStandingMemoryBlock",
            "code:internal/memory/standing.go#SortStandingMemory",
            "code:internal/memory/standing.go#CheckStandingBudget",
            "code:internal/memory/standing.go#StandingMemoryFullError"
          ]
        },
        {
          "text": "Owns the explicit-request rule for proactive memory: HasExplicitMemoryCue scans the owner's message for a closed list of English cues (remember, save, note, keep in mind, don't forget and a few more), and only a cue plus the model's requested_by_owner shows the approval card; anything else is queued for review.",
          "refs": [
            "code:internal/memory/proposal_inbox.go#HasExplicitMemoryCue",
            "code:internal/plugins/memory_proactive.go#Memory.routeToInbox",
            "doc:cmd/evie/docs/active/memory.spec.md#proactive-memory"
          ]
        },
        {
          "text": "Defines the review inbox and background-pass records: QueuedMemoryProposal holds exactly one prepared remember, correction or retirement plus an optional Global version, and the background types describe a due conversation, its owner-message batch, the comparison pool, a run, its commit and the owner-facing status.",
          "refs": [
            "code:internal/memory/proposal_inbox.go#QueuedMemoryProposal",
            "code:internal/memory/proposal_inbox.go#MemoryProposalItem",
            "code:internal/memory/proposal_inbox.go#MemoryProposalApproval",
            "code:internal/memory/background.go#BackgroundBatch",
            "code:internal/memory/background.go#BackgroundPool",
            "code:internal/memory/background.go#BackgroundCommit",
            "code:internal/memory/background.go#BackgroundStatus"
          ]
        },
        {
          "text": "Defines retrieval: RetrievalQuery (requests only, never authority), sourced RetrievalEvidence and its reference form, result statuses that tell an empty search from a failed or exhausted one, and the content-free RetrievalReceipt a context snapshot records.",
          "refs": [
            "code:internal/memory/retrieval.go#RetrievalQuery",
            "code:internal/memory/retrieval.go#RetrievalEvidence",
            "code:internal/memory/retrieval.go#RetrievalResult",
            "code:internal/memory/retrieval_receipt.go#RetrievalReceipt",
            "term:Conversation Excerpt"
          ]
        },
        {
          "text": "FindRecallTimeReference reads one explicit past time in a request (\"yesterday\", \"last week\", \"three days ago\", \"in March\", \"2026-09-14\") without a model, and Window resolves it to a half-open range in the owner's time zone; retrieval only prefers evidence inside it and never widens what a reader may see.",
          "refs": [
            "code:internal/memory/recall_time.go#FindRecallTimeReference",
            "code:internal/memory/recall_time.go#RecallTimeReference.Window"
          ]
        },
        {
          "text": "Defines the Stage 4 compiler contracts: a pinned Compiler Generation whose identity hashes the model, tokenizer and template digests and every policy, the sealed CompilerRequest window of selected sources and recorded omissions, untrusted ExtractorCandidate output and the stored Memory Candidate.",
          "refs": [
            "code:internal/memory/compiler.go#CompilerGeneration",
            "code:internal/memory/compiler_contract.go#CompilerGenerationIdentity",
            "code:internal/memory/compiler.go#CompilerRequest",
            "code:internal/memory/compiler.go#ExtractorCandidate",
            "code:internal/memory/compiler.go#MemoryCandidate",
            "term:Compiler Generation",
            "term:Memory Candidate"
          ]
        },
        {
          "text": "DecodeCompilerJSON is the strict decoder both the compiler and the background pass use for model output: one object, no duplicate or unknown keys, every field present under its canonical name, at most 128 KiB.",
          "refs": [
            "code:internal/memory/compiler_contract.go#DecodeCompilerJSON",
            "code:internal/memory/compiler_contract.go#CompilerMaxBytes",
            "code:internal/memorybackground/prompt.go#parseExtraction"
          ]
        },
        {
          "text": "Defines owner candidate review: a CandidateRef pins one interpretation and review revision, a ReviewPreview binds a complete atomic group by hash, and an accepted review becomes an OwnerReviewOperation, Semantic Operation schema version 6.",
          "refs": [
            "code:internal/memory/candidate_review.go#CandidateRef",
            "code:internal/memory/candidate_review.go#ReviewPreview",
            "code:internal/memory/candidate_review.go#OwnerReviewOperation",
            "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview"
          ]
        },
        {
          "text": "HasRetrievalSecret is the one secret fence (private keys, sk-, GitHub and AWS key shapes, password and token assignments) that retrieval results, the standing block, background candidates and inbox proposals all pass through.",
          "refs": [
            "code:internal/memory/retrieval_render.go#HasRetrievalSecret",
            "code:internal/memory/standing.go#RenderStandingMemoryBlock",
            "code:internal/memorybackground/prompt.go#usableCandidate",
            "code:internal/agent/retrieval.go#retrievalTurn.search"
          ]
        }
      ],
      "owns": [
        {
          "text": "Holds no tables and no handles: every type here is persisted by eviedb, and the only disk access is CanonicalProjectRoot resolving a project folder through symlinks.",
          "refs": [
            "code:internal/memory/scope.go#CanonicalProjectRoot",
            "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
            "code:internal/eviedb/events.go#Store.AppendEventWithLease"
          ]
        },
        {
          "text": "Owns the canonical Claim text used wherever memories are shown or compared, \"subject — predicate: value\" with a Denied: prefix for denied polarity, never the source's own words.",
          "refs": [
            "code:internal/memory/background.go#BackgroundClaimText",
            "code:internal/memory/standing.go#StandingMemoryItem",
            "code:internal/eviedb/semantic_standing.go#standingClaimText"
          ]
        },
        {
          "text": "Owns the content-free receipts each context_snapshot stores: the standing block's Claim IDs, byte count and SHA-256, and the retrieval receipt's evidence references and status.",
          "refs": [
            "code:internal/memory/standing.go#StandingMemoryReceipt",
            "code:internal/memory/standing.go#NewStandingMemoryReceipt",
            "code:internal/memory/event.go#ContextSnapshotPayload",
            "code:internal/memory/retrieval_receipt.go#RetrievalReceipt"
          ]
        },
        {
          "text": "Owns the versioned constants other packages record: compaction prompt versions compaction-v2 and turn-compaction-v1, compiler policies such as owner-assertions-v1, and the token arithmetic that converts budgets with a calibrated bytes-per-token ratio.",
          "refs": [
            "code:internal/memory/event.go#ContextCompactionPromptVersion",
            "code:internal/memory/turn_compaction.go#ContextTurnCompactionPromptVersion",
            "code:internal/memory/compiler_contract.go#CompilerPolicyVersion",
            "code:internal/memory/event.go#ContextBudgetBytes",
            "code:internal/memory/event.go#ContextTokenEstimate"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "It declares no interfaces of its own: consumers own the seams and speak these types, as the Memory Plugin's SemanticMemoryKernel, the agent's SemanticMemory and MemoryRetrieval, and the background pass's Store do, and *eviedb.Store implements them all.",
          "refs": [
            "code:internal/plugins/memory.go#SemanticMemoryKernel",
            "code:internal/agent/semantic_memory.go#SemanticMemory",
            "code:internal/agent/retrieval.go#MemoryRetrieval",
            "code:internal/memorybackground/pass.go#Store"
          ]
        },
        {
          "text": "TurnLease, with its holder, fencing token and generation, is the write fence every Apply and every background commit takes; UnexpiredAt only reads a snapshot and never proves durable ownership.",
          "refs": [
            "code:internal/memory/lease.go#TurnLease",
            "code:internal/memory/lease.go#TurnLease.UnexpiredAt",
            "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
          ]
        },
        {
          "text": "ApprovalPayload binds an owner decision to the proposal and prepared hashes, and ProposalHashes carries the same proposal with \"Always applies\" switched, so the owner can flip the switch and still approve exactly what was shown.",
          "refs": [
            "code:internal/memory/event.go#ApprovalPayload",
            "code:internal/memory/standing.go#ProposalHashes",
            "code:internal/plugins/memory.go#preparedSemanticMutation"
          ]
        },
        {
          "text": "MemoryProposalApproval, MemoryProposalResult and BackgroundStatus are the JSON contracts of the web Memory review area and `evie memory-background status`.",
          "refs": [
            "code:internal/memory/proposal_inbox.go#MemoryProposalApproval",
            "code:internal/memory/proposal_inbox.go#MemoryProposalResult",
            "code:internal/memory/background.go#BackgroundStatus",
            "code:internal/web/memory_proposals.go#MemoryProposalInbox",
            "command:evie memory-background status"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "The package is broader than its name: Workspace explore, build and merge-check settings, project root resolution and terminal-safe display labels live here because sessions and scopes do.",
          "refs": [
            "code:internal/memory/scope.go#WorkspaceExplore",
            "code:internal/memory/scope.go#WorkspaceBuild",
            "code:internal/memory/scope.go#BuildVerify",
            "code:internal/memory/title.go#WorkspaceDisplayLabel"
          ]
        },
        {
          "text": "A Claim accepted before the standing tier has no classification and reads as a fact that is not standing; nothing is rewritten, and an approved memory_set_standing is the only way to make that Claim standing.",
          "refs": [
            "code:internal/memory/standing.go#ClaimKind",
            "code:internal/memory/standing.go#ClaimStandingRequest",
            "doc:cmd/evie/docs/active/memory.spec.md#standing-memory"
          ]
        },
        {
          "text": "The standing budget counts only what the approving session sees, so another conversation's session-scoped standing memories are not counted when a Global memory is approved, and a later conversation can render slightly over 500 tokens; nothing is ever truncated.",
          "refs": [
            "code:internal/memory/standing.go#CheckStandingBudget",
            "code:internal/eviedb/semantic_standing.go#Store.enforceStandingBudget",
            "doc:cmd/evie/docs/active/memory.spec.md#standing-memory"
          ]
        },
        {
          "text": "A cue alone is not a request: \"Do you remember…?\" contains \"remember\", so the model must also set requested_by_owner; a requested_by_owner the message does not support goes to the inbox with reason no_explicit_cue.",
          "refs": [
            "code:internal/memory/proposal_inbox.go#HasExplicitMemoryCue",
            "code:internal/memory/proposal_inbox.go#MemoryProposalNoExplicitCue",
            "code:internal/plugins/memory_proactive.go#Memory.routeToInbox"
          ]
        },
        {
          "text": "RetrievalQuery's Relevance and RecallTime are set only by Automatic Recall, and its anchor, covered and refresh references have no JSON form, so model arguments cannot set them; the turn clears and refills the anchor and covered set itself before every search.",
          "refs": [
            "code:internal/memory/retrieval.go#RetrievalQuery",
            "code:internal/agent/retrieval.go#retrievalTurn.search"
          ]
        },
        {
          "text": "Hard Erasure is not implemented: the semantic projection tables are append-only by trigger, and Retirement is the only way to take knowledge out of ordinary recall, as ADR 0050 planned.",
          "refs": [
            "code:internal/eviedb/semantic_replay.go#semanticProjectionAppendOnlyTriggerSQL",
            "adr:0050",
            "term:Hard Erasure",
            "term:Retirement"
          ]
        },
        {
          "text": "Stage evolution, compared with the original drawing: the original shows three stages and says future retrieval \"may supply sourced context; Stage 3 does not inject it\"; today the compiler, the background pass, the review inbox, retrieval, Automatic Recall, the standing block and session episodes all exist; the standing block reaches every request and Automatic Recall adds evidence at the start of each turn.",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/standing_memory.go#Session.standingMemory",
            "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic",
            "doc:docs/architecture/memory/stage-evolution.d2"
          ]
        },
        {
          "text": "Turn context flow, compared with the original: each request now also carries the working-folder note, repository instructions, the standing block and a memory evidence message before the active root; compaction may be in-turn as well as rolling; a provider context-length rejection gets one compact-and-retry; and a committed compaction is offered to the background pass.",
          "refs": [
            "code:internal/agent/context.go#ContextComposer.projectAtStart",
            "code:internal/memory/turn_compaction.go#EventContextTurnCompacted",
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/agent/compaction_flush.go#Session.requestCompactionFlush",
            "doc:docs/architecture/memory/turn-context-flow.d2"
          ]
        },
        {
          "text": "Semantic write path, compared with the original: the original has one entry, an explicit owner request shown on a card; today three reach Apply (the approval card, the review inbox for proactive and background proposals, and owner review of compiler candidates), and an inbox approval appends its approval event and runs Apply as two separate writes under the source conversation's lease.",
          "refs": [
            "code:internal/plugins/memory_proactive.go#Memory.routeToInbox",
            "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal",
            "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview",
            "doc:docs/architecture/memory/semantic-write-path.d2"
          ]
        },
        {
          "text": "Apply re-reads the stored approval event and compares its hashes only for new Graph Links, lifecycle changes that touch a Graph Link (schema version 5), standing changes and Promotions; remember, correct and ordinary retire or restore operations rely on their caller having stored the approval first, and check the lease, hashes, idempotency key, source event and scope vector instead.",
          "refs": [
            "code:internal/eviedb/semantic_graph.go#validateExactSemanticApproval",
            "code:internal/eviedb/semantic_graph.go#Store.ApplyCreateGraphLink",
            "code:internal/eviedb/semantic_lifecycle.go#Store.ApplyMemoryLifecycle",
            "code:internal/eviedb/semantic_lifecycle.go#lifecycleIncludesGraphLink",
            "code:internal/eviedb/semantic_standing.go#Store.ApplyClaimStanding",
            "code:internal/eviedb/semantic_promotion.go#Store.ApplyPromotion",
            "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
            "code:internal/eviedb/semantic_correction.go#Store.ApplyCorrectClaim"
          ]
        },
        {
          "text": "Semantic domain model, compared with the original and the Stage 3 spec: both describe cached current-state columns as a disposable read optimization, but the projection tables are append-only by trigger, so a lifecycle or eligibility column keeps its creation value and current state comes from the latest semantic_state_events row; Claim classifications and the projection quarantine are tables the original does not show.",
          "refs": [
            "code:internal/eviedb/semantic_replay.go#semanticProjectionAppendOnlyTriggerSQL",
            "code:internal/eviedb/semantic_lifecycle.go#loadLatestState",
            "code:internal/eviedb/semantic_standing.go#loadStandingMemory",
            "doc:docs/architecture/memory/semantic-domain-model.d2",
            "doc:cmd/evie/docs/active/semantic-memory-stage-3.spec.md#implementation-decisions"
          ]
        },
        {
          "text": "Graph Link endpoints and state events name their object by kind and ID, so they have no foreign keys and the data diagram draws no line from them to the Claims, Entities or Source Links they point at.",
          "refs": [
            "table:evie.semantic_graph_links",
            "table:evie.semantic_state_events",
            "code:internal/eviedb/semantic_graph.go#requireActiveGraphEndpoint"
          ]
        },
        {
          "text": "Scope and promotion, compared with the original: it says model text cannot select scope and only explicit Promotion widens reuse; today a remember may carry a destination recommendation that approval confirms, so a Workspace conversation can save straight to Global, the background pass offers a \"Save everywhere instead\" version, and earlier conversations of the same Context Scope are searchable as excerpts (ADR 0066).",
          "refs": [
            "code:internal/memory/destination.go#ResolveMemoryDestination",
            "code:internal/eviedb/semantic_destination.go#validateRecommendedDestination",
            "code:internal/memorybackground/pass.go#Runner.prepareEverywhere",
            "code:internal/eviedb/retrieval_conversation.go#Store.readConversationPass",
            "adr:0066",
            "doc:docs/architecture/memory/scope-promotion.d2"
          ]
        },
        {
          "text": "Temporal lifecycle, compared with the original: unsupported is drawn as a lifecycle state, but it is never stored; it is computed at read time for an active Claim with no eligible Source Link. A superseded Claim cannot be restored, and a changed correction leaves the old Claim answering for valid times before the effective time while an error correction hides it.",
          "refs": [
            "code:internal/eviedb/semantic_lifecycle.go#Store.InspectSemanticObjectAtScopeAndTime",
            "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
            "code:internal/eviedb/semantic_correction.go#Store.inspectClaimsInScope",
            "doc:docs/architecture/memory/temporal-lifecycle.d2"
          ]
        },
        {
          "text": "Replay and recovery, compared with the original and ADR 0060: the shadow is a full VACUUM INTO copy of the database in a private temporary folder, replayed by calling the production Apply methods under a replay lease; startup checks quarantine scopes too; and a successful rebuild clears every quarantine row, not only the scopes it found divergent.",
          "refs": [
            "code:internal/eviedb/semantic_replay.go#Store.buildSemanticProjectionShadow",
            "code:internal/eviedb/semantic_replay.go#replayLease",
            "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup",
            "code:internal/eviedb/semantic_replay.go#Store.swapSemanticProjection",
            "adr:0060",
            "doc:docs/architecture/memory/replay-recovery.d2"
          ]
        },
        {
          "text": "The original Structurizr model predates the compiler, retrieval, the background pass and the standing tier; its \"Conformance Evaluation\" container is today the memoryeval package and the tests that load it, and its Memory Plugin adapters live in internal/plugins.",
          "refs": [
            "doc:docs/architecture/memory/workspace.dsl",
            "pkg:internal/memoryeval",
            "code:internal/plugins/memory.go#Memory"
          ]
        }
      ],
      "terms": [
        "Episodic Memory",
        "Working Memory",
        "Semantic Memory",
        "Automatic Recall",
        "Conversation Excerpt",
        "Context Scope",
        "Entity",
        "Alias",
        "Claim",
        "Unsupported Claim",
        "Graph Link",
        "Predicate",
        "Claim Polarity",
        "Predicate Cardinality",
        "Typed Literal",
        "Scope Revision",
        "Semantic Operation",
        "Memory Operation Proposal",
        "Memory Candidate",
        "Compiler Generation",
        "Compilation Coverage",
        "Valid Time",
        "Transaction Time",
        "Retirement",
        "Hard Erasure",
        "Promotion",
        "Memory Inspection",
        "Kernel",
        "Action Approval"
      ],
      "diagrams": [
        {
          "id": "memory-stage-evolution",
          "title": "Memory stage evolution",
          "kind": "graph",
          "summary": {
            "text": "How Evie's memory grew, redrawn from today's code: the original three stages (Episodic, Working and Semantic Memory), then the compiler and the background pass with their review paths, retrieval and Automatic Recall, and the architecture plan's standing block and session episodes; the original drawing stops at Stage 3.",
            "refs": [
              "doc:cmd/evie/docs/active/memory.spec.md#staged-build-order",
              "doc:docs/memory-architecture.md#plan-stages-each-one-reviewable-change",
              "doc:docs/architecture/memory/stage-evolution.d2"
            ]
          },
          "lanes": [
            {
              "id": "s1",
              "title": "Stage 1 · Episodic Memory"
            },
            {
              "id": "s2",
              "title": "Stage 2 · Working Memory"
            },
            {
              "id": "s3",
              "title": "Stage 3 · Semantic Memory"
            },
            {
              "id": "s4",
              "title": "Stage 4 · Compiler and background pass"
            },
            {
              "id": "s5",
              "title": "Stage 5 · Retrieval and recall"
            },
            {
              "id": "plan",
              "title": "Architecture plan tiers"
            }
          ],
          "nodes": [
            {
              "id": "sessions",
              "label": "Sessions with an immutable Context Scope",
              "lane": "s1"
            },
            {
              "id": "leases",
              "label": "Durable turn lease and fencing token",
              "lane": "s1"
            },
            {
              "id": "events",
              "label": "Append-only provider-neutral events",
              "node": "container:evie-db",
              "lane": "s1"
            },
            {
              "id": "resume",
              "label": "Restart from accepted events",
              "lane": "s1"
            },
            {
              "id": "composer",
              "label": "Canonical bounded request composer",
              "node": "pkg:internal/agent",
              "lane": "s2"
            },
            {
              "id": "projection",
              "label": "Tool-result admission and projection",
              "lane": "s2"
            },
            {
              "id": "compaction",
              "label": "Rolling and in-turn compaction",
              "lane": "s2"
            },
            {
              "id": "snapshots",
              "label": "Content-free context snapshots",
              "lane": "s2"
            },
            {
              "id": "operations",
              "label": "Accepted Semantic Operations",
              "lane": "s3"
            },
            {
              "id": "graph",
              "label": "Temporal Entity and Claim projection",
              "lane": "s3"
            },
            {
              "id": "scope",
              "label": "Exact scopes, destinations and Promotion",
              "lane": "s3"
            },
            {
              "id": "inspect",
              "label": "Current, historical and provenance reads",
              "lane": "s3"
            },
            {
              "id": "recovery",
              "label": "Replay, quarantine and rebuild",
              "lane": "s3"
            },
            {
              "id": "compiler",
              "label": "Compiler: Memory Candidates (opt-in, local model)",
              "node": "container:compiler-host",
              "lane": "s4"
            },
            {
              "id": "candidateReview",
              "label": "Owner candidate review",
              "lane": "s4"
            },
            {
              "id": "background",
              "label": "Background pass",
              "node": "pkg:internal/memorybackground",
              "lane": "s4"
            },
            {
              "id": "inbox",
              "label": "Review inbox",
              "lane": "s4"
            },
            {
              "id": "index",
              "label": "Derived full-text and dense indexes",
              "node": "container:retrieval-maintenance",
              "lane": "s5"
            },
            {
              "id": "recall",
              "label": "Automatic Recall",
              "lane": "s5"
            },
            {
              "id": "search",
              "label": "Model-directed memory and conversation search",
              "lane": "s5"
            },
            {
              "id": "standing",
              "label": "Standing memory block",
              "lane": "plan"
            },
            {
              "id": "proactive",
              "label": "Proactive in-turn proposals",
              "lane": "plan"
            },
            {
              "id": "episodes",
              "label": "Session episodes",
              "lane": "plan"
            }
          ],
          "edges": [
            {
              "from": "sessions",
              "to": "leases",
              "label": "one live turn per session",
              "refs": [
                "code:internal/memory/lease.go#TurnLease",
                "code:internal/eviedb/turn_owner.go#SessionTurnOwner.Acquire"
              ]
            },
            {
              "from": "leases",
              "to": "events",
              "label": "every append is fenced",
              "refs": [
                "code:internal/eviedb/events.go#Store.AppendEventWithLease"
              ]
            },
            {
              "from": "events",
              "to": "resume",
              "label": "history reconstructed after restart",
              "refs": [
                "code:internal/agent/request_history.go#Session.turnEvents",
                "code:internal/agent/history.go#History"
              ]
            },
            {
              "from": "events",
              "to": "composer",
              "label": "selected into each request",
              "refs": [
                "code:internal/agent/context.go#ContextComposer.projectAtStart"
              ]
            },
            {
              "from": "composer",
              "to": "projection",
              "label": "old tool results bounded",
              "refs": [
                "code:internal/agent/tool_results.go#admitToolResult",
                "code:internal/agent/context.go#ContextComposer.projectAtStart"
              ]
            },
            {
              "from": "composer",
              "to": "compaction",
              "label": "pressure decides",
              "refs": [
                "code:internal/agent/automatic_compaction.go#selectAutomaticCompactionFrom"
              ]
            },
            {
              "from": "composer",
              "to": "snapshots",
              "label": "one per request",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#ContextSnapshotPayload"
              ]
            },
            {
              "from": "events",
              "to": "operations",
              "label": "cited as exact evidence",
              "refs": [
                "code:internal/memory/semantic.go#SemanticSource",
                "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral"
              ]
            },
            {
              "from": "operations",
              "to": "graph",
              "label": "deterministic projection",
              "refs": [
                "code:internal/eviedb/semantic.go#recordAcceptedSemanticOperation"
              ]
            },
            {
              "from": "scope",
              "to": "operations",
              "label": "scope vector checked at Apply",
              "refs": [
                "code:internal/eviedb/semantic.go#validateSemanticScopeVector"
              ]
            },
            {
              "from": "graph",
              "to": "inspect",
              "label": "exact pinned reads",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#Store.InspectClaims",
                "code:internal/eviedb/semantic_graph.go#Store.TraverseSemanticNeighborhood"
              ]
            },
            {
              "from": "operations",
              "to": "recovery",
              "label": "replayed into a shadow",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.VerifySemanticProjection"
              ]
            },
            {
              "from": "events",
              "to": "compiler",
              "label": "selected ranges",
              "refs": [
                "code:internal/memory/compiler.go#CompilationSelection",
                "code:internal/eviedb/compiler_reconciliation.go#Store.ReconcileCompilerEvidence"
              ]
            },
            {
              "from": "compiler",
              "to": "candidateReview",
              "label": "unaccepted candidates",
              "refs": [
                "code:internal/eviedb/candidate_review_prepare.go#Store.PrepareOwnerCandidateReview"
              ]
            },
            {
              "from": "candidateReview",
              "to": "operations",
              "label": "owner_candidate_review",
              "refs": [
                "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview"
              ]
            },
            {
              "from": "events",
              "to": "background",
              "label": "owner messages since the cursor",
              "refs": [
                "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundBatch"
              ]
            },
            {
              "from": "compaction",
              "to": "background",
              "label": "compaction flush",
              "refs": [
                "code:internal/agent/compaction_flush.go#Session.requestCompactionFlush"
              ]
            },
            {
              "from": "background",
              "to": "inbox",
              "label": "exact prepared proposals",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.commit"
              ]
            },
            {
              "from": "proactive",
              "to": "inbox",
              "label": "unrequested remembers",
              "refs": [
                "code:internal/plugins/memory_proactive.go#Memory.routeToInbox"
              ]
            },
            {
              "from": "inbox",
              "to": "operations",
              "label": "owner approval runs Apply",
              "refs": [
                "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal"
              ]
            },
            {
              "from": "events",
              "to": "index",
              "label": "messages indexed",
              "refs": [
                "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex"
              ]
            },
            {
              "from": "graph",
              "to": "index",
              "label": "Claims indexed",
              "refs": [
                "code:internal/eviedb/retrieval_index.go#Store.refreshMemoryClaim",
                "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex"
              ]
            },
            {
              "from": "index",
              "to": "recall",
              "label": "scoped search",
              "refs": [
                "code:internal/eviedb/retrieval.go#Store.SearchMemory"
              ]
            },
            {
              "from": "index",
              "to": "search",
              "label": "scoped search",
              "refs": [
                "code:internal/eviedb/retrieval.go#Store.SearchMemory",
                "code:internal/agent/retrieval.go#retrievalTurn.searchForTool"
              ]
            },
            {
              "from": "recall",
              "to": "composer",
              "label": "EVIE_MEMORY_DATA before the active root",
              "refs": [
                "code:internal/agent/retrieval.go#retrievalTurn.renderProjection",
                "code:internal/agent/context.go#ContextComposer.projectAtStart"
              ]
            },
            {
              "from": "graph",
              "to": "standing",
              "label": "active standing Claims",
              "refs": [
                "code:internal/eviedb/semantic_standing.go#loadStandingMemory"
              ]
            },
            {
              "from": "standing",
              "to": "composer",
              "label": "rendered on every request",
              "refs": [
                "code:internal/agent/standing_memory.go#Session.standingMemory",
                "code:internal/memory/standing.go#RenderStandingMemoryBlock"
              ]
            },
            {
              "from": "compaction",
              "to": "episodes",
              "label": "latest rolling summary per session",
              "refs": [
                "code:internal/eviedb/session_episodes.go#appendSessionEpisode"
              ]
            },
            {
              "from": "episodes",
              "to": "search",
              "label": "memory_search_episodes",
              "refs": [
                "code:internal/eviedb/session_episodes.go#Store.SearchSessionEpisodes"
              ]
            }
          ]
        },
        {
          "id": "memory-turn-context-flow",
          "title": "Turn context flow",
          "kind": "sequence",
          "summary": {
            "text": "How one owner turn builds every provider request from durable state, redrawn from today's turn loop: lease, root user_message, history reload, standing memory and Automatic Recall, composition and compaction, a content-free snapshot, a lease-authorized provider call, and the fenced tool lifecycle, repeated until the model answers without tools.",
            "refs": [
              "code:internal/agent/agent.go#Session.Send",
              "code:internal/agent/turn.go#Session.runOwnedTurn",
              "doc:docs/architecture/memory/turn-context-flow.d2"
            ]
          },
          "trigger": {
            "text": "The owner sends a message in a REPL or web conversation; the frontend calls Session.Send.",
            "refs": [
              "code:internal/agent/agent.go#Session.Send",
              "code:internal/web/serve.go#Server.runTurn"
            ]
          },
          "participants": [
            "person:owner",
            "pkg:internal/agent",
            "pkg:internal/eviedb",
            "pkg:internal/memorybackground",
            "pkg:internal/openrouter",
            "external:openrouter",
            "pkg:internal/tools"
          ],
          "steps": [
            {
              "from": "person:owner",
              "to": "pkg:internal/agent",
              "label": "Sends a message",
              "kind": "call",
              "refs": [
                "code:internal/agent/agent.go#Session.Send"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Acquires the 30-second turn lease; a heartbeat renews it every 10 seconds",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/agent.go#Session.Send",
                "code:internal/agent/ownership.go#defaultTurnTiming",
                "code:internal/eviedb/turn_owner.go#SessionTurnOwner.Acquire"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Appends the owner's text as the root user_message before any provider or tool work",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/agent.go#Session.Send",
                "code:internal/memory/event.go#EventUserMessage",
                "code:internal/eviedb/history.go#SessionHistory.Append"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Reloads accepted events (decoding only those new since the last request) and reconstructs the compaction chain",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/request_history.go#Session.turnEvents",
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Reads repository instructions once per turn and the standing set for every request",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/standing_memory.go#Session.standingMemory",
                "code:internal/eviedb/semantic_standing.go#SessionHistory.StandingMemory"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "First iteration only: Automatic Recall searches accepted memory and conversation excerpts",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic",
                "code:internal/eviedb/retrieval.go#Store.SearchMemory"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/agent",
              "label": "Composes: instructions, working-folder note, repository guidance, standing block, summary, bounded history, memory evidence before the active root, then working context",
              "kind": "note",
              "refs": [
                "code:internal/agent/context.go#ContextComposer.projectAtStart",
                "code:internal/agent/context.go#ContextComposer.Compose"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/agent",
              "label": "Measures the request; one automatic compaction decision per iteration, whole-turn or in-turn",
              "kind": "note",
              "refs": [
                "code:internal/agent/automatic_compaction.go#selectAutomaticCompactionFrom",
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "When required, appends a validated context_compacted or context_turn_compacted summary",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/automatic_compaction.go#Session.performAutomaticCompaction",
                "code:internal/memory/event.go#EventContextCompacted",
                "code:internal/memory/turn_compaction.go#EventContextTurnCompacted"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/memorybackground",
              "label": "Offers the compaction's frontier to the background pass, without waiting",
              "kind": "async",
              "via": "call",
              "refs": [
                "code:internal/agent/compaction_flush.go#Session.requestCompactionFlush",
                "code:internal/memorybackground/pass.go#Runner.RequestCompactionFlush"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Revalidates held memory evidence, then appends context_snapshot: counts, frontiers, hashes and receipts, never content",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/retrieval.go#retrievalTurn.projection",
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/memory/event.go#ContextSnapshotPayload"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Re-authorizes the lease immediately before the provider starts",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn",
                "code:internal/agent/ownership.go#TurnOwnership"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/openrouter",
              "label": "Streams the composed request",
              "kind": "call",
              "via": "import",
              "refs": [
                "code:internal/agent/turn.go#Session.callProvider",
                "code:internal/openrouter/client.go#Client.ChatStream"
              ]
            },
            {
              "from": "pkg:internal/openrouter",
              "to": "external:openrouter",
              "label": "Chat Completions (or Responses) over HTTPS",
              "kind": "call",
              "via": "network",
              "refs": [
                "code:internal/openrouter/client.go#Client.ChatStream"
              ]
            },
            {
              "from": "pkg:internal/openrouter",
              "to": "pkg:internal/agent",
              "label": "Streamed response; a transient failure is retried only before any output streamed",
              "kind": "return",
              "via": "import",
              "refs": [
                "code:internal/agent/turn.go#Session.callProvider",
                "code:internal/agent/turn.go#providerRetryDelay"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Appends assistant_message; without tool calls this is the turn's durable success",
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
              "label": "Appends a tool_intent for each call",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/agent.go#toolIntentInput",
                "code:internal/memory/event.go#EventToolIntent"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/tools",
              "label": "Prepare, approve, execute, with the lease re-authorized before preparation and before execution",
              "kind": "call",
              "via": "import",
              "refs": [
                "code:internal/tools/registry.go#Toolset.ExecuteWithApprovalAuthorizedCompletion",
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "Appends the approval (with proposal and prepared hashes for memory) and the terminal tool outcome",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/agent.go#approvalEventInput",
                "code:internal/agent/agent.go#semanticApprovalEventInput",
                "code:internal/agent/agent.go#toolOutcomeInput"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/agent",
              "label": "Recomposes from the newly accepted events and loops",
              "kind": "note",
              "refs": [
                "code:internal/agent/turn.go#Session.runOwnedTurn"
              ]
            },
            {
              "from": "pkg:internal/agent",
              "to": "pkg:internal/eviedb",
              "label": "On a durable failure appends turn_failed or turn_interrupted; always releases the lease",
              "kind": "call",
              "via": "call",
              "refs": [
                "code:internal/agent/turn.go#Session.appendTerminal",
                "code:internal/agent/agent.go#Session.Send"
              ]
            }
          ]
        },
        {
          "id": "memory-semantic-write-path",
          "title": "Semantic write path",
          "kind": "graph",
          "summary": {
            "text": "How a change becomes accepted Semantic Memory, redrawn from today's code: a change starts from an owner request, Evie's own proposal, the background pass or a compiler candidate; three review paths (the approval card, the review inbox and candidate review) gather the owner's decision; and Apply revalidates the exact approved proposal and commits it in one SQLite transaction.",
            "refs": [
              "code:internal/eviedb/semantic.go#Store.PrepareRememberLiteral",
              "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
              "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal",
              "doc:docs/architecture/memory/semantic-write-path.d2"
            ]
          },
          "lanes": [
            {
              "id": "intent",
              "title": "1 · Intent becomes a complete proposal"
            },
            {
              "id": "review",
              "title": "2 · The owner reviews the exact proposal"
            },
            {
              "id": "commit",
              "title": "3 · Revalidate and commit atomically"
            }
          ],
          "nodes": [
            {
              "id": "request",
              "label": "Owner asks in a turn (remember, correct, promote, link, retire, restore, set standing)",
              "lane": "intent"
            },
            {
              "id": "proactive",
              "label": "Evie proposes on its own (no explicit cue)",
              "lane": "intent"
            },
            {
              "id": "background",
              "label": "Background pass finds it in owner messages",
              "node": "pkg:internal/memorybackground",
              "lane": "intent"
            },
            {
              "id": "compiler",
              "label": "Compiler Memory Candidate",
              "node": "container:compiler-host",
              "lane": "intent"
            },
            {
              "id": "source",
              "label": "Source: the owner's user_message, bound to an exact span",
              "lane": "intent"
            },
            {
              "id": "prepare",
              "label": "Kernel Prepare: scope from the session, revision vector, generated IDs, proposal and prepared hashes",
              "node": "pkg:internal/eviedb",
              "lane": "intent"
            },
            {
              "id": "card",
              "label": "Approval card",
              "lane": "review"
            },
            {
              "id": "inbox",
              "label": "Review inbox",
              "lane": "review"
            },
            {
              "id": "refresh",
              "label": "Stale proposal prepared again",
              "lane": "review"
            },
            {
              "id": "candidateReview",
              "label": "Candidate review preview bound by hash",
              "lane": "review"
            },
            {
              "id": "decision",
              "label": "Owner decides: approve, decline, expire",
              "lane": "review"
            },
            {
              "id": "approvalEvent",
              "label": "approval event with both hashes",
              "lane": "review"
            },
            {
              "id": "apply",
              "label": "Kernel Apply with the exact approved proposal",
              "lane": "commit"
            },
            {
              "id": "guards",
              "label": "Guards: lease fence, hashes, idempotency key, source event, scope vector, quarantine, maintenance lock, standing budget",
              "lane": "commit"
            },
            {
              "id": "transaction",
              "label": "One transaction: operation, operation scopes, graph rows, state events, classification, Scope Revisions",
              "node": "container:evie-db",
              "lane": "commit"
            },
            {
              "id": "result",
              "label": "Stable result: operation and object IDs",
              "lane": "commit"
            },
            {
              "id": "nochange",
              "label": "No change: declined, expired, stale or refused",
              "lane": "commit"
            }
          ],
          "edges": [
            {
              "from": "request",
              "to": "prepare",
              "label": "memory tools adapt to Prepare",
              "refs": [
                "code:internal/plugins/memory.go#Memory.rememberLiteralTool",
                "code:internal/plugins/memory.go#SemanticMemoryKernel"
              ]
            },
            {
              "from": "proactive",
              "to": "prepare",
              "label": "requested_by_owner false",
              "refs": [
                "code:internal/plugins/memory_proactive.go#Memory.routeToInbox"
              ]
            },
            {
              "from": "background",
              "to": "prepare",
              "label": "ADD, UPDATE or INVALIDATE",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.prepare"
              ]
            },
            {
              "from": "source",
              "to": "prepare",
              "label": "loaded from SQLite, never from the model",
              "refs": [
                "code:internal/eviedb/semantic.go#Store.PrepareRememberLiteral",
                "code:internal/eviedb/semantic_source_binding.go#bindOwnerSource"
              ]
            },
            {
              "from": "prepare",
              "to": "card",
              "label": "explicit request with a cue",
              "refs": [
                "code:internal/plugins/memory_proactive.go#Memory.routeToInbox",
                "code:internal/memory/proposal_inbox.go#HasExplicitMemoryCue"
              ]
            },
            {
              "from": "prepare",
              "to": "inbox",
              "label": "queued exactly as prepared",
              "refs": [
                "code:internal/eviedb/memory_proposal_inbox.go#Store.QueueMemoryProposal",
                "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
              ]
            },
            {
              "from": "inbox",
              "to": "refresh",
              "label": "its scopes moved",
              "refs": [
                "code:internal/eviedb/memory_proposal_inbox.go#Store.refreshInboxProposal"
              ]
            },
            {
              "from": "refresh",
              "to": "inbox",
              "label": "next revision",
              "refs": [
                "code:internal/eviedb/memory_proposal_inbox.go#Store.refreshInboxProposal"
              ]
            },
            {
              "from": "compiler",
              "to": "candidateReview",
              "label": "evie memory-review prepare",
              "refs": [
                "code:internal/eviedb/candidate_review_prepare.go#Store.PrepareOwnerCandidateReview",
                "command:evie memory-review prepare"
              ]
            },
            {
              "from": "card",
              "to": "decision",
              "label": "Action Approval",
              "refs": [
                "code:internal/tools/registry.go#Approver",
                "code:internal/web/approvals.go#Server.approver"
              ]
            },
            {
              "from": "inbox",
              "to": "decision",
              "label": "web Review tab",
              "refs": [
                "code:internal/web/memory_proposals.go#Server.handleMemoryProposalApprove",
                "code:internal/web/ui/src/candidateInbox/ProposalInbox.tsx"
              ]
            },
            {
              "from": "decision",
              "to": "approvalEvent",
              "label": "approved, stored before Apply",
              "refs": [
                "code:internal/agent/agent.go#semanticApprovalEventInput",
                "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal"
              ]
            },
            {
              "from": "decision",
              "to": "nochange",
              "label": "declined or expired",
              "refs": [
                "code:internal/memory/event.go#ApprovalDeclined",
                "code:internal/eviedb/memory_proposal_inbox.go#Store.DeclineMemoryProposal"
              ]
            },
            {
              "from": "approvalEvent",
              "to": "apply",
              "label": "the prepared tool runs",
              "refs": [
                "code:internal/plugins/memory.go#preparedSemanticMutation"
              ]
            },
            {
              "from": "candidateReview",
              "to": "apply",
              "label": "evie memory-review resolve",
              "refs": [
                "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview",
                "command:evie memory-review resolve"
              ]
            },
            {
              "from": "apply",
              "to": "guards",
              "label": "revalidate everything",
              "refs": [
                "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral",
                "code:internal/eviedb/semantic.go#validateSemanticScopeVector",
                "code:internal/eviedb/semantic_standing.go#Store.enforceStandingBudget"
              ]
            },
            {
              "from": "guards",
              "to": "transaction",
              "label": "all pass",
              "refs": [
                "code:internal/eviedb/semantic.go#recordAcceptedSemanticOperation",
                "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral"
              ]
            },
            {
              "from": "guards",
              "to": "nochange",
              "label": "stale revision, changed source, quarantined scope, memory full",
              "refs": [
                "code:internal/eviedb/semantic.go#validateSemanticScopeVector",
                "code:internal/memory/standing.go#ErrStandingMemoryFull"
              ]
            },
            {
              "from": "transaction",
              "to": "result",
              "label": "same IDs on every surface",
              "refs": [
                "code:internal/memory/semantic.go#RememberLiteralResult",
                "code:internal/memory/proposal_inbox.go#MemoryProposalResult"
              ]
            }
          ]
        },
        {
          "id": "memory-semantic-domain-model",
          "title": "Semantic domain model",
          "kind": "er",
          "summary": {
            "text": "The tables that hold accepted Semantic Memory, drawn from the real schema: accepted operations and their per-scope revision effects, the scoped projection of Predicates, Entities, Aliases, Claims, Source Links, corrections, Graph Links and Promotions, the append-only state events and Claim classifications, the projection quarantine, and the events that Source Links cite.",
            "refs": [
              "code:internal/eviedb/semantic.go#ensureSemanticSchema",
              "code:internal/eviedb/semantic_standing.go#ensureSemanticClaimClassifications",
              "code:internal/eviedb/semantic_replay.go#semanticProjectionTableNames",
              "doc:docs/architecture/memory/semantic-domain-model.d2"
            ]
          },
          "database": "evie",
          "tables": [
            "semantic_operations",
            "semantic_operation_scopes",
            "semantic_scopes",
            "semantic_predicates",
            "semantic_entities",
            "semantic_aliases",
            "semantic_claims",
            "semantic_source_links",
            "semantic_claim_corrections",
            "semantic_graph_links",
            "semantic_promotions",
            "semantic_promotion_entities",
            "semantic_state_events",
            "semantic_claim_classifications",
            "semantic_projection_quarantine",
            "events"
          ]
        },
        {
          "id": "memory-scope-promotion",
          "title": "Scope and promotion",
          "kind": "graph",
          "summary": {
            "text": "Which scopes a conversation reads and writes, redrawn from today's code: each session reads exactly Global, its own Workspace or project, and itself; a new memory goes to a scope derived from the session, possibly widened by an approved destination; and Promotion copies one narrower Claim into a broader scope while the original stays unchanged.",
            "refs": [
              "code:internal/eviedb/semantic_correction.go#allowedSemanticReadScopeKeys",
              "code:internal/memory/destination.go#ResolveMemoryDestination",
              "code:internal/eviedb/semantic_promotion.go#promotionPathAllowed",
              "doc:docs/architecture/memory/scope-promotion.d2"
            ]
          },
          "lanes": [
            {
              "id": "reads",
              "title": "Exact read sets"
            },
            {
              "id": "writes",
              "title": "Where new memory goes"
            },
            {
              "id": "promotion",
              "title": "Promotion"
            }
          ],
          "nodes": [
            {
              "id": "globalSession",
              "label": "Global conversation: reads global + session; default write global",
              "lane": "reads"
            },
            {
              "id": "workspaceSession",
              "label": "Workspace conversation: reads global + workspace + session; default write the Workspace",
              "lane": "reads"
            },
            {
              "id": "projectSession",
              "label": "Project conversation: reads global + project + session; default write the project",
              "lane": "reads"
            },
            {
              "id": "excluded",
              "label": "Never read: other sessions' memory, sibling Workspaces and projects, quarantined scopes",
              "lane": "reads"
            },
            {
              "id": "excerpts",
              "label": "Conversation excerpts: earlier conversations of the same Context Scope only",
              "lane": "reads"
            },
            {
              "id": "destination",
              "label": "Destination recommendation: everywhere, workspace or session, resolved from the session and shown on approval",
              "lane": "writes"
            },
            {
              "id": "everywhere",
              "label": "Background Global version: Save everywhere instead",
              "node": "pkg:internal/memorybackground",
              "lane": "writes"
            },
            {
              "id": "narrower",
              "label": "Narrower accepted Claim (session, Workspace or project) stays unchanged",
              "lane": "promotion"
            },
            {
              "id": "promotionProposal",
              "label": "Promotion proposal: active Claim with eligible sources, source and destination revisions",
              "lane": "promotion"
            },
            {
              "id": "broader",
              "label": "Broader Claim with reused or created Entities and copied Source Links",
              "lane": "promotion"
            },
            {
              "id": "mapping",
              "label": "semantic_promotions and promotion entities map source to destination",
              "lane": "promotion"
            }
          ],
          "edges": [
            {
              "from": "globalSession",
              "to": "excluded",
              "label": "scope boundary",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#allowedSemanticReadScopeKeys",
                "code:internal/eviedb/semantic.go#requireSemanticScopeKeysAvailable"
              ]
            },
            {
              "from": "workspaceSession",
              "to": "excluded",
              "label": "scope boundary",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#allowedSemanticReadScopeKeys"
              ]
            },
            {
              "from": "projectSession",
              "to": "excluded",
              "label": "scope boundary",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#allowedSemanticReadScopeKeys"
              ]
            },
            {
              "from": "workspaceSession",
              "to": "excerpts",
              "label": "same Context Scope (ADR 0066)",
              "refs": [
                "code:internal/eviedb/retrieval_conversation.go#Store.readConversationPass",
                "adr:0066"
              ]
            },
            {
              "from": "globalSession",
              "to": "excerpts",
              "label": "Global history from Global only",
              "refs": [
                "code:internal/eviedb/retrieval_conversation.go#Store.readConversationPass"
              ]
            },
            {
              "from": "workspaceSession",
              "to": "destination",
              "label": "remember with a destination",
              "refs": [
                "code:internal/memory/destination.go#ResolveMemoryDestination",
                "code:internal/eviedb/semantic_destination.go#validateRecommendedDestination"
              ]
            },
            {
              "from": "destination",
              "to": "everywhere",
              "label": "background ADD Evie thinks holds everywhere",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.prepareEverywhere",
                "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal"
              ]
            },
            {
              "from": "narrower",
              "to": "promotionProposal",
              "label": "owner picks one exact Claim",
              "refs": [
                "code:internal/eviedb/semantic_promotion.go#Store.PreparePromotion",
                "code:internal/eviedb/semantic_promotion.go#promotionPathAllowed"
              ]
            },
            {
              "from": "promotionProposal",
              "to": "broader",
              "label": "one approved atomic operation",
              "refs": [
                "code:internal/eviedb/semantic_promotion.go#Store.ApplyPromotion"
              ]
            },
            {
              "from": "broader",
              "to": "mapping",
              "label": "records the identity mapping",
              "refs": [
                "code:internal/eviedb/semantic_promotion.go#Store.ApplyPromotion",
                "table:evie.semantic_promotions",
                "table:evie.semantic_promotion_entities"
              ]
            },
            {
              "from": "mapping",
              "to": "narrower",
              "label": "ID-level provenance; unreadable source text withheld",
              "refs": [
                "code:internal/eviedb/semantic_promotion.go#Store.PreparePromotion"
              ]
            }
          ]
        },
        {
          "id": "memory-temporal-lifecycle",
          "title": "Claim temporal lifecycle",
          "kind": "state",
          "summary": {
            "text": "The lifecycle of one Claim, redrawn from today's code: states are appended as semantic_state_events under Transaction Time and never edited, while Valid Time says when the Claim holds in the world; exact reads ask valid_at, as_known_at or both, and only an active Claim with an eligible Source Link is current.",
            "refs": [
              "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
              "code:internal/eviedb/semantic_correction.go#Store.inspectClaimsInScope",
              "code:internal/memory/semantic.go#ClaimQuery",
              "doc:docs/architecture/memory/temporal-lifecycle.d2"
            ]
          },
          "states": [
            {
              "id": "active",
              "label": "active and supported: in current reads within its Valid Time",
              "refs": [
                "code:internal/memory/semantic.go#SemanticStateActive",
                "code:internal/eviedb/semantic_correction.go#Store.inspectClaimsInScope"
              ]
            },
            {
              "id": "unsupported",
              "label": "unsupported (computed): active, but no eligible Source Link",
              "refs": [
                "code:internal/memory/semantic.go#SemanticStatusUnsupported",
                "code:internal/eviedb/semantic_lifecycle.go#Store.InspectSemanticObjectAtScopeAndTime"
              ]
            },
            {
              "id": "retired",
              "label": "retired: hidden from current reads, kept for history",
              "refs": [
                "code:internal/memory/semantic.go#SemanticStateRetired",
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions"
              ]
            },
            {
              "id": "supersededChanged",
              "label": "superseded by a changed correction: still answers for valid times before the effective time",
              "refs": [
                "code:internal/memory/semantic.go#CorrectionChanged",
                "code:internal/eviedb/semantic_correction.go#correctionValidTimeEffect",
                "code:internal/eviedb/semantic_correction.go#Store.inspectClaimsInScope"
              ]
            },
            {
              "id": "supersededError",
              "label": "superseded by an error correction: never current again",
              "refs": [
                "code:internal/memory/semantic.go#CorrectionError",
                "code:internal/eviedb/semantic_correction.go#Store.inspectClaimsInScope"
              ]
            }
          ],
          "initial": "active",
          "transitions": [
            {
              "from": "active",
              "to": "retired",
              "label": "retire_memory (an Entity's retirement lists its dependents)",
              "refs": [
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
                "code:internal/eviedb/semantic_lifecycle.go#currentEntityDependents"
              ]
            },
            {
              "from": "retired",
              "to": "active",
              "label": "restore_memory, only if its dependencies are still legal",
              "refs": [
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
                "code:internal/eviedb/semantic_lifecycle.go#validateRestorationDependencies"
              ]
            },
            {
              "from": "active",
              "to": "unsupported",
              "label": "retract_source of its last eligible Source Link",
              "refs": [
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
                "code:internal/memory/semantic.go#LifecycleRetractSource"
              ]
            },
            {
              "from": "unsupported",
              "to": "active",
              "label": "restore_source while the Claim is active",
              "refs": [
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions",
                "code:internal/eviedb/semantic_lifecycle.go#requireSourceClaimActive"
              ]
            },
            {
              "from": "unsupported",
              "to": "retired",
              "label": "retire_memory (the latest state is still active)",
              "refs": [
                "code:internal/eviedb/semantic_lifecycle.go#deriveLifecycleTransitions"
              ]
            },
            {
              "from": "active",
              "to": "supersededChanged",
              "label": "correct_claim, mode changed: old Valid Time ends where the replacement's begins",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#Store.PrepareCorrectClaim",
                "code:internal/eviedb/semantic_correction.go#correctionValidTimeEffect"
              ]
            },
            {
              "from": "active",
              "to": "supersededError",
              "label": "correct_claim, mode error: the replacement inherits the old Valid Time",
              "refs": [
                "code:internal/eviedb/semantic_correction.go#Store.PrepareCorrectClaim",
                "code:internal/eviedb/semantic_correction.go#correctionValidTimeEffect"
              ]
            }
          ]
        },
        {
          "id": "memory-replay-recovery",
          "title": "Replay and recovery",
          "kind": "graph",
          "summary": {
            "text": "How the live semantic projection is checked and repaired, redrawn from today's code: accepted operations are the canonical history, a cheap check runs on every database open, the owner's /memory verify replays them into a private copy and compares per scope, divergent scopes are quarantined, and /memory rebuild swaps in a verified projection under a fenced lock.",
            "refs": [
              "code:internal/eviedb/semantic_replay.go#Store.VerifySemanticProjection",
              "code:internal/eviedb/semantic_replay.go#Store.OwnerRebuildSemanticProjection",
              "code:cmd/evie/repl.go#handleMemoryCommand",
              "doc:docs/architecture/memory/replay-recovery.d2"
            ]
          },
          "nodes": [
            {
              "id": "operations",
              "label": "semantic_operations: accepted history, schema versions 1 to 7",
              "node": "container:evie-db"
            },
            {
              "id": "live",
              "label": "Live projection rows"
            },
            {
              "id": "startup",
              "label": "Every open: revision and state-event frontiers, known schema versions, semantic foreign keys"
            },
            {
              "id": "copy",
              "label": "VACUUM INTO a private temporary database"
            },
            {
              "id": "replay",
              "label": "Shadow replay: reset the projection, re-run each operation's Apply in scope-frontier order"
            },
            {
              "id": "unknown",
              "label": "Unknown kind or version, or a gap in a scope frontier: replay fails at that operation"
            },
            {
              "id": "compare",
              "label": "Compare per scope: canonical row hashes, revisions, operation frontiers, foreign keys"
            },
            {
              "id": "match",
              "label": "Match: live state stays in service"
            },
            {
              "id": "quarantine",
              "label": "Quarantine affected scopes: their reads and writes fail; other scopes and Episodic Memory keep working"
            },
            {
              "id": "rebuild",
              "label": "/memory rebuild: fenced maintenance lock, self-verified shadow, foreign-key check"
            },
            {
              "id": "swap",
              "label": "One transaction: replace projection rows, restore append-only triggers, clear quarantine"
            }
          ],
          "edges": [
            {
              "from": "operations",
              "to": "live",
              "label": "each accepted transaction writes both",
              "refs": [
                "code:internal/eviedb/semantic.go#recordAcceptedSemanticOperation",
                "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral"
              ]
            },
            {
              "from": "live",
              "to": "startup",
              "label": "on open",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup"
              ]
            },
            {
              "from": "startup",
              "to": "quarantine",
              "label": "inconsistent scope",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#checkSemanticProjectionStartup"
              ]
            },
            {
              "from": "operations",
              "to": "copy",
              "label": "/memory verify",
              "refs": [
                "code:cmd/evie/repl.go#handleMemoryCommand",
                "code:internal/eviedb/semantic_replay.go#Store.buildSemanticProjectionShadow"
              ]
            },
            {
              "from": "copy",
              "to": "replay",
              "label": "no model, network or plugin",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#resetSemanticProjection",
                "code:internal/eviedb/semantic_replay.go#replaySemanticOperations",
                "code:internal/eviedb/semantic_replay.go#Store.replaySemanticOperation"
              ]
            },
            {
              "from": "replay",
              "to": "unknown",
              "label": "fails closed",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#semanticReplayHandler",
                "code:internal/eviedb/semantic_replay.go#replaySemanticOperations"
              ]
            },
            {
              "from": "unknown",
              "to": "quarantine",
              "label": "scopes the operation touched",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.quarantineReplayOperation"
              ]
            },
            {
              "from": "replay",
              "to": "compare",
              "label": "snapshot both",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#snapshotSemanticProjection",
                "code:internal/eviedb/semantic_replay.go#compareSemanticProjectionSnapshots"
              ]
            },
            {
              "from": "compare",
              "to": "match",
              "label": "equal",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.VerifySemanticProjection"
              ]
            },
            {
              "from": "compare",
              "to": "quarantine",
              "label": "divergent",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.quarantineProjectionReport"
              ]
            },
            {
              "from": "quarantine",
              "to": "rebuild",
              "label": "owner action",
              "refs": [
                "code:cmd/evie/repl.go#handleMemoryCommand",
                "code:internal/eviedb/semantic_replay.go#Store.OwnerRebuildSemanticProjection"
              ]
            },
            {
              "from": "operations",
              "to": "rebuild",
              "label": "canonical input",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.OwnerRebuildSemanticProjection"
              ]
            },
            {
              "from": "rebuild",
              "to": "swap",
              "label": "only after complete verification",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.swapSemanticProjection"
              ]
            },
            {
              "from": "swap",
              "to": "live",
              "label": "verified projection restored",
              "refs": [
                "code:internal/eviedb/semantic_replay.go#Store.swapSemanticProjection"
              ]
            }
          ]
        }
      ],
      "links": [
        "doc:docs/architecture/memory/stage-evolution.d2",
        "doc:docs/architecture/memory/turn-context-flow.d2",
        "doc:docs/architecture/memory/semantic-write-path.d2",
        "doc:docs/architecture/memory/semantic-domain-model.d2",
        "doc:docs/architecture/memory/scope-promotion.d2",
        "doc:docs/architecture/memory/temporal-lifecycle.d2",
        "doc:docs/architecture/memory/replay-recovery.d2",
        "doc:docs/architecture/memory/workspace.dsl",
        "doc:docs/architecture/memory/svg/01-system-context.svg",
        "doc:docs/architecture/memory/svg/02-runtime-containers.svg",
        "doc:docs/architecture/memory/svg/03-runtime-components.svg",
        "doc:docs/architecture/memory/svg/04-stage-evolution.svg",
        "doc:docs/architecture/memory/svg/05-turn-context-flow.svg",
        "doc:docs/architecture/memory/svg/06-semantic-write-path.svg",
        "doc:docs/architecture/memory/svg/07-semantic-domain-model.svg",
        "doc:docs/architecture/memory/svg/08-scope-promotion.svg",
        "doc:docs/architecture/memory/svg/09-temporal-lifecycle.svg",
        "doc:docs/architecture/memory/svg/10-replay-recovery.svg",
        "doc:docs/memory-architecture.md",
        "doc:cmd/evie/docs/active/memory.spec.md",
        "doc:cmd/evie/docs/active/memory.decisions.md",
        "doc:cmd/evie/docs/active/semantic-memory-stage-3.spec.md",
        "doc:cmd/evie/docs/active/semantic-memory-stage-4.spec.md",
        "doc:cmd/evie/docs/active/memory-stage-5-retrieval.spec.md",
        "adr:0034",
        "adr:0047",
        "adr:0048",
        "adr:0050",
        "adr:0052",
        "adr:0056",
        "adr:0059",
        "adr:0060",
        "adr:0066"
      ]
    },
    {
      "id": "pkg:internal/memorybackground",
      "level": "component",
      "kind": "package",
      "title": "memorybackground",
      "group": "memory",
      "summary": {
        "text": "Runs the background memory pass, off unless EVIE_REMOTE_MEMORY=on with OPENROUTER_API_KEY, or EVIE_COMPILER_CONFIG for a local extractor: when a root conversation goes idle, gathers 20 new owner messages, or has owner messages a committed compaction summarized, it reads only those owner messages, extracts candidate memories, compares them with existing ones, and queues exact ADD, UPDATE or INVALIDATE proposals in the owner's review inbox without applying anything.",
        "refs": [
          "code:internal/memorybackground/config.go#FromEnv",
          "code:internal/memorybackground/pass.go#Runner.runSession",
          "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions",
          "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
        ]
      },
      "does": [
        {
          "text": "FromEnv resolves the configuration and never fails: OpenRouter when EVIE_REMOTE_MEMORY=on and OPENROUTER_API_KEY are set, otherwise the pinned local extractor named by EVIE_COMPILER_CONFIG, otherwise disabled, and any bad setting disables the pass with a reason the owner can act on.",
          "refs": [
            "code:internal/memorybackground/config.go#FromEnv",
            "code:internal/memorybackground/config.go#Config",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost"
          ]
        },
        {
          "text": "Run ticks every minute until shutdown; each Tick records the activation frontier once, takes the cross-process runner lease, stops when today's spend has reached a cap, and runs at most four due conversations one at a time, renewing the runner lease before each.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.Run",
            "code:internal/memorybackground/pass.go#Runner.Tick",
            "code:internal/memorybackground/config.go#DefaultTick",
            "code:internal/memorybackground/config.go#DefaultSessionsPerTick",
            "code:internal/eviedb/memory_background.go#Store.AcquireMemoryBackgroundRunner"
          ]
        },
        {
          "text": "RequestCompactionFlush only wakes a sleeping Run early; a committed compaction makes its conversation due by itself, and a compaction-due conversation runs pass after pass up to the compaction's frontier (at most 64).",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.RequestCompactionFlush",
            "code:internal/memorybackground/pass.go#Runner.wait",
            "code:internal/memorybackground/pass.go#Runner.passThrough",
            "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions"
          ]
        },
        {
          "text": "Each pass reads a bounded batch of the conversation's owner messages since its cursor (30 messages, 24 KiB, each cut at 4 KiB with a visible marker); a message with a detected secret or a local slash command moves the cursor but is never sent.",
          "refs": [
            "code:internal/memorybackground/config.go#DefaultBatchMessages",
            "code:internal/memorybackground/config.go#DefaultBatchBytes",
            "code:internal/memorybackground/prompt.go#truncateText",
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundBatch"
          ]
        },
        {
          "text": "Extract: one JSON-mode call asks for at most eight owner-subject memories, each with kind, applicability, Predicate token and label, cardinality, one typed value, polarity and the index of the stating message, listing the Predicate tokens already in use so they are reused.",
          "refs": [
            "code:internal/memorybackground/prompt.go#extractRequest",
            "code:internal/memorybackground/prompt.go#extractSystem",
            "code:internal/memorybackground/prompt.go#extractedMemory",
            "code:internal/memorybackground/config.go#DefaultCandidateLimit"
          ]
        },
        {
          "text": "Route: in a Global conversation every memory targets Global and a Workspace memory is dropped; in a Workspace or project conversation every memory stays in that scope, and \"profile\" only marks Evie's suggestion that it holds everywhere.",
          "refs": [
            "code:internal/memorybackground/prompt.go#parseExtraction",
            "code:internal/memorybackground/prompt.go#usableCandidate"
          ]
        },
        {
          "text": "Compare: the conversation's readable current memories and pending remembers are ranked by a deterministic lexical score (same Predicate token, label, value and text overlap) plus a small boost when the full-text or dense retriever also returns the Claim, keeping the top eight above a 0.25 floor.",
          "refs": [
            "code:internal/memorybackground/similarity.go#similarityScore",
            "code:internal/memorybackground/similarity.go#rankSimilar",
            "code:internal/memorybackground/similarity.go#similarityFloor",
            "code:internal/memorybackground/pass.go#Runner.denseMatches",
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundPool"
          ]
        },
        {
          "text": "Decide: an exact duplicate is a NOOP and a candidate with nothing similar is an ADD without a model call; the rest go to one decision call that chooses ADD, UPDATE (changed or error), INVALIDATE or NOOP against memories shown under short handles.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.propose",
            "code:internal/memorybackground/similarity.go#exactDuplicate",
            "code:internal/memorybackground/prompt.go#decideSystem",
            "code:internal/memorybackground/prompt.go#parseDecisions"
          ]
        },
        {
          "text": "Prepare: ADD goes through PrepareRememberLiteral, UPDATE through PrepareCorrectClaim (a changed correction takes the stating message's time as its effective time) and INVALIDATE through a retire PrepareMemoryLifecycle, the same Kernel paths an approval card uses.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.prepare",
            "code:internal/eviedb/semantic.go#Store.PrepareRememberLiteral",
            "code:internal/eviedb/semantic_correction.go#Store.PrepareCorrectClaim",
            "code:internal/eviedb/semantic_lifecycle.go#Store.PrepareMemoryLifecycle"
          ]
        },
        {
          "text": "For a Workspace or project ADD that Evie thinks holds everywhere, it also prepares the Global version under the same idempotency key, so approving either version resolves the proposal.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.prepareEverywhere",
            "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal"
          ]
        },
        {
          "text": "Commit: it takes the conversation's turn lease for ten seconds, retrying up to twelve times five seconds apart, and queues the proposals and moves the cursor in one compare-and-set transaction; a conversation busy for the whole minute defers the run.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.commit",
            "code:internal/memorybackground/pass.go#commitLeaseDuration",
            "code:internal/memorybackground/config.go#DefaultCommitAttempts",
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
          ]
        },
        {
          "text": "Status reports the configuration, today's spend and what remains, the last run and last problem, skipped batches, pending background proposals, each recent conversation's cursor and the compaction flushes still owed, without any message, proposal or memory text.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.Status",
            "code:cmd/evie/memory_background.go#runMemoryBackgroundManagement",
            "route:/api/memory/background/status"
          ]
        }
      ],
      "owns": [
        {
          "text": "Holds no tables: its durable state is eviedb's activation and runner-lease row, one cursor per root conversation and one row per run, and its proposals land in the review inbox.",
          "refs": [
            "table:evie.memory_background_state",
            "table:evie.memory_background_cursors",
            "table:evie.memory_background_runs",
            "table:evie.memory_proposal_inbox",
            "code:internal/eviedb/memory_background.go#memoryBackgroundSchema"
          ]
        },
        {
          "text": "Owns the extraction and decision prompts and their strict output contracts, versioned memory-background-v1 and recorded with the model identity on every run.",
          "refs": [
            "code:internal/memorybackground/prompt.go#PromptVersion",
            "code:internal/memorybackground/prompt.go#ErrMalformed",
            "code:internal/memorybackground/pass.go#Runner.runSession"
          ]
        },
        {
          "text": "Owns spend accounting: before each call it reserves the estimated input (three bytes per token) plus the whole output allowance against a 32,000-token pass budget and today's caps of 300,000 tokens and $0.25, priced from the OpenRouter catalog or a deliberately high fallback.",
          "refs": [
            "code:internal/memorybackground/prompt.go#ModelRequest.EstimatedTokens",
            "code:internal/memorybackground/pass.go#Runner.call",
            "code:internal/memorybackground/pass.go#Runner.capExceeded",
            "code:internal/memorybackground/config.go#DefaultPassTokens",
            "code:internal/memorybackground/config.go#FallbackPricing"
          ]
        },
        {
          "text": "Owns the idempotency keys of its proposals, derived from the conversation, the stating message and the proposed change, so a pass repeated after a crash proposes the identical change under the identical key.",
          "refs": [
            "code:internal/memorybackground/pass.go#idempotencyKey"
          ]
        },
        {
          "text": "In memory a Runner keeps only its holder ID, the current price, whether it has activated, and a one-slot channel that wakes it for a flush.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Store is the consumer-owned persistence seam (activation, runner lease, due conversations, batch, pool, the three Prepare calls, turn lease, commit, failure, spend and progress); *eviedb.Store implements it.",
          "refs": [
            "code:internal/memorybackground/pass.go#Store",
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost"
          ]
        },
        {
          "text": "Searcher is optional: when the store also offers SearchMemory, its accepted-memory hits boost similarity, and when it fails only the boost is lost.",
          "refs": [
            "code:internal/memorybackground/pass.go#Searcher",
            "code:internal/memorybackground/pass.go#Runner.denseMatches"
          ]
        },
        {
          "text": "Model is one structured completion (Identity and Complete) that must not retry; OpenRouterModel sends Chat Completions with JSON output, temperature 0, low reasoning and no tools, and LocalModel uses the loopback extractor's chat.",
          "refs": [
            "code:internal/memorybackground/pass.go#Model",
            "code:internal/memorybackground/models.go#OpenRouterModel.Complete",
            "code:internal/memorybackground/models.go#LocalModel.Complete",
            "code:internal/memorybackground/models.go#ChatClient",
            "code:internal/memorybackground/models.go#LocalCompleter"
          ]
        },
        {
          "text": "Runner satisfies the agent's CompactionMemoryFlush, and cmd/evie hands it to every REPL and serve conversation so committed compactions are offered to the pass.",
          "refs": [
            "code:internal/agent/compaction_flush.go#CompactionMemoryFlush",
            "code:internal/agent/compaction_flush.go#WithCompactionMemoryFlush",
            "code:cmd/evie/memory_background.go#memoryBackgroundHost",
            "code:cmd/evie/main.go#main"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "It reads only owner user messages of root conversations, never assistant, tool, web, compiler or delegated text, so third-party text cannot plant a fact about the owner; a delegated conversation never offers a compaction flush.",
          "refs": [
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundBatch",
            "code:internal/agent/compaction_flush.go#Session.requestCompactionFlush",
            "code:internal/memorybackground/prompt.go#extractSystem"
          ]
        },
        {
          "text": "It never commits while a live turn holds the conversation: the turn lease fences the commit, and the cursor's compare-and-set makes a pass that another process already covered record itself as superseded, only so its spend still counts.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.commit",
            "code:internal/memorybackground/pass.go#Runner.superseded",
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
          ]
        },
        {
          "text": "A pending proposal is never changed, a Claim outside the conversation's own Context Scope cannot be corrected or retired from it, a Claim gets at most one change per pass, and an identical retired or superseded Claim is skipped because restoring it is the owner's explicit act.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.propose",
            "code:internal/memorybackground/pass.go#Runner.prepare",
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundPool"
          ]
        },
        {
          "text": "Only literal memories are extracted; an Entity-to-Entity memory still needs the in-turn memory_remember_entity tool.",
          "refs": [
            "code:internal/memorybackground/prompt.go#extractedMemory",
            "code:internal/memorybackground/pass.go#Runner.prepare",
            "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
          ]
        },
        {
          "text": "Three consecutive malformed replies, or a batch that cannot fit the pass budget, skip the batch: the cursor moves past it and the skipped run stays visible; a provider or network failure never skips, and backs the conversation off from one minute doubling to one hour.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.fail",
            "code:internal/memorybackground/pass.go#Runner.record",
            "code:internal/memorybackground/config.go#DefaultMalformedSkip",
            "code:internal/memorybackground/config.go#DefaultMaxBackoff"
          ]
        },
        {
          "text": "Only a failure that proved nothing was generated (no response, 429, 502, 503 or 504) is retried, twice, honoring Retry-After but never waiting more than 30 seconds; shutdown cancels the in-flight call and commits nothing, and the cursor lets the next process redo the batch.",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.call",
            "code:internal/memorybackground/models.go#classifyProviderError",
            "code:internal/memorybackground/pass.go#Runner.Run"
          ]
        },
        {
          "text": "The memory architecture plan suggested the model's cheaper :batch variant, but ValidateRemoteModel refuses it, as it does floating latest aliases and the auto router, because the pass does not use OpenRouter's asynchronous Batch API.",
          "refs": [
            "code:internal/memorybackground/config.go#ValidateRemoteModel",
            "doc:docs/memory-architecture.md#decisions-david-2026-10-03"
          ]
        },
        {
          "text": "The plan's decision 4 routes each proposal to profile or Workspace scope; since the 2026-10-12 amendment the code keeps a Workspace or project conversation's memories in that scope and offers Global only as a second version.",
          "refs": [
            "code:internal/memorybackground/prompt.go#parseExtraction",
            "doc:docs/memory-architecture.md#decisions-david-2026-10-03",
            "doc:cmd/evie/docs/active/memory.decisions.md"
          ]
        },
        {
          "text": "The package comment names only the idle and turn triggers; the compaction flush is a third trigger that ignores both.",
          "refs": [
            "code:internal/memorybackground/config.go",
            "code:internal/memory/background.go#BackgroundTriggerCompaction",
            "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions"
          ]
        },
        {
          "text": "The spec says background proposals are labelled \"Found in your conversations\" in the review inbox; the web inbox has no such label and only marks a proposal whose owner message held no explicit cue.",
          "refs": [
            "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass",
            "code:internal/web/ui/src/candidateInbox/ProposalInbox.tsx",
            "code:internal/memory/proposal_inbox.go#MemoryProposalItem"
          ]
        },
        {
          "text": "Real-model proposal precision and recall are unmeasured: the package's precision probe scripts the model's mistakes and so measures only the deterministic filtering layers.",
          "refs": [
            "code:internal/memorybackground/precision_test.go#TestProposalPrecisionProbeOnAFixedCorpus",
            "doc:cmd/evie/docs/active/memory.spec.md#stage-4---durable-asynchronous-memory-compiler"
          ]
        }
      ],
      "terms": [
        "Semantic Memory",
        "Episodic Memory",
        "Context Scope",
        "Claim",
        "Predicate",
        "Typed Literal",
        "Retirement"
      ],
      "diagrams": [
        {
          "id": "memorybackground-pass-run",
          "title": "One background pass",
          "kind": "state",
          "summary": {
            "text": "The path of one pass over one conversation and the five run outcomes it can record; only a succeeded or skipped run moves the conversation's cursor.",
            "refs": [
              "code:internal/memorybackground/pass.go#Runner.runSession",
              "code:internal/memory/background.go#BackgroundRunStatus"
            ]
          },
          "states": [
            {
              "id": "due",
              "label": "due: compaction owed, 20 owner messages, or idle 10 minutes",
              "refs": [
                "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions",
                "code:internal/memorybackground/config.go#DefaultIdle",
                "code:internal/memorybackground/config.go#DefaultTurns"
              ]
            },
            {
              "id": "batch",
              "label": "batch read since the cursor",
              "refs": [
                "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundBatch",
                "code:internal/memorybackground/pass.go#Runner.runSession"
              ]
            },
            {
              "id": "proposing",
              "label": "extract, compare, decide, prepare",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.propose"
              ]
            },
            {
              "id": "committing",
              "label": "waiting for the conversation's turn lease",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.commit"
              ]
            },
            {
              "id": "succeeded",
              "label": "succeeded: proposals queued, cursor moved",
              "refs": [
                "code:internal/memory/background.go#BackgroundRunSucceeded",
                "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
              ]
            },
            {
              "id": "deferred",
              "label": "deferred: cursor kept, batch read again later",
              "refs": [
                "code:internal/memory/background.go#BackgroundRunDeferred",
                "code:internal/memorybackground/pass.go#Runner.fail"
              ]
            },
            {
              "id": "failed",
              "label": "failed: cursor kept, backoff doubles",
              "refs": [
                "code:internal/memory/background.go#BackgroundRunFailed",
                "code:internal/memorybackground/pass.go#Runner.record"
              ]
            },
            {
              "id": "skipped",
              "label": "skipped: cursor moved past a persistently malformed batch",
              "refs": [
                "code:internal/memory/background.go#BackgroundRunSkipped",
                "code:internal/memorybackground/pass.go#Runner.fail"
              ]
            },
            {
              "id": "superseded",
              "label": "superseded: another pass covered the batch; spend recorded",
              "refs": [
                "code:internal/memory/background.go#BackgroundRunSuperseded",
                "code:internal/memorybackground/pass.go#Runner.superseded"
              ]
            }
          ],
          "initial": "due",
          "transitions": [
            {
              "from": "due",
              "to": "batch",
              "label": "runner lease held and under today's caps",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.Tick",
                "code:internal/memorybackground/pass.go#Runner.capExceeded"
              ]
            },
            {
              "from": "batch",
              "to": "proposing",
              "label": "owner messages to send",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.runSession"
              ]
            },
            {
              "from": "batch",
              "to": "committing",
              "label": "only withheld messages or commands: nothing to send",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.runSession"
              ]
            },
            {
              "from": "proposing",
              "to": "committing",
              "label": "proposals prepared",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.runSession",
                "code:internal/memorybackground/pass.go#Runner.commit"
              ]
            },
            {
              "from": "proposing",
              "to": "failed",
              "label": "provider or store failure, or malformed output below the skip threshold",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.fail"
              ]
            },
            {
              "from": "proposing",
              "to": "skipped",
              "label": "third consecutive malformed reply or over the pass budget",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.fail",
                "code:internal/memorybackground/config.go#DefaultMalformedSkip"
              ]
            },
            {
              "from": "proposing",
              "to": "deferred",
              "label": "daily cap reached between calls, or shutdown after spending",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.fail"
              ]
            },
            {
              "from": "committing",
              "to": "succeeded",
              "label": "one fenced transaction",
              "refs": [
                "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
              ]
            },
            {
              "from": "committing",
              "to": "deferred",
              "label": "a live turn kept the conversation for twelve attempts",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.commit",
                "code:internal/memorybackground/config.go#DefaultCommitAttempts"
              ]
            },
            {
              "from": "committing",
              "to": "superseded",
              "label": "cursor moved: another pass committed first",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.commit",
                "code:internal/memory/background.go#ErrBackgroundCursorMoved"
              ]
            },
            {
              "from": "committing",
              "to": "failed",
              "label": "commit failed",
              "refs": [
                "code:internal/memorybackground/pass.go#Runner.commit"
              ]
            }
          ]
        }
      ],
      "links": [
        "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass",
        "doc:cmd/evie/docs/active/memory.spec.md#memory-flush-before-compaction",
        "doc:docs/memory-architecture.md#decisions-david-2026-10-03"
      ]
    },
    {
      "id": "pkg:internal/memoryeval",
      "level": "component",
      "kind": "package",
      "title": "memoryeval",
      "group": "memory",
      "summary": {
        "text": "Holds the versioned, model-free contracts of Evie's memory evaluations: the Stage 3 semantic conformance fixture and report, the seeded scale corpus and its scorer, and the offline Stage 4 release assessment; it runs no model, network call or capability itself.",
        "refs": [
          "code:internal/memoryeval/evaluation.go#Manifest",
          "code:internal/memoryeval/scale_corpus.go#GenerateScaleCorpus",
          "code:internal/memoryeval/stage4_release.go#AssessStage4Release",
          "code:internal/memoryeval/evaluation.go"
        ]
      },
      "does": [
        {
          "text": "Keeps four evaluation panels separate and in a fixed order (semantic conformance, learned extraction, retrieval provenance, answer abstention) and refuses a manifest that drops, reorders or combines them, so one score cannot hide a safety failure.",
          "refs": [
            "code:internal/memoryeval/evaluation.go#PanelOrder",
            "code:internal/memoryeval/evaluation.go#Manifest.Validate",
            "adr:0053"
          ]
        },
        {
          "text": "LoadManifest decodes a fixture manifest strictly, requires all 25 Stage 3 coverage items (scopes, corrections, lifecycle, Promotion, replay, restart, recovery and more), an isolated new database and typed evidence per case, and checks the manifest's own SHA-256.",
          "refs": [
            "code:internal/memoryeval/evaluation.go#LoadManifest",
            "code:internal/memoryeval/evaluation.go#requiredStage3Coverage",
            "code:internal/memoryeval/evaluation.go#Manifest.ContentSHA256"
          ]
        },
        {
          "text": "Defines the conformance report: run, fixture and environment identities, per-panel and per-case results under a failure taxonomy (scope leakage, replay divergence, temporal boundary error and others), and latency metrics summarized and compared with a baseline run.",
          "refs": [
            "code:internal/memoryeval/evaluation.go#Report",
            "code:internal/memoryeval/evaluation.go#FailureTaxonomy",
            "code:internal/memoryeval/evaluation.go#SummarizeMetric",
            "code:internal/memoryeval/evaluation.go#Report.ApplyBaseline",
            "code:internal/memoryeval/evaluation.go#Report.Summarize"
          ]
        },
        {
          "text": "GenerateScaleCorpus builds a seeded synthetic ten-month history (corpus memory-scale-replay-v6) of personal topics, project areas, planted facts, Claims, corrections and retirements, with gold labels and probes for Automatic Recall, memory_search and conversation search.",
          "refs": [
            "code:internal/memoryeval/scale_corpus.go#GenerateScaleCorpus",
            "code:internal/memoryeval/scale_corpus.go#ScaleCorpusVersion",
            "code:internal/memoryeval/scale_corpus.go#ScalePathAutomatic",
            "doc:cmd/evie/docs/active/memory-scale-eval.md#what-runs"
          ]
        },
        {
          "text": "ScoreScale grades the evidence each probe delivered as required, tolerated or unwanted into a versioned report (version 6), and ScaleTargetsFor lists which targets are met.",
          "refs": [
            "code:internal/memoryeval/scale_score.go#ScoreScale",
            "code:internal/memoryeval/scale_score.go#ScaleReportVersion",
            "code:internal/memoryeval/scale_score.go#ScaleTargetsFor",
            "code:internal/memoryeval/scale_score.go#ScaleClassRequired"
          ]
        },
        {
          "text": "AssessStage4Release grades a frozen Stage 4 release submission (plan, approvals, custody, execution and numeric gates) offline from metadata and hashes; VerifyStage4Evidence binds the referenced artifacts to that exact submission.",
          "refs": [
            "code:internal/memoryeval/stage4_release.go#AssessStage4Release",
            "code:internal/memoryeval/stage4_release.go#Stage4Submission",
            "code:internal/memoryeval/stage4_release_evidence.go#VerifyStage4Evidence"
          ]
        }
      ],
      "owns": [
        {
          "text": "Holds no tables and no runtime state; its inputs are fixture files and in-memory values, and its recorded scale baselines are JSON files under its testdata.",
          "refs": [
            "code:internal/memoryeval/testdata/scale-baseline-default.json",
            "code:internal/memoryeval/testdata/scale-baseline-large.json",
            "doc:cmd/evie/docs/fixtures/semantic-memory/evaluation/v1/manifest.json"
          ]
        },
        {
          "text": "Owns the version constants that keep comparisons honest: a baseline is only compared against the same corpus and report version.",
          "refs": [
            "code:internal/memoryeval/scale_corpus.go#ScaleCorpusVersion",
            "code:internal/memoryeval/scale_score.go#ScaleReportVersion"
          ]
        }
      ],
      "interfaces": [
        {
          "text": "Its exported functions are used only by tests and one script: eviedb's Stage 3 conformance test loads the checked-in manifest, the package's own scale replay drives real agent turns, the Memory Plugin and SQLite with a scripted provider, and scripts/memory-stage4-release validates release metadata.",
          "refs": [
            "code:internal/eviedb/semantic_evaluation_test.go#TestSemanticEvaluationStage3Corpus",
            "code:internal/memoryeval/scale_replay_test.go#TestMemoryScaleReplayDefaultTier",
            "code:scripts/memory-stage4-release/main.go#run"
          ]
        },
        {
          "text": "ScaleGapDenseScan mirrors the production signal memory.RetrievalGapDenseScan, and the replay test fails if the two drift apart.",
          "refs": [
            "code:internal/memoryeval/scale_score.go#ScaleGapDenseScan",
            "code:internal/memory/retrieval.go#RetrievalGapDenseScan",
            "code:internal/memoryeval/scale_replay_test.go#runScaleTier"
          ]
        }
      ],
      "gotchas": [
        {
          "text": "No production package imports it, so its contracts only run under go test and the release script, never in serve or the REPL.",
          "refs": [
            "code:internal/eviedb/semantic_evaluation_test.go#TestSemanticEvaluationStage3Corpus",
            "code:scripts/memory-stage4-release/main.go#main"
          ]
        },
        {
          "text": "A panel with no cases reports not_populated rather than passed, so a report cannot present an unexercised system as healthy.",
          "refs": [
            "code:internal/memoryeval/evaluation.go#EmptyPanels",
            "code:internal/memoryeval/evaluation.go#Report.Summarize",
            "code:internal/memoryeval/evaluation.go#StatusNotPopulated"
          ]
        },
        {
          "text": "The default scale tier is skipped under go test -short, and the large dense tier runs only with EVIE_MEMORY_SCALE_EVAL=large.",
          "refs": [
            "code:internal/memoryeval/scale_replay_test.go#TestMemoryScaleReplayDefaultTier",
            "code:internal/memoryeval/scale_replay_test.go#TestMemoryScaleReplayLargeTier"
          ]
        },
        {
          "text": "Every Stage 4 release report states its own limits: the assessment makes no model call and reads no holdout source, and retrieval and answer quality are deferred to Stage 5.",
          "refs": [
            "code:internal/memoryeval/stage4_release.go#AssessStage4Release",
            "code:scripts/memory-stage4-release/main.go"
          ]
        }
      ],
      "terms": [
        "Semantic Memory",
        "Automatic Recall",
        "Claim",
        "Promotion",
        "Scope Revision"
      ],
      "links": [
        "doc:cmd/evie/docs/active/memory-scale-eval.md",
        "adr:0053",
        "doc:cmd/evie/docs/fixtures/semantic-memory/evaluation/v1/manifest.json"
      ]
    }
  ],
  "edges": [
    {
      "from": "pkg:internal/memorybackground",
      "to": "pkg:internal/eviedb",
      "via": "call",
      "label": "reads batches and comparison pools, prepares proposals and commits each pass through Store",
      "refs": [
        "code:internal/memorybackground/pass.go#Store",
        "code:cmd/evie/memory_background.go#newMemoryBackgroundHost"
      ]
    },
    {
      "from": "pkg:internal/agent",
      "to": "pkg:internal/memorybackground",
      "via": "call",
      "label": "offers committed compactions through CompactionMemoryFlush",
      "refs": [
        "code:internal/agent/compaction_flush.go#CompactionMemoryFlush",
        "code:cmd/evie/memory_background.go#memoryBackgroundHost"
      ]
    }
  ],
  "flows": [
    {
      "id": "memory-compilation",
      "title": "Memory compilation",
      "summary": {
        "text": "The optional compiler host turns selected owner-conversation evidence into unaccepted Memory Candidates with a pinned local model, recording Compilation Coverage; nothing becomes Semantic Memory until the owner accepts a candidate in review.",
        "refs": [
          "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost",
          "code:internal/eviedb/compiler_work.go#Store.runCompilerClaim",
          "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview",
          "term:Compilation Coverage"
        ]
      },
      "trigger": {
        "text": "serve or the REPL starts with EVIE_COMPILER_CONFIG naming a pinned local extractor; work comes from activations opened before the background pass replaced them, or from an owner's `evie memory-backfill select`.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:cmd/evie/compiler_host.go#startConfiguredCompilerHost",
          "code:cmd/evie/compiler_activation.go#errCompilerActivationRetired",
          "command:evie memory-backfill select"
        ]
      },
      "participants": [
        "container:compiler-host",
        "pkg:cmd/evie",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/localextractor",
        "external:ollama",
        "person:owner",
        "container:management-cli"
      ],
      "steps": [
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/localextractor",
          "label": "Decodes the strict, size-bounded configuration and builds the loopback Ollama extractor",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/compiler_activation.go#readCompilerHostConfiguration",
            "code:internal/localextractor/ollama.go#New"
          ]
        },
        {
          "from": "container:compiler-host",
          "to": "pkg:internal/eviedb",
          "label": "Runs RunCompilerHost in its own goroutine with the extractor keyed by its Compiler Generation identity; shutdown waits at most five seconds",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:cmd/evie/compiler_host.go#startCompilerHost",
            "code:internal/memory/compiler_contract.go#CompilerGenerationIdentity",
            "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Checks for ready work without the write lock, then reconciles activation and backfill roots into sealed jobs, one bounded step at a time",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/compiler_reconciliation.go#compilerHostWork",
            "code:internal/eviedb/compiler_reconciliation.go#Store.ReconcileCompilerEvidence",
            "code:internal/eviedb/compiler_history_reconciliation.go#Store.ReconcileCompilerHistory"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Claims the next due job (staged results first, then new evidence ahead of history, with history served after eight new attempts), seals the accepted-state snapshot on the first attempt, reserves capacity and takes a 30-second fenced lease",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/compiler_worker.go#Store.RunCompilerStep",
            "code:internal/eviedb/compiler_worker.go#claimCompilerJob"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/localextractor",
          "label": "Calls Extract with a 120-second deadline while a heartbeat checks the fence every 100 ms",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/eviedb/compiler_work.go#Store.runCompilerClaim",
            "code:internal/eviedb/compiler_work.go#CompilerExtractor"
          ]
        },
        {
          "from": "pkg:internal/localextractor",
          "to": "external:ollama",
          "label": "POST /api/generate with the pinned model, template and output schema",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract"
          ]
        },
        {
          "from": "external:ollama",
          "to": "pkg:internal/localextractor",
          "label": "Raw JSON output",
          "kind": "return",
          "via": "network",
          "refs": [
            "code:internal/localextractor/ollama.go#Ollama.Extract"
          ]
        },
        {
          "from": "pkg:internal/localextractor",
          "to": "pkg:internal/eviedb",
          "label": "Raw output and completion evidence",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/eviedb/compiler_work.go#CompilerExtraction",
            "code:internal/eviedb/compiler_work.go#Store.runCompilerClaim"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/eviedb",
          "label": "Validates strictly: request binding, at most 16 candidates, offered sources only; a bad reply fails the attempt with a reason for retry",
          "kind": "note",
          "refs": [
            "code:internal/eviedb/compiler_validation.go#validateCompilerOutput",
            "code:internal/eviedb/compiler_work.go#Store.failCompilerAttempt"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Stages the sealed envelope, then publishes candidates, the candidate group and coverage and releases the job, all fenced",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/compiler_work.go#Store.stageCompilerResult",
            "code:internal/eviedb/compiler_work.go#Store.publishCompilerResult",
            "table:evie.memory_compiler_candidates",
            "table:evie.memory_compiler_coverage"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:management-cli",
          "label": "Reviews candidates with evie memory-review (or the web Review tab)",
          "kind": "call",
          "refs": [
            "code:cmd/evie/candidate_review.go#runOwnerReviewManagement",
            "command:evie memory-review prepare",
            "route:/api/memory/candidates/prepare"
          ]
        },
        {
          "from": "container:management-cli",
          "to": "pkg:internal/eviedb",
          "label": "Prepares a preview bound by hash, then resolves it",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/eviedb/candidate_review_prepare.go#Store.PrepareOwnerCandidateReview",
            "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Accepting appends an owner_candidate_review operation (schema version 6) with its Claims and Source Links",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/candidate_review_apply.go#Store.ResolveOwnerCandidateReview",
            "code:internal/memory/candidate_review.go#OwnerReviewOperation"
          ]
        }
      ]
    },
    {
      "id": "background-memory-pass",
      "title": "Background memory pass",
      "summary": {
        "text": "Once enabled (EVIE_REMOTE_MEMORY=on with OPENROUTER_API_KEY, or EVIE_COMPILER_CONFIG), the pass reads a due conversation's new owner messages, asks a pinned model what is worth remembering, compares it with existing memory, and commits exact prepared proposals to the review inbox under the conversation's turn lease; the owner's approval later applies one through the ordinary Kernel path.",
        "refs": [
          "code:internal/memorybackground/config.go#FromEnv",
          "code:internal/memorybackground/pass.go#Runner.runSession",
          "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass",
          "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal",
          "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
        ]
      },
      "trigger": {
        "text": "Every minute while serve or the REPL runs, or earlier when a committed compaction wakes the runner; a conversation is due after 10 idle minutes, 20 new owner messages, or a compaction that summarized owner messages past its cursor.",
        "refs": [
          "code:internal/memorybackground/pass.go#Runner.Run",
          "code:internal/memorybackground/pass.go#Runner.RequestCompactionFlush",
          "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions"
        ]
      },
      "participants": [
        "container:memory-background-pass",
        "pkg:cmd/evie",
        "pkg:internal/agent",
        "pkg:internal/memorybackground",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/openrouter",
        "external:openrouter",
        "person:owner",
        "container:web-ui",
        "pkg:internal/web"
      ],
      "steps": [
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/memorybackground",
          "label": "Resolves the configuration and builds the pinned model and the Runner",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/memory_background.go#newMemoryBackgroundHost",
            "code:internal/memorybackground/config.go#FromEnv",
            "code:internal/memorybackground/pass.go#NewRunner"
          ]
        },
        {
          "from": "container:memory-background-pass",
          "to": "pkg:internal/memorybackground",
          "label": "Reads the remote model's catalog price (10 seconds at most, else the fallback), then runs Runner.Run in its own goroutine until shutdown, which waits at most five seconds",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:cmd/evie/memory_background.go#memoryBackgroundHost.Start",
            "code:internal/memorybackground/pass.go#Runner.Run"
          ]
        },
        {
          "from": "pkg:cmd/evie",
          "to": "pkg:internal/agent",
          "label": "Gives every REPL and serve conversation the Runner as its compaction flush",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:cmd/evie/main.go#main",
            "code:internal/agent/compaction_flush.go#WithCompactionMemoryFlush"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/memorybackground",
          "label": "After a committed compaction, offers its frontier; this only wakes the loop",
          "kind": "async",
          "via": "call",
          "refs": [
            "code:internal/agent/compaction_flush.go#Session.requestCompactionFlush",
            "code:internal/agent/automatic_compaction.go#Session.performAutomaticCompaction",
            "code:internal/memorybackground/pass.go#Runner.RequestCompactionFlush"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Records activation once, takes the runner lease, and checks today's spend against the caps",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.Tick",
            "code:internal/memorybackground/pass.go#Runner.capExceeded",
            "code:internal/eviedb/memory_background.go#Store.EnsureMemoryBackgroundActivation",
            "code:internal/eviedb/memory_background.go#Store.AcquireMemoryBackgroundRunner"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Lists due conversations: compaction-owed first, never one held by a live turn, archived, delegated or backing off",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Reads cursors, owner messages, compaction frontiers and turn leases",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/memory_background.go#Store.DueMemoryBackgroundSessions",
            "table:evie.memory_background_cursors"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Reads the owner-message batch and the comparison pool (readable memories, pending remembers, Predicate vocabulary)",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundBatch",
            "code:internal/eviedb/memory_background.go#Store.MemoryBackgroundPool"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/openrouter",
          "label": "Extract call with the owner messages and the Predicate vocabulary: JSON output, temperature 0, low reasoning, no tools",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/memorybackground/prompt.go#extractRequest",
            "code:internal/memorybackground/models.go#OpenRouterModel.Complete",
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/openrouter",
          "to": "external:openrouter",
          "label": "Chat Completions request to the pinned model",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/openrouter",
          "to": "pkg:internal/memorybackground",
          "label": "Reply text and reported usage",
          "kind": "return",
          "via": "import",
          "refs": [
            "code:internal/memorybackground/models.go#OpenRouterModel.Complete"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/memorybackground",
          "label": "Validates and routes the candidates, ranks similar memories, settles exact duplicates and new memories without a model, and sends the rest to one decide call the same way",
          "kind": "note",
          "refs": [
            "code:internal/memorybackground/prompt.go#parseExtraction",
            "code:internal/memorybackground/similarity.go#rankSimilar",
            "code:internal/memorybackground/similarity.go#exactDuplicate",
            "code:internal/memorybackground/prompt.go#decideRequest"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Prepares each ADD, UPDATE or INVALIDATE through the approval card's Prepare paths",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.prepare",
            "code:internal/eviedb/semantic.go#Store.PrepareRememberLiteral",
            "code:internal/eviedb/semantic_correction.go#Store.PrepareCorrectClaim",
            "code:internal/eviedb/semantic_lifecycle.go#Store.PrepareMemoryLifecycle"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Takes the conversation's turn lease for 10 seconds, retrying while a live turn holds it",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.commit",
            "code:internal/eviedb/turn_leases.go#Store.AcquireTurnLease"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Commits the pass",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.commit",
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "One lease-fenced transaction: inbox rows (duplicates and declined ones skipped), the run record, and the cursor compare-and-set",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/memory_background.go#Store.CommitMemoryBackgroundPass",
            "table:evie.memory_proposal_inbox",
            "table:evie.memory_background_runs"
          ]
        },
        {
          "from": "pkg:internal/memorybackground",
          "to": "pkg:internal/eviedb",
          "label": "Releases the turn lease",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/memorybackground/pass.go#Runner.commit",
            "code:internal/eviedb/turn_leases.go#Store.ReleaseTurnLease"
          ]
        },
        {
          "from": "person:owner",
          "to": "container:web-ui",
          "label": "Approves a background proposal in the Memory review inbox, with the \"Always applies\" switch as set",
          "kind": "call",
          "refs": [
            "code:internal/web/ui/src/candidateInbox/ProposalInbox.tsx"
          ]
        },
        {
          "from": "container:web-ui",
          "to": "pkg:internal/web",
          "label": "POST /api/memory/proposals/approve with the listed prepared hash",
          "kind": "call",
          "via": "http",
          "refs": [
            "code:internal/web/ui/src/api/memoryProposals.ts",
            "route:/api/memory/proposals/approve",
            "code:internal/web/memory_proposals.go#Server.handleMemoryProposalApprove"
          ]
        },
        {
          "from": "pkg:internal/web",
          "to": "pkg:internal/eviedb",
          "label": "ApproveMemoryProposal through the MemoryProposalInbox interface",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/web/memory_proposals.go#MemoryProposalInbox",
            "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "Takes the source conversation's lease, refreshes a stale proposal, appends the approval event, then runs the ordinary Apply",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/memory_proposal_inbox.go#Store.ApproveMemoryProposal",
            "code:internal/eviedb/semantic.go#Store.ApplyRememberLiteral"
          ]
        }
      ]
    },
    {
      "id": "automatic-recall",
      "title": "Automatic Recall",
      "summary": {
        "text": "At the start of a turn the agent plans a small query from the owner's message and recent topics, asks the Kernel in the session's own scopes for two accepted memories and two conversation excerpts, plus two accepted memories for each of up to two exact selectors in the message (an ID, or a token like task-12), so up to six accepted-memory results and two excerpts; it revalidates them and sends them as one framed data message just before the active request.",
        "refs": [
          "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic",
          "code:internal/agent/retrieval_automatic.go#planAutomaticRecall",
          "code:internal/agent/retrieval.go#retrievalTurn.renderProjection",
          "term:Automatic Recall"
        ]
      },
      "trigger": {
        "text": "The first model request of each turn, unless the session turned Automatic Recall off; it needs EVIE_REMOTE_MEMORY=on and a composition with memory_search or memory_search_conversations.",
        "refs": [
          "code:internal/agent/turn.go#Session.runOwnedTurn",
          "code:internal/agent/agent.go#WithAutomaticMemoryRecall",
          "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic"
        ]
      },
      "participants": [
        "container:retrieval-maintenance",
        "pkg:internal/agent",
        "pkg:internal/eviedb",
        "container:evie-db",
        "pkg:internal/localembedding",
        "external:ollama",
        "pkg:internal/openrouter",
        "external:openrouter"
      ],
      "steps": [
        {
          "from": "container:retrieval-maintenance",
          "to": "pkg:internal/eviedb",
          "label": "Keeps the full-text and dense indexes caught up, independently of turns",
          "kind": "async",
          "via": "call",
          "refs": [
            "code:cmd/evie/memory_retrieval_host.go#startMemoryRetrievalHost",
            "code:internal/eviedb/retrieval_conversation_index.go#Store.RefreshMemoryIndex"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/agent",
          "label": "Plans the query: the current message, up to two relevant earlier messages from the last 16, the summary for a follow-up, and any explicit past time, plus up to two exact selectors (a UUID, or a token of letters, digits and a separator); an acknowledgement searches nothing",
          "kind": "note",
          "refs": [
            "code:internal/agent/retrieval_automatic.go#planAutomaticRecall",
            "code:internal/agent/retrieval_automatic.go#explicitRecallSelectors",
            "code:internal/memory/recall_time.go#FindRecallTimeReference"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "SearchMemory for accepted memory: two results, 6 KiB, scope taken from the session; then once per exact selector, two results each",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic",
            "code:internal/agent/retrieval.go#retrievalTurn.search",
            "code:internal/agent/retrieval.go#MemoryRetrieval"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/localembedding",
          "label": "Embeds the query when dense retrieval is configured and the text holds no secret",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/eviedb/retrieval_dense_query.go#prepareDenseQuery",
            "code:internal/localembedding/client.go#Client.Embed"
          ]
        },
        {
          "from": "pkg:internal/localembedding",
          "to": "external:ollama",
          "label": "POST /api/embed with the pinned embedding model",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/localembedding/client.go#Client.Embed"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "container:evie-db",
          "label": "One read-only transaction: exact and Alias, full-text, valid-during, temporal, dense and graph candidates fused by reciprocal rank, then the relevance gate",
          "kind": "call",
          "via": "sql",
          "refs": [
            "code:internal/eviedb/retrieval.go#Store.SearchMemory",
            "code:internal/eviedb/retrieval_graph.go#Store.acceptedRetrievalCandidates",
            "code:internal/eviedb/retrieval_graph.go#addRetrievalRank",
            "code:internal/eviedb/retrieval_claim_relevance.go#newClaimRelevanceGate"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/agent",
          "label": "Sourced evidence and a status that tells empty from failed or partial",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/memory/retrieval.go#RetrievalResult",
            "code:internal/agent/retrieval.go#retrievalTurn.search"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "SearchMemory for conversation excerpts from earlier conversations of the same Context Scope",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/retrieval_automatic.go#retrievalTurn.automatic",
            "code:internal/eviedb/retrieval_conversation.go#Store.searchConversations",
            "code:internal/eviedb/retrieval_conversation.go#Store.readConversationPass"
          ]
        },
        {
          "from": "pkg:internal/eviedb",
          "to": "pkg:internal/agent",
          "label": "Excerpts that cleared the relevance floor, skipping what the request already shows",
          "kind": "return",
          "via": "call",
          "refs": [
            "code:internal/eviedb/retrieval_conversation.go#Store.searchConversations"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Revalidates the held evidence just before composing; anything no longer current is withheld",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/retrieval.go#retrievalTurn.projection",
            "code:internal/eviedb/retrieval.go#Store.RevalidateMemoryEvidence"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/agent",
          "label": "Renders one EVIE_MEMORY_DATA user message within the turn's 36 KiB memory budget, places it before the active root, and drops whole items if the request lacks headroom",
          "kind": "note",
          "refs": [
            "code:internal/agent/retrieval.go#retrievalTurn.renderProjection",
            "code:internal/agent/retrieval.go#retrievalTurnBytes",
            "code:internal/agent/retrieval_context.go#retrievalTurn.fitContext",
            "code:internal/agent/context.go#ContextComposer.projectAtStart"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/eviedb",
          "label": "Stores the context_snapshot with a content-free retrieval receipt of the evidence references",
          "kind": "call",
          "via": "call",
          "refs": [
            "code:internal/agent/turn.go#Session.runOwnedTurn",
            "code:internal/memory/event.go#ContextSnapshotPayload",
            "code:internal/memory/retrieval_receipt.go#RetrievalReceipt"
          ]
        },
        {
          "from": "pkg:internal/agent",
          "to": "pkg:internal/openrouter",
          "label": "Sends the request carrying the memory data",
          "kind": "call",
          "via": "import",
          "refs": [
            "code:internal/agent/turn.go#Session.callProvider",
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        },
        {
          "from": "pkg:internal/openrouter",
          "to": "external:openrouter",
          "label": "Chat request over HTTPS",
          "kind": "call",
          "via": "network",
          "refs": [
            "code:internal/openrouter/client.go#Client.ChatStream"
          ]
        }
      ]
    }
  ]
}
);
