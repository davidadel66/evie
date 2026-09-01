## Problem Statement

Evie's capabilities are currently wired into one static tool registry. Adding,
removing, grouping, or replacing a capability requires editing the harness, and
every session sees essentially the same tool surface. That makes it difficult to
grow Evie into a personal runtime that can support focused areas such as coding
and Cairo's Kitchen without sending every tool schema on every request or
letting feature-specific code take ownership of safety, credentials, history,
and recovery.

The owner needs a first-party plugin system that makes Evie modular and
inspectable while preserving a small trusted Kernel. Sessions need explicit,
versioned Agent Presets instead of inferred session types or model-selected
plugin activation. The first implementation must prove the architecture with
capabilities Evie already owns, without accepting untrusted executable code or
prematurely turning every subsystem into a plugin.

## Solution

Add a Plugin Manager to the Kernel. It discovers first-party Go plugins compiled
into Evie, validates their Plugin Manifests and dependencies, starts and stops
them reversibly, and gathers their focused Capability Provider contributions.
The first production provider role is tools. Existing web and finance
capabilities become the proof plugins.

Add immutable built-in Agent Presets, including `standard`, and reviewed
user-authored presets. A session explicitly receives one preset at creation and
persists a Composition Receipt for the exact resolved capabilities. The model
does not activate plugins, plugins do not grant authority, and an unavailable
required capability fails visibly instead of changing the composition
silently.

The initial implementation establishes concrete later seams for model, memory,
sandbox, subagent, workflow-integration, and UI-extension providers, but it does
not implement those provider roles yet.

## User Stories

1. As Evie's owner, I want capabilities grouped into first-party plugins, so that Evie can grow without every feature being wired directly into the harness.
2. As Evie's owner, I want a small non-removable Kernel, so that plugins cannot replace the safety, authority, durability, or recovery rules supervising them.
3. As Evie's owner, I want plugins to be automatically loaded after I enable them, so that ordinary sessions do not repeatedly ask me for plugin activation permission.
4. As Evie's owner, I want plugin loading to remain separate from Action Approval, so that making code available does not authorize consequential actions.
5. As Evie's owner, I want first-party compiled plugins initially, so that I can validate the composition model before accepting externally distributed executable code.
6. As Evie's owner, I want to enable and disable an already-compiled plugin without rebuilding Evie, so that I can change the available system deliberately.
7. As Evie's owner, I want new or modified plugin code to require a rebuild and restart, so that Evie does not hot-swap executable code inside a running process.
8. As Evie's owner, I want every plugin to have a stable identity and version, so that sessions and diagnostics can name the implementation they used.
9. As Evie's owner, I want every Capability to have a canonical namespaced Capability ID, so that similarly named tools from different plugins never collide.
10. As Evie's owner, I want Capability Contracts versioned independently from plugin implementations, so that compatible maintenance releases do not invalidate every consumer.
11. As Evie's owner, I want incompatible Kernel, plugin, dependency, and Capability Contract versions rejected before use, so that Evie fails closed instead of improvising.
12. As Evie's owner, I want required and optional plugin dependencies distinguished, so that required failures block unsafe partial compositions while optional omissions remain visible.
13. As Evie's owner, I want dependency cycles rejected with a useful explanation, so that startup does not hang or choose an arbitrary load order.
14. As Evie's owner, I want failed plugin initialization cleaned up completely, so that partial registrations cannot leak into later sessions.
15. As Evie's owner, I want plugin lifecycle states visible, so that I can distinguish disabled, waiting, loading, ready, failed, stopping, and stopped plugins.
16. As Evie's owner, I want Plugin Health separated from Connection Readiness, so that broken code and an unconnected external account are diagnosed differently.
17. As Evie's owner, I want Evie to start in a degraded state when an optional plugin fails, so that one extension does not make the Kernel and management surfaces unavailable.
18. As Evie's owner, I want affected presets and dependents blocked when a required plugin fails, so that a session never starts with a silently reduced composition.
19. As Evie's owner, I want a `standard` built-in Agent Preset, so that sessions created outside a Workspace or project have a predictable default composition.
20. As Evie's owner, I want each session assigned exactly one Agent Preset at creation, so that its capability set is stable and understandable.
21. As Evie's owner, I want preset changes to affect new sessions only, so that editing a preset cannot change a live conversation unexpectedly.
22. As Evie's owner, I want no mid-session preset switching initially, so that history, instructions, and tool availability remain reproducible.
23. As Evie's owner, I want built-in presets to be immutable and impossible to shadow, so that a user file cannot silently replace Evie's known defaults.
24. As Evie's owner, I want user-authored presets reviewed and versioned in procedural Git, so that I can inspect, approve, diff, and roll back composition changes.
25. As Evie's owner, I want a session to pin the exact approved preset content, so that later edits or name reuse cannot change what a resumed session means.
26. As Evie's owner, I want missing required preset capabilities to prevent session creation with a repair message, so that Evie never silently falls back to `standard`.
27. As Evie's owner, I want missing optional preset capabilities shown as warnings, so that useful reduced behavior is explicit.
28. As Evie's owner, I want a preset with a missing Account Connection to remain usable, so that a healthy session can guide me through connecting the account.
29. As Evie's owner, I want every session to retain a Composition Receipt, so that I can audit the preset, plugins, capabilities, schemas, instructions, and non-secret configuration it received.
30. As Evie's owner, I want credentials excluded from presets and receipts, so that reviewed configuration and history never become secret stores.
31. As Evie's owner, I want provider choice explicit rather than determined by load order, so that two implementations of a similar capability cannot change behavior accidentally.
32. As Evie's owner, I want all tool schemas in a session's fixed preset supplied to the model, so that tool availability is simple and deterministic initially.
33. As Evie's owner, I want only compact skill summaries supplied automatically, so that full reviewed instructions consume context only when the model needs them.
34. As Evie's owner, I want a compatible replacement provider recorded separately from the original Composition Receipt, so that audit history names the code that actually resumed the session.
35. As Evie's owner, I want disabling a plugin to block new dependent work, so that an intentional shutdown takes effect immediately.
36. As Evie's owner, I want in-flight external work reconciled before a plugin stops, so that disabling a plugin cannot pretend an already-sent effect was cancelled.
37. As a plugin author, I want one small lifecycle interface and focused Capability Provider interfaces, so that a plugin implements only the roles it contributes.
38. As a plugin author, I want interfaces owned by the consuming Kernel module, so that provider implementations do not dictate Evie's orchestration design.
39. As a maintainer, I want current web and finance behavior preserved through plugins, so that the first architecture slice proves composition without changing user-visible results.
40. As a maintainer, I want later provider roles named but unimplemented, so that future model, memory, sandbox, subagent, workflow, and UI work has deliberate seams without speculative machinery.

## Implementation Decisions

- The Kernel remains ordinary trusted code. It owns session and event history,
  the Plugin Manager, credential mediation, approval and Standing Authority
  enforcement, and durable recovery. These are not first-phase plugins.
- The first implementation accepts only First-party Plugins shipped with Evie
  or developed in the owner's local source tree. Plugins are Go modules compiled
  into the Evie executable.
- Adding or changing plugin code requires rebuilding and restarting Evie. The
  Plugin Manager may start and stop compiled plugins at runtime; no Go dynamic
  plugin loading or executable-package hot reload is introduced.
- Every Plugin Manifest declares a stable plugin ID, implementation version,
  supported Kernel interface version, required and optional dependency ranges,
  and contributed Capability Contracts.
- Duplicate plugin IDs and duplicate Capability IDs are errors. Filesystem
  order, registration order, and first-loaded wins are never resolution rules.
- The Plugin Manager owns discovery, dependency validation, topological load
  order, cycle rejection, lifecycle transitions, registration rollback,
  status inspection, and reverse-order cleanup.
- Plugin Lifecycle uses the explicit states `disabled`, `waiting`, `loading`,
  `ready`, `failed`, `stopping`, and `stopped`. State changes and failures are
  observable without exposing credentials.
- Required dependency failure blocks the dependent plugin. Missing optional
  dependencies produce warnings and omit only the declared optional
  contributions.
- Plugin Health describes whether code and required dependencies work.
  Connection Readiness describes whether required accounts and external
  resources are usable. The two states are stored and displayed separately.
- The common plugin lifecycle interface is intentionally small: identity and
  manifest access plus start and stop behavior. Capability contribution uses
  focused consumer-owned interfaces rather than one universal provider
  interface.
- Phase 1 implements the tool-provider role and defines versioned interface
  points for later roles without implementing their behavior.
- The current web search/fetch tools and finance tools become separate proof
  plugins. Their existing externally observable schemas, execution behavior,
  approval behavior, and results remain unchanged unless this spec explicitly
  says otherwise.
- The primary composition seam is the Plugin Manager resolving an Agent Preset
  into one immutable session toolset and Composition Receipt. The agent
  conversation module consumes that resolved toolset instead of consulting a
  process-global static list.
- An Agent Preset is a named, versioned list of required and optional
  Capability IDs plus any reviewed skill catalog entries and non-secret
  configuration references needed to compose a session.
- Evie ships immutable built-in presets. `standard` is the default for a
  session with no Context Scope. Built-ins have reserved identities and cannot
  be shadowed by user presets.
- User-authored presets are reviewed procedural assets stored in procedural
  Git. Approval activates one canonical content hash. New sessions resolve the
  approved version; existing sessions retain their pinned version.
- Preset creation and resolution validate required plugin and Capability
  availability. There is no automatic fallback to another preset. Missing
  optional contributions are preserved as diagnostics.
- A missing Account Connection does not affect Preset Validity. The resolved
  tool may report that connection setup is required, and Connection Readiness
  remains inspectable.
- Every session persists an immutable Composition Receipt containing the preset
  ID and content version, Evie version, exact plugin IDs and versions,
  Capability IDs and contract versions, tool schema hashes, instruction hashes,
  and non-secret configuration references.
- Every conversational model request initially receives all tool schemas in
  the pinned Agent Preset. It receives only a compact name-and-summary catalog
  for skills and loads a full skill body on demand.
- Provider implementations are selected explicitly by canonical identity and
  compatible contract. Later Model Policies and other consumers use the same
  rule.
- If the exact implementation in a Composition Receipt is unavailable, resume
  is allowed only when the replacement explicitly declares compatibility with
  every pinned Capability Contract and schema. Evie appends a Compatibility
  Resolution naming the exact replacement; it never rewrites the original
  receipt.
- Disabling a plugin blocks new dependent sessions and Workflow Runs. Already
  dispatched external requests finish or enter reconciliation before the
  plugin stops, and dependent runs pause before their next covered operation.
- The Kernel and generic plugin/preset management surfaces remain available in
  degraded startup. Only failure of the Kernel itself prevents Evie from
  starting.
- Generic CLI and web management expose installed/enabled state, dependencies,
  health, Connection Readiness, presets, validation errors, and receipts. Phase
  1 does not permit plugins to inject custom management UI.
- The implementation is divided into independently reviewable slices even
  though this document defines the whole Phase 1 behavior: manifest and manager,
  tool-provider extraction, lifecycle and dependencies, built-in presets,
  receipts and resume compatibility, reviewed user presets, and management
  surfaces.

## Testing Decisions

- Good tests assert behavior through the Plugin Manager's composition interface
  and the existing agent conversation interface. They do not assert private
  registry maps, internal load-loop calls, or a particular package layout.
- The primary seam uses tiny deterministic fake compiled plugins with manifests,
  dependency graphs, lifecycle hooks, and tool contributions. The same seam
  exercises validation, resolution, cleanup, and diagnostics.
- One high-level agent acceptance suite sends scripted model turns through a
  resolved preset and proves that the request contains exactly the preset's
  schemas, calls route to the selected provider, absent tools remain unknown,
  and tool-result ordering remains unchanged.
- Lifecycle tests cover disabled-to-ready, live stop and restart, partial-start
  rollback, required-dependency failure, optional-dependency warning, dependency
  cycles, duplicate IDs, cancellation, and reverse-order cleanup.
- Preset tests cover immutable built-ins, reserved-name rejection, canonical
  hashing, reviewed user versions, missing required and optional capabilities,
  lack of fallback, connection-not-ready behavior, and changes affecting only
  new sessions.
- Composition Receipt tests use real SQLite and process reopen to prove exact
  round trips, absence of credentials, immutable original receipts, and audited
  Compatibility Resolutions.
- Compatibility tests distinguish plugin implementation versions from
  Capability Contract versions and reject undeclared substitutions, changed
  schemas, changed contracts, and unsupported Kernel versions.
- Degraded-startup tests prove that the Kernel and management inspection work
  when an optional plugin fails and that only affected presets and dependents
  are blocked.
- Existing tool registry and agent tests are prior art for schema selection,
  unknown-tool behavior, approvals, cancellation, and result ordering. Existing
  SQLite reopen tests are prior art for receipt durability.
- Existing web and finance behavior receives regression coverage before and
  after extraction into proof plugins.
- The full repository verification command must pass after every independently
  reviewable implementation slice.

## Out of Scope

- Third-party plugin installation, remote marketplaces, signatures, publisher
  trust, or untrusted executable code.
- Runtime loading of new plugin code, Go dynamic plugins, WebAssembly plugins,
  or an external-process plugin protocol.
- Converting the Kernel, event history, approvals, credentials, or durable
  recovery into plugins.
- Model-provider, memory-provider, sandbox-provider, subagent-provider,
  workflow-integration, or UI-extension implementations.
- Dynamic model-directed plugin activation, per-request provider discovery, or
  mid-session Agent Preset switching.
- On-demand tool-schema routing within an already selected preset.
- Square, Google Workspace, payment, or Cairo's Kitchen implementations.
- A public compatibility promise for independently distributed plugins.

## Further Notes

This is the first half of the plugin architecture. The second half is
deliberately preserved as a future set of concrete provider seams: model
providers consumed by Model Policies, memory providers consumed by context
composition, one coherent sandbox/execution provider consumed by tools and
subagents, subagent providers consumed by orchestration, workflow integration
consumed by the Workflow Runtime, and UI extensions consumed by generic
frontend extension points. Each seam should be implemented only when it has a
real consumer and at least two meaningful adapters.

DeepSeek Harness is the architectural reference for explicit composition and
fixed presets, not a behavior to copy blindly. Evie improves on its mutable
preset-ID persistence by reviewing user presets, pinning content identities,
and recording complete non-secret Composition Receipts.

Related specifications: Workspace scope is tracked in issue #71, the Workflow
Runtime in issue #72, and the required memory-model amendment in issue #73.
