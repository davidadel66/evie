## Parent

[Subagents: Preset-composed foreground research with Kernel supervision (#155)](https://github.com/davidadel66/evie/issues/155)

## What to build

Make the immutable research Agent Preset resolve, execute a scripted research request, and reopen with exactly the same capability composition. This is the focused prefactoring outcome that removes implicit built-in inheritance for a restricted session while preserving current primary-session behavior.

Use the existing Plugin Manager, Composition Receipt, and conversation interfaces. This ticket does not register the Subagents Plugin in production, expose delegation in the standard preset, or convert unrelated built-ins into Plugins.

## Acceptance criteria

- [ ] The built-in research preset selects only the existing Web search and fetch Capability Contracts. The complete executable capability set and Composition Receipt agree; no hidden built-in execution abilities are appended.
- [ ] Through a composed scripted session, approved Web research succeeds while invented invocations of filesystem, shell, database, scheduling, finance, Todo, Memory, or delegation capabilities are rejected by execution, not merely absent from model schemas.
- [ ] Missing required research capabilities and incompatible contracts reject composition with an actionable explanation and no fallback to standard or another preset.
- [ ] Persisting and reopening the research session reconstructs its exact pinned capability and instruction-reference identities. Unsupported or incompatible pinned compositions fail visibly.
- [ ] Existing standard-preset versions and persisted primary-session receipts reconstruct unchanged. The new restricted-base selection does not change ordinary primary-session execution.
- [ ] The research preset is available to trusted composition and normal preset inspection without making a partially implemented delegation capability available to production parents.
- [ ] Acceptance uses the existing composed conversation with deterministic model/Web execution and real temporary SQLite. Supporting checks cover preset resolution, dispatch rejection, and exact receipt reopening.
- [ ] Run focused composition/conversation checks and the full repository verification required for code changes; document any changed behavior and its demonstration.

## Blocked by

None (can start immediately).
