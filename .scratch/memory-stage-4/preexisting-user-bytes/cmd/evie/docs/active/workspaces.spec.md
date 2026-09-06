## Problem Statement

Evie currently understands global sessions and filesystem-project sessions.
That model works for coding, but an ongoing area such as Cairo's Kitchen is not
a repository and should not be flattened into global memory or one chat. It has
its own knowledge, approved workflows, external accounts, business resources,
configuration, presets, and retention expectations.

Without a first-class Workspace, Evie would either leak information between
unrelated parts of the owner's life or rely on the model to infer scope from
conversation text. Neither is an acceptable access boundary. The owner needs an
explicit, durable way to enter Cairo's Kitchen and know exactly which memory,
connections, resources, and capabilities are available there.

## Solution

Add Workspace as a durable Context Scope alongside filesystem project and no
Context Scope. A session is explicitly created in exactly one Workspace,
project, or neither, and the selection cannot change during the session. A
reviewed Workspace Revision defines its default and allowed Agent Presets,
allowed Connection IDs and resources, reviewed business configuration, and
retention policy.

Workspace Access is a ceiling, not authority: a workflow still needs a narrower
approved Standing Authority, and a preset only exposes capabilities. Sessions
pin the Workspace Revision used at creation, while later access removals take
effect immediately as safety revocations. Workspace memory remains isolated,
and promotion to global memory is explicit.

## User Stories

1. As Evie's owner, I want Cairo's Kitchen represented as a Workspace, so that its operating context is durable without pretending it is a code project.
2. As Evie's owner, I want a Workspace to have a stable identity independent of its display name, so that renaming it does not break sessions, memory, or workflows.
3. As Evie's owner, I want Workspace configuration reviewed and versioned, so that changes to its access boundary and behavior are inspectable and reversible.
4. As Evie's owner, I want to create a session explicitly inside a Workspace, so that Evie does not guess access from my words.
5. As Evie's owner, I want sessions started outside a Workspace to remain outside it, so that mentioning Cairo's Kitchen cannot silently expose restaurant data.
6. As Evie's owner, I want Evie to suggest opening a relevant Workspace session when useful, so that explicit isolation does not make navigation cumbersome.
7. As Evie's owner, I want a session assigned one Workspace, one filesystem project, or neither, so that unrelated scoped data cannot be combined accidentally.
8. As Evie's owner, I want the Context Scope fixed for a session's lifetime, so that a conversation cannot change its data boundary midway through history.
9. As Evie's owner, I want every Workspace session to retain normal session memory, so that conversational continuity remains local to that session.
10. As Evie's owner, I want genuinely owner-wide global memory available in a Workspace, so that stable preferences do not need to be duplicated.
11. As Evie's owner, I want other Workspaces' and projects' memory excluded, so that Cairo's Kitchen cannot retrieve unrelated personal or coding information.
12. As Evie's owner, I want new durable memories created in Cairo to default to the Cairo Workspace, so that local facts do not become globally visible.
13. As Evie's owner, I want global-memory promotion to be explicit, so that a Cairo-specific fact cannot silently become owner-wide knowledge.
14. As Evie's owner, I want a Workspace to declare one default Agent Preset, so that starting a Cairo session automatically receives the intended capabilities.
15. As Evie's owner, I want a Workspace to declare all allowed Agent Presets, so that unrelated or overly powerful compositions cannot appear there accidentally.
16. As Evie's owner, I want to select a non-default allowed preset when creating a session, so that the Workspace can support more than one deliberate mode.
17. As Evie's owner, I want a disallowed preset rejected rather than partially applied, so that Workspace capability boundaries remain understandable.
18. As Evie's owner, I want a session to pin its Workspace Revision and Agent Preset version, so that ordinary configuration edits do not change a live conversation.
19. As Evie's owner, I want changes to a default preset to affect new sessions only, so that existing sessions remain reproducible.
20. As Evie's owner, I want Workspace Access to list allowed Account Connections and external resources, so that Cairo's Kitchen has an explicit outer data boundary.
21. As Evie's owner, I want one Account Connection usable from multiple Workspaces when appropriate, so that I do not repeat the same login unnecessarily.
22. As Evie's owner, I want each Workspace to restrict the resources reachable through a shared connection, so that sharing a Google login does not share every spreadsheet.
23. As Evie's owner, I want removing a shared connection from Cairo to leave other Workspaces connected, so that revocation remains local.
24. As Evie's owner, I want adding Workspace Access to require reviewed configuration, so that new data reach is explicit.
25. As Evie's owner, I want removing Workspace Access to take effect immediately, so that a safety revocation is not delayed by pinned revisions.
26. As Evie's owner, I want affected Workflow Runs paused before their next operation after revocation, so that an old approval cannot bypass the new ceiling.
27. As Evie's owner, I want a Workspace's access boundary separated from Standing Authority, so that an available account does not authorize every workflow action.
28. As Evie's owner, I want presets separated from authority, so that showing Square tools to the model does not authorize Square mutations.
29. As Evie's owner, I want reviewed business configuration stored separately from credentials and run state, so that each kind of information receives the right durability and secrecy rules.
30. As Evie's owner, I want a behavior-affecting configuration change to create a reviewed workflow version, so that mutable Workspace data cannot silently alter approved automation.
31. As Evie's owner, I want credential rotation behind the same Connection ID to preserve approved references, so that routine token refresh does not create needless reviews.
32. As Evie's owner, I want switching accounts or resources to require review, so that a stable label cannot be used to redirect approved behavior.
33. As Evie's owner, I want Workspace retention and export rules, so that sensitive restaurant records have explicit lifecycle expectations.
34. As Evie's owner, I want Workspace configuration and access changes audited, so that I can see who changed what and which sessions or runs were affected.
35. As Evie's owner, I want generic CLI and web management for Workspaces, so that I can create, inspect, revise, archive, and enter them without editing internal storage.
36. As a maintainer, I want Workspace session creation to extend the existing durable scope model, so that project and Workspace safety use the same proven transaction boundary.
37. As a maintainer, I want the Workspace domain separated from plugins, so that reusable Cairo code is not confused with the owner's actual restaurant data and accounts.

## Implementation Decisions

- Workspace is a first-class Context Scope. It is not a Feature Plugin, Agent
  Preset, filesystem project, session, or Workflow Definition.
- A Workspace has a random stable ID, display name, lifecycle state, creation
  and update timestamps, and a current reviewed Workspace Revision.
- Each session has exactly one of: a Workspace reference and revision snapshot,
  a project reference and root snapshot, or neither. The database enforces the
  exclusivity rather than relying on callers to populate fields correctly.
- Every session still has global and session scope. A Workspace session may
  access global, its one Workspace, and its one session scope; it may not access
  project or other-Workspace scope.
- Context Scope is selected explicitly at session creation. Intent recognition
  may produce a suggestion to open a new session but cannot alter the current
  session or grant scope.
- The current Workspace Revision is a reviewed procedural asset. Its canonical
  content identifies the default Agent Preset, allowed Agent Presets, allowed
  Connection IDs and resource bounds, non-secret reviewed business
  configuration, and retention/export policy.
- Workspace revisions use the procedural proposal, validation, diff, approval,
  canonical hash, Git history, and rollback model. Credentials never appear in
  the procedural repository.
- Session creation validates the requested preset against the selected
  Workspace Revision and pins the exact Workspace and preset identities in the
  session's durable context and Composition Receipt.
- A Workspace preset allowlist constrains capability exposure. Workspace Access
  independently constrains accounts, resources, and memory. Standing Authority
  on an approved Workflow Definition must be a subset of Workspace Access.
- An access addition applies through a reviewed new Workspace Revision. An
  access removal is also persisted as a new revision but is enforced immediately
  as a safety revocation against existing sessions and Workflow Runs.
- Pinned Workspace configuration remains the reproducibility source for ordinary
  behavior, but the effective access set is the intersection of that revision
  and every later revocation.
- One Kernel-owned Account Connection may be referenced by several Workspaces.
  Workspace Access binds that Connection ID to explicit resource selectors.
  Removing one reference never deletes or disconnects the shared connection.
- Credential refresh and rotation that preserve the external account identity
  occur behind the same Connection ID. Changing account identity, Connection
  ID, or a bounded external resource is a reviewed access change.
- Reviewed configuration that affects a Workflow Definition is copied or
  referenced by immutable content identity in that definition. An approved run
  never reads an unpinned mutable value that can change its calculations,
  recipients, resources, schedule, or authority.
- The Kernel's secret storage owns credential material. Procedural Git owns
  reviewed Workspace configuration. SQLite owns Workspace indexes, sessions,
  memory, audit events, operational state, and Workflow Runs.
- New semantic memories default to the active Context Scope. Moving a memory
  from Workspace to global uses an explicit promotion operation with provenance;
  retrieval never widens scope.
- Generic Workspace management is Kernel-owned. Plugins do not inject custom
  Workspace administration UI in the first implementation.
- Archive and hard-deletion behavior are not conflated. Archive prevents new
  sessions and scheduled runs while retaining history. Any later hard erasure
  must obey the memory and workflow retention policies and cannot strand active
  or Outcome Unknown work.
- The primary testing seam is durable session/scope creation and resume through
  the existing store behavior, with a thin acceptance test through the current
  session chooser or equivalent web management flow.
- Implementation is split into independently reviewable slices: Workspace
  identity and persistence, reviewed revisions, session creation and preset
  validation, memory-scope integration, Connection ID/resource bounds,
  revocation propagation, and management surfaces.

## Testing Decisions

- Good tests assert observable scope, access, and session behavior through the
  session/scope store and public management flow. They do not test SQL statement
  text, private structs, or frontend rendering details.
- Real SQLite tests create global, project, and Workspace sessions and prove the
  database accepts exactly one Context Scope, freezes the correct revision, and
  reconstructs it after process reopen.
- Existing project registration, relocation, chooser race, archive, and session
  resume tests are prior art for stable identity, atomic selection, stale-view
  rejection, and immutable snapshots.
- Scope-matrix tests prove a Workspace session can retrieve global, Workspace,
  and its own session memory while excluding every project, sibling Workspace,
  and unrelated session.
- Promotion tests prove new memories default to Workspace and only an explicit
  operation can create a linked global claim.
- Preset tests prove the default is selected, an allowed override works, a
  disallowed preset fails, missing required capabilities fail, and existing
  sessions retain their pinned revision after ordinary edits.
- Access tests use fake shared Account Connections and resource selectors to
  prove independent per-Workspace bounds, no credential duplication, local
  removal, and no cross-Workspace disconnection.
- Revocation race tests prove a previously pinned session or Workflow Run cannot
  begin a newly forbidden action after the revocation commits.
- Configuration tests prove secrets are rejected, behavior-affecting values are
  pinned into workflow review, and unpinned edits cannot change an approved run.
- CLI and HTTP acceptance tests cover create, list, inspect, revise, archive,
  explicit session entry, suggested-but-not-automatic entry, and actionable
  validation failures.
- Persistence tests reopen the procedural repository and SQLite database to
  prove reviewed revision identity, audit history, and operational references
  remain consistent without making Git and SQLite pretend to share a
  transaction.
- The full repository verification command must pass after each implementation
  slice.

## Out of Scope

- Sessions combining more than one Workspace or combining a Workspace with a
  filesystem project.
- Automatic Workspace attachment based on model inference or conversation text.
- Nested Workspaces, Workspace inheritance, teams, multiple owners, or
  role-based multi-user access.
- Reviewed project-to-Workspace links.
- Custom UI supplied by plugins.
- Hard-erasure semantics beyond preserving an explicit future seam.
- Implementing Cairo's Kitchen business rules, Square access, Google Sheets
  operations, payment sending, or tip calculations.
- Granting Standing Authority merely because a connection or preset is allowed.

## Further Notes

Cairo's Kitchen is the motivating Workspace and later vertical slice. The
separation is intentional: a Cairo Feature Plugin may contribute reusable
language, deterministic capabilities, and workflow templates, while the Cairo
Workspace contains the owner's actual memory, accounts, resources,
configuration, approved definitions, and run history.

Workspace adds a new memory scope and therefore depends on the focused memory
model amendment. Workspace preset behavior also depends on Plugin System Phase
1. Those dependencies do not authorize merging the modules into one
implementation change.

Dependencies and related specifications: Plugin System Phase 1 is issue #70,
the Workflow Runtime is issue #72, and the memory-model amendment is issue #73.
