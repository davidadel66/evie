# Evie

Evie is a personal agent runtime that selects and uses installed capabilities
to help its owner while keeping runtime lifecycle and consequential actions
explicitly distinguishable.

## Language

**Plugin**:
An installed extension that contributes one or more capabilities to Evie.
_Avoid_: Tool, skill

**First-party Plugin**:
A plugin shipped with Evie or developed as part of the owner's local Evie
environment, rather than obtained from an outside publisher.
_Avoid_: Third-party plugin

**Capability**:
A named ability Evie can use to fulfill a request. A capability may include
tools, instructions, connectors, or workflows supplied by a plugin.
_Avoid_: Plugin, tool

**Capability ID**:
The canonical plugin-namespaced identity of a Capability, used by presets and
workflows without relying on display names or plugin load order.
_Avoid_: Tool label, capability schema

**Capability Contract**:
The versioned behavioral and schema boundary a Capability promises to its
consumers independently of the implementation version of its provider.
_Avoid_: Plugin version, tool description

**Task**:
A durable, owner-visible unit of intended work that may be decomposed and
updated by the owner or an authorized agent. Evie may create a top-level Task
when durable tracking is useful; incidental agent planning is not a Task.
_Avoid_: Todo item, transient agent plan, workflow run

**Subtask**:
A Task whose parent is another Task in the same scope. Subtasks use the same
model as Tasks and may themselves have Subtasks.
_Avoid_: Checklist text, agent run, separate subtask type

**Task Focus**:
The one explicitly selected Task whose relevant open descendants are supplied
to a session as working context. Focus does not itself grant Task access.
_Avoid_: Entire backlog, inferred task, Task grant

**Task Tree**:
One top-level Task and its recursively nested Subtasks. Evie creates top-level
Task Trees; authorized subagents may contribute beneath a granted root.
_Avoid_: Flat task list, project, workflow run

**Task Access Grant**:
Bounded authority for a session or subagent to read, contribute to, or manage
one Task and all its current and future descendants, without revealing its
ancestors or siblings. Effective authority is the intersection of the grant
and the agent's available capabilities; Task Focus is not a grant. In the
initial model, only Evie issues grants and each grant lasts for its authorized
subagent session or run rather than becoming a standing access-control entry.
_Avoid_: Agent preset, Task Focus, prompt context

**Task Claim**:
A session-scoped indication that an agent is actively working on one Task. A
claim remains valid while its owning session or run is active and is released
when that execution ends or recovery finds it is no longer running. It
coordinates concurrent work without replacing the agent-run record or
permanently locking the Task. Progress, result, and completion updates require
an active claim; child creation and metadata corrections do not.
_Avoid_: Task ownership, access grant, agent execution

**Capability Provider**:
A plugin role that contributes one specific family of capabilities through its
own focused interface, such as tools, models, sandboxes, or subagents.
_Avoid_: Plugin, universal provider

**Connector Plugin**:
A plugin that contributes reusable capabilities backed by a particular
external system, independently of any one personal or business workflow.
_Avoid_: Feature plugin

**Feature Plugin**:
A plugin that contributes the language, rules, and procedures for a particular
area of the owner's life or work by composing reusable capabilities.
_Avoid_: Connector plugin, workflow

**Workspace**:
A durable, owner-defined scope for an ongoing area of life or work. It groups
that area's memory, Procedural Workflows, allowed Account Connections, business
configuration, and default Agent Preset without pretending the area is a code
project or a plugin.
_Avoid_: Filesystem project, session, feature plugin

**Context Scope**:
The one durable area, either a Workspace or a filesystem project, whose scoped
memory and configuration a session may use in addition to global and session
memory.
_Avoid_: Agent preset, account connection

**Working Memory**:
The disposable, request-specific projection Evie assembles for a model call
from selected history, summaries, instructions, and retrieved knowledge.
_Avoid_: Working context, event history

**Episodic Memory**:
The immutable chronological evidence of sessions, turns, tool activity, and
their recorded outcomes.
_Avoid_: Working memory, semantic memory

**Semantic Memory**:
Accepted source-linked entities, temporal claims, and relationships that
represent what Evie knows without replacing their Episodic Memory evidence.
_Avoid_: Event history, procedural memory, objective truth

**Entity**:
A stable scope-aware identity for a person, organization, place, object, or
concept referenced by Semantic Memory.
_Avoid_: Name, alias, claim

**Context Entity**:
The canonical scope-local Entity that represents one registered Workspace,
project, or session inside Semantic Memory.
_Avoid_: Context Scope, duplicate project entity, global promotion

**Alias**:
An explicitly accepted name or identifier for an Entity that does not merge
identities merely because their text is equal.
_Avoid_: Entity, display name, fuzzy match

**Claim**:
An immutable accepted proposition relating a subject Entity to another Entity
or typed literal together with scope, provenance, temporal meaning, and
lifecycle history.
_Avoid_: Event, objective fact, graph link

**Unsupported Claim**:
An accepted Claim with no eligible active Source Link. It remains historical but
is excluded from normal current retrieval until eligible support is restored.
_Avoid_: Retired claim, deleted claim, rejected candidate

**Graph Link**:
An explicitly accepted typed relationship between semantic objects for
structure or provenance when the relationship is not an ordinary Claim.
_Avoid_: Claim, inferred similarity, cross-scope import

**Predicate**:
The canonical validated relationship token of a Claim, kept distinct from its
human-readable wording and from automatically inferred synonyms.
_Avoid_: Display label, free-form relation text

**Claim Polarity**:
Whether an exact Claim proposition is affirmed or denied. Missing knowledge is
unknown rather than an implicit denial.
_Avoid_: Predicate negation, confidence, absence

**Predicate Cardinality**:
Whether a Predicate normally relates one subject to one or many simultaneous
objects. It identifies potential conflicts without deleting conflicting Claims.
_Avoid_: Database uniqueness, Claim Polarity

**Typed Literal**:
A non-Entity Claim object represented by one closed value kind with canonical
encoding and equality rules.
_Avoid_: Arbitrary JSON, floating-point value, entity

**Scope Revision**:
A monotonic version of accepted Semantic Memory within one scope used for
ordering, concurrency checks, and reproducible projections.
_Avoid_: Workspace revision, database timestamp

**Semantic Operation**:
An accepted append-only change that creates or transitions Semantic Memory
state together with its provenance.
_Avoid_: Event, extraction candidate, workflow run

**Memory Operation Proposal**:
An unaccepted typed Semantic Memory change prepared in direct response to an
explicit owner request and shown for approval before it becomes an operation.
_Avoid_: Semantic operation, automatically extracted candidate

**Memory Candidate**:
A source-linked interpretation proposed from episodic evidence that has not
been accepted into Semantic Memory. It is excluded from ordinary graph
traversal and retrieval until an approved rule emits a Semantic Operation.
_Avoid_: Claim, remembered fact, Memory Operation Proposal

**Compiler Generation**:
One fixed extraction configuration under which Episodic Memory produces Memory
Candidates, kept distinct from accepted Semantic Memory and other generations.
_Avoid_: Semantic revision, model name, extraction attempt

**Compilation Coverage**:
The record of which selected episodic ranges a Compiler Generation has processed
and which gaps remain. Its contiguous frontier does not cross an unresolved gap.
_Avoid_: Accepted memory, candidate count, all retained history

**Valid Time**:
The interval in the represented world during which a Claim applies.
_Avoid_: Recorded time, acceptance time

**Transaction Time**:
The time at which Evie accepted a Claim or one of its lifecycle transitions.
_Avoid_: Valid time, event time

**Retirement**:
A reversible lifecycle transition that excludes eligible knowledge from normal
current retrieval while preserving its evidence and history.
_Avoid_: Hard erasure, supersession

**Hard Erasure**:
Permanent coordinated removal of information and its retained representations
across Evie's memory layers and projections.
_Avoid_: Retirement, ordinary deletion

**Promotion**:
An accepted Semantic Operation that creates broader-scoped knowledge linked to
its narrower-scoped evidence while leaving the original knowledge unchanged.
_Avoid_: Scope mutation, cross-scope import

**Procedural Memory**:
Reviewed and versioned definitions and instructions for how Evie should work,
kept separate from factual knowledge and changing execution state.
_Avoid_: Semantic memory, workflow run

**Memory Capability**:
A session-bound ability exposed through plugin composition for inspecting,
querying, or explicitly changing memory without owning memory truth or scope.
_Avoid_: Memory store, memory provider

**Memory Inspection**:
A read-only scoped view of entities, Claims, provenance, temporal history, and
lifecycle state intended for owner understanding rather than model retrieval.
_Avoid_: Retrieval, direct database access, graph export

**Workspace Revision**:
One reviewed and versioned state of a Workspace's configuration and access
boundary. Sessions pin the revision from which they started, subject to later
safety revocations.
_Avoid_: Workflow definition, memory snapshot

**Workspace Access**:
The outer boundary of Account Connections, resources, and memory that a
Workspace permits its sessions and workflows to reference. It does not itself
authorize an action.
_Avoid_: Standing authority, account connection

**Skill**:
Reviewed instructions and resources loaded into the model's working context for
the model to interpret. A skill does not own durable execution state.
_Avoid_: Workflow definition

**Procedural Workflow**:
Reviewed repeatable behavior represented by a Workflow Definition and performed
through Workflow Runs.
_Avoid_: Skill, ad hoc tool sequence

**Workflow Definition**:
An approved and versioned declarative graph describing the state, steps, and
transitions of a Procedural Workflow together with its requested Standing
Authority. A proposal is not a definition until the owner accepts it.
_Avoid_: Workflow run, skill

**Workflow Approval**:
The owner's acceptance of one exact Workflow Definition version and its
requested Standing Authority for future runs.
_Avoid_: Action approval, account connection

**Standing Authority**:
The bounded operations, accounts, resources, schedules, recipients, and limits
that an approved Workflow Definition may exercise across runs without repeated
Action Approval.
_Avoid_: Account connection, unrestricted access

**Workflow Run**:
One durable execution of a pinned Workflow Definition, including its current
state, checkpoints, interruptions, attempts, and outcomes.
_Avoid_: Session, workflow definition

**Successor Run**:
A new Workflow Run created by an explicit migration from an unfinished older
run, linked to its predecessor and carrying only validated state and durable
effect evidence.
_Avoid_: Rewritten run, retry

**Execution Composition**:
The exact capability providers, versions, Connection IDs, schemas, and
non-secret configuration pinned for one Workflow Run independently of any
session's Agent Preset.
_Avoid_: Agent preset, standing authority

**Compatibility Resolution**:
A durable record that a newer provider implementation may continue work pinned
to an older, unchanged Capability Contract, including the exact replacement
version and evidence used.
_Avoid_: Silent upgrade, workflow migration

**Resource Conflict Key**:
A stable identity declared by an external-effect capability so Evie can prevent
concurrent writes that could interfere with the same external resource.
_Avoid_: Run key, effect idempotency key

**Retry Policy**:
The bounded, node-specific rules governing whether and how a failed attempt may
run again. Outcome Unknown is never retryable by policy.
_Avoid_: Recovery, replay

**Effect Intent**:
A durable declaration recorded before a Workflow Run attempts an external
effect, including the stable identity needed to reconcile or deduplicate it.
_Avoid_: Effect receipt, planned action

**Effect Receipt**:
Durable evidence of the accepted response or observed outcome of an external
effect.
_Avoid_: Effect intent, checkpoint

**Outcome Unknown**:
A Workflow Run state in which an Effect Intent exists but Evie cannot yet prove
whether the external effect occurred.
_Avoid_: Failed, retryable

**AI Node**:
A workflow step that uses a model to produce typed proposed data for later
deterministic validation. It cannot directly perform an external effect.
_Avoid_: Agent, capability call

**Model Policy**:
A named and versioned mapping from an AI Node to an exact model provider,
model, parameters, and output schema. A Workflow Run pins the policy version it
uses.
_Avoid_: Agent preset, model preference

**Run Key**:
A deterministic, workflow-specific identity for the logical execution that
Evie uses to prevent duplicate manual or scheduled starts.
_Avoid_: Workflow run ID, effect idempotency key

**Workflow Schedule**:
A reviewed trigger included in a Workflow Definition that may create Workflow
Runs according to its declared timing and catch-up policy.
_Avoid_: Background run, cron entry

**Catch-up Policy**:
The reviewed rule for whether and how a Workflow Schedule creates runs for
occurrences missed while Evie was unavailable.
_Avoid_: Retry policy, run deduplication

**Foreground Run**:
A Workflow Run whose progress and interaction are presented through an active
session.
_Avoid_: Chat session workflow

**Background Run**:
A Workflow Run that advances independently of an active session and can report
progress or outcomes back to one.
_Avoid_: Cron job, detached session

**Durable Notification**:
A persisted, deduplicated notice that a Workflow Run completed, failed, or
needs attention. External channels may mirror it but are not its source of
truth.
_Avoid_: Workflow event, transient message

**Plugin Manager**:
The trusted part of Evie that discovers enabled plugins and manages their
loading, dependencies, health, and cleanup.
_Avoid_: Agent, approver

**Plugin Manifest**:
A plugin's declarative identity, version, Kernel compatibility, dependencies,
and contributed Capability Contracts.
_Avoid_: Agent preset, plugin configuration

**Plugin Health**:
Whether a plugin's code and required dependencies loaded and operate correctly.
It is independent of whether an external account is connected.
_Avoid_: Connection readiness

**Plugin Lifecycle**:
The reversible progression through which the Plugin Manager enables, waits for,
loads, operates, fails, and stops a plugin and its contributions.
_Avoid_: Workflow run

**Connection Readiness**:
Whether the Account Connections and external resources required for a
capability are currently usable.
_Avoid_: Plugin health

**Composition Receipt**:
A durable, content-free identity of the exact Agent Preset, plugins,
capabilities, schemas, instructions, and non-secret configuration references a
session received.
_Avoid_: Preset name, credentials

**Enabled Plugin**:
An installed plugin that the owner has made eligible for automatic loading.
_Avoid_: Active plugin

**Loaded Plugin**:
An enabled plugin whose contributions are registered with Evie's Plugin
Manager. Loading does not mean its capabilities are visible to the model.
_Avoid_: Active plugin, selected capability

**Agent Preset**:
A named and versioned composition of plugins and capabilities assigned when a
session is created. The session remains pinned to that exact approved version,
and the preset does not grant authority to execute consequential actions.
_Avoid_: Profile, session type

**Preset Validity**:
Whether an Agent Preset's required plugins and capabilities can be resolved.
It is independent of whether external accounts are connected.
_Avoid_: Connection readiness, plugin health

**Kernel**:
The non-removable part of Evie that supervises plugins and preserves the
runtime's safety, authority, durability, and recovery guarantees.
_Avoid_: Core plugin

**Account Connection**:
The established credentials and consent that allow a capability to access an
external account.
_Avoid_: Plugin loading, action approval

**Connection ID**:
An opaque non-secret reference to one Account Connection. Presets, workflows,
and receipts may contain the reference but never the credentials it represents.
_Avoid_: Access token, account credential

**Action Approval**:
The owner's explicit permission for a particular consequential action to
execute when Standing Authority does not cover it or the Workflow Definition
requires an interruption. It is independent of whether the providing plugin is
loaded.
_Avoid_: Plugin approval
