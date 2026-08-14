# F01 - Project registry and scoped sessions

Status: candidate, unapproved. Priority: first.

## Purpose

Keep EVIE centralized while allowing each conversation to be either global or
immutably bound to one registered project. Project-aware tools resolve paths
from that session's scope, never from where the EVIE process was launched.

## How OpenCode does it

OpenCode creates an instance context containing the project, selected directory,
and worktree. Project-aware services consume that context instead of reading a
package-global cwd. File tools distinguish paths inside the worktree from
external directories and request separate permission for external access.

OpenCode's newer location-mutation code resolves existing ancestors and
canonical paths before authorizing a mutation. Its older shipping file boundary
is less consistent around Unix symlinks, which is a warning not to copy lexical
prefix checks alone.

Sources:

- [`project/instance-context.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/project/instance-context.ts)
- [`core/location-mutation.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/core/src/location-mutation.ts#L84-L149)

## EVIE today

- EVIE starts as one centralized assistant rather than a per-repository CLI.
- `Session` has no project or session ID.
- `read_file` and `edit_file` resolve relative paths from EVIE's process cwd.
- Bash maintains a separate package-global persistent cwd.
- A Bash `cd` can therefore make shell and file tools disagree about the same
  relative path.
- Multiple sessions or children would share Bash cwd state.
- `memory.spec.md` already designs an immutable canonical `ScopeContext`, but it
  is not implemented and currently governs memory scope rather than all tools.

## Product model

EVIE owns a durable project registry:

```text
EVIE
|- Global conversations
`- Registered projects
   |- evie
   |  `- Project conversations
   `- another-project
      `- Project conversations
```

Opening EVIE never silently selects a repository. A new conversation starts in
`global` scope unless David explicitly selects a registered project. Selecting a
project creates or resumes a project-scoped session; it does not mutate the
scope of the current conversation.

A global session has no project root, project instructions, project memory, Git
state, or coding profile. It retains EVIE's personal-assistant tools and global
memory. When a global request names a registered project or a path inside one,
EVIE offers to continue in that project's session rather than quietly acquiring
project authority.

A project session receives global memory plus only that project's instructions,
memory, Git state, cwd, and coding capabilities. It cannot switch project scope
mid-conversation. Cross-project work requires another session or an explicitly
configured read-only reference under F29.

## Project registry

Registering a project is an explicit operation that:

- accepts an existing directory selected by David;
- resolves a canonical absolute root and rejects duplicate canonical roots;
- assigns a durable random project ID that does not expose or derive from the
  path;
- records a user-facing name, canonical root, trust state, and timestamps;
- detects Git metadata for context but accepts non-Git projects; and
- never scans parent directories or imports the launch cwd automatically.

The random ID survives a deliberate project relocation. Relocation is an
explicit registry operation: it validates the new canonical root and preserves
the old path as audit history. Existing sessions retain their original root
snapshot and become unavailable/stale if that path disappears; a move does not
silently redirect an old session's filesystem authority.

Removing a project from the active picker archives the registry entry. It does
not delete sessions, events, memory, snapshots, or source files. Destructive
forget/delete semantics remain separate.

## Session scope

Resolve an immutable `ScopeContext` once when a session is created:

```text
session ID
scope kind: global | project
optional durable project ID and display name
optional canonical project-root snapshot
optional session-owned cwd, initially the project root
optional Git worktree/common-repository identity
project trust state at session creation
```

Global sessions carry no project fields. In project sessions, all relative
project tools and Bash start from the session-owned cwd. `cd` updates cwd but
never changes the immutable project root or project ID.

Project scope and filesystem confinement are separate concepts. Default coding
operations should target the project root. An absolute path outside it should be
classified as external and follow an explicit allow/ask/deny policy rather than
silently becoming part of the project.

For a new path, canonicalize the nearest existing ancestor, verify it is within
the authorized root, then recheck immediately before mutation. This closes the
gap where the final file does not yet exist and `EvalSymlinks` cannot resolve it.

## First slice

- Durable register/list/archive project operations.
- `global` or one registered project per session.
- Global is the default; launch cwd and `EVIE_PROJECT_ROOT` never imply scope.
- Explicit project selection before creating/resuming a project session.
- Canonical absolute root with symlink resolution.
- Durable random project ID plus unique canonical root.
- Session-owned cwd shared consistently by Bash and file tools.
- Project-relative path rendering in model results.
- External paths identified but not necessarily forbidden; F05 decides policy.

## Not in the first slice

- Multi-root workspaces.
- Automatic repository switching after `cd`.
- Automatic project registration or launch-directory detection.
- Remote repositories or containers.
- Filesystem sandbox claims.

## Acceptance evidence

- Starting EVIE from inside a repository still creates a global session unless a
  project is explicitly selected.
- Global sessions contain no project instructions, project memory, or project
  coding authority.
- Selecting a different project creates/resumes a different session rather than
  changing the current session's scope.
- Bash, read, and edit resolve the same relative path after `cd`.
- Two sessions can hold different cwd values without interference.
- Two registered projects cannot resolve to the same canonical root.
- Archiving a project preserves its sessions and memory.
- A symlink or `..` path cannot disguise an external mutation as project-local.
- Deleting the remembered cwd produces an explicit fallback/error, not a scope
  change.
- Project identity remains unchanged when cwd changes.

## Open decisions

1. Is external access default `ask` for project coding profiles while EVIE's
   global personal-assistant scope remains broad?
2. What explicit UI/API confirms project trust during registration?
3. Can an archived project be reactivated at a different root, or must relocation
   happen before archival?
