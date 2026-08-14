# F29 - Project reference repositories

Status: deferred, unapproved.

## Purpose

Give a development session read-only access to explicitly named sibling
directories or external repositories that provide useful source and
documentation without making them part of the writable project.

## How OpenCode does it

OpenCode references can point to a local absolute path or a Git repository with
an optional branch, description, and hidden flag. Described references are
listed in the environment prompt with their materialized paths. Reference
directories are allowed through the external-directory policy for agents that
can use them.

Remote Git references are cloned into a shared cache. Refresh fetches and hard
resets the tracked checkout, so the source describes this as "newest wins" and a
reader can observe it moving underneath a session. A branch can be selected,
but the configuration does not pin one immutable commit.

Sources:

- [`core/reference.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/core/src/reference.ts)
- [`schema/reference.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/schema/src/reference.ts)
- [`core/repository-cache.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/core/src/repository-cache.ts)
- [`session/system.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/system.ts#L67-L102)

## EVIE assessment

Defer until EVIE repeatedly needs cross-repository context. F01 should remain a
single writable project root; references are a separate read-only capability,
not implicit workspace expansion.

## Proposed EVIE adaptation

Start with explicitly configured local references:

- canonical absolute path, stable name, description, and trust/provenance;
- read/search only through a reference-specific capability;
- never load reference `AGENTS.md`/`CLAUDE.md` as governing instructions;
- label every result with reference name and path;
- do not let project-relative writes resolve into a reference.

Remote Git references are a later stage. Prefer an explicitly approved fetch and
an immutable commit captured for the session, not a checkout that refreshes
under active readers. Cache under a user-only directory, record remote URL,
requested branch, resolved commit, fetch time, and content provenance. Network
access and credentials need the normal permission/secret policy.

Reference content is untrusted data. A referenced repository cannot grant tools,
change profiles, install dependencies, or become executable merely by being
available for search.

## Acceptance evidence

- Project writes and Bash policy cannot silently treat a reference as writable.
- Reference instructions remain data and never override project/EVIE rules.
- A session reads one recorded commit even if a remote branch advances.
- Cross-project/reference access is explicit in tool calls and context snapshots.
- Cached files and repository metadata use user-only permissions and cleanup
  never follows paths outside the cache root.

## Open decisions

1. What first real project needs a reference rather than ordinary web/Git CLI
   research?
2. Should local references also freeze a snapshot/hash for reproducibility?
3. Are private Git remotes allowed, and how are credentials kept out of events?
