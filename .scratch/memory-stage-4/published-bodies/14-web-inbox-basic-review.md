## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Expose the already-supported simple candidate inbox and exact accept/reject flow through HTTP and the web UI. This is a complete narrow owner journey over the shared Kernel contract, with bounded listing and source inspection after the originating conversation closes.

## Acceptance criteria

- [ ] Require explicit applicable scope and show bounded/paginated candidate state and detail with exact source, original authority, generation and reviewed effects.
- [ ] Let the owner accept or reject a supported simple candidate after the source session closes, using the same typed owner authority and exact preview binding as CLI review.
- [ ] Preserve current origin, approval and request protections. Invalid scope, hidden source, stale revision or resolved candidate states receive the same Kernel outcome as other surfaces.
- [ ] Refresh state after successful resolution and show actionable stale/conflict/error states without silently accepting changed effects or losing owner intent.
- [ ] Keep unaccepted suggestions separate from normal accepted Semantic Memory views. Accepted detail and provenance match the reviewed exact projection.
- [ ] Verify the browser interaction, HTTP adapter and persisted/reopened outcome against the CLI/Kernel contract, including stale responses and scope changes.
- [ ] Run focused HTTP/frontend and real-SQLite acceptance tests and repository-required full change verification. Defer editing and compound effects to the advanced web-review slice.

## Blocked by

- [Accept or reject a candidate after its conversation closes](https://github.com/davidadel66/evie/issues/140)

